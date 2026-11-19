// zlib-ng wrapper. The engine wraps zlib once so XP3 segment decompression and
// save-container compression never see z_stream or Bytef.
export module enginec.third_party.zlib;

import std;

export namespace enginec::third_party::zlib {

enum class wrapper : std::uint8_t { raw, zlib_stream, gzip };
enum class status : std::uint8_t { ok, need_more, finished, error };

// Decompresses a complete buffer. `expected` is an upper bound when the caller
// knows the original size (XP3 index); zero means "grow dynamically".
std::optional<std::vector<std::uint8_t>> decompress(std::span<const std::uint8_t> input,
                                                    wrapper wrap,
                                                    std::size_t expected,
                                                    std::string& error);

std::vector<std::uint8_t> compress(std::span<const std::uint8_t> input, wrapper wrap,
                                   int level, std::string& error);

// Detects the container from the first bytes (gzip magic / zlib header check).
wrapper sniff(std::span<const std::uint8_t> prefix) noexcept;

} // namespace enginec::third_party::zlib
