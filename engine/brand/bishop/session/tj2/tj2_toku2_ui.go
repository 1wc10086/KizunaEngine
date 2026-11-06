package tj2

// tj2_toku2_ui.go 是 特別授業2（Toku2 界面家族）的标题、系统菜单、鉴赏室与配置。
//
// 为什么不复用 tj2_classic_ui.go 的 GK 坐标：Toku2 的素材是另一套 TJ2.EXE
// 界面（title03、cgmode1-3、scene1-3、music1-2），按钮位置和页容量都不同。
// 两者共用“停在界面等点击”的状态机，但布局按 profile 分流，避免互相覆盖。
//
// 所有坐标都是游戏自带的 640x480 画布坐标；宿主只按画面缩放铺透明热区。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

const (
	// 标题/鉴赏按钮的实测矩形（见素材像素分析）。
	toku2TitleX = 464
	toku2TitleW = 160
	toku2TitleH = 40

	toku2CGPerPage    = 20
	toku2ScenePerPage = 20
)

var (
	toku2GalleryPrevRect = tj2Rect{X: 120, Y: 424, W: 112, H: 40}
	toku2GalleryExitRect = tj2Rect{X: 264, Y: 424, W: 112, H: 40}
	toku2GalleryNextRect = tj2Rect{X: 408, Y: 424, W: 112, H: 40}
	toku2MusicStopRect   = tj2Rect{X: 344, Y: 424, W: 112, H: 40}
	toku2MusicExitRect   = tj2Rect{X: 470, Y: 424, W: 112, H: 40}
)

// ---- 标题 ----

// EnterTitle 把会话切到标题。脚本 VM 保持不动：玩家点“初めから”时
// 才 Restart，点“続きから”只恢复存档。
func (s *Tj2Session) EnterTitle() state.SessionStep {
	s.titleMode = true
	s.titleHover = -1
	s.warnVisible = false
	s.uiMode = tj2UINone
	s.uiReturnTitle = false
	s.uiReturnSystem = false
	s.messagePages = nil
	s.messagePage = 0
	s.messageHover = 0
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearChoices()
	s.gameState.SetMessageWindowVisible(false)
	s.gameState.ClearBgm()
	s.gameState.ClearSoundEffect()
	s.gameState.ClearVoice()
	s.bgmTrack = -1
	s.standees.Clear()
	s.stage.ClearAll()
	// 用 title03 而不是 title01：title03 自带“鑑賞室”项。我们还没有原版
	// GAMEFLG.DAT 的解析，默认放行鉴赏室，避免玩家验收时看不到入口。
	if image := s.images.Get("title03"); image != nil {
		s.stage.SetBackground("title03", image, 0, 0)
	} else if image := s.images.Get("title01"); image != nil {
		s.stage.SetBackground("title01", image, 0, 0)
	}
	s.stage.Invalidate()
	core.Logf("tj2: tok2 title presented")
	return state.SessionAwaitTitle
}

// composeTitleScreen 画标题底图与悬停高亮。
func (s *Tj2Session) composeTitleScreen() bool {
	dirty := s.stage.ComposeTo(s.frame, false)
	if !s.IsToku2UI() {
		return dirty
	}
	items := s.titleMenuItemsForProfile()
	if s.titleHover < 0 || s.titleHover >= len(items) {
		return dirty
	}
	overlay := s.classicKeyedImage("title04")
	if overlay == nil {
		overlay = s.classicKeyedImage("title02")
	}
	if overlay == nil {
		return dirty
	}
	item := items[s.titleHover]
	s.frame.BlitRegion(overlay, item.X, item.Y, item.W, item.H, item.X, item.Y, 255)
	return true
}

