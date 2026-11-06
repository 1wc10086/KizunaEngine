package config

import "testing"

func TestParseLegacyConfigScreen(t *testing.T) {
	document := ParseIniDocumentText(`[config screen]
config base = config_base
check mark = cfg_check
screen mode = 408, 173
effect detail = detailed_effect, 2, 507, 221
volume channels = 7
volume0 bar = cfg_volume_bar, 1, 496, 168
volume0 min = cfg_guage_min, 2, 454, 168
characters = 2
character0 = cfg_ch1, 2, 873, 581
character0 detail = cfg_voice_detail, 2, 873, 669

[detailed effect setting]
base = des_base
cutin = 749, 311
`)

	screen := parseLegacyConfigScreen(document)
	if screen.Base != "config_base" || screen.ScreenMode.X != 408 || screen.ScreenMode.Y != 173 {
		t.Fatalf("screen=%+v", screen)
	}
	if len(screen.Volumes) != 1 || screen.Volumes[0].Bar.Image != "cfg_volume_bar" {
		t.Fatalf("volumes=%+v", screen.Volumes)
	}
	if len(screen.Characters) != 1 || screen.Characters[0].Detail.Y != 669 {
		t.Fatalf("characters=%+v", screen.Characters)
	}
	if screen.Effect == nil || len(screen.Effect.Toggles) != 1 || screen.Effect.Toggles[0].Key != "cutin" {
		t.Fatalf("effect=%+v", screen.Effect)
	}
}

func TestParseLegacyConfigBgmSelectorAndAhe(t *testing.T) {
	document := ParseIniDocumentText(`[config screen]
config base = config_base
bgm selector pos0 = 84, 352
bgm selector pos1 = 284, 352

[detailed effect setting]
base = des_base
steam mode = 588, 248
ahe mode = 408, 276
`)

	screen := parseLegacyConfigScreen(document)
	if len(screen.BgmSelectors) != 2 ||
		screen.BgmSelectors[0].X != 84 || screen.BgmSelectors[1].X != 284 {
		t.Fatalf("bgm selectors = %+v", screen.BgmSelectors)
	}
	if screen.Effect == nil {
		t.Fatal("missing effect page")
	}
	keys := make([]string, 0, len(screen.Effect.Toggles))
	for _, toggle := range screen.Effect.Toggles {
		keys = append(keys, toggle.Key)
	}
	if len(keys) != 2 || keys[0] != "steam mode" || keys[1] != "ahe mode" {
		t.Fatalf("effect toggles = %v", keys)
	}
}
