package session

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/core"
)

// systemui_render.go 负责把系统界面画到引擎画布上。
//
// 所有素材都走 SceneComposer 的 ImageCache：同一张底图/按钮图在剧情里可能已经
// 解码过，复用缓存避免切界面时重复解 3MB 的整屏图。绘制本身不做缩放，
// 因为配置坐标就是游戏坐标系；只有缩略图需要缩放。

const (
	thumbWidth  = 126
	thumbHeight = 96
)

// drawSystemOverlays 在剧情帧画完后叠加系统界面。
//
// 返回是否画了东西：调用方据此强制把这一帧交给宿主。系统界面是覆盖层，
// 底下的 composer.Compose 可能因为"没有变化"返回 false，但界面刚打开/悬停
// 变化时必须重传，否则宿主看到的是上一帧。
func (s *Session) drawSystemOverlays(canvas *gfx.BgraCanvas) bool {
	if canvas == nil {
		return false
	}
	if s.customMapHelp && s.config != nil && len(s.config.CustomMap.HelpImage) > 0 {
		// SL 的工作画面 HELP：整屏说明图盖住底下的面板和按钮。
		// 任意点击会由 pointerCustomUI 关闭。
		if s.drawImageAt(canvas, s.config.CustomMap.HelpImage, 0, 0) {
			return true
		}
	}
	if s.slgState != nil {
		if s.drawSLGState(canvas) {
			return true
		}
	}
	if s.ui == nil && s.slgVM != nil && s.gameState != nil && s.gameState.Selecting() &&
		s.mapSource == "custom" && len(s.customWindows) > 0 {
		// [custom info windows] 是原引擎叠在自定义地图右侧的角色信息窗。
		// update 先跑（闪烁图标），draw 再画；两边共用同一张画布作为 store。
		for _, name := range s.customWindows {
			s.slgVM.UpdateWindow(name, canvas)
			s.slgVM.DrawWindow(name, canvas, 0, 0)
		}
		return true
	}
	if s.ui != nil {
		s.drawSystemUI(canvas)
		return true
	}
	// mros 等作品的地图把 [game menu] 当成常驻底部工具条（原版地图截图里
	// SAVE/LOAD/CONFIG/LOG 一排一直都在）。地图选择态没有对白窗口，这里
	// 直接按配置画出底图和按钮；对话框模式下仍由 MessageWindowRenderer
	// 负责，避免同一排按钮画两遍。
	if s.customMapToolbarVisible() {
		s.drawGameMenu(canvas)
		return true
	}
	// 引擎对话框模式下底部工具条由 SceneComposer 的 MessageWindowRenderer
	// 常驻绘制，不能再叠一份 [game menu]，否则按钮会画两遍。
	if s.menuVisible && !s.gameState.ShowMessageFrame() {
		s.drawGameMenu(canvas)
		return true
	}
	return false
}

// customMapToolbarVisible 判断当前是否“脚本自绘地图”主画面。
//
// 事件内的三选一也用 SELECT_TAIL "custom map"，但它铺的是 select_* 选项条，
// 原版没有底部 SAVE/LOAD 工具条。地图卡则用 map_* 或（牝教師4）icon_* 前缀，
// 这些屏幕显示 [game menu]，避免工具条压住选择界面。
func (s *Session) customMapToolbarVisible() bool {
	if s == nil || s.gameState == nil || !s.gameState.Selecting() || s.mapSource != "custom" {
		return false
	}
	if s.config == nil || s.config.GameMenu.Empty() {
		return false
	}
	for _, button := range s.gameState.Buttons() {
		name := strings.ToLower(button.Image)
		// 牝教師4 的地图卡是 icon_chN_EV01 / icon_skip / icon_gohome，
		// 原版截图里底部整排 FACE…HOLD 一直可见；只认 map_ 会让整条
		// 工具条和上面的按钮一起消失。select_* 三选一仍不显示。
		if strings.HasPrefix(name, "map_") || strings.HasPrefix(name, "icon_") {
			return true
		}
	}
	return false
}

func (s *Session) drawSystemUI(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	// 原引擎的系统界面是"独立画面"，不是半透明覆盖层：底图里刻意留了透明格
	// 用来透出底层，但底层应当是黑色而不是刚刚的标题/剧情。不先铺黑，
	// 标题画面会从存档界面的透明区域透出来（真机表现为"两套 UI 叠在一起"）。
	//
	// Karte 是例外：OH/OHIE 的原版把半透明病历卡叠在当前场景（走廊/地图）上，
	// 周围仍能看到背景。OHIE 的 karte_select_base 是 984x569 的居中合成图，
	// 四边本来就透明；铺黑会变成一块黑边，和原作不符。
	// 満淫電車2 的 dress_base 是透明覆盖层，原版叠在当前剧情画面上；
	// 铺黑会把背景场景整块吃掉，只剩按钮。
	if ui.kind != uiKarte && !(ui.kind == uiDress && s.config != nil && s.config.Dress.Atlas) {
		canvas.Clear(0, 0, 0, 255)
	}
	switch ui.kind {
	case uiDataSave, uiDataLoad:
		s.drawDataScreen(canvas)
	case uiDataItem:
		s.drawItemScreen(canvas)
	case uiMuseum:
		s.drawMuseumMain(canvas)
	case uiMuseumStrip:
		s.drawMuseumStrip(canvas)
	case uiMuseumCG:
		s.drawMuseumCG(canvas)
	case uiMuseumScene:
		s.drawMuseumScene(canvas)
	case uiScenePlaylist:
		s.drawScenePlaylist(canvas)
	case uiMuseumMusic:
		s.drawMuseumMusic(canvas)
	case uiCGViewer:
		s.drawCGViewer(canvas)
	case uiConfig:
		s.drawConfigScreen(canvas)
	case uiLegacyConfig:
		s.drawLegacyConfigScreen(canvas)
	case uiMuseumBGV:
		s.drawBGVMuseum(canvas)
	case uiMuseumBU:
		s.drawBustupMuseum(canvas)
	case uiDress:
		s.drawDressScreen(canvas)
	case uiLog:
		s.drawLogScreen(canvas)
	case uiStatus:
		s.drawStatusScreen(canvas)
	case uiStatusSLG:
		s.drawSLGStatusScreen(canvas)
	case uiStatusAncient:
		s.drawAncientStatusScreen(canvas)
	case uiStatusLegacy:
		s.drawLegacyStatusScreen(canvas)
	case uiKarte:
		s.drawKarteScreen(canvas)
	case uiTarget:
		s.drawTargetSelect(canvas)
	case uiTargetCard:
		s.drawTargetCardSelect(canvas)
	}
}

