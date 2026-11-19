/// 翻译流程的前置校验错误码。
///
/// 用常量而不是枚举有两个原因：state 里已经有 `raw`/内核异常的字符串型
/// error，混用两种类型要到处做类型判断；而这些码只在本模块内产生、
/// 只由本模块的 view 翻译成文案，字符串足够且序列化友好。
class TranslationErrorCode {
  static const String noFile = 'no-file';
  static const String noPreset = 'no-preset';
  static const String noFormat = 'no-format';

  const TranslationErrorCode._();
}
