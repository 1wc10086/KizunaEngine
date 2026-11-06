import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'kernel_bridge.dart';

final kernelBridgeProvider = Provider<KernelBridge>((ref) {
  return KernelBridge.open();
});
