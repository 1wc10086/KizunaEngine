package session

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_chart.go 是 CHART（场景流程图）界面。
//
// 原引擎把"状態"画面拆成两种页：inventory 页按脚本属性贴状态图，CHART 页则
// 画一张可滚动的流程地图。两者共用 define.bsa:status_layout，但 CHART 的格子
// 和连线另存在 chart.db 里（见 config/chart.go）。
//
// 玩家从哪里进来：工具条的 status 按钮（这些作品图上写的是 btn_chart）。
// 点击一个场景格子会弹确认框，确认后从该场景开始回放；看完回放会回到 CHART
// 原来的页和滚动位置（chartRestore），而不是被弹回第一页。
//
// 坐标要分清两套：
//   - 格子的 pos 已经是作画分辨率（YK 1366x768 / Yakata 1024x576）。
//   - 连线的坐标按 1024 宽排版，画之前乘 screenWidth/1024 落到作画坐标。
//
// 因此格子不能再缩放，连线却必须缩放，否则线和格子会整体错开一截。

// chartUIState 是一屏 CHART 的可变状态。
type chartUIState struct {
	layout *config.ChartLayout
	db     *config.ChartDatabase
	page   int
	// scrollX/scrollY 是"画布可视区左上角对应的地图坐标"。
	// 画格子时用 block.pos - scroll，拖动/滚轮只改这两个值。
	scrollX int
	scrollY int
	// thumbPage/thumbs 是当前页的格子缩略图缓存：一页最多几十格，每格
	// 由背景 + 若干立绘合成，每帧重算会把整屏图反复缩放几十次。
	thumbPage int
	thumbs    []*gfx.BgraCanvas
	// hoverBlock 是鼠标命中的格子下标（-1 = 没有）；只用来画高亮框。
	hoverBlock int
	// confirm/pendingLabel 是"是否从选中场景开始回放"的确认框状态。
	confirm      bool
	pendingLabel string
}

// chartRestoreState 是回放前 CHART 的浏览位置。
//
// 回放结束会重新构建 systemUI，页和滚动值只能从这里恢复；只存页码会让玩家
// 在长地图（YK 第一页纵向 2500+ 像素）里每次都被弹回顶部。
type chartRestoreState struct {
	page    int
	scrollX int
	scrollY int
}

// chartRect 是 CHART 的可视区域（屏幕坐标）。
type chartRect struct {
	left   int
	top    int
	right  int
	bottom int
}

func (r chartRect) width() int {
	return max(0, r.right-r.left)
}

func (r chartRect) height() int {
	return max(0, r.bottom-r.top)
}

func (r chartRect) contains(x, y int) bool {
	return x >= r.left && x < r.right && y >= r.top && y < r.bottom
}

// ensureChartDatabase 惰性读取 chart.db。
//
// chartDBDone 区分"还没读"和"资源不存在"：没有 chart.db 的作品只显示
// status_layout 的普通页，CHART 按钮退化成原版的无操作，而不是每帧重读 BSA。
func (s *Session) ensureChartDatabase() *config.ChartDatabase {
	if s.chartDBDone {
		return s.chartDB
	}
	s.chartDBDone = true
	if s.config == nil || s.res == nil {
		return nil
	}
	name := s.config.ChartDatabase
	if name == "" {
		name = "chart"
	}
	location, ok := s.res.TryGet(name)
	if !ok || location == nil {
		core.Logf("chart: database '%s' not found", name)
		return nil
	}
	database, err := config.ParseChartDatabase(location.Read())
	if err != nil {
		core.Logf("chart: parse '%s' failed: %v", name, err)
		return nil
	}
	s.chartDB = database
	core.Logf("chart: database '%s' pages=%d", name, len(database.Pages))
	return s.chartDB
}

// statusChartLayout 从 status_layout 里取 CHART 页的排版。
//
// 返回第一个带 chart 的页；没有就返回 nil，调用方保持原状态画面。
func statusChartLayout(layout *config.StatusScreenLayout) *config.ChartLayout {
	if layout == nil {
		return nil
	}
	for index := range layout.Pages {
		if layout.Pages[index].Chart != nil {
			return layout.Pages[index].Chart
		}
	}
	return nil
}

