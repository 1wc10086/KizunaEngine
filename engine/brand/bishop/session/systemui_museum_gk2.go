package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// systemui_museum_gk2.go 是学園2 老版（BSScript Rev.2）鉴赏室的渲染与交互。
//
// 老版 bsxx.cfg 没有鉴赏室坐标，组页签只能靠 cg_hit 的像素值判断：
// 页签之间是斜切的，外接矩形会串组，所以必须按屏幕像素查掩码。
// 所有入口都以 Museum.Gakuen2 为前提，旧作品的绘制和命中不会走到这里。

const (
	gk2MuseumGroupFirstValue = 96
	gk2MuseumGroupStep       = 3
	gk2MuseumGroupCount      = 7
	gk2MuseumPageSize        = 6
	gk2ThumbW                = 120
	gk2ThumbH                = 150
	gk2MusicTrackCount       = 22
)

var gk2MuseumThumbPoints = [gk2MuseumPageSize]config.UIPoint{
	{X: 500, Y: 40}, {X: 650, Y: 40},
	{X: 500, Y: 210}, {X: 650, Y: 210},
	{X: 500, Y: 380}, {X: 650, Y: 380},
}

// 每组条带的素材名，实际资源为 <名字>_thumb，顺序即该组的显示顺序。
var gk2CGStrips = [gk2MuseumGroupCount][]string{
	{"cg0", "cg1", "cg2"}, {"cg3", "cg4", "cg5"}, {"cg6", "cg7", "cg8"},
	{"cg9", "cg10", "cg11"}, {"cg12"}, {"cg15"}, {"cg18"},
}

var gk2SceneStrips = [gk2MuseumGroupCount][]string{
	{"scene0", "scene1"}, {"scene3", "scene4"}, {"scene6", "scene7"},
	{"scene9", "scene10"}, {"scene12"}, {"scene15"}, {"scene18"},
}

type gk2Rect struct{ x, y, w, h int }

func (r gk2Rect) contains(x, y int) bool {
	return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h
}

var (
	gk2CGExitRect      = gk2Rect{0, 530, 200, 60}
	gk2CGPrevRect      = gk2Rect{0, 200, 80, 200}
	gk2CGNextRect      = gk2Rect{720, 200, 80, 200}
	gk2PrevPageRect    = gk2Rect{500, 540, 40, 40}
	gk2NextPageRect    = gk2Rect{748, 540, 40, 40}
	gk2MusicExitRect   = gk2Rect{0, 540, 190, 60}
	gk2MusicMinRect    = gk2Rect{286, 555, 30, 28}
	gk2MusicMaxRect    = gk2Rect{500, 555, 30, 28}
	gk2MusicVolumeRect = gk2Rect{320, 561, 180, 18}
)

// gk2MuseumPage 保存鉴赏室的掩码缓存与当前组的页码。
// 掩码解码一次就够，之后鼠标每次移动只查表；页码在切换组时清零。
type gk2MuseumPage struct {
	loaded bool
	ok     bool
	width  int
	height int
	mask   []byte
	boxes  [gk2MuseumGroupCount]gk2Rect
	page   int
}

// gk2GroupOfValue 把掩码像素值映射为组下标：96..114 每隔 3 一组，其它值不是页签。
func gk2GroupOfValue(value int) int {
	if value < gk2MuseumGroupFirstValue || value > gk2MuseumGroupFirstValue+3*(gk2MuseumGroupCount-1) {
		return -1
	}
	if (value-gk2MuseumGroupFirstValue)%gk2MuseumGroupStep != 0 {
		return -1
	}
	return (value - gk2MuseumGroupFirstValue) / gk2MuseumGroupStep
}

