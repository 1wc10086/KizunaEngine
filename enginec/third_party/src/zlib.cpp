// zlib-ng wrapper implementation.
//
// Two details drive the shape of this file:
//  * zlib's input length and avail_in are 32-bit (uInt), so a buffer larger than
//    that is fed in chunks; a single cast would silently truncate.
//  * inflate() can return Z_BUF_ERROR for reasons that are not fatal (no progress
//    yet) and Z_OK forever for a truncated stream, so the loop tracks whether the
//    previous call moved at all and treats a stall with no input left as truncation.

module;

#include <zlib.h>

module enginec.third_party.zlib;

import std;

namespace enginec::third_party::zlib
{

namespace
{

// No single asset in KiriKiri is close to a gigabyte: XP3 segments are bounded by
// the index and save containers hold a thumbnail plus a script cursor. The ceiling
// turns a hostile or corrupt stream into an error instead of an OOM abort on a phone.
inline constexpr std::size_t max_buffer_size = std::size_t{1} << 30;
inline constexpr std::size_t io_chunk = std::size_t{1} << 16;
inline constexpr std::size_t max_io = std::size_t{1} << 30;

int window_bits(wrapper wrap) noexcept
{
    switch (wrap)
    {
    case wrapper::raw:
        return -15;
    case wrapper::gzip:
        return 15 + 16;
    case wrapper::zlib_stream:
        break;
    }
    return 15;
}

std::string code_text(int code)
{
    return "zlib error " + std::to_string(code);
}

// deflateEnd/inflateEnd must run on every exit path, including the ones taken when
// the C library reported a failure.
struct inflate_guard
{
    z_stream* stream = nullptr;

    ~inflate_guard()
    {
        if (stream != nullptr)
        {
            inflateEnd(stream);
        }
    }
};

struct deflate_guard
{
    z_stream* stream = nullptr;

