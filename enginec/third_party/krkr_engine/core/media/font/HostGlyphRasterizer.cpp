#include "tjsCommHead.h"

#include "HostGlyphRasterizer.h"

#include "CharacterData.h"
#include "PlatformTextHook.h"
#include "TVPFontstruc.h"

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstddef>
#include <cstdint>
#include <cstdlib>
#include <memory>
#include <mutex>
#include <optional>
#include <span>
#include <string>
#include <unordered_map>
#include <vector>

// Defined in core/render/LayerBitmap.cpp (no public header). The engine caches the
// first bitmap it sees per (font, glyph) in its font cache, so a FreeType fallback
// taken while the host is still warming up would stick for the rest of the game.
// The host callback is synchronous but the platform rasterizer behind it is not:
// a miss is a request that is answered later. Dropping the engine cache a little
// after the request makes the glyph be asked for again, so the host bitmap is
// picked up. The refresh is bounded per glyph (see the retry budget below), so a
// run the host can never render costs one extra rasterization, not an endless
// cache-thrash loop.
extern void TVPClearFontCache();

namespace {

// A broken host reply must not make the engine allocate arbitrary amounts of
// memory; the text stack is asked for one glyph at a time.
constexpr int max_bitmap_side = 4096;
constexpr tjs_int max_font_size = 512;

// Flutter lays a single line out with ascent/(ascent+descent) of roughly 0.8 for
// every common system font (Roboto 0.79, Noto CJK 0.80, DejaVu 0.80). The hook
// reports only width/height, so this ratio is what maps the bitmap's top edge
// onto the baseline the engine uses for its own ascent line.
constexpr double line_ascent_ratio = 0.8;

// The glyph cache handoff. The engine caches the first bitmap it sees per
// (font, glyph) in LayerBitmap.cpp's font cache, while the host answers a miss
// asynchronously (it requests rasterization and returns false for now). Without
// dropping that cache the FreeType fallback of the first frame would stick for
// the rest of the game and the host path would never be used. The trade-offs:
//   * a refresh clears the whole engine cache, so it is rate limited to give the
//     host time to answer and to keep the re-rasterization cost bounded;
//   * only glyphs whose request is old enough are retried, so a miss that is
//     still in flight does not burn its retries;
//   * a glyph the host can never render ends up on FreeType after its budget;
//   * after several refreshes without a single host-served glyph the handoff
//     stops, so a disabled/hung host never thrashes the cache;
//   * the bookkeeping map is bounded so a pathological game cannot grow it.
constexpr std::size_t max_tracked_glyphs = 8192;
constexpr unsigned max_refresh_cycles = 2;
constexpr unsigned max_refreshes_without_hit = 3;
constexpr auto refresh_delay = std::chrono::milliseconds(1000);

struct refresh_state
{
    struct pending_glyph
    {
        unsigned left;                                // refreshes still allowed
        std::chrono::steady_clock::time_point since;  // when the host missed it
    };

    std::mutex mutex;
    // Kept after the budget reaches 0 so an immediate re-miss does not re-arm a
    // glyph the host cannot render. Erased on a hit.
    std::unordered_map<std::uint64_t, pending_glyph> pending;
    std::chrono::steady_clock::time_point last_refresh{};
    unsigned refreshes_without_hit = 0;
};

refresh_state& refreshes()
{
    static refresh_state state;
    return state;
}

std::uint64_t glyph_key(const tTVPFont& font, tjs_wchar ch)
{
    std::uint64_t hash = 1469598103934665603ull; // FNV-1a
    const auto mix = [&hash](std::uint64_t value)
    {
        for (int i = 0; i < 8; ++i)
        {
            hash ^= (value >> (i * 8)) & 0xff;
            hash *= 1099511628211ull;
        }
    };
    for (const char* p = font.Face.c_str(); p != nullptr && *p != '\0'; ++p)
    {
        hash ^= static_cast<unsigned char>(*p);
        hash *= 1099511628211ull;
    }
    mix(static_cast<std::uint64_t>(static_cast<std::uint32_t>(font.Height)));
    mix(font.Flags);
    mix(ch);
    return hash;
}

// Registers a glyph the host could not serve yet; a later refresh re-asks it.
void schedule_glyph_refresh(std::uint64_t key)
{
    refresh_state& state = refreshes();
    std::lock_guard<std::mutex> lock(state.mutex);
    if (state.pending.size() >= max_tracked_glyphs)
        state.pending.clear(); // bound the bookkeeping, not the rendering
    if (state.pending.find(key) != state.pending.end())
        return; // already scheduled; keep its original timestamp and budget
    state.pending.emplace(key, refresh_state::pending_glyph{max_refresh_cycles,
                                                            std::chrono::steady_clock::now()});
    // Start the delay at the first miss: otherwise every miss in the same frame
    // would see an "old" timestamp and clear the cache once per glyph.
    if (state.last_refresh == std::chrono::steady_clock::time_point{})
        state.last_refresh = std::chrono::steady_clock::now();
}

// The host answered: the handoff works (and this glyph needs no retry any more).
void note_host_hit(std::uint64_t key)
{
    refresh_state& state = refreshes();
    std::lock_guard<std::mutex> lock(state.mutex);
    state.pending.erase(key);
    state.refreshes_without_hit = 0;
}

// Decides (and records) whether the engine glyph cache should be dropped. Split
// out so the actual clear runs outside the bookkeeping lock: it walks and releases
// the whole cache and must not hold up other canvases asking for glyphs.
bool take_refresh_slot()
{
    refresh_state& state = refreshes();
    std::lock_guard<std::mutex> lock(state.mutex);
    const auto now = std::chrono::steady_clock::now();
    if (state.pending.empty() || now - state.last_refresh < refresh_delay)
        return false;

    bool retry_left = false;
    for (auto& [key, entry] : state.pending)
    {
        (void)key;
        if (entry.left == 0 || now - entry.since < refresh_delay)
            continue;
        --entry.left;
        retry_left = true;
    }
    if (!retry_left)
        return false;

    // Several refreshes in a row without a single host bitmap mean the host is
    // not answering (uninstalled after the fact, broken backend): give up rather
    // than clearing the engine cache forever.
    if (++state.refreshes_without_hit > max_refreshes_without_hit)
    {
        state.pending.clear();
        return false;
    }
    state.last_refresh = now;
    return true;
}

// Called before asking the host: drops the engine's glyph cache once the host had
// time to answer the previous requests, so those glyphs get asked for again.
void maybe_refresh_glyph_cache()
{
    if (take_refresh_slot())
        TVPClearFontCache();
}

// Ink bounds inside the host bitmap (inclusive, in bitmap pixels).
struct ink_box
{
    int left;
    int top;
    int right;
    int bottom;

