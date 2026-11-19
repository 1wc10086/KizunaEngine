package session

import (
	"math"
	"strings"

	"kizuna/enginego/brand/blackpackage/script"
	"kizuna/enginego/brand/blackpackage/stage"
	"kizuna/enginego/core"
)

// 本文件把原引擎“自己画在 800x600 帧里”的界面补回来：
//   - 对白信息框 + 工具按钮（ログ/文字消/オート/スキップ/セーブ/ロード/メニュー）
//   - 剧情选择肢
//   - 标题的画像閲覧/音楽モード/シーン回想/続きから/セーブ/環境設定
//
// 宿主只负责把帧贴到屏幕和把点击坐标送回来；所有文字、按钮、缩略图都由
// 引擎写进 stage.Frame，避免再弹 Flutter 的对话框/选择肢。

type uiButton struct {
	X, Y, W, H int32
	Label      string
	Action     string
	Arg        string
	Index      int
}

type uiScreen struct {
	Kind    string // gallery/music/scene/save/load/system/choice/view/config
	Title   string
	Page    int
	Items   []string
	Buttons []uiButton
	Choices []script.Choice
}

const (
	uiWidth  = frameWidth
	uiHeight = frameHeight
)

func (s *Session) clearUI() {
	s.ui = nil
}

// ---- 像素绘制 ----

func blendPixel(dst []byte, i int, r, g, b, a byte) {
	if i < 0 || i+3 >= len(dst) {
		return
	}
	ia := int(a)
	inv := 255 - ia
	dst[i] = byte((int(r)*ia + int(dst[i])*inv) / 255)
	dst[i+1] = byte((int(g)*ia + int(dst[i+1])*inv) / 255)
	dst[i+2] = byte((int(b)*ia + int(dst[i+2])*inv) / 255)
	dst[i+3] = 255
}

func (s *Session) fill(dst []byte, x, y, w, h int, color uint32) {
	a := byte(color >> 24)
	r := byte(color >> 16)
	g := byte(color >> 8)
	b := byte(color)
	for yy := y; yy < y+h; yy++ {
		if yy < 0 || yy >= uiHeight {
			continue
		}
		for xx := x; xx < x+w; xx++ {
			if xx < 0 || xx >= uiWidth {
				continue
			}
			blendPixel(dst, (yy*uiWidth+xx)*4, r, g, b, a)
		}
	}
}

func (s *Session) stroke(dst []byte, x, y, w, h, t int, color uint32) {
	s.fill(dst, x, y, w, t, color)
	s.fill(dst, x, y+h-t, w, t, color)
	s.fill(dst, x, y, t, h, color)
	s.fill(dst, x+w-t, y, t, h, color)
}

// drawText 把一行文字作为一条 core.TextRun 追加到本次合成。
//
// 引擎不再把字形栅格进 stage.Frame：Flutter 用平台字体绘制。y 是行盒
// 顶边（旧逐字绘制的入参），这里换算成宿主需要的基线，整行才不会上移。
// Shadow=1 复刻旧实现那层 1px 黑色阴影（浅色背景上的可读性靠它）。
func (s *Session) drawText(x, y int, text string, size int, color uint32) int {
	if text == "" {
		return 0
	}
	s.textRuns = append(s.textRuns, core.TextRun{
		Text:   text,
		X:      int32(x),
		Y:      int32(y + textBaseline(size)),
		Size:   int32(size),
		Color:  color,
		Shadow: 1,
	})
	return textPixelWidth(text, size)
}

// textBaseline 返回行盒顶边到基线的距离，与 core/textrender 的 0.86em 一致。
func textBaseline(size int) int {
	if size <= 0 {
		return 0
	}
	return int(math.Round(float64(size) * 0.86))
}

func textPixelWidth(text string, size int) int {
	w := 0
	for _, ch := range text {
		if ch == '\u3000' {
			w += size
		} else if ch < 0x80 {
			w += size / 2
		} else {
			w += size
		}
	}
	return w
}

