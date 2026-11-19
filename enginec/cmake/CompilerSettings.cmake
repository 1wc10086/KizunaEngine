# Compiler flags and the libc++ decision.
#
# The std module is built from *libc++*, so every translation unit has to compile
# against the same libc++ headers. On Linux clang defaults to libstdc++, which would
# produce two different `std` modules and an ABI mismatch that only shows up as heap
# corruption at runtime, so the switch is explicit.

option(ENGINEC_USE_LIBCXX
    "Compile against libc++ (required when the std module comes from libc++)" ON)

function(enginec_configure_libcxx)
    if(ANDROID OR MSVC OR NOT ENGINEC_USE_LIBCXX)
        return()
    endif()
    if(NOT CMAKE_CXX_COMPILER_ID MATCHES "Clang")
        return()
    endif()

    add_compile_options(-stdlib=libc++)
    add_link_options(-stdlib=libc++)
    # clang's driver already knows where libc++ lives on macOS and on distros that
    # install it as the default; only Linux needs the extra link names.
    if(UNIX AND NOT APPLE)
        link_libraries(c++ c++abi)
    endif()
endfunction()

# Must run before any target is created: add_compile_options only affects targets
# declared afterwards.
enginec_configure_libcxx()

# libc++'s std.cppm enables its POSIX thread support when the compiler is invoked
# with -pthread (it defines _REENTRANT). The NDK toolchain adds that flag to normal
# targets, so compiling std.cppm without it produces a BMI whose thread support is
# disabled and every `import std;` then fails with a configuration mismatch. Make
# the flag explicit and global so the module and its consumers agree.
if(CMAKE_CXX_COMPILER_ID MATCHES "Clang|GNU" AND NOT WIN32)
    add_compile_options(-pthread)
    add_link_options(-pthread)
endif()

function(enginec_apply_common_flags target)
    target_compile_features(${target} PUBLIC cxx_std_23)

    if(CMAKE_CXX_COMPILER_ID MATCHES "Clang|GNU")
        target_compile_options(${target} PRIVATE
            -fno-omit-frame-pointer
            -ffunction-sections
            -fdata-sections
            -fvisibility=hidden
            -fvisibility-inlines-hidden)
        # --gc-sections/--as-needed are ELF (GNU ld/lld) options; Apple's ld
        # rejects them. dead_strip is the Mach-O equivalent for the section part.
        if(APPLE)
            target_link_options(${target} PRIVATE -Wl,-dead_strip)
        else()
            target_link_options(${target} PRIVATE -Wl,--gc-sections -Wl,--as-needed)
        endif()
    endif()

    if(ENGINEC_WARNINGS AND CMAKE_CXX_COMPILER_ID MATCHES "Clang|GNU")
        target_compile_options(${target} PRIVATE -Wall -Wextra -Wno-unused-parameter)
    endif()

    if(ENGINEC_ENABLE_LTO)
        include(CheckIPOSupported)
        check_ipo_supported(RESULT _ipo_ok OUTPUT _ipo_error)
        if(_ipo_ok)
            set_property(TARGET ${target} PROPERTY INTERPROCEDURAL_OPTIMIZATION TRUE)
        else()
            message(WARNING "LTO unavailable: ${_ipo_error}")
        endif()
    endif()
endfunction()

# Vendored code is compiled with warnings off: a warning in TJS2 or libpng must
# never fail our build, and we cannot fix it here.
function(enginec_third_party_target target)
    # PCRE2 exports pcre2-8 as an alias of pcre2-8-static when BUILD_SHARED_LIBS is
    # OFF; set_target_properties rejects alias targets, so resolve to the real one.
    get_target_property(_enginec_alias ${target} ALIASED_TARGET)
    if(_enginec_alias)
        set(target ${_enginec_alias})
    endif()
    set_target_properties(${target} PROPERTIES POSITION_INDEPENDENT_CODE ON)
    target_compile_options(${target} PRIVATE
        $<$<C_COMPILER_ID:Clang,GNU>:-w>
        $<$<CXX_COMPILER_ID:Clang,GNU>:-w>)
endfunction()

# Codecs that reinterpret differently typed pointers (libjpeg, TJS2's octet paths)
# rely on that working; the optimiser must not assume strict aliasing.
function(enginec_third_party_no_strict_aliasing target)
    get_target_property(_enginec_alias ${target} ALIASED_TARGET)
    if(_enginec_alias)
        set(target ${_enginec_alias})
    endif()
    target_compile_options(${target} PRIVATE
        $<$<C_COMPILER_ID:Clang,GNU>:-fno-strict-aliasing>
        $<$<CXX_COMPILER_ID:Clang,GNU>:-fno-strict-aliasing>)
endfunction()
