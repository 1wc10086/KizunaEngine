// psd.dll: the PSD class and the psd:// storage media.
//
// Scripts use two things from this plugin:
//   * `new PSD()` + load()/getLayerData() to pull one layer of a layered
//     Photoshop document into a Layer, which is how a lot of character art is
//     shipped; and
//   * `psd://file.psd/root/folder/layer.bmp` (or .../id/<layer id>.bmp), which
//     make the same layers look like ordinary image files to the engine's
//     loader and to Layer.loadImages().
// Both are built on the reader in this directory, which is a module
// (enginec.krkr.psd, see psd.cppm); this file stays a plain TU because the TJS
// bridge it registers against is macro-based and macros cannot cross a module
// boundary.
//
// #include here is limited to the engine headers the TJS surface requires; the
// PSD data model and its algorithms come from the module.
#include "tjsCommHead.h"

#include "TVPStorage.h"
#include "TVPDebug.h"
#include "TVPMsg.h"
#include "UtilStreams.h"
#include "drawable.h"
#include "ncbind/ncbind.hpp"
#include "tjsArray.h"
#include "tjsDictionary.h"

import std;
import enginec.krkr.psd;

#define NCB_MODULE_NAME TJS_N("psd.dll")

namespace
{

ttstr layer_name(const kzpsd::Layer& layer)
{
    if (!layer.nameUnicode.empty())
        return ttstr(kzpsd::utf16_to_utf8(layer.nameUnicode));
    return ttstr(layer.name);
}

// PSD blend modes projected onto the engine's Layer.type values. Modes the
// engine cannot express become "normal", which is what the reference does.
int blend_to_layer_type(kzpsd::BlendMode mode)
{
    switch (mode)
    {
        case kzpsd::BlendMode::Darken: return ltPsDarken;
        case kzpsd::BlendMode::Multiply: return ltPsMultiplicative;
        case kzpsd::BlendMode::ColorBurn: return ltPsColorBurn;
        case kzpsd::BlendMode::LinearBurn: return ltPsSubtractive;
        case kzpsd::BlendMode::Lighten: return ltPsLighten;
        case kzpsd::BlendMode::Screen: return ltPsScreen;
        case kzpsd::BlendMode::ColorDodge: return ltPsColorDodge;
        case kzpsd::BlendMode::LinearDodge: return ltPsAdditive;
        case kzpsd::BlendMode::Overlay: return ltPsOverlay;
        case kzpsd::BlendMode::SoftLight: return ltPsSoftLight;
        case kzpsd::BlendMode::HardLight: return ltPsHardLight;
        case kzpsd::BlendMode::Difference: return ltPsDifference;
        case kzpsd::BlendMode::Exclusion: return ltPsExclusion;
        default: return ltPsNormal;
    }
}

class PSDStorage;

// One loaded document. The object identity matters: scripts keep the instance
// alive and the storage keeps a weak pointer so `psd://` reuses the parsed file.
class PSD
{
public:
    explicit PSD(iTJSDispatch2* objthis) : objthis_(objthis) {}
    ~PSD();

    static tjs_error factory(PSD** result, tjs_int numparams, tTJSVariant** param,
                             iTJSDispatch2* objthis);

    bool load(const tjs_char* filename);
    int get_width() const { return document_.loaded ? document_.width : -1; }
    int get_height() const { return document_.loaded ? document_.height : -1; }
    int get_channels() const { return document_.loaded ? document_.channels : -1; }
    int get_depth() const { return document_.loaded ? document_.depth : -1; }
    int get_color_mode() const { return document_.loaded ? static_cast<int>(document_.mode) : -1; }
    int get_layer_count() const
    {
        return document_.loaded ? static_cast<int>(document_.layers.size()) : -1;
    }

    int getLayerType(int no);
    ttstr getLayerName(int no);
    tTJSVariant getLayerInfo(int no);
    void getLayerData(iTJSDispatch2* layer, int no);
    void getLayerDataRaw(iTJSDispatch2* layer, int no);
    void getLayerDataMask(iTJSDispatch2* layer, int no);
    tTJSVariant getSlices();
    tTJSVariant getGuides();
    bool getBlend(iTJSDispatch2* layer);
    tTJSVariant getLayerComp();
    void clearStorageCache();

    // Storage support. `domain` is the lowercased file name the object was
    // loaded from; the storage uses it to resolve psd:// names.
    bool CheckExistentStorage(const ttstr& name, int* layerIndex) const;
    void GetListAt(const ttstr& pathname, iTVPStorageLister* lister) const;
    tTJSBinaryStream* openLayerImage(const ttstr& name) const;

private:
    void clearData();
    void checkLayerNo(int no) const;
    void startStorage() const;
    ttstr layer_path(const kzpsd::Layer& layer) const;
    ttstr layer_path_name(const kzpsd::Layer& layer) const;
    void write_layer(iTJSDispatch2* layer, int no, kzpsd::ImageMode mode);

