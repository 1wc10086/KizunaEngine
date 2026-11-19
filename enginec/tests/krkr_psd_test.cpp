// Host test for the PSD reader (plugins/psdfile).
//
// The plugin's script surface is covered by the krkr smoke test, but pixel and
// mask semantics are easiest to prove here: the test builds PSD files in memory
// (one per compression/edge case) and checks the decoded BGRA bytes directly.
//
// The reader is a module, so this test imports it instead of including headers;
// that is also what proves the module's exported API covers everything the
// plugin needs.
import std;

import enginec.krkr.psd;

namespace
{

using Bytes = std::vector<std::uint8_t>;

int failures = 0;

void check(bool condition, const char* what)
{
    if (!condition)
    {
        std::cerr << "psd test: FAILED: " << what << "\n";
        ++failures;
    }
}

void check_equal(std::uint32_t actual, std::uint32_t expected, const char* what)
{
    if (actual != expected)
    {
        std::cerr << "psd test: FAILED: " << what << ": got 0x" << std::hex << actual
                  << " expected 0x" << expected << std::dec << "\n";
        ++failures;
    }
}

void u8(Bytes& out, std::uint32_t value) { out.push_back(static_cast<std::uint8_t>(value)); }

void u16(Bytes& out, std::uint32_t value)
{
    out.push_back(static_cast<std::uint8_t>(value >> 8));
    out.push_back(static_cast<std::uint8_t>(value));
}

void u32(Bytes& out, std::uint32_t value)
{
    out.push_back(static_cast<std::uint8_t>(value >> 24));
    out.push_back(static_cast<std::uint8_t>(value >> 16));
    out.push_back(static_cast<std::uint8_t>(value >> 8));
    out.push_back(static_cast<std::uint8_t>(value));
}

void i32(Bytes& out, std::int32_t value) { u32(out, static_cast<std::uint32_t>(value)); }

void put(Bytes& out, const Bytes& data) { out.insert(out.end(), data.begin(), data.end()); }

void tag(Bytes& out, const char* text)
{
    for (int i = 0; i < 4; ++i)
        out.push_back(static_cast<std::uint8_t>(text[i]));
}

Bytes unicode_name(const std::string& text)
{
    Bytes out;
    u32(out, static_cast<std::uint32_t>(text.size()));
    for (unsigned char ch : text)
        u16(out, ch);
    return out;
}

// Descriptor id: four-character keys use a zero length, longer keys carry it.
Bytes descriptor_id(const std::string& key)
{
    Bytes out;
    if (key.size() == 4)
    {
        u32(out, 0);
        out.insert(out.end(), key.begin(), key.end());
    }
    else
    {
        u32(out, static_cast<std::uint32_t>(key.size()));
        out.insert(out.end(), key.begin(), key.end());
    }
    return out;
}

Bytes value_long(std::int32_t value)
{
    Bytes out;
    tag(out, "long");
    i32(out, value);
    return out;
}

Bytes value_bool(bool value)
{
    Bytes out;
    tag(out, "bool");
    u8(out, value ? 1 : 0);
    return out;
}

Bytes value_text(const std::string& text)
{
    Bytes out;
    tag(out, "TEXT");
    const auto name = unicode_name(text);
    put(out, name);
    return out;
}

Bytes value_list(const std::vector<Bytes>& items)
{
    Bytes out;
    tag(out, "VlLs");
    u32(out, static_cast<std::uint32_t>(items.size()));
    for (const auto& item : items)
    {
        tag(out, "Objc");
        put(out, item);
    }
    return out;
}

Bytes descriptor(const std::vector<std::pair<std::string, Bytes>>& items)
{
    Bytes out;
    const auto name = unicode_name("");
    put(out, name);
    u32(out, 0); // class id: "null"
    tag(out, "null");
    u32(out, static_cast<std::uint32_t>(items.size()));
    for (const auto& [key, value] : items)
    {
        const auto id = descriptor_id(key);
        put(out, id);
        put(out, value);
    }
    return out;
}

Bytes descriptor_block(const std::vector<std::pair<std::string, Bytes>>& items)
{
    Bytes out;
    u32(out, 16);
    const auto body = descriptor(items);
    put(out, body);
    return out;
}

Bytes resource(std::uint16_t id, const Bytes& data)
{
    Bytes out;
    tag(out, "8BIM");
    u16(out, id);
    u8(out, 0); // empty Pascal name + its pad byte
    u8(out, 0);
    u32(out, static_cast<std::uint32_t>(data.size()));
    put(out, data);
    if (data.size() & 1u)
        u8(out, 0);
    return out;
}

Bytes additional_block(const char* key, const Bytes& data)
{
    Bytes out;
    tag(out, "8BIM");
    tag(out, key);
    u32(out, static_cast<std::uint32_t>(data.size()));
    put(out, data);
    if (data.size() & 1u)
        u8(out, 0);
    return out;
}

Bytes pascal_name(const std::string& name)
{
    Bytes out;
    u8(out, static_cast<std::uint32_t>(name.size()));
    out.insert(out.end(), name.begin(), name.end());
    while (out.size() % 4 != 0)
        u8(out, 0);
    return out;
}

// One layer of the synthetic document.
struct TestLayer
{
    std::string name;
    int top = 0;
    int left = 0;
    int bottom = 0;
    int right = 0;
    std::vector<std::pair<int, Bytes>> channels;
    Bytes additional;    // additional info blocks
    Bytes mask;          // mask info payload (without the length prefix)
    int flags = 0;
    char blend[5] = "norm";
};

struct TestDocument
{
    int width = 4;
    int height = 4;
    int channels = 3;
    int depth = 8;
    int mode = 3;
    std::vector<TestLayer> layers;
    Bytes resources;
    Bytes merged;
    std::vector<std::pair<char[5], Bytes>> repeatedLayers; // Lr16/Lr32
};

Bytes build_layer_record(const TestLayer& layer)
{
    Bytes out;
    i32(out, layer.top);
    i32(out, layer.left);
    i32(out, layer.bottom);
    i32(out, layer.right);
    u16(out, static_cast<std::uint32_t>(layer.channels.size()));
    for (const auto& [id, payload] : layer.channels)
    {
        u16(out, static_cast<std::uint16_t>(id));
        u32(out, static_cast<std::uint32_t>(payload.size()));
    }
    tag(out, "8BIM");
    for (int i = 0; i < 4; ++i)
        u8(out, static_cast<std::uint8_t>(layer.blend[i]));
    u8(out, 255); // opacity
    u8(out, 0);   // clipping
    u8(out, layer.flags);
    u8(out, 0);   // filler

    Bytes extra;
    if (layer.mask.empty())
    {
        u32(extra, 0);
    }
    else
    {
        u32(extra, static_cast<std::uint32_t>(layer.mask.size()));
        put(extra, layer.mask);
    }
    u32(extra, 0); // blending ranges
    const auto name = pascal_name(layer.name);
    put(extra, name);
    put(extra, layer.additional);

    u32(out, static_cast<std::uint32_t>(extra.size()));
    put(out, extra);
    return out;
}

Bytes build_layer_info(const TestDocument& document)
{
    Bytes records;
    Bytes channelData;
    for (const auto& layer : document.layers)
    {
        const auto record = build_layer_record(layer);
        put(records, record);
        for (const auto& [id, payload] : layer.channels)
            put(channelData, payload);
    }
    Bytes out;
    u16(out, static_cast<std::uint16_t>(document.layers.size()));
    put(out, records);
    put(out, channelData);
    return out;
}

Bytes build_psd(const TestDocument& document)
{
    Bytes out;
    tag(out, "8BPS");
    u16(out, 1);
    for (int i = 0; i < 6; ++i)
        u8(out, 0);
    u16(out, static_cast<std::uint32_t>(document.channels));
    u32(out, static_cast<std::uint32_t>(document.height));
    u32(out, static_cast<std::uint32_t>(document.width));
    u16(out, static_cast<std::uint32_t>(document.depth));
    u16(out, static_cast<std::uint32_t>(document.mode));
    u32(out, 0); // colour mode data
    u32(out, static_cast<std::uint32_t>(document.resources.size()));
    put(out, document.resources);

    Bytes block;
    const auto layerInfo = build_layer_info(document);
    u32(block, static_cast<std::uint32_t>(layerInfo.size()));
    put(block, layerInfo);
    u32(block, 0); // global layer mask info
    for (const auto& [key, info] : document.repeatedLayers)
    {
        tag(block, "8BIM");
        tag(block, key);
        u32(block, static_cast<std::uint32_t>(info.size() + 4));
        u32(block, static_cast<std::uint32_t>(info.size()));
        put(block, info);
    }
    u32(out, static_cast<std::uint32_t>(block.size()));
    put(out, block);

    u16(out, 0); // merged image: raw
    put(out, document.merged);
    return out;
}

// Builds a raw channel payload: compression tag 0 then `bytes`.
Bytes raw_channel(const Bytes& bytes)
{
    Bytes out;
    u16(out, 0);
    put(out, bytes);
    return out;
}

// PackBits is only worthwhile with repeated bytes; the test document uses one
// run and one literal per row so both paths of the decoder are exercised.
Bytes packbits_channel(const Bytes& pixels, int width, int height)
{
    Bytes rows;
    Bytes packed;
    for (int y = 0; y < height; ++y)
    {
        const Bytes row(pixels.begin() + y * width, pixels.begin() + (y + 1) * width);
        Bytes encoded;
        std::size_t x = 0;
        while (x < row.size())
        {
            // No runs in the test data: emit a literal chunk.
            const auto count = std::min<std::size_t>(128, row.size() - x);
            u8(encoded, static_cast<std::uint32_t>(count - 1));
            encoded.insert(encoded.end(), row.begin() + x, row.begin() + x + count);
            x += count;
        }
        u16(rows, static_cast<std::uint32_t>(encoded.size()));
        put(packed, encoded);
    }
    Bytes out;
    u16(out, 1);
    put(out, rows);
    put(out, packed);
    return out;
}

Bytes zip_channel(const Bytes& pixels, int width, int height, bool prediction)
{
    Bytes data = pixels;
    if (prediction)
    {
        for (int y = 0; y < height; ++y)
        {
            auto* row = data.data() + y * width;
            for (int x = width - 1; x > 0; --x)
                row[x] = static_cast<std::uint8_t>(row[x] - row[x - 1]);
        }
    }
    // Minimal deflate via zlib is not available in the test; store instead.
    Bytes out;
    u16(out, prediction ? 3 : 2);
    // zlib stream: header 0x78 0x01, uncompressed deflate blocks, adler32.
    Bytes stream;
    u8(stream, 0x78);
    u8(stream, 0x01);
    std::size_t offset = 0;
    while (offset < data.size())
    {
        const auto chunk = std::min<std::size_t>(65535, data.size() - offset);
        u8(stream, offset + chunk == data.size() ? 1 : 0); // BFINAL, stored
        // Stored-block LEN/NLEN are little-endian, unlike the PSD fields.
        u8(stream, chunk & 0xff);
        u8(stream, (chunk >> 8) & 0xff);
        u8(stream, (~chunk) & 0xff);
        u8(stream, ((~chunk) >> 8) & 0xff);
        stream.insert(stream.end(), data.begin() + offset, data.begin() + offset + chunk);
        offset += chunk;
    }
    std::uint32_t a = 1;
    std::uint32_t b = 0;
    for (unsigned char ch : data)
    {
        a = (a + ch) % 65521;
        b = (b + a) % 65521;
    }
    u32(stream, (b << 16) | a);
    put(out, stream);
    return out;
}

std::uint32_t bgra(std::uint8_t r, std::uint8_t g, std::uint8_t b, std::uint8_t a)
{
    return (static_cast<std::uint32_t>(r) << 16) | (static_cast<std::uint32_t>(g) << 8) | b |
           (static_cast<std::uint32_t>(a) << 24);
}

// Merged PSD image data is planar: the whole red plane, then green, then blue.
Bytes make_planar_rgb(int width, int height)
{
    Bytes out;
    for (int plane = 0; plane < 3; ++plane)
    {
        for (int y = 0; y < height; ++y)
        {
            for (int x = 0; x < width; ++x)
            {
                const int column = x % 4;
                out.push_back(static_cast<std::uint8_t>(plane == column ? 255 : (column == 3 ? 255 : 0)));
            }
        }
    }
    return out;
}

Bytes make_plane(std::uint8_t value, int width, int height)
{
    return Bytes(static_cast<std::size_t>(width) * height, value);
}

kzpsd::Document parse_or_fail(const Bytes& bytes)
{
    kzpsd::Document document;
    std::string error;
    const bool ok = document.parse(Bytes(bytes), error);
    if (!ok)
        std::cerr << "psd test: parse failed: " << error << "\n";
    check(ok, "parse");
    return document;
}

void test_compression()
{
    for (int variant = 0; variant < 4; ++variant)
    {
        TestDocument spec;
        TestLayer layer;
        layer.name = "Layer1";
        layer.right = 4;
        layer.bottom = 4;
        const auto red = make_plane(255, 4, 4);
        const auto green = make_plane(0, 4, 4);
        const auto blue = make_plane(0, 4, 4);
        const auto alpha = make_plane(255, 4, 4);

        auto encode = [variant](const Bytes& pixels)
        {
            switch (variant)
            {
                case 0: return raw_channel(pixels);
                case 1: return packbits_channel(pixels, 4, 4);
                case 2: return zip_channel(pixels, 4, 4, false);
                default: return zip_channel(pixels, 4, 4, true);
            }
        };

        layer.channels = {{0, encode(red)}, {1, encode(green)}, {2, encode(blue)}, {-1, encode(alpha)}};
        spec.layers.push_back(layer);
        spec.merged = make_planar_rgb(4, 4);

        auto document = parse_or_fail(build_psd(spec));
        check(document.layers.size() == 1, "one layer");
        std::vector<std::uint8_t> buffer(4 * 4 * 4);
        check(kzpsd::render_layer(document, document.layers[0], buffer.data(), 16,
                                  kzpsd::ImageMode::MaskedImage, kzpsd::bgra),
              "render layer");
        const auto pixel = reinterpret_cast<std::uint32_t*>(buffer.data())[0];
        check_equal(pixel, bgra(255, 0, 0, 255), "red pixel");
    }
}

void test_mask_and_alpha()
{
    TestDocument spec;
    TestLayer layer;
    layer.name = "Masked";
    layer.right = 4;
    layer.bottom = 4;
    layer.channels = {{0, raw_channel(make_plane(10, 4, 4))},
                      {1, raw_channel(make_plane(20, 4, 4))},
                      {2, raw_channel(make_plane(30, 4, 4))},
                      {-1, raw_channel(make_plane(200, 4, 4))},
                      {-2, raw_channel(Bytes{255, 255, 0, 0})}};
    // Mask rectangle (1,1)-(3,3) with default colour 0: outside it alpha is
    // cleared, inside it is multiplied by the mask.
    Bytes mask;
    i32(mask, 1);
    i32(mask, 1);
    i32(mask, 3);
    i32(mask, 3);
    u8(mask, 0); // default colour
    u8(mask, 0); // flags
    u8(mask, 0); // padding
    layer.mask = mask;
    spec.layers.push_back(layer);
    spec.merged = make_planar_rgb(4, 4);

    auto document = parse_or_fail(build_psd(spec));
    check(document.layers[0].mask.present, "mask present");
    check(document.layers[0].mask.width == 2 && document.layers[0].mask.height == 2, "mask size");

    std::vector<std::uint8_t> buffer(4 * 4 * 4);
    check(kzpsd::render_layer(document, document.layers[0], buffer.data(), 16,
                              kzpsd::ImageMode::MaskedImage, kzpsd::bgra),
          "render masked");

    auto alpha_at = [&buffer](int x, int y)
    { return static_cast<std::uint8_t>(buffer[y * 16 + x * 4 + 3]); };
    check(alpha_at(0, 0) == 0, "alpha outside mask is cleared");
    check(alpha_at(1, 1) == 200, "alpha multiplied by mask value 255");
    check(alpha_at(2, 2) == 0, "alpha multiplied by mask value 0");

    // Raw image mode ignores both alpha and the mask.
    check(kzpsd::render_layer(document, document.layers[0], buffer.data(), 16,
                              kzpsd::ImageMode::Image, kzpsd::bgra),
          "render raw");
    check(alpha_at(0, 0) == 255, "raw alpha stays opaque");

    // Mask mode yields a grayscale image of the mask rectangle.
    std::vector<std::uint8_t> maskBuffer(2 * 2 * 4);
    check(kzpsd::render_layer(document, document.layers[0], maskBuffer.data(), 8,
                              kzpsd::ImageMode::Mask, kzpsd::bgra),
          "render mask");
    check_equal(reinterpret_cast<std::uint32_t*>(maskBuffer.data())[0], bgra(255, 255, 255, 255),
                "mask pixel 0");
    check_equal(reinterpret_cast<std::uint32_t*>(maskBuffer.data())[3], bgra(0, 0, 0, 255),
                "mask pixel 3");
}

void test_layer_info_and_comps()
{
    TestDocument spec;

    // Document-level comps: two named states, the second one last applied.
    auto comp1 = descriptor({{"compID", value_long(111)},
                             {"capturedInfo", value_long(7)},
                             {"Nm  ", value_text("Bright")},
                             {"comment", value_text("first comp")}});
    auto comp2 = descriptor({{"compID", value_long(222)},
                             {"capturedInfo", value_long(5)},
                             {"Nm  ", value_text("Dark")}});
    spec.resources = resource(
        1065, descriptor_block({{"lastAppliedComp", value_long(222)},
                                {"list", value_list({comp1, comp2})}}));

    TestLayer layer;
    layer.name = "Layer1";
    layer.right = 4;
    layer.bottom = 4;
    layer.channels = {{0, raw_channel(make_plane(1, 4, 4))},
                      {1, raw_channel(make_plane(2, 4, 4))},
                      {2, raw_channel(make_plane(3, 4, 4))}};

    // Per-layer comp membership under 'shmd'/'cmls'.
    Bytes offsetObject;
    tag(offsetObject, "Objc");
    const auto offsetBody = descriptor({{"Hrzn", value_long(3)}, {"Vrtc", value_long(-2)}});
    put(offsetObject, offsetBody);

    Bytes compListValue;
    tag(compListValue, "VlLs");
    u32(compListValue, 1);
    const auto compIdValue = value_long(111);
    put(compListValue, compIdValue);

    const auto settings = descriptor({{"compList", compListValue},
                                      {"Ofst", offsetObject},
                                      {"enab", value_bool(true)}});

    Bytes cmls;
    u32(cmls, 16);
    const auto compsDescriptor = descriptor({{"layerSettings", value_list({settings})}});
    put(cmls, compsDescriptor);

    Bytes shmd;
    u32(shmd, 1);
    tag(shmd, "8BIM");
    tag(shmd, "cmls");
    u8(shmd, 0);
    u8(shmd, 0);
    u8(shmd, 0);
    u8(shmd, 0);
    u32(shmd, static_cast<std::uint32_t>(cmls.size()));
    put(shmd, cmls);

    Bytes lyidPayload;
    u32(lyidPayload, 7);
    put(layer.additional, additional_block("luni", unicode_name("Layer One")));
    put(layer.additional, additional_block("lyid", lyidPayload));
    put(layer.additional, additional_block("shmd", shmd));

    spec.layers.push_back(layer);
    spec.merged = make_planar_rgb(4, 4);

    auto document = parse_or_fail(build_psd(spec));
    check(document.layers.size() == 1, "comps: layer count");
    check(document.layers[0].layerId == 7, "comps: layer id");
    check(document.layers[0].nameUnicode.size() == 9, "comps: unicode name");
    check(document.lastAppliedCompId == 222, "comps: last applied");
    check(document.layerComps.size() == 2, "comps: count");
    if (document.layerComps.size() == 2)
    {
        check(document.layerComps[0].id == 111, "comps: first id");
        check(document.layerComps[0].name == "Bright", "comps: first name");
        check(document.layerComps[0].recordVisibility, "comps: visibility flag");
        check(document.layerComps[0].recordPosition, "comps: position flag");
        check(document.layerComps[0].recordAppearance, "comps: appearance flag");
        check(document.layerComps[1].id == 222, "comps: second id");
        check(document.layerComps[1].name == "Dark", "comps: second name");
    }
    check(document.layers[0].layerComps.size() == 1, "comps: layer membership");
    if (!document.layers[0].layerComps.empty())
    {
        const auto& info = document.layers[0].layerComps.begin()->second;
        check(info.id == 111, "comps: membership id");
        check(info.offsetX == 3 && info.offsetY == -2, "comps: offset");
        check(info.enabled, "comps: enabled");
    }
}

void test_slices_and_guides()
{
    // Guides resource 1032: version, grid, count, (location, direction).
    Bytes guides;
    u32(guides, 1);
    u32(guides, 8);
    u32(guides, 8);
    u32(guides, 2);
    i32(guides, 10);
    u8(guides, 0); // vertical
    i32(guides, 20);
    u8(guides, 1); // horizontal

    TestDocument spec;
    spec.resources = resource(1032, guides);
    TestLayer layer;
    layer.name = "Layer1";
    layer.right = 4;
    layer.bottom = 4;
    layer.channels = {{0, raw_channel(make_plane(1, 4, 4))}};
    spec.layers.push_back(layer);
    spec.merged = make_planar_rgb(4, 4);

    auto document = parse_or_fail(build_psd(spec));
    check(document.guides.enabled, "guides enabled");
    check(document.guides.guides.size() == 2, "guide count");
    if (document.guides.guides.size() == 2)
    {
        check(document.guides.guides[0].vertical, "first guide is vertical");
        check(document.guides.guides[0].location == 10, "first guide position");
        check(!document.guides.guides[1].vertical, "second guide is horizontal");
    }
}

void test_16_bit_and_repeated_layer_info()
{
    TestDocument spec;
    spec.depth = 16;
    TestLayer layer;
    layer.name = "Layer1";
    layer.right = 4;
    layer.bottom = 4;
    // 16-bit channel: two bytes per sample, big endian. 0x8000 -> 0x80.
    Bytes plane16;
    for (int i = 0; i < 16; ++i)
    {
        plane16.push_back(0x80);
        plane16.push_back(0x00);
    }
    layer.channels = {{0, raw_channel(plane16)},
                      {1, raw_channel(plane16)},
                      {2, raw_channel(plane16)}};
    spec.layers.push_back(layer);
    spec.merged = make_planar_rgb(4, 4);

    auto document = parse_or_fail(build_psd(spec));
    std::vector<std::uint8_t> buffer(4 * 4 * 4);
    check(kzpsd::render_layer(document, document.layers[0], buffer.data(), 16,
                              kzpsd::ImageMode::Image, kzpsd::bgra),
          "render 16-bit");
    check_equal(reinterpret_cast<std::uint32_t*>(buffer.data())[0], bgra(0x80, 0x80, 0x80, 255),
                "16-bit high byte");

    // Lr16 replaces the primary layer table. The repeated table's layer paints
    // 0x40 so the two are distinguishable.
    Bytes plane16b;
    for (int i = 0; i < 16; ++i)
    {
        plane16b.push_back(0x40);
        plane16b.push_back(0x00);
    }
    TestDocument repeated;
    repeated.depth = 16;
    TestLayer repeatedLayer = layer;
    repeatedLayer.name = "Repeated";
    repeatedLayer.channels = {{0, raw_channel(plane16b)},
                              {1, raw_channel(plane16b)},
                              {2, raw_channel(plane16b)}};
    repeated.layers.push_back(repeatedLayer);
    const auto repeatedInfo = build_layer_info(repeated);
    char key[5] = "Lr16";
    spec.repeatedLayers.emplace_back();
    std::memcpy(spec.repeatedLayers[0].first, key, 5);
    spec.repeatedLayers[0].second = repeatedInfo;

    auto document2 = parse_or_fail(build_psd(spec));
    check(document2.layers.size() == 1, "repeated: layer count");
    check(document2.layers[0].name == "Repeated", "repeated: table wins");
    check(kzpsd::render_layer(document2, document2.layers[0], buffer.data(), 16,
                              kzpsd::ImageMode::Image, kzpsd::bgra),
          "render repeated");
    check_equal(reinterpret_cast<std::uint32_t*>(buffer.data())[0], bgra(0x40, 0x40, 0x40, 255),
                "repeated pixels");
}

void test_merged_image()
{
    TestDocument spec;
    TestLayer layer;
    layer.name = "Layer1";
    layer.right = 4;
    layer.bottom = 4;
    layer.channels = {{0, raw_channel(make_plane(1, 4, 4))}};
    spec.layers.push_back(layer);
    spec.merged = make_planar_rgb(4, 4);

    auto document = parse_or_fail(build_psd(spec));
    std::vector<std::uint8_t> buffer(4 * 4 * 4);
    check(kzpsd::render_merged(document, buffer.data(), 16, kzpsd::bgra), "render merged");
    check_equal(reinterpret_cast<std::uint32_t*>(buffer.data())[0], bgra(255, 0, 0, 255),
                "merged red");
    check_equal(reinterpret_cast<std::uint32_t*>(buffer.data())[1], bgra(0, 255, 0, 255),
                "merged green");
    check_equal(reinterpret_cast<std::uint32_t*>(buffer.data())[2], bgra(0, 0, 255, 255),
                "merged blue");
}

} // namespace

int main()
{
    test_compression();
    test_mask_and_alpha();
    test_layer_info_and_comps();
    test_slices_and_guides();
    test_16_bit_and_repeated_layer_info();
    test_merged_image();
    if (failures != 0)
        return 1;
    std::cout << "psd reader tests passed\n";
    return 0;
}
