package config

// applyBuiltinConfigScreen 为 KD（毎日けだものっ!!，800x600）填入旧式设置画面。
//
// 底图 config_base 与标签位置应对照 bishop/游戏/KD/manual/imgs/config.png，但实现时
// 该图无法读取（当前代理没有图像输入），滑块、勾选、按钮坐标由 cfg_* 素材尺寸推测，未逐像素核对。
//
// 竖向滑杆：底图已经画好轨道，只需要 cfg_knob 滑块。Knob 的 X/Y 作为轨道左上角，
// TrackHeight 为轨道高度，Bar 留空。滑块 y 从 118 走到 298：值为最小时在顶端，
// 最大时在底端（见 systemui_config_legacy.go 的 setLegacyGaugeAt）；原图是否如此未核对。
// Min/Max 两端按钮放在轨道上下两端：cfg_min 在 y=108（顶），cfg_max 在 y=300（底）。
//
// 复合条按钮：cfg_button 是 128x144 的 6 帧竖排图，Back/Title/Exit 分别固定画第
// 3/4/5 帧（Start），FixedFrame 让它们不随悬停切换。
//
// 音量下标即引擎的音量通道号：0 master、1 BGM、2 SE、3 BGS、4 VOICE、5 BGV、6 減衰。
// KD 没有 master/BGS/減衰，这些下标留空。颜色下标：0 赤、1 緑、2 青、3 不透明。
// 切片按引擎下标排列；各条的 X 坐标是推测值，并非都按原图从左到右排列。
func applyBuiltinConfigScreen(cfg *BsxxConfiguration) {
	cfg.LegacyConfig = LegacyConfigScreen{
		Base:              "config_base",
		HitTable:          "config_hit",
		CheckMark:         "cfg_mark",
		RadioButton:       "cfg_mark",
		CheckMarkFrames:   3,
		RadioFrames:       3,
		CharacterOffFrame: 1,

		// 勾选标记图是 100x36 的椭圆，坐标是"标记左上角"，要让它套住
		// config_hit 里的 36x24 命中块：命中块中心 - (50,18)。
		VoiceLoop:  UIPoint{X: 112, Y: 2},
		ScreenMode: UIPoint{X: 140, Y: 154},
		AllEffect:  UIPoint{X: 140, Y: 210},
		SkipUnread: UIPoint{X: 380, Y: 450},
		AutoSelect: []UIPoint{
			{X: 22, Y: 310},
			{X: 114, Y: 310},
			{X: 206, Y: 310},
		},
		// スキップ処理 通常/高速/次の選択肢まで：(260/376/484,532) 的命中块居中。
		SkipModes: []UIPoint{
			{X: 254, Y: 524},
			{X: 368, Y: 524},
			{X: 480, Y: 524},
		},

		// ON/OFF 分两点画时的 OFF 坐标，取值来源：原版 config.png 中 OFF 文本的居中点，
		// 减去 cfg_mark 宽度 100 的一半，得到标记左边界。VoiceLoop 文本约 x=230 → 180；
		// SkipUnread 文本约 x=491 → 441。Y 与对应 ON 点相同。
		// 两个单选分组的颜色：射精位置用青色（第 1 帧），スキップ処理用黄色（第 2 帧）。
		VoiceLoopOff:        UIPoint{X: 180, Y: 2},
		SkipUnreadOff:       UIPoint{X: 441, Y: 450},
		AutoSelectMarkFrame: 1,
		SkipModeMarkFrame:   2,

		// 速度：文字速度在左（x=356），自动速度在右（x=392）。
		Speeds: []LegacyGauge{
			builtinVerticalGauge(356),
			builtinVerticalGauge(392),
		},
		// 颜色按引擎下标排列：赤(476)、緑(512)、青(548)、不透明(440)。
		Colors: []LegacyGauge{
			builtinVerticalGauge(476),
			builtinVerticalGauge(512),
			builtinVerticalGauge(548),
			builtinVerticalGauge(440),
		},
		// 音量：下标 1 BGM(676)、2 SE(604)、4 VOICE(640)、5 BGV(712)；0/3/6 留空。
		Volumes: []LegacyGauge{
			{},
			builtinVerticalGauge(676),
			builtinVerticalGauge(604),
			{},
			builtinVerticalGauge(640),
			builtinVerticalGauge(712),
			{},
		},

		// cfg_ch1..cfg_ch6 位于 (16+60*i, 32)，60x60，2 帧；KD 的发行素材
		// 只有 6 个头像，第 7 个不配置（CharacterVoice 本身已扩到 8 槽，
		// 给 AFDVD 的 7 个角色用）。
		Characters: []LegacyCharacter{
			{Button: builtinCharacterButton("cfg_ch1", 0)},
			{Button: builtinCharacterButton("cfg_ch2", 1)},
			{Button: builtinCharacterButton("cfg_ch3", 2)},
			{Button: builtinCharacterButton("cfg_ch4", 3)},
			{Button: builtinCharacterButton("cfg_ch5", 4)},
			{Button: builtinCharacterButton("cfg_ch6", 5)},
		},

		Reset: UIButton{Image: "cfg_reset", Frames: 2, X: 60, Y: 424},
		Back:  UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 488, Start: 3, FixedFrame: true},
		Title: UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 516, Start: 4, FixedFrame: true},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 544, Start: 5, FixedFrame: true},
	}
}

// builtinVerticalGauge 生成 KD 右侧一条竖向滑杆：轨道 x 由参数给出，y 固定为 118，
// 高 180；两端按钮 cfg_min/cfg_max 在同一列。
func builtinVerticalGauge(x int) LegacyGauge {
	return LegacyGauge{
		Knob:        UIButton{Image: "cfg_knob", Frames: 5, X: x, Y: 118},
		Min:         UIButton{Image: "cfg_min", Frames: 2, X: x, Y: 108},
		Max:         UIButton{Image: "cfg_max", Frames: 2, X: x, Y: 300},
		Vertical:    true,
		TrackHeight: 180,
	}
}

// builtinCharacterButton 生成第 index 个角色页签，排列在 y=32 的一行上。
func builtinCharacterButton(image string, index int) UIButton {
	return UIButton{Image: image, Frames: 2, X: 16 + 60*index, Y: 32}
}