    int width() const { return right - left + 1; }
    int height() const { return bottom - top + 1; }
};

struct malloc_free
{
    void operator()(std::uint8_t* p) const { std::free(p); }
};

// Font family as the host's text stack wants it: UTF-8 (ttstr is UTF-8 in this
// build), without the '@' vertical-writing marker and without the comma
// separated fallback list the engine keeps internally.
std::string host_family_name(const ttstr& face)
{
    const char* name = face.c_str();
    if (name == nullptr)
        return {};
    if (*name == '@')
        ++name;
    std::string family(name);
    if (const auto comma = family.find(','); comma != std::string::npos)
        family.erase(comma);
    return family;
}

// The host returns the whole line box (baseline padding included); the engine's
// glyph cache and per-draw textures should hold ink instead. Alpha is the
// coverage: the host composites premultiplied RGBA, so alpha is exactly the
// anti-aliased mask whatever colour was requested. Empty (space and friends) is
// reported as an empty optional, and the pen advance is kept by the caller.
std::optional<ink_box> find_ink(std::span<const std::uint8_t> rgba, int width, int height, int pitch)
{
    ink_box box{width, height, -1, -1};
    for (int y = 0; y < height; ++y)
    {
        const std::uint8_t* row = rgba.data() + static_cast<std::size_t>(y) * pitch;
        for (int x = 0; x < width; ++x)
        {
            if (row[static_cast<std::size_t>(x) * 4 + 3] == 0)
                continue;
            box.left = std::min(box.left, x);
            box.right = std::max(box.right, x);
            box.top = std::min(box.top, y);
            box.bottom = std::max(box.bottom, y);
        }
    }
    if (box.right < box.left)
        return std::nullopt;
    return box;
}

} // namespace

