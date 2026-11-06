package wire

const (
	OpOpen      uint16 = 1
	OpInfo      uint16 = 2
	OpStart     uint16 = 3
	OpRun       uint16 = 4
	OpAdvance   uint16 = 5
	OpChoose    uint16 = 6
	OpPointer   uint16 = 7
	OpCompose   uint16 = 8
	OpScenes    uint16 = 9
	OpSlots     uint16 = 10
	OpSave      uint16 = 11
	OpLoad      uint16 = 12
	OpAssetSize uint16 = 13
	OpReadAsset uint16 = 14
	OpAudio     uint16 = 15
	OpText      uint16 = 16
	OpChoices   uint16 = 17
	OpState     uint16 = 18
	OpTextList  uint16 = 19
	OpLastError uint16 = 20
	OpBrands    uint16 = 21
	OpDetect    uint16 = 22
)

const (
	OpScan          uint16 = 23
	OpSubmitName    uint16 = 24
	OpAudioDone     uint16 = 25
	OpMapButtons    uint16 = 26
	OpMapMask       uint16 = 27
	OpSelectItems   uint16 = 28
	OpSelectFrame   uint16 = 29
	OpComposeImages uint16 = 30
	OpSetPreview    uint16 = 31
	OpSlotPreview   uint16 = 32
	OpCgCatalog     uint16 = 33
	OpSetOption     uint16 = 34
	OpKey           uint16 = 35
	OpWheel         uint16 = 36
	// OpFrame 是"一次 RPC 取齐"：step + text + state + audio 事件列表一起返回。
	// 宿主每次 _sync 原本要 4~6 个 RPC（每个都是一次 isolate 往返 + handle.mu），
	// 合并后只剩一次。字段号全部复用旧 op 的字段号（step=f10，text=f30/f31/f32，
	// state=f35..f79，audio 列表=f26），旧 op 保留，Dart 侧用同一套读法。
	OpFrame uint16 = 37
	// OpTitleMenu 取主界面菜单项；OpTitleSelect 回传玩家点了哪一项。
	// 主界面是"引擎驱动、宿主绘制"的界面，必须有两个方向：宿主问布局、宿主报点击。
	OpTitleMenu   uint16 = 38
	OpTitleSelect uint16 = 39
	// OpMovie 取当前待播影片的绝对路径 + 序号，供宿主用 fvp 解码；
	// OpMovieDone 在播完后通知引擎继续脚本。
	OpMovie     uint16 = 40
	OpMovieDone uint16 = 41
)

const (
	StatusOK         uint16 = 0
	StatusError      uint16 = 1
	StatusNotFound   uint16 = 2
	StatusBadRequest uint16 = 3
)
