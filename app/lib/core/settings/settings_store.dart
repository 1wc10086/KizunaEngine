import 'dart:convert';
import 'dart:io';

class JsonFileStore {
  JsonFileStore(this.file);

  final File file;

  Future<Map<String, Object?>?> read() async {
    try {
      if (!await file.exists()) {
        return null;
      }
      final raw = await file.readAsString();
      if (raw.trim().isEmpty) {
        return null;
      }
      final decoded = jsonDecode(raw);
      if (decoded is Map<String, Object?>) {
        return decoded;
      }
      if (decoded is Map) {
        return decoded.map((key, value) => MapEntry('$key', value));
      }
    } catch (_) {
      return null;
    }
    return null;
  }

  Future<void> write(Map<String, Object?> data) async {
    await file.parent.create(recursive: true);
    final tmp = File('${file.path}.tmp');
    await tmp.writeAsString(jsonEncode(data), flush: true);
    try {
      await tmp.rename(file.path);
    } on FileSystemException {
      if (await file.exists()) {
        await file.delete();
      }
      await tmp.rename(file.path);
    }
  }
}
