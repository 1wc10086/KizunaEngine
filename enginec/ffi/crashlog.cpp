// Native crash logging.
//
// Why: signals (SIGSEGV & co) kill the process before any Dart error hook can
// run, and Android's tombstone lives where a player cannot reach it. So the
// report is written from the signal handler itself into the app-private
// directory Dart hands us (the same `crashes/` folder the Dart-side reports
// use), rotated by the existing crash-*.log pruning on the next start.
//
// Two hard rules this file must keep (both learned the hard way):
//
// 1. Handlers are installed only when Dart hands us the log directory (engine
//    spawn), never at library load. A process-wide SIGSEGV handler replaces
//    ART's fault manager (implicit null checks) and whatever GPU drivers
//    installed, and a fault that those subsystems handle benignly would then
//    land here — writing a report while holding their locks — and hang the app
//    at startup.
//
// 2. The handler CHAINS: it saves the previous sigaction at install time and,
//    after writing the report, restores it and returns. The faulting
//    instruction then re-executes and the previous handler (ART's fault
//    manager, a GPU driver, or the default action) sees the signal — so a
//    benign fault ART resolves into a Java exception still resolves, and a real
//    crash still dies with Android's own tombstone. The price is that a
//    benignly-resolved fault leaves a spurious report behind; treat single
//    reports with suspicion, not certainty.
//
// Async-signal-safety: the report fd is opened eagerly in
// KzcEngineSetCrashLogDirectory; the handler only formats into stack buffers
// and write()s. snprintf/dladdr/_Unwind_Backtrace are not formally
// signal-safe but are the practical norm for crash handlers; __android_log_*
// is deliberately NOT called here (its internal lock can be held by the
// faulting thread — a classic crash-handler deadlock).
//
// This file stays a plain TU (it imports only the C ABI declarations): it is
// process glue that must link and run before anything else, including during
// static initialization of the vendored engine, so it takes no dependency on the
// std module or on a module initializer of its own. The POSIX/system headers
// below are the only #include left, and they are exactly the "external library"
// case: the standard library does not provide sigaction/ucontext/dlfcn.

import enginec.ffi.api;

#include <csignal>
#include <cstdint>
#include <cstdarg>
#include <cstdio>
#include <cstring>
#include <ctime>
#include <filesystem>
#include <system_error>

#include <dlfcn.h>
#include <fcntl.h>
#include <unistd.h>
#include <unwind.h>

#if defined(__ANDROID__)
#include <android/log.h>
#endif

#if defined(__linux__) || defined(__ANDROID__)
#include <sys/syscall.h>
#include <sys/types.h>
#include <ucontext.h>
#endif

