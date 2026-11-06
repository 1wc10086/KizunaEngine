package session

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_bgv.go 是 BGV（背景语音）鉴赏室的渲染与交互。
//
// 布局来自 define.bsa 的 bgv_layout：一张底图、右上角的关闭按钮、底部一条
// 全局控制条（停止/最小/最大/音量），以及每个角色一栏（自己的停止/音量按钮、
// 滚动条和最多 25 行曲目名）。曲目的音频文件在语音包里，点一行就发一条
// 播放事件给宿主，和剧情语音走同一条通道。
//
// 和鉴赏室的其它页面不同，这里没有"解锁"概念：BGV 本来就是给玩家试听的，
// 所以所有曲目都可点。

// bgvCharacterCount 是布局里的角色栏数（bgv_layout 里固定 5 个角色）。
const bgvCharacterCount = 5

// bgvWheelStep 是一次滚轮滚动几行。原始布局一栏最多 25 行，
// 滚 1 行几乎看不出变化，滚 3 行手感才合适。
const bgvWheelStep = 3

// bgvMaxVisibleLines 是可见行数的上限（布局里 interval/lines 都是 25）。
const bgvMaxVisibleLines = 25

// bgvTrackRow 是屏幕上的一行曲目：属于哪个角色、曲目本身、以及点击区域。
type bgvTrackRow struct {
	// char 是角色下标（0..4），和 CharacterBgv 的槽位一致。
	char int
	// track 是曲目本体（显示名 + 音频文件名）。
	track config.BGVTrack
	// x/y/w/h 是这一行的点击区域。
	x int
	y int
	w int
	h int
}

// bgvMuseumState 是 BGV 鉴赏室的状态。
type bgvMuseumState struct {
	// layout 是 bgv_layout 的解析结果。
	layout *config.BGVLayout
	// scroll 是每个角色列表的滚动偏移。
	scroll [bgvCharacterCount]int
	// rows 是当前可见的行（所有角色加起来）。
	rows []bgvTrackRow
	// hover 是当前悬停的行下标，-1 表示没有。
	hover int
	// bgvPlaying 表示当前有一条 BGV 在播。
	bgvPlaying bool
	// playingChar 是正在播放的角色下标（-1 表示没有）。
	// 不能存"可见行下标"：滚动会重建 rows，行下标会漂移，停止按钮就会
	// 停错角色或者完全停不掉。
	playingChar int
}

// ---------------------------------------------------------------- 打开

// openBGVMuseum 打开 BGV 鉴赏室。
//
// 布局来自 define.bsa 的 bgv_layout；没有这个文件的作品就进不去这一页，
// 调用方（鉴赏室主菜单）会退回主菜单。
func (s *Session) openBGVMuseum() bool {
	if s.ui == nil {
		return false
	}
	if s.res == nil {
		return false
	}
	// 从别的页签再进 BGV 时，先把上一条试听停掉：新状态里 bgvPlaying 是
	// false，留着声音会让停止按钮失效，也更像是 bug。
	if s.ui.bgv != nil && s.ui.bgv.bgvPlaying {
		s.stopBGVTrack(0)
	}
	location, ok := s.res.TryGet("bgv_layout")
	if !ok || location == nil {
		core.Logf("ui: bgv_layout not found")
		return false
	}
	layout, err := config.LoadBGVLayout(location.Read())
	if err != nil {
		core.Logf("ui: bgv_layout parse failed: %v", err)
		return false
	}

	state := &bgvMuseumState{
		layout:      layout,
		hover:       -1,
		playingChar: -1,
	}
	s.ui.kind = uiMuseumBGV
	s.ui.bgv = state
	// 换页签进来时清掉上一页的拖动状态，否则第一帧会按旧坐标算出一个大位移。
	s.ui.lastDragY = -1
	s.ui.bgvDragRemainder = 0
	s.ui.bgvDragColumn = -1
	s.buildBGVRows()
	s.composer.Invalidate()
	core.Logf("ui: bgv museum opened (%d characters)", len(layout.Characters))
	return true
}