// selectToku2TitleItem 处理 Toku2 标题的四个动作。
func (s *Tj2Session) selectToku2TitleItem(key string) state.SessionStep {
	switch key {
	case "start":
		s.titleMode = false
		s.titleHover = -1
		s.showWarning()
		return state.SessionAwaitInput
	case "load":
		if !s.QuickSaveExists() {
			return state.SessionAwaitTitle
		}
		s.titleMode = false
		s.titleHover = -1
		s.uiReturnTitle = false
		if s.LoadFrom(0) {
			s.finishClassicLoad()
			return state.SessionAwaitInput
		}
		return s.EnterTitle()
	case "museum":
		s.titleMode = false
		s.titleHover = -1
		s.uiReturnTitle = true
		return s.openGalleryRoot()
	case "quit":
		s.gameState.SetAwaitingInput(false)
		return state.SessionFinished
	default:
		return state.SessionAwaitTitle
	}
}

// ---- 警告画面 ----

func (s *Tj2Session) showWarning() {
	s.warnVisible = true
	s.stage.ClearAll()
	if image := s.images.Get("KEIKOKU"); image != nil {
		s.stage.SetBackground("KEIKOKU", image, 0, 0)
	}
	s.stage.Invalidate()
	s.gameState.SetMessageWindowVisible(false)
	core.Logf("tj2: tok2 warning presented")
}

func (s *Tj2Session) dismissWarning() state.SessionStep {
	if !s.warnVisible {
		return state.SessionAwaitInput
	}
	s.warnVisible = false
	s.titleMode = false
	s.Restart()
	core.Logf("tj2: tok2 warning dismissed, starting SCN000")
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) composeWarning() bool {
	s.stage.ComposeTo(s.frame, false)
	return true
}

// ---- 游戏内菜单按钮 ----

const (
	toku2MenuButtonX = 592
	toku2MenuButtonY = 6
	toku2MenuButtonW = 42
	toku2MenuButtonH = 28
)

func (s *Tj2Session) engineMenuButtonHit(x, y int) bool {
	if !s.IsToku2UI() {
		return false
	}
	return x >= toku2MenuButtonX && x < toku2MenuButtonX+toku2MenuButtonW &&
		y >= toku2MenuButtonY && y < toku2MenuButtonY+toku2MenuButtonH
}

func (s *Tj2Session) drawEngineMenuButton() {
	if !s.IsToku2UI() {
		return
	}
	s.frame.FillBlend(toku2MenuButtonX, toku2MenuButtonY, toku2MenuButtonW, toku2MenuButtonH, 0, 0, 0, 150)
	for i := 0; i < 3; i++ {
		s.frame.Fill(toku2MenuButtonX+8, toku2MenuButtonY+7+i*6, toku2MenuButtonW-16, 2, 235, 235, 235, 255)
	}
}

// ---- 系统菜单 ----

type toku2MenuItem struct {
	key   string
	label string
	rect  tj2Rect
}

func (s *Tj2Session) toku2SystemItems() []toku2MenuItem {
	labels := []string{"セーブ", "ロード", "環境設定", "鑑賞室", "タイトルへ", "終了", "閉じる"}
	keys := []string{"save", "load", "config", "museum", "title", "quit", "close"}
	items := make([]toku2MenuItem, 0, len(keys))
	for i := range keys {
		items = append(items, toku2MenuItem{
			key:   keys[i],
			label: labels[i],
			rect:  tj2Rect{X: 200, Y: 142 + i*36, W: 240, H: 30},
		})
	}
	return items
}

func (s *Tj2Session) openToku2System() state.SessionStep {
	s.beginClassicUI(tj2UISystem, false)
	return state.SessionAwaitSelect
}

func (s *Tj2Session) toku2SystemClick(x, y int) state.SessionStep {
	for _, item := range s.toku2SystemItems() {
		if !tj2Hit(x, y, item.rect) {
			continue
		}
		switch item.key {
		case "save":
			s.uiReturnSystem = false
			s.uiReturnTitle = false
			return s.quickSave()
		case "load":
			s.uiReturnSystem = false
			s.uiReturnTitle = false
			return s.quickLoad()
		case "config":
			s.uiReturnSystem = true
			s.uiMode = tj2UIConfig
			s.uiRevision++
			return state.SessionAwaitSelect
		case "museum":
			s.uiReturnTitle = false
			return s.openGalleryRoot()
		case "title":
			return s.EnterTitle()
		case "quit":
			return state.SessionFinished
		case "close":
			s.uiMode = tj2UINone
			s.uiRevision++
			s.stage.Invalidate()
			return s.currentStepForUI()
		}
	}
	return state.SessionAwaitSelect
}

