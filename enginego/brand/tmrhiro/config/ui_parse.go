// 本文件把 _system 里的界面段解析成 config/ui.go 里的结构体。
//
// 为什么单独一个文件：这些段的字段顺序（尤其 SAVE_BASE_POS 那种十几位连写的
// 布局行）只有对照原引擎的读法才有意义，散在 session 里会把"第 7 个字段是行距"
// 这类知识重复很多遍。解析集中在这里后，session 只按语义使用。
package config

import (
	"strconv"
	"strings"
)

// UILayout 是一整套界面布局（从 _system 解析而来）。
type UILayout struct {
	Message  MessageWindow
	Toolbar  Toolbar
	Menu     MenuBar
	Save     SlotBoard
	Load     SlotBoard
	System   SystemScreen
	View     ViewScreen
	Album    AlbumScreen
	Select   SelectScreen
	Chara    CharaView
	ScreenW  int
	ScreenH  int
	CharaL   int
	CharaR   int
	Defaults map[string]int
	// ReadColor 是已读文本颜色（TEXT_READ_COLOR），未读用 TextColor。
	ReadColor uint32
}

// ParseUI 从 _system 解析界面布局。缺段时返回带零值的结构，调用方按 Empty() 判断。
func ParseUI(ini *Ini) *UILayout {
	if ini == nil {
		return &UILayout{}
	}
	ui := &UILayout{Defaults: make(map[string]int)}
	ui.ScreenW = ini.Int("GAMESYSTEM", "DEF_WINDOW_WIDTH", 1280)
	ui.ScreenH = ini.Int("GAMESYSTEM", "DEF_WINDOW_HEIGHT", 720)
	ui.CharaL = ini.Int("GAMESYSTEM", "CHARA_SPR_LEFT", -300)
	ui.CharaR = ini.Int("GAMESYSTEM", "CHARA_SPR_RIGHT", 300)
	ui.Message = parseMessageWindow(ini)
	ui.Toolbar = parseToolbar(ini)
	ui.Menu = parseMenuBar(ini)
	ui.Save = parseSlotBoard(ini, "SAVE", 10, 10)
	ui.Load = parseSlotBoard(ini, "LOAD", 10, 10)
	ui.System = parseSystemScreen(ini)
	ui.View = parseViewScreen(ini)
	ui.Album = parseAlbumScreen(ini)
	ui.Select = parseSelectScreen(ini)
	// [CHARAVIEW]/[BG]/[CHARAxx] 不在 _system 里，而在独立条目 _charaview。
	// 这里留空，由 ParseCharaView 在 session 读到那份文本后填充。
	ui.Chara = CharaView{}
	ui.ReadColor = ParseColor(ini.Get("MESSAGEWINDOW", "TEXT_READ_COLOR"))
	for _, key := range ini.Keys("SYSTEM") {
		if !strings.HasPrefix(key, "SYS_DEF_") {
			continue
		}
		if values := ini.Ints("SYSTEM", key); len(values) > 0 {
			ui.Defaults[key] = values[0]
		}
	}
	return ui
}

