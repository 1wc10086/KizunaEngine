// Filesystem helpers (POSIX + Win32 through std::filesystem).
export module enginec.utility.files;

import std;

export namespace enginec {

bool file_exists(const std::string& path);
bool directory_exists(const std::string& path);
std::optional<std::vector<std::uint8_t>> read_file(const std::string& path);
bool write_file(const std::string& path, std::span<const std::uint8_t> data);
bool make_directories(const std::string& path);
std::vector<std::string> list_directory(const std::string& path);
std::string join_path(std::string_view a, std::string_view b);
std::string parent_path(const std::string& path);
std::string filename(const std::string& path);
std::string current_time_iso8601();

} // namespace enginec
