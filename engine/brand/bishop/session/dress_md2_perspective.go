package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/gfx"
)

// dress_md2_perspective.go 实现 MD2 DRESS 画面的拖拽透視。
//
// 手册：按住左键在立绘衣服区域拖拽，用 4 个笔「衣服消去/衣服復活/
// 下着消去/下着復活」把光标范围透視；切换立绘/关闭画面时重置，
// 不能存档。只在 `s.config.Dress.Atlas`（MD2）启用。
//
// 实现：与屏幕同尺寸的掩码（1 字节/像素，只用 0/1/2 三种标记）+ 立绘
// 后台画布。渲染时先画当前阶段，再把掩码区域用更少衣服的 stage 图
// 覆盖：mask 1 = 衣服消去（显示 stage+1），mask 2 = 下着消去（显示
// stage 2）；笔 1/3 是擦回（excise 的反笔），把对应像素的标记清掉
// 回到普通 stage。Pointer op 1（press）/0（hover 移动）/3（release）
// 按 md2DressPen 刷格：左键拖动时 op 0 在 pressHandled 期间也应刷格，
// 与普通悬停只高亮不同——原版透視是按住拖动边画边透，所以这里
// 拖动期间的 hover 坐标也走刷格。
//
// 立绘素材名：剧本里已经在舞台上的 layer 是 bu01f0011a0 这一格式
// （末位是 stage）；透視用同一套名字把末位换成 1/2 试探存在性，
// 找不到就不覆盖那区域（避免把立绘弄丢）。
// 为什么不用另一套阶段图：MD2 立绘名末位就是阶段号，换一位就够了，
// 再找另一套命名只会产生找不到的图。

// md2DressPerspState 是一份透視状态，不写进存档，关闭 DRESS 时丢弃。
//
// mask 与画布是一一对应的：nil 表示还没有拖过，渲染时不做覆盖。
type md2DressPerspState struct {
	mask []byte
	w    int
	h    int
}

// md2DressPerspEnsure 保证掩码与当前画布同尺寸，尺寸变化时丢弃重建。
func (s *Session) md2DressPerspEnsure(state *md2DressPerspState, w, h int) *md2DressPerspState {
	if state == nil {
		state = &md2DressPerspState{}
	}
	if w <= 0 {
		w = s.width
	}
	if h <= 0 {
		h = s.height
	}
	if w <= 0 || h <= 0 {
		return state
	}
	if state.mask == nil || state.w != w || state.h != h {
		state.mask = make([]byte, w*h)
		state.w = w
		state.h = h
	}
	return state
}

// md2DressPerspBrush 按笔刷一格掩码。
//
// pen: 0=衣服消去（mask 1）、1=衣服復活（清 mask 1）、
//
//	2=下着消去（mask 2）、3=下着復活（清 mask 2）。
//
// 1/3 只清掉自己那一类的标记，不去动另一类的，避免两笔互相覆盖。
func (s *Session) md2DressPerspBrush(state *md2DressPerspState, x, y int) {
	if state == nil || state.mask == nil {
		return
	}
	if x < 0 || y < 0 || x >= state.w || y >= state.h {
		return
	}
	pen := s.md2DressPen
	idx := y*state.w + x
	switch pen {
	case 0:
		state.mask[idx] = 1
	case 1:
		if state.mask[idx] == 1 {
			state.mask[idx] = 0
		}
	case 2:
		state.mask[idx] = 2
	case 3:
		if state.mask[idx] == 2 {
			state.mask[idx] = 0
		}
	}
}

// md2DressPerspHasMask 判断掩码里是否有实际刷过的区域。
func md2DressPerspHasMask(state *md2DressPerspState) bool {
	if state == nil || state.mask == nil {
		return false
	}
	for _, v := range state.mask {
		if v != 0 {
			return true
		}
	}
	return false
}

// md2DressTransparentName 把立绘名末位 stage 换成目标 stage，
// 素材不存在时返回空（调用方此时不覆盖那块区域）。
func (s *Session) md2DressTransparentName(name string, stage int) string {
	if len(name) < 11 || name[0] != 'b' || name[1] != 'u' {
		return ""
	}
	last := name[len(name)-1]
	if last < '0' || last > '2' {
		return ""
	}
	candidate := name[:len(name)-1] + string(byte('0'+stage))
	if candidate == name {
		return candidate
	}
	if s.resourceExists(candidate) {
		return candidate
	}
	return ""
}

