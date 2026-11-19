#pragma once

#include <string>
#include <vector>

// ---------------------------------------------------------------------------
// Extra resource roots (host-provided, cross-platform)
// ---------------------------------------------------------------------------
// The wire protocol carries `extraDirs` (op_open f7): multi-disc installs keep
// scripts on one disc and graphics/voice on another, and the library scanner
// reports every companion directory. enginego feeds them into its resource
// index; the KiriKiri port has to translate them into the engine's own concept,
// which is the auto-path list (the same mechanism Initialize.tjs uses for
// "bgm/", "scenario/", …).
//
// They must be appended BEFORE TVPInitializeStartupScript, otherwise the game's
// first storage lookups miss the companion discs. The SDL entry point runs the
// whole startup sequence internally, so the adapter publishes the list here and
// sdl3_app.cpp drains it right after TVPParseArguments.
inline std::vector<std::string> TVPHostExtraRoots;

// Appends every queued root to the engine's auto-path list and clears the queue.
// Called by the SDL entry point after argument parsing, before the startup script.
void TVPHostMountExtraRoots();
