import 'dart:convert';
import 'dart:typed_data';

class KzWireException implements Exception {
  final String message;
  const KzWireException(this.message);

  @override
  String toString() => 'KzWireException: $message';
}

class KzFieldTypes {
  static const int u32 = 1;
  static const int i32 = 2;
  static const int u64 = 3;
  static const int str = 4;
  static const int bytes = 5;
  static const int list = 6;
  static const int boolean = 7;
  static const int f64 = 8;
}

class KzWriter {
  final BytesBuilder _builder = BytesBuilder();

  void _head(int id, int type, int length) {
    final head = ByteData(8);
    head.setUint16(0, id, Endian.little);
    head.setUint8(2, type);
    head.setUint8(3, 0);
    head.setUint32(4, length, Endian.little);
    _builder.add(head.buffer.asUint8List());
  }

  void u32(int id, int value) {
    _head(id, KzFieldTypes.u32, 4);
    final body = ByteData(4);
    body.setUint32(0, value, Endian.little);
    _builder.add(body.buffer.asUint8List());
  }

  void i32(int id, int value) {
    _head(id, KzFieldTypes.i32, 4);
    final body = ByteData(4);
    body.setInt32(0, value, Endian.little);
    _builder.add(body.buffer.asUint8List());
  }

  void u64(int id, int value) {
    _head(id, KzFieldTypes.u64, 8);
    final body = ByteData(8);
    body.setUint64(0, value, Endian.little);
    _builder.add(body.buffer.asUint8List());
  }

  void f64(int id, double value) {
    _head(id, KzFieldTypes.f64, 8);
    final body = ByteData(8);
    body.setFloat64(0, value, Endian.little);
    _builder.add(body.buffer.asUint8List());
  }

  void boolean(int id, bool value) {
    _head(id, KzFieldTypes.boolean, 1);
    _builder.add(Uint8List.fromList(<int>[value ? 1 : 0]));
  }

  void str(int id, String value) {
    final body = utf8.encode(value);
    _head(id, KzFieldTypes.str, body.length);
    _builder.add(body);
  }

  void raw(int id, Uint8List value) {
    _head(id, KzFieldTypes.bytes, value.length);
    _builder.add(value);
  }

  void strList(int id, List<String> values) {
    final sub = KzWriter();
    for (final value in values) {
      sub.str(0, value);
    }
    final body = sub.takeBytes();
    _head(id, KzFieldTypes.list, body.length);
    _builder.add(body);
  }

  void list(int id, void Function(KzWriter sub) build) {
    final sub = KzWriter();
    build(sub);
    final body = sub.takeBytes();
    _head(id, KzFieldTypes.list, body.length);
    _builder.add(body);
  }

  void nested(void Function(KzWriter sub) build) {
    final sub = KzWriter();
    build(sub);
    _builder.add(sub.takeBytes());
  }

  Uint8List takeBytes() => _builder.takeBytes();
}

class _KzRawField {
  final int type;
  final Uint8List value;
  const _KzRawField(this.type, this.value);
}

class KzReader {
  final Map<int, _KzRawField> _fields;

  KzReader(Uint8List payload) : _fields = _parse(payload);

  static Map<int, _KzRawField> _parse(Uint8List payload) {
    final fields = <int, _KzRawField>{};
    final data = ByteData.sublistView(payload);
    var offset = 0;
    while (offset + 8 <= payload.length) {
      final id = data.getUint16(offset, Endian.little);
      final type = data.getUint8(offset + 2);
      final length = data.getUint32(offset + 4, Endian.little);
      offset += 8;
      if (length > payload.length - offset) {
        break;
      }
      fields[id] =
          _KzRawField(type, Uint8List.sublistView(payload, offset, offset + length));
      offset += length;
    }
    return fields;
  }

  Uint8List? _value(int id, int type) {
    final field = _fields[id];
    if (field == null || field.type != type) {
      return null;
    }
    return field.value;
  }

  int u32(int id) {
    final value = _value(id, KzFieldTypes.u32);
    if (value == null || value.length < 4) {
      return 0;
    }
    return ByteData.sublistView(value).getUint32(0, Endian.little);
  }

  int i32(int id) {
    final value = _value(id, KzFieldTypes.i32);
    if (value == null || value.length < 4) {
      return 0;
    }
    return ByteData.sublistView(value).getInt32(0, Endian.little);
  }

  int u64(int id) {
    final value = _value(id, KzFieldTypes.u64);
    if (value == null || value.length < 8) {
      return 0;
    }
    return ByteData.sublistView(value).getUint64(0, Endian.little);
  }

