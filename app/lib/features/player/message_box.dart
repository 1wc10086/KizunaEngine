import 'package:flutter/material.dart';

class MessageBox extends StatelessWidget {
  final String speaker;
  final String text;
  final bool visible;

  const MessageBox(
      {super.key,
      required this.speaker,
      required this.text,
      required this.visible});

  @override
  Widget build(BuildContext context) {
    if (!visible || text.isEmpty) {
      return const SizedBox.shrink();
    }
    return Positioned(
      left: 12,
      right: 12,
      bottom: 12,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: const Color(0xCC000000),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            if (speaker.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(bottom: 4),
                child: Text(
                  speaker,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.titleSmall?.copyWith(
                        color: const Color(0xFFFFFFFF),
                        fontWeight: FontWeight.bold,
                      ),
                ),
              ),
            Text(
              text,
              style: Theme.of(context)
                  .textTheme
                  .bodyMedium
                  ?.copyWith(color: const Color(0xFFFFFFFF)),
            ),
          ],
        ),
      ),
    );
  }
}
