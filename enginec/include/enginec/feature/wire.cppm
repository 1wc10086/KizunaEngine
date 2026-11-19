// Kizuna wire protocol (client <-> native core).
//
// Byte-for-byte identical to Go's enginego/wire: a 16-byte request header
// ("KZEQ", op, seq, payload length) followed by TLV fields (id u16, type u8,
// reserved u8, length u32, value). The Flutter bridge already speaks this
// protocol, so libenginec can be dropped in without touching the Dart codec.
//
// Field numbers are frozen. Adding a field means picking an unused id; reusing one
// would silently reinterpret an old client's value (the reason the Go side has so
// many comments about field collisions).
export module enginec.feature.wire;

import std;

export namespace enginec::wire {

inline constexpr std::uint16_t op_open = 1;
inline constexpr std::uint16_t op_info = 2;
inline constexpr std::uint16_t op_start = 3;
inline constexpr std::uint16_t op_run = 4;
inline constexpr std::uint16_t op_advance = 5;
inline constexpr std::uint16_t op_choose = 6;
inline constexpr std::uint16_t op_pointer = 7;
inline constexpr std::uint16_t op_compose = 8;
inline constexpr std::uint16_t op_scenes = 9;
inline constexpr std::uint16_t op_slots = 10;
inline constexpr std::uint16_t op_save = 11;
inline constexpr std::uint16_t op_load = 12;
inline constexpr std::uint16_t op_asset_size = 13;
inline constexpr std::uint16_t op_read_asset = 14;
inline constexpr std::uint16_t op_audio = 15;
inline constexpr std::uint16_t op_text = 16;
inline constexpr std::uint16_t op_choices = 17;
inline constexpr std::uint16_t op_state = 18;
inline constexpr std::uint16_t op_text_list = 19;
inline constexpr std::uint16_t op_last_error = 20;
inline constexpr std::uint16_t op_brands = 21;
inline constexpr std::uint16_t op_detect = 22;
inline constexpr std::uint16_t op_scan = 23;
inline constexpr std::uint16_t op_submit_name = 24;
inline constexpr std::uint16_t op_audio_done = 25;
inline constexpr std::uint16_t op_map_buttons = 26;
inline constexpr std::uint16_t op_map_mask = 27;
inline constexpr std::uint16_t op_select_items = 28;
inline constexpr std::uint16_t op_select_frame = 29;
inline constexpr std::uint16_t op_compose_images = 30;
inline constexpr std::uint16_t op_set_preview = 31;
inline constexpr std::uint16_t op_slot_preview = 32;
inline constexpr std::uint16_t op_cg_catalog = 33;
inline constexpr std::uint16_t op_set_option = 34;
inline constexpr std::uint16_t op_key = 35;
inline constexpr std::uint16_t op_wheel = 36;
inline constexpr std::uint16_t op_frame = 37;
inline constexpr std::uint16_t op_title_menu = 38;
inline constexpr std::uint16_t op_title_select = 39;
inline constexpr std::uint16_t op_movie = 40;
inline constexpr std::uint16_t op_movie_done = 41;
inline constexpr std::uint16_t op_submit_yes_no = 42;
inline constexpr std::uint16_t op_prompt_dialog = 43;
inline constexpr std::uint16_t op_prompt_text = 44;

inline constexpr std::uint16_t status_ok = 0;
inline constexpr std::uint16_t status_error = 1;
inline constexpr std::uint16_t status_not_found = 2;
inline constexpr std::uint16_t status_bad_request = 3;

// Shared field ids. Only the ones the C++ core reads/writes are listed; the
// complete table lives in the Go implementation and in protocol_consts.dart.
namespace field {
inline constexpr std::uint16_t f1 = 1;
inline constexpr std::uint16_t f2 = 2;
inline constexpr std::uint16_t f3 = 3;
inline constexpr std::uint16_t f4 = 4;
inline constexpr std::uint16_t f5 = 5;
// Scene count in OpInfo; Dart reads it as KzField.f6.
inline constexpr std::uint16_t f6 = 6;
inline constexpr std::uint16_t f7 = 7;
inline constexpr std::uint16_t f8 = 8;
inline constexpr std::uint16_t f9 = 9;
inline constexpr std::uint16_t f10 = 10;
inline constexpr std::uint16_t f11 = 11;
inline constexpr std::uint16_t f12 = 12;
inline constexpr std::uint16_t f13 = 13;
inline constexpr std::uint16_t f14 = 14;
inline constexpr std::uint16_t f15 = 15;
inline constexpr std::uint16_t f16 = 16;
inline constexpr std::uint16_t f17 = 17;
inline constexpr std::uint16_t f18 = 18;
inline constexpr std::uint16_t f19 = 19;
inline constexpr std::uint16_t f20 = 20;
inline constexpr std::uint16_t f21 = 21;
inline constexpr std::uint16_t f22 = 22;
inline constexpr std::uint16_t f24 = 24;
inline constexpr std::uint16_t f25 = 25;
inline constexpr std::uint16_t f26 = 26;
inline constexpr std::uint16_t f27 = 27;
inline constexpr std::uint16_t f28 = 28;
inline constexpr std::uint16_t f29 = 29;
inline constexpr std::uint16_t f30 = 30;
inline constexpr std::uint16_t f31 = 31;
inline constexpr std::uint16_t f32 = 32;
inline constexpr std::uint16_t f33 = 33;
inline constexpr std::uint16_t f34 = 34;
inline constexpr std::uint16_t f35 = 35;
inline constexpr std::uint16_t f36 = 36;
inline constexpr std::uint16_t f37 = 37;
inline constexpr std::uint16_t f38 = 38;
inline constexpr std::uint16_t f39 = 39;
inline constexpr std::uint16_t f40 = 40;
inline constexpr std::uint16_t f41 = 41;
inline constexpr std::uint16_t f42 = 42;
inline constexpr std::uint16_t f44 = 44;
inline constexpr std::uint16_t f45 = 45;
inline constexpr std::uint16_t f46 = 46;
inline constexpr std::uint16_t f47 = 47;
inline constexpr std::uint16_t f48 = 48;
inline constexpr std::uint16_t f49 = 49;
inline constexpr std::uint16_t f57 = 57;
inline constexpr std::uint16_t f58 = 58;
inline constexpr std::uint16_t f59 = 59;
inline constexpr std::uint16_t f60 = 60;
inline constexpr std::uint16_t f63 = 63;
inline constexpr std::uint16_t f64 = 64;
// Scene group/index inside an OpScenes item; Dart reads KzField.f65/f66.
inline constexpr std::uint16_t f65 = 65;
inline constexpr std::uint16_t f66 = 66;
inline constexpr std::uint16_t f68 = 68;
inline constexpr std::uint16_t f69 = 69;
inline constexpr std::uint16_t f70 = 70;
inline constexpr std::uint16_t f71 = 71;
inline constexpr std::uint16_t f72 = 72;
inline constexpr std::uint16_t f73 = 73;
inline constexpr std::uint16_t f74 = 74;
inline constexpr std::uint16_t f75 = 75;
inline constexpr std::uint16_t f76 = 76;
inline constexpr std::uint16_t f77 = 77;
inline constexpr std::uint16_t f78 = 78;
inline constexpr std::uint16_t f79 = 79;
inline constexpr std::uint16_t f80 = 80;
inline constexpr std::uint16_t f81 = 81;
inline constexpr std::uint16_t f82 = 82;
inline constexpr std::uint16_t f83 = 83;
inline constexpr std::uint16_t f84 = 84;
inline constexpr std::uint16_t f85 = 85;
inline constexpr std::uint16_t f86 = 86;
inline constexpr std::uint16_t f87 = 87;
inline constexpr std::uint16_t f88 = 88;
inline constexpr std::uint16_t f89 = 89;
inline constexpr std::uint16_t f90 = 90;
inline constexpr std::uint16_t f91 = 91;
inline constexpr std::uint16_t f92 = 92;
inline constexpr std::uint16_t f93 = 93;
inline constexpr std::uint16_t f94 = 94;
inline constexpr std::uint16_t f95 = 95;
inline constexpr std::uint16_t f96 = 96;
inline constexpr std::uint16_t f97 = 97;
inline constexpr std::uint16_t f98 = 98;
inline constexpr std::uint16_t f99 = 99;
inline constexpr std::uint16_t f100 = 100;
inline constexpr std::uint16_t f101 = 101;
inline constexpr std::uint16_t f102 = 102;
inline constexpr std::uint16_t f103 = 103;
// Prompt-dialog payload (op 43), named for the same ids protocol_consts.dart
// freezes. The KiriKiri port draws its prompts into the frame and answers
// Active=false, so these are only written when a dialog exposer exists; naming
// them here keeps a future one from picking a conflicting number.
inline constexpr std::uint16_t prompt_title = f93;
inline constexpr std::uint16_t prompt_message = f94;
inline constexpr std::uint16_t prompt_edit = f95;
inline constexpr std::uint16_t prompt_button_count = f96;
inline constexpr std::uint16_t prompt_buttons = f97;
inline constexpr std::uint16_t prompt_button_id = f98;
inline constexpr std::uint16_t prompt_button_width = f99;
inline constexpr std::uint16_t prompt_button_height = f100;
inline constexpr std::uint16_t prompt_heading = f101;
// 104..112 are the text-run extension shared with enginego: the core returns the
// positioned text it wants the host to draw, so no font lives in the native core.
inline constexpr std::uint16_t text_runs = 104;
inline constexpr std::uint16_t run_text = 105;
inline constexpr std::uint16_t run_size = 106;
inline constexpr std::uint16_t run_color = 107;
inline constexpr std::uint16_t run_outline = 108;
inline constexpr std::uint16_t run_shadow = 109;
inline constexpr std::uint16_t run_align = 110;
inline constexpr std::uint16_t run_width = 111;
inline constexpr std::uint16_t run_bold = 112;
// 113..116: compose surface handshake. The host sends the geometry of the buffer it
// allocated (113 width / 114 height) and the reply carries the game's current
// canvas size (115/116), so a game that switches from its boot-options dialog to a
// full-size window is presented 1:1 instead of being scaled into a stale surface.
inline constexpr std::uint16_t surface_width = 113;
inline constexpr std::uint16_t surface_height = 114;
inline constexpr std::uint16_t canvas_width = 115;
inline constexpr std::uint16_t canvas_height = 116;
// 117: gain for the movie's audio track (0..1). KAG's `@movie volume=` reaches
// the engine as the overlay's `audioVolume`; the host applies it to its player.
inline constexpr std::uint16_t movie_audio_gain = 117;
// 118: "start this voice paused" flag on an audio item. KAG's SoundBuffer sets
// `paused` *before* `play()`, so without this the host would have to start the
// sample and then pause it, producing an audible blip for every `@bgm paused=true`.
inline constexpr std::uint16_t audio_paused = 118;
} // namespace field

enum class type : std::uint8_t {
    u32 = 1,
    i32 = 2,
    u64 = 3,
    str = 4,
    bytes_ = 5,
    list = 6,
    boolean = 7,
    f64 = 8,
};

struct field_value {
    type kind = type::str;
    std::span<const std::uint8_t> data;
};

// Parses the TLV payload. Duplicate ids keep the last value, exactly like the Go
// reader (which writes into a map); callers that emit lists must use one field id.
class reader {
public:
    reader() = default;
    explicit reader(std::span<const std::uint8_t> payload);

