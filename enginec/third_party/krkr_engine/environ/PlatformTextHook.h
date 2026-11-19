#pragma once
#include <cstdint>
// Host glyph rasterization hook.
//
// Why: cross-platform text rendering is delegated to the Flutter host (Flutter's
// text stack: TextPainter/Impeller + platform fonts). The engine keeps doing text
// layout and composition (it knows ruby, effects, message-window layout); only
// glyph rasterization is outsourced, so font availability and hinting quality
// come from the platform once instead of shipping font files per game.
//
// Flow: the font rasterizer seam (core/media/font/FontRasterizer.h) calls
// TVPTextRenderHook per glyph run; when the hook is null or returns false the
// FreeType rasterizer renders as before (standalone tools, tests, host-less runs).

struct TVPTextRun
{
    const char16_t* text; // UTF-16 run, not NUL-terminated (often one char)
    int length;
    const char* family;   // UTF-8 requested font family (may be "")
    float size;
    std::uint32_t color;  // RGBA
    // TVP_TF_BOLD / TVP_TF_ITALIC from TVPFontstruc.h. FreeType synthesizes these
    // when the face has no bold/slanted cut; the host must synthesize them too or
    // a game's bold/italic text silently renders regular on the Flutter path.
    std::uint32_t flags;
};

struct TVPTextBitmap
{
    // RGBA8, row-major, tightly packed (pitch == width * 4). Allocated by the
    // callee with malloc(); the engine releases it with free().
    std::uint8_t* pixels;
    int width;
    int height;
    int pitch;
    int advance;  // horizontal advance in pixels for the whole run
    // Distance from the bitmap's top edge to the alphabetic baseline, in pixels.
    // The old engine guessed this as 0.8 * height, which shifts text by a pixel or
    // two on fonts whose ascent ratio differs. 0 means "host did not report it"
    // and the engine keeps the old estimate.
    int baseline;
};

// Return true when the host rendered `run` into *out; false falls back to the
// engine's FreeType rasterizer.
//
// Engine-side consumption (core/media/font/HostGlyphRasterizer.cpp): the engine
// asks for opaque white (0xFFFFFFFF) and uses the alpha channel as the coverage
// mask, because the engine tints glyphs with the text colour at draw time. The
// bitmap is expected to be the whole line box; the engine trims the transparent
// margin, places the box top on its own ascent line and keeps the engine's
// advance (GetTextExtent measures with FreeType, so drawing must not drift).
typedef bool (*TVPTextRenderCallback)(const TVPTextRun& run, TVPTextBitmap* out);

inline TVPTextRenderCallback TVPTextRenderHook = nullptr;
