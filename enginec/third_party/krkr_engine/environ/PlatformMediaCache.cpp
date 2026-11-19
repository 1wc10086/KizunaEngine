#include "tjsCommHead.h"

#include "PlatformMediaCache.h"

#include "Platform.h"
#include "PlatformFile.h"
#include "TVPStorage.h"
#include "UtilStreams.h"

#include <algorithm>
#include <array>
#include <cctype>
#include <cstdint>
#include <cstdio>
#include <filesystem>
#include <fstream>
#include <mutex>
#include <string>
#include <system_error>
#include <vector>

// ---------------------------------------------------------------------------
// Local media materialisation for host-delegated playback
// ---------------------------------------------------------------------------
//
// The previous implementation (one copy per hook) had two defects that made every
// sound silent on a normal XP3 install:
//
//   1. It called TVPGetLocallyAccessibleName() with the *raw* storage name. That
//      helper is a "walk the path and fix case" routine: for "bgm/title.ogg" it
//      returns what it managed to resolve before the first missing component.
//      A loose file works by accident; an XP3 entry resolves to a path that does
//      not exist. The old code then handed that path to the host, whose decoder
//      failed, and - because a failed load is silent by design - the game simply
//      had no audio.
//   2. The extracted copy was written non-atomically into the *host* cache with a
//      hash-only name. Two hooks materialising the same storage concurrently
//      (BGM start + SE start on the first frame) could read a half-written file,
//      and the extension was derived from the raw name, so a name without one
//      produced a file the host could not probe.
//
// The sequence below mirrors what upstream does for its own temp files
// (tTVPLocalTempStorageHolder): resolve through the storage system, and when the
// storage only exists inside an archive, copy it out with a stream holder. The
// difference is that the copy is atomic and named deterministically, so repeated
// requests for the same sample hit the cache instead of re-extracting.
//
// Which helper belongs where:
//   * TVPGetPlacedPath   - storage-system lookup (folders, XP3, auto-path). Use it
//                          to learn whether the name is resolvable at all.
//   * TVPGetLocallyAccessibleName - file://-only mapping. It answers "" for an
//                          in-archive name, which is exactly the signal needed to
//                          choose "copy out" over "hand out the path".
//   * TVPCheckExistentLocalFile  - the mapped path must really be a file; the
//                          mapping can return a prefix for a partially matching
//                          path.

