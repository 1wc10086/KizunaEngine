package state

type StageButton struct {
	Image  string
	X      int
	Y      int
	Value  int
	Frames int
	Width  int
	Height int
	// Disabled 表示按钮当前不可点（例如工作画面的 START 在四人
	// 都还没选工作时不可用）。渲染层画第 3 帧（灰态），命中判定跳过。
	// 条件由 Session 每帧根据 define.bsa 的按钮定义重新计算。
	Disabled bool
	// HitMask 是地图卡的像素命中掩码（<image>_hit）。非 nil 时点击判定
	// 只看掩码，不再用整张图的矩形；KZK、屈辱2 的卡图都是 1024x768
	// 全屏透明叠层，矩形命中会让所有卡重叠成同一个热区。
	HitMask *StageHitMask
	// HitX/HitY/HitWidth/HitHeight 是掩码非零区域的屏幕坐标外接矩形。
	// 只用于把地图热区导出给宿主（MapButton 的 X/Y/Width/Height）和
	// 按索引点击时算中心点；绘制仍用 X/Y 和图片自身尺寸。
	HitX      int
	HitY      int
	HitWidth  int
	HitHeight int
	// IgnoreImageOffset 表示 X/Y 已是最终屏幕坐标，渲染时不要再叠加图片
	// 自带的 OffsetX/OffsetY。旧式标题菜单图（KD 的 title_menu_*）在
	// BSS-Composition 里就把屏幕位置写进了图头；配置又给了一遍同样的坐标，
	// 两套相加会把按钮画到屏幕外。现代作品的按钮图 Offset 通常是 0，
	// 所以只有标题按钮显式打开这个开关。
	IgnoreImageOffset bool
	// Frame 与 HoverOnly 只给标题这类“平时不画、悬停才亮”的按钮用：HoverOnly 为真时
	// 平时不画，悬停时只画 Frame 那一帧。非 HoverOnly 的按钮完全沿用原来的帧选择，
	// 所以 Frame 的零值不会误切帧。
	Frame     int
	HoverOnly bool
	// HoverFrame/HasHoverFrame 指定悬停时画第几帧。HasHoverFrame=false
	// 时沿用默认"第 1 帧"。
	//
	// OH 的地图卡是 5 帧竖排：0..3 是表情变化，第 4 帧才是带名字的高亮
	// 卡面（[button object] 的 `map_ch1_norm on = 4`）。固定画第 1 帧会
	// 显示成另一张普通表情，玩家看不出卡片已经被选中。
	// 用独立布尔位而不是 -1 哨兵，是因为大量旧代码直接构造 StageButton{},
	// 零值 0 会被误当成"悬停帧=0"。
	HoverFrame    int
	HasHoverFrame bool
	// Clip/Atlas/AtlasAxis/HoverClip 与 config.UIButton 同义：BSXScript 1.0
	// 这一代把标题菜单、地图站点、设置控件烘进一张大图，按钮只画其中一个
	// 矩形。Atlas 关闭时 Clip 的旧语义保持不变（整图竖排裁切）。
	Clip      [4]int
	Atlas     bool
	AtlasAxis int
	HoverClip [4]int
	// Layer 是 CHAR 分配的舞台层号（type=160 的地图卡）。
	//
	// 放課後3 的剧本先用 CHAR 把角色卡登记成按钮（此时坐标是 0,0），
	// 画完所有卡后再用 LAYER_PARAM type=0 按层号把卡移到
	// #X.mapXa/mapYa 记录的地点。没有层号就无法把"这条 LAYER_PARAM
	// 要移动的卡"和按钮对应起来，所有卡会叠在左上角。
	//
	// -1 表示按钮不绑定层（IMAGE/IMAGE_EX 直接给了坐标的按钮）。
	Layer int
}

const StageButtonFrameCount = 2

func NewStageButton(image string, x, y, value int) StageButton {
	return NewStageButtonWithSize(image, x, y, value, 0, 0, 0)
}

func NewStageButtonWithFrames(image string, x, y, value, frames int) StageButton {
	return NewStageButtonWithSize(image, x, y, value, frames, 0, 0)
}

func NewStageButtonWithSize(image string, x, y, value, frames, width, height int) StageButton {
	if frames <= 1 {
		frames = StageButtonFrameCount
	}
	return StageButton{Image: image, X: x, Y: y, Value: value, Frames: frames, Width: width, Height: height, Frame: -1, Layer: -1, HoverFrame: -1}
}