// ---------------------------------------------------------------- 基础绘制

// drawFrameButton 按状态帧画一个竖排按钮。
//
// frames 来自配置；图片解码器会把整张竖排图当成单帧，所以不能依赖
// image.FrameCount，必须用配置的帧数切。
func (s *Session) drawFrameButton(canvas *gfx.BgraCanvas, button config.UIButton, frame int) {
	if button.Empty() {
		return
	}
	image := s.buttonImage(button)
	if image == nil {
		return
	}
	frames := max(1, button.Frames)
	if button.FixedFrame {
		frame = clampInt(button.Start, 0, frames-1)
	} else {
		frame = clampInt(frame+button.Start, 0, frames-1)
	}
	if button.Atlas {
		// BSXScript 1.0 的精灵图：Clip 是单帧矩形，多帧沿配置给的方向排列。
		// 悬停帧常常在图上另一块（存档页页码），优先用 HoverClip。
		srcX, srcY, width, height := button.Clip[0], button.Clip[1], button.Clip[2], button.Clip[3]
		if frame > 0 && button.HoverClip[2] > 0 && button.HoverClip[3] > 0 {
			srcX, srcY = button.HoverClip[0], button.HoverClip[1]
			width, height = button.HoverClip[2], button.HoverClip[3]
		} else if button.AtlasAxis == 1 {
			srcX += frame * width
		} else {
			srcY += frame * height
		}
		if width <= 0 || height <= 0 ||
			srcX < 0 || srcY < 0 || srcX+width > image.Width || srcY+height > image.Height {
			return
		}
		canvas.DrawRegion(image, srcX, srcY, width, height, button.X, button.Y)
		return
	}
	height := image.Height / frames
	if height <= 0 {
		return
	}
	if button.Clip[2] > 0 && button.Clip[3] > 0 {
		// Clip 只截取图片的一块（save_load 的 SAVE/LOAD 同图）。帧仍按竖排高度切，
		// 所以 Y 要叠加 frame*height，与整图按钮的帧定位保持一致。
		srcX, srcY := button.Clip[0], button.Clip[1]+frame*height
		if srcX < 0 || srcY < 0 || srcX+button.Clip[2] > image.Width || srcY+button.Clip[3] > image.Height {
			return
		}
		canvas.DrawRegion(image, srcX, srcY, button.Clip[2], button.Clip[3], button.X, button.Y)
		return
	}
	canvas.DrawRegion(image, 0, frame*height, image.Width, height, button.X, button.Y)
}

// buttonImage 取按钮真正要画的那张图。
//
// 普通按钮就是资源图本身；`image#N` 的复合图按钮取第 N 个部件。
// 部件自带屏幕坐标（OffsetX/Y），但 UIButton 的 X/Y 才是布局位置，
// 所以这里只取像素，不把部件的偏移带出去。
func (s *Session) buttonImage(button config.UIButton) *gfx.BsgImage {
	if s.composer == nil || button.Empty() {
		return nil
	}
	image := s.composer.Load(button.Image)
	if image == nil {
		return nil
	}
	if button.Part > 0 && button.Part <= len(image.Parts) {
		return image.Parts[button.Part-1]
	}
	return image
}

func (s *Session) drawImageAt(canvas *gfx.BgraCanvas, name string, x, y int) bool {
	if len(name) == 0 {
		return false
	}
	image := s.composer.Load(name)
	if image == nil {
		return false
	}
	canvas.Draw(image, x, y)
	return true
}

// drawDigitStrip 用预渲染数字条画一个整数。
//
// 原引擎不加载字体，0-9 是竖排图；这里从低位往高位画，space 为负表示
// 高位在左边（配置里的 day no offset / percent number space 都是负数）。
func (s *Session) drawDigitStrip(canvas *gfx.BgraCanvas, strip config.UIButton, value, x, y, space int) {
	if strip.Empty() || value < 0 {
		return
	}
	image := s.composer.Load(strip.Image)
	if image == nil {
		return
	}
	frames := strip.Frames
	if frames <= 0 {
		frames = 10
	}
	frameHeight := image.Height / frames
	frameWidth := image.Width
	if strip.Atlas && strip.Clip[2] > 0 && strip.Clip[3] > 0 {
		frameHeight = strip.Clip[3]
		frameWidth = strip.Clip[2]
	}
	if frameHeight <= 0 || frameWidth <= 0 {
		return
	}
	if space == 0 {
		space = -frameWidth
	}
	// 至少画一位（0 也要显示），并且给一个上限防止损坏的配置把死循环带进来。
	for digits := 0; digits < 10; digits++ {
		digit := value % 10
		value /= 10
		frame := clampInt(digit, 0, frames-1)
		if strip.Atlas {
			srcX, srcY, width, height := strip.Clip[0], strip.Clip[1], strip.Clip[2], strip.Clip[3]
			if strip.AtlasAxis == 1 {
				srcX += frame * width
			} else {
				srcY += frame * height
			}
			canvas.DrawRegion(image, srcX, srcY, width, height, x, y)
		} else {
			canvas.DrawRegion(image, 0, frame*frameHeight, image.Width, frameHeight, x, y)
		}
		x += space
		if value == 0 {
			break
		}
	}
}

// drawLayerScaled 按原引擎的屏幕坐标把一个图层缩放绘制到目标画布。
//
// BSG 的 BSS-Composition 不是一张整图：每个部件都带绝对屏幕坐标
// （例如脸在 637,81），必须逐个按 part.OffsetX/OffsetY 绘制；普通 BSG
// 则使用 image.OffsetX/OffsetY。旧实现把整张图拉伸到 (0,0)，对合成图来说
// 脸会被拉满整个格子、多角色场景叠成一团，封面看起来就像换成了别人。
//
// scaleX/scaleY 是"屏幕坐标 → 目标画布"的比例；查看器传 1，缩略图传
// thumb/屏幕尺寸。返回是否真的画上了至少一个部件。
func (s *Session) drawLayerScaled(dst *gfx.BgraCanvas, image *gfx.BsgImage, scaleX, scaleY float64) bool {
	if dst == nil || image == nil {
		return false
	}
	if len(image.Parts) > 0 {
		drawn := false
		for _, part := range image.Parts {
			if part == nil {
				continue
			}
			x := int(math.Round(float64(part.OffsetX) * scaleX))
			y := int(math.Round(float64(part.OffsetY) * scaleY))
			width := int(math.Round(float64(part.Width) * scaleX))
			height := int(math.Round(float64(part.Height) * scaleY))
			if width <= 0 || height <= 0 {
				continue
			}
			dst.BlitScaled(part, x, y, width, height, 255)
			drawn = true
		}
		return drawn
	}
	x := int(math.Round(float64(image.OffsetX) * scaleX))
	y := int(math.Round(float64(image.OffsetY) * scaleY))
	width := int(math.Round(float64(image.Width) * scaleX))
	height := int(math.Round(float64(image.Height) * scaleY))
	if width <= 0 || height <= 0 {
		return false
	}
	dst.BlitScaled(image, x, y, width, height, 255)
	return true
}

