package config

// builtin_bishop_ancient.go 给 Ancient BSScript 作品（特別病棟 旧版与 Win10 版）补内置布局。
//
// 这一代发行目录里既没有 bsxx.cfg，也没有 bssystem 配置：标题、对话框、工具条全部由原引擎
// 按固定坐标画出。坐标是按 tbxdl.exe 的对象表和素材像素量出的。
// 这一代不走 Rev 布局（builtin_bishop_legacy.go），KD 的坐标和素材签名与它对不上。
// 只在配置为空且素材齐全时生效；现代作品有 bsxx.cfg，不会进到这里。

var builtinAncientAssets = []string{
	"title_base", "title_menu", "message", "unagashi", "item",
	"config_base", "log", "save", "load", "save_load", "information",
	"Sys_title", "Sys_back", "Sys_quit",
}

// ApplyBuiltinAncientDefaults 在 Ancient 作品没有配置时填入标题、对话框、工具条以及各个子画面。
// 只填空缺项；素材签名不全时返回 false 且不修改 cfg。
// 注意 msgwnd / msgwnd_b 不在素材清单里：原版对话框只用 message 底图，msgwnd_b 是另一套布局。
func ApplyBuiltinAncientDefaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || probe.Exists == nil {
		return false
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	for _, name := range builtinAncientAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	// 原作固定 800x600，不跟随窗口探测：标题坐标是按这个画布量出来的。
	cfg.Width = 800
	cfg.Height = 600
	applyBuiltinAncientTitle(cfg)
	applyBuiltinAncientMessage(cfg)
	applyBuiltinAncientFont(cfg)
	applyBuiltinAncientToolbar(cfg)
	// 以下子画面由各自的工作流实现，签名固定；它们只写各自的布局字段，
	// 不会覆盖上面的标题/对话框/工具条。
	applyBuiltinAncientDataScreen(cfg)
	applyBuiltinAncientMuseum(cfg)
	applyBuiltinAncientConfig(cfg)
	applyBuiltinAncientLog(cfg)
	applyBuiltinAncientInformation(cfg)
	applyBuiltinAncientStatus(cfg)
	return true
}

func applyBuiltinAncientTitle(cfg *BsxxConfiguration) {
	// 菜单项坐标是 title_menu 帧图贴在 title_base 上的左上角（原引擎量出的值）。
	// title_menu 的 7 帧分别是 7 个菜单项的悬停态，平时只靠 title_base 自带的白字，
	// 所以 HoverOnly：未悬停时不画，否则高亮帧会压在白字上。
	positions := [7][2]int{{125, 440}, {325, 440}, {525, 440}, {125, 480}, {325, 480}, {525, 480}, {325, 520}}
	keys := [7]string{"start", "load", "config", "museum_music", "museum_cg", "museum_scene", "quit"}
	entries := make([]TitleMenuEntry, 0, len(keys))
	for index, key := range keys {
		entries = append(entries, TitleMenuEntry{
			Key:       key,
			Image:     "title_menu",
			Frames:    7,
			X:         positions[index][0],
			Y:         positions[index][1],
			Frame:     index,
			HoverOnly: true,
		})
	}
	// 标题 BGM 固定为 bgm08；不设的话进入标题时沿用上一段剧情的 BGM。
	cfg.Title = TitlePhase{Base: "title_base", BgmName: "bgm08", Entries: entries}
}

func applyBuiltinAncientMessage(cfg *BsxxConfiguration) {
	// message 是 800x160 底图，贴在 (0,440)，已自带正文框与右侧工具框。
	// BgColor 为 0 时底图原样贴出，不叠半透明底色，否则会把原版的纸纹压暗。
	cfg.Message = MessageWindow{
		Base:            "message",
		BasePosSet:      true,
		BaseX:           0,
		BaseY:           440,
		Speed:           100,
		BgColor:         0,
		NameX:           50,
		NameY:           14,
		NameW:           520,
		NameH:           24,
		NameColor:       0xffffffff,
		TextX:           50,
		TextY:           40,
		TextW:           520,
		TextH:           96,
		Unagashi:        "unagashi",
		UnagashiX:       570,
		UnagashiY:       105,
		UnagashiCellW:   30,
		UnagashiCellH:   30,
		UnagashiColumns: 1,
		UnagashiFrames:  4,
		UnagashiSpeed:   400,
	}
}

func applyBuiltinAncientFont(cfg *BsxxConfiguration) {
	// 字体文件保持空：本作没有 bsxx.ttf，引用不存在的字体会让正文回退到不可控的默认字形。
	cfg.Font = FontSettings{
		Size:       20,
		Width:      22,
		Height:     24,
		Edge:       1,
		Shadow:     1,
		LineLength: 34,
		Lines:      4,
	}
}

func applyBuiltinAncientToolbar(cfg *BsxxConfiguration) {
	// 12 个工具按钮各自是独立图（45x108 = 4 帧 45x27），叠在 message 右侧工具框之上。
	// 不设 GameMenu.Base：底图已经由 message 提供，再叠一张就是“两个信息框”。
	// Overlay 表示按钮是叠在对话框上的独立控件，点击命中按按钮包围盒判定（见 session/messagewindow.go）。
	cfg.GameMenu = GameMenu{
		ShowArea: [4]int{0, 440, 800, 160},
		Overlay:  true,
	}
	// 键名与 session.activateMessageButton 的 switch 一致；图名对应 Graphics2.bsa 里的 *.bsg。
	entries := [12]struct{ key, image string }{
		{"skip", "skip"}, {"auto", "auto"}, {"delete", "delete"},
		{"quick save", "qsave"}, {"quick load", "qload"}, {"log", "log"},
		{"save", "save"}, {"load", "load"}, {"status", "status"},
		{"v.play", "vplay"}, {"config", "config"}, {"quit", "quit"},
	}
	// 按钮位置与规格的 3 列 x 4 行网格一致：按行填充，键顺序与规格列表相同。
	columns := [3]int{627, 675, 723}
	rows := [4]int{459, 489, 519, 549}
	for index, entry := range entries {
		cfg.GameMenu.Buttons = append(cfg.GameMenu.Buttons, MenuButton{
			Key: entry.key,
			UIButton: UIButton{
				Image:  entry.image,
				Frames: 4,
				X:      columns[index%3],
				Y:      rows[index/3],
			},
		})
	}
}
