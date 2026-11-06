package session

import (
	"os"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

func (s *Session) ComposeImages(names []string) (core.Frame, bool) {
	if s.composer == nil || s.width <= 0 || s.height <= 0 {
		return core.Frame{}, false
	}
	canvas := gfx.NewBgraCanvas(s.width, s.height)
	drawn := false
	for _, name := range names {
		image := s.composer.Load(name)
		if image == nil {
			continue
		}
		// 合成图（BSS-Composition）的部件坐标是屏幕绝对坐标，
		// 不能把整张合成图当作偏移为 0 的单图来画。
		if s.drawLayerScaled(canvas, image, 1, 1) {
			drawn = true
		}
	}
	if !drawn {
		return core.Frame{}, false
	}
	out := canvas.ToImage(0, 0)
	if out == nil {
		return core.Frame{}, false
	}
	return core.Frame{Width: out.Width, Height: out.Height, Pixels: out.Pixels}, true
}

func (s *Session) SetPreview(f core.Frame) {
	if s.composer == nil || s.gameState == nil {
		return
	}
	if f.Width <= 0 || f.Height <= 0 || len(f.Pixels) < f.Width*f.Height*4 {
		s.composer.Frame().Clear(0, 0, 0, 255)
		s.gameState.SetFillAlpha(0)
		return
	}
	image := gfx.NewBsgImage(f.Width, f.Height, 0, 0, f.Pixels)
	s.composer.Frame().Clear(0, 0, 0, 255)
	s.composer.DrawImage(image, 0, 0)
	s.gameState.SetFillAlpha(0)
}

func (s *Session) SelectFrame(name string) (core.Frame, bool) {
	if s.composer == nil {
		return core.Frame{}, false
	}
	image := s.composer.Images.Get(name)
	if image == nil {
		return core.Frame{}, false
	}
	frames := image.FrameCount
	if frames < 2 {
		frames = 2
	}
	if image.Width <= 0 || image.Height <= 0 || frames <= 0 {
		return core.Frame{}, false
	}
	unit := image.Height / frames
	if unit <= 0 {
		return core.Frame{}, false
	}
	stride := image.Stride()
	if stride <= 0 {
		stride = image.Width * 4
	}
	need := 2 * unit * stride
	if len(image.Pixels) < need {
		return core.Frame{}, false
	}
	pixels := append([]byte(nil), image.Pixels[:need]...)
	return core.Frame{Width: image.Width, Height: 2 * unit, Pixels: pixels}, true
}

func (s *Session) CgGroups() []core.CgGroup {
	catalog := s.ensureCg()
	if catalog == nil {
		return nil
	}
	out := make([]core.CgGroup, 0, len(catalog.Groups))
	for _, group := range catalog.Groups {
		variants := make([][]string, 0, len(group.Variants))
		for _, variant := range group.Variants {
			variants = append(variants, append([]string(nil), variant.Images...))
		}
		out = append(out, core.CgGroup{Variants: variants})
	}
	return out
}

func (s *Session) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	if s.saves == nil {
		return core.SlotPreview{}, false
	}
	stored := s.saves.Read(int(slot))
	if stored == nil {
		return core.SlotPreview{}, false
	}
	preview := core.SlotPreview{Summary: stored.Summary, Speaker: stored.Speaker, Text: stored.Text}
	if !stored.SavedAt.IsZero() {
		preview.SavedAt = stored.SavedAt.Format(time.RFC3339)
	}
	if stored.ThumbnailWidth > 0 && stored.ThumbnailHeight > 0 && len(stored.Thumbnail) > 0 {
		preview.Thumb = core.Frame{
			Width:  stored.ThumbnailWidth,
			Height: stored.ThumbnailHeight,
			Pixels: append([]byte(nil), stored.Thumbnail...),
		}
	}
	return preview, true
}