// drawLayerAt 在目标画布的 (x,y) 处画一张图，正确处理 BSS-Composition：
// 合成图的部件坐标是相对整张图的偏移，必须逐个叠加；普通图则用自己的 OffsetX/Y。
// 状態画面的角色图/事件图都是这种"小图 + 部件偏移"，直接 Draw(image,x,y) 会漏掉部件位置。
func (s *Session) drawLayerAt(canvas *gfx.BgraCanvas, name string, x, y int) bool {
	if canvas == nil || len(name) == 0 {
		return false
	}
	image := s.composer.Load(name)
	if image == nil {
		return false
	}
	if len(image.Parts) > 0 {
		drawn := false
		for _, part := range image.Parts {
			if part == nil {
				continue
			}
			canvas.Draw(part, x+part.OffsetX, y+part.OffsetY)
			drawn = true
		}
		return drawn
	}
	canvas.Draw(image, x+image.OffsetX, y+image.OffsetY)
	return true
}

// thumbnailCanvas 返回一组图层缩放到缩略图尺寸后的画布。
//
// 缓存是必要的：鉴赏室一页 31 格，每格 2~3 张 1024x768 图层，
// 不缓存的话每帧要缩放近 300MB 像素。缓存键用图层名组合，
// 同一张 CG 的多个变体自然分开。
func (s *Session) thumbnailCanvas(images []string) *gfx.BgraCanvas {
	return s.thumbnailCanvasSized(images, thumbWidth, thumbHeight)
}

// thumbnailCanvasSized 按配置格子尺寸生成缩略图。
//
// 鉴赏室的格子尺寸来自 thumbN pos 的间距（224x123 / 207x127），不是固定
// 的 126x96；缓存键必须带尺寸，否则同一张 CG 在两个尺寸间来回切换会
// 拿到上一尺寸的画布。
func (s *Session) thumbnailCanvasSized(images []string, width, height int) *gfx.BgraCanvas {
	if len(images) == 0 || width <= 0 || height <= 0 {
		return nil
	}
	key := thumbKey(images) + "@" + strconv.Itoa(width) + "x" + strconv.Itoa(height)
	if entry, ok := s.thumbs[key]; ok {
		entry.used++
		return entry.canvas
	}
	canvas := gfx.NewBgraCanvas(width, height)
	scaleX, scaleY := 1.0, 1.0
	if s.width > 0 {
		scaleX = float64(width) / float64(s.width)
	}
	if s.height > 0 {
		scaleY = float64(height) / float64(s.height)
	}
	drawn := false
	for _, name := range images {
		image := s.composer.Load(name)
		if image == nil {
			continue
		}
		if s.drawLayerScaled(canvas, image, scaleX, scaleY) {
			drawn = true
		}
	}
	if !drawn {
		return nil
	}
	if s.thumbs == nil {
		s.thumbs = map[string]*thumbCacheEntry{}
	}
	if len(s.thumbs) >= 320 {
		s.pruneThumbnails()
	}
	s.thumbs[key] = &thumbCacheEntry{canvas: canvas, used: 1}
	return canvas
}

func (s *Session) pruneThumbnails() {
	// 简单清空而不是做完整 LRU：缩略图重新生成只是慢一点，
	// 而维护一张精确的 LRU 会增加状态和锁的复杂度。
	clear(s.thumbs)
}

func thumbKey(images []string) string {
	key := ""
	for _, name := range images {
		key += name
		key += "|"
	}
	return key
}

// ---------------------------------------------------------------- 游戏内菜单条

func (s *Session) drawGameMenu(canvas *gfx.BgraCanvas) {
	layout := s.config.GameMenu
	if layout.Empty() {
		return
	}
	s.drawImageAt(canvas, layout.Base, layout.BaseX, layout.BaseY)
	for index, button := range layout.Buttons {
		if !supportedMenuKeys[button.Key] {
			continue
		}
		frame := 0
		if s.menuHover == index {
			frame = 1
		}
		// VIEW/STRIP 是状态按钮：4 帧约定为 常态/悬停/启用/启用+悬停。
		// 画成第 1 帧会让玩家看不出当前是否已经切过视点或脱过衣。
		if s.gameMenuToggleOn(button.Key) {
			frame += 2
		}
		s.drawFrameButton(canvas, button.UIButton, frame)
	}
	// 开关指示灯叠在按钮图之后：按钮图只有底色，状态由 switch_lamp 表示。
	if !layout.CutInIcon.Empty() && s.settings.Cutin {
		s.drawImageAt(canvas, layout.CutInIcon.Image,
			layout.BaseX+layout.CutInIcon.X, layout.BaseY+layout.CutInIcon.Y)
	}
	if !layout.HoldIcon.Empty() && s.settings.Hold {
		s.drawImageAt(canvas, layout.HoldIcon.Image,
			layout.BaseX+layout.HoldIcon.X, layout.BaseY+layout.HoldIcon.Y)
	}
}

// ---------------------------------------------------------------- 存档 / 读档

