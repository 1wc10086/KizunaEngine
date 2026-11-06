package session

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/runtime/bs"
	"kizuna/engine/brand/bishop/runtime/bsmap"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

func (s *Session) MapButtons() []core.MapButton {
	return s.mapButtons()
}

func (s *Session) mapButtons() []core.MapButton {
	buttons := s.gameState.Buttons()
	result := make([]core.MapButton, 0, len(buttons))
	for index := range buttons {
		button := buttons[index]
		x, y, width, height := s.buttonRect(button)
		label := ""
		if uint(index) < uint(len(s.mapItems)) {
			// 显示名和脚本标签分开：label scan 的 Label 必须保持
			// tht_map/lyz_map 原样，chooseMapItem 才能 SeekBsLabel。
			label = bsmap.BsMapDisplayName(s.mapItems[index].Label)
		}
		result = append(result, core.MapButton{
			Index:  uint32(index),
			Image:  button.Image,
			X:      int32(x),
			Y:      int32(y),
			Width:  uint32(width),
			Height: uint32(height),
			Value:  int32(button.Value),
			Label:  label,
		})
	}
	return result
}

// buttonRect 返回按钮的有效热区（屏幕坐标）。
//
// 有 _hit 掩码的地图卡用掩码非零区域的外接矩形：KZK/屈辱2 的卡图是
// 整屏 1024x768 叠图，若把整张图当热区，所有卡会重叠成同一个矩形，
// 点击任何位置都只会命中列表里的第一张卡。没有掩码的按钮仍然按
// 图片真实尺寸（竖排图按帧数切）计算。
func (s *Session) buttonRect(button state.StageButton) (x, y, width, height int) {
	if button.HitMask != nil {
		return button.HitX, button.HitY, max(0, button.HitWidth), max(0, button.HitHeight)
	}
	x, y = button.X, button.Y
	sizeWidth, sizeHeight := s.buttonImageSize(button)
	width = button.Width
	if width <= 0 {
		width = sizeWidth
	}
	height = button.Height
	if height <= 0 {
		height = sizeHeight
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return x, y, width, height
}

// buttonImageSize 取按钮图的像素尺寸；竖排多帧图只算一帧的高度。
func (s *Session) buttonImageSize(button state.StageButton) (int, int) {
	if len(button.Image) == 0 || s.composer == nil {
		return 0, 0
	}
	loaded := s.composer.Load(button.Image)
	if loaded == nil {
		return 0, 0
	}
	width := loaded.Width
	height := loaded.Height
	frames := button.Frames
	if frames <= 0 {
		frames = state.StageButtonFrameCount
	}
	if height > 0 && frames > 0 {
		height = height / frames
	} else {
		height = 0
	}
	return width, height
}

func (s *Session) MapMask() (bool, int, int) {
	if s.mapMask == nil {
		return false, 0, 0
	}
	return true, s.mapMask.Width, s.mapMask.Height
}

// openBsMap 处理 SELECT_TAIL "map"：地图目录有候选地点时打开引擎地图
// （BSXScript 2.0/3.1 的裏教師系），并停在等待选择态。目录为空或地图
// profile 没有产出任何可点项时返回 false，调用方继续走通用路径，
// 保持"没有地图数据的作品不受影响"。
func (s *Session) openBsMap() bool {
	if s.mapCatalog == nil || s.mapCatalog.IsEmpty() {
		return false
	}
	// AllowEmpty：UK/UKIE 即使当前没有角色出门也要打开地图（底图 + 時間を進める），
	// 否则 SELECT_TAIL 被跳过，脚本会拿上一张图的 #result 继续分派。
	layout := s.mapCatalog.Resolve(s)
	if layout.IsEmpty() && !layout.AllowEmpty {
		return false
	}
	return s.beginMapSelect() == state.SessionAwaitSelect
}

func (s *Session) beginMapSelect() state.SessionStep {
	layout := s.mapCatalog.Resolve(s)
	// 三射面談按时段换整张底图：time rangeN 命中时用 base by timeN 的
	// base/over/on 覆盖默认的白天图。必须在建舞台层之前改，否则第一帧
	// 会先闪一张白天图。
	if layout.TimeSlotValid && s.config != nil && !layout.FixedBackground {
		if len(layout.FrameImage) == 0 && s.config.MapPhase.Frame.Image != "" {
			layout.FrameImage = s.config.MapPhase.Frame.Image
			layout.FrameX, layout.FrameY = s.config.MapPhase.Frame.X, s.config.MapPhase.Frame.Y
		}
		if base, ok := s.config.MapPhase.BaseForTime(layout.TimeValue); ok {
			if base.Base != "" {
				layout.Background = base.Base
			}
			if base.On != "" {
				layout.Overlay = base.On
			}
			if base.Over != "" {
				layout.OverlayHover = base.Over
			}
		}
	}
	s.clearMapStage()
	s.gameState.EndSelect()
	if layout.IsEmpty() && !layout.AllowEmpty {
		core.Logf("script: map without candidates, continuing")
		return state.SessionContinue
	}
	// 先清掉上一张地图遗留的结果，否则 #result 仍是旧值，剧本分派时会误开角色场景。
	// 两个条件都必须是"显式配置过"：ResultPropertyKey 的零值对其它 profile
	// 就是"没有结果变量"，用 >=0 会把属性槽位 0 当成有效槽位清零。
	if len(layout.ResultProperty) > 0 || layout.ResultPropertyKey > 0 {
		s.writeOne(layout.ResultProperty, layout.ResultPropertyKey, 0)
	}
	for _, item := range layout.Items {
		index := len(s.mapItems)
		s.mapItems = append(s.mapItems, item)
		s.gameState.AddButton(state.StageButton{
			X:      item.HitX,
			Y:      item.HitY,
			Value:  index,
			Frames: 1,
			Width:  item.HitWidth,
			Height: item.HitHeight,
			Layer:  -1,
		})
	}
	// 系统按钮（config/log/status/qsave/qload/save/load）不进 mapItems，下标排在角色卡之后。
	// 点击时由 chooseButton 按 Tool 分派到系统界面，不写 #character/#place。
	s.mapSystemCount = 0
	s.mapSystemTools = s.mapSystemTools[:0]
	for _, button := range layout.SystemButtons {
		// 纯热区按钮（MDDVD 的 map_base 内烤按钮）：没有独立图片，只给
		// 屏幕矩形。旧逻辑要求 Image 存在，会把这些按钮整排丢掉。
		if len(button.Image) == 0 && (button.Hit[2] <= 0 || button.Hit[3] <= 0) {
			continue
		}
		if len(button.Image) > 0 && !s.res.Exists(button.Image) {
			continue
		}
		frames := button.Frames
		if frames <= 0 {
			frames = 3
		}
		x, y, width, height := button.X, button.Y, button.Width, button.Height
		if button.Hit[2] > 0 && button.Hit[3] > 0 {
			x, y, width, height = button.Hit[0], button.Hit[1], button.Hit[2], button.Hit[3]
		}
		stageButton := state.NewStageButtonWithSize(button.Image, x, y, 0, frames, width, height)
		if button.Frame >= 0 {
			stageButton.Frame = button.Frame
		}
		s.gameState.AddButton(stageButton)
		s.mapSystemTools = append(s.mapSystemTools, button.Tool)
		s.mapSystemCount++
	}
	// [map phase] 按钮由 composer 的 drawButtons 绘制（悬停为第 1 帧），
	// 这里只进按钮表；若再加进舞台层，同一按钮会被画两次。
	if s.config != nil && s.composer != nil && !s.config.MapPhase.Empty() && layout.Source == bsmap.BsMapDataFileName {
		phase := s.config.MapPhase
		for _, button := range phase.Buttons {
			if len(button.Image) == 0 || !s.res.Exists(button.Image) {
				continue
			}
			frames := button.Frames
			if frames <= 0 {
				frames = 1
			}
			width, height := 0, 0
			if image := s.composer.Load(button.Image); image != nil {
				width = image.Width
				height = image.Height / frames
			}
			s.gameState.AddButton(state.NewStageButtonWithSize(button.Image, button.X, button.Y, 0, frames, width, height))
			s.mapSystemCount++
		}
	}
	s.showMapStage(layout)
	if layout.HasMask() {
		s.mapMask = layout.Mask
	} else {
		s.mapMask = nil
	}
	s.mapSource = layout.Source
	s.mapLayout = layout
	s.mapHoverRow = -1
	s.mapPreview = -1
	s.gameState.BeginSelect()
	// 学園（GK）的自动存档格：地图铺好后写一次内部 AutoSlot，读档时
	// 直接回到这张地图。只对配置了 AutoSlotGrid 的作品生效。
	if s.config != nil && s.config.DataScreen.AutoSlotGrid {
		s.pendingAutoSave = true
	}
	core.Logf("script: map select %d item(s) from %s", len(s.mapItems), layout.Source)
	if layout.Source == bsmap.BsMapGakuen2Source {
		// 地图头像的判定依赖这两组属性；出问题时这一行就能定位是"没人置位"
		// 还是"绘制没走到"，不用再去翻 stage 日志。
		s.logGK2MapFaces()
	}
	return state.SessionAwaitSelect
}

// logGK2MapFaces 在每次打开地图时记录一次六名角色的头像判定输入。
func (s *Session) logGK2MapFaces() {
	cur := make([]int, 0, bsmap.BsMapGakuen2RoleCount)
	for index := 0; index < bsmap.BsMapGakuen2RoleCount; index++ {
		sinkou := s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(index))
		if s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(index)) == 1 ||
			(sinkou >= 1 && sinkou <= 6) {
			cur = append(cur, index)
		}
	}
	core.Logf("script: map gakuen2: faces %v (map=%d,%d,%d,%d,%d,%d sinkou=%d,%d,%d,%d,%d,%d)",
		cur,
		s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(0)), s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(1)),
		s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(2)), s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(3)),
		s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(4)), s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(5)),
		s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(0)), s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(1)),
		s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(2)), s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(3)),
		s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(4)), s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(5)))
}

