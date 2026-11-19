// libjpeg-turbo wrapper implementation.
//
// libjpeg's error manager calls exit() by default and expects the application to
// override error_exit with a longjmp back into the decoder. Two consequences shape
// this file:
//  * every object whose destructor must run lives in a heap-allocated job, assigned
//    before setjmp; C leaves automatic variables modified between setjmp and
//    longjmp indeterminate, and several here (the scanline buffer) are modified;
//  * the longjmp target is inside decode(), so the job is destroyed by the ordinary
//    return path even when libjpeg bailed out.

module;

// jpeglib.h deliberately does not include <setjmp.h>, but the documented way to
// recover from libjpeg's error_exit callback is setjmp/longjmp; nothing else in
// the library offers a non-aborting failure path.
#include <setjmp.h>
#include <stddef.h>
#include <stdio.h>

#include <jpeglib.h>

module enginec.third_party.jpeg;

import std;

namespace enginec::third_party::jpeg
{

namespace
{

// JPEG itself caps dimensions at 65500; the byte budget is what actually protects
// a phone, since 4 bytes per pixel multiplies quickly. 16M pixels is a 4096x4096
// image, i.e. at most 64 MiB of BGRA.
inline constexpr int max_dimension = 16384;
inline constexpr std::size_t max_pixels = std::size_t{1} << 24;

struct error_state
{
    jpeg_error_mgr base{};
    jmp_buf jump{};
    char message[JMSG_LENGTH_MAX]{};
};

void on_error_exit(j_common_ptr info)
{
    auto* state = reinterpret_cast<error_state*>(info->err);
    state->base.format_message(info, state->message);
    std::longjmp(state->jump, 1);
}

void on_output_message(j_common_ptr)
{
    // libjpeg narrates corrupt-but-recoverable data through this hook; the engine
    // has no per-asset log sink at this layer and the pixels still arrive.
}

// Owns every libjpeg and C++ allocation of one decode. A heap instance is used so
// the longjmp path never leaves a half-constructed automatic object behind.
struct decode_job
{
    jpeg_decompress_struct info{};
    error_state error{};
    bool created = false;
    std::vector<std::uint8_t> scanline;
    std::vector<std::uint8_t> pixels;

    ~decode_job()
    {
        if (created)
        {
            jpeg_destroy_decompress(&info);
        }
    }
};

// Adobe CMYK/YCCK samples are stored ink-free (255 means no ink), and libjpeg hands
// them through unchanged; a plain non-Adobe CMYK file stores the ink amount, so its
// ink-free value is the complement. Compositing the colourants against K
// multiplicatively is the same approximation browsers and every non-ICC decoder use.
void cmyk_to_bgra(const std::uint8_t* row, int width, bool adobe, std::uint8_t* out)
{
    for (int x = 0; x < width; ++x)
    {
        const int c = row[x * 4 + 0];
        const int m = row[x * 4 + 1];
        const int y = row[x * 4 + 2];
        const int k = row[x * 4 + 3];
        const int fc = adobe ? c : 255 - c;
        const int fm = adobe ? m : 255 - m;
        const int fy = adobe ? y : 255 - y;
        const int fk = adobe ? k : 255 - k;
        out[x * 4 + 0] = static_cast<std::uint8_t>((fy * fk + 127) / 255);
        out[x * 4 + 1] = static_cast<std::uint8_t>((fm * fk + 127) / 255);
        out[x * 4 + 2] = static_cast<std::uint8_t>((fc * fk + 127) / 255);
        out[x * 4 + 3] = 0xFF;
    }
}

bool valid_extent(int width, int height)
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

    // Every JPEG begins with SOI (FF D8); checking it keeps a PNG or bare garbage
    // from reaching libjpeg's error path at all.
    if (data.size() < 2 || data[0] != 0xFF || data[1] != 0xD8)
    {
        error = "missing JPEG start-of-image marker";
        return std::nullopt;
    }
    if (data.size() > static_cast<std::size_t>(std::numeric_limits<unsigned long>::max()))
    {
        error = "JPEG stream is too large to address";
        return std::nullopt;
    }

