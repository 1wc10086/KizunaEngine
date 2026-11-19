//---------------------------------------------------------------------------
// 离屏自动化测试宿主（桌面 SDL3 实现）
//---------------------------------------------------------------------------
#include "tjsCommHead.h"

#include <SDL3/SDL_keyboard.h>

#include <algorithm>
#include <bit>
#include <cctype>
#include <cstdint>
#include <cstring>
#include <string>

#include "PlatformAutoTest.h"
#include "PlatformView.h"
#include "TVPCompositor.h"
#include "TVPDebug.h"
#include "TVPScript.h"
#include "TVPSettings.h"
#include "TVPStorage.h"
#include "TVPGraphicsLoader.h"
#include "LayerBitmap.h"
#include "WindowManager.h"
#include "tjsError.h"

namespace
{
bool g_offscreen = false;
bool g_silent = false;
// 本次运行是否用到了测试宿主：主循环与送显路径的唯一判断，普通进游戏恒假
bool g_active = false;
tjs_uint64 g_frameBudget = 0;
std::string g_dumpPath;
std::string g_harness;
std::string g_lastError;

// 最近一次送显的整帧（RGBA 字节序，紧凑行距，alpha 恒为 0xff）
std::vector<uint8_t> g_frame;
tjs_int g_frameWidth = 0;
tjs_int g_frameHeight = 0;
tjs_int g_framePitch = 0;
tjs_uint64 g_frameCount = 0;
bool g_captureArmed = false;

// 内容版本：只有像素真的变了才自增。compose 拿它和上一帧比较，替掉过去
// "每次合成都把整帧扫一遍算 FNV" 的做法——1080p 一次就是 620 万次乘加，
// 30fps 下等于白烧掉一整个核，而结论还不如逐像素比较精确。
tjs_uint64 g_frameVersion = 0;
// 按需计算的像素指纹：导出/诊断要的时候才扫，结果缓存到下一次内容变化。
tjs_uint64 g_frameChecksum = 0;
bool g_frameChecksumValid = false;

// alpha 所在的字节按本机字节序取，大端机器上也不会把颜色写坏。
constexpr uint32_t alphaMask = std::endian::native == std::endian::little
                                   ? uint32_t{0xff000000u}
                                   : uint32_t{0x000000ffu};

// 缓存帧与刚回读的像素是否只有 alpha 不同。缓存里的 alpha 恒为 0xff，而合成
// 缓冲的 alpha 可以是 0（-holdalpha 关闭时），所以比较时先把后者的 alpha 补齐。
//
// 一次比较就是 8MB 级别的读（1080p 读两份）。按 64 位两个字一起比：循环次数减半、
// 编译器更容易向量化，这在这条路径上（每个送显帧都要跑一遍）是实打实的节省。
// alpha 掩码是"每 32 位像素的高字节"，64 位下就是两个字节位置。
constexpr uint64_t alphaMask64 = std::endian::native == std::endian::little
                                     ? uint64_t{0xff000000ff000000ull}
                                     : uint64_t{0x000000ff000000ffull};

// 一行像素里有没有非 alpha 的差异。行内按 8 字节比（循环次数减半、利于向量化），
// 收尾走 4 字节；两边行首不满足 8 字节对齐时整行退回 4 字节。
bool RowDiffers(const uint32_t* cached, const uint32_t* incoming, size_t words)
{
    const bool pairAligned = (words % 2) == 0 &&
                             (reinterpret_cast<std::uintptr_t>(cached) % 8) == 0 &&
                             (reinterpret_cast<std::uintptr_t>(incoming) % 8) == 0;
    size_t i = 0;
    if (pairAligned)
    {
        const auto* a64 = reinterpret_cast<const uint64_t*>(cached);
        const auto* b64 = reinterpret_cast<const uint64_t*>(incoming);
        const size_t pairs = words / 2;
        for (size_t p = 0; p < pairs; ++p)
            if (a64[p] != (b64[p] | alphaMask64))
                return true;
        i = pairs * 2;
    }
    for (; i < words; ++i)
        if (cached[i] != (incoming[i] | alphaMask))
            return true;
    return false;
}

bool SameRgb(const uint8_t* cached, const uint8_t* incoming, size_t rowBytes, int height,
             int sourcePitch)
{
    for (int y = 0; y < height; ++y)
    {
        const size_t words = rowBytes / 4;
        if (RowDiffers(reinterpret_cast<const uint32_t*>(cached + static_cast<size_t>(y) * rowBytes),
                       reinterpret_cast<const uint32_t*>(incoming + static_cast<size_t>(y) * sourcePitch),
                       words))
            return false;
    }
    return true;
}

bool TVPGetCommandLineNumber(const tjs_char* name, tjs_int64& value)
{
    tTJSVariant opt;
    if (!TVPGetCommandLine(name, &opt))
        return false;
    ttstr str(opt);
    if (str == TJS_N("yes"))
        return false;
    value = str.AsInteger();
    return true;
}

bool TVPGetCommandLineString(const tjs_char* name, std::string& value)
{
    tTJSVariant opt;
    if (!TVPGetCommandLine(name, &opt))
        return false;
    ttstr str(opt);
    if (str == TJS_N("yes"))
        return false;
    value = str.c_str();
    return true;
}

bool TVPGetCommandLineFlag(const tjs_char* name)
{
    tTJSVariant opt;
    return TVPGetCommandLine(name, &opt);
}

std::string imageFormatOf(const std::string& path)
{
    const size_t dot = path.find_last_of('.');
    if (dot == std::string::npos)
        return "png";
    std::string ext = path.substr(dot + 1);
    std::transform(ext.begin(), ext.end(), ext.begin(),
                   [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
    if (ext == "bmp")
        return "bmp";
    return "png";
}

tTVPMouseButton mouseButtonOf(tjs_int button)
{
    switch (button)
    {
        case 1:
            return mbRight;
        case 2:
            return mbMiddle;
        default:
            return mbLeft;
    }
}

// 收下回读到的整帧：行序统一成紧凑行距，并清掉预约（后端约定自上而下行序）
//
// 送到纹理里的这一帧随后会被下一次合成覆盖，所以必须整帧拷进 g_frame；但
// 拷贝可以省掉的情况不少：画面没变时引擎根本不会送显，而送显的帧又常常逐字节
// 相同（静态场景里的光标闪烁、只有 alpha 变化的半透明层）。比较与拷贝都以
// 32 位像素为单位：alpha 不计入比较（缓存里的 alpha 恒为 0xff，而合成缓冲的
// alpha 可以是 0），因此"只有 alpha 变了"不会被当成新画面，静态画面下的开销
// 就是这一遍比较。
void StoreCapturedFrame(const uint8_t* rgba, int width, int height, int pitch)
{
    g_captureArmed = false;
    if (!rgba || width <= 0 || height <= 0 || pitch <= 0)
    {
        g_lastError = "frame readback returned no pixels";
        return;
    }

    const size_t rowBytes = static_cast<size_t>(width) * 4;
    const size_t total = rowBytes * static_cast<size_t>(height);
    const bool sameGeometry =
        width == g_frameWidth && height == g_frameHeight && g_frame.size() == total;
    // 32 位访问要求源行距也是 4 的倍数。这台后端（LockTexture 返回 width*4）恒真，
    // 但保留一条保守分支：行距不对齐时退回逐字节路径，结果完全一样，只是慢。
    const bool wordAligned = pitch % 4 == 0;
    if (sameGeometry && wordAligned && SameRgb(g_frame.data(), rgba, rowBytes, height, pitch))
    {
        g_lastError.clear();
        if (!g_dumpPath.empty())
            g_captureArmed = true;
        return;
    }

    g_frameWidth = width;
    g_frameHeight = height;
    g_framePitch = static_cast<tjs_int>(rowBytes);
    g_frame.resize(total);
    if (wordAligned)
    {
        // 合成缓冲的 alpha 通道可以全是 0（-holdalpha 关闭时），窗口内容本身是不透明的，
        // 截图按不透明写，否则导出图片整幅透明 —— 所以拷贝和置 alpha 合成一遍。
        //
        // 帧内动画（对白框逐字、立绘淡入）通常只改屏幕的一部分：同一几何下按行比较，
        // 只重写真正变了的行。比较是 8 字节一次（见 RowDiffers），比"整帧扫描 + 整帧
        // 重写"省掉一次全幅读；全屏动画（OP、转场）下退化成每行一次比较，代价可忽略。
        const bool incremental = sameGeometry && wordAligned;
        for (int y = 0; y < height; ++y)
        {
            uint32_t* dst =
                reinterpret_cast<uint32_t*>(g_frame.data() + static_cast<size_t>(y) * rowBytes);
            const uint32_t* src = reinterpret_cast<const uint32_t*>(rgba + static_cast<size_t>(y) * pitch);
            const size_t words = rowBytes / 4;
            if (incremental && !RowDiffers(dst, src, words))
                continue;
            for (size_t i = 0; i < words; ++i)
                dst[i] = src[i] | alphaMask;
        }
    }
    else
    {
        for (int y = 0; y < height; ++y)
            std::memcpy(g_frame.data() + static_cast<size_t>(y) * rowBytes,
                        rgba + static_cast<size_t>(y) * pitch, rowBytes);
        for (size_t i = 3; i < total; i += 4)
            g_frame[i] = 0xff;
    }

    ++g_frameVersion;
    g_frameChecksumValid = false;
    g_lastError.clear();
    // -dump 要的是"最后一帧"：画面不变时不会送显，所以持续预约，保证缓存始终最新
    if (!g_dumpPath.empty())
        g_captureArmed = true;
}
} // namespace

namespace krkrsdl3
{
void TVPAutoTestSetup()
{
    g_offscreen = TVPGetCommandLineFlag(TJS_N("-offscreen"));
    g_silent = TVPGetCommandLineFlag(TJS_N("-silent"));

    tjs_int64 frames = 0;
    if (TVPGetCommandLineNumber(TJS_N("-frames"), frames) && frames > 0)
        g_frameBudget = static_cast<tjs_uint64>(frames);
    TVPGetCommandLineString(TJS_N("-dump"), g_dumpPath);
    TVPGetCommandLineString(TJS_N("-at_harness"), g_harness);
    g_captureArmed = !g_dumpPath.empty();
    g_active = g_offscreen || g_silent || g_frameBudget != 0 || !g_dumpPath.empty() ||
               !g_harness.empty();
}

bool TVPAutoTestActive()
{
    return g_active;
}

bool TVPAutoTestOffscreen()
{
    return g_offscreen;
}

bool TVPAutoTestSilent()
{
    return g_silent;
}

tjs_uint64 TVPAutoTestFrameBudget()
{
    return g_frameBudget;
}

const char* TVPAutoTestDumpPath()
{
    return g_dumpPath.c_str();
}

const char* TVPAutoTestLastError()
{
    return g_lastError.c_str();
}

void TVPAutoTestSetLastError(const char* reason)
{
    g_lastError = reason ? reason : "";
}

void TVPAutoTestArmCapture()
{
    // 预约即激活：脚本由游戏自身加载、没带任何命令行开关时也能取帧
    g_active = true;
    g_captureArmed = true;
}

void TVPAutoTestCaptureTexture(void* texture, int width, int height)
{
    // 只在按需抓取时回读整帧：每帧都回读会把离屏跑拖慢一个数量级
    if (!g_captureArmed || !texture)
        return;
    iTVPRenderBackend* backend = TVPGetRenderBackend();
    int pitch = 0;
    uint8_t* pixels = backend ? backend->LockTexture(texture, pitch) : nullptr;
    if (!pixels)
    {
        g_captureArmed = false;
        g_lastError = "renderer cannot read back the presented texture";
        return;
    }
    StoreCapturedFrame(pixels, width, height, pitch);
}

bool TVPAutoTestCapturePixels(const uint8_t* pixels, int width, int height, int pitch)
{
    // 软件后端的绘制缓冲本身就是 CPU 像素：直接收下，省掉"拷进窗口贴图再回读"
    // 的那一遍整帧拷贝。调用点（DrawDevice::Show）只在本帧被预约时才会走到这里。
    if (!g_captureArmed || !pixels)
        return false;
    StoreCapturedFrame(pixels, width, height, pitch);
    return true;
}

bool TVPAutoTestGetFrame(std::vector<uint8_t>& pixels, tjs_int& width, tjs_int& height,
                         tjs_int& pitch)
{
    if (g_frame.empty())
    {
        g_lastError = "no frame captured yet";
        return false;
    }
    pixels = g_frame;
    width = g_frameWidth;
    height = g_frameHeight;
    pitch = g_framePitch;
    return true;
}

bool TVPAutoTestFrameSize(tjs_int& width, tjs_int& height)
{
    if (g_frame.empty())
        return false;
    width = g_frameWidth;
    height = g_frameHeight;
    return true;
}

bool TVPAutoTestFrameView(const uint8_t** pixels, tjs_int& width, tjs_int& height,
                          tjs_int& pitch)
{
    // 借用而不是拷贝：合成路径每帧都要 8MB 级别的像素，再拷一份纯属浪费。
    // 指针在下次 StoreCapturedFrame 之前有效，而所有抓帧与合成都在引擎线程上
    // 串行发生（enginec 的 engine_core 用一把锁串行化全部 RPC），所以调用方
    // 不需要额外同步。
    if (g_frame.empty())
        return false;
    if (pixels)
        *pixels = g_frame.data();
    width = g_frameWidth;
    height = g_frameHeight;
    pitch = g_framePitch;
    return true;
}

tjs_uint64 TVPAutoTestFrameVersion()
{
    return g_frameVersion;
}

tjs_uint64 TVPAutoTestChecksum()
{
    // 只对 RGB 做 FNV-1a，一个像素混合一次（旧实现每字节一次）。值本身不落盘、
    // 只用来判等，所以减少混合次数不影响任何可见行为；结果缓存到内容变化为止，
    // 而合成路径已经改用 TVPAutoTestFrameVersion，这里只在导出/诊断时才会被调到。
    if (g_frame.empty())
        return 0;
    if (!g_frameChecksumValid)
    {
        tjs_uint64 hash = 1469598103934665603ull;
        for (tjs_int y = 0; y < g_frameHeight; ++y)
        {
            const uint8_t* row = g_frame.data() + static_cast<size_t>(y) * g_framePitch;
            for (tjs_int x = 0; x < g_frameWidth; ++x)
            {
                const uint8_t* p = row + x * 4;
                hash ^= static_cast<tjs_uint64>(p[0]) |
                        (static_cast<tjs_uint64>(p[1]) << 8) |
                        (static_cast<tjs_uint64>(p[2]) << 16);
                hash *= 1099511628211ull;
            }
        }
        g_frameChecksum = hash;
        g_frameChecksumValid = true;
    }
    return g_frameChecksum;
}

tjs_int TVPAutoTestBrightness()
{
    if (g_frame.empty())
        return -1;
    tjs_uint64 sum = 0;
    for (tjs_int y = 0; y < g_frameHeight; ++y)
    {
        const uint8_t* row = g_frame.data() + static_cast<size_t>(y) * g_framePitch;
        for (tjs_int x = 0; x < g_frameWidth; ++x)
        {
            const uint8_t* p = row + x * 4;
            sum += (p[0] * 299 + p[1] * 587 + p[2] * 114) / 1000;
        }
    }
    return static_cast<tjs_int>(sum / (static_cast<tjs_uint64>(g_frameWidth) * g_frameHeight));
}

bool TVPAutoTestSaveFrame(const char* path)
{
    std::vector<uint8_t> pixels;
    tjs_int width = 0, height = 0, pitch = 0;
    if (!TVPAutoTestGetFrame(pixels, width, height, pitch))
        return false;

    try
    {
        tTVPBaseTexture image(static_cast<tjs_uint>(width), static_cast<tjs_uint>(height), 32);
        image.Update(pixels.data(), static_cast<unsigned int>(pitch), 0, 0, width, height);
        const ttstr name = TVPNormalizeStorageName(ttstr(path));
        TVPSaveImage(name, ttstr(imageFormatOf(path).c_str()), &image, nullptr);
    }
    catch (const std::exception& e)
    {
        g_lastError = e.what();
        return false;
    }
    catch (const tjs_char* e)
    {
        g_lastError = e ? e : "";
        return false;
    }
    catch (...)
    {
        g_lastError = "save frame failed";
        return false;
    }
    return true;
}

tjs_uint64 TVPAutoTestFrameCount()
{
    return g_frameCount;
}

void TVPAutoTestNoteFrame()
{
    ++g_frameCount;
}

tjs_int TVPAutoTestStepFrames(tjs_int count)
{
    // interval=0 让 TVPDrawSceneOnce 立即跑完整一帧（与模态窗口同一驱动路径）
    tjs_int stepped = 0;
    for (tjs_int i = 0; i < count; ++i)
    {
        if (TVPDrawSceneOnce(0) != 0)
            break;
        ++stepped;
        if (g_frameBudget && g_frameCount >= g_frameBudget)
            break;
    }
    return stepped;
}

void TVPAutoTestRunHarness()
{
    if (g_harness.empty())
        return;
    try
    {
        TVPExecuteStorage(TVPNormalizeStorageName(ttstr(g_harness.c_str())));
    }
    catch (const TJS::eTJSScriptError& e)
    {
        g_lastError = std::string("harness: ") + e.GetMessage().c_str();
        TVPAddImportantLog(ttstr(g_lastError.c_str()));
    }
    catch (const TJS::eTJS& e)
    {
        g_lastError = std::string("harness: ") + e.GetMessage().c_str();
        TVPAddImportantLog(ttstr(g_lastError.c_str()));
    }
    catch (const std::exception& e)
    {
        g_lastError = std::string("harness: ") + e.what();
        TVPAddImportantLog(ttstr(g_lastError.c_str()));
    }
}

void TVPAutoTestWarpMouse(tjs_int x, tjs_int y)
{
    krkrsdl3::KRKR_Trig_MouseMove(x, y);
}

void TVPAutoTestMouseButton(bool down, tjs_int button, tjs_int x, tjs_int y)
{
    const tTVPMouseButton btn = mouseButtonOf(button);
    if (down)
        krkrsdl3::KRKR_Trig_MouseDown(btn, x, y);
    else
        krkrsdl3::KRKR_Trig_MouseUp(btn, x, y);
}

void TVPAutoTestMouseWheel(tjs_int x, tjs_int y, float dx, float dy)
{
    krkrsdl3::KRKR_Trig_MouseScroll(dx, dy, x, y);
}

bool TVPAutoTestKeyDown(const char* key, bool down)
{
    const SDL_Scancode scancode = SDL_GetScancodeFromName(key);
    if (scancode == SDL_SCANCODE_UNKNOWN)
    {
        g_lastError = std::string("unknown scancode name: ") + (key ? key : "");
        return false;
    }
    if (down)
        krkrsdl3::KRKR_Trig_KeyDown(scancode);
    else
        krkrsdl3::KRKR_Trig_KeyUp(scancode);
    return true;
}

void TVPAutoTestTextInput(const char* utf8)
{
    if (utf8)
        krkrsdl3::KRKR_Trig_TextInput(std::string(utf8));
}
} // namespace krkrsdl3