func (s *Session) drawDataScreen(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	layout := s.config.DataScreen
	// Backdrop 垫在整屏底图下面：零值/空字符串不画，保持旧行为。
	// 有值时在 (0,0) 铺它，SaveBase/LoadBase（常为半透明或非整屏）覆盖其上。
	s.drawImageAt(canvas, layout.Backdrop, 0, 0)
	base := layout.LoadBase
	if ui.kind == uiDataSave {
		base = layout.SaveBase
	}
	s.drawImageAt(canvas, base, 0, 0)
	// 左上角大号模式名（放課後2 的 cfg_title）：SAVE/LOAD/CONFIG 三帧。
	if !layout.TitleLabel.Empty() {
		frame := 0
		if ui.kind == uiDataLoad {
			frame = 1
		}
		s.drawFrameButton(canvas, layout.TitleLabel, frame)
	}
	if !layout.PageSelectFrame.Empty() {
		s.drawFrameButton(canvas, layout.PageSelectFrame, 0)
	}
	s.ensureSlotPage()
	s.ensureSlotOrder()
	for index, slot := range layout.Slots {
		slotIndex := s.visibleSlotIndex(ui.page, index)
		if slotIndex >= save.GridSlotCount {
			continue
		}
		if layout.SlotFrame == "dm_slot" && !layout.SlotFrameGeneric {
			s.drawGakuen2DataSlot(canvas, ui, layout, index, slot, slotIndex)
			continue
		}
		if layout.SlotFrame != "" {
			s.drawAncientDataSlot(canvas, ui, layout, index, slot, slotIndex)
			continue
		}
		// 通用（无 SlotFrame）槽位：缩略图位置可由 SlotThumb 配置覆盖；
		// SlotThumb 为零值时保持旧行为（槽位左上 +1/+1）。
		thumbX, thumbY := slot.X+1, slot.Y+1
		if layout.SlotThumb[2] > 0 && layout.SlotThumb[3] > 0 {
			thumbX, thumbY = slot.X+layout.SlotThumb[0], slot.Y+layout.SlotThumb[1]
		}
		thumb, hasThumb := ui.slotThumbs[slotIndex]
		if hasThumb {
			canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, thumbX, thumbY, 255)
		}
		// 精灵图世代的空格子：自动存档格画 AUTO ONLY 占位图，普通空格
		// 画 NO DATA 小标签。两者都只有源矩形，没有独立图片名。
		if !hasThumb {
			if layout.AutoSlotGrid && ui.page == 0 && index == layout.AutoSlotIndex {
				if layout.AutoOnlyButton.Atlas {
					button := layout.AutoOnlyButton
					button.X, button.Y = slot.X, slot.Y
					s.drawFrameButton(canvas, button, 0)
				}
			} else if layout.NoDataButton.Atlas {
				button := layout.NoDataButton
				button.X = slot.X + 8
				button.Y = slot.Y + max(0, slotHeight(s)-layout.NoDataButton.Clip[3]-6)
				s.drawFrameButton(canvas, button, 0)
			}
		}
		// 配置了 SlotTextX/Y 时在槽位内画槽号/日期/摘要文字（引擎字体）。
		// 零值（SlotTextX==0 && SlotTextY==0）保持旧行为：不画文字；悬停
		// 选框逻辑在下方，与文字叠加互不干扰。
		if layout.SlotTextX != 0 || layout.SlotTextY != 0 {
			if s.textFont != nil {
				textX := slot.X + layout.SlotTextX
				textY := slot.Y + layout.SlotTextY
				lineGap := layout.SlotTextLineGap
				if lineGap <= 0 {
					lineGap = 24
				}
				color := layout.SlotTextColor
				if color == 0 {
					color = 0xffffffff
				}
				no := slotIndex + max(0, layout.NoBegin)
				label := strconv.Itoa(no) + "."
				if layout.SlotNumberPrefix != "" || layout.SlotNumberWidth > 0 {
					label = layout.SlotNumberPrefix + fmt.Sprintf("%0*d", max(1, layout.SlotNumberWidth), no)
				}
				number := s.textFont.Render(label, 18, 1, 1, lineGap, 160, 1, color)
				rendering.DrawTextImage(canvas, number, textX, textY)
				stored := ui.slotSaved[slotIndex]
				if stored != nil && !stored.SavedAt.IsZero() {
					stamp := stored.SavedAt.Format("2006/01/02 15:04")
					image := s.textFont.Render(stamp, 15, 1, 1, lineGap, 200, 1, color)
					rendering.DrawTextImage(canvas, image, textX, textY+lineGap)
				}
				if stored != nil && stored.Summary != "" {
					image := s.textFont.Render(stored.Summary, 14, 1, 1, 20, 200, 2, color)
					rendering.DrawTextImage(canvas, image, textX, textY+lineGap*2)
				}
			}
		}
		if ui.hover.kind == hitSlot && ui.hover.index == index {
			// select frame 的坐标是 -1,-1：原引擎以槽位左上角为基准贴边框，
			// 所以这里直接以槽位坐标偏移 -2/-1，不能用配置里的绝对坐标。
			if !layout.SelectFrame.Empty() {
				button := layout.SelectFrame
				button.X, button.Y = slot.X-3, slot.Y-3
				s.drawFrameButton(canvas, button, 0)
			}
		}
	}
	for index, button := range layout.PageButtons {
		frame := 0
		if ui.hover.kind == hitPage && ui.hover.index == index {
			frame = 1
		}
		if index == ui.page {
			frame = 2
		}
		s.drawFrameButton(canvas, button, frame)
	}
	// 旧式作品把页码数字做成竖排三态图（dm_page）：每页占 1 帧，
	// 常态/当前/悬停三组各 11 帧。底图只有刻度线，数字必须由引擎画。
	// GK 的页码是"当前页"两位数（dm_pagenum 0..9 各一帧），不是逐页按钮。
	// 十位固定画 0（0..9 页），个位画当前页；PageMax 超过 10 时公式仍然成立。
	if !layout.PageNumber.Empty() {
		drawPageDigit(canvas, s, layout.PageNumber, ui.page/10, layout.PageNumberX, layout.PageNumberY)
		drawPageDigit(canvas, s, layout.PageNumber, ui.page%10, layout.PageNumberX+layout.PageNumberGap, layout.PageNumberY)
	}
	if !layout.PageNumbers.Empty() {
		frames := max(1, layout.PageNumbers.Frames)
		// PageNumberStart 让"页码 1"可以取数字条第 1 帧（MK 的 dm_number
		// 是 0..9 十帧）。三组 11 帧的旧条（学園2）起点仍为 0，行为不变。
		start := layout.PageNumberStart
		// 数字条只有"每个页码一帧"（MK 的 dm_number 10 帧）时，没有第二、
		// 第三组状态帧；当前页/悬停仍用同一帧，状态由下面的 dm_pagenum
		// 背景块表达。否则当前页的数字会因为 frame=11+index 越界而消失。
		digitOnly := frames <= 10
		drawDigit := func(frame, x, y int) {
			if frame < 0 || frame >= frames {
				return
			}
			s.drawFrameButton(canvas, config.UIButton{
				Image: layout.PageNumbers.Image, Frames: frames,
				X: x, Y: y, Start: frame, FixedFrame: true,
			}, 0)
		}
		for index, point := range layout.PageNumberPos {
			if digitOnly {
				// 0..9 的十帧数字条没有"10"这一帧；第 10 页要拆成
				// 十位和个位两颗数字。其余页仍画单帧。
				number := index + start
				if number < frames {
					drawDigit(number, point.X, point.Y)
				} else if number < 100 {
					drawDigit(number/10, point.X-6, point.Y)
					drawDigit(number%10, point.X+6, point.Y)
				}
				continue
			}
			frame := index + start
			if ui.hover.kind == hitPage && ui.hover.index == index {
				frame = 22 + index + start
			} else if index == ui.page {
				frame = 11 + index + start
			}
			drawDigit(frame, point.X, point.Y)
		}
	}
	// 排序按钮画在底图上；这里只补一条当前排序的下划线，
	// 让玩家知道"新到旧/旧到新"已经生效。
	if ui.sortMode >= 0 && ui.sortMode < len(layout.SortButtons) {
		point := layout.SortButtons[ui.sortMode]
		canvas.Fill(point.X, point.Y+24, 120, 2, 255, 255, 255, 180)
	}
	// MenuSave/MenuLoad 是存档/读档的左侧菜单覆盖图（201x600），画在底图与
	// 页签/页码等装饰之后、模式页签/按钮之前——与原作“菜单压在装饰上”
	// 的层序一致。零值保持旧行为：不叠这层菜单。
	menuOverlay := layout.MenuSave
	if ui.kind == uiDataLoad {
		menuOverlay = layout.MenuLoad
	}
	if menuOverlay != "" {
		s.drawImageAt(canvas, menuOverlay, 0, 0)
	}
	// MK 的模式页签是白字，底图是白色装饰底：先给三块标签铺一条深色底，
	// 否则 SAVE/LOAD/CONFIG 与背景同色，玩家找不到入口。
	if layout.ModeLabelBackdrop {
		for _, button := range []config.UIButton{layout.SaveMode, layout.LoadMode, layout.Config, layout.Item} {
			if button.Empty() {
				continue
			}
			image := s.buttonImage(button)
			if image == nil {
				continue
			}
			height := image.Height / max(1, button.Frames)
			if height <= 0 {
				continue
			}
			canvas.FillBlend(button.X, button.Y, image.Width, height, 0, 0, 0, 190)
		}
	}
	// 底部"存档/读档"页签：当前模式高亮，另一个可点击切换。
	saveFrame := 0
	if ui.kind == uiDataSave {
		saveFrame = 2
	} else if ui.hover.kind == hitSaveMode {
		saveFrame = 1
	}
	loadFrame := 0
	if ui.kind == uiDataLoad {
		loadFrame = 2
	} else if ui.hover.kind == hitLoadMode {
		loadFrame = 1
	}
	// save_load 这类两帧页签只有常态/激活两帧，没有悬停帧；悬停时按常态画，避免越界取帧。
	if layout.SaveMode.Frames == 2 {
		saveFrame = 0
		if ui.kind == uiDataSave {
			saveFrame = 1
		}
		loadFrame = 0
		if ui.kind == uiDataLoad {
			loadFrame = 1
		}
	}
	s.drawFrameButton(canvas, layout.SaveMode, saveFrame)
	s.drawFrameButton(canvas, layout.LoadMode, loadFrame)
	configFrame := 0
	if ui.hover.kind == hitDataConfig {
		configFrame = 1
	}
	s.drawFrameButton(canvas, layout.Config, configFrame)
	itemFrame := 0
	if ui.hover.kind == hitDataItem {
		itemFrame = 1
	}
	s.drawFrameButton(canvas, layout.Item, itemFrame)
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{{hitBack, layout.Back}, {hitTitle, layout.Title}, {hitExit, layout.Exit}} {
		frame := 0
		if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
	// 学園2 的存档画面右栏是常驻的槽位详情（用 dm_save 底图核对过）：
	// 底图里已经烘焙了「NO DATA.」占位、六名女主角的姓名和六个空条框，
	// 引擎只要在 (620,83) 盖 160x120 缩略图、再把六根攻略度条画进条框。
	// 只有学園2 的存档页带右栏的槽位详情/攻略度条；牝教師2 老版同样用
	// dm_slot 框，但右侧没有这块面板，不加重叠的大缩略图。
	if !ui.confirm && layout.SlotFrame == "dm_slot" && layout.Confirm.MeterImage != "" {
		slot := ui.confirmSlot
		if ui.hover.kind == hitSlot {
			slot = s.visibleSlotIndex(ui.page, ui.hover.index)
		}
		if slot < 0 {
			slot = 0
		}
		if thumb, ok := ui.slotThumbs[slot]; ok {
			canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, 620, 83, 255)
		}
		s.drawSlotViewerMeters(canvas, slot)
	}
	if ui.confirm {
		s.drawConfirm(canvas)
	}
}