    iTJSDispatch2* objthis_ = nullptr;
    kzpsd::Document document_;
    ttstr domain_;

    // Built on first storage query: layer paths and ids are only needed when a
    // script actually opens a psd:// name.
    mutable bool storageStarted_ = false;
    mutable std::map<ttstr, std::map<ttstr, int>> pathMap_;
    mutable std::map<int, int> layerIdIdxMap_;
};

// ---------------------------------------------------------------------------
// psd:// storage media
// ---------------------------------------------------------------------------
class PSDStorage : public iTVPStorageMedia
{
public:
    PSDStorage() : refCount_(1) {}

    void AddRef() override { ++refCount_; }
    void Release() override
    {
        if (refCount_ == 1)
            delete this;
        else
            --refCount_;
    }

    void GetName(ttstr& name) override { name = TJS_N("psd"); }
    void NormalizeDomainName(ttstr&) override {}
    void NormalizePathName(ttstr&) override {}

    bool CheckExistentStorage(const ttstr& name) override
    {
        ttstr path;
        PSD* psd = resolve(name, path);
        if (psd == nullptr)
            return false;
        return psd->CheckExistentStorage(path, nullptr);
    }

    tTJSBinaryStream* Open(const ttstr& name, tjs_uint32 flags) override
    {
        if (flags != TJS_BS_READ)
            TVPThrowExceptionMessage(TJS_N("%1:cannot open psdfile"), name);
        ttstr path;
        PSD* psd = resolve(name, path);
        auto* stream = psd != nullptr ? psd->openLayerImage(path) : nullptr;
        if (stream == nullptr)
            TVPThrowExceptionMessage(TJS_N("%1:cannot open psdfile"), name);
        return stream;
    }

    void GetListAt(const ttstr& name, iTVPStorageLister* lister) override
    {
        ttstr path;
        PSD* psd = resolve(name, path);
        // The reference lists nothing when the domain does not resolve, so a
        // missing file is not an error here.
        if (psd == nullptr)
            return;
        psd->GetListAt(path, lister);
    }

    void GetLocallyAccessibleName(ttstr& name) override { name = TJS_N(""); }

    void add(const ttstr& name, PSD* psd) { files_[name] = psd; }
    void remove(PSD* psd)
    {
        for (auto it = files_.begin(); it != files_.end();)
            it = it->second == psd ? files_.erase(it) : std::next(it);
        if (cached_ == psd)
            cached_ = nullptr;
    }
    void clearCache() { cached_ = nullptr; }

private:
    ~PSDStorage() override = default;

    // Splits "file.psd/root/..." into the file name and the remaining path.
    static bool split(const ttstr& name, ttstr& domain, ttstr& path)
    {
        const tjs_char* text = name.c_str();
        const tjs_char* slash = TJS_strchr(text, '/');
        if (slash == nullptr)
            return false;
        domain = ttstr(text, static_cast<tjs_int>(slash - text));
        path = ttstr(slash + 1);
        domain.ToLowerCase();
        return true;
    }

    // Resolves a media name to the document that owns it and the path inside
    // that document. A name for a file no script has opened yet is loaded on
    // demand and kept here: retail resolves psd:// against the file directly,
    // and games rely on that (they never construct PSD just to read a layer).
    PSD* resolve(const ttstr& name, ttstr& path)
    {
        ttstr domain;
        if (!split(name, domain, path))
            return nullptr;
        if (cached_ != nullptr && cachedDomain_ == domain)
            return cached_;
        const auto it = files_.find(domain);
        if (it != files_.end())
        {
            cached_ = it->second;
            cachedDomain_ = domain;
            return cached_;
        }

        // The document owns itself here; PSD::load registers it in files_ so the
        // next lookup takes the fast path. `domain` is the exact name scripts
        // use, which is also what load() records as its storage key.
        auto document = std::make_unique<PSD>(nullptr);
        if (!document->load(domain.c_str()))
            return nullptr;
        PSD* raw = document.get();
        owned_[domain] = std::move(document);
        cached_ = raw;
        cachedDomain_ = domain;
        return raw;
    }

    tjs_uint refCount_;
    std::map<ttstr, PSD*> files_;
    std::map<ttstr, std::unique_ptr<PSD>> owned_;
    PSD* cached_ = nullptr;
    ttstr cachedDomain_;
};

PSDStorage* storage = nullptr;

} // namespace

// The class must be registered before the storage can resolve names; both live
// in this translation unit, so the storage only needs the forward declaration.
PSD::~PSD() { clearData(); }

