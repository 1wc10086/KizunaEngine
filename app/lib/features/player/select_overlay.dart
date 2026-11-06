import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import '../../core/bridge/engine_bridge.dart';
import 'frame_painter.dart';

class SelectOverlay extends StatefulWidget {
  final List<SelectItem> items;
  final Map<String, EngineFrame> frames;
  final int engineWidth;
  final int engineHeight;
  final Future<void> Function(int x, int y) onPick;
  final Future<void> Function(int x, int y) onHover;

  const SelectOverlay(
      {super.key,
      required this.items,
      required this.frames,
      required this.engineWidth,
      required this.engineHeight,
      required this.onPick,
      required this.onHover});

  @override
  State<SelectOverlay> createState() => _SelectOverlayState();
}

class _SelectOverlayState extends State<SelectOverlay> {
  final Map<String, ui.Image> _images = {};
  final Map<String, EngineFrame> _decoded = {};
  int _hovered = -1;

  @override
  void initState() {
    super.initState();
    _syncImages();
  }

  @override
  void didUpdateWidget(SelectOverlay oldWidget) {
    super.didUpdateWidget(oldWidget);
    _syncImages();
  }

  @override
  void dispose() {
    for (final image in _images.values) {
      image.dispose();
    }
    _images.clear();
    super.dispose();
  }

  void _syncImages() {
    final stale = <String>[];
    for (final key in _images.keys) {
      if (!widget.frames.containsKey(key)) {
        stale.add(key);
      }
    }
    for (final key in stale) {
      _images.remove(key)?.dispose();
      _decoded.remove(key);
    }
    for (final entry in widget.frames.entries) {
      final current = _decoded[entry.key];
      if (current != null &&
          identical(current.pixels, entry.value.pixels) &&
          _images.containsKey(entry.key)) {
        continue;
      }
      _decoded[entry.key] = entry.value;
      final frame = entry.value;
      decodeRgbaImage(frame.pixels, frame.width, frame.height).then((img) {
        if (!mounted) {
          img.dispose();
          return;
        }
        if (!identical(_decoded[entry.key]?.pixels, frame.pixels)) {
          img.dispose();
          return;
        }
        _images[entry.key]?.dispose();
        _images[entry.key] = img;
        setState(() {});
      }).catchError((_) {});
    }
  }

  @override
  Widget build(BuildContext context) {
    if (widget.items.isEmpty ||
        widget.engineWidth <= 0 ||
        widget.engineHeight <= 0) {
      return const SizedBox.shrink();
    }
    return Positioned.fill(
      child: LayoutBuilder(
        builder: (context, constraints) {
          final scale =
              constraints.maxWidth / widget.engineWidth.toDouble();
          return Stack(
            children: <Widget>[
              for (final item in widget.items)
                Positioned(
                  left: item.x * scale,
                  top: item.y * scale,
                  width: (item.width <= 0 ? 1 : item.width) * scale,
                  height: (item.height <= 0 ? 1 : item.height) * scale,
                  child: MouseRegion(
                    onEnter: (_) {
                      _hovered = item.index;
                      widget.onHover(
                          item.x + item.width ~/ 2, item.y + item.height ~/ 2);
                      setState(() {});
                    },
                    onExit: (_) {
                      if (_hovered == item.index) {
                        _hovered = -1;
                        setState(() {});
                      }
                    },
                    child: Builder(
                      builder: (itemContext) {
                        return GestureDetector(
                          behavior: HitTestBehavior.opaque,
                          onTapDown: (details) {
                            final box = itemContext.size;
                            final local = details.localPosition;
                            final dx = box == null || box.width <= 0
                                ? item.width ~/ 2
                                : (local.dx / scale).round();
                            final dy = box == null || box.height <= 0
                                ? item.height ~/ 2
                                : (local.dy / scale).round();
                            widget.onPick(item.x + dx, item.y + dy);
                          },
                          child: RepaintBoundary(
                            child: CustomPaint(
                              painter: _SelectItemPainter(
                                image: _images[item.frame],
                                highlighted: _hovered == item.index,
                                text: item.text,
                                textX: item.textX * scale,
                                textY: item.textY * scale,
                                scale: scale,
                                alignment: _alignOf(item.alignment),
                                defaultColor: Color(item.defaultColor),
                                selectColor: Color(item.selectColor),
                              ),
                            ),
                          ),
                        );
                      },
                    ),
                  ),
                ),
            ],
          );
        },
      ),
    );
  }

  TextAlign _alignOf(int value) {
    switch (value) {
      case 1:
        return TextAlign.center;
      case 2:
        return TextAlign.right;
      default:
        return TextAlign.left;
    }
  }
}

class _SelectItemPainter extends CustomPainter {
  final ui.Image? image;
  final bool highlighted;
  final String text;
  final double textX;
  final double textY;
  final double scale;
  final TextAlign alignment;
  final Color defaultColor;
  final Color selectColor;

  const _SelectItemPainter({
    required this.image,
    required this.highlighted,
    required this.text,
    required this.textX,
    required this.textY,
    required this.scale,
    required this.alignment,
    required this.defaultColor,
    required this.selectColor,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final img = image;
    if (img != null && img.height > 1) {
      final half = img.height / 2.0;
      final src = Rect.fromLTWH(
          0, highlighted ? half : 0, img.width.toDouble(), half);
      final paint = Paint()..filterQuality = FilterQuality.medium;
      canvas.drawImageRect(img, src, Offset.zero & size, paint);
    }
    if (text.isEmpty) {
      return;
    }
    final painter = TextPainter(
      text: TextSpan(
        text: text,
        style: TextStyle(
          color: highlighted ? selectColor : defaultColor,
          fontSize: 16 * (scale <= 0 ? 1 : scale),
        ),
      ),
      textAlign: alignment,
      textDirection: TextDirection.ltr,
      maxLines: 4,
    );
    final width = size.width - textX;
    if (width > 0) {
      painter.layout(minWidth: 0, maxWidth: width);
      painter.paint(canvas, Offset(textX, textY));
    }
    painter.dispose();
  }

  @override
  bool shouldRepaint(_SelectItemPainter oldDelegate) {
    return oldDelegate.image != image ||
        oldDelegate.highlighted != highlighted ||
        oldDelegate.text != text ||
        oldDelegate.textX != textX ||
        oldDelegate.textY != textY ||
        oldDelegate.scale != scale ||
        oldDelegate.defaultColor != defaultColor ||
        oldDelegate.selectColor != selectColor;
  }
}