// drawGakuen2DataSlot 画学園2 的 9 格存档：dm_slot 三态框 + 缩略图 +
// 空格 NO DATA + 第一格 AUTO 标记。dm_slot 是 170x130，槽位坐标是
// 160x120，所以框要向外扩 5px 才能套住缩略图。
// drawPageDigit 从页码数字条里取指定数字画到 (x,y)。
func drawPageDigit(canvas *gfx.BgraCanvas, s *Session, strip config.UIButton, digit, x, y int) {
	if strip.Empty() || digit < 0 {
		return
	}
	frames := max(1, strip.Frames)
	if digit >= frames {
		digit = frames - 1
	}
	image := s.buttonImage(strip)
	if image == nil {
		return
	}
	cellHeight := image.Height / frames
	if cellHeight <= 0 {
		return
	}
	canvas.DrawRegion(image, 0, digit*cellHeight, image.Width, cellHeight, x, y)
}

func (s *Session) drawGakuen2DataSlot(canvas *gfx.BgraCanvas, ui *systemUI, layout config.DataScreen, index int, slot config.UIPoint, slotIndex int) {
	stored := ui.slotSaved[slotIndex]
	frame := 0
	if stored == nil {
		frame = 2
	}
	if ui.hover.kind == hitSlot && ui.hover.index == index {
		frame = clampInt(layout.SlotFrameHover, 0, max(1, layout.SlotFrameFrames)-1)
	} else if ui.confirm && ui.confirmSlot == slotIndex {
		frame = clampInt(layout.SlotFrameSelected, 0, max(1, layout.SlotFrameFrames)-1)
	}
	// dm_slotbase 是槽位框下面的金色织纹底板；dm_slot 的边框中心透明，
	// 没有缩略图的空格子必须靠它才不是一块黑。
	if layout.SlotBase != "" {
		s.drawImageAt(canvas, layout.SlotBase, slot.X, slot.Y)
	}
	// dm_slot 的边框中心透明，尺寸比缩略图大一圈：按素材实际尺寸居中，
	// 不能写死 -5（那是学園2 170x130 框的偏移；放課後2 是 176x136 → -8）。
	insetX, insetY := 5, 5
	if image := s.composer.Load(layout.SlotFrame); image != nil {
		cellHeight := image.Height / max(1, layout.SlotFrameFrames)
		contentW, contentH := 160, 120
		if layout.SlotThumb[2] > 0 && layout.SlotThumb[3] > 0 {
			contentW, contentH = layout.SlotThumb[2], layout.SlotThumb[3]
		}
		insetX = (image.Width - contentW) / 2
		insetY = (cellHeight - contentH) / 2
	}
	s.drawFrameButton(canvas, config.UIButton{
		Image: layout.SlotFrame, Frames: max(1, layout.SlotFrameFrames),
		X: slot.X - insetX, Y: slot.Y - insetY,
	}, frame)
	if thumb, ok := ui.slotThumbs[slotIndex]; ok {
		canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, slot.X+1, slot.Y+1, 255)
	} else if layout.NoData != "" {
		if image := s.composer.Load(layout.NoData); image != nil {
			x := slot.X + (160-image.Width)/2
			y := slot.Y + (120-image.Height)/2
			canvas.DrawRegion(image, 0, 0, image.Width, image.Height, x, y)
		}
	}
	if layout.AutoSlotIndex >= 0 && index == layout.AutoSlotIndex && layout.AutoOnly != "" {
		if image := s.composer.Load(layout.AutoOnly); image != nil {
			// dm_auto 是纯透明掩码（alpha 全 0），按普通绘制看不见；
			// 有内容时才贴图，否则用引擎字体补一个 AUTO 标签。
			if bounds := image.ContentBounds(); bounds.Width > 0 && bounds.Height > 0 {
				canvas.DrawRegion(image, bounds.X, bounds.Y, bounds.Width, bounds.Height, slot.X+27, slot.Y+3)
			} else if s.textFont != nil {
				label := s.textFont.Render("AUTO", 16, 1, 1, 20, 60, 1, 0xffffffff)
				rendering.DrawTextImage(canvas, label, slot.X+12, slot.Y+4)
			}
		}
	}
}

