package menus

import (
	"strings"

	"kizuna/enginego/brand/overdose/save"
)

// 为什么解锁判定要落在 save.SystemData 的字符串集合上：
// save 包已经把三个解锁列表（CgUnlocks/SceneUnlocks/MusicUnlocks）
// 序列化进 system.json；宿主的 GalleryUnlocker（core）按
// “按名查 CG、按下标查场景”回问，session 需要用目录把两边
// 对齐。这里用名字集合而非下标位图，是为了让手改 system.json
// 也能生效（与 save.VariableSnapshot 的可读键同理）。
//
// 全新安装（system.json 不存在或列表为空）默认全部未解锁——
// 与原引擎“通关/见过才解锁”的语义一致；不要在 session 里
// 预填全解锁，否则第一周目就能看完全部 CG。

// Unlocks 是基于 SystemData 的解锁查询（纯函数，无状态）。
type Unlocks struct {
	Cg     map[string]struct{}
	Scene  map[string]struct{}
	Music  map[string]struct{}
	Scenes []string // 场景键的表顺序（SceneUnlocked 按下标查）
}

// NewUnlocks 从 SystemData 构建解锁集合。data 为 nil 时全空
// （全新游戏，全部锁定），调用方据此决定鉴赏是否可见。
func NewUnlocks(data *save.SystemData) *Unlocks {
	u := &Unlocks{
		Cg:    make(map[string]struct{}),
		Scene: make(map[string]struct{}),
		Music: make(map[string]struct{}),
	}
	if data == nil {
		return u
	}
	for _, n := range data.CgUnlocks {
		if n = strings.TrimSpace(n); n != "" {
			u.Cg[n] = struct{}{}
		}
	}
	for _, n := range data.SceneUnlocks {
		if n = strings.TrimSpace(n); n != "" {
			u.Scene[n] = struct{}{}
			u.Scenes = append(u.Scenes, n)
		}
	}
	for _, n := range data.MusicUnlocks {
		if n = strings.TrimSpace(n); n != "" {
			u.Music[n] = struct{}{}
		}
	}
	return u
}

// CgUnlockedByName 按图片名查询（core.GalleryUnlocker）。
// 变体名与缩略图名（thumb_xxx）都认——宿主传的可能是不带
// 前缀的 cg 名（list_cg.tbl 的原名），去前缀比对避免漏判。
func (u *Unlocks) CgUnlockedByName(name string) bool {
	if u == nil || len(u.Cg) == 0 {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if _, ok := u.Cg[name]; ok {
		return true
	}
	if strings.HasPrefix(name, "thumb_") {
		_, ok := u.Cg[strings.TrimPrefix(name, "thumb_")]
		return ok
	}
	_, ok := u.Cg["thumb_"+name]
	return ok
}

// SceneUnlocked 按场景表下标查询（core.GalleryUnlocker）。
// 下标越界返回 false（未解锁），而不是报错——宿主在越界时
// 也应显示锁定态，不要让一个坏下标把整个鉴赏页拖崩。
func (u *Unlocks) SceneUnlocked(index int) bool {
	if u == nil || index < 0 || index >= len(u.Scenes) {
		return false
	}
	_, ok := u.Scene[u.Scenes[index]]
	return ok
}

// MusicUnlocked 按曲目名查询。
func (u *Unlocks) MusicUnlocked(name string) bool {
	if u == nil || len(u.Music) == 0 {
		return false
	}
	name = strings.TrimSuffix(strings.TrimSpace(name), ".ogg")
	_, ok := u.Music[name]
	return ok
}

// MarkCg 记一条 CG 解锁（已存在返回 false）。
func (u *Unlocks) MarkCg(name string) bool {
	if u == nil {
		return false
	}
	name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "thumb_"))
	if name == "" {
		return false
	}
	if _, ok := u.Cg[name]; ok {
		return false
	}
	u.Cg[name] = struct{}{}
	return true
}

// MarkScene 记一条场景解锁（已存在返回 false）。
func (u *Unlocks) MarkScene(key string) bool {
	if u == nil {
		return false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if _, ok := u.Scene[key]; ok {
		return false
	}
	u.Scene[key] = struct{}{}
	u.Scenes = append(u.Scenes, key)
	return true
}

// MarkMusic 记一首曲目的解锁（已存在返回 false）。
func (u *Unlocks) MarkMusic(name string) bool {
	if u == nil {
		return false
	}
	name = strings.TrimSuffix(strings.TrimSpace(name), ".ogg")
	if name == "" {
		return false
	}
	if _, ok := u.Music[name]; ok {
		return false
	}
	u.Music[name] = struct{}{}
	return true
}

// Apply 把解锁集合写回 SystemData（去重、保持插入序）。
// 调用方负责落盘（SystemStore.Save），这里只保证写回结构
// 与解锁集合一致，不做 IO。
func (u *Unlocks) Apply(data *save.SystemData) {
	if u == nil || data == nil {
		return
	}
	data.CgUnlocks = sortedKeys(u.Cg)
	data.SceneUnlocks = append([]string(nil), u.Scenes...)
	data.MusicUnlocks = sortedKeys(u.Music)
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// 排序保证 system.json 的 diff 可读（与“插入序无关”的快照一致）。
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	// 小集合（解锁通常 <200 项），插入排序足够且无分配；
	// 大集合也只在此处 Apply 时调用，不在热路径。
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j] < s[j-1] {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}

// CgKey 归一化 CG 键：去 thumb_ 前缀、去空白。
// session 在宏 ev/CG 出现时用它把 stage 的图片名对到目录键上。
func CgKey(name string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "thumb_"))
}

// SceneKey 归一化场景键：优先 sceneId，缺失时退回 script。
func SceneKey(e SceneEntry) string {
	if e.SceneID != "" {
		return e.SceneID
	}
	return e.Script
}

// MusicKey 归一化曲目键：去 .ogg 后缀。
func MusicKey(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), ".ogg")
}

// BgmKey 是 BGM 文件名到曲目键的转换（bgm01.ogg → bgm01）。
func BgmKey(file string) string {
	return MusicKey(file)
}