// wrapText 按像素宽度折行；日文没有空格断点，超出硬切。
func wrapText(text string, size, maxWidth int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		var line []rune
		width := 0
		for _, ch := range para {
			cw := size
			if ch < 0x80 {
				cw = size / 2
			}
			if width+cw > maxWidth && len(line) > 0 {
				out = append(out, string(line))
				line = line[:0]
				width = 0
			}
			line = append(line, ch)
			width += cw
		}
		if len(line) > 0 {
			out = append(out, string(line))
		}
	}
	return out
}

// ---- 信息框 ----

var messageToolbar = []string{"ログ", "文字消", "オート", "スキップ", "セーブ", "ロード", "メニュー"}

func (s *Session) messageButtonRects() []uiButton {
	out := make([]uiButton, 0, len(messageToolbar))
	for i, label := range messageToolbar {
		out = append(out, uiButton{
			X: int32(22 + i*108), Y: 552, W: 100, H: 26,
			Label: label, Action: "msg:" + label,
		})
	}
	return out
}

// drawMessageWindow 把原版对白窗画进帧。位置取屏幕下沿：正文 3~4 行、
// 底部一排工具按钮，和 800x600 原版的布局一致。
func (s *Session) drawMessageWindow(dst []byte) {
	// 标题、全屏 UI、CG 大图预览都不该出现对白框；以前漏了 titleOverlay
	// 的 "view"，画廊里打开大图时对白框会盖在 CG 上。
	if s.atTitle || s.ui != nil || s.titleOverlay == "view" {
		return
	}
	x, y, w, h := 12, 424, 776, 164
	s.fill(dst, x, y, w, h, 0xD00A1018)
	s.stroke(dst, x, y, w, h, 2, 0xFF8FA6BC)
	s.fill(dst, x+3, y+3, w-6, 2, 0x60406080)

	if s.speaker != "" {
		s.drawText(x+16, y+8, s.speaker, 16, 0xFFFFD070)
	}
	lines := wrapText(s.text, 16, w-36)
	if len(lines) > 4 {
		lines = lines[:4]
	}
	for i, line := range lines {
		s.drawText(x+16, y+32+i*24, line, 16, 0xFFF4F4F4)
	}
	for _, b := range s.messageButtonRects() {
		s.drawButton(dst, b)
	}
}

func (s *Session) drawButton(dst []byte, b uiButton) {
	s.fill(dst, int(b.X), int(b.Y), int(b.W), int(b.H), 0xB0182838)
	s.stroke(dst, int(b.X), int(b.Y), int(b.W), int(b.H), 1, 0xFF95A8BA)
	tw := textPixelWidth(b.Label, 14)
	tx := int(b.X) + (int(b.W)-tw)/2
	ty := int(b.Y) + (int(b.H)-14)/2
	s.drawText(tx, ty, b.Label, 14, 0xFFF0F0F0)
}

// ---- UI 画面 ----

func (s *Session) drawUI(dst []byte) {
	ui := s.ui
	if ui == nil {
		return
	}
	bg := uint32(0xB8000000)
	if ui.Kind != "choice" {
		// 标题子界面/存读档/设置是独立画面；原来的标题图不应该透出来。
		bg = 0xFF080C12
	}
	s.fill(dst, 0, 0, uiWidth, uiHeight, bg)
	title := ui.Title
	if title != "" {
		tw := textPixelWidth(title, 20)
		s.drawText((uiWidth-tw)/2, 24, title, 20, 0xFFFFE08A)
		s.fill(dst, 100, 52, 600, 1, 0x80FFFFFF)
	}
	for _, b := range ui.Buttons {
		if ui.Kind == "gallery" && b.Action == "gallery_open" && b.Arg != "" {
			s.drawThumb(dst, b)
			continue
		}
		s.drawButton(dst, b)
	}
	if ui.Kind == "view" {
		s.drawText(16, uiHeight-28, "クリックで戻る", 14, 0xFFFFFFFF)
	}
}

