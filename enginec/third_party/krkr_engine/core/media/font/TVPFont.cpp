#include "tjsCommHead.h"

#include <algorithm>
#include <cctype>
#include <filesystem>
#include <fstream>
#include <iterator>
#include "TVPFont.h"
#include <ft2build.h>
#include FT_TRUETYPE_IDS_H
#include FT_SFNT_NAMES_H
#include FT_FREETYPE_H
#include "TVPStorage.h"
#include "TVPDebug.h"
#include "TVPMsg.h"
#include "Platform.h"
#include "PlatformFile.h"
#include "StringUtil.h"

// 崩溃面包屑（enginec/ffi/crashlog.cpp）：字体枚举各阶段留一行定位。
extern "C" void KzcEngineCrumb(const char* message);

tTJSHashTable<ttstr, TVPFontNamePathInfo, tTVPttstrHash> TVPFontNames;

//---------------------------------------------------------------------------

static ttstr TVPDefaultFontName;
static tTVPFont TVPDefaultFont;

static FT_Library TVPFontLibrary;
static FT_Library& TVPGetFontLibrary()
{
    if (!TVPFontLibrary)
    {
        FT_Error error = FT_Init_FreeType(&TVPFontLibrary);
        if (error)
            TVPThrowExceptionMessage((ttstr(TJS_N("Initialize FreeType failed, error = ")) +
                                      TJSIntegerToString((tjs_int)error))
                                         .c_str());
        TVPInitFontNames();
    }
    return TVPFontLibrary;
}
static void TVPReleaseFontLibrary()
{
    if (TVPFontLibrary)
    {
        FT_Done_FreeType(TVPFontLibrary);
    }
}

static int TVPInternalEnumFonts(
    FT_Byte* pBuf,
    int buflen,
    const ttstr& FontPath,
    const std::function<tTJSBinaryStream*(TVPFontNamePathInfo*)>& getter)
{
    unsigned int faceCount = 0;
    FT_Face fontface;
    FT_Error error = FT_New_Memory_Face(TVPGetFontLibrary(), pBuf, buflen, 0, &fontface);
    if (error)
    {
        TVPAddLog(ttstr(TJS_N("Load Font \"") + FontPath + "\" failed (" +
                        TJSIntegerToString((int)error) + ")"));
        return faceCount;
    }
    int nFaceNum = fontface->num_faces;
    for (int i = 0; i < nFaceNum; ++i)
    {
        if (i > 0)
        {
            if (FT_New_Memory_Face(TVPGetFontLibrary(), pBuf, buflen, i, &fontface))
            {
                continue;
            }
        }
        if (FT_IS_SCALABLE(fontface))
        {
            FT_UInt namecount = FT_Get_Sfnt_Name_Count(fontface);
            int addCount = 0;
            for (FT_UInt i = 0; i < namecount; ++i)
            {
                FT_SfntName name;
                if (FT_Get_Sfnt_Name(fontface, i, &name))
                {
                    continue;
                }
                if (name.name_id != TT_NAME_ID_FONT_FAMILY)
                {
                    continue;
                }
                if (name.platform_id != TT_PLATFORM_MICROSOFT)
                {
                    continue;
                }
                switch (name.language_id)
                { // for CJK names
                    case TT_MS_LANGID_JAPANESE_JAPAN:
                    case TT_MS_LANGID_CHINESE_GENERAL:
                    case TT_MS_LANGID_CHINESE_TAIWAN:
                    case TT_MS_LANGID_CHINESE_PRC:
                    case TT_MS_LANGID_CHINESE_HONG_KONG:
                    case TT_MS_LANGID_CHINESE_SINGAPORE:
                    case TT_MS_LANGID_KOREAN_EXTENDED_WANSUNG_KOREA:
                    case TT_MS_LANGID_KOREAN_JOHAB_KOREA:
                        break;
                    default:
                        continue;
                }
                ttstr fontname;
                if (name.encoding_id == TT_MS_ID_UNICODE_CS)
                {
                    std::vector<tjs_char> tmp;
                    int namelen = name.string_len / 2;
                    tmp.resize(namelen + 1);
                    for (int j = 0; j < namelen; ++j)
                    {
                        tmp[j] = (name.string[j * 2] << 8) | (name.string[j * 2 + 1]);
                    }
                    fontname = &tmp.front();
                }
                else
                {
                    continue;
                }
                TVPFontNamePathInfo info;
                info.Path = FontPath;
                info.Index = i;
                info.Getter = getter;
                TVPFontNames.Add(fontname, info);
                addCount = 1;
            }
            /*if (!addCount)*/ {
                ttstr fontname((tjs_char*)fontface->family_name);
                TVPFontNamePathInfo info;
                info.Path = FontPath;
                info.Index = i;
                info.Getter = getter;
                TVPFontNames.Add(fontname, info);
            }
            ++faceCount;
        }

        FT_Done_Face(fontface);
    }
    return faceCount;
}

