// PCRE2 wrapper implementation.
//
// Three things this layer has to get right, all of them visible to TJS scripts:
//  * Patterns and subjects are UTF-8. TJS2 strings come from CP932-shifted scripts
//    and can contain bytes that are not valid UTF-8, so the pattern is compiled with
//    PCRE2_MATCH_INVALID_UTF: an invalid sequence then acts as a barrier instead of
//    turning every later match on that string into an error.
//  * Byte offsets are handed back unchanged; tjsRegExp.cpp divides them by
//    sizeof(tjs_char), so converting to character offsets here would break it.
//  * A pathological pattern must not hang the UI thread, so every match runs with
//    explicit match/depth limits, and the JIT uses a bounded stack.

module;

#define PCRE2_CODE_UNIT_WIDTH 8
#include <pcre2.h>

module enginec.third_party.pcre2;

import std;

namespace enginec::third_party::pcre2
{

namespace
{

// The engine's strings are short KAG lines; 10M backtracking steps is far beyond
// anything legitimate and still finishes in well under a frame on a phone.
inline constexpr std::uint32_t match_limit = 10'000'000u;
inline constexpr std::uint32_t depth_limit = 10'000u;
// PCRE2's default JIT stack is 32 KiB, which complex real-world patterns exhaust
// (PCRE2_ERROR_JIT_STACKLIMIT) on long lines.
inline constexpr std::size_t jit_stack_start = 64 * 1024;
inline constexpr std::size_t jit_stack_max = 1024 * 1024;

std::size_t unmatched() noexcept
{
    // The header documents "-1/-1 for an unmatched group". size_t cannot hold -1,
    // and sign-extending the pattern is what the shim's int conversion expects.
    return static_cast<std::size_t>(-1);
}

std::string describe(int code)
{
    PCRE2_UCHAR buffer[256]{};
    const int written = pcre2_get_error_message(code, buffer, sizeof(buffer));
    if (written > 0)
    {
        // The buffer is only NUL-terminated when it fits, so the returned length is
        // authoritative; treating it as a C string would read past a truncated one.
        return std::string(reinterpret_cast<const char*>(buffer),
                           static_cast<std::size_t>(written));
    }
    return "PCRE2 error " + std::to_string(code);
}

} // namespace

struct regex::impl
{
    pcre2_code* code = nullptr;
    pcre2_match_context* context = nullptr;
    pcre2_jit_stack* stack = nullptr;
    std::uint32_t captures = 0;
    // PCRE2 requires a JIT stack to be used by one match at a time, and this object
    // is shared between the engine thread and the FFI caller.
    mutable std::mutex lock;

    impl() = default;
    impl(const impl&) = delete;
    impl& operator=(const impl&) = delete;

    ~impl()
    {
        reset();
    }