    ~deflate_guard()
    {
        if (stream != nullptr)
        {
            deflateEnd(stream);
        }
    }
};

} // namespace

std::optional<std::vector<std::uint8_t>> decompress(std::span<const std::uint8_t> input,
                                                    wrapper wrap, std::size_t expected,
                                                    std::string& error)
{
    error.clear();
    if (expected > max_buffer_size)
    {
        error = "declared size " + std::to_string(expected) + " exceeds the decode limit";
        return std::nullopt;
    }

    const std::size_t limit = expected != 0 ? expected : max_buffer_size;

    try
    {
        z_stream stream{};
        const int init = inflateInit2(&stream, window_bits(wrap));
        if (init != Z_OK)
        {
            error = "inflateInit2 failed: " + code_text(init);
            return std::nullopt;
        }
        inflate_guard guard{&stream};

        // A fixed scratch buffer keeps the steady state free of reallocation; the
        // result grows once per chunk, which is the cheapest correct shape when the
        // final size is unknown.
        std::vector<std::uint8_t> scratch(io_chunk);
        std::vector<std::uint8_t> out;
        // A declared size is only a hint; reserving it verbatim would turn a lie in
        // the XP3 index into an allocation failure before any data is decoded.
        out.reserve(io_chunk);

        std::size_t in_pos = 0;
        for (;;)
        {
            if (stream.avail_in == 0 && in_pos < input.size())
            {
                const std::size_t amount = std::min(input.size() - in_pos, max_io);
                stream.next_in = const_cast<Bytef*>(input.data() + in_pos);
                stream.avail_in = static_cast<uInt>(amount);
                in_pos += amount;
            }

            const std::size_t unread_before = (input.size() - in_pos) + stream.avail_in;
            stream.next_out = scratch.data();
            stream.avail_out = static_cast<uInt>(scratch.size());

            const int code = inflate(&stream, Z_NO_FLUSH);
            const std::size_t produced = scratch.size() - stream.avail_out;
            out.insert(out.end(), scratch.begin(), scratch.begin() + produced);

            if (code == Z_STREAM_END)
            {
                break;
            }
            if (code != Z_OK && code != Z_BUF_ERROR)
            {
                error = "inflate failed: " + code_text(code);
                return std::nullopt;
            }
            if (out.size() > limit)
            {
                error = "decompressed data exceeds the declared size of " + std::to_string(limit) +
                        " bytes";
                return std::nullopt;
            }

            const std::size_t unread_after = (input.size() - in_pos) + stream.avail_in;
            if (produced == 0 && unread_after == unread_before)
            {
                error = "compressed stream is truncated";
                return std::nullopt;
            }
        }

        if (out.size() > limit)
        {
            error = "decompressed data exceeds the declared size of " + std::to_string(limit) +
                    " bytes";
            return std::nullopt;
        }
        return out;
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while decompressing";
        return std::nullopt;
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return std::nullopt;
    }
    catch (...)
    {
        error = "unknown failure while decompressing";
        return std::nullopt;
    }
}

std::vector<std::uint8_t> compress(std::span<const std::uint8_t> input, wrapper wrap, int level,
                                   std::string& error)
{
    error.clear();

    try
    {
        z_stream stream{};
        const int effective = level < 0 ? Z_DEFAULT_COMPRESSION : std::min(level, 9);
        const int init = deflateInit2(&stream, effective, Z_DEFLATED, window_bits(wrap), 8,
                                      Z_DEFAULT_STRATEGY);
        if (init != Z_OK)
        {
            error = "deflateInit2 failed: " + code_text(init);
            return {};
        }
        deflate_guard guard{&stream};

        std::vector<std::uint8_t> scratch(io_chunk);
        std::vector<std::uint8_t> out;
        out.reserve(io_chunk);

        std::size_t in_pos = 0;
        for (;;)
        {
            if (stream.avail_in == 0 && in_pos < input.size())
            {
                const std::size_t amount = std::min(input.size() - in_pos, max_io);
                stream.next_in = const_cast<Bytef*>(input.data() + in_pos);
                stream.avail_in = static_cast<uInt>(amount);
                in_pos += amount;
            }

            stream.next_out = scratch.data();
            stream.avail_out = static_cast<uInt>(scratch.size());

            const uInt available_before = stream.avail_in;
            const int code = deflate(&stream, Z_FINISH);
            const std::size_t produced = scratch.size() - stream.avail_out;
            out.insert(out.end(), scratch.begin(), scratch.begin() + produced);

            if (code == Z_STREAM_END)
            {
                break;
            }
            if (code != Z_OK && code != Z_BUF_ERROR)
            {
                error = "deflate failed: " + code_text(code);
                return {};
            }
            // With Z_FINISH, deflate keeps emitting until the stream ends. Neither
            // output nor consumed input means no later call can make progress, so
            // the loop would otherwise spin forever on a broken stream state.
            if (produced == 0 && stream.avail_in == available_before)
            {
                error = "deflate stalled";
                return {};
            }
            if (out.size() > max_buffer_size)
            {
                error = "compressed output exceeds the encode limit";
                return {};
            }
        }

        return out;
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while compressing";
        return {};
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return {};
    }
    catch (...)
    {
        error = "unknown failure while compressing";
        return {};
    }
}

wrapper sniff(std::span<const std::uint8_t> prefix) noexcept
{
    if (prefix.size() >= 2 && prefix[0] == 0x1F && prefix[1] == 0x8B)
    {
        return wrapper::gzip;
    }
    if (prefix.size() >= 2)
    {
        // RFC 1950: CM must be 8 (deflate), CINFO <= 7, and the two-byte header is
        // a multiple of 31. XP3 also stores raw deflate segments, which fail this
        // check and fall through to `raw`.
        const unsigned cmf = prefix[0];
        const unsigned flg = prefix[1];
        if ((cmf & 0x0Fu) == 8u && (cmf >> 4) <= 7u && ((cmf << 8) | flg) % 31u == 0u)
        {
            return wrapper::zlib_stream;
        }
    }
    return wrapper::raw;
}

} // namespace enginec::third_party::zlib
