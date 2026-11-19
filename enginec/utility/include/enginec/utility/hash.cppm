// MD5, matching the Go engine's game cache directory naming.
export module enginec.utility.hash;

import std;

export namespace enginec {

std::array<std::uint8_t, 16> md5(std::span<const std::uint8_t> data);
std::string md5_hex(std::string_view text);

} // namespace enginec