func (s *Session) drawThumb(dst []byte, b uiButton) {
	img := s.stage.LookupImage(b.Arg)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		s.fill(dst, int(b.X), int(b.Y), int(b.W), int(b.H), 0x80303030)
		return
	}
	// 最近邻缩放到 160x120，保持原图比例（居中留黑边）。
	dw, dh := int(b.W), int(b.H)
	scale := float64(dw) / float64(img.Width)
	if sh := float64(dh) / float64(img.Height); sh < scale {
		scale = sh
	}
	nw := int(float64(img.Width) * scale)
	nh := int(float64(img.Height) * scale)
	ox := int(b.X) + (dw-nw)/2
	oy := int(b.Y) + (dh-nh)/2
	s.fill(dst, int(b.X), int(b.Y), dw, dh, 0xFF000000)
	for yy := 0; yy < nh; yy++ {
		sy := yy * img.Height / nh
		for xx := 0; xx < nw; xx++ {
			sx := xx * img.Width / nw
			si := (sy*img.Width + sx) * 4
			di := ((oy+yy)*uiWidth + ox + xx) * 4
			if di < 0 || di+3 >= len(dst) {
				continue
			}
			dst[di] = img.Pixels[si]
			dst[di+1] = img.Pixels[si+1]
			dst[di+2] = img.Pixels[si+2]
			dst[di+3] = 255
		}
	}
	s.stroke(dst, int(b.X), int(b.Y), dw, dh, 1, 0xFF8090A0)
	if name := b.Label; name != "" {
		s.drawText(int(b.X), int(b.Y)+dh+2, name, 12, 0xFFE0E0E0)
	}
}

// uiClick 处理引擎自绘界面的点击。返回 true 表示已消费。
func (s *Session) uiClick(x, y int32) bool {
	ui := s.ui
	if ui == nil {
		return false
	}
	for _, b := range ui.Buttons {
		if x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
			s.uiAction(ui, b)
			return true
		}
	}
	return true // 界面打开时，点空白也不要穿透推进剧情
}

