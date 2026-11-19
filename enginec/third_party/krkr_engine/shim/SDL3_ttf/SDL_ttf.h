// Minimal SDL_ttf shim.
//
// The only consumer is environ/sdl3/sdl3_input.cpp's IME overlay. Flutter owns
// text input in this build, so the overlay is not used; stubbing the four calls
// avoids pulling HarfBuzz + a second font stack into the Android/iOS binary.
#pragma once

#include <SDL3/SDL.h>
#include <SDL3/SDL_events.h>
#include <SDL3/SDL_mouse.h>
#include <SDL3/SDL_render.h>
#include <SDL3/SDL_timer.h>
#include <SDL3/SDL_log.h>

struct TTF_Font;
using TTF_Font = struct TTF_Font;

inline bool TTF_Init(void) { return false; }
inline TTF_Font* TTF_OpenFont(const char*, float) { return nullptr; }
inline void TTF_CloseFont(TTF_Font*) {}
inline SDL_Surface* TTF_RenderText_Blended(TTF_Font*, const char*, size_t, SDL_Color)
{
    return nullptr;
}
