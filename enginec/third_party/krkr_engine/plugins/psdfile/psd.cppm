// PSD reader as a C++23 module.
//
// This is the one subsystem under third_party/krkr_engine that is ours rather than
// vendored upstream code (the KiriKiri core and TJS2 are upstream and cannot become
// modules: their headers carry ~5000 macros such as TJS_N and the NCB registration
// family, and macros never cross a module boundary). The PSD reader, by contrast,
// is plain std C++ plus zlib, so it is expressed as an interface module with three
// implementation units and no header at all:
//
//   psd.cppm          this file (declarations)
//   PSDDocument.cpp   module enginec.krkr.psd  (parse)
//   PSDPixels.cpp     module enginec.krkr.psd  (decode / compose)
//   PSDDescriptor.cpp module enginec.krkr.psd  (Photoshop descriptor tree)
//
// The TJS-facing plugin (PSDPlugin.cpp) is still a normal TU: it needs the engine's
// macro-based headers, so it imports this module and includes only what TJS
// requires. That keeps the macro-heavy surface out of this module.
//
// Data model: the file is parsed once into a Document; every later query (layer
// info, pixels, psd:// paths) works on that model, never on the raw bytes.
export module enginec.krkr.psd;

import std;

export namespace kzpsd
{

// ---------------------------------------------------------------------------
// Bounds-checked big-endian cursor over the PSD file image.
//
// Every read is checked once, here, instead of at each call site: a truncated or
// hostile file then leaves `failed()` set and the parser stops at the first
// inconsistency, rather than reading past the buffer.
// ---------------------------------------------------------------------------
class Reader
{
public:
    Reader() = default;
    Reader(std::span<const std::uint8_t> bytes)
        : begin_(bytes.data()), cursor_(bytes.data()), end_(bytes.data() + bytes.size())
    {
    }

    bool valid() const { return !failed_; }
    bool eof() const { return cursor_ >= end_; }
    std::size_t remaining() const { return static_cast<std::size_t>(end_ - cursor_); }
    std::size_t position() const { return static_cast<std::size_t>(cursor_ - begin_); }

    // Offset of the cursor inside the buffer this reader ultimately spans. A
    // sub-reader created by take() keeps pointing into the same buffer, so this
    // stays file-relative and can be stored as a channel's payload offset.
    std::size_t file_offset(const std::uint8_t* base) const
    {
        return static_cast<std::size_t>(cursor_ - base);
    }

    // Returns a reader over the next `size` bytes and consumes them from this
    // one. A short block is clamped to what is left and marks this reader failed.
    Reader take(std::size_t size)
    {
        if (!ensure(size))
            return Reader{};
        Reader sub(std::span<const std::uint8_t>(cursor_, size));
        cursor_ += size;
        return sub;
    }

    bool skip(std::size_t size)
    {
        if (!ensure(size))
            return false;
        cursor_ += size;
        return true;
    }

    bool read(void* destination, std::size_t size)
    {
        if (!ensure(size))
            return false;
        std::memcpy(destination, cursor_, size);
        cursor_ += size;
        return true;
    }

    std::uint8_t u8()
    {
        if (!ensure(1))
            return 0;
        return *cursor_++;
    }

    std::uint16_t u16()
    {
        if (!ensure(2))
            return 0;
        const auto value = static_cast<std::uint16_t>((cursor_[0] << 8) | cursor_[1]);
        cursor_ += 2;
        return value;
    }

    std::int16_t i16() { return static_cast<std::int16_t>(u16()); }

    std::uint32_t u32()
    {
        if (!ensure(4))
            return 0;
        const auto value = (static_cast<std::uint32_t>(cursor_[0]) << 24) |
                           (static_cast<std::uint32_t>(cursor_[1]) << 16) |
                           (static_cast<std::uint32_t>(cursor_[2]) << 8) |
                           static_cast<std::uint32_t>(cursor_[3]);
        cursor_ += 4;
        return value;
    }