func (s *Tj2Session) drawToku2System() {
	s.frame.FillBlend(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 150)
	s.frame.FillBlend(180, 90, 280, 330, 20, 14, 10, 235)
	s.drawUIBorder(tj2Rect{X: 180, Y: 90, W: 280, H: 330})
	s.drawUIText("システム", 270, 98, 24, 28, 120, 1, tj2UIColorCream)
	for i, item := range s.toku2SystemItems() {
		if s.uiHover == i {
			s.frame.FillBlend(item.rect.X, item.rect.Y, item.rect.W, item.rect.H, 230, 140, 80, 80)
		}
		s.drawUIButton(item.rect, item.label)
	}
}

// ---- 鉴赏室根菜单 ----

type toku2GalleryEntry struct {
	key   string
	label string
	rect  tj2Rect
}

func (s *Tj2Session) toku2GalleryRootItems() []toku2GalleryEntry {
	return []toku2GalleryEntry{
		{key: "cg", label: "CG鑑賞", rect: tj2Rect{X: 190, Y: 150, W: 260, H: 44}},
		{key: "scene", label: "シーン回想", rect: tj2Rect{X: 190, Y: 210, W: 260, H: 44}},
		{key: "music", label: "音楽鑑賞", rect: tj2Rect{X: 190, Y: 270, W: 260, H: 44}},
		{key: "back", label: "戻る", rect: tj2Rect{X: 190, Y: 350, W: 260, H: 44}},
	}
}

func (s *Tj2Session) openGalleryRoot() state.SessionStep {
	s.beginClassicUI(tj2UIGalleryRoot, false)
	return state.SessionAwaitSelect
}

func (s *Tj2Session) toku2GalleryRootClick(x, y int) state.SessionStep {
	for _, item := range s.toku2GalleryRootItems() {
		if !tj2Hit(x, y, item.rect) {
			continue
		}
		switch item.key {
		case "cg":
			s.classicThumbNames()
			s.beginClassicUI(tj2UICG, false)
			return state.SessionAwaitSelect
		case "scene":
			s.classicThumbNames()
			s.beginClassicUI(tj2UIScene, false)
			return state.SessionAwaitSelect
		case "music":
			s.openClassicSound()
			return state.SessionAwaitSelect
		case "back":
			return s.leaveGalleryRoot()
		}
	}
	return state.SessionAwaitSelect
}

// leaveGalleryRoot 根据入口返回标题或游戏。
func (s *Tj2Session) leaveGalleryRoot() state.SessionStep {
	s.uiMode = tj2UINone
	s.uiRevision++
	s.stage.Invalidate()
	if s.uiReturnTitle {
		s.uiReturnTitle = false
		return s.EnterTitle()
	}
	s.gameState.SetAwaitingInput(false)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) drawToku2GalleryRoot() {
	s.frame.FillBlend(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 170)
	s.frame.FillBlend(170, 100, 300, 330, 22, 16, 12, 235)
	s.drawUIBorder(tj2Rect{X: 170, Y: 100, W: 300, H: 330})
	s.drawUIText("鑑賞室", 270, 108, 28, 32, 120, 1, tj2UIColorCream)
	for i, item := range s.toku2GalleryRootItems() {
		if s.uiHover == i {
			s.frame.FillBlend(item.rect.X, item.rect.Y, item.rect.W, item.rect.H, 230, 140, 80, 80)
		}
		s.drawUIButton(item.rect, item.label)
	}
}

// ---- 配置 ----

type toku2ConfigItem struct {
	key   string
	label string
	rect  tj2Rect
}