// buildBGVRows 重建可见行。
//
// 行的几何来自角色的 view.pos：x 是左边界，y 是第一行的顶，width 是可用
// 宽度，height 是行高（布局里 25，正好一行顶到下一栏的控制条）。
//
// 起始行要按"最后一屏必须填满"夹紧：滚动到底时 start = len(tracks)-visible，
// 这样最后一行正好顶在列表底部，不会滚出一片空白（以前夹在 len(tracks)-1，
// 最后一栏只显示一行，其余都是空的）。
func (s *Session) buildBGVRows() {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return
	}
	state := ui.bgv
	state.rows = state.rows[:0]

	for index, character := range state.layout.Characters {
		if index >= bgvCharacterCount {
			break
		}
		view := character.View
		visible := bgvVisibleLines(view)
		lineHeight := view.Height
		if lineHeight <= 0 {
			lineHeight = 25
		}
		start := view.IntervalStart + state.scroll[index]
		if start < 0 {
			start = 0
		}
		// 滚到底时也不越过"最后一屏填满"的位置。
		if limit := max(0, len(character.Tracks)-visible); start > limit {
			start = limit
		}
		rows := min(visible, len(character.Tracks)-start)
		for row := 0; row < rows; row++ {
			item := start + row
			state.rows = append(state.rows, bgvTrackRow{
				char:  index,
				track: character.Tracks[item],
				x:     view.X + view.OffsetX,
				y:     view.Y + view.OffsetY + row*lineHeight,
				w:     view.Width,
				h:     lineHeight,
			})
		}
	}
}

// bgvVisibleLines 返回一栏里同时可见的行数。
//
// 布局里 interval:[0, 25] 是取哪一段，lines: 25 是能显示几行，两个都要
// 满足，所以取小的；上限 25 行（布局里的最大值）。
func bgvVisibleLines(view config.BGVView) int {
	lines := min(max(1, view.IntervalCount), max(1, view.Lines))
	return min(lines, bgvMaxVisibleLines)
}

// ---------------------------------------------------------------- 绘制

// drawBGVMuseum 画 BGV 鉴赏室。
func (s *Session) drawBGVMuseum(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return
	}
	state := ui.bgv
	layout := state.layout

	s.drawBGVBase(canvas, layout.Base)
	drawBGVButton(s, canvas, layout.Close, bgvHoverFrame(ui, hitBGVClose, 0))

	// 全局控制条。
	s.drawBGVControl(canvas, layout.Control, 0)

	// 每个角色一栏。
	for index, character := range layout.Characters {
		if index >= bgvCharacterCount {
			break
		}
		s.drawBGVControl(canvas, character.Control, index+1)
		// 滚动条：轨道固定，滑块按当前滚动比例停在轨道里。
		s.drawBGVScrollBar(canvas, index)
	}

	// 曲目名。文字用引擎自带的字体渲染器：曲目的显示名是日文，原作也是
	// 用同一套字库排的，用位图字库会缺字。
	// 字号取布局 font.block 的第一项（bgv_layout 是 [22,22]）。以前写死 13，
	// 比原版小一整圈，五栏列表看起来空荡且难读。
	fontSize := layout.Font.Block[0]
	if fontSize <= 0 {
		fontSize = 22
	}
	lineHeight := layout.Font.Block[1]
	if lineHeight <= 0 {
		lineHeight = 22
	}
	for index, row := range state.rows {
		if index == state.hover {
			canvas.Fill(row.x-2, row.y-1, row.w+4, row.h, 255, 255, 255, 40)
		}
		rendering.DrawText(canvas, s.textFont, row.track.Name,
			row.x, row.y, fontSize, 1, 1, lineHeight, row.w, 1, 0xffffffff)
	}
}

// drawBGVBase 画底图。
//
// 底图常常是合成图（BSS-Composition）；composer.Load 已经把它拍平，
// 只有尺寸和画布不一致时才需要缩放，这时用第一层缩放。
func (s *Session) drawBGVBase(canvas *gfx.BgraCanvas, name string) {
	if len(name) == 0 {
		return
	}
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	if image.Width == canvas.Width && image.Height == canvas.Height {
		canvas.Draw(image, 0, 0)
		return
	}
	source := image
	if len(image.Parts) > 0 {
		source = image.Parts[0]
	}
	canvas.BlitScaled(source, 0, 0, canvas.Width, canvas.Height, 255)
}

