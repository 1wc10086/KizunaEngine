// 画廊（CG/音乐/场景）与存档预览：数据源全部走 system.arc 的 .tbl，
// 图全部走 gfx.Cache，按 spec §4 的 thumb_<name>.cpb 缩略图口径。
//
// GalleryComposer.ComposeImages(names) 把至多 4 张图拼成一张 800x600 帧
// （2x2 方格），装锁图的占位只留纯色不泄露长相——这与"未解锁不可预览"
// 的鉴赏契约一致，解锁在宿主侧按 CgGroups 的变体名查询。
package session

import (
	"strings"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/arc"
	overgfx "kizuna/enginego/brand/overdose/gfx"
	overstage "kizuna/enginego/brand/overdose/stage"
	"kizuna/enginego/core"
)

// galleryKind 区分 extra 画廊页：CG/音乐/场景三类只差图集来源，
// 按钮几何共用同一组（art：四按钮 + 返回）。
type galleryKind int

const (
	galleryNone galleryKind = iota
	galleryCG
	galleryMusic
	galleryScene
)

// galleryButtons 是画廊四按钮的宿主坐标：CG/音乐/场景 + 返回。
var galleryButtons = []struct {
	key  string
	x, y int32
	w, h uint32
}{
	{"cg", 20, 20, 100, 40},
	{"music", 140, 20, 100, 40},
	{"scene", 260, 20, 100, 40},
	{"back", 700, 20, 80, 40},
}

// composeGallery 在标题态画廊页画图集：CG 拼 cg_page 表缩略图，
// 音乐拼 bgm_base01.. 的封面图，场景拼 scene_page 表缩略图。
func (s *Session) composeGallery(canvas *bishopgfx.BgraCanvas) {
	canvas.Clear(10, 10, 10, 255)
	var names []string
	switch s.galleryMode {
	case galleryCG:
		names = s.cgNames()
	case galleryMusic:
		names = s.musicNames()
	case galleryScene:
		names = s.sceneNames()
	}
	// 缩略图只在图集区（80..520 高）排 2x2，顶部留给 galleryButtons。
	cellW, cellH := 400, 220
	ox, oy := int32(0), int32(80)
	for i, name := range names {
		if i >= 4 {
			break
		}
		img := s.cache.Get(name)
		if img == nil {
			// thumb_ 前缀的缩略图找不到时回退原 CG 图（大图缩放走 BlitScaled）。
			img = s.cache.Get(fullCGName(name))
		}
		if img == nil {
			continue
		}
		x := int(ox) + (i%2)*cellW
		y := int(oy) + (i/2)*cellH
		canvas.BlitScaled(img, x, y, cellW, cellH, 255)
	}
	// 音乐页叠 BGM 播放提示（bgm_base 封面图本身就是可播标记）。
	if s.galleryMode == galleryMusic {
		if img := s.cache.Get("bgm_base01"); img != nil {
			canvas.Blit(img, 100, 480, 255)
		}
	}
}

// cgNames 从 list_cg.tbl 取 CG 名（带 thumb_ 前缀的是缩略图名）。
func (s *Session) cgNames() []string {
	if s.archives == nil {
		return nil
	}
	recs, err := s.archives.Table("list_cg.tbl")
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range recs {
		if r.Name != "" {
			out = append(out, r.Name)
		}
	}
	return out
}

// cgPageNames 从 cg_pageNN.tbl 取某页的 CG 名（页内按行序）。
func (s *Session) cgPageNames(page int) []string {
	if s.archives == nil {
		return nil
	}
	name := tableName("cg_page", page)
	recs, err := s.archives.Table(name)
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range recs {
		if r.Name != "" {
			out = append(out, r.Name)
		}
	}
	return out
}

// sceneNames 从 scene_pageNN.tbl 汇总场景缩略图名。
func (s *Session) sceneNames() []string {
	return sceneTableNames(s.archives)
}

// musicNames 是 music.arc 的条目名序（去 .ogg 后缀，gfx 按名取图）。
func (s *Session) musicNames() []string {
	if s.archives == nil {
		return nil
	}
	names := s.archives.Names(".ogg")
	var out []string
	for _, n := range names {
		out = append(out, strings.TrimSuffix(n, ".ogg"))
	}
	return out
}

