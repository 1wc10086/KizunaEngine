// Engine facade consumed by the C ABI.
//
// One engine_core owns one game. invoke() is the whole RPC surface: it parses the
// wire request, dispatches to the KiriKiri port and returns a wire response. The
// class is deliberately the only place that knows both the protocol and the
// engine, which is what keeps ffi/ a ~100-line pointer adapter with no logic of
// its own (the Go implementation has the same split between cmd/engine and capi).
export module enginec.feature.engine;

import std;

import enginec.feature.wire;

export namespace enginec {

class engine_core {
public:
    engine_core();
    ~engine_core();
    engine_core(const engine_core&) = delete;
    engine_core& operator=(const engine_core&) = delete;

    // Thread safety: the Flutter bridge calls this from two isolates (logic and
    // compose). The Go core serializes them with two locks; this core uses one
    // mutex because compose is cheap enough in C++ and a single lock makes
    // create/destroy unambiguous (destroy waits for an in-flight compose, which is
    // the use-after-free the two-worker design otherwise invites).
    std::vector<std::uint8_t> invoke(std::span<const std::uint8_t> request);

    [[nodiscard]] std::string version() const;

private:
    struct impl;
    std::unique_ptr<impl> impl_;
};

} // namespace enginec
