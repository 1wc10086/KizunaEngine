# Host-only smoke tests. They do not need a device or a game: each test builds a
# synthetic XP3/KAG/TJS input, which is what makes them useful in CI while the
# Android APK only ships the release library.
add_executable(enginec_tests core_tests.cpp)
target_link_libraries(enginec_tests PRIVATE enginec_core)
enginec_apply_common_flags(enginec_tests)
add_test(NAME enginec_tests COMMAND enginec_tests)

# Drives the krkr module end to end with a synthetic .ks (no assets required).
add_executable(enginec_krkr_smoke krkr_smoke.cpp)
target_link_libraries(enginec_krkr_smoke PRIVATE enginec_core)
if(UNIX AND NOT APPLE)
    target_link_options(enginec_krkr_smoke PRIVATE -rdynamic)
endif()
enginec_apply_common_flags(enginec_krkr_smoke)
add_test(NAME enginec_krkr_smoke COMMAND enginec_krkr_smoke)

# Drives the client protocol through engine_core, not the engine directly.
add_executable(enginec_wire_smoke wire_smoke.cpp)
target_link_libraries(enginec_wire_smoke PRIVATE enginec_core)
enginec_apply_common_flags(enginec_wire_smoke)
add_test(NAME enginec_wire_smoke COMMAND enginec_wire_smoke)

# PSD reader: builds synthetic documents in memory and checks decoded pixels,
# masks, layer comps and the 16-bit layer table. The reader is a module
# (plugins/psdfile/psd.cppm), so the test imports it; the module units are
# compiled here too, which keeps this test dependent on zlib only instead of the
# whole TJS runtime.
add_executable(enginec_psd_tests krkr_psd_test.cpp)
# The reader is its own module target (plugins/psdfile/psd.cppm + three units), so
# the test imports it instead of compiling the parser itself: that is also what
# proves the exported API covers everything PSDPlugin.cpp needs.
target_link_libraries(enginec_psd_tests PRIVATE enginec_krkr_psd zlib-ng enginec::std)
enginec_apply_common_flags(enginec_psd_tests)
add_test(NAME enginec_psd_tests COMMAND enginec_psd_tests)