func (s *Session) SetOption(key uint32, value bool) {
	if s.gameState == nil {
		return
	}
	if key == 0 {
		if s.gameState.ShowMessageFrame() != value {
			s.gameState.SetShowMessageFrame(value)
			// 切换对话框由谁绘制时必须立刻重绘：否则引擎窗口会残留在帧位图上，
			// 或 Flutter 已经隐藏 MessageBox 而引擎还没画。
			s.composer.Invalidate()
		}
		return
	}
	if key == 1 {
		// 宿主"跳过中"状态：引擎据此画原版 SKIPPING 条。
		s.skipOverlay = value
		if s.composer != nil {
			s.composer.SetSkipOverlay(value)
		}
	}
}

func (s *Session) buildCatalog() []core.Scene {
	story := storyEntryCandidates[0]
	for _, candidate := range storyEntryCandidates {
		if s.script.FindLabel(candidate) >= 0 {
			story = candidate
			break
		}
	}
	out := []core.Scene{{ID: 0, Name: story}}
	catalog := s.ensureScenes()
	if catalog == nil {
		return out
	}
	for _, scene := range catalog.Scenes {
		// 宿主回调 Start(entry) 时用这个名字去跑回放；必须是真实标签
		// （SceneId=L01_001_replay），label_01_001 只是解锁标识。
		name := scene.SceneId
		if len(name) == 0 {
			name = scene.Label
		}
		entry := core.Scene{Name: name, Title: scene.Title, Images: append([]string(nil), scene.Images...)}
		entry.ID = uint32(len(out))
		if scene.Group >= 0 {
			entry.Group = uint32(scene.Group)
		}
		if scene.Index >= 0 {
			entry.Index = uint32(scene.Index)
		}
		out = append(out, entry)
	}
	return out
}

func (s *Session) ensureCg() *data.CgCatalog {
	if s.cgDone {
		return s.cg
	}
	s.cgDone = true
	// 学園2 老版的 cg.dat 不是现代分布版的布局解析器能读的格式；资源名
	// g<角色><场景><差分> 本身就是完整目录，直接按名字重建更快也更可靠。
	if s.config != nil && s.config.Museum.Gakuen2 {
		s.cg = s.buildGK2CgCatalog()
		// 老版没有现代意义的 save/common.dat，引擎无法把解锁位映射到
		// g<角色><场景><子号> 的目录顺序；按原版“发行版自带全解锁”处理，
		// 否则玩家点任何一格都会被解锁判定挡回去。
		s.unlockAll = true
		return s.cg
	}
	path := findFileInRoots(s.resourceLookupRoots(), "cg.dat")
	if path == "" {
		// 下载版/移植版可能没有 cg.dat（黒の教室 モーションプラス）。
		// 用 EV 图层名重建目录；拿不到原名字表顺序，所以同时标记 unlockAll。
		s.cg = s.fallbackCgCatalog()
		s.unlockAll = true
		return s.cg
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		core.Logf("catalog: cg.dat failed: %s", err.Error())
		return nil
	}
	catalog, err := data.ParseCgCatalog(raw)
	if err != nil || catalog == nil || catalog.VariantCount() == 0 {
		core.Logf("catalog: cg.dat layout not recognised")
		return nil
	}
	core.Logf("catalog: cg.dat groups=%d variants=%d", len(catalog.Groups), catalog.VariantCount())
	s.cg = catalog
	return s.cg
}

// museumScenePageCount 返回场景鉴赏可显示的页数。
//
// 页签图标就是原引擎的分页：淫獄/侵蝕 是 6 个角色事件页 + 6 个通常场景页，
// 黒の教室 是 5 名角色 + 「その他」共 6 页。图标数量取自 bsxx.cfg，
// 万一配置缺失才退回 6 页；上限 12 防止损坏配置画出满屏页签。
func (s *Session) museumScenePageCount() int {
	pages := 0
	if s.config != nil {
		pages = len(s.config.Museum.Scene.PageButtons)
	}
	if pages < 1 {
		pages = 6
	}
	if pages > 12 {
		pages = 12
	}
	return pages
}

