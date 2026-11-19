#pragma once
// Host media materialisation (audio samples and movies).
//
// Why this exists as its own module: the engine does not decode sound or video in
// this build. The Flutter host does (flutter_soloud / fvp), and it can only open
// *local files*. A krkr game keeps almost everything inside data.xp3, so before a
// playback request crosses the wire it has to be turned into a real path. Two
// subsystems need exactly that - the wave-sound-buffer hook
// (environ/PlatformAudioHook.h) and the video overlay (environ/PlatformMovieHook.h)
// - and they used to carry two slightly different copies of the logic. One copy
// was wrong in a way that silently killed every sound (see the comment on
// TVPMaterializeHostMedia).
//
// Rules this file implements:
//   * a loose file is handed out as-is (no copy, the game directory is already
//     readable by the host);
//   * an archive entry is copied once into the host cache directory, atomically,
//     under a content-addressed name that keeps a usable extension;
//   * nothing here may throw: the callers run inside the engine's playback and
//     script threads, where an exception would either drop a request or take the
//     game down. A failure returns an empty string and the host reports "nothing
//     to play".
//
// Threading: the engine calls this from the script thread (audio hook) and the
// playback thread (movie overlay). The adapter also reads the cache directory
// during open(). All entry points are internally synchronised.
#include <string>
#include <string_view>

// Cache directory the host handed over in op_open (for example
// <app>/cache/krkr/<game>). Empty means "use the system temp directory".
void TVPSetHostMediaCacheDir(const std::string& directory);

// Current cache directory (empty when the host did not provide one).
std::string TVPGetHostMediaCacheDir();

// Resolves `storage` (as written by the game, e.g. "bgm/title.ogg" or
// "file://./voice/001.ogg", loose or inside an XP3) to a local path the host can
// open. `kind` groups the extracted copy ("audio", "movie") and must be a short
// ASCII token. Returns an empty string when the storage cannot be materialised.
std::string TVPMaterializeHostMedia(std::string_view storage, std::string_view kind);
