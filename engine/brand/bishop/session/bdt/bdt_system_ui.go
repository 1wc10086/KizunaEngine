package bdt

import (
	"strconv"
	"strings"
	"sync"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

// bdt_system_ui.go 实现 BDT（放課後～濡れた制服～）原引擎的内置界面。
//
// AF.EXE 把标题、鉴赏室、设置和存读档都做成了引擎画面：底图是一张 640x480
// 的 BMP，命中区是另一张同尺寸的纯色图（s_*h）。脚本里的 T 指令只负责"打开
// 标题并等待选择"。这里按同一套素材还原：
//   - 标题：s_title1 已经烤了菜单文字，s_title3 是 7 条命中带；
//   - 音楽鑑賞室：s_mu01 一页 20 首，s_mu04 是 20 条命中带 + 戻る；
//   - CG/シーン鑑賞室：s_cgNNn / s_scNNn 是 6x4 / 6x4 的预制缩略图页，
//     s_cgNNh / s_scNNh 给翻页和缩略格命中；
//   - 設定：s_sys012 音環境設定、s_sys013 画面環境設定；
//   - 続きから：s_sys011 呼出帳，20 行（01..19 + AT）。
//
// 这些界面不写进脚本，所以必须由引擎在 Compose 之后覆盖绘制，并在 Pointer
// 里先于"推进对白"消费点击。

type bdtSystemKind int

const (
	bdtSysNone bdtSystemKind = iota
	bdtSysMusic
	bdtSysCG
	bdtSysScene
	bdtSysConfigSound
	bdtSysConfigScreen
	bdtSysLoad
)

type bdtSystemUI struct {
	kind bdtSystemKind
	page int
	// musicRow / thumb 是当前播放曲目或选中的缩略格，-1 表示没有。
	musicRow int
	thumb    int
	// 音环境设定的三条滑杆（0..1）。BDT 只有一个 BGM 通道写进 GameState，
	// SE/语音音量在 Session 里单独保存并乘到 AudioEvent。
	musicVolume  float64
	effectVolume float64
	voiceVolume  float64
	// 画面环境设定的开关。
	fullscreen bool
	effectOn   bool
	skipUnread bool
	fastText   bool
	measure    int
	skipFast   bool
	autoSpeed  float64
	// 7 个角色的语音开关（放課後 有 7 人）。
	characterOn [7]bool
	// 音声設定页里当前按下的角色，用于高亮。
	character int

	font *textrender.Renderer
	// slots 是打开呼出帳时抓的一份存档列表，避免每帧 Compose 都去读盘。
	slots []core.Slot
}

var bdtUIFontOnce sync.Once
var bdtUIFont *textrender.Renderer

func sharedBDTFont() *textrender.Renderer {
	bdtUIFontOnce.Do(func() {
		font, err := textrender.New()
		if err != nil {
			core.Logf("bdt ui: text renderer unavailable: %v", err)
			return
		}
		bdtUIFont = font
	})
	return bdtUIFont
}

func (s *Session) openSystemUI(kind bdtSystemKind) {
	if s.sys == nil {
		s.sys = &bdtSystemUI{
			musicRow: -1, thumb: -1,
			musicVolume: s.gameState.BgmVolume(), effectVolume: s.sfxVolume, voiceVolume: s.voiceVolume,
			autoSpeed: 0.5, fullscreen: true, effectOn: true,
			font: sharedBDTFont(),
		}
		for index := range s.sys.characterOn {
			s.sys.characterOn[index] = true
		}
	}
	s.sys.kind = kind
	if kind == bdtSysCG || kind == bdtSysScene {
		s.sys.page = 0
		s.sys.thumb = -1
	}
	if kind == bdtSysLoad {
		s.sys.slots = s.Slots()
	}
	s.composerInvalidate()
}

// closeSystemUI 只把界面切到 none，保留结构体里的设置和页签状态。
// 关掉再打开設定时音量/角色开关/画面选项不会全部回到默认值。
func (s *Session) closeSystemUI() {
	if s.sys == nil {
		return
	}
	s.sys.kind = bdtSysNone
	s.composerInvalidate()
}

// bdtSystemActive 判断当前是否真的有一个系统界面盖在标题/剧情上。
// sys 指针在第一次打开后一直保留，所以不能再直接用 s.sys != nil 判断。
func (s *Session) bdtSystemActive() bool {
	return (s.sys != nil && s.sys.kind != bdtSysNone) || s.maninScreenActive()
}

func (s *Session) composerInvalidate() {
	if s.stage != nil {
		s.stage.revision++
	}
}

// ---------------------------------------------------------------- 绘制

func (s *Session) drawSystemUI(canvas *gfx.BgraCanvas) {
	ui := s.sys
	if ui == nil || ui.kind == bdtSysNone || canvas == nil {
		return
	}
	switch ui.kind {
	case bdtSysMusic:
		s.drawMusicUI(canvas, ui)
	case bdtSysCG:
		s.drawGalleryUI(canvas, ui, false)
	case bdtSysScene:
		s.drawGalleryUI(canvas, ui, true)
	case bdtSysConfigSound:
		s.drawConfigSoundUI(canvas, ui)
	case bdtSysConfigScreen:
		s.drawConfigScreenUI(canvas, ui)
	case bdtSysLoad:
		s.drawLoadUI(canvas, ui)
	}
}

func (s *Session) bdtImage(name string) *gfx.BsgImage {
	if s.images == nil || name == "" {
		return nil
	}
	return s.images.Get(name)
}

func (s *Session) drawBDTImage(canvas *gfx.BgraCanvas, name string) bool {
	image := s.bdtImage(name)
	if image == nil {
		return false
	}
	canvas.Draw(image, 0, 0)
	return true
}

func (s *Session) drawMusicUI(canvas *gfx.BgraCanvas, ui *bdtSystemUI) {
	canvas.Clear(0, 0, 0, 255)
	s.drawBDTImage(canvas, "s_mu01")
	for index := 0; index < 20; index++ {
		if index != ui.musicRow {
			continue
		}
		x, y := 31, 72+(index%10)*34
		if index >= 10 {
			x = 336
		}
		canvas.FillBlend(x, y, 272, 20, 20, 60, 120, 90)
	}
}

func (s *Session) drawGalleryUI(canvas *gfx.BgraCanvas, ui *bdtSystemUI, scene bool) {
	canvas.Clear(0, 0, 0, 255)
	prefix := "s_cg"
	pageCount := 6
	if scene {
		prefix = "s_sc"
		pageCount = 3
	}
	if ui.page < 0 {
		ui.page = 0
	}
	if ui.page >= pageCount {
		ui.page = pageCount - 1
	}
	name := prefix + twoDigits(ui.page) + "n"
	if !s.drawBDTImage(canvas, name) {
		// 某些页只有 b（空格子）没有 n；至少显示底图，别黑屏。
		s.drawBDTImage(canvas, prefix+twoDigits(ui.page)+"b")
	}
	if ui.thumb >= 0 {
		if x, y, ok := bdtGalleryThumbRect(scene, ui.thumb); ok {
			canvas.FillBlend(x-2, y-2, 84, 64, 255, 255, 255, 120)
			canvas.FillBlend(x-1, y-1, 82, 62, 0, 0, 0, 120)
		}
	}
}

func (s *Session) drawConfigSoundUI(canvas *gfx.BgraCanvas, ui *bdtSystemUI) {
	canvas.Clear(0, 0, 0, 255)
	s.drawBDTImage(canvas, "s_sys012")
	// 三条竖向音量条：顶部是最大，底部是静音。底图已经画了彩色电平条，
	// 这里只画当前位置的白色横线。
	volumes := []float64{ui.musicVolume, ui.effectVolume, ui.voiceVolume}
	columns := []int{69, 189, 311}
	for index, x := range columns {
		y := 127 + int((1-volumes[index])*254)
		canvas.Fill(x, y, 59, 2, 255, 255, 255, 230)
	}
	// 7 个角色开关：关闭时画一条红叉。
	for index := 0; index < 7; index++ {
		if ui.characterOn[index] {
			continue
		}
		x, y := 542, 128+37*index
		canvas.Fill(x, y+12, 40, 2, 40, 40, 255, 220)
	}
}

func (s *Session) drawConfigScreenUI(canvas *gfx.BgraCanvas, ui *bdtSystemUI) {
	canvas.Clear(0, 0, 0, 255)
	s.drawBDTImage(canvas, "s_sys013")
	mark := func(x, y, w, h int, on bool) {
		if on {
			canvas.FillBlend(x, y, w, h, 40, 40, 255, 90)
		}
	}
	mark(70, 130, 80, 30, ui.fullscreen)
	mark(70, 170, 80, 30, !ui.fullscreen)
	mark(70, 260, 80, 30, ui.effectOn)
	mark(70, 350, 80, 30, ui.skipUnread)
	mark(210, 146, 80, 30, !ui.fastText)
	mark(210, 204, 80, 30, ui.fastText)
	mark(350, 130, 80, 30, ui.measure == 0)
	mark(350, 170, 80, 30, ui.measure == 1)
	mark(350, 210, 80, 30, ui.measure == 2)
	mark(210, 310, 120, 30, !ui.skipFast)
	mark(330, 310, 120, 30, ui.skipFast)
	// オート速度：右侧竖条。
	y := 122 + int((1-ui.autoSpeed)*257)
	canvas.Fill(510, y, 39, 2, 255, 255, 255, 230)
}

// bdtDisplaySlots 把 20 行显示位置映射到引擎存档槽。
//
// 原作是 01..19 + AT：左列 01..10、右列 11..19、右下角 AT（自动存档）。
// 引擎的 BDT 存档是 24 个顺序槽，宿主存档面板也用同一套下标；这里按
// 0..19 顺序映射，保证前 20 个存档都能在呼出帳里看到（AT 行暂时复用
// 第 20 槽，BDT 没有独立的自动存档通道）。
func bdtDisplaySlots() [20]int {
	var slots [20]int
	for index := range slots {
		slots[index] = index
	}
	return slots
}

func (s *Session) drawLoadUI(canvas *gfx.BgraCanvas, ui *bdtSystemUI) {
	canvas.Clear(0, 0, 0, 255)
	s.drawBDTImage(canvas, "s_sys011")
	all := ui.slots
	if len(all) == 0 {
		all = s.Slots()
	}
	slots := bdtDisplaySlots()
	for row := 0; row < 20; row++ {
		slot := slots[row]
		x := 61
		if row >= 10 {
			x = 330
		}
		y := 101 + (row%10)*32
		if slot < 0 || slot >= len(all) || !all[slot].Exists || ui.font == nil {
			continue
		}
		title := strings.TrimSpace(all[slot].Summary)
		if title == "" {
			title = strings.TrimSpace(all[slot].Title)
		}
		if title == "" {
			title = "No Data"
		}
		image := ui.font.Render(title, 12, 0, 0, 15, 230, 1, 0xff202020)
		if image != nil {
			rendering.DrawTextImage(canvas, image, x+8, y+3)
		}
		if all[slot].SavedAt != "" && ui.font != nil {
			date := ui.font.Render(all[slot].SavedAt, 10, 0, 0, 12, 230, 1, 0xff606060)
			if date != nil {
				rendering.DrawTextImage(canvas, date, x+8, y+17)
			}
		}
	}
}

// ---------------------------------------------------------------- 命中/交互

func bdtGalleryThumbRect(scene bool, index int) (int, int, bool) {
	if index < 0 || index >= 24 {
		return 0, 0, false
	}
	row, col := index/6, index%6
	if scene {
		y := 70 + row*80
		x := 40 + col*90
		if row%2 == 1 {
			x += 31
		}
		return x, y, true
	}
	return 30 + col*100, 77 + row*80, true
}

func (s *Session) pointerSystemUI(x, y int) bool {
	ui := s.sys
	if ui == nil {
		return false
	}
	// 公共的三个顶部按钮：戻る / タイトル / 終了。
	// 只有設定/呼出帳用这一排；音楽鑑賞室自己的戻る更宽（521..630），
	// CG/シーン页顶部没有按钮，不能套用。
	if (ui.kind == bdtSysConfigSound || ui.kind == bdtSysConfigScreen || ui.kind == bdtSysLoad) &&
		x >= 351 && x < 630 && y >= 9 && y < 51 {
		switch {
		case x < 443:
			s.closeSystemUI()
		case x < 537:
			s.closeSystemUI()
			s.beginTitle()
		default:
			s.setHostRequest("exit")
			s.closeSystemUI()
		}
		return true
	}
	switch ui.kind {
	case bdtSysMusic:
		// 右下/右上的戻る按钮（s_mu01 的 521..630 / 10..49）。
		if x >= 521 && x < 630 && y >= 10 && y < 50 {
			s.closeSystemUI()
			return true
		}
		if y >= 72 && y < 72+10*34 {
			row := (y - 72) / 34
			if x >= 31 && x < 303 {
				s.playMuseumTrack(row)
				return true
			}
			if x >= 336 && x < 608 {
				s.playMuseumTrack(row + 10)
				return true
			}
		}
		return true
	case bdtSysCG, bdtSysScene:
		// 触摸端没有右键：点左上角"CG鑑賞室/シーン鑑賞室"标题回标题。
		if x >= 0 && x < 300 && y >= 0 && y < 60 {
			s.closeSystemUI()
			return true
		}
		scene := ui.kind == bdtSysScene
		pageCount := 6
		if scene {
			pageCount = 3
		}
		// 翻页箭头：491/531/571（s_cgNNh / s_scNNh）。
		if y >= 391 && y < 409 {
			switch {
			case x >= 491 && x < 528:
				ui.page = 0
				ui.thumb = -1
				return true
			case x >= 531 && x < 568:
				if ui.page > 0 {
					ui.page--
				}
				ui.thumb = -1
				return true
			case x >= 571 && x < 608:
				if ui.page+1 < pageCount {
					ui.page++
				}
				ui.thumb = -1
				return true
			}
		}
		for index := 0; index < 24; index++ {
			tx, ty, ok := bdtGalleryThumbRect(scene, index)
			if ok && x >= tx && x < tx+80 && y >= ty && y < ty+60 {
				ui.thumb = index
				return true
			}
		}
		return true
	case bdtSysConfigSound:
		// 标题文字/顶部圆形图标区域用于在音/画面设置之间切换（原引擎两个
		// 入口分别从游戏菜单的"音設定/画面設定"打开，标题只有"設定"）。
		if x >= 0 && x < 350 && y >= 0 && y < 56 {
			ui.kind = bdtSysConfigScreen
			return true
		}
		columns := []int{69, 189, 311}
		for index, cx := range columns {
			if x >= cx-10 && x < cx+70 && y >= 120 && y < 390 {
				value := 1 - float64(y-127)/254
				value = max(0, min(1, value))
				switch index {
				case 0:
					ui.musicVolume = value
					s.gameState.SetBgmVolume(value)
				case 1:
					ui.effectVolume = value
					s.sfxVolume = value
				case 2:
					ui.voiceVolume = value
					s.voiceVolume = value
				}
				return true
			}
		}
		for index := 0; index < 7; index++ {
			cx, cy := 542, 128+37*index
			if x >= cx && x < cx+40 && y >= cy && y < cy+27 {
				ui.characterOn[index] = !ui.characterOn[index]
				ui.character = index
				return true
			}
		}
		return true
	case bdtSysConfigScreen:
		if x >= 0 && x < 350 && y >= 0 && y < 56 {
			ui.kind = bdtSysConfigSound
			return true
		}
		type rect struct{ x, y, w, h int }
		hit := func(r rect) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }
		switch {
		case hit(rect{70, 130, 80, 30}):
			ui.fullscreen = true
		case hit(rect{70, 170, 80, 30}):
			ui.fullscreen = false
		case hit(rect{70, 260, 80, 30}):
			ui.effectOn = !ui.effectOn
		case hit(rect{70, 350, 80, 30}):
			ui.skipUnread = !ui.skipUnread
		case hit(rect{210, 146, 80, 30}):
			ui.fastText = false
		case hit(rect{210, 204, 80, 30}):
			ui.fastText = true
		case hit(rect{350, 130, 80, 30}):
			ui.measure = 0
		case hit(rect{350, 170, 80, 30}):
			ui.measure = 1
		case hit(rect{350, 210, 80, 30}):
			ui.measure = 2
		case hit(rect{210, 310, 120, 30}):
			ui.skipFast = false
		case hit(rect{330, 310, 120, 30}):
			ui.skipFast = true
		case x >= 500 && x < 560 && y >= 120 && y < 390:
			ui.autoSpeed = max(0, min(1, 1-float64(y-122)/257))
		}
		return true
	case bdtSysLoad:
		slots := bdtDisplaySlots()
		for row := 0; row < 20; row++ {
			x0 := 61
			if row >= 10 {
				x0 = 330
			}
			y0 := 101 + (row%10)*32
			if x >= x0 && x < x0+248 && y >= y0 && y < y0+31 {
				if s.loadFromSystemUI(slots[row]) {
					return true
				}
				return true
			}
		}
		return true
	}
	return true
}

func (s *Session) playMuseumTrack(index int) {
	if index < 0 || index >= 20 || s.sys == nil {
		return
	}
	s.sys.musicRow = index
	// s_mu01 的 20 行对应 bgm02..bgm21；bgm22 是事件短曲，不在原曲名表里。
	s.SetBgm(index+2, 0)
}

// loadFromSystemUI 读取一个存档槽；成功后关闭界面并恢复剧情。
func (s *Session) loadFromSystemUI(slot int) bool {
	slots := s.Slots()
	if slot < 0 || slot >= len(slots) || !slots[slot].Exists {
		return false
	}
	step, err := s.Load(uint32(slot))
	if err != nil {
		core.Logf("bdt ui: load slot %d failed: %v", slot, err)
		return false
	}
	s.sys.kind = bdtSysNone
	_ = step
	core.Logf("bdt ui: loaded slot %d", slot)
	return true
}

// twoDigits 是 BDT 素材名的两位编号（s_cg00n / s_mu01 …）。
func twoDigits(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}
