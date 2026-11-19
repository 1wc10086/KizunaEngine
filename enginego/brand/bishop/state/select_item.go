package state

type SelectItem struct {
	Index  int
	Text   string
	Frame  string
	X      int
	Y      int
	Width  int
	Height int
	// Frames 是选项条图片的竖排帧数；0 表示旧的 2 帧约定。
	Frames int
	// SelectedFrame 是悬停时替换选项条用的图片名。零值/空字符串保持旧行为：
	// 悬停切 Frame 的悬停帧；有值时画 SelectedFrame 的同帧位切片。
	SelectedFrame string
	TextX         int
	TextY         int
	Alignment     int
	DefaultColor  uint32
	SelectColor   uint32
	// Head/Foot 是选项条上下装饰图；HeadHeight/FootHeight 参与整体排布。
	// 命中区域只算中间 40px 左右的条，装饰图不扩大点击范围。
	Head       string
	Foot       string
	HeadHeight int
	FootHeight int
}

func (s SelectItem) Color(active bool) uint32 {
	if active {
		return s.SelectColor
	}
	return s.DefaultColor
}