namespace {

// tjs_char is `char` holding UTF-8 on every platform this port targets, but
// std::filesystem::path interprets a narrow string as the *native* encoding on
// Windows. These two helpers keep the boundary explicit so a Japanese game path
// survives on all five platforms and the returned string is always UTF-8 for the
// Dart side.
std::filesystem::path path_from_utf8(std::string_view text)
{
#if defined(_WIN32)
    return std::filesystem::path(
        std::u8string(reinterpret_cast<const char8_t*>(text.data()), text.size()));
#else
    return std::filesystem::path(std::string(text));
#endif
}

std::string path_to_utf8(const std::filesystem::path& path)
{
#if defined(_WIN32)
    const std::u8string text = path.u8string();
    return std::string(reinterpret_cast<const char*>(text.data()), text.size());
#else
    return path.string();
#endif
}

std::mutex g_cache_mutex;
std::string g_cache_dir;

// FNV-1a. Used only to derive a stable file name, so collisions are not a
// security boundary; the length check below keeps them from mattering anyway.
std::uint64_t fnv1a(std::string_view text)
{
    std::uint64_t hash = 1469598103934665603ull;
    for (unsigned char ch : text)
    {
        hash ^= ch;
        hash *= 1099511628211ull;
    }
    return hash;
}

// Extension of a storage name, lower-cased and sanitised: the host picks its
// decoder from this, so it must survive into the cache file name. Anything that is
// not a short ASCII alphanumeric run is dropped rather than escaped - a query
// string or a stray character would otherwise end up spelled into a file name.
std::string sanitized_extension(std::string_view storage)
{
    const std::string_view name = storage.substr(storage.find_last_of("/\\") + 1);
    const auto dot = name.find_last_of('.');
    if (dot == std::string_view::npos || dot + 1 >= name.size())
        return {};
    // Hand-rolled ASCII check/lowering: Android's bionic provides tolower both as
    // an inline in <ctype.h> and in namespace std, which makes std::tolower
    // ambiguous under the NDK's libc++ (a hard error, not a warning).
    std::string ext;
    for (std::size_t i = dot + 1; i < name.size() && ext.size() < 10; ++i)
    {
        char ch = name[i];
        if (ch >= 'A' && ch <= 'Z')
            ch = static_cast<char>(ch - 'A' + 'a');
        else if (!((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')))
            break;
        ext.push_back(ch);
    }
    return ext.empty() ? std::string() : "." + ext;
}

// Absolute cache root, created on demand. The host's directory is used when it
// provided one; otherwise the system temp directory gets a per-process folder.
// Returning a path (never throwing) keeps the callers free of try/catch noise.
std::filesystem::path cache_root(std::string_view kind)
{
    std::filesystem::path root;
    {
        std::lock_guard<std::mutex> lock(g_cache_mutex);
        if (!g_cache_dir.empty())
            root = path_from_utf8(g_cache_dir);
    }
    if (root.empty())
        root = std::filesystem::temp_directory_path() / "kizuna-media";
    root /= std::string(kind);
    std::error_code ec;
    std::filesystem::create_directories(root, ec);
    return root;
}

// Copies the whole storage into `target` and renames it into place, so a
// concurrent reader either sees the finished file or nothing at all. A partial
// file left behind by a crash is removed on failure.
bool copy_storage_to_file(const ttstr& storage, const std::filesystem::path& target)
{
    std::filesystem::path partial = target;
    partial += ".part";
    std::error_code ec;
    std::filesystem::remove(partial, ec);
    {
        std::ofstream out(partial, std::ios::binary | std::ios::trunc);
        if (!out)
            return false;
        try
        {
            tTVPStreamHolder stream(storage, TJS_BS_READ);
            if (stream.Get() == nullptr)
            {
                out.close();
                std::filesystem::remove(partial, ec);
                return false;
            }
            std::array<char, 128 * 1024> buffer{};
            for (;;)
            {
                const tjs_uint read =
                    stream->Read(buffer.data(), static_cast<tjs_uint>(buffer.size()));
                if (read == 0)
                    break;
                out.write(buffer.data(), static_cast<std::streamsize>(read));
                if (!out)
                    break;
            }
            // Close the archive entry before renaming: on some platforms an open
            // handle keeps the temp file locked.
            stream.Close();
        }
        catch (...)
        {
            out.close();
            std::filesystem::remove(partial, ec);
            return false;
        }
        out.flush();
        if (!out)
        {
            out.close();
            std::filesystem::remove(partial, ec);
            return false;
        }
    }
    std::filesystem::rename(partial, target, ec);
    if (ec)
    {
        std::filesystem::remove(partial, ec);
        return false;
    }
    return true;
}

} // namespace

void TVPSetHostMediaCacheDir(const std::string& directory)
{
    std::lock_guard<std::mutex> lock(g_cache_mutex);
    g_cache_dir = directory;
}

std::string TVPGetHostMediaCacheDir()
{
    std::lock_guard<std::mutex> lock(g_cache_mutex);
    return g_cache_dir;
}

std::string TVPMaterializeHostMedia(std::string_view storage, std::string_view kind)
{
    if (storage.empty())
        return {};

    // Nothing here may throw: the audio hook runs while the engine holds a
    // sound-buffer lock and the movie overlay runs inside the script, so an
    // escaping storage/filesystem exception would either drop a request or take
    // the game down. Every failure path returns "" (unplayable).
    try
    {
        // Scripts sometimes pass a name with a query suffix ("movie.mpg?loop") and
        // sometimes a bare name the auto-path search has to resolve. Normalising
        // first makes the lookup and the cache key canonical.
        ttstr name(storage.data(), static_cast<int>(storage.size()));
        name = TVPNormalizeStorageName(name);

        // Fast path: a loose file is handed out as-is. Normalisation turned a
        // script-relative name ("bgm/x.ogg") into an absolute one under the
        // project directory, which is what makes the file media able to map it.
        // The existence check matters: for an in-archive entry this answers "" and
        // for a partially matching path it can answer a prefix, so "non-empty"
        // alone is not a usable signal.
        const ttstr local = TVPGetLocallyAccessibleName(name);
        if (!local.IsEmpty() && TVPCheckExistentLocalFile(local))
            return local.AsStdString();

        // In-archive (or unmapped) storage: verify it is resolvable, then copy it
        // out once. TVPGetPlacedPath also searches the auto-path table, which is
        // how multi-disc installs find samples on a companion root.
        if (TVPGetPlacedPath(name).IsEmpty())
            return {};

        const std::filesystem::path root = cache_root(kind);
        if (root.empty())
            return {};

        // The key includes the game's own root: sample names are only unique inside
        // one title ("bgm/01.ogg" exists in almost every game), and the cache
        // directory is shared by every game the player opens.
        char hex[40];
        std::snprintf(hex, sizeof(hex), "%016llx",
                      static_cast<unsigned long long>(fnv1a(
                          TVPProjectDir.AsStdString() + name.AsStdString())));
        const std::filesystem::path target =
            root / (std::string(hex) + sanitized_extension(storage));

        std::error_code ec;
        if (std::filesystem::is_regular_file(target, ec) && !ec)
            return path_to_utf8(target);

        if (!copy_storage_to_file(name, target))
        {
            // A concurrent request may have won the rename; the finished copy is
            // just as good as ours, so accept it instead of reporting "unplayable".
            if (!std::filesystem::is_regular_file(target, ec) || ec)
                return {};
        }
        return path_to_utf8(target);
    }
    catch (...)
    {
        return {};
    }
}
