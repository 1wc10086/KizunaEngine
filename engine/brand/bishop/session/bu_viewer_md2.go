package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// bu_viewer_md2.go 是 満淫電車2 的立ち絵鉴赏查看器。
//
// 素材命名（已从 md2.exe 反汇编与资源索引确认）：
//   - 身体图：bu<角色2位><视角 f|u>0<服装2位><下着1位>a<阶段0|1|2>
//     例如 bu01f0011a0、bu01f0016a2、bu02u0025a1。
//     视角 f/u 来自 cfg view signature=fu；服装取 cfg `chN wear`
//     （1,2,3,0，末尾 0 已在解析时去掉）；下着 1..7 来自 cfg
//     `chN under wear`；阶段 0=着衣/1=下着/2=全裸。
//   - 表情图：bu<角色2位><f|u>0001a<表情2位>，表情号来自 cfg
//     `chN face pose1`（1,2,3,4,6,7,8,9,10,11,12）；同目录还有
//     `...aNNeye` 眨眼层。
//   - 背景：资源里是 bg/bgNNNN.bsg（75 张，含变体），setting5 repeat x=30
//     表示 30 个背景开关（面板 pos 58,118，space 18,18）。
//
// 6 组开关（switch max state=3：常态/悬停/选中）：
//   - setting0 視点(2)=f/u；setting1 ぬぎぬぎ(3)=身体阶段 a0..a2；
//   - setting2 服装(4)=cfg chN wear 列表；setting3 下着(7)=1..7；
//   - setting4 表情(11)=cfg face pose1；setting5 背景(30)。
//   每组的 pos/space/repeat x 已在 cfg 解析到 config.BuViewer.Settings。
//
// 立绘鉴赏缩放：手册要求滚轮最多 2 倍、放大后拖拽平移、右键点击隐藏/
// 显示底部按钮。zoom/pan/panelShown 放在 md2BuState 里，滚轮方向：
// 宿主 wheel delta=-1 是向上（放大）、+1 向下（缩小）；触屏拖动用
// Pointer op 6。项目没有单独的 Pointer/Drag 回调，缩放与平移由系统
// 层在 Pointer op 1/3/6 与滚轮分派到 wheelBuex / dragSystemUI 时，
// 回调本文件的 md2BuWheel / md2BuDragOp6 / md2BuTogglePanel；本文件
// 暴露的入口函数不修改系统层，只读写 md2BuState。

// md2BuState 是 MD2 立绘鉴赏查看器的一份状态。
type md2BuState struct {
	character int
	// view 是视角字符 f/u（setting0，两档）。
	view byte
	// stage 是身体阶段 a0..a2（setting1）。
	stage int
	// wear 是服装编号（setting2，取 cfg chN wear 列表）。
	wear int
	// under 是下着编号 1..7（setting3）。
	under int
	// face 是表情编号（setting4，取 cfg face pose1）。
	face int
	// background 是背景下标 0..29（setting5）。
	background int
	// zoom/pan/panelShown 是手册的缩放/平移/面板显隐，重置见 openMD2BuViewer。
	zoom       float64
	panX       int
	panY       int
	panelShown bool
}

// md2BuViewLetters 是 setting0 的两档视角（cfg view signature=fu）。
func md2BuViewLetters(viewer config.BuViewer) []byte {
	var views []byte
	for i := 0; i < len(viewer.ViewSignature) && len(views) < 2; i++ {
		c := viewer.ViewSignature[i]
		if c == 'f' || c == 'u' {
			views = append(views, c)
		}
	}
	if len(views) == 0 {
		views = []byte{'f', 'u'}
	}
	return views
}

// md2BuWears 返回当前角色的服装编号表（cfg chN wear）。
func (s *Session) md2BuWears(state *md2BuState) []int {
	if state == nil {
		return []int{1}
	}
	viewer := s.config.Museum.BuViewer
	if state.character >= 0 && state.character < len(viewer.Characters) {
		if list := viewer.Characters[state.character].Wear; len(list) > 0 {
			return list
		}
	}
	return []int{1}
}

// md2BuFaces 返回当前角色的表情编号表（cfg chN face pose1）。
func (s *Session) md2BuFaces(state *md2BuState) []int {
	if state == nil {
		return []int{1}
	}
	viewer := s.config.Museum.BuViewer
	if state.character >= 0 && state.character < len(viewer.Characters) {
		faces := viewer.Characters[state.character].Faces
		if len(faces) > 0 && len(faces[0]) > 0 {
			return faces[0]
		}
	}
	return []int{1}
}

