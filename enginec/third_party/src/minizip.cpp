// minizip-ng wrapper implementation.
//
// The archive is opened through a plain OS stream rather than mz_zip_reader: the
// reader API adds its own handle type and callback plumbing, while the ZIP layer
// underneath is the same. Two behaviours matter for KiriKiri:
//  * names are matched case-insensitively with '/' and '\' treated alike, because
//    KiriKiri storage names are lower-case with '/' regardless of the ZIP author;
//  * a CRC mismatch is a real failure (a half-copied patch archive), so the entry
//    close error is propagated instead of returning the bytes that did decode.

module;

#include <mz.h>
#include <mz_strm.h>
#include <mz_strm_os.h>
#include <mz_zip.h>

module enginec.third_party.minizip;

import std;

namespace enginec::third_party::minizip
{

namespace
{

// A ZIP entry is materialised in memory by this wrapper, so the cap is the same
// one the other decoders use; the engine streams large media through XP3 instead.
inline constexpr std::uint64_t max_entry_size = std::uint64_t{1} << 30;
inline constexpr std::size_t read_chunk = std::size_t{1} << 16;

std::string error_text(std::int32_t code)
{
    switch (code)
    {
    case MZ_OK:
        return "no error";
    case MZ_STREAM_ERROR:
        return "stream error";
    case MZ_DATA_ERROR:
        return "data error (corrupt compressed stream)";
    case MZ_MEM_ERROR:
        return "out of memory";
    case MZ_BUF_ERROR:
        return "buffer error";
    case MZ_VERSION_ERROR:
        return "unsupported version";
    case MZ_END_OF_LIST:
        return "end of entry list";
    case MZ_END_OF_STREAM:
        return "end of stream";
    case MZ_PARAM_ERROR:
        return "invalid parameter";
    case MZ_FORMAT_ERROR:
        return "not a valid ZIP archive";
    case MZ_INTERNAL_ERROR:
        return "internal error";
    case MZ_CRC_ERROR:
        return "CRC mismatch (archive is corrupt)";
    case MZ_CRYPT_ERROR:
        return "decryption failed";
    case MZ_EXIST_ERROR:
        return "entry does not exist";
    case MZ_PASSWORD_ERROR:
        return "password required";
    case MZ_SUPPORT_ERROR:
        return "unsupported ZIP feature";
    case MZ_HASH_ERROR:
        return "hash verification failed";
    case MZ_OPEN_ERROR:
        return "cannot open file";
    case MZ_CLOSE_ERROR:
        return "cannot close file";
    case MZ_SEEK_ERROR:
        return "seek failed";
    case MZ_TELL_ERROR:
        return "tell failed";
    case MZ_READ_ERROR:
        return "read failed";
    case MZ_WRITE_ERROR:
        return "write failed";
    case MZ_SIGN_ERROR:
        return "signature check failed";
    case MZ_SYMLINK_ERROR:
        return "symbolic link entry";
    default:
        break;
    }
    return "minizip error " + std::to_string(code);
}

// Closes the current entry however read() leaves the loop: an exception thrown
// between read_open and close would otherwise leave the ZIP handle with a live
// decompression stream, and the next read() would leak it.
struct entry_guard
{
    void* zip = nullptr;

    ~entry_guard()
    {
        if (zip != nullptr)
        {
            const std::int32_t ignored = mz_zip_entry_close(zip);
            static_cast<void>(ignored);
        }
    }
};

bool is_directory(const mz_zip_file& file) noexcept
{
    if (mz_zip_attrib_is_dir(file.external_fa, file.version_madeby) == MZ_OK)
    {
        return true;
    }
    if (file.filename == nullptr || file.filename_size == 0)
    {
        return false;
    }
    const char last = file.filename[file.filename_size - 1];
    return last == '/' || last == '\\';
}

} // namespace

struct archive::impl
{
    void* stream = nullptr;
    void* zip = nullptr;
    std::vector<entry> listing;
    mutable std::mutex lock;