    [[nodiscard]] std::uint32_t u32(std::uint16_t id) const;
    [[nodiscard]] std::int32_t i32(std::uint16_t id) const;
    [[nodiscard]] std::uint64_t u64(std::uint16_t id) const;
    [[nodiscard]] double f64(std::uint16_t id) const;
    [[nodiscard]] bool boolean(std::uint16_t id) const;
    [[nodiscard]] std::string str(std::uint16_t id) const;
    [[nodiscard]] std::span<const std::uint8_t> raw(std::uint16_t id) const;
    [[nodiscard]] std::vector<std::string> str_list(std::uint16_t id) const;
    void each(std::uint16_t id, const std::function<void(reader)>& fn) const;
    [[nodiscard]] bool has(std::uint16_t id) const;

private:
    std::unordered_map<std::uint16_t, field_value> fields_;
};

class writer {
public:
    void str(std::uint16_t id, std::string_view value);
    void u32(std::uint16_t id, std::uint32_t value);
    void i32(std::uint16_t id, std::int32_t value);
    void u64(std::uint16_t id, std::uint64_t value);
    void f64(std::uint16_t id, double value);
    void boolean(std::uint16_t id, bool value);
    void raw(std::uint16_t id, std::span<const std::uint8_t> value);
    void str_list(std::uint16_t id, const std::vector<std::string>& values);
    void list(std::uint16_t id, const std::function<void(writer&)>& fn);

    [[nodiscard]] const std::vector<std::uint8_t>& data() const noexcept { return buffer_; }

private:
    void head(std::uint16_t id, type kind, std::size_t length);
    std::vector<std::uint8_t> buffer_;
};

struct request {
    std::uint16_t op = 0;
    std::uint32_t seq = 0;
    reader payload;
};

// Returns nullopt on a short/malformed header. The caller answers with
// status_bad_request using seq 0, mirroring the Go parser.
std::optional<request> parse_request(std::span<const std::uint8_t> data);

std::vector<std::uint8_t> build_response(std::uint16_t status, std::uint32_t seq,
                                         const std::vector<std::uint8_t>& payload);
std::vector<std::uint8_t> build_error(std::uint32_t seq, std::string_view message,
                                      std::uint16_t status = status_error);

} // namespace enginec::wire
