package config

import "testing"

// buexLayoutSample 是从原引擎 define.bsa 的 museum_buex_layout 抄来的样例，
// 只做了裁剪：注释去掉了大部分，data_store.edit_command 里多出来的 "base"
// 按钮（原文件里有）被删掉，这样存档命令正好是 save/load/toslot/output/clear
// 五个。坐标和算式保持原样，用来验证 "215+135*0" 这类写法确实被求值。
const buexLayoutSample = `{
	main_menu: {
		setting_bgv:    { name:"buex_main_bgv",       max_state:4, pos:[424,   5] }
		bg_select:      { name:"buex_main_changebg",  max_state:4, pos:[304,   5] }
		setting_bu:     { name:"buex_main_bureplace", max_state:4, pos:[784,   5] }
		edit_message:   { name:"buex_main_setmsg",    max_state:4, pos:[544,   5] }
		data_store:     { name:"buex_main_save",      max_state:4, pos:[664,   5] }
		back_to_thumbs: { name:"buex_main_back",      max_state:3, pos:[904,   5] }
	}

	notice: {
		bgv:     "buex_notice_bgv"
		bg:      "buex_notice_bg"
		bu:      "buex_notice_bu"
		msgwnd:  "buex_notice_msgwnd"
		freemsg: "buex_notice_freemsg"
		data:    "buex_notice_data"
	}

	loading: { name:"loading", max_state:8, pos:[0,0] }

	setting_bgv: {
	}

	bg_select: {
		base: "buex_bgbase"

		number_font: { name:"museum_cg_parts", max_state:11, pos:[114, 69], next_pos:[-13, 0] }

		subject:{
			pos: [2, -15],
			size: .5,
		}

		page_select: [
			[
				{ name:"museum_page41", max_state:4, pos:[  10,  97] }
				{ name:"museum_page42", max_state:4, pos:[  10, 124] }
			],
			[
				{ name:"museum_page43", max_state:4, pos:[  10, 189] }
				{ name:"museum_page44", max_state:4, pos:[  10, 287] }
				{ name:"museum_page45", max_state:4, pos:[  10, 375] }
				{ name:"museum_page46", max_state:4, pos:[  10, 463] }
				{ name:"museum_page47", max_state:4, pos:[  10, 551] }
				{ name:"museum_page48", max_state:4, pos:[  10, 639] }
			]
		]

		thumb_pos: [
			[ 215+135*0,  94+103*0],[ 215+135*1,  94+103*0],[ 215+135*2,  94+103*0],[ 215+135*3,  94+103*0],[ 215+135*4,  94+103*0],[ 215+135*5,  94+103*0],
			[ 215+135*0,  94+103*1],[ 215+135*1,  94+103*1],[ 215+135*2,  94+103*1],[ 215+135*3,  94+103*1],[ 215+135*4,  94+103*1],[ 215+135*5,  94+103*1],
			[ 215+135*0,  94+103*2],[ 215+135*1,  94+103*2],[ 215+135*2,  94+103*2],[ 215+135*3,  94+103*2],[ 215+135*4,  94+103*2],[ 215+135*5,  94+103*2],
			[ 215+135*0,  94+103*3],[ 215+135*1,  94+103*3],[ 215+135*2,  94+103*3],[ 215+135*3,  94+103*3],[ 215+135*4,  94+103*3],[ 215+135*5,  94+103*3],
			[ 215+135*0,  94+103*4],[ 215+135*1,  94+103*4],[ 215+135*2,  94+103*4],[ 215+135*3,  94+103*4],[ 215+135*4,  94+103*4],[ 215+135*5,  94+103*4],
			[ 215+135*0,  94+103*5],[ 215+135*1,  94+103*5],[ 215+135*2,  94+103*5],[ 215+135*3,  94+103*5],[ 215+135*4,  94+103*5],[ 215+135*5,  94+103*5],
		]
	}

	setting_bu: {
		base: "buex_bubase"

		buttons: {
			order_up:      { name:"buex_order_up",        max_state:3, pos:[597, 644] }
			order_down:    { name:"buex_order_down",      max_state:3, pos:[970, 644] }
			on_off_button: { name:"dress_view_appearance",max_state:4, pos:[  2,  63] }
			pos_pointer:   { name:"buex_view_pointer",    max_state:2, pos:[509, 743] }
			switch:        { name:"buex_view_switch",     max_state:3 }
			hair_mode:     { name:"buex_view_hair",       max_state:5, pos:[ 33, 674] }
			cutin_mode:    { name:"buex_view_cutin",      max_state:5, pos:[ 33, 704] }
			steam_mode:    { name:"buex_view_steam",      max_state:5, pos:[ 33, 644] }
		}

		pos_region:   [   0, 743, 1019,  25]

		switchs: [
			{ pos:[290, 647], space:[17,  0], repeat_x:2  }
			{ pos:[195, 647], space:[17,  0], repeat_x:2  }
			{ pos:[224, 707], space:[17,  0], repeat_x:7  }
			{ pos:[195, 677], space:[17,  0], repeat_x:11 }
			{ pos:[429, 677], space:[17,  0], repeat_x:5  }
			{ pos:[372, 647], space:[17,  0], repeat_x:13 }
		]

		pose_signature: "ab"
		view_signature: "fu"
		ngng_convert_table: [0, 1, 2, 3, 4, 5, 6]

		characters: [
			{ icon:{ name:"dress_view_ch1", max_state:3, pos:[620+70*0,  644] }, info:{ wear:[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11], under_wear:5, face:[ [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12],     [13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24] ] } }
			{ icon:{ name:"dress_view_ch2", max_state:3, pos:[620+70*1,  644] }, info:{ wear:[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11], under_wear:5, face:[ [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13], [14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24] ] } }
			{ icon:{ name:"dress_view_ch3", max_state:3, pos:[620+70*2,  644] }, info:{ wear:[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11], under_wear:5, face:[ [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13], [14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24] ] } }
			{ icon:{ name:"dress_view_ch4", max_state:3, pos:[620+70*3,  644] }, info:{ wear:[1, 2, 3, 4, 5, 6, 7, 8, 9, 10],     under_wear:5, face:[ [1, 2, 3, 4,    6, 7, 8, 9, 10, 11, 12],     [13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24] ] } }
			{ icon:{ name:"dress_view_ch5", max_state:3, pos:[620+70*4,  644] }, info:{ wear:[1, 2, 3, 4, 5, 6, 7],               under_wear:5, face:[ [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],             [11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22] ] } }
		]
	}

	edit_message: {
		base: "buex_embase"

		sub_menu: [
			{ name:"buex_sub_1", max_state:4, pos:[784,  35] }
			{ name:"buex_sub_2", max_state:4, pos:[784,  62] }
		]

		edit_command: {
			fontsize:    { name:"buex_em_fontsize",    max_state:3 }
			fontcolor:   { name:"buex_em_fontcolor",   max_state:3 }
			typesetting: { name:"buex_em_typesetting", max_state:3 }
			balloon:     { name:"buex_em_balloon",     max_state:3 }
			delete:      { name:"buex_em_delete",      max_state:3 }
		}

		color_picker: {
			base: "color_picker_base"
			hit:  "color_picker_hit"

			radio_button: "color_picker_radio"
			target: [ [ 200,  50], [345, 50] ]

			close: { name:"color_picker_close", max_state:2, pos:[461,   5] }

			color_bar: [
				{ bar:{ name:"cfgx_red_bar",           pos:[209,  81] }, min:{ name:"color_picker_min", max_state:2, pos:[133,  80] }, max:{ name:"color_picker_max", max_state:2, pos:[415,  80] } }
				{ bar:{ name:"cfgx_green_bar",         pos:[209, 111] }, min:{ name:"color_picker_min", max_state:2, pos:[133, 110] }, max:{ name:"color_picker_max", max_state:2, pos:[415, 110] } }
				{ bar:{ name:"cfgx_blue_bar",          pos:[209, 141] }, min:{ name:"color_picker_min", max_state:2, pos:[133, 140] }, max:{ name:"color_picker_max", max_state:2, pos:[415, 140] } }
				{ bar:{ name:"cfgx_concentration_bar", pos:[209, 171] }, min:{ name:"color_picker_min", max_state:2, pos:[133, 170] }, max:{ name:"color_picker_max", max_state:2, pos:[415, 170] } }
			]
		}
	}

	data_store: {
		base: "buex_dsbase"

		komaban: "komaban"

		edit_command: {
			save:   { name:"buex_ds_cmd_save",   max_state:3 }
			load:   { name:"buex_ds_cmd_load",   max_state:3 }
			toslot: { name:"buex_ds_cmd_toslot", max_state:3 }
			output: { name:"buex_ds_output", max_state:3, pos:[  42, 504] }
			clear:  { name:"buex_ds_clear",  max_state:3, pos:[  42, 554] }
		}

		thumb_pos: [
			[ 215+135*0,  94+103*0],[ 215+135*1,  94+103*0],[ 215+135*2,  94+103*0],[ 215+135*3,  94+103*0],[ 215+135*4,  94+103*0],[ 215+135*5,  94+103*0],
			[ 215+135*0,  94+103*1],[ 215+135*1,  94+103*1],[ 215+135*2,  94+103*1],[ 215+135*3,  94+103*1],[ 215+135*4,  94+103*1],[ 215+135*5,  94+103*1],
			[ 215+135*0,  94+103*2],[ 215+135*1,  94+103*2],[ 215+135*2,  94+103*2],[ 215+135*3,  94+103*2],[ 215+135*4,  94+103*2],[ 215+135*5,  94+103*2],
			[ 215+135*0,  94+103*3],[ 215+135*1,  94+103*3],[ 215+135*2,  94+103*3],[ 215+135*3,  94+103*3],[ 215+135*4,  94+103*3],[ 215+135*5,  94+103*3],
			[ 215+135*0,  94+103*4],[ 215+135*1,  94+103*4],[ 215+135*2,  94+103*4],[ 215+135*3,  94+103*4],[ 215+135*4,  94+103*4],[ 215+135*5,  94+103*4],
			[ 215+135*0,  94+103*5],[ 215+135*1,  94+103*5],[ 215+135*2,  94+103*5],[ 215+135*3,  94+103*5],[ 215+135*4,  94+103*5],[ 215+135*5,  94+103*5],
		]

		slot_pos: [
			[  43,  94+103*0 ], [  43,  94+103*1 ], [  43,  94+103*2 ], [  43,  94+103*3 ]
		]
	}
}`

