#pragma once

#include "TVPWaveManager.h"

//---------------------------------------------------------------------------
// Host-delegated silent wave decoder
//---------------------------------------------------------------------------
// Why: this build has no Vorbis/MP3/Opus decoder compiled in (see the plugin
// list in third_party/krkr_engine/CMakeLists.txt) and the sound device is SDL's
// dummy one; the real audio file is materialized and played by the host (Flutter)
// from the PlatformAudioHook events. The engine-side decode is a formality that
// only has to keep the wave-sound-buffer state machine (Open/length/position/
// loop and the Play/Stop events) consistent with what the host is doing.
//
// Without this decoder TVPCreateWaveDecoder throws TVPUnknownWaveFormat from
// tTJSNI_WaveSoundBuffer::Open, so the game dies before Play ever reports the
// sound to the host. The creator below answers for every storage and extension
// with silence, which makes the "host owns playback" design work for every
// container (and for ports whose audio files have been trimmed away).
//
// Reported format/duration is best effort (only the RIFF header of ".wav" is
// parsed; parsing it is cheap and the file was already rejected by the strict
// RIFF decoder). Everything else is reported as 44100Hz/16bit/stereo with an
// unknown (zero) length.

class tTVPWD_HostSilent : public tTVPWaveDecoder
{
public:
    tTVPWD_HostSilent(const ttstr& storagename, const ttstr& extension);

    void GetFormat(tTVPWaveFormat& format) override;
    bool Render(void* buf, tjs_uint bufsamplelen, tjs_uint& rendered) override;
    bool SetPosition(tjs_uint64 samplepos) override;

private:
    tTVPWaveFormat Format;
    tjs_uint64 Position;
};

class tTVPWDC_HostSilent : public tTVPWaveDecoderCreator
{
public:
    tTVPWaveDecoder* Create(const ttstr& storagename, const ttstr& extension) override;
};

