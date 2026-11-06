package session

import (
	"strconv"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_museum.go 是鉴赏室的渲染与交互。
//
// 原引擎的鉴赏室不是一个列表：它把底图、左侧角色页签、缩略图网格、达成率
// 都做成了素材，引擎只负责按 bsxx.cfg 的坐标把图贴上去。这里沿用同一套素材，
// 因此玩家看到的排版和原版一致；数据（CG 目录、解锁位、场景标签）来自
// cg.dat / scene.dat / save/common.dat，与画廊无关。

func (s *Session) openMuseumMode(index int) {
	layout := s.config.Museum
	if index < 0 || index >= len(layout.Modes) {
		return
	}
	ui := s.ui
	if ui == nil {
		return
	}
	// 切到别的鉴赏页签时 BGV 试听必须停：它和剧情语音共用人数通道，
	// 留着播会让新页签的背景音乐/环境音混在一起，而且停止按钮已经不可见。
	if ui.kind == uiMuseumBGV && layout.Modes[index].Key != "bgv" {
		if ui.bgv != nil && ui.bgv.bgvPlaying {
			s.stopBGVTrack(0)
		}
		ui.bgv = nil
	}
	switch layout.Modes[index].Key {
	case "cg":
		ui.kind = uiMuseumCG
		ui.cgGroup = 0
		ui.cgEntry = 0
		ui.cgVariant = 0
	case "scene":
		ui.kind = uiMuseumScene
		ui.scenePage = 0
	case "music":
		ui.kind = uiMuseumMusic
		ui.musicTrack = 0
	case "bgv":
		if !s.openBGVMuseum() {
			core.Logf("ui: BGV museum unavailable")
		}
	case "strip":
		// 牝教師2 老版：museum_chs 角色卡 + bu*/gb* 组合查看器。
		if layout.MK2Strip && s.openMK2StripMuseum() {
			return
		}
		if layout.MKStrip && s.openMKStripMuseum() {
			return
		}
		if len(layout.StripPortraits) > 0 {
			ui.kind = uiMuseumStrip
			s.composer.Invalidate()
			return
		}
		// すくーるヘブン 初回限定版（BSScript Rev.3）：没有 buex/legacy/KD/dress
		// 任何一条可走的布局，但有自己的 museum_ch1..8 角色卡与 cl 立绘族，
		// 先试这一代专属页，失败再按旧顺序回退。
		if s.openSHStripMuseum() {
			return
		}
		if !s.openBustupMuseum() {
			// 没有 buex/legacy 布局时先试 KD 的 sd_* 整身图页；
			// 再退回脱衣/立ち絵设定，至少不要卡在鉴赏主菜单。
			if !s.openKDStripMuseum() && !s.openDressScreen() {
				core.Logf("ui: standing gallery unavailable")
			}
		}
	default:
		// bgv/strip 需要额外的音频通道/换装数据，当前作品没有接入；
		// 留在主菜单而不是弹一个空页面。
		core.Logf("ui: museum mode '%s' is not implemented", layout.Modes[index].Key)
		return
	}
	s.composer.Invalidate()
}

// ---------------------------------------------------------------- 绘制

func (s *Session) drawMuseumMain(canvas *gfx.BgraCanvas) {
	layout := s.config.Museum
	if !s.drawLayerAt(canvas, layout.Base, 0, 0) {
		// MKIE / MK2IE 的下载版归档里没有 museum_base（[museum phase] 却仍然
		// 指向它）。原引擎在缺图时保留黑色清屏，所以这里显式铺黑：否则鉴赏
		// 主菜单会把进入之前的标题/剧情画面透在页签后面，像是界面没刷新。
		canvas.Clear(0, 0, 0, 255)
	}
	s.drawMuseumTabs(canvas)
}

func (s *Session) drawMuseumTabs(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.Museum
	active := ""
	switch ui.kind {
	case uiMuseumCG:
		active = "cg"
	case uiMuseumScene:
		active = "scene"
	case uiMuseumMusic:
		active = "music"
	case uiMuseumBGV:
		active = "bgv"
	case uiMuseumBU, uiMuseumStrip:
		active = "strip"
	}
	for index, mode := range layout.Modes {
		frame := 0
		if mode.Key == active {
			// 旧作第 2 帧是黑字（AFDVD），选中态几乎看不见；配置给了
			// ActiveFrame 时按作品指定，零值保持其它品牌原来的第 2 帧。
			frame = 2
			if mode.ActiveFrame > 0 {
				frame = mode.ActiveFrame
			}
		} else if ui.hover.kind == hitMuseumMode && ui.hover.index == index {
			frame = 1
		}
		button := mode.UIButton
		// 状态优先的复合图（MK 的 museum_menu：0..4 常态、5..9 悬停、
		// 10..14 选中）不能用 frame 直接切；把状态帧折算成实际行号，
		// 再让 drawFrameButton 固定画那一行。
		if button.FrameStep > 0 {
			button.FixedFrame = true
			button.Start = button.FrameOffset + frame*button.FrameStep
		}
		s.drawFrameButton(canvas, button, frame)
	}
	frame := 0
	if ui.hover.kind == hitMuseumReturn {
		frame = 1
	}
	button := layout.Return
	// 返回键和页签在同一张状态优先图里（第 4 行常态、第 9 行悬停），
	// 同样按 FrameStep 折算；其它作品零值时行为不变。
	if button.FrameStep > 0 {
		button.FixedFrame = true
		button.Start = button.FrameOffset + frame*button.FrameStep
	}
	s.drawFrameButton(canvas, button, frame)
}

func (s *Session) drawMuseumCG(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.Museum
	page := layout.CG
	if layout.Gakuen2 {
		s.drawGK2Museum(canvas, false)
		return
	}
	if layout.Ancient {
		s.drawAncientMuseumGrid(canvas, false)
		return
	}
	catalog := s.ensureCg()
	groups := 0
	if catalog != nil {
		groups = len(catalog.Groups)
	}
	group := 0
	if groups > 0 {
		group = clampInt(ui.cgGroup, 0, groups-1)
	}
	// museum_base/museum_cg/museum_cg_base 都是 BSS-Composition：
	// drawImageAt 只画第一块，会把页签/底纹的其余部件丢掉。
	s.drawLayerAt(canvas, layout.Base, 0, 0)
	s.drawLayerAt(canvas, page.UnderBase, 0, 0)
	// MDDVD 的每个分组有一张整屏底图（cg0_base..cg4_base），分页高亮
	// 已经烤在底图里；只画 page.Base 时翻页后标题/页签永远停在第 1 页。
	if !page.Atlas {
		s.drawLayerAt(canvas, museumPageBaseName(page.Base, page.Bases, group), 0, 0)
	}
	if !page.PageBase.Empty() {
		s.drawFrameButton(canvas, page.PageBase, 0)
	}
	// 精灵图世代：标题条和左栏底板是 base 里的矩形。
	if page.Atlas {
		if !page.Title.Empty() {
			s.drawFrameButton(canvas, page.Title, 0)
		}
		if !page.Menu.Empty() {
			s.drawFrameButton(canvas, page.Menu, 0)
		}
	}

	entries := []data.CgEntry(nil)
	if groups > 0 {
		entries = catalog.Groups[group].Entries
	}
	for index, button := range page.PageButtons {
		if groups > 0 && index >= groups {
			break
		}
		frame := 0
		if index == group {
			frame = 2
		} else if ui.hover.kind == hitCGPage && ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
	cellW, cellH := museumThumbSize(page.ThumbW, page.ThumbH)
	strip := ""
	if group >= 0 && group < len(page.ThumbStrips) {
		strip = page.ThumbStrips[group]
	}
	stripFrames := s.museumThumbStripFrames(strip, cellH)
	count := len(entries)
	// 有预制缩略图条时以条里的帧数上限为准；目录解析失败也仍能显示缩略图。
	if stripFrames > 0 && (count == 0 || stripFrames < count) {
		count = stripFrames
	}
	for index, point := range page.Thumbs {
		if index >= count {
			break
		}
		// 旧作（AFDVD）的格子是独立素材 museum_empty：白框 + 黑底。
		// 画面区已经按内边距缩到 128x99，框画在 point 左上 8px，
		// 两者不会互相盖住；没有缩略图时就是原版的空框。
		if !page.ThumbFrame.Empty() {
			frame := page.ThumbFrame
			frame.X, frame.Y = point.X-8, point.Y-8
			s.drawFrameButton(canvas, frame, 0)
		}
		if index < len(page.ThumbFrames) {
			frame := page.ThumbFrames[index]
			frame.X, frame.Y = point.X, point.Y
			s.drawFrameButton(canvas, frame, 0)
		}
		thumbPoint := point
		thumbPoint.X += page.ThumbOffset.X
		thumbPoint.Y += page.ThumbOffset.Y
		if s.drawMuseumThumbStrip(canvas, strip, index, thumbPoint, cellH) {
			// 预制缩略图本身就是原版画面；不再从完整 CG 合成。
		} else if index < len(entries) {
			entry := entries[index]
			thumb := s.thumbnailCanvasSized(entryThumbImages(entry), cellW, cellH)
			if thumb != nil {
				canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, thumbPoint.X, thumbPoint.Y, 255)
			}
		}
		if index < len(entries) && !s.cgEntryUnlocked(entries[index]) {
			// 未解锁的格子保留轮廓但压暗，玩家能看出"这里有一张 CG"，
			// 又不会被剧透。原引擎同样不画未解锁的缩略图内容。
			canvas.FillBlend(thumbPoint.X, thumbPoint.Y, cellW, cellH, 0, 0, 0, 176)
		}
		if ui.hover.kind == hitCGThumb && ui.hover.index == index {
			s.drawThumbFrameSized(canvas, point.X, point.Y, cellW, cellH)
		}
	}
	s.drawPercent(canvas, page.Percent, page.PercentSpace, s.cgUnlockPercentGroup(group))
	s.drawMuseumTabs(canvas)
}

// museumThumbStripFrames 返回预制缩略图条里有多少帧。
//
// cg0_thumb 这类图是 120x1800 的竖排条，解码器按图头 FrameCount 记录帧数；
// 个别素材头里没写，就按"总高 / 单格高"推算。
func (s *Session) museumThumbStripFrames(strip string, cellHeight int) int {
	if strip == "" || s.composer == nil {
		return 0
	}
	image := s.composer.Load(strip)
	if image == nil {
		return 0
	}
	frames := image.FrameCount
	if frames <= 1 && cellHeight > 0 {
		frames = image.Height / max(1, cellHeight)
	}
	if frames <= 0 {
		return 0
	}
	return frames
}

// drawMuseumThumbStrip 从竖排缩略图条里画第 frame 格。
func (s *Session) drawMuseumThumbStrip(canvas *gfx.BgraCanvas, strip string, frame int, point config.UIPoint, cellHeight int) bool {
	if strip == "" || s.composer == nil {
		return false
	}
	image := s.composer.Load(strip)
	if image == nil {
		return false
	}
	frames := image.FrameCount
	if frames <= 1 && cellHeight > 0 {
		frames = image.Height / max(1, cellHeight)
	}
	if frames <= 0 || frame < 0 || frame >= frames {
		return false
	}
	height := image.Height / frames
	if height <= 0 {
		return false
	}
	canvas.DrawRegion(image, 0, frame*height, image.Width, height, point.X, point.Y)
	return true
}

// drawMuseumStrip 画旧式立绘鉴赏页：museum_base + 页签 + 整身立绘部件。
// すくーるヘブン 初回限定版走 shStrip 状态的两态绘制（角色卡/立绘查看），
// 其余作品保持 StripPortraits 的原行为。
func (s *Session) drawMuseumStrip(canvas *gfx.BgraCanvas) {
	layout := s.config.Museum
	if ui := s.ui; ui != nil && ui.mkStrip != nil {
		s.drawMKStripMuseum(canvas)
		return
	}
	if ui := s.ui; ui != nil && ui.mk2Strip != nil {
		s.drawMK2StripMuseum(canvas)
		return
	}
	if ui := s.ui; ui != nil && ui.shStrip != nil {
		s.drawSHStripMuseum(canvas)
		return
	}
	s.drawImageAt(canvas, layout.Base, 0, 0)
	for _, portrait := range layout.StripPortraits {
		s.drawFrameButton(canvas, portrait, 0)
	}
	s.drawMuseumTabs(canvas)
}

// museumPageBaseName 选"每个分页一张整屏底图"的作品（MDDVD）当前页的底图。
// bases 为空时退回单张 base；索引越界时夹到最后一页，避免损坏的配置黑屏。
func museumPageBaseName(base string, bases []string, index int) string {
	if len(bases) == 0 {
		return base
	}
	index = clampInt(index, 0, len(bases)-1)
	return bases[index]
}

func (s *Session) drawMuseumScene(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.Museum
	page := layout.Scene
	if layout.Gakuen2 {
		s.drawGK2Museum(canvas, true)
		return
	}
	if layout.Ancient {
		s.drawAncientMuseumGrid(canvas, true)
		return
	}
	s.drawLayerAt(canvas, layout.Base, 0, 0)
	s.drawLayerAt(canvas, page.UnderBase, 0, 0)
	// 同 CG 页：MDDVD 的 sc0_base / sc1_base 每页一张，页签高亮烤在底图里。
	if !page.Atlas {
		s.drawLayerAt(canvas, museumPageBaseName(page.Base, page.Bases, ui.scenePage), 0, 0)
	}
	if page.Atlas {
		if !page.Title.Empty() {
			s.drawFrameButton(canvas, page.Title, 0)
		}
		if !page.Menu.Empty() {
			s.drawFrameButton(canvas, page.Menu, 0)
		}
	}

	catalog := s.ensureScenes()
	if catalog == nil || len(catalog.Scenes) == 0 {
		s.drawMuseumTabs(canvas)
		return
	}
	// 每页对应 scene.dat 的一个分组（角色事件/通常场景），不是按格子数顺序切。
	// 页签图标数量就是可显示的页数；没有图标时退回分组数。
	pages := catalog.Pages()
	pageCount := s.museumScenePageCount()
	if pageCount < len(pages) {
		core.Logf("ui: scene has %d group(s) but only %d page icon(s)", len(pages), pageCount)
		pages = pages[:pageCount]
	}
	ui.scenePage = clampInt(ui.scenePage, 0, max(0, pageCount-1))
	if ui.scenePage >= len(pages) {
		ui.scenePage = max(0, len(pages)-1)
	}
	for index, button := range page.PageButtons {
		if index >= pageCount || index >= len(pages) {
			break
		}
		frame := 0
		if index == ui.scenePage {
			frame = 2
		} else if ui.hover.kind == hitScenePage && ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
	var pageScenes []int
	if ui.scenePage >= 0 && ui.scenePage < len(pages) {
		pageScenes = pages[ui.scenePage].Scenes
	}
	cellW, cellH := museumThumbSize(page.ThumbW, page.ThumbH)
	strip := ""
	if ui.scenePage >= 0 && ui.scenePage < len(page.ThumbStrips) {
		strip = page.ThumbStrips[ui.scenePage]
	}
	stripFrames := s.museumThumbStripFrames(strip, cellH)
	count := len(pageScenes)
	if stripFrames > 0 && (count == 0 || stripFrames < count) {
		count = stripFrames
	}
	for index, point := range page.Thumbs {
		if index >= count {
			break
		}
		// 旧作（AFDVD）的格子是独立素材 museum_empty：白框 + 黑底。
		// 画面区已经按内边距缩到 128x99，框画在 point 左上 8px，
		// 两者不会互相盖住；没有缩略图时就是原版的空框。
		if !page.ThumbFrame.Empty() {
			frame := page.ThumbFrame
			frame.X, frame.Y = point.X-8, point.Y-8
			s.drawFrameButton(canvas, frame, 0)
		}
		if index < len(page.ThumbFrames) {
			frame := page.ThumbFrames[index]
			frame.X, frame.Y = point.X, point.Y
			s.drawFrameButton(canvas, frame, 0)
		}
		thumbPoint := point
		thumbPoint.X += page.ThumbOffset.X
		thumbPoint.Y += page.ThumbOffset.Y
		if s.drawMuseumThumbStrip(canvas, strip, index, thumbPoint, cellH) {
			// 预制缩略图条。
		} else if index < len(pageScenes) {
			scene := catalog.Scenes[pageScenes[index]]
			thumb := s.thumbnailCanvasSized(scene.Images, cellW, cellH)
			if thumb != nil {
				canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, thumbPoint.X, thumbPoint.Y, 255)
			}
		}
		if index < len(pageScenes) && !s.sceneUnlockedAt(pageScenes[index]) {
			canvas.FillBlend(point.X, point.Y, cellW, cellH, 0, 0, 0, 176)
		}
		if ui.hover.kind == hitSceneThumb && ui.hover.index == index {
			s.drawThumbFrameSized(canvas, point.X, point.Y, cellW, cellH)
		}
	}
	s.drawPercent(canvas, page.Percent, page.PercentSpace, s.sceneUnlockPercentPage(pages, ui.scenePage))
	// 顶部两个选择模式：点击缩略图是"立即播放"还是"加入播放列表"。
	// 三帧按钮：0 常态、1 悬停、2 选中。
	for index, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitScenePlayNow, page.PlayNow},
		{hitSceneAddPlaylist, page.AddPlaylist},
	} {
		frame := 0
		if ui.sceneMode == index {
			frame = 2
		} else if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
	// 播放列表入口是两帧按钮：0 常态、1 悬停。
	playlistFrame := 0
	if ui.hover.kind == hitSceneOpenPlaylist {
		playlistFrame = 1
	}
	s.drawFrameButton(canvas, page.PlaylistButton, playlistFrame)
	// 旧式场景页没有 play now / add playlist 按钮图，只在右侧文字旁画一个
	// 8x8 的选择标记；当前模式用 sceneMode 决定标记落在哪个坐标。
	if len(page.ClickModePoints) > 0 && !page.ClickMode.Empty() {
		point := page.ClickModePoints[clampInt(ui.sceneMode, 0, len(page.ClickModePoints)-1)]
		button := page.ClickMode
		button.X, button.Y = point.X, point.Y
		s.drawFrameButton(canvas, button, 0)
	}
	s.drawMuseumTabs(canvas)
}