func (s *Session) showMapStage(layout bsmap.BsMapLayout) {
	if s.mapShown {
		return
	}
	table := make([]state.BsLayerEntry, 0, len(layout.Items)+1)
	if len(layout.Background) > 0 && s.res.Exists(layout.Background) {
		table = append(table, state.NewBsLayerEntry(
			layout.Background,
			0,
			0,
			state.StageLayerScreen,
			[]state.LayerArtwork{{Image: layout.Background, Frames: 1, AnimatedPart: -1}},
		))
	}
	// Overlay 是 base on（map_on）：三射面談把"地点圆点"烘焙在这张图里，
	// 底图本身没有标记。它必须画在角色卡之前，否则会把头像盖住。
	if len(layout.Overlay) > 0 && s.res.Exists(layout.Overlay) {
		table = append(table, state.NewBsLayerEntry(
			layout.Overlay,
			0,
			0,
			state.StageLayerCharacter,
			[]state.LayerArtwork{{Image: layout.Overlay, Frames: 1, AnimatedPart: -1}},
		))
	}
	// 装饰层（学園2 的周数/星期/残数）只画不点，放在地点图层之前；
	// 它们本身就在 map_base 的日期栏空白处，不需要参与命中。
	for _, item := range layout.Decor {
		if len(item.Image) == 0 || !s.res.Exists(item.Image) {
			continue
		}
		artwork := state.LayerArtwork{
			Image: item.Image, Frames: 1, AnimatedPart: -1,
			Frame: item.Frame, CellHeight: item.CellHeight,
			X: item.DrawX, Y: item.DrawY,
		}
		table = append(table, state.NewBsLayerEntry(item.Image, item.DrawX, item.DrawY, state.StageLayerCharacter, []state.LayerArtwork{artwork}))
	}
	for _, item := range layout.Items {
		if len(item.Image) == 0 || !s.res.Exists(item.Image) {
			continue
		}
		cell := s.mapCellHeight(item)
		artwork := state.LayerArtwork{Image: item.Image, Frames: 1, AnimatedPart: -1, Frame: item.Frame, CellHeight: cell}
		if item.DrawX >= 0 && item.DrawY >= 0 {
			artwork.X = item.DrawX
			artwork.Y = item.DrawY
		}
		table = append(table, state.NewBsLayerEntry(item.Image, artwork.X, artwork.Y, state.StageLayerCharacter, []state.LayerArtwork{artwork}))
	}
	// FrameImage 是地图的 info 面板/边框（map_frame、MK 的 message 底条）。
	// 它必须画在系统按钮之前：牝教師2 的 map_menu_base 把按钮文字烘焙在底图上，
	// MK 的 message 右侧半透明面板也要压在按钮下面，否则按钮会被整块盖住。
	if len(layout.FrameImage) > 0 && s.res.Exists(layout.FrameImage) {
		table = append(table, state.NewBsLayerEntry(
			layout.FrameImage,
			layout.FrameX,
			layout.FrameY,
			state.StageLayerCharacter,
			[]state.LayerArtwork{{Image: layout.FrameImage, X: layout.FrameX, Y: layout.FrameY, Frames: 1, AnimatedPart: -1}},
		))
	}
	// FrameImage2 与 FrameImage 同性质，只是多一块不参与命中的面板。
	// 放課後2 用它贴左上角的周数/曜日卡，FrameImage 则贴右下角的系统菜单底板。
	if len(layout.FrameImage2) > 0 && s.res.Exists(layout.FrameImage2) {
		table = append(table, state.NewBsLayerEntry(
			layout.FrameImage2,
			layout.Frame2X,
			layout.Frame2Y,
			state.StageLayerCharacter,
			[]state.LayerArtwork{{Image: layout.FrameImage2, X: layout.Frame2X, Y: layout.Frame2Y, Frames: 1, AnimatedPart: -1}},
		))
	}
	// 系统按钮也是三帧竖排图（常态/悬停/按下），舞台层只贴第 0 帧，
	// 悬停帧由按钮渲染按 hoveredButton 补画（与角色卡同一套约定）。
	// 缺这一段时按钮能点但看不见——玩家只能"盲点"地图底部。
	for _, button := range layout.SystemButtons {
		if len(button.Image) == 0 || !s.res.Exists(button.Image) {
			// 纯热区按钮已经烤在底图上，不能再当图层画一遍。
			continue
		}
		frames := button.Frames
		if frames <= 0 {
			frames = 3
		}
		// 舞台层的绘制只看 artwork 自己的 X/Y：NewBsLayerEntry 的 x/y 只是
		// 记账，不会替 artwork 平移。漏掉这两个字段时按钮会被画到 (0,0)
		// （牝教師2 底部工具条曾整排叠在左上角）。
		cellHeight := button.Height
		drawX, drawY := button.X, button.Y
		if image := s.composer.Load(button.Image); image != nil {
			if cellHeight <= 0 && frames > 1 && image.Height > 0 {
				cellHeight = image.Height / frames
			}
			// 有些地图按钮图自带屏幕坐标（牝教師2 的 map_nexttime 图头就是
			// 688,8）。渲染层会再加一次图头偏移，所以这里先减掉，保证
			// 最终落在配置/代码给出的位置上；图头为 0 的按钮不受影响。
			drawX -= image.OffsetX
			drawY -= image.OffsetY
		}
		artwork := state.LayerArtwork{Image: button.Image, Frames: frames, AnimatedPart: -1,
			CellHeight: cellHeight, X: drawX, Y: drawY, Frame: button.Frame}
		table = append(table, state.NewBsLayerEntry(button.Image, button.X, button.Y, state.StageLayerCharacter, []state.LayerArtwork{artwork}))
	}
	// Tbxdl 的楼层列表 mapmenu 是一张 306x374 的"常态 / 悬停"双列图。
	// 只把左列作为常驻图层，悬停行由 drawMapHover 在合成后补画右列，
	// 这样既不会让右列溢出到屏幕外，也不需要每帧重建整张舞台。
	if len(layout.MenuImage) > 0 && layout.MenuFrameWidth > 0 && s.res.Exists(layout.MenuImage) {
		artwork := state.LayerArtwork{
			Image: layout.MenuImage, Frames: 1, AnimatedPart: -1,
			X: layout.MenuX, Y: layout.MenuY,
			ClipX: 0, ClipY: 0, ClipWidth: layout.MenuFrameWidth,
		}
		table = append(table, state.NewBsLayerEntry(layout.MenuImage, artwork.X, artwork.Y, state.StageLayerCharacter, []state.LayerArtwork{artwork}))
	}
	if len(table) == 0 {
		return
	}
	s.mapBackup = append([]state.BsLayerEntry(nil), s.gameState.BsLayers()...)
	s.mapShown = true
	s.gameState.SetBsLayers(table)
	core.Logf("gfx: map stage %d layer(s), %d layer(s) parked", len(table), len(s.mapBackup))
}

