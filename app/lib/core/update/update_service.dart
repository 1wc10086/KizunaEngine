import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';

/// 一条可用的更新信息。
///
/// [downloadUrl] 是 Android 可直接下载的 APK 链接（按当前 ABI 选定）；
/// 其他平台不给 APK——[pageUrl] 才是发布页，桌面/iOS 点“下载”时用浏览器打开它。
/// 这样既不会把安卓包推给桌面用户，也保留了“只有 Android 应用内下载”的约束。
class ReleaseInfo {
  final String version;
  final String title;
  final String notes;
  final String downloadUrl;
  final String pageUrl;
  final DateTime publishedAt;
  final String tag;

  const ReleaseInfo({
    required this.version,
    required this.title,
    required this.notes,
    required this.downloadUrl,
    required this.pageUrl,
    required this.publishedAt,
    required this.tag,
  });
}

class UpdateService {
  /// 更新发布仓库（对终端用户公开）。
  ///
  /// 注意它和 CI 构建仓库不是同一个：`messiah18envious/expert-journey`
  /// 只用来跑 GitHub Actions，仓库本身是私有的，匿名设备读不到 release；
  /// 应用内的检查/下载必须指向公开的发布仓库，否则所有人都会 403/404。
  static const String repositorySlug = '1wc10086/KizunaEngine';
  static const String repositoryUrl = 'https://github.com/$repositorySlug';
  static const String issuesUrl = '$repositoryUrl/issues';
  static const String latestReleaseUrl = '$repositoryUrl/releases/latest';

  static const String _userAgent = 'KizunaEngine';
  static const String _acceptHeader = 'application/vnd.github+json';
  static const String _releasesApiUrl =
      'https://api.github.com/repos/$repositorySlug/releases?per_page=20';
  static const String _atomFeedUrl =
      'https://github.com/$repositorySlug/releases.atom';

  static const Map<String, Object?> _apiHeaders = <String, Object?>{
    'Accept': _acceptHeader,
    'User-Agent': _userAgent,
  };

  static const Map<String, Object?> _downloadHeaders = <String, Object?>{
    'User-Agent': _userAgent,
  };

  /// GitHub 直链在部分网络环境下会超时/重置。Android 的应用内下载按顺序
  /// 在原始链接前加镜像前缀重试；桌面/iOS 仍然直接打开浏览器，避免把镜像
  /// 的可用性绑死在应用里。
  static const List<String> githubMirrors = <String>[
    'https://gh-proxy.org/',
    'https://v4.gh-proxy.org/',
    'https://v6.gh-proxy.org/',
    'https://cdn.gh-proxy.org/',
  ];

  static List<String> downloadCandidates(String url) {
    if (url.isEmpty) {
      return const <String>[];
    }
    return <String>[
      url,
      for (final mirror in githubMirrors) '$mirror$url',
    ];
  }

  /// 旧版资产名里带版本号（KizunaEngine-1.2.3.apk），保留这个正则兼容旧 release。
  static final RegExp _assetVersionRegex = RegExp(
      r'KizunaEngine-(\d+\.\d+\.\d+)',
      caseSensitive: false);

  /// release 名称 / tag 里的版本号：`KizunaEngine v1.2.3`、`v1.2.3` 都能命中。
  /// 前后用“非数字”边界，避免把 11.2.3 截成 1.2.3。
  static final RegExp _versionInText = RegExp(
      r'(?:^|[^0-9])v?(\d+\.\d+\.\d+)(?![0-9])',
      caseSensitive: false);

  static final RegExp _atomEntryRegex = RegExp(
      r'<entry\b[^>]*>(.*?)</entry>',
      caseSensitive: false,
      dotAll: true);
  static final RegExp _atomTitleRegex = RegExp(
      r'<title[^>]*>(.*?)</title>',
      caseSensitive: false,
      dotAll: true);
  static final RegExp _atomLinkRegex = RegExp(
      r'<link\b[^>]*href="([^"]+)"',
      caseSensitive: false);
  static final RegExp _tagStripRegex = RegExp(r'<[^>]+>');