    impl() = default;
    impl(const impl&) = delete;
    impl& operator=(const impl&) = delete;

    ~impl()
    {
        reset();
    }

    void reset() noexcept
    {
        if (zip != nullptr)
        {
            mz_zip_close(zip);
            mz_zip_delete(&zip);
            zip = nullptr;
        }
        if (stream != nullptr)
        {
            mz_stream_os_close(stream);
            mz_stream_os_delete(&stream);
            stream = nullptr;
        }
        listing.clear();
    }
};

archive::archive() : impl_(std::make_unique<impl>()) {}

archive::~archive() = default;

archive::archive(archive&&) noexcept = default;

archive& archive::operator=(archive&&) noexcept = default;

bool archive::open(std::string_view path, std::string& error)
{
    error.clear();
    if (impl_ == nullptr)
    {
        impl_ = std::make_unique<impl>();
    }
    if (path.empty())
    {
        error = "archive path is empty";
        return false;
    }

    // A string_view need not be NUL-terminated, so the path is copied before the
    // C API is handed a pointer.
    std::string name(path);

    std::lock_guard<std::mutex> guard(impl_->lock);
    impl_->reset();

    try
    {
        impl_->stream = mz_stream_os_create();
        if (impl_->stream == nullptr)
        {
            error = "cannot allocate file stream";
            return false;
        }
        const std::int32_t opened = mz_stream_os_open(impl_->stream, name.c_str(), MZ_OPEN_MODE_READ);
        if (opened != MZ_OK)
        {
            error = "cannot open " + name + ": " + error_text(opened);
            impl_->reset();
            return false;
        }

        impl_->zip = mz_zip_create();
        if (impl_->zip == nullptr)
        {
            error = "cannot allocate ZIP handle";
            impl_->reset();
            return false;
        }
        const std::int32_t ready = mz_zip_open(impl_->zip, impl_->stream, MZ_OPEN_MODE_READ);
        if (ready != MZ_OK)
        {
            error = name + ": " + error_text(ready);
            impl_->reset();
            return false;
        }

        std::int32_t status = mz_zip_goto_first_entry(impl_->zip);
        while (status == MZ_OK)
        {
            mz_zip_file* file = nullptr;
            if (mz_zip_entry_get_info(impl_->zip, &file) != MZ_OK || file == nullptr)
            {
                error = name + ": central directory is damaged";
                impl_->reset();
                return false;
            }

            if (!is_directory(*file))
            {
                entry item;
                item.name = file->filename != nullptr ? std::string(file->filename) : std::string();
                item.size = file->uncompressed_size < 0
                                ? 0
                                : static_cast<std::uint64_t>(file->uncompressed_size);
                item.compressed_size = file->compressed_size < 0
                                           ? 0
                                           : static_cast<std::uint64_t>(file->compressed_size);
                impl_->listing.push_back(std::move(item));
            }

            status = mz_zip_goto_next_entry(impl_->zip);
        }

        // Only the documented end-of-list condition is a clean finish; anything else
        // means the central directory runs past the end of the file.
        if (status != MZ_END_OF_LIST)
        {
            error = name + ": " + error_text(status);
            impl_->reset();
            return false;
        }

        return true;
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while opening archive";
        impl_->reset();
        return false;
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        impl_->reset();
        return false;
    }
    catch (...)
    {
        error = "unknown failure while opening archive";
        impl_->reset();
        return false;
    }
}

void archive::close()
{
    if (impl_ == nullptr)
    {
        return;
    }
    std::lock_guard<std::mutex> guard(impl_->lock);
    impl_->reset();
}

const std::vector<entry>& archive::entries() const noexcept
{
    static const std::vector<entry> empty;
    if (impl_ == nullptr)
    {
        return empty;
    }
    return impl_->listing;
}

std::optional<std::vector<std::uint8_t>> archive::read(std::string_view name,
                                                       std::string& error) const
{
    error.clear();
    if (impl_ == nullptr || impl_->zip == nullptr)
    {
        error = "archive is not open";
        return std::nullopt;
    }
    if (name.empty())
    {
        error = "entry name is empty";
        return std::nullopt;
    }

    // A string_view is not NUL-terminated and the error messages below embed the
    // name, so one copy is made before any C API sees it.
    std::string key(name);

    std::lock_guard<std::mutex> guard(impl_->lock);

    try
    {
        const std::int32_t located = mz_zip_locate_entry(impl_->zip, key.c_str(), 1);
        if (located == MZ_END_OF_LIST)
        {
            error = "entry not found: " + key;
            return std::nullopt;
        }
        if (located != MZ_OK)
        {
            error = "cannot locate " + key + ": " + error_text(located);
            return std::nullopt;
        }

        mz_zip_file* file = nullptr;
        if (mz_zip_entry_get_info(impl_->zip, &file) != MZ_OK || file == nullptr)
        {
            error = "cannot read the header of " + key;
            return std::nullopt;
        }
        if ((file->flag & MZ_ZIP_FLAG_ENCRYPTED) != 0)
        {
            error = "entry is encrypted: " + key;
            return std::nullopt;
        }
        if (is_directory(*file))
        {
            error = "entry is a directory: " + key;
            return std::nullopt;
        }

        const std::int64_t declared = file->uncompressed_size;
        if (declared < 0)
        {
            error = "entry has an invalid size: " + key;
            return std::nullopt;
        }
        if (static_cast<std::uint64_t>(declared) > max_entry_size)
        {
            error = "entry " + key + " exceeds the extraction limit";
            return std::nullopt;
        }

        const std::int32_t opened = mz_zip_entry_read_open(impl_->zip, 0, nullptr);
        if (opened != MZ_OK)
        {
            error = "cannot open " + key + ": " + error_text(opened);
            return std::nullopt;
        }
        // Armed immediately after the successful open: everything below can throw
        // or return early, and the ZIP handle must not keep a live entry stream.
        entry_guard guard_entry{impl_->zip};

        std::vector<std::uint8_t> out;
        // The central directory's size is attacker-controlled, so reserving it
        // verbatim would let a bad archive force a huge allocation up front.
        out.reserve(static_cast<std::size_t>(std::min<std::uint64_t>(
            static_cast<std::uint64_t>(declared), read_chunk)));

        for (;;)
        {
            const std::size_t before = out.size();
            out.resize(before + read_chunk);
            const std::int32_t got = mz_zip_entry_read(
                impl_->zip, out.data() + before, static_cast<std::int32_t>(read_chunk));

            if (got < 0)
            {
                out.resize(before);
                error = "read failed in " + key + ": " + error_text(got);
                return std::nullopt;
            }
            out.resize(before + static_cast<std::size_t>(got));
            if (out.size() > max_entry_size)
            {
                error = "entry " + key + " exceeds the extraction limit";
                return std::nullopt;
            }
            if (got == 0)
            {
                break;
            }
        }

        // Closing verifies the CRC over everything that was read, so a truncated or
        // tampered entry is rejected here rather than handed to the caller.
        const std::int32_t closed = mz_zip_entry_close(impl_->zip);
        guard_entry.zip = nullptr;
        if (closed != MZ_OK)
        {
            error = "cannot finish " + key + ": " + error_text(closed);
            return std::nullopt;
        }

        return out;
    }
    catch (const std::bad_alloc&)
    {
        error = "out of memory while extracting " + key;
        return std::nullopt;
    }
    catch (const std::exception& failure)
    {
        error = failure.what();
        return std::nullopt;
    }
    catch (...)
    {
        error = "unknown failure while extracting " + key;
        return std::nullopt;
    }
}

} // namespace enginec::third_party::minizip
