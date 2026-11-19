// PCRE2 wrapper.
//
// TJS2's native RegExp class used Oniguruma. This project replaces it with PCRE2
// (actively maintained, JIT, UTF-8) and keeps the original TJS surface unchanged:
// the shim under third_party/pcre2_shim implements the small Oniguruma API subset
// tjsRegExp.cpp calls on top of this wrapper's C functions.
export module enginec.third_party.pcre2;

import std;

export namespace enginec::third_party::pcre2 {

inline constexpr std::uint32_t option_none = 0;
inline constexpr std::uint32_t option_ignore_case = 1u;
inline constexpr std::uint32_t option_multiline = 2u;
inline constexpr std::uint32_t option_dotall = 4u;
inline constexpr std::uint32_t option_utf = 8u;

class regex {
public:
    regex();
    ~regex();
    regex(const regex&) = delete;
    regex& operator=(const regex&) = delete;
    regex(regex&&) noexcept;
    regex& operator=(regex&&) noexcept;

    bool compile(std::string_view pattern, std::uint32_t options, std::string& error);
    // Returns [start,end) byte offsets of group `index`; -1/-1 when unmatched.
    std::optional<std::pair<std::size_t, std::size_t>> find(std::string_view subject,
                                                           std::size_t start,
                                                           std::size_t group,
                                                           std::string& error) const;
    [[nodiscard]] int group_count() const noexcept;
    [[nodiscard]] bool valid() const noexcept;

private:
    struct impl;
    std::unique_ptr<impl> impl_;
};

} // namespace enginec::third_party::pcre2