func (s *Session) mapCellHeight(item bsmap.BsMapItem) int {
	if len(item.Image) == 0 || item.HitHeight <= 0 {
		return 0
	}
	image := s.composer.Load(item.Image)
	if image == nil {
		return 0
	}
	// 不能因为 FrameCount>1 就放弃切帧：牝教師2 的 map_ch*/map_item
	// 是 64x128 的两帧（常态/张嘴）竖排图，HitHeight 正是单帧高度，
	// 按 HitHeight 切才能只画一张脸。
	if image.Height <= item.HitHeight {
		return 0
	}
	if image.Height%item.HitHeight == 0 {
		return item.HitHeight
	}
	return 0
}

func (s *Session) hideMapStage() {
	if !s.mapShown {
		return
	}
	s.mapShown = false
	s.gameState.SetBsLayers(s.mapBackup)
	core.Logf("gfx: map stage restored %d layer(s)", len(s.mapBackup))
	s.mapBackup = nil
}

func (s *Session) clearMapStage() {
	s.mapBackup = nil
	s.mapShown = false
	s.mapItems = nil
	s.mapSystemCount = 0
	s.mapMask = nil
	s.mapSource = ""
	s.md2Map = false
	s.mapLayout = bsmap.BsMapLayoutEmpty
	s.mapHoverRow = -1
	s.mapPreview = -1
	s.pendingAutoSave = false
	s.selectButtons = nil
	s.clearCustomUI()
}

func (s *Session) cancelMapSelect() state.SessionStep {
	if !s.gameState.Selecting() || len(s.mapItems) == 0 {
		return state.SessionContinue
	}
	source := s.mapSource
	cancelLabel := s.mapLayout.CancelLabel
	resultProperty, resultKey := s.mapLayout.ResultProperty, s.mapLayout.ResultPropertyKey
	s.mapItems = nil
	s.mapMask = nil
	s.mapSource = ""
	s.mapLayout = bsmap.BsMapLayoutEmpty
	s.mapHoverRow = -1
	s.mapPreview = -1
	s.clearCustomUI()
	s.gameState.EndSelect()
	s.hideMapStage()
	if source == bsmap.BsMapDataFileName && (len(resultProperty) > 0 || resultKey > 0) {
		// 右键等同于"时间を進める"：把结果清零，避免上一张地图的 #result 被再次分派。
		s.writeOne(resultProperty, resultKey, 0)
	}
	// Tbxdl 的右键/取消语义是"仕事に戻る"（mp_end），不是回标题。
	// 直接 seek 到列表第 11 行的标签，玩家才会回到工作画面继续当天行程。
	if source == bsmap.BsMapTbxdlSystemParameterFile {
		if cancelLabel == "" {
			cancelLabel = "mp_end"
		}
		if s.machine.SeekBsLabel(cancelLabel) {
			core.Logf("script: map cancelled -> '%s'", cancelLabel)
			return s.run(commandBudget)
		}
		if s.machine.SeekBsLabel(bs.TitleLabel) {
			core.Logf("script: map cancelled -> 'title' (no '%s')", cancelLabel)
			return s.run(commandBudget)
		}
	}
	core.Logf("script: map cancelled, continuing in place")
	return s.run(commandBudget)
}

// drawMapCardHover 在地图合成后补画鼠标下那张卡的第二帧。
//
// 牝教師2 的 map_ch*/map_item 是"常态/张嘴"两帧竖排图：舞台层只贴第 0 帧，
// 悬停帧必须后补（和其它地图按钮的悬停约定一致）。只对 map.dat 地图生效，
// 且必须有确定的帧高，避免把别的 profile 的整图裁掉一半。
func (s *Session) drawMapCardHover(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.ui != nil || s.mapSource != bsmap.BsMapDataFileName || len(s.mapItems) == 0 {
		return false
	}
	index := s.gameState.HoveredButton()
	if index < 0 || index >= len(s.mapItems) {
		return false
	}
	item := s.mapItems[index]
	if len(item.Image) == 0 || item.CellHeight <= 0 || item.DrawX < 0 || item.DrawY < 0 {
		return false
	}
	image := s.composer.Load(item.Image)
	if image == nil || image.Height < item.CellHeight*2 {
		return false
	}
	canvas.DrawRegion(image, 0, item.CellHeight, image.Width, item.CellHeight, item.DrawX, item.DrawY)
	return true
}

// drawMapHover 在地图合成后补画 Tbxdl 楼层列表的悬停行。
//
// mapmenu.bsg 把常态列和悬停列并排烘焙在同一张 306x374 图里：左列 153px
// 是常态、右列 153px 是悬停。舞台层只贴左列（见 showMapStage），这里按
// 指针命中的行把右列对应 153x34 条带盖在同一行上。
func (s *Session) drawMapHover(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil || s.ui != nil {
		// 存读档/设置/回想等系统界面盖在地图上时，地图的日期卡、头像和
		// 悬停帧不能再画到界面之上。
		return false
	}
	if s.mapSource == bsmap.BsMapGakuen2Source {
		return s.drawGK2MapOverlay(canvas)
	}
	if s.mapSource == bsmap.BsMapDataFileName {
		return s.drawBsMapOverlay(canvas)
	}
	if s.mapSource == bsmap.BsMapGkSource {
		return s.drawGkMapOverlay(canvas)
	}
	if s.mapSource != bsmap.BsMapTbxdlSystemParameterFile || !s.mapShown {
		return false
	}
	row := s.mapHoverRow
	layout := s.mapLayout
	if row < 0 || row >= layout.MenuFrames || len(layout.MenuImage) == 0 {
		return false
	}
	frameWidth, rowHeight := layout.MenuFrameWidth, layout.MenuRowHeight
	if frameWidth <= 0 || rowHeight <= 0 {
		return false
	}
	image := s.composer.Load(layout.MenuImage)
	if image == nil {
		return false
	}
	sourceY := row * rowHeight
	if sourceY < 0 || sourceY+rowHeight > image.Height {
		return false
	}
	canvas.DrawRegion(image, frameWidth, sourceY, frameWidth, rowHeight,
		layout.MenuX, layout.MenuY+sourceY)
	return true
}

