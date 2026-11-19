# The C++23 core: every module interface plus its implementation units.
#
# Module entities carry module linkage and hidden visibility, so they cannot cross a
# shared-library boundary. This target is therefore always static; the shared object
# the Flutter app loads (enginec_ffi) is built separately and re-exports only the C
# ABI. Same split as LiarUtil.Flutter.

set(ENGINEC_INTERFACE_UNITS
    "${CMAKE_SOURCE_DIR}/include/enginec.cppm"
    "${CMAKE_SOURCE_DIR}/include/enginec/feature/wire.cppm"
    "${CMAKE_SOURCE_DIR}/include/enginec/feature/engine.cppm"
    "${CMAKE_SOURCE_DIR}/utility/include/enginec/utility/error.cppm"
    "${CMAKE_SOURCE_DIR}/utility/include/enginec/utility/bytes.cppm"
    "${CMAKE_SOURCE_DIR}/utility/include/enginec/utility/text.cppm"
    "${CMAKE_SOURCE_DIR}/utility/include/enginec/utility/files.cppm"
    "${CMAKE_SOURCE_DIR}/utility/include/enginec/utility/log.cppm"
    "${CMAKE_SOURCE_DIR}/utility/include/enginec/utility/hash.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/zlib.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/png.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/jpeg.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/webp.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/pcre2.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/minizip.cppm"
    "${CMAKE_SOURCE_DIR}/third_party/include/enginec/third_party/krkr.cppm")

set(ENGINEC_IMPLEMENTATION_UNITS
    "${CMAKE_SOURCE_DIR}/third_party/src/zlib.cpp"
    "${CMAKE_SOURCE_DIR}/third_party/src/png.cpp"
    "${CMAKE_SOURCE_DIR}/third_party/src/jpeg.cpp"
    "${CMAKE_SOURCE_DIR}/third_party/src/webp.cpp"
    "${CMAKE_SOURCE_DIR}/third_party/src/pcre2.cpp"
    "${CMAKE_SOURCE_DIR}/third_party/src/minizip.cpp"
    "${CMAKE_SOURCE_DIR}/third_party/src/krkr.cpp"
    "${CMAKE_SOURCE_DIR}/utility/src/text.cpp"
    "${CMAKE_SOURCE_DIR}/utility/src/files.cpp"
    "${CMAKE_SOURCE_DIR}/utility/src/log.cpp"
    "${CMAKE_SOURCE_DIR}/utility/src/hash.cpp"
    "${CMAKE_SOURCE_DIR}/src/feature/wire.cpp"
    "${CMAKE_SOURCE_DIR}/src/feature/engine.cpp")

add_library(enginec_core STATIC)
add_library(enginec::core ALIAS enginec_core)

target_sources(enginec_core
    PUBLIC FILE_SET CXX_MODULES
        BASE_DIRS
            "${CMAKE_SOURCE_DIR}/include"
            "${CMAKE_SOURCE_DIR}/utility/include"
            "${CMAKE_SOURCE_DIR}/third_party/include"
        FILES ${ENGINEC_INTERFACE_UNITS})

target_sources(enginec_core PRIVATE ${ENGINEC_IMPLEMENTATION_UNITS})

# std is part of the public surface (every module speaks std:: types). The native
# libraries and the KiriKiri port stay private: only third_party/src/krkr*.cpp
# includes their headers, so consumers see the wrapper module and nothing else.
target_link_libraries(enginec_core
    PUBLIC enginec::std
    PRIVATE
        # Whole-archive: NCB plugins and TJS native classes register through
        # static constructors and define no symbol the linker can see. A plain
        # archive link drops those objects and every plugin silently disappears.
        "$<LINK_LIBRARY:WHOLE_ARCHIVE,enginec_krkr_engine>"
        enginec_png
        enginec_minizip
        zlib-ng
        enginec_jpeg
        webp
        pcre2-8)

set_target_properties(enginec_core PROPERTIES
    CXX_VISIBILITY_PRESET hidden
    VISIBILITY_INLINES_HIDDEN ON)

enginec_apply_common_flags(enginec_core)