// drawAncientMuseumGrid 画 Tbxdl 世代的 CG/Scene 鉴赏页。
//
// 这一代不是 [museum phase] 的页签模型：底图 cg_base/sc_base 上方固定
// 5x4 个 120x120 缩略格，右上角四个小按钮控制上一组/回到第一组/下一组/返回。
// CG 与 Scene 的字段布局相同，用 scene 参数选择数据源，避免复制两份绘制。
func (s *Session) drawAncientMuseumGrid(canvas *gfx.BgraCanvas, scene bool) {
	ui := s.ui
	if ui == nil || s.config == nil {
		return
	}
	layout := s.config.Museum
	base := layout.CG.Base
	thumbs := layout.CG.Thumbs
	thumbW, thumbH := layout.CG.ThumbW, layout.CG.ThumbH
	navPrev, navTop, navNext, navBack := layout.CG.NavPrev, layout.CG.NavTop, layout.CG.NavNext, layout.CG.NavBack
	thumbFrame := layout.CG.ThumbFrame
	hitPrev, hitTop, hitNext, hitBack := hitCGPrev, hitCGPage, hitCGNext, hitCGExit
	hitThumb := hitCGThumb
	if scene {
		base = layout.Scene.Base
		thumbs = layout.Scene.Thumbs
		thumbW, thumbH = layout.Scene.ThumbW, layout.Scene.ThumbH
		navPrev, navTop, navNext, navBack = layout.Scene.NavPrev, layout.Scene.NavTop, layout.Scene.NavNext, layout.Scene.NavBack
		thumbFrame = layout.Scene.ThumbFrame
		hitThumb = hitSceneThumb
		if navPrev.Empty() {
			navPrev, navTop, navNext, navBack = layout.CG.NavPrev, layout.CG.NavTop, layout.CG.NavNext, layout.CG.NavBack
		}
	}
	s.drawLayerAt(canvas, base, 0, 0)
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{{hitPrev, navPrev}, {hitTop, navTop}, {hitNext, navNext}, {hitBack, navBack}} {
		frame := 0
		if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}

	cellW, cellH := museumThumbSize(thumbW, thumbH)
	var images [][]string
	if scene {
		catalog := s.ensureScenes()
		if catalog == nil {
			return
		}
		pages := catalog.Pages()
		pageCount := s.museumScenePageCount()
		if pageCount < len(pages) {
			pages = pages[:pageCount]
		}
		ui.scenePage = clampInt(ui.scenePage, 0, max(0, len(pages)-1))
		if ui.scenePage >= len(pages) {
			return
		}
		for _, index := range pages[ui.scenePage].Scenes {
			if index >= 0 && index < len(catalog.Scenes) {
				images = append(images, catalog.Scenes[index].Images)
			}
		}
	} else {
		catalog := s.ensureCg()
		if catalog == nil || len(catalog.Groups) == 0 {
			return
		}
		ui.cgGroup = clampInt(ui.cgGroup, 0, len(catalog.Groups)-1)
		for _, entry := range catalog.Groups[ui.cgGroup].Entries {
			images = append(images, entryThumbImages(entry))
		}
	}
	for index, point := range thumbs {
		if index >= len(images) {
			break
		}
		// 缩略图外框是 126x96 的黑底白边图，必须画在缩略图之前：
		// 它只负责边框/空格占位，画在缩略图上会盖住 120x90 的画面。
		if !thumbFrame.Empty() {
			button := thumbFrame
			button.X, button.Y = point.X-3, point.Y-3
			s.drawFrameButton(canvas, button, 0)
		}
		thumb := s.thumbnailCanvasSized(images[index], cellW, cellH)
		if thumb != nil {
			canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, point.X, point.Y, 255)
		}
		unlocked := false
		if scene {
			// 场景的解锁位按 scene.dat 行序，已经由 drawMuseumScene 的原有逻辑共用。
			unlocked = s.ancientSceneThumbUnlocked(index)
		} else {
			catalog := s.ensureCg()
			if catalog != nil && ui.cgGroup >= 0 && ui.cgGroup < len(catalog.Groups) && index < len(catalog.Groups[ui.cgGroup].Entries) {
				unlocked = s.cgEntryUnlocked(catalog.Groups[ui.cgGroup].Entries[index])
			}
		}
		if !unlocked {
			canvas.FillBlend(point.X, point.Y, cellW, cellH, 0, 0, 0, 176)
		}
		if ui.hover.kind == hitThumb && ui.hover.index == index {
			canvas.FillBlend(point.X-1, point.Y-1, cellW+2, 2, 255, 255, 255, 200)
			canvas.FillBlend(point.X-1, point.Y+cellH-1, cellW+2, 2, 255, 255, 255, 200)
		}
	}
}

