# `import std;` needs a standard library module. The NDK and a homebrew/apt libc++
# both ship std.cppm next to the compiler; compiling it once into a static library is
# what LiarUtil.Flutter does and keeps every module unit free of standard headers.

set(ENGINEC_STD_MODULE_SOURCE "" CACHE FILEPATH
    "libc++ std module source (std.cppm); auto-detected next to the compiler when empty")

if(ANDROID)
    # Bionic exposes ctype macros and fortify wrappers in its headers; the std module
    # is one big translation unit, and those macros break libc++'s inline definitions.
    set(_enginec_std_flags
        "-D__BIONIC_CTYPE_INLINE=inline;-U_FORTIFY_SOURCE;-D_FORTIFY_SOURCE=0")
else()
    set(_enginec_std_flags "")
endif()
set(ENGINEC_STD_MODULE_FLAGS "${_enginec_std_flags}" CACHE STRING
    "Extra compile options applied to the std module only")

function(enginec_configure_std_module)
    set(source "${ENGINEC_STD_MODULE_SOURCE}")
    if(MSVC)
        # MSVC ships its standard module as <VS>/VC/Tools/MSVC/<ver>/modules/std.ixx.
        # Compiling it as an ordinary CXX_MODULES file set (the same route as the
        # libc++ std.cppm) works with Ninja and avoids CMAKE_CXX_MODULE_STD's
        # toolchain-probe requirements. The workflow passes its path explicitly.
        if(NOT source OR NOT EXISTS "${source}")
            message(FATAL_ERROR
                "MSVC needs -DENGINEC_STD_MODULE_SOURCE=<path to std.ixx>")
        endif()
        get_filename_component(_dir "${source}" DIRECTORY)
        message(STATUS "std module source: ${source}")
        add_library(enginec_std STATIC)
        add_library(enginec::std ALIAS enginec_std)
        target_sources(enginec_std PUBLIC FILE_SET CXX_MODULES
            BASE_DIRS "${_dir}"
            FILES "${source}")
        target_compile_features(enginec_std PUBLIC cxx_std_23)
        return()
    endif()
    set(source "${ENGINEC_STD_MODULE_SOURCE}")
    if(NOT source)
        get_filename_component(_bin "${CMAKE_CXX_COMPILER}" DIRECTORY)
        get_filename_component(_root "${_bin}" DIRECTORY)
        foreach(candidate
                "${_root}/share/libc++/v1/std.cppm"
                "${_root}/libcxx/modules/std.cppm"
                "${_root}/../share/libc++/v1/std.cppm"
                "${_bin}/../share/libc++/v1/std.cppm")
            if(EXISTS "${candidate}")
                set(source "${candidate}")
                break()
            endif()
        endforeach()
        # Debian/Ubuntu install clang to /usr/bin but keep libc++'s module under
        # /usr/lib/llvm-<version>/; pick the highest installed one.
        if(NOT source)
            file(GLOB _enginec_llvm_dirs "${_root}/lib/llvm-*")
            list(SORT _enginec_llvm_dirs)
            list(REVERSE _enginec_llvm_dirs)
            foreach(_llvm_dir IN LISTS _enginec_llvm_dirs)
                if(EXISTS "${_llvm_dir}/share/libc++/v1/std.cppm")
                    set(source "${_llvm_dir}/share/libc++/v1/std.cppm")
                    break()
                endif()
            endforeach()
        endif()
    endif()

    if(NOT source OR NOT EXISTS "${source}")
        message(FATAL_ERROR
            "no libc++ std module found next to ${CMAKE_CXX_COMPILER}; pass "
            "-DENGINEC_STD_MODULE_SOURCE=<path to std.cppm>")
    endif()

    get_filename_component(_dir "${source}" DIRECTORY)
    message(STATUS "std module source: ${source}")

    add_library(enginec_std STATIC)
    add_library(enginec::std ALIAS enginec_std)
    target_sources(enginec_std PUBLIC FILE_SET CXX_MODULES
        BASE_DIRS "${_dir}"
        FILES "${source}")
    target_compile_options(enginec_std PRIVATE ${ENGINEC_STD_MODULE_FLAGS})
endfunction()