void PSD::clearData()
{
    if (storage != nullptr)
        storage->remove(this);
    storageStarted_ = false;
    pathMap_.clear();
    layerIdIdxMap_.clear();
    document_ = kzpsd::Document{};
    domain_ = TJS_N("");
}

tjs_error PSD::factory(PSD** result, tjs_int, tTJSVariant**, iTJSDispatch2* objthis)
{
    *result = new PSD(objthis);
    return TJS_S_OK;
}

bool PSD::load(const tjs_char* filename)
{
    clearData();

    ttstr placed = TVPGetPlacedPath(filename);
    if (placed.IsEmpty())
        placed = filename;

    tTJSBinaryStream* stream = nullptr;
    try
    {
        stream = TVPCreateStream(placed, TJS_BS_READ);
    }
    catch (...)
    {
        return false;
    }
    if (stream == nullptr)
        return false;

    std::vector<std::uint8_t> bytes;
    try
    {
        const auto size = stream->GetSize();
        if (size == 0 || size > static_cast<tjs_uint64>(std::numeric_limits<std::size_t>::max()))
        {
            delete stream;
            return false;
        }
        bytes.resize(static_cast<std::size_t>(size));
        stream->ReadBuffer(bytes.data(), static_cast<tjs_uint>(size));
    }
    catch (...)
    {
        delete stream;
        return false;
    }
    delete stream;

    std::string error;
    if (!document_.parse(std::move(bytes), error))
    {
        // A script only sees load() == false; naming the reason here is the only
        // way to tell a truncated file from an unsupported variant.
        TVPAddLog(ttstr(TJS_N("(warn) psd: cannot load %1: ")) + ttstr(filename) +
                  TJS_N(": ") + ttstr(error));
        return false;
    }

    // The storage key is the bare file name: scripts write
    // "psd://name.psd/root/..." and the storage splits at the first slash, so a
    // full path or a media prefix must not leak into the key.
    std::string key = ttstr(filename).AsStdString();
    if (const auto scheme = key.find("://"); scheme != std::string::npos)
        key.erase(0, scheme + 3);
    if (const auto slash = key.find_last_of('/'); slash != std::string::npos)
        key.erase(0, slash + 1);
    for (auto& ch : key)
    {
        if (ch >= 'A' && ch <= 'Z')
            ch = static_cast<char>(ch - 'A' + 'a');
    }
    domain_ = ttstr(key);
    if (storage != nullptr)
        storage->add(domain_, this);
    return true;
}

void PSD::checkLayerNo(int no) const
{
    if (!document_.loaded)
        TVPThrowExceptionMessage(TJS_N("no data"));
    if (no < 0 || no >= static_cast<int>(document_.layers.size()))
        TVPThrowExceptionMessage(TJS_N("not such layer"));
}

int PSD::getLayerType(int no)
{
    checkLayerNo(no);
    return static_cast<int>(document_.layers[static_cast<std::size_t>(no)].type);
}

ttstr PSD::getLayerName(int no)
{
    checkLayerNo(no);
    return layer_name(document_.layers[static_cast<std::size_t>(no)]);
}

tTJSVariant PSD::getLayerInfo(int no)
{
    checkLayerNo(no);
    const auto& layer = document_.layers[static_cast<std::size_t>(no)];

    tTJSVariant result;
    ncbDictionaryAccessor dict;
    if (!dict.IsValid())
        return result;

    dict.SetValue(TJS_N("top"), layer.top);
    dict.SetValue(TJS_N("left"), layer.left);
    dict.SetValue(TJS_N("bottom"), layer.bottom);
    dict.SetValue(TJS_N("right"), layer.right);
    dict.SetValue(TJS_N("width"), layer.width);
    dict.SetValue(TJS_N("height"), layer.height);
    dict.SetValue(TJS_N("opacity"), layer.opacity);
    dict.SetValue(TJS_N("fill_opacity"), layer.fillOpacity);
    bool hasMaskChannel = false;
    for (const auto& channel : layer.channels)
    {
        if (channel.id == -2 || channel.id == -3)
        {
            hasMaskChannel = true;
            break;
        }
    }
    dict.SetValue(TJS_N("mask"), hasMaskChannel);
    dict.SetValue(TJS_N("type"), blend_to_layer_type(layer.blend));
    dict.SetValue(TJS_N("layer_type"), static_cast<int>(layer.type));
    dict.SetValue(TJS_N("blend_mode"), static_cast<int>(layer.blend));
    dict.SetValue(TJS_N("visible"), (layer.flags & 2) == 0);
    dict.SetValue(TJS_N("name"), layer_name(layer));
    dict.SetValue(TJS_N("clipping"), layer.clipping);
    dict.SetValue(TJS_N("layer_id"), layer.layerId);
    dict.SetValue(TJS_N("obsolete"), (layer.flags & 4) != 0);
    dict.SetValue(TJS_N("transparency_protected"), (layer.flags & 1) != 0);
    dict.SetValue(TJS_N("pixel_data_irrelevant"), (layer.flags & 16) != 0);
    if (layer.parent >= 0)
        dict.SetValue(TJS_N("group_layer_id"),
                      document_.layers[static_cast<std::size_t>(layer.parent)].layerId);

    if (!layer.layerComps.empty())
    {
        ncbDictionaryAccessor comps;
        if (comps.IsValid())
        {
            for (const auto& entry : layer.layerComps)
            {
                const auto& comp = entry.second;
                ncbDictionaryAccessor item;
                if (!item.IsValid())
                    continue;
                item.SetValue(TJS_N("id"), comp.id);
                item.SetValue(TJS_N("offset_x"), comp.offsetX);
                item.SetValue(TJS_N("offset_y"), comp.offsetY);
                item.SetValue(TJS_N("enable"), comp.enabled);
                comps.SetValue(comp.id, item.GetDispatch());
            }
            dict.SetValue(TJS_N("layer_comp"), comps.GetDispatch());
        }
    }

    result = dict;
    return result;
}