func (s *Session) drawAncientDataSlot(canvas *gfx.BgraCanvas, ui *systemUI, layout config.DataScreen, index int, slot config.UIPoint, slotIndex int) {
	frameImage := layout.SlotFrame
	frames := max(1, layout.SlotFrameFrames)
	if layout.AutoSlotIndex >= 0 && ui.page == 0 && index == layout.AutoSlotIndex && layout.SlotFrameAuto != "" {
		frameImage = layout.SlotFrameAuto
	}
	stored := ui.slotSaved[slotIndex]
	frame := 0
	if stored == nil && layout.SlotEmptyFrame >= 0 {
		frame = clampInt(layout.SlotEmptyFrame, 0, frames-1)
	}
	if ui.hover.kind == hitSlot && ui.hover.index == index {
		frame = clampInt(layout.SlotFrameHover, 0, frames-1)
	}
	if ui.confirm && ui.confirmSlot == slotIndex {
		frame = clampInt(layout.SlotFrameSelected, 0, frames-1)
	}
	if layout.SlotFrameOpaque {
		if image := s.buttonImage(config.UIButton{Image: frameImage, Frames: frames}); image != nil {
			cell := image.Height / frames
			if cell > 0 {
				canvas.BlitOpaqueRegion(image, 0, frame*cell, image.Width, cell, slot.X, slot.Y)
			}
		}
	} else {
		s.drawFrameButton(canvas, config.UIButton{Image: frameImage, Frames: frames, X: slot.X, Y: slot.Y}, frame)
	}

	thumbX, thumbY := slot.X+layout.SlotThumb[0], slot.Y+layout.SlotThumb[1]
	thumbW, thumbH := layout.SlotThumb[2], layout.SlotThumb[3]
	if thumbW <= 0 || thumbH <= 0 {
		thumbW, thumbH = thumbWidth, thumbHeight
	}
	if thumb, ok := ui.slotThumbs[slotIndex]; ok {
		canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, thumbX, thumbY, 255)
	} else {
		noData := layout.NoData
		if noData == "" {
			noData = "save_nodata"
		}
		if image := s.composer.Load(noData); image != nil {
			// 空格子保留原版的“NO DATA”占位，不要画成纯黑，玩家才知道这里能存。
			x := thumbX + (thumbW-image.Width)/2
			y := thumbY + (thumbH-image.Height)/2
			canvas.DrawRegion(image, 0, 0, image.Width, image.Height, x, y)
		}
	}
	textX := slot.X + layout.SlotTextX
	textY := slot.Y + layout.SlotTextY
	lineGap := layout.SlotTextLineGap
	if lineGap <= 0 {
		lineGap = 30
	}
	color := layout.SlotTextColor
	if color == 0 {
		color = 0xffffffff
	}
	if s.textFont != nil {
		no := slotIndex + max(0, layout.NoBegin)
		label := strconv.Itoa(no) + "."
		if layout.SlotNumberPrefix != "" || layout.SlotNumberWidth > 0 {
			label = layout.SlotNumberPrefix + fmt.Sprintf("%0*d", max(1, layout.SlotNumberWidth), no)
		}
		number := s.textFont.Render(label, 18, 1, 1, lineGap, 160, 1, color)
		rendering.DrawTextImage(canvas, number, textX, textY)
		if stored != nil && !stored.SavedAt.IsZero() {
			stamp := stored.SavedAt.Format("2006/01/02 15:04")
			image := s.textFont.Render(stamp, 15, 1, 1, lineGap, 200, 1, color)
			rendering.DrawTextImage(canvas, image, textX, textY+lineGap)
		}
		if stored != nil && stored.Summary != "" {
			image := s.textFont.Render(stored.Summary, 14, 1, 1, 20, 200, 2, color)
			rendering.DrawTextImage(canvas, image, textX, textY+lineGap*2)
		}
	}
}

