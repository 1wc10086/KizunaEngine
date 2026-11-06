class KzOp {
  static const int open = 1;
  static const int info = 2;
  static const int start = 3;
  static const int run = 4;
  static const int advance = 5;
  static const int choose = 6;
  static const int pointer = 7;
  static const int compose = 8;
  static const int scenes = 9;
  static const int slots = 10;
  static const int save = 11;
  static const int load = 12;
  static const int assetSize = 13;
  static const int readAsset = 14;
  static const int audio = 15;
  static const int text = 16;
  static const int choices = 17;
  static const int state = 18;
  static const int textList = 19;
  static const int lastError = 20;
  static const int brands = 21;
  static const int detect = 22;
  static const int scan = 23;
  static const int submitName = 24;
  static const int audioDone = 25;
  static const int mapButtons = 26;
  static const int mapMask = 27;
  static const int selectItems = 28;
  static const int selectFrame = 29;
  static const int composeImages = 30;
  static const int setPreview = 31;
  static const int slotPreview = 32;
  static const int cgCatalog = 33;
  static const int setOption = 34;
  static const int key = 35;
  static const int wheel = 36;
  // OpFrame：一次 RPC 取齐 step/text/state/audio（字段号复用既有 op，见 engine_bridge.frame()）。
  static const int frame = 37;
  // 主界面（标题菜单）：引擎给布局，宿主回点击。
  static const int titleMenu = 38;
  static const int titleSelect = 39;
  // 影片：引擎给路径，宿主用 fvp 播完后回调。
  static const int movie = 40;
  static const int movieDone = 41;
}

class KzStatus {
  static const int ok = 0;
  static const int error = 1;
  static const int notFound = 2;
  static const int badRequest = 3;
}

class KzStep {
  static const int cont = 0;
  static const int awaitInput = 1;
  static const int awaitChoice = 2;
  static const int awaitSelect = 3;
  static const int wait = 4;
  static const int finished = 5;
  static const int faulted = 6;
  static const int awaitName = 7;
  // awaitTitle：停在主界面（标题菜单），等宿主回传点击。
  static const int awaitTitle = 8;
  // awaitMovie：脚本在等影片播完，宿主用 fvp 播完再调 movieDone。
  static const int awaitMovie = 9;
}

class KzPointer {
  static const int move = 0;
  static const int press = 1;
  static const int release = 2;
  static const int tap = 3;
  static const int rightClick = 4;
  static const int cancelMapSelect = 5;
  // drag：触屏纵向拖动（引擎坐标）。只滚动当前打开的系统界面（回想/BGV），
  // 绝不能推进对白——宿主靠这一点区分"滑动"与"点击"。
  static const int drag = 6;
}

class KzAudioKind {
  static const int bgm = 0;
  static const int se = 1;
  static const int voice = 2;
  static const int stopBgm = 3;
  static const int stopVoice = 4;
  static const int stopSe = 5;
  static const int voiceSequence = 6;
  static const int volume = 7;
  static const int fadeOut = 8;
  static const int stopAll = 9;
}

class KzOption {
  static const int showMessageFrame = 0;
  // 宿主跳过状态：引擎据此绘制原版 SKIPPING 覆盖条。
  static const int skipOverlay = 1;
}

class KzField {
  static const int f1 = 1;
  static const int f2 = 2;
  static const int f3 = 3;
  static const int f4 = 4;
  static const int f5 = 5;
  static const int f6 = 6;
  static const int f7 = 7;
  static const int f8 = 8;
  static const int f9 = 9;
  static const int f10 = 10;
  static const int f11 = 11;
  static const int f12 = 12;
  static const int f13 = 13;
  static const int f14 = 14;
  static const int f15 = 15;
  static const int f16 = 16;
  static const int f17 = 17;
  static const int f18 = 18;
  static const int f19 = 19;
  static const int f20 = 20;
  static const int f21 = 21;
  static const int f22 = 22;
  static const int f24 = 24;
  static const int f25 = 25;
  static const int f26 = 26;
  static const int f27 = 27;
  static const int f28 = 28;
  static const int f29 = 29;
  static const int f30 = 30;
  static const int f31 = 31;
  static const int f32 = 32;
  static const int f33 = 33;
  static const int f34 = 34;
  static const int f35 = 35;
  static const int f36 = 36;
  static const int f37 = 37;
  static const int f38 = 38;
  static const int f39 = 39;
  static const int f40 = 40;
  static const int f41 = 41;
  static const int f42 = 42;
  static const int f44 = 44;
  static const int f45 = 45;
  static const int f46 = 46;
  static const int f47 = 47;
  static const int f48 = 48;
  static const int f49 = 49;
  static const int f51 = 51;
  static const int f52 = 52;
  static const int f53 = 53;
  static const int f54 = 54;
  static const int f56 = 56;
  static const int f57 = 57;
  static const int f58 = 58;
  static const int f59 = 59;
  static const int f60 = 60;
  static const int f61 = 61;
  static const int f62 = 62;
  static const int f63 = 63;
  static const int f64 = 64;
  static const int f65 = 65;
  static const int f66 = 66;
  static const int f67 = 67;
  static const int f68 = 68;
  static const int f69 = 69;
  static const int f70 = 70;
  static const int f71 = 71;
  static const int f72 = 72;
  static const int f73 = 73;
  static const int f74 = 74;
  static const int f75 = 75;
  static const int f76 = 76;
  static const int f77 = 77;
  static const int f78 = 78;
  static const int f79 = 79;
  static const int f80 = 80;
  static const int f81 = 81;
  static const int f82 = 82;
  static const int f83 = 83;
  static const int f84 = 84;
  static const int f85 = 85;
  static const int f86 = 86;
  // f87：鉴赏条目（CG 变体 / 场景）是否已解锁。
  // CG 与场景共用同一个字段号：语义都是"这一项在鉴赏里是否可见"，
  // 宿主用同一个读法，不必为两者各记一个编号。
  static const int f87 = 87;
  // f88..f91：宿主自动推进/跳过策略。MessageUnread 是"当前句在显示前未读"，
  // 其余三项直接来自引擎设置；放在同一组里读取，避免跨帧不一致。
  static const int f88 = 88;
  static const int f89 = 89;
  static const int f90 = 90;
  static const int f91 = 91;
}