// drawBsMapOverlay 补画 UK/UKIE 地图上不参与命中的动态元素。
//
// 三块内容都是"每帧变值"的图，不能塞进舞台层（舞台层只贴固定帧）：
//   - 悬停角色的第 1 帧：map_f* 是"常态 / 绿框悬停"两帧竖排，而 BSG 头的
//     FrameCount 恒为 1，所以舞台层只切第 0 帧，悬停帧在这里盖上去。
//   - 周数 / 曜日卡（map_wno / map_wday）：值来自脚本维护的 #week_no / #week_day。
//   - 时段卡（map_time）：4 帧是 4/3/2/1，画第 value-1 帧。
func (s *Session) drawBsMapOverlay(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil || s.config == nil || !s.mapShown {
		return false
	}
	phase := s.config.MapPhase
	if phase.Empty() {
		return false
	}
	drawn := false
	// 悬停角色高亮：按钮下标与 mapItems 一一对应（角色卡在按钮表最前面）。
	if index := s.gameState.HoveredButton(); index >= 0 && index < len(s.mapItems) {
		item := s.mapItems[index]
		if item.HitHeight > 0 {
			if image := s.composer.Load(item.Image); image != nil && image.Height >= item.HitHeight*2 {
				canvas.DrawRegion(image, 0, item.HitHeight, image.Width, item.HitHeight, item.DrawX, item.DrawY)
				drawn = true
			}
		}
	}
	if week, weekday, ok := s.bsMapWeekValues(phase); ok {
		if s.drawBsMapStrip(canvas, phase.WeekNo, week) {
			drawn = true
		}
		if s.drawBsMapStrip(canvas, phase.WeekDay, weekday) {
			drawn = true
		}
	}
	// 三射面談的 #time 是 4..7，而 map_time 只有 4 帧；layout.TimeSlot 是
	// map.dat 的 TimeValues 换算出的 0 基槽位，不能再用 #time-1。
	if phase.TimeZone.Image != "" {
		strip := s.bsMapTimeStrip(phase.TimeZone)
		if s.mapLayout.TimeSlotValid {
			if s.drawBsMapStrip(canvas, strip, s.mapLayout.TimeSlot) {
				drawn = true
			}
		} else {
			timeVariable := phase.TimeVariable
			if timeVariable == "" {
				timeVariable = "#time"
			}
			// #time 是 1 基的 4 个时段；时段图从上到下是 4/3/2/1，
			// 画 value-1 让第 1 时段显示"4"、第 4 时段显示"1"（剩余行动格）。
			if clock := s.readProperty(timeVariable); clock >= 1 && clock <= strip.Frames {
				if s.drawBsMapStrip(canvas, strip, clock-1) {
					drawn = true
				}
			}
		}
	}
	// DAY 卡片里的日期数字（day no）。它和时段图一样每格重画，
	// 不能塞进舞台层；值来自脚本维护的 #day。
	if phase.DayStrip.Image != "" && len(s.mapLayout.DayVariable) > 0 {
		if day := s.readProperty(s.mapLayout.DayVariable); day >= 0 {
			if s.drawBsMapDay(canvas, phase, day) {
				drawn = true
			}
		}
	}
	if s.drawBsMapMoveCount(canvas, phase) {
		drawn = true
	}
	if s.drawBsMapMeter(canvas, phase) {
		drawn = true
	}
	return drawn
}

// drawBsMapMoveCount 画「当前是第几次移动」。
//
// 放課後2 的 map_info_count 是 3 帧（1/2/3），脚本每走一步把 #time 加一
// （4→5→6→7 进下校時間），所以帧号 = #time - MoveCountBase。超出 0..2
// 的时段（早晨/放学后）原版不画这块，这里同样跳过。
func (s *Session) drawBsMapMoveCount(canvas *gfx.BgraCanvas, phase config.MapPhase) bool {
	if phase.MoveCount.Image == "" || phase.MoveCountBase == 0 || s.composer == nil {
		return false
	}
	variable := phase.MoveCountVariable
	if variable == "" {
		variable = "#time"
	}
	value := s.readProperty(variable) - phase.MoveCountBase
	if value < 0 || value >= phase.MoveCount.Frames {
		return false
	}
	return s.drawBsMapStrip(canvas, phase.MoveCount, value)
}

// drawBsMapMeter 画右下角的「絶倫メーター」。
//
// map_z_base 是 72x126 的外框，msg_z 是 7 帧 56x110 的瓶子；原版把瓶子
// 画在外框内 (8,8)，帧号 = (#zeturin+19)/20（0..6），超过 6 仍停在最后一帧。
// 没有该配置的作品（MeterStep 为 0）完全跳过。
func (s *Session) drawBsMapMeter(canvas *gfx.BgraCanvas, phase config.MapPhase) bool {
	if phase.Meter.Image == "" || phase.MeterStep <= 0 || s.composer == nil {
		return false
	}
	drawn := false
	if phase.MeterBase.Image != "" {
		if stack := s.composer.Load(phase.MeterBase.Image); stack != nil {
			canvas.Draw(stack, phase.MeterBase.X+stack.OffsetX, phase.MeterBase.Y+stack.OffsetY)
			drawn = true
		}
	}
	variable := phase.MeterVariable
	if variable == "" {
		variable = "#zeturin"
	}
	value := s.readProperty(variable)
	if value < 0 {
		value = 0
	}
	frame := (value + phase.MeterStep - 1) / phase.MeterStep
	if frame >= phase.Meter.Frames {
		frame = phase.Meter.Frames - 1
	}
	if s.drawBsMapStrip(canvas, phase.Meter, frame) {
		drawn = true
	}
	return drawn
}

