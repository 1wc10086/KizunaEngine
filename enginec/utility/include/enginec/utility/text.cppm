// UTF conversion helpers used by paths, save metadata and the wire protocol.
export module enginec.utility.text;

import std;

export namespace enginec {

std::string utf16le_to_utf8(std::span<const std::uint8_t> data);
std::vector<std::uint8_t> utf8_to_utf16le(std::string_view text);
std::string cp932_to_utf8(std::string_view data);
std::string utf8_to_cp932(std::string_view data);
std::string trim(std::string_view text);
std::string ascii_lower(std::string_view text);
bool equals_ignore_case(std::string_view a, std::string_view b);
// Replaces '\\' with '/' and lowercases ASCII, matching KiriKiri's storage names.
std::string normalize_storage_name(std::string_view name);

} // namespace enginec
