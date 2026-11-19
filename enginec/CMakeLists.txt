cmake_minimum_required(VERSION 3.28)

# KizunaEngine native core (`enginec`).
#
# Structure and build rules deliberately mirror LiarUtil.Flutter: the tree is split
# into include/ (feature contracts), src/ (feature implementations), utility/
# (shared helpers), third_party/ (the *only* place a non-std header is included) and
# ffi/ (the C ABI the Flutter bridge opens). Every translation unit except the
# vendored third_party tree speaks `import std;`; third_party wraps a C library once
# and everything above it only imports the wrapper module.
#
# Why the strict split matters here: TJS2, XP3, PNG/JPEG/WebP and PCRE2 are all
# pre-existing C/C++ code with their own macros (`TJS_N`, `Bytef`, `pcre2_*`). If a
# feature module included them directly, those macros would leak into our public
# surface and a library upgrade could silently break an unrelated module. Keeping
# them in third_party means one file changes per library, not ten.
project(KizunaEngineC
    VERSION 1.0.0
    DESCRIPTION "KiriKiri (krkr) core with a Flutter front end"
    LANGUAGES C CXX)

# Apple's platform files are Objective-C++ (NSBundle/UIKit/Metal).
if(APPLE)
    enable_language(OBJCXX)
endif()

list(APPEND CMAKE_MODULE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/cmake")

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
# CMake must scan every translation unit for module dependencies; without this the
# imported modules are invisible to the ninja dyndep graph and builds fail with
# "module not found" depending on file order.
set(CMAKE_CXX_SCAN_FOR_MODULES ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

if(NOT CMAKE_BUILD_TYPE AND NOT CMAKE_CONFIGURATION_TYPES)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "Build type" FORCE)
endif()

include(ProjectOptions)
include(StdModule)
include(CompilerSettings)
include(Dependencies)

enginec_configure_std_module()

add_subdirectory(third_party)
add_subdirectory(utility)
add_subdirectory(src)
add_subdirectory(ffi)

if(ENGINEC_BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()