func (s *Tj2Session) toku2ConfigItems() []toku2ConfigItem {
	return []toku2ConfigItem{
		{key: "speed0", label: "文字速度 遅い", rect: tj2Rect{X: 180, Y: 150, W: 280, H: 30}},
		{key: "speed1", label: "文字速度 普通", rect: tj2Rect{X: 180, Y: 186, W: 280, H: 30}},
		{key: "speed2", label: "文字速度 速い", rect: tj2Rect{X: 180, Y: 222, W: 280, H: 30}},
		{key: "speed3", label: "文字速度 瞬間", rect: tj2Rect{X: 180, Y: 258, W: 280, H: 30}},
		{key: "skip", label: "既読もスキップ", rect: tj2Rect{X: 180, Y: 296, W: 280, H: 28}},
		{key: "bgm", label: "音楽", rect: tj2Rect{X: 180, Y: 330, W: 280, H: 28}},
		{key: "se", label: "効果音", rect: tj2Rect{X: 180, Y: 362, W: 280, H: 28}},
		{key: "voice", label: "音声", rect: tj2Rect{X: 180, Y: 394, W: 280, H: 28}},
		{key: "back", label: "戻る", rect: tj2Rect{X: 180, Y: 430, W: 280, H: 32}},
	}
}

func (s *Tj2Session) toku2ConfigClick(x, y int) state.SessionStep {
	for _, item := range s.toku2ConfigItems() {
		if !tj2Hit(x, y, item.rect) {
			continue
		}
		switch {
		case strings.HasPrefix(item.key, "speed"):
			s.textSpeedIndex = int(item.key[5] - '0')
			s.gameState.SetTextSpeed([]float64{0.5, 1.0, 1.6, 4.0}[s.textSpeedIndex])
		case item.key == "skip":
			s.skipUnread = !s.skipUnread
		case item.key == "bgm":
			s.bgmOn = !s.bgmOn
		case item.key == "se":
			s.seOn = !s.seOn
		case item.key == "voice":
			s.voiceOn = !s.voiceOn
		case item.key == "back":
			if s.uiReturnSystem {
				s.uiReturnSystem = false
				s.uiMode = tj2UISystem
			} else if s.uiReturnTitle {
				s.uiMode = tj2UINone
				return s.EnterTitle()
			} else {
				s.uiMode = tj2UINone
				s.stage.Invalidate()
				return s.currentStepForUI()
			}
		}
		s.uiRevision++
		return state.SessionAwaitSelect
	}
	return state.SessionAwaitSelect
}

func (s *Tj2Session) drawToku2Config() {
	s.frame.FillBlend(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 170)
	s.frame.FillBlend(170, 90, 300, 390, 20, 14, 10, 250)
	s.drawUIBorder(tj2Rect{X: 170, Y: 90, W: 300, H: 390})
	s.drawUIText("環境設定", 270, 106, 26, 30, 120, 1, tj2UIColorCream)
	for i, item := range s.toku2ConfigItems() {
		label := item.label
		switch item.key {
		case "speed0", "speed1", "speed2", "speed3":
			if int(item.key[5]-'0') == s.textSpeedIndex {
				label = "▶ " + label
			}
		case "skip":
			if s.skipUnread {
				label += "  ON"
			} else {
				label += "  OFF"
			}
		case "bgm":
			if s.bgmOn {
				label += "  ON"
			} else {
				label += "  OFF"
			}
		case "se":
			if s.seOn {
				label += "  ON"
			} else {
				label += "  OFF"
			}
		case "voice":
			if s.voiceOn {
				label += "  ON"
			} else {
				label += "  OFF"
			}
		}
		if s.uiHover == i {
			s.frame.FillBlend(item.rect.X, item.rect.Y, item.rect.W, item.rect.H, 230, 140, 80, 80)
		}
		s.drawUIButton(item.rect, label)
	}
}

// ---- 回想（LOG） ----

const toku2BacklogMax = 100

func (s *Tj2Session) appendMessageHistory(page string) {
	page = strings.TrimSpace(page)
	if page == "" {
		return
	}
	s.messageHistory = append(s.messageHistory, page)
	if len(s.messageHistory) > toku2BacklogMax {
		s.messageHistory = s.messageHistory[len(s.messageHistory)-toku2BacklogMax:]
	}
}

