package gothic

// gothic_global.go 保存 MAZE 的跨周目数据。
//
// 原引擎把 CG 解锁写在安装目录的 prof.dat，把其它全局设置写在
// usr0.dat。这里不直接改这两个文件：
//   - prof.dat 的字段顺序就是鉴赏室的目录顺序，但文件本身没有
//     版本号/长度校验，写坏一次会让原版也读不出来；
//   - usr0.dat 是 120 字节的二进制结构，字段含义尚未完全确认。
//
// 因此引擎把“已解锁 CG / H 场景 / 设置”落在 save/gothic/global.json，
// 读取时仍会解析 prof.dat 的 1 标记作为初始值。这样既不会破坏原版
// 存档，又能让新解锁在后续周目里继续生效。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/core"
)

const gothicGlobalVersion = 1

// gothicThumbnailAtlasCount 是 sumbnail.gsa 里的格子数：8 列 x 11 行。
// 之后的事件 CG/背景图册没有预生成缩略图，鉴赏室用原图缩放补足。
const gothicThumbnailAtlasCount = 88

type gothicGlobalData struct {
	Version int `json:"version"`
	// CG 以图片名（不含扩展名、小写）为键。名字而不是下标：
	// prof.dat 的行序是鉴赏目录，重排/补丁后下标会漂移。
	CG      map[string]bool `json:"cg"`
	H       map[string]bool `json:"h"`
	Cleared bool            `json:"cleared"`

	PlayerFirst string `json:"playerFirst"`
	PlayerLast  string `json:"playerLast"`

	// TextSpeed: 0=ページ単位, 1=高速, 2=標準, 3=低速（原版设置画面的四项）
	TextSpeed int `json:"textSpeed"`
	// Background: 0=黒, 1=赤, 2=青, 3=セピア
	Background int  `json:"background"`
	Effects    bool `json:"effects"`
	BgmOn      bool `json:"bgmOn"`
	SeOn       bool `json:"seOn"`
	// Toku1 的「音声」「既読メッセージスキップ」是独立开关；MAZE 没有这两项。
	VoiceOn  bool `json:"voiceOn"`
	SkipRead bool `json:"skipRead"`
	// Siru 是 男汁/女汁/乳汁 三档（0=少ない,1=普通,2=多い）。
	Siru [3]int `json:"siru"`
}

func newGothicGlobalData() *gothicGlobalData {
	return &gothicGlobalData{
		Version:     gothicGlobalVersion,
		CG:          map[string]bool{},
		H:           map[string]bool{},
		PlayerFirst: "浩一",
		PlayerLast:  "広瀬",
		TextSpeed:   2,
		Background:  0,
		Effects:     true,
		BgmOn:       true,
		SeOn:        true,
		VoiceOn:     true,
		SkipRead:    true,
		Siru:        [3]int{1, 1, 1},
	}
}

func loadGothicGlobal(path string) *gothicGlobalData {
	data := newGothicGlobalData()
	raw, err := os.ReadFile(path)
	if err != nil {
		return data
	}
	loaded := newGothicGlobalData()
	if err := json.Unmarshal(raw, loaded); err != nil {
		core.Logf("gothic-global: parse '%s' failed: %v", path, err)
		return data
	}
	if loaded.CG == nil {
		loaded.CG = map[string]bool{}
	}
	if loaded.H == nil {
		loaded.H = map[string]bool{}
	}
	if loaded.PlayerFirst == "" {
		loaded.PlayerFirst = data.PlayerFirst
	}
	if loaded.PlayerLast == "" {
		loaded.PlayerLast = data.PlayerLast
	}
	if loaded.TextSpeed < 0 || loaded.TextSpeed > 3 {
		loaded.TextSpeed = data.TextSpeed
	}
	if loaded.Background < 0 || loaded.Background > 3 {
		loaded.Background = data.Background
	}
	for i := range loaded.Siru {
		if loaded.Siru[i] < 0 || loaded.Siru[i] > 2 {
			loaded.Siru[i] = 1
		}
	}
	loaded.Version = gothicGlobalVersion
	return loaded
}

func (g *gothicGlobalData) save(path string) {
	if g == nil || path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		core.Logf("gothic-global: mkdir failed: %v", err)
		return
	}
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		core.Logf("gothic-global: encode failed: %v", err)
		return
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o644); err != nil {
		core.Logf("gothic-global: write failed: %v", err)
		return
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		core.Logf("gothic-global: rename failed: %v", err)
	}
}