// drawBsMapDay 按 [map phase] 的 day no / day unit / day sp 画 DAY 卡片。
//
// 数字条是竖排 0..9；原引擎从个位开始向左补高位，day no offset 是负数
// （三射面談 -18）。这里先算出最高位的位置，再从高位画到个位，保证
// "1" 与 "10" 的单位对齐方式与原版一致。
//
// SMIE 的 map_frame 只有黑色底板，DAY 字样来自 day unit（info_unit）；
// SM 的 map_frame 把 "7 DAY" 烘焙在底板上、day unit 被注释掉。两种都要
// 能正确显示：有 day unit 就画，没有则保留底板上的 DAY。
func (s *Session) drawBsMapDay(canvas *gfx.BgraCanvas, phase config.MapPhase, value int) bool {
	if canvas == nil || s.composer == nil {
		return false
	}
	value += phase.DayAdjustment
	if value < 0 {
		return false
	}
	// 日期越过门槛时整块换成 day sp（SMIE 是 "OPENING"）。
	if value >= phase.DayBorder {
		if phase.DaySpecial.Image == "" {
			return false
		}
		image := s.composer.Load(phase.DaySpecial.Image)
		if image == nil {
			return false
		}
		canvas.Draw(image, phase.DaySpecial.X+image.OffsetX, phase.DaySpecial.Y+image.OffsetY)
		return true
	}
	strip := phase.DayStrip
	if len(strip.Image) == 0 {
		return false
	}
	image := s.composer.Load(strip.Image)
	if image == nil {
		return false
	}
	frames := strip.Frames
	if frames <= 0 {
		frames = 10
	}
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return false
	}
	offset := phase.DayOffset
	if offset == 0 {
		offset = -image.Width
	}
	digits := 1
	for probe := value; probe >= 10; probe /= 10 {
		digits++
	}
	topX := strip.X + (digits-1)*offset
	for index := 0; index < digits; index++ {
		digit := (value / pow10(digits-1-index)) % 10
		x := topX - index*offset
		canvas.DrawRegion(image, 0, digit*frameHeight, image.Width, frameHeight,
			x+image.OffsetX, strip.Y+image.OffsetY)
	}
	// day unit（DAY）固定贴在数字右侧。SMIE 的 map_frame 没有字样，必须画。
	if phase.DayUnit.Image != "" {
		if unit := s.composer.Load(phase.DayUnit.Image); unit != nil {
			canvas.Draw(unit, phase.DayUnit.X+unit.OffsetX, phase.DayUnit.Y+unit.OffsetY)
		}
	}
	return true
}

func pow10(exponent int) int {
	result := 1
	for index := 0; index < exponent; index++ {
		result *= 10
	}
	return result
}

// bsMapWeekValues 取地图右上角周数/曜日卡要显示的值。
//
// 优先用脚本自己维护的 #week_no / #week_day：UK 在每天开头（eyecatch）
// 用 `#week_no = #day / 7`、`#week_day = #day % 7` 写它们，直接采用比
// 引擎再算一遍可靠（曜日图的 0 帧是日曜日，和脚本 %7 的顺序一致）。
// 没有这两个属性的作品才退回按 #day 换算。
func (s *Session) bsMapWeekValues(phase config.MapPhase) (int, int, bool) {
	// 两个属性都为 0 只可能是"第 1 天还没写"（脚本跳过了 eyecatch），
	// 不能让地图卡把它当成 第一週 日曜日，退回下面的 day/7、day%7。
	if week := s.readProperty("#week_no"); week >= 0 {
		if weekday := s.readProperty("#week_day"); weekday >= 0 && (week != 0 || weekday != 0) {
			return week, bsMapShiftWeekDay(weekday, phase.WeekDayShift), true
		}
	}
	dayVariable := phase.DayVariable
	if dayVariable == "" {
		dayVariable = "#day"
	}
	day := s.readProperty(dayVariable)
	if day < 0 {
		return 0, 0, false
	}
	days := s.config.Information.WeekDays
	if days <= 0 {
		days = 7
	}
	offset := day - s.config.Information.WeekStart
	if offset < 0 {
		return 0, 0, false
	}
	weekday := bsMapShiftWeekDay(offset, phase.WeekDayShift)
	// 一周 6 天（放課後2：月～週末）时曜日必须按 6 取模，
	// 不能再用写死的 7：第 6 天会被算成日曜日，整周错一位。
	// 其它作品（7 天的 MK/黒の教室、KD 的 10 进制日期）保持原行为。
	if days == 6 {
		weekday = ((offset % days) + phase.WeekDayShift) % days
		if weekday < 0 {
			weekday += days
		}
	}
	return offset / days, weekday, true
}

// bsMapShiftWeekDay 把脚本的曜日编号换算成素材帧序。
// 默认 0=日曜日；牝教師的 map_week 从 MONDAY 开始，配置给 -1。
func bsMapShiftWeekDay(weekday, shift int) int {
	const days = 7
	return ((weekday+shift)%days + days) % days
}

// bsMapTimeStrip 修正时段图帧数。
//
// BSXX 的 `time zone` 值在不同发行版里写法不一致：MKIE 写的是
// `map_time, 1, 624, 548`，但 map_time 实际是 42x184 的四帧（早晚各一张
// 太阳图标）。按 1 帧画会把整条 184px 的竖条贴到 104px 高的信息面板上，
// 既溢出屏幕又只显示最上面一格。这里只对"高是宽的 4 倍左右"的竖条按 4 帧
// 处理，其它作品的单帧时段图保持原样。
func (s *Session) bsMapTimeStrip(strip config.DigitStrip) config.DigitStrip {
	if strip.Frames > 1 || s.composer == nil {
		return strip
	}
	image := s.composer.Load(strip.Image)
	if image == nil || image.Width <= 0 {
		return strip
	}
	if image.Height >= image.Width*4 {
		strip.Frames = 4
	}
	return strip
}

// drawBsMapStrip 按 value 取一条竖排图的帧并贴到配置坐标。
func (s *Session) drawBsMapStrip(canvas *gfx.BgraCanvas, strip config.DigitStrip, value int) bool {
	if canvas == nil || s.composer == nil || len(strip.Image) == 0 || strip.Frames <= 0 {
		return false
	}
	image := s.composer.Load(strip.Image)
	if image == nil || image.Height < strip.Frames {
		return false
	}
	cellHeight := image.Height / strip.Frames
	if cellHeight <= 0 {
		return false
	}
	// Start 把曜日等偏移到图中的后半段（放課後2 的 map_info_str 前 5 帧是
	// 周数、5..11 才是月..日）。零值对其它作品没有影响。
	frame := core.Clamp(value+strip.Start, 0, strip.Frames-1)
	canvas.DrawRegion(image, 0, frame*cellHeight, image.Width, cellHeight,
		strip.X+image.OffsetX, strip.Y+image.OffsetY)
	return true
}

// drawGK2MapOverlay 画学園2 地图的目标头像、悬停高亮、十字准星与
// 「Move Place」预览卡。
//
// 原版地图是三张全屏叠图：map_base（航拍底图）、map_on（可点区域的白色描边，
// 黑底做色键）、map_site（跟随鼠标的准星），再叠一排目标头像。点地点后先弹
// map_backN 的 430x326 预览卡（图里已经烘焙了 Move Place 标题），再点一次才移动。
//
// 绘制顺序照着原引擎的层叠关系：头像在最下面（预览卡会盖住它），然后是
// 描边/悬停，最后才是跟着指针走的准星。
func (s *Session) drawGK2MapOverlay(canvas *gfx.BgraCanvas) bool {
	drawn := s.drawGK2MapFaces(canvas)
	if s.mapPreview >= 0 {
		name := "map_back" + strconv.Itoa(s.mapPreview)
		if s.drawImageAt(canvas, name, 185, 137) {
			drawn = true
		}
	}
	if s.mapPreview < 0 && s.gameState != nil && s.gameState.HoveredButton() >= 0 {
		// map_on 的黑底是透明色键，只复制白色描边/按钮。
		s.drawGK2Region(canvas, "map_on", gk2Rect{0, 0, s.width, s.height})
		drawn = true
	}
	if s.mapPointerX > 0 || s.mapPointerY > 0 {
		// map_site 是三帧准星，原点在图片中心。
		if image := s.composer.Load("map_site"); image != nil {
			frameHeight := image.Height / 3
			if frameHeight > 0 {
				x := s.mapPointerX - image.Width/2
				y := s.mapPointerY - frameHeight/2
				canvas.DrawRegion(image, 0, 0, image.Width, frameHeight, x, y)
				drawn = true
			}
		}
	}
	return drawn
}

