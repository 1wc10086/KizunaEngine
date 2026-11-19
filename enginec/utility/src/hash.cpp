// MD5 (RFC 1321), implemented here rather than pulled from a library.
//
// The Go engine names a game's cache directory `<cache>/kizuna/<md5(lowercased abs
// path)>` and hashes audio keys the same way (enginego/core/cache.go). Hashing a path
// is not a security decision, so MD5's broken collision resistance is irrelevant
// here; what matters is that the digest is byte-identical to Go's crypto/md5, which
// means the message length is appended little-endian and the output words are
// little-endian too.
//
// The implementation is the plain RFC 1321 loop: no tables beyond the 64 sine
// constants, one 64-byte block compiled at a time, and a buffered tail so callers can
// hash a 512 MiB asset without a second copy of it.

module enginec.utility.hash;

import std;

namespace enginec {

namespace {

constexpr std::array<std::uint32_t, 64> md5_shifts = {
    7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
    5, 9,  14, 20, 5, 9,  14, 20, 5, 9,  14, 20, 5, 9,  14, 20,
    4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
    6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
};

// floor(abs(sin(i + 1)) * 2^32): the constants RFC 1321 specifies as "the integer
// part of the absolute value of the sine".
constexpr std::array<std::uint32_t, 64> md5_constants = {
    0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee, 0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
    0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be, 0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
    0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa, 0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
    0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed, 0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
    0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c, 0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
    0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05, 0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
    0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039, 0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
    0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1, 0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391,
};

std::uint32_t rotate_left(std::uint32_t value, unsigned amount) noexcept
{
    return (value << amount) | (value >> (32u - amount));
}

std::uint32_t load_le32(const std::uint8_t* data) noexcept
{
    return static_cast<std::uint32_t>(data[0]) |
           (static_cast<std::uint32_t>(data[1]) << 8) |
           (static_cast<std::uint32_t>(data[2]) << 16) |
           (static_cast<std::uint32_t>(data[3]) << 24);
}

// Absorbs one 64-byte block into the chaining state.
void md5_block(std::array<std::uint32_t, 4>& state, const std::uint8_t* block) noexcept
{
    std::array<std::uint32_t, 16> words{};
    for (std::size_t index = 0; index < words.size(); ++index)
    {
        words[index] = load_le32(block + index * 4);
    }

    std::uint32_t a = state[0];
    std::uint32_t b = state[1];
    std::uint32_t c = state[2];
    std::uint32_t d = state[3];

    for (std::uint32_t index = 0; index < 64u; ++index)
    {
        std::uint32_t mixed = 0;
        std::uint32_t word = 0;
        if (index < 16)
        {
            mixed = (b & c) | (~b & d);
            word = index;
        }
        else if (index < 32)
        {
            mixed = (d & b) | (~d & c);
            word = (5 * index + 1) % 16;
        }
        else if (index < 48)
        {
            mixed = b ^ c ^ d;
            word = (3 * index + 5) % 16;
        }
        else
        {
            mixed = c ^ (b | ~d);
            word = (7 * index) % 16;
        }

        const auto sum = a + mixed + md5_constants[index] + words[word];
        a = d;
        d = c;
        c = b;
        b += rotate_left(sum, md5_shifts[index]);
    }

    state[0] += a;
    state[1] += b;
    state[2] += c;
    state[3] += d;
}

// Streams the input in 64-byte blocks, keeping the partial tail between calls. The
// length counter is 64-bit, so hashing a multi-gigabyte archive stays correct.
class md5_digest {
public:
    void update(std::span<const std::uint8_t> data)
    {
        total_bytes_ += data.size();

        std::size_t offset = 0;
        // Finish the block left over from the previous update first.
        if (filled_ > 0)
        {
            const auto take = std::min(buffer_.size() - filled_, data.size());
            if (take > 0)
            {
                std::copy_n(data.data(), take, buffer_.data() + filled_);
                filled_ += take;
                offset += take;
            }
            if (filled_ == buffer_.size())
            {
                md5_block(state_, buffer_.data());
                filled_ = 0;
            }
        }

        // Whole blocks straight from the caller's buffer: no copy for the bulk of an
        // asset or a script.
        while (data.size() - offset >= buffer_.size())
        {
            md5_block(state_, data.data() + offset);
            offset += buffer_.size();
        }

        if (offset < data.size())
        {
            filled_ = data.size() - offset;
            std::copy_n(data.data() + offset, filled_, buffer_.data());
        }
        // When offset == data.size() the earlier step either completed a block
        // (filled_ == 0) or left an incomplete one untouched (filled_ > 0); in both
        // cases filled_ already holds the right value and must not be reset.
    }

    std::array<std::uint8_t, 16> finish()
    {
        // Padding: 0x80, zeros until the tail is 56 mod 64, then the message length in
        // bits as a little-endian 64-bit integer. Two zero blocks are needed when the
        // 0x80 marker itself crosses into the length area, which is why the tail buffer
        // holds 128 bytes rather than 64.
        const std::uint64_t bit_length = total_bytes_ * 8;

        std::array<std::uint8_t, 128> tail{};
        std::copy_n(buffer_.data(), filled_, tail.data());
        tail[filled_] = 0x80;

        std::size_t length_offset = 0;
        if (filled_ < 56)
        {
            length_offset = 56;
        }
        else
        {
            length_offset = 120;
        }
        for (int index = 0; index < 8; ++index)
        {
            tail[length_offset + static_cast<std::size_t>(index)] =
                static_cast<std::uint8_t>(bit_length >> (index * 8));
        }

        md5_block(state_, tail.data());
        if (length_offset == 120)
        {
            md5_block(state_, tail.data() + 64);
        }

        std::array<std::uint8_t, 16> out{};
        for (std::size_t index = 0; index < state_.size(); ++index)
        {
            out[index * 4 + 0] = static_cast<std::uint8_t>(state_[index]);
            out[index * 4 + 1] = static_cast<std::uint8_t>(state_[index] >> 8);
            out[index * 4 + 2] = static_cast<std::uint8_t>(state_[index] >> 16);
            out[index * 4 + 3] = static_cast<std::uint8_t>(state_[index] >> 24);
        }
        return out;
    }

private:
    std::array<std::uint32_t, 4> state_ = {0x67452301u, 0xefcdab89u, 0x98badcfeu, 0x10325476u};
    std::array<std::uint8_t, 64> buffer_{};
    std::size_t filled_ = 0;
    std::uint64_t total_bytes_ = 0;
};

} // namespace

std::array<std::uint8_t, 16> md5(std::span<const std::uint8_t> data)
{
    md5_digest digest;
    digest.update(data);
    return digest.finish();
}

std::string md5_hex(std::string_view text)
{
    // text.data() may be null for an empty view; a span over (nullptr, 0) is legal but
    // an explicit empty span avoids relying on that corner of the standard.
    const auto bytes = text.empty()
                           ? std::span<const std::uint8_t>{}
                           : std::span<const std::uint8_t>(
                                 reinterpret_cast<const std::uint8_t*>(text.data()), text.size());
    const auto digest = md5(bytes);
    static constexpr char digits[] = "0123456789abcdef";

    // Go's hex.EncodeToString is lower case, and the value ends up in a directory
    // name that must match between the two engines.
    std::string out;
    out.reserve(digest.size() * 2);
    for (const auto byte : digest)
    {
        out.push_back(digits[byte >> 4]);
        out.push_back(digits[byte & 0x0F]);
    }
    return out;
}

} // namespace enginec
