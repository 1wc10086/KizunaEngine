package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadConfigScreenSample(t *testing.T) {
	screen := loadConfigScreenSample(t)

	if screen.Base != "cfgx_base" {
		t.Fatalf("base = %q", screen.Base)
	}

	font := screen.Font
	if font.Size != 14 || font.Weight != 400 {
		t.Fatalf("font = %dx%d", font.Size, font.Weight)
	}
	if font.FontFace != "ＭＳ 明朝" || font.EudcFontFace != "bsxx.ttf" {
		t.Fatalf("font face = %q/%q", font.FontFace, font.EudcFontFace)
	}
	// 0xffffffff 必须按无符号颜色读，而不是被当成负数。
	if font.Color != 0xffffffff {
		t.Fatalf("font color = %#x", font.Color)
	}
	if font.Align != "" {
		t.Fatalf("font align = %q, want empty", font.Align)
	}

	if len(screen.Categories) != 3 {
		t.Fatalf("%d categories, want 3", len(screen.Categories))
	}
	wantLayouts := []string{"screen", "text", "sound"}
	for index, layout := range wantLayouts {
		category := screen.Categories[index]
		if category.Layout != layout {
			t.Fatalf("category %d layout = %q, want %q", index, category.Layout, layout)
		}
		if !category.Description.HasImage {
			t.Fatalf("category %d should have an image description", index)
		}
	}

	first := screen.Categories[0]
	if first.Button.Name != "cfgx_category_screen" || first.Button.MaxState != 4 {
		t.Fatalf("first category button = %+v", first.Button)
	}
	if first.Button.X != 77 || first.Button.Y != 127 {
		t.Fatalf("first category button pos = %d,%d", first.Button.X, first.Button.Y)
	}
	if first.Description.Image != "cfgx_category_screen_desc" ||
		first.Description.X != 77 || first.Description.Y != 157 {
		t.Fatalf("first category description = %+v", first.Description)
	}

	if len(screen.Common) != 4 {
		t.Fatalf("%d common buttons, want 4", len(screen.Common))
	}
	if screen.Common[0].No != 124 || screen.Common[1].No != 125 ||
		screen.Common[2].No != 126 || screen.Common[3].No != 127 {
		t.Fatalf("common no = %d/%d/%d/%d",
			screen.Common[0].No, screen.Common[1].No,
			screen.Common[2].No, screen.Common[3].No)
	}
	if screen.Common[0].Button.Name != "cfgx_reset" ||
		screen.Common[0].Button.X != 77 || screen.Common[0].Button.Y != 181 {
		t.Fatalf("reset button = %+v", screen.Common[0].Button)
	}

	if len(screen.Pages) != 3 {
		t.Fatalf("%d pages, want 3", len(screen.Pages))
	}
	for _, name := range wantLayouts {
		if _, ok := screen.Pages[name]; !ok {
			t.Fatalf("missing %q page", name)
		}
	}
	if screen.Pages["screen"].Base != "cfgx_base_screen" {
		t.Fatalf("screen base = %q", screen.Pages["screen"].Base)
	}
	if screen.Pages["text"].Base != "cfgx_base_text" {
		t.Fatalf("text base = %q", screen.Pages["text"].Base)
	}
	if screen.Pages["sound"].Base != "cfgx_base_sound" {
		t.Fatalf("sound base = %q", screen.Pages["sound"].Base)
	}

	volume := mustObject(t, screen.Pages["sound"].Functions["volume"], "sound volume")
	master := mustObject(t, volume["master"], "volume master")
	masterBar := mustBar(t, master, "volume master")
	if masterBar.Name != "cfgx_volume_bar" || masterBar.X != 231 || masterBar.Y != 374 {
		t.Fatalf("master volume bar = %+v, want [231,374]", masterBar)
	}

	messageSpeed := mustObject(t,
		screen.Pages["text"].Functions["message speed"], "message speed")
	messageBar := mustBar(t, messageSpeed, "message speed")
	if messageBar.Name != "cfgx_text_bar" || messageBar.X != 743 || messageBar.Y != 300 {
		t.Fatalf("message speed bar = %+v, want [743,300]", messageBar)
	}

	// "317+30*N" 是原配置里的写法，必须按算式求值，不能当成字符串。
	actors, ok := Array(screen.Pages["sound"].Functions["actor setting"])
	if !ok {
		t.Fatal("actor setting should be an array")
	}
	secondActor := mustObject(t, actors[1], "actor 2")
	toggle := mustObject(t, secondActor["toggle"], "actor 2 toggle")
	button, ok := ParseButton(toggle)
	if !ok {
		t.Fatalf("actor 2 toggle is not a button: %+v", toggle)
	}
	if button.Y != 317+30*1 {
		t.Fatalf("actor 2 toggle y = %d, want %d", button.Y, 317+30*1)
	}
}