// initStatusChart 给刚打开的"状態"界面挂上 CHART 状态。
//
// 只有布局里真的有 chart 页、且 chart.db 能解析出页时才启用；否则保持
// 原来的 inventory/achievement 绘制路径。
func (s *Session) initStatusChart(layout *config.StatusScreenLayout) {
	if s.ui == nil {
		return
	}
	chart := statusChartLayout(layout)
	if chart == nil {
		return
	}
	database := s.ensureChartDatabase()
	if database == nil || len(database.Pages) == 0 {
		core.Logf("chart: no pages, keeping status screen")
		return
	}

	state := &chartUIState{
		layout:     chart,
		db:         database,
		thumbPage:  -1,
		hoverBlock: -1,
	}
	restored := false
	if restore := s.chartRestore; restore != nil {
		state.page = restore.page
		state.scrollX = restore.scrollX
		state.scrollY = restore.scrollY
		s.chartRestore = nil
		restored = true
	}
	state.page = clampInt(state.page, 0, len(database.Pages)-1)
	if !restored {
		// 第一次打开时优先落在"进行中"的那个场景所在的一页；找不到才留在
		// 第一页。这样玩家从剧情里点 CHART 不会先看到序章的地图。
		if page, _, ok := chartFindBlock(database, s.currentSceneLabel, -1); ok {
			state.page = page
		}
	}
	s.ui.chart = state
	if !restored && s.currentSceneLabel != "" {
		s.chartFocusCurrent(state)
	}
	core.Logf("ui: chart opened page=%d/%d", state.page+1, len(database.Pages))
}

// chartArea 把布局的 draw_area 转成屏幕矩形。
//
// draw_area 写的是 [左, 上, 右, 下]，不是 [x,y,宽,高]。YK 是
// [213,0,1366,768]，Yakata 是 [162,0,1024,576]。缺字段时按整屏兜底。
func (s *Session) chartArea(layout *config.ChartLayout) chartRect {
	area := chartRect{left: 0, top: 0, right: s.width, bottom: s.height}
	if layout == nil {
		return area
	}
	if layout.DrawArea[2] > layout.DrawArea[0] {
		area.left = layout.DrawArea[0]
		area.right = layout.DrawArea[2]
	}
	if layout.DrawArea[3] > layout.DrawArea[1] {
		area.top = layout.DrawArea[1]
		area.bottom = layout.DrawArea[3]
	}
	area.left = clampInt(area.left, 0, max(0, s.width))
	area.top = clampInt(area.top, 0, max(0, s.height))
	area.right = clampInt(area.right, area.left, max(area.left, s.width))
	area.bottom = clampInt(area.bottom, area.top, max(area.top, s.height))
	return area
}

// chartThumbSize 是每个格子的缩略图尺寸。
//
// thumb_ratio 是"相对屏幕宽度的比例"（.094），高度沿用这些作品的 16:9
// 画幅。YK 得到 128x72，Yakata 得到 96x54，正好和 chart_next_block 素材
// 的实际尺寸一致。
func (s *Session) chartThumbSize(layout *config.ChartLayout) (int, int) {
	ratio := config.ChartDefaultThumbRatio
	if layout != nil && layout.ThumbRatio > 0 {
		ratio = layout.ThumbRatio
	}
	width := int(float64(s.width) * ratio)
	if width < 16 {
		width = 16
	}
	height := width * 9 / 16
	if height < 9 {
		height = 9
	}
	return width, height
}

// ---------------------------------------------------------------- 绘制

func (s *Session) drawChartScreen(canvas *gfx.BgraCanvas, state *chartUIState) {
	if canvas == nil || state == nil || state.layout == nil || state.db == nil {
		return
	}
	area := s.chartArea(state.layout)
	state.hoverBlock = -1
	if s.ui != nil && s.ui.hover.kind == hitChartBlock {
		state.hoverBlock = s.ui.hover.index
	}

	// chart_base 是左侧页签栏的底图（宽 241 / 180），不是整屏底图。
	// 画布区域保持黑色，地图缩略图才有对比度。
	canvas.Clear(0, 0, 0, 255)
	if !s.drawLayerAt(canvas, state.layout.Base, 0, 0) {
		canvas.Fill(0, 0, s.width, s.height, 26, 24, 22, 255)
	}

	s.ensureChartThumbs(state)
	s.drawChartLines(canvas, state, area)
	s.drawChartBlocks(canvas, state, area)
	s.drawChartButtons(canvas, state)
	s.drawChartSceneName(canvas, state, area)
	if state.confirm {
		s.drawChartConfirm(canvas, state)
	}
}

