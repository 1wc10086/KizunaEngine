import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../model/translation_errors.dart';
import '../model/translation_state.dart';

/// 把 viewmodel 报的事件码翻成当前语言的句子。
///
/// 为什么不在 viewmodel 里直接拼字符串：语言可以在任务跑到一半时切走，
/// state 里存的是拼接后的中文就再也换不回来了（运行中也不会重建 state）。
/// 这里只做纯映射，不碰 state，所以视图随时可以按最新 locale 重画。
String translationLogText(AppLocalizations l10n, TranslationLog log) {
  switch (log.code) {
    case 'started':
      return l10n.translationLogStarted(log.detail);
    case 'canceling':
      return l10n.translationLogCanceling;
    case 'canceled':
      return l10n.translationLogCanceled;
    case 'finished':
      return l10n.translationLogFinished;
    case 'translated':
      return l10n.translationLogTranslated(log.detail);
    case 'failed':
      return log.detail.isEmpty
          ? l10n.translationLogFailed
          : '${l10n.translationLogFailed}: ${log.detail}';
    case 'residual':
      return l10n.translationLogResidual(log.detail);
    default:
      // 'raw' 与未知码：内核已经给的是给人看的文字（模型返回的错误等），
      // 原样显示，翻译反而会丢信息。
      return log.detail;
  }
}

/// 前置校验错误码 → 可操作提示。
String translationErrorText(AppLocalizations l10n, String code) {
  switch (code) {
    case TranslationErrorCode.noFile:
      return l10n.translationAddFileFirst;
    case TranslationErrorCode.noPreset:
      return l10n.translationAddPresetFirst;
    case TranslationErrorCode.noFormat:
      return l10n.translationSelectFormatFirst;
    default:
      // 内核异常原文，原样呈现便于排查。
      return code;
  }
}

/// 运行进度文案：`starting` 是 viewmodel 给"已提交、还没收到第一个进度事件"
/// 的占位码。
String translationProgressText(AppLocalizations l10n, TranslationState state) {
  if (state.progressText.isEmpty) {
    return l10n.translationNotStarted;
  }
  if (state.progressText == 'starting') {
    return l10n.translationStarting;
  }
  return state.progressText;
}
