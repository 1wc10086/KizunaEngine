package session

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_mk_strip.go 是「牝教師～淫辱の教室～」（MK，BSScript Rev.5）的
// 立ち絵鉴赏。
//
// 这一代没有 define.bsa 的 museum_buex_layout，也没有 [bu viewer] 配置：
// 素材只说明了两件事：
//   - museum_ch1..6 是六张 72x528 的整身角色条（美雪/麻琴/律子/沙耶/理保子/たまき）；
//   - bum_base 是 600x152 的控制面板，烘着「距離 / 服装 / 表情 / 视点 /
//     背景」与「ポーズ / ぬぎぬぎ」七行标签，bum_point 是 16x32 的两态指针。
//
// 立绘本体沿用剧本的 cs 命名：6 字符名 cs<角色><场景><距离><表情>，
// 身体图是 cs<前5位>0 + 视点(a/b/c) + 服装(a/b/c)。鉴赏室把选择页的
// 角色序号直接当 cs 的角色号，距离/服装/表情/视点/ポーズ/背景都可循环，
// 与原版面板一一对应。精确的按钮热区没有可核对的常量表，这里按面板
// 标签本身的排版划分行，绘制与命中共用同一套常量。

const (
	mkStripPanelX = 100
	mkStripPanelY = 448
	mkStripPanelW = 600
	mkStripPanelH = 152

	// 面板左侧五行标签的基准 y（相对面板左上角）。bum_base 的行距约 22px。
	mkStripRow0Y  = 8
	mkStripRowGap = 22
	// 右侧「ポーズ / ぬぎぬぎ」两行的 y。
	mkStripPoseY = 8
	mkStripNudeY = 62

	mkStripLeftValueX  = 92
	mkStripRightValueX = 352
	mkStripPointerX    = 66
	mkStripPointerRX   = 254

	// 远距离构图与 runtime/bs 的 cs 合成保持一致（0/376/650，缩放 0.6）。
	mkStripFarScale = 0.6
	mkStripFarX     = 160
	mkStripFarY     = 60
)

var mkStripSectionOffsets = [3]int{0, 376, 650}

// mkStripState 是立ち絵鉴赏的运行状态。
type mkStripState struct {
	// character 是选择页的卡片下标（0..5）；-1 表示停在选择页。
	character int
	// far 为真时画全身（遠い），否则画当前视点的一块（近い）。
	far  bool
	wear int
	view int
	// face 是表情编号（1..6），必须是 poses[pose]+face 的素材之一。
	face int
	pose int
	bg   int
	// poses 是当前角色可用的 cs 前缀（cs + 角色 + 场景 + 姿势）。
	poses []string
	// faces 是当前 pose 前缀可用的表情编号，按升序。
	faces []int
	// backgrounds 是背景候选（gb*，按名字排序）。
	backgrounds []string
	// focus 是鼠标当前所在行：0..4 左侧，5 ポーズ，6 ぬぎぬぎ，-1 无。
	focus int
}

// openMKStripMuseum 打开 MK 的立ち絵页。
//
// 已经在查看页时只回到角色选择页，保留玩家挑好的背景；再次点角色卡
// 才重建 poses/faces，避免切换页签把状态清空。
func (s *Session) openMKStripMuseum() bool {
	if s.ui == nil || s.composer == nil || s.res == nil {
		return false
	}
	for index := 1; index <= 6; index++ {
		if !s.res.Exists("museum_ch" + strconv.Itoa(index)) {
			return false
		}
	}
	if !s.res.Exists("bum_base") || !s.res.Exists("bum_point") {
		return false
	}
	if s.ui.kind == uiMuseumStrip && s.ui.mkStrip != nil {
		s.ui.mkStrip.character = -1
		s.composer.Invalidate()
		return true
	}
	state := &mkStripState{
		character:   -1,
		far:         true,
		backgrounds: fallbackBackgroundNames(s.resourceImageNames()),
		focus:       -1,
	}
	// 默认选一张确实存在的背景；gb00001a 是原版最常见的室内/校舍底图。
	for index, name := range state.backgrounds {
		if name == "gb00001a" {
			state.bg = index
			break
		}
	}
	s.ui.kind = uiMuseumStrip
	s.ui.mkStrip = state
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	core.Logf("ui: mk standing gallery opened (%d background(s))", len(state.backgrounds))
	return true
}