//---------------------------------------------------------------------------
void TVPGetAllFontList(std::vector<ttstr>& list)
{
    auto itend = TVPFontNames.GetLast();
    for (auto it = TVPFontNames.GetFirst(); it != itend; ++it)
    {
        list.push_back(it.GetKey());
    }
}

void TVPInitFontNames()
{
    static bool TVPFontNamesInit = false;
    // enumlate all fonts
    if (TVPFontNamesInit)
        return;
    TVPFontNamesInit = true;
    do
    {
        ttstr userFont = "";
        if (!userFont.IsEmpty() && TVPEnumFontsProc(userFont))
            break;

        KzcEngineCrumb("fonts: default.ttf");
        if (TVPEnumFontsProc(TVPProjectDir + "default.ttf"))
            break;
        KzcEngineCrumb("fonts: default.ttc");
        if (TVPEnumFontsProc(TVPProjectDir + "default.ttc"))
            break;
        KzcEngineCrumb("fonts: default.otf");
        if (TVPEnumFontsProc(TVPProjectDir + "default.otf"))
            break;
        KzcEngineCrumb("fonts: default.otc");
        if (TVPEnumFontsProc(TVPProjectDir + "default.otc"))
            break;
        KzcEngineCrumb("fonts: no project font");

        tTVPMemoryStream* stream = nullptr;
        try
        {
            stream = GetResourceStream("DroidSansFallback.ttf");
        }
        catch (...)
        {
            // A portable build may not ship the reference Linux resource font.
            // Games that need a font ship their own (default.ttf or System.addFont).
            stream = nullptr;
        }
        if (stream != nullptr)
        {
            TVPInternalEnumFonts((tjs_uint8*)stream->GetInternalBuffer(), stream->GetSize(),
                                 "DroidSansFallback.ttf",
                                 [](TVPFontNamePathInfo* info) -> tTJSBinaryStream*
                                 { return GetResourceStream(info->Path.AsStdString()); });
            delete stream;
        }
    } while (false);
    // Portable hosts (Android/iOS/CI) may not ship the reference Linux resource
    // font and the game may keep its fonts inside an XP3. Load one known system
    // font directly from disk so TJS text rendering always has a face.
    if (TVPFontNames.GetCount() == 0)
    {
        KzcEngineCrumb("fonts: system fallback");

        // `path` is absolute, so the empty getter makes TVPCreateFontStream reopen
        // it when glyphs are needed; the bytes only have to live through the
        // enumeration below.
        const auto add_font_file = [](const std::string& path) -> bool
        {
            std::ifstream file(path, std::ios::binary);
            if (!file)
                return false;
            std::vector<FT_Byte> bytes((std::istreambuf_iterator<char>(file)),
                                       std::istreambuf_iterator<char>());
            if (bytes.empty())
                return false;
            TVPInternalEnumFonts(bytes.data(), static_cast<tjs_uint>(bytes.size()), path.c_str(), {});
            return TVPFontNames.GetCount() > 0;
        };

        const char* candidates[] = {
#if defined(_KRKRSDL3_ANDROID)
            "/system/fonts/NotoSansCJK-Regular.ttc",
            "/system/fonts/NotoSansCJKjp-Regular.otf",
            "/system/fonts/NotoSansSC-Regular.otf",
            "/system/fonts/DroidSansFallbackFull.ttf",
            "/system/fonts/DroidSansFallback.ttf",
#elif defined(_KRKRSDL3_IOS) || defined(_KRKRSDL3_MACOS)
            // iOS keeps PingFang under LanguageSupport; macOS puts it at the top
            // level and Hiragino in Supplemental. Both bundle layouts are tried.
            "/System/Library/Fonts/LanguageSupport/PingFang.ttc",
            "/System/Library/Fonts/PingFang.ttc",
            "/System/Library/Fonts/Core/PingFang.ttc",
            "/System/Library/Fonts/Hiragino Sans GB.ttc",
            "/System/Library/Fonts/Supplemental/Hiragino Sans GB.ttc",
            "/System/Library/Fonts/Hiragino Kaku Gothic W3.ttc",
            "/System/Library/Fonts/Helvetica.ttc",
#elif defined(_KRKRSDL3_WINDOWS)
            "C:\\Windows\\Fonts\\msgothic.ttc",
            "C:\\Windows\\Fonts\\meiryo.ttc",
            "C:\\Windows\\Fonts\\YuGothM.ttc",
            "C:\\Windows\\Fonts\\msyh.ttc",
            "C:\\Windows\\Fonts\\simsun.ttc",
            "C:\\Windows\\Fonts\\malgun.ttf",
#else
            "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
            "/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
            "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
            "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
#endif
        };
        for (const char* path : candidates)
        {
            if (add_font_file(path))
                break;
        }

        // Last resort: some vendors rename the CJK font (or ship it only as a
        // language-specific TTC), so a filename list can never cover every build.
        // Enumerate the font directory, try names that look like a CJK face first,
        // and stop at the first face that enumerates. The cap keeps startup bounded
        // on systems with hundreds of fonts.
        if (TVPFontNames.GetCount() == 0)
        {
            KzcEngineCrumb("fonts: scanning font directory");
            const char* font_dirs[] = {
#if defined(_KRKRSDL3_ANDROID)
                "/system/fonts", "/product/fonts", "/system/font",
#elif defined(_KRKRSDL3_IOS) || defined(_KRKRSDL3_MACOS)
                // iOS and macOS split system fonts across these subdirectories;
                // the scan is recursive, so the top-level dir suffices, but listing
                // them keeps the first candidates cheap.
                "/System/Library/Fonts",
                "/System/Library/Fonts/LanguageSupport",
                "/System/Library/Fonts/Supplemental",
                "/Library/Fonts",
#elif defined(_KRKRSDL3_WINDOWS)
                "C:\\Windows\\Fonts",
#else
                "/usr/share/fonts", "/usr/local/share/fonts",
#endif
            };
            std::vector<std::filesystem::path> files;
            // Depth-limited recursive walk: Linux/macOS/iOS keep fonts in language
            // subdirectories, so a flat iterator finds nothing there. The cap keeps
            // the walk bounded on systems with thousands of font files.
            constexpr std::size_t max_collected = 512;
            for (const char* dir : font_dirs)
            {
                std::error_code ec;
                std::filesystem::recursive_directory_iterator it(
                    dir, std::filesystem::directory_options::skip_permission_denied, ec);
                for (; !ec && it != std::filesystem::recursive_directory_iterator();
                     it.increment(ec))
                {
                    if (files.size() >= max_collected)
                        break;
                    std::error_code type_ec;
                    if (!it->is_regular_file(type_ec))
                        continue;
                    const std::string ext = it->path().extension().string();
                    if (ext == ".ttf" || ext == ".ttc" || ext == ".otf" || ext == ".otc")
                        files.push_back(it->path());
                }
                if (files.size() >= max_collected)
                    break;
            }
            std::stable_sort(files.begin(), files.end(),
                             [](const std::filesystem::path& a, const std::filesystem::path& b)
                             {
                                 const auto cjk_first = [](const std::filesystem::path& p)
                                 {
                                     std::string name = p.filename().string();
                                     // ASCII only: font file names are ASCII, and
                                     // std::tolower is ambiguous on bionic.
                                     for (char& c : name)
                                     {
                                         if (c >= 'A' && c <= 'Z')
                                             c = static_cast<char>(c - 'A' + 'a');
                                     }
                                     return name.find("cjk") != std::string::npos ||
                                            name.find("noto") != std::string::npos ||
                                            name.find("droid") != std::string::npos ||
                                            name.find("sans") != std::string::npos;
                                 };
                                 return cjk_first(a) && !cjk_first(b);
                             });
            int scanned = 0;
            for (const auto& path : files)
            {
                if (scanned >= 12)
                    break;
                ++scanned;
                if (add_font_file(path.string()))
                    break;
            }
        }
    }

    if (TVPFontNames.GetCount() > 0)
    {
        // Pick a usable default. The table preserves insertion order and the last
        // inserted face used to win; in a TTC that is often the monospace or a
        // regional variant (e.g. "Noto Sans Mono CJK KR"), which changes kanji
        // forms and Latin spacing. Prefer a Japanese face, then any non-monospace
        // face; ties keep the first (the face the font file itself starts with).
        auto best = TVPFontNames.GetFirst();
        int best_score = -1000;
        for (auto it = TVPFontNames.GetFirst();; ++it)
        {
            ttstr low(it.GetKey());
            low.ToLowerCase();
            int score = 0;
            if (low.IndexOf(TJS_N("jp")) != -1 || low.IndexOf(TJS_N("japan")) != -1)
                score += 2;
            if (low.IndexOf(TJS_N("mono")) != -1)
                score -= 3;
            if (score > best_score)
            {
                best = it;
                best_score = score;
            }
            if (it == TVPFontNames.GetLast())
                break;
        }
        TVPDefaultFontName = best.GetKey();
    }

    // check exePath + "/fonts/*.ttf"
    {
        std::vector<ttstr> list;
        auto lister = [&](const ttstr& name, tTVPLocalFileInfo* s)
        {
            if (s->Mode & (S_IFREG | S_IFDIR))
            {
                list.emplace_back(name);
            }
        };
        TVPGetLocalFileListAt(TVPGetLocallyAccessibleName(TVPProjectDir + "fonts"), lister);
        auto itend = list.end();
        for (auto it = list.begin(); it != itend; ++it)
        {
            TVPEnumFontsProc(*it);
        }
    }

    if (TVPDefaultFontName.IsEmpty())
    {
        KzcEngineCrumb("fonts: none found, showing message box");
        TVPShowSimpleMessageBox(("Could not found any font.\nPlease ensure "
                                 "that at least \"default.ttf\" exists"),
                                "Exception Occured");
    }
}