    std::int32_t i32() { return static_cast<std::int32_t>(u32()); }

    std::uint64_t u64()
    {
        if (!ensure(8))
            return 0;
        std::uint64_t value = 0;
        for (int i = 0; i < 8; ++i)
            value = (value << 8) | cursor_[i];
        cursor_ += 8;
        return value;
    }

    std::int64_t i64() { return static_cast<std::int64_t>(u64()); }

    float f32()
    {
        const auto bits = u32();
        return std::bit_cast<float>(bits);
    }

    double f64()
    {
        const auto bits = u64();
        return std::bit_cast<double>(bits);
    }

    // Four-byte signature or four-character key. Not NUL-terminated; callers
    // compare it as an integer where possible.
    std::uint32_t tag()
    {
        if (!ensure(4))
            return 0;
        const std::uint32_t value = (static_cast<std::uint32_t>(cursor_[0]) << 24) |
                                    (static_cast<std::uint32_t>(cursor_[1]) << 16) |
                                    (static_cast<std::uint32_t>(cursor_[2]) << 8) |
                                    static_cast<std::uint32_t>(cursor_[3]);
        cursor_ += 4;
        return value;
    }

    // UTF-16BE string: a 32-bit code-unit count followed by the units.
    bool utf16(std::u16string& out)
    {
        const auto length = u32();
        // One code unit is two bytes; reject lengths no real file can satisfy
        // before reserving, so a corrupt count cannot request gigabytes.
        if (length > remaining() / 2)
        {
            failed_ = true;
            return false;
        }
        out.clear();
        out.reserve(length);
        for (std::uint32_t i = 0; i < length; ++i)
            out.push_back(static_cast<char16_t>(u16()));
        return !failed_;
    }

private:
    bool ensure(std::size_t size)
    {
        if (failed_)
            return false;
        if (remaining() < size)
        {
            failed_ = true;
            return false;
        }
        return true;
    }

    const std::uint8_t* begin_ = nullptr;
    const std::uint8_t* cursor_ = nullptr;
    const std::uint8_t* end_ = nullptr;
    bool failed_ = false;
};

// PSD four-character codes, e.g. make_tag("luni").
constexpr std::uint32_t make_tag(char a, char b, char c, char d)
{
    return (static_cast<std::uint32_t>(static_cast<unsigned char>(a)) << 24) |
           (static_cast<std::uint32_t>(static_cast<unsigned char>(b)) << 16) |
           (static_cast<std::uint32_t>(static_cast<unsigned char>(c)) << 8) |
           static_cast<std::uint32_t>(static_cast<unsigned char>(d));
}

inline constexpr std::uint32_t tag_8bim = make_tag('8', 'B', 'I', 'M');
inline constexpr std::uint32_t tag_8b64 = make_tag('8', 'B', '6', '4');
inline constexpr std::uint32_t tag_8bps = make_tag('8', 'B', 'P', 'S');

// ---------------------------------------------------------------------------
// File format model
// ---------------------------------------------------------------------------

// Values are script-visible through the constants registered on the PSD class,
// so the numbering must stay identical to the reference plugin.
enum class ColorMode : int
{
    Bitmap = 0,
    Grayscale = 1,
    Indexed = 2,
    Rgb = 3,
    Cmyk = 4,
    Multichannel = 7,
    Duotone = 8,
    Lab = 9,
};

enum class LayerType : int
{
    Normal = 0,
    Hidden = 1, // section divider that closes a folder
    Folder = 2,
    Adjust = 3,
    Fill = 4,
    Text = 5,
};

enum class BlendMode : int
{
    Invalid = -1,
    Normal = 0,
    Dissolve,
    Darken,
    Multiply,
    ColorBurn,
    LinearBurn,
    Lighten,
    Screen,
    ColorDodge,
    LinearDodge,
    Overlay,
    SoftLight,
    HardLight,
    VividLight,
    LinearLight,
    PinLight,
    HardMix,
    Difference,
    Exclusion,
    Hue,
    Saturation,
    Color,
    Luminosity,
    PassThrough,
    DarkerColor,
    LighterColor,
    Subtract,
    Divide,
};

// Which part of a layer the caller wants: the RGB image alone, the image with
// the layer mask folded into alpha, or the mask as a grayscale image.
enum class ImageMode
{
    Image,
    MaskedImage,
    Mask,
};

// A layer mask rectangle. `defaultColor` decides what the mask means outside
// its own rectangle: 0 hides, 255 shows.
struct Mask
{
    bool present = false;
    int top = 0;
    int left = 0;
    int bottom = 0;
    int right = 0;
    int width = 0;
    int height = 0;
    int defaultColor = 0;
    int flags = 0;
};

// One channel of one layer. `length` counts the two-byte compression tag and
// the payload, so the next channel starts at `dataOffset + length`.
struct Channel
{
    int id = 0; // 0..n colour planes, -1 transparency, -2/-3 masks
    std::uint32_t length = 0;
    std::size_t dataOffset = 0;
};

// Per-layer entry of a layer comp: where the layer sits in that comp and
// whether the comp shows it.
struct LayerCompInfo
{
    int id = 0;
    int offsetX = 0;
    int offsetY = 0;
    bool enabled = false;
};

// Document-level layer comp (the named states Photoshop stores in the file).
struct LayerComp
{
    int id = 0;
    bool recordVisibility = false;
    bool recordPosition = false;
    bool recordAppearance = false;
    std::string name;
    std::string comment;
};

struct Layer
{
    int top = 0;
    int left = 0;
    int bottom = 0;
    int right = 0;
    int width = 0;
    int height = 0;