func (s *Tj2Session) openBacklog() state.SessionStep {
	s.beginClassicUI(tj2UIBacklog, false)
	return state.SessionAwaitSelect
}

func (s *Tj2Session) drawToku2Backlog() {
	s.frame.FillBlend(0, 0, ScreenWidth, ScreenHeight, 8, 8, 14, 255)
	s.drawUIText("バックログ", 26, 14, 24, 28, 300, 1, tj2UIColorCream)
	lines := s.messageHistory
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	y := 56
	for _, line := range lines {
		line = strings.ReplaceAll(line, "\n", " ")
		s.drawUIText(clipTj2Text(line, 44), 26, y, 18, 22, 588, 1, tj2UIColorWhite)
		y += 30
	}
	s.drawUIButton(tj2Rect{X: 264, Y: 430, W: 112, H: 34}, "閉じる")
}

// ---- 界面状态共用 ----

// QuickSaveExists 原版只有一个存档文件（GAMEFLG.DAT）。
func (s *Tj2Session) QuickSaveExists() bool {
	return s.tj2Saves.Inspect(0).Exists
}

// quickSave 直接覆盖唯一存档，不打开槽位界面。
func (s *Tj2Session) quickSave() state.SessionStep {
	s.uiMode = tj2UINone
	s.uiRevision++
	s.stage.Invalidate()
	// 缩略图取当前剧情帧；先清掉上一次存档界面留下的缓存。
	s.uiSaveThumb = nil
	if !s.SaveTo(0) {
		core.Logf("tj2: quick save failed")
	}
	return s.currentStepForUI()
}

// quickLoad 直接读取唯一存档。
func (s *Tj2Session) quickLoad() state.SessionStep {
	s.uiMode = tj2UINone
	s.uiRevision++
	s.stage.Invalidate()
	if !s.LoadFrom(0) {
		core.Logf("tj2: quick load missing")
		return s.currentStepForUI()
	}
	return s.finishClassicLoad()
}

// currentStepForUI 关闭系统界面后回到“等玩家点击”的剧情状态。
func (s *Tj2Session) currentStepForUI() state.SessionStep {
	if s.gameState.AwaitingInput() {
		return state.SessionAwaitInput
	}
	if s.gameState.AwaitingChoice() {
		return state.SessionAwaitChoice
	}
	return state.SessionAwaitInput
}

// toku2UIHoverAt 给新界面提供悬停下标；-1 表示没有。
func (s *Tj2Session) toku2UIHoverAt(x, y int) int {
	hits := s.toku2UIHits()
	for i, rect := range hits {
		if tj2Hit(x, y, rect) {
			return i
		}
	}
	return -1
}

func (s *Tj2Session) toku2UIHits() []tj2Rect {
	switch s.uiMode {
	case tj2UISystem:
		items := s.toku2SystemItems()
		out := make([]tj2Rect, 0, len(items))
		for _, item := range items {
			out = append(out, item.rect)
		}
		return out
	case tj2UIGalleryRoot:
		items := s.toku2GalleryRootItems()
		out := make([]tj2Rect, 0, len(items))
		for _, item := range items {
			out = append(out, item.rect)
		}
		return out
	case tj2UIConfig:
		items := s.toku2ConfigItems()
		out := make([]tj2Rect, 0, len(items))
		for _, item := range items {
			out = append(out, item.rect)
		}
		return out
	case tj2UIBacklog:
		return []tj2Rect{{X: 264, Y: 430, W: 112, H: 34}}
	}
	return nil
}