// drawChartButtons 画左侧的 A–H 页码和"显示当前场景"。
//
// 按钮图第 0 帧是命中掩码，不画；1 常态、2 悬停、3 当前页。
func (s *Session) drawChartButtons(canvas *gfx.BgraCanvas, state *chartUIState) {
	pageIndex := 0
	for _, button := range state.layout.Buttons {
		if isChartNowSceneButton(button) {
			continue
		}
		if pageIndex >= len(state.db.Pages) {
			pageIndex++
			continue
		}
		frame := 1
		if s.ui != nil && s.ui.hover.kind == hitChartPage && s.ui.hover.index == pageIndex {
			frame = 2
		}
		if pageIndex == state.page {
			frame = 3
		}
		s.drawFrameButton(canvas, chartUIButton(button), frame)
		pageIndex++
	}
	if state.layout.NowScene.Name != "" {
		frame := 1
		if s.ui != nil && s.ui.hover.kind == hitChartNowScene {
			frame = 2
		}
		s.drawFrameButton(canvas, chartUIButton(state.layout.NowScene), frame)
	}
}

// drawChartConfirm 画"从选中场景开始播放吗"的确认框。
//
// 确认底图是整屏图（dialog 画在中间），所以直接铺满 0,0；OK/Cancel 的坐标
// 是屏幕绝对坐标，沿用布局原值。
func (s *Session) drawChartConfirm(canvas *gfx.BgraCanvas, state *chartUIState) {
	confirm := state.layout.Confirm
	if !s.drawLayerAt(canvas, confirm.Base.Name, 0, 0) {
		canvas.FillBlend(0, 0, s.width, s.height, 0, 0, 0, 170)
	}
	if confirm.OK.Name != "" {
		frame := 1
		if s.ui != nil && s.ui.hover.kind == hitChartConfirmOK {
			frame = 2
		}
		s.drawFrameButton(canvas, chartUIButton(confirm.OK), frame)
	}
	if confirm.Cancel.Name != "" {
		frame := 1
		if s.ui != nil && s.ui.hover.kind == hitChartConfirmCancel {
			frame = 2
		}
		s.drawFrameButton(canvas, chartUIButton(confirm.Cancel), frame)
	}
}

// drawChartLines 画格子之间的连线。
//
// 连线坐标属于 1024 宽的虚拟画布，先乘 screenWidth/1024 再减去滚动值。
// 只画可视区内的像素：地图纵向可达 2500+，全画会做大量无用填充。
func (s *Session) drawChartLines(canvas *gfx.BgraCanvas, state *chartUIState, area chartRect) {
	page := state.chartPage()
	if page == nil {
		return
	}
	scale := float64(s.width) / 1024.0
	for _, line := range page.Lines {
		points := make([]config.UIPoint, 0, len(line.Points)+2)
		points = append(points, config.UIPoint{X: line.Source.X, Y: line.Source.Y})
		points = append(points, line.Points...)
		points = append(points, config.UIPoint{X: line.Destination.X, Y: line.Destination.Y})
		for index := 0; index+1 < len(points); index++ {
			x0 := area.left + int(float64(points[index].X)*scale) - state.scrollX
			y0 := area.top + int(float64(points[index].Y)*scale) - state.scrollY
			x1 := area.left + int(float64(points[index+1].X)*scale) - state.scrollX
			y1 := area.top + int(float64(points[index+1].Y)*scale) - state.scrollY
			s.drawChartSegment(canvas, area, x0, y0, x1, y1)
		}
	}
}

// drawChartSegment 用 Bresenham 画一段 1px 半透明白线。
//
// BgraCanvas 没有线段原语；线宽保持 1px，避免相邻线在地图缩略图之间糊成一片。
func (s *Session) drawChartSegment(canvas *gfx.BgraCanvas, area chartRect, x0, y0, x1, y1 int) {
	dx := chartAbs(x1 - x0)
	dy := -chartAbs(y1 - y0)
	stepX := 1
	if x0 > x1 {
		stepX = -1
	}
	stepY := 1
	if y0 > y1 {
		stepY = -1
	}
	err := dx + dy
	for {
		if area.contains(x0, y0) {
			canvas.FillBlend(x0, y0, 1, 1, 220, 220, 220, 120)
		}
		if x0 == x1 && y0 == y1 {
			return
		}
		double := 2 * err
		if double >= dy {
			err += dy
			x0 += stepX
		}
		if double <= dx {
			err += dx
			y0 += stepY
		}
	}
}

