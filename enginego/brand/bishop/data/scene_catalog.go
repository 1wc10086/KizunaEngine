package data

import (
	"sort"
	"strings"
)

const (
	sceneLabelPrefix  = "label"
	sceneReplaySuffix = "_replay"
)

type ReplayScene struct {
	Group   int
	Index   int
	SceneId string
	Label   string
	Images  []string
	Title   string
}

type SceneCatalog struct {
	Scenes []ReplayScene
	Titles map[string]string
	// labelIndex 是 Scenes[].Label 的反查表（惰性建立，见 IndexOfLabel）。
	labelIndex map[string]int
}

// ScenePage 是场景鉴赏的一页：一个页签对应 scene.dat 的一个分组。
//
// 场景页不是"把整张表按缩略格数顺序切开"。原引擎的页签图标
// （museum_page21…26 / museum_page31…36 / 黒の教室 的角色名页签）
// 与 scene.dat 的分组列一一对应：前 6 页是各角色的事件场景，
// 后 6 页是通常场景。按顺序切页会把后一组的场景挤进前一页，
// 玩家就会在某个角色页里看到别的角色的封面。
type ScenePage struct {
	Group int
	// Scenes 是场景在 SceneCatalog.Scenes 里的下标，保持 scene.dat 行序。
	// 解锁位是按行序记录的，显示分组不能重排这张表。
	Scenes []int
}

// Pages 按 Group 升序把场景下标分组。
func (c *SceneCatalog) Pages() []ScenePage {
	if c == nil || len(c.Scenes) == 0 {
		return nil
	}
	groups := make([]int, 0, 12)
	seen := make(map[int]struct{}, 12)
	for index := range c.Scenes {
		group := c.Scenes[index].Group
		if group < 0 {
			group = 0
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	sort.Ints(groups)

	pageOf := make(map[int]int, len(groups))
	pages := make([]ScenePage, len(groups))
	for page, group := range groups {
		pageOf[group] = page
		pages[page].Group = group
	}
	for index := range c.Scenes {
		group := c.Scenes[index].Group
		if group < 0 {
			group = 0
		}
		page := pageOf[group]
		pages[page].Scenes = append(pages[page].Scenes, index)
	}
	return pages
}

// IndexOfLabel 反查场景标签在场景表里的下标，用于全局存档的场景解锁位。
//
// 全局存档按 scene.dat 的行序记录解锁状态，而剧情里能拿到的是标签名，
// 所以需要这层映射。找不到返回 -1（该标签不是可回放的场景）。
//
// 反查表惰性建立：215 条线性扫描挂在"每次跳标签"上虽然不致命，
// 但一局游戏要跳几万次标签，建一次表更省。
func (c *SceneCatalog) IndexOfLabel(label string) int {
	if c == nil || len(label) == 0 {
		return -1
	}
	if c.labelIndex == nil {
		c.labelIndex = make(map[string]int, len(c.Scenes))
		for index := range c.Scenes {
			// 回放要按 SceneId（L01_001_replay）找场景；解锁标记和旧的播放列表
			// 用的是 label_ 列，两种写法都要能反查。
			for _, name := range c.Scenes[index].LookupNames() {
				if name == "" {
					continue
				}
				if _, exists := c.labelIndex[name]; !exists {
					c.labelIndex[name] = index
				}
			}
		}
	}
	if index, ok := c.labelIndex[label]; ok {
		return index
	}
	return -1
}

// lookupNames 返回这个场景可以被引用的名字：回放 ID、去掉 _replay 的普通标签、
// 以及 scene.dat 最后一列的解锁标识。
func (s ReplayScene) LookupNames() []string {
	names := make([]string, 0, 3)
	if s.SceneId != "" {
		names = append(names, s.SceneId)
		plain := strings.TrimSuffix(s.SceneId, sceneReplaySuffix)
		if plain != s.SceneId {
			names = append(names, plain)
		}
	}
	if s.Label != "" && s.Label != s.SceneId {
		names = append(names, s.Label)
	}
	return names
}

func ParseSceneCatalog(sceneData []byte, nameData []byte) *SceneCatalog {
	titles := parseSceneTitles(nameData)
	text := string(sceneData)
	scenes := []ReplayScene{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if len(line) == 0 || line[0] == ';' {
			continue
		}
		columns := strings.Split(line, ",")
		if len(columns) < 3 {
			continue
		}
		group, okGroup := parseSceneInt(strings.TrimSpace(columns[0]))
		index, okIndex := parseSceneInt(strings.TrimSpace(columns[1]))
		if !okGroup || !okIndex {
			continue
		}
		sceneId := cleanSceneValue(columns[2])
		if len(sceneId) == 0 {
			continue
		}
		label := ""
		images := []string{}
		titleColumn := ""
		// 旧版/特別病棟 的 scene.dat 只有 "group,index,sceneId" 三列：
		// sceneId 本身就是回放标签与场景图名（htm_iv01_00）。现代作品的
		// 额外列才是图片列表，不能把缺少图片列当成"没有图片"而整页空白。
		if len(columns) == 3 {
			images = append(images, sceneId)
			label = sceneId
		}
		for i := 3; i < len(columns); i++ {
			value := cleanSceneValue(columns[i])
			if len(value) == 0 {
				continue
			}
			if len(value) >= len(sceneLabelPrefix) && strings.EqualFold(value[:len(sceneLabelPrefix)], sceneLabelPrefix) {
				label = value
				continue
			}
			// scene.dat 的第 4 列在 特別授業3SLG 里是日文场景名，不是图片。
			// 靠"含非 ASCII"区分：图片名是纯 ASCII（ev01_001_1p00），
			// 场景名一定带假名/汉字；把它当图片会让缩略图加载失败，
			// 整格变成空白，而且标题永远不显示。
			if containsNonASCII(value) {
				if titleColumn == "" {
					titleColumn = value
				}
				continue
			}
			images = append(images, value)
		}
		key := sceneTitleKey(sceneId)
		title, ok := titles[key]
		if !ok {
			if titleColumn != "" {
				title = titleColumn
			} else {
				title = key
			}
		}
		scenes = append(scenes, ReplayScene{Group: group, Index: index, SceneId: sceneId, Label: label, Images: images, Title: title})
	}
	return &SceneCatalog{Scenes: scenes, Titles: titles}
}

// containsNonASCII 判断字符串里是否有非 ASCII 字符。
func containsNonASCII(value string) bool {
	for _, character := range value {
		if character > 0x7f {
			return true
		}
	}
	return false
}

func cleanSceneValue(value string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "\""))
}

