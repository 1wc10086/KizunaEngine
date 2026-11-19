import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import 'core/settings/settings_models.dart';
import 'core/theme/app_theme.dart';
import 'core/theme/palette.dart';
import 'features/settings/viewmodel/settings_notifier.dart';
import 'router/router.dart';

/// 组合根的视图层：只负责把"设置 + 路由 + 主题"映射成 MaterialApp。
///
/// 订阅的是 appearanceSettingsProvider 而不是整份 AppSettings：后者包含
/// gameRoot/autoUpdateCheck 这类与外观无关的键，改动它们不该重建整棵树。
class KizunaApp extends ConsumerWidget {
  const KizunaApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    final appearance = ref.watch(appearanceSettingsProvider);
    final seed = paletteColor(appearance.paletteId);
    return MaterialApp.router(
      onGenerateTitle: (context) => AppLocalizations.of(context).appName,
      theme: buildAppTheme(
        brightness: Brightness.light,
        seed: seed,
        amoled: appearance.amoled,
      ),
      darkTheme: buildAppTheme(
        brightness: Brightness.dark,
        seed: seed,
        amoled: appearance.amoled,
      ),
      themeMode: switch (appearance.themeMode) {
        AppThemeMode.system => ThemeMode.system,
        AppThemeMode.light => ThemeMode.light,
        AppThemeMode.dark => ThemeMode.dark,
      },
      locale: switch (appearance.language) {
        AppLanguage.system => null,
        AppLanguage.zh => const Locale('zh'),
        AppLanguage.en => const Locale('en'),
      },
      localizationsDelegates: <LocalizationsDelegate<dynamic>>[
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: const <Locale>[Locale('zh'), Locale('en')],
      builder: (context, child) {
        final media = MediaQuery.of(context);
        return MediaQuery(
          data: media.copyWith(
            textScaler: TextScaler.linear(appearance.fontScale),
          ),
          child: child ?? const SizedBox.shrink(),
        );
      },
      routerConfig: router,
    );
  }
}
