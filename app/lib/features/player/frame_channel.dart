import 'dart:typed_data';

import 'package:flutter/foundation.dart';

/// 引擎交给宿主绘制的一段文字（frame 通道内的不可变 DTO）。
///
/// 字形不再栅格进帧位图：折行、基线、行距仍由引擎算好，Flutter 只按
/// [x]/[y]（引擎坐标系，与帧位图同尺寸）用平台字体画出来。这样不需要内置 TTF，
/// 又保留了原版排版坐标。[y] 是基线而不是顶边：Flutter 字体的 ascent 与旧内嵌
/// 字体不同，按顶边对齐会让整行上下漂移。
///
/// 引擎的 align 字段不往下带：引擎已经把 [x] 算成这一行的实际起点，宿主按左
/// 对齐画即可。
class TextRun {
  final String text;
  final int x;
  final int y;

  /// 字号（引擎像素）。
  final int size;

  /// 正文色，0xAARRGGBB。
  final int color;

  /// 描边色，0xAARRGGBB；alpha 为 0 表示不描边。
  final int outline;

  /// 阴影偏移（引擎像素，0 表示不画阴影）。
  final int shadow;

  /// 该行可用的最大宽度（引擎像素，0 表示不限制）。
  final int width;

  final bool bold;

  const TextRun({
    required this.text,
    required this.x,
    required this.y,
    required this.size,
    required this.color,
    this.outline = 0,
    this.shadow = 0,
    this.width = 0,
    this.bold = false,
  });
}

/// 一帧合成结果。[generation] 是发布序号（单调递增），用来判定"这是不是新的一帧"。
///
/// [bitmap] 只在位图内容真的换了时自增。合成侧会复用像素缓冲（三块轮转，见
/// PlayerNotifier._nextPixelBuffer），所以 `identical(bytes)` 不再能代表"同一张图"；
/// 顶层框架要判断"要不要重新解码成 ui.Image"，看的就是这个号。只换文字的那次
/// 发布沿用上一个 bitmap 号，宿主因此不会白解一遍同样的像素。
///
/// [bytes] 交给 UI 后只读：合成侧在它被轮转回写之前不会碰它（中间隔着解码完成），
/// [runs] 与这一帧配套的文字覆盖层，同样只读。
class FrameData {
  final Uint8List bytes;
  final int width;
  final int height;
  final int generation;
  final int bitmap;
  final List<TextRun> runs;

  const FrameData({
    required this.bytes,
    required this.width,
    required this.height,
    required this.generation,
    required this.bitmap,
    this.runs = const <TextRun>[],
  });
}

/// 合成 isolate → UI 的帧通道（播放页专用）。
///
/// 帧位图以前挂在 Riverpod 的 PlayerState 上，每合成一帧就整份 state 换一次，
/// 播放页所有 playerProvider 订阅者（画面、文本框、按钮、overlay）都要跟着重算一遍
/// 选择器；而位图实际只影响画面本身。拆到这里之后，帧到达只惊动监听 [frame] 的
/// FrameSourceView，其余控件不再跟着 30fps 抖。
class FrameBus {
  /// 最新一帧；null 表示还没合成过。
  final ValueNotifier<FrameData?> frame = ValueNotifier<FrameData?>(null);

  // UI 解码超过这个时长还没结束就当作卡住：不再拦合成，画面优先。
  static const int _decodeStuckMs = 300;

  int _generation = 0;
  int _bitmap = 0;
  bool _decoding = false;
  int _decodeSince = 0;

  /// 发布一帧。[bytes] 从此只读，由调用方保证不再写。
  ///
  /// [runs] 是这一帧的文字覆盖层：位图与文字必须同批发布，文字坐标只对刚合成
  /// 的这一帧有效，分成两次通知会让画面和文字错开一拍。
  ///
  /// [bitmapChanged] 为 false 表示位图像素与上一帧完全相同（只是文字变了）：
  /// 复用上次的 bitmap 号，FrameView 就不会白解一遍 ui.Image。
  void publish(Uint8List bytes,
      {required int width,
      required int height,
      List<TextRun> runs = const <TextRun>[],
      bool bitmapChanged = true}) {
    _generation++;
    if (bitmapChanged) {
      _bitmap++;
    }
    frame.value = FrameData(
      bytes: bytes,
      width: width,
      height: height,
      generation: _generation,
      bitmap: _bitmap,
      runs: runs,
    );
  }

  /// UI isolate 解码忙闲上报（FrameView 驱动）。
  void setDecoding(bool value) {
    if (value == _decoding) {
      return;
    }
    _decoding = value;
    _decodeSince = value ? DateTime.now().millisecondsSinceEpoch : 0;
  }

  /// 合成侧背压：UI 正在解码（且没卡死）时为真，这一次合成触发就跳过。
  ///
  /// 跳过不丢画面：Go 侧每次合成都是按引擎当前状态重算整帧、dirty 判定只决定要不要回包，
  /// 下一次触发拿到的仍是最新状态，只是这一拍的采样点往后挪一个 tick；
  /// 反过来，UI 解不完的帧本来也会在 FrameView 里被更新的帧顶掉，先去要它只是白跑一趟
  /// 合成 + 3.7MB 拷回。
  bool get decodeBusy {
    if (!_decoding) {
      return false;
    }
    final now = DateTime.now().millisecondsSinceEpoch;
    return now - _decodeSince < _decodeStuckMs;
  }

  void dispose() {
    frame.dispose();
  }
}
