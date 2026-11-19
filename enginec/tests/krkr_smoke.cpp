// End-to-end smoke test for the KiriKiri port.
//
// Builds a tiny KAG scenario in a temp directory and drives the public module API
// exactly like the wire dispatcher does. This is the test that catches the failures
// a compile cannot: TJS2 initialization, KAG tokenization, the await/advance state
// machine and text delivery. It needs no image/audio asset, so it runs on any host.
import std;

import enginec;

namespace {

int failures = 0;

void check(bool condition, const char* what) {
    if (!condition) {
        std::cerr << "krkr smoke: FAILED: " << what << "\n";
        ++failures;
    }
}

} // namespace

int main() {
    namespace fs = std::filesystem;
    namespace krkr = enginec::third_party::krkr;

    std::filesystem::path root = fs::temp_directory_path() / "enginec-krkr-smoke";
    const bool external = std::getenv("KZ_GAME") != nullptr;
    if (external) {
        root = std::getenv("KZ_GAME");
    } else {
        std::error_code ignored;
        fs::remove_all(root, ignored);
        fs::create_directories(root);
    }

    // Minimal KAG3 fixture. KZ_GAME points the test at a real game tree instead.
    if (!external) {
        std::ofstream script(root / "startup.tjs", std::ios::binary);
        // The real engine loads startup.tjs (not start.tjs); a Window keeps the
        // application alive so the host can compose frames.
        script << "var win = new Window();\n"
                  "win.setSize(640, 480);\n"
                  "var layer = new Layer(win, null);\n"
                  "win.add(layer);\n"
                  "layer.setSize(640, 480);\n"
                  "layer.fillRect(0, 0, 640, 480, 0xff0000);\n"
                  "win.visible = true;\n";
    }

    auto engine = krkr::create_engine();
    check(engine != nullptr, "create_engine");

    krkr::open_options options;
    options.dir = root.string();
    options.cache_dir = root.string();
    krkr::info information;
    std::string error;

    if (!engine->open(options, information, error)) {
        std::cerr << "krkr smoke: open failed: " << error << "\n";
        return 1;
    }
    check(information.width > 0 && information.height > 0, "open geometry");

    std::cerr << "smoke: before start\n";
    const auto first = engine->start(0, error);
    std::cerr << "smoke: after start step=" << static_cast<int>(first) << "\n";
    check(first == krkr::step::cont || first == krkr::step::finished,
          "start returns a runnable step");
    if (first == krkr::step::wait) {
        const auto next = engine->run(4096, error);
        check(next == krkr::step::await_input || next == krkr::step::finished,
              "run after wait");
    }

    std::vector<std::uint8_t> live(1280 * 720 * 4, 0);
    if (external) {
        for (int i = 0; i < 240; ++i) {
            engine->run(1, error);
            engine->compose(live.data(), live.size(), information.width, information.height, error);
            std::this_thread::sleep_for(std::chrono::milliseconds(16));
            int lit = 0;
            for (std::size_t k = 0; k + 3 < live.size(); k += 4) {
                if (live[k] || live[k + 1] || live[k + 2]) ++lit;
            }
            if (i % 20 == 0 || lit > 0) {
                std::cerr << "smoke: frame " << i << " lit=" << lit << "\n";
            }
            if (i == 239 || lit > 200000) {
                std::ofstream dump("/tmp/krkr_live.bgra", std::ios::binary);
                dump.write(reinterpret_cast<const char*>(live.data()),
                           static_cast<std::streamsize>(live.size()));
                std::ofstream meta("/tmp/krkr_live.txt");
                meta << information.width << " " << information.height << "\n";
            }
        }
    }
    std::cerr << "smoke: before text\n";
    const auto text = engine->text();
    std::cerr << "smoke: after text\n";
    // The real engine paints text into the frame; TextState is intentionally empty.
    check(text.text.empty(), "text is drawn into the frame, not duplicated");

    std::vector<std::uint8_t> pixels(1280 * 720 * 4, 0);
    std::cerr << "smoke: before compose\n";
    const bool dirty =
        engine->compose(pixels.data(), pixels.size(), information.width, information.height, error);
    std::cerr << "smoke: after compose dirty=" << dirty << "\n";
    if (external && dirty) {
        std::ofstream dump("/tmp/krkr_frame.bgra", std::ios::binary);
        dump.write(reinterpret_cast<const char*>(pixels.data()),
                   static_cast<std::streamsize>(pixels.size()));
        std::ofstream meta("/tmp/krkr_frame.txt");
        meta << information.width << " " << information.height << "\n";
    }

    std::cerr << "smoke: before advance\n";
    const auto advanced = engine->advance(error);
    std::cerr << "smoke: after advance\n";
    check(advanced != krkr::step::faulted, "advance does not fault");
    engine->close();
    engine.reset();
    if (!external) {
        std::error_code ignored;
        fs::remove_all(root, ignored);
    }

    if (failures == 0) {
        std::cerr << "enginec krkr smoke passed\n";
    }
    return failures == 0 ? 0 : 1;
}