// md2BuBodyName 按精确命名拼出身体图名，组合不存在时按回退序试探。
//
// 命名：bu<角色2位><视角>0<服装2位><下着1位>a<阶段>，例如 bu01f0016a2。
// 回退序：当前组合 → 下着换默认（cfg under wear）→ 服装换第一件 →
// 阶段退到 0。每一项都要过 resourceExists，不能用旧的猜测格式。
func (s *Session) md2BuBodyName(state *md2BuState) string {
	if state == nil || s.res == nil {
		return ""
	}
	character := state.character + 1
	viewer := s.config.Museum.BuViewer
	view := state.view
	if view != 'f' && view != 'u' {
		view = 'f'
	}
	wears := s.md2BuWears(state)
	wear := state.wear
	if wear <= 0 {
		wear = 1
	}
	under := state.under
	if under < 1 || under > 7 {
		under = 1
		if state.character >= 0 && state.character < len(viewer.Characters) {
			if d := viewer.Characters[state.character].UnderWear; d >= 1 && d <= 7 {
				under = d
			}
		}
	}
	stage := clampInt(state.stage, 0, 2)
	// 回退：下着/服装/阶段逐级换，至少保证 stage=0 着衣能画出来。
	defaultUnder := under
	if state.character >= 0 && state.character < len(viewer.Characters) {
		if d := viewer.Characters[state.character].UnderWear; d >= 1 && d <= 7 {
			defaultUnder = d
		}
	}
	wearTries := []int{wear}
	if len(wears) > 0 && wears[0] != wear {
		wearTries = append(wearTries, wears[0])
	}
	if len(wearTries) == 1 && wear != 1 {
		wearTries = append(wearTries, 1)
	}
	for _, w := range wearTries {
		for _, u := range []int{under, defaultUnder, 1} {
			for _, st := range []int{stage, 0} {
				name := fmt.Sprintf("bu%02d%c0%02d%da%d", character, view, w, u, st)
				if s.resourceExists(name) {
					return name
				}
			}
		}
	}
	// 兜底：着衣基础图，避免整屏空白。
	for _, w := range wearTries {
		name := fmt.Sprintf("bu%02d%c0%02d%da0", character, view, w, defaultUnder)
		if s.resourceExists(name) {
			return name
		}
	}
	return ""
}

// md2BuFaceName 返回表情图名，不存在则返回空（渲染层此时不叠表情）。
func (s *Session) md2BuFaceName(state *md2BuState) string {
	if state == nil {
		return ""
	}
	character := state.character + 1
	view := state.view
	if view != 'f' && view != 'u' {
		view = 'f'
	}
	face := state.face
	if face <= 0 {
		face = 1
	}
	name := fmt.Sprintf("bu%02d%c0001a%02d", character, view, face)
	if s.resourceExists(name) {
		return name
	}
	return ""
}

// md2BuBackgroundNames 返回背景图名表（setting5，30 个开关）。
//
// 资源里是 bg/bgNNNN.bsg 的 75 张（含变体），资源索引按图名收录后
// 就是 bgNNNN；旧代码用 bg0011/bg0051/bg0101 的硬编码不走开关。
func (s *Session) md2BuBackgroundNames() []string {
	names := fallbackBackgroundNames(s.resourceImageNames())
	if len(names) > 0 {
		return names
	}
	fallback := []string{"bg0011", "bg0051", "bg0101"}
	out := fallback[:0:0]
	for _, name := range fallback {
		if s.resourceExists(name) {
			out = append(out, name)
		}
	}
	return out
}

// md2BuBackgroundName 返回当前背景图名，不存在返回空。
func (s *Session) md2BuBackgroundName(state *md2BuState) string {
	if state == nil {
		return ""
	}
	names := s.md2BuBackgroundNames()
	if len(names) == 0 {
		return ""
	}
	index := clampInt(state.background, 0, len(names)-1)
	if !s.resourceExists(names[index]) {
		for i, name := range names {
			if s.resourceExists(name) {
				index = i
				break
			}
		}
	}
	return names[index]
}

func (s *Session) openMD2BuViewer(character int) bool {
	viewer := s.config.Museum.BuViewer
	if !viewer.Atlas || s.ui == nil {
		return false
	}
	state := &md2BuState{
		character:  clampInt(character, 0, 6),
		view:       'f',
		stage:      0,
		zoom:       1,
		panelShown: true,
	}
	// 初始值取 cfg 表：服装取第一件，下着取 cfg 默认，表情取第一张。
	if state.character < len(viewer.Characters) {
		list := viewer.Characters[state.character].Wear
		if len(list) > 0 {
			state.wear = list[0]
		}
		if d := viewer.Characters[state.character].UnderWear; d >= 1 && d <= 7 {
			state.under = d
		}
	}
	if state.wear <= 0 {
		state.wear = 1
	}
	if state.under < 1 || state.under > 7 {
		state.under = 1
	}
	faces := s.md2BuFaces(state)
	if len(faces) > 0 {
		state.face = faces[0]
	} else {
		state.face = 1
	}
	s.ui.md2Bu = state
	s.ui.kind = uiMuseumBU
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return true
}