// parseMessageWindow 解析 [MESSAGEWINDOW]。
func parseMessageWindow(ini *Ini) MessageWindow {
	section := "MESSAGEWINDOW"
	m := MessageWindow{
		X:             ini.Int(section, "MW_DRAW_POS_X", 0),
		Y:             ini.Int(section, "MW_DRAW_POS_Y", 463),
		Width:         ini.Int(section, "MW_DRAW_WIDTH", 1280),
		Height:        ini.Int(section, "MW_DRAW_HEIGHT", 257),
		FrameDraw:     ini.Int(section, "MW_FRAM_DRAW", 0) != 0,
		GraphicCount:  ini.Int(section, "MW_GRAPHIC_COUNT", 1),
		FontName:      ini.Get(section, "TEXT_FONT_NAME"),
		FontSize:      ini.Int(section, "TEXT_FONT", 26),
		FontWeight:    ini.Int(section, "TEXT_FONT_WEIGHT", 0),
		LineMax:       ini.Int(section, "TEXT_LINE_MAX", 2),
		TextMax:       ini.Int(section, "TEXT_MAX", 68),
		FrontH:        ini.Int(section, "TEXT_FRONT_H", 0),
		BackH:         ini.Int(section, "TEXT_BACK_H", 0),
		BracketsH:     ini.Int(section, "TEXT_BRACKETS_H", 0),
		TextX:         ini.Int(section, "TEXT_DRAW_POS_X", 90),
		TextY:         ini.Int(section, "TEXT_DRAW_POS_Y", 135),
		TextNext:      ini.Int(section, "TEXT_DRAW_NEXT", 32),
		NameDraw:      ini.Int(section, "NP_DRAW_FLAG", 0) != 0,
		NameX:         ini.Int(section, "NP_DRAW_POS_X", 90),
		NameY:         ini.Int(section, "NP_DRAW_POS_Y", 110),
		NameWidth:     ini.Int(section, "NP_DRAW_WIDTH", 320),
		NameHeight:    ini.Int(section, "NP_DRAW_HEIGHT", 36),
		NamePlusX:     ini.Int(section, "NAME_DRAW_PLUS_X", 0),
		NamePlusY:     ini.Int(section, "NAME_DRAW_PLUS_Y", 0),
		WaitX:         ini.Int(section, "CW_DRAW_POS_X", 1103),
		WaitY:         ini.Int(section, "CW_DRAW_POS_Y", 177),
		WaitW:         ini.Int(section, "CW_DRAW_WIDTH", 36),
		WaitH:         ini.Int(section, "CW_DRAW_HEIGHT", 36),
		WaitTimeMs:    ini.Int(section, "CW_DRAW_TIME", 50),
		WaitFrames:    ini.Int(section, "CW_DRAW_PAT", 1),
		TextDrawShade: ini.Int("SYSTEM", "SYS_DEF_TEXT_DRAW_SHADOW", 0),
	}
	if ini.Has("MESSAGE", "MES_POS_X") {
		// MES_POS_X/Y 与 MES_WIDTH/HEIGHT 是单值（不是逗号列表），
		// 这里直接放在 MessageWindow 的确认框几何字段里。
		m.ConfirmX = ini.Int("MESSAGE", "MES_POS_X", 450)
		m.ConfirmY = ini.Int("MESSAGE", "MES_POS_Y", 267)
		m.ConfirmW = ini.Int("MESSAGE", "MES_WIDTH", 380)
		m.ConfirmH = ini.Int("MESSAGE", "MES_HEIGHT", 166)
	}
	// [MESSAGE] 是原版确认框（"ゲームを終了します。" + YES/NO）的几何：
	// MES_POS_X/Y 是对话框左上角，MES_WIDTH/HEIGHT 是底图尺寸，
	// MES_B_* 是 YES/NO 按钮的起点与尺寸、MES_B_NEXT_X 是两按钮之间的步进。
	// 以前只在运行里读这些键，确认框只能凭猜给一个 392x200 的盒子，
	// 按钮位置与底图对不上（"ゲーム終了"的弹窗按钮文字错乱就是这么来的）。
	m.ButtonX = ini.Int("MESSAGE", "MES_B_POS_X", 69)
	m.ButtonY = ini.Int("MESSAGE", "MES_B_POS_Y", 99)
	m.ButtonW = ini.Int("MESSAGE", "MES_B_WIDTH", 108)
	m.ButtonH = ini.Int("MESSAGE", "MES_B_HEIGHT", 30)
	m.ButtonNextX = ini.Int("MESSAGE", "MES_B_NEXT_X", 134)

	m.TextColor = ParseColor(ini.Get(section, "TEXT_COLOR"))
	m.ReadColor = ParseColor(ini.Get(section, "TEXT_READ_COLOR"))
	m.VulgarColor = ParseColor(ini.Get(section, "VULGAR_WORD_COLOR"))
	return m
}

// parseToolbar 解析 [MESSAGEWINDOW] 的 TWB_* 组。
//
// 字段顺序：启用, 种类, X, Y, 源X, 源Y, 宽, 高。
// 种类 0 的按钮属于消息窗底部工具条，Y 相对消息窗左上角；种类 1 的按钮属于
// 暂停菜单，Y 是屏幕绝对坐标（手册截图里菜单位于屏幕顶端）。
func parseToolbar(ini *Ini) Toolbar {
	t := Toolbar{
		Width:  ini.Int("MESSAGEWINDOW", "TWM_WIDTH_01", 1280),
		Height: ini.Int("MESSAGEWINDOW", "TWM_HEIGHT_01", 48),
	}
	for _, key := range ini.Keys("MESSAGEWINDOW") {
		if !strings.HasPrefix(key, "TWB_") {
			continue
		}
		button, ok := parseToolbarButton(key, ini.Get("MESSAGEWINDOW", key), false)
		if !ok || button.Kind != 0 {
			continue
		}
		t.Buttons = append(t.Buttons, button)
	}
	return t
}

