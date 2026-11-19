/// 应用版本（X.Y.Z，不含 build 号）。
///
/// `KIZUNA_VERSION` 是 Dart 侧**唯一**的构建期版本输入，不另设常量：
/// CI 从 `app/pubspec.yaml` 的 `version:` 行取 X.Y.Z，再
/// `--dart-define=KIZUNA_VERSION=<name>` 注入；`app/pubspec.yaml` 因此是全仓
/// 唯一的版本来源。应用内更新比较、关于页与启动日志都用这个值。
///
/// 本地 `flutter run` 没有传 `--dart-define` 时回落到下面的默认值，只用于
/// 开发期显示，不代表真实构建版本。不用 package_info_plus 是为了少一个
/// 原生插件，版本号在构建期就固定了。
const String kAppVersion =
    String.fromEnvironment('KIZUNA_VERSION', defaultValue: '1.0.0');