  double f64(int id) {
    final value = _value(id, KzFieldTypes.f64);
    if (value == null || value.length < 8) {
      return 0.0;
    }
    return ByteData.sublistView(value).getFloat64(0, Endian.little);
  }

  /// 读布尔字段。
  ///
  /// [fallback] 是"字段完全缺失"时的返回值。默认 false 符合协议里"没写就是关"的约定；
  /// 但像鉴赏解锁这类"缺失应视为可用"的语义（旧版引擎不返回该字段，
  /// 此时不该把整个鉴赏目录变灰）需要显式传 true。
  bool boolean(int id, {bool fallback = false}) {
    final value = _value(id, KzFieldTypes.boolean);
    if (value == null || value.isEmpty) {
      return fallback;
    }
    return value[0] != 0;
  }

  String str(int id) {
    final value = _value(id, KzFieldTypes.str);
    if (value == null) {
      return '';
    }
    return utf8.decode(value, allowMalformed: true);
  }

  Uint8List raw(int id) {
    final value = _value(id, KzFieldTypes.bytes);
    if (value == null) {
      return Uint8List(0);
    }
    return Uint8List.fromList(value);
  }

  List<String> strList(int id) {
    final field = _fields[id];
    if (field == null || field.type != KzFieldTypes.list) {
      return const <String>[];
    }
    final out = <String>[];
    final payload = field.value;
    final data = ByteData.sublistView(payload);
    var offset = 0;
    while (offset + 8 <= payload.length) {
      final type = data.getUint8(offset + 2);
      final length = data.getUint32(offset + 4, Endian.little);
      offset += 8;
      if (length > payload.length - offset) {
        break;
      }
      if (type == KzFieldTypes.str) {
        out.add(utf8.decode(
            Uint8List.sublistView(payload, offset, offset + length),
            allowMalformed: true));
      }
      offset += length;
    }
    return out;
  }

  void each(int id, void Function(KzReader sub) visit) {
    final field = _fields[id];
    if (field == null || field.type != KzFieldTypes.list) {
      return;
    }
    final payload = field.value;
    final data = ByteData.sublistView(payload);
    var offset = 0;
    while (offset + 8 <= payload.length) {
      final type = data.getUint8(offset + 2);
      final length = data.getUint32(offset + 4, Endian.little);
      offset += 8;
      if (length > payload.length - offset) {
        return;
      }
      if (type == KzFieldTypes.list) {
        visit(KzReader(
            Uint8List.sublistView(payload, offset, offset + length)));
      }
      offset += length;
    }
  }
}

class KzResponse {
  final int status;
  final int seq;
  final KzReader reader;
  const KzResponse(this.status, this.seq, this.reader);
}

Uint8List kzBuildRequest(int op, int seq, Uint8List payload) {
  final out = Uint8List(16 + payload.length);
  final data = ByteData.sublistView(out);
  out[0] = 0x4B;
  out[1] = 0x5A;
  out[2] = 0x45;
  out[3] = 0x51;
  data.setUint16(4, op, Endian.little);
  data.setUint16(6, 0, Endian.little);
  data.setUint32(8, seq, Endian.little);
  data.setUint32(12, payload.length, Endian.little);
  out.setRange(16, out.length, payload);
  return out;
}

Uint8List kzBuildErrorResponse(int seq, String message) {
  final writer = KzWriter();
  writer.str(42, message);
  return kzBuildResponse(1, seq, writer.takeBytes());
}

Uint8List kzBuildResponse(int status, int seq, Uint8List payload) {
  final out = Uint8List(16 + payload.length);
  final data = ByteData.sublistView(out);
  out[0] = 0x4B;
  out[1] = 0x5A;
  out[2] = 0x45;
  out[3] = 0x53;
  data.setUint16(4, status, Endian.little);
  data.setUint16(6, 0, Endian.little);
  data.setUint32(8, seq, Endian.little);
  data.setUint32(12, payload.length, Endian.little);
  out.setRange(16, out.length, payload);
  return out;
}

KzResponse kzParseResponse(Uint8List bytes) {
  if (bytes.length < 16) {
    throw const KzWireException('response too short');
  }
  if (bytes[0] != 0x4B ||
      bytes[1] != 0x5A ||
      bytes[2] != 0x45 ||
      bytes[3] != 0x53) {
    throw const KzWireException('bad response magic');
  }
  final data = ByteData.sublistView(bytes);
  final status = data.getUint16(4, Endian.little);
  final seq = data.getUint32(8, Endian.little);
  final length = data.getUint32(12, Endian.little);
  if (length > bytes.length - 16) {
    throw const KzWireException('truncated response payload');
  }
  return KzResponse(
      status, seq, KzReader(Uint8List.sublistView(bytes, 16, 16 + length)));
}
