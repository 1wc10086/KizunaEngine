// Implementation of the Kizuna wire codec.
//
// Deliberately mirrors enginego/wire byte for byte: clients built against the Go
// engine must be able to talk to this core without a protocol version bump. The
// reader keeps spans into the caller's buffer instead of copying (the request lives
// for the length of one dispatch), and every length is validated against the bytes
// actually available so a hostile packet can never make us index past the end.

module enginec.feature.wire;

import std;

namespace enginec::wire {

namespace {

constexpr std::size_t header_size = 16;
constexpr std::size_t tlv_head_size = 8;

std::uint16_t load_u16(std::span<const std::uint8_t> data, std::size_t offset) noexcept
{
    if (offset + 2 > data.size())
    {
        return 0;
    }
    return static_cast<std::uint16_t>(
        static_cast<std::uint16_t>(data[offset]) |
        (static_cast<std::uint16_t>(data[offset + 1]) << 8));
}

std::uint32_t load_u32(std::span<const std::uint8_t> data, std::size_t offset) noexcept
{
    if (offset + 4 > data.size())
    {
        return 0;
    }
    return static_cast<std::uint32_t>(data[offset]) |
           (static_cast<std::uint32_t>(data[offset + 1]) << 8) |
           (static_cast<std::uint32_t>(data[offset + 2]) << 16) |
           (static_cast<std::uint32_t>(data[offset + 3]) << 24);
}

std::uint64_t load_u64(std::span<const std::uint8_t> data, std::size_t offset) noexcept
{
    if (offset + 8 > data.size())
    {
        return 0;
    }
    return static_cast<std::uint64_t>(load_u32(data, offset)) |
           (static_cast<std::uint64_t>(load_u32(data, offset + 4)) << 32);
}

void store_u16(std::vector<std::uint8_t>& out, std::size_t offset, std::uint16_t value)
{
    out[offset] = static_cast<std::uint8_t>(value & 0xff);
    out[offset + 1] = static_cast<std::uint8_t>((value >> 8) & 0xff);
}

void store_u32(std::vector<std::uint8_t>& out, std::size_t offset, std::uint32_t value)
{
    for (int shift = 0; shift < 32; shift += 8)
    {
        out[offset + static_cast<std::size_t>(shift / 8)] = static_cast<std::uint8_t>(value >> shift);
    }
}

// std::string(const char*, size) is undefined for a null pointer even with size 0,
// and an empty field is common, so empty values are built without touching data().
std::string as_string(std::span<const std::uint8_t> value)
{
    if (value.empty())
    {
        return {};
    }
    return std::string(reinterpret_cast<const char*>(value.data()), value.size());
}

// Walks the TLV elements of a list body. A declared length that overruns the body is
// the signature of a truncated list: the walk ends there, because emitting a clamped
// half-value would invent data and skipping the element would shift every later
// field. Ending the walk yields the same prefix the Go reader returns.
template <typename Fn>
void walk_list(std::span<const std::uint8_t> body, Fn&& visit)
{
    while (body.size() >= tlv_head_size)
    {
        const auto kind = static_cast<type>(body[2]);
        const std::size_t declared = load_u32(body, 4);
        const std::size_t available = body.size() - tlv_head_size;
        if (declared > available)
        {
            return;
        }
        visit(kind, body.subspan(tlv_head_size, declared));
        body = body.subspan(tlv_head_size + declared);
    }
}

} // namespace

reader::reader(std::span<const std::uint8_t> payload)
{
    // A duplicate id must keep the last value (the Go reader writes into a map with
    // the same semantics), so later fields overwrite earlier ones.
    std::size_t offset = 0;
    while (payload.size() - offset >= tlv_head_size)
    {
        const auto id = load_u16(payload, offset);
        const auto kind = static_cast<type>(payload[offset + 2]);
        const auto length = load_u32(payload, offset + 4);
        offset += tlv_head_size;
        if (static_cast<std::size_t>(length) > payload.size() - offset)
        {
            // Truncated tail: the field is dropped rather than stored with a short
            // span, because every accessor would otherwise read a partial value.
            break;
        }
        fields_.insert_or_assign(id, field_value{kind, payload.subspan(offset, length)});
        offset += length;
    }
}

bool reader::has(std::uint16_t id) const
{
    return fields_.contains(id);
}

std::span<const std::uint8_t> reader::raw(std::uint16_t id) const
{
    const auto found = fields_.find(id);
    if (found == fields_.end())
    {
        return {};
    }
    return found->second.data;
}

std::uint32_t reader::u32(std::uint16_t id) const
{
    return load_u32(raw(id), 0);
}

std::int32_t reader::i32(std::uint16_t id) const
{
    return static_cast<std::int32_t>(u32(id));
}

std::uint64_t reader::u64(std::uint16_t id) const
{
    return load_u64(raw(id), 0);
}

double reader::f64(std::uint16_t id) const
{
    return std::bit_cast<double>(u64(id));
}

bool reader::boolean(std::uint16_t id) const
{
    const auto value = raw(id);
    return !value.empty() && value.front() != 0;
}

std::string reader::str(std::uint16_t id) const
{
    return as_string(raw(id));
}

std::vector<std::string> reader::str_list(std::uint16_t id) const
{
    std::vector<std::string> out;
    const auto found = fields_.find(id);
    if (found == fields_.end() || found->second.kind != type::list)
    {
        return out;
    }
    walk_list(found->second.data, [&out](type kind, std::span<const std::uint8_t> value) {
        // The element id is ignored on read: the Go writer emits 0 for every entry,
        // but older clients used varying ids and must stay readable.
        if (kind == type::str)
        {
            out.push_back(as_string(value));
        }
    });
    return out;
}

void reader::each(std::uint16_t id, const std::function<void(reader)>& fn) const
{
    if (!fn)
    {
        return;
    }
    const auto found = fields_.find(id);
    if (found == fields_.end() || found->second.kind != type::list)
    {
        return;
    }
    walk_list(found->second.data, [&fn](type kind, std::span<const std::uint8_t> value) {
        // Nested lists are the only shape `each` hands out; scalar entries are read
        // by the typed accessors on the parent list instead.
        if (kind == type::list)
        {
            fn(reader(value));
        }
    });
}

void writer::head(std::uint16_t id, type kind, std::size_t length)
{
    const auto base = buffer_.size();
    buffer_.resize(base + tlv_head_size);
    store_u16(buffer_, base, id);
    buffer_[base + 2] = static_cast<std::uint8_t>(kind);
    buffer_[base + 3] = 0; // reserved, always emitted as zero
    store_u32(buffer_, base + 4, static_cast<std::uint32_t>(length));
}

void writer::str(std::uint16_t id, std::string_view value)
{
    head(id, type::str, value.size());
    // string_view::data() may be null when empty; forming iterators from it would be
    // undefined even for a zero-length range.
    if (!value.empty())
    {
        const auto* first = reinterpret_cast<const std::uint8_t*>(value.data());
        buffer_.insert(buffer_.end(), first, first + value.size());
    }
}

void writer::u32(std::uint16_t id, std::uint32_t value)
{
    head(id, type::u32, 4);
    const auto base = buffer_.size();
    buffer_.resize(base + 4);
    store_u32(buffer_, base, value);
}

void writer::i32(std::uint16_t id, std::int32_t value)
{
    head(id, type::i32, 4);
    const auto base = buffer_.size();
    buffer_.resize(base + 4);
    store_u32(buffer_, base, static_cast<std::uint32_t>(value));
}

void writer::u64(std::uint16_t id, std::uint64_t value)
{
    head(id, type::u64, 8);
    const auto base = buffer_.size();
    buffer_.resize(base + 8);
    store_u32(buffer_, base, static_cast<std::uint32_t>(value));
    store_u32(buffer_, base + 4, static_cast<std::uint32_t>(value >> 32));
}

void writer::f64(std::uint16_t id, double value)
{
    head(id, type::f64, 8);
    const auto base = buffer_.size();
    buffer_.resize(base + 8);
    const std::uint64_t bits = std::bit_cast<std::uint64_t>(value);
    store_u32(buffer_, base, static_cast<std::uint32_t>(bits));
    store_u32(buffer_, base + 4, static_cast<std::uint32_t>(bits >> 32));
}

void writer::boolean(std::uint16_t id, bool value)
{
    head(id, type::boolean, 1);
    buffer_.push_back(value ? std::uint8_t{1} : std::uint8_t{0});
}

void writer::raw(std::uint16_t id, std::span<const std::uint8_t> value)
{
    head(id, type::bytes_, value.size());
    if (!value.empty())
    {
        buffer_.insert(buffer_.end(), value.begin(), value.end());
    }
}

void writer::str_list(std::uint16_t id, const std::vector<std::string>& values)
{
    list(id, [&values](writer& sub) {
        for (const auto& value : values)
        {
            sub.str(0, value);
        }
    });
}

void writer::list(std::uint16_t id, const std::function<void(writer&)>& fn)
{
    const auto base = buffer_.size();
    head(id, type::list, 0);
    if (fn)
    {
        fn(*this);
    }
    // Patch the length once the body exists. The offset is captured rather than a
    // pointer because the body may reallocate the buffer while it is appended to.
    store_u32(buffer_, base + 4, static_cast<std::uint32_t>(buffer_.size() - base - tlv_head_size));
}

std::optional<request> parse_request(std::span<const std::uint8_t> data)
{
    if (data.size() < header_size)
    {
        return std::nullopt;
    }
    if (data[0] != 'K' || data[1] != 'Z' || data[2] != 'E' || data[3] != 'Q')
    {
        return std::nullopt;
    }
    const auto op = load_u16(data, 4);
    // Bytes 6-7 are reserved and ignored: old clients filled them with padding.
    const auto seq = load_u32(data, 8);
    const std::size_t payload_size = load_u32(data, 12);
    if (payload_size > data.size() - header_size)
    {
        // A payload shorter than the header claims cannot be trusted at all; the Go
        // parser rejects the whole request rather than reading a partial field set.
        return std::nullopt;
    }

    request out;
    out.op = op;
    out.seq = seq;
    out.payload = reader(data.subspan(header_size, payload_size));
    return out;
}

std::vector<std::uint8_t> build_response(std::uint16_t status, std::uint32_t seq,
                                         const std::vector<std::uint8_t>& payload)
{
    // The header is written first and the payload appended afterwards, so the length
    // field is exact without pre-sizing the buffer to its final shape.
    std::vector<std::uint8_t> out(header_size, 0);
    out[0] = 'K';
    out[1] = 'Z';
    out[2] = 'E';
    out[3] = 'S';
    store_u16(out, 4, status);
    store_u16(out, 6, 0);
    store_u32(out, 8, seq);
    store_u32(out, 12, static_cast<std::uint32_t>(payload.size()));
    out.insert(out.end(), payload.begin(), payload.end());
    return out;
}

std::vector<std::uint8_t> build_error(std::uint32_t seq, std::string_view message,
                                      std::uint16_t status)
{
    // Field 42 is the message field the Dart decoder reads from an error response; it
    // is not part of the shared field table because only errors carry it.
    writer body;
    body.str(field::f42, message);
    return build_response(status, seq, body.data());
}

} // namespace enginec::wire