// mkStripCharacterPoses 收集一个角色可用的 cs 前缀。
//
// 只接受 6 字符 cs<角色><场景><姿势><表情>；身体图必须存在，否则这个
// 姿势在鉴赏室里只能画出头。角色号与 museum_chN 的 N 一一对应。
func (s *Session) mkStripCharacterPoses(character int) []string {
	if character < 1 || s.res == nil {
		return nil
	}
	prefix := "cs" + strconv.Itoa(character)
	seen := map[string]bool{}
	result := make([]string, 0, 32)
	for _, name := range s.resourceImageNames() {
		if len(name) != 6 || !strings.HasPrefix(name, prefix) {
			continue
		}
		if name[5] < '1' || name[5] > '6' || !mkStripNameChar(name[3]) || !mkStripNameChar(name[4]) {
			continue
		}
		posePrefix := name[:5]
		if seen[posePrefix] {
			continue
		}
		// 任一套身体图存在即可；全缺时这个姿势无法显示。
		found := false
		for _, view := range []string{"a", "b", "c"} {
			for _, wear := range []string{"a", "b", "c"} {
				if s.res.Exists(posePrefix + "0" + view + wear) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			continue
		}
		seen[posePrefix] = true
		result = append(result, posePrefix)
	}
	sort.Strings(result)
	return result
}

// mkStripNameChar 判断 cs 名里的场景/姿势位是否合法（数字或小写字母）。
func mkStripNameChar(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'z')
}

// mkStripFacesFor 返回 pose 前缀下存在的表情编号（1..6）。
func (s *Session) mkStripFacesFor(posePrefix string) []int {
	result := make([]int, 0, 6)
	for digit := 1; digit <= 6; digit++ {
		if s.res.Exists(posePrefix + strconv.Itoa(digit)) {
			result = append(result, digit)
		}
	}
	return result
}

// mkStripSelectCharacter 进入某个角色的查看页并重建可选元素。
func (s *Session) mkStripSelectCharacter(index int) {
	state := s.ui.mkStrip
	if state == nil || index < 0 || index >= 6 {
		return
	}
	state.character = index
	state.poses = s.mkStripCharacterPoses(index + 1)
	state.pose = 0
	state.face = 0
	state.view = 0
	state.wear = 0
	state.far = true
	state.focus = -1
	s.mkStripRefreshFaces()
	core.Logf("ui: mk standing viewer character=%d poses=%d", index+1, len(state.poses))
}

// mkStripRefreshFaces 在换姿势后按新前缀重新收集表情，并把当前值夹回范围。
func (s *Session) mkStripRefreshFaces() {
	state := s.ui.mkStrip
	if state == nil {
		return
	}
	if state.pose < 0 || state.pose >= len(state.poses) {
		state.faces = nil
		state.face = 0
		return
	}
	state.faces = s.mkStripFacesFor(state.poses[state.pose])
	if len(state.faces) == 0 {
		state.face = 0
		return
	}
	valid := false
	for _, digit := range state.faces {
		if digit == state.face {
			valid = true
			break
		}
	}
	if !valid {
		state.face = state.faces[0]
	}
}

func (s *Session) mkStripNextFace(delta int) {
	state := s.ui.mkStrip
	if state == nil || len(state.faces) == 0 {
		return
	}
	index := 0
	for i, digit := range state.faces {
		if digit == state.face {
			index = i
			break
		}
	}
	index = ((index+delta)%len(state.faces) + len(state.faces)) % len(state.faces)
	state.face = state.faces[index]
}

// drawMKStripMuseum 画角色选择页或查看页。
func (s *Session) drawMKStripMuseum(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.mkStrip == nil {
		return
	}
	layout := s.config.Museum
	state := ui.mkStrip
	if state.character < 0 {
		s.drawImageAt(canvas, layout.Base, 0, 0)
		for index, card := range layout.StripPortraits {
			s.drawFrameButton(canvas, card, 0)
			if ui.hover.kind == hitMK2Char && ui.hover.index == index {
				image := s.buttonImage(card)
				if image != nil {
					canvas.Fill(card.X-2, card.Y-2, image.Width+4, 2, 255, 255, 255, 220)
					canvas.Fill(card.X-2, card.Y+image.Height, image.Width+4, 2, 255, 255, 255, 220)
					canvas.Fill(card.X-2, card.Y-2, 2, image.Height+4, 255, 255, 255, 220)
					canvas.Fill(card.X+image.Width, card.Y-2, 2, image.Height+4, 255, 255, 255, 220)
				}
			}
		}
		s.drawMuseumTabs(canvas)
		return
	}
	if len(state.backgrounds) > 0 {
		s.drawImageAt(canvas, state.backgrounds[state.bg], 0, 0)
	}
	s.drawMKStripStandee(canvas, state)
	s.drawImageAt(canvas, "bum_base", mkStripPanelX, mkStripPanelY)
	s.drawMKStripValues(canvas, state)
	s.drawMuseumTabs(canvas)
}