// drawChartBlocks 画格子缩略图。
//
// 格子位置是中心点（line 的端点正好在中心上下 ±24 处），所以贴图时减去
// 半宽/半高。可视区裁剪用 BlitCanvasRegion 的源矩形，避免缩略图盖到左侧
// 页码栏上。
func (s *Session) drawChartBlocks(canvas *gfx.BgraCanvas, state *chartUIState, area chartRect) {
	page := state.chartPage()
	if page == nil {
		return
	}
	thumbW, thumbH := s.chartThumbSize(state.layout)
	for index, block := range page.Blocks {
		if index >= len(state.thumbs) || state.thumbs[index] == nil {
			continue
		}
		thumb := state.thumbs[index]
		x := area.left + block.X - state.scrollX - thumbW/2
		y := area.top + block.Y - state.scrollY - thumbH/2

		sourceX := max(0, area.left-x)
		sourceY := max(0, area.top-y)
		targetX := max(area.left, x)
		targetY := max(area.top, y)
		width := min(thumbW-sourceX, area.right-targetX)
		height := min(thumbH-sourceY, area.bottom-targetY)
		if width <= 0 || height <= 0 {
			continue
		}
		canvas.BlitCanvasRegion(thumb, sourceX, sourceY, width, height, targetX, targetY, 255)

		// 高亮：悬停用白框，进行中的场景用金框。两者同时成立时金色优先，
		// 玩家更容易找到"我现在在哪"。
		if block.Label != "" && block.Label == s.currentSceneLabel {
			drawChartBorder(canvas, area, targetX, targetY, width, height, 255, 214, 90)
		} else if index == state.hoverBlock {
			drawChartBorder(canvas, area, targetX, targetY, width, height, 255, 255, 255)
		}
	}
}

// drawChartBorder 画 2px 高亮框，只落在可视区内。
func drawChartBorder(canvas *gfx.BgraCanvas, area chartRect, x, y, width, height int, red, green, blue byte) {
	if width <= 0 || height <= 0 {
		return
	}
	clip := func(x, y, w, h int) {
		left := max(x, area.left)
		top := max(y, area.top)
		right := min(x+w, area.right)
		bottom := min(y+h, area.bottom)
		if left < right && top < bottom {
			canvas.FillBlend(left, top, right-left, bottom-top, blue, green, red, 220)
		}
	}
	clip(x, y, width, 2)
	clip(x, y+height-2, width, 2)
	clip(x, y, 2, height)
	clip(x+width-2, y, 2, height)
}

// drawChartSceneName 画右上角的场景名。
//
// 悬停格子时显示悬停格子的标题，否则显示当前进行中的场景；标题来自
// scene_name.db，缺失时退回标签本身。
func (s *Session) drawChartSceneName(canvas *gfx.BgraCanvas, state *chartUIState, area chartRect) {
	if s.textFont == nil || s.config == nil {
		return
	}
	label := ""
	if page := state.chartPage(); page != nil && state.hoverBlock >= 0 && state.hoverBlock < len(page.Blocks) {
		label = page.Blocks[state.hoverBlock].Label
	}
	if label == "" {
		label = s.currentSceneLabel
	}
	if label == "" || strings.HasPrefix(label, "@") {
		return
	}
	title := s.chartSceneTitle(label)
	if title == "" {
		return
	}

	font := s.config.Font
	size := max(10, font.Size)
	width := max(40, area.width()-32)
	image := s.textFont.Render(title, size, 1, 1, max(1, font.Height), width, 1, 0xffffffff)
	if image == nil {
		return
	}
	textWidth := image.Bounds().Dx()
	x := state.layout.SceneName.X
	y := state.layout.SceneName.Y
	switch strings.ToLower(state.layout.SceneName.Align) {
	case "right", "1":
		x -= textWidth
	case "center", "2":
		x -= textWidth / 2
	}
	x = clampInt(x, area.left, max(area.left, area.right-textWidth))
	y = clampInt(y, area.top, max(area.top, area.bottom-1))
	rendering.DrawTextImage(canvas, image, x, y)
}