// ancientSceneThumbUnlocked 保守地按当前场景页的槽位判定解锁。
// 拿不到目录时按已解锁处理，宁可显示缩略图也不要把整页压黑。
func (s *Session) ancientSceneThumbUnlocked(index int) bool {
	catalog := s.ensureScenes()
	if catalog == nil {
		return true
	}
	pages := catalog.Pages()
	pageCount := s.museumScenePageCount()
	if pageCount < len(pages) {
		pages = pages[:pageCount]
	}
	if s.ui == nil || s.ui.scenePage < 0 || s.ui.scenePage >= len(pages) {
		return true
	}
	slots := pages[s.ui.scenePage].Scenes
	if index < 0 || index >= len(slots) {
		return true
	}
	return s.sceneUnlockedAt(slots[index])
}

func (s *Session) drawMuseumMusic(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.Museum
	if layout.Gakuen2 {
		s.drawGK2Music(canvas)
		return
	}
	page := layout.Music
	s.drawImageAt(canvas, layout.Base, 0, 0)
	if !page.PageBase.Empty() {
		s.drawFrameButton(canvas, page.PageBase, 0)
	}
	if !page.Atlas {
		s.drawImageAt(canvas, page.Base, 0, 0)
	}
	if page.Atlas {
		if !page.Title.Empty() {
			s.drawFrameButton(canvas, page.Title, 0)
		}
		if !page.Menu.Empty() {
			s.drawFrameButton(canvas, page.Menu, 0)
		}
		if !page.ButtonFrame.Empty() {
			s.drawFrameButton(canvas, page.ButtonFrame, 0)
		}
		if !page.VolumeFrame.Empty() {
			s.drawFrameButton(canvas, page.VolumeFrame, 0)
		}
	}
	if page.BakedTracks {
		// 曲名已经烘焙在 PageBase 里：Tracks 只登记每行的命中矩形，
		// 逐条绘制会把 20 行全部变成紫色高亮。只补画悬停/当前行。
		if !page.TrackSelect.Empty() {
			if ui.hover.kind == hitMusicTrack && ui.hover.index != ui.musicTrack {
				s.drawFrameButton(canvas, page.TrackSelect, ui.hover.index)
			}
			if ui.musicTrack >= 0 {
				s.drawFrameButton(canvas, page.TrackSelect, ui.musicTrack)
			}
		}
		if !page.ControlBase.Empty() {
			s.drawFrameButton(canvas, page.ControlBase, 0)
		}
		for _, item := range []struct {
			kind   uiHitKind
			button config.UIButton
		}{
			{hitMusicPlay, page.Play},
			{hitMusicStop, page.Stop},
			{hitMusicPrev, page.Prev},
			{hitMusicNext, page.Next},
			{hitMusicMin, page.Min},
			{hitMusicMax, page.Max},
		} {
			frame := 0
			if ui.hover.kind == item.kind {
				frame = 1
			}
			s.drawFrameButton(canvas, item.button, frame)
		}
		s.drawMuseumMusicVolume(canvas, page)
		s.drawMuseumTabs(canvas)
		return
	}
	if layout.Ancient {
		// Tbxdl 的音乐页把曲名和播放/停止图标都烘焙在 music_base 里；
		// 只需要把当前曲目（以及悬停曲目）的高亮条贴上去。
		// 不能再逐条画 Tracks，否则 22 行会同时变成紫色高亮。
		if !page.TrackSelect.Empty() {
			if ui.hover.kind == hitMusicTrack && ui.hover.index != ui.musicTrack {
				s.drawFrameButton(canvas, page.TrackSelect, ui.hover.index)
			}
			if ui.musicTrack >= 0 {
				s.drawFrameButton(canvas, page.TrackSelect, ui.musicTrack)
			}
		}
		s.drawMuseumMusicVolume(canvas, page)
		return
	}
	// KD 的控制盒底图包含四个按钮空槽和竖向音量轨道；先画底图，
	// 再把图标/滑块叠上去，否则按钮后面的空槽会丢掉。
	if !page.ControlBase.Empty() {
		s.drawFrameButton(canvas, page.ControlBase, 0)
	}
	for index, track := range page.Tracks {
		frame := 0
		if index == ui.musicTrack && ui.musicPlaying {
			frame = 2
		} else if ui.hover.kind == hitMusicTrack && ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, track, frame)
	}
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitMusicPlay, page.Play},
		{hitMusicStop, page.Stop},
		{hitMusicPrev, page.Prev},
		{hitMusicNext, page.Next},
		{hitMusicMin, page.Min},
		{hitMusicMax, page.Max},
	} {
		frame := 0
		if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
	s.drawMuseumMusicVolume(canvas, page)
	s.drawMuseumTabs(canvas)
}