// gk2DecodeGroupMask 解码 cg_hit 为屏幕方向的 8 位索引缓冲。
// 图头的第 0 行在底部，读取时要逐行翻转，否则页签上下颠倒。
func (s *Session) gk2DecodeGroupMask() (int, int, []byte, bool) {
	if s.res == nil {
		return 0, 0, nil, false
	}
	location, ok := s.res.TryGet("cg_hit")
	if !ok || location == nil {
		return 0, 0, nil, false
	}
	data := location.Read()
	if len(data) == 0 || !gfx.IsBsgGraphics(data, 0) {
		return 0, 0, nil, false
	}
	header, err := gfx.ParseBsgHeader(data, 0)
	if err != nil || header.ColorMode != gfx.BsgColorModeIndexed8 {
		return 0, 0, nil, false
	}
	width, height := header.Width, header.Height
	if width <= 0 || height <= 0 || header.PixelCount() != width*height {
		return 0, 0, nil, false
	}
	end := header.DataOffset + header.DataSize
	if header.DataOffset < 0 || header.DataOffset > end || end > len(data) {
		return 0, 0, nil, false
	}
	buffer := make([]byte, width*height)
	source := data[header.DataOffset:end]
	switch header.Compression {
	case gfx.BsgCompressionNone:
		copy(buffer, source)
	case gfx.BsgCompressionLz:
		gfx.UnpackBsgLz(source, buffer, 0, 1)
	default:
		gfx.UnpackBsgRle(source, buffer, 0, 1)
	}
	mask := make([]byte, width*height)
	for row := 0; row < height; row++ {
		copy(mask[(height-1-row)*width:(height-row)*width], buffer[row*width:(row+1)*width])
	}
	return width, height, mask, true
}

// gk2GroupBoxes 求每个组页签的外接矩形，只用于画选中/悬停边框；命中仍按像素判定。
func gk2GroupBoxes(width, height int, mask []byte) [gk2MuseumGroupCount]gk2Rect {
	var minX, minY, maxX, maxY [gk2MuseumGroupCount]int
	for g := range minX {
		minX[g], minY[g], maxX[g], maxY[g] = width, height, -1, -1
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			g := gk2GroupOfValue(int(mask[y*width+x]))
			if g < 0 {
				continue
			}
			minX[g] = min(minX[g], x)
			minY[g] = min(minY[g], y)
			maxX[g] = max(maxX[g], x)
			maxY[g] = max(maxY[g], y)
		}
	}
	var boxes [gk2MuseumGroupCount]gk2Rect
	for g := range boxes {
		if maxX[g] < 0 {
			continue
		}
		boxes[g] = gk2Rect{minX[g], minY[g], maxX[g] - minX[g] + 1, maxY[g] - minY[g] + 1}
	}
	return boxes
}

func (s *Session) gk2EnsureMask(state *gk2MuseumPage) bool {
	if state.loaded {
		return state.ok
	}
	state.loaded = true
	width, height, mask, ok := s.gk2DecodeGroupMask()
	if !ok {
		core.Logf("ui: gk2 museum cg_hit mask unavailable")
		return false
	}
	state.width, state.height, state.mask = width, height, mask
	state.boxes = gk2GroupBoxes(width, height, mask)
	state.ok = true
	return true
}

// gk2TabAt 返回屏幕坐标 (x,y) 处的组页签，不在页签上返回 -1。
func gk2TabAt(state *gk2MuseumPage, x, y int) int {
	if !state.ok || x < 0 || y < 0 || x >= state.width || y >= state.height {
		return -1
	}
	return gk2GroupOfValue(int(state.mask[y*state.width+x]))
}

func gk2PageCount(count int) int {
	if count <= 0 {
		return 1
	}
	return (count + gk2MuseumPageSize - 1) / gk2MuseumPageSize
}

// gk2GroupItems 返回某组的条目列表：CG 为组内格下标，场景为 scene.dat 里的下标。
func (s *Session) gk2GroupItems(scene bool, group int) []int {
	if scene {
		catalog := s.ensureScenes()
		if catalog == nil {
			return nil
		}
		var out []int
		for index := range catalog.Scenes {
			if catalog.Scenes[index].Group == group {
				out = append(out, index)
			}
		}
		return out
	}
	catalog := s.ensureCg()
	if catalog == nil || group < 0 || group >= len(catalog.Groups) {
		return nil
	}
	out := make([]int, len(catalog.Groups[group].Entries))
	for index := range out {
		out[index] = index
	}
	return out
}

func (s *Session) gk2ItemUnlocked(scene bool, group, index int) bool {
	if scene {
		return s.sceneUnlockedAt(index)
	}
	catalog := s.ensureCg()
	if catalog == nil || group < 0 || group >= len(catalog.Groups) {
		return false
	}
	entries := catalog.Groups[group].Entries
	if index < 0 || index >= len(entries) {
		return false
	}
	return s.cgEntryUnlocked(entries[index])
}