func TestLoadBuexLayoutSample(t *testing.T) {
	layout, err := LoadBuexLayout([]byte(buexLayoutSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// 顶部主菜单必须按固定次序出现，键名也要原样保留。
	wantMenu := []string{
		"setting_bgv", "bg_select", "setting_bu",
		"edit_message", "data_store", "back_to_thumbs",
	}
	if len(layout.Menu) != len(wantMenu) {
		t.Fatalf("%d menu entries, want %d", len(layout.Menu), len(wantMenu))
	}
	for index, key := range wantMenu {
		if layout.Menu[index].Key != key {
			t.Fatalf("menu %d key = %q, want %q", index, layout.Menu[index].Key, key)
		}
	}
	first := layout.Menu[0].Button
	if first.Name != "buex_main_bgv" || first.MaxState != 4 || first.X != 424 || first.Y != 5 {
		t.Fatalf("first menu button = %+v", first)
	}
	back := layout.Menu[len(layout.Menu)-1].Button
	if back.Name != "buex_main_back" || back.MaxState != 3 || back.X != 904 || back.Y != 5 {
		t.Fatalf("back button = %+v", back)
	}

	if len(layout.Notices) != 6 {
		t.Fatalf("%d notices, want 6", len(layout.Notices))
	}
	if layout.Notices["bgv"] != "buex_notice_bgv" {
		t.Fatalf("notice bgv = %q", layout.Notices["bgv"])
	}
	if layout.Notices["data"] != "buex_notice_data" {
		t.Fatalf("notice data = %q", layout.Notices["data"])
	}

	if layout.Loading.Name != "loading" || layout.Loading.MaxState != 8 {
		t.Fatalf("loading = %+v", layout.Loading)
	}

	// 背景选择页。
	if layout.Bg.Base != "buex_bgbase" {
		t.Fatalf("bg base = %q", layout.Bg.Base)
	}
	if layout.Bg.Subject != (UIPoint{X: 2, Y: -15}) {
		t.Fatalf("bg subject = %+v", layout.Bg.Subject)
	}
	numberFont := layout.Bg.NumberFont
	if numberFont.Name != "museum_cg_parts" || numberFont.MaxState != 11 ||
		numberFont.X != 114 || numberFont.Y != 69 {
		t.Fatalf("number font = %+v", numberFont)
	}
	if len(layout.Bg.PageGroups) != 2 {
		t.Fatalf("%d page groups, want 2", len(layout.Bg.PageGroups))
	}
	if len(layout.Bg.PageGroups[0]) != 2 {
		t.Fatalf("page group 0 has %d buttons, want 2", len(layout.Bg.PageGroups[0]))
	}
	if len(layout.Bg.PageGroups[1]) != 6 {
		t.Fatalf("page group 1 has %d buttons, want 6", len(layout.Bg.PageGroups[1]))
	}
	if layout.Bg.PageGroups[0][0].Name != "museum_page41" {
		t.Fatalf("page group 0 first = %+v", layout.Bg.PageGroups[0][0])
	}
	if layout.Bg.PageGroups[1][5].Name != "museum_page48" {
		t.Fatalf("page group 1 last = %+v", layout.Bg.PageGroups[1][5])
	}
	if len(layout.Bg.ThumbPos) != 36 {
		t.Fatalf("%d thumb positions, want 36", len(layout.Bg.ThumbPos))
	}
	if layout.Bg.ThumbPos[0] != (UIPoint{X: 215, Y: 94}) {
		t.Fatalf("first thumb = %+v", layout.Bg.ThumbPos[0])
	}
	// 算式必须真的被求值，而不是当成字符串。
	last := layout.Bg.ThumbPos[35]
	if last.X != 215+135*5 || last.Y != 94+103*5 {
		t.Fatalf("last thumb = %+v, want %d,%d", last, 215+135*5, 94+103*5)
	}

	// 立ち絵页。
	if layout.Setting.Base != "buex_bubase" {
		t.Fatalf("setting base = %q", layout.Setting.Base)
	}
	if len(layout.Setting.Buttons) != 8 {
		t.Fatalf("%d setting buttons, want 8", len(layout.Setting.Buttons))
	}
	orderUp := layout.Setting.Buttons["order_up"]
	if orderUp.Name != "buex_order_up" || orderUp.MaxState != 3 || orderUp.X != 597 || orderUp.Y != 644 {
		t.Fatalf("order_up = %+v", orderUp)
	}
	// switch 没有写 pos，坐标应当是 0 而不是解析失败。
	switchButton := layout.Setting.Buttons["switch"]
	if switchButton.Name != "buex_view_switch" || switchButton.X != 0 || switchButton.Y != 0 {
		t.Fatalf("switch = %+v", switchButton)
	}
	if layout.Setting.Buttons["hair_mode"].Name != "buex_view_hair" {
		t.Fatalf("hair_mode = %+v", layout.Setting.Buttons["hair_mode"])
	}
	if layout.Setting.PosRegion != [4]int{0, 743, 1019, 25} {
		t.Fatalf("pos region = %v", layout.Setting.PosRegion)
	}

	// repeat_x 是每行列数；总颗数由语义（服装表/脱衣表/表情表）决定。
	wantColumns := []int{2, 2, 7, 11, 5, 13}
	wantSwitchPos := [][2]int{
		{290, 647}, {195, 647}, {224, 707},
		{195, 677}, {429, 677}, {372, 647},
	}
	if len(layout.Setting.Switches) != len(wantColumns) {
		t.Fatalf("%d switch groups, want %d", len(layout.Setting.Switches), len(wantColumns))
	}
	for index, group := range layout.Setting.Switches {
		if group.Columns != wantColumns[index] {
			t.Fatalf("switch %d columns = %d, want %d", index, group.Columns, wantColumns[index])
		}
		if group.Count != 0 {
			t.Fatalf("switch %d count = %d, want 0 (derived from semantics)", index, group.Count)
		}
		if group.X != wantSwitchPos[index][0] || group.Y != wantSwitchPos[index][1] {
			t.Fatalf("switch %d pos = %d,%d, want %d,%d", index,
				group.X, group.Y, wantSwitchPos[index][0], wantSwitchPos[index][1])
		}
		if group.SpaceX != 17 || group.SpaceY != 0 {
			t.Fatalf("switch %d space = %d,%d, want 17,0", index, group.SpaceX, group.SpaceY)
		}
	}

	if layout.Setting.PoseSignature != "ab" || layout.Setting.ViewSignature != "fu" {
		t.Fatalf("signatures = %q/%q", layout.Setting.PoseSignature, layout.Setting.ViewSignature)
	}
	if len(layout.Setting.StripConvert) != 7 {
		t.Fatalf("%d strip convert entries, want 7", len(layout.Setting.StripConvert))
	}
	for index, value := range layout.Setting.StripConvert {
		if value != index {
			t.Fatalf("strip convert %d = %d, want %d", index, value, index)
		}
	}

	if len(layout.Setting.Characters) != 5 {
		t.Fatalf("%d characters, want 5", len(layout.Setting.Characters))
	}
	character := layout.Setting.Characters[0]
	if character.Icon.Name != "dress_view_ch1" || character.Icon.X != 620 || character.Icon.Y != 644 {
		t.Fatalf("character 1 icon = %+v", character.Icon)
	}
	if len(character.Wear) != 11 {
		t.Fatalf("character 1 has %d wear entries, want 11", len(character.Wear))
	}
	if character.UnderWear != 5 {
		t.Fatalf("character 1 under wear = %d, want 5", character.UnderWear)
	}
	if len(character.Faces) != 2 {
		t.Fatalf("character 1 has %d face rows, want 2", len(character.Faces))
	}
	for index, row := range character.Faces {
		if len(row) != 12 {
			t.Fatalf("character 1 face row %d has %d entries, want 12", index, len(row))
		}
	}
	if character.Faces[0][0] != 1 || character.Faces[1][0] != 13 {
		t.Fatalf("character 1 faces = %v", character.Faces)
	}
	lastCharacter := layout.Setting.Characters[4]
	if lastCharacter.Icon.Name != "dress_view_ch5" || lastCharacter.Icon.X != 620+70*4 {
		t.Fatalf("character 5 icon = %+v", lastCharacter.Icon)
	}
	if len(lastCharacter.Wear) != 7 {
		t.Fatalf("character 5 has %d wear entries, want 7", len(lastCharacter.Wear))
	}

	// 消息编辑页。
	if layout.Edit.Base != "buex_embase" {
		t.Fatalf("edit base = %q", layout.Edit.Base)
	}
	if len(layout.Edit.SubMenu) != 2 {
		t.Fatalf("%d sub menu buttons, want 2", len(layout.Edit.SubMenu))
	}
	if layout.Edit.SubMenu[0].Name != "buex_sub_1" || layout.Edit.SubMenu[0].X != 784 {
		t.Fatalf("sub menu 0 = %+v", layout.Edit.SubMenu[0])
	}
	if len(layout.Edit.Commands) != 5 {
		t.Fatalf("%d edit commands, want 5", len(layout.Edit.Commands))
	}
	if layout.Edit.Commands["balloon"].Name != "buex_em_balloon" {
		t.Fatalf("balloon command = %+v", layout.Edit.Commands["balloon"])
	}

	picker := layout.Edit.Picker
	if picker.Base != "color_picker_base" || picker.Hit != "color_picker_hit" ||
		picker.Radio != "color_picker_radio" {
		t.Fatalf("picker = %+v", picker)
	}
	if len(picker.Targets) != 2 {
		t.Fatalf("%d picker targets, want 2", len(picker.Targets))
	}
	if picker.Targets[0] != (UIPoint{X: 200, Y: 50}) ||
		picker.Targets[1] != (UIPoint{X: 345, Y: 50}) {
		t.Fatalf("picker targets = %+v", picker.Targets)
	}
	if picker.Close.Name != "color_picker_close" || picker.Close.MaxState != 2 ||
		picker.Close.X != 461 || picker.Close.Y != 5 {
		t.Fatalf("picker close = %+v", picker.Close)
	}
	if len(picker.Bars) != 4 {
		t.Fatalf("%d color bars, want 4", len(picker.Bars))
	}
	if picker.Bars[0].Bar.Name != "cfgx_red_bar" ||
		picker.Bars[0].Bar.X != 209 || picker.Bars[0].Bar.Y != 81 {
		t.Fatalf("first color bar = %+v", picker.Bars[0].Bar)
	}
	if picker.Bars[3].Bar.Name != "cfgx_concentration_bar" {
		t.Fatalf("last color bar = %+v", picker.Bars[3].Bar)
	}
	if picker.Bars[0].Min.Name != "color_picker_min" || picker.Bars[0].Min.X != 133 {
		t.Fatalf("first color bar min = %+v", picker.Bars[0].Min)
	}
	if picker.Bars[0].Max.Name != "color_picker_max" || picker.Bars[0].Max.X != 415 {
		t.Fatalf("first color bar max = %+v", picker.Bars[0].Max)
	}

	// 存档页。
	if layout.Store.Base != "buex_dsbase" {
		t.Fatalf("store base = %q", layout.Store.Base)
	}
	if layout.Store.Komaban != "komaban" {
		t.Fatalf("store komaban = %q", layout.Store.Komaban)
	}
	if len(layout.Store.Commands) != 5 {
		t.Fatalf("%d store commands, want 5", len(layout.Store.Commands))
	}
	if layout.Store.Commands["output"].Name != "buex_ds_output" {
		t.Fatalf("output command = %+v", layout.Store.Commands["output"])
	}
	if layout.Store.Commands["clear"].Name != "buex_ds_clear" {
		t.Fatalf("clear command = %+v", layout.Store.Commands["clear"])
	}
	if len(layout.Store.ThumbPos) != 36 {
		t.Fatalf("%d store thumb positions, want 36", len(layout.Store.ThumbPos))
	}
	if len(layout.Store.SlotPos) != 4 {
		t.Fatalf("%d slot positions, want 4", len(layout.Store.SlotPos))
	}
	if layout.Store.SlotPos[0] != (UIPoint{X: 43, Y: 94}) {
		t.Fatalf("first slot = %+v", layout.Store.SlotPos[0])
	}
}

func TestLoadBuexLayoutWithoutSections(t *testing.T) {
	// 缺少大部分段落的文档不能报错，缺的段落到零值。
	layout, err := LoadBuexLayout([]byte(`{
		main_menu: {
			bg_select: { name:"buex_main_changebg", max_state:4, pos:[304, 5] }
		}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(layout.Menu) != 1 || layout.Menu[0].Key != "bg_select" {
		t.Fatalf("menu = %+v", layout.Menu)
	}
	if layout.Notices != nil {
		t.Fatalf("notices = %+v, want nil", layout.Notices)
	}
	if layout.Loading.Name != "" {
		t.Fatalf("loading = %+v", layout.Loading)
	}
	if layout.Bg.Base != "" || layout.Bg.Subject != (UIPoint{}) {
		t.Fatalf("bg = %+v", layout.Bg)
	}
	if layout.Setting.Base != "" || layout.Setting.Buttons != nil ||
		layout.Setting.Switches != nil || layout.Setting.Characters != nil {
		t.Fatalf("setting = %+v", layout.Setting)
	}
	if layout.Edit.Base != "" || layout.Edit.Commands != nil || layout.Edit.Picker.Bars != nil {
		t.Fatalf("edit = %+v", layout.Edit)
	}
	if layout.Store.Base != "" || layout.Store.Komaban != "" || layout.Store.Commands != nil {
		t.Fatalf("store = %+v", layout.Store)
	}

	empty, err := LoadBuexLayout([]byte("{}"))
	if err != nil {
		t.Fatalf("parse empty document: %v", err)
	}
	if len(empty.Menu) != 0 || len(empty.Bg.ThumbPos) != 0 {
		t.Fatalf("empty document = %+v", empty)
	}
}

func TestLoadBuexLayoutKeepsEveryCommand(t *testing.T) {
	// 原文件的存档命令里还有一项 "base"；解析器不按名字过滤，全部留下。
	layout, err := LoadBuexLayout([]byte(`{
		data_store: {
			base: "buex_dsbase"
			komaban: "komaban"
			edit_command: {
				base:   { name:"buex_ds_cmd_base" }
				save:   { name:"buex_ds_cmd_save",   max_state:3 }
				load:   { name:"buex_ds_cmd_load",   max_state:3 }
				toslot: { name:"buex_ds_cmd_toslot", max_state:3 }
				output: { name:"buex_ds_output", max_state:3, pos:[ 42, 504] }
				clear:  { name:"buex_ds_clear",  max_state:3, pos:[ 42, 554] }
			}
		}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(layout.Store.Commands) != 6 {
		t.Fatalf("%d store commands, want 6", len(layout.Store.Commands))
	}
	if layout.Store.Commands["base"].Name != "buex_ds_cmd_base" {
		t.Fatalf("base command = %+v", layout.Store.Commands["base"])
	}
}

func TestParseBuexSwitchGroupsColumns(t *testing.T) {
	root, err := ParseLayoutDocument([]byte(`{
		switchs: [
			{ pos:[4, 100], space:[32, 32], repeat_x:2 }
			{ pos:[4, 164], space:[32, 32], repeat_x:5 }
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	groups := parseBuexSwitchGroups(root["switchs"])
	if len(groups) != 2 {
		t.Fatalf("%d groups, want 2", len(groups))
	}
	// repeat_x 只是"每行列数"；总颗数由调用方按语义决定。
	if groups[0].Columns != 2 || groups[0].Count != 0 {
		t.Fatalf("group 0 = %+v", groups[0])
	}
	if groups[1].Columns != 5 {
		t.Fatalf("group 1 columns = %d, want 5", groups[1].Columns)
	}
}