// drawMKStripStandee 按当前距离/视点/服装/表情画 cs 合成立绘。
func (s *Session) drawMKStripStandee(canvas *gfx.BgraCanvas, state *mkStripState) {
	if state.pose < 0 || state.pose >= len(state.poses) || s.composer == nil {
		return
	}
	prefix := state.poses[state.pose]
	view := string(rune('a' + clampInt(state.view, 0, 2)))
	wear := string(rune('a' + clampInt(state.wear, 0, 2)))
	face := prefix + strconv.Itoa(max(1, state.face))
	draw := func(name string, x, y int, scale float64) {
		image := s.composer.Load(name)
		if image == nil {
			return
		}
		if scale == 1 {
			canvas.Draw(image, x, y)
			return
		}
		canvas.BlitScaled(image, x, y,
			max(1, int(float64(image.Width)*scale)),
			max(1, int(float64(image.Height)*scale)), 255)
	}
	if state.far {
		for index, offset := range mkStripSectionOffsets {
			body := prefix + "0" + string(rune('a'+index)) + wear
			draw(body, mkStripFarX, mkStripFarY+int(float64(offset)*mkStripFarScale), mkStripFarScale)
		}
		draw(face, mkStripFarX, mkStripFarY, mkStripFarScale)
		return
	}
	draw(prefix+"0"+view+wear, 0, 0, 1)
	// 视点 b/c 是胸/腰特写，头已出框；只有 a 才叠表情。
	if state.view == 0 {
		draw(face, 0, 0, 1)
	}
}

// drawMKStripValues 把七行当前值画在 bum_base 的冒号右侧。
func (s *Session) drawMKStripValues(canvas *gfx.BgraCanvas, state *mkStripState) {
	if s.textFont == nil {
		return
	}
	left := []string{
		"遠い",
		"着衣",
		fmt.Sprintf("%d/%d", max(0, state.face), len(state.faces)),
		"顔",
		"",
	}
	if !state.far {
		left[0] = "近い"
	}
	switch state.wear {
	case 1:
		left[1] = "下着"
	case 2:
		left[1] = "全裸"
	}
	switch state.view {
	case 1:
		left[3] = "胸"
	case 2:
		left[3] = "腰"
	}
	if len(state.backgrounds) > 0 {
		left[4] = state.backgrounds[state.bg]
	}
	for index, text := range left {
		rendering.DrawText(canvas, s.textFont, text,
			mkStripPanelX+mkStripLeftValueX,
			mkStripPanelY+mkStripRow0Y+index*mkStripRowGap,
			16, 1, 1, 20, 220, 1, 0xFFFFFFFF)
	}
	right := []string{
		fmt.Sprintf("%d/%d", state.pose+1, len(state.poses)),
		"OFF",
	}
	if state.wear == 2 {
		right[1] = "ON"
	}
	rendering.DrawText(canvas, s.textFont, right[0],
		mkStripPanelX+mkStripRightValueX, mkStripPanelY+mkStripPoseY,
		16, 1, 1, 20, 180, 1, 0xFFFFFFFF)
	rendering.DrawText(canvas, s.textFont, right[1],
		mkStripPanelX+mkStripRightValueX, mkStripPanelY+mkStripNudeY,
		16, 1, 1, 20, 180, 1, 0xFFFFFFFF)
	// 指针画在当前行左侧；bum_point 是两帧（常态/蓝色高亮）。
	if state.focus >= 0 {
		pointer := s.composer.Load("bum_point")
		if pointer != nil {
			frameHeight := pointer.Height / 2
			frame := 0
			if state.focus == 5 || state.focus == 6 || state.focus >= 0 {
				frame = 1
			}
			x := mkStripPanelX + mkStripPointerX
			y := mkStripPanelY + mkStripRow0Y + state.focus*mkStripRowGap
			if state.focus == 5 {
				x = mkStripPanelX + mkStripPointerRX
				y = mkStripPanelY + mkStripPoseY
			} else if state.focus == 6 {
				x = mkStripPanelX + mkStripPointerRX
				y = mkStripPanelY + mkStripNudeY
			}
			canvas.DrawRegion(pointer, 0, frame*frameHeight, pointer.Width, frameHeight, x, y)
		}
	}
}

