import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/bridge/engine_bridge.dart';
import '../../core/platform/folder_picker.dart';
import '../../core/widgets/kizuna_loading.dart';
import 'game_card.dart';
import 'library_notifier.dart';

class LibraryScreen extends ConsumerStatefulWidget {
  const LibraryScreen({super.key});

  @override
  ConsumerState<LibraryScreen> createState() => _LibraryScreenState();
}

class _LibraryScreenState extends ConsumerState<LibraryScreen> {
  bool _searching = false;
  String _query = '';
  final TextEditingController _searchController = TextEditingController();

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  void _toggleSearch() {
    setState(() {
      _searching = !_searching;
      if (!_searching) {
        _query = '';
        _searchController.clear();
      }
    });
  }

  List<ScanEntry> _filter(List<ScanEntry> entries) {
    final query = _query.trim().toLowerCase();
    if (query.isEmpty) {
      return entries;
    }
    return entries.where((entry) {
      return entry.name.toLowerCase().contains(query) ||
          entry.brandName.toLowerCase().contains(query);
    }).toList();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final root = ref.watch(gameRootProvider);
    final games = ref.watch(libraryProvider);
    return Scaffold(
      appBar: AppBar(
        title: _searching
            ? TextField(
                controller: _searchController,
                autofocus: true,
                decoration: InputDecoration(
                  hintText: l10n.librarySearchHint,
                  border: InputBorder.none,
                ),
                onChanged: (value) {
                  setState(() {
                    _query = value;
                  });
                },
              )
            : Text(l10n.navGames),
        actions: <Widget>[
          IconButton(
            tooltip: _searching ? l10n.commonClose : l10n.librarySearch,
            icon: Icon(_searching ? Icons.close : Icons.search),
            onPressed: _toggleSearch,
          ),
          IconButton(
            tooltip: l10n.libraryScan,
            icon: const Icon(Icons.refresh),
            onPressed: () {
              ref.read(libraryProvider.notifier).rescan();
            },
          ),
        ],
      ),
      body: Builder(
        builder: (context) {
          if (root.isEmpty) {
            return _RootEmptyView(
              onSet: () {
                _editRoot(context, ref, root);
              },
            );
          }
          return games.when(
            loading: () => Center(
              child: KizunaLoadingIndicator(
                semanticsLabel: l10n.commonLoading,
              ),
            ),
            error: (error, stackTrace) => Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: <Widget>[
                  Text('${l10n.commonError}: $error'),
                  const SizedBox(height: 12),
                  FilledButton(
                    onPressed: () {
                      ref.read(libraryProvider.notifier).rescan();
                    },
                    child: Text(l10n.commonRetry),
                  ),
                ],
              ),
            ),
            data: (entries) {
              final filtered = _filter(entries);
              if (entries.isEmpty) {
                return Center(child: Text(l10n.libraryNoGames));
              }
              if (filtered.isEmpty) {
                return Center(child: Text(l10n.libraryNoMatch));
              }
              return ListView.builder(
                padding: const EdgeInsets.fromLTRB(12, 12, 12, 24),
                itemCount: filtered.length,
                itemBuilder: (context, index) {
                  final entry = filtered[index];
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: GameCard(
                      entry: entry,
                      onTap: () {
                        context.push(
                            '/player/${Uri.encodeComponent(entry.directory)}',
                            extra: entry);
                      },
                    ),
                  );
                },
              );
            },
          );
        },
      ),
    );
  }

  Future<void> _editRoot(
      BuildContext context, WidgetRef ref, String initial) async {
    final l10n = AppLocalizations.of(context)!;
    final controller = TextEditingController(text: initial);
    String? picked;
    final value = await showDialog<String>(
      context: context,
      builder: (dialogContext) {
        return StatefulBuilder(
          builder: (context, setState) {
            return AlertDialog(
              title: Text(l10n.settingsGameRoot),
              content: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  TextField(
                    controller: controller,
                    autofocus: true,
                    decoration:
                        InputDecoration(hintText: l10n.settingsGameRootHint),
                  ),
                  if (FolderPicker.supported)
                    Align(
                      alignment: Alignment.centerLeft,
                      child: TextButton.icon(
                        icon: const Icon(Icons.folder_open),
                        label: Text(l10n.settingsPickFolder),
                        onPressed: () async {
                          final result = await FolderPicker.pick();
                          if (result == null) {
                            return;
                          }
                          var path = result.path;
                          if ((path == null || path.isEmpty) &&
                              result.uri != null) {
                            path = await FolderPicker.mirror(
                              result.uri!,
                              l10n.settingsDefaultRoot,
                            );
                          }
                          if (path != null && path.isNotEmpty) {
                            picked = path;
                            controller.text = path;
                            setState(() {});
                          }
                        },
                      ),
                    ),
                ],
              ),
              actions: <Widget>[
                TextButton(
                  onPressed: () {
                    Navigator.of(dialogContext).pop();
                  },
                  child: Text(l10n.commonCancel),
                ),
                FilledButton(
                  onPressed: () {
                    Navigator.of(dialogContext)
                        .pop(picked ?? controller.text.trim());
                  },
                  child: Text(l10n.commonConfirm),
                ),
              ],
            );
          },
        );
      },
    );
    controller.dispose();
    if (value != null) {
      await ref.read(gameRootProvider.notifier).set(value);
      ref.invalidate(libraryProvider);
    }
  }
}

class _RootEmptyView extends StatelessWidget {
  final VoidCallback onSet;

  const _RootEmptyView({required this.onSet});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Text(l10n.libraryNoGames),
          const SizedBox(height: 12),
          FilledButton(
            onPressed: onSet,
            child: Text(l10n.settingsGameRoot),
          ),
        ],
      ),
    );
  }
}