// drawMuseumMusicVolume 画 BGM 鉴赏的音量条。
//
// 新版作品的 volume 键给的是整条横条（侵蝕/淫獄/KDIE），原样画即可；
// KD 给的是滑块，位置由 BGM 音量决定：最大音量在轨道顶端，静音在底端。
func (s *Session) drawMuseumMusicVolume(canvas *gfx.BgraCanvas, page config.MuseumMusicPage) {
	if page.Volume.Empty() {
		return
	}
	if page.VolumeFill {
		// 横向电平条：按当前 BGM 音量截取左侧一段。原图的青色条是满格，
		// 直接整条画出来会让玩家误以为音量永远是最大。
		image := s.buttonImage(page.Volume)
		if image == nil {
			return
		}
		height := image.Height / max(1, page.Volume.Frames)
		width := int(float64(image.Width) * max(0, min(1, s.settings.BgmVolume)))
		if width <= 0 || height <= 0 {
			return
		}
		canvas.DrawRegion(image, 0, 0, width, height, page.Volume.X, page.Volume.Y)
		return
	}
	if page.Volume.Atlas && page.Volume.Clip[2] > 0 && page.Volume.Clip[3] > 0 {
		// 精灵图音量条是竖排填充条：按 BGM 音量从底部截取一段，
		// 否则永远画满格，和原版音量位置对不上。
		image := s.buttonImage(page.Volume)
		if image == nil {
			return
		}
		srcX, srcY := page.Volume.Clip[0], page.Volume.Clip[1]
		width, fullHeight := page.Volume.Clip[2], page.Volume.Clip[3]
		fraction := max(0, min(1, s.settings.BgmVolume))
		height := int(float64(fullHeight)*fraction + 0.5)
		if height <= 0 {
			return
		}
		canvas.DrawRegion(image, srcX, srcY+fullHeight-height, width, height,
			page.Volume.X, page.Volume.Y+fullHeight-height)
		return
	}
	if !page.VolumeKnob {
		s.drawFrameButton(canvas, page.Volume, 0)
		return
	}
	image := s.buttonImage(page.Volume)
	if image == nil {
		return
	}
	height := image.Height / max(1, page.Volume.Frames)
	if height <= 0 {
		return
	}
	top := page.VolumeRegion[1]
	if top == 0 {
		top = page.Volume.Y
	}
	track := page.VolumeTrackHeight
	if track <= 0 {
		track = page.VolumeRegion[3] - height
	}
	if track < 0 {
		track = 0
	}
	volume := max(0, min(1, s.settings.BgmVolume))
	y := top + int(float64(track)*(1-volume))
	// 滑块的 X 以 Volume.X 为准；只有配置没给 X 时才按 region 居中。
	// KD 的 region 为触屏命中加宽过，不能拿它来算绘制位置。
	x := page.Volume.X
	if x == 0 && page.VolumeRegion[2] > 0 {
		x = page.VolumeRegion[0] + (page.VolumeRegion[2]-image.Width)/2
	}
	canvas.DrawRegion(image, 0, 0, image.Width, height, x, y)
}

// drawThumbFrame 在缩略图外画一圈选中框。
func (s *Session) drawThumbFrame(canvas *gfx.BgraCanvas, x, y int) {
	frame := s.config.DataScreen.SelectFrame
	if frame.Empty() {
		return
	}
	image := s.composer.Load(frame.Image)
	if image == nil {
		return
	}
	canvas.DrawRegion(image, 0, 0, image.Width, image.Height, x-2, y-1)
}

func (s *Session) drawPercent(canvas *gfx.BgraCanvas, strip config.UIButton, space, percent int) {
	if strip.Empty() {
		return
	}
	s.drawDigitStrip(canvas, strip, percent, strip.X, strip.Y, space)
}

func (s *Session) drawCGViewer(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	if s.config.Museum.Gakuen2 {
		s.drawGK2CGViewer(canvas)
		return
	}
	layout := s.config.Museum.Viewer
	if ui.cgCanvas != nil {
		if ui.cgZoom > 1 {
			// 放大渲染：把整张 CG 按 zoom 缩放，以画面中心为基准偏移 pan，
			// 再画控制盒（控制盒不缩放）。为什么中心基准：滚轮放大时
			// 中心不动，clamp 保证图像始终盖满画面（缩放后宽=width*zoom，
			// pan 范围 ±width*(zoom-1)/2）。
			scaledW := int(float64(s.width) * ui.cgZoom)
			scaledH := int(float64(s.height) * ui.cgZoom)
			// clamp 一遍：防御 pan 已越界（例如刚初始化）导致边界残留。
			panX := clampInt(ui.cgPanX, -s.cgPanLimitX(), s.cgPanLimitX())
			panY := clampInt(ui.cgPanY, -s.cgPanLimitY(), s.cgPanLimitY())
			dstX := (s.width-scaledW)/2 + panX
			dstY := (s.height-scaledH)/2 + panY
			scaled := ui.cgCanvas.ScaleTo(scaledW, scaledH)
			if scaled != nil {
				canvas.BlitCanvasRegion(scaled, 0, 0, scaledW, scaledH, dstX, dstY, 255)
			}
		} else {
			canvas.BlitCanvasRegion(ui.cgCanvas, 0, 0, s.width, s.height, 0, 0, 255)
		}
	}
	// KD 的右键只是隐藏控制盒，CG 本身继续显示；新版作品没有这个开关，
	// panelShown 恒为 true。
	panelShown := !layout.TogglePanel || ui.cgPanelShown
	if !panelShown {
		return
	}
	if layout.Atlas {
		button := layout.Base
		button.X, button.Y = layout.PanelX, layout.PanelY
		s.drawFrameButton(canvas, button, 0)
	} else {
		s.drawImageAt(canvas, layout.Base.Image, layout.PanelX, layout.PanelY)
	}
	// CG 查看器的按钮帧约定由配置决定：
	//   - 新版作品：0 帧是白色命中掩码，常态从 1 开始，悬停 +1；
	//   - KD：0..4 常态，5..9 红色悬停（ButtonBase=0、HoverOffset=5）。
	base, hoverOffset := cgViewerFrameDefaults(layout)
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitCGPrev, layout.Prev},
		{hitCGNext, layout.Next},
		{hitCGExit, layout.Exit},
		{hitCGConfig, layout.Config},
	} {
		if item.button.Empty() {
			continue
		}
		frame := base
		if layout.Atlas {
			frame = 0
			if ui.hover.kind == item.kind {
				frame = 1
			}
		} else if ui.hover.kind == item.kind {
			frame = base + hoverOffset
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
	// 底栏上的当前序号：原引擎用 10 帧数字条，这里画"当前变体号"。
	if !layout.Number.Empty() && ui.cgCanvas != nil {
		s.drawDigitStrip(canvas, layout.Number, ui.cgVariant+1, layout.NowNoX, layout.NowNoY, layout.NowNoSpaceX)
	} else if ui.cgCanvas != nil && s.textFont != nil && layout.Prev.Empty() && layout.Next.Empty() {
		// 特別授業3SLG 的 [cg viewer] 没有 `number` 键；不给任何序号会让
		// 玩家不知道当前是第几张。用引擎字体画一个最小的 "n / m"。
		// 有按钮的查看器（MK 的 cgm_btn）不能画：序号会和前一张按钮重叠。
		total := s.cgVariantCount()
		if total > 0 {
			caption := strconv.Itoa(ui.cgVariant+1) + " / " + strconv.Itoa(total)
			image := s.textFont.Render(caption, 16, 1, 1, 20, 200, 1, 0xffffffff)
			if image != nil {
				rendering.DrawTextImage(canvas, image, layout.PanelX+12, layout.PanelY+14)
			}
		}
	}
	s.drawCGViewerToggles(canvas)
	s.drawCGViewerBGV(canvas)
}

// cgViewerFrameDefaults 归一化 CG 查看器按钮的帧约定。
//
// ButtonBase/HoverOffset 的零值对 KD 是有意义的（常态 0、悬停 +5），
// 所以只有两者都是 0 时才认为是旧配置，退回新版作品的 1/1 约定。
func cgViewerFrameDefaults(layout config.CGViewer) (base, hoverOffset int) {
	base, hoverOffset = layout.ButtonBase, layout.HoverOffset
	if base == 0 && hoverOffset == 0 {
		return 1, 1
	}
	if hoverOffset <= 0 {
		hoverOffset = 1
	}
	return base, hoverOffset
}

// museumThumbSize 返回格子尺寸；配置没有 thumbN 时退回旧的 126x96。
func museumThumbSize(width, height int) (int, int) {
	if width <= 0 {
		width = thumbWidth
	}
	if height <= 0 {
		height = thumbHeight
	}
	return width, height
}

// drawThumbFrameSized 按格子尺寸画悬停边框（比缩略图略大）。
func (s *Session) drawThumbFrameSized(canvas *gfx.BgraCanvas, x, y, width, height int) {
	if s.config == nil {
		return
	}
	button := s.config.DataScreen.SelectFrame
	if button.Empty() {
		canvas.Fill(x, y, width, height, 255, 255, 255, 40)
		return
	}
	if image := s.composer.Load(button.Image); image != nil {
		canvas.Draw(image, x-4, y-4)
		return
	}
	canvas.Fill(x, y, width, height, 255, 255, 255, 40)
}

// cgVariantCount 返回当前 CG 当前组的变体总数，用于缺 number 配置时的序号。
func (s *Session) cgVariantCount() int {
	catalog := s.ensureCg()
	if catalog == nil || s.ui == nil || s.ui.cgGroup < 0 || s.ui.cgGroup >= len(catalog.Groups) {
		return 0
	}
	entries := catalog.Groups[s.ui.cgGroup].Entries
	if s.ui.cgEntry < 0 || s.ui.cgEntry >= len(entries) {
		return 0
	}
	return len(entries[s.ui.cgEntry].Variants)
}

// cgUnlockPercentGroup 只统计当前页签对应的分组。
//
// 整本统计会让"由梨香页"显示全作品达成率，玩家看到 60% 但该页明明
// 全解锁；原引擎每页的 percent 是分组内的比例。
func (s *Session) cgUnlockPercentGroup(group int) int {
	catalog := s.ensureCg()
	if catalog == nil || group < 0 || group >= len(catalog.Groups) {
		return s.cgUnlockPercent()
	}
	total, unlocked := 0, 0
	for _, entry := range catalog.Groups[group].Entries {
		total++
		if s.cgEntryUnlocked(entry) {
			unlocked++
		}
	}
	if total == 0 {
		return 0
	}
	return unlocked * 100 / total
}

// sceneUnlockPercentPage 只统计当前页签的场景。
func (s *Session) sceneUnlockPercentPage(pages []data.ScenePage, page int) int {
	if page < 0 || page >= len(pages) {
		return s.sceneUnlockPercent()
	}
	total, unlocked := 0, 0
	for _, index := range pages[page].Scenes {
		total++
		if s.sceneUnlockedAt(index) {
			unlocked++
		}
	}
	if total == 0 {
		return 0
	}
	return unlocked * 100 / total
}

// drawCGViewerToggles 画切入/头发/水汽三个演出开关。
//
// 这三个按钮图有 5 帧：0 是命中掩码（原引擎用它算点击区，永远不显示），
// 1 是"关"的常态，2 是"关"的悬停，3 是"开"的常态，4 是"开的悬停"。
// 所以常态帧是 1（关）或 3（开），鼠标悬停时再加 1。
//
// 开关状态直接取设置里的值：原引擎的 CG 查看器和剧情共用同一份演出开关，
// 在这里改设置也就改了后续剧情的演出方式。
func (s *Session) drawCGViewerToggles(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.Museum.Viewer
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
		on     bool
	}{
		{hitCGCutin, layout.CutIn, s.settings.Cutin},
		{hitCGHair, layout.Hair, s.settings.UnderHair},
		{hitCGSteam, layout.Steam, s.settings.Steam},
		// アヘ顔按钮的"开启"语义是隐藏アヘ顔，正好对应 settings.AheOff。
		{hitCGAhe, layout.Ahe, s.settings.AheOff},
	} {
		if item.button.Empty() {
			continue
		}
		// KD 的 CutIn 只有“黑/红”两态：常态帧 3，开启或悬停时画 8。
		// 新版作品的按钮图是 5 帧（掩码/关/关悬停/开/开悬停），沿用原逻辑。
		if layout.Atlas {
			frame := 0
			if item.on || ui.hover.kind == item.kind {
				frame = 1
			}
			s.drawFrameButton(canvas, item.button, frame)
			continue
		}
		if layout.ToggleHoverOffset > 0 {
			frame := 0
			if item.on || ui.hover.kind == item.kind {
				frame = layout.ToggleHoverOffset
			}
			s.drawFrameButton(canvas, item.button, frame)
			continue
		}
		frame := cgViewerToggleFrame(item.on, ui.hover.kind == item.kind)
		s.drawFrameButton(canvas, item.button, frame)
	}
}