void PSD::write_layer(iTJSDispatch2* layerObject, int no, kzpsd::ImageMode mode)
{
    if (layerObject == nullptr ||
        !layerObject->IsInstanceOf(0, 0, 0, TJS_N("Layer"), layerObject))
        TVPThrowExceptionMessage(TJS_N("not layer"));
    checkLayerNo(no);

    auto& layer = document_.layers[static_cast<std::size_t>(no)];
    const bool maskMode = mode == kzpsd::ImageMode::Mask;

    // Only ordinary layers carry pixels. A folder exposes just its mask, which
    // is how Photoshop lets a group act as a clipping mask.
    if (layer.type != kzpsd::LayerType::Normal &&
        !(maskMode && layer.type == kzpsd::LayerType::Folder))
        TVPThrowExceptionMessage(TJS_N("invalid layer type"));

    int width = maskMode ? layer.mask.width : layer.width;
    int height = maskMode ? layer.mask.height : layer.height;
    bool dummyMask = false;
    if (maskMode && (width <= 0 || height <= 0))
    {
        // A layer may have a mask channel without a mask rectangle. The
        // reference still hands scripts a 1x1 layer filled with the mask's
        // default colour, so callers can rely on a usable layer object.
        width = 1;
        height = 1;
        dummyMask = true;
    }
    if (width <= 0 || height <= 0)
        return; // a zero-sized layer has no pixels to read

    ncbPropAccessor object(layerObject);
    object.SetValue(TJS_N("left"), maskMode ? layer.mask.left : layer.left);
    object.SetValue(TJS_N("top"), maskMode ? layer.mask.top : layer.top);
    object.SetValue(TJS_N("opacity"), layer.opacity);
    object.SetValue(TJS_N("fill_opacity"), layer.fillOpacity);
    object.SetValue(TJS_N("width"), width);
    object.SetValue(TJS_N("height"), height);
    object.SetValue(TJS_N("type"), blend_to_layer_type(layer.blend));
    object.SetValue(TJS_N("visible"), (layer.flags & 2) == 0);
    object.SetValue(TJS_N("imageLeft"), 0);
    object.SetValue(TJS_N("imageTop"), 0);
    object.SetValue(TJS_N("imageWidth"), width);
    object.SetValue(TJS_N("imageHeight"), height);
    object.SetValue(TJS_N("name"), layer_name(layer));
    if (maskMode)
        object.SetValue(TJS_N("defaultMaskColor"), layer.mask.defaultColor);

    auto* buffer = reinterpret_cast<std::uint8_t*>(
        object.GetValue(TJS_N("mainImageBufferForWrite"), ncbTypedefs::Tag<tjs_intptr_t>()));
    const auto pitch = object.GetValue(TJS_N("mainImageBufferPitch"), ncbTypedefs::Tag<tjs_int>());
    if (buffer == nullptr || pitch <= 0)
        TVPThrowExceptionMessage(TJS_N("cannot access layer buffer"));

    if (dummyMask)
    {
        buffer[0] = buffer[1] = buffer[2] = static_cast<std::uint8_t>(layer.mask.defaultColor);
        buffer[3] = 0xff;
        return;
    }
    if (!kzpsd::render_layer(document_, layer, buffer, pitch, mode, kzpsd::bgra))
        TVPThrowExceptionMessage(TJS_N("cannot decode psd layer"));
}

void PSD::getLayerData(iTJSDispatch2* layer, int no)
{
    write_layer(layer, no, kzpsd::ImageMode::MaskedImage);
}

