// Process-wide log sink.
//
// The Go engine routes every line through one logger that owns an engine.log in the
// game's cache directory, and the Dart side reads that file when a game refuses to
// start. This module is the C++ half of that contract: one global sink, lines stamped
// with a wall-clock time, appended rather than truncated (a crash and the following
// retry must both survive in the file), flushed per line so a killed process leaves
// the last event behind, and echoed to stderr so host runs need no file at all.
//
// The mutex is held across the formatted write. Game threads log from asset loads and
// the FFI thread logs from dispatch, so interleaving must be impossible; per-line
// flushing makes the critical section one write(2), which costs nothing next to the
// decode work that triggered the log.

module enginec.utility.log;

import std;

namespace enginec {

namespace {

std::mutex& sink_mutex()
{
    static std::mutex mutex;
    return mutex;
}

// Guarded by sink_mutex(). The stream is kept out of the mutex object itself so the
// lock can be a plain std::lock_guard over both fields.
struct sink_state {
    std::string path;
    std::ofstream file;
};

sink_state& sink()
{
    static sink_state state;
    return state;
}

void append_two_digits(std::string& out, unsigned value)
{
    out.push_back(static_cast<char>('0' + (value / 10) % 10));
    out.push_back(static_cast<char>('0' + value % 10));
}

// Local time on purpose: a log is read next to the phone's clock and next to the Go
// engine's `15:04:05.000` prefix, not against a UTC reference.
//
// std::localtime is the C time function and fills one shared static std::tm, so its
// result is copied immediately while the caller holds sink_mutex(). Staying with the
// ISO C entry point keeps this translation unit free of platform headers; the copy is
// what makes the shared buffer harmless for our own threads.
std::string timestamp()
{
    const auto now = std::chrono::system_clock::now();
    const auto millis =
        std::chrono::duration_cast<std::chrono::milliseconds>(now.time_since_epoch()) % 1000;
    const auto seconds_point = std::chrono::floor<std::chrono::seconds>(now);
    const auto time_value = std::chrono::system_clock::to_time_t(seconds_point);

    const std::tm* shared = std::localtime(&time_value);
    if (shared == nullptr)
    {
        return "00:00:00.000";
    }
    const std::tm local = *shared;

    std::string out;
    out.reserve(12);
    append_two_digits(out, static_cast<unsigned>(local.tm_hour));
    out.push_back(':');
    append_two_digits(out, static_cast<unsigned>(local.tm_min));
    out.push_back(':');
    append_two_digits(out, static_cast<unsigned>(local.tm_sec));
    out.push_back('.');
    const auto millisecond_count = static_cast<unsigned>(millis.count());
    out.push_back(static_cast<char>('0' + (millisecond_count / 100) % 10));
    out.push_back(static_cast<char>('0' + (millisecond_count / 10) % 10));
    out.push_back(static_cast<char>('0' + millisecond_count % 10));
    return out;
}

// Creates the parent directory and opens the file for append. Truncating would erase
// the previous run's tail, which is exactly the part a bug report needs.
void open_file_locked(const std::string& path)
{
    auto& state = sink();
    state.path = path;
    if (state.file.is_open())
    {
        state.file.close();
    }
    if (path.empty())
    {
        return;
    }

    std::error_code ec;
    const std::filesystem::path native(path);
    if (native.has_parent_path())
    {
        std::filesystem::create_directories(native.parent_path(), ec);
        if (ec)
        {
            return;
        }
    }
    state.file.open(native, std::ios::binary | std::ios::app);
    if (!state.file)
    {
        state.file.clear();
    }
}

void emit_locked(std::string_view level, std::string_view message)
{
    std::string line;
    line.reserve(level.size() + message.size() + 24);
    line.append(timestamp());
    line.push_back(' ');
    line.append(level);
    line.push_back(' ');
    line.append(message);
    line.push_back('\n');

    auto& state = sink();
    if (state.file.is_open())
    {
        state.file.write(line.data(), static_cast<std::streamsize>(line.size()));
        // Flushing every line is the durability guarantee described at the top. A
        // failed write (full disk) leaves the stream in a fail state, and clearing it
        // keeps the following lines from being dropped without a trace.
        state.file.flush();
        if (!state.file)
        {
            state.file.clear();
        }
    }

    // std::cerr is written under the same mutex so a line cannot be split by another
    // thread between the file write and the console echo.
    std::cerr << line;
    std::cerr.flush();
}

void write_line(std::string_view level, std::string_view message)
{
    // set_log_file already opened the file, so nothing here touches the filesystem
    // beyond the append itself; a failed open leaves file closed and only stderr
    // carries the line, which is the documented degradation.
    const std::lock_guard lock(sink_mutex());
    emit_locked(level, message);
}

} // namespace

void set_log_file(std::string path)
{
    const std::lock_guard lock(sink_mutex());
    open_file_locked(path);
}

void log_info(std::string_view message)
{
    write_line("I", message);
}

void log_error(std::string_view message)
{
    write_line("E", message);
}

std::string log_path()
{
    const std::lock_guard lock(sink_mutex());
    return sink().path;
}

} // namespace enginec
