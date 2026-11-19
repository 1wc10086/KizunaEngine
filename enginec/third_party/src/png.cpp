// libpng wrapper implementation.
//
// libpng signals failures by calling png_error, whose default handler prints to
// stderr and calls exit(). That is unusable in an engine that must report a broken
// game asset as an error, so this file installs its own error handler and lets
// libpng's longjmp land inside the same function that started the decode.
//
// Everything that can be modified between setjmp and longjmp is owned through a
// pointer (heap memory) rather than being a mutable automatic variable, because C
// leaves automatic variables modified across a longjmp indeterminate. The pointers
// themselves are assigned once, before setjmp.

module;

// libpng's error path needs jmp_buf, and its documentation requires <setjmp.h> to
// be included before png.h so the two agree on the buffer layout.
#include <setjmp.h>

#include <png.h>

module enginec.third_party.png;

import std;

namespace enginec::third_party::png
{

namespace
{

// KiriKiri backgrounds top out around 1920x1080; 16384 is already far beyond any
// asset a game ships and keeps a corrupt IHDR from requesting gigabytes.
inline constexpr int max_dimension = 16384;
// 16M pixels is a 4096x4096 image, i.e. at most 64 MiB of BGRA. Anything larger is
// rejected before allocation rather than risking the process being killed on a phone.
inline constexpr std::size_t max_pixels = std::size_t{1} << 24;

bool valid_extent(int width, int height) noexcept
{
    if (width <= 0 || height <= 0 || width > max_dimension || height > max_dimension)
    {
        return false;
    }
    const std::size_t pixels = static_cast<std::size_t>(width) * static_cast<std::size_t>(height);
    return pixels <= max_pixels;
}

void on_error(png_structp png, png_const_charp)
{
    png_longjmp(png, 1);
}

void on_warning(png_structp, png_const_charp)
{
    // libpng warns about benign issues (unknown ancillary chunks, iCCP profile
    // quirks) that every real game library contains; they never affect pixels.
}

struct read_source
{
    std::span<const std::uint8_t> data;
    std::size_t offset = 0;
};

void read_callback(png_structp png, png_bytep out, png_size_t count)
{
    auto* source = static_cast<read_source*>(png_get_io_ptr(png));
    if (source == nullptr || source->offset > source->data.size() ||
        count > source->data.size() - source->offset)
    {
        png_error(png, "unexpected end of PNG data");
    }
    std::memcpy(out, source->data.data() + source->offset, count);
    source->offset += count;
}

struct read_job
{
    read_source source;
    png_structp png = nullptr;
    png_infop info = nullptr;
    image result;

    ~read_job()
    {
        if (png != nullptr)
        {
            png_destroy_read_struct(&png, info != nullptr ? &info : nullptr, nullptr);
        }
    }
};

struct write_sink
{
    std::vector<std::uint8_t> bytes;
};

void write_callback(png_structp png, png_bytep data, png_size_t count)
{
    auto* sink = static_cast<write_sink*>(png_get_io_ptr(png));
    if (sink == nullptr)
    {
        png_error(png, "missing write sink");
    }
    // libpng is C: a std::bad_alloc escaping here would unwind through its frames,
    // so it is converted into the longjmp-based failure path instead.
    try
    {
        sink->bytes.insert(sink->bytes.end(), data, data + count);
    }
    catch (...)
    {
        png_error(png, "out of memory while encoding PNG");
    }
}

struct write_job
{
    write_sink sink;
    png_structp png = nullptr;
    png_infop info = nullptr;

