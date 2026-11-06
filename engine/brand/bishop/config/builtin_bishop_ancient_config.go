package config

// builtin_bishop_ancient_config.go 给 Tbxdl 世代（特別病棟 旧版 / Win10）补设置画面与回想。
//
// config_base 已经把 MESSAGE / SOUND / SCREEN 三块标签和六位角色名烘焙在底图上，
// 引擎只负责在标签右侧贴"当前值"的选项图（cfg_itemN）和几条值条：
//   - cfg_item1  OFF / コマ数固定 / 時間固定（未読 Skip 的三种处理）
//   - cfg_item2  OFF / ON（VoiceRepeat、画面効果、汁設定）
//   - cfg_item3  通常 / 高速 / 選択肢まで（Skip 速度）
//   - cfg_item4  11 帧横向值条（文字速度、BGM/SE/Voice 音量）
//   - cfg_item5  Window / 全画面（画面表示設定）
//   - cfg_item6  11 帧横向值条（Auto 速度）
// 这些图不是各作品的 cfg_mark/cfg_knob，不能用 KD 的竖向勾选逻辑。
// 坐标按 config_base 的行位置量出；底图尺寸 800x600，全部落在画面内。

// applyBuiltinAncientConfig 填入设置画面。调用方只在配置为空时才会进到这里。
func applyBuiltinAncientConfig(cfg *BsxxConfiguration) {
	if cfg == nil {
		return
	}
	cfg.LegacyConfig = LegacyConfigScreen{
		Base: "config_base",
		// cfg_itemN 自己就是选项图，不需要额外的勾选图标。
		CheckMarkFrames: 1,
		RadioFrames:     1,
		// 角色语音关闭时的帧号沿用 KD 约定；本作没有 cfg_chN，Characters 留空。
		CharacterOffFrame: 1,

		// MESSAGE：文字速度、未読 Skip、Skip 速度、Auto 速度。
		Speeds: []LegacyGauge{
			{Bar: UIButton{Image: "cfg_item4", Frames: 11, X: 220, Y: 145}, ValueFrame: true},
			{Bar: UIButton{Image: "cfg_item6", Frames: 11, X: 220, Y: 270}, ValueFrame: true},
		},
		SkipUnreadButton: UIButton{Image: "cfg_item1", Frames: 3, X: 220, Y: 175},
		SkipSpeedButton:  UIButton{Image: "cfg_item3", Frames: 3, X: 220, Y: 210},

		// SOUND：BGM / SE / Voice 三条值条；VoiceRepeat 是 OFF/ON。
		Volumes: []LegacyGauge{
			{},
			{Bar: UIButton{Image: "cfg_item4", Frames: 11, X: 560, Y: 130}, ValueFrame: true},
			{Bar: UIButton{Image: "cfg_item4", Frames: 11, X: 560, Y: 170}, ValueFrame: true},
			{},
			{Bar: UIButton{Image: "cfg_item4", Frames: 11, X: 560, Y: 210}, ValueFrame: true},
		},
		VoiceRepeatButton: UIButton{Image: "cfg_item2", Frames: 2, X: 560, Y: 250},

		// SCREEN：画面表示設定、画面効果、汁設定。
		ScreenModeButton:   UIButton{Image: "cfg_item5", Frames: 2, X: 220, Y: 365},
		ScreenEffectButton: UIButton{Image: "cfg_item2", Frames: 2, X: 220, Y: 400},
		JuiceButton:        UIButton{Image: "cfg_item2", Frames: 2, X: 220, Y: 435},

		// 底部三键复用存档画面的 Sys_* 素材，与原版导航条一致。
		Back:  UIButton{Image: "Sys_back", Frames: 3, X: 639, Y: 551},
		Title: UIButton{Image: "Sys_title", Frames: 3, X: 685, Y: 551},
		Exit:  UIButton{Image: "Sys_quit", Frames: 3, X: 731, Y: 551},
	}
}

// applyBuiltinAncientLog 填入回想。
// 注意这里不设 Base：Ancient 回想是叠在当前剧情上的半透明黑层，
// 引擎会先恢复打开前抓下的 backdrop，再叠 BackColor。
func applyBuiltinAncientLog(cfg *BsxxConfiguration) {
	if cfg == nil {
		return
	}
	cfg.Log = LogScreen{
		BackColor:    0xcc000000,
		HasBackColor: true,
		Lines:        22,
		LeftPos:      69,
		Width:        680,
		Bar:          UIButton{Image: "log_meter", Frames: 1, X: 776, Y: 24},
		VoiceIcon:    "log_vp",
	}
}