func TestLoadConfigScreenInlineFormat(t *testing.T) {
	// 覆盖样例里出现但容易被标准 JSON 解析器拒绝的写法：
	// 无引号键、无逗号、可选逗号、';' 注释、算式和字符串转义。
	document := `{
		; 注释里的 { [ , : 都不影响解析
		base: "base_inline"

		"description font":{
			size: 14,
			weight: 400
			fontface: "F"
			eudcfontface: "E"
			color: 0xffffffff
		}

		"category button":[
			{layout:"screen", image:{name:"cat_screen", max_state:4,
				pos:[(16 + 184) * 2, 20]},
				description:{image:"cat_screen_desc", pos:[7,8]}}
		]

		"common button": [
			{no:124, image:{name:"reset", max_state:3, pos:[0, 4]}}
		]

		layout:{
			"screen":{
				base:"page_base"
				function:{
					"speed":{
						bar:{name:"bar_x", pos:[5, 6]}
						text:{text:"line1\nline2"}
					}
				}
			}
		}
	}`

	screen, err := LoadConfigScreen([]byte(document))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if screen.Base != "base_inline" {
		t.Fatalf("base = %q", screen.Base)
	}
	if screen.Font.Color != 0xffffffff {
		t.Fatalf("color = %#x", screen.Font.Color)
	}
	if len(screen.Categories) != 1 {
		t.Fatalf("%d categories, want 1", len(screen.Categories))
	}
	category := screen.Categories[0]
	if category.Button.X != 400 || category.Button.Y != 20 {
		t.Fatalf("category button pos = %d,%d, want 400,20",
			category.Button.X, category.Button.Y)
	}
	if !category.Description.HasImage || category.Description.X != 7 {
		t.Fatalf("description = %+v", category.Description)
	}
	if len(screen.Common) != 1 || screen.Common[0].No != 124 {
		t.Fatalf("common = %+v", screen.Common)
	}

	speed := mustObject(t, screen.Pages["screen"].Functions["speed"], "speed")
	bar := mustBar(t, speed, "speed")
	if bar.Name != "bar_x" || bar.X != 5 || bar.Y != 6 {
		t.Fatalf("bar = %+v", bar)
	}
	text := mustObject(t, speed["text"], "speed text")
	textValue, ok := Text(text["text"])
	if !ok || !strings.Contains(textValue, "\n") {
		t.Fatalf("escaped text = %q, want a newline", textValue)
	}
}

func TestLoadConfigScreenRejectsMalformed(t *testing.T) {
	if _, err := LoadConfigScreen([]byte(`{ base: "x"`)); err == nil {
		t.Fatal("an unterminated object should fail")
	}
	if _, err := LoadConfigScreen([]byte(`{ size: 1 + }`)); err == nil {
		t.Fatal("an incomplete expression should fail")
	}
	if _, err := LoadConfigScreen([]byte(`[]`)); err == nil {
		t.Fatal("a non-object document should fail")
	}
}

func loadConfigScreenSample(t *testing.T) *ConfigScreen {
	t.Helper()

	path := os.Getenv("KIZUNA_CONFIG_LAYOUT")
	if len(path) == 0 {
		path = "/tmp/system_config_ex.layout"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("sample layout not found at %s", path)
		}
		t.Fatalf("read sample: %v", err)
	}

	screen, err := LoadConfigScreen(data)
	if err != nil {
		t.Fatalf("parse sample %s: %v", path, err)
	}
	return screen
}

func mustObject(t *testing.T, value any, name string) map[string]any {
	t.Helper()

	object, ok := Object(value)
	if !ok {
		t.Fatalf("%s is not an object: %T", name, value)
	}
	return object
}

func mustBar(t *testing.T, object map[string]any, name string) ConfigButton {
	t.Helper()

	button, ok := ParseButton(object["bar"])
	if !ok {
		t.Fatalf("%s has no bar button", name)
	}
	return button
}
