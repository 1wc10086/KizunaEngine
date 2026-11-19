package config

import "testing"

// 构造“长度前缀行”文本。
func build(lines []string) []byte {
	out := make([]byte, 0, 128)
	for _, line := range lines {
		out = append(out, byte(len(line)), byte(len(line)>>8))
		out = append(out, []byte(line)...)
	}
	return out
}

func TestParseIniSections(t *testing.T) {
	data := build([]string{
		"[MESSAGEWINDOW]",
		"TEXT_DRAW_POS_X=90",
		"TEXT_FONT=26",
		"[SYSTEM]",
		"SYS_DEF_AUTOSPEED=6",
		"SYS_DEF_SOUND_VOLUME=65,90,80,80",
	})
	ini, err := ParseIni(data)
	if err != nil {
		t.Fatalf("ParseIni: %v", err)
	}
	if got := ini.Int("MESSAGEWINDOW", "TEXT_FONT", 0); got != 26 {
		t.Fatalf("TEXT_FONT = %d", got)
	}
	vols := ini.Ints("SYSTEM", "SYS_DEF_SOUND_VOLUME")
	if len(vols) != 4 || vols[0] != 65 || vols[3] != 80 {
		t.Fatalf("音量 = %v", vols)
	}
	if keys := ini.Keys("MESSAGEWINDOW"); len(keys) != 2 || keys[0] != "TEXT_DRAW_POS_X" {
		t.Fatalf("键顺序 = %v", keys)
	}
}

func TestParseMacroStripsPlaceholders(t *testing.T) {
	data := build([]string{
		"[FILEMACRO]",
		"fm_A01_0001=A01_00_00-00_a",
		"fm_A02_0002=A02_00_00-00_a/A02_00_00-01_-",
		"fm_empty=",
	})
	table, err := ParseMacro(data)
	if err != nil {
		t.Fatalf("ParseMacro: %v", err)
	}
	if parts := table["fm_A01_0001"]; len(parts) != 1 || parts[0] != "A01_00_00-00_a" {
		t.Fatalf("单部件 = %v", parts)
	}
	// '-' 是“该层留空”的占位符，必须被丢掉。
	if parts := table["fm_A02_0002"]; len(parts) != 1 || parts[0] != "A02_00_00-00_a" {
		t.Fatalf("占位部件 = %v", parts)
	}
	if parts, ok := table["fm_empty"]; !ok || len(parts) != 0 {
		t.Fatalf("空宏 = %v, %v", parts, ok)
	}
}

func TestIsTextConfigRejectsScriptContainer(t *testing.T) {
	// .sct 的头是 u32 记录数；头两个字节当 u16 长度会立刻越界。
	if IsTextConfig([]byte{2, 0, 0, 0, 6, 0, 1, 2, 3, 4}) {
		t.Fatal(".sct 被误判成文本配置")
	}
}