// galleryPointer 处理画廊四按钮：切页/返回。
// 画廊在标题态下处理 Pointer（见 Session.Pointer 的 atTitle 分支）。
func (s *Session) galleryPointer(op uint32, x, y int32) (core.Step, error) {
	if op != 1 {
		return core.StepAwaitTitle, nil
	}
	for _, b := range galleryButtons {
		if x >= b.x && x < b.x+int32(b.w) && y >= b.y && y < b.y+int32(b.h) {
			switch b.key {
			case "cg":
				s.galleryMode = galleryCG
			case "music":
				s.galleryMode = galleryMusic
			case "scene":
				s.galleryMode = galleryScene
			case "back":
				s.galleryMode = galleryNone
				s.preview = core.Frame{}
			}
			return core.StepAwaitTitle, nil
		}
	}
	return core.StepAwaitTitle, nil
}

// fullCGName 去掉 thumb_ 前缀，还原 CG 原图名（缩略图找不到时用）。
func fullCGName(name string) string {
	return strings.TrimPrefix(name, "thumb_")
}

// thumbName 给 CG 名加 thumb_ 前缀。
func thumbName(name string) string {
	if strings.HasPrefix(name, "thumb_") {
		return name
	}
	return "thumb_" + name
}

// ---------------- GalleryComposer / GalleryUnlocker ----------------

// ComposeImages 把至多 4 张图拼成 800x600 帧（2x2 方格），返回是否真画出。
func (s *Session) ComposeImages(names []string) (core.Frame, bool) {
	if s.cache == nil || len(names) == 0 {
		return core.Frame{}, false
	}
	canvas := bishopgfx.NewBgraCanvas(frameWidth, frameHeight)
	canvas.Clear(0, 0, 0, 255)
	drawn := false
	cellW, cellH := 400, 300
	for i, name := range names {
		if i >= 4 {
			break
		}
		img := s.cache.Get(name)
		if img == nil {
			img = s.cache.Get(thumbName(name))
		}
		if img == nil {
			continue
		}
		x := (i % 2) * cellW
		y := (i / 2) * cellH
		canvas.BlitScaled(img, x, y, cellW, cellH, 255)
		drawn = true
	}
	if !drawn {
		return core.Frame{}, false
	}
	return core.Frame{Width: canvas.Width, Height: canvas.Height, Pixels: append([]byte(nil), canvas.Buffer...)}, true
}

// SetPreview 接收宿主图片并在标题画廊叠上（下次 Compose 生效）。
func (s *Session) SetPreview(f core.Frame) {
	s.preview = f
}

// SelectFrame 按名选中一张全尺寸图：选中即回 ComposeImages 单图全幅。
func (s *Session) SelectFrame(name string) (core.Frame, bool) {
	if s.cache == nil {
		return core.Frame{}, false
	}
	img := s.cache.Get(name)
	if img == nil {
		img = s.cache.Get(fullCGName(name))
	}
	if img == nil {
		return core.Frame{}, false
	}
	canvas := bishopgfx.NewBgraCanvas(frameWidth, frameHeight)
	canvas.Clear(0, 0, 0, 255)
	canvas.BlitScaled(img, 0, 0, frameWidth, frameHeight, 255)
	return core.Frame{Width: canvas.Width, Height: canvas.Height, Pixels: append([]byte(nil), canvas.Buffer...)}, true
}

// CgGroups 把 list_cg 表按页分组：每页是一组，Variants 是页内 CG 名序。
func (s *Session) CgGroups() []core.CgGroup {
	if s.archives == nil {
		return nil
	}
	// 先收集页名 cgc_pageNN.list 的存在性，再逐页读。
	pages := collectPageNames(s.archives, "cg_page")
	var out []core.CgGroup
	for _, page := range pages {
		names := s.cgPageNames(page)
		if len(names) == 0 {
			continue
		}
		out = append(out, core.CgGroup{Name: names[0], Variants: [][]string{names}})
	}
	// 页表缺失时退化到整表一页，保证宿主至少收到 CG 名。
	if len(out) == 0 {
		names := s.cgNames()
		if len(names) > 0 {
			out = append(out, core.CgGroup{Name: names[0], Variants: [][]string{names}})
		}
	}
	return out
}