// cgViewerToggleFrame 返回演出开关该画第几帧。
//
// 5 帧约定：0 是命中掩码（永不显示）、1 是"关"、2 是"关"的悬停、
// 3 是"开"、4 是"开"的悬停。所以常态帧是 1 或 3，悬停时再加 1。
func cgViewerToggleFrame(on, hover bool) int {
	frame := 1
	if on {
		frame = 3
	}
	if hover {
		frame++
	}
	return frame
}

// drawCGViewerBGV 画 BGV 音量条与它的详情按钮。
//
// "bgv detail" 按钮图有 3 帧：0 是命中掩码、1 是常态、2 是悬停；
// 点它展开/收起下面那条音量条。
func (s *Session) drawCGViewerBGV(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.Museum.Viewer
	frame := 1
	if ui.hover.kind == hitCGBGVDetail {
		frame = 2
	}
	s.drawFrameButton(canvas, layout.BGVDetail, frame)
	if !ui.cgBGVolumeShown {
		return
	}
	// 音量条底图是单帧图；真正的"当前音量"由白色滑块标出来。
	s.drawFrameButton(canvas, layout.BGVVolume, 0)
	s.drawCGBGVVolumeMarker(canvas, layout.BGVVolumeRegion)
}

// drawCGBGVVolumeMarker 在 BGV 音量条上画白色的滑块标记。
//
// 标记固定 3 像素宽，位置按音量（0..1）在条宽上的比例插值；
// 音量本身存在设置里，所以这里画出来的位置和剧情里的 BGV 音量始终一致。
func (s *Session) drawCGBGVVolumeMarker(canvas *gfx.BgraCanvas, region [4]int) {
	if region[2] <= 0 || region[3] <= 0 {
		return
	}
	width := 3
	if region[2] < width {
		width = region[2]
	}
	x := cgViewerMarkerX(region, s.settings.BgvVolume)
	y := region[1]
	canvas.Fill(x, y, width, region[3], 255, 255, 255, 255)
}

// cgViewerMarkerX 返回音量滑块标记的左边界。
//
// 标记宽 3 像素，滑到最右边时右边缘正好贴着区域的右边界，
// 所以位置按"区域宽 - 标记宽"插值。
func cgViewerMarkerX(region [4]int, volume float64) int {
	width := 3
	if region[2] < width {
		width = region[2]
	}
	if width <= 0 {
		return region[0]
	}
	volume = max(0, min(1, volume))
	return region[0] + int(float64(region[2]-width)*volume)
}

// ---------------------------------------------------------------- 命中

