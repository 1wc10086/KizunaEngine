package menus

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"kizuna/enginego/brand/overdose/arc"
)

// 为什么分组键用“最后一个 '-' 之前的整段”而不是头两字母：
// list_cg.tbl 里变体名形如 cga01-01 / cgxb1-01 / cgt05-01，
// 前缀是 c g + 角色字母 + 数字（cga01、cgxb1、cgt05…），长度可变，
// 按固定宽度截会把 cgya01 与 cgya02 截成同一组或把同组拆开。
// 最后一个 '-' 之前的部分就是原引擎意义上的“同一张 CG 的不同差分”，
// 且表内顺序已按前缀排好，分组时只合并相邻同前缀、保持原序即可。
var cgVariantRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]+)-([0-9]+)$`)

// CgPrefix 返回变体名所属组的前缀；非变体名返回空。
// 后缀形如 _blur（cga02-04_blur）的名字不算变体——它在 list_cg.tbl
// 里带后缀单列，不是 cg_page 表的正条目，混进分组会让鉴赏出现
// 找不到缩略图的幽灵变体。
func CgPrefix(name string) (string, bool) {
	m := cgVariantRe.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// IsCgVariant 判断名字是否为 CG 变体（cgXX-YY 形）。
func IsCgVariant(name string) bool {
	_, ok := CgPrefix(name)
	return ok
}

// thumbVariants 把一条页表记录按“首条 thumb + 其后变体”拆开。
// cg_page 的结构是 [thumb_cgXX-YY, cgXX-YY, cgXX-YY+1, …]，
// scene_page 是 [thumb_..., script, sceneId, script]（见 BuildScenes）。
// 空名直接跳过——页表尾部大量 0 填充子记录，留着会污染分组。
func thumbVariants(names []string) (thumb string, rest []string) {
	for _, n := range names {
		if n == "" {
			continue
		}
		if strings.HasPrefix(n, "thumb_") {
			if thumb == "" {
				thumb = n
			}
			continue
		}
		rest = append(rest, n)
	}
	return thumb, rest
}

func namesOf(recs []arc.Record) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Name)
	}
	return out
}

// BuildCgGroups 从 list_cg.tbl 的记录构建 CG 分组。
//
// 只收录变体名（cgXX-YY）：表里前 3 条是 black/cinemascope/white，
// 中间混有 bg*、map_*、ed*、tak_* 等非 CG 名，它们不是鉴赏对象，
// 收进来会让 CgGroups 出现点开无图的空组。变体内部保持表顺序——
// 表顺序就是原引擎的展示顺序，排序会打乱差分次序。
func BuildCgGroups(recs []arc.Record) []CgGroup {
	var groups []CgGroup
	indexByPrefix := make(map[string]int)
	for _, r := range recs {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		prefix, ok := CgPrefix(name)
		if !ok {
			continue
		}
		if gi, exists := indexByPrefix[prefix]; exists {
			groups[gi].Variants = append(groups[gi].Variants, name)
			groups[gi].Images = append(groups[gi].Images, []string{name})
			continue
		}
		indexByPrefix[prefix] = len(groups)
		groups = append(groups, CgGroup{
			Name:     prefix,
			Variants: []string{name},
			Images:   [][]string{{name}},
		})
	}
	for i := range groups {
		if len(groups[i].Variants) > 0 {
			groups[i].Thumb = ThumbName(groups[i].Variants[0])
		}
	}
	return groups
}

// BuildScenes 从各 scene_pageNN.tbl 的记录构建场景条目。
//
// 每页记录的真实形状（实测 19 页）：
//
//	[thumb_<cg>, <script>, scene_<id>, <script>?]
//
// 例如 scene_page01 → [thumb_cga01-01, sop3, scene_a001, sop3]。
// 只有 2 条的页（如 scene_page07 [thumb_cgb13-01, sb06]）没有
// sceneId，用 script 兜底当 Title/SceneID，保证每条非空记录都有
// 可展示的条目。空名（页尾填充）不占序号。
func BuildScenes(pages []ScenePage) []SceneEntry {
	var out []SceneEntry
	idx := 0
	for _, p := range pages {
		recs := make([]string, 0, len(p.Records))
		for _, n := range p.Records {
			n = strings.TrimSpace(n)
			if n != "" {
				recs = append(recs, n)
			}
		}
		if len(recs) == 0 {
			continue
		}
		thumb := ""
		rest := make([]string, 0, len(recs))
		for _, n := range recs {
			if strings.HasPrefix(n, "thumb_") {
				if thumb == "" {
					thumb = n
				}
				continue
			}
			rest = append(rest, n)
		}
		entry := SceneEntry{
			Index: idx,
			Page:  p.Page,
			Thumb: thumb,
		}
		// rest[0] 是脚本，rest[1]（若有）是 sceneId，rest[2] 是重复的脚本。
		if len(rest) > 0 {
			entry.Script = rest[0]
		}
		if len(rest) > 1 {
			entry.SceneID = rest[1]
		}
		if entry.SceneID == "" {
			entry.SceneID = entry.Script
		}
		if entry.SceneID == "" {
			entry.SceneID = thumb
		}
		entry.Title = entry.SceneID
		if entry.Thumb != "" {
			entry.Images = []string{entry.Thumb}
		} else if entry.Script != "" {
			entry.Images = []string{entry.Script}
		}
		out = append(out, entry)
		idx++
	}
	return out
}

// BuildMusic 从 music.arc 的条目名构建曲目表。
//
// music.arc 只有 10 首（bgm01..bgm10.ogg），但 Set.Names 会把所有
// archive 的 .ogg 都扫出来（sound/voice 也叫 .ogg），所以这里只收
// bgm 前缀——否则音乐鉴赏会列出几千条语音。排序后保证 bgm01 在前，
// 原引擎的 bgm_btn1..9 按钮与此顺序对应。
func BuildMusic(names []string) []MusicTrack {
	var files []string
	for _, n := range names {
		base := strings.TrimSuffix(strings.TrimSpace(n), ".ogg")
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if !strings.HasPrefix(strings.ToLower(base), "bgm") {
			continue
		}
		files = append(files, strings.ToLower(base)+".ogg")
	}
	sort.Strings(files)
	seen := make(map[string]struct{}, len(files))
	var out []MusicTrack
	for _, f := range files {
		name := strings.TrimSuffix(f, ".ogg")
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, MusicTrack{
			Index:  len(out),
			Name:   name,
			File:   f,
			Title:  name,
			Unlock: name,
		})
	}
	return out
}

// BuildScripts 从 list_script.tbl 构建脚本表。
// 保留表顺序（150 条，main 在 147）——Start(entry) 的 entry 下标
// 与此顺序对应，排序会让 entry 指向错误的脚本。
func BuildScripts(recs []arc.Record) []ScriptEntry {
	var out []ScriptEntry
	for _, r := range recs {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		out = append(out, ScriptEntry{Index: len(out), Name: name})
	}
	return out
}

// Load 从 Source 读取全部系统表并构建 Catalog。
// 任一表缺失（系统表未烘焙/旧包）只留空、不返回错：
// session 打开时系统表缺失仍应能进标题，少掉鉴赏目录不该
// 阻断主流程（与 OpenSet “单座坏了不丢全局”同理）。
func Load(src Source) *Catalog {
	c := &Catalog{
		ScriptSet: make(map[string]struct{}),
		CgSet:     make(map[string]struct{}),
		SceneSet:  make(map[string]struct{}),
		MusicSet:  make(map[string]struct{}),
	}
	if src == nil {
		return c
	}
	if recs, err := src.Table("list_script.tbl"); err == nil {
		c.Scripts = BuildScripts(recs)
		for _, s := range c.Scripts {
			c.ScriptSet[s.Name] = struct{}{}
		}
	}
	if recs, err := src.Table("list_cg.tbl"); err == nil {
		c.Cg = BuildCgGroups(recs)
		for _, g := range c.Cg {
			for _, v := range g.Variants {
				c.CgSet[v] = struct{}{}
			}
		}
	}
	// 页表按页号 01..19 依次读，缺页跳过（cg 5 页、scene 19 页）。
	for page := 1; page <= 19; page++ {
		name := fmt.Sprintf("scene_page%02d.tbl", page)
		recs, err := src.Table(name)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(recs))
		for _, r := range recs {
			if strings.TrimSpace(r.Name) != "" {
				names = append(names, r.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		c.ScenePages = append(c.ScenePages, ScenePage{Page: page, Records: names})
	}
	for page := 1; page <= 5; page++ {
		name := fmt.Sprintf("cg_page%02d.tbl", page)
		recs, err := src.Table(name)
		if err != nil {
			continue
		}
		thumb, rest := thumbVariants(namesOf(recs))
		if thumb == "" && len(rest) == 0 {
			continue
		}
		c.CgPages = append(c.CgPages, CgPage{Page: page, Thumb: thumb, Variants: rest})
	}
	c.Scenes = BuildScenes(c.ScenePages)
	for _, s := range c.Scenes {
		if s.SceneID != "" {
			c.SceneSet[s.SceneID] = struct{}{}
		}
		if s.Script != "" {
			c.SceneSet[s.Script] = struct{}{}
		}
	}
	c.Music = BuildMusic(src.Names(".ogg"))
	for _, m := range c.Music {
		c.MusicSet[m.Name] = struct{}{}
		c.MusicSet[m.Unlock] = struct{}{}
	}
	return c
}

// ScriptIndex 返回脚本名在表里的下标，找不到返回 -1。
func (c *Catalog) ScriptIndex(name string) int {
	if c == nil {
		return -1
	}
	for _, s := range c.Scripts {
		if s.Name == name {
			return s.Index
		}
	}
	return -1
}

// ScriptName 返回下标对应的脚本名，越界返回空。
func (c *Catalog) ScriptName(index int) string {
	if c == nil || index < 0 || index >= len(c.Scripts) {
		return ""
	}
	return c.Scripts[index].Name
}

// CgGroupOf 返回变体名所属组，找不到返回 nil。
func (c *Catalog) CgGroupOf(variant string) *CgGroup {
	if c == nil {
		return nil
	}
	prefix, ok := CgPrefix(variant)
	if !ok {
		return nil
	}
	for i := range c.Cg {
		if c.Cg[i].Name == prefix {
			return &c.Cg[i]
		}
	}
	return nil
}

// HasCgVariant 判断名字是否在 CG 目录里。
func (c *Catalog) HasCgVariant(name string) bool {
	if c == nil {
		return false
	}
	_, ok := c.CgSet[name]
	return ok
}

// HasScene 判断场景键（sceneId 或 script）是否在目录里。
func (c *Catalog) HasScene(key string) bool {
	if c == nil {
		return false
	}
	_, ok := c.SceneSet[key]
	return ok
}

// HasMusic 判断曲目名是否在目录里（bgm01 与 bgm01.ogg 都认）。
func (c *Catalog) HasMusic(name string) bool {
	if c == nil {
		return false
	}
	name = strings.TrimSuffix(strings.TrimSpace(name), ".ogg")
	_, ok := c.MusicSet[name]
	return ok
}
