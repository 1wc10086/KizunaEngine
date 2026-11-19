// PSD parser: turns the file bytes into the model in psd.cppm.
//
// The layout follows the published PSD specification. Where the reference
// plugin ignores a field we ignore it too, but everything a script can observe
// (layer geometry, names, blend mode, ids, folder structure, masks, guides,
// slices) is parsed even when a game never asks for it: a wrong default here
// would surface as a wrong pixel much later.
module enginec.krkr.psd;

import std;

namespace kzpsd
{
namespace
{

// Refuse absurd geometry before any multiplication. Photoshop itself caps a
// document at 300000 x 300000; the tighter bound keeps size arithmetic inside
// int and the decoded buffer inside what a phone can allocate.
constexpr int max_dimension = 300000;
constexpr int max_channels = 64;

// Declared before use because the resource pass calls into the slice parser and
// the layer pass calls into the section-divider helpers.
void parse_slices(Reader& data, Document& document);
void parse_layer_metadata(Reader& block, Layer& layer);
void parse_layer_comps(Reader& data, Document& document);
void parse_section_divider(Reader& block, Layer& layer);
void apply_layer_type_key(std::uint32_t key, Layer& layer);

bool parse_header(Reader& reader, Document& document, std::string& error)
{
    if (reader.tag() != tag_8bps)
    {
        error = "not a PSD file";
        return false;
    }
    const auto version = reader.u16();
    if (version != 1)
    {
        error = "unsupported PSD version";
        return false;
    }
    if (!reader.skip(6)) // reserved, must be zero
    {
        error = "truncated header";
        return false;
    }
    document.channels = reader.u16();
    document.height = static_cast<int>(reader.u32());
    document.width = static_cast<int>(reader.u32());
    document.depth = reader.u16();
    const auto mode = reader.u16();
    if (!reader.valid())
    {
        error = "truncated header";
        return false;
    }
    if (document.width <= 0 || document.height <= 0 || document.width > max_dimension ||
        document.height > max_dimension)
    {
        error = "invalid image size";
        return false;
    }
    if (document.channels <= 0 || document.channels > max_channels)
    {
        error = "invalid channel count";
        return false;
    }
    if (document.depth != 1 && document.depth != 8 && document.depth != 16 &&
        document.depth != 32)
    {
        error = "unsupported bit depth";
        return false;
    }
    document.mode = static_cast<ColorMode>(mode);
    return true;
}

// Colour mode data is the indexed palette for mode 2 and print settings
// otherwise; only the palette has script-visible meaning.
void parse_color_mode(Reader& reader, Document& document)
{
    const auto size = reader.u32();
    if (size == 0 || !reader.valid())
        return;
    auto block = reader.take(size);
    if (document.mode != ColorMode::Indexed)
        return;
    // The palette is three planes of 256 bytes. A short or odd-sized palette is
    // still usable for the entries it contains, so copy what is there.
    const auto count = std::min<std::size_t>(block.remaining(), 768);
    document.colorTable.resize(768, 0);
    block.read(document.colorTable.data(), count);
}

// Parse the image resource section: a sequence of tagged records. Only the
// records listed here affect scripts; the rest are skipped by their length.
void parse_image_resources(Reader& reader, Document& document)
{
    while (reader.remaining() >= 12 && reader.valid())
    {
        const auto signature = reader.tag();
        if (signature != tag_8bim && signature != make_tag('M', 'e', 'S', 'a') &&
            signature != make_tag('P', 'H', 'U', 'T'))
            return; // no longer at a record boundary; stop instead of guessing
        const auto id = reader.u16();
        const auto nameLength = reader.u8();
        if (!reader.skip(nameLength))
            return;
        if (((nameLength + 1) & 1) != 0) // Pascal string occupies nameLength+1 bytes
            reader.skip(1);
        const auto size = reader.u32();
        if (!reader.valid())
            return;
        auto data = reader.take(size);
        if (size & 1u)
            reader.skip(1); // records are padded to an even length

        switch (id)
        {
            case 1032: // grid and guides
            {
                data.u32(); // version
                document.guides.horizontalGrid = static_cast<int>(data.u32());
                document.guides.verticalGrid = static_cast<int>(data.u32());
                const auto count = data.u32();
                // Each guide is five bytes; reject counts the payload cannot hold
                // so a corrupt count does not spin the loop.
                if (count > data.remaining() / 5)
                    break;
                for (std::uint32_t i = 0; i < count; ++i)
                {
                    Guide guide;
                    guide.location = static_cast<int>(data.u32());
                    guide.vertical = data.u8() == 0;
                    document.guides.guides.push_back(guide);
                }
                document.guides.enabled = true;
                break;
            }
            case 1046: // indexed colour table count
                document.colorTable.resize(768, 0);
                break;
            case 1047: // transparency index
                document.transparencyIndex = data.i16();
                break;
            case 1050: // slices
                parse_slices(data, document);
                break;
            case 1065: // layer comps
                parse_layer_comps(data, document);
                break;
            default:
                break;
        }
    }
}

void parse_slices(Reader& data, Document& document)
{
    const auto version = data.u32();
    if (version != 6)
    {
        // Versions 7/8 store a descriptor instead; nothing here is script-visible
        // in a way this port supports, so leave the resource disabled.
        return;
    }
    auto& slices = document.slices;
    slices.boundingLeft = data.i32();
    slices.boundingTop = data.i32();
    slices.boundingRight = data.i32();
    slices.boundingBottom = data.i32();
    data.utf16(slices.groupName);
    const auto count = data.u32();
    // Every item has at least 48 bytes of fixed fields plus five strings; use a
    // conservative minimum so a corrupt count cannot allocate without bound.
    if (count > data.remaining() / 12)
        return;
    for (std::uint32_t i = 0; i < count; ++i)
    {
        SliceItem item;
        item.id = data.i32();
        item.groupId = data.i32();
        item.origin = data.i32();
        if (item.origin == 1)
            item.associatedLayerId = data.i32();
        data.utf16(item.name);
        item.type = data.i32();
        item.left = data.i32();
        item.top = data.i32();
        item.right = data.i32();
        item.bottom = data.i32();
        data.utf16(item.url);
        data.utf16(item.target);
        data.utf16(item.message);
        data.utf16(item.altTag);
        item.cellTextIsHtml = data.u8() != 0;
        data.utf16(item.cellText);
        item.horizontalAlign = data.i32();
        item.verticalAlign = data.i32();
        const auto a = data.u8();
        const auto r = data.u8();
        const auto g = data.u8();
        const auto b = data.u8();
        item.color = (static_cast<std::uint32_t>(a) << 24) |
                     (static_cast<std::uint32_t>(r) << 16) |
                     (static_cast<std::uint32_t>(g) << 8) | b;
        slices.items.push_back(std::move(item));
        if (!data.valid())
            return;
    }
    slices.enabled = true;
}

// Reads an integer field of a descriptor, returning `fallback` when the key is
// absent or holds another type.
std::int64_t descriptor_integer(const DescriptorValue* object, std::string_view key,
                                std::int64_t fallback)
{
    const auto* value = object != nullptr ? object->find(key) : nullptr;
    if (value == nullptr)
        return fallback;
    switch (value->kind)
    {
        case DescriptorValue::Kind::Integer: return value->integer;
        case DescriptorValue::Kind::Real: return static_cast<std::int64_t>(value->real);
        case DescriptorValue::Kind::Bool: return value->boolean ? 1 : 0;
        default: return fallback;
    }
}

std::string descriptor_text(const DescriptorValue* object, std::string_view key)
{
    const auto* value = object != nullptr ? object->find(key) : nullptr;
    return value != nullptr && value->kind == DescriptorValue::Kind::Text ? value->text
                                                                          : std::string();
}

// Layer comps are stored twice: the document-level resource names them, the
// per-layer 'shmd'/'cmls' block records where each layer sits in every comp.
void parse_layer_comps(Reader& data, Document& document)
{
    DescriptorValue descriptor;
    if (!read_descriptor_block(data, descriptor))
        return;
    document.lastAppliedCompId = static_cast<int>(
        descriptor_integer(&descriptor, "lastAppliedComp", -1));
    const auto* list = descriptor.find("list");
    if (list == nullptr || list->kind != DescriptorValue::Kind::List)
        return;
    for (const auto& entry : list->items)
    {
        if (entry.kind != DescriptorValue::Kind::Object)
            continue;
        LayerComp comp;
        comp.id = static_cast<int>(descriptor_integer(&entry, "compID", -1));
        const auto captured = descriptor_integer(&entry, "capturedInfo", 0);
        comp.recordVisibility = (captured & 1) != 0;
        comp.recordPosition = (captured & 2) != 0;
        comp.recordAppearance = (captured & 4) != 0;
        comp.name = descriptor_text(&entry, "Nm  ");
        comp.comment = descriptor_text(&entry, "comment");
        document.layerComps.push_back(std::move(comp));
    }
}

void parse_layer_metadata(Reader& block, Layer& layer)
{
    const auto count = block.u32();
    if (!block.valid() || count > block.remaining() / 12)
        return;
    for (std::uint32_t i = 0; i < count; ++i)
    {
        block.tag(); // signature
        const auto key = block.tag();
        block.u8();  // copy-on-sheet-duplicate flag
        block.skip(3);
        const auto size = block.u32();
        if (!block.valid())
            return;
        auto data = block.take(size);

        // 'cmls' holds a descriptor whose layerSettings list describes this
        // layer inside every comp.
        if (key != make_tag('c', 'm', 'l', 's'))
            continue;
        DescriptorValue descriptor;
        if (!read_descriptor_block(data, descriptor))
            continue;
        const auto* settings = descriptor.find("layerSettings");
        if (settings == nullptr || settings->kind != DescriptorValue::Kind::List)
            continue;
        bool previousEnabled = true;
        for (const auto& comp : settings->items)
        {
            if (comp.kind != DescriptorValue::Kind::Object)
                continue;
            const auto* compList = comp.find("compList");
            if (compList == nullptr || compList->kind != DescriptorValue::Kind::List ||
                compList->items.empty() ||
                compList->items.front().kind != DescriptorValue::Kind::Integer)
                continue;
            LayerCompInfo info;
            info.id = static_cast<int>(compList->items.front().integer);
            const auto* offset = comp.find("Ofst");
            if (offset != nullptr && offset->kind == DescriptorValue::Kind::Object)
            {
                info.offsetX = static_cast<int>(descriptor_integer(offset, "Hrzn", 0));
                info.offsetY = static_cast<int>(descriptor_integer(offset, "Vrtc", 0));
            }
            // 'enab' is delta-encoded: a missing value means "same as the
            // previous comp", which is how Photoshop keeps the stream short.
            const auto* enabled = comp.find("enab");
            if (enabled != nullptr && enabled->kind == DescriptorValue::Kind::Bool)
                previousEnabled = enabled->boolean;
            info.enabled = previousEnabled;
            layer.layerComps[info.id] = info;
        }
    }
}

// Layer extra data: mask, blending ranges, Pascal name, then a sequence of
// "additional layer information" records, each carrying a four-character key.

void parse_layer_extra(Reader& extra, Layer& layer)
{
    // Mask data.
    const auto maskSize = extra.u32();
    if (extra.valid() && maskSize >= 18)
    {
        auto maskReader = extra.take(maskSize);
        layer.mask.top = maskReader.i32();
        layer.mask.left = maskReader.i32();
        layer.mask.bottom = maskReader.i32();
        layer.mask.right = maskReader.i32();
        layer.mask.defaultColor = maskReader.u8();
        layer.mask.flags = maskReader.u8();
        layer.mask.present = maskReader.valid();
        layer.mask.width = layer.mask.right - layer.mask.left;
        layer.mask.height = layer.mask.bottom - layer.mask.top;
    }
    else if (maskSize > 0)
    {
        extra.skip(maskSize);
    }

    // Blending ranges are not interpreted by the reference plugin either, but
    // they must be consumed to reach the name.
    const auto rangesSize = extra.u32();
    extra.skip(rangesSize);

    // Pascal string, padded so the following record starts on a 4-byte boundary.
    const auto nameLength = extra.u8();
    if (nameLength > 0)
    {
        std::string name(nameLength, '\0');
        extra.read(name.data(), nameLength);
        layer.name = std::move(name);
    }
    const auto padding = (4 - ((nameLength + 1) & 3)) & 3;
    extra.skip(padding);

    while (extra.remaining() >= 12 && extra.valid())
    {
        const auto signature = extra.tag();
        if (signature != tag_8bim && signature != tag_8b64)
            return;
        const auto key = extra.tag();
        const auto size = extra.u32();
        if (!extra.valid())
            return;
        auto block = extra.take(size);
        if (size & 1u)
            extra.skip(1); // additional info is padded to an even length

        switch (key)
        {
            case make_tag('l', 'u', 'n', 'i'):
                block.utf16(layer.nameUnicode);
                break;
            case make_tag('l', 'y', 'i', 'd'):
                layer.layerId = block.i32();
                break;
            case make_tag('i', 'O', 'p', 'a'):
                layer.fillOpacity = block.u8();
                break;
            case make_tag('s', 'h', 'm', 'd'):
                parse_layer_metadata(block, layer);
                break;
            case make_tag('l', 's', 'c', 't'):
            case make_tag('l', 's', 'd', 'k'):
                parse_section_divider(block, layer);
                break;
            default:
                apply_layer_type_key(key, layer);
                break;
        }
    }
}

void parse_section_divider(Reader& block, Layer& layer)
{
    const auto divider = block.u32();
    switch (divider)
    {
        case 1:
        case 2:
            layer.type = LayerType::Folder;
            break;
        case 3:
            layer.type = LayerType::Hidden;
            break;
        default:
            break;
    }
    if (block.remaining() >= 8)
    {
        block.u32(); // blend signature
        const auto key = block.tag();
        if (key != 0)
            layer.blend = blend_mode_from_key(static_cast<int>(key));
    }
}

// Adjustment and fill layers carry a key that identifies their effect; the
// reference maps the presence of any of them to a layer type. Pixel data is
// irrelevant for such layers.
void apply_layer_type_key(std::uint32_t key, Layer& layer)
{
    switch (key)
    {
        case make_tag('g', 'r', 'd', 'm'):
        case make_tag('l', 'e', 'v', 'l'):
        case make_tag('c', 'u', 'r', 'v'):
        case make_tag('h', 'u', 'e', ' '):
        case make_tag('h', 'u', 'e', '2'):
        case make_tag('b', 'l', 'n', 'c'):
        case make_tag('n', 'v', 'r', 't'):
        case make_tag('p', 'o', 's', 't'):
        case make_tag('t', 'h', 'r', 's'):
        case make_tag('s', 'e', 'l', 'c'):
        case make_tag('b', 'r', 'i', 't'):
        case make_tag('m', 'i', 'x', 'r'):
        case make_tag('c', 'l', 'r', 'L'):
        case make_tag('p', 'h', 'f', 'l'):
        case make_tag('b', 'l', 'w', 'h'):
        case make_tag('v', 'i', 'b', 'A'):
        case make_tag('e', 'x', 'p', 'A'):
            layer.type = LayerType::Adjust;
            break;
        case make_tag('S', 'o', 'C', 'o'):
        case make_tag('G', 'd', 'F', 'l'):
        case make_tag('P', 't', 'F', 'l'):
            layer.type = LayerType::Fill;
            break;
        default:
            break;
    }
}

// The layer block: record table followed by the raw channel payloads. The
// payloads are not decoded here; only their offsets are recorded.
bool parse_layer_info(Reader& layerInfo, Document& document, std::string& error,
                       const std::uint8_t* fileBase)
{
    const auto count = std::abs(static_cast<int>(layerInfo.i16()));
    if (!layerInfo.valid())
    {
        error = "truncated layer count";
        return false;
    }
    if (count > static_cast<int>(layerInfo.remaining()) / 6)
    {
        error = "invalid layer count";
        return false;
    }

    document.layers.reserve(static_cast<std::size_t>(count));
    for (int i = 0; i < count; ++i)
    {
        Layer layer;
        layer.top = layerInfo.i32();
        layer.left = layerInfo.i32();
        layer.bottom = layerInfo.i32();
        layer.right = layerInfo.i32();
        layer.width = layer.right - layer.left;
        layer.height = layer.bottom - layer.top;

        const auto channelCount = layerInfo.u16();
        if (!layerInfo.valid() || channelCount > max_channels)
        {
            error = "invalid layer channel count";
            return false;
        }
        layer.channels.reserve(channelCount);
        for (std::uint16_t c = 0; c < channelCount; ++c)
        {
            Channel channel;
            channel.id = layerInfo.i16();
            channel.length = layerInfo.u32();
            layer.channels.push_back(channel);
        }

        if (layerInfo.tag() != tag_8bim)
        {
            error = "unsupported layer signature";
            return false;
        }
        const auto blendKey = layerInfo.tag();
        layer.blendKey = static_cast<int>(blendKey);
        layer.blend = blend_mode_from_key(static_cast<int>(blendKey));
        layer.opacity = layerInfo.u8();
        layer.clipping = layerInfo.u8();
        layer.flags = layerInfo.u8();
        layerInfo.u8(); // filler

        const auto extraSize = layerInfo.u32();
        auto extra = layerInfo.take(extraSize);
        if (!layerInfo.valid())
        {
            error = "truncated layer record";
            return false;
        }
        parse_layer_extra(extra, layer);

        document.layers.push_back(std::move(layer));
    }

    // Channel payloads follow the records in the same order as the records'
    // channel lists. `length` includes the compression tag.
    for (auto& layer : document.layers)
    {
        for (auto& channel : layer.channels)
        {
            channel.dataOffset = layerInfo.file_offset(fileBase);
            if (!layerInfo.skip(channel.length))
            {
                error = "truncated layer channel data";
                return false;
            }
        }
    }
    return true;
}

// Section dividers order folders: a folder opens a group, a hidden divider
// closes it. Layers are stored bottom first, so the pass runs backwards.
void link_parents(Document& document)
{
    std::vector<int> stack;
    stack.push_back(-1);
    for (int i = static_cast<int>(document.layers.size()) - 1; i >= 0; --i)
    {
        Layer& layer = document.layers[static_cast<std::size_t>(i)];
        layer.parent = stack.back();
        if (layer.type == LayerType::Folder)
            stack.push_back(i);
        else if (layer.type == LayerType::Hidden)
        {
            if (stack.size() > 1)
                stack.pop_back();
        }
    }
}

void parse_layer_and_mask(Reader& reader, Document& document, std::string& error)
{
    const auto size = reader.u32();
    auto block = reader.take(size);
    if (!reader.valid())
    {
        error = "truncated layer block";
        return;
    }

    const auto layerInfoSize = block.u32();
    if (block.valid() && layerInfoSize > 0)
    {
        auto layerInfo = block.take(layerInfoSize);
        if (!parse_layer_info(layerInfo, document, error, document.file.data()))
            return;
        link_parents(document);
    }

    // Global layer mask info is not script-visible; skip it by its length.
    const auto globalMaskSize = block.u32();
    block.skip(globalMaskSize);

    // A 16/32-bit document repeats the whole layer table under '8BIM Lr16' /
    // 'Lr32' with the same record layout. The primary table above is the 8-bit
    // one, so the repeated block replaces it when present: that is the data the
    // depth-aware decoder needs.
    while (block.remaining() >= 12 && block.valid())
    {
        if (block.tag() != tag_8bim)
            return;
        const auto key = block.tag();
        if (key != make_tag('L', 'r', '1', '6') && key != make_tag('L', 'r', '3', '2'))
            return;
        const auto size = block.u32();
        if (!block.valid())
            return;
        auto repeated = block.take(size);
        const auto repeatedSize = repeated.u32();
        if (!repeated.valid() || repeatedSize == 0)
            return;
        auto repeatedInfo = repeated.take(repeatedSize);
        // Parsing into a fresh document would lose the resources parsed so far;
        // reset only the layer table and rebuild it from this block.
        document.layers.clear();
        if (!parse_layer_info(repeatedInfo, document, error, document.file.data()))
            return;
        link_parents(document);
        return;
    }
}

} // namespace

BlendMode blend_mode_from_key(int key)
{
    switch (key)
    {
        case make_tag('n', 'o', 'r', 'm'): return BlendMode::Normal;
        case make_tag('d', 'i', 's', 's'): return BlendMode::Dissolve;
        case make_tag('d', 'a', 'r', 'k'): return BlendMode::Darken;
        case make_tag('m', 'u', 'l', ' '): return BlendMode::Multiply;
        case make_tag('i', 'd', 'i', 'v'): return BlendMode::ColorBurn;
        case make_tag('l', 'b', 'r', 'n'): return BlendMode::LinearBurn;
        case make_tag('l', 'i', 't', 'e'): return BlendMode::Lighten;
        case make_tag('s', 'c', 'r', 'n'): return BlendMode::Screen;
        case make_tag('d', 'i', 'v', ' '): return BlendMode::ColorDodge;
        case make_tag('l', 'd', 'd', 'g'): return BlendMode::LinearDodge;
        case make_tag('o', 'v', 'e', 'r'): return BlendMode::Overlay;
        case make_tag('s', 'L', 'i', 't'): return BlendMode::SoftLight;
        case make_tag('h', 'L', 'i', 't'): return BlendMode::HardLight;
        case make_tag('v', 'L', 'i', 't'): return BlendMode::VividLight;
        case make_tag('l', 'L', 'i', 't'): return BlendMode::LinearLight;
        case make_tag('p', 'L', 'i', 't'): return BlendMode::PinLight;
        case make_tag('h', 'M', 'i', 'x'): return BlendMode::HardMix;
        case make_tag('d', 'i', 'f', 'f'): return BlendMode::Difference;
        case make_tag('s', 'm', 'u', 'd'): return BlendMode::Exclusion;
        case make_tag('h', 'u', 'e', ' '): return BlendMode::Hue;
        case make_tag('s', 'a', 't', ' '): return BlendMode::Saturation;
        case make_tag('c', 'o', 'l', 'r'): return BlendMode::Color;
        case make_tag('l', 'u', 'm', 'i'): return BlendMode::Luminosity;
        case make_tag('p', 'a', 's', 's'): return BlendMode::PassThrough;
        case make_tag('d', 'k', 'C', 'l'): return BlendMode::DarkerColor;
        case make_tag('l', 'g', 'C', 'l'): return BlendMode::LighterColor;
        case make_tag('f', 's', 'u', 'b'): return BlendMode::Subtract;
        case make_tag('f', 'd', 'i', 'v'): return BlendMode::Divide;
        default: return BlendMode::Invalid;
    }
}

const Layer* find_layer(const Document& document, int layerId)
{
    for (const auto& layer : document.layers)
    {
        if (layer.layerId == layerId)
            return &layer;
    }
    return nullptr;
}

bool Document::parse(std::vector<std::uint8_t>&& bytes, std::string& error)
{
    file = std::move(bytes);
    Reader reader(std::span<const std::uint8_t>(file.data(), file.size()));

    if (!parse_header(reader, *this, error))
        return false;
    parse_color_mode(reader, *this);
    if (!reader.valid())
    {
        error = "truncated colour mode data";
        return false;
    }
    // Each section is parsed through its own bounded reader. The resource list
    // is self-delimiting only by its outer length, and the loop probes one
    // signature past the last record to stop; without a bound that probe eats
    // the first four bytes of the following section.
    const auto resourceSize = reader.u32();
    auto resources = reader.take(resourceSize);
    if (!reader.valid())
    {
        error = "truncated image resources";
        return false;
    }
    parse_image_resources(resources, *this);
    parse_layer_and_mask(reader, *this, error);
    if (!error.empty())
        return false;

    // The merged image follows the layer block; its first two bytes are the
    // compression tag. Some files end right after the layer block, which is
    // legal, so a missing merged image only disables getBlend().
    imageDataOffset = reader.position();

    loaded = true;
    return true;
}

} // namespace kzpsd