int TVPEnumFontsProc(const ttstr& FontPath)
{
    if (!TVPIsExistentStorageNoSearch(FontPath))
    {
        return 0;
    }

    tTJSBinaryStream* Stream = TVPCreateStream(FontPath, TJS_BS_READ);
    if (!Stream)
    {
        return 0;
    }
    int bufflen = Stream->GetSize();
    std::vector<FT_Byte> buf;
    buf.resize(bufflen);
    Stream->ReadBuffer(&buf.front(), bufflen);
    delete Stream;
    return TVPInternalEnumFonts(&buf.front(), bufflen, FontPath, nullptr);
}

const ttstr& TVPGetDefaultFontName()
{
    return TVPDefaultFontName;
}

tTJSBinaryStream* TVPCreateFontStream(const ttstr& fontname)
{
    TVPFontNamePathInfo* info = TVPFindFont(fontname);
    if (!info)
    {
        info = TVPFontNames.Find(TVPDefaultFontName);
        if (!info)
            return nullptr;
    }
    if (info->Getter)
    {
        return info->Getter(info);
    }
    return TVPCreateBinaryStreamForRead(info->Path, TJS_N(""));
}

//---------------------------------------------------------------------------
TVPFontNamePathInfo* TVPFindFont(const ttstr& fontname)
{
    // check existence of font
    TVPInitFontNames();

    TVPFontNamePathInfo* info = nullptr;
    if (!fontname.IsEmpty() && fontname[0] == TJS_N('@'))
    { // vertical version
        info = TVPFontNames.Find(fontname.c_str() + 1);
    }
    if (!info)
    {
        info = TVPFontNames.Find(fontname);
    }
    return info;
}