func (s *Session) hitMuseumCG(x, y int) uiHit {
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if s.config.Museum.Gakuen2 {
		return s.hitGK2Museum(x, y, false)
	}
	if s.config.Museum.Ancient {
		return s.hitAncientMuseum(x, y, false)
	}
	page := s.config.Museum.CG
	for index, button := range page.PageButtons {
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitCGPage, index: index}
		}
	}
	catalog := s.ensureCg()
	if catalog != nil && ui.cgGroup >= 0 && ui.cgGroup < len(catalog.Groups) {
		entries := catalog.Groups[ui.cgGroup].Entries
		cellW, cellH := museumThumbSize(page.ThumbW, page.ThumbH)
		for index, point := range page.Thumbs {
			if index >= len(entries) {
				break
			}
			if x >= point.X && y >= point.Y && x < point.X+cellW && y < point.Y+cellH {
				return uiHit{kind: hitCGThumb, index: index}
			}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) hitMuseumScene(x, y int) uiHit {
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if s.config.Museum.Gakuen2 {
		return s.hitGK2Museum(x, y, true)
	}
	if s.config.Museum.Ancient {
		return s.hitAncientMuseum(x, y, true)
	}
	page := s.config.Museum.Scene
	// 顶部模式按钮/播放列表入口先命中：它们在缩略图上方，不会和格子重叠。
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{{hitScenePlayNow, page.PlayNow}, {hitSceneAddPlaylist, page.AddPlaylist}, {hitSceneOpenPlaylist, page.PlaylistButton}} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	catalog := s.ensureScenes()
	pages := []data.ScenePage(nil)
	if catalog != nil {
		pages = catalog.Pages()
	}
	pageCount := s.museumScenePageCount()
	if pageCount < len(pages) {
		pages = pages[:pageCount]
	}
	for index, button := range page.PageButtons {
		if index >= pageCount || index >= len(pages) {
			break
		}
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitScenePage, index: index}
		}
	}
	// 旧式选择模式：标记图很小，按 16x16 的可点范围判定，手指也点得到。
	if len(page.ClickModePoints) > 0 {
		for index, point := range page.ClickModePoints {
			if x >= point.X-4 && y >= point.Y-4 && x < point.X+16 && y < point.Y+16 {
				if index == 0 {
					return uiHit{kind: hitScenePlayNow}
				}
				return uiHit{kind: hitSceneAddPlaylist}
			}
		}
	}
	if ui.scenePage >= 0 && ui.scenePage < len(pages) {
		cellW, cellH := museumThumbSize(page.ThumbW, page.ThumbH)
		for index, point := range page.Thumbs {
			if index >= len(pages[ui.scenePage].Scenes) {
				break
			}
			if x >= point.X && y >= point.Y && x < point.X+cellW && y < point.Y+cellH {
				return uiHit{kind: hitSceneThumb, index: index}
			}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) hitMuseumMusic(x, y int) uiHit {
	if s.config.Museum.Gakuen2 {
		return s.hitGK2Music(x, y)
	}
	page := s.config.Museum.Music
	for index, track := range page.Tracks {
		if hitButton(s, track, x, y) {
			return uiHit{kind: hitMusicTrack, index: index}
		}
	}
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitMusicPlay, page.Play},
		{hitMusicStop, page.Stop},
		{hitMusicPrev, page.Prev},
		{hitMusicNext, page.Next},
		{hitMusicMin, page.Min},
		{hitMusicMax, page.Max},
	} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	// 音量轨道：优先用配置的 region（KD 滑块），其它作品的 volume 图就是
	// 整条横条，按图片矩形命中。
	if region := page.VolumeRegion; region[2] > 0 && region[3] > 0 {
		if x >= region[0] && y >= region[1] && x < region[0]+region[2] && y < region[1]+region[3] {
			return uiHit{kind: hitMusicVolume}
		}
	}
	if hitButton(s, page.Volume, x, y) {
		return uiHit{kind: hitMusicVolume}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitAncientMuseum 处理 Tbxdl CG/Scene 页的命中。
// 导航按钮复用查看器的 hitCGPrev/hitCGPage/hitCGNext/hitCGExit，
// 这样 activateMuseumCG/Scene 不需要再引入一套动作类型。
func (s *Session) hitAncientMuseum(x, y int, scene bool) uiHit {
	layout := s.config.Museum
	navPrev, navTop, navNext, navBack := layout.CG.NavPrev, layout.CG.NavTop, layout.CG.NavNext, layout.CG.NavBack
	thumbs, thumbW, thumbH := layout.CG.Thumbs, layout.CG.ThumbW, layout.CG.ThumbH
	hitThumb := hitCGThumb
	if scene {
		navPrev, navTop, navNext, navBack = layout.Scene.NavPrev, layout.Scene.NavTop, layout.Scene.NavNext, layout.Scene.NavBack
		if navPrev.Empty() {
			navPrev, navTop, navNext, navBack = layout.CG.NavPrev, layout.CG.NavTop, layout.CG.NavNext, layout.CG.NavBack
		}
		thumbs, thumbW, thumbH = layout.Scene.Thumbs, layout.Scene.ThumbW, layout.Scene.ThumbH
		hitThumb = hitSceneThumb
	}
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{{hitCGPrev, navPrev}, {hitCGPage, navTop}, {hitCGNext, navNext}, {hitCGExit, navBack}} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	cellW, cellH := museumThumbSize(thumbW, thumbH)
	for index, point := range thumbs {
		if x >= point.X && y >= point.Y && x < point.X+cellW && y < point.Y+cellH {
			return uiHit{kind: hitThumb, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) hitCGViewer(x, y int) uiHit {
	layout := s.config.Museum.Viewer
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if s.config.Museum.Gakuen2 {
		return s.hitGK2CGViewer(x, y)
	}
	// 记下命中点：BGV 音量条按点击位置取比例，激活时要用。
	ui.cgPointerX = x
	ui.cgPointerY = y
	// KD：右键隐藏控制盒后，左键点画面仍然切换下一张差分。
	if layout.TogglePanel && !ui.cgPanelShown {
		if layout.ClickAdvance {
			return uiHit{kind: hitCGNext}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	// 前一页/后一页/退出/设置先判：它们在原版里位置固定，不能被新按钮抢走。
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{{hitCGPrev, layout.Prev}, {hitCGNext, layout.Next}, {hitCGExit, layout.Exit}, {hitCGConfig, layout.Config}} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	// 三个演出开关：5 帧按钮，命中区按配置里的帧数算高度。
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{{hitCGCutin, layout.CutIn}, {hitCGHair, layout.Hair}, {hitCGSteam, layout.Steam}, {hitCGAhe, layout.Ahe}} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	// BGV 音量条的可点击区是配置里的 region，底图本身也算。
	if s.hitCGBGVVolume(x, y) {
		return uiHit{kind: hitCGBGVVolume}
	}
	if hitButton(s, layout.BGVDetail, x, y) {
		return uiHit{kind: hitCGBGVDetail}
	}
	// KD 原作：左键点画面任意空白处切换下一张差分。
	if layout.ClickAdvance {
		return uiHit{kind: hitCGNext}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitCGBGVVolume 判断点击是否落在 BGV 音量条上。
//
// 可点击区以配置里的 "bgv volume region"（x、y、宽、高）为准：条上的底图
// 可能比区域窄，只按底图判会让两端点不到。
func (s *Session) hitCGBGVVolume(x, y int) bool {
	layout := s.config.Museum.Viewer
	region := layout.BGVVolumeRegion
	if region[2] > 0 && region[3] > 0 {
		if x >= region[0] && y >= region[1] && x < region[0]+region[2] && y < region[1]+region[3] {
			return true
		}
	}
	return hitButton(s, layout.BGVVolume, x, y)
}

// cgViewerButtonRect 计算按钮的命中矩形（左、上、右、下）。
//
// 规则和 hitButton 一致：宽用图片宽，高用"图片高 / 配置帧数"。
// 图片尺寸由调用方传入，这样没有素材时也能检查按钮之间的几何关系。
func cgViewerButtonRect(button config.UIButton, imageWidth, imageHeight int) (int, int, int, int) {
	if button.Empty() {
		return 0, 0, -1, -1
	}
	height := button.FrameHeight(imageHeight)
	return button.X, button.Y, button.X + imageWidth, button.Y + height
}

// ---------------------------------------------------------------- 交互

func (s *Session) activateMuseumCG(hit uiHit) {
	ui := s.ui
	if ui == nil {
		return
	}
	if s.config.Museum.Gakuen2 {
		s.activateGK2Museum(hit, false)
		return
	}
	catalog := s.ensureCg()
	switch hit.kind {
	case hitCGPrev:
		if s.config.Museum.Ancient && catalog != nil && len(catalog.Groups) > 0 {
			ui.cgGroup = (ui.cgGroup - 1 + len(catalog.Groups)) % len(catalog.Groups)
			ui.cgEntry, ui.cgVariant = 0, 0
		}
	case hitCGNext:
		if s.config.Museum.Ancient && catalog != nil && len(catalog.Groups) > 0 {
			ui.cgGroup = (ui.cgGroup + 1) % len(catalog.Groups)
			ui.cgEntry, ui.cgVariant = 0, 0
		}
	case hitCGExit:
		if s.config.Museum.Ancient {
			s.closeSystemUI()
			return
		}
	case hitCGPage:
		if s.config.Museum.Ancient {
			ui.cgGroup, ui.cgEntry, ui.cgVariant = 0, 0, 0
			return
		}
		if catalog != nil && hit.index >= 0 && hit.index < len(catalog.Groups) {
			ui.cgGroup = hit.index
			ui.cgEntry = 0
			ui.cgVariant = 0
		}
	case hitCGThumb:
		ui.cgEntry = hit.index
		ui.cgVariant = 0
		ui.kind = uiCGViewer
		ui.cgCanvas = s.renderCGEntry(ui.cgGroup, ui.cgEntry, ui.cgVariant)
		// 进查看器时重置缩放：换 CG 后从整屏开始，避免上一张的 zoom 串扰。
		s.resetCGZoom()
		// 打开查看器时把底栏开关的状态同步成设置里的值：
		// 切入/头发/水汽在原版里和剧情共用一份设置。
		ui.cgCutin = s.settings.Cutin
		ui.cgHair = s.settings.UnderHair
		ui.cgSteam = s.settings.Steam
		// BGV 音量条默认展开。
		ui.cgBGVolumeShown = true
		// KD 的控制盒默认显示，右键可收起。
		ui.cgPanelShown = true
	}
}

func (s *Session) activateMuseumScene(hit uiHit) {
	ui := s.ui
	if ui == nil {
		return
	}
	if s.config.Museum.Gakuen2 {
		s.activateGK2Museum(hit, true)
		return
	}
	catalog := s.ensureScenes()
	if catalog == nil {
		return
	}
	switch hit.kind {
	case hitCGPrev, hitCGNext, hitCGExit, hitCGPage:
		if !s.config.Museum.Ancient {
			break
		}
		pages := catalog.Pages()
		pageCount := s.museumScenePageCount()
		if pageCount < len(pages) {
			pages = pages[:pageCount]
		}
		if len(pages) == 0 {
			return
		}
		switch hit.kind {
		case hitCGPrev:
			ui.scenePage = (ui.scenePage - 1 + len(pages)) % len(pages)
		case hitCGNext:
			ui.scenePage = (ui.scenePage + 1) % len(pages)
		case hitCGPage:
			ui.scenePage = 0
		case hitCGExit:
			s.closeSystemUI()
		}
	case hitScenePlayNow:
		ui.sceneMode = 0
	case hitSceneAddPlaylist:
		ui.sceneMode = 1
	case hitSceneOpenPlaylist:
		ui.kind = uiScenePlaylist
		ui.playlistPage = 0
		ui.playlistSelected = -1
	case hitScenePage:
		ui.scenePage = max(0, hit.index)
	case hitSceneThumb:
		pages := catalog.Pages()
		pageCount := s.museumScenePageCount()
		if pageCount < len(pages) {
			pages = pages[:pageCount]
		}
		if ui.scenePage < 0 || ui.scenePage >= len(pages) {
			return
		}
		slots := pages[ui.scenePage].Scenes
		if hit.index < 0 || hit.index >= len(slots) {
			return
		}
		sceneIndex := slots[hit.index]
		if sceneIndex < 0 || sceneIndex >= len(catalog.Scenes) {
			return
		}
		scene := catalog.Scenes[sceneIndex]
		// 回放/播放列表要存 SceneId（L01_001_replay），不是场景表最后那一列的
		// label_01_001：后者是解锁用的标识，脚本里根本没有这个标签。
		if len(scene.SceneId) == 0 {
			core.Logf("ui: scene #%d has no scene id", sceneIndex)
			return
		}
		// "加入播放列表"模式下点缩略图不播放，而是把它塞进列表。
		if ui.sceneMode == 1 {
			if s.addSceneToPlaylist(scene.SceneId) {
				core.Logf("playlist: added '%s'", scene.SceneId)
			}
			return
		}
		s.startSceneReplay(scene.SceneId, uiMuseumScene, ui.scenePage)
	}
}

// startSceneReplay 从鉴赏室启动一段场景回放。
//
// 回放结束后要回到原页面（场景页或播放列表），所以先记下返回页再关界面。
// 鉴赏室是从主界面进来的，titleMode 还是 true；回放脚本不能被 titleMode
// 挡住，否则 startReplay 后第一帧就返回 AwaitTitle。
func (s *Session) startSceneReplay(label string, returnKind screenKind, returnPage int) bool {
	if label == "" {
		return false
	}
	s.replayReturn = returnKind
	s.replayPage = returnPage
	s.replayTitleMode = s.titleMode
	s.titleMode = false
	s.closeSystemUI()
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.gameState.SetMessageWindowVisible(true)
	if !s.startReplay(label) {
		core.Logf("ui: scene '%s' replay label not found", label)
		s.replayReturn = uiNone
		// 界面已经被关掉了，必须原样打开回去；否则脚本状态停在标题/上一句，
		// 下一次点击会直接跑回主界面。
		s.titleMode = s.replayTitleMode
		// CHART 不是普通 status 画面：chart.db/页码要重新挂上，
		// 直接 beginSystemUI 会得到一张没有地图的空状态页。
		if returnKind == uiStatus && s.openStatusScreen() {
			return false
		}
		s.beginSystemUI(returnKind)
		if s.ui != nil && returnKind == uiMuseumScene {
			s.ui.scenePage = returnPage
		}
		return false
	}
	s.lastStep = s.finishRun(s.run(commandBudget))
	return true
}

// ---------------------------------------------------------------- 场景播放列表

// drawScenePlaylist 画 [scene playlist] 子画面。
func (s *Session) drawScenePlaylist(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	playlist := s.config.Museum.Scene.Playlist
	s.drawImageAt(canvas, playlist.Base, 0, 0)

	pages := s.scenePlaylistPages()
	ui.playlistPage = clampInt(ui.playlistPage, 0, pages-1)
	for index, button := range playlist.PageButtons {
		if index >= pages {
			break
		}
		frame := 0
		if index == ui.playlistPage {
			frame = 2
		} else if ui.hover.kind == hitPlaylistPage && ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}

	columns := max(1, playlist.Columns)
	slotMax := s.scenePlaylistSlotMax()
	for slot := 0; slot < slotMax; slot++ {
		x := playlist.ThumbX + (slot%columns)*playlist.ThumbW
		y := playlist.ThumbY + (slot/columns)*playlist.ThumbH
		label := s.scenePlaylistSlot(ui.playlistPage, slot)
		if label == "" {
			canvas.Fill(x+4, y+3, thumbWidth, thumbHeight, 18, 16, 24, 255)
			continue
		}
		if scene, ok := s.sceneByLabel(label); ok {
			if thumb := s.thumbnailCanvas(scene.Images); thumb != nil {
				canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, x+4, y+3, 255)
			}
		}
		if slot == ui.playlistSelected || (ui.hover.kind == hitPlaylistSlot && ui.hover.index == slot) {
			s.drawThumbFrame(canvas, x, y)
		}
	}

	// 滚动条：底图是一根轨道，用一条 3px 的白线标出当前页位置。
	bar := playlist.ScrollBar
	s.drawFrameButton(canvas, bar, 0)
	if image := s.composer.Load(bar.Image); image != nil && pages > 1 {
		fraction := float64(ui.playlistPage) / float64(pages-1)
		height := image.Height / max(1, bar.Frames)
		y := bar.Y + int(fraction*float64(max(0, height-24)))
		canvas.Fill(bar.X+2, y, 3, 24, 255, 255, 255, 220)
	}

	playFrame := 0
	if ui.hover.kind == hitPlaylistPlay {
		playFrame = 1
	}
	closeFrame := 0
	if ui.hover.kind == hitPlaylistClose {
		closeFrame = 1
	}
	s.drawFrameButton(canvas, playlist.Play, playFrame)
	s.drawFrameButton(canvas, playlist.Close, closeFrame)
}

// hitScenePlaylist 命中播放列表的页签、槽位、播放/关闭。
func (s *Session) hitScenePlaylist(x, y int) uiHit {
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	playlist := s.config.Museum.Scene.Playlist
	if hitButton(s, playlist.Close, x, y) {
		return uiHit{kind: hitPlaylistClose}
	}
	if hitButton(s, playlist.Play, x, y) {
		return uiHit{kind: hitPlaylistPlay}
	}
	pages := s.scenePlaylistPages()
	for index, button := range playlist.PageButtons {
		if index >= pages {
			break
		}
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitPlaylistPage, index: index}
		}
	}
	columns := max(1, playlist.Columns)
	slotMax := s.scenePlaylistSlotMax()
	for slot := 0; slot < slotMax; slot++ {
		x0 := playlist.ThumbX + (slot%columns)*playlist.ThumbW
		y0 := playlist.ThumbY + (slot/columns)*playlist.ThumbH
		if x >= x0 && y >= y0 && x < x0+playlist.ThumbW && y < y0+playlist.ThumbH {
			return uiHit{kind: hitPlaylistSlot, index: slot}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateScenePlaylist 处理播放列表上的点击。
func (s *Session) activateScenePlaylist(hit uiHit) {
	ui := s.ui
	if ui == nil {
		return
	}
	switch hit.kind {
	case hitPlaylistClose:
		ui.kind = uiMuseumScene
		ui.playlistSelected = -1
	case hitPlaylistPage:
		ui.playlistPage = hit.index
		ui.playlistSelected = -1
	case hitPlaylistSlot:
		ui.playlistSelected = hit.index
	case hitPlaylistPlay:
		slot := ui.playlistSelected
		if slot < 0 {
			slot = 0
		}
		label := s.scenePlaylistSlot(ui.playlistPage, slot)
		if label == "" {
			return
		}
		// 旧版播放列表存的是 label_01_001；统一换成真正的回放标签。
		if scene, ok := s.sceneByLabel(label); ok && scene.SceneId != "" {
			label = scene.SceneId
		}
		s.startSceneReplay(label, uiScenePlaylist, ui.playlistPage)
	}
	s.composer.Invalidate()
}

func (s *Session) activateMuseumMusic(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil {
		return
	}
	if s.config.Museum.Gakuen2 {
		s.activateGK2Music(hit, x)
		return
	}
	page := s.config.Museum.Music
	// 只有按在音量轨道上才保持拖动标记；其它按钮会把它关掉，
	// 避免在曲目列表上滑动时误调 BGM 音量。
	ui.musicVolumeDrag = hit.kind == hitMusicVolume
	switch hit.kind {
	case hitMusicTrack:
		if hit.index >= 0 && hit.index < len(page.Tracks) {
			ui.musicTrack = hit.index
			s.playMuseumTrack(hit.index)
		}
	case hitMusicPlay:
		s.playMuseumTrack(ui.musicTrack)
	case hitMusicStop:
		s.gameState.ClearBgm()
		s.musicUpdate()
	case hitMusicPrev:
		if len(page.Tracks) > 0 {
			ui.musicTrack = (ui.musicTrack - 1 + len(page.Tracks)) % len(page.Tracks)
			s.playMuseumTrack(ui.musicTrack)
		}
	case hitMusicNext:
		if len(page.Tracks) > 0 {
			ui.musicTrack = (ui.musicTrack + 1) % len(page.Tracks)
			s.playMuseumTrack(ui.musicTrack)
		}
	case hitMusicMin:
		s.setMuseumBgmVolume(0)
	case hitMusicMax:
		s.setMuseumBgmVolume(1)
	case hitMusicVolume:
		s.setMuseumBgmVolumeAt(page, x, y)
	}
}

// setMuseumBgmVolumeAt 按音量轨道的点击位置设置 BGM 音量。
//
// KD 是竖向滑块：最大音量在顶端、静音在底端，fraction = 1 - (y-top)/track；
// AFDVD 是底部横向电平条（VolumeFill），必须按 x 取比例，否则点条上的
// 任意位置都只会根据 y（几乎不变）算出同一个音量。
func (s *Session) setMuseumBgmVolumeAt(page config.MuseumMusicPage, x, y int) {
	if page.VolumeFill {
		left := page.VolumeRegion[0]
		width := page.VolumeRegion[2]
		if width <= 0 {
			left = page.Volume.X
			if image := s.buttonImage(page.Volume); image != nil {
				width = image.Width
			}
		}
		if width <= 0 {
			return
		}
		fraction := float64(x-left) / float64(width)
		s.setMuseumBgmVolume(fraction)
		return
	}
	top := page.VolumeRegion[1]
	if top == 0 {
		top = page.Volume.Y
	}
	track := page.VolumeTrackHeight
	if track <= 0 {
		if image := s.buttonImage(page.Volume); image != nil {
			track = page.VolumeRegion[3] - image.Height/max(1, page.Volume.Frames)
		}
	}
	if track <= 0 {
		return
	}
	fraction := 1 - float64(y-top)/float64(track)
	s.setMuseumBgmVolume(fraction)
}

func (s *Session) setMuseumBgmVolume(volume float64) {
	s.settings.BgmVolume = clampVolumeSetting(volume)
	s.saveGameSettings()
	s.refreshAudioVolumes()
	s.invalidateUI()
}

func (s *Session) playMuseumTrack(index int) {
	page := s.config.Museum.Music
	if index < 0 || index >= len(page.Names) || len(page.Names[index]) == 0 {
		return
	}
	s.gameState.SetBgm(page.Names[index])
	s.musicUpdate()
}

func (s *Session) musicUpdate() {
	s.pollAudio()
	if s.ui == nil {
		return
	}
	_, playing := s.gameState.Bgm()
	s.ui.musicPlaying = playing
}

func (s *Session) activateCGViewer(hit uiHit) {
	ui := s.ui
	if ui == nil {
		return
	}
	switch hit.kind {
	case hitCGExit:
		ui.kind = uiMuseumCG
		ui.cgCanvas = nil
		// Exit 回缩略图时重置缩放：下次打开查看器从整屏开始。
		s.resetCGZoom()
		return
	case hitCGConfig:
		// KD 底栏的 CFG：打开原版设置画面，关闭后回到同一张 CG。
		s.openCGViewerConfig()
		return
	case hitCGPrev:
		s.stepCGViewer(-1)
	case hitCGNext:
		s.stepCGViewer(1)
	case hitCGCutin:
		// 切入/头发/水汽三个开关：原引擎在 CG 查看器里改的是和剧情共用的
		// 演出设置，所以这里改设置本身，而不是只改查看器里的临时状态。
		s.settings.Cutin = !s.settings.Cutin
		ui.cgCutin = s.settings.Cutin
		s.saveGameSettings()
		s.invalidateUI()
	case hitCGHair:
		s.settings.UnderHair = !s.settings.UnderHair
		ui.cgHair = s.settings.UnderHair
		s.saveGameSettings()
		s.invalidateUI()
	case hitCGSteam:
		s.settings.Steam = !s.settings.Steam
		ui.cgSteam = s.settings.Steam
		s.saveGameSettings()
		s.invalidateUI()
	case hitCGAhe:
		s.settings.AheOff = !s.settings.AheOff
		s.saveGameSettings()
		s.applyDisplaySettings()
		// アヘ顔OFF 换的是素材名，已经合成好的 canvas 不会自己变；
		// 用同样的参数再合成一次，玩家立刻能看到效果。
		ui.cgCanvas = s.renderCGEntry(ui.cgGroup, ui.cgEntry, ui.cgVariant)
		s.invalidateUI()
	case hitCGBGVVolume:
		// 音量条：按点击位置取比例，存进设置里（0..1），
		// 这样剧情里播放的 BGV 也跟着变。
		s.setCGBGVVolume(ui.cgPointerX)
	case hitCGBGVDetail:
		ui.cgBGVolumeShown = !ui.cgBGVolumeShown
		s.invalidateUI()
	}
}

// setCGBGVVolume 按点击位置设置 BGV 音量。
//
// region 是配置里的音量条区域（x、y、宽、高）；值按千分比（0..1000）算，
// 写进设置时再除回 0..1。音量存在设置里而不是查看器的临时状态，
// 是因为它和剧情里的 BGV 通道音量是同一个值。
func (s *Session) setCGBGVVolume(x int) {
	region := s.config.Museum.Viewer.BGVVolumeRegion
	if region[2] <= 0 {
		return
	}
	value := cgBGVVolumeValue(region, x)
	s.settings.BgvVolume = clampVolumeSetting(float64(value) / 1000)
	s.saveGameSettings()
	s.refreshAudioVolumes()
	s.invalidateUI()
}

// cgBGVVolumeValue 按点击位置换算 BGV 音量，返回千分比（0..1000）。
func cgBGVVolumeValue(region [4]int, x int) int {
	if region[2] <= 0 {
		return 0
	}
	return clampInt((x-region[0])*1000/region[2], 0, 1000)
}

// invalidateUI 请求重画系统界面。
func (s *Session) invalidateUI() {
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// stepCGViewer 在"同一格的变体"之间切换；走到头就跨到相邻格。
// 原引擎的箭头语义是浏览整本画册，这样玩家不会在只有一张图的格上卡住。
func (s *Session) stepCGViewer(delta int) {
	ui := s.ui
	catalog := s.ensureCg()
	if ui == nil || catalog == nil || len(catalog.Groups) == 0 {
		return
	}
	group := clampInt(ui.cgGroup, 0, len(catalog.Groups)-1)
	entries := catalog.Groups[group].Entries
	if len(entries) == 0 {
		return
	}
	entry := clampInt(ui.cgEntry, 0, len(entries)-1)
	variant := ui.cgVariant + delta
	if variant >= 0 && variant < len(entries[entry].Variants) {
		ui.cgVariant = variant
	} else {
		entry += delta
		if entry < 0 {
			group--
			if group < 0 {
				group = len(catalog.Groups) - 1
			}
			entries = catalog.Groups[group].Entries
			entry = len(entries) - 1
		} else if entry >= len(entries) {
			group = (group + 1) % len(catalog.Groups)
			entries = catalog.Groups[group].Entries
			entry = 0
		}
		if len(entries) == 0 {
			return
		}
		entry = clampInt(entry, 0, len(entries)-1)
		ui.cgGroup = group
		ui.cgEntry = entry
		ui.cgVariant = 0
	}
	ui.cgCanvas = s.renderCGEntry(ui.cgGroup, ui.cgEntry, ui.cgVariant)
	// 换差分/换格后重置缩放：差分切换时保持上一张的 zoom 会让人
	// 误以为是同一张图在变，任务要求换差分时重置为 1。
	s.resetCGZoom()
}

func (s *Session) renderCGEntry(group, entry, variant int) *gfx.BgraCanvas {
	catalog := s.ensureCg()
	if catalog == nil || group < 0 || group >= len(catalog.Groups) {
		return nil
	}
	entries := catalog.Groups[group].Entries
	if entry < 0 || entry >= len(entries) || len(entries[entry].Variants) == 0 {
		return nil
	}
	variants := entries[entry].Variants
	variant = clampInt(variant, 0, len(variants)-1)
	images := variants[variant].Images
	if len(images) == 0 {
		return nil
	}
	canvas := gfx.NewBgraCanvas(s.width, s.height)
	drawn := false
	for _, name := range images {
		image := s.composer.Load(name)
		if image == nil {
			continue
		}
		// CG 由底图 + 表情/身体部件合成，部件的坐标是屏幕绝对坐标；
		// 直接 Draw(image, 0, 0) 会把脸/手画到左上角。
		if s.drawLayerScaled(canvas, image, 1, 1) {
			drawn = true
		}
	}
	if !drawn {
		return nil
	}
	return canvas
}

// ---------------------------------------------------------------- 数据

func entryThumbImages(entry data.CgEntry) []string {
	if len(entry.Variants) == 0 {
		return nil
	}
	return entry.Variants[0].Images
}

func (s *Session) cgEntryUnlocked(entry data.CgEntry) bool {
	for _, variant := range entry.Variants {
		for _, image := range variant.Images {
			if s.CgUnlockedByName(image) {
				return true
			}
		}
	}
	return false
}

func (s *Session) cgUnlockPercent() int {
	catalog := s.ensureCg()
	if catalog == nil {
		return 0
	}
	total, unlocked := 0, 0
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			total++
			if s.cgEntryUnlocked(entry) {
				unlocked++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return unlocked * 100 / total
}

func (s *Session) sceneUnlockPercent() int {
	catalog := s.ensureScenes()
	if catalog == nil || len(catalog.Scenes) == 0 {
		return 0
	}
	unlocked := 0
	for index := range catalog.Scenes {
		if s.sceneUnlockedAt(index) {
			unlocked++
		}
	}
	return unlocked * 100 / len(catalog.Scenes)
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
