#include "tjsCommHead.h"

#include "PlatformHostPaths.h"

#include "Platform.h"
#include "TVPStorage.h"

// ---------------------------------------------------------------------------
// Host-provided extra resource roots
// ---------------------------------------------------------------------------
// The adapter (third_party/src/krkr.cpp) fills TVPHostExtraRoots from op_open's
// extraDirs before SDL_AppInit runs; sdl3_app.cpp calls the drain below right
// after argument parsing, i.e. before the game's startup script executes.
//
// Two things matter here:
//   * TVPAddAutoPath throws on a missing trailing delimiter, so the normalized
//     root is appended with one;
//   * a companion root that does not exist must be skipped, not thrown: the
//     scanner can report a directory the user later moved away, and a throw here
//     would abort startup (same failure class as "Cannot find storage
//     startup.tjs").
void TVPHostMountExtraRoots()
{
    if (TVPHostExtraRoots.empty())
        return;

    for (const std::string& root : TVPHostExtraRoots)
    {
        if (root.empty())
            continue;
        std::string normalized = root;
        // A companion root can be an archive (Disk3/voice.xp3) or a folder. The
        // engine marks archives by a trailing '>' (Initialize.tjs does the same
        // for patch.xp3); folders take a trailing slash. TVPAddAutoPath throws on
        // neither, so both forms have to be built here.
        auto lower = [](std::string text)
        {
            for (char& c : text)
            {
                if (c >= 'A' && c <= 'Z')
                    c = static_cast<char>(c - 'A' + 'a');
            }
            return text;
        };
        const std::string lowered = lower(normalized);
        if (lowered.size() >= 4 && lowered.compare(lowered.size() - 4, 4, ".xp3") == 0)
        {
            normalized.push_back('>');
        }
        else if (normalized.back() != '/' && normalized.back() != '\\')
        {
            normalized.push_back('/');
        }
        try
        {
            // addAutoPath semantics: files under this root become resolvable by
            // their bare name (the engine's "current folder" lookup is tried first,
            // so the primary root keeps priority).
            TVPAddAutoPath(ttstr(normalized));
        }
        catch (...)
        {
            // Unreadable/missing companion disc: the primary root still works.
        }
    }
    TVPHostExtraRoots.clear();
}
