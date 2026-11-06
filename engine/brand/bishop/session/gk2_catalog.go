package session

import (
	"fmt"
	"os"
	"strings"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/core"
)

// gk2_catalog.go 是学園2 老版（BSScript Rev.2）鉴赏室的目录构建。
//
// 老版 cg.dat 的解析结果不可靠，CG 目录只能从资源名推导：g<组><场景两位><差分>。
// 同一场景的 h/p 差分归为同一格的变体；无后缀的基础图排在最前。

const gk2CgGroupCount = 7

var gk2CgSuffixes = gk2CgVariantSuffixes()

func gk2CgVariantSuffixes() []string {
	suffixes := []string{""}
	for n := 1; n <= 9; n++ {
		suffixes = append(suffixes, fmt.Sprintf("h%d", n), fmt.Sprintf("p%d", n))
	}
	return suffixes
}

// gk2CgGroupOf 把资源号映射到组页签：g1..g6 各占一组，g7/g8 合并到第 7 组（その他）。
func gk2CgGroupOf(resource int) int {
	if resource >= 7 {
		return gk2CgGroupCount - 1
	}
	return resource - 1
}

// gk2CgBase 把资源名拆成“主图名 + 差分后缀”。
//
// 命名是 g<角色 1..8><场景两位><子号两位>[hN|pN]：g10101 是主图，
// g10101h2 是同一格的发型差分，g10201 已经是另一个场景。主图和差分要
// 归到同一 entry，否则鉴赏室会把每个差分都当成独立的一格。
func gk2CgBase(name string) (base string, suffix string, ok bool) {
	if len(name) < 6 || name[0] != 'g' || name[1] < '1' || name[1] > '8' {
		return "", "", false
	}
	for i := 2; i < 6; i++ {
		if name[i] < '0' || name[i] > '9' {
			return "", "", false
		}
	}
	rest := name[6:]
	if rest == "" {
		return name, "", true
	}
	if len(rest) < 2 || (rest[0] != 'h' && rest[0] != 'p') {
		return "", "", false
	}
	for i := 1; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return "", "", false
		}
	}
	return name[:6], rest, true
}

// buildGK2CgCatalog 按资源名枚举 CG。
//
// 老版 cg.dat 的布局与现代解析器不兼容，而资源名本身就是完整目录：
// g<角色><场景><子号>，hN/pN 是差分。这里只遍历一次资源名表，
// 不做 g1..g8 × 100 × 100 的存在性探测，避免首次打开鉴赏室卡几秒。
func (s *Session) buildGK2CgCatalog() *data.CgCatalog {
	if s.res == nil {
		return nil
	}
	catalog := &data.CgCatalog{Groups: make([]data.CgGroup, gk2CgGroupCount)}
	for index := range catalog.Groups {
		catalog.Groups[index].Index = index
	}
	// base -> entry 下标；同一 base 的 hN/pN 依次追加为变体。
	entryOf := make(map[string]int)
	for _, name := range s.resourceImageNames() {
		base, _, ok := gk2CgBase(name)
		if !ok {
			continue
		}
		resource := int(name[1] - '0')
		group := gk2CgGroupOf(resource)
		if group < 0 || group >= len(catalog.Groups) {
			continue
		}
		index, exists := entryOf[base]
		if !exists {
			index = len(catalog.Groups[group].Entries)
			entryOf[base] = index
			catalog.Groups[group].Entries = append(catalog.Groups[group].Entries, data.CgEntry{Index: index})
		}
		entry := &catalog.Groups[group].Entries[index]
		variant := data.CgVariant{
			Entry:   index,
			Variant: len(entry.Variants),
			Images:  []string{name},
		}
		entry.Variants = append(entry.Variants, variant)
		catalog.Groups[group].Variants = append(catalog.Groups[group].Variants, variant)
		catalog.Names = append(catalog.Names, name)
	}
	core.Logf("catalog: gk2 cg groups=%d variants=%d", gk2CgGroupCount, len(catalog.Names))
	return catalog
}

// gk2ScenePrefixGroups 是场景图名前缀到组页签的映射，顺序与 bsMapGakuen2RoleLabels 无关，只按前缀匹配。
var gk2ScenePrefixGroups = []struct {
	prefix string
	group  int
}{{"hrn", 0}, {"mik", 1}, {"kwr", 2}, {"izm", 3}, {"ayn", 4}, {"kyk", 5}}

// gk2SceneGroupOf 按图名前缀决定场景页；匹配不到的归入第 7 页（その他）。
func gk2SceneGroupOf(image string) int {
	for _, item := range gk2ScenePrefixGroups {
		if strings.HasPrefix(image, item.prefix) {
			return item.group
		}
	}
	return gk2CgGroupCount - 1
}

// gk2ReplayLabelOf 去掉末尾的 _NN 序号得到回放标签：hrn_ev01_00 → hrn_ev01。
// 只去掉全数字的后缀，避免把 ev01 这类带字母的名字截坏。
func gk2ReplayLabelOf(image string) string {
	cut := strings.LastIndexByte(image, '_')
	if cut <= 0 || cut == len(image)-1 {
		return image
	}
	if strings.Trim(image[cut+1:], "0123456789") != "" {
		return image
	}
	return image[:cut]
}

// loadGK2SceneCatalog 读 scene.dat 并按前缀重映射组。
//
// 注意 SceneId 只有在剧本里真的存在对应标签时才保留：否则回放会找不到入口，
// 调用方会只记日志，不能把不存在的标签当成可播放场景。
func (s *Session) loadGK2SceneCatalog() *data.SceneCatalog {
	path := findFileInRoots(s.resourceLookupRoots(), "scene.dat")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		core.Logf("catalog: gk2 scene.dat failed: %s", err.Error())
		return nil
	}
	var titles []byte
	if s.res != nil {
		if location, ok := s.res.TryGet("scene_name"); ok && location != nil {
			titles = location.Read()
		}
	}
	catalog := data.ParseSceneCatalog(raw, titles)
	for index := range catalog.Scenes {
		scene := &catalog.Scenes[index]
		image := scene.SceneId
		if len(scene.Images) > 0 {
			image = scene.Images[0]
		}
		scene.Group = gk2SceneGroupOf(image)
		label := gk2ReplayLabelOf(scene.SceneId)
		scene.SceneId = ""
		if label != "" && s.script != nil && s.script.FindLabel(label) >= 0 {
			scene.SceneId = label
		}
		scene.Label = scene.SceneId
	}
	core.Logf("catalog: gk2 scene.dat scenes=%d", len(catalog.Scenes))
	return catalog
}