// gk2ComposeThumb 为条带之外的格子现场合成缩略图。
//
// 条带只收录发行版预先裁好的部分 CG；目录里多出来的场景/差分仍然要能浏览，
// 这时退回通用的 thumbnailCanvasSized（内部有缓存，翻回同一页不会重复解码）。
func (s *Session) gk2ComposeThumb(scene bool, group, index int) *gfx.BgraCanvas {
	if scene {
		catalog := s.ensureScenes()
		if catalog == nil || index < 0 || index >= len(catalog.Scenes) {
			return nil
		}
		return s.thumbnailCanvasSized(catalog.Scenes[index].Images, gk2ThumbW, gk2ThumbH)
	}
	catalog := s.ensureCg()
	if catalog == nil || group < 0 || group >= len(catalog.Groups) {
		return nil
	}
	entries := catalog.Groups[group].Entries
	if index < 0 || index >= len(entries) {
		return nil
	}
	return s.thumbnailCanvasSized(entryThumbImages(entries[index]), gk2ThumbW, gk2ThumbH)
}

// gk2Frame 是条带里的一帧：某张条带图的第 idx 帧，帧高 height。
type gk2Frame struct {
	image  *gfx.BsgImage
	index  int
	height int
}

// gk2StripFrames 把一组的条带按顺序展开为帧列表。
//
// 每条条带的帧数不是固定的：竖构图 CG 是 120x150，横构图是 120x90，
// 所以不能拿“组内条目数”去切第一张图（cg0 有 6 帧、cg2 只有 2 帧）。
// 素材高度总是 90 的倍数，优先按 150 判断，否则退回 90；这样同一条带内
// 不会把一张 180 高的横构图切成两张残图。
func (s *Session) gk2StripFrames(strips []string) []gk2Frame {
	if s.composer == nil {
		return nil
	}
	var frames []gk2Frame
	for _, name := range strips {
		image := s.composer.Load(name + "_thumb")
		if image == nil {
			continue
		}
		frameHeight := 0
		switch {
		case image.Height%150 == 0:
			frameHeight = 150
		case image.Height%90 == 0:
			frameHeight = 90
		}
		if frameHeight <= 0 {
			continue
		}
		for index := 0; index < image.Height/frameHeight; index++ {
			frames = append(frames, gk2Frame{image: image, index: index, height: frameHeight})
		}
	}
	return frames
}

// ---------------------------------------------------------------- 绘制

func gk2TabKind(scene bool) uiHitKind {
	if scene {
		return hitScenePage
	}
	return hitCGPage
}

func gk2ThumbKind(scene bool) uiHitKind {
	if scene {
		return hitSceneThumb
	}
	return hitCGThumb
}

