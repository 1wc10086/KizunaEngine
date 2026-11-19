// Wire-level smoke test: drives engine_core exactly like the Flutter bridge.
//
// The krkr smoke test validates the engine; this one validates the protocol layer
// (header status/seq, field ids, the raw pointer handed to OpCompose, and the
// frame/state/text fields Dart parses). A field-number drift is invisible to a
// compile and would only surface as a blank screen on a phone.
import std;

import enginec;

namespace {

int failures = 0;

void check(bool condition, const char* what) {
    if (!condition) {
        std::cerr << "wire smoke: FAILED: " << what << "\n";
        ++failures;
    }
}

std::vector<std::uint8_t> request(std::uint16_t op, std::uint32_t seq,
                                  const std::vector<std::uint8_t>& payload = {}) {
    std::vector<std::uint8_t> out(16, 0);
    out[0] = 'K'; out[1] = 'Z'; out[2] = 'E'; out[3] = 'Q';
    out[4] = static_cast<std::uint8_t>(op);
    out[5] = static_cast<std::uint8_t>(op >> 8);
    out[8] = static_cast<std::uint8_t>(seq);
    out[12] = static_cast<std::uint8_t>(payload.size());
    out.insert(out.end(), payload.begin(), payload.end());
    return out;
}

} // namespace

int main() {
    namespace fs = std::filesystem;
    namespace wire = enginec::wire;

    const auto root = fs::temp_directory_path() / "enginec-wire-smoke";
    std::error_code ignored;
    fs::remove_all(root, ignored);
    fs::create_directories(root);
    {
        std::ofstream script(root / "startup.tjs", std::ios::binary);
        // The real engine loads startup.tjs (not start.tjs); a Window keeps the
        // application alive so the host can compose frames.
        std::ofstream probe(root / "start.tjs", std::ios::binary);
        probe << "// probe marker\n";
        script << "var win = new Window();\n"
                  "win.setSize(640, 480);\n"
                  "var layer = new Layer(win, null);\n"
                  "win.add(layer);\n"
                  "layer.setSize(640, 480);\n"
                  "layer.fillRect(0, 0, 640, 480, 0xff0000);\n"
                  "win.visible = true;\n";
    }

    enginec::engine_core core;

    wire::writer open_payload;
    open_payload.str(wire::field::f1, root.string());
    open_payload.str(wire::field::f10, root.string());
    auto response = core.invoke(request(wire::op_open, 1, open_payload.data()));
    // Responses share the TLV shape but with the KZES magic; parse_request rejects
    // them by design, so the header is decoded manually here.
    check(!wire::parse_request(response).has_value(),
          "parse_request rejects a response");
    check(response.size() >= 16 && response[0] == 'K' && response[3] == 'S', "response magic");
    check(response.size() >= 6 && (response[4] | response[5] << 8) == wire::status_ok,
          "open status ok");

    auto payload_of = [](const std::vector<std::uint8_t>& frame) {
        const std::size_t size = frame.size() >= 16
            ? (frame[12] | frame[13] << 8 | frame[14] << 16 | frame[15] << 24) : 0;
        return std::span<const std::uint8_t>(frame).subspan(16, size);
    };

    response = core.invoke(request(wire::op_start, 2, {}));
    wire::reader start_reader(payload_of(response));
    // OpStart replies with f1=true only (the Go capi does the same); the first
    // await comes from the following OpRun.
    check(start_reader.boolean(wire::field::f1), "start ok");
    response = core.invoke(request(wire::op_run, 21,
                                   [] { wire::writer w; w.u32(wire::field::f9, 4096);
                                        return w.data(); }()));
    wire::reader run_reader(payload_of(response));
    check(run_reader.u32(wire::field::f10) == 0 ||
              run_reader.u32(wire::field::f10) == 5,
          "run returns a live step");

    response = core.invoke(request(wire::op_frame, 3, {}));
    wire::reader frame_reader(payload_of(response));
    // await_movie (9) is legitimate: the script can park on a host-played movie.
    check(frame_reader.u32(wire::field::f10) <= 9, "frame reports the step");
    // Real KiriKiri paints text into the frame; TextState stays empty.
    check(frame_reader.str(wire::field::f31).empty(), "text is not duplicated");

    std::vector<std::uint8_t> pixels(1280 * 720 * 4, 0);
    wire::writer compose_payload;
    compose_payload.u64(wire::field::f15, reinterpret_cast<std::uint64_t>(pixels.data()));
    compose_payload.u32(wire::field::f16, static_cast<std::uint32_t>(pixels.size()));
    response = core.invoke(request(wire::op_compose, 4, compose_payload.data()));
    wire::reader compose_reader(payload_of(response));
    check(response.size() >= 16 && (response[4] | response[5] << 8) == wire::status_ok,
          "compose status ok");
    // The engine may not have presented yet; status and a valid response matter.
    check(response.size() >= 16, "compose answers");

    response = core.invoke(request(wire::op_advance, 5, {}));
    check(response.size() >= 6 && (response[4] | response[5] << 8) == wire::status_ok,
          "advance status ok");

    // Asset ops go through the engine's storage layer. A name that is not there is
    // answered with size 0 (not an error), and a zero destination must never be
    // dereferenced. These keep the frame/step fields out of the assertion so a game
    // fixture without assets can still run the test.
    wire::writer asset_payload;
    asset_payload.str(wire::field::f1, "no-such-asset.bin");
    response = core.invoke(request(wire::op_asset_size, 6, asset_payload.data()));
    wire::reader asset_reader(payload_of(response));
    check(response.size() >= 16 && (response[4] | response[5] << 8) == wire::status_ok,
          "asset size status ok");
    check(asset_reader.u32(wire::field::f25) == 0, "missing asset has size 0");

    response = core.invoke(request(wire::op_read_asset, 7, asset_payload.data()));
    wire::reader read_reader(payload_of(response));
    check(response.size() >= 16 && (response[4] | response[5] << 8) == wire::status_ok,
          "read asset status ok");
    check(read_reader.u32(wire::field::f25) == 0, "null destination writes nothing");

    // No script in the fixture asks for a movie: the op answers with an empty path
    // instead of failing, which is what the host polls every frame.
    response = core.invoke(request(wire::op_movie, 8, {}));
    wire::reader movie_reader(payload_of(response));
    check(response.size() >= 16 && (response[4] | response[5] << 8) == wire::status_ok,
          "movie status ok");
    check(movie_reader.str(wire::field::f68).empty(), "no pending movie");
    // The host dedupes on f69, so "no request" must answer 0 rather than a serial
    // left over from a finished movie.
    check(movie_reader.u32(wire::field::f69) == 0, "no pending movie serial");

    fs::remove_all(root, ignored);
    if (failures == 0) {
        std::cout << "enginec wire smoke passed\n";
    }
    return failures == 0 ? 0 : 1;
}
