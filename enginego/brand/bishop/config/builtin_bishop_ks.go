package config

// builtin_bishop_ks.go 给「教育指導」（KS，BSXScript 1.0）补内置的消息窗工具条。
//
// KS 的 bsxx.cfg 只有 `[game menu] base = gamemenu_base`，没有 `[system button]`：
// 原引擎把工具按钮的坐标编译在 ks.exe 里。发行版连 gamemenu_base.bsg 都没带，
// 但 btn_skip/btn_auto/... 这组 4 帧按钮图是齐的。没有这份布局时引擎不会
// 画任何工具按钮，玩家只能看到对话框，无法存档/读档/设置/回想。
//
// 坐标按 KS 手册截图换算到 800x600（手册图是 320x240）：
//   - 左侧竖排 SKIP/AUTO/VOICE；
//   - 底部一排 Q.SAVE/Q.LOAD/SAVE/LOAD/CONFIG/ITEM/LOG/DRESS/CUT IN；
//   - 右侧 VIEW 与上/下两个视角箭头。
//
// KSIE（BSXScript 3.1）的 bsxx.cfg 自带 [system button]，且坐标与截图一致
// （HOLD/AUTO/CUT-IN + SKIP/VOICE/... ），所以只在 Buttons 为空时补；
// 已有配置的作品完全不受影响。

// builtinKsToolbarAssets 是这套布局的素材指纹。只有 KS/KSIE 这一代同时
// 带 map_ch11、map_item 和 btn_cutin；其它现代作品即使有同名按钮图，
// 也不会同时满足全部条件。
var builtinKsToolbarAssets = []string{
	"btn_skip", "btn_auto", "btn_voice",
	"btn_qsave", "btn_qload", "btn_save", "btn_load",
	"btn_config", "btn_item", "btn_log", "btn_dress", "btn_cutin",
	"btn_view_u", "btn_view_m", "btn_view_d",
	"map_ch11", "map_item", "msg_base",
}

// ApplyBuiltinKsToolbar 在 [system button] 缺失且素材吻合时填入 KS 的
// 消息窗工具条。返回 true 表示改写了 cfg。
func ApplyBuiltinKsToolbar(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || probe.Exists == nil {
		return false
	}
	// 已有显式配置的作品不要覆盖：KSIE 的 [system button] 是原作数据，
	// 按钮顺序与 KS 不同（HOLD/AUTO/CUT-IN + SKIP/VOICE/...）。
	if len(cfg.GameMenu.Buttons) > 0 {
		return false
	}
	for _, name := range builtinKsToolbarAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	// 地点名条是这一代地图独有的素材；再加一道保险，避免其它带
	// btn_cutin 的现代作品被误判成 KS。
	if !probe.Exists("plase_1") && !probe.Exists("map_plase_marker1") {
		return false
	}
	// KS 没有 gamemenu_base.bsg；底图留空，按钮直接叠在对话框上。
	cfg.GameMenu.Overlay = true
	cfg.GameMenu.ShowArea = [4]int{0, 456, 800, 144}

	type entry struct {
		key   string
		image string
		x     int
		y     int
	}
	// 左侧竖排：x=17，y 相对 800x600 的对话框底边（消息框在 y=456）。
	// 按钮图 64x104 = 4 帧 64x26。
	entries := []entry{
		{"skip", "btn_skip", 17, 485},
		{"auto", "btn_auto", 17, 512},
		{"voice", "btn_voice", 17, 539},
		// 底部一排：80x80 = 4 帧 80x20，间距 77，和原版按钮图宽度一致。
		{"quick save", "btn_qsave", 17, 576},
		{"quick load", "btn_qload", 94, 576},
		{"save", "btn_save", 171, 576},
		{"load", "btn_load", 248, 576},
		{"config", "btn_config", 325, 576},
		{"status", "btn_item", 402, 576},
		{"log", "btn_log", 479, 576},
		{"dress", "btn_dress", 556, 576},
		{"cut in", "btn_cutin", 633, 576},
		// 右侧视角箭头：26x78 = 3 帧 26x26。
		{"view up", "btn_view_u", 747, 491},
		{"view middle", "btn_view_m", 747, 516},
		{"view down", "btn_view_d", 747, 540},
	}
	cfg.GameMenu.Buttons = make([]MenuButton, 0, len(entries))
	for _, item := range entries {
		cfg.GameMenu.Buttons = append(cfg.GameMenu.Buttons, MenuButton{
			Key: item.key,
			UIButton: UIButton{
				Image:  item.image,
				Frames: 4,
				X:      item.x,
				Y:      item.y,
			},
		})
	}
	return true
}
