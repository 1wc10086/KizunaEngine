package bdt

type BdtVariant struct {
	Name                 string
	UsesContainerArchive bool
	// WindowImage 是主对话框素材（放課後 的 sb_win：640x120，蓝色为透明键）。
	// 为空时退回纯色矩形（旧 Manin 路线）。
	WindowImage string
	// WindowImageColorKey 是素材里的透明色（0xRRGGBB）。sb_win 用纯蓝。
	WindowImageColorKey uint32
	// TextWidth 是正文最大宽度；0 时按窗口宽减去左右边距。
	TextWidth         int
	WindowLeft        int
	WindowTop         int
	WindowWidth       int
	WindowHeight      int
	TextLeft          int
	TextTop           int
	TextLineHeight    int
	TextCharsPerLine  int
	FaceLeft          int
	FaceTop           int
	FaceSize          int
	SystemBarImage    string
	NoFacePlaceholder string
}

var ContainerVariant = BdtVariant{
	Name:                 "container",
	UsesContainerArchive: true,
	// 原引擎把 sb_win 贴在屏幕底部：640x120，可见框体 x=36..531、y=4..95。
	// 旧实现用 640x240 的纯色矩形，正好高出一倍，玩家会看到信息框占了半个屏幕。
	WindowImage:         "sb_win",
	WindowImageColorKey: 0x0000FF,
	WindowLeft:          0,
	WindowTop:           360,
	WindowWidth:         640,
	WindowHeight:        120,
	TextLeft:            44,
	TextTop:             390,
	TextWidth:           480,
	TextLineHeight:      20,
	TextCharsPerLine:    37,
	FaceLeft:            0,
	FaceTop:             0,
	FaceSize:            0,
	SystemBarImage:      "",
	NoFacePlaceholder:   "",
}

var ManinVariant = BdtVariant{
	Name:                 "manin",
	UsesContainerArchive: false,
	// sb_win 是 640x120，纯黄 (0xFFFF00) 是透明键；左 96x96 放表情，
	// 右侧 480x96 放正文。坐标来自素材像素，不是估算。
	WindowImage:         "sb_win",
	WindowImageColorKey: 0x00FFFF00,
	WindowLeft:          0,
	WindowTop:           360,
	WindowWidth:         640,
	WindowHeight:        120,
	TextLeft:            132,
	TextTop:             402,
	TextWidth:           480,
	TextLineHeight:      20,
	TextCharsPerLine:    34,
	FaceLeft:            12,
	FaceTop:             380,
	FaceSize:            96,
	SystemBarImage:      "qb_win",
	NoFacePlaceholder:   "futaxxxx",
}