namespace {

// Crash state is global and lock-free on purpose: the handler must work when
// any thread of the engine dies, including ones that never touched Dart.
char g_dir[512] = {0};
int g_crash_fd = -1; // pre-opened crash report; -1 = fall back to stderr
volatile sig_atomic_t g_handling = 0;

// Previous dispositions for chaining (see rule 2 in the file comment).
constexpr int kSignalCount = 6;
const int g_signals[kSignalCount] = {SIGSEGV, SIGABRT, SIGBUS, SIGFPE, SIGILL, SIGTRAP};
struct sigaction g_previous[kSignalCount] = {};
bool g_previous_saved = false;

constexpr int kMaxFrames = 48;

void write_all(int fd, const char* data, size_t length)
{
    while (length > 0)
    {
        const ssize_t written = ::write(fd, data, length);
        if (written <= 0)
        {
            return;
        }
        data += written;
        length -= static_cast<size_t>(written);
    }
}

void emit(int fd, const char* text)
{
    write_all(fd, text, std::strlen(text));
}

void emit_line(int fd, const char* fmt, ...) __attribute__((format(printf, 2, 3)));

void emit_line(int fd, const char* fmt, ...)
{
    char buffer[512];
    va_list args;
    va_start(args, fmt);
    const int length = std::vsnprintf(buffer, sizeof(buffer) - 2, fmt, args);
    va_end(args);
    if (length <= 0)
    {
        return;
    }
    const size_t count = static_cast<size_t>(length) < sizeof(buffer) - 2
                             ? static_cast<size_t>(length)
                             : sizeof(buffer) - 2;
    buffer[count] = '\n';
    buffer[count + 1] = '\0';
    write_all(fd, buffer, count + 1);
}

const char* signal_name(int sig)
{
    switch (sig)
    {
    case SIGSEGV: return "SIGSEGV";
    case SIGABRT: return "SIGABRT";
    case SIGBUS: return "SIGBUS";
    case SIGFPE: return "SIGFPE";
    case SIGILL: return "SIGILL";
    case SIGTRAP: return "SIGTRAP";
    default: return "SIGNAL";
    }
}

void write_registers(int fd, void* uctx)
{
#if defined(__ANDROID__) && defined(__aarch64__)
    if (uctx == nullptr)
    {
        return;
    }
    const auto* uc = static_cast<const ucontext_t*>(uctx);
    emit_line(fd, "regs: pc=%p lr=%p sp=%p fp=%p",
              reinterpret_cast<void*>(uc->uc_mcontext.pc),
              reinterpret_cast<void*>(uc->uc_mcontext.regs[30]),
              reinterpret_cast<void*>(uc->uc_mcontext.sp),
              reinterpret_cast<void*>(uc->uc_mcontext.regs[29]));
#else
    (void)fd;
    (void)uctx;
#endif
}

// 统一的帧打印：模块名 + 模块内偏移（离线可用符号表解析），有符号时附带。
// 偏移是必须的：发布 .so 被 strip 后静态函数无名，仅裸 pc 无法归因。
void emit_frame(int fd, const char* tag, int index, std::uintptr_t pc)
{
    Dl_info info = {};
    if (dladdr(reinterpret_cast<const void*>(pc), &info) != 0)
    {
        const char* module = info.dli_fname != nullptr ? info.dli_fname : "?";
        const auto offset = reinterpret_cast<const char*>(pc) - static_cast<const char*>(info.dli_fbase);
        if (info.dli_sname != nullptr)
        {
            emit_line(fd, "%s#%02d pc %p  %s (%s+0x%tx)", tag, index,
                      reinterpret_cast<const void*>(pc), module, info.dli_sname,
                      reinterpret_cast<const char*>(pc) - static_cast<const char*>(info.dli_saddr));
        }
        else
        {
            emit_line(fd, "%s#%02d pc %p  %s (base+0x%tx)", tag, index,
                      reinterpret_cast<const void*>(pc), module, offset);
        }
    }
    else
    {
        emit_line(fd, "%s#%02d pc %p", tag, index, reinterpret_cast<const void*>(pc));
    }
}

struct UnwindState
{
    int fd;
    int count;
};

_Unwind_Reason_Code unwind_frame(_Unwind_Context* context, void* arg)
{
    auto* state = static_cast<UnwindState*>(arg);
    if (state->count >= kMaxFrames)
    {
        return _URC_END_OF_STACK;
    }
    const uintptr_t pc = _Unwind_GetIP(context);
    if (pc == 0)
    {
        return _URC_NO_REASON;
    }
    emit_frame(state->fd, "", state->count, pc);
    state->count++;
    return _URC_NO_REASON;
}

// Restore the previous disposition for `sig` and return: the faulting
// instruction re-executes, and the previous handler (ART's fault manager, a
// driver, or the default action) receives the signal. This is what keeps ART's
// implicit null checks and driver-level handlers working while our logger is
// installed (rule 2).
void chain_to_previous(int sig)
{
    for (int i = 0; i < kSignalCount; i++)
    {
        if (g_signals[i] == sig)
        {
            sigaction(sig, &g_previous[i], nullptr);
            return;
        }
    }
    std::signal(sig, SIG_DFL);
}

void crash_handler(int sig, siginfo_t* info, void* uctx)
{
    // Recursion guard: a fault inside the handler must not loop. Default
    // disposition from here: dying twice is better than hanging.
    if (g_handling != 0)
    {
        std::signal(sig, SIG_DFL);
        std::raise(sig);
        return;
    }
    g_handling = 1;

    const int fd = g_crash_fd >= 0 ? g_crash_fd : STDERR_FILENO;

    emit(fd, "\n=== KizunaEngine native crash ===\n");
    {
        const std::time_t now = std::time(nullptr);
        std::tm tm_snapshot{};
#if defined(__ANDROID__) || defined(__linux__)
        localtime_r(&now, &tm_snapshot);
#endif
        char stamp[64];
        std::strftime(stamp, sizeof(stamp), "%Y-%m-%d %H:%M:%S", &tm_snapshot);
        emit_line(fd, "time  : %s", stamp);
    }
    emit_line(fd, "signal: %d (%s), code %d", sig, signal_name(sig),
              info != nullptr ? info->si_code : 0);
    emit_line(fd, "fault : %p", info != nullptr ? info->si_addr : nullptr);
#if defined(__ANDROID__) || defined(__linux__)
    emit_line(fd, "pid   : %d, tid %ld", static_cast<int>(::getpid()),
              static_cast<long>(::syscall(SYS_gettid)));
#endif
    write_registers(fd, uctx);

    // Header first: if the unwind itself faults or stalls, the signal and
    // registers are already on disk.
    emit(fd, "--- backtrace ---\n");
    UnwindState state{fd, 0};
    _Unwind_Backtrace(unwind_frame, &state);
#if defined(__ANDROID__) && defined(__aarch64__)
    // _Unwind_Backtrace 在 signal 栈上经常断链（被 strip 的静态帧没有 unwind
    // 信息就停）。arm64 帧指针链永远打一份：fp 指向 [prev_fp, prev_lr]。
    // 坏链可能让遍历本身出错——报告头已落盘，可接受。
    if (uctx != nullptr)
    {
        emit(fd, "--- frame pointer chain ---\n");
        const auto* uc = static_cast<const ucontext_t*>(uctx);
        const unsigned long long* fp =
            reinterpret_cast<const unsigned long long*>(uc->uc_mcontext.regs[29]);
        int depth = 0;
        while (fp != nullptr && depth < kMaxFrames)
        {
            if ((reinterpret_cast<unsigned long long>(fp) & 7ull) != 0)
            {
                break; // 非对齐帧指针 = 链已损坏
            }
            const unsigned long long saved_fp = fp[0];
            const unsigned long long saved_lr = fp[1];
            if (saved_lr == 0)
            {
                break;
            }
            // PAC 签名的返回地址高位是垃圾，清掉高位认证码再打印/查符号。
            const unsigned long long lr = saved_lr & 0x00007fffffffffffULL;
            emit_frame(fd, "fp", depth, static_cast<uintptr_t>(lr));
            depth++;
            fp = reinterpret_cast<const unsigned long long*>(saved_fp);
        }
    }
#endif
    if (state.count == 0)
    {
        emit(fd, "(unwind produced no frames; see tombstone in logcat)\n");
    }
    emit(fd, "=== end of report ===\n");
    if (g_crash_fd >= 0)
    {
        ::fsync(g_crash_fd);
    }

    g_handling = 0;
    chain_to_previous(sig);
}

void install_handlers()
{
    struct sigaction action = {};
    action.sa_sigaction = crash_handler;
    action.sa_flags = SA_SIGINFO | SA_ONSTACK;
    sigemptyset(&action.sa_mask);
    for (int i = 0; i < kSignalCount; i++)
    {
        // Save whatever was there (ART's fault manager, a GPU driver, ...) so
        // the handler can chain instead of hijacking it.
        sigaction(g_signals[i], nullptr, &g_previous[i]);
        sigaction(g_signals[i], &action, nullptr);
    }
    g_previous_saved = true;
}

} // namespace

