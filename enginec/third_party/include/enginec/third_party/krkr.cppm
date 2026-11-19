// KiriKiri core as a C++23 module.
//
// Everything below this boundary is the original-style C++ port (third_party/krkr).
// Feature modules only see these std-typed structures; no TJS2/XP3/PNG header ever
// leaks upward. The conversion layer is mechanical and lives in third_party/src/
// krkr.cpp, which is the only translation unit that includes both sides.
export module enginec.third_party.krkr;

import std;

export namespace enginec::third_party::krkr {

enum class step : std::int32_t {
    cont = 0,
    await_input = 1,
    await_choice = 2,
    await_select = 3,
    wait = 4,
    finished = 5,
    faulted = 6,
    await_name = 7,
    await_title = 8,
    await_movie = 9,
};

struct info {
    int width = 0;
    int height = 0;
    std::string title;
    std::string version;
    int scene_count = 0;
    int entry_count = 0;
};

struct text_state {
    std::string speaker;
    std::string text;
    std::uint32_t awaiting = 0;
};

struct choice {
    std::string name;
    std::int32_t value = 0;
};

struct audio_event {
    std::uint32_t kind = 0;
    std::string path;
    double volume = 1.0;
    bool loop = false;
    std::uint32_t channel = 0xFFFFFFFFu;
    std::uint32_t group = 0xFFFFFFFFu;
    std::vector<std::string> names;
    std::uint32_t frames = 0;
    // Start this voice paused (KAG sets `paused` before `play()`). Only the
    // krkr port sets it; the Go brands always start audible.
    bool paused = false;
};

struct engine_state {
    std::string bgm;
    std::uint32_t pc = 0;
    std::uint32_t scene = 0;
    bool animating = false;
    std::uint32_t wait_ms = 0;
    bool message_window_visible = false;
    bool engine_message_window = false;
    bool awaiting_name = false;
    bool prompt_is_yes_no = false;
    bool engine_prompt = false;
    bool backlog_active = false;
    bool replay_active = false;
    std::uint32_t instruction_start = 0;
    std::string name_prompt;
    std::string host_request;
    std::uint32_t host_request_serial = 0;
    bool message_unread = false;
    bool cease_skip = true;
    bool skip_unread = false;
    int auto_wait = 50;
};

struct slot_info {
    std::uint32_t index = 0;
    std::string title;
    std::string saved_at;
    std::string summary;
    bool exists = false;
};

struct scene_info {
    std::uint32_t id = 0;
    std::string name;
    std::string title;
    std::uint32_t group = 0;
    std::uint32_t index = 0;
    std::vector<std::string> images;
    bool unlocked = false;
};

struct text_unit {
    std::string id;
    std::string text;
    std::string file;
};

// A movie the host has to play (the engine has no decoder in this build).
// `serial` is the overlay's own id (environ/PlatformMovieHook.h owns serials): it
// changes with every request, so the host can tell a new movie from the same one
// polled again (the host layer already dedupes on it), and it is the number the
// adapter hands back to TVPMovieNotifyFinished. One serial for one request, both
// on the wire and in the completion. Completion is the payload-less movie-done op:
// the adapter finishes whatever is pending.
struct movie_request {
    std::string path;
    int serial = 0;
    // Gain the host applies to the movie's audio track (KAG's `@movie volume=`).
    double audio_gain = 1.0;
};

// Glyph run handed to the host's text stack. Same fields, order and types as the C
// ABI (enginec.ffi.api: KzcTextRun/KzcTextBitmap); the FFI layer copies
// field by field rather than punning one struct as the other.
struct text_run {
    const char16_t* text = nullptr; // UTF-16, not NUL-terminated
    std::int32_t length = 0;
    const char* family = nullptr; // UTF-8 family name, may be ""
    float size = 0.0f;
    std::uint32_t color = 0; // RGBA
    // TVP_TF_BOLD / TVP_TF_ITALIC bits; the host synthesizes them like FreeType.
    std::uint32_t flags = 0;
};

struct text_bitmap {
    // RGBA8, tightly packed; allocated with malloc() by the host, released by the
    // engine with free() (see environ/PlatformTextHook.h).
    std::uint8_t* pixels = nullptr;
    std::int32_t width = 0;
    std::int32_t height = 0;
    std::int32_t pitch = 0;
    std::int32_t advance = 0;
    // Top edge -> alphabetic baseline, in pixels; 0 means "not reported".
    std::int32_t baseline = 0;
};

// Returns false to let the engine rasterize the run with FreeType.
using text_render_callback = bool (*)(const text_run* run, text_bitmap* out);

// Installs (or clears, with nullptr) the host glyph rasterizer. Process-global,
// like TVPAudioEventHook: the engine calls it synchronously on whatever thread
// drives the renderer, which is the thread calling into this module.
void set_text_renderer(text_render_callback callback);

struct open_options {
    std::string dir;
    std::string data_dir;
    std::string cache_dir;
    std::vector<std::string> extra_dirs;
    std::string brand_id;
    std::string script;
};

struct probe_result {
    bool is_krkr = false;
    std::string data_dir;
    std::string script;
    std::string title;
    int width = 0;
    int height = 0;
    std::vector<std::string> archives;
};

// Checks a directory for KiriKiri markers. Defined here (not only on engine) so the
// Detect op can answer before any game is opened.
probe_result probe(std::string_view directory);
bool detect(std::string_view directory);

class engine {
public:
    engine();
    ~engine();
    engine(const engine&) = delete;
    engine& operator=(const engine&) = delete;