// drawGK2MapFaces 画「今天地图上有哪些目标」的头像行。
//
// 三个数据来源都必须照原版：
//   - 出现条件：#hrn_map..#kyk_map 等于 1（剧本在事件里置位、选完地点清零），
//     顺序固定为 春菜/美久/香織/泉/綾乃/恭子。
//   - 坐标：MAPPRMS.DAT 0x68 的 28 组 (x, y)，1..7 人各一组，取左上角。
//   - 帧：map_face 80x1204 竖排 14 帧，第 i 个角色用第 i 帧（0..5 绿框）；
//     帧高写死 86，因为 BSG 头里的 FrameCount 是 1。
//
// 头像不参与命中，点击仍由地点掩码决定，所以这里只画不改按钮表。
func (s *Session) drawGK2MapFaces(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil {
		return false
	}
	present := make([]int, 0, bsmap.BsMapGakuen2RoleCount)
	for index := 0; index < bsmap.BsMapGakuen2RoleCount; index++ {
		// 两个条件任一成立就画：sinkou（路线进行到 1..6 段）或 map（今天有事件）。
		// 只看 #X_map 的话，序章到第一个事件之间整排都是空的；sinkou 限定
		// 1..6 是因为 MAPPRMS 的条件程序只测这六段，越大越不代表"在地图上"，
		// 原版也不会把超出范围的残值当成出勤。
		sinkou := s.sessionProperty(bsmap.BsMapGakuen2RoleSinkouProperty(index))
		if s.sessionProperty(bsmap.BsMapGakuen2RoleProperty(index)) == 1 ||
			(sinkou >= 1 && sinkou <= 6) {
			present = append(present, index)
		}
	}
	if len(present) == 0 {
		return false
	}
	image := s.composer.Load(bsmap.BsMapGakuen2FaceImage)
	if image == nil {
		return false
	}
	drawn := false
	for slot, role := range present {
		x, y, ok := bsmap.BsMapGakuen2FaceSlot(len(present), slot)
		if !ok {
			continue
		}
		sourceY := role * bsmap.BsMapGakuen2FaceCellHeight
		if sourceY+bsmap.BsMapGakuen2FaceCellHeight > image.Height {
			continue
		}
		canvas.DrawRegion(image, 0, sourceY, image.Width,
			bsmap.BsMapGakuen2FaceCellHeight, x, y)
		drawn = true
	}
	return drawn
}

// beginCustomMap 进入脚本自绘地图的选择态。
//
// 与 beginMapSelect 的区别：[custom map phase] 没有地点表，底图和按钮
// 都是剧本用 IMAGE/IMAGE_EX type=160 画的，按钮已经在 gameState 里。
// 这里不能 hideMapStage（会把剧本刚画好的舞台换回去），只清掉上一张地图的
// 记账并打开选择态。
//
// SL（BSXScript 2.0）的行程/工作画面是个例外：剧本只画 work_bg 和
// AM/PM 工作面板，START 等按钮留在 bsxx.cfg 里由原引擎补画。没有这个
// 兜底，选择态里一个按钮都没有，玩家会永远停在工作画面上。
func (s *Session) beginCustomMap() {
	s.mapItems = nil
	s.mapSystemCount = 0
	s.mapMask = nil
	s.mapBackup = nil
	s.mapShown = false
	s.mapSource = "custom"
	// KZK 的 map_repeat 在锁卡（#select_value == 1000）时只重发
	// SELECT_TAIL "custom map"，不会重新铺卡；EndSelect 已清空按钮表，
	// 这里必须把上一屏的卡补回来，否则 BeginSelect 后一个热区都没有。
	if len(s.gameState.Buttons()) == 0 && len(s.selectButtons) > 0 {
		for _, button := range s.selectButtons {
			s.gameState.AddButton(button)
		}
		core.Logf("select: restored %d map button(s) for custom map re-loop", len(s.selectButtons))
	}
	// 不能 EndSelect：它会清空剧本刚用 IMAGE_EX type=160 加进来的按钮。
	s.gameState.BeginSelect()
	// 行动卡双卡分支只加 1 次 #layercount，后几张卡会留在屏幕外；
	// 按实际卡数套用同一张 4…9 卡网格（只对 #action_point 作品生效）。
	s.arrangeActionCards()
	s.ensureCustomMapButtons()
	s.refreshCustomMapButtons()
	core.Logf("script: custom map select %d button(s)", len(s.gameState.Buttons()))
}