// composeToku2UIScreen 是 uiMode 为 Toku2 专属界面时的绘制入口。
func (s *Tj2Session) composeToku2UIScreen() bool {
	switch s.uiMode {
	case tj2UISystem:
		// 系统/配置/回想是覆盖层：先重画剧情帧，再叠面板。不能依赖
		// 上一帧，否则从存档界面切过来会把存档面板留在背后。
		s.stage.Invalidate()
		s.stage.ComposeTo(s.frame, false)
		s.drawToku2System()
	case tj2UIGalleryRoot:
		s.stage.Invalidate()
		s.stage.ComposeTo(s.frame, false)
		s.drawToku2GalleryRoot()
	case tj2UIConfig:
		s.stage.Invalidate()
		s.stage.ComposeTo(s.frame, false)
		s.drawToku2Config()
	case tj2UIBacklog:
		s.stage.Invalidate()
		s.stage.ComposeTo(s.frame, false)
		s.drawToku2Backlog()
	default:
		return false
	}
	return true
}

// drawToku2Keyed 在系统界面上叠加带绿键的覆盖层。
func (s *Tj2Session) drawToku2Keyed(name string) {
	if overlay := s.classicKeyedImage(name); overlay != nil {
		s.frame.Blit(overlay, 0, 0, 255)
	}
}

// drawToku2GalleryFrame 画鉴赏室底图并返回本页按钮是否可用。
//
// 原版用 cgmode1/2/3（首/末/中间页）区分按钮组合；素材里按钮文字已经烘焙，
// 我们只画对应的底图，命中区按同一组矩形。
func (s *Tj2Session) drawToku2GalleryFrame(kind string, page, pages int) {
	base := kind + "1"
	switch {
	case pages <= 1:
		base = kind + "1"
	case page == 0:
		base = kind + "1"
	case page >= pages-1:
		base = kind + "2"
	default:
		base = kind + "3"
	}
	if image := s.images.Get(base); image != nil {
		s.frame.Blit(image, 0, 0, 255)
	}
}

func (s *Tj2Session) toku2GalleryPageButtons(page, pages int) (prev, exit, next bool) {
	return page > 0, true, page < pages-1
}

// toku2CellRect 计算 5x4 鉴赏格；x=68+106*c，y=76+84*r。
func toku2CellRect(index int) tj2Rect {
	return tj2Rect{
		X: 68 + (index%5)*106,
		Y: 76 + (index/5)*84,
		W: 80,
		H: 60,
	}
}

func (s *Tj2Session) drawToku2GalleryGrid(kind string, all []string, page int) {
	pages := (len(all) + toku2CGPerPage - 1) / toku2CGPerPage
	if pages < 1 {
		pages = 1
	}
	s.drawToku2GalleryFrame(kind, page, pages)
	start := page * toku2CGPerPage
	lockedFilter := len(s.unlockedGallery) > 0
	for i := 0; i < toku2CGPerPage && start+i < len(all); i++ {
		rect := toku2CellRect(i)
		if lockedFilter {
			if _, ok := s.unlockedGallery[start+i]; !ok {
				// 原版只显示已解锁的 CG；没有任何解锁数据时（首次游玩）
				// 退回全显示，避免鉴赏室全黑无法验证。
				s.frame.Fill(rect.X, rect.Y, rect.W, rect.H, 0, 0, 0, 255)
				continue
			}
		}
		if image := s.images.Get(all[start+i]); image != nil {
			s.frame.BlitScaled(image, rect.X, rect.Y, rect.W, rect.H, 255)
		}
		if s.uiHover == start+i {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 80, 70)
		}
	}
	s.drawUIText(fmt.Sprintf("%d / %d", page+1, pages), 20, 430, 18, 22, 100, 1, tj2UIColorWhite)
}

// drawToku2GalleryButtons 目前不需要画字：cgmode1/2/3 与 scene1/2/3
// 已经把「前頁/終了/次頁」烘焙进底图，再叠一遍会重影。
// 保留函数是为了让翻页/退出逻辑和绘制入口一一对应。
func (s *Tj2Session) drawToku2GalleryButtons(page, pages int) {
	_, _, _ = s.toku2GalleryPageButtons(page, pages)
}

