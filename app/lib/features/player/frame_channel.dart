import 'dart:typed_data';

import 'package:flutter/foundation.dart';

/// 一帧合成结果。[generation] 是发布序号（单调递增），用来判定"这是不是新的一帧"。
///
/// [bytes] 交给 UI 后只读：合成侧随即换新缓冲，不会再写这一块内存。
class FrameData {
  final Uint8List bytes;
  final int width;
  final int height;
  final int generation;

  const FrameData({
    required this.bytes,
    required this.width,
    required this.height,
    required this.generation,
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
  bool _decoding = false;
  int _decodeSince = 0;

  /// 发布一帧。[bytes] 从此只读，由调用方保证不再写。
  void publish(Uint8List bytes, {required int width, required int height}) {
    _generation++;
    frame.value = FrameData(
      bytes: bytes,
      width: width,
      height: height,
      generation: _generation,
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