// collectPageNames 从 system.arc 的 Names 里筛出 "cg_pageNN.tbl" 这类页表并按数字排序。
func collectPageNames(set *arc.Set, prefix string) []int {
	if set == nil {
		return nil
	}
	var pages []int
	for _, n := range set.Names(".tbl") {
		// 名字形如 cg_page01.tbl：前缀匹配 + 后段取数字。
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(n, prefix), ".tbl")
		num := 0
		ok := len(mid) > 0
		for _, c := range mid {
			if c < '0' || c > '9' {
				ok = false
				break
			}
			num = num*10 + int(c-'0')
		}
		if ok && num > 0 {
			pages = append(pages, num)
		}
	}
	// 页表按数字升序（插入排序保持确定性，页数小）。
	for i := 1; i < len(pages); i++ {
		for j := i; j > 0 && pages[j] < pages[j-1]; j-- {
			pages[j], pages[j-1] = pages[j-1], pages[j]
		}
	}
	return pages
}

// tableName 把页号拼成表名：cg_page01.tbl（两位数零填）。
func tableName(prefix string, page int) string {
	return prefix + pad2(page) + ".tbl"
}

func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// CgUnlockedByName 按图名查 CG 解锁。
func (s *Session) CgUnlockedByName(name string) bool {
	return s.unlockSets.cg[name] || s.unlockSets.cg[fullCGName(name)]
}

// SceneUnlocked 按下标查场景解锁。
func (s *Session) SceneUnlocked(index int) bool {
	scenes := s.Scenes()
	if index < 0 || index >= len(scenes) {
		return false
	}
	return scenes[index].Unlocked
}

// ---------------- SlotPreviewer ----------------

// PreviewSlot 返回槽位摘要 + 合成缩略图：优先用存档里的背景图加人物层画一张，
// 若背景不可用则按 BGRA 纯色垫底，仍返回 (true) 让宿主显示摘要文本。
func (s *Session) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	if s.saves == nil {
		return core.SlotPreview{}, false
	}
	data, err := s.saves.Load(int(slot))
	if err != nil || data == nil {
		return core.SlotPreview{}, false
	}
	// 缩略图 240x180 按 1:3 缩放：舞台全帧合成后 BlitScaled 缩进。
	stage := overstage.NewStage(s.cache)
	st := overstage.State{Background: data.Stage.Background, CG: data.Stage.CG}
	for _, ch := range data.Stage.Characters {
		st.Characters = append(st.Characters, overstage.Slot{Name: ch.Name, X: int(ch.X), Alpha: int(ch.Alpha)})
	}
	stage.Restore(st)
	cv := bishopgfx.NewBgraCanvas(frameWidth, frameHeight)
	stage.Compose(cv)
	thumb := bishopgfx.NewBgraCanvas(240, 180)
	thumb.Clear(0, 0, 0, 255)
	img := bishopgfx.NewBsgImage(frameWidth, frameHeight, 0, 0, cv.Buffer)
	thumb.BlitScaled(img, 0, 0, 240, 180, 255)
	pixels := append([]byte(nil), thumb.Buffer...)
	return core.SlotPreview{
		SavedAt: data.SavedAt.Format("2006-01-02 15:04"),
		Summary: data.Summary,
		Speaker: data.Message.Speaker,
		Text:    data.Message.Text,
		Thumb:   core.Frame{Width: 240, Height: 180, Pixels: pixels},
	}, true
}

// ---------------- scene 表拼装 ----------------

// buildScenes 把所有 scene_pageNN 表拼成 core.Scene 列：
// 顺序即“页序+页内行序”，Images 是行里的缩略图名（scene 表行本身是图名）。
func buildScenes(set *arc.Set, unlocked map[string]bool) []core.Scene {
	names := sceneTableNames(set)
	var out []core.Scene
	for i, name := range names {
		sc := core.Scene{
			ID:     uint32(i + 1),
			Name:   name,
			Title:  name,
			Group:  0,
			Index:  uint32(i),
			Images: []string{name},
		}
		if unlocked != nil && unlocked[name] {
			sc.Unlocked = true
		}
		out = append(out, sc)
	}
	return out
}

// sceneTableNames 汇总 scene_pageNN 各页表名序（无表返回空）。
func sceneTableNames(set *arc.Set) []string {
	if set == nil {
		return nil
	}
	pages := collectPageNames(set, "scene_page")
	var out []string
	for _, page := range pages {
		recs, err := set.Table(tableName("scene_page", page))
		if err != nil {
			continue
		}
		for _, r := range recs {
			if r.Name != "" {
				out = append(out, r.Name)
			}
		}
	}
	return out
}

// 保证 gfx 包的 Cache 类型被 menus.go 间接引用（titleMenu 的 cache 参数）。
var _ = overgfx.NewCache