    void reset() noexcept
    {
        if (stack != nullptr)
        {
            pcre2_jit_stack_free(stack);
            stack = nullptr;
        }
        if (context != nullptr)
        {
            pcre2_match_context_free(context);
            context = nullptr;
        }
        if (code != nullptr)
        {
            pcre2_code_free(code);
            code = nullptr;
        }
        captures = 0;
    }
};

regex::regex() : impl_(std::make_unique<impl>()) {}

regex::~regex() = default;

regex::regex(regex&&) noexcept = default;

regex& regex::operator=(regex&&) noexcept = default;

bool regex::compile(std::string_view pattern, std::uint32_t options, std::string& error)
{
    error.clear();
    // A moved-from regex has a null impl_; recreating it keeps compile usable,
    // which is what the Oniguruma shim expects after it moves a regex into place.
    if (impl_ == nullptr)
    {
        impl_ = std::make_unique<impl>();
    }

    // Recompiling frees the code, context and JIT stack that a concurrent find() may
    // be using, so the same lock as find() has to cover this.
    std::lock_guard<std::mutex> serialise(impl_->lock);
    impl_->reset();

    std::uint32_t flags = PCRE2_UTF | PCRE2_UCP | PCRE2_MATCH_INVALID_UTF;
    if ((options & option_ignore_case) != 0)
    {
        flags |= PCRE2_CASELESS;
    }
    if ((options & option_multiline) != 0)
    {
        flags |= PCRE2_MULTILINE;
    }
    if ((options & option_dotall) != 0)
    {
        flags |= PCRE2_DOTALL;
    }

    int code = 0;
    PCRE2_SIZE offset = 0;
    impl_->code = pcre2_compile(reinterpret_cast<PCRE2_SPTR>(pattern.data()),
                                static_cast<PCRE2_SIZE>(pattern.size()), flags, &code, &offset,
                                nullptr);
    if (impl_->code == nullptr)
    {
        error = "regular expression error at offset " + std::to_string(offset) + ": " +
                describe(code);
        return false;
    }

    std::uint32_t captures = 0;
    pcre2_pattern_info(impl_->code, PCRE2_INFO_CAPTURECOUNT, &captures);
    impl_->captures = captures;

    // JIT is an optimisation only: an unsupported pattern item (or a build without
    // JIT) leaves PCRE2_ERROR_JIT_BADOPTION here, and pcre2_match then interprets.
    pcre2_jit_compile(impl_->code, PCRE2_JIT_COMPLETE);

    impl_->context = pcre2_match_context_create(nullptr);
    if (impl_->context != nullptr)
    {
        pcre2_set_match_limit(impl_->context, match_limit);
        pcre2_set_depth_limit(impl_->context, depth_limit);

        impl_->stack = pcre2_jit_stack_create(jit_stack_start, jit_stack_max, nullptr);
        if (impl_->stack != nullptr)
        {
            pcre2_jit_stack_assign(impl_->context, nullptr, impl_->stack);
        }
    }
    return true;
}

std::optional<std::pair<std::size_t, std::size_t>> regex::find(std::string_view subject,
                                                               std::size_t start,
                                                               std::size_t group,
                                                               std::string& error) const
{
    error.clear();

    if (impl_ == nullptr || impl_->code == nullptr)
    {
        error = "regular expression has not been compiled";
        return std::nullopt;
    }
    if (group > impl_->captures)
    {
        error = "capture group " + std::to_string(group) + " does not exist";
        return std::nullopt;
    }
    if (start > subject.size())
    {
        error = "start offset is past the end of the subject";
        return std::nullopt;
    }

    // Callers pass offsets produced by earlier matches, but TJS also supports
    // lastIndex values a script computed by hand, which may land inside a
    // multi-byte character. Starting there is an error in UTF mode, so step to the
    // next character boundary instead of failing the match.
    while (start < subject.size() &&
           (static_cast<std::uint8_t>(subject[start]) & 0xC0u) == 0x80u)
    {
        ++start;
    }

    pcre2_match_data* data = pcre2_match_data_create_from_pattern(impl_->code, nullptr);
    if (data == nullptr)
    {
        error = "out of memory while matching";
        return std::nullopt;
    }
    const auto release = [](pcre2_match_data* value) { pcre2_match_data_free(value); };
    std::unique_ptr<pcre2_match_data, decltype(release)> guard(data, release);

    std::lock_guard<std::mutex> serialise(impl_->lock);

    const int result = pcre2_match(impl_->code, reinterpret_cast<PCRE2_SPTR>(subject.data()),
                                   static_cast<PCRE2_SIZE>(subject.size()),
                                   static_cast<PCRE2_SIZE>(start), 0, data, impl_->context);
    if (result == PCRE2_ERROR_NOMATCH)
    {
        return std::nullopt;
    }
    if (result < 0)
    {
        error = describe(result);
        return std::nullopt;
    }

    // pcre2_get_ovector_count is capacity, which can exceed the pattern's capture
    // count when the match data came from a pattern with more groups; checking
    // `group` against the stored count above already ruled that out.
    if (group >= static_cast<std::size_t>(pcre2_get_ovector_count(data)))
    {
        return std::make_pair(unmatched(), unmatched());
    }

    const PCRE2_SIZE* ovector = pcre2_get_ovector_pointer(data);
    const PCRE2_SIZE begin = ovector[group * 2];
    const PCRE2_SIZE end = ovector[group * 2 + 1];
    if (begin == PCRE2_UNSET || end == PCRE2_UNSET)
    {
        return std::make_pair(unmatched(), unmatched());
    }
    return std::make_pair(static_cast<std::size_t>(begin), static_cast<std::size_t>(end));
}

int regex::group_count() const noexcept
{
    return impl_ == nullptr ? 0 : static_cast<int>(impl_->captures);
}

bool regex::valid() const noexcept
{
    return impl_ != nullptr && impl_->code != nullptr;
}

} // namespace enginec::third_party::pcre2