// gothicProfile 区分同一 brand 下的两套 Gothic 排版。
//
// MAZE（2000）与 特別病棟 旧版（Toku1）共用 MAINCON.SCS/GSA，
// 但字体尺寸、正文位置、菜单和存档槽数都不同。只用字体宽度判断
// 不够：字体缺失时会退回 0。因此打开时按数据目录里的标志文件
// 决定 profile，字体尺寸只作为二次校验。
type gothicProfile int

const (
	gothicProfileUnknown gothicProfile = iota
	gothicProfileMaze
	gothicProfileToku1
)

func detectGothicProfile(dataDir string, font *GothicFontFile) gothicProfile {
	// MAZE.EXE / sumbnail.gsa 是 MAZE 独有的锚点。prof.dat 不作为首要
	// 依据：Toku1 也可能带 CG 目录，单看它会误判。
	if findGothicFile(dataDir, "MAZE.EXE") != "" ||
		findGothicFile(dataDir, "SUMBNAIL.GSA") != "" {
		return gothicProfileMaze
	}
	if font != nil && font.GlyphWidth == 22 {
		return gothicProfileToku1
	}
	if findGothicFile(dataDir, "PROF.DAT") != "" {
		return gothicProfileMaze
	}
	return gothicProfileToku1
}

// gothicProfileNames 返回 prof.dat 里的 CG 顺序。
// 这是鉴赏室页签的权威顺序；缺失或格式损坏时调用方会退回扫描目录。
func (s *Session) loadGothicProfileCatalog() {
	s.cgNames = nil
	s.cgSet = map[string]bool{}
	path := findGothicFile(s.dataDir, "PROF.DAT")
	if path != "" {
		if raw, err := os.ReadFile(path); err == nil {
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			for index, line := range lines {
				fields := strings.Fields(strings.TrimSpace(line))
				if len(fields) == 0 {
					continue
				}
				// 第一行是条目数（113）。它可能被某些补丁改错，
				// 所以只跳过、不依赖它做循环上限。
				if index == 0 && len(fields) == 1 {
					continue
				}
				name := strings.ToLower(strings.TrimSuffix(fields[0], filepath.Ext(fields[0])))
				if name == "" {
					continue
				}
				s.cgNames = append(s.cgNames, name)
				s.cgSet[name] = true
				if len(fields) >= 2 && fields[1] == "1" && s.global != nil {
					s.global.CG[name] = true
				}
			}
		}
	}
	if len(s.cgNames) == 0 {
		s.cgNames = s.scanGothicCGNames()
	}
}

func (s *Session) scanGothicCGNames() []string {
	if s.catalog == nil {
		return nil
	}
	names := make([]string, 0, 128)
	for _, name := range s.catalog.Names() {
		if !isGothicAlbumImage(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isGothicAlbumImage 只收录原版鉴赏目录里的三类图：
// gi*（事件 CG）、gb*（背景）、end*（结局）。gc* 是立绘差分，
// 不在 prof.dat 的 113 条里，混进网格会打乱解锁位。
func isGothicAlbumImage(stem string) bool {
	lower := strings.ToLower(stem)
	return strings.HasPrefix(lower, "gi") ||
		strings.HasPrefix(lower, "gb") ||
		strings.HasPrefix(lower, "end")
}

func (s *Session) markCG(name string) {
	if s.global == nil || len(name) == 0 {
		return
	}
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if stem == "" {
		return
	}
	if !s.cgSet[stem] {
		return
	}
	if s.global.CG[stem] {
		return
	}
	s.global.CG[stem] = true
	s.globalDirty = true
}

func (s *Session) markHScene(index int) {
	if s.global == nil || index < 1 || index > 21 {
		return
	}
	key := hSceneKey(index)
	if s.global.H[key] {
		return
	}
	s.global.H[key] = true
	s.globalDirty = true
}

func hSceneKey(index int) string {
	const digits = "0123456789"
	return "h" + string([]byte{digits[(index/10)%10], digits[index%10]})
}

func (s *Session) flushGlobal() {
	if s.globalDirty && s.globalPath != "" && s.global != nil {
		s.global.save(s.globalPath)
		s.globalDirty = false
	}
}