void PSD::getLayerDataRaw(iTJSDispatch2* layer, int no)
{
    write_layer(layer, no, kzpsd::ImageMode::Image);
}

void PSD::getLayerDataMask(iTJSDispatch2* layer, int no)
{
    write_layer(layer, no, kzpsd::ImageMode::Mask);
}

tTJSVariant PSD::getSlices()
{
    if (!document_.loaded)
        TVPThrowExceptionMessage(TJS_N("no data"));
    tTJSVariant result;
    if (!document_.slices.enabled)
        return result;

    ncbDictionaryAccessor dict;
    ncbArrayAccessor array;
    if (!dict.IsValid() || !array.IsValid())
        return result;

    const auto& slices = document_.slices;
    dict.SetValue(TJS_N("top"), slices.boundingTop);
    dict.SetValue(TJS_N("left"), slices.boundingLeft);
    dict.SetValue(TJS_N("bottom"), slices.boundingBottom);
    dict.SetValue(TJS_N("right"), slices.boundingRight);
    dict.SetValue(TJS_N("name"), ttstr(kzpsd::utf16_to_utf8(slices.groupName)));
    for (std::size_t i = 0; i < slices.items.size(); ++i)
    {
        const auto& item = slices.items[i];
        ncbDictionaryAccessor entry;
        if (!entry.IsValid())
            continue;
        entry.SetValue(TJS_N("id"), item.id);
        entry.SetValue(TJS_N("group_id"), item.groupId);
        entry.SetValue(TJS_N("origin"), item.origin);
        entry.SetValue(TJS_N("type"), item.type);
        entry.SetValue(TJS_N("left"), item.left);
        entry.SetValue(TJS_N("top"), item.top);
        entry.SetValue(TJS_N("right"), item.right);
        entry.SetValue(TJS_N("bottom"), item.bottom);
        entry.SetValue(TJS_N("color"), static_cast<tjs_int64>(item.color));
        entry.SetValue(TJS_N("cell_text_is_html"), item.cellTextIsHtml);
        entry.SetValue(TJS_N("horizontal_alignment"), item.horizontalAlign);
        entry.SetValue(TJS_N("vertical_alignment"), item.verticalAlign);
        entry.SetValue(TJS_N("associated_layer_id"), item.associatedLayerId);
        entry.SetValue(TJS_N("name"), ttstr(kzpsd::utf16_to_utf8(item.name)));
        entry.SetValue(TJS_N("url"), ttstr(kzpsd::utf16_to_utf8(item.url)));
        entry.SetValue(TJS_N("target"), ttstr(kzpsd::utf16_to_utf8(item.target)));
        entry.SetValue(TJS_N("message"), ttstr(kzpsd::utf16_to_utf8(item.message)));
        entry.SetValue(TJS_N("alt_tag"), ttstr(kzpsd::utf16_to_utf8(item.altTag)));
        entry.SetValue(TJS_N("cell_text"), ttstr(kzpsd::utf16_to_utf8(item.cellText)));
        array.SetValue(static_cast<tjs_int32>(i), entry.GetDispatch());
    }
    dict.SetValue(TJS_N("slices"), array.GetDispatch());
    result = dict;
    return result;
}

tTJSVariant PSD::getGuides()
{
    if (!document_.loaded)
        TVPThrowExceptionMessage(TJS_N("no data"));
    tTJSVariant result;
    if (!document_.guides.enabled)
        return result;

    ncbDictionaryAccessor dict;
    ncbArrayAccessor vertical, horizontal;
    if (!dict.IsValid() || !vertical.IsValid() || !horizontal.IsValid())
        return result;

    dict.SetValue(TJS_N("horz_grid"), document_.guides.horizontalGrid);
    dict.SetValue(TJS_N("vert_grid"), document_.guides.verticalGrid);
    tjs_int32 v = 0;
    tjs_int32 h = 0;
    for (const auto& guide : document_.guides.guides)
    {
        if (guide.vertical)
            vertical.SetValue(v++, guide.location);
        else
            horizontal.SetValue(h++, guide.location);
    }
    dict.SetValue(TJS_N("vertical"), vertical.GetDispatch());
    dict.SetValue(TJS_N("horizontal"), horizontal.GetDispatch());
    result = dict;
    return result;
}