// drawGK2Museum 画老版 CG（scene=false）或 Scene（scene=true）鉴赏页。
func (s *Session) drawGK2Museum(canvas *gfx.BgraCanvas, scene bool) {
	ui := s.ui
	if ui == nil {
		return
	}
	base, group := "cg_base", ui.cgGroup
	if scene {
		base, group = "scene_base", ui.scenePage
	}
	group = clampInt(group, 0, gk2MuseumGroupCount-1)
	s.drawLayerAt(canvas, base, 0, 0)

	state := &ui.gk2Page
	if s.gk2EnsureMask(state) {
		for g := 0; g < gk2MuseumGroupCount; g++ {
			box := state.boxes[g]
			if box.w == 0 {
				continue
			}
			// 只在悬停时描边：选中态的整身剪影外接框非常大，常驻描边会像
			// 一个把角色框住的白框，反而不像原版。
			if ui.hover.kind != gk2TabKind(scene) || ui.hover.index != g {
				continue
			}
			gk2DrawBorder(canvas, box, 2)
		}
	}

	items := s.gk2GroupItems(scene, group)
	strips := gk2CGStrips[group]
	if scene {
		strips = gk2SceneStrips[group]
	}
	frames := s.gk2StripFrames(strips)
	// 老素材的条带只收录了一部分 CG；超出条带的格子从完整图像现场合成缩略图，
	// 这样每一格都能点开，而不是翻到第二页就只剩空白。
	visible := len(items)
	pages := gk2PageCount(visible)
	state.page = clampInt(state.page, 0, pages-1)
	for k, point := range gk2MuseumThumbPoints {
		n := state.page*gk2MuseumPageSize + k
		if n >= visible {
			break
		}
		cellHeight := gk2ThumbH
		y := point.Y
		if n < len(frames) {
			frame := frames[n]
			cellHeight = frame.height
			y = point.Y + (gk2ThumbH-frame.height)/2
			canvas.DrawRegion(frame.image, 0, frame.index*frame.height, frame.image.Width, frame.height, point.X, y)
		} else {
			thumb := s.gk2ComposeThumb(scene, group, items[n])
			if thumb == nil {
				continue
			}
			canvas.BlitCanvasRegion(thumb, 0, 0, thumb.Width, thumb.Height, point.X, y, 255)
			cellHeight = thumb.Height
		}
		if !s.gk2ItemUnlocked(scene, group, items[n]) {
			canvas.FillBlend(point.X, y, gk2ThumbW, cellHeight, 0, 0, 0, 176)
		}
		if ui.hover.kind == gk2ThumbKind(scene) && ui.hover.index == k {
			canvas.FillBlend(point.X-1, y-1, gk2ThumbW+2, 2, 255, 255, 255, 200)
			canvas.FillBlend(point.X-1, y+cellHeight-1, gk2ThumbW+2, 2, 255, 255, 255, 200)
		}
	}

	if pages > 1 {
		s.drawAlignedText(canvas, "<", gk2PrevPageRect.x+20, gk2PrevPageRect.y+6, 24, 0xffffffff, 2, 40)
		s.drawAlignedText(canvas, ">", gk2NextPageRect.x+20, gk2NextPageRect.y+6, 24, 0xffffffff, 2, 40)
		s.drawAlignedText(canvas, fmt.Sprintf("%d / %d", state.page+1, pages), 624, 548, 18, 0xffffffff, 2, 120)
	}
	s.drawFrameButton(canvas, gk2ExitButton(), gk2ButtonFrame(ui.hover.kind == hitCGExit))
}

// gk2ExitButton 是老版 CG/Scene 页与查看器共用的 EXIT 按钮（2 帧：常态/悬停）。
func gk2ExitButton() config.UIButton {
	return config.UIButton{Image: "cg_exit", Frames: 2, X: 20, Y: 540}
}

func gk2ButtonFrame(hover bool) int {
	if hover {
		return 1
	}
	return 0
}

// gk2DrawBorder 用 4 条实线画矩形边框，宽度 width 像素。
func gk2DrawBorder(canvas *gfx.BgraCanvas, r gk2Rect, width int) {
	canvas.Fill(r.x, r.y, r.w, width, 255, 255, 255, 255)
	canvas.Fill(r.x, r.y+r.h-width, r.w, width, 255, 255, 255, 255)
	canvas.Fill(r.x, r.y, width, r.h, 255, 255, 255, 255)
	canvas.Fill(r.x+r.w-width, r.y, width, r.h, 255, 255, 255, 255)
}

// drawGK2Region 从同名素材里裁出屏幕矩形 rect，原位贴回（music_over/music_on 都是全屏图）。
//
// 这两张图是“黑底 + 高亮行”的色键素材，不能整块不透明贴上去：黑色会盖住
// 底图的走廊/人物。这里只复制非黑像素，等价于原引擎的黑色透明色键。
// 一屏 22 行里最多高亮一行，逐像素复制 336x32 的开销可以忽略。
func (s *Session) drawGK2Region(canvas *gfx.BgraCanvas, name string, rect gk2Rect) {
	if s.composer == nil || canvas == nil {
		return
	}
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	stride := image.Width * 4
	if stride <= 0 || len(image.Pixels) < stride*image.Height {
		return
	}
	left := max(0, rect.x)
	top := max(0, rect.y)
	right := min(image.Width, rect.x+rect.w)
	bottom := min(image.Height, rect.y+rect.h)
	right = min(right, canvas.Width)
	bottom = min(bottom, canvas.Height)
	for y := top; y < bottom; y++ {
		sourceRow := y * stride
		targetRow := y * canvas.Width * 4
		for x := left; x < right; x++ {
			source := sourceRow + x*4
			if image.Pixels[source] < 8 && image.Pixels[source+1] < 8 && image.Pixels[source+2] < 8 {
				continue
			}
			target := targetRow + x*4
			copy(canvas.Buffer[target:target+4], image.Pixels[source:source+4])
		}
	}
}

