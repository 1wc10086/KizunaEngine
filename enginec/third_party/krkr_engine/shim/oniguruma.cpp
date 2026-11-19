// PCRE2-backed implementation of the Oniguruma subset declared in oniguruma.h.
//
// The mapping is intentionally literal: byte offsets in = byte offsets out, which
// is what tjsRegExp.cpp relies on when it divides beg/end by sizeof(tjs_char).
// Patterns are always UTF-8 because TJS2 strings are UTF-8 in this port.
#include "oniguruma.h"

#include <pcre2.h>

#include <cstring>
#include <cstdio>
#include <cstdlib>
#include <string>
#include <vector>

struct OnigRegexType {
    pcre2_code* code = nullptr;
    std::uint32_t capture_count = 0;
};

OnigEncodingType OnigEncodingUTF8;
OnigSyntaxType OnigSyntaxPerl;

namespace {

thread_local std::string last_error;

int compile_failed(int code) {
    UChar buffer[ONIG_MAX_ERROR_MESSAGE_LEN]{};
    pcre2_get_error_message(code, buffer, sizeof(buffer));
    last_error.assign(reinterpret_cast<const char*>(buffer));
    return code;
}

} // namespace

extern "C" {

int onig_new(regex_t** reg, const UChar* pattern, const UChar* pattern_end,
             OnigOptionType option, const OnigEncodingType*, const OnigSyntaxType*,
             OnigErrorInfo* einfo) {
    if (reg == nullptr || pattern == nullptr || pattern_end == nullptr) {
        return -1;
    }
    std::uint32_t flags = PCRE2_UTF | PCRE2_UCP;
    if ((option & ONIG_OPTION_IGNORECASE) != 0) {
        flags |= PCRE2_CASELESS;
    }
    if ((option & ONIG_OPTION_MULTILINE) != 0) {
        flags |= PCRE2_MULTILINE;
    }
    if ((option & ONIG_OPTION_SINGLELINE) != 0) {
        flags |= PCRE2_DOTALL;
    }
    int error_code = 0;
    PCRE2_SIZE error_offset = 0;
    auto* code = pcre2_compile(pattern, static_cast<PCRE2_SIZE>(pattern_end - pattern), flags,
                               &error_code, &error_offset, nullptr);
    if (code == nullptr) {
        if (einfo != nullptr) {
            einfo->par = pattern + error_offset;
            einfo->par_end = pattern_end;
        }
        return compile_failed(error_code);
    }
    auto* regex = new OnigRegexType();
    regex->code = code;
    std::uint32_t capture = 0;
    pcre2_pattern_info(code, PCRE2_INFO_CAPTURECOUNT, &capture);
    regex->capture_count = capture;

    if (*reg != nullptr) {
        onig_free(*reg);
    }
    *reg = regex;
    return 0;
}

void onig_free(regex_t* reg) {
    if (reg == nullptr) {
        return;
    }
    if (reg->code != nullptr) {
        pcre2_code_free(reg->code);
    }
    delete reg;
}

int onig_search(const regex_t* reg, const UChar* str, const UChar* end,
                const UChar* start, const UChar*, OnigRegion* region, OnigOptionType) {
    if (reg == nullptr || reg->code == nullptr || str == nullptr || start == nullptr) {
        return ONIG_MISMATCH;
    }
    const auto length = static_cast<PCRE2_SIZE>(end - str);
    const auto offset = static_cast<PCRE2_SIZE>(start - str);
    if (offset > length) {
        return ONIG_MISMATCH;
    }

    pcre2_match_data* match = pcre2_match_data_create_from_pattern(reg->code, nullptr);
    if (match == nullptr) {
        return ONIG_MISMATCH;
    }
    const int result = pcre2_match(reg->code, str, length, offset, 0, match, nullptr);
    if (result < 0) {
        pcre2_match_data_free(match);
        if (result != PCRE2_ERROR_NOMATCH) {
            UChar buffer[ONIG_MAX_ERROR_MESSAGE_LEN]{};
            pcre2_get_error_message(result, buffer, sizeof(buffer));
            last_error.assign(reinterpret_cast<const char*>(buffer));
        }
        return ONIG_MISMATCH;
    }

    const std::uint32_t groups = reg->capture_count + 1;
    if (region != nullptr) {
        if (region->beg == nullptr || region->allocated < static_cast<int>(groups)) {
            std::free(region->beg);
            std::free(region->end);
            region->beg = static_cast<int*>(std::calloc(groups, sizeof(int)));
            region->end = static_cast<int*>(std::calloc(groups, sizeof(int)));
            region->allocated = static_cast<int>(groups);
        }
        region->num_regs = static_cast<int>(groups);
        PCRE2_SIZE* ovector = pcre2_get_ovector_pointer(match);
        for (std::uint32_t index = 0; index < groups; ++index) {
            const PCRE2_SIZE begin = ovector[index * 2];
            const PCRE2_SIZE finish = ovector[index * 2 + 1];
            if (begin == PCRE2_UNSET || finish == PCRE2_UNSET) {
                region->beg[index] = -1;
                region->end[index] = -1;
            } else {
                region->beg[index] = static_cast<int>(begin);
                region->end[index] = static_cast<int>(finish);
            }
        }
    }

    const int matched = static_cast<int>(pcre2_get_ovector_pointer(match)[0]);
    pcre2_match_data_free(match);
    return matched;
}

int onig_error_code_to_str(UChar* s, int err_code, OnigErrorInfo*) {
    if (s == nullptr) {
        return 0;
    }
    if (!last_error.empty()) {
        std::strcpy(reinterpret_cast<char*>(s), last_error.c_str());
        return static_cast<int>(last_error.size());
    }
    const int written = std::snprintf(reinterpret_cast<char*>(s), ONIG_MAX_ERROR_MESSAGE_LEN,
                                      "regex error %d", err_code);
    return written > 0 ? written : 0;
}

void onig_end(void) {}

OnigRegion* onig_region_new(void) {
    auto* region = static_cast<OnigRegion*>(std::calloc(1, sizeof(OnigRegion)));
    return region;
}

void onig_region_free(OnigRegion* region, int free_self) {
    if (region == nullptr) {
        return;
    }
    std::free(region->beg);
    std::free(region->end);
    region->beg = nullptr;
    region->end = nullptr;
    region->allocated = 0;
    region->num_regs = 0;
    if (free_self != 0) {
        std::free(region);
    }
}

void onig_region_clear(OnigRegion* region) {
    if (region == nullptr) {
        return;
    }
    for (int index = 0; index < region->allocated; ++index) {
        region->beg[index] = -1;
        region->end[index] = -1;
    }
    region->num_regs = 0;
}

} // extern "C"