// hitMKStrip 返回选择页卡片或查看页面板的行号。
//
// 行号复用 hitMK2Prev/hitMK2Next 两个已有种类：查看页只需要"点一行循环
// 它的值"，不需要为每一行新增命中类型。index 含义：
// 0..4 左侧五行，5 ポーズ，6 ぬぎぬぎ。
func (s *Session) hitMKStrip(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.mkStrip == nil || s.config == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.mkStrip
	if state.character < 0 {
		for index, card := range s.config.Museum.StripPortraits {
			image := s.buttonImage(card)
			if image == nil {
				continue
			}
			if x >= card.X && y >= card.Y && x < card.X+image.Width && y < card.Y+image.Height {
				return uiHit{kind: hitMK2Char, index: index}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	state.focus = -1
	for index := 0; index < 5; index++ {
		top := mkStripPanelY + mkStripRow0Y + index*mkStripRowGap - 3
		if x >= mkStripPanelX && x < mkStripPanelX+260 && y >= top && y < top+mkStripRowGap {
			state.focus = index
			return uiHit{kind: hitMK2Next, index: index}
		}
	}
	if x >= mkStripPanelX+260 && x < mkStripPanelX+mkStripPanelW {
		if y >= mkStripPanelY+mkStripPoseY-3 && y < mkStripPanelY+mkStripPoseY+mkStripRowGap {
			state.focus = 5
			return uiHit{kind: hitMK2Next, index: 5}
		}
		if y >= mkStripPanelY+mkStripNudeY-3 && y < mkStripPanelY+mkStripNudeY+mkStripRowGap {
			state.focus = 6
			return uiHit{kind: hitMK2Next, index: 6}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateMKStrip 处理选择页/查看页的点击。
func (s *Session) activateMKStrip(hit uiHit) {
	ui := s.ui
	if ui == nil || ui.mkStrip == nil {
		return
	}
	state := ui.mkStrip
	switch hit.kind {
	case hitMK2Char:
		s.mkStripSelectCharacter(hit.index)
	case hitMK2Next:
		switch hit.index {
		case 0:
			state.far = !state.far
		case 1:
			state.wear = (state.wear + 1) % 3
		case 2:
			s.mkStripNextFace(1)
		case 3:
			state.view = (state.view + 1) % 3
		case 4:
			if len(state.backgrounds) > 0 {
				state.bg = (state.bg + 1) % len(state.backgrounds)
			}
		case 5:
			if len(state.poses) > 0 {
				state.pose = (state.pose + 1) % len(state.poses)
				s.mkStripRefreshFaces()
			}
		case 6:
			// ぬぎぬぎ是"全裸开关"：ON 时服装固定 c，OFF 回到着衣 a。
			if state.wear == 2 {
				state.wear = 0
			} else {
				state.wear = 2
			}
		}
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// backMKStripViewer 处理右键：查看页先回选择页。
func (s *Session) backMKStripViewer() bool {
	if s.ui == nil || s.ui.kind != uiMuseumStrip || s.ui.mkStrip == nil {
		return false
	}
	if s.ui.mkStrip.character < 0 {
		return false
	}
	s.ui.mkStrip.character = -1
	s.composer.Invalidate()
	return true
}

// wheelMKStrip 用滚轮换背景（查看页最常调整的是背景）。
func (s *Session) wheelMKStrip(delta int32) bool {
	if s.ui == nil || s.ui.kind != uiMuseumStrip || s.ui.mkStrip == nil {
		return false
	}
	state := s.ui.mkStrip
	if state.character < 0 || len(state.backgrounds) == 0 || delta == 0 {
		return false
	}
	state.bg = ((state.bg+int(delta))%len(state.backgrounds) + len(state.backgrounds)) % len(state.backgrounds)
	s.composer.Invalidate()
	return true
}