// drawBGVControl 画一组控制按钮（全局 target=0，角色 target=1..5）。
//
// 滚动条不在这里画：轨道和滑块由 drawBGVScrollBar 统一负责。
func (s *Session) drawBGVControl(canvas *gfx.BgraCanvas, control config.BGVControl, target int) {
	ui := s.ui
	drawBGVButton(s, canvas, control.Stop, bgvHoverFrame(ui, hitBGVStop, target))
	// min/max 共用 hitBGVVolume，只看 kind+target 会让两个按钮同时亮；
	// 再用悬停坐标确认指针确实落在哪一个按钮里。
	drawBGVButton(s, canvas, control.Min, bgvHoverButtonFrame(s, control.Min, hitBGVVolume, target))
	drawBGVButton(s, canvas, control.Max, bgvHoverButtonFrame(s, control.Max, hitBGVVolume, target))
	s.drawBGVVolume(canvas, control.Volume, target)
}

// bgvHoverButtonFrame 返回按钮的常态帧（1）或悬停帧（2）。
//
// 除了命中种类和 target，还要求指针坐标落在该按钮的真实矩形内：音量条、
// min、max 都是 hitBGVVolume，只有坐标能把它们区分开。
func bgvHoverButtonFrame(s *Session, button config.ConfigButton, kind uiHitKind, target int) int {
	ui := s.ui
	if ui == nil || ui.hover.kind != kind || ui.hover.index != target {
		return 1
	}
	if hitBGVButton(s, button, ui.hoverX, ui.hoverY) {
		return 2
	}
	return 1
}

// drawBGVScrollBar 画角色栏的滚动条：轨道固定，滑块按滚动比例移动。
//
// 布局里的滚动条图是整条轨道，不是滑块；以前把轨道本身上下挪，拖动时
// 轨道会跟着跑出栏位。现在轨道静态画在 button.Y，白色滑块在轨道内移动。
func (s *Session) drawBGVScrollBar(canvas *gfx.BgraCanvas, index int) {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return
	}
	layout := ui.bgv.layout
	if index < 0 || index >= len(layout.Characters) {
		return
	}
	button := layout.Characters[index].Control.ScrollBar
	if len(button.Name) == 0 {
		return
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return
	}
	drawBGVButton(s, canvas, button, 0)
	_, barY, _, trackH, thumbH, maxScroll, ok := s.bgvBarGeometry(index)
	if !ok {
		return
	}
	fraction := float64(ui.bgv.scroll[index]) / float64(maxScroll)
	thumbY := barY + int(fraction*float64(trackH-thumbH))
	canvas.Fill(button.X+2, thumbY, max(3, image.Width-4), thumbH, 255, 255, 255, 140)
}

