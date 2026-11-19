#pragma once

#include "tjsCommHead.h"

struct tTVPFont;
class tTVPCharacterData;

//---------------------------------------------------------------------------
// Host glyph rasterization (environ/PlatformTextHook.h)
//---------------------------------------------------------------------------
// Why: the engine keeps text layout and composition, but the glyph bitmap comes
// from the platform text stack so font availability/hinting quality are the
// platform's instead of a per-game font bundle. FreeTypeFontRasterizer calls this
// before FreeType; a null/false hook (or an unusable reply) means "render with
// FreeType as before", so standalone tools and the first frames of a warm-up
// behave exactly like the original rasterizer.
//
// Returns a new character data owned by the caller, or nullptr to fall back.
// `ascent` is the engine's ascent for the current face (pixels from the ascent
// line to the baseline); it places the host's line box against the engine's
// baseline and is what the engine uses for its own glyphs. `advance` is the
// engine's advance for the same glyph and is preferred over the host's: the
// engine measures text with its own metrics (GetTextExtent), so the drawn
// advance has to match or a centred/wrapped line drifts.
[[nodiscard]] tTVPCharacterData* TVPGetHostGlyph(const tTVPFont& font,
                                                 tjs_wchar ch,
                                                 bool antialiased,
                                                 tjs_int ascent,
                                                 tjs_int advance);