bool PSD::getBlend(iTJSDispatch2* layerObject)
{
    if (layerObject == nullptr ||
        !layerObject->IsInstanceOf(0, 0, 0, TJS_N("Layer"), layerObject))
        TVPThrowExceptionMessage(TJS_N("not layer"));
    if (!document_.loaded)
        return false;

    ncbPropAccessor object(layerObject);
    object.SetValue(TJS_N("width"), document_.width);
    object.SetValue(TJS_N("height"), document_.height);
    object.SetValue(TJS_N("imageLeft"), 0);
    object.SetValue(TJS_N("imageTop"), 0);
    object.SetValue(TJS_N("imageWidth"), document_.width);
    object.SetValue(TJS_N("imageHeight"), document_.height);

    auto* buffer = reinterpret_cast<std::uint8_t*>(
        object.GetValue(TJS_N("mainImageBufferForWrite"), ncbTypedefs::Tag<tjs_intptr_t>()));
    const auto pitch = object.GetValue(TJS_N("mainImageBufferPitch"), ncbTypedefs::Tag<tjs_int>());
    if (buffer == nullptr || pitch <= 0)
        return false;
    return kzpsd::render_merged(document_, buffer, pitch, kzpsd::bgra);
}

tTJSVariant PSD::getLayerComp()
{
    if (!document_.loaded)
        TVPThrowExceptionMessage(TJS_N("no data"));
    tTJSVariant result;
    if (document_.layerComps.empty())
        return result;

    ncbDictionaryAccessor dict;
    ncbArrayAccessor array;
    if (!dict.IsValid() || !array.IsValid())
        return result;

    dict.SetValue(TJS_N("last_applied_id"), document_.lastAppliedCompId);
    for (std::size_t i = 0; i < document_.layerComps.size(); ++i)
    {
        const auto& comp = document_.layerComps[i];
        ncbDictionaryAccessor entry;
        if (!entry.IsValid())
            continue;
        entry.SetValue(TJS_N("id"), comp.id);
        entry.SetValue(TJS_N("record_visibility"), comp.recordVisibility);
        entry.SetValue(TJS_N("record_position"), comp.recordPosition);
        entry.SetValue(TJS_N("record_appearance"), comp.recordAppearance);
        entry.SetValue(TJS_N("name"), ttstr(comp.name));
        entry.SetValue(TJS_N("comment"), ttstr(comp.comment));
        array.SetValue(static_cast<tjs_int32>(i), entry.GetDispatch());
    }
    dict.SetValue(TJS_N("comps"), array.GetDispatch());
    result = dict;
    return result;
}

void PSD::clearStorageCache()
{
    if (storage != nullptr)
        storage->clearCache();
}

// ---------------------------------------------------------------------------
// psd:// name resolution
// ---------------------------------------------------------------------------
ttstr PSD::layer_path_name(const kzpsd::Layer& layer) const
{
    ttstr name = layer_name(layer);
    name.Replace(TJS_N("/"), TJS_N("_"), true);
    name.ToLowerCase();
    return name;
}

ttstr PSD::layer_path(const kzpsd::Layer& layer) const
{
    ttstr name;
    const kzpsd::Layer* parent = layer.parent >= 0 ? &document_.layers[static_cast<std::size_t>(layer.parent)] : nullptr;
    while (parent != nullptr)
    {
        name = layer_path_name(*parent) + TJS_N("/") + name;
        parent = parent->parent >= 0 ? &document_.layers[static_cast<std::size_t>(parent->parent)] : nullptr;
    }
    return ttstr(TJS_N("root/")) + name;
}

void PSD::startStorage() const
{
    if (storageStarted_)
        return;
    storageStarted_ = true;
    for (int i = static_cast<int>(document_.layers.size()) - 1; i >= 0; --i)
    {
        const auto& layer = document_.layers[static_cast<std::size_t>(i)];
        if (layer.type != kzpsd::LayerType::Normal)
            continue;
        pathMap_[layer_path(layer)][layer_path_name(layer)] = i;
        if (layer.layerId >= 0)
            layerIdIdxMap_[layer.layerId] = i;
    }
}

bool PSD::CheckExistentStorage(const ttstr& name, int* layerIndex) const
{
    startStorage();
    const tjs_char* text = name.c_str();

    if (TJS_strncmp(text, TJS_N("id/"), 3) == 0)
    {
        text += 3;
        const tjs_char* dot = TJS_strchr(text, '.');
        if (dot == nullptr || TJS_strcmp(dot, TJS_N(".bmp")) != 0)
            return false;
        ttstr digits(text, static_cast<tjs_int>(dot - text));
        if (digits.IsEmpty())
            return false;
        for (const tjs_char* p = digits.c_str(); *p != 0; ++p)
        {
            if (*p < '0' || *p > '9')
                return false;
        }
        const auto it = layerIdIdxMap_.find(TJS_atoi(digits.c_str()));
        if (it == layerIdIdxMap_.end())
            return false;
        if (layerIndex != nullptr)
            *layerIndex = it->second;
        return true;
    }

    const tjs_char* slash = TJS_strrchr(text, '/');
    if (slash == nullptr)
        return false;
    const ttstr path(text, static_cast<tjs_int>(slash - text + 1));
    const tjs_char* file = slash + 1;
    const tjs_char* dot = TJS_strchr(file, '.');
    if (dot == nullptr || TJS_strcmp(dot, TJS_N(".bmp")) != 0)
        return false;
    ttstr base(file, static_cast<tjs_int>(dot - file));

    const auto pathIt = pathMap_.find(path);
    if (pathIt == pathMap_.end())
        return false;
    const auto nameIt = pathIt->second.find(base);
    if (nameIt == pathIt->second.end())
        return false;
    if (layerIndex != nullptr)
        *layerIndex = nameIt->second;
    return true;
}

