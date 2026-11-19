# Build switches shared by every target. Everything that changes the shape of the
# artefact is a cache variable so CI can pin it without editing files.

if(ANDROID)
    set(_enginec_tests_default OFF) # the device cannot run host executables
else()
    set(_enginec_tests_default ON)
endif()

option(ENGINEC_BUILD_TESTS "Build the host test executable" ${_enginec_tests_default})
option(ENGINEC_ENABLE_LTO "Enable interprocedural optimisation" OFF)
option(ENGINEC_WARNINGS "Enable extended compiler warnings" ON)
option(ENGINEC_BUILD_SHARED
    "Build libenginec as a shared library (needed for the Flutter FFI binding)"
    ON)

# The Flutter build consumes the native library through jniLibs. Pointing this at
# app/android/app/src/main/jniLibs lets `cmake --build ... --target flutter_install`
# place the .so where Gradle expects it, so CI never hard-codes an ABI path.
set(ENGINEC_FLUTTER_JNILIBS "" CACHE PATH
    "jniLibs directory that receives <abi>/libenginec.so")

# Compiled into the C ABI version string and shown in the app's about dialog, so an
# installed APK whose native core is stale is immediately visible.
set(ENGINEC_VERSION "${PROJECT_VERSION}" CACHE STRING "Reported core version")
