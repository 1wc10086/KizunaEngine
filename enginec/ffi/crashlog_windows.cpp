// Native crash logging for Windows (MSVC).
//
// Why a separate file: crashlog.cpp is written against POSIX (sigaction,
// ucontext, dladdr, _Unwind_Backtrace, unistd) and cannot compile with MSVC. It
// still owns the ABI — the enginec.ffi.api module declares
// KzcEngineSetCrashLogDirectory and KzcEngineCrumb on every platform — so this
// file provides the same two entry
// points with the Windows equivalents:
//
//   * breadcrumbs  -> engine-crumb.log (append, one fopen per line)
//   * fatal faults -> SetUnhandledExceptionFilter writes exception code and the
//                     faulting address into crash-<stamp>-native.log.
//
// Deliberately no DbgHelp stack walk: it needs the PDB files that only the
// developer build has, and LoadLibrary inside a crash filter can deadlock on the
// loader lock. Code + address are enough to pin the failure with the PDB.
//
// This is a plain TU (no C++23 module) so it links before the engine, exactly
// like its POSIX counterpart.

import enginec.ffi.api;

#include <cstdarg>
#include <cstdio>
#include <cstring>
#include <ctime>
#include <filesystem>
#include <system_error>

#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <windows.h>

namespace {

char g_dir[512] = {0};
char g_crash_path[640] = {0};
volatile LONG g_handling = 0;
LPTOP_LEVEL_EXCEPTION_FILTER g_previous_filter = nullptr;

void format_stamp(char* buffer, std::size_t length, bool file_name)
{
    SYSTEMTIME now;
    GetLocalTime(&now);
    if (file_name)
    {
        std::snprintf(buffer, length, "%04u%02u%02u-%02u%02u%02u", now.wYear, now.wMonth,
                      now.wDay, now.wHour, now.wMinute, now.wSecond);
    }
    else
    {
        std::snprintf(buffer, length, "%04u-%02u-%02u %02u:%02u:%02u", now.wYear, now.wMonth,
                      now.wDay, now.wHour, now.wMinute, now.wSecond);
    }
}

void emit_line(std::FILE* file, const char* format, ...)
{
    char buffer[512];
    va_list args;
    va_start(args, format);
    const int length = std::vsnprintf(buffer, sizeof(buffer) - 2, format, args);
    va_end(args);
    if (length <= 0)
    {
        return;
    }
    const std::size_t count = static_cast<std::size_t>(length) < sizeof(buffer) - 2
                                  ? static_cast<std::size_t>(length)
                                  : sizeof(buffer) - 2;
    buffer[count] = '\n';
    buffer[count + 1] = '\0';
    std::fwrite(buffer, 1, count + 1, file);
}

LONG WINAPI unhandled_filter(EXCEPTION_POINTERS* info)
{
    // A fault inside the filter must not recurse; fall through to the previous
    // filter (or the default termination) instead of hanging.
    if (InterlockedCompareExchange(&g_handling, 1, 0) != 0)
    {
        return g_previous_filter != nullptr ? g_previous_filter(info)
                                            : EXCEPTION_CONTINUE_SEARCH;
    }

    if (g_crash_path[0] != '\0')
    {
        if (std::FILE* file = std::fopen(g_crash_path, "ab"))
        {
            char stamp[64];
            format_stamp(stamp, sizeof(stamp), false);
            emit_line(file, "\n=== KizunaEngine native crash ===");
            emit_line(file, "time  : %s", stamp);
            EXCEPTION_RECORD* record = info != nullptr ? info->ExceptionRecord : nullptr;
            emit_line(file, "code  : 0x%08lx",
                      record != nullptr ? static_cast<unsigned long>(record->ExceptionCode)
                                        : 0ul);
            emit_line(file, "addr  : %p", record != nullptr ? record->ExceptionAddress : nullptr);
            emit_line(file, "pid   : %lu, tid %lu", GetCurrentProcessId(), GetCurrentThreadId());
            emit_line(file, "=== end of report ===");
            std::fflush(file);
            std::fclose(file);
        }
    }

    g_handling = 0;
    return g_previous_filter != nullptr ? g_previous_filter(info) : EXCEPTION_CONTINUE_SEARCH;
}

} // namespace

extern "C" __declspec(dllexport) void KzcEngineSetCrashLogDirectory(const char* dir)
{
    if (dir == nullptr || *dir == '\0')
    {
        return;
    }
    std::snprintf(g_dir, sizeof(g_dir), "%s", dir);

    // Normal context: create the directory now so the filter only has to append.
    try
    {
        std::error_code ec;
        std::filesystem::create_directories(g_dir, ec);
    }
    catch (...)
    {
    }

    char stamp[32];
    format_stamp(stamp, sizeof(stamp), true);
    std::snprintf(g_crash_path, sizeof(g_crash_path), "%s\\crash-%s-native.log", g_dir, stamp);

    if (std::FILE* file = std::fopen(g_crash_path, "ab"))
    {
        emit_line(file, "--- crash log session (pid %lu) ---", GetCurrentProcessId());
        std::fclose(file);
    }

    // Install once. The filter chains to whatever was there (CRT, a debugger, ...)
    // so a real crash still reaches the default handler after the report is written.
    if (g_previous_filter == nullptr)
    {
        g_previous_filter = SetUnhandledExceptionFilter(&unhandled_filter);
    }
}

extern "C" __declspec(dllexport) void KzcEngineCrumb(const char* message)
{
    const char* text = message != nullptr ? message : "";
    SYSTEMTIME now;
    GetLocalTime(&now);

    char line[640];
    const int length = std::snprintf(line, sizeof(line) - 1, "%02u:%02u:%02u %s\n", now.wHour,
                                     now.wMinute, now.wSecond, text);
    if (length <= 0)
    {
        return;
    }

    if (g_dir[0] != '\0')
    {
        char path[600];
        std::snprintf(path, sizeof(path), "%s\\engine-crumb.log", g_dir);
        if (std::FILE* file = std::fopen(path, "ab"))
        {
            std::fwrite(line, 1, static_cast<std::size_t>(length), file);
            std::fclose(file);
        }
    }
    std::fputs(line, stderr);
}