void PSD::GetListAt(const ttstr& pathname, iTVPStorageLister* lister) const
{
    startStorage();
    if (pathname == TJS_N("id/"))
    {
        for (const auto& entry : layerIdIdxMap_)
            lister->Add(ttstr(entry.first) + TJS_N(".bmp"));
        return;
    }
    const auto it = pathMap_.find(pathname);
    if (it == pathMap_.end())
        return;
    for (const auto& entry : it->second)
        lister->Add(entry.first + TJS_N(".bmp"));
}

tTJSBinaryStream* PSD::openLayerImage(const ttstr& name) const
{
    int index = 0;
    if (!CheckExistentStorage(name, &index))
        return nullptr;
    if (index >= static_cast<int>(document_.layers.size()))
        return nullptr;
    const auto& layer = document_.layers[static_cast<std::size_t>(index)];
    if (layer.type != kzpsd::LayerType::Normal || layer.width <= 0 || layer.height <= 0)
        return nullptr;

    const int width = layer.width;
    const int height = layer.height;
    const int pitch = width * 4;
    const auto imageSize = static_cast<std::size_t>(pitch) * height;
    const auto headerSize = 14 + 40;

    std::vector<std::uint8_t> bmp(headerSize + imageSize, 0);
    // 14-byte BITMAPFILEHEADER: type, size, reserved, pixel offset. Little
    // endian, packed, which is exactly the layout Windows writes.
    bmp[0] = 'B';
    bmp[1] = 'M';
    const auto fileSize = static_cast<std::uint32_t>(bmp.size());
    bmp[2] = static_cast<std::uint8_t>(fileSize);
    bmp[3] = static_cast<std::uint8_t>(fileSize >> 8);
    bmp[4] = static_cast<std::uint8_t>(fileSize >> 16);
    bmp[5] = static_cast<std::uint8_t>(fileSize >> 24);
    bmp[10] = static_cast<std::uint8_t>(headerSize);
    // 40-byte BITMAPINFOHEADER: size, width, height, planes, bpp, compression.
    const auto write32 = [&bmp](std::size_t offset, std::uint32_t value) {
        bmp[offset] = static_cast<std::uint8_t>(value);
        bmp[offset + 1] = static_cast<std::uint8_t>(value >> 8);
        bmp[offset + 2] = static_cast<std::uint8_t>(value >> 16);
        bmp[offset + 3] = static_cast<std::uint8_t>(value >> 24);
    };
    write32(14, 40);
    write32(18, static_cast<std::uint32_t>(width));
    write32(22, static_cast<std::uint32_t>(height));
    bmp[26] = 1; // planes
    bmp[28] = 32; // bits per pixel

    // BMP rows run bottom-up, so the last source row is written first.
    if (!kzpsd::render_layer(document_, layer, bmp.data() + headerSize + static_cast<std::size_t>(height - 1) * pitch,
                             -pitch, kzpsd::ImageMode::MaskedImage, kzpsd::bgra))
        return nullptr;

    // The engine's BMP loader reads a stream; give it an owned copy so the
    // vector can go away when this function returns.
    auto* stream = new tTVPMemoryStream();
    stream->Write(bmp.data(), static_cast<tjs_uint>(bmp.size()));
    stream->Seek(0, TJS_BS_SEEK_SET);
    return stream;
}

