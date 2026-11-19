// Byte-level helpers shared by the wire codec and save containers.
export module enginec.utility.bytes;

import std;

export namespace enginec {

using bytes = std::vector<std::uint8_t>;

inline std::uint32_t read_u32(std::span<const std::uint8_t> data, std::size_t offset) noexcept {
    if (offset + 4 > data.size()) {
        return 0;
    }
    return static_cast<std::uint32_t>(data[offset]) |
           (static_cast<std::uint32_t>(data[offset + 1]) << 8) |
           (static_cast<std::uint32_t>(data[offset + 2]) << 16) |
           (static_cast<std::uint32_t>(data[offset + 3]) << 24);
}

inline std::uint64_t read_u64(std::span<const std::uint8_t> data, std::size_t offset) noexcept {
    return static_cast<std::uint64_t>(read_u32(data, offset)) |
           (static_cast<std::uint64_t>(read_u32(data, offset + 4)) << 32);
}

inline void append_u32(bytes& out, std::uint32_t value) {
    for (int shift = 0; shift < 32; shift += 8) {
        out.push_back(static_cast<std::uint8_t>(value >> shift));
    }
}

inline void append_u64(bytes& out, std::uint64_t value) {
    append_u32(out, static_cast<std::uint32_t>(value));
    append_u32(out, static_cast<std::uint32_t>(value >> 32));
}

inline void append_string(bytes& out, std::string_view text) {
    append_u32(out, static_cast<std::uint32_t>(text.size()));
    out.insert(out.end(), text.begin(), text.end());
}

} // namespace enginec