// parseMenuBar 解析暂停菜单：优先用 TWB_* 里种类 1 的项，其次 [MENU]/[MAPMENU]。
func parseMenuBar(ini *Ini) MenuBar {
	bar := MenuBar{
		Kind:   1,
		Width:  ini.Int("MENU", "TWB_WIDTH", 1280),
		Height: ini.Int("MENU", "TWB_HEIGHT", 48),
	}
	for _, key := range ini.Keys("MESSAGEWINDOW") {
		if !strings.HasPrefix(key, "TWB_") {
			continue
		}
		button, ok := parseToolbarButton(key, ini.Get("MESSAGEWINDOW", key), false)
		if !ok || button.Kind != 1 {
			continue
		}
		bar.Buttons = append(bar.Buttons, button)
	}
	return bar
}

// parseToolbarButton 解析一行按钮字段。
func parseToolbarButton(key, value string, yIsRelative bool) (ToolbarButton, bool) {
	fields := splitFields(value)
	if len(fields) < 8 {
		return ToolbarButton{}, false
	}
	button := ToolbarButton{
		Key:       strings.TrimPrefix(key, "TWB_"),
		Enabled:   atoiField(fields[0]) != 0,
		Kind:      atoiField(fields[1]),
		X:         atoiField(fields[2]),
		Y:         atoiField(fields[3]),
		SrcX:      atoiField(fields[4]),
		SrcY:      atoiField(fields[5]),
		W:         atoiField(fields[6]),
		H:         atoiField(fields[7]),
		Image:     "sys_window_button",
		FrameW:    atoiField(fields[6]),
		FrameH:    atoiField(fields[7]),
		FramesCnt: 1,
	}
	_ = yIsRelative
	return button, true
}

// parseSlotBoard 解析 [SAVE] / [LOAD]。
//
// SAVE_BASE_POS 的字段顺序（原引擎逐字段读，顺序不是常识排列）：
//
//	0 x, 1 y, 2/3 保留, 4 格宽, 5 格高, 6 槽数, 7 行数, 8 列数, 9 保留,
//	10 行步进, 11..12 保留, 13 列步进, 14..16 保留
//
// 格数、行数、列数三者都写出来是原版的冗余：启动时按格数 × 行列铺板。
func parseSlotBoard(ini *Ini, section string, pageMax, slotMax int) SlotBoard {
	board := SlotBoard{
		PageMax:  ini.Int(section, section+"_PAGE_MAX", pageMax),
		SlotMax:  ini.Int(section, section+"_COUNT_MAX", slotMax),
		SaveType: ini.Int(section, section+"_TYPE", 0),
	}
	base := splitFields(ini.Get(section, section+"_BASE_POS"))
	if len(base) >= 14 {
		board.BaseX = atoiField(base[0])
		board.BaseY = atoiField(base[1])
		board.TileW = atoiField(base[4])
		board.TileH = atoiField(base[5])
		if count := atoiField(base[6]); count > 0 {
			board.SlotMax = count
		}
		board.Rows = atoiField(base[7])
		board.Cols = atoiField(base[8])
		board.GapY = atoiField(base[10])
		board.GapX = atoiField(base[13])
	}
	if board.Cols <= 0 {
		board.Cols = 2
	}
	if board.Rows <= 0 {
		board.Rows = (board.SlotMax + board.Cols - 1) / board.Cols
	}
	text := splitFields(ini.Get(section, section+"_B_TEXT_POS"))
	if len(text) >= 3 {
		board.TextX = atoiField(text[0])
		board.TextY = atoiField(text[1])
		board.TextNext = atoiField(text[2])
	}
	thumb := splitFields(ini.Get(section, section+"_B_THUM_POS"))
	if len(thumb) >= 2 {
		board.ThumbX = atoiField(thumb[0])
		board.ThumbY = atoiField(thumb[1])
	}
	thumbSize := splitFields(ini.Get(section, section+"_THUM_SIZE"))
	if len(thumbSize) >= 2 {
		board.ThumbW = atoiField(thumbSize[0])
		board.ThumbH = atoiField(thumbSize[1])
	}
	page := splitFields(ini.Get(section, section+"_PAGE_POS"))
	if len(page) >= 6 {
		board.PageX = atoiField(page[0])
		board.PageY = atoiField(page[1])
		board.PageSrcX = atoiField(page[2])
		board.PageSrcY = atoiField(page[3])
		board.PageW = atoiField(page[4])
		board.PageH = atoiField(page[5])
	}
	if len(page) >= 10 {
		board.PageStepX = atoiField(page[9])
	}
	if board.PageStepX <= 0 {
		board.PageStepX = board.PageW + 6
	}
	ret := splitFields(ini.Get(section, section+"_RETURN_POS"))
	if len(ret) >= 6 {
		board.ReturnX = atoiField(ret[0])
		board.ReturnY = atoiField(ret[1])
		board.ReturnSrcX = atoiField(ret[2])
		board.ReturnSrcY = atoiField(ret[3])
		board.ReturnW = atoiField(ret[4])
		board.ReturnH = atoiField(ret[5])
	}
	auto := splitFields(ini.Get(section, section+"_AUTO_POS"))
	if len(auto) >= 6 {
		board.AutoX = atoiField(auto[0])
		board.AutoY = atoiField(auto[1])
		board.AutoSrcX = atoiField(auto[2])
		board.AutoSrcY = atoiField(auto[3])
		board.AutoW = atoiField(auto[4])
		board.AutoH = atoiField(auto[5])
	}
	del := splitFields(ini.Get(section, section+"_DEL_POS"))
	if len(del) >= 6 {
		board.DelX = atoiField(del[0])
		board.DelY = atoiField(del[1])
		board.DelSrcX = atoiField(del[2])
		board.DelSrcY = atoiField(del[3])
		board.DelW = atoiField(del[4])
		board.DelH = atoiField(del[5])
	}
	return board
}