// chartSceneTitle 把场景标签翻成 scene_name.db 里的标题。
func (s *Session) chartSceneTitle(label string) string {
	if label == "" {
		return ""
	}
	if catalog := s.ensureScenes(); catalog != nil {
		if title, ok := catalog.Titles[label]; ok && title != "" {
			return title
		}
	}
	return label
}

// ensureChartThumbs 重建当前页的格子缩略图。
//
// 每个格子都在一张全屏虚拟画布上按原版构图（背景 + 带水平偏移的立绘）画好，
// 再缩到 thumb 大小。这样 bustup 的 ContainerOffset/部件偏移不需要在缩略图
// 坐标系里重算一遍，和原版预览的构图保持一致。
func (s *Session) ensureChartThumbs(state *chartUIState) {
	if state.thumbPage == state.page && len(state.thumbs) > 0 {
		return
	}
	page := state.chartPage()
	if page == nil {
		state.thumbs = nil
		state.thumbPage = state.page
		return
	}

	thumbW, thumbH := s.chartThumbSize(state.layout)
	virtual := gfx.NewBgraCanvas(max(1, s.width), max(1, s.height))
	thumbs := make([]*gfx.BgraCanvas, len(page.Blocks))
	for index, block := range page.Blocks {
		if len(block.Thumbs) == 0 {
			continue
		}
		// 特殊格（*chart_next_block）是一张现成的缩略图，不需要合成。
		if special := chartSpecialThumb(block); special != nil {
			image := s.composer.Load(special.Name)
			if image == nil {
				continue
			}
			node := gfx.NewBgraCanvas(thumbW, thumbH)
			node.BlitScaled(image, 0, 0, thumbW, thumbH, 255)
			thumbs[index] = node
			continue
		}

		virtual.Clear(0, 0, 0, 0)
		if chartLayeredThumbs(block.Thumbs) {
			for _, thumb := range block.Thumbs {
				s.drawChartThumbLayer(virtual, thumb)
			}
		} else {
			// 事件 CG 列表是同一场的若干候选帧，不是叠加层；取第一张能读到的。
			for _, thumb := range block.Thumbs {
				image := s.composer.Load(thumb.Name)
				if image == nil {
					continue
				}
				virtual.Draw(image, image.OffsetX, image.OffsetY)
				break
			}
		}
		source := &gfx.BsgImage{
			Width:      virtual.Width,
			Height:     virtual.Height,
			Pixels:     virtual.Buffer,
			FrameCount: 1,
		}
		node := gfx.NewBgraCanvas(thumbW, thumbH)
		node.BlitScaled(source, 0, 0, thumbW, thumbH, 255)
		thumbs[index] = node
	}
	state.thumbs = thumbs
	state.thumbPage = state.page
}

// drawChartThumbLayer 把一张缩略图层画到全屏虚拟画布上。
//
// 带逗号的立绘用 OffsetX 左右分列；合成图要先画部件再带上容器偏移，
// 和鉴赏室画立绘的规则一致，否则人物会整体偏到画面一角。
func (s *Session) drawChartThumbLayer(canvas *gfx.BgraCanvas, thumb config.ChartThumb) {
	image := s.composer.Load(thumb.Name)
	if image == nil {
		return
	}
	if len(image.Parts) > 0 {
		for _, part := range image.Parts {
			if part == nil {
				continue
			}
			canvas.Draw(part,
				thumb.OffsetX+image.ContainerOffsetX+part.OffsetX,
				image.ContainerOffsetY+part.OffsetY)
		}
		return
	}
	canvas.Draw(image, thumb.OffsetX+image.OffsetX, image.OffsetY)
}

// chartSpecialThumb 返回特殊缩略图（名字前带 '*'）。
func chartSpecialThumb(block config.ChartBlock) *config.ChartThumb {
	for index := range block.Thumbs {
		if block.Thumbs[index].Special {
			return &block.Thumbs[index]
		}
	}
	return nil
}

