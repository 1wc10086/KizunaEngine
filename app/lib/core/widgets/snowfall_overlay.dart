import 'dart:math' as math;

import 'package:flutter/material.dart';

/// AppBar 里的降雪。
///
/// 实现取舍：
/// * 所有雪花画在同一块 CustomPaint 上，而不是每片一个 widget：
///   三十来个带 transform 的小 widget 每帧都要走布局，画布只需要一次 paint。
/// * 用固定随机种子，雪形在每次启动/重建之间保持一致，方便复现视觉问题。
/// * `repaint: controller` 让动画只触发重绘，不触发 build。
/// * 系统开启"移除动画"（无障碍）时停表，只留一帧静态雪点。
class SnowfallOverlay extends StatefulWidget {
  const SnowfallOverlay({
    super.key,
    this.flakeCount = 26,
    this.period = const Duration(seconds: 11),
  });

  final int flakeCount;
  final Duration period;

  @override
  State<SnowfallOverlay> createState() => _SnowfallOverlayState();
}

class _SnowfallOverlayState extends State<SnowfallOverlay>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: widget.period,
  );
  late final List<_Snowflake> _flakes =
      _generateFlakes(widget.flakeCount);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _controller.stop();
    } else if (!_controller.isAnimating) {
      _controller.repeat();
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    // 亮色主题下纯白雪花几乎看不见，改用低透明的 primary 雪点；
    // 暗色主题保持白色。两边都读得出来是"雪"，而不是脏点。
    final Color color = scheme.brightness == Brightness.dark
        ? Colors.white
        : scheme.primary;
    return IgnorePointer(
      child: RepaintBoundary(
        child: CustomPaint(
          painter: _SnowPainter(
            progress: _controller,
            flakes: _flakes,
            color: color,
          ),
          child: const SizedBox.expand(),
        ),
      ),
    );
  }
}

class _Snowflake {
  const _Snowflake({
    required this.x,
    required this.start,
    required this.speed,
    required this.sway,
    required this.radius,
    required this.opacity,
  });

  /// 横向基准位置（0..1）。
  final double x;

  /// 起始纵向相位（0..1），让雪花一开始就散开而不是同时从顶上落下。
  final double start;

  /// 相对下落速度。
  final double speed;

  /// 横向摆动频率。
  final double sway;

  final double radius;
  final double opacity;
}

List<_Snowflake> _generateFlakes(int count) {
  final math.Random random = math.Random(0x5A0F);
  return List<_Snowflake>.generate(count, (int index) {
    return _Snowflake(
      x: random.nextDouble(),
      start: random.nextDouble(),
      speed: 0.6 + random.nextDouble() * 0.8,
      sway: 0.5 + random.nextDouble() * 1.5,
      radius: 0.9 + random.nextDouble() * 1.8,
      opacity: 0.25 + random.nextDouble() * 0.45,
    );
  });
}

class _SnowPainter extends CustomPainter {
  _SnowPainter({
    required this.progress,
    required this.flakes,
    required this.color,
  }) : super(repaint: progress);

  final Animation<double> progress;
  final List<_Snowflake> flakes;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) {
      return;
    }
    final double time = progress.value;
    final Paint paint = Paint()..style = PaintingStyle.fill;
    for (final _Snowflake flake in flakes) {
      final double life = (flake.start + time * flake.speed) % 1.0;
      final double dy = life * (size.height + 8) - 4;
      final double dx = flake.x * size.width +
          math.sin((life * flake.sway + flake.start) * math.pi * 2) * 7;
      // 接近底边时淡出，避免雪花在 AppBar 下边缘"啪"地消失。
      final double fade =
          life < 0.85 ? 1.0 : ((1.0 - life) / 0.15).clamp(0.0, 1.0);
      paint.color = color.withValues(alpha: flake.opacity * fade);
      canvas.drawCircle(Offset(dx, dy), flake.radius, paint);
    }
  }

  @override
  bool shouldRepaint(covariant _SnowPainter oldDelegate) {
    return oldDelegate.color != color || oldDelegate.flakes != flakes;
  }
}