func sceneTitleKey(sceneId string) string {
	cut := -1
	for i := 0; i+len(sceneReplaySuffix) <= len(sceneId); i++ {
		if strings.EqualFold(sceneId[i:i+len(sceneReplaySuffix)], sceneReplaySuffix) {
			cut = i
			break
		}
	}
	if cut > 0 {
		return sceneId[:cut]
	}
	return sceneId
}

func parseSceneTitles(nameData []byte) map[string]string {
	titles := make(map[string]string)
	if len(nameData) == 0 {
		return titles
	}
	text := string(nameData)
	position := 0
	for position < len(text) {
		open := strings.IndexByte(text[position:], '"')
		if open < 0 {
			break
		}
		open += position
		rel := strings.IndexByte(text[open+1:], '"')
		if rel < 0 {
			break
		}
		close := open + 1 + rel
		relColon := strings.IndexByte(text[close+1:], ':')
		if relColon < 0 {
			break
		}
		colon := close + 1 + relColon
		relValueOpen := strings.IndexByte(text[colon+1:], '"')
		if relValueOpen < 0 {
			break
		}
		valueOpen := colon + 1 + relValueOpen
		relValueClose := strings.IndexByte(text[valueOpen+1:], '"')
		if relValueClose < 0 {
			break
		}
		valueClose := valueOpen + 1 + relValueClose
		titles[text[open+1:close]] = text[valueOpen+1 : valueClose]
		position = valueClose + 1
	}
	return titles
}

func parseSceneInt(text string) (int, bool) {
	negative := false
	if strings.HasPrefix(text, "+") || strings.HasPrefix(text, "-") {
		negative = strings.HasPrefix(text, "-")
		text = text[1:]
	}
	if len(text) == 0 {
		return 0, false
	}
	value := 0
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
		value = value*10 + int(text[i]-'0')
	}
	if negative {
		value = -value
	}
	return value, true
}