func (s *Session) uiAction(ui *uiScreen, b uiButton) {
	switch b.Action {
	case "back":
		s.clearUI()
		if s.atTitle {
			s.step = core.StepAwaitTitle
		} else {
			s.step = core.StepAwaitInput
		}
	case "page_next":
		ui.Page++
		s.rebuildUIPage(ui)
	case "page_prev":
		if ui.Page > 0 {
			ui.Page--
		}
		s.rebuildUIPage(ui)
	case "gallery_open":
		name := b.Arg
		s.clearUI()
		s.titleOverlay = "view"
		s.step = core.StepAwaitInput
		// 大图预览必须画进屏幕层：Compose 只合成原版的屏幕层，画到别处
		// （以前用 0/250 号离屏层）会整帧全黑。先填黑再载图，避免 CG 的
		// 透明边缘透出上一屏内容。
		if !s.stage.HasLayer(stage.ScreenLayer) {
			s.stage.CreateLayer(stage.ScreenLayer, []int32{0, 0, 0, frameWidth, frameHeight, 0, 0})
		}
		s.stage.Fill(stage.ScreenLayer, 0, 0, frameWidth, frameHeight, 0)
		s.stage.LoadImage(stage.ScreenLayer, name, nil)
	case "view_back":
		s.clearUI()
		s.startTitle()
		s.titleOverlay = "gallery"
		s.openGallery()
	case "music_play":
		if path := s.materialize(b.Arg); path != "" {
			s.pending = append(s.pending, core.AudioEvent{
				Kind: 0, Path: path, Volume: s.bgmVolume, Loop: true,
				Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF,
			})
			s.bgm = b.Arg
		}
	case "scene_start":
		s.clearUI()
		s.titleOverlay = ""
		s.atTitle = false
		if err := s.vm.Start(b.Arg); err == nil {
			s.step = core.StepAwaitInput
		}
	case "choice":
		if b.Index >= 0 && b.Index < len(ui.Choices) {
			c := ui.Choices[b.Index]
			s.clearUI()
			s.choices = nil
			s.vm.ChooseValue(c.Value)
			s.RunNow()
		}
	case "save":
		// Index 是槽位号（1 起），与 save.Store 的 numbering 一致。
		if b.Index > 0 {
			if err := s.Save(uint32(b.Index), ""); err != nil {
				s.lastErr = err.Error()
			}
			s.openSaveScreen()
		}
	case "load":
		if b.Index > 0 {
			if _, err := s.Load(uint32(b.Index)); err != nil {
				// 读档失败必须让玩家看到原因（空槽/损坏），否则表现就是“点了没反应”。
				// 用信息框直接告知玩家，避免“点了没反应”的观感。
				s.lastErr = err.Error()
				s.clearUI()
				s.titleOverlay = ""
				s.speaker = ""
				s.text = "このデータは読み込めません"
				s.step = core.StepAwaitInput
				return
			}
			s.clearUI()
			s.atTitle = false
		}
	case "msg:ログ":
		s.clearUI()
		s.openBacklogUI()
	case "msg:文字消":
		s.text = ""
		s.speaker = ""
	case "msg:オート":
		s.hostRequest = "auto"
		s.hostSerial++
	case "msg:スキップ":
		s.hostRequest = "skip"
		s.hostSerial++
	case "msg:セーブ":
		s.openSaveScreen()
	case "msg:ロード":
		s.openLoadScreen()
	case "msg:メニュー":
		s.openSystemMenu()
	case "sys:save":
		s.openSaveScreen()
	case "sys:load":
		s.openLoadScreen()
	case "sys:backlog":
		s.clearUI()
		s.openBacklogUI()
	case "sys:config":
		s.openConfigScreen()
	case "sys:title":
		s.clearUI()
		s.startTitle()
	case "sys:close":
		s.clearUI()
		s.step = core.StepAwaitInput
	case "cfg:bgm":
		s.bgmVolume = cycleVolume(s.bgmVolume)
		s.saveSettings()
		s.openConfigScreen()
	case "cfg:se":
		s.seVolume = cycleVolume(s.seVolume)
		s.pending = append(s.pending, core.AudioEvent{Kind: 7, Volume: s.seVolume, Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF})
		s.saveSettings()
		s.openConfigScreen()
	case "cfg:voice":
		s.voiceVolume = cycleVolume(s.voiceVolume)
		s.saveSettings()
		s.openConfigScreen()
	case "cfg:auto":
		s.autoSpeed = (s.autoSpeed + 10) % 110
		s.saveSettings()
		s.openConfigScreen()
	}
}

func cycleVolume(v float64) float64 {
	switch {
	case v < 0.25:
		return 0.5
	case v < 0.5:
		return 0.75
	case v < 0.75:
		return 1.0
	default:
		return 0.0
	}
}

func (s *Session) rebuildUIPage(ui *uiScreen) {
	switch ui.Kind {
	case "gallery":
		s.galleryPage(ui.Page)
	case "save":
		s.openSaveScreen()
	case "load":
		s.openLoadScreen()
	case "music":
		s.openMusicMode()
	case "scene":
		s.openSceneRecall()
	}
}

// ---- 标题子界面 ----