// md2DressDrawPerspSolve 把掩码区域的像素用更少衣服的 stage 图覆盖。
//
// 看的是“舞台上现成的第二张图”的几何：同一名字只改末位，宽高不变，
// 所以直接按原立绘位置把第二张的像素按 mask 贴过去，形状对得齐。
// 用 compose 临时画布在原图位置画 stage+1 和 stage 2 两份，
// 再按 mask 把对应像素抄回主画布；alpha 用 stage 图自己的 alpha，
// 保证衣服边缘的半透明不被硬覆盖。
func (s *Session) md2DressDrawPerspSolve(canvas *gfx.BgraCanvas, state *md2DressPerspState) {
	if state == nil || state.mask == nil || canvas == nil {
		return
	}
	if state.w != canvas.Width || state.h != canvas.Height {
		state = s.md2DressPerspEnsure(state, canvas.Width, canvas.Height)
	}
	if !md2DressPerspHasMask(state) {
		return
	}
	// 找到舞台上的立绘层名（bu 开头、末位为 stage 的那块就是它）。
	standeeName := ""
	if s.gameState != nil {
		for _, layer := range s.gameState.Layers() {
			for _, art := range layer.Artworks() {
				name := art.Image
				if len(name) >= 11 && name[0] == 'b' && name[1] == 'u' {
					standeeName = name
				}
			}
		}
	}
	if standeeName == "" {
		return
	}
	stage := clampInt(s.md2DressStage, 0, 2)
	// stage+1 只在当前 stage<2 时有意义；stage 2 是全裸图。
	nextStage := stage + 1
	if nextStage > 2 {
		nextStage = 2
	}
	nextName := s.md2DressTransparentName(standeeName, nextStage)
	nakedName := s.md2DressTransparentName(standeeName, 2)
	// 两张都失败时什么都不画：原立绘还在，避免透視把屏擦空。
	if nextName == "" && nakedName == "" {
		return
	}
	nextLayer := gfx.NewBgraCanvas(canvas.Width, canvas.Height)
	nakedLayer := gfx.NewBgraCanvas(canvas.Width, canvas.Height)
	s.md2DressPerspPaint(nextLayer, nextName)
	s.md2DressPerspPaint(nakedLayer, nakedName)
	// 按 mask 抄像素：mask 1 取 nextLayer，mask 2 取 nakedLayer；
	// 其余保持主画布不变。
	target := canvas.Buffer
	nextPx := nextLayer.Buffer
	nakedPx := nakedLayer.Buffer
	for i := 0; i < len(state.mask); i++ {
		v := state.mask[i]
		if v == 0 {
			continue
		}
		off := i * 4
		var src []byte
		if v == 1 {
			src = nextPx
		} else {
			src = nakedPx
		}
		// src alpha 为 0 说明这块没有更少衣服的图（衣服只在胸口等），
		// 此时保持原像素，避免把下着区误涂成裸体。
		if src[off+3] == 0 {
			continue
		}
		target[off] = src[off]
		target[off+1] = src[off+1]
		target[off+2] = src[off+2]
		target[off+3] = src[off+3]
	}
}

// md2DressPerspPaint 在临时层上按舞台位置画一张立绘图。
// 位置直接用 layer 在舞台上的 Artworks 坐标；图是合成图，用与剧情
// 相同的 Draw 路径画。
func (s *Session) md2DressPerspPaint(layer *gfx.BgraCanvas, name string) {
	if name == "" || layer == nil {
		return
	}
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	if s.gameState != nil {
		for _, l := range s.gameState.Layers() {
			for _, art := range l.Artworks() {
				if art.Image == "" {
					continue
				}
				// 同尺寸同名族的那张 artwork 的坐标就是立绘落点。
				if len(art.Image) >= 11 && len(name) >= 11 && art.Image[:10] == name[:10] {
					s.drawBuexComposedFrames(layer, image, art.X, art.Y, 1, 1)
					return
				}
			}
		}
	}
	// 找不到舞台层时按整张图居中放：宁可对不齐也不让透視画在错位上，
	// 上层 mask 的 alpha 门会挡住大部分误擦。
	x := (layer.Width - image.Width) / 2
	y := (layer.Height - image.Height) / 2
	s.drawBuexComposedFrames(layer, image, x, y, 1, 1)
}

// md2DressPerspReset 清空透視（切换立绘/关闭画面时调用，不能存档）。
func (s *Session) md2DressPerspReset() {
	s.md2DressPersp = nil
	var _ = fmt.Sprintf
}

// md2DressBrushAt 是系统层在 Pointer op 1/0/3 时调用的刷格入口。
//
// 只在 MD2 DRESS（Atlas）启用，且要点在立绘的衣服区域附近；
// 坐标在按钮上时不刷，避免点笔时连带刷到衣服。
// 为什么不用 hitSystemUI 的 hit：brush 需要坐标而不是 hit，
// 从绘图面区分要不要画远比从 hit 区分经济。
func (s *Session) md2DressBrushAt(x, y int) {
	if s.config == nil || !s.config.Dress.Atlas {
		return
	}
	if s.ui == nil || s.ui.kind != uiDress {
		return
	}
	if s.md2DressPersp == nil {
		s.md2DressPersp = s.md2DressPerspEnsure(s.md2DressPersp, s.width, s.height)
		if s.md2DressPersp == nil {
			return
		}
		if s.md2DressPersp.mask == nil {
			return
		}
	}
	// 点在笔/状态/返回按钮上不刷透视：左键拖动只在衣服区域刷。
	for _, button := range s.config.Dress.Pens {
		if hitButton(s, button, x, y) {
			return
		}
	}
	for _, button := range s.config.Dress.States {
		if hitButton(s, button, x, y) {
			return
		}
	}
	if hitButton(s, s.config.Dress.Back, x, y) {
		return
	}
	s.md2DressPerspBrush(s.md2DressPersp, x, y)
}