extern "C" __attribute__((visibility("default"))) void
KzcEngineSetCrashLogDirectory(const char* dir)
{
    if (dir == nullptr || *dir == '\0')
    {
        return;
    }
    std::snprintf(g_dir, sizeof(g_dir), "%s", dir);

    // Normal context: create the directory and open the report file eagerly so
    // the signal handler never has to. The name matches the Dart-side
    // crash-*.log convention, so the existing pruning rotates these too.
    try
    {
        std::error_code ec;
        std::filesystem::create_directories(g_dir, ec);
    }
    catch (...)
    {
    }

    const std::time_t now = std::time(nullptr);
    std::tm tm_snapshot{};
#if defined(__ANDROID__) || defined(__linux__)
    localtime_r(&now, &tm_snapshot);
#endif
    char name[640];
    std::snprintf(name, sizeof(name), "%s/crash-%04d%02d%02d-%02d%02d%02d-native.log", g_dir,
                  tm_snapshot.tm_year + 1900, tm_snapshot.tm_mon + 1, tm_snapshot.tm_mday,
                  tm_snapshot.tm_hour, tm_snapshot.tm_min, tm_snapshot.tm_sec);
    const int fd = ::open(name, O_CREAT | O_WRONLY | O_APPEND | O_CLOEXEC, 0644);
    if (fd >= 0)
    {
        if (g_crash_fd >= 0)
        {
            ::close(g_crash_fd);
        }
        g_crash_fd = fd;
        emit_line(fd, "--- crash log session (pid %d) ---", static_cast<int>(::getpid()));
    }
    // Handlers are installed here — at engine spawn, not at library load — and
    // chain to the previous disposition (rules 1 and 2 in the file comment).
    if (!g_previous_saved)
    {
        install_handlers();
    }
}

extern "C" __attribute__((visibility("default"))) void KzcEngineCrumb(const char* message)
{
    const char* text = message != nullptr ? message : "";
    char line[640];
    const std::time_t now = std::time(nullptr);
    std::tm tm_snapshot{};
#if defined(__ANDROID__) || defined(__linux__)
    localtime_r(&now, &tm_snapshot);
#endif
    const int length =
        std::snprintf(line, sizeof(line) - 1, "%02d:%02d:%02d %s\n", tm_snapshot.tm_hour,
                      tm_snapshot.tm_min, tm_snapshot.tm_sec, text);
    if (length <= 0)
    {
        return;
    }

    // Breadcrumbs go where the crash report goes; open-per-line keeps this out
    // of the signal path entirely and is cheap at engine-startup frequency.
    if (g_dir[0] != '\0')
    {
        char path[600];
        std::snprintf(path, sizeof(path), "%s/engine-crumb.log", g_dir);
        const int fd = ::open(path, O_CREAT | O_WRONLY | O_APPEND | O_CLOEXEC, 0644);
        if (fd >= 0)
        {
            write_all(fd, line, static_cast<size_t>(length));
            ::close(fd);
        }
    }
#if defined(__ANDROID__)
    __android_log_write(ANDROID_LOG_ERROR, "kizuna-crash", line);
#endif
    std::fputs(line, stderr);
}