    ~write_job()
    {
        if (png != nullptr)
        {
            png_destroy_write_struct(&png, info != nullptr ? &info : nullptr);
        }
    }
};

} // namespace

std::optional<image> decode(std::span<const std::uint8_t> data, std::string& error)
{
    error.clear();

    if (data.size() < 8 || png_sig_cmp(data.data(), 0, 8) != 0)
    {
        error = "not a PNG stream";
        return std::nullopt;
    }

    try
    {
        auto job = std::make_unique<read_job>();
        job->source.data = data;

        job->png = png_create_read_struct(PNG_LIBPNG_VER_STRING, nullptr, on_error, on_warning);
        if (job->png == nullptr)
        {
            error = "png_create_read_struct failed";
            return std::nullopt;
        }
        job->info = png_create_info_struct(job->png);
        if (job->info == nullptr)
        {
            error = "png_create_info_struct failed";
            return std::nullopt;
        }

        if (setjmp(png_jmpbuf(job->png)) != 0)
        {
            error = "PNG stream is corrupt or truncated";
            return std::nullopt;
        }

        png_set_user_limits(job->png, static_cast<png_uint_32>(max_dimension),
                            static_cast<png_uint_32>(max_dimension));
        png_set_read_fn(job->png, &job->source, read_callback);
        png_read_info(job->png, job->info);

        png_uint_32 width = 0;
        png_uint_32 height = 0;
        int bit_depth = 0;
        int color_type = 0;
        int interlace = 0;
        int compression = 0;
        int filter = 0;
        png_get_IHDR(job->png, job->info, &width, &height, &bit_depth, &color_type, &interlace,
                     &compression, &filter);

        if (width > static_cast<png_uint_32>(max_dimension) ||
            height > static_cast<png_uint_32>(max_dimension) ||
            !valid_extent(static_cast<int>(width), static_cast<int>(height)))
        {
            error = "PNG dimensions are unsupported";
            return std::nullopt;
        }

        const bool has_trns = png_get_valid(job->png, job->info, PNG_INFO_tRNS) != 0;
        const bool has_alpha = (color_type & PNG_COLOR_MASK_ALPHA) != 0 || has_trns;

        if (bit_depth == 16)
        {
            png_set_strip_16(job->png);
        }
        if (color_type == PNG_COLOR_TYPE_PALETTE)
        {
            png_set_palette_to_rgb(job->png);
        }
        if (color_type == PNG_COLOR_TYPE_GRAY && bit_depth < 8)
        {
            png_set_expand_gray_1_2_4_to_8(job->png);
        }
        if (has_trns)
        {
            png_set_tRNS_to_alpha(job->png);
        }
        if ((color_type & PNG_COLOR_MASK_COLOR) == 0)
        {
            // KiriKiri bitmaps are always BGR(A); expanding gray here means the
            // row layout is uniform and no per-pixel branch is needed later.
            png_set_gray_to_rgb(job->png);
        }
        if (!has_alpha)
        {
            png_set_add_alpha(job->png, 0xFF, PNG_FILLER_AFTER);
        }
        // libpng applies the colour-channel swap before appending the filler byte,
        // so an RGB row ends up as B,G,R,A without any per-pixel work here.
        png_set_bgr(job->png);

        const int passes = png_set_interlace_handling(job->png);
        png_read_update_info(job->png, job->info);

        if (png_get_channels(job->png, job->info) != 4 || png_get_bit_depth(job->png, job->info) != 8)
        {
            error = "PNG could not be reduced to 8-bit BGRA";
            return std::nullopt;
        }

        const std::size_t stride = static_cast<std::size_t>(width) * 4;
        if (png_get_rowbytes(job->png, job->info) != stride)
        {
            error = "PNG row layout is not 4 bytes per pixel";
            return std::nullopt;
        }

        job->result.width = static_cast<int>(width);
        job->result.height = static_cast<int>(height);
        // Zeroed because libpng's interlace passes write only the pixels they own;
        // every pixel is covered by some pass, so zeroes are never observable.
        job->result.bgra.assign(stride * static_cast<std::size_t>(height), 0);

        for (int pass = 0; pass < passes; ++pass)
        {
            for (png_uint_32 y = 0; y < height; ++y)
            {
                png_bytep row = job->result.bgra.data() + static_cast<std::size_t>(y) * stride;
                png_read_row(job->png, row, nullptr);
            }
        }

        return std::move(job->result);
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while decoding PNG";
        return std::nullopt;
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return std::nullopt;
    }
    catch (...)
    {
        error = "unknown failure while decoding PNG";
        return std::nullopt;
    }
}

std::vector<std::uint8_t> encode(const image& source, std::string& error)
{
    error.clear();

    if (!valid_extent(source.width, source.height))
    {
        error = "PNG dimensions are unsupported";
        return {};
    }
    const std::size_t stride = static_cast<std::size_t>(source.width) * 4;
    const std::size_t needed = stride * static_cast<std::size_t>(source.height);
    if (source.bgra.size() < needed)
    {
        error = "pixel buffer is smaller than width*height*4";
        return {};
    }

    try
    {
        auto job = std::make_unique<write_job>();

        job->png = png_create_write_struct(PNG_LIBPNG_VER_STRING, nullptr, on_error, on_warning);
        if (job->png == nullptr)
        {
            error = "png_create_write_struct failed";
            return {};
        }
        job->info = png_create_info_struct(job->png);
        if (job->info == nullptr)
        {
            error = "png_create_info_struct failed";
            return {};
        }

        if (setjmp(png_jmpbuf(job->png)) != 0)
        {
            error = "PNG encoding failed";
            return {};
        }

        png_set_write_fn(job->png, &job->sink, write_callback, nullptr);
        png_set_compression_level(job->png, 6);
        png_set_IHDR(job->png, job->info, static_cast<png_uint_32>(source.width),
                     static_cast<png_uint_32>(source.height), 8, PNG_COLOR_TYPE_RGB_ALPHA,
                     PNG_INTERLACE_NONE, PNG_COMPRESSION_TYPE_DEFAULT, PNG_FILTER_TYPE_DEFAULT);
        // The engine's buffers are BGRA; swapping on write keeps the caller's
        // layout untouched and still yields a spec-compliant RGBA file.
        png_set_bgr(job->png);
        png_write_info(job->png, job->info);

        for (int y = 0; y < source.height; ++y)
        {
            png_write_row(job->png, source.bgra.data() + static_cast<std::size_t>(y) * stride);
        }
        png_write_end(job->png, job->info);

        return std::move(job->sink.bytes);
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while encoding PNG";
        return {};
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return {};
    }
    catch (...)
    {
        error = "unknown failure while encoding PNG";
        return {};
    }
}

} // namespace enginec::third_party::png