    std::vector<Channel> channels;

    int blendKey = 0; // four-character code, exposed through getLayerInfo
    BlendMode blend = BlendMode::Normal;
    int opacity = 255;
    int fillOpacity = 255;
    int clipping = 0;
    int flags = 0;

    int layerId = -1;
    LayerType type = LayerType::Normal;
    Mask mask;

    std::string name;           // raw Pascal-string name (usually ASCII)
    std::u16string nameUnicode; // 'luni' record, preferred when present
    int parent = -1;            // index into Document::layers for a folder child

    // Filled from 'shmd'/'cmls': the layer's state in every comp it takes part in.
    std::map<int, LayerCompInfo> layerComps;
};

struct Guide
{
    int location = 0;
    bool vertical = false;
};

struct SliceItem
{
    int id = 0;
    int groupId = 0;
    int origin = 0;
    int associatedLayerId = -1;
    std::u16string name;
    int type = 0;
    int left = 0;
    int top = 0;
    int right = 0;
    int bottom = 0;
    std::u16string url;
    std::u16string target;
    std::u16string message;
    std::u16string altTag;
    bool cellTextIsHtml = false;
    std::u16string cellText;
    int horizontalAlign = 0;
    int verticalAlign = 0;
    std::uint32_t color = 0;
};

struct SliceResource
{
    bool enabled = false;
    int boundingTop = 0;
    int boundingLeft = 0;
    int boundingBottom = 0;
    int boundingRight = 0;
    std::u16string groupName;
    std::vector<SliceItem> items;
};

struct GuideResource
{
    bool enabled = false;
    int horizontalGrid = 1;
    int verticalGrid = 1;
    std::vector<Guide> guides;
};

// The whole parsed document. `file` keeps the bytes alive because channel data
// is decoded lazily: decoding every layer of a large PSD up front would multiply
// the memory a game needs for an asset it may never touch.
struct Document
{
    int width = 0;
    int height = 0;
    int channels = 0;
    int depth = 8;
    ColorMode mode = ColorMode::Rgb;

    std::vector<std::uint8_t> file;
    std::size_t imageDataOffset = 0; // merged image, right after the layer block

