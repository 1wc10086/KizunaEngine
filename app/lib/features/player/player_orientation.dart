import 'package:flutter/services.dart';

Future<void> enterLandscape() async {
  await _apply(
    const <DeviceOrientation>[
      DeviceOrientation.landscapeLeft,
      DeviceOrientation.landscapeRight,
    ],
    SystemUiMode.immersiveSticky,
  );
}

Future<void> enterPortrait() async {
  await _apply(
    const <DeviceOrientation>[
      DeviceOrientation.portraitUp,
    ],
    SystemUiMode.edgeToEdge,
  );
}

Future<void> restoreSystemUi() async {
  try {
    await SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  } catch (_) {}
}

Future<void> _apply(
  List<DeviceOrientation> orientations,
  SystemUiMode mode,
) async {
  try {
    await SystemChrome.setPreferredOrientations(orientations);
  } catch (_) {}
  try {
    await SystemChrome.setEnabledSystemUIMode(mode);
  } catch (_) {}
}