tTVPCharacterData* TVPGetHostGlyph(const tTVPFont& font,
                                   tjs_wchar ch,
                                   bool antialiased,
                                   tjs_int ascent,
                                   tjs_int advance)
{
    if (TVPTextRenderHook == nullptr)
        return nullptr;

    const tjs_int size = font.Height < 0 ? -font.Height : font.Height;
    if (size <= 0 || size > max_font_size)
        return nullptr;
    // The engine hands over one UTF-16 code unit at a time, so a character
    // outside the BMP arrives as a lone surrogate that no text stack can
    // rasterise on its own; keep those on FreeType like before.
    if (ch >= 0xD800 && ch <= 0xDFFF)
        return nullptr;

    const std::string family = host_family_name(font.Face);
    const std::uint64_t key = glyph_key(font, ch);
    // Drop the engine's stale FreeType glyphs so those missed last time get asked
    // again now that the host had time to answer (see refresh_state above).
    maybe_refresh_glyph_cache();
    // Opaque white: the engine tints the coverage mask with the text colour at
    // draw time (InternalBlendText -> ApplyColorMap), so asking the host for a
    // coloured bitmap would apply the colour twice. White also keeps the host's
    // per-colour cache at one entry per glyph.
    char16_t run_text[1] = {static_cast<char16_t>(ch)};
    TVPTextRun run{};
    run.text = run_text;
    run.length = 1;
    run.family = family.c_str();
    run.size = static_cast<float>(size);
    run.color = 0xFFFFFFFFu;
    // Keep FreeType's synthetic styles: FT_GlyphSlot_Embolden/Oblique in the
    // engine's own path, the matching FontWeight/FontStyle in the host's.
    run.flags = font.Flags & (TVP_TF_BOLD | TVP_TF_ITALIC);

    TVPTextBitmap bitmap{};
    bool rendered = false;
    try
    {
        rendered = TVPTextRenderHook(run, &bitmap);
    }
    catch (...)
    {
        rendered = false; // a host callback must never take the renderer down
    }
    // Free whatever the host handed over, even on a bogus reply.
    const std::unique_ptr<std::uint8_t, malloc_free> pixels(bitmap.pixels);
    if (!rendered)
    {
        // The host did not have this glyph cached yet (it answers asynchronously);
        // schedule a glyph-cache refresh so the next draw asks again.
        schedule_glyph_refresh(key);
        return nullptr;
    }
    if (pixels == nullptr || bitmap.width <= 0 || bitmap.height <= 0 ||
        bitmap.width > max_bitmap_side || bitmap.height > max_bitmap_side ||
        bitmap.pitch < bitmap.width * 4 || bitmap.pitch > max_bitmap_side * 4)
        return nullptr;
    note_host_hit(key);

    const int width = bitmap.width;
    const int height = bitmap.height;
    const int pitch = bitmap.pitch;
    const std::span<const std::uint8_t> rgba(pixels.get(),
                                             static_cast<std::size_t>(pitch) * height);

    const std::optional<ink_box> ink = find_ink(rgba, width, height, pitch);

    // The engine measures text with its own glyph metrics (GetTextExtent), so the
    // drawn advance must come from the same place or centred/wrapped lines drift.
    // The host advance is only the fallback when the face has no metric for ch.
    tGlyphMetrics metrics{};
    metrics.CellIncX = advance > 0 ? advance
                                   : (bitmap.advance > 0 ? bitmap.advance
                                                        : (ink ? ink->width() : 0));
    metrics.CellIncY = 0;

    // The bitmap's top edge sits on the engine's ascent line. Use the host's exact
    // top->baseline distance when it reported one: the old fixed ratio (0.8 of the
    // line box) is a different number for nearly every font and shifted text up or
    // down by a pixel or two. A zero/insane value keeps the old estimate so an
    // older host DLL still works.
    tjs_int baseline = bitmap.baseline;
    if (baseline <= 0 || baseline > height)
        baseline = static_cast<tjs_int>(std::lround(height * line_ascent_ratio));
    const tjs_int origin_y0 = ascent > 0 ? ascent - baseline : 0;

    tTVPCharacterData* data = nullptr;
    if (!ink)
    {
        // Fully transparent run (space and friends): keep the advance, draw nothing.
        data = new tTVPCharacterData(nullptr, 0, 0, origin_y0, 0, 0, metrics);
        data->Pitch = 0;
        data->Gray = 256;
        data->FullColored = false;
        return data;
    }

    const ink_box& box = *ink;
    std::vector<std::uint8_t> coverage(static_cast<std::size_t>(box.width()) * box.height());
    for (int y = 0; y < box.height(); ++y)
    {
        const std::uint8_t* row = rgba.data() + static_cast<std::size_t>(box.top + y) * pitch;
        std::uint8_t* dst = coverage.data() + static_cast<std::size_t>(y) * box.width();
        for (int x = 0; x < box.width(); ++x)
        {
            const std::uint8_t a = row[static_cast<std::size_t>(box.left + x) * 4 + 3];
            // Non-AA text must stay binary: the engine multiplies this mask into
            // the frame, so intermediate values would show as grey fringes.
            dst[x] = antialiased ? a : (a >= 128 ? 0xFF : 0);
        }
    }
    data = new tTVPCharacterData(coverage.data(), box.width(), box.left, origin_y0 + box.top,
                                 box.width(), box.height(), metrics);
    data->Gray = 256; // 256 gray levels, same as tFreeTypeFace's glyphs
    data->FullColored = false;

    // The host rasterises only the glyph; underline/strikeout are engine
    // decorations (tFreeTypeFace draws them into the bitmap the same way). The
    // positions mirror FreeType's face-metric defaults: underline just below the
    // baseline, strikeout at 70% of the ascent. AddHorizontalLine needs a real
    // bitmap to work on, which the empty-glyph branch above does not have.
    if (ascent > 0)
    {
        const tjs_int thickness = std::max<tjs_int>(1, size / 14);
        if (font.Flags & TVP_TF_UNDERLINE)
            data->AddHorizontalLine(ascent + std::max<tjs_int>(1, size / 12), thickness, 255);
        if (font.Flags & TVP_TF_STRIKEOUT)
            data->AddHorizontalLine(ascent * 7 / 10, thickness, 255);
    }
    return data;
}