    try
    {
        auto job = std::make_unique<decode_job>();

        job->info.err = jpeg_std_error(&job->error.base);
        job->error.base.error_exit = on_error_exit;
        job->error.base.output_message = on_output_message;

        if (setjmp(job->error.jump) != 0)
        {
            error = job->error.message[0] != '\0' ? std::string(job->error.message)
                                                  : std::string("JPEG decoding failed");
            return std::nullopt;
        }

        jpeg_create_decompress(&job->info);
        job->created = true;

        jpeg_mem_src(&job->info, data.data(), static_cast<unsigned long>(data.size()));
        jpeg_read_header(&job->info, TRUE);

        const int width = static_cast<int>(job->info.image_width);
        const int height = static_cast<int>(job->info.image_height);
        if (!valid_extent(width, height))
        {
            error = "JPEG dimensions are unsupported";
            return std::nullopt;
        }

        // 4-component files (CMYK/YCCK) need the colour multiplier below; everything
        // else is asked for BGRA directly so no per-pixel swap is needed.
        const bool cmyk = job->info.num_components == 4;
        if (cmyk)
        {
            job->info.out_color_space = JCS_CMYK;
        }
        else
        {
#ifdef JCS_EXTENSIONS
            job->info.out_color_space = JCS_EXT_BGRA;
#else
            job->info.out_color_space =
                job->info.num_components == 1 ? JCS_GRAYSCALE : JCS_RGB;
#endif
        }

        jpeg_start_decompress(&job->info);

        const int out_components = static_cast<int>(job->info.output_components);
        const std::size_t stride = static_cast<std::size_t>(width) * 4;

        job->pixels.assign(stride * static_cast<std::size_t>(height), 0xFF);
        job->scanline.resize(static_cast<std::size_t>(job->info.output_width) *
                                 static_cast<std::size_t>(out_components) +
                             8);

        const bool adobe = cmyk && job->info.saw_Adobe_marker != 0;

        while (job->info.output_scanline < job->info.output_height)
        {
            const std::size_t y = static_cast<std::size_t>(job->info.output_scanline);
            JSAMPROW rows[1] = {job->scanline.data()};
            if (jpeg_read_scanlines(&job->info, rows, 1) != 1)
            {
                error = "JPEG scanline could not be decoded";
                return std::nullopt;
            }
            std::uint8_t* out = job->pixels.data() + y * stride;
            const std::uint8_t* src = job->scanline.data();

            if (cmyk)
            {
                cmyk_to_bgra(src, width, adobe, out);
            }
            else if (out_components == 4)
            {
                std::memcpy(out, src, stride);
            }
            else if (out_components == 3)
            {
                for (int x = 0; x < width; ++x)
                {
                    out[x * 4 + 0] = src[static_cast<std::size_t>(x) * 3 + 2];
                    out[x * 4 + 1] = src[static_cast<std::size_t>(x) * 3 + 1];
                    out[x * 4 + 2] = src[static_cast<std::size_t>(x) * 3 + 0];
                    out[x * 4 + 3] = 0xFF;
                }
            }
            else if (out_components == 1)
            {
                for (int x = 0; x < width; ++x)
                {
                    const std::uint8_t gray = src[x];
                    out[x * 4 + 0] = gray;
                    out[x * 4 + 1] = gray;
                    out[x * 4 + 2] = gray;
                    out[x * 4 + 3] = 0xFF;
                }
            }
            else
            {
                error = "unsupported JPEG component count " + std::to_string(out_components);
                return std::nullopt;
            }
        }

        jpeg_finish_decompress(&job->info);

        image result;
        result.width = width;
        result.height = height;
        result.bgra = std::move(job->pixels);
        return result;
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while decoding JPEG";
        return std::nullopt;
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return std::nullopt;
    }
    catch (...)
    {
        error = "unknown failure while decoding JPEG";
        return std::nullopt;
    }
}

} // namespace enginec::third_party::jpeg