  /// ABI 标签到资产名分段的别名表。MainActivity.abiTag() 只会返回
  /// arm64 / arm32 / x64，但 release 里的资产可能写成 armeabi-v7a、x86_64
  /// 这类 NDK 名称，按顺序都认。
  static const Map<String, List<String>> _abiAliases = <String, List<String>>{
    'arm64': <String>['arm64', 'aarch64'],
    'arm32': <String>['arm32', 'armeabi-v7a', 'armv7'],
    'x64': <String>['x64', 'x86_64', 'amd64'],
  };

  final Dio _dio;

  UpdateService({Dio? dio})
      : _dio = dio ??
            Dio(BaseOptions(
              connectTimeout: const Duration(seconds: 20),
              receiveTimeout: const Duration(seconds: 60),
            ));

  /// 查询更新。
  ///
  /// [abi] 只在 Android 传入（arm64/arm32/x64）：它既决定挑哪个 APK，
  /// 也决定要不要资产列表。其他平台传 null，版本号从 release 名称/tag 读，
  /// 不依赖任何 APK 资产。
  Future<ReleaseInfo?> check({
    required String installedVersion,
    String? abi,
  }) async {
    final installed = parseVersion(installedVersion);
    ReleaseInfo? fromApi;
    try {
      fromApi = await _fetchFromApi(installed, abi);
    } catch (_) {
      fromApi = null;
    }
    if (fromApi != null) {
      return fromApi;
    }
    try {
      return await _fetchFromAtom(installed, abi);
    } catch (_) {
      return null;
    }
  }

  Future<String> download({
    required String url,
    required String targetPath,
    void Function(double? progress)? onProgress,
    CancelToken? cancelToken,
  }) async {
    if (url.isEmpty) {
      throw ArgumentError('URL is required', 'url');
    }
    final parent = File(targetPath).parent;
    await parent.create(recursive: true);
    final partPath = '$targetPath.part';
    try {
      await _dio.download(
        url,
        partPath,
        cancelToken: cancelToken,
        deleteOnError: true,
        options: Options(headers: _downloadHeaders),
        onReceiveProgress: onProgress == null
            ? null
            : (received, total) {
                if (total > 0) {
                  onProgress((received / total).clamp(0.0, 1.0).toDouble());
                } else {
                  onProgress(null);
                }
              },
      );
    } catch (_) {
      _tryDelete(partPath);
      rethrow;
    }
    await File(partPath).rename(targetPath);
    return targetPath;
  }

  void removeStale(String targetPath) {
    _tryDelete(targetPath);
    _tryDelete('$targetPath.part');
  }

  /// 从资产名、release 名称、tag 里依次提取版本号。
  ///
  /// 必须三段都看：rolling release 的 tag 固定是 `latest`，版本号只能放在
  /// release 名称里；而老 release 的版本号又只在 APK 文件名里。
  static String? resolveReleaseVersion(
    String assetName,
    String tag, {
    String name = '',
  }) {
    final assetMatch = _assetVersionRegex.firstMatch(assetName);
    final assetVersion = assetMatch?.group(1);
    if (assetVersion != null && assetVersion.isNotEmpty) {
      return assetVersion;
    }
    for (final text in <String>[name, tag]) {
      final match = _versionInText.firstMatch(text);
      final version = match?.group(1);
      if (version != null && version.isNotEmpty) {
        return version;
      }
    }
    return null;
  }

  static List<int>? parseVersion(String text) {
    var stripped = text.trim();
    if (stripped.isEmpty) {
      return null;
    }
    if (stripped.startsWith('v') || stripped.startsWith('V')) {
      stripped = stripped.substring(1);
    }
    final dash = stripped.indexOf(RegExp(r'[-+]'));
    if (dash >= 0) {
      stripped = stripped.substring(0, dash);
    }
    final parts = stripped.split('.').where((part) => part.isNotEmpty).toList();
    if (parts.isEmpty) {
      return null;
    }
    final length = parts.length < 3 ? 3 : parts.length;
    final numbers = List<int>.filled(length, 0);
    for (var i = 0; i < length; i++) {
      if (i < parts.length) {
        numbers[i] = int.tryParse(parts[i]) ?? 0;
      }
    }
    return numbers;
  }

  static int? compareVersions(String left, List<int>? right) {
    final leftParts = parseVersion(left);
    if (leftParts == null || right == null) {
      return null;
    }
    final length =
        leftParts.length > right.length ? leftParts.length : right.length;
    for (var i = 0; i < length; i++) {
      final l = i < leftParts.length ? leftParts[i] : 0;
      final r = i < right.length ? right[i] : 0;
      if (l != r) {
        return l < r ? -1 : 1;
      }
    }
    return 0;
  }