// parseSystemScreen 解析 [SYSTEM] 的控件表与左侧导航。
func parseSystemScreen(ini *Ini) SystemScreen {
	screen := SystemScreen{PageMax: ini.Int("SYSTEM", "SYSTEM_PAGE_MAX", 4)}
	for _, key := range ini.Keys("SYSTEM") {
		if !strings.HasPrefix(key, "SYSTEM_PARTS_") {
			continue
		}
		fields := splitFields(ini.Get("SYSTEM", key))
		if len(fields) < 8 {
			continue
		}
		// 字段顺序见 config.SystemPart 的注释，两类控件的差异都在 9~18：
		//
		//	按钮组：9 总数, 10 每行数, 11 行数, 12 屏幕步进, 14 图集步进,
		//	        17 行距（多行时）
		//	滑杆：  9 条数, 13 条间行距（不透明度 33 / 音量 56）, 18 轨道长度
		//
		// 12/14 的符号表示排列方向（负 = 配置里的 X 是最右一格、向左边铺），
		// 不能取绝对值：ON|OFF 的 SrcX 指向 ON、图集步进 -160，屏幕步进 -168，
		// 第一格（ON）落在 X + (perRow-1)*step。取绝对值会把 ON/OFF 左右颠倒。
		part := SystemPart{
			Page: partPage(key),
			Key:  fields[1],
			Kind: fields[2],
			X:    atoiField(fields[3]),
			Y:    atoiField(fields[4]),
			SrcX: atoiField(fields[5]),
			SrcY: atoiField(fields[6]),
			W:    atoiField(fields[7]),
			H:    atoiField(fields[8]),
			Raw:  fields,
		}
		if len(fields) > 9 {
			part.Total = atoiField(fields[9])
		}
		if part.Total <= 0 {
			part.Total = 1
		}
		if len(fields) > 10 {
			part.PerRow = atoiField(fields[10])
		}
		if len(fields) > 11 {
			part.Rows = atoiField(fields[11])
		}
		if part.PerRow <= 0 || part.PerRow > part.Total {
			part.PerRow = part.Total
		}
		if part.Rows <= 0 {
			part.Rows = (part.Total + part.PerRow - 1) / part.PerRow
		}
		if len(fields) > 12 {
			part.ColStep = atoiField(fields[12])
		}
		if len(fields) > 14 {
			part.SrcStep = atoiField(fields[14])
		}
		if part.Kind == "SW_SLIDE" {
			// 滑杆的条间行距在字段 13（不透明度 4 条各 33 高；音量页的
			// BGM/ボイス/通常SE…每组两条，行距 56），轨道长度在字段 18。
			if len(fields) > 13 {
				if v := atoiField(fields[13]); v > 0 {
					part.RowStepY = v
				}
			}
			if len(fields) > 18 {
				part.SlideX = atoiField(fields[18])
			}
			if part.SlideX <= 0 {
				part.SlideX = part.W
			}
		} else if part.Rows > 1 && len(fields) > 17 {
			if v := atoiField(fields[17]); v > 0 {
				part.RowStepY = v
			}
		}
		if part.RowStepY <= 0 {
			part.RowStepY = part.H
		}
		// 屏幕步进为 0 表示"只占一格"（SW_ONE、默认按钮之类），
		// 按单元宽排不会错；图集步进缺省同值。
		if part.ColStep == 0 {
			part.ColStep = part.W
		}
		if part.SrcStep == 0 {
			part.SrcStep = part.ColStep
		}
		screen.Parts = append(screen.Parts, part)
		// 9901..9904 是左侧导航：页签、游戏终了、标题画面、回首页（返回）。
		switch part.Key {
		case "S_PAGE", "S_GAME", "S_TITLE", "S_END":
			screen.PageButtons = append(screen.PageButtons, Rect{X: part.X, Y: part.Y, W: part.W, H: part.H})
		}
	}
	// 页签纵向排列：S_PAGE 的 (5) 个按钮，每个 144x25，步进 30。
	if len(screen.PageButtons) > 0 {
		first := screen.PageButtons[0]
		screen.PageButtons = screen.PageButtons[:0]
		for i := 0; i < screen.PageMax; i++ {
			screen.PageButtons = append(screen.PageButtons, Rect{X: first.X, Y: first.Y + i*30, W: first.W, H: first.H})
		}
	}
	for _, part := range screen.Parts {
		switch part.Key {
		case "S_GAME":
			screen.GameButton = Rect{X: part.X, Y: part.Y, W: part.W, H: part.H}
		case "S_TITLE":
			screen.TitleButton = Rect{X: part.X, Y: part.Y, W: part.W, H: part.H}
		case "S_END":
			screen.EndButton = Rect{X: part.X, Y: part.Y, W: part.W, H: part.H}
		}
	}
	return screen
}

