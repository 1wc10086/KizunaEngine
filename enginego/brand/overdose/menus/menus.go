// Package menus 是 overdose（YUI）系统菜单/鉴赏的目录层。
//
// 为什么单独成包而不是塞进 session：目录数据（脚本名、CG 变体、场景条目、
// 音乐曲目、标题/存档/配置的资源名与矩形）只来自 system.arc 的 .tbl 与
// music.arc 的条目名 + graphic.arc 的图片名，不依赖 VM、舞台与音频状态。
// 把它从 session 里拆出来，session 只负责拿目录去组装核心接口
// （TitleMenu / Scenes / CgGroups / GalleryComposer / GalleryUnlocker /
// SlotPreviewer 数据源），而本包不 import session，单测也能不拉起整台
// 引擎就断言目录正确性。
//
// 目录的蓝本是 overdose_spec.md §4/§7：
//   - list_script.tbl / list_cg.tbl：每条 160 字节（u32 哈希 + 76 字节
//     CP932 名），只认名字，哈希不参与分组；
//   - scene_pageNN.tbl / cg_pageNN.tbl：40960 字节一条 = 256 条 160 字节
//     子记录，只有头部记录数条子记录有效（无效子记录全是空名，见
//     arc.ParseTable 的注释）；
//   - 缩略图是普通图形条目 thumb_<name>.cpb，标题背景 title_bg.cpb
//     实测 800x600（与游戏画面同尺寸，Compose 直接铺满衔接）。
//
// 本包只暴露可值拷贝的纯数据结构体 + 无状态 helper，不持有 arc 句柄，
// 因此可以被 session 在 Open 时构建一次、之后只读复用。
package menus

import (
	"strings"

	"kizuna/enginego/brand/overdose/arc"
)

// 画面尺寸：原版标题/存档/配置背景均为 800x600 整图（spec §7），
// 与运行时 BGRA 画面同尺寸。布局矩形都以此为坐标系，不要按 640x480 猜。
const (
	ScreenW = 800
	ScreenH = 600
)

// Rect 是 800x600 坐标系下的一块矩形。X/Y 可为负（按钮图自带偏移时
// Session 侧再加图头偏移），W/H 为像素尺寸；零值矩形表示“未定位”。
type Rect struct {
	X, Y, W, H int
}

// MenuItem 是标题菜单一项的静态描述：资源名 + 命中矩形 + 显示标签。
// Session 把它转成 core.TitleMenuItem（Key/Label 回传宿主），
// 因此 Label 直接用 key 字符串（start/load/config/extra/quit），
// 与 core 的 Label=Key 约定一致（bishop 的 TitleMenu 也是这么做的）。
type MenuItem struct {
	Key        string // start | load | config | extra | quit
	Image      string // title_start 等（不带 .cpb）
	ImageOn    string // 按下态：<Image>-o，graphic.arc 里确实成对存在
	Label      string // 宿主展示用文本，默认等于 Key
	Rect       Rect
	TitleLabel string // 同 Label 的别名，方便两边取值
}

// ScriptEntry 是 list_script.tbl 的一行。
type ScriptEntry struct {
	Index int
	Name  string
}

// CgGroup 是一组同前缀的 CG 变体（见 BuildCgGroups 的分组规则）。
// 保留 Name（前缀）供 session 日志/解锁反查用；session 转
// core.CgGroup 时只取 Variants 即可。
type CgGroup struct {
	Name     string     // 前缀，如 cga01
	Variants []string   // 变体名（保持表顺序），如 cga01-01, cga01-02, …
	Images   [][]string // 每变体一张图（单图变体），与 Variants 平行
	Thumb    string     // thumb_<Variants[0]>，缺失时为空
}

// SceneEntry 是 scene_pageNN.tbl 解出来的一条场景。
// 结构见 BuildScenes 的注释：每页 [thumb, script, sceneId, script?]。
type SceneEntry struct {
	Index   int      // 全局序号（按页序、页内序累加，空条目不占号）
	Page    int      // 页号，1 起（scene_page01 → 1）
	Thumb   string   // thumb_xxx（不带 .cpb），空表示无缩略图
	Script  string   // 脚本名，如 sop3 / sa07（回放/进入时的脚本）
	SceneID string   // 场景标识，如 scene_a001（解锁/存档的键）
	Title   string   // 展示名：优先 SceneID，缺失时退回 Script
	Images  []string // 展示图：有缩略图时 [Thumb]，否则 [Script]
}

// MusicTrack 是 music.arc 的一首曲目。
type MusicTrack struct {
	Index  int    // 排序后序号
	Name   string // 不带 .ogg，如 bgm01（存档/解锁的键）
	File   string // 带 .ogg，如 bgm01.ogg（arc 条目名）
	Title  string // 展示名，默认等于 Name
	Unlock string // 解锁键，默认等于 Name
}

// ExtraScreen 是 extra（鉴赏入口）的一个页签。
type ExtraScreen struct {
	Key   string // cg | music | scene
	Image string // extra_cg 等（spec §7 的入口名；图里实际以 cg_btn* 呈现，见 ExtraLayout）
	Rect  Rect
}

// CgPage 是一页 CG 页表（去空、去 thumb 后只留变体名）。
type CgPage struct {
	Page     int
	Thumb    string // 第一子记录的 thumb_xxx
	Variants []string
}

// ScenePage 是一页场景页表（保留原始子记录顺序，去空名）。
type ScenePage struct {
	Page    int
	Records []string
}

// Source 是 Catalog 的输入面：只需按名读表与列名。
// *arc.Set 原样满足；单测用假实现即可，不需要真游戏数据。
type Source interface {
	Table(name string) ([]arc.Record, error)
	// Names 列出某类条目名（music 用 ".ogg"）。
	Names(ext string) []string
}

// Catalog 是从系统表构建出来的完整目录。构建后只读，
// helper 均为无副作用的纯函数，方便 session 在任意线程调用。
type Catalog struct {
	Scripts []ScriptEntry
	Cg      []CgGroup
	Scenes  []SceneEntry
	Music   []MusicTrack

	// ScriptSet / CgSet / SceneSet / MusicSet 是去重后的集合，
	// 供解锁反查与“是否在目录里”判断用（顺序无关）。
	ScriptSet map[string]struct{}
	CgSet     map[string]struct{}
	SceneSet  map[string]struct{}
	MusicSet  map[string]struct{}

	// 页表原始页：CgPages[i] 对应 cg_page(i+1).tbl，ScenePages 同理。
	// 保留是为了将来按页取缩略图/变体用，当前 Cg/Scenes 已扁平化。
	CgPages    []CgPage
	ScenePages []ScenePage
}

// ThumbName 返回变体/场景的缩略图名（不带 .cpb）。
// 缩略图在 graphic.arc 里叫 thumb_<name>.cpb，spec §4 已确认。
func ThumbName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "thumb_") {
		return name
	}
	return "thumb_" + name
}
