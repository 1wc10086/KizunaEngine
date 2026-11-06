package gothic

// gothic_ui_draw.go 负责把 gothic_ui.go 的状态画到 stage.Frame 上。
//
// 所有界面都使用游戏自己的 640x480 坐标系和 GOTHIC2.DAT 字体；
// Flutter 侧只叠透明的点击热区，因此这里的每个矩形必须与
// gothic_ui.go 的 uiItems 保持一致（同一份常量思路，改坐标时两边一起改）。

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
)

// drawChoiceHighlight 在舞台合成后给悬停的选择肢画一圈边框。
// 只画边框而不是半透明底：选项文字由舞台的字体合成，后画的填充会盖住字。
func (s *Session) drawChoiceHighlight() {
	if !s.choiceActive || s.ui != nil {
		return
	}
	index := s.gameState.HoveredButton()
	if index < 0 || index >= len(s.choiceItems) {
		return
	}
	y := 120 + index*25
	s.drawUIBorder(s.stage.Frame, 74, y, GothicScreenWidth-148, 25, true)
}

// drawMenuButton 画右上角的汉堡按钮。它属于游戏 HUD，不是系统界面：
// 只在剧情里出现，菜单打开后由 ui 覆盖层接管。
func (s *Session) drawMenuButton() {
	canvas := s.stage.Frame
	canvas.FillBlend(592, 6, 42, 28, 0, 0, 0, 150)
	canvas.Fill(600, 12, 26, 2, 235, 235, 235, 255)
	canvas.Fill(600, 18, 26, 2, 235, 235, 235, 255)
	canvas.Fill(600, 24, 26, 2, 235, 235, 235, 255)
}

func (s *Session) drawGothicUI() bool {
	if s.ui == nil || s.stage == nil {
		return false
	}
	canvas := s.stage.Frame
	switch s.ui.mode {
	case uiTitle:
		s.drawTitleHover(canvas)
	case uiOption:
		s.drawOption(canvas)
	case uiJuice:
		s.drawJuice(canvas)
	case uiGalleryMenu:
		s.drawGalleryMenu(canvas)
	case uiAlbum:
		s.drawAlbum(canvas)
	case uiKaisou:
		s.drawKaisou(canvas)
	case uiMusic:
		s.drawMusic(canvas)
	case uiPreview:
		s.drawPreview(canvas)
	case uiSlots:
		s.drawSlots(canvas)
	case uiIngame:
		s.drawIngame(canvas)
	case uiStaffRoll:
		s.drawStaffRoll(canvas)
	default:
		return false
	}
	return true
}

// drawUIFullImage 用素材铺满画面。GSA 的 OffsetX/Y 是原图在 640x480
// 画布上的位置（gameover 这类小图带偏移），不能一律画到 (0,0)。
func (s *Session) drawUIFullImage(canvas *gfx.BgraCanvas, name string) bool {
	if canvas == nil || s.catalog == nil {
		return false
	}
	image := s.catalog.Resolve(name)
	if image == nil {
		return false
	}
	canvas.Clear(0, 0, 0, 255)
	canvas.Draw(image, image.OffsetX, image.OffsetY)
	return true
}

func (s *Session) drawUIPanel(canvas *gfx.BgraCanvas, x, y, w, h int) {
	canvas.FillBlend(x, y, w, h, 0, 0, 0, 205)
	s.drawUIBorder(canvas, x, y, w, h, false)
}

func (s *Session) drawUIBorder(canvas *gfx.BgraCanvas, x, y, w, h int, active bool) {
	blue, green, red := byte(70), byte(120), byte(150)
	if active {
		blue, green, red = 230, 220, 90
	}
	canvas.Fill(x, y, w, 2, blue, green, red, 255)
	canvas.Fill(x, y+h-2, w, 2, blue, green, red, 255)
	canvas.Fill(x, y, 2, h, blue, green, red, 255)
	canvas.Fill(x+w-2, y, 2, h, blue, green, red, 255)
}

func (s *Session) drawUIText(canvas *gfx.BgraCanvas, text string, x, y, maxChars int) {
	if canvas == nil || s.font == nil {
		return
	}
	line := toFullWidth(text)
	if maxChars > 0 && len([]rune(line)) > maxChars {
		line = string([]rune(line)[:maxChars])
	}
	DrawGothicText(canvas, s.font, line, x, y, 0)
}

func (s *Session) drawUITextWidth(canvas *gfx.BgraCanvas, text string, x, y, maxWidth int) {
	if maxWidth <= 0 {
		return
	}
	advance := s.font.Advance
	if advance <= 0 {
		advance = 1
	}
	s.drawUIText(canvas, text, x, y, maxWidth/advance)
}

