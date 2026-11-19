// Process-wide log sink. The Go engine writes <cache>/kizuna/<md5>/engine.log; the
// C++ core appends to the same file when it is given a cache directory, so a
// failure on a phone produces one log instead of two disconnected traces.
export module enginec.utility.log;

import std;

export namespace enginec {

void set_log_file(std::string path);
void log_info(std::string_view message);
void log_error(std::string_view message);
std::string log_path();

} // namespace enginec