// toku2GalleryClick 处理鉴赏室的翻页/退出/格子。返回 (step, handled)。
func (s *Tj2Session) toku2GalleryClick(x, y int, all []string, kind string) (state.SessionStep, bool) {
	pages := (len(all) + toku2CGPerPage - 1) / toku2CGPerPage
	if pages < 1 {
		pages = 1
	}
	prev, exit, next := s.toku2GalleryPageButtons(s.uiPage, pages)
	if prev && tj2Hit(x, y, toku2GalleryPrevRect) && s.uiPage > 0 {
		s.uiPage--
		s.uiHover = -1
		s.uiRevision++
		return state.SessionAwaitSelect, true
	}
	if next && tj2Hit(x, y, toku2GalleryNextRect) && s.uiPage+1 < pages {
		s.uiPage++
		s.uiHover = -1
		s.uiRevision++
		return state.SessionAwaitSelect, true
	}
	if exit && tj2Hit(x, y, toku2GalleryExitRect) {
		return s.openGalleryRoot(), true
	}
	start := s.uiPage * toku2CGPerPage
	lockedFilter := len(s.unlockedGallery) > 0
	for i := 0; i < toku2CGPerPage && start+i < len(all); i++ {
		if tj2Hit(x, y, toku2CellRect(i)) {
			if lockedFilter {
				if _, ok := s.unlockedGallery[start+i]; !ok {
					return state.SessionAwaitSelect, false
				}
			}
			s.uiSelected = start + i
			s.uiCgOpen = true
			s.uiRevision++
			return state.SessionAwaitSelect, true
		}
	}
	return state.SessionAwaitSelect, false
}

// toku2FullImageName 由缩略图名得到正片名；THUMB 的条目名去扩展名 + .KG。
func toku2FullImageName(thumb string) string {
	stem := thumb
	if dot := strings.LastIndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	return stem + ".KG"
}

// drawToku2FullView 画 CG/场景的大图预览与翻页按钮。
func (s *Tj2Session) drawToku2FullView(all []string) {
	s.frame.Clear(0, 0, 0, 255)
	if s.uiSelected >= 0 && s.uiSelected < len(all) {
		if _, ok := s.unlockedGallery[s.uiSelected]; len(s.unlockedGallery) > 0 && !ok {
			s.drawUIText("LOCKED", 270, 220, 24, 28, 120, 1, tj2UIColorDim)
			return
		}
		if image := s.images.Get(toku2FullImageName(all[s.uiSelected])); image != nil {
			s.frame.Draw(image, image.OffsetX, image.OffsetY)
		}
	}
	s.frame.FillBlend(0, 424, ScreenWidth, 56, 0, 0, 0, 150)
	s.drawUIText("前頁", toku2GalleryPrevRect.X+8, toku2GalleryPrevRect.Y+6, 20, 24, 100, 1, tj2UIColorCream)
	s.drawUIText("戻る", toku2GalleryExitRect.X+20, toku2GalleryExitRect.Y+6, 20, 24, 80, 1, tj2UIColorCream)
	s.drawUIText("次頁", toku2GalleryNextRect.X+8, toku2GalleryNextRect.Y+6, 20, 24, 100, 1, tj2UIColorCream)
	if len(all) > 0 {
		s.drawUIText(fmt.Sprintf("%d / %d", s.uiSelected+1, len(all)), 20, 432, 16, 20, 90, 1, tj2UIColorWhite)
	}
}

func (s *Tj2Session) toku2FullClick(x, y int, all []string) bool {
	switch {
	case tj2Hit(x, y, toku2GalleryPrevRect):
		if len(all) > 0 {
			s.uiSelected--
			if s.uiSelected < 0 {
				s.uiSelected = len(all) - 1
			}
			s.uiPage = s.uiSelected / toku2CGPerPage
		}
	case tj2Hit(x, y, toku2GalleryNextRect):
		if len(all) > 0 {
			s.uiSelected++
			if s.uiSelected >= len(all) {
				s.uiSelected = 0
			}
			s.uiPage = s.uiSelected / toku2CGPerPage
		}
	case tj2Hit(x, y, toku2GalleryExitRect):
		s.uiCgOpen = false
	default:
		return false
	}
	s.uiRevision++
	return true
}