func (s *Session) openGallery() {
	seen := make([]string, 0, len(s.seenImages))
	for name := range s.seenImages {
		seen = append(seen, name)
	}
	if len(seen) == 0 && s.set != nil {
		for _, n := range s.set.Names() {
			low := strings.ToLower(n)
			if !strings.HasSuffix(low, ".pt1") || strings.HasSuffix(low, "m.pt1") {
				continue
			}
			if strings.HasPrefix(low, "x0a") || strings.HasPrefix(low, "x0b") ||
				strings.HasPrefix(low, "x0c") || strings.HasPrefix(low, "x0d") ||
				strings.HasPrefix(low, "x0f") || strings.HasPrefix(low, "x0g") {
				seen = append(seen, n)
			}
		}
	}
	sortStrings(seen)
	s.ui = &uiScreen{Kind: "gallery", Title: "画像閲覧", Items: seen}
	s.titleOverlay = ""
	s.galleryPage(0)
	if s.atTitle {
		s.step = core.StepAwaitTitle
	} else {
		s.step = core.StepAwaitInput
	}
}

func (s *Session) galleryPage(page int) {
	ui := s.ui
	if ui == nil {
		return
	}
	total := len(ui.Items)
	const per = 8
	if page*per >= total {
		page = 0
	}
	ui.Page = page
	ui.Buttons = nil
	start := page * per
	end := start + per
	if end > total {
		end = total
	}
	for i := start; i < end; i++ {
		n := i - start
		col, row := n%4, n/4
		ui.Buttons = append(ui.Buttons, uiButton{
			X: int32(40 + col*190), Y: int32(96 + row*170), W: 160, H: 120,
			Label: ui.Items[i], Action: "gallery_open", Arg: ui.Items[i], Index: i,
		})
	}
	s.uiFooters(ui, page, total, per)
}

// uiFooters 给列表类界面补上翻页/返回按钮。
func (s *Session) uiFooters(ui *uiScreen, page, total, per int) {
	y := int32(uiHeight - 48)
	x := int32(80)
	if page > 0 {
		ui.Buttons = append(ui.Buttons, uiButton{X: x, Y: y, W: 140, H: 34, Label: "前のページ", Action: "page_prev"})
		x += 150
	}
	if (page+1)*per < total {
		ui.Buttons = append(ui.Buttons, uiButton{X: x, Y: y, W: 140, H: 34, Label: "次のページ", Action: "page_next"})
		x += 150
	}
	ui.Buttons = append(ui.Buttons, uiButton{X: x, Y: y, W: 140, H: 34, Label: "戻る", Action: "back"})
}

func (s *Session) openMusicMode() {
	seen := make([]string, 0, len(s.seenBgm))
	for name := range s.seenBgm {
		seen = append(seen, name)
	}
	if len(seen) == 0 && s.set != nil {
		for _, n := range s.set.Names() {
			low := strings.ToLower(n)
			if strings.HasSuffix(low, ".wa1") && strings.HasPrefix(low, "z0") {
				seen = append(seen, n)
			}
		}
	}
	sortStrings(seen)
	s.ui = &uiScreen{Kind: "music", Title: "音楽モード", Items: seen}
	s.musicPage(0)
	s.titleOverlay = ""
	s.step = core.StepAwaitTitle
	if !s.atTitle {
		s.step = core.StepAwaitInput
	}
}

func (s *Session) musicPage(page int) {
	ui := s.ui
	if ui == nil {
		return
	}
	total := len(ui.Items)
	const per = 8
	if page*per >= total {
		page = 0
	}
	ui.Page = page
	ui.Buttons = nil
	start := page * per
	end := start + per
	if end > total {
		end = total
	}
	for i := start; i < end; i++ {
		n := i - start
		ui.Buttons = append(ui.Buttons, uiButton{
			X: 120, Y: int32(96 + n*46), W: 560, H: 38,
			Label: ui.Items[i], Action: "music_play", Arg: ui.Items[i], Index: i,
		})
	}
	s.uiFooters(ui, page, total, per)
}

