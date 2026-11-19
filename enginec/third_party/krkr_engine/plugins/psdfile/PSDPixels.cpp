// Channel decoding and pixel composition.
//
// The three on-disk channel encodings share one entry point: `decode_plane`
// always returns 8-bit samples, because a Layer buffer is 32-bit BGRA on every
// backend. 16/32-bit documents are narrowed here instead of at each merge site.
//
// zlib is a C library, so its header belongs in the global module fragment —
// the one place in this unit where #include is legitimate. Everything else is
// the standard library, imported.
module;

#include <zlib.h>

module enginec.krkr.psd;

import std;

namespace kzpsd
{
namespace
{

constexpr std::uint16_t compression_raw = 0;
constexpr std::uint16_t compression_rle = 1;
constexpr std::uint16_t compression_zip = 2;
constexpr std::uint16_t compression_zip_prediction = 3;

struct Plane
{
    int width = 0;
    int height = 0;
    std::vector<std::uint8_t> pixels; // 8-bit, width * height
};

int bytes_per_sample(int depth)
{
    switch (depth)
    {
        case 16: return 2;
        case 32: return 4;
        default: return 1;
    }
}

// One decoded row of PackBits data. `-128` is a no-op, negative counts repeat
// the next byte, non-negative counts copy count+1 literal bytes.
bool decode_packbits_row(const std::uint8_t* source, std::size_t sourceSize,
                         std::uint8_t* destination, std::size_t rowBytes,
                         std::size_t& consumed)
{
    std::size_t input = 0;
    std::size_t output = 0;
    while (output < rowBytes)
    {
        if (input >= sourceSize)
            return false;
        const auto control = static_cast<std::int8_t>(source[input++]);
        if (control == -128)
            continue;
        if (control >= 0)
        {
            const auto count = static_cast<std::size_t>(control) + 1;
            if (input + count > sourceSize || output + count > rowBytes)
                return false;
            std::memcpy(destination + output, source + input, count);
            input += count;
            output += count;
        }
        else
        {
            const auto count = static_cast<std::size_t>(-static_cast<int>(control)) + 1;
            if (input >= sourceSize || output + count > rowBytes)
                return false;
            std::memset(destination + output, source[input++], count);
            output += count;
        }
    }
    consumed = input;
    return true;
}

// Inflates one zip-compressed channel and reports how many input bytes it used,
// which is what lets several channels share one payload in the merged image.
bool zip_inflate(const std::uint8_t* source, std::size_t sourceSize,
                 std::uint8_t* destination, std::size_t destinationSize,
                 std::size_t& consumed)
{
    z_stream stream = {};
    if (inflateInit(&stream) != Z_OK)
        return false;
    stream.next_in = const_cast<Bytef*>(source);
    stream.avail_in = static_cast<uInt>(sourceSize);
    stream.next_out = destination;
    stream.avail_out = static_cast<uInt>(destinationSize);
    const auto result = inflate(&stream, Z_FINISH);
    consumed = static_cast<std::size_t>(stream.total_in);
    inflateEnd(&stream);
    return result == Z_STREAM_END && stream.total_out == destinationSize;
}

// Undoes the horizontal delta applied before zip compression. PSD applies it to
// 8/32-bit data per byte (no carry) but to 16-bit samples as big-endian 16-bit
// additions, so the two cases are kept separate.
void undo_prediction(std::vector<std::uint8_t>& raw, int width, int height, int depth)
{
    if (depth == 8)
    {
        for (int y = 0; y < height; ++y)
        {
            auto* row = raw.data() + static_cast<std::size_t>(y) * width;
            for (int x = 1; x < width; ++x)
                row[x] = static_cast<std::uint8_t>(row[x] + row[x - 1]);
        }
    }
    else if (depth == 16)
    {
        for (int y = 0; y < height; ++y)
        {
            auto* row = raw.data() + static_cast<std::size_t>(y) * width * 2;
            for (int x = 1; x < width; ++x)
            {
                const auto low = static_cast<unsigned>(row[x * 2 + 1]) + row[(x - 1) * 2 + 1];
                const auto high = static_cast<unsigned>(row[x * 2]) + row[(x - 1) * 2] + (low >> 8);
                row[x * 2] = static_cast<std::uint8_t>(high);
                row[x * 2 + 1] = static_cast<std::uint8_t>(low);
            }
        }
    }
    else
    {
        // 32-bit float data is deltad byte by byte.
        const auto rowBytes = static_cast<std::size_t>(width) * 4;
        for (int y = 0; y < height; ++y)
        {
            auto* row = raw.data() + static_cast<std::size_t>(y) * rowBytes;
            for (std::size_t x = 4; x < rowBytes; ++x)
                row[x] = static_cast<std::uint8_t>(row[x] + row[x - 4]);
        }
    }
}

// Decodes one channel to 8-bit samples. `width`/`height` describe the plane's
// own rectangle (a mask can be smaller than the layer).
bool decode_plane(const Document& document, const Channel& channel, int width, int height,
                  Plane& plane)
{
    if (width <= 0 || height <= 0)
        return false;
    const auto bytes = static_cast<std::size_t>(channel.length);
    if (channel.dataOffset + bytes > document.file.size() || bytes < 2)
        return false;

    const auto* base = document.file.data() + channel.dataOffset;
    const auto compression = static_cast<std::uint16_t>((base[0] << 8) | base[1]);
    const std::uint8_t* payload = base + 2;
    const auto payloadSize = bytes - 2;
    const int depth = document.depth;

    if (depth == 1)
    {
        const auto rowBytes = static_cast<std::size_t>((width + 7) / 8);
        const auto expected = rowBytes * static_cast<std::size_t>(height);
        if (payloadSize < expected)
            return false;
        plane.width = width;
        plane.height = height;
        plane.pixels.resize(static_cast<std::size_t>(width) * height);
        for (int y = 0; y < height; ++y)
        {
            const auto* row = payload + rowBytes * static_cast<std::size_t>(y);
            auto* out = plane.pixels.data() + static_cast<std::size_t>(y) * width;
            for (int x = 0; x < width; ++x)
            {
                const bool set = (row[x / 8] & (0x80 >> (x % 8))) != 0;
                out[x] = set ? 0x00 : 0xff; // Photoshop bitmap: 1 = black
            }
        }
        return true;
    }

    const auto sampleBytes = static_cast<std::size_t>(bytes_per_sample(depth));
    const auto rowBytes = static_cast<std::size_t>(width) * sampleBytes;
    const auto expected = rowBytes * static_cast<std::size_t>(height);
    std::vector<std::uint8_t> raw(expected);

    switch (compression)
    {
        case compression_raw:
            if (payloadSize < expected)
                return false;
            std::memcpy(raw.data(), payload, expected);
            break;

        case compression_rle:
        {
            const auto tableBytes = static_cast<std::size_t>(height) * 2;
            if (payloadSize < tableBytes)
                return false;
            std::size_t offset = tableBytes;
            for (int y = 0; y < height; ++y)
            {
                // The per-row byte count is informational: decoding stops when
                // the row is full, and PackBits consumes exactly one row.
                std::size_t consumed = 0;
                if (!decode_packbits_row(payload + offset, payloadSize - offset,
                                         raw.data() + static_cast<std::size_t>(y) * rowBytes,
                                         rowBytes, consumed))
                    return false;
                offset += consumed;
            }
            break;
        }

        case compression_zip:
        case compression_zip_prediction:
        {
            std::size_t used = 0;
            if (!zip_inflate(payload, payloadSize, raw.data(), expected, used))
                return false;
            if (compression == compression_zip_prediction)
                undo_prediction(raw, width, height, depth);
            break;
        }

        default:
            return false; // unknown compression tag
    }

    plane.width = width;
    plane.height = height;
    plane.pixels.resize(static_cast<std::size_t>(width) * height);

    switch (depth)
    {
        case 8:
            std::memcpy(plane.pixels.data(), raw.data(), static_cast<std::size_t>(width) * height);
            break;
        case 16:
            // Narrow by keeping the high byte: the engine composites 8-bit.
            for (std::size_t i = 0; i < plane.pixels.size(); ++i)
                plane.pixels[i] = raw[i * 2];
            break;
        case 32:
            for (std::size_t i = 0; i < plane.pixels.size(); ++i)
            {
                float value = 0.0f;
                std::memcpy(&value, raw.data() + i * 4, sizeof(value));
                const auto scaled = std::lround(value * 255.0f);
                plane.pixels[i] = static_cast<std::uint8_t>(std::clamp<long>(scaled, 0, 255));
            }
            break;
        default:
            return false;
    }
    return true;
}

// Locates the plane for a channel id inside a layer, if the layer has one.
const Channel* find_channel(const Layer& layer, int id)
{
    for (const auto& channel : layer.channels)
    {
        if (channel.id == id)
            return &channel;
    }
    return nullptr;
}

// Alpha from the layer's own transparency channel, or a fully opaque plane.
bool decode_alpha(const Document& document, const Layer& layer, Plane& alpha)
{
    const auto* channel = find_channel(layer, -1);
    if (channel && decode_plane(document, *channel, layer.width, layer.height, alpha))
        return true;
    alpha.width = layer.width;
    alpha.height = layer.height;
    alpha.pixels.assign(static_cast<std::size_t>(layer.width) * layer.height, 0xff);
    return layer.width > 0 && layer.height > 0;
}

// Folds the layer mask into alpha exactly as the reference does: multiply inside
// the mask rectangle, and outside it either keep alpha (default 255) or clear it
// (default 0).
void apply_mask(const Layer& layer, const Plane& mask, Plane& alpha)
{
    if (!layer.mask.present || mask.width <= 0 || mask.height <= 0)
        return;
    const auto& rect = layer.mask;
    for (int y = 0; y < alpha.height; ++y)
    {
        const int absoluteY = layer.top + y;
        for (int x = 0; x < alpha.width; ++x)
        {
            const int absoluteX = layer.left + x;
            auto& value = alpha.pixels[static_cast<std::size_t>(y) * alpha.width + x];
            if (absoluteX >= rect.left && absoluteX < rect.right && absoluteY >= rect.top &&
                absoluteY < rect.bottom)
            {
                const int maskX = absoluteX - rect.left;
                const int maskY = absoluteY - rect.top;
                if (maskX >= 0 && maskX < mask.width && maskY >= 0 && maskY < mask.height)
                {
                    const auto m =
                        mask.pixels[static_cast<std::size_t>(maskY) * mask.width + maskX];
                    value = static_cast<std::uint8_t>((value * m + 127) / 255);
                }
            }
            else if (rect.defaultColor == 0)
            {
                value = 0;
            }
        }
    }
}

std::uint32_t pack(ColorFormat format, std::uint8_t red, std::uint8_t green,
                   std::uint8_t blue, std::uint8_t alpha)
{
    return (static_cast<std::uint32_t>(red) << format.redShift) |
           (static_cast<std::uint32_t>(green) << format.greenShift) |
           (static_cast<std::uint32_t>(blue) << format.blueShift) |
           (static_cast<std::uint32_t>(alpha) << format.alphaShift);
}

// Converts one row of CMYK samples to BGRA. Same formula as the reference:
// each ink subtracts from white, with black applied last.
std::uint8_t cmyk_to_rgb(std::uint8_t ink, std::uint8_t key)
{
    return static_cast<std::uint8_t>((static_cast<int>(255 - ink) * (255 - key) + 127) / 255);
}

// Writes `width` pixels of one layer row into `destination`.
bool merge_row(const Document& document, int y, int width, const Plane& red, const Plane& green,
               const Plane& blue, const Plane& alpha, const Plane& gray, const Plane& cmykC,
               const Plane& cmykM, const Plane& cmykY, const Plane& cmykK, const Plane& index,
               const Plane& bitmap, std::uint8_t* destination, ColorFormat format)
{
    auto* pixels = reinterpret_cast<std::uint32_t*>(destination);
    for (int x = 0; x < width; ++x)
    {
        const auto offset = static_cast<std::size_t>(y) * width + x;
        std::uint8_t a = alpha.pixels.empty() ? 0xff : alpha.pixels[offset];
        std::uint8_t r = 0;
        std::uint8_t g = 0;
        std::uint8_t b = 0;
        switch (document.mode)
        {
            case ColorMode::Bitmap:
            {
                const auto value = bitmap.pixels[offset];
                r = g = b = value;
                break;
            }
            case ColorMode::Grayscale:
            case ColorMode::Duotone:
            {
                r = g = b = gray.pixels[offset];
                break;
            }
            case ColorMode::Indexed:
            {
                const auto entry = index.pixels[offset];
                const auto paletteOffset = static_cast<std::size_t>(entry) * 3;
                if (paletteOffset + 2 < document.colorTable.size())
                {
                    r = document.colorTable[paletteOffset];
                    g = document.colorTable[paletteOffset + 1];
                    b = document.colorTable[paletteOffset + 2];
                }
                if (entry == document.transparencyIndex)
                    a = 0;
                break;
            }
            case ColorMode::Cmyk:
            {
                const auto c = cmykC.pixels[offset];
                const auto m = cmykM.pixels[offset];
                const auto yy = cmykY.pixels[offset];
                const auto k = cmykK.pixels[offset];
                r = cmyk_to_rgb(c, k);
                g = cmyk_to_rgb(m, k);
                b = cmyk_to_rgb(yy, k);
                break;
            }
            default: // RGB, multichannel and L*a*b* fall back to the first three planes
            {
                r = red.pixels[offset];
                g = green.pixels[offset];
                b = blue.pixels[offset];
                break;
            }
        }
        pixels[x] = pack(format, r, g, b, a);
    }
    return true;
}

} // namespace

bool render_layer(const Document& document, const Layer& layer, std::uint8_t* destination,
                  int pitch, ImageMode mode, ColorFormat format)
{
    // A negative pitch is legal: BMP rows are stored bottom-up and the caller
    // hands us the address of the last row.
    if (destination == nullptr || pitch == 0)
        return false;

    const int width = mode == ImageMode::Mask ? layer.mask.width : layer.width;
    const int height = mode == ImageMode::Mask ? layer.mask.height : layer.height;
    if (width <= 0 || height <= 0)
        return false;

    Plane red, green, blue, gray, cmykC, cmykM, cmykY, cmykK, index, bitmap;

    if (mode == ImageMode::Mask)
    {
        const auto* mask = find_channel(layer, -2);
        const Channel* channel = mask ? mask : find_channel(layer, -3);
        if (!channel || !decode_plane(document, *channel, width, height, gray))
            return false;
        // A mask is read back as a grayscale image with full alpha, whatever the
        // document's colour mode is.
        for (int y = 0; y < height; ++y)
        {
            auto* pixels = reinterpret_cast<std::uint32_t*>(
                destination + static_cast<std::size_t>(y) * pitch);
            const auto* source = gray.pixels.data() + static_cast<std::size_t>(y) * width;
            for (int x = 0; x < width; ++x)
            {
                const auto value = source[x];
                pixels[x] = pack(format, value, value, value, 0xff);
            }
        }
        return true;
    }

    const auto* redChannel = find_channel(layer, 0);
    const auto* greenChannel = find_channel(layer, 1);
    const auto* blueChannel = find_channel(layer, 2);
    const auto* keyChannel = find_channel(layer, 3); // CMYK key / extra plane

    switch (document.mode)
    {
        case ColorMode::Bitmap:
            if (!redChannel || !decode_plane(document, *redChannel, width, height, bitmap))
                return false;
            break;
        case ColorMode::Grayscale:
        case ColorMode::Duotone:
            if (!redChannel || !decode_plane(document, *redChannel, width, height, gray))
                return false;
            break;
        case ColorMode::Indexed:
            if (!redChannel || !decode_plane(document, *redChannel, width, height, index))
                return false;
            break;
        case ColorMode::Cmyk:
            if (!redChannel || !greenChannel || !blueChannel || !keyChannel)
                return false;
            if (!decode_plane(document, *redChannel, width, height, cmykC) ||
                !decode_plane(document, *greenChannel, width, height, cmykM) ||
                !decode_plane(document, *blueChannel, width, height, cmykY) ||
                !decode_plane(document, *keyChannel, width, height, cmykK))
                return false;
            break;
        default:
            if (!redChannel || !greenChannel || !blueChannel)
                return false;
            if (!decode_plane(document, *redChannel, width, height, red) ||
                !decode_plane(document, *greenChannel, width, height, green) ||
                !decode_plane(document, *blueChannel, width, height, blue))
                return false;
            break;
    }

    // Raw image mode deliberately ignores alpha and the mask: scripts use it to
    // read the unmodified plane.
    Plane alpha;
    if (mode == ImageMode::MaskedImage)
    {
        decode_alpha(document, layer, alpha);
        if (const auto* maskChannel = find_channel(layer, -2))
        {
            Plane mask;
            if (decode_plane(document, *maskChannel, layer.mask.width, layer.mask.height, mask))
                apply_mask(layer, mask, alpha);
        }
        else if (const auto* realMask = find_channel(layer, -3))
        {
            Plane mask;
            if (decode_plane(document, *realMask, layer.mask.width, layer.mask.height, mask))
                apply_mask(layer, mask, alpha);
        }
    }

    for (int y = 0; y < height; ++y)
    {
        merge_row(document, y, width, red, green, blue, alpha, gray, cmykC, cmykM, cmykY, cmykK,
                  index, bitmap, destination + static_cast<std::size_t>(y) * pitch, format);
    }
    return true;
}

bool render_merged(const Document& document, std::uint8_t* destination, int pitch,
                   ColorFormat format)
{
    if (destination == nullptr || pitch <= 0 || document.imageDataOffset + 2 > document.file.size())
        return false;

    const auto* base = document.file.data() + document.imageDataOffset;
    const auto compression = static_cast<std::uint16_t>((base[0] << 8) | base[1]);
    const std::uint8_t* payload = base + 2;
    const auto payloadSize = document.file.size() - document.imageDataOffset - 2;

    const int width = document.width;
    const int height = document.height;
    const int depth = document.depth;
    const auto sampleBytes = static_cast<std::size_t>(bytes_per_sample(depth));
    const auto rowBytes = static_cast<std::size_t>(width) * sampleBytes;
    const auto planeBytes = rowBytes * static_cast<std::size_t>(height);
    const auto channelCount = static_cast<std::size_t>(document.channels);

    std::vector<Plane> planes;
    planes.resize(channelCount);

    std::size_t consumed = 0;
    if (compression == compression_rle)
    {
        // RLE stores all row byte counts for all channels before the row data.
        const auto tableBytes = channelCount * static_cast<std::size_t>(height) * 2;
        if (payloadSize < tableBytes)
            return false;
        consumed = tableBytes;
    }

    for (std::size_t plane = 0; plane < channelCount; ++plane)
    {
        std::vector<std::uint8_t> raw(planeBytes);
        if (compression == compression_raw)
        {
            if (consumed + planeBytes > payloadSize)
                return false;
            std::memcpy(raw.data(), payload + consumed, planeBytes);
            consumed += planeBytes;
        }
        else if (compression == compression_rle)
        {
            for (int y = 0; y < height; ++y)
            {
                // Row byte counts are not needed: each PackBits row decodes as
                // many bytes as the row itself holds.
                std::size_t rowConsumed = 0;
                if (!decode_packbits_row(payload + consumed, payloadSize - consumed,
                                         raw.data() + static_cast<std::size_t>(y) * rowBytes,
                                         rowBytes, rowConsumed))
                    return false;
                consumed += rowConsumed;
            }
        }
        else if (compression == compression_zip || compression == compression_zip_prediction)
        {
            std::size_t used = 0;
            if (!zip_inflate(payload + consumed, payloadSize - consumed, raw.data(), planeBytes,
                             used))
                return false;
            consumed += used;
            if (compression == compression_zip_prediction)
                undo_prediction(raw, width, height, depth);
        }
        else
        {
            return false;
        }

        auto& out = planes[plane];
        out.width = width;
        out.height = height;
        out.pixels.resize(static_cast<std::size_t>(width) * height);
        switch (depth)
        {
            case 1:
                for (std::size_t i = 0; i < out.pixels.size(); ++i)
                {
                    const auto byte = raw[i / 8];
                    out.pixels[i] = (byte & (0x80 >> (i % 8))) ? 0x00 : 0xff;
                }
                break;
            case 8:
                std::memcpy(out.pixels.data(), raw.data(), out.pixels.size());
                break;
            case 16:
                for (std::size_t i = 0; i < out.pixels.size(); ++i)
                    out.pixels[i] = raw[i * 2];
                break;
            case 32:
                for (std::size_t i = 0; i < out.pixels.size(); ++i)
                {
                    float value = 0.0f;
                    std::memcpy(&value, raw.data() + i * 4, sizeof(value));
                    const auto scaled = std::lround(value * 255.0f);
                    out.pixels[i] =
                        static_cast<std::uint8_t>(std::clamp<long>(scaled, 0, 255));
                }
                break;
            default:
                return false;
        }
    }

    // In the merged image the first planes are colour and an extra plane is
    // alpha, in the same order the colour mode defines.
    int baseChannels = 3;
    switch (document.mode)
    {
        case ColorMode::Bitmap:
        case ColorMode::Grayscale:
        case ColorMode::Indexed:
        case ColorMode::Duotone: baseChannels = 1; break;
        case ColorMode::Cmyk: baseChannels = 4; break;
        default: baseChannels = 3; break;
    }
    Plane empty;
    Plane alphaPlane;
    const bool hasAlpha = static_cast<int>(channelCount) > baseChannels;
    if (hasAlpha)
        alphaPlane = planes[static_cast<std::size_t>(baseChannels)];

    const Plane& p0 = planes[0];
    const Plane& p1 = channelCount > 1 ? planes[1] : empty;
    const Plane& p2 = channelCount > 2 ? planes[2] : empty;
    const Plane& p3 = channelCount > 3 ? planes[3] : empty;

    for (int y = 0; y < height; ++y)
    {
        if (document.mode == ColorMode::Bitmap || document.mode == ColorMode::Grayscale ||
            document.mode == ColorMode::Duotone)
        {
            merge_row(document, y, width, p0, p0, p0, alphaPlane, p0, p0, p0, p0, p0, p0, p0,
                      destination + static_cast<std::size_t>(y) * pitch, format);
        }
        else if (document.mode == ColorMode::Cmyk)
        {
            merge_row(document, y, width, p0, p1, p2, alphaPlane, empty, p0, p1, p2, p3, empty,
                      empty, destination + static_cast<std::size_t>(y) * pitch, format);
        }
        else
        {
            merge_row(document, y, width, p0, p1, p2, alphaPlane, empty, empty, empty, empty,
                      empty, p0, empty,
                      destination + static_cast<std::size_t>(y) * pitch, format);
        }
    }
    return true;
}

} // namespace kzpsd