func (s *Session) ensureScenes() *data.SceneCatalog {
	if s.scenesDone {
		return s.scenes
	}
	s.scenesDone = true
	// 学園2 老版的 scene.dat 是 19 个原始分组；鉴赏页只有 7 个角色页签，
	// 必须按图名前缀重映射并补上真实回放标签。
	if s.config != nil && s.config.Museum.Gakuen2 {
		s.scenes = s.loadGK2SceneCatalog()
		s.unlockAll = true
		return s.scenes
	}
	path := findFileInRoots(s.resourceLookupRoots(), "scene.dat")
	if path == "" {
		// 同上：按 EV 图名重建场景表，并标记 unlockAll。
		s.scenes = s.fallbackSceneCatalog()
		s.unlockAll = true
		return s.scenes
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		core.Logf("catalog: scene.dat failed: %s", err.Error())
		return nil
	}
	var titles []byte
	if location, ok := s.res.TryGet("scene_name"); ok && location != nil {
		titles = location.Read()
	}
	s.scenes = data.ParseSceneCatalog(raw, titles)
	// Tbxdl 的 scene.dat 只有 "group,index,sceneId" 三列，sceneId 是剧本标签
	// 而不是图片名（htm_iv01_00）。不补图片列，场景鉴赏会整页黑格。
	s.fillAncientSceneImages()
	core.Logf("catalog: scene.dat scenes=%d", len(s.scenes.Scenes))
	return s.scenes
}

// fillAncientSceneImages 为三列 scene.dat 补齐场景缩略图。
//
// Tbxdl 的 scene.dat 只有 "group,index,sceneId" 三列，sceneId 是剧本标签
// （htm_iv01_00）而不是图片名。cg.dat 的文本目录按"组 -> 格 -> 变体"排列，
// 与 scene.dat 的扁平顺序一一对应（5 组共 100 格；前 55 格对应 55 个场景）。
// 直接按扁平下标取 cg.dat 的第一变体，既不用逐标签跑脚本，也能显示原版封面。
func (s *Session) fillAncientSceneImages() {
	if s.scenes == nil || len(s.scenes.Scenes) == 0 {
		return
	}
	first := s.scenes.Scenes[0]
	if len(first.Images) != 1 || first.Images[0] != first.SceneId {
		return
	}
	if s.res != nil && s.res.Exists(first.Images[0]) {
		return
	}
	catalog := s.ensureCg()
	if catalog == nil {
		return
	}
	flat := make([][]string, 0, 128)
	for _, group := range catalog.Groups {
		for _, entry := range group.Entries {
			if len(entry.Variants) > 0 && len(entry.Variants[0].Images) > 0 {
				flat = append(flat, entry.Variants[0].Images)
			} else {
				flat = append(flat, nil)
			}
		}
	}
	filled := 0
	for index := range s.scenes.Scenes {
		if index >= len(flat) || len(flat[index]) == 0 {
			continue
		}
		s.scenes.Scenes[index].Images = append([]string(nil), flat[index]...)
		filled++
	}
	core.Logf("catalog: ancient scene thumbnails filled=%d/%d", filled, len(s.scenes.Scenes))
}

func (s *Session) startReplay(sceneID string) bool {
	label := s.resolveReplayLabel(sceneID)
	offset := -1
	if len(label) > 0 {
		offset = s.script.FindLabel(label)
	}
	if offset < 0 {
		return false
	}
	s.gameState.Reset()
	s.clearMapStage()
	s.machine.RestoreState(vm.EntryState(offset))
	s.replay = true
	// CHART 在回放期间也要把"进行中"标在当前场景上。
	s.currentSceneLabel = strings.TrimSuffix(label, replaySuffix)
	s.resetAudioCache()
	core.Logf("replay: start '%s'", label)
	return true
}

func (s *Session) resolveReplayLabel(sceneID string) string {
	if s.script.FindLabel(sceneID) >= 0 {
		return sceneID
	}
	if strings.HasSuffix(sceneID, replaySuffix) {
		plain := strings.TrimSuffix(sceneID, replaySuffix)
		if s.script.FindLabel(plain) >= 0 {
			return plain
		}
		return ""
	}
	wrapper := sceneID + replaySuffix
	if s.script.FindLabel(wrapper) >= 0 {
		return wrapper
	}
	return ""
}
