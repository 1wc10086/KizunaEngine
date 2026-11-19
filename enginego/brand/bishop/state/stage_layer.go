package state

type StageLayer struct {
	id       int
	kind     StageLayerKind
	name     string
	alpha    float64
	visible  bool
	revision int
	artworks []LayerArtwork
}

func NewStageLayer(id int, kind StageLayerKind) *StageLayer {
	return &StageLayer{id: id, kind: kind, alpha: 1.0, visible: true}
}

func (l *StageLayer) Id() int {
	return l.id
}

func (l *StageLayer) Kind() StageLayerKind {
	return l.kind
}

func (l *StageLayer) Name() string {
	return l.name
}

func (l *StageLayer) Alpha() float64 {
	return l.alpha
}

func (l *StageLayer) SetAlpha(value float64) {
	l.alpha = value
}

func (l *StageLayer) Visible() bool {
	return l.visible
}

func (l *StageLayer) SetVisible(value bool) {
	l.visible = value
}

func (l *StageLayer) Revision() int {
	return l.revision
}

func (l *StageLayer) Artworks() []LayerArtwork {
	return l.artworks
}

func (l *StageLayer) IsDrawable() bool {
	return len(l.artworks) > 0 && l.alpha > 0 && l.visible
}

func (l *StageLayer) Set(name string, kind StageLayerKind, artworks []LayerArtwork) {
	l.kind = kind
	l.name = name
	l.artworks = append(l.artworks[:0], artworks...)
	l.revision++
}

// Append 把图层追加到已有内容后面，而不是替换。
//
// IMAGE_EX 的"前景层"语义是往同一层上继续贴图：黒の教室 的地图先用
// IMAGE 铺底图，再用 IMAGE_EX 依次画地点事件条、周数卡片、星期卡片等
// 十几个部件。每条都替换整层的话，最后只剩最后一个部件，而且丢失了
// 各自的坐标（表现为小字/周卡片跑到别处）。
func (l *StageLayer) Append(name string, kind StageLayerKind, artworks []LayerArtwork) {
	l.kind = kind
	l.name = name
	l.artworks = append(l.artworks, artworks...)
	l.revision++
}

// Translate 把这一层里的所有部件按同一个位移移动。
//
// LAYER_PARAM type=0 是"把层号 N 移到 (x,y)"。地图卡是一层一张图，
// 直接设到目标点即可；对多部件层（日期/周数组合）必须保持部件之间的
// 相对位置，所以按 layer 内部第一个部件的当前位置算 delta 后整体平移，
// 而不是把每个部件都吸到同一点。
func (l *StageLayer) Translate(dx, dy int) {
	if dx == 0 && dy == 0 {
		return
	}
	for index := range l.artworks {
		l.artworks[index].X += dx
		l.artworks[index].Y += dy
	}
	l.revision++
}

// ReplaceArtwork 原位替换一个部件（CHANGE_IMAGE 对"多层合成层"的语义）。
// 只换图、不动坐标，其它部件保持原样：地图上切换某个地点的状态条时，
// 不能把同层的日期/周数/其它地点条一起清掉。
func (l *StageLayer) ReplaceArtwork(index int, artwork LayerArtwork) bool {
	if index < 0 || index >= len(l.artworks) {
		return false
	}
	l.artworks[index] = artwork
	l.revision++
	return true
}

func (l *StageLayer) Reset() {
	l.name = ""
	l.alpha = 1.0
	l.visible = true
	l.artworks = l.artworks[:0]
	l.revision++
}