    bool open(const open_options& options, info& out, std::string& error);
    void close();
    [[nodiscard]] bool is_open() const noexcept;

    step start(std::uint32_t entry, std::string& error);
    // Every stepping op runs at most one engine frame and returns it; when a movie
    // is pending they return step::await_movie without touching the engine.
    step run(std::uint32_t budget, std::string& error);
    step advance(std::string& error);
    step choose(std::uint32_t index, std::string& error);
    step pointer(std::uint32_t op, std::int32_t x, std::int32_t y, std::string& error);
    step press_key(std::uint32_t code, std::string& error);
    step wheel(std::int32_t delta, std::string& error);

    // Converts the frame captured by the last stepping op into the host's RGBA
    // buffer. Never runs the engine: compose is a separate RPC path in the host
    // (render throttle), and iterating here would advance the script a second time
    // per tick. Returns false when the pixels are unchanged ("not dirty").
    //
    // `surface_width`/`surface_height` describe the destination buffer the caller
    // actually passed in. The game may change its draw buffer at any time (its boot
    // options window is 480x280, the game itself 1920x1080), so the host resizes its
    // surface when canvas_size() changes and announces the new geometry here; the
    // engine then composes 1:1 instead of scaling into a stale, too-small buffer.
    bool compose(std::uint8_t* dst, std::size_t n, int surface_width, int surface_height,
                 std::string& error);

    // Native size of the frame the game currently presents. The host compares it
    // with its own surface and reallocates on change.
    struct frame_size
    {
        int width = 0;
        int height = 0;
    };
    [[nodiscard]] frame_size canvas_size() const;

    [[nodiscard]] info information() const;
    [[nodiscard]] std::vector<scene_info> scenes() const;
    [[nodiscard]] std::vector<slot_info> slots() const;
    bool save(std::uint32_t slot, std::string_view title, std::string& error);
    step load(std::uint32_t slot, std::string& error);

    [[nodiscard]] std::size_t asset_size(std::string_view path) const;
    std::size_t read_asset(std::string_view path, std::uint8_t* dst, std::size_t n) const;

    std::vector<audio_event> take_audio();
    [[nodiscard]] text_state text() const;
    [[nodiscard]] std::vector<choice> choices() const;
    [[nodiscard]] engine_state state() const;
    [[nodiscard]] std::vector<text_unit> text_list() const;
    [[nodiscard]] std::string last_error() const;

    bool submit_name(std::string_view value);
    bool submit_yes_no(bool yes);
    void notify_audio_completed(std::string_view file, bool voice, std::uint32_t channel);
    void set_option(std::uint32_t key, bool value);

    // Reports the host-owned movie (empty path when none) and completes it. The
    // request stays pending across polls and is consumed by finish_movie(), which
    // quotes its serial to the overlay and then runs one frame so the caller gets
    // the step that follows the wait.
    step finish_movie(std::string& error);
    [[nodiscard]] movie_request current_movie() const;
    [[nodiscard]] step current_step() const noexcept;

private:
    struct impl;
    std::unique_ptr<impl> impl_;
};

std::unique_ptr<engine> create_engine();
std::string version_string();

} // namespace enginec::third_party::krkr
