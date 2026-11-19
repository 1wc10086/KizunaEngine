// 鉴赏（データ閲覧）用接口的实现。
//
// 原版的鉴赏是脚本自绘的（view 脚本 + AGRD 的 cgmode*/hsmode* 底图 +
// _var 的 [HCG] 名字表）。这里把“有哪些 CG / 怎么合成一张 / 取哪一帧”
// 接到 core 的 Gallery* 接口上：宿主用 op 33 拿目录、op 30 拿合成图，
// 都和 bishop/overdose 走同一条协议，不需要为 TMR-Hiro 新增 op。
package session

import (
	"sort"
	"strings"

	"kizuna/enginego/brand/tmrhiro/grd"
	"kizuna/enginego/brand/tmrhiro/stage"
	"kizuna/enginego/core"
)

// CgGroups 返回鉴赏分组。
//
// 分组规则取自名字本身：fm_A01_0001 的组是 “A01”，同组的变体按编号排序。
// 原版的鉴赏页一行是一组（A01..A19 / B01..B20 …），所以按组前缀切分即可。
func (s *Session) CgGroups() []core.CgGroup {
	if len(s.cgList) == 0 {
		return nil
	}
	groups := make(map[string][]string)
	order := make([]string, 0, 16)
	for _, name := range s.cgList {
		key := cgGroupKey(name)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], name)
	}
	sort.Strings(order)
	out := make([]core.CgGroup, 0, len(order))
	for _, key := range order {
		variants := make([][]string, 0, len(groups[key]))
		for _, name := range groups[key] {
			// 每个“变体”就是一条宏：合成时一次性贴出所有部件。
			variants = append(variants, []string{name})
		}
		out = append(out, core.CgGroup{Name: key, Variants: variants})
	}
	return out
}

// cgGroupKey 取 fm_A01_0001 → "A01"。
func cgGroupKey(name string) string {
	trimmed := strings.TrimPrefix(name, "fm_")
	if idx := strings.Index(trimmed, "_"); idx > 0 {
		return trimmed[:idx]
	}
	return trimmed
}

// ComposeImages 把若干图片名合成一张（宿主鉴赏缩略图用）。
//
// 名字可能同时是宏（fm_*）与部件（A01_00_00-00_a），两种都走 stage 的
// 解析路径；合成时按传入顺序叠加，第一张为底。
func (s *Session) ComposeImages(names []string) (core.Frame, bool) {
	if s.stage == nil || len(names) == 0 {
		return core.Frame{}, false
	}
	var base *grd.Image
	for _, name := range names {
		img := s.stage.ResolveImage(name)
		if img == nil || img.Width <= 0 || img.Height <= 0 {
			continue
		}
		if base == nil {
			base = img
			continue
		}
		base = stage.MergeImages(base, img)
	}
	if base == nil {
		return core.Frame{}, false
	}
	return core.Frame{Width: base.Width, Height: base.Height, Pixels: base.Pixels}, true
}

// SelectFrame 返回单张图（鉴赏点开变体时用）。
func (s *Session) SelectFrame(name string) (core.Frame, bool) {
	if s.stage == nil {
		return core.Frame{}, false
	}
	img := s.stage.ResolveImage(name)
	if img == nil {
		return core.Frame{}, false
	}
	return core.Frame{Width: img.Width, Height: img.Height, Pixels: img.Pixels}, true
}

// SetPreview 保存宿主传来的预览图（原版鉴赏页右下角的“预览框”）。
func (s *Session) SetPreview(frame core.Frame) { s.preview = frame }

// Scenes 返回可回放的场景。
//
// TMR-Hiro 没有独立的“场景表”，_var 的 [HSC] 段就是可回放的 H 场景脚本名；
// 每个脚本名对应一条鉴赏条目，封面取该脚本第一张 CG（由宿主按需合成）。
func (s *Session) Scenes() []core.Scene {
	if len(s.sceneList) == 0 {
		return nil
	}
	out := make([]core.Scene, 0, len(s.sceneList))
	for i, name := range s.sceneList {
		out = append(out, core.Scene{
			ID:     uint32(i + 1),
			Name:   name,
			Title:  name,
			Group:  0,
			Index:  uint32(i),
			Images: nil,
		})
	}
	return out
}

// CgUnlockedByName 报告该 CG 是否已解锁。
//
// 原版把解锁位写在全局存档里；本移植按"剧本里真的出现过这张 CG"记进
// s.unlockedCG（见 absorb：切到整屏 CG 时置位），并且 MEMORY COMPLETE 的
// 100%/0% 按钮直接改这个集合。以前"资源存在就算解锁"会让鉴赏从一开始
// 就是全开的，与原作的收集玩法完全不符。
func (s *Session) CgUnlockedByName(name string) bool {
	if s == nil {
		return false
	}
	return s.unlockedCG[name]
}

// sceneUnlockedByName 按脚本名报告场景是否已解锁。
func (s *Session) sceneUnlockedByName(name string) bool {
	if s == nil || s.unlockedScene == nil {
		return false
	}
	for i, candidate := range s.sceneList {
		if candidate == name {
			return s.unlockedScene[i]
		}
	}
	return false
}

// SceneUnlocked 报告场景是否可见。
//
// 场景在剧本里被完整播放过才解锁（absorb 在脚本结束 H 场景时置位）；
// 越界或没解锁都返回 false，界面按 nodata 画。
func (s *Session) SceneUnlocked(index int) bool {
	if s == nil || index < 0 || index >= len(s.sceneList) {
		return false
	}
	return s.unlockedScene[index]
}

// unlockCG 记录一张 CG 已解锁（剧本切到该图时调用）。
func (s *Session) unlockCG(name string) {
	if name == "" || s.unlockedCG == nil {
		return
	}
	s.unlockedCG[name] = true
}

// unlockScene 按脚本名解锁场景（场景回想列表用的是脚本名）。
func (s *Session) unlockScene(scriptName string) {
	if scriptName == "" || s.unlockedScene == nil {
		return
	}
	for i, name := range s.sceneList {
		if name == scriptName {
			s.unlockedScene[i] = true
		}
	}
}

// isEventCG 判断一个 fm_ 宏是"整屏事件 CG"还是"立绘"。
//
// 命名规则来自本作的资源表：事件 CG 的宏是 fm_<字母><两位数字>_<四位序号>
// （A/B/C/D/E/F 组），立绘是 fm_Z01_0001 / fm_Z02_0001 这类 Z 组。
// 用字母段而不是"资源尺寸"判断：尺寸要解图才知道，而这条记录出现时
// 正好在合成线程的临界区外，解图会把第一次切 CG 拖慢一帧。
func isEventCG(name string) bool {
	if len(name) < 6 || name[:3] != "fm_" {
		return false
	}
	switch name[3] {
	case 'A', 'B', 'C', 'D', 'E', 'F':
		return true
	}
	return false
}

// galleryPercent 返回 CG 鉴赏的达成率（0..100）。
//
// 原版把"已解锁的 CG 数 / 总数"算成百分比显示在 CGMODE 页右上角。
// 场景回想（HSC）与 BGM 页共用同一个数字（本作底图上三页都是「達成率」）。
func (s *Session) galleryPercent() int {
	if len(s.cgList) == 0 {
		return 0
	}
	unlocked := 0
	for _, name := range s.cgList {
		if s.unlockedCG[name] {
			unlocked++
		}
	}
	return unlocked * 100 / len(s.cgList)
}