// md2BuSwitchSize 返回开关按钮的单格尺寸。
func md2BuSwitchSize(viewer config.BuViewer) (int, int) {
	width, height := 16, 16
	if image := viewer.Switch; !image.Empty() {
		if image.Clip[2] > 0 {
			width = image.Clip[2]
		}
		if image.Clip[3] > 0 {
			height = image.Clip[3]
		}
	}
	if width <= 0 {
		width = 16
	}
	if height <= 0 {
		height = 16
	}
	return width, height
}

// md2BuSwitchPos 返回第 idx 个开关在面板上的左上角（setting 的 pos/space/repeat x）。
func md2BuSwitchPos(viewer config.BuViewer, settingIndex, idx, switchW, switchH int) (int, int) {
	setting := md2BuSetting(viewer, settingIndex)
	columns := setting.Columns
	if columns <= 0 {
		columns = 1
	}
	col := idx % columns
	row := idx / columns
	px := viewer.BaseX + setting.X + col*setting.SpaceX
	py := viewer.BaseY + setting.Y + row*setting.SpaceY
	return px, py
}

// md2BuDrawSwitch 画一组开关；selected 是当前选中的下标，hoverKind/hoverIndex 用于悬停。
func (s *Session) md2BuDrawSwitch(canvas *gfx.BgraCanvas, viewer config.BuViewer, settingIndex, count, selected int) {
	ui := s.ui
	switchW, switchH := md2BuSwitchSize(viewer)
	_ = switchH
	for index := 0; index < count; index++ {
		x, y := md2BuSwitchPos(viewer, settingIndex, index, switchW, switchH)
		frame := 0
		if index == selected {
			frame = 2
		} else if ui != nil && ui.hover.index == index && md2BuHoverMatches(ui.hover, settingIndex) {
			frame = 1
		}
		button := viewer.Switch
		button.X, button.Y = x, y
		s.drawFrameButton(canvas, button, frame)
	}
}

// md2BuHoverMatches 判断悬停命中是否属于这一组开关。
// 旧 hitBUAppearance 复用在 stage/其它组上，各组绘制时靠索引对齐即可；
// 这里只要不是返回/关闭就都按停留处理，选中帧已在上面单独给出。
func md2BuHoverMatches(hit uiHit, settingIndex int) bool {
	return hit.kind == hitBUAppearance || hit.kind == hitBUTarget || hit.kind == hitBUView ||
		hit.kind == hitBUWear || hit.kind == hitBUUnder || hit.kind == hitBUFace ||
		hit.kind == hitBUBackground
}

// md2BuZoomPan 供渲染层计算缩放/平移后的图与位置。
// zoom 在 1..2 之间，pan 受 zoom 限幅，保证图像始终盖满画面。
func (s *Session) md2BuZoomPan(state *md2BuState, canvasWidth, canvasHeight int) (zoom float64, panX, panY int) {
	if state == nil {
		return 1, 0, 0
	}
	zoom = state.zoom
	if zoom < 1 {
		zoom = 1
	}
	if zoom > 2 {
		zoom = 2
	}
	limitX := int(float64(canvasWidth) * (zoom - 1) / 2)
	limitY := int(float64(canvasHeight) * (zoom - 1) / 2)
	panX = clampInt(state.panX, -limitX, limitX)
	panY = clampInt(state.panY, -limitY, limitY)
	return zoom, panX, panY
}

