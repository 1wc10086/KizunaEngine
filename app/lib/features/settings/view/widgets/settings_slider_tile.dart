import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class SettingsSliderTile extends AbstractSettingsTile {
  const SettingsSliderTile({
    super.key,
    required this.icon,
    required this.title,
    this.description,
    required this.min,
    required this.max,
    this.divisions,
    required this.value,
    required this.labelBuilder,
    required this.onChangeEnd,
  });

  final IconData icon;
  final String title;
  final String? description;
  final double min;
  final double max;
  final int? divisions;
  final double value;
  final String Function(double value) labelBuilder;
  final ValueChanged<double> onChangeEnd;

  @override
  Widget build(BuildContext context) {
    return CustomSettingsTile(
      child: (info) => _SliderTileBody(
        topTile: info.isTopTile,
        bottomTile: info.isBottomTile,
        divider: info.needDivider,
        icon: icon,
        title: title,
        description: description,
        min: min,
        max: max,
        divisions: divisions,
        value: value,
        labelBuilder: labelBuilder,
        onChangeEnd: onChangeEnd,
      ),
    );
  }
}

class _SliderTileBody extends ConsumerStatefulWidget {
  const _SliderTileBody({
    required this.topTile,
    required this.bottomTile,
    required this.divider,
    required this.icon,
    required this.title,
    this.description,
    required this.min,
    required this.max,
    this.divisions,
    required this.value,
    required this.labelBuilder,
    required this.onChangeEnd,
  });

  final bool topTile;
  final bool bottomTile;
  final bool divider;
  final IconData icon;
  final String title;
  final String? description;
  final double min;
  final double max;
  final int? divisions;
  final double value;
  final String Function(double value) labelBuilder;
  final ValueChanged<double> onChangeEnd;

  @override
  ConsumerState<_SliderTileBody> createState() => _SliderTileBodyState();
}

class _SliderTileBodyState extends ConsumerState<_SliderTileBody> {
  late double _value;
  bool _dragging = false;

  @override
  void initState() {
    super.initState();
    _value = widget.value;
  }

  @override
  void didUpdateWidget(covariant _SliderTileBody oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!_dragging && oldWidget.value != widget.value) {
      _value = widget.value;
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final noteStyle = TextStyle(
      color: theme.hintColor,
      fontWeight: FontWeight.w500,
      fontSize: 13,
    );
    final description = widget.description;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        ClipRRect(
          borderRadius: BorderRadius.vertical(
            top: Radius.circular(widget.topTile ? 20 : 3),
            bottom: Radius.circular(widget.bottomTile ? 20 : 3),
          ),
          child: Material(
            color: theme.brightness == Brightness.light
                ? theme.colorScheme.surfaceContainerLowest
                : theme.colorScheme.surfaceContainerHigh,
            child: Row(
              children: <Widget>[
                Padding(
                  padding: const EdgeInsets.only(left: 16),
                  child: IconTheme.merge(
                    data: IconThemeData(color: theme.colorScheme.onSurface),
                    child: Icon(widget.icon),
                  ),
                ),
                Expanded(
                  child: Padding(
                    padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        Row(
                          children: <Widget>[
                            Expanded(
                              child: Text(
                                widget.title,
                                style: TextStyle(
                                  color: theme.colorScheme.onSurface,
                                  fontWeight: FontWeight.w600,
                                  fontSize: 15,
                                ),
                              ),
                            ),
                            Text(widget.labelBuilder(_value), style: noteStyle),
                          ],
                        ),
                        if (description != null) ...<Widget>[
                          const SizedBox(height: 2),
                          Text(description, style: noteStyle),
                        ],
                        Slider(
                          value: _value.clamp(widget.min, widget.max).toDouble(),
                          min: widget.min,
                          max: widget.max,
                          divisions: widget.divisions,
                          label: widget.labelBuilder(_value),
                          onChanged: (value) {
                            setState(() {
                              _dragging = true;
                              _value = value;
                            });
                          },
                          onChangeEnd: (value) {
                            setState(() {
                              _dragging = false;
                              _value = value;
                            });
                            widget.onChangeEnd(value);
                          },
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
        if (widget.divider) const SizedBox(height: 2),
      ],
    );
  }
}