  Future<ReleaseInfo?> _fetchFromApi(List<int>? installed, String? abi) async {
    final response = await _dio.get<dynamic>(
      _releasesApiUrl,
      options: Options(headers: _apiHeaders),
    );
    if (!_isSuccess(response.statusCode)) {
      return null;
    }
    final releases = _decodeJson(response.data);
    if (releases is! List) {
      return null;
    }
    // 列表按发布时间倒序；取第一条满足“更新 + 有对应资产”的即可。
    for (final release in releases) {
      final candidate = _releaseFromJson(release, installed, abi);
      if (candidate != null) {
        return candidate;
      }
    }
    return null;
  }

  /// Atom 兜底：API 被限流（60 次/小时/IP）时仍能拿到最新 release 的标题和链接。
  ///
  /// Atom 没有资产列表，但下载链接可以由 tag 直接拼出来：
  /// `.../releases/download/<tag>/android.<abi>.apk`。所以 Android 的兜底
  /// 依然是应用内下载，而不是把 HTML 页面当 APK 下下来。
  Future<ReleaseInfo?> _fetchFromAtom(
    List<int>? installed,
    String? abi,
  ) async {
    final response = await _dio.get<String>(
      _atomFeedUrl,
      options: Options(
        headers: _downloadHeaders,
        responseType: ResponseType.plain,
      ),
    );
    if (!_isSuccess(response.statusCode)) {
      return null;
    }
    final xml = response.data ?? '';
    final entry = _atomEntryRegex.firstMatch(xml);
    if (entry == null) {
      return null;
    }
    final body = entry.group(1) ?? '';
    final titleMatch = _atomTitleRegex.firstMatch(body);
    if (titleMatch == null) {
      return null;
    }
    final linkMatch = _atomLinkRegex.firstMatch(body);
    if (linkMatch == null) {
      return null;
    }
    final pageUrl = (linkMatch.group(1) ?? '').trim();
    final title = _stripTags(titleMatch.group(1) ?? '').trim();
    if (pageUrl.isEmpty) {
      return null;
    }
    final tag = _tagFromReleaseUrl(pageUrl);
    final version = resolveReleaseVersion('', tag, name: title);
    if (version == null) {
      return null;
    }
    final compared = compareVersions(version, installed);
    if (compared == null || compared <= 0) {
      return null;
    }
    String downloadUrl = pageUrl;
    if (abi != null && tag.isNotEmpty) {
      final name = 'android.${_canonicalAbi(abi)}.apk';
      downloadUrl =
          '$repositoryUrl/releases/download/${Uri.encodeComponent(tag)}/$name';
    }
    return ReleaseInfo(
      version: version,
      title: title.isEmpty ? version : title,
      notes: '',
      downloadUrl: downloadUrl,
      pageUrl: pageUrl,
      publishedAt: _parseDate(null),
      tag: tag.isEmpty ? title : tag,
    );
  }

  static ReleaseInfo? _releaseFromJson(
    dynamic raw,
    List<int>? installed,
    String? abi,
  ) {
    if (raw is! Map) {
      return null;
    }
    final tag = raw['tag_name'] is String ? raw['tag_name'] as String : '';
    final name = raw['name'] is String ? raw['name'] as String : '';
    final notes = raw['body'] is String ? raw['body'] as String : '';
    final published = _parseDate(
        raw['published_at'] is String ? raw['published_at'] as String : null);
    final pageUrl = raw['html_url'] is String && (raw['html_url'] as String).isNotEmpty
        ? raw['html_url'] as String
        : (tag.isEmpty ? repositoryUrl : '$repositoryUrl/releases/tag/$tag');
    final assets = raw['assets'];

    Map<dynamic, dynamic>? asset;
    if (abi != null) {
      // 只有 Android 需要并强制要求 APK 资产：找不到当前 ABI 的包时跳过，
      // 不能退回到别的架构（装上去会闪退）。
      if (assets is! List) {
        return null;
      }
      asset = _selectAsset(assets, abi);
      if (asset == null) {
        return null;
      }
    }
    final assetName = asset == null ? '' : _assetName(asset);
    final version = resolveReleaseVersion(assetName, tag, name: name);
    if (version == null) {
      return null;
    }
    final compared = compareVersions(version, installed);
    if (compared == null || compared <= 0) {
      return null;
    }
    final assetUrl =
        asset != null && asset['browser_download_url'] is String
            ? asset['browser_download_url'] as String
            : '';
    if (abi != null && assetUrl.isEmpty) {
      return null;
    }
    return ReleaseInfo(
      version: version,
      title: name.isEmpty ? tag : name,
      notes: notes,
      downloadUrl: abi == null ? pageUrl : assetUrl,
      pageUrl: pageUrl,
      publishedAt: published,
      tag: tag,
    );
  }

