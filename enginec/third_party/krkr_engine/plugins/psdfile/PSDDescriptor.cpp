// Photoshop descriptor tree (see psd.cppm for the module layout).
module enginec.krkr.psd;

import std;

namespace kzpsd
{
namespace
{

constexpr std::uint32_t tag_reference = make_tag('o', 'b', 'j', ' ');
constexpr std::uint32_t tag_global_object = make_tag('G', 'l', 'b', 'O');
constexpr std::uint32_t tag_object = make_tag('O', 'b', 'j', 'c');
constexpr std::uint32_t tag_list = make_tag('V', 'l', 'L', 's');
constexpr std::uint32_t tag_double = make_tag('d', 'o', 'u', 'b');
constexpr std::uint32_t tag_unit_float = make_tag('U', 'n', 't', 'F');
constexpr std::uint32_t tag_text = make_tag('T', 'E', 'X', 'T');
constexpr std::uint32_t tag_enum = make_tag('e', 'n', 'u', 'm');
constexpr std::uint32_t tag_integer = make_tag('l', 'o', 'n', 'g');
constexpr std::uint32_t tag_bool = make_tag('b', 'o', 'o', 'l');
constexpr std::uint32_t tag_class1 = make_tag('t', 'y', 'p', 'e');
constexpr std::uint32_t tag_class2 = make_tag('G', 'l', 'b', 'C');
constexpr std::uint32_t tag_alias = make_tag('a', 'l', 'i', 's');
constexpr std::uint32_t tag_raw_data = make_tag('t', 'd', 't', 'a');

constexpr std::uint32_t ref_property = make_tag('p', 'r', 'o', 'p');
constexpr std::uint32_t ref_class = make_tag('C', 'l', 's', 's');
constexpr std::uint32_t ref_enum = make_tag('E', 'n', 'm', 'r');
constexpr std::uint32_t ref_offset = make_tag('r', 'e', 'l', 'e');
constexpr std::uint32_t ref_identifier = make_tag('I', 'd', 'n', 't');
constexpr std::uint32_t ref_index = make_tag('i', 'n', 'd', 'x');
constexpr std::uint32_t ref_name = make_tag('n', 'a', 'm', 'e');

// An "id" is a length-prefixed byte string; a length of zero means the usual
// four-character key.
std::string read_id(Reader& reader)
{
    auto length = reader.u32();
    if (length == 0)
        length = 4;
    if (length > reader.remaining())
    {
        reader.skip(reader.remaining());
        return {};
    }
    std::string text(length, '\0');
    reader.read(text.data(), length);
    const auto nul = text.find('\0');
    if (nul != std::string::npos)
        text.resize(nul);
    return text;
}

bool read_value(Reader& reader, DescriptorValue& out);

// Reference entries store how to address an object (property name, class,
// identifier...). Only their length matters for parsing a comps tree, but each
// form has to be consumed exactly or the next item is misaligned.
bool skip_reference(Reader& reader)
{
    const auto count = reader.u32();
    for (std::uint32_t i = 0; i < count; ++i)
    {
        const auto kind = reader.tag();
        switch (kind)
        {
            case ref_property:
            {
                std::u16string name;
                reader.utf16(name);
                read_id(reader);
                read_id(reader);
                break;
            }
            case ref_class:
            {
                std::u16string name;
                reader.utf16(name);
                read_id(reader);
                break;
            }
            case ref_enum:
            {
                std::u16string name;
                reader.utf16(name);
                read_id(reader);
                read_id(reader);
                read_id(reader);
                break;
            }
            case ref_offset:
            {
                std::u16string name;
                reader.utf16(name);
                read_id(reader);
                reader.i32();
                break;
            }
            case ref_identifier:
            case ref_index:
                reader.i32();
                break;
            case ref_name:
            {
                std::u16string name;
                reader.utf16(name);
                break;
            }
            default:
                return false;
        }
        if (!reader.valid())
            return false;
    }
    return true;
}

bool read_value(Reader& reader, DescriptorValue& out)
{
    const auto type = reader.tag();
    switch (type)
    {
        case tag_object:
        case tag_global_object:
            return read_descriptor(reader, out);
        case tag_reference:
            return skip_reference(reader);
        case tag_list:
        {
            out.kind = DescriptorValue::Kind::List;
            const auto count = reader.u32();
            // A list element is at least its four-byte type, so a count larger
            // than the remaining bytes cannot be genuine.
            if (count > reader.remaining() / 4)
                return false;
            out.items.reserve(count);
            for (std::uint32_t i = 0; i < count; ++i)
            {
                DescriptorValue item;
                if (!read_value(reader, item))
                    return false;
                out.items.push_back(std::move(item));
            }
            return true;
        }
        case tag_double:
            out.kind = DescriptorValue::Kind::Real;
            out.real = reader.f64();
            return reader.valid();
        case tag_unit_float:
        {
            out.kind = DescriptorValue::Kind::Real;
            out.classId = read_id(reader);
            out.real = reader.f64();
            return reader.valid();
        }
        case tag_text:
        {
            out.kind = DescriptorValue::Kind::Text;
            std::u16string text;
            reader.utf16(text);
            out.text = utf16_to_utf8(text);
            return reader.valid();
        }
        case tag_enum:
        {
            out.kind = DescriptorValue::Kind::Enum;
            out.typeId = read_id(reader);
            out.enumId = read_id(reader);
            return reader.valid();
        }
        case tag_integer:
            out.kind = DescriptorValue::Kind::Integer;
            out.integer = reader.i32();
            return reader.valid();
        case tag_bool:
            out.kind = DescriptorValue::Kind::Bool;
            out.boolean = reader.u8() != 0;
            return reader.valid();
        case tag_class1:
        case tag_class2:
        {
            out.kind = DescriptorValue::Kind::Class;
            std::u16string name;
            reader.utf16(name);
            out.text = utf16_to_utf8(name);
            out.classId = read_id(reader);
            return reader.valid();
        }
        case tag_alias:
        {
            out.kind = DescriptorValue::Kind::Alias;
            const auto size = reader.u32();
            if (size > reader.remaining())
                return false;
            out.text.resize(size);
            reader.read(out.text.data(), size);
            return reader.valid();
        }
        case tag_raw_data:
            // Raw data carries no length prefix, so the rest of the block cannot
            // be skipped reliably. The reference parser stops here as well.
            return false;
        default:
            return false;
    }
}

} // namespace

std::string utf16_to_utf8(const std::u16string& text)
{
    std::string out;
    out.reserve(text.size());
    for (std::size_t index = 0; index < text.size(); ++index)
    {
        std::uint32_t code = text[index];
        if (code >= 0xd800 && code <= 0xdbff && index + 1 < text.size())
        {
            const auto low = text[index + 1];
            if (low >= 0xdc00 && low <= 0xdfff)
            {
                code = 0x10000 + ((code - 0xd800) << 10) + (low - 0xdc00);
                ++index;
            }
        }
        if (code < 0x80)
            out.push_back(static_cast<char>(code));
        else if (code < 0x800)
        {
            out.push_back(static_cast<char>(0xc0 | (code >> 6)));
            out.push_back(static_cast<char>(0x80 | (code & 0x3f)));
        }
        else if (code < 0x10000)
        {
            out.push_back(static_cast<char>(0xe0 | (code >> 12)));
            out.push_back(static_cast<char>(0x80 | ((code >> 6) & 0x3f)));
            out.push_back(static_cast<char>(0x80 | (code & 0x3f)));
        }
        else
        {
            out.push_back(static_cast<char>(0xf0 | (code >> 18)));
            out.push_back(static_cast<char>(0x80 | ((code >> 12) & 0x3f)));
            out.push_back(static_cast<char>(0x80 | ((code >> 6) & 0x3f)));
            out.push_back(static_cast<char>(0x80 | (code & 0x3f)));
        }
    }
    return out;
}

bool read_descriptor(Reader& reader, DescriptorValue& out)
{
    out = DescriptorValue{};
    std::u16string name;
    reader.utf16(name);
    read_id(reader); // class id

    out.kind = DescriptorValue::Kind::Object;
    const auto count = reader.u32();
    if (!reader.valid() || count > reader.remaining() / 8)
        return false;
    for (std::uint32_t i = 0; i < count; ++i)
    {
        std::string key = read_id(reader);
        DescriptorValue value;
        if (!read_value(reader, value))
            return false;
        out.fields.emplace(std::move(key), std::move(value));
    }
    return reader.valid();
}

bool read_descriptor_block(Reader& reader, DescriptorValue& out)
{
    const auto version = reader.u32();
    if (version != 16)
        return false;
    return read_descriptor(reader, out);
}

} // namespace kzpsd