// chartLayeredThumbs 判断一组 thumb 是"叠加构图"还是"候选帧"。
//
// 规则来自数据本身：有水平偏移，或出现 bu 开头的立绘，就必须按图层叠；
// 纯 ev 开头的若干张属于同一场景的候选帧，只取第一张，避免不透明 CG
// 一层层互相盖住。
func chartLayeredThumbs(thumbs []config.ChartThumb) bool {
	if len(thumbs) <= 1 {
		return false
	}
	for _, thumb := range thumbs {
		if thumb.OffsetX != 0 || strings.HasPrefix(thumb.Name, "bu") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- 命中与交互

func (s *Session) hitChartScreen(x, y int) uiHit {
	state := s.chartState()
	if state == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if state.confirm {
		if state.layout.Confirm.OK.Name != "" && hitButton(s, chartUIButton(state.layout.Confirm.OK), x, y) {
			return uiHit{kind: hitChartConfirmOK}
		}
		if state.layout.Confirm.Cancel.Name != "" && hitButton(s, chartUIButton(state.layout.Confirm.Cancel), x, y) {
			return uiHit{kind: hitChartConfirmCancel}
		}
		return uiHit{kind: hitNone, index: -1}
	}

	pageIndex := 0
	for _, button := range state.layout.Buttons {
		if isChartNowSceneButton(button) {
			continue
		}
		if pageIndex < len(state.db.Pages) && hitButton(s, chartUIButton(button), x, y) {
			return uiHit{kind: hitChartPage, index: pageIndex}
		}
		pageIndex++
	}
	if state.layout.NowScene.Name != "" && hitButton(s, chartUIButton(state.layout.NowScene), x, y) {
		return uiHit{kind: hitChartNowScene}
	}

	area := s.chartArea(state.layout)
	if !area.contains(x, y) {
		return uiHit{kind: hitNone, index: -1}
	}
	page := state.chartPage()
	if page == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	// 屏幕坐标 → 地图坐标。格子 pos 是中心点，命中区就是缩略图矩形。
	mapX := x - area.left + state.scrollX
	mapY := y - area.top + state.scrollY
	thumbW, thumbH := s.chartThumbSize(state.layout)
	for index, block := range page.Blocks {
		if mapX >= block.X-thumbW/2 && mapX < block.X+thumbW/2 &&
			mapY >= block.Y-thumbH/2 && mapY < block.Y+thumbH/2 {
			return uiHit{kind: hitChartBlock, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateChartScreen(hit uiHit) {
	state := s.chartState()
	if state == nil {
		return
	}
	if state.confirm {
		switch hit.kind {
		case hitChartConfirmOK:
			label := state.pendingLabel
			state.confirm = false
			state.pendingLabel = ""
			if label == "" {
				return
			}
			// 先记下页/滚动位置，回放结束后 openStatusScreen 会用它恢复。
			s.chartRestore = &chartRestoreState{
				page:    state.page,
				scrollX: state.scrollX,
				scrollY: state.scrollY,
			}
			s.startSceneReplay(label, uiStatus, 0)
		case hitChartConfirmCancel:
			state.confirm = false
			state.pendingLabel = ""
		}
		return
	}

	switch hit.kind {
	case hitChartPage:
		s.chartSwitchPage(state, hit.index)
	case hitChartNowScene:
		s.chartFocusCurrent(state)
	case hitChartBlock:
		page := state.chartPage()
		if page == nil || hit.index < 0 || hit.index >= len(page.Blocks) {
			return
		}
		label := strings.TrimSpace(page.Blocks[hit.index].Label)
		if label == "" {
			return
		}
		// @N 是原引擎的"下一页"格，不是可回放场景。
		if strings.HasPrefix(label, "@") {
			if number, err := strconv.Atoi(strings.TrimPrefix(label, "@")); err == nil {
				s.chartSwitchPage(state, number)
			}
			return
		}
		state.pendingLabel = label
		state.confirm = true
	}
}

// chartSwitchPage 切换到另一页地图，并清掉缩略图缓存。
func (s *Session) chartSwitchPage(state *chartUIState, page int) {
	if state == nil || state.db == nil {
		return
	}
	if page < 0 || page >= len(state.db.Pages) {
		return
	}
	state.page = page
	state.thumbPage = -1
	state.thumbs = nil
	state.scrollX = 0
	state.scrollY = 0
	state.hoverBlock = -1
	state.confirm = false
	s.chartClampScroll(state)
}

// chartFocusCurrent 跳到进行中的场景格子并把它放到可视区中央。
func (s *Session) chartFocusCurrent(state *chartUIState) {
	if state == nil || state.db == nil || s.currentSceneLabel == "" {
		return
	}
	page, block, ok := chartFindBlock(state.db, s.currentSceneLabel, state.page)
	if !ok {
		return
	}
	if page != state.page {
		state.page = page
		state.thumbPage = -1
		state.thumbs = nil
	}
	area := s.chartArea(state.layout)
	state.scrollX = block.X - area.width()/2
	state.scrollY = block.Y - area.height()/2
	s.chartClampScroll(state)
}

// chartFindBlock 在所有页里找标签对应的格子；先看 preferPage。
func chartFindBlock(database *config.ChartDatabase, label string, preferPage int) (int, config.ChartBlock, bool) {
	if database == nil || label == "" {
		return 0, config.ChartBlock{}, false
	}
	if preferPage >= 0 && preferPage < len(database.Pages) {
		for _, block := range database.Pages[preferPage].Blocks {
			if block.Label == label {
				return preferPage, block, true
			}
		}
	}
	for page := range database.Pages {
		if page == preferPage {
			continue
		}
		for _, block := range database.Pages[page].Blocks {
			if block.Label == label {
				return page, block, true
			}
		}
	}
	return 0, config.ChartBlock{}, false
}

// chartClampScroll 把滚动值限制在当前页内容的范围内。
//
// 内容比可视区小时停在 0，避免把地图拖出屏幕只剩黑边。
func (s *Session) chartClampScroll(state *chartUIState) {
	page := state.chartPage()
	if page == nil || len(page.Blocks) == 0 {
		state.scrollX = 0
		state.scrollY = 0
		return
	}
	thumbW, thumbH := s.chartThumbSize(state.layout)
	minX, minY := page.Blocks[0].X, page.Blocks[0].Y
	maxX, maxY := minX, minY
	for _, block := range page.Blocks {
		minX = min(minX, block.X)
		minY = min(minY, block.Y)
		maxX = max(maxX, block.X)
		maxY = max(maxY, block.Y)
	}
	area := s.chartArea(state.layout)
	maxScrollX := max(0, maxX+thumbW/2-area.width())
	maxScrollY := max(0, maxY+thumbH/2-area.height())
	state.scrollX = clampInt(state.scrollX, 0, maxScrollX)
	state.scrollY = clampInt(state.scrollY, 0, maxScrollY)
}

// wheelChart 用滚轮上下滚动地图；CHART 的页签切换走左侧按钮。
func (s *Session) wheelChart(delta int32) {
	state := s.chartState()
	if state == nil || state.confirm || delta == 0 {
		return
	}
	state.scrollY += int(delta) * 80
	s.chartClampScroll(state)
	s.composer.Invalidate()
}

// dragChart 处理 CHART 的二维拖动。
//
// 手指向右/向下拖表示"抓住地图往右/下挪"，所以滚动值是减去位移。
// 第一帧只对齐起点（pointerSystemUI 已经初始化 lastDragX/Y）。
func (s *Session) dragChart(x, y int) bool {
	ui := s.ui
	if ui == nil || ui.chart == nil {
		return false
	}
	state := ui.chart
	lastX, lastY := ui.lastDragX, ui.lastDragY
	ui.lastDragX, ui.lastDragY = x, y
	if state.confirm || lastX < 0 || lastY < 0 {
		return false
	}
	beforeX, beforeY := state.scrollX, state.scrollY
	state.scrollX += lastX - x
	state.scrollY += lastY - y
	s.chartClampScroll(state)
	return state.scrollX != beforeX || state.scrollY != beforeY
}

// ---------------------------------------------------------------- 小工具

func (s *Session) chartState() *chartUIState {
	if s.ui == nil {
		return nil
	}
	return s.ui.chart
}

// chartPage 返回当前页；页码越界时返回 nil。
func (state *chartUIState) chartPage() *config.ChartPage {
	if state == nil || state.db == nil || state.page < 0 || state.page >= len(state.db.Pages) {
		return nil
	}
	return &state.db.Pages[state.page]
}

func isChartNowSceneButton(button config.ConfigButton) bool {
	return strings.Contains(button.Name, "now_scene")
}

func chartUIButton(button config.ConfigButton) config.UIButton {
	return config.UIButton{
		Image:  button.Name,
		Frames: max(1, button.MaxState),
		X:      button.X,
		Y:      button.Y,
	}
}

// chartAbs 是 CHART 专用的整数绝对值。
// 不用 config 包的 absInt：那个是布局解析器内部函数，跨包引用会把两个
// 无关模块绑在一起；也不用浮点 math.Abs，避免把像素坐标来回转换。
func chartAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
