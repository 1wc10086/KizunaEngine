import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import 'core/settings/settings_models.dart';
import 'core/theme/app_theme.dart';
import 'core/theme/palette.dart';
import 'features/settings/settings_notifier.dart';
import 'router/router.dart';

class KizunaApp extends ConsumerWidget {
  const KizunaApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    final settings = ref.watch(appSettingsProvider).value ?? const AppSettings();
    final seed = paletteColor(settings.themePaletteId);
    return MaterialApp.router(
      onGenerateTitle: (context) => AppLocalizations.of(context).appName,
      theme: buildAppTheme(
        brightness: Brightness.light,
        seed: seed,
        amoled: settings.amoled,
      ),
      darkTheme: buildAppTheme(
        brightness: Brightness.dark,
        seed: seed,
        amoled: settings.amoled,
      ),
      themeMode: switch (settings.themeMode) {
        AppThemeMode.system => ThemeMode.system,
        AppThemeMode.light => ThemeMode.light,
        AppThemeMode.dark => ThemeMode.dark,
      },
      locale: switch (settings.language) {
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
            textScaler: TextScaler.linear(settings.fontScale),
          ),
          child: child ?? const SizedBox.shrink(),
        );
      },
      routerConfig: router,
    );
  }
}
