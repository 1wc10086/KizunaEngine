#include "tjsCommHead.h"

#include "HostSilentWaveDecoder.h"

#include "UtilStreams.h"

#include <algorithm>
#include <array>
#include <cstddef>
#include <cstdint>
#include <cstring>
#include <utility>

namespace {

constexpr unsigned default_samples_per_sec = 44100;
constexpr unsigned default_channels = 2;
constexpr unsigned default_bits_per_sample = 16;

// Safety valve for containers whose length is unknown: after an hour of silence
// the stream reports end-of-stream. The host plays the real file on its own
// timeline; this only keeps a buffer from decoding silence forever when the
// script never stops it.
constexpr tjs_uint64 max_unknown_silence_samples = 3600ull * default_samples_per_sec;

// RIFF WAVE format tags we care about (the rest is reported as generic PCM).
enum class wave_tag : std::uint16_t
{
    pcm = 0x0001,
    ieee_float = 0x0003,
    extensible = 0xfffe,
};

struct wav_format
{
    unsigned samples_per_sec = default_samples_per_sec;
    unsigned channels = default_channels;
    unsigned bits_per_sample = default_bits_per_sample;
    unsigned bytes_per_sample = default_bits_per_sample / 8;
    unsigned block_align = default_channels * (default_bits_per_sample / 8);
    tjs_uint64 total_samples = 0;
    bool is_float = false;
};

// Best-effort RIFF walk. The strict RIFF decoder already rejected this file, so
// anything odd here is normal: every failure path keeps the generic defaults
// instead of throwing. Only the fields the buffer layer actually needs are read.
wav_format probe_riff(const ttstr& storagename)
{
    wav_format out;
    try
    {
        tTVPStreamHolder stream(storagename, TJS_BS_READ);
        tTJSBinaryStream* s = stream.Get();
        if (s == nullptr)
            return out;

        const tjs_uint64 size = s->GetSize();
        std::array<char, 4> tag{};
        if (s->Read(tag.data(), 4) != 4 || std::memcmp(tag.data(), "RIFF", 4) != 0)
            return out;
        s->ReadI32LE(); // RIFF chunk size; the chunk walk below is the real bound
        if (s->Read(tag.data(), 4) != 4 || std::memcmp(tag.data(), "WAVE", 4) != 0)
            return out;

        bool have_fmt = false;
        while (s->GetPosition() + 8 <= size)
        {
            if (s->Read(tag.data(), 4) != 4)
                break;
            const tjs_uint32 chunk_size = s->ReadI32LE();
            const tjs_uint64 chunk_start = s->GetPosition();
            if (chunk_start + chunk_size > size)
                break; // truncated or lying header

            if (std::memcmp(tag.data(), "fmt ", 4) == 0 && chunk_size >= 16)
            {
                const auto format_tag = static_cast<wave_tag>(s->ReadI16LE());
                const unsigned channels = s->ReadI16LE();
                const tjs_uint32 rate = s->ReadI32LE();
                s->ReadI32LE(); // average bytes per second
                const unsigned block_align = s->ReadI16LE();
                // Container width; a WAVEFORMATEXTENSIBLE may store fewer valid
                // bits per sample than the container it packs into.
                const unsigned container_bits = s->ReadI16LE();
                unsigned bits = container_bits;
                bool is_float = format_tag == wave_tag::ieee_float;
                if (format_tag == wave_tag::extensible)
                {
                    // cbSize, valid bits, channel mask, subformat GUID.
                    s->ReadI16LE();
                    if (chunk_size < 40)
                        break;
                    const unsigned valid_bits = s->ReadI16LE();
                    if (valid_bits != 0 && valid_bits <= container_bits)
                        bits = valid_bits;
                    s->ReadI32LE();
                    std::array<tjs_uint8, 16> guid{};
                    if (s->Read(guid.data(), 16) != 16)
                        break;
                    // The KSDATAFORMAT subtype GUID starts with the format tag.
                    is_float = guid[0] == std::to_underlying(wave_tag::ieee_float);
                }
                const unsigned bytes_per_sample = container_bits / 8;
                // Only whole-byte PCM the engine's filters understand; anything
                // else is reported with the generic defaults instead.
                if (container_bits % 8 != 0 || (is_float && container_bits != 32) ||
                    (bits % 8) != 0)
                    break;
                if (channels >= 1 && channels <= 8 && rate >= 8000 && rate <= 192000 &&
                    bits >= 8 && bits <= 32 && block_align >= channels * bytes_per_sample)
                {
                    out.samples_per_sec = rate;
                    out.channels = channels;
                    out.bits_per_sample = bits;
                    out.bytes_per_sample = bytes_per_sample;
                    out.block_align = block_align;
                    out.is_float = is_float;
                    have_fmt = true;
                }
            }
            else if (std::memcmp(tag.data(), "data", 4) == 0)
            {
                if (have_fmt)
                    out.total_samples = chunk_size / out.block_align;
                return out;
            }

            // Chunks are word aligned; a chunk may be shorter than what we read
            // from it, so always jump to the declared end.
            const tjs_uint64 next = chunk_start + chunk_size + (chunk_size & 1u);
            if (next > size || s->Seek(static_cast<tjs_int64>(next), TJS_BS_SEEK_SET) != next)
                break;
        }
    }
    catch (...)
    {
        // Not RIFF / unreadable: keep the defaults.
    }
    return out;
}

} // namespace