func (s *Session) drawConfirm(canvas *gfx.BgraCanvas) {
	layout := s.config.DataScreen.Confirm
	ui := s.ui
	if layout.Base != "" {
		s.drawImageAt(canvas, layout.Base, layout.BaseX, layout.BaseY)
	}
	// resave_ok/resave_cancel 的第 0 帧是白色命中掩码，1 常态、2 悬停。
	okFrame := 1
	if ui.hover.kind == hitConfirmOK {
		okFrame = 2
	}
	cancelFrame := 1
	if ui.hover.kind == hitConfirmCancel {
		cancelFrame = 2
	}
	s.drawFrameButton(canvas, layout.OK, okFrame)
	s.drawFrameButton(canvas, layout.Cancel, cancelFrame)
	s.drawSlotViewerDetail(canvas, ui.confirmSlot)
	s.drawSlotViewerMeters(canvas, ui.confirmSlot)
}

// drawSlotViewerDetail 画存档确认窗口里的槽位详情：
// 槽号、保存日期/时间、缩略图、正文预览、游戏内日期和场景名。
//
// 这些文字在原版里是"图片数字条 + 引擎字体"混排，所以槽号/日期用
// slotviewer_font 的 10 帧字条，场景名/正文用引擎字体。
func (s *Session) drawSlotViewerDetail(canvas *gfx.BgraCanvas, slot int) {
	layout := s.config.DataScreen.Confirm
	if layout.NoHeader.Empty() && layout.Numbers.Empty() && layout.SceneNameSize <= 0 {
		return
	}
	if layout.NoHeader.Image != "" {
		s.drawImageAt(canvas, layout.NoHeader.Image, layout.NoHeader.X, layout.NoHeader.Y)
	}
	if !layout.Numbers.Empty() {
		// 槽号从 1 开始显示。
		s.drawDigitStrip(canvas, layout.Numbers, slot+1, layout.SlotNoX, layout.SlotNoY, layout.SlotNoOffset)
	}
	var saved *save.SaveData
	if s.saves != nil {
		saved = s.saves.Read(slot)
	}
	stamp := time.Now()
	if saved != nil {
		stamp = saved.SavedAt
	}
	if !layout.Numbers.Empty() {
		// yyyy/mm/dd 和 hh:mm:ss：数字字条第 10/11 帧是 '/' 和 ':'。
		s.drawDateDigits(canvas, layout.Numbers, stamp, layout.DateX, layout.DateY, layout.DateOffset)
		s.drawTimeDigits(canvas, layout.Numbers, stamp, layout.TimeX, layout.TimeY, layout.TimeOffset)
	}
	// 游戏内日期（行動開始から X 日目）复用信息窗的取值和字条。
	values := s.informationWindowValues()
	if values.Day >= 0 {
		if !layout.DayStrip.Empty() {
			s.drawDigitStrip(canvas, layout.DayStrip, values.Day, layout.DayStrip.X, layout.DayStrip.Y, layout.DayNoOffset)
		}
		if !layout.DayUnit.Empty() {
			s.drawFrameButton(canvas, layout.DayUnit, 0)
		}
	}
	if saved != nil && s.textFont != nil {
		// 场景标题：布局里给的是右对齐（align=1 是右对齐）。
		if saved.Summary != "" {
			s.drawAlignedText(canvas, saved.Summary, layout.SceneNameX, layout.SceneNameY,
				layout.SceneNameSize, layout.SceneNameColor, layout.SceneNameAlign, 0)
		}
		// 正文预览。
		if saved.Text != "" && layout.TextArea[2] > 0 {
			size := layout.FontSize[0]
			if size <= 0 {
				size = 14
			}
			s.drawAlignedText(canvas, saved.Text, layout.TextArea[0], layout.TextArea[1],
				size, layout.TextColor, 0, layout.TextArea[2])
		}
		// 缩略图：存档里是 BGRA 原始像素。
		if len(saved.Thumbnail) > 0 && saved.ThumbnailWidth > 0 && saved.ThumbnailHeight > 0 {
			image := gfx.NewBsgImage(saved.ThumbnailWidth, saved.ThumbnailHeight, 0, 0, saved.Thumbnail)
			canvas.BlitScaled(image, layout.Thumbnail.X, layout.Thumbnail.Y, thumbWidth, thumbHeight, 255)
		}
	}
}

// drawSlotViewerMeters 画存档详情右栏的六名女主角攻略度条。
// frame0 是整条轨道，frame1 按 pts/MeterMax 截取前段覆盖在上面。
func (s *Session) drawSlotViewerMeters(canvas *gfx.BgraCanvas, slot int) {
	layout := s.config.DataScreen.Confirm
	if layout.MeterImage == "" {
		return
	}
	image := s.composer.Load(layout.MeterImage)
	if image == nil {
		return
	}
	var saved *save.SaveData
	if s.saves != nil {
		saved = s.saves.Read(slot)
	}
	meterMax := layout.MeterMax
	if meterMax <= 0 {
		meterMax = 100
	}
	nameSize := layout.MeterNameSize
	if nameSize <= 0 {
		nameSize = 14
	}
	nameColor := layout.MeterNameColor
	if nameColor == 0 {
		nameColor = 0xffffffff
	}
	x, y := layout.MeterRect[0], layout.MeterRect[1]
	w, h := layout.MeterRect[2], layout.MeterRect[3]
	for i := range layout.MeterNames {
		rowY := y + i*layout.MeterGap
		if name := layout.MeterNames[i]; name != "" && s.textFont != nil {
			text := s.textFont.Render(name, nameSize, 1, 1, nameSize+4, w, 1, nameColor)
			rendering.DrawTextImage(canvas, text, x, rowY-layout.MeterNameY)
		}
		canvas.DrawRegion(image, 0, layout.MeterTrackFrame*h, w, h, x, rowY)
		pts := clampInt(s.slotMeterValue(saved, layout.MeterProperties[i]), 0, meterMax)
		if fill := pts * w / meterMax; fill > 0 {
			canvas.DrawRegion(image, 0, layout.MeterFillFrame*h, fill, h, x, rowY)
		}
	}
}

