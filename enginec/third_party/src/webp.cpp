// libwebp wrapper implementation.
//
// WebPDecodeBGRA allocates through the library and hands back an opaque pointer;
// on a phone a slightly-too-large image would otherwise abort the process, so the
// size is validated up front and the returned buffer is copied into a vector that
// the engine owns before WebPFree is called on every path.

module;

#include <webp/decode.h>

module enginec.third_party.webp;

import std;

namespace enginec::third_party::webp
{

namespace
{

// Same budget as the PNG/JPEG wrappers: a corrupt header must not turn into an
// allocation request the device cannot satisfy. 16M pixels is at most 64 MiB BGRA.
inline constexpr int max_dimension = 16384;
inline constexpr std::size_t max_pixels = std::size_t{1} << 24;

bool valid_extent(int width, int height) noexcept
{
    if (width <= 0 || height <= 0 || width > max_dimension || height > max_dimension)
    {
        return false;
    }
    return static_cast<std::size_t>(width) * static_cast<std::size_t>(height) <= max_pixels;
}

} // namespace

std::optional<image> decode(std::span<const std::uint8_t> data, std::string& error)
{
    error.clear();

    if (data.empty())
    {
        error = "empty WebP stream";
        return std::nullopt;
    }
    // RIFF....WEBP: WebPGetInfo accepts a bare VP8/VP8L bitstream too, but KiriKiri
    // assets are always RIFF-framed, and rejecting anything else keeps the quick
    // format sniff in image.cpp honest.
    if (data.size() < 12 || std::memcmp(data.data(), "RIFF", 4) != 0 ||
        std::memcmp(data.data() + 8, "WEBP", 4) != 0)
    {
        error = "not a WebP stream";
        return std::nullopt;
    }

    try
    {
        int width = 0;
        int height = 0;
        if (WebPGetInfo(data.data(), data.size(), &width, &height) == 0)
        {
            error = "WebP header is invalid";
            return std::nullopt;
        }
        if (!valid_extent(width, height))
        {
            error = "WebP dimensions are unsupported";
            return std::nullopt;
        }

        std::uint8_t* decoded = WebPDecodeBGRA(data.data(), data.size(), &width, &height);
        if (decoded == nullptr)
        {
            error = "WebP decoding failed";
            return std::nullopt;
        }
        // The pointer is owned by libwebp and must be freed even when the copy
        // below throws, so the copy happens under a guard rather than after.
        std::unique_ptr<std::uint8_t, void (*)(void*)> owner(decoded, &WebPFree);

        if (!valid_extent(width, height))
        {
            error = "WebP dimensions are unsupported";
            return std::nullopt;
        }

        const std::size_t stride = static_cast<std::size_t>(width) * 4;
        image result;
        result.width = width;
        result.height = height;
        result.bgra.assign(decoded, decoded + stride * static_cast<std::size_t>(height));
        return result;
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while decoding WebP";
        return std::nullopt;
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return std::nullopt;
    }
    catch (...)
    {
        error = "unknown failure while decoding WebP";
        return std::nullopt;
    }
}

} // namespace enginec::third_party::webp