// gk2MusicRow 返回第 index 首曲目的命中/高亮矩形。左列 0..10，右列 11..21。
func gk2MusicRow(index int) gk2Rect {
	if index < 11 {
		return gk2Rect{34, 37 + 40*index, 336, 32}
	}
	return gk2Rect{423, 37 + 40*(index-11), 336, 32}
}

func (s *Session) drawGK2Music(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	s.drawLayerAt(canvas, "music_base", 0, 0)
	hover := -1
	if ui.hover.kind == hitMusicTrack {
		hover = ui.hover.index
	}
	for index := 0; index < gk2MusicTrackCount; index++ {
		switch {
		case ui.musicPlaying && index == ui.musicTrack:
			s.drawGK2Region(canvas, "music_on", gk2MusicRow(index))
		case index == hover:
			s.drawGK2Region(canvas, "music_over", gk2MusicRow(index))
		}
	}
	volume := clampVolumeSetting(s.settings.BgmVolume)
	knobX := gk2MusicVolumeRect.x + int(float64(gk2MusicVolumeRect.w-18)*volume)
	s.drawImageAt(canvas, "music_volume", knobX, gk2MusicVolumeRect.y)
}

// drawGK2CGViewer 画老版 CG 查看器：等比缩放到屏幕，留黑边。
func (s *Session) drawGK2CGViewer(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	canvas.Fill(0, 0, s.width, s.height, 0, 0, 0, 255)
	if image := ui.cgCanvas; image != nil && image.Width > 0 && image.Height > 0 && s.width > 0 && s.height > 0 {
		dw, dh := s.width, s.height
		if image.Width*s.height > image.Height*s.width {
			dh = image.Height * s.width / image.Width
		} else {
			dw = image.Width * s.height / image.Height
		}
		x, y := (s.width-dw)/2, (s.height-dh)/2
		source := image
		if dw != image.Width || dh != image.Height {
			source = image.ScaleTo(dw, dh)
		}
		if source != nil {
			canvas.BlitCanvasRegion(source, 0, 0, dw, dh, x, y, 255)
		}
	}
	s.drawFrameButton(canvas, gk2ExitButton(), gk2ButtonFrame(ui.hover.kind == hitCGExit))
	s.drawAlignedText(canvas, "<", 30, 280, 40, 0xffffffff, 0, 40)
	s.drawAlignedText(canvas, ">", 760, 280, 40, 0xffffffff, 0, 40)
	total := max(1, s.cgVariantCount())
	s.drawAlignedText(canvas, fmt.Sprintf("%d / %d", ui.cgVariant+1, total), s.width/2, 560, 18, 0xffffffff, 2, 200)
}

// ---------------------------------------------------------------- 命中

func (s *Session) hitGK2Museum(x, y int, scene bool) uiHit {
	ui := s.ui
	none := uiHit{kind: hitNone, index: -1}
	if ui == nil {
		return none
	}
	state := &ui.gk2Page
	if !s.gk2EnsureMask(state) {
		return none
	}
	if gk2CGExitRect.contains(x, y) {
		return uiHit{kind: hitCGExit}
	}
	if gk2PrevPageRect.contains(x, y) {
		return uiHit{kind: hitCGPrev}
	}
	if gk2NextPageRect.contains(x, y) {
		return uiHit{kind: hitCGNext}
	}
	if g := gk2TabAt(state, x, y); g >= 0 {
		return uiHit{kind: gk2TabKind(scene), index: g}
	}
	for k, point := range gk2MuseumThumbPoints {
		if (gk2Rect{point.X, point.Y, gk2ThumbW, gk2ThumbH}).contains(x, y) {
			return uiHit{kind: gk2ThumbKind(scene), index: k}
		}
	}
	return none
}