// slotMeterValue 从存档里取攻略度属性；存档缺失或下标越界时按 0 处理。
func (s *Session) slotMeterValue(saved *save.SaveData, name string) int {
	if saved == nil || name == "" {
		return 0
	}
	index := s.script.GetPropertyIndex(name)
	if index < 0 || index >= len(saved.Machine.Properties) {
		return 0
	}
	return int(saved.Machine.Properties[index])
}

// drawDateDigits 画 yyyy/mm/dd。
func (s *Session) drawDateDigits(canvas *gfx.BgraCanvas, strip config.UIButton, stamp time.Time, x, y, spacing int) {
	if spacing == 0 {
		spacing = 14
	}
	s.drawFixedDigits(canvas, strip, stamp.Year(), 4, x, y, spacing)
	s.drawFontFrame(canvas, strip, 10, x+4*spacing, y)
	s.drawFixedDigits(canvas, strip, int(stamp.Month()), 2, x+5*spacing, y, spacing)
	s.drawFontFrame(canvas, strip, 10, x+7*spacing, y)
	s.drawFixedDigits(canvas, strip, stamp.Day(), 2, x+8*spacing, y, spacing)
}

// drawTimeDigits 画 hh:mm:ss。
func (s *Session) drawTimeDigits(canvas *gfx.BgraCanvas, strip config.UIButton, stamp time.Time, x, y, spacing int) {
	if spacing == 0 {
		spacing = 14
	}
	s.drawFixedDigits(canvas, strip, stamp.Hour(), 2, x, y, spacing)
	s.drawFontFrame(canvas, strip, 11, x+2*spacing, y)
	s.drawFixedDigits(canvas, strip, stamp.Minute(), 2, x+3*spacing, y, spacing)
	s.drawFontFrame(canvas, strip, 11, x+5*spacing, y)
	s.drawFixedDigits(canvas, strip, stamp.Second(), 2, x+6*spacing, y, spacing)
}

// drawFontFrame 画字条里的一个非数字帧（'/' 或 ':'）。
func (s *Session) drawFontFrame(canvas *gfx.BgraCanvas, strip config.UIButton, frame, x, y int) {
	if strip.Empty() {
		return
	}
	image := s.composer.Load(strip.Image)
	if image == nil {
		return
	}
	frames := strip.Frames
	if frames <= 0 {
		frames = 12
	}
	frame = clampInt(frame, 0, frames-1)
	height := image.Height / frames
	if height <= 0 {
		return
	}
	canvas.DrawRegion(image, 0, frame*height, image.Width, height, x, y)
}

// drawFixedDigits 从高位到低位画固定位数的数字（日期/时间用）。
func (s *Session) drawFixedDigits(canvas *gfx.BgraCanvas, strip config.UIButton, value, digits, x, y, spacing int) {
	if strip.Empty() || digits <= 0 {
		return
	}
	if spacing == 0 {
		spacing = 14
	}
	image := s.composer.Load(strip.Image)
	if image == nil {
		return
	}
	frames := strip.Frames
	if frames <= 0 {
		frames = 10
	}
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return
	}
	divisor := 1
	for i := 1; i < digits; i++ {
		divisor *= 10
	}
	for i := 0; i < digits; i++ {
		digit := (value / divisor) % 10
		canvas.DrawRegion(image, 0, digit*frameHeight, image.Width, frameHeight, x+i*spacing, y)
		divisor /= 10
	}
}

// drawAlignedText 画一段带对齐的文字；align 0 左、1 右、2 中（[slot viewer] 的约定）。
func (s *Session) drawAlignedText(canvas *gfx.BgraCanvas, text string, x, y, size int, color uint32, align, maxWidth int) {
	if s.textFont == nil || text == "" || size <= 0 {
		return
	}
	width := maxWidth
	if width <= 0 {
		width = s.width - x
	}
	rendered := s.textFont.Render(text, size, 1, 0, size+4, width, 6, color)
	if rendered == nil {
		return
	}
	drawX := x
	switch align {
	case 1:
		drawX = x - rendered.Bounds().Dx()
	case 2:
		drawX = x - rendered.Bounds().Dx()/2
	}
	rendering.DrawTextImage(canvas, rendered, drawX, y)
}

// ensureSlotPage 在换页 / 存档后重新读取当页缩略图。
//
// 一页 28 个槽位，每个存档文件含完整 VM 快照；每帧读 28 个文件会让翻页卡顿，
// 所以按页缓存。存档/读档后把 slotPage 置 -1 强制刷新。
func (s *Session) ensureSlotPage() {
	ui := s.ui
	if ui == nil || s.saves == nil {
		return
	}
	if ui.slotPage == ui.page && ui.slotThumbs != nil {
		return
	}
	ui.slotPage = ui.page
	ui.slotThumbs = map[int]*gfx.BgraCanvas{}
	ui.slotSaved = map[int]*save.SaveData{}
	layout := s.config.DataScreen
	// Tbxdl 的缩略图格是 160x120；现代作品沿用 126x96。
	width, height := thumbWidth, thumbHeight
	if layout.SlotThumb[2] > 0 && layout.SlotThumb[3] > 0 {
		width, height = layout.SlotThumb[2], layout.SlotThumb[3]
	}
	for index := 0; index < layout.SlotsPerPage; index++ {
		slotIndex := s.visibleSlotIndex(ui.page, index)
		if slotIndex >= save.GridSlotCount {
			break
		}
		stored := s.saves.Read(slotIndex)
		if layout.AutoSlotGrid && ui.page == 0 && index == layout.AutoSlotIndex {
			// 第 0 页左上角显示内部自动档，不占用手动网格的第 0 格。
			stored = s.saves.Read(save.AutoSlot)
		}
		if stored == nil {
			continue
		}
		ui.slotSaved[slotIndex] = stored
		if stored.ThumbnailWidth <= 0 || stored.ThumbnailHeight <= 0 || len(stored.Thumbnail) == 0 {
			continue
		}
		image := gfx.NewBsgImage(stored.ThumbnailWidth, stored.ThumbnailHeight, 0, 0, stored.Thumbnail)
		thumb := gfx.NewBgraCanvas(width, height)
		thumb.BlitScaled(image, 0, 0, width, height, 255)
		ui.slotThumbs[slotIndex] = thumb
	}
	core.Logf("ui: data screen page %d thumbnails=%d", ui.page, len(ui.slotThumbs))
}
