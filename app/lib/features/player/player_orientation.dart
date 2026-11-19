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

/// 播放页进入时的固定姿态：横屏 + 沉浸式。
///
/// 需求是"游玩时整个界面都是游戏画面，且默认直接横屏"，所以这里在页面
/// initState 就切，不等布局算完——晚一帧用户就会先看到一帧竖屏再被掰过去。
Future<void> enterPlayerLandscape() => enterLandscape();

/// 离开播放页时恢复竖屏 + 边到边。
///
/// 桌面/平板允许横屏常驻，但手机上的外壳是竖屏设计：不恢复的话，回到游戏库
/// 会以横屏呈现，卡片会被压扁。
Future<void> leavePlayer() => enterPortrait();

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
