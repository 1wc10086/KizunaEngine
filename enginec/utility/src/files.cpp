// Filesystem helpers built only on std::filesystem.
//
// Every failure path must be non-fatal and observable: a missing archive, an
// unwritable cache directory or a directory entry we are not allowed to read is a
// normal condition in a game engine (patches ship partially, users delete folders),
// so these functions answer with false/nullopt instead of throwing. std::filesystem's
// throwing overloads are never used because the error_code overloads also avoid the
// cost of an exception on the hot path of asset lookups.

module enginec.utility.files;

import std;

namespace enginec {

namespace {

namespace fs = std::filesystem;

bool is_separator(char character) noexcept
{
    return character == '/' || character == '\\';
}

// Drops trailing separators, but never the root itself: "/" and "C:/" must stay
// intact, otherwise a root path would become empty. Treating '\\' as a separator on
// every platform matches how KiriKiri scripts write storage paths, so a path taken
// from a .ks file yields the same parent on Android as it would on Windows.
std::string strip_trailing_separators(std::string path)
{
    std::size_t keep = 1;
    if (path.size() >= 2 && path[1] == ':')
    {
        keep = 2;
    }
    while (path.size() > keep && is_separator(path.back()))
    {
        path.pop_back();
    }
    return path;
}

// Civil date from a Unix timestamp, by the days-to-civil algorithm. Doing the
// arithmetic here instead of calling std::gmtime keeps this function free of the
// shared static buffer that makes the C time functions unsafe across threads.
struct civil_time {
    int year = 1970;
    unsigned month = 1;
    unsigned day = 1;
    unsigned hour = 0;
    unsigned minute = 0;
    unsigned second = 0;
};

civil_time break_down_utc(std::int64_t seconds_since_epoch)
{
    constexpr std::int64_t seconds_per_day = 86400;
    std::int64_t days = seconds_since_epoch / seconds_per_day;
    std::int64_t remainder = seconds_since_epoch % seconds_per_day;
    if (remainder < 0)
    {
        remainder += seconds_per_day;
        --days;
    }

    civil_time out;
    out.hour = static_cast<unsigned>(remainder / 3600);
    out.minute = static_cast<unsigned>((remainder % 3600) / 60);
    out.second = static_cast<unsigned>(remainder % 60);

    // Shift the epoch to 0000-03-01 so leap days land at the end of the cycle, then
    // undo the shift after splitting into 400-year blocks.
    days += 719468;
    const std::int64_t era = (days >= 0 ? days : days - 146096) / 146097;
    const auto day_of_era = static_cast<unsigned>(days - era * 146097);
    const unsigned year_of_era =
        (day_of_era - day_of_era / 1460 + day_of_era / 36524 - day_of_era / 146096) / 365;
    const unsigned day_of_year =
        day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    const unsigned month_index = (5 * day_of_year + 2) / 153;
    out.day = day_of_year - (153 * month_index + 2) / 5 + 1;
    out.month = month_index < 10 ? month_index + 3 : month_index - 9;
    out.year = static_cast<int>(year_of_era) + static_cast<int>(era) * 400 +
               (out.month <= 2 ? 1 : 0);
    return out;
}

void append_digits(std::string& out, unsigned value, unsigned width)
{
    // Fixed-width decimal without stream manipulators: the timestamp always needs
    // leading zeros, and <iomanip> is not part of what `import std;` guarantees.
    unsigned scale = 1;
    for (unsigned index = 1; index < width; ++index)
    {
        scale *= 10;
    }
    for (unsigned index = 0; index < width; ++index)
    {
        out.push_back(static_cast<char>('0' + (value / scale) % 10));
        scale /= 10;
    }
}

} // namespace

bool file_exists(const std::string& path)
{
    if (path.empty())
    {
        return false;
    }
    std::error_code ec;
    // A directory also satisfies exists(); callers asking for a file mean a regular
    // file, and treating a directory as readable data would produce a zero-length
    // read that looks like an empty asset.
    return fs::is_regular_file(fs::path(path), ec) && !ec;
}

bool directory_exists(const std::string& path)
{
    if (path.empty())
    {
        return false;
    }
    std::error_code ec;
    return fs::is_directory(fs::path(path), ec) && !ec;
}

std::optional<std::vector<std::uint8_t>> read_file(const std::string& path)
{
    if (path.empty())
    {
        return std::nullopt;
    }

    std::error_code ec;
    const fs::path native(path);
    if (!fs::is_regular_file(native, ec) || ec)
    {
        return std::nullopt;
    }

    const auto size = fs::file_size(native, ec);
    if (ec)
    {
        return std::nullopt;
    }
    // An asset larger than this cannot be a supported image or script and would only
    // exhaust the phone's heap when read in one piece.
    constexpr std::uintmax_t maximum_size = 512ull * 1024ull * 1024ull;
    if (size > maximum_size)
    {
        return std::nullopt;
    }

    std::ifstream input(native, std::ios::binary);
    if (!input)
    {
        return std::nullopt;
    }

    std::vector<std::uint8_t> out(static_cast<std::size_t>(size));
    if (!out.empty())
    {
        input.read(reinterpret_cast<char*>(out.data()), static_cast<std::streamsize>(out.size()));
        if (input.gcount() != static_cast<std::streamsize>(out.size()))
        {
            // The file shrank or the read failed mid-way; a partial asset is never
            // safe to hand to a decoder.
            return std::nullopt;
        }
    }
    return out;
}

bool write_file(const std::string& path, std::span<const std::uint8_t> data)
{
    if (path.empty())
    {
        return false;
    }

    std::error_code ec;
    const fs::path native(path);
    if (native.has_parent_path())
    {
        // Save slots and cache files land in directories created by the engine, but a
        // caller-supplied path may not exist yet; creating it here is what lets the
        // FFI write a log or a save with a single call.
        fs::create_directories(native.parent_path(), ec);
        if (ec)
        {
            return false;
        }
    }

    std::ofstream output(native, std::ios::binary | std::ios::trunc);
    if (!output)
    {
        return false;
    }
    if (!data.empty())
    {
        output.write(reinterpret_cast<const char*>(data.data()),
                     static_cast<std::streamsize>(data.size()));
    }
    output.flush();
    if (!output)
    {
        return false;
    }
    output.close();
    return static_cast<bool>(output);
}

bool make_directories(const std::string& path)
{
    if (path.empty())
    {
        return false;
    }
    std::error_code ec;
    const fs::path native(path);
    // create_directories reports false with no error when the directory exists, so
    // the result must come from the error code plus a re-check, not the return value.
    fs::create_directories(native, ec);
    if (ec)
    {
        return false;
    }
    return fs::is_directory(native, ec) && !ec;
}

std::vector<std::string> list_directory(const std::string& path)
{
    std::vector<std::string> out;
    if (path.empty())
    {
        return out;
    }

    std::error_code ec;
    // The error_code constructor and increment are used instead of the range-for
    // form because range-for calls the throwing increment: one unreadable entry would
    // otherwise take the whole listing (and the caller) down with an exception.
    fs::directory_iterator iterator(fs::path(path), ec);
    if (ec)
    {
        return out;
    }
    const fs::directory_iterator end;
    while (iterator != end)
    {
        // Names only (no directory prefix): callers join them with the directory they
        // already know, and archive indexes store exactly that shape.
        out.push_back(iterator->path().filename().string());
        iterator.increment(ec);
        if (ec)
        {
            break;
        }
    }
    // std::filesystem makes no ordering promise; a stable order keeps zone listings,
    // asset pickers and tests reproducible.
    std::sort(out.begin(), out.end());
    return out;
}

std::string join_path(std::string_view a, std::string_view b)
{
    if (a.empty())
    {
        return std::string(b);
    }
    if (b.empty())
    {
        return std::string(a);
    }
    // A component that is absolute replaces the base, which is how KiriKiri storage
    // paths are written when a script names an explicit file. The check is textual
    // rather than fs::path::is_absolute() so a Windows path read from a script is
    // still recognized on Android, where it would otherwise become a child of 'a'.
    const bool rooted = is_separator(b.front()) ||
                        (b.size() >= 2 && b[1] == ':' &&
                         ((b.front() >= 'A' && b.front() <= 'Z') ||
                          (b.front() >= 'a' && b.front() <= 'z')));
    if (rooted)
    {
        return std::string(b);
    }

    std::string out(a);
    if (!is_separator(out.back()))
    {
        out.push_back('/');
    }
    out.append(b);
    return out;
}

std::string parent_path(const std::string& path)
{
    if (path.empty())
    {
        return {};
    }
    std::string trimmed = strip_trailing_separators(path);
    const std::size_t last = trimmed.find_last_of("/\\");
    if (last == std::string::npos)
    {
        return {};
    }
    // "C:\x" -> "C:" and "/x" -> "/": the separator belongs to the parent, not to the
    // child, which is what fs::path::parent_path() reports and what the storage layer
    // re-joins.
    if (last == 0)
    {
        return std::string(1, trimmed[0]);
    }
    // A doubled separator leaves "a/" behind; strip it so the parent never ends in one.
    return strip_trailing_separators(trimmed.substr(0, last));
}

std::string filename(const std::string& path)
{
    if (path.empty())
    {
        return {};
    }
    // A trailing separator must not turn a directory path into an empty name: storage
    // code passes directory paths here to label an entry, and a naive substring after
    // the last separator would answer "" for "saves/".
    const std::string trimmed = strip_trailing_separators(path);
    const std::size_t last = trimmed.find_last_of("/\\");
    if (last == std::string::npos)
    {
        return trimmed;
    }
    return trimmed.substr(last + 1);
}

std::string current_time_iso8601()
{
    // Save slots display and re-parse this string, so it must be RFC 3339 in UTC
    // ("2026-11-12T12:34:56Z"): the Dart side sorts slots by it and the Go engine
    // writes time.RFC3339, which has the same shape. Seconds are enough because a
    // save's sub-second precision is meaningless to the UI.
    const auto now = std::chrono::system_clock::now();
    const auto seconds =
        std::chrono::duration_cast<std::chrono::seconds>(now.time_since_epoch()).count();
    const auto civil = break_down_utc(static_cast<std::int64_t>(seconds));

    std::string out;
    out.reserve(20);
    append_digits(out, static_cast<unsigned>(civil.year), 4);
    out.push_back('-');
    append_digits(out, civil.month, 2);
    out.push_back('-');
    append_digits(out, civil.day, 2);
    out.push_back('T');
    append_digits(out, civil.hour, 2);
    out.push_back(':');
    append_digits(out, civil.minute, 2);
    out.push_back(':');
    append_digits(out, civil.second, 2);
    out.push_back('Z');
    return out;
}

} // namespace enginec