// parseViewScreen 解析 [VIEW]。
func parseViewScreen(ini *Ini) ViewScreen {
	v := ViewScreen{}
	percent := splitFields(ini.Get("VIEW", "VIEW_CG_PERCENT"))
	if len(percent) >= 3 {
		v.PercentX = atoiField(percent[1])
		v.PercentY = atoiField(percent[2])
	}
	// VIEW_NUMBER_SIZE=20,26：达成率数字的显示尺寸（sys_number 的单个数字）。
	if size := splitFields(ini.Get("VIEW", "VIEW_NUMBER_SIZE")); len(size) >= 2 {
		v.NumberW = atoiField(size[0])
		v.NumberH = atoiField(size[1])
	}
	graphic := splitFields(ini.Get("VIEW", "VIEW_GRAPHIC_COUNT"))
	if len(graphic) >= 5 {
		v.Rows = atoiField(graphic[1])
		v.Cols = atoiField(graphic[2])
		v.CellGapX = atoiField(graphic[3])
	}
	return v
}

// parseAlbumScreen 解析 [ALBUM]。
func parseAlbumScreen(ini *Ini) AlbumScreen {
	album := AlbumScreen{}
	pos := splitFields(ini.Get("ALBUM", "POSITION"))
	if len(pos) >= 3 {
		album.PosX = atoiField(pos[0])
		album.PosY = atoiField(pos[1])
		if scale, err := parseFloatField(pos[2]); err == nil {
			album.Scale = scale
		}
	}
	if len(pos) >= 4 {
		album.PageMax = atoiField(pos[3])
	}
	next := splitFields(ini.Get("ALBUM", "NEXT"))
	if len(next) >= 6 {
		album.Next = Rect{X: atoiField(next[0]), Y: atoiField(next[1]), W: atoiField(next[4]), H: atoiField(next[5])}
	}
	prev := splitFields(ini.Get("ALBUM", "PREV"))
	if len(prev) >= 6 {
		album.Prev = Rect{X: atoiField(prev[0]), Y: atoiField(prev[1]), W: atoiField(prev[4]), H: atoiField(prev[5])}
	}
	ret := splitFields(ini.Get("ALBUM", "RETURN"))
	if len(ret) >= 5 {
		album.Return = Rect{X: atoiField(ret[0]), Y: atoiField(ret[1]), W: atoiField(ret[3]), H: atoiField(ret[4])}
	}
	return album
}