func (s *Session) openSceneRecall() {
	seen := make([]string, 0, len(s.seenScenes))
	for name := range s.seenScenes {
		low := strings.ToLower(name)
		if !strings.HasSuffix(low, ".so4") {
			continue
		}
		switch low {
		case "x0_start.so4", "a0_staff.so4", "a0_menu.so4", "a0_load.so4",
			"init.so4", "saveload.so4", "savesave.so4", "setup.so4", "setup2.so4", "uninst.so4":
			continue
		}
		seen = append(seen, name)
	}
	if len(seen) == 0 {
		seen = []string{"A0_000.SO4", "A0_001.SO4", "A0_002.SO4", "A0_003.SO4"}
	}
	sortStrings(seen)
	s.ui = &uiScreen{Kind: "scene", Title: "シーン回想", Items: seen}
	s.scenePage(0)
	s.titleOverlay = ""
	s.step = core.StepAwaitTitle
	if !s.atTitle {
		s.step = core.StepAwaitInput
	}
}

func (s *Session) scenePage(page int) {
	ui := s.ui
	if ui == nil {
		return
	}
	total := len(ui.Items)
	const per = 8
	if page*per >= total {
		page = 0
	}
	ui.Page = page
	ui.Buttons = nil
	start := page * per
	end := start + per
	if end > total {
		end = total
	}
	for i := start; i < end; i++ {
		n := i - start
		ui.Buttons = append(ui.Buttons, uiButton{
			X: 120, Y: int32(96 + n*46), W: 560, H: 38,
			Label: ui.Items[i], Action: "scene_start", Arg: ui.Items[i], Index: i,
		})
	}
	s.uiFooters(ui, page, total, per)
}

func (s *Session) openSaveScreen() {
	if s.store == nil {
		return
	}
	slots := s.store.List(saveSlots)
	s.ui = &uiScreen{Kind: "save", Title: "セーブ", Items: make([]string, len(slots))}
	for i, sl := range slots {
		label := "No." + pad2(i+1) + "  （空き）"
		if sl.Exists {
			label = "No." + pad2(i+1) + "  " + sl.Title
		}
		s.ui.Items[i] = label
	}
	s.savePage(0)
	s.step = core.StepAwaitInput
}

func (s *Session) savePage(page int) {
	ui := s.ui
	if ui == nil {
		return
	}
	const per = 8
	total := len(ui.Items)
	if page*per >= total {
		page = 0
	}
	ui.Page = page
	ui.Buttons = nil
	start := page * per
	end := start + per
	if end > total {
		end = total
	}
	for i := start; i < end; i++ {
		n := i - start
		ui.Buttons = append(ui.Buttons, uiButton{
			X: 80, Y: int32(90 + n*46), W: 640, H: 38,
			Label: ui.Items[i], Action: "save", Index: i + 1,
		})
	}
	s.uiFooters(ui, page, total, per)
}

func (s *Session) openLoadScreen() {
	if s.store == nil {
		return
	}
	slots := s.store.List(saveSlots)
	s.ui = &uiScreen{Kind: "load", Title: "ロード", Items: make([]string, len(slots))}
	for i, sl := range slots {
		label := "No." + pad2(i+1) + "  （空き）"
		if sl.Exists {
			label = "No." + pad2(i+1) + "  " + sl.Title
		}
		s.ui.Items[i] = label
	}
	s.loadPage(0)
	s.step = core.StepAwaitInput
}

func (s *Session) loadPage(page int) {
	ui := s.ui
	if ui == nil {
		return
	}
	const per = 8
	total := len(ui.Items)
	if page*per >= total {
		page = 0
	}
	ui.Page = page
	ui.Buttons = nil
	start := page * per
	end := start + per
	if end > total {
		end = total
	}
	for i := start; i < end; i++ {
		n := i - start
		ui.Buttons = append(ui.Buttons, uiButton{
			X: 80, Y: int32(90 + n*46), W: 640, H: 38,
			Label: ui.Items[i], Action: "load", Index: i + 1,
		})
	}
	s.uiFooters(ui, page, total, per)
}

