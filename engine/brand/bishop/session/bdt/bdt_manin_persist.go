package bdt

import (
	"encoding/json"
	"os"
	"path/filepath"

	"kizuna/engine/core"
)

// bdt_manin_persist.go 负责 manin 路的两个旁车文件：
//   - manin/global.json：跨存档的设置、主角名、CG/场景解锁；
//   - manin/slotNNN.json：单个存档的責め言葉、现场标题、车站等。
//
// BDT 的二进制槽格式保持不变（AFDVD 与 manin 共用），所以 manin 专有字段
// 不塞进 slotNNN.bds，避免版本号变更让旧存档失效。读失败一律回退默认值：
// 存档损坏时宁可丢设置，也不能让游戏打不开。

const (
	maninGlobalFile   = "global.json"
	maninSlotFileBase = "slot"
)

func maninDataDir(saveDir string) string {
	if saveDir == "" {
		return ""
	}
	return filepath.Join(saveDir, "manin")
}

func maninGlobalPath(saveDir string) string {
	if saveDir == "" {
		return ""
	}
	return filepath.Join(maninDataDir(saveDir), maninGlobalFile)
}

func maninSlotPath(saveDir string, slot int) string {
	if saveDir == "" {
		return ""
	}
	return filepath.Join(maninDataDir(saveDir), maninSlotFileBase+threeDigits(slot)+".json")
}

func threeDigits(value int) string {
	if value < 0 {
		value = 0
	}
	digits := []byte{'0', '0', '0'}
	for i := 2; i >= 0; i-- {
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits)
}

func maninDefaultSettings() maninSettings {
	settings := maninSettings{
		Fullscreen:  true,
		EffectOn:    true,
		BgmOn:       true,
		SeOn:        true,
		VoiceOn:     true,
		SkipFast:    0,
		SkipUnread:  true,
		AutoSpeed:   5,
		VolumeBgm:   0.8,
		VolumeSe:    0.8,
		VolumeVoice: 0.8,
	}
	for i := range settings.VoiceByGirl {
		settings.VoiceByGirl[i] = true
	}
	return settings
}

func maninDefaultGlobal() maninGlobal {
	return maninGlobal{
		Name:          "松井 一朗",
		Settings:      maninDefaultSettings(),
		UnlockedCG:    map[string]bool{},
		UnlockedScene: map[string]bool{},
	}
}

func loadManinGlobal(saveDir string) maninGlobal {
	global := maninDefaultGlobal()
	path := maninGlobalPath(saveDir)
	if path == "" {
		return global
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return global
	}
	// 先放进默认值副本再反序列化：旧版本文件缺字段时会保留默认，而不是变成
	// 全 false（全 false 会让 BGM/SE 默认静音，玩家会以为引擎坏了）。
	if err := json.Unmarshal(data, &global); err != nil {
		core.Logf("bdt manin: global read failed: %v", err)
		return maninDefaultGlobal()
	}
	if global.UnlockedCG == nil {
		global.UnlockedCG = map[string]bool{}
	}
	if global.UnlockedScene == nil {
		global.UnlockedScene = map[string]bool{}
	}
	return global
}

func saveManinGlobal(saveDir string, global maninGlobal) {
	path := maninGlobalPath(saveDir)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		core.Logf("bdt manin: global mkdir failed: %v", err)
		return
	}
	data, err := json.MarshalIndent(global, "", "  ")
	if err != nil {
		core.Logf("bdt manin: global encode failed: %v", err)
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		core.Logf("bdt manin: global write failed: %v", err)
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		core.Logf("bdt manin: global rename failed: %v", err)
		_ = os.Remove(temporary)
	}
}

func loadManinSlotData(saveDir string, slot int, fallback maninSlotData) maninSlotData {
	path := maninSlotPath(saveDir, slot)
	if path == "" {
		return fallback
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return fallback
	}
	out := fallback
	if err := json.Unmarshal(data, &out); err != nil {
		core.Logf("bdt manin: slot %d sidecar read failed: %v", slot, err)
		return fallback
	}
	return out
}

func saveManinSlotData(saveDir string, slot int, data maninSlotData) {
	path := maninSlotPath(saveDir, slot)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		core.Logf("bdt manin: slot mkdir failed: %v", err)
		return
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		core.Logf("bdt manin: slot %d encode failed: %v", slot, err)
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, encoded, 0o644); err != nil {
		core.Logf("bdt manin: slot %d write failed: %v", slot, err)
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		core.Logf("bdt manin: slot %d rename failed: %v", slot, err)
		_ = os.Remove(temporary)
	}
}

func (u *maninRuntime) unlockCG(name string) bool {
	if u == nil || name == "" {
		return false
	}
	if u.global.UnlockedCG == nil {
		u.global.UnlockedCG = map[string]bool{}
	}
	if u.global.UnlockedCG[name] {
		return false
	}
	u.global.UnlockedCG[name] = true
	return true
}

func (u *maninRuntime) unlockScene(name string) bool {
	if u == nil || name == "" {
		return false
	}
	if u.global.UnlockedScene == nil {
		u.global.UnlockedScene = map[string]bool{}
	}
	if u.global.UnlockedScene[name] {
		return false
	}
	u.global.UnlockedScene[name] = true
	return true
}

func (u *maninRuntime) cgUnlocked(name string) bool {
	return u != nil && u.global.UnlockedCG != nil && u.global.UnlockedCG[name]
}

func (u *maninRuntime) sceneUnlocked(name string) bool {
	return u != nil && u.global.UnlockedScene != nil && u.global.UnlockedScene[name]
}