// md2BuWheel 处理 BU 查看器滚轮：delta=-1 向上（放大）、+1 向下（缩小）。
func (s *Session) md2BuWheel(delta int32) bool {
	ui := s.ui
	if ui == nil || ui.md2Bu == nil || delta == 0 {
		return false
	}
	state := ui.md2Bu
	steps := -int(delta)
	if state.zoom == 0 {
		state.zoom = 1
	}
	state.zoom = clampFloat(state.zoom+float64(steps)*cgZoomStep, 1, cgZoomMax)
	if state.zoom == 1 {
		state.panX = 0
		state.panY = 0
	} else {
		// 缩放变化后立刻钳住 pan，防止缩小时 pan 越界留黑边。
		limitX := int(float64(s.width) * (state.zoom - 1) / 2)
		limitY := int(float64(s.height) * (state.zoom - 1) / 2)
		state.panX = clampInt(state.panX, -limitX, limitX)
		state.panY = clampInt(state.panY, -limitY, limitY)
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return true
}

// md2BuDragOp6 处理触屏拖动（Pointer op 6）：放大后平移视图。
func (s *Session) md2BuDragOp6(x, y int) bool {
	ui := s.ui
	if ui == nil || ui.md2Bu == nil {
		return false
	}
	state := ui.md2Bu
	if state.zoom <= 1 {
		return false
	}
	if ui.lastDragX < 0 {
		ui.lastDragX = x
		return false
	}
	limitX := int(float64(s.width) * (state.zoom - 1) / 2)
	limitY := int(float64(s.height) * (state.zoom - 1) / 2)
	state.panX = clampInt(state.panX+(x-ui.lastDragX), -limitX, limitX)
	state.panY = clampInt(state.panY+(y-ui.lastDragY), -limitY, limitY)
	ui.lastDragX = x
	ui.lastDragY = y
	return true
}

// md2BuTogglePanel 处理右键点击：隐藏/显示底部按钮（手册）。
func (s *Session) md2BuTogglePanel() bool {
	ui := s.ui
	if ui == nil || ui.md2Bu == nil {
		return false
	}
	ui.md2Bu.panelShown = !ui.md2Bu.panelShown
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return true
}

func (s *Session) drawMD2BuViewer(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.md2Bu == nil {
		return
	}
	state := ui.md2Bu
	viewer := s.config.Museum.BuViewer
	// 背景：setting5 背景开关选中的那张；不存在退回第一张可用的。
	background := s.md2BuBackgroundName(state)
	if background != "" {
		s.drawImageAt(canvas, background, 0, 0)
	} else {
		canvas.Clear(0, 0, 0, 255)
	}
	// 立绘：身体图 + 表情图（身体图只画第 0 帧，合成图走 drawBuexComposedFrames）。
	if name := s.md2BuBodyName(state); name != "" {
		if image := s.composer.Load(name); image != nil {
			limit := viewer.BaseY
			if limit <= 0 {
				limit = canvas.Height
			}
			width, height := image.Width, image.Height
			if height > limit && height > 0 {
				width = image.Width * limit / height
				height = limit
			}
			zoom, panX, panY := s.md2BuZoomPan(state, canvas.Width, canvas.Height)
			if zoom > 1 {
				width = int(float64(width) * zoom)
				height = int(float64(height) * zoom)
			}
			x := (canvas.Width-width)/2 + panX
			y := limit - height + panY
			canvas.BlitScaled(image, x, y, max(1, width), max(1, height), 255)
			if faceName := s.md2BuFaceName(state); faceName != "" {
				if face := s.composer.Load(faceName); face != nil {
					s.drawBuexComposedFrames(canvas, face, x, y, float64(width)/float64(max(1, image.Width)), s.buexEyePattern())
				}
			}
		}
	}
	// 控制面板；panelShown=false 时手册要求隐藏底部按钮。
	if !state.panelShown {
		return
	}
	if viewer.Base != "" {
		s.drawImageAt(canvas, viewer.Base, viewer.BaseX, viewer.BaseY)
	}
	// 6 组开关：setting0..5，每组按配置进驻，帧 0常态/1悬停/2选中。
	views := md2BuViewLetters(viewer)
	s.md2BuDrawSwitch(canvas, viewer, 0, len(views), md2BuIndexOfByte(views, state.view))
	s.md2BuDrawSwitch(canvas, viewer, 1, 3, state.stage)
	wears := s.md2BuWears(state)
	s.md2BuDrawSwitch(canvas, viewer, 2, len(wears), md2BuIndexOfInt(wears, state.wear))
	s.md2BuDrawSwitch(canvas, viewer, 3, 7, state.under-1)
	faces := s.md2BuFaces(state)
	s.md2BuDrawSwitch(canvas, viewer, 4, len(faces), md2BuIndexOfInt(faces, state.face))
	backgrounds := s.md2BuBackgroundNames()
	s.md2BuDrawSwitch(canvas, viewer, 5, len(backgrounds), state.background)
	// 返回提示：点面板外/右键都可回选人页。
	if s.textFont != nil {
		if image := s.textFont.Render("BACK", 16, 1, 1, 20, 80, 1, 0xffffffcc); image != nil {
			rendering.DrawTextImage(canvas, image, canvas.Width-90, 12)
		}
	}
}

func md2BuIndexOfByte(list []byte, value byte) int {
	for index, item := range list {
		if item == value {
			return index
		}
	}
	return 0
}

func md2BuIndexOfInt(list []int, value int) int {
	for index, item := range list {
		if item == value {
			return index
		}
	}
	return 0
}

func md2BuSetting(viewer config.BuViewer, index int) config.BuViewerSetting {
	if index >= 0 && index < len(viewer.Settings) {
		return viewer.Settings[index]
	}
	return config.BuViewerSetting{Columns: 1}
}

func (s *Session) md2BuHit(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.md2Bu == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.md2Bu
	viewer := s.config.Museum.BuViewer
	if !state.panelShown {
		// 面板隐藏时不再命中开关，点面板外仍可返回。
		if viewer.Base == "" {
			return uiHit{kind: hitBUClose}
		}
		image := s.composer.Load(viewer.Base)
		if image != nil && x >= viewer.BaseX && y >= viewer.BaseY &&
			x < viewer.BaseX+image.Width && y < viewer.BaseY+image.Height {
			return uiHit{kind: hitNone, index: -1}
		}
		return uiHit{kind: hitBUClose}
	}
	switchW, switchH := md2BuSwitchSize(viewer)
	_ = switchH
	// 命中顺序：背景(5) 最底层先判，视角(0) 最后，避免大组盖住小组合。
	hitGroup := func(settingIndex, count int, kind uiHitKind) (uiHit, bool) {
		for index := 0; index < count; index++ {
			px, py := md2BuSwitchPos(viewer, settingIndex, index, switchW, switchH)
			if x >= px && y >= py && x < px+switchW && y < py+switchH {
				return uiHit{kind: kind, index: index}, true
			}
		}
		return uiHit{}, false
	}
	if hit, ok := hitGroup(5, len(s.md2BuBackgroundNames()), hitBUBackground); ok {
		return hit
	}
	if hit, ok := hitGroup(4, len(s.md2BuFaces(state)), hitBUFace); ok {
		return hit
	}
	if hit, ok := hitGroup(3, 7, hitBUUnder); ok {
		return hit
	}
	if hit, ok := hitGroup(2, len(s.md2BuWears(state)), hitBUWear); ok {
		return hit
	}
	if hit, ok := hitGroup(1, 3, hitBUNgng); ok {
		return hit
	}
	if hit, ok := hitGroup(0, len(md2BuViewLetters(viewer)), hitBUView); ok {
		return hit
	}
	// 点面板内部空白保持当前画面；只有点面板外才返回角色选择页。
	if viewer.Base != "" {
		image := s.composer.Load(viewer.Base)
		if image != nil && x >= viewer.BaseX && y >= viewer.BaseY &&
			x < viewer.BaseX+image.Width && y < viewer.BaseY+image.Height {
			return uiHit{kind: hitNone, index: -1}
		}
	}
	return uiHit{kind: hitBUClose}
}

func (s *Session) activateMD2Bu(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil || ui.md2Bu == nil {
		return
	}
	state := ui.md2Bu
	viewer := s.config.Museum.BuViewer
	switch hit.kind {
	case hitBUView:
		views := md2BuViewLetters(viewer)
		if hit.index >= 0 && hit.index < len(views) {
			state.view = views[hit.index]
		}
	case hitBUNgng:
		state.stage = clampInt(hit.index, 0, 2)
	case hitBUWear:
		if list := s.md2BuWears(state); hit.index >= 0 && hit.index < len(list) {
			state.wear = list[hit.index]
		}
	case hitBUUnder:
		if hit.index >= 0 && hit.index < 7 {
			state.under = hit.index + 1
		}
	case hitBUFace:
		if list := s.md2BuFaces(state); hit.index >= 0 && hit.index < len(list) {
			state.face = list[hit.index]
		}
	case hitBUBackground:
		if hit.index >= 0 && hit.index < 30 {
			state.background = hit.index
		}
	case hitBUAppearance:
		// 兼容旧路径：把组合点击按顺序落到视点/阶段/服装上。
		state.stage = clampInt(hit.index, 0, 2)
	case hitBUTarget:
		if list := s.md2BuWears(state); hit.index >= 0 && hit.index < len(list) {
			state.wear = list[hit.index]
		}
	case hitBUClose:
		ui.md2Bu = nil
		core.Logf("ui: md2 bu viewer -> select")
	}
	// 换了任何组合都把缩放/平移清零：原版切 立绘/阶段/服装时重置视图，
	// 否则放大后换图，人物会留在画面外。
	if state != nil {
		state.zoom = 1
		state.panX = 0
		state.panY = 0
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}