namespace
{

PSDStorage* ensure_storage()
{
    if (storage == nullptr)
    {
        storage = new PSDStorage();
        TVPRegisterStorageMedia(storage);
    }
    return storage;
}

} // namespace

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------
NCB_REGISTER_CLASS(PSD)
{
    Factory(&ClassT::factory);

    Variant(TJS_N("color_mode_bitmap"), static_cast<tjs_int64>(0));
    Variant(TJS_N("color_mode_grayscale"), static_cast<tjs_int64>(1));
    Variant(TJS_N("color_mode_indexed"), static_cast<tjs_int64>(2));
    Variant(TJS_N("color_mode_rgb"), static_cast<tjs_int64>(3));
    Variant(TJS_N("color_mode_cmyk"), static_cast<tjs_int64>(4));
    Variant(TJS_N("color_mode_multichannel"), static_cast<tjs_int64>(7));
    Variant(TJS_N("color_mode_duotone"), static_cast<tjs_int64>(8));
    Variant(TJS_N("color_mode_lab"), static_cast<tjs_int64>(9));

    Variant(TJS_N("blend_mode_normal"), static_cast<tjs_int64>(0));
    Variant(TJS_N("blend_mode_dissolve"), static_cast<tjs_int64>(1));
    Variant(TJS_N("blend_mode_darken"), static_cast<tjs_int64>(2));
    Variant(TJS_N("blend_mode_multiply"), static_cast<tjs_int64>(3));
    Variant(TJS_N("blend_mode_color_burn"), static_cast<tjs_int64>(4));
    Variant(TJS_N("blend_mode_linear_burn"), static_cast<tjs_int64>(5));
    Variant(TJS_N("blend_mode_lighten"), static_cast<tjs_int64>(6));
    Variant(TJS_N("blend_mode_screen"), static_cast<tjs_int64>(7));
    Variant(TJS_N("blend_mode_color_dodge"), static_cast<tjs_int64>(8));
    Variant(TJS_N("blend_mode_linear_dodge"), static_cast<tjs_int64>(9));
    Variant(TJS_N("blend_mode_overlay"), static_cast<tjs_int64>(10));
    Variant(TJS_N("blend_mode_soft_light"), static_cast<tjs_int64>(11));
    Variant(TJS_N("blend_mode_hard_light"), static_cast<tjs_int64>(12));
    Variant(TJS_N("blend_mode_vivid_light"), static_cast<tjs_int64>(13));
    Variant(TJS_N("blend_mode_linear_light"), static_cast<tjs_int64>(14));
    Variant(TJS_N("blend_mode_pin_light"), static_cast<tjs_int64>(15));
    Variant(TJS_N("blend_mode_hard_mix"), static_cast<tjs_int64>(16));
    Variant(TJS_N("blend_mode_difference"), static_cast<tjs_int64>(17));
    Variant(TJS_N("blend_mode_exclusion"), static_cast<tjs_int64>(18));
    Variant(TJS_N("blend_mode_hue"), static_cast<tjs_int64>(19));
    Variant(TJS_N("blend_mode_saturation"), static_cast<tjs_int64>(20));
    Variant(TJS_N("blend_mode_color"), static_cast<tjs_int64>(21));
    Variant(TJS_N("blend_mode_luminosity"), static_cast<tjs_int64>(22));
    Variant(TJS_N("blend_mode_pass_through"), static_cast<tjs_int64>(23));
    Variant(TJS_N("blend_mode_darker_color"), static_cast<tjs_int64>(24));
    Variant(TJS_N("blend_mode_lighter_color"), static_cast<tjs_int64>(25));
    Variant(TJS_N("blend_mode_subtract"), static_cast<tjs_int64>(26));
    Variant(TJS_N("blend_mode_divide"), static_cast<tjs_int64>(27));

    Variant(TJS_N("layer_type_normal"), static_cast<tjs_int64>(0));
    Variant(TJS_N("layer_type_hidden"), static_cast<tjs_int64>(1));
    Variant(TJS_N("layer_type_folder"), static_cast<tjs_int64>(2));
    Variant(TJS_N("layer_type_adjust"), static_cast<tjs_int64>(3));
    Variant(TJS_N("layer_type_fill"), static_cast<tjs_int64>(4));

    NCB_METHOD(load);
    NCB_PROPERTY_RO(width, get_width);
    NCB_PROPERTY_RO(height, get_height);
    NCB_PROPERTY_RO(channels, get_channels);
    NCB_PROPERTY_RO(depth, get_depth);
    NCB_PROPERTY_RO(color_mode, get_color_mode);
    NCB_PROPERTY_RO(layer_count, get_layer_count);

    NCB_METHOD(getLayerType);
    NCB_METHOD(getLayerName);
    NCB_METHOD(getLayerInfo);
    NCB_METHOD(getLayerData);
    NCB_METHOD(getLayerDataRaw);
    NCB_METHOD(getLayerDataMask);
    NCB_METHOD(getSlices);
    NCB_METHOD(getGuides);
    NCB_METHOD(getBlend);
    NCB_METHOD(getLayerComp);
    NCB_METHOD(clearStorageCache);
}

static void psd_init()
{
    ensure_storage();
}

static void psd_done()
{
    if (storage != nullptr)
    {
        TVPUnregisterStorageMedia(storage);
        storage->Release();
        storage = nullptr;
    }
}

NCB_PRE_REGIST_CALLBACK(psd_init);
NCB_POST_UNREGIST_CALLBACK(psd_done);
