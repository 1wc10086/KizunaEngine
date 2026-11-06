package config

import "testing"

// 特別授業3SLG 旧版只有 4 个页签（没有 BGV），页签语义必须来自图片名。
func TestParseMuseumModesUseImageName(t *testing.T) {
	document := ParseIniDocumentText(`
[museum phase]
base = museum_base
mode0 = museum_music_mode, 4, 50, 384
mode0 state = 0x00000001
mode1 = museum_cg_mode, 4, 51, 416
mode1 state = 0x00000002
mode2 = museum_scene_mode, 4, 50, 448
mode2 state = 0x00000004
mode3 = museum_strip_mode, 4, 48, 480
mode3 state = 0x00000008
return to title = museum_back, 2, 50, 528
`)
	museum := parseMuseum(document)
	want := []string{"music", "cg", "scene", "strip"}
	if len(museum.Modes) != len(want) {
		t.Fatalf("%d modes, want %d", len(museum.Modes), len(want))
	}
	for index, key := range want {
		if museum.Modes[index].Key != key {
			t.Fatalf("mode%d = %q, want %q", index, museum.Modes[index].Key, key)
		}
	}
}

// 牝教師3 的立绘鉴赏不是 museum_buex_layout，而是 [bu viewer] 的多角色界面：
// [museum phase] multi bu = 1 必须被读出来，否则引擎只画选中角色，
// 角色卡旁的 ON/OFF 也只画一个，和原版截图（多人同屏、各自 ON/OFF）不符。
func TestParseMuseumMultiBu(t *testing.T) {
	document := ParseIniDocumentText(`
[museum phase]
base = museum_base
multi bu = 1
`)
	museum := parseMuseum(document)
	if !museum.MultiBu {
		t.Fatal("multi bu = 1 should enable MultiBu")
	}
	document = ParseIniDocumentText("[museum phase]\nbase = museum_base\n")
	if parseMuseum(document).MultiBu {
		t.Fatal("missing multi bu must keep MultiBu false")
	}
}

func TestParseMuseumModeFallback(t *testing.T) {
	document := ParseIniDocumentText(`
[museum phase]
base = museum_base
mode0 = custom_icon_a, 2, 0, 0
mode1 = custom_icon_b, 2, 0, 0
`)
	museum := parseMuseum(document)
	// 无法从图名归类时退回原来的下标顺序。
	if len(museum.Modes) != 2 || museum.Modes[0].Key != "music" || museum.Modes[1].Key != "bgv" {
		t.Fatalf("fallback modes = %+v", museum.Modes)
	}
}

// SL（BSXScript 2.0）的立ち絵鉴赏页签图叫 museum_dress_mode，内容就是
// strip 页。它必须被识别成 strip，否则会掉进按位置回退的 scene，
// 玩家点立绘鉴赏时看到的还是回想页，像是按钮没反应。
func TestMuseumModeKeyDressAlias(t *testing.T) {
	cases := map[string]string{
		"museum_dress_mode": "strip",
		"museum_bu_mode":    "strip",
		"museum_mode_strip": "strip",
		"museum_bgv_mode":   "bgv",
		"museum_cg_mode":    "cg",
		"museum_scene_mode": "scene",
		"museum_music_mode": "music",
		"custom_icon_a":     "",
	}
	for image, want := range cases {
		if got := museumModeKey(image); got != want {
			t.Fatalf("museumModeKey(%q) = %q, want %q", image, got, want)
		}
	}
}

func TestParseLegacyBuViewer(t *testing.T) {
	document := ParseIniDocumentText(`
[museum phase - bu]
base = museum_bu
target0 = museum_bu_ch1, 2, 0, 0
target0 no = 0
target1 = museum_bu_ch2, 2, 0, 0
target1 no = 2

[bu viewer]
base = dress_view_base
exit button = dress_view_back, 3, 912, 544
on/off button = dress_view_appearance, 4, 0, 28
switch image = dress_view_switch, 3
pos region = 122, 556, 780, 20
default order = 1, 2, 3, 4
character icon0 = dress_view_ch1, 3, 8, 20
character icon1 = dress_view_ch2, 3, 8, 60
enable view control = 1
settings = 7
setting0 pos = 10, 260
setting0 space = 16, 16
setting0 repeat x = 3
setting4 pos = -16, -16
setting5 pos = 10, 296
setting5 space = 16, 16
setting5 repeat x = 6
setting6 pos = 10, 436
setting6 space = 16, 16
setting6 repeat x = 6
pose signature = abc
ngng level = 3
ngng convert table = 0, 3, 5
ch1 wear = 1, 2, 3, 0
ch1 under wear = 1
ch1 face pose1 = 1, 2, 3
ch2 wear = 1, 2, 0
ch2 face pose1 = 4, 5
`)
	page := parseMuseumBUPage(document)
	if len(page.Targets) != 2 || page.Targets[1].Character != 2 {
		t.Fatalf("targets = %+v", page.Targets)
	}
	viewer := parseBuViewer(document)
	if viewer.Empty() {
		t.Fatal("viewer should not be empty")
	}
	if len(viewer.Icons) != 2 || len(viewer.Characters) != 2 {
		t.Fatalf("icons=%d characters=%d", len(viewer.Icons), len(viewer.Characters))
	}
	// 末尾的 0 表示"没有这一件"，必须去掉。
	if got := viewer.Characters[0].Wear; len(got) != 3 {
		t.Fatalf("ch1 wear = %v", got)
	}
	// setting0..6 共 7 组；缺项占位，保证组下标和渲染层的语义对齐。
	if len(viewer.Settings) != 7 {
		t.Fatalf("settings = %+v", viewer.Settings)
	}
	if !viewer.Settings[1].Disabled {
		t.Fatal("negative setting pos must be treated as disabled")
	}
	if viewer.NgngLevel != 3 || len(viewer.NgngConvert) != 3 {
		t.Fatalf("ngng level=%d convert=%v", viewer.NgngLevel, viewer.NgngConvert)
	}
}

// SL 的 [game menu] base pos = (452,412)；sync button pos = 1 时
// [system button] 的坐标是相对这个基点的。只加 Y 会把工具栏留在左边、
// 和正文重叠；必须同时加 X（侵蝕/淫獄 的 BaseX=0，不受影响）。
func TestParseGameMenuSyncPosition(t *testing.T) {
	document := ParseIniDocumentText(`
[game menu]
base = gamemenu_base
base pos = 452, 412
sync button pos = 1
[system button]
skip = btn_skip, 4, 36, 141, game
`)
	menu := parseGameMenu(document)
	if len(menu.Buttons) != 1 {
		t.Fatalf("buttons = %d", len(menu.Buttons))
	}
	button := menu.Buttons[0]
	if button.X != 488 || button.Y != 553 {
		t.Fatalf("skip at %d,%d, want 488,553", button.X, button.Y)
	}
}
