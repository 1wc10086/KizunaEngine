// Oniguruma API subset implemented on top of PCRE2.
//
// TJS2's native RegExp class is written against Oniguruma. The project replaces the
// regex library with PCRE2 (maintained, JIT, UTF-8); this header keeps tjsRegExp.cpp
// byte-for-byte unchanged so the TJS-visible semantics (lastIndex, global search,
// split/replace with callbacks) stay identical. Only the names and flags that file
// actually uses are provided — this is not a general Oniguruma compatibility layer.
#pragma once

#include <cstddef>
#include <cstdint>

#ifdef __cplusplus
extern "C" {
#endif

typedef unsigned char UChar;
typedef unsigned int OnigOptionType;
typedef struct OnigRegexType regex_t;
typedef struct OnigRegion OnigRegion;
typedef struct OnigEncodingType { int dummy; } OnigEncodingType;
typedef struct OnigSyntaxType { int dummy; } OnigSyntaxType;
typedef struct OnigErrorInfo {
    const UChar* par;
    const UChar* par_end;
} OnigErrorInfo;

struct OnigRegion {
    int allocated;
    int num_regs;
    int* beg;
    int* end;
    const UChar* history_root;
};

enum {
    ONIG_OPTION_NONE = 0,
    ONIG_OPTION_IGNORECASE = 1,
    ONIG_OPTION_EXTEND = 2,
    ONIG_OPTION_MULTILINE = 4,
    ONIG_OPTION_SINGLELINE = 8,
    ONIG_OPTION_FIND_LONGEST = 16,
    ONIG_OPTION_FIND_NOT_EMPTY = 32,
    ONIG_OPTION_NEGATE_SINGLELINE = 64,
    ONIG_OPTION_DONT_CAPTURE_GROUP = 128,
    ONIG_OPTION_CAPTURE_GROUP = 256,
    ONIG_OPTION_MAXBIT = 1 << 20
};

#define ONIG_OPTION_DEFAULT ONIG_OPTION_NONE
#define ONIG_MISMATCH (-1)
#define ONIG_MAX_ERROR_MESSAGE_LEN 90

extern OnigEncodingType OnigEncodingUTF8;
extern OnigSyntaxType OnigSyntaxPerl;
#define ONIG_ENCODING_UTF8 (&OnigEncodingUTF8)
#define ONIG_SYNTAX_PERL (&OnigSyntaxPerl)

int onig_new(regex_t** reg, const UChar* pattern, const UChar* pattern_end,
             OnigOptionType option, const OnigEncodingType* enc,
             const OnigSyntaxType* syntax, OnigErrorInfo* einfo);
void onig_free(regex_t* reg);
int onig_search(const regex_t* reg, const UChar* str, const UChar* end,
                const UChar* start, const UChar* range, OnigRegion* region,
                OnigOptionType option);
int onig_error_code_to_str(UChar* s, int err_code, OnigErrorInfo* einfo);
void onig_end(void);

OnigRegion* onig_region_new(void);
void onig_region_free(OnigRegion* region, int free_self);
void onig_region_clear(OnigRegion* region);

#ifdef __cplusplus
}
#endif
