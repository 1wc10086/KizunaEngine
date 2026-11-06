package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/data/scriptdb"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/core"
)

// scriptdata.go 统一加载 BSXScript 3.3 作品的数据表：var_db 数值表和成就表。
//
// 这两份数据都是"按作品资源名配置、按名字寻址"的：剧本和 Lua 只写
// `#ID_BP[0,'従順度']` / `実績解除 ID17`，不知道 CSV 文件叫什么。把加载
// 放在一个入口，普通 AVG（没有 var_db）也能安全调用：资源缺失时返回 nil
// 或空表，调用方按 0/未解锁处理。

// resourceReader 把 GameResourceIndex 适配成 scriptdb.Reader。
// 单列出来是为了让数据层不依赖 resource 包，避免 session 的加载顺序被
// BSA 打开时机绑死。
func (s *Session) resourceReader() scriptdb.Reader {
	return func(name string) ([]byte, bool) {
		if s.res == nil || name == "" {
			return nil, false
		}
		location, ok := s.res.TryGet(name)
		if !ok || location == nil {
			return nil, false
		}
		return location.Read(), true
	}
}

// scriptDatabaseName 读取 [main window] db name；缺省值时原引擎不加载
// var_db，普通 AVG 因此没有数值表。
func (s *Session) scriptDatabaseName() string {
	if s.config == nil {
		return ""
	}
	return strings.TrimSpace(s.config.Raw(config.WindowSection, "db name", ""))
}

// ensureScriptDB 惰性加载 var_db。
//
// 加载后即使表为空也保留 DB 对象：SLG 的 @アジト補正 是 Lua 运行时用
// SetDB 新建的，空 DB 仍然可以按需建表。
func (s *Session) ensureScriptDB() *scriptdb.DB {
	if s.scriptDBDone {
		return s.scriptDB
	}
	s.scriptDBDone = true
	name := s.scriptDatabaseName()
	if name == "" {
		return nil
	}
	s.scriptDB = scriptdb.Load(s.resourceReader())
	core.Logf("scriptdb: '%s' loaded (%d table(s))", name, s.scriptDB.TableCount())
	return s.scriptDB
}

// ensureAchievements 惰性读取 achievement db。
func (s *Session) ensureAchievements() *data.AchievementTable {
	if s.achievementsDone {
		return s.achievements
	}
	s.achievementsDone = true
	if s.config == nil {
		return nil
	}
	name := strings.TrimSpace(s.config.Raw(config.WindowSection, "achievement db", ""))
	if name == "" {
		return nil
	}
	content, ok := s.resourceReader()(name)
	if !ok {
		core.Logf("achievements: '%s' not found", name)
		return nil
	}
	s.achievements = data.ParseAchievementTable(content)
	s.loadAchievementFlags()
	core.Logf("achievements: '%s' loaded (%d entries)", name, s.achievements.Len())
	return s.achievements
}

func (s *Session) achievementUnlocked(id string) bool {
	if s.unlockedAchievements == nil {
		return false
	}
	return s.unlockedAchievements[id]
}

// unlockAchievement 记录解锁。返回 false 表示之前已经解锁，调用方据此
// 决定是否显示提示，避免读档后重复弹同一个成就。
func (s *Session) unlockAchievement(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if s.unlockedAchievements == nil {
		s.unlockedAchievements = make(map[string]bool)
	}
	if s.unlockedAchievements[id] {
		return false
	}
	s.unlockedAchievements[id] = true
	s.persistAchievementFlag(id)
	core.Logf("achievements: unlocked %s", id)
	return true
}

// loadAchievementFlags 从 common.dat 的成就表读出跨周目解锁状态。
//
// 成就表在原引擎的全局存档里，结构是定长 12 字节：`IDnn` + 1 字节解锁位
// + 7 字节保留。表在 Prefix 里、偏移随作品/版本变化，所以按 "IDnn" 扫描并
// 校验后面 7 个字节全 0；不用固定偏移，换作品也能读。读不到就保持未解锁，
// 不会把状态画面变成空白页。
func (s *Session) loadAchievementFlags() {
	if s.achievements == nil {
		return
	}
	content, err := os.ReadFile(s.commonSavePath())
	if err != nil {
		return
	}
	loaded := 0
	for _, entry := range s.achievements.Entries() {
		if offset := findAchievementFlag(content, entry.ID); offset >= 0 && content[offset] != 0 {
			if s.unlockedAchievements == nil {
				s.unlockedAchievements = make(map[string]bool)
			}
			s.unlockedAchievements[entry.ID] = true
			loaded++
		}
	}
	if loaded > 0 {
		core.Logf("achievements: %d unlocked flag(s) restored from common.dat", loaded)
	}
}

// persistAchievementFlag 只改 1 字节解锁位后原子写回，保留原文件的其余内容。
//
// 不重写整个 GlobalSave：牝教師3 没有 cg.dat/scene.dat，GlobalSave 的锚点
// 校验会失败；直接按名字定位成就字节，既不会破坏原引擎的设置块，也不需要
// 理解 Prefix 的布局。
func (s *Session) persistAchievementFlag(id string) {
	path := s.commonSavePath()
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	offset := findAchievementFlag(content, id)
	if offset < 0 || content[offset] != 0 {
		return
	}
	content[offset] = 1
	temp := path + ".tmp"
	if err := os.WriteFile(temp, content, 0o644); err != nil {
		return
	}
	if err := os.Rename(temp, path); err != nil {
		core.Logf("achievements: write '%s' failed: %v", path, err)
	}
}

// commonSavePath 找原引擎实际使用的 common.dat。
//
// 多数作品把它放在 data/save 下；牝教師3 的发行包是 `Game/data` + 根目录
// `Save/save`（启动器把存档放在游戏根之外）。只查 data/save 会读不到玩家的
// 成就/CG 进度，写回时还会在错误的位置新建一份空档。
func (s *Session) commonSavePath() string {
	candidates := []string{
		filepath.Join(dataSaveDir(s.dataDir), "common.dat"),
		filepath.Join(filepath.Dir(s.dataDir), "save", "common.dat"),
		filepath.Join(s.dir, "Save", "save", "common.dat"),
		filepath.Join(filepath.Dir(filepath.Dir(s.dataDir)), "Save", "save", "common.dat"),
	}
	for _, candidate := range candidates {
		if core.PathExists(candidate) {
			return candidate
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return save.GlobalSavePath(s.dataDir)
}

// findAchievementFlag 返回解锁位字节的下标，找不到返回 -1。
func findAchievementFlag(content []byte, id string) int {
	needle := []byte(id)
	if len(needle) != 4 {
		return -1
	}
	for start := 0; start+12 <= len(content); start++ {
		if !bytes.Equal(content[start:start+4], needle) {
			continue
		}
		// 记录是 12 字节：ID(4) + flag(1) + 3 个 0 + 一个 int32（通常是 4）。
		// 只校验 flag 后面的 3 个 0；把后面 4 字节也算进去会永远匹配失败。
		reserved := true
		for index := start + 5; index < start+8; index++ {
			if content[index] != 0 {
				reserved = false
				break
			}
		}
		if reserved {
			return start + 4
		}
	}
	return -1
}