// parseSelectScreen 解析 [SELECT]。
func parseSelectScreen(ini *Ini) SelectScreen {
	return SelectScreen{
		Width:     ini.Int("SELECT", "SELECT_WIDTH", 928),
		Height:    ini.Int("SELECT", "SELECT_HEIGHT", 67),
		PosX:      0,
		PosY:      ini.Int("SELECT", "SELECT_POSY", 280),
		NextX:     ini.Int("SELECT", "SELECT_NEXT_X", 0),
		NextY:     ini.Int("SELECT", "SELECT_NEXT_Y", 4),
		FontSize:  ini.Int("SELECT", "SELECT_TEXT_FONT_SIZE", 24),
		TextType:  ini.Int("SELECT", "SELECT_TEXT_TYPE", 0),
		TextPlusX: ini.Int("SELECT", "SELECT_TEXT_PLUS_X", 0),
		TextPlusY: ini.Int("SELECT", "SELECT_TEXT_PLUS_Y", 0),
	}
}

// ParseCharaView 解析独立的 _charaview 条目。
//
// 为什么单独一个入口：立绘鉴赏器的配置（含 [CHARA01..06] 的服装/表情表与
// [BG] 的背景名表）在 srp.pac 的 `_charaview` 条目里，而不在 _system。
// ParseUI 只能看到 _system，所以这一份必须由 session 单独读。
func ParseCharaView(ini *Ini) CharaView {
	return parseCharaView(ini)
}