// restoreCustomMapScreen 从存档恢复脚本自绘地图的按钮表。
//
// 只恢复按钮/来源，不重跑地图脚本：存档时的 PC 已经停在 SELECT_TAIL
// 之后，玩家再点一张卡，chooseButton 会写结果并从下一条指令继续，
// 和实时游玩完全一致。舞台层/背景由 SaveData.ApplyTo 恢复。
func (s *Session) restoreCustomMapScreen(buttons []save.SaveMapButton) {
	s.mapItems = nil
	s.mapSystemCount = 0
	s.mapMask = nil
	s.mapShown = false
	s.mapSource = "custom"
	s.mapLayout = bsmap.BsMapLayoutEmpty
	s.mapHoverRow = -1
	s.mapPreview = -1
	s.pendingAutoSave = false
	s.selectButtons = nil
	s.clearCustomUI()
	for _, button := range buttons {
		stageButton := state.NewStageButtonWithSize(
			button.Image, button.X, button.Y, button.Value, button.Frames,
			button.Width, button.Height)
		stageButton.Layer = button.Layer
		stageButton.Disabled = button.Disabled
		stageButton.HoverFrame = button.HoverFrame
		stageButton.HasHoverFrame = button.HasHoverFrame
		s.gameState.AddButton(stageButton)
	}
	s.gameState.BeginSelect()
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// chooseMapNextTime 处理 UK/UKIE 地图上的「時間を進める」。
// 原引擎在这里结束当天这一格、直接推进到下一时段：脚本的 map 标签在
// SELECT_TAIL 之后按 #result 1..10 分派角色场景，0 表示谁也没见，
// 于是什么也不调用，主循环把 #time 加一后继续。这里必须把 #result
// 清零，否则上一张地图的结果会被重复消费。
func (s *Session) chooseMapNextTime() state.SessionStep {
	layout := s.mapLayout
	s.mapItems = nil
	s.mapSystemCount = 0
	s.mapMask = nil
	s.mapSource = ""
	s.mapLayout = bsmap.BsMapLayoutEmpty
	s.mapHoverRow = -1
	s.mapPreview = -1
	s.gameState.EndSelect()
	s.hideMapStage()
	if len(layout.ResultProperty) > 0 || layout.ResultPropertyKey > 0 {
		// 牝教師2 的两代把 9999 当「時間を進める」；其它作品保持清零语义。
		value := 0
		if layout.NextValue != 0 {
			value = layout.NextValue
		}
		s.writeOne(layout.ResultProperty, layout.ResultPropertyKey, value)
	}
	core.Logf("script: map next time")
	return s.run(commandBudget)
}

func (s *Session) chooseButton(index int) state.SessionStep {
	buttons := s.gameState.Buttons()
	if !s.gameState.Selecting() || index < 0 || index >= len(buttons) {
		return state.SessionContinue
	}
	if index < len(s.mapItems) {
		return s.chooseMapItem(index)
	}
	button := buttons[index]
	if index < len(s.mapItems)+s.mapSystemCount {
		if s.mapSource == bsmap.BsMapDataFileName && customMapToolKey(button.Image) == "next" {
			return s.chooseMapNextTime()
		}
		// profile 直接给了功能键时以它为准；旧 profile 没有 Tool，仍按图片名推断。
		tool := ""
		if offset := index - len(s.mapItems); offset >= 0 && offset < len(s.mapSystemTools) {
			tool = s.mapSystemTools[offset]
		}
		handled := false
		if tool != "" {
			handled = s.handleCustomMapToolKey(tool)
		} else {
			handled = s.handleCustomMapTool(button)
		}
		// 系统按钮只打开系统界面，永远不结束选择、不写 #select_value。工具键认不出时
		// 也必须停在原地：落到下面的写回路径会把地图当成已选中某张卡。
		if !handled {
			core.Logf("custom ui: map system button '%s' (tool '%s') has no action, staying on map", button.Image, tool)
		}
		return state.SessionContinue
	}
	if button.Disabled {
		// work_start 在四人未选齐工作时是禁用态（work_start.txt 的
		// conditions.disable）；点了不结束选择，否则剧本会带着 0 继续。
		return state.SessionContinue
	}
	if s.handleCustomMapTool(button) {
		return state.SessionContinue
	}
	if button.Image == "work_help" && s.config != nil && len(s.config.CustomMap.HelpImage) > 0 {
		// SL 的 HELP 是引擎自绘的整屏说明图，不交给剧本：剧本没有
		// #select_value == 2 的分支。再点任意位置关闭。
		s.customMapHelp = true
		if s.composer != nil {
			s.composer.Invalidate()
		}
		return state.SessionContinue
	}
	// 快照必须在 EndSelect 清空按钮表之前抓：KZK 的锁卡会把
	// #select_value 写成 1000 并原地跳回 map_repeat，脚本不会重画卡片。
	if !s.machine.IsBsScript() {
		s.selectButtons = append([]state.StageButton(nil), buttons...)
	}
	s.gameState.EndSelect()
	s.clearCustomUI()
	s.writeSelectResult(button, index)
	core.Logf("select: map target '%s' value=%d", button.Image, button.Value)
	return s.run(commandBudget)
}

// restoreSelectButtons 在脚本原地重发 SELECT_TAIL 时恢复上一屏按钮。
//
// 只用于“点了不可用/锁住的卡”这种剧本循环：EndSelect 已经清掉按钮表，
// 剧本却只重新 SELECT_TAIL 而不再画卡。恢复后重新 BeginSelect，
// 宿主就能继续在地图上操作。背景一旦重画（新的一天/新地点），
// 快照会在主循环里作废，不会把旧卡带到新地图上。
func (s *Session) restoreSelectButtons() {
	if len(s.selectButtons) == 0 {
		return
	}
	for _, button := range s.selectButtons {
		s.gameState.AddButton(button)
	}
	s.gameState.BeginSelect()
	core.Logf("select: restored %d map button(s) after a script-driven re-loop", len(s.selectButtons))
}

// mapPointerHit 返回指针下的按钮下标。
//
// UK/UKIE 的地图除了角色卡矩形，还用 map_hit 掩码把整个地点区域映射到
// 该地点的第一张角色卡：只判 56px 的脸部矩形时，点在「寮」「校舎」等地名
// 上完全没有反应，而原引擎的命中就是按地点区域算的。掩码只对配置了
// [map phase] 的 map.dat 地图启用，其它 profile 的行为保持不变。
func (s *Session) mapPointerHit(x, y int) int {
	if index := s.hitButton(x, y); index >= 0 {
		return index
	}
	// mask 只由真正带像素热区的 profile 设置（map_hit 等），
	// 不需要再要求 [map phase]：牝教師2（Rev.6）没有 cfg，却同样用
	// map_hit 把整片地点映射到卡上。没有 mask 的作品行为不变。
	if s.mapMask != nil && len(s.mapItems) > 0 {
		if mapped := s.mapMask.ItemAt(x, y); mapped >= 0 && mapped < len(s.mapItems) {
			return mapped
		}
	}
	return -1
}

func (s *Session) chooseMapPixel(x int, y int) state.SessionStep {
	if !s.gameState.Selecting() {
		return state.SessionContinue
	}
	// 有 mask 的作品（BSScript 的 MAP 命令）按像素掩码命中，一个地点可以有
	// 不规则的轮廓；没有 mask 的作品（BSXScript 3.x 用 ImageExtended type=160
	// 直接铺按钮图）则退回按钮矩形命中。
	//
	// 这条 fallback 是必须的：现代化 Bishop 剧本根本不走 mapflow，`mapMask`
	// 始终为 nil，早期实现直接 return，导致行动选择画面看似有按钮却完全点不动，
	// 玩家会卡在第一天、`#day` 也永远不会增长。Pointer 路径本身会先尝试
	// hitButton，但宿主的选择窗口/辅助输入也可能调用这里，所以两处都要成立。
	// map.dat 地图的多张脸可能共用同一个地点掩码，先按每张脸自己的矩形命中，
	// 才能点中具体角色而不是掩码里的第一张。
	if s.mapSource == bsmap.BsMapDataFileName {
		if index := s.hitButton(x, y); index >= 0 {
			return s.chooseButton(index)
		}
	}
	if s.mapMask != nil && len(s.mapItems) > 0 {
		index := s.mapMask.ItemAt(x, y)
		if index >= 0 && index < len(s.mapItems) {
			return s.chooseMapItem(index)
		}
	}
	if index := s.hitButton(x, y); index >= 0 {
		return s.chooseButton(index)
	}
	return state.SessionContinue
}

func (s *Session) chooseMapItem(index int) state.SessionStep {
	item := s.mapItems[index]
	label := bsmap.SelectBsMapTbxdlBranch(item, s.readMapProperty)
	if !item.HasProperty() && len(label) == 0 {
		core.Logf("script: map item %d has no action, staying on map", index)
		return state.SessionContinue
	}
	s.mapItems = nil
	s.mapSystemCount = 0
	s.mapSystemTools = nil
	s.mapMask = nil
	s.mapSource = ""
	s.mapPreview = -1
	s.gameState.EndSelect()
	s.hideMapStage()
	if item.HasProperty() {
		s.writeMapProperty(item)
		return s.run(commandBudget)
	}
	if len(label) == 0 {
		core.Logf("script: map item %d has no action, continuing in place", index)
		return s.run(commandBudget)
	}
	if !s.machine.SeekBsLabel(label) {
		core.Logf("script: map label '%s' is missing, continuing in place", label)
		return s.run(commandBudget)
	}
	core.Logf("script: map label '%s' selected", label)
	return s.run(commandBudget)
}

func (s *Session) readMapProperty(key int) int {
	isProperty, slot, ok := s.script.ResolveSlot(key)
	if !ok {
		return 0
	}
	if isProperty {
		return int(s.machine.Property(slot))
	}
	variables := s.machine.Variables()
	if uint(slot) < uint(len(variables)) {
		return int(variables[slot])
	}
	return 0
}

func (s *Session) writeMapProperty(item bsmap.BsMapItem) {
	s.writeOne(item.Property, item.PropertyKey, item.Value)
	if item.HasSecondProperty() {
		s.writeOne(item.Property2, item.Property2Key, item.Value2)
	}
}

func (s *Session) writeOne(name string, key int, value int) {
	// 属性名优先。地图 profile 传入的 key 有两套语义：UK/三射面談/牝教師2
	// 传的是"属性表下标"（由 GetPropertyIndex 得到），TBXDL 的 branch 传的
	// 才是系统参数键。ResolveSlot 修好之后，把属性下标当系统键解析可能
	// 落到别的属性上（例如 10 在 OH 是 #miya_karte 而不是第 10 个属性），
	// 所以有名字时永远按名字写，key 只在名字解析不到时兜底。
	if len(name) > 0 {
		if named := s.script.GetPropertyIndex(name); named >= 0 {
			s.machine.SetProperty(named, int32(value))
			core.Logf("script: map '%s' = %d", name, value)
			return
		}
	}
	if key >= 0 {
		if isProperty, slot, ok := s.script.ResolveSlot(key); ok {
			if isProperty {
				s.machine.SetProperty(slot, int32(value))
				core.Logf("script: map slot %d = %d", key, value)
				return
			}
			if variables := s.machine.Variables(); uint(slot) < uint(len(variables)) {
				variables[slot] = int32(value)
				core.Logf("script: map slot %d = %d", key, value)
				return
			}
		}
	}
	core.Logf("script: map property '%s' (key %d) not found", name, key)
}

func (s *Session) ByName(name string) int {
	index := s.script.GetPropertyIndex(name)
	if index < 0 {
		return -1
	}
	return int(s.machine.Property(index))
}

func (s *Session) ByKey(key int) int {
	isProperty, index, ok := s.script.ResolveSlot(key)
	if !ok {
		return -1
	}
	if isProperty {
		return int(s.machine.Property(index))
	}
	if variables := s.machine.Variables(); uint(index) < uint(len(variables)) {
		return int(variables[index])
	}
	return -1
}

func (s *Session) writeChoiceResult(option vm.SelectOption) {
	s.machine.ClearSelectOptions()
	// 普通选择肢固定写 #select_value / #select_type，**不能**复用
	// [custom map phase] result：三射面談把那个变量配成 #result，用于地图卡/
	// 命令按钮的结果；如果普通确认框也写 #result，会把刚选中的命令
	// （0xb0）覆盖成 0/1，afternoon_run 就永远分派不到对应角色。
	valueSlot := s.script.GetPropertyIndex("#select_value")
	typeSlot := s.script.GetPropertyIndex("#select_type")
	value := option.Value
	// Kind=-1 的 FETCH：第三个字段是选项文本的消息号（決定=2、やり直す=3），
	// 真正的选择结果是 0 基序号。午后命令画面的确认框用
	// `#select_value == 1` 判断「やり直す」。Kind=-2（其它作品的普通选项）
	// 仍把 Value 当结果值。
	if option.Kind == -1 {
		value = option.Index
	}
	if valueSlot >= 0 {
		s.machine.SetProperty(valueSlot, int32(value))
	}
	if typeSlot >= 0 {
		s.machine.SetProperty(typeSlot, int32(option.Index))
	}
	core.Logf("choice: select value=%d type=%d kind=%d", value, option.Index, option.Kind)
}

func (s *Session) resolveSelectSlots() {
	if s.valueSlot == unresolvedSlot {
		s.valueSlot = s.script.GetPropertyIndex(s.config.SelectResultVariable())
	}
	if s.typeSlot == unresolvedSlot {
		s.typeSlot = s.script.GetPropertyIndex(s.config.SelectTypeVariable())
		if s.typeSlot < 0 {
			// 特別授業3SLG 的 [custom map phase] 写的是 #custom_map_type，
			// 但剧本属性表里实际叫 #select_type；按配置名找不到时退回它。
			s.typeSlot = s.script.GetPropertyIndex("#select_type")
		}
	}
}

func (s *Session) writeSelectResult(button state.StageButton, index int) {
	s.resolveSelectSlots()
	if s.valueSlot >= 0 {
		s.machine.SetProperty(s.valueSlot, int32(button.Value))
	}
	if s.typeSlot >= 0 {
		s.machine.SetProperty(s.typeSlot, int32(index+1))
	}
	// 标准 #select_value 槽：原引擎对自定义地图按钮会额外记录"第几个按钮"
	// （0 基）。三射面談的午后命令画面（afternoon_result）用
	// `#select_value == 1` 判断「家に帰る」（第二个按钮），=0 才是执行命令。
	// 这些作品的 [custom map phase] result 指向 #result，上面的写入不会碰
	// #select_value；必须补这一份，否则点任何按钮都会落进同一个分支。
	// 若作品自己把 result 配成 #select_value（侵蝕/淫獄），按钮值已经写进去，
	// 不能再覆盖成序号。
	if !isSelectValueVariable(s.selectResultVariableName()) {
		if slot := s.script.GetPropertyIndex("#select_value"); slot >= 0 &&
			slot != s.valueSlot && slot != s.typeSlot {
			s.machine.SetProperty(slot, int32(index))
		}
	}
	// [custom map phase] select object：教育指導 IE 的目标卡用它记录
	// "第几张被选中"（1 基），剧本的 target_loop_confirmed 会按这个值
	// 把卡片移到中央并播放确认动画。不写的话 select_no 一直是 0，
	// 确认循环会一直走到 count 之外。
	if s.config != nil {
		if name := s.config.SelectObjectVariable(); len(name) > 0 {
			if slot := s.script.GetPropertyIndex(name); slot >= 0 {
				s.machine.SetProperty(slot, int32(index+1))
			}
		}
	}
}

// isSelectValueVariable 判断配置的 result 变量是否就是 #select_value。
func isSelectValueVariable(name string) bool {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "#")
	return strings.EqualFold(name, "select_value")
}