// drawToku2Music 画音乐鉴赏：music1 底图 + music2 绿键覆盖层。
func (s *Tj2Session) drawToku2Music() {
	if image := s.images.Get("music1"); image != nil {
		s.frame.Blit(image, 0, 0, 255)
	}
	s.drawToku2Keyed("music2")
	for i := 0; i < len(s.soundTracks) && i < 19; i++ {
		rect := s.toku2MusicRowRect(i)
		if s.uiSelected == i {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 160, 80, 90)
		} else if s.uiHover == i {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 70, 45)
		}
	}
}

func (s *Tj2Session) toku2MusicRowRect(index int) tj2Rect {
	if index < 10 {
		return tj2Rect{X: 48, Y: 64 + index*40, W: 248, H: 32}
	}
	return tj2Rect{X: 344, Y: 64 + (index-10)*40, W: 248, H: 32}
}

func (s *Tj2Session) toku2MusicClick(x, y int) state.SessionStep {
	if tj2Hit(x, y, toku2MusicStopRect) {
		s.stopClassicSound()
		s.uiRevision++
		return state.SessionAwaitSelect
	}
	if tj2Hit(x, y, toku2MusicExitRect) {
		s.stopClassicSound()
		return s.openGalleryRoot()
	}
	for i := 0; i < len(s.soundTracks) && i < 19; i++ {
		if tj2Hit(x, y, s.toku2MusicRowRect(i)) {
			s.uiSelected = i
			s.playClassicSound()
			s.uiRevision++
			return state.SessionAwaitSelect
		}
	}
	return state.SessionAwaitSelect
}

// Toku2 的线索图标（PCLICK）是地图光标，不是本文件职责；地图仍走
// tj2_map_screen.go 的现代分支，避免改动已适配的地图逻辑。

// markGalleryByName 把剧情里出现过的 CG 名映射到 THUMB 网格下标。
//
// 原版在显示 CG 时写 GAMEFLG.DAT 的解锁位；我们还没有该文件格式，
// 但可以按缩略图名做同样的“看过即解锁”，鉴赏室就不会全黑。
func (s *Tj2Session) markGalleryByName(name string) {
	if s.disposed || name == "" {
		return
	}
	// 剧情里第一次出现 CG 时鉴赏室还没打开过，缩略图列表可能尚未建立。
	s.classicThumbNames()
	if len(s.cgThumbs) == 0 {
		return
	}
	stem := strings.ToUpper(strings.TrimSuffix(name, ".KG"))
	stem = strings.TrimSuffix(stem, ".kg")
	for index, thumb := range s.cgThumbs {
		thumbStem := strings.ToUpper(thumb)
		if dot := strings.LastIndexByte(thumbStem, '.'); dot >= 0 {
			thumbStem = thumbStem[:dot]
		}
		if thumbStem == stem {
			s.UnlockGallery(index)
			return
		}
	}
}

// loadGalleryUnlocks 读取鉴赏解锁位。原版存在 GAMEFLG.DAT；我们还没有
// 该文件的位图布局，先把“看过哪些缩略图”存成同目录的 gallery.json，
// 至少跨启动保留进度。
func (s *Tj2Session) loadGalleryUnlocks() {
	if s.galleryPath == "" {
		return
	}
	data, err := os.ReadFile(s.galleryPath)
	if err != nil || len(data) == 0 {
		return
	}
	var indexes []int
	if json.Unmarshal(data, &indexes) != nil {
		return
	}
	for _, index := range indexes {
		if index >= 0 {
			s.unlockedGallery[index] = struct{}{}
		}
	}
}

func (s *Tj2Session) flushGalleryUnlocks() {
	if s.galleryPath == "" || len(s.unlockedGallery) == 0 {
		return
	}
	indexes := make([]int, 0, len(s.unlockedGallery))
	for index := range s.unlockedGallery {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	data, err := json.Marshal(indexes)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.galleryPath), 0o755)
	_ = os.WriteFile(s.galleryPath, data, 0o644)
}