tjs_uint32 tTVPttstrHash::Make(const ttstr& val)
{
    const tjs_char* ptr = val.c_str();
    if (*ptr == 0)
        return 0;
    tjs_uint32 v = 0;
    while (*ptr)
    {
        v += *ptr;
        v += (v << 10);
        v ^= (v >> 6);
        ptr++;
    }
    v += (v << 3);
    v ^= (v >> 11);
    v += (v << 15);
    if (!v)
        v = (tjs_uint32)-1;
    return v;
}

bool TVPFontExists(const ttstr& name)
{
    TVPFontNamePathInfo* t = TVPFontNames.Find(name);
    return t != NULL;
}

ttstr TVPGetBeingFont(ttstr fonts)
{
    // retrieve being font in the system.
    // font candidates are given by "fonts", separated by comma.

    bool vfont;

    if (fonts.c_str()[0] == TJS_N('@'))
    { // for vertical writing
        fonts = fonts.c_str() + 1;
        vfont = true;
    }
    else
    {
        vfont = false;
    }

    static bool force_default_font = false;
    if (!force_default_font)
    {
        bool prev_empty_name = false;
        while (fonts != TJS_N(""))
        {
            ttstr fontname;
            int pos = fonts.IndexOf(TJS_N(","));
            if (pos != -1)
            {
                fontname = Trim(fonts.SubString(0, pos));
                fonts = fonts.SubString(pos + 1, -1);
            }
            else
            {
                fontname = Trim(fonts);
                fonts = TJS_N("");
            }

            // no existing check if previously specified font candidate is empty
            // eg. ",Fontname"

            if (fontname != TJS_N("") && (prev_empty_name || TVPFontExists(fontname)))
            {
                if (vfont && fontname.c_str()[0] != TJS_N('@'))
                {
                    return TJS_N("@") + fontname;
                }
                else
                {
                    return fontname;
                }
            }

            prev_empty_name = (fontname == TJS_N(""));
        }
    }

    if (vfont)
    {
        return ttstr(TJS_N("@")) + TVPGetDefaultFontName();
    }
    else
    {
        return TVPGetDefaultFontName();
    }
}

void TVPCreateDefaultFont()
{
    static bool TVPDefaultLOGFONTCreated = false;
    if (TVPDefaultLOGFONTCreated)
        return;
    TVPDefaultLOGFONTCreated = true;
    TVPDefaultFont.Height = -12;
    TVPDefaultFont.Flags = 0;
    TVPDefaultFont.Angle = 0;
    TVPDefaultFont.Face = ttstr(TVPGetDefaultFontName());
}

const tTVPFont& TVPGetDefaultFont()
{
    return TVPDefaultFont;
}