    std::vector<Layer> layers;
    std::vector<std::uint8_t> colorTable; // 256 * RGB for indexed images
    int transparencyIndex = -1;
    GuideResource guides;
    SliceResource slices;
    std::vector<LayerComp> layerComps;
    int lastAppliedCompId = -1;

    bool loaded = false;

    // Parses `bytes` (which is moved into `file`) and fills every field above.
    // On failure `error` carries a short reason; the document stays unloaded.
    bool parse(std::vector<std::uint8_t>&& bytes, std::string& error);
};

// Layer helpers shared by the plugin and the storage media.
const Layer* find_layer(const Document& document, int layerId);

// Maps a PSD four-character blend key to the engine's blend enum.
BlendMode blend_mode_from_key(int key);

// ---------------------------------------------------------------------------
// Photoshop descriptor tree
//
// Layer comps, slice metadata and several layer settings are stored as
// "descriptors": a small typed tree with four-character keys. Only the comps
// path needs it today, but the reader is generic because the same structure
// appears in resources and in per-layer blocks with different roots.
//
// The reference plugin builds a heap tree of polymorphic nodes for this. A flat
// value tree is enough here: nothing walks descriptors repeatedly, and copying
// the few nodes a layer comp contains is cheaper than owning pointers.
// ---------------------------------------------------------------------------
struct DescriptorValue
{
    enum class Kind
    {
        Null,
        Bool,
        Integer,
        Real,
        Text,
        Enum,
        Class,
        List,
        Object,
        Reference,
        Alias,
    };

    Kind kind = Kind::Null;
    bool boolean = false;
    std::int64_t integer = 0;
    double real = 0.0;
    std::string text;    // UTF-8
    std::string classId; // four-character class or unit key
    std::string typeId;  // enum type key
    std::string enumId;  // enum value key
    std::vector<DescriptorValue> items;            // Kind::List
    std::map<std::string, DescriptorValue> fields; // Kind::Object

    const DescriptorValue* find(std::string_view key) const
    {
        const auto it = fields.find(std::string(key));
        return it != fields.end() ? &it->second : nullptr;
    }
};

// Reads one descriptor body (Unicode name, class id, items). Returns false when
// the structure contains a type this reader cannot skip, in which case the
// caller must abandon the containing block instead of guessing offsets.
bool read_descriptor(Reader& reader, DescriptorValue& out);

// Reads the resource/layer form: a 32-bit version (must be 16) then a body.
bool read_descriptor_block(Reader& reader, DescriptorValue& out);

// UTF-16BE -> UTF-8, shared with the plugin.
std::string utf16_to_utf8(const std::u16string& text);

// ---------------------------------------------------------------------------
// Channel decoding and pixel composition
//
// Layers are stored as separate planes, each compressed independently, and the
// plugin has to produce the BGRA buffer a Layer expects. Three compression
// schemes exist and all three are supported here:
//   0 raw, 1 PackBits/RLE, 2 zip, 3 zip with horizontal delta prediction.
// ---------------------------------------------------------------------------

// RGBA byte order used by the engine's layer buffers.
struct ColorFormat
{
    unsigned redShift;
    unsigned greenShift;
    unsigned blueShift;
    unsigned alphaShift;
};

// Windows DIB byte order, which is what Layer.mainImageBufferForWrite holds.
inline constexpr ColorFormat bgra = {16, 8, 0, 24};

// Fills `destination` (a layer buffer with `pitch` bytes per row) with the
// requested part of `layer`. Returns false when the layer holds no usable
// pixels, which the caller turns into a script-level error.
bool render_layer(const Document& document, const Layer& layer, std::uint8_t* destination,
                  int pitch, ImageMode mode, ColorFormat format);

// Fills `destination` with the document's merged image (what Photoshop shows
// when the file is opened). Fails when the file has no merged image section.
bool render_merged(const Document& document, std::uint8_t* destination, int pitch,
                   ColorFormat format);

} // namespace kzpsd