//---------------------------------------------------------------------------
tTVPWD_HostSilent::tTVPWD_HostSilent(const ttstr& storagename, const ttstr& extension)
  : Position(0)
{
    Format.SamplesPerSec = default_samples_per_sec;
    Format.Channels = default_channels;
    Format.BitsPerSample = default_bits_per_sample;
    Format.BytesPerSample = default_bits_per_sample / 8;
    Format.TotalSamples = 0;
    Format.TotalTime = 0;
    Format.SpeakerConfig = 0;
    Format.IsFloat = false;
    Format.Seekable = true;

    if (extension == TJS_N(".wav"))
    {
        const wav_format wav = probe_riff(storagename);
        Format.SamplesPerSec = wav.samples_per_sec;
        Format.Channels = wav.channels;
        Format.BitsPerSample = wav.bits_per_sample;
        Format.BytesPerSample = wav.bytes_per_sample;
        Format.IsFloat = wav.is_float;
        Format.TotalSamples = wav.total_samples;
        Format.TotalTime = wav.total_samples * 1000 / wav.samples_per_sec;
    }
}
//---------------------------------------------------------------------------
void tTVPWD_HostSilent::GetFormat(tTVPWaveFormat& format)
{
    format = Format;
}
//---------------------------------------------------------------------------
bool tTVPWD_HostSilent::Render(void* buf, tjs_uint bufsamplelen, tjs_uint& rendered)
{
    // Known length: end where a real decoder would, so the loop manager rewinds
    // at the loop point and a one-shot reports end-of-stream to the script.
    // Unknown length: stream silence; the host owns when the sound really stops.
    const tjs_uint64 limit = Format.TotalSamples != 0 ? Format.TotalSamples
                                                      : max_unknown_silence_samples;
    const tjs_uint64 remain = Position < limit ? limit - Position : 0;

    rendered = static_cast<tjs_uint>(remain < bufsamplelen ? remain : bufsamplelen);
    if (rendered != 0)
    {
        const std::size_t bytes =
            static_cast<std::size_t>(rendered) * Format.Channels * Format.BytesPerSample;
        std::fill_n(static_cast<std::uint8_t*>(buf), bytes, std::uint8_t{0});
        Position += rendered;
    }
    return rendered != 0 && rendered == bufsamplelen;
}
//---------------------------------------------------------------------------
bool tTVPWD_HostSilent::SetPosition(tjs_uint64 samplepos)
{
    if (Format.TotalSamples != 0)
    {
        if (samplepos > Format.TotalSamples)
            return false;
    }
    else if (samplepos > max_unknown_silence_samples)
    {
        samplepos = max_unknown_silence_samples;
    }
    Position = samplepos;
    return true;
}
//---------------------------------------------------------------------------
tTVPWaveDecoder* tTVPWDC_HostSilent::Create(const ttstr& storagename, const ttstr& extension)
{
    // Unconditional, including storages that do not resolve: a port that strips
    // audio or a script that names a missing file must not abort the game before
    // the Play hook runs. The host materialisation then fails and simply reports
    // no audio, which is the same audible outcome without the crash. probe_riff
    // already swallows the open error and keeps the generic defaults.
    return new tTVPWD_HostSilent(storagename, extension);
}
//---------------------------------------------------------------------------
