package save

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"kizuna/enginego/brand/overdose/script"
)

// 存档往返一致性（任务要求）：写一个带变量/舞台/音频/栈的 SlotData，
// 读回后逐字段相等。重点覆盖 6 个命名空间的槽位转换——变量的写读
// 桥接是这里唯一与 script 包互动的地方，必须钉住。
func TestSlotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)

	// 构造带真实变量写入的 VS，验证 Export→Import 闭环。
	vs := &script.VariableStore{}
	_ = vs.SetBool(0, 21, true)      // 全局 bool（剧情进度）
	_ = vs.SetInt(1, 64, 123)        // 全局 int（day/route 计数）
	_ = vs.SetString(2, 0, "bg011b") // 背景名（global string var0）
	_ = vs.SetString(2, 1, "yui")    // 角色名（global string var1）
	_ = vs.SetBool(3, 5, true)       // local bool
	_ = vs.SetInt(4, 0, -7)          // local int 负数
	_ = vs.SetString(5, 2, "ミヤコ")    // local string（角色名表）

	data := &SlotData{
		Title:      "第1日・序章",
		Summary:    "ミヤコ: ――匂い。",
		SavedAt:    time.Date(2024, 11, 9, 12, 0, 0, 0, time.FixedZone("UTC", 0)),
		Script:     "main",
		Stack:      []FrameState{{Script: "main", Offset: 0}, {Script: "s101", Offset: 256}},
		PC:         256,
		InstrIndex: 3,
		Variables:  ExportVariables(vs),
		Stage: StageState{
			Background:     "bg011b",
			Characters:     []LayerState{{Name: "yui", X: 400, Alpha: 255, Visible: true}},
			CG:             "",
			Fade:           "black",
			MessageVisible: true,
		},
		Audio:   AudioState{BgmFile: "bgm01", BgmLoop: true, BgmVol: 100},
		Message: MessageState{Speaker: "ミヤコ", Text: "――匂い。"},
	}

	if err := st.Save(3, data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// 原子落盘：临时文件必须已被 rename 清走。
	if _, err := os.Stat(filepath.Join(dir, "slot003.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("临时文件残留: %v", err)
	}
	// 槽位列举与存在性
	if slots := st.Slots(); len(slots) != 1 || slots[0] != 3 {
		t.Fatalf("Slots = %v, want [3]", slots)
	}
	if !st.Exists(3) || st.Exists(4) {
		t.Fatalf("Exists 判定不符")
	}

	loaded, err := st.Load(3)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatalf("Load 返回 nil")
	}
	if !data.SavedAt.Equal(loaded.SavedAt) {
		t.Fatalf("SavedAt = %v, want %v", loaded.SavedAt, data.SavedAt)
	}
	// 逐字段等值（忽略 Version/Slot 这两个 Store 填的字段）
	check := func(name string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
	check("Title", loaded.Title, data.Title)
	check("Summary", loaded.Summary, data.Summary)
	check("Script", loaded.Script, data.Script)
	check("Stack", loaded.Stack, data.Stack)
	check("PC", loaded.PC, data.PC)
	check("InstrIndex", loaded.InstrIndex, data.InstrIndex)
	check("Variables", loaded.Variables, data.Variables)
	check("Stage", loaded.Stage, data.Stage)
	check("Audio", loaded.Audio, data.Audio)
	check("Message", loaded.Message, data.Message)
	if loaded.Version != FormatVersion || loaded.Slot != 3 {
		t.Fatalf("Version/Slot = %d/%d, want %d/3", loaded.Version, loaded.Slot, FormatVersion)
	}

	// 变量回写：重新构造空 VS 后 Import 应得回同一批值。
	vs2 := &script.VariableStore{}
	ImportVariables(vs2, loaded.Variables)
	b, _ := vs2.GetBool(0, 21)
	if !b {
		t.Errorf("bool(0,21) 未恢复")
	}
	n, _ := vs2.GetInt(1, 64)
	if n != 123 {
		t.Errorf("int(1,64) = %d, want 123", n)
	}
	s, _ := vs2.GetString(2, 0)
	if s != "bg011b" {
		t.Errorf("string(2,0) = %q", s)
	}
	s1, _ := vs2.GetString(2, 1)
	if s1 != "yui" {
		t.Errorf("string(2,1) = %q", s1)
	}
	lb, _ := vs2.GetBool(3, 5)
	if !lb {
		t.Errorf("bool(3,5) 未恢复")
	}
	ln, _ := vs2.GetInt(4, 0)
	if ln != -7 {
		t.Errorf("int(4,0) = %d, want -7", ln)
	}
	ls, _ := vs2.GetString(5, 2)
	if ls != "ミヤコ" {
		t.Errorf("string(5,2) = %q", ls)
	}
	// 默认槽位（未写过）恢复后仍是默认值。
	d, _ := vs2.GetInt(1, 90)
	if d != 0 {
		t.Errorf("int(1,90) 应默认 0，实 %d", d)
	}

	// 空变量集导出 nil、导入 noop，不panic。
	if ExportVariables(&script.VariableStore{}) != nil {
		t.Errorf("空变量应导出 nil")
	}
	ImportVariables(nil, data.Variables) // 不应 panic
	ImportVariables(vs2, nil)

	// 覆盖写：同槽二次 Save 应替换内容。
	data.Title = "更新"
	if err := st.Save(3, data); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}
	again, _ := st.Load(3)
	if again.Title != "更新" {
		t.Fatalf("overwrite Title = %q", again.Title)
	}

	// Delete 幂等
	if err := st.Delete(3); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if st.Exists(3) {
		t.Fatalf("Delete 后仍存在")
	}
	if err := st.Delete(3); err != nil {
		t.Fatalf("Delete 幂等: %v", err)
	}
	none, err := st.Load(3)
	if err != nil || none != nil {
		t.Fatalf("Delete 后 Load = %+v, err=%v", none, err)
	}
}

func TestSystemRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sys := NewSystemStore(dir)

	data := &SystemData{
		CgUnlocks:    []string{"cgA02-04", "cgB01-01"},
		SceneUnlocks: []string{"s101", "s102"},
		MusicUnlocks: []string{"bgm01", "bgm10"},
		ReadTextKeys: []string{"s101#3", "s101#4"},
		Settings: Settings{
			TextSpeed:   60,
			AutoEnabled: true,
			AutoDelayMs: 800,
			SkipEnabled: true,
			BgmVolume:   80,
			SeVolume:    90,
			VoiceVolume: 100,
		},
	}
	if err := sys.Save(data); err != nil {
		t.Fatalf("System Save: %v", err)
	}
	loaded, err := sys.Load()
	if err != nil {
		t.Fatalf("System Load: %v", err)
	}
	// Store 负责写 Version；比较内容时去掉这个字段。
	want := *data
	want.Version = loaded.Version
	if !reflect.DeepEqual(loaded, &want) {
		t.Fatalf("System round-trip mismatch:\n got %+v\nwant %+v", loaded, &want)
	}

	// 全新安装：不存在时返回 (nil, nil)
	empty := NewSystemStore(t.TempDir())
	got, err := empty.Load()
	if err != nil || got != nil {
		t.Fatalf("empty Load = %+v, err=%v", got, err)
	}

	// 槽位 Store 与 SystemStore 指向同一目录（session.Close 只落一份）
	st := New(dir)
	if st.System() == nil || st.System().Path() == "" {
		t.Fatalf("Store.System 未初始化")
	}
}

func TestVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	data := &SlotData{Title: "x"}
	if err := st.Save(1, data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// 手工篡改版本：读取必须拒绝，而不是尽力恢复。
	path := st.SlotPath(1)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// 把 "version" 处的 1 换成 99
	for i := 0; i+9 < len(raw); i++ {
		if string(raw[i:i+9]) == `"version"` {
			raw[i+10] = '9'
			raw[i+11] = '9'
			break
		}
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := st.Load(1); err == nil {
		t.Fatalf("版本不符应拒绝")
	}
}