// bgvBarGeometry 返回一栏滚动条的几何。
//
// 轨道高按 max_state 切帧；没有可滚内容（maxScroll == 0）时 ok 为 false，
// 绘制和命中都不应该再给这条轨道热区。
func (s *Session) bgvBarGeometry(index int) (x, y, w, trackH, thumbH, maxScroll int, ok bool) {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	layout := ui.bgv.layout
	if index < 0 || index >= len(layout.Characters) || index >= bgvCharacterCount {
		return 0, 0, 0, 0, 0, 0, false
	}
	character := layout.Characters[index]
	button := character.Control.ScrollBar
	if len(button.Name) == 0 {
		return 0, 0, 0, 0, 0, 0, false
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	trackH = image.Height / max(1, button.MaxState)
	visible := bgvVisibleLines(character.View)
	maxScroll = max(0, len(character.Tracks)-visible)
	thumbH = max(16, trackH*visible/max(1, len(character.Tracks)))
	return button.X, button.Y, image.Width, trackH, thumbH, maxScroll, maxScroll > 0
}

// scrollBGVTo 把一栏的滚动条滑块中心对到指针 y 上。
func (s *Session) scrollBGVTo(index, y int) {
	ui := s.ui
	if ui == nil || ui.bgv == nil {
		return
	}
	_, barY, _, trackH, thumbH, maxScroll, ok := s.bgvBarGeometry(index)
	if !ok {
		return
	}
	scroll := maxScroll
	if travel := trackH - thumbH; travel > 0 {
		fraction := float64(y-barY-thumbH/2) / float64(travel)
		fraction = max(0, min(1, fraction))
		scroll = int(fraction*float64(maxScroll) + 0.5)
	}
	scroll = clampInt(scroll, 0, maxScroll)
	if scroll == ui.bgv.scroll[index] {
		return
	}
	ui.bgv.scroll[index] = scroll
	s.buildBGVRows()
	s.composer.Invalidate()
}

// drawBGVVolume 画音量条，并在上面标出当前刻度。
//
// 底图本身没有指针，玩家看不出自己拖到了哪里，所以照设置画面的做法补一条
// 3px 的白色刻度。
func (s *Session) drawBGVVolume(canvas *gfx.BgraCanvas, volume config.BGVVolume, target int) {
	drawBGVButton(s, canvas, volume.Button, 0)
	if volume.HitWidth <= 0 {
		return
	}
	fraction := s.bgvVolume(target)
	x := volume.HitX + int(float64(volume.HitWidth-3)*fraction)
	canvas.Fill(x, volume.HitY, 3, volume.HitHeight, 255, 255, 255, 220)
}

// bgvHoverFrame 返回常态帧（1）或悬停帧（2）。
// 这批按钮图的第 0 帧是白色命中掩码，不能直接画。
func bgvHoverFrame(ui *systemUI, kind uiHitKind, target int) int {
	if ui == nil {
		return 1
	}
	if ui.hover.kind == kind && ui.hover.index == target {
		return 2
	}
	return 1
}

// drawBGVButton 按 max_state 切帧画一张多状态按钮图。
//
// 按钮图是竖排的：整张图高 = 帧数 × 单帧高，所以要自己切，不能用解码器的
// 帧数（解码器把整张竖排图当成一帧）。
func drawBGVButton(s *Session, canvas *gfx.BgraCanvas, button config.ConfigButton, frame int) {
	if len(button.Name) == 0 {
		return
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return
	}
	states := max(1, button.MaxState)
	frame = clampInt(frame, 0, states-1)
	height := image.Height / states
	if height <= 0 {
		return
	}
	canvas.DrawRegion(image, 0, frame*height, image.Width, height, button.X, button.Y)
}

// ---------------------------------------------------------------- 命中

// hitBGVMuseum 判定鼠标位置命中了什么。
//
// 命中种类：关闭、曲目、停止、音量、滚动条。min/max 按钮和音量条都归到
// "音量"这一类，具体是哪一个由激活时再用点击坐标判断。
func (s *Session) hitBGVMuseum(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.bgv
	layout := state.layout

	if hitBGVButton(s, layout.Close, x, y) {
		state.hover = -1
		return uiHit{kind: hitBGVClose}
	}
	// BGV 底图顶部保留了鉴赏室页签；点页签直接切到对应页面，
	// 不用先按"戻る"回主菜单。close 先判，避免和页签右侧重叠。
	if hit, ok := s.hitMuseumTabs(x, y); ok {
		return hit
	}
	if hit := s.hitBGVControl(layout.Control, 0, x, y); hit.kind != hitNone {
		state.hover = -1
		return hit
	}
	for index, character := range layout.Characters {
		if index >= bgvCharacterCount {
			break
		}
		if hit := s.hitBGVControl(character.Control, index+1, x, y); hit.kind != hitNone {
			state.hover = -1
			return hit
		}
	}

	// 滚动条在曲目列表右侧，不属于任何一行；先判轨道，再判行。
	for index := range layout.Characters {
		if index >= bgvCharacterCount {
			break
		}
		if barX, barY, barW, trackH, _, _, ok := s.bgvBarGeometry(index); ok &&
			x >= barX && y >= barY && x < barX+barW && y < barY+trackH {
			state.hover = -1
			return uiHit{kind: hitBGVBar, index: index}
		}
	}

	for index, row := range state.rows {
		if x >= row.x && y >= row.y && x < row.x+row.w && y < row.y+row.h {
			state.hover = index
			return uiHit{kind: hitBGVTrack, index: index}
		}
	}
	state.hover = -1
	return uiHit{kind: hitNone, index: -1}
}

// hitBGVControl 判定一组控制按钮上的命中。
func (s *Session) hitBGVControl(control config.BGVControl, target, x, y int) uiHit {
	if hitBGVButton(s, control.Stop, x, y) {
		return uiHit{kind: hitBGVStop, index: target}
	}
	if hitBGVButton(s, control.Min, x, y) {
		return uiHit{kind: hitBGVVolume, index: target}
	}
	if hitBGVButton(s, control.Max, x, y) {
		return uiHit{kind: hitBGVVolume, index: target}
	}
	volume := control.Volume
	if volume.HitWidth > 0 && volume.HitHeight > 0 {
		if x >= volume.HitX && y >= volume.HitY &&
			x < volume.HitX+volume.HitWidth && y < volume.HitY+volume.HitHeight {
			return uiHit{kind: hitBGVVolume, index: target}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitBGVButton 用按钮图的真实尺寸做命中。
func hitBGVButton(s *Session, button config.ConfigButton, x, y int) bool {
	if len(button.Name) == 0 {
		return false
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return false
	}
	states := max(1, button.MaxState)
	height := image.Height / states
	return x >= button.X && y >= button.Y && x < button.X+image.Width && y < button.Y+height
}

// ---------------------------------------------------------------- 交互

// activateBGVMuseum 处理 BGV 鉴赏室上的点击。
func (s *Session) activateBGVMuseum(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil || ui.bgv == nil {
		return
	}
	switch hit.kind {
	case hitBGVClose:
		s.closeBGVMuseum()
	case hitBGVTrack:
		s.playBGVTrack(hit.index)
	case hitBGVStop:
		s.stopBGVTrack(hit.index)
	case hitBGVVolume:
		s.adjustBGVVolume(hit.index, x, y)
	case hitBGVBar:
		s.scrollBGVTo(hit.index, y)
	}
}

// ---------------------------------------------------------------- 滚轮

// wheelBGV 处理 BGV 鉴赏室上的滚轮。
//
// 滚轮事件本身不带坐标，只能靠"最后一次指针位置"判断滚哪一栏；指针不落在
// 任何一栏的曲目区里时退回第一栏。每次滚 3 行，滚动上限是"最后一屏填满"
// 的位置（和 buildBGVRows 里的夹紧一致），所以滚到底不会出现半空栏。
func (s *Session) wheelBGV(delta int32) {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return
	}
	state := ui.bgv
	column := s.bgvWheelColumn()
	if column < 0 || column >= bgvCharacterCount ||
		column >= len(state.layout.Characters) {
		column = 0
	}
	character := state.layout.Characters[column]
	visible := bgvVisibleLines(character.View)
	maxScroll := max(0, len(character.Tracks)-visible)
	scroll := clampInt(state.scroll[column]+int(delta)*bgvWheelStep, 0, maxScroll)
	if scroll == state.scroll[column] {
		return
	}
	state.scroll[column] = scroll
	s.buildBGVRows()
	s.composer.Invalidate()
}

// bgvWheelColumn 返回滚轮该作用在哪一栏（0..4）。
//
// 栏的横向范围用整栏宽度：从本栏 view.X 到下一栏 view.X（最后一栏到画面
// 右边缘），而不是 view.Width —— 后者只是文字的可用宽度，比整栏窄，
// 指针落在滚动条那一侧时就找不到栏了。纵向还必须落在曲目列表里
// （view.Y .. view.Y+可见行数×行高），否则视为没命中，退回第一栏。
func (s *Session) bgvWheelColumn() int {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return 0
	}
	x := s.lastPointerX
	y := s.lastPointerY
	if x == 0 || y == 0 {
		// 从来没有过指针位置：滚第一栏。
		return 0
	}
	layout := ui.bgv.layout
	for index, character := range layout.Characters {
		if index >= bgvCharacterCount {
			break
		}
		view := character.View
		left := view.X
		right := s.width
		if index+1 < len(layout.Characters) {
			right = layout.Characters[index+1].View.X
		}
		if x < left || x >= right {
			continue
		}
		visible := bgvVisibleLines(view)
		if y < view.Y || y >= view.Y+visible*view.Height {
			return 0
		}
		return index
	}
	return 0
}

// closeBGVMuseum 关掉 BGV 鉴赏室，回到鉴赏室主菜单。
//
// 正在播的 BGV 一起停掉：鉴赏室主菜单是另一个画面，留着声音会显得是 bug。
func (s *Session) closeBGVMuseum() {
	ui := s.ui
	if ui == nil {
		return
	}
	s.stopBGVTrack(0)
	ui.kind = uiMuseum
	ui.bgv = nil
	s.composer.Invalidate()
}

// playBGVTrack 播放一条 BGV。
//
// 走语音通道（Kind 2）：BGV 的音频文件和角色语音在同一个包里，宿主按语音
// 播放即可；音量是"BGV 通道音量 × 该角色的 BGV 音量"。
func (s *Session) playBGVTrack(index int) {
	ui := s.ui
	if ui == nil || ui.bgv == nil {
		return
	}
	state := ui.bgv
	if index < 0 || index >= len(state.rows) {
		return
	}
	row := state.rows[index]
	s.audio = append(s.audio, core.AudioEvent{
		Kind:    2,
		Path:    s.materialize(row.track.File),
		Volume:  s.volumeGain(kindBgv) * s.characterGain(row.track.File, false),
		Channel: noTarget,
		Group:   noTarget,
	})
	state.bgvPlaying = true
	state.playingChar = row.char
}

// stopBGVTrack 停止播放。
//
// target 是 0（全局停止）或者 1..5（角色栏上的停止）；角色栏的停止只停掉
// 正在播的那条，如果播的是别的角色就什么也不做。
func (s *Session) stopBGVTrack(target int) {
	ui := s.ui
	if ui == nil || ui.bgv == nil {
		return
	}
	state := ui.bgv
	if !state.bgvPlaying {
		return
	}
	if target > 0 {
		// 比较角色下标而不是行下标：列表滚动后可见行会重建，行下标不再
		// 指向当初播放的那一条。
		if state.playingChar != target-1 {
			return
		}
	}
	s.audio = append(s.audio, core.AudioEvent{
		Kind:    4,
		Channel: noTarget,
		Group:   noTarget,
	})
	state.bgvPlaying = false
	state.playingChar = -1
}

// adjustBGVVolume 调整音量。
//
// target 是 0（全局音量条）或 1..5（角色栏的音量条）。min/max 按钮和音量条
// 共用同一个命中种类，靠点击坐标区分：点在按钮上就是 0/1，点在条上是按位置
// 取比例。
func (s *Session) adjustBGVVolume(target, x, y int) {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return
	}
	control := s.bgvControl(target)
	if control == nil {
		return
	}

	value := -1
	switch {
	case hitBGVButton(s, control.Min, x, y):
		value = 0
	case hitBGVButton(s, control.Max, x, y):
		value = 1000
	default:
		volume := control.Volume
		if volume.HitWidth <= 0 {
			return
		}
		fraction := float64(x-volume.HitX) / float64(volume.HitWidth)
		fraction = max(0, min(1, fraction))
		value = int(fraction * 1000)
	}

	s.setBGVVolume(target, value)
	s.saveGameSettings()
	s.refreshAudioVolumes()
	s.composer.Invalidate()
}

// bgvControl 取一组控制按钮：0 是全局，1..5 是各角色。
func (s *Session) bgvControl(target int) *config.BGVControl {
	ui := s.ui
	if ui == nil || ui.bgv == nil || ui.bgv.layout == nil {
		return nil
	}
	layout := ui.bgv.layout
	if target == 0 {
		return &layout.Control
	}
	index := target - 1
	if index < 0 || index >= len(layout.Characters) {
		return nil
	}
	return &layout.Characters[index].Control
}

// bgvVolume 读某个音量条的当前值（0..1）。
func (s *Session) bgvVolume(target int) float64 {
	if target == 0 {
		return s.settings.BgvVolume
	}
	slot := target - 1
	if slot < 0 || slot >= len(s.settings.CharacterBgv) {
		return 1
	}
	return s.settings.CharacterBgv[slot]
}

// setBGVVolume 写某个音量条的当前值（0..1000）。
//
// 全局音量条改的是 BGV 通道音量；角色栏的音量条改的是该角色自己的 BGV
// 音量（个别音声設定），和剧情里播放该角色 BGV 时乘上的增益是同一个值。
func (s *Session) setBGVVolume(target, value int) {
	value = clampInt(value, 0, 1000)
	if target == 0 {
		s.settings.BgvVolume = clampVolumeSetting(float64(value) / 1000)
		return
	}
	slot := target - 1
	if slot < 0 || slot >= len(s.settings.CharacterBgv) {
		return
	}
	s.settings.CharacterBgv[slot] = clampVolumeSetting(float64(value) / 1000)
}
