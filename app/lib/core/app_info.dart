/// 应用版本。
///
/// CI 用 `--dart-define=KIZUNA_VERSION=<pubspec version>` 注入，保证应用内
/// 更新比较的是真实构建版本；本地 `flutter run` 没有传参时回落到 1.0.0。
/// 不用 package_info_plus 是为了少一个原生插件，版本号在构建期就固定了。
const String kAppVersion =
    String.fromEnvironment('KIZUNA_VERSION', defaultValue: '1.0.0');
