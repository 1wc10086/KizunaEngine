import 'package:url_launcher/url_launcher.dart';

/// 在系统浏览器里打开链接。
///
/// 之前这类跳转走 `kizuna/installer` 平台通道，但那条通道只在 Android 的
/// MainActivity 里注册；iOS/桌面调用会抛 MissingPluginException，表现为
/// “关于”页的仓库、反馈按钮全部无效。url_launcher 由各平台官方实现，
/// 统一从 externalApplication 打开，避免应用内 WebView 与登录态纠缠。
Future<bool> openExternalUrl(String url) async {
  final uri = Uri.tryParse(url);
  if (uri == null || !uri.hasScheme) {
    return false;
  }
  try {
    return await launchUrl(uri, mode: LaunchMode.externalApplication);
  } catch (_) {
    return false;
  }
}