// parseCharaView 解析 [CHARAVIEW] 与 [BG]。
//
// 列表行 CharaStat=6,1008,40,0,30,156,... 的字段是
// "项数, x, y, 保留, 格宽, 格高, 步进"；按钮行 B_Scale=1088,481,776,0,76,30,-80,0,-76,0
// 是 "屏幕x, 屏幕y, 图集x, 图集y, 宽, 高" 之后**再接一组**同格式的字段，
// 指的是同一行上第二个按钮（拡大↔縮小 / ◀↔▶）。
func parseCharaView(ini *Ini) CharaView {
	view := CharaView{FontSize: ini.Int("CHARAVIEW", "FontSize", 14)}
	view.Characters = splitList(ini.Get("CHARAVIEW", "CharaName"))
	// CharaStat=6,1008,40,0,30,156,30：项数, x, y, 保留, 格高, 格宽, 步进。
	// 注意第 5 个字段是"高"、第 6 个是"宽"——原版的竖列表是 156 宽 30 高，
	// 反过来读会让每一行盖住下面五行（点第 3 行结果命中第 1 行）。
	if stat := splitFields(ini.Get("CHARAVIEW", "CharaStat")); len(stat) >= 7 {
		view.ListX = atoiField(stat[1])
		view.ListY = atoiField(stat[2])
		view.ListH = atoiField(stat[4])
		view.ListW = atoiField(stat[5])
		view.ListStep = atoiField(stat[6])
	}
	if stat := splitFields(ini.Get("CHARAVIEW", "BGStat")); len(stat) >= 3 {
		view.BGX = atoiField(stat[1])
		view.BGY = atoiField(stat[2])
		if len(stat) >= 7 {
			view.BGH = atoiField(stat[4])
			view.BGW = atoiField(stat[5])
			view.BGStep = atoiField(stat[6])
		}
	}
	// [BG] 段：BG01=BG0100_a,コックピット_通常。第一段是 GRD 名、第二段是显示名。
	// 以前只解析 [CHARAVIEW]，背景列表永远是空的 —— 界面就只剩黑底。
	for _, key := range ini.Keys("BG") {
		fields := splitList(ini.Get("BG", key))
		if len(fields) == 0 {
			continue
		}
		entry := ViewEntry{Name: fields[0], Label: fields[0]}
		if len(fields) >= 2 {
			entry.Label = fields[1]
		}
		view.BGList = append(view.BGList, entry)
	}
	// B_Hide=1008,567,1008,0,156,30：屏幕 x, 屏幕 y, 图集 x, 图集 y, 宽, 高。
	// RectAt(fields,0) 会把 (x,y,srcX,srcY) 拼成一个矩形（宽=srcX），
	// 结果 H=0、Contains 恒 false —— 「戻る」点了没反应就是这个。
	parseBtn := func(fields []string, offset int) (Rect, Rect, bool) {
		if len(fields) < offset+6 {
			return Rect{}, Rect{}, false
		}
		w, h := atoiField(fields[offset+4]), atoiField(fields[offset+5])
		if w <= 0 || h <= 0 {
			return Rect{}, Rect{}, false
		}
		screen := Rect{X: atoiField(fields[offset]), Y: atoiField(fields[offset+1]), W: w, H: h}
		src := Rect{X: atoiField(fields[offset+2]), Y: atoiField(fields[offset+3]), W: w, H: h}
		return screen, src, true
	}
	// 第二个按钮是**相对偏移**：B_Scale=1088,481,776,0,76,30,-80,0,-76,0 表示
	// 同尺寸的按钮在屏幕 (1088-80, 481)、图集 (776-76, 0)，即 拡大 在 縮小 左边。
	// B_MoveY=...,0,86,156,0 是 ▲ 上、▼ 下偏 86px。按绝对坐标读会把两个箭头
	// 叠在同一处——"按钮重叠"就是这么来的。
	// 注意第二组只有 4 个字段（屏幕 dx,dy + 图集 dx,dy），尺寸沿用第一个按钮。
	both := func(key string) (Rect, Rect, Rect, Rect) {
		fields := splitFields(ini.Get("CHARAVIEW", key))
		a, as, okA := parseBtn(fields, 0)
		if !okA {
			return Rect{}, Rect{}, Rect{}, Rect{}
		}
		if len(fields) < 10 {
			return a, as, Rect{}, Rect{}
		}
		dx, dy := atoiField(fields[6]), atoiField(fields[7])
		sdx, sdy := atoiField(fields[8]), atoiField(fields[9])
		b := Rect{X: a.X + dx, Y: a.Y + dy, W: a.W, H: a.H}
		bs := Rect{X: as.X + sdx, Y: as.Y + sdy, W: as.W, H: as.H}
		return a, as, b, bs
	}
	var ok bool
	view.Hide, view.HideSrc, _, _ = both("B_Hide")
	view.End, view.EndSrc, _, _ = both("B_End")
	view.Init, view.InitSrc, _, _ = both("B_Init")
	view.MoveX, view.MoveXSrc, view.MoveX2, view.MoveX2Src = both("B_MoveX")
	view.MoveY, view.MoveYSrc, view.MoveY2, view.MoveY2Src = both("B_MoveY")
	view.Scale, view.ScaleSrc, view.Scale2, view.Scale2Src = both("B_Scale")
	_ = ok
	// 四行选择条两侧的小箭头：图集 sys_charaview_button 里 ◀ 是 (157,0,38,30)、
	// ▶ 是 (197,0,38,30)。原版每一行都是 "◀ 名称 ▶" 的三段结构（截图可见），
	// 箭头本身不是配置项，是固定素材。
	view.RowArrowLsrc = Rect{X: 157, Y: 0, W: 38, H: 30}
	view.RowArrowRsrc = Rect{X: 197, Y: 0, W: 38, H: 30}
	// CHARA01..CHARA06 六段：每段是一个"角色页"，带自己的服装/表情表。
	for page := 1; page <= 6; page++ {
		section := "CHARA0" + itoaInt(page)
		if ini.Get(section, "CharaName") == "" {
			continue
		}
		view.Entries = append(view.Entries, CharaEntry{
			Page:        section,
			Prefix:      ini.Get(section, "CharaName"),
			Costumes:    splitList(ini.Get(section, "CharaCostumeName")),
			CostumeList: splitList(ini.Get(section, "CharaCostumeList")),
			Expressions: splitList(ini.Get(section, "CharaExpressionNameP01")),
			ExprList:    splitList(ini.Get(section, "CharaExpressionListP01")),
		})
	}
	return view
}

// itoaInt 是解析阶段的小工具（避免为了一个数字引入 fmt）。
func itoaInt(value int) string {
	if value < 10 {
		return string(rune('0' + value))
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10))
}

func splitFields(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseFloatField(value string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

// partPage 取 SYSTEM_PARTS_0101 里的 "01"（页号，1 起）。
func partPage(key string) int {
	const prefix = "SYSTEM_PARTS_"
	if !strings.HasPrefix(key, prefix) || len(key) < len(prefix)+2 {
		return 0
	}
	page, err := strconv.Atoi(key[len(prefix) : len(prefix)+2])
	if err != nil {
		return 0
	}
	return page
}