// hitGK2CGViewer 命中查看器的 EXIT 与左右两侧的上一张/下一张热区。
func (s *Session) hitGK2CGViewer(x, y int) uiHit {
	switch {
	case gk2CGExitRect.contains(x, y):
		return uiHit{kind: hitCGExit}
	case gk2CGPrevRect.contains(x, y):
		return uiHit{kind: hitCGPrev}
	case gk2CGNextRect.contains(x, y):
		return uiHit{kind: hitCGNext}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) hitGK2Music(x, y int) uiHit {
	switch {
	case gk2MusicExitRect.contains(x, y):
		return uiHit{kind: hitExit}
	case gk2MusicMinRect.contains(x, y):
		return uiHit{kind: hitMusicMin}
	case gk2MusicMaxRect.contains(x, y):
		return uiHit{kind: hitMusicMax}
	case gk2MusicVolumeRect.contains(x, y):
		return uiHit{kind: hitMusicVolume}
	}
	for index := 0; index < gk2MusicTrackCount; index++ {
		if gk2MusicRow(index).contains(x, y) {
			return uiHit{kind: hitMusicTrack, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// ---------------------------------------------------------------- 激活

func (s *Session) activateGK2Museum(hit uiHit, scene bool) {
	ui := s.ui
	if ui == nil {
		return
	}
	state := &ui.gk2Page
	group := ui.cgGroup
	if scene {
		group = ui.scenePage
	}
	group = clampInt(group, 0, gk2MuseumGroupCount-1)
	items := s.gk2GroupItems(scene, group)
	visible := len(items)
	pages := gk2PageCount(visible)
	switch hit.kind {
	case hitCGExit:
		// 老版从标题直接进入某个鉴赏页，没有“鉴赏主菜单”这一层；
		// EXIT 必须回标题，否则会停在一张没有底图的空 uiMuseum 上。
		s.closeSystemUI()
		return
	case hitCGPage, hitScenePage:
		if scene {
			ui.scenePage = hit.index
		} else {
			ui.cgGroup = hit.index
		}
		state.page = 0
	case hitCGPrev:
		if state.page > 0 {
			state.page--
		}
	case hitCGNext:
		if state.page < pages-1 {
			state.page++
		}
	case hitCGThumb, hitSceneThumb:
		if hit.index < 0 || hit.index >= gk2MuseumPageSize {
			return
		}
		n := state.page*gk2MuseumPageSize + hit.index
		if n >= visible || !s.gk2ItemUnlocked(scene, group, items[n]) {
			return
		}
		if scene {
			s.gk2OpenScene(items[n])
			return
		}
		ui.cgEntry = items[n]
		ui.cgVariant = 0
		ui.kind = uiCGViewer
		ui.cgCanvas = s.renderCGEntry(group, ui.cgEntry, ui.cgVariant)
	}
	s.invalidateUI()
}

// gk2OpenScene 用 scene.dat 解析出的回放标签开始场景回放；没有标签时只记日志。
func (s *Session) gk2OpenScene(index int) {
	catalog := s.ensureScenes()
	if catalog == nil || index < 0 || index >= len(catalog.Scenes) {
		return
	}
	label := catalog.Scenes[index].SceneId
	if label == "" {
		core.Logf("ui: gk2 scene %d has no replay label", index)
		return
	}
	s.startSceneReplay(label, uiMuseumScene, s.ui.scenePage)
}

func (s *Session) activateGK2Music(hit uiHit, x int) {
	ui := s.ui
	if ui == nil {
		return
	}
	switch hit.kind {
	case hitExit:
		// 音乐页的 EXIT 和 CG/Scene 页一样直接回标题。
		s.closeSystemUI()
		return
	case hitMusicTrack:
		// 再点正在播放的那一行即停止，与原版的开关语义一致。
		if ui.musicPlaying && ui.musicTrack == hit.index {
			s.gameState.ClearBgm()
			s.musicUpdate()
			break
		}
		ui.musicTrack = hit.index
		s.gameState.SetBgm(fmt.Sprintf("bgm%02d", hit.index+1))
		s.musicUpdate()
	case hitMusicMin:
		s.setMuseumBgmVolume(0)
	case hitMusicMax:
		s.setMuseumBgmVolume(1)
	case hitMusicVolume:
		s.setGK2BgmVolumeAt(x)
	}
	s.invalidateUI()
}

// setGK2BgmVolumeAt 按滑块中心的横向位置换算音量；滑块宽 18，中心偏移 9。
func (s *Session) setGK2BgmVolumeAt(x int) {
	span := gk2MusicVolumeRect.w - 18
	if span <= 0 {
		return
	}
	s.setMuseumBgmVolume(float64(x-gk2MusicVolumeRect.x-9) / float64(span))
}

// 查看器的 EXIT/Prev/Next 复用 activateCGViewer：它已经按 cgGroup/cgEntry/
// cgVariant 走 stepCGViewer，而学園2 的目录也填好了这三层的对应关系。
