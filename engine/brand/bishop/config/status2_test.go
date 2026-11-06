package config

import "testing"

// OH/OHIE 的 [status2 screen] 有三种容易读错的键：
//   - register 是系统参数键，不是属性下标；
//   - lvN message voice 是四段键名，不能只匹配 "lvN message"；
//   - chN voice button / chN dr voice button 共用同一张 4 帧按钮图。
func TestParseStatus2Screen(t *testing.T) {
	document := ParseIniDocumentText(`
[status2 screen]
base = karte_select_base
target0 = karte_select_0, 3, 30, 202
target1 = karte_select_1, 3, 212, 202
back button = karte_back, 3, 712, 570
register = 10, 11

ch0 base = karte_base0
ch0 message pos = 56, 302
ch0 diagnosis pos = 56, 410
ch0 sd pos = 404, 192
ch0 lv1 message = karte_base0_a1mes
ch0 lv2 message = karte_base0_a2mes
ch0 lv1 message voice = 01_09041_001
ch0 lv1 diagnosis = karte_base0_b1mes
ch0 lv1 diagnosis voice = 05_09041_001
ch0 lv1 sd = karte_base0_a1sd
ch0 voice button = karte_voice, 4, 338, 376
ch0 dr voice button = karte_voice, 4, 338, 484

ch1 base = karte_base1
ch1 lv1 message = karte_base1_a1mes
`)

	screen := parseStatus2Screen(document)
	if !screen.HasLayout() {
		t.Fatal("status2 layout should be usable")
	}
	if len(screen.Targets) != 2 || len(screen.Characters) != 2 {
		t.Fatalf("targets=%d characters=%d", len(screen.Targets), len(screen.Characters))
	}
	if len(screen.Register) != 2 || screen.Register[0] != 10 || screen.Register[1] != 11 {
		t.Fatalf("register=%v", screen.Register)
	}
	if screen.Characters[0].Messages[0] != "karte_base0_a1mes" ||
		screen.Characters[0].Messages[1] != "karte_base0_a2mes" {
		t.Fatalf("messages=%v", screen.Characters[0].Messages)
	}
	if screen.Characters[0].MessageVoices[0] != "01_09041_001" ||
		screen.Characters[0].DiagnosisVoices[0] != "05_09041_001" {
		t.Fatalf("voices=%v / %v",
			screen.Characters[0].MessageVoices, screen.Characters[0].DiagnosisVoices)
	}
	if screen.Characters[0].VoiceButton.Frames != 4 ||
		screen.Characters[0].DiagnosisButton.Y != 484 {
		t.Fatalf("voice buttons=%+v %+v",
			screen.Characters[0].VoiceButton, screen.Characters[0].DiagnosisButton)
	}
	if screen.Characters[0].MessagePos.Y != 302 || screen.Characters[0].SDPos.X != 404 {
		t.Fatalf("positions=%+v %+v", screen.Characters[0].MessagePos, screen.Characters[0].SDPos)
	}
}