  /// 按当前 ABI 选 APK，优先精确的 `android.<abi>.apk`。
  ///
  /// 匹配顺序：
  /// 1. 资产名 == `android.<别名>.apk`（当前 CI 的命名）；
  /// 2. 资产名含 `android` 且含别名（兼容 `KizunaEngine-1.0.0-android-arm64.apk`）；
  /// 3. 资产名只含别名（旧资产没有 android 前缀时）。
  /// 都找不到返回 null——绝不 fallback 到第一个 APK，那会装错架构。
  static Map<dynamic, dynamic>? _selectAsset(
    List<dynamic> assets,
    String abi,
  ) {
    final apks = <Map<dynamic, dynamic>>[];
    for (final asset in assets) {
      if (asset is Map && _assetName(asset).toLowerCase().endsWith('.apk')) {
        apks.add(asset);
      }
    }
    if (apks.isEmpty) {
      return null;
    }
    final canonical = _canonicalAbi(abi);
    final aliases = _abiAliases[canonical] ?? <String>[canonical];
    for (final alias in aliases) {
      for (final asset in apks) {
        if (_assetName(asset).toLowerCase() == 'android.$alias.apk') {
          return asset;
        }
      }
    }
    for (final alias in aliases) {
      for (final asset in apks) {
        final name = _assetName(asset).toLowerCase();
        if (name.contains('android') && name.contains(alias)) {
          return asset;
        }
      }
    }
    for (final alias in aliases) {
      for (final asset in apks) {
        if (_assetName(asset).toLowerCase().contains(alias)) {
          return asset;
        }
      }
    }
    return null;
  }

  /// 把 Android 的 ABI 标签收敛到三种发布名。
  static String _canonicalAbi(String abi) {
    final value = abi.trim().toLowerCase();
    if (value.contains('arm64') || value.contains('aarch64')) {
      return 'arm64';
    }
    if (value.contains('arm') || value.contains('armeabi')) {
      return 'arm32';
    }
    if (value.contains('x86') || value.contains('x64') || value.contains('amd64')) {
      return 'x64';
    }
    return 'arm64';
  }

  /// 从 `https://github.com/<owner>/<repo>/releases/tag/<tag>` 取 tag。
  static String _tagFromReleaseUrl(String url) {
    final uri = Uri.tryParse(url);
    if (uri == null) {
      return '';
    }
    final segments = uri.pathSegments;
    if (segments.length >= 2 && segments[segments.length - 2] == 'tag') {
      return segments.last;
    }
    return '';
  }

  static String _assetName(Map<dynamic, dynamic> asset) =>
      asset['name'] is String ? asset['name'] as String : '';

  static bool _isSuccess(int? statusCode) =>
      statusCode != null && statusCode >= 200 && statusCode < 300;

  static String _stripTags(String text) =>
      text.isEmpty ? '' : text.replaceAll(_tagStripRegex, '');

  static DateTime _parseDate(String? text) {
    if (text != null && text.isNotEmpty) {
      final parsed = DateTime.tryParse(text);
      if (parsed != null) {
        return parsed.toUtc();
      }
    }
    return DateTime.now().toUtc();
  }

  static dynamic _decodeJson(dynamic data) {
    if (data is String) {
      try {
        return jsonDecode(data);
      } catch (_) {
        return null;
      }
    }
    return data;
  }

  static void _tryDelete(String path) {
    try {
      final file = File(path);
      if (file.existsSync()) {
        file.deleteSync();
      }
    } catch (_) {}
  }
}
