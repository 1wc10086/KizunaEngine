package session

import (
	"fmt"
	"math"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_status_slg.go 是脚本驱动状态画面（[status screen] script name）。
//
// 特別授業3SLG 不提供 status_layout，而是 embed.bsa:status_screen.lua 里
// 四页原生界面：基本参数、调教次数、结果、实绩。这里按 Lua 的坐标与数据源
// 复刻，而不是引入整个 Lua 运行时：四页的排版在脚本里是固定的，数据则来自
// var_db（#ID_BP/#ID_TC/#ID_LOG）。这样状态按钮立刻可用，也不会因为 Lua
// 初始化失败把整个游戏卡住。
//
// 坐标全部来自 status_screen.lua；改作品时只需要确认四张底图存在，
// 缺图/缺表按空页处理，不让玩家掉进黑屏。

const (
	slgStatusBasic = iota
	slgStatusTraining
	slgStatusResult
	slgStatusAchievement
	slgStatusPageCount
)

var slgStatusCharacters = []string{"早紀", "ありさ", "美香子"}

var slgStatusBaseNames = []string{
	"status_basic_base",
	"status_training_base",
	"status_training_base",
	"status_achievement_base",
}

type slgStatusState struct {
	page     int
	scroll   int
	dragging bool
}

func (s *Session) slgStatus() *slgStatusState {
	if s.ui == nil {
		return nil
	}
	if s.ui.slg == nil {
		s.ui.slg = &slgStatusState{}
	}
	return s.ui.slg
}

// slgStatusScale 旧版状态画面相对新版 3.3 的缩放。
//
// 两版 status_screen.lua 的坐标是严格的 4/3 关系（3.3 是 1366x768，
// 旧版是 1024x576）：342/256、43/32、152/114 全部吻合。写死 3.3 坐标会让
// 旧版的页签跑到 y=697（画面只有 576 高）、数值列跑到 x=1228（画面只有
// 1024 宽），玩家看到的就是"状態画面整页错位/空白"。
// 所以所有坐标都经过同一比例换算，新版仍按 1.0 原样绘制。
func (s *Session) slgStatusScale() float64 {
	if s.height <= 0 || s.height == 768 {
		return 1
	}
	return float64(s.height) / 768.0
}

func (s *Session) slgStatusPx(value int) int {
	return int(math.Round(float64(value) * s.slgStatusScale()))
}

func (s *Session) slgStatusFont(value int) int {
	return max(8, s.slgStatusPx(value))
}

func (s *Session) openSLGStatusScreen() bool {
	if s.gameState == nil || s.composer == nil || !s.canOpenSystemUI() {
		return false
	}
	s.beginSystemUI(uiStatusSLG)
	s.statusPage = 0
	s.statusScroll = 0
	s.ui.slg = &slgStatusState{}
	core.Logf("ui: SLG status screen opened")
	return true
}

func (s *Session) drawSLGStatusScreen(canvas *gfx.BgraCanvas) {
	state := s.slgStatus()
	if state == nil || canvas == nil {
		return
	}
	state.page = clampInt(state.page, 0, slgStatusPageCount-1)
	s.drawLayerAt(canvas, "status_base", 0, 0)
	if page := slgStatusBaseNames[state.page]; page != "" {
		s.drawLayerAt(canvas, page, 0, 0)
	}
	switch state.page {
	case slgStatusBasic:
		s.drawSLGStatusBasic(canvas)
	case slgStatusTraining:
		s.drawSLGStatusList(canvas, "#ID_TC")
	case slgStatusResult:
		s.drawSLGStatusList(canvas, "#ID_LOG")
	case slgStatusAchievement:
		s.drawSLGStatusAchievement(canvas)
	}
	for index, button := range s.slgStatusTabs() {
		if button.Empty() {
			continue
		}
		// 两帧：0 常态、1 选中。当前页签用选中帧，玩家能看出自己在哪一页。
		frame := 0
		if index == state.page {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
}

// slgStatusTabs 是四个页签与返回按钮。按钮图都是两帧（常态/选中）。
func (s *Session) slgStatusTabs() []config.UIButton {
	return []config.UIButton{
		{Image: "status_basic_param", Frames: 2, X: s.slgStatusPx(64), Y: s.slgStatusPx(128)},
		{Image: "status_training_count", Frames: 2, X: s.slgStatusPx(64), Y: s.slgStatusPx(168)},
		{Image: "status_result", Frames: 2, X: s.slgStatusPx(65), Y: s.slgStatusPx(208)},
		{Image: "status_achievements", Frames: 2, X: s.slgStatusPx(64), Y: s.slgStatusPx(288)},
		{Image: "status_back", Frames: 2, X: s.slgStatusPx(65), Y: s.slgStatusPx(697)},
	}
}

func (s *Session) drawSLGStatusBasic(canvas *gfx.BgraCanvas) {
	db := s.ensureScriptDB()
	if db == nil {
		return
	}
	params := []string{"調教進行度", "従順度", "調教受容度", "気力", "BT値", "BT値上昇率"}
	sensitivities := []string{"口感度", "胸感度", "性器感度", "尻感度", "肌感度"}
	for index, name := range slgStatusCharacters {
		column := name
		offset := s.slgStatusPx(342) * index
		columnX := s.slgStatusPx(544) + offset
		progress := db.GetNamed("#ID_BP", column, params[0])
		s.drawSLGText(canvas, fmt.Sprintf("%d%%", progress), columnX, s.slgStatusPx(234), s.slgStatusFont(16), 0xffffffff)
		s.drawSLGText(canvas, fmt.Sprintf("%d", db.GetNamed("#ID_BP", column, params[1])), columnX, s.slgStatusPx(277), s.slgStatusFont(16), 0xffffffff)
		s.drawSLGText(canvas, fmt.Sprintf("%d", db.GetNamed("#ID_BP", column, params[2])), columnX, s.slgStatusPx(320), s.slgStatusFont(16), 0xffffffff)
		s.drawSLGText(canvas, fmt.Sprintf("%d", db.GetNamed("#ID_BP", column, params[3])), columnX, s.slgStatusPx(363), s.slgStatusFont(16), 0xffffffff)
		bt := db.GetNamed("#ID_BP", column, params[4])
		rate := db.GetNamed("#ID_BP", column, params[5])
		s.drawSLGText(canvas, fmt.Sprintf("%d(%d%%)", bt, rate), columnX, s.slgStatusPx(406), s.slgStatusFont(16), 0xffffffff)

		for row, parameter := range sensitivities {
			value := db.GetNamed("#ID_BP", column, parameter)
			x := s.slgStatusPx(442) + offset
			y := s.slgStatusPx(485 + 24*row)
			width := s.slgStatusPx(208)
			fill := int(float64(value) / 999.0 * float64(width))
			fill = clampInt(fill, 0, width)
			// 原版是紫→品红的渐变；这里用两层半透明色近似，避免引入
			// 额外的渐变 API，同时保留"条越长数值越大"的可读性。
			canvas.Fill(x, y, width, s.slgStatusPx(17), 30, 20, 80, 255)
			if fill > 0 {
				canvas.Fill(x+width-fill, y, fill, s.slgStatusPx(17), 40, 120, 220, 220)
			}
			s.drawSLGText(canvas, fmt.Sprintf("%d", value), x+width-s.slgStatusPx(58), y-s.slgStatusPx(2), s.slgStatusFont(14), 0xffffffff)
			s.drawSLGText(canvas, "/999", x+width-s.slgStatusPx(26), y+s.slgStatusPx(4), s.slgStatusFont(11), 0xffffffff)
		}
		// 状态位：status_icon1..8 各两帧。图标 9 在发行版里缺失，
		// 原 Lua 也会在 i=9 时拿到 nil，这里只画存在的 8 个。
		status := db.GetNamed("#ID_BP", column, "状態")
		for icon := 0; icon < 8; icon++ {
			name := fmt.Sprintf("status_icon%d", icon+1)
			image := s.composer.Load(name)
			if image == nil {
				continue
			}
			x := s.slgStatusPx(370+58*(icon%5)) + offset
			y := s.slgStatusPx(656 + 46*(icon/5))
			frame := 0
			if status&(1<<icon) != 0 {
				frame = 1
			}
			canvas.DrawRegion(image, 0, frame*(image.Height/2), image.Width, image.Height/max(1, 2), x, y)
		}
	}
}

// drawSLGStatusList 画"调教次数/结果"两页：三列各 18 行，行高 32。
//
// 原版每列有独立滚动条；这里共用一个滚动量，触屏上更直观，也不会有
// "拖动左列结果右列跳"的困惑。列表内容来自 var_db 的行顺序。
func (s *Session) drawSLGStatusList(canvas *gfx.BgraCanvas, dbName string) {
	db := s.ensureScriptDB()
	if db == nil {
		return
	}
	rows := db.RowCount(dbName)
	if rows == 0 {
		return
	}
	// 原 Lua 只把 count >= 0 的行为放进列表（-1 表示该角色还没解锁这个
	// 行为）；直接按 CSV 行号画会把 -1 也显示出来，既不是原版也会让
	// 已解锁项被挤到滚动区外。
	type entry struct {
		name  string
		value int32
	}
	lists := make([][]entry, len(slgStatusCharacters))
	total := 0
	for index, column := range slgStatusCharacters {
		for rowIndex := 0; rowIndex < rows; rowIndex++ {
			value := db.Get(dbName, rowIndex, column)
			if value < 0 {
				continue
			}
			lists[index] = append(lists[index], entry{name: db.RowName(dbName, rowIndex), value: value})
		}
		total = max(total, len(lists[index]))
	}
	const visible = 18
	maxScroll := max(0, total-visible)
	scroll := clampInt(s.statusScroll, 0, maxScroll)
	s.statusScroll = scroll
	for index, column := range slgStatusCharacters {
		_ = column
		offset := s.slgStatusPx(342) * index
		for row := 0; row < visible && scroll+row < len(lists[index]); row++ {
			item := lists[index][scroll+row]
			y := s.slgStatusPx(120 + (row+1)*32)
			if image := s.composer.Load("status_titem_basen"); image != nil {
				canvas.DrawRegion(image, 0, 0, image.Width, image.Height, s.slgStatusPx(346)+offset, y)
			}
			s.drawSLGText(canvas, item.name, s.slgStatusPx(352)+offset, y+s.slgStatusPx(8), s.slgStatusFont(14), 0xffffffff)
			s.drawSLGText(canvas, fmt.Sprintf("%d", item.value), s.slgStatusPx(612)+offset, y+s.slgStatusPx(5), s.slgStatusFont(15), 0xffffffff)
		}
		// 滚动条：status_tbar 位于每列右上。
		s.drawSLGStatusBar(canvas, "status_tbar", s.slgStatusPx(672)+offset, s.slgStatusPx(144), scroll, maxScroll, total, visible)
	}
}

func (s *Session) drawSLGStatusAchievement(canvas *gfx.BgraCanvas) {
	table := s.ensureAchievements()
	if table == nil {
		return
	}
	entries := table.Entries()
	const visible = 18
	maxScroll := max(0, len(entries)-visible)
	scroll := clampInt(s.statusScroll, 0, maxScroll)
	s.statusScroll = scroll
	for row := 0; row < visible && scroll+row < len(entries); row++ {
		entry := entries[scroll+row]
		unlocked := s.achievementUnlocked(entry.ID)
		name, notes := entry.Name, entry.Notes
		if !unlocked && entry.Secret {
			name, notes = "？？？", "？？？？？"
		}
		color := uint32(0xffffffff)
		if !unlocked {
			color = 0xff606060
		}
		y := s.slgStatusPx(152 + row*32)
		s.drawSLGText(canvas, name, s.slgStatusPx(373), y, s.slgStatusFont(14), color)
		s.drawSLGText(canvas, notes, s.slgStatusPx(639), y, s.slgStatusFont(14), color)
	}
	s.drawSLGStatusBar(canvas, "status_abar", s.slgStatusPx(1312), s.slgStatusPx(149), scroll, maxScroll, len(entries), visible)
}

// drawSLGStatusBar 画滚动条的滑块。原图是整条轨道，滑块用半透明亮条表示。
func (s *Session) drawSLGStatusBar(canvas *gfx.BgraCanvas, name string, x, y, scroll, maxScroll, total, visible int) {
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	canvas.Draw(image, x, y)
	if maxScroll <= 0 || total <= 0 {
		return
	}
	thumbHeight := max(18, image.Height*visible/total)
	fraction := float64(scroll) / float64(maxScroll)
	thumbY := y + int(fraction*float64(max(image.Height-thumbHeight, 0)))
	canvas.Fill(x, thumbY, max(4, image.Width-4), thumbHeight, 255, 255, 255, 170)
}

func (s *Session) drawSLGText(canvas *gfx.BgraCanvas, text string, x, y, size int, color uint32) {
	if s.textFont == nil || text == "" {
		return
	}
	image := s.textFont.Render(text, size, 1, 1, size+4, 520, 1, color)
	if image != nil {
		rendering.DrawTextImage(canvas, image, x, y)
	}
}

// ---------------------------------------------------------------- 交互

func (s *Session) hitSLGStatusScreen(x, y int) uiHit {
	for index, button := range s.slgStatusTabs() {
		if hitButton(s, button, x, y) {
			if index == 4 {
				return uiHit{kind: hitStatusButton, index: 4}
			}
			return uiHit{kind: hitStatusMode, index: index}
		}
	}
	if s.slgStatusScrollable() {
		// 整条右侧区域都能按：滚动条很窄，手指命中需要余量。
		if x >= s.width-80 && y >= 120 && y <= 720 {
			return uiHit{kind: hitStatusScroll}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) slgStatusScrollable() bool {
	state := s.slgStatus()
	return state != nil && state.page != slgStatusBasic
}

func (s *Session) activateSLGStatusScreen(hit uiHit, x, y int) {
	state := s.slgStatus()
	if state == nil {
		return
	}
	switch hit.kind {
	case hitStatusMode:
		if hit.index >= 0 && hit.index < 4 {
			state.page = hit.index
			s.statusPage = hit.index
			s.statusScroll = 0
			state.dragging = false
			s.composer.Invalidate()
		}
	case hitStatusButton:
		s.closeSystemUI()
	case hitStatusScroll:
		state.dragging = true
		s.scrollSLGStatusTo(y)
	}
}

func (s *Session) scrollSLGStatusTo(y int) {
	dbTotal := 0
	if state := s.slgStatus(); state != nil {
		switch state.page {
		case slgStatusTraining:
			if db := s.ensureScriptDB(); db != nil {
				dbTotal = db.RowCount("#ID_TC")
			}
		case slgStatusResult:
			if db := s.ensureScriptDB(); db != nil {
				dbTotal = db.RowCount("#ID_LOG")
			}
		case slgStatusAchievement:
			if table := s.ensureAchievements(); table != nil {
				dbTotal = table.Len()
			}
		}
	}
	maxScroll := max(0, dbTotal-18)
	if maxScroll == 0 {
		return
	}
	trackTop, trackHeight := 144, 592
	fraction := float64(y-trackTop) / float64(max(1, trackHeight))
	fraction = max(0, min(1, fraction))
	scroll := int(fraction*float64(maxScroll) + 0.5)
	if scroll != s.statusScroll {
		s.statusScroll = scroll
		s.composer.Invalidate()
	}
}

func (s *Session) wheelSLGStatus(delta int32) {
	state := s.slgStatus()
	if state == nil {
		return
	}
	total := 0
	switch state.page {
	case slgStatusTraining:
		if db := s.ensureScriptDB(); db != nil {
			total = db.RowCount("#ID_TC")
		}
	case slgStatusResult:
		if db := s.ensureScriptDB(); db != nil {
			total = db.RowCount("#ID_LOG")
		}
	case slgStatusAchievement:
		if table := s.ensureAchievements(); table != nil {
			total = table.Len()
		}
	default:
		return
	}
	maxScroll := max(0, total-18)
	s.statusScroll = clampInt(s.statusScroll+int(delta), 0, maxScroll)
	s.composer.Invalidate()
}

func (s *Session) dragSLGStatus(y int) bool {
	state := s.slgStatus()
	if state == nil || !state.dragging {
		return false
	}
	s.scrollSLGStatusTo(y)
	return true
}

// slgStatusColumnIndex 保留给后续"按列滚动"扩展：原版三列独立滚动，
// 当前共用一个滚动量，但列名映射已经在这里固定，避免以后再猜列名。
func slgStatusColumnIndex(column string) int {
	for index, name := range slgStatusCharacters {
		if strings.EqualFold(name, column) {
			return index
		}
	}
	return -1
}
