// Wire dispatcher for the KiriKiri core.
//
// Field numbers, status codes and reply shapes are taken one-for-one from
// enginego/capi/capi.go, so a Flutter client built against the Go engine cannot tell
// the two cores apart. This file is the only place that knows both the protocol and
// the port's API; all stateful work stays inside krkr::engine.
//
// The differences from capi.go are deliberate and confined to safety:
//   * a compose/read destination of 0 is never dereferenced (Go would fault);
//   * a failed open leaves the previously opened game untouched instead of storing a
//     half-opened engine in the handle;
//   * ops the KiriKiri port has no backend for (map/select/gallery/title interfaces)
//     return status OK with an empty payload, matching what Go sends for a brand that
//     does not implement them.
// Compose is not followed by any byte-order swap: the port writes the host byte
// order (RGBA — what FramePainter's PixelFormat.rgba8888 reads) straight into the
// host buffer (see third_party/src/krkr.cpp's engine::compose), which is the same
// exemption the Go core grants brands declaring ComposeWritesBGRA (despite that
// historical name, it means "already in host order").

module enginec.feature.engine;

import std;

import enginec.feature.wire;
import enginec.third_party.krkr;

namespace enginec
{

// Outside the unnamed namespace so engine_core::impl (declared at namespace scope
// below) resolves them too.
namespace krkr = third_party::krkr;
namespace fields = wire::field;

namespace
{

constexpr std::string_view brand_id = "krkr";
constexpr std::string_view brand_name = "KiriKiri";
constexpr std::string_view not_open_message = "engine: not open";

std::vector<std::uint8_t> reply_ok(std::uint32_t seq, const wire::writer& body)
{
    return wire::build_response(wire::status_ok, seq, body.data());
}

std::vector<std::uint8_t> reply_empty(std::uint32_t seq)
{
    return wire::build_response(wire::status_ok, seq, {});
}

std::vector<std::uint8_t> reply_bad_request(std::uint32_t seq)
{
    return wire::build_response(wire::status_bad_request, seq, {});
}

std::vector<std::uint8_t> reply_error(std::uint32_t seq, std::string_view message)
{
    return wire::build_error(seq, message);
}

std::vector<std::uint8_t> reply_true(std::uint32_t seq)
{
    wire::writer body;
    body.boolean(fields::f1, true);
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> reply_step(std::uint32_t seq, krkr::step value)
{
    wire::writer body;
    body.u32(fields::f10, static_cast<std::uint32_t>(static_cast<std::int32_t>(value)));
    return reply_ok(seq, body);
}

// A port that fails without filling `error` would otherwise send an empty message,
// which the host cannot display.
std::string_view message_or(std::string_view message, std::string_view fallback) noexcept
{
    return message.empty() ? fallback : message;
}

// Every step-returning op has the same contract: a non-empty error means failure
// even when a step came back, so one helper keeps every call site aligned with
// capi.go instead of letting one of them forget the error check.
template <typename Action>
std::vector<std::uint8_t> run_step_op(std::uint32_t seq, Action&& action)
{
    std::string error;
    const auto result = action(error);
    if (!error.empty())
    {
        return reply_error(seq, error);
    }
    return reply_step(seq, result);
}

// Ops Go answers with an empty OK payload while no engine exists. The host polls
// several of these before open() completes (movie, prompt, audio done); answering
// with an error would surface as a spurious failure at startup.
bool answers_without_engine(std::uint16_t op) noexcept
{
    switch (op)
    {
    case wire::op_submit_name:
    case wire::op_submit_yes_no:
    case wire::op_audio_done:
    case wire::op_map_buttons:
    case wire::op_map_mask:
    case wire::op_select_items:
    case wire::op_select_frame:
    case wire::op_compose_images:
    case wire::op_set_preview:
    case wire::op_slot_preview:
    case wire::op_cg_catalog:
    case wire::op_set_option:
    case wire::op_key:
    case wire::op_wheel:
    case wire::op_title_menu:
    case wire::op_title_select:
    case wire::op_movie:
    case wire::op_movie_done:
    case wire::op_prompt_dialog:
    case wire::op_prompt_text:
        return true;
    default:
        return false;
    }
}

// Ops that only make sense once a game is open. Kept apart from the whitelist above
// only so a request for an op this core never implements is still answered with
// status_bad_request before open, as the protocol contract states.
bool is_engine_op(std::uint16_t op) noexcept
{
    switch (op)
    {
    case wire::op_info:
    case wire::op_start:
    case wire::op_run:
    case wire::op_advance:
    case wire::op_choose:
    case wire::op_pointer:
    case wire::op_compose:
    case wire::op_scenes:
    case wire::op_slots:
    case wire::op_save:
    case wire::op_load:
    case wire::op_asset_size:
    case wire::op_read_asset:
    case wire::op_audio:
    case wire::op_text:
    case wire::op_choices:
    case wire::op_state:
    case wire::op_text_list:
    case wire::op_last_error:
    case wire::op_frame:
    case wire::op_submit_name:
    case wire::op_submit_yes_no:
    case wire::op_audio_done:
    case wire::op_set_option:
    case wire::op_key:
    case wire::op_wheel:
    case wire::op_movie:
    case wire::op_movie_done:
    case wire::op_prompt_dialog:
    case wire::op_prompt_text:
        return true;
    default:
        return false;
    }
}

void write_text_fields(wire::writer& body, const krkr::text_state& text)
{
    body.str(fields::f30, text.speaker);
    body.str(fields::f31, text.text);
    body.u32(fields::f32, text.awaiting);
}

void write_state_fields(wire::writer& body, const krkr::engine_state& state)
{
    body.str(fields::f35, state.bgm);
    body.u32(fields::f36, state.pc);
    body.u32(fields::f37, state.scene);
    body.boolean(fields::f38, state.animating);
    body.u32(fields::f45, state.wait_ms);
    body.boolean(fields::f46, state.message_window_visible);
    body.boolean(fields::f82, state.engine_message_window);
    body.boolean(fields::f47, state.awaiting_name);
    body.boolean(fields::f92, state.prompt_is_yes_no);
    body.boolean(fields::f103, state.engine_prompt);
    body.boolean(fields::f48, state.backlog_active);
    body.boolean(fields::f49, state.replay_active);
    body.u32(fields::f60, state.instruction_start);
    body.str(fields::f79, state.name_prompt);
    // Host action requests carry a serial so the host can tell a fresh request from
    // the same one it reads again on every state poll.
    body.str(fields::f80, state.host_request);
    body.u32(fields::f81, state.host_request_serial);
    // 88..91 must arrive together: "skip stops on unread text" is decided from the
    // unread flag and the two settings in the same snapshot.
    body.boolean(fields::f88, state.message_unread);
    body.boolean(fields::f89, state.cease_skip);
    body.boolean(fields::f90, state.skip_unread);
    body.u32(fields::f91, static_cast<std::uint32_t>(state.auto_wait));
}

void write_audio_list(wire::writer& body, const std::vector<krkr::audio_event>& audio)
{
    body.list(fields::f26, [&audio](wire::writer& outer) {
        for (const auto& event : audio)
        {
            outer.list(0, [&event](wire::writer& item) {
                item.u32(fields::f27, event.kind);
                item.str(fields::f1, event.path);
                item.f64(fields::f28, event.volume);
                item.boolean(fields::f29, event.loop);
                item.u32(fields::f3, event.channel);
                item.u32(fields::f69, event.group);
                item.str_list(fields::f70, event.names);
                item.u32(fields::f71, event.frames);
                item.boolean(fields::audio_paused, event.paused);
            });
        }
    });
}

std::vector<std::uint8_t> op_open(std::unique_ptr<krkr::engine>& slot, std::uint32_t seq,
                                  const wire::reader& payload)
{
    const std::string dir = payload.str(fields::f1);
    const std::string brand = payload.str(fields::f44);
    std::vector<std::string> extra_dirs = payload.str_list(fields::f7);
    const std::string cache_dir = payload.str(fields::f10);

    // An explicit id must name a brand this core has; "krkr" is the id the library
    // scanner reports for a KiriKiri game, so it is the only accepted value.
    if (!brand.empty() && std::string_view(brand) != brand_id)
    {
        return reply_error(seq, "engine: unknown brand: " + brand);
    }
    // Without an id the directory decides, exactly like core.Detect in Go: a
    // directory that is not a KiriKiri game must not be opened as one.
    if (brand.empty() && !krkr::detect(dir))
    {
        return wire::build_error(seq, "engine: no brand matches directory: " + dir,
                                 wire::status_not_found);
    }

    auto candidate = krkr::create_engine();
    if (!candidate)
    {
        return reply_error(seq, "engine: cannot create engine");
    }
    krkr::open_options options;
    options.dir = dir;
    options.cache_dir = cache_dir;
    options.extra_dirs = std::move(extra_dirs);
    options.brand_id = brand;

    krkr::info info;
    std::string error;
    if (!candidate->open(options, info, error))
    {
        // The previous game (if any) stays usable: replacing it with a failed
        // instance would turn a mistyped path into a dead handle.
        return reply_error(seq, message_or(error, "engine: open failed"));
    }

    slot = std::move(candidate);
    wire::writer body;
    body.boolean(fields::f1, true);
    body.u32(fields::f2, static_cast<std::uint32_t>(info.width));
    body.u32(fields::f3, static_cast<std::uint32_t>(info.height));
    body.str(fields::f4, info.title);
    body.str(fields::f5, info.version);
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_brands(std::uint32_t seq)
{
    wire::writer body;
    body.list(fields::f18, [](wire::writer& outer) {
        outer.list(0, [](wire::writer& item) {
            item.str(fields::f19, brand_id);
            item.str(fields::f20, brand_name);
        });
    });
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_detect(std::uint32_t seq, const wire::reader& payload)
{
    const std::string dir = payload.str(fields::f1);
    // Same predicate open() uses when no brand id is supplied, so a directory the
    // scanner advertises as KiriKiri is exactly one open() will accept.
    const bool matches = krkr::detect(dir);
    wire::writer body;
    body.str(fields::f19, matches ? brand_id : std::string_view{});
    body.str(fields::f20, matches ? brand_name : std::string_view{});
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_info(const krkr::engine& core, std::uint32_t seq)
{
    const auto info = core.information();
    wire::writer body;
    body.u32(fields::f2, static_cast<std::uint32_t>(info.width));
    body.u32(fields::f3, static_cast<std::uint32_t>(info.height));
    body.str(fields::f4, info.title);
    body.u32(fields::f6, static_cast<std::uint32_t>(info.scene_count));
    body.u32(fields::f7, static_cast<std::uint32_t>(info.entry_count));
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_scenes(const krkr::engine& core, std::uint32_t seq)
{
    const auto scenes = core.scenes();
    wire::writer body;
    body.list(fields::f18, [&scenes](wire::writer& outer) {
        for (const auto& scene : scenes)
        {
            outer.list(0, [&scene](wire::writer& item) {
                item.u32(fields::f19, scene.id);
                item.str(fields::f20, scene.name);
                item.u32(fields::f65, scene.group);
                item.u32(fields::f66, scene.index);
                item.str(fields::f80, scene.title);
                item.str_list(fields::f81, scene.images);
                // f87 is shared with the CG unlock bits: both mean "visible in the
                // gallery", so the host reads it with one rule.
                item.boolean(fields::f87, scene.unlocked);
            });
        }
    });
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_slots(const krkr::engine& core, std::uint32_t seq)
{
    const auto slots = core.slots();
    wire::writer body;
    body.list(fields::f21, [&slots](wire::writer& outer) {
        for (const auto& slot : slots)
        {
            outer.list(0, [&slot](wire::writer& item) {
                item.u32(fields::f11, slot.index);
                item.str(fields::f22, slot.title);
                item.str(fields::f24, slot.saved_at);
                item.str(fields::f63, slot.summary);
                item.boolean(fields::f64, slot.exists);
            });
        }
    });
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_compose(krkr::engine& core, std::uint32_t seq,
                                     const wire::reader& payload)
{
    const std::uint64_t address = payload.u64(fields::f15);
    const std::uint32_t length = payload.u32(fields::f16);
    if (address == 0)
    {
        // The host owns this buffer; writing through a null address is the one
        // failure the compose path cannot recover from, so it is refused.
        return reply_error(seq, "engine: compose buffer missing");
    }

    auto* dst = reinterpret_cast<std::uint8_t*>(static_cast<std::uintptr_t>(address));
    const int surface_width = static_cast<int>(payload.u32(fields::surface_width));
    const int surface_height = static_cast<int>(payload.u32(fields::surface_height));
    std::string error;
    const bool dirty =
        core.compose(dst, static_cast<std::size_t>(length), surface_width, surface_height, error);
    if (!error.empty())
    {
        return reply_error(seq, error);
    }
    // dirty=false means the buffer was left untouched and the host keeps the
    // texture it already uploaded. The canvas size rides on every reply so the host
    // can reallocate when the game changes resolution; adopting a new surface also
    // forces dirty=true, so the new buffer is painted before it is presented.
    const auto canvas = core.canvas_size();
    wire::writer body;
    body.boolean(fields::f17, dirty);
    body.u32(fields::canvas_width, static_cast<std::uint32_t>(canvas.width));
    body.u32(fields::canvas_height, static_cast<std::uint32_t>(canvas.height));
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_asset_size(const krkr::engine& core, std::uint32_t seq,
                                        const wire::reader& payload)
{
    const auto size = core.asset_size(payload.str(fields::f1));
    wire::writer body;
    body.u32(fields::f25, static_cast<std::uint32_t>(size));
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_read_asset(const krkr::engine& core, std::uint32_t seq,
                                        const wire::reader& payload)
{
    const std::string path = payload.str(fields::f1);
    const std::uint64_t address = payload.u64(fields::f15);
    const std::uint32_t length = payload.u32(fields::f16);

    std::uint32_t written = 0;
    // A stale null/zero-length destination is answered with "nothing written" rather
    // than dereferenced: a race with a closed game must not take the process down.
    if (address != 0 && length != 0)
    {
        auto* dst = reinterpret_cast<std::uint8_t*>(static_cast<std::uintptr_t>(address));
        const std::size_t count = core.read_asset(path, dst, static_cast<std::size_t>(length));
        // The port returns what it copied; never claim more than the host's buffer.
        written = static_cast<std::uint32_t>(count < length ? count : length);
    }
    wire::writer body;
    body.u32(fields::f25, written);
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> op_frame(krkr::engine& core, std::uint32_t seq)
{
    wire::writer body;
    body.u32(fields::f10,
             static_cast<std::uint32_t>(static_cast<std::int32_t>(core.current_step())));
    write_text_fields(body, core.text());
    write_state_fields(body, core.state());
    // take_audio() drains the queue, exactly like the Go Audio() it replaces; the
    // host merges these events into the same playback queue as the audio op.
    write_audio_list(body, core.take_audio());
    return reply_ok(seq, body);
}

std::vector<std::uint8_t> dispatch(std::unique_ptr<krkr::engine>& slot,
                                   const wire::request& request)
{
    const std::uint32_t seq = request.seq;

    switch (request.op)
    {
    case wire::op_open:
        return op_open(slot, seq, request.payload);
    case wire::op_brands:
        return op_brands(seq);
    case wire::op_detect:
        return op_detect(seq, request.payload);
    default:
        break;
    }

    if (slot == nullptr || !slot->is_open())
    {
        if (answers_without_engine(request.op))
        {
            return reply_empty(seq);
        }
        if (is_engine_op(request.op))
        {
            return reply_error(seq, not_open_message);
        }
        return reply_bad_request(seq);
    }

    auto& core = *slot;
    switch (request.op)
    {
    case wire::op_info:
        return op_info(core, seq);
    case wire::op_start:
    {
        std::string error;
        // Start reports its step through the following frame op in Go as well; only
        // the failure flag is part of this reply.
        core.start(request.payload.u32(fields::f8), error);
        if (!error.empty())
        {
            return reply_error(seq, error);
        }
        return reply_true(seq);
    }
    case wire::op_run:
        return run_step_op(seq, [&core, &request](std::string& error) {
            return core.run(request.payload.u32(fields::f9), error);
        });
    case wire::op_advance:
        return run_step_op(seq, [&core](std::string& error) {
            return core.advance(error);
        });
    case wire::op_choose:
        return run_step_op(seq, [&core, &request](std::string& error) {
            return core.choose(request.payload.u32(fields::f11), error);
        });
    case wire::op_pointer:
        return run_step_op(seq, [&core, &request](std::string& error) {
            return core.pointer(request.payload.u32(fields::f12),
                                request.payload.i32(fields::f13),
                                request.payload.i32(fields::f14), error);
        });
    case wire::op_compose:
        return op_compose(core, seq, request.payload);
    case wire::op_scenes:
        return op_scenes(core, seq);
    case wire::op_slots:
        return op_slots(core, seq);
    case wire::op_save:
    {
        const std::uint32_t index = request.payload.u32(fields::f11);
        const std::string title = request.payload.str(fields::f24);
        std::string error;
        if (!core.save(index, title, error))
        {
            return reply_error(seq, message_or(error, "engine: save failed"));
        }
        return reply_true(seq);
    }
    case wire::op_load:
        return run_step_op(seq, [&core, &request](std::string& error) {
            return core.load(request.payload.u32(fields::f11), error);
        });
    case wire::op_asset_size:
        return op_asset_size(core, seq, request.payload);
    case wire::op_read_asset:
        return op_read_asset(core, seq, request.payload);
    case wire::op_frame:
        return op_frame(core, seq);
    case wire::op_audio:
    {
        wire::writer body;
        write_audio_list(body, core.take_audio());
        return reply_ok(seq, body);
    }
    case wire::op_text:
    {
        wire::writer body;
        write_text_fields(body, core.text());
        return reply_ok(seq, body);
    }
    case wire::op_choices:
    {
        const auto choices = core.choices();
        wire::writer body;
        body.list(fields::f33, [&choices](wire::writer& outer) {
            for (const auto& choice : choices)
            {
                outer.list(0, [&choice](wire::writer& item) {
                    item.str(fields::f20, choice.name);
                    item.i32(fields::f34, choice.value);
                });
            }
        });
        return reply_ok(seq, body);
    }
    case wire::op_state:
    {
        wire::writer body;
        write_state_fields(body, core.state());
        return reply_ok(seq, body);
    }
    case wire::op_text_list:
    {
        const auto units = core.text_list();
        wire::writer body;
        body.list(fields::f39, [&units](wire::writer& outer) {
            for (const auto& unit : units)
            {
                outer.list(0, [&unit](wire::writer& item) {
                    item.str(fields::f19, unit.id);
                    item.str(fields::f40, unit.text);
                    item.str(fields::f41, unit.file);
                });
            }
        });
        return reply_ok(seq, body);
    }
    case wire::op_last_error:
    {
        wire::writer body;
        body.str(fields::f42, core.last_error());
        return reply_ok(seq, body);
    }
    case wire::op_submit_name:
    {
        wire::writer body;
        body.boolean(fields::f1, core.submit_name(request.payload.str(fields::f1)));
        return reply_ok(seq, body);
    }
    case wire::op_submit_yes_no:
    {
        wire::writer body;
        body.boolean(fields::f1, core.submit_yes_no(request.payload.boolean(fields::f2)));
        return reply_ok(seq, body);
    }
    case wire::op_audio_done:
    {
        const std::string file = request.payload.str(fields::f1);
        core.notify_audio_completed(file, request.payload.boolean(fields::f2),
                                    request.payload.u32(fields::f3));
        return reply_true(seq);
    }
    case wire::op_set_option:
    {
        core.set_option(request.payload.u32(fields::f57),
                        request.payload.boolean(fields::f7));
        return reply_true(seq);
    }
    case wire::op_key:
        return run_step_op(seq, [&core, &request](std::string& error) {
            return core.press_key(request.payload.u32(fields::f58), error);
        });
    case wire::op_wheel:
        return run_step_op(seq, [&core, &request](std::string& error) {
            return core.wheel(request.payload.i32(fields::f59), error);
        });
    case wire::op_movie:
    {
        const auto movie = core.current_movie();
        wire::writer body;
        body.str(fields::f68, movie.path);
        body.u32(fields::f69, static_cast<std::uint32_t>(movie.serial));
        body.f64(fields::movie_audio_gain, movie.audio_gain);
        return reply_ok(seq, body);
    }
    case wire::op_movie_done:
        return run_step_op(seq, [&core](std::string& error) {
            return core.finish_movie(error);
        });
    case wire::op_prompt_dialog:
    {
        // The port has no native prompt dialog to describe; Active=false makes the
        // host fall back to its own name dialog, which is the Go behaviour for a
        // brand without PromptDialogExposer.
        wire::writer body;
        body.boolean(fields::f1, false);
        return reply_ok(seq, body);
    }
    case wire::op_prompt_text:
        // Text rendering belongs to the host here, so there is nothing to forward.
        return reply_true(seq);
    case wire::op_scan:
        // The library scanner lives above the wire layer in Go; nothing below it can
        // enumerate a game library, so this is an unimplemented op, not an empty one.
        return reply_bad_request(seq);
    // Gallery/map/select/title interfaces have no backend in this port. Go answers
    // exactly this way for a brand that does not implement them (status OK, no
    // payload), and the host reads it as "nothing to draw". A bad request here would
    // throw inside the host's frame sync and stall the player instead of degrading.
    case wire::op_map_buttons:
    case wire::op_map_mask:
    case wire::op_select_items:
    case wire::op_select_frame:
    case wire::op_compose_images:
    case wire::op_set_preview:
    case wire::op_slot_preview:
    case wire::op_cg_catalog:
    case wire::op_title_menu:
    case wire::op_title_select:
        return reply_empty(seq);
    default:
        // Reserved or unknown numbers: only a client bug can produce them.
        return reply_bad_request(seq);
    }
}

} // namespace

struct engine_core::impl
{
    // One lock for every op, compose included: the port's state machine was not
    // written for concurrent callers, and destroy must wait for an in-flight compose.
    std::mutex mutex;
    std::unique_ptr<krkr::engine> core;
};

engine_core::engine_core() : impl_(std::make_unique<impl>())
{
}

engine_core::~engine_core() = default;

std::vector<std::uint8_t> engine_core::invoke(std::span<const std::uint8_t> request)
{
    try
    {
        const std::scoped_lock guard(impl_->mutex);
        if (request.empty())
        {
            return wire::build_response(wire::status_bad_request, 0, {});
        }
        const auto parsed = wire::parse_request(request);
        if (!parsed)
        {
            // seq is unknown on a malformed header, exactly as in the Go parser.
            return wire::build_response(wire::status_bad_request, 0, {});
        }
        return dispatch(impl_->core, *parsed);
    }
    catch (const std::exception&)
    {
        // Letting an exception cross the FFI boundary terminates the host process;
        // the Go core would never do that, so a failure becomes an error reply.
        return wire::build_error(0, "engine panic");
    }
    catch (...)
    {
        return wire::build_error(0, "engine panic");
    }
}

std::string engine_core::version() const
{
    // Same literal KzEngineVersion reports: two different strings for one binary
    // would make a stale native core indistinguishable in the about dialog.
    return "kizuna-enginec/1.0.0";
}

} // namespace enginec