func (s *Session) openSystemMenu() {
	s.ui = &uiScreen{Kind: "system", Title: "メニュー"}
	labels := []struct{ label, action string }{
		{"セーブ", "sys:save"}, {"ロード", "sys:load"}, {"回想", "sys:backlog"},
		{"環境設定", "sys:config"}, {"タイトルへ", "sys:title"}, {"閉じる", "sys:close"},
	}
	for i, l := range labels {
		s.ui.Buttons = append(s.ui.Buttons, uiButton{
			X: 220, Y: int32(110 + i*52), W: 360, H: 42, Label: l.label, Action: l.action,
		})
	}
	s.step = core.StepAwaitInput
}

func (s *Session) openConfigScreen() {
	s.ui = &uiScreen{Kind: "config", Title: "環境設定"}
	rows := []struct {
		label, action string
		value         string
	}{
		{"BGM 音量", "cfg:bgm", pct(s.bgmVolume)},
		{"効果音 音量", "cfg:se", pct(s.seVolume)},
		{"音声 音量", "cfg:voice", pct(s.voiceVolume)},
		{"オート速度", "cfg:auto", itoa(s.autoSpeed)},
	}
	for i, r := range rows {
		s.ui.Buttons = append(s.ui.Buttons, uiButton{
			X: 220, Y: int32(110 + i*52), W: 360, H: 42,
			Label: r.label + "   " + r.value, Action: r.action,
		})
	}
	s.ui.Buttons = append(s.ui.Buttons, uiButton{
		X: 220, Y: int32(110 + len(rows)*52), W: 360, H: 42, Label: "戻る", Action: "back",
	})
	s.step = core.StepAwaitInput
}

func (s *Session) buildChoiceUI() {
	ch := s.vm.Choices
	ui := &uiScreen{Kind: "choice", Title: "選択", Choices: append([]script.Choice(nil), ch...)}
	for i, c := range ch {
		ui.Buttons = append(ui.Buttons, uiButton{
			X: 100, Y: int32(160 + i*58), W: 600, H: 46,
			Label: c.Text, Action: "choice", Index: i,
		})
	}
	s.ui = ui
	s.step = core.StepAwaitInput
}

// RunNow 推进一次 VM 并把结果 step 写回（选择肢/回想点击后使用）。
func (s *Session) RunNow() {
	if s.vm == nil {
		return
	}
	if st, err := s.Run(commandBudget); err == nil {
		s.step = st
	}
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func pct(v float64) string {
	return itoa(int(v*100)) + "%"
}

// openBacklogUI 打开「回想」（バックログ）。原版是引擎绘制的滚动文本窗，
// 这里用分页 + 引擎文字绘制，避免再交给宿主界面。
func (s *Session) openBacklogUI() {
	items := make([]string, 0, 200)
	start := 0
	if len(s.backlog) > 200 {
		start = len(s.backlog) - 200
	}
	for _, u := range s.backlog[start:] {
		t := strings.TrimSpace(u.Text)
		r := []rune(t)
		if len(r) > 54 {
			t = string(r[:54]) + "…"
		}
		if t != "" {
			items = append(items, t)
		}
	}
	s.ui = &uiScreen{Kind: "backlog", Title: "バックログ", Items: items}
	s.backlogPage(len(items) / 6)
	s.step = core.StepAwaitInput
}

func (s *Session) backlogPage(page int) {
	ui := s.ui
	if ui == nil {
		return
	}
	const per = 6
	total := len(ui.Items)
	if page*per >= total {
		page = 0
	}
	ui.Page = page
	ui.Buttons = nil
	start := page * per
	end := start + per
	if end > total {
		end = total
	}
	for i := start; i < end; i++ {
		n := i - start
		ui.Buttons = append(ui.Buttons, uiButton{
			X: 40, Y: int32(80 + n*66), W: 720, H: 58,
			Label: ui.Items[i], Action: "backlog_row", Index: i,
		})
	}
	s.uiFooters(ui, page, total, per)
}