func (s *Session) selectResultVariableName() string {
	if s.config == nil {
		return ""
	}
	return s.config.SelectResultVariable()
}

func (s *Session) buildSelectWindow() {
	window := s.config.SelectWindow
	design := window.Design(s.machine.SelectArgument())
	frame := s.composer.Images.Get(design.Item)
	frameWidth := 0
	frameHeight := 0
	if frame != nil {
		frameWidth = frame.Width
		frames := design.Frames
		if frames <= 0 {
			frames = state.StageButtonFrameCount
		}
		frameHeight = frame.Height / max(1, frames)
		if frameHeight < 1 {
			frameHeight = 1
		}
	}
	headHeight, footHeight := s.selectFrameExtraHeights(design)
	items := rendering.BuildSelectWindow(design, s.width, s.height, frameWidth, frameHeight, headHeight, footHeight, window.DatumLine, s.gameState.Choices())
	s.gameState.SetSelectItems(items)
	core.Logf("choice: select window '%s' %d item(s)", design.Item, len(items))
}

// selectFrameExtraHeights 取选项条上下装饰图的高度。
func (s *Session) selectFrameExtraHeights(design config.SelectDesign) (int, int) {
	if !design.HeaderFooter || s.composer == nil {
		return 0, 0
	}
	head, foot := 0, 0
	if image := s.composer.Images.Get(design.Item + "_head"); image != nil {
		head = image.Height
	}
	if image := s.composer.Images.Get(design.Item + "_foot"); image != nil {
		foot = image.Height
	}
	return head, foot
}