func (s *Session) drawUITextCentered(canvas *gfx.BgraCanvas, text string, y int) {
	line := toFullWidth(text)
	advance := s.font.Advance
	x := (GothicScreenWidth - advance*len([]rune(line))) / 2
	if x < 0 {
		x = 0
	}
	DrawGothicText(canvas, s.font, line, x, y, 0)
}

// toFullWidth 把 ASCII 可见字符映射成全角字符。
//
// GOTHIC2.DAT 只有 JIS X 0208 字形，没有半角 ASCII；原版用 GDI
// 的 ＭＳ ゴシック 补半角，我们的字体表里只有全角区。界面里的
// 数字/日期/槽号因此先转全角再查表，避免“槽 1”只画出“槽”。
func toFullWidth(text string) string {
	var builder strings.Builder
	for _, r := range text {
		switch {
		case r == ' ':
			builder.WriteRune('\u3000')
		case r >= '!' && r <= '~':
			// ASCII -> 全角 ASCII 的正确偏移是 0xFEE0（'!'=0x21 -> U+FF01）。
			// 早期写成 -0x20+0xFF01 会把 '1' 变成 '２'、'E' 变成 'Ｆ'。
			builder.WriteRune(r + 0xFEE0)
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (s *Session) drawTitleHover(canvas *gfx.BgraCanvas) {
	if canvas == nil || s.ui.hover < 0 {
		return
	}
	rects := s.titleRects()
	if s.ui.hover >= len(rects) {
		return
	}
	rect := rects[s.ui.hover]
	hover := s.catalog.Resolve("title02")
	if hover == nil {
		return
	}
	canvas.BlitRegion(hover, rect[0], rect[1], rect[2], rect[3], rect[0], rect[1], 255)
}

func titleItemRects() [][4]int {
	return [][4]int{
		{456, 148, 160, 34},
		{456, 205, 160, 34},
		{456, 262, 160, 34},
		{456, 319, 160, 34},
		{456, 376, 160, 34},
	}
}

func (s *Session) drawOption(canvas *gfx.BgraCanvas) {
	if s.profile == gothicProfileToku1 {
		s.drawToku1Option(canvas)
		return
	}
	s.drawUIFullImage(canvas, "option0.gsa")
	if s.global == nil {
		return
	}
	// 姓名输入框：底图是两块纯黑，直接把当前姓名写进去。
	canvas.Fill(300, 86, 100, 26, 0, 0, 0, 255)
	canvas.Fill(455, 86, 130, 26, 0, 0, 0, 255)
	s.drawUITextWidth(canvas, s.global.PlayerLast, 304, 86, 96)
	s.drawUITextWidth(canvas, s.global.PlayerFirst, 459, 86, 126)
	// 当前值用青色小方块标出来；底图没有选中态帧。
	mark := func(x, y int) { canvas.Fill(x, y, 10, 10, 230, 220, 90, 255) }
	if s.global.Effects {
		mark(250, 180)
	} else {
		mark(435, 180)
	}
	switch s.global.TextSpeed {
	case 0:
		mark(250, 228)
	case 1:
		mark(380, 228)
	case 2:
		mark(465, 228)
	default:
		mark(550, 228)
	}
	switch s.global.Background {
	case 0:
		mark(250, 276)
	case 1:
		mark(335, 276)
	case 2:
		mark(400, 276)
	default:
		mark(465, 276)
	}
	if s.global.BgmOn {
		mark(250, 326)
	} else {
		mark(435, 326)
	}
	if s.global.SeOn {
		mark(250, 368)
	} else {
		mark(435, 368)
	}
	if s.ui.hover >= 0 {
		hits := s.optionItems()
		if s.ui.hover < len(hits) {
			h := hits[s.ui.hover]
			s.drawUIBorder(canvas, h.x, h.y, h.w, h.h, true)
		}
	}
}

func (s *Session) drawJuice(canvas *gfx.BgraCanvas) {
	s.drawUIFullImage(canvas, "mode0.gsa")
	if s.global == nil {
		return
	}
	xs := []int{222, 370, 518}
	ys := []int{163, 245, 327}
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			if s.global.Siru[row] != col {
				continue
			}
			x := xs[col] - 26
			y := ys[row] - 26
			s.drawUIBorder(canvas, x, y, 52, 52, true)
		}
	}
	if s.ui.hover >= 0 {
		hits := s.juiceItems()
		if s.ui.hover < len(hits) {
			h := hits[s.ui.hover]
			s.drawUIBorder(canvas, h.x, h.y, h.w, h.h, true)
		}
	}
}

func (s *Session) drawGalleryMenu(canvas *gfx.BgraCanvas) {
	s.drawUIFullImage(canvas, "title03.gsa")
	s.drawUIPanel(canvas, 160, 140, 320, 260)
	s.drawUITextCentered(canvas, "鑑賞室", 155)
	if s.ui.hover >= 0 {
		hits := s.uiItems()
		if s.ui.hover < len(hits) {
			h := hits[s.ui.hover]
			s.drawUIBorder(canvas, h.x, h.y, h.w, h.h, true)
		}
	}
	for _, hit := range s.uiItems() {
		s.drawUITextCentered(canvas, hit.label, hit.y+8)
	}
}

func (s *Session) drawAlbum(canvas *gfx.BgraCanvas) {
	if s.profile == gothicProfileToku1 {
		s.drawToku1Album(canvas)
		return
	}
	s.drawUIFullImage(canvas, "album.gsa")
	thumb := s.catalog.Resolve("sumbnail")
	start := s.ui.page * 12
	for i := 0; i < 12; i++ {
		position := start + i
		rect := albumCellRect(i)
		if position >= len(s.cgNames) {
			continue
		}
		name := s.cgNames[position]
		unlocked := s.global != nil && s.global.CG[name]
		if unlocked && thumb != nil && position < gothicThumbnailAtlasCount {
			column := position % 8
			row := position / 8
			// sumbnail 是 8 列 x 11 行、每格 113x113 的方形图册，
			// 只收录前 88 个事件 CG；其余条目从原图即时缩放。
			drawW, drawH := 112, 84
			x := rect[0] + (rect[2]-drawW)/2
			y := rect[1] + 7
			canvas.BlitScaledRegion(thumb, x, y, drawW, drawH, 255, column*113, row*113, 113, 113)
		} else if unlocked {
			image := s.galleryImage(name)
			if image != nil {
				canvas.BlitScaled(image, rect[0]+7, rect[1]+7, rect[2]-14, rect[3]-14, 255)
			} else {
				canvas.Fill(rect[0]+4, rect[1]+4, rect[2]-8, rect[3]-8, 0, 0, 0, 255)
			}
		} else {
			canvas.Fill(rect[0]+4, rect[1]+4, rect[2]-8, rect[3]-8, 0, 0, 0, 255)
		}
		if s.ui.hover == i+3 {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
	}
	s.drawUIText(canvas, fmt.Sprintf("%d / %d", s.ui.page+1, maxInt(1, (len(s.cgNames)+11)/12)), 300, 34, 10)
}

func (s *Session) drawKaisou(canvas *gfx.BgraCanvas) {
	if s.profile == gothicProfileToku1 {
		s.drawToku1Kaisou(canvas)
		return
	}
	s.drawUIFullImage(canvas, "kaisou1.gsa")
	start := s.ui.page * 12
	for i := 0; i < 12; i++ {
		scene := start + i + 1
		if scene > 21 {
			continue
		}
		rect := albumCellRect(i)
		if s.hSceneUnlocked(scene) {
			image := s.catalog.Resolve(s.hSceneImage(scene))
			if image != nil {
				canvas.BlitScaled(image, rect[0]+7, rect[1]+7, rect[2]-14, rect[3]-14, 255)
			}
		} else {
			canvas.Fill(rect[0]+4, rect[1]+4, rect[2]-8, rect[3]-8, 0, 0, 0, 255)
		}
		if s.ui.hover == i+3 {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
	}
	s.drawUIText(canvas, fmt.Sprintf("%d / 2", s.ui.page+1), 300, 34, 10)
}

func (s *Session) drawMusic(canvas *gfx.BgraCanvas) {
	if s.profile == gothicProfileToku1 {
		s.drawToku1Music(canvas)
		return
	}
	s.drawUIFullImage(canvas, "music0.gsa")
	for i := 0; i < 10; i++ {
		rect := musicCellRect(i)
		if s.ui.playingTrack == i {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
		if s.ui.hover == i+1 {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
	}
}

func (s *Session) drawPreview(canvas *gfx.BgraCanvas) {
	canvas.Clear(0, 0, 0, 255)
	if s.ui.preview == "" {
		return
	}
	image := s.galleryImage(s.ui.preview)
	if image == nil {
		return
	}
	if image.Width <= GothicScreenWidth && image.Height <= GothicScreenHeight {
		canvas.Draw(image, image.OffsetX, image.OffsetY)
	} else {
		scale := minFloat(float64(GothicScreenWidth)/float64(image.Width), float64(GothicScreenHeight)/float64(image.Height))
		w := int(float64(image.Width) * scale)
		h := int(float64(image.Height) * scale)
		canvas.BlitScaled(image, (GothicScreenWidth-w)/2, (GothicScreenHeight-h)/2, w, h, 255)
	}
	s.drawUITextCentered(canvas, "クリックで戻る", 448)
}

func (s *Session) drawSlots(canvas *gfx.BgraCanvas) {
	canvas.Clear(0, 0, 0, 255)
	canvas.FillBlend(0, 0, GothicScreenWidth, GothicScreenHeight, 10, 10, 20, 235)
	title := "ロード"
	if s.ui.slotSave {
		title = "セーブ"
	}
	s.drawUITextCentered(canvas, title, 18)
	for i := 0; i < s.saveSlotCount; i++ {
		rect := s.slotCellRect(i)
		canvas.FillBlend(rect[0], rect[1], rect[2], rect[3], 0, 0, 0, 180)
		active := s.ui.hover == i
		s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], active)
		info := GothicSaveSlotInfo{Index: i}
		if i < len(s.ui.slotInfos) {
			info = s.ui.slotInfos[i]
		}
		if s.saveSlotCount > 15 {
			// 24 槽用紧凑格：缩略图占上半，槽号占下半；日期/摘要放不下，
			// 由宿主的存档列表（Slots/PreviewSlot）补充。
			var preview *GothicSavePreview
			if i < len(s.ui.slotPreviews) {
				preview = s.ui.slotPreviews[i]
			}
			if preview != nil && preview.ThumbnailWidth > 0 && preview.ThumbnailHeight > 0 &&
				len(preview.Thumbnail) == preview.ThumbnailWidth*preview.ThumbnailHeight*4 {
				image := gfx.NewBsgImage(preview.ThumbnailWidth, preview.ThumbnailHeight, 0, 0, preview.Thumbnail)
				canvas.BlitScaled(image, rect[0]+10, rect[1]+2, 80, 60, 255)
			}
			s.drawUIText(canvas, fmt.Sprintf("%d", i+1), rect[0]+4, rect[1]+64, 4)
			continue
		}
		s.drawUIText(canvas, fmt.Sprintf("%d", i+1), rect[0]+8, rect[1]+6, 4)
		if info.Exists {
			stamp := ""
			if info.HasSavedAt {
				stamp = info.SavedAt.Format("2006/01/02 15:04")
			}
			s.drawUITextWidth(canvas, stamp, rect[0]+34, rect[1]+6, rect[2]-40)
			s.drawUITextWidth(canvas, info.Summary, rect[0]+8, rect[1]+34, rect[2]-110)
			var preview *GothicSavePreview
			if i < len(s.ui.slotPreviews) {
				preview = s.ui.slotPreviews[i]
			}
			if preview != nil && preview.ThumbnailWidth > 0 && preview.ThumbnailHeight > 0 &&
				len(preview.Thumbnail) == preview.ThumbnailWidth*preview.ThumbnailHeight*4 {
				image := gfx.NewBsgImage(preview.ThumbnailWidth, preview.ThumbnailHeight, 0, 0, preview.Thumbnail)
				canvas.BlitScaled(image, rect[0]+rect[2]-88, rect[1]+6, 80, 60, 255)
			}
		} else {
			s.drawUIText(canvas, "EMPTY", rect[0]+34, rect[1]+28, 8)
		}
	}
	s.drawUIText(canvas, "EXIT", 556, 446, 8)
}

func (s *Session) drawIngame(canvas *gfx.BgraCanvas) {
	if s.profile == gothicProfileToku1 {
		// Toku1 的对话框在下方，半透明面板会把正文透出来；这里用接近不透明的底。
		canvas.FillBlend(190, 110, 260, 320, 0, 0, 0, 255)
		s.drawUIBorder(canvas, 190, 110, 260, 320, false)
		s.drawUITextCentered(canvas, "メニュー", 120)
	} else {
		s.drawUIPanel(canvas, 190, 120, 260, 300)
		s.drawUITextCentered(canvas, "メニュー", 132)
	}
	hits := s.uiItems()
	for i, hit := range hits {
		if s.ui.hover == i {
			canvas.FillBlend(hit.x, hit.y, hit.w, hit.h, 40, 90, 120, 180)
			s.drawUIBorder(canvas, hit.x, hit.y, hit.w, hit.h, true)
		}
		s.drawUITextCentered(canvas, hit.label, hit.y+8)
	}
}

func (s *Session) drawStaffRoll(canvas *gfx.BgraCanvas) {
	index := s.ui.staffIndex
	if index < 0 || index >= 8 {
		return
	}
	base := s.catalog.Resolve(fmt.Sprintf("stbk%02d", index+1))
	overlay := s.catalog.Resolve(fmt.Sprintf("staff%02d", index+1))
	canvas.Clear(0, 0, 0, 255)
	if base != nil {
		canvas.Draw(base, base.OffsetX, base.OffsetY)
	}
	if overlay != nil {
		canvas.Draw(overlay, overlay.OffsetX, overlay.OffsetY)
	}
	s.drawUITextCentered(canvas, "クリックで次へ", 448)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
