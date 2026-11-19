package config

// builtin_bishop_afdvd_config.go 给 AFDVD 补旧式设置画面。
//
// config_base / config_hit 都是 800x600 的整屏素材，命中表用纯色块标出
// 每个可点区域。这里把命中表里的坐标按功能分组，不再让引擎猜：
//   - 画面設定：全画面表示、アンチエイリアス两个复选框；
//   - 画面効果、計量設定：各三个单选；
//   - 射精位置：10 行单选（每行 17px，正好落在面板内）；
//   - 文章設定：メッセージ速度/オート速度各 10 个离散刻度点，
//     スキップ速度行实际是"未読もスキップ"复选框 + 三档单选；
//   - 文字背景：赤/緑/青/濃度四条横向滑杆；
//   - 音量設定：BGM/音声/効果音三条横向滑杆；
//   - 音声設定：7 个角色头像开关 + ループ再生；
//   - 底栏：記録/呼出/環境設定三个页签。
//
// 坐标全部来自 config_hit 的色块包围盒（x,y 取色块左上角；勾选/单选图标
// 是 16x16，正好覆盖底图上的空框/空心圆）。

func applyBuiltinAFDVDConfig(cfg *BsxxConfiguration) {
	cfg.LegacyConfig = LegacyConfigScreen{
		Base:            "config_base",
		HitTable:        "config_hit",
		CheckMark:       "cfg_mark",
		RadioButton:     "cfg_mark",
		CheckMarkFrames: 3,
		RadioFrames:     3,

		// 标签在图标右侧，整行都可点（否则玩家点"全画面表示"没反应）。
		ToggleHitWidth: 160,
		ScreenMode:     UIPoint{X: 64, Y: 112},
		SquareMode:     UIPoint{X: 64, Y: 132},
		SkipUnread:     UIPoint{X: 528, Y: 152},
		VoiceLoop:      UIPoint{X: 88, Y: 536},

		SkipModeMarkFrame: 1,
		SkipModes: []UIPoint{
			{X: 528, Y: 172},
			{X: 528, Y: 192},
			{X: 528, Y: 212},
		},
		RadioGroups: []LegacyRadioGroup{
			{Key: "screen effect", MarkFrame: 1, HitWidth: 140, Points: []UIPoint{
				{X: 64, Y: 192}, {X: 64, Y: 212}, {X: 64, Y: 232},
			}},
			{Key: "measure", MarkFrame: 1, HitWidth: 140, Points: []UIPoint{
				{X: 212, Y: 192}, {X: 212, Y: 212}, {X: 212, Y: 232},
			}},
			{
				Key: "auto select pos", MarkFrame: 1, HitWidth: 140, Points: afdvdAutoSelectPoints(),
				// 原作这一栏的文字没有烘焙在 config_base 里，必须由引擎画。
				// 文案取自 AFDVD 设置画面截图/手册。
				// 原作一次露出 10 行。命中表的灰块是 20px 一行，但面板内高
				// 只够 10 行 × 18px；文字和单选点共用 18px 行距，最后一行
				// 仍落在面板底边之内。
				Labels: []string{
					"手前に中出し", "中に出す", "口内に中出し", "口内に出す",
					"顔にぶっかける", "顔に出す", "腹にぶっかける", "腹に出す",
					"胸にぶっかける", "胸に出す",
				},
			},
		},

		Speeds: []LegacyGauge{
			afdvdDotGauge(116),
			afdvdDotGauge(136),
		},
		// 文字背景四条：底图已经画好彩色横条，引擎只画当前位置竖线。
		Colors: []LegacyGauge{
			afdvdMarkerGauge(280),
			afdvdMarkerGauge(300),
			afdvdMarkerGauge(320),
			afdvdMarkerGauge(340),
		},
		// 音量通道下标固定为 master/BGM/SE/BGS/voice/BGV/减衰；
		// 本作只暴露 BGM(1)/音声(4)/効果音(2)，其余槽位留空占位。
		Volumes: []LegacyGauge{
			{},
			afdvdMarkerGauge(408),
			afdvdMarkerGauge(448),
			{},
			afdvdMarkerGauge(428),
		},

		Characters: []LegacyCharacter{
			{Button: afdvdCharacterButton(1, 312)},
			{Button: afdvdCharacterButton(2, 372)},
			{Button: afdvdCharacterButton(3, 432)},
			{Button: afdvdCharacterButton(4, 492)},
			{Button: afdvdCharacterButton(5, 552)},
			{Button: afdvdCharacterButton(6, 612)},
			{Button: afdvdCharacterButton(7, 672)},
		},
		// cfg_chN 只有一帧，不区分开/关；保持默认 -1 不额外切帧。
		CharacterOffFrame: -1,

		Reset: UIButton{Image: "cfg_reset", Frames: 2, X: 364, Y: 4},
		Back:  UIButton{Image: "cfg_button", Frames: 6, X: 511, Y: 10, Start: 0, FixedFrame: true},
		Title: UIButton{Image: "cfg_button", Frames: 6, X: 605, Y: 10, Start: 1, FixedFrame: true},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, X: 699, Y: 10, Start: 2, FixedFrame: true},

		ModeSave:   UIButton{Image: "cfg_mode_save", Frames: 3, X: 298, Y: 576},
		ModeLoad:   UIButton{Image: "cfg_mode_load", Frames: 3, X: 402, Y: 576},
		ModeConfig: UIButton{Image: "cfg_mode_config", Frames: 3, X: 506, Y: 576},
	}
}

// afdvdAutoSelectPoints 是射精位置列表的 10 个单选点：左列 x=52，
// 从 y=296 起每行 17px。命中表的灰块是 20px 一行，但面板内高只够
// 10×17；文字和圆点共用这套坐标，保证最后一行不被面板底边裁掉。
func afdvdAutoSelectPoints() []UIPoint {
	points := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		points = append(points, UIPoint{X: 52, Y: 296 + index*17})
	}
	return points
}

// afdvdDotGauge 生成"遅・・・・速"的十个离散刻度点。cfg_mark 第 3 帧
// 是蓝色菱形，正好套在底图的圆点上。
func afdvdDotGauge(y int) LegacyGauge {
	dots := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		dots = append(dots, UIPoint{X: 548 + index*16, Y: y})
	}
	return LegacyGauge{
		Dots:    dots,
		DotMark: UIButton{Image: "cfg_mark", Frames: 3},
	}
}

// afdvdMarkerGauge 生成一条横向滑杆：轨道从 x=492 到 691，宽 200、高 16。
func afdvdMarkerGauge(y int) LegacyGauge {
	return LegacyGauge{
		MarkerOnly:  true,
		Knob:        UIButton{X: 492, Y: y},
		TrackWidth:  200,
		TrackHeight: 16,
	}
}

// afdvdCharacterButton 生成第 n 个角色头像（1 基部件）。
func afdvdCharacterButton(index, x int) UIButton {
	return UIButton{Image: "cfg_ch" + itoa(index), Frames: 1, X: x, Y: 513}
}
