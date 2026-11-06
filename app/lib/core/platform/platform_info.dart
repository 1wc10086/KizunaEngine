import 'package:flutter/foundation.dart';

bool get isAndroidPlatform => defaultTargetPlatform == TargetPlatform.android;
bool get isIOSPlatform => defaultTargetPlatform == TargetPlatform.iOS;
bool get isMacOSPlatform => defaultTargetPlatform == TargetPlatform.macOS;
bool get isWindowsPlatform => defaultTargetPlatform == TargetPlatform.windows;
bool get isLinuxPlatform => defaultTargetPlatform == TargetPlatform.linux;

/// 需要处理屏幕方向/系统 UI 的平台。桌面窗口没有“方向”概念，
/// 调 SystemChrome.setPreferredOrientations 只会得到无意义的结果。
bool get isMobilePlatform => isAndroidPlatform || isIOSPlatform;

bool get isDesktopPlatform =>
    isWindowsPlatform || isMacOSPlatform || isLinuxPlatform;
