package config

// builtin_bishop_gk2_config.go 给 学園2～淫虐の図式～ 老版（BSScript Rev.2）补旧式设置画面布局。
//
// 这一代的设置底图 config_base 已经烘焙好标签，引擎只负责贴值图、滑杆和角色页签。
// 坐标来自 config_hit 与各 cfg_* 素材的核对结果。只在 applyBuiltinGK2 中调用，其它作品不会走到这里。

func applyBuiltinGK2Config(cfg *BsxxConfiguration) {
	cfg.LegacyConfig = LegacyConfigScreen{
		Base:     "config_base",
		HitTable: "config_hit",
		Back:     UIButton{Image: "cfg_back", Frames: 2, X: 30, Y: 552},
		Title:    UIButton{Image: "cfg_return", Frames: 2, X: 379, Y: 558},
		Exit:     UIButton{Image: "cfg_quit", Frames: 2, X: 639, Y: 560},
		// 角色页签帧：0 常态 / 1 语音关闭（灰头像）/ 2 当前选中，与素材帧号一致。
		CharacterOffFrame:      1,
		CharacterSelectedFrame: 2,
	}

	// 音量通道固定 7 个，下标与 legacyGaugeValue 对齐：1=BGM 2=SE 3=環境音 4=音声。
	// 0、5、6 在本作不存在，留空即可，addGauge 会跳过。
	cfg.LegacyConfig.Volumes = make([]LegacyGauge, 7)
	cfg.LegacyConfig.Volumes[1] = gk2VolumeBar(113)
	cfg.LegacyConfig.Volumes[2] = gk2VolumeBar(203)
	cfg.LegacyConfig.Volumes[3] = gk2VolumeBar(143)
	cfg.LegacyConfig.Volumes[4] = gk2VolumeBar(173)

	// 文字速度 / 自动速度。滑块沿 192px 轨道横向移动，量化为 12 档。
	cfg.LegacyConfig.Speeds = []LegacyGauge{
		{Knob: UIButton{Image: "cfg_meter2", Frames: 1, X: 555, Y: 424}, HorizontalKnob: true, TrackWidth: 192, Steps: 12},
		{Knob: UIButton{Image: "cfg_meter2", Frames: 1, X: 555, Y: 492}, HorizontalKnob: true, TrackWidth: 192, Steps: 12},
	}

	names := []string{"cfg_haruna", "cfg_miku", "cfg_kaori", "cfg_izumi", "cfg_ayano", "cfg_kyouko", "cfg_etc"}
	points := [...]UIPoint{{54, 306}, {130, 306}, {206, 306}, {282, 306}, {92, 364}, {168, 364}, {244, 364}}
	for index, name := range names {
		cfg.LegacyConfig.Characters = append(cfg.LegacyConfig.Characters, LegacyCharacter{
			Button: UIButton{Image: name, Frames: 3, X: points[index].X, Y: points[index].Y},
		})
	}

	cfg.LegacyConfig.ValueGroups = []LegacyValueGroup{
		{Key: "screen mode", Options: []LegacyValueOption{
			gk2Option("cfg_window", 527, 83, 0),
			gk2Option("cfg_full", 652, 83, 1),
		}},
		{Key: "screen effect", Options: []LegacyValueOption{
			gk2Option("cfg_effect_fixed", 527, 129, 1),
			gk2Option("cfg_effect_skip", 652, 129, 2),
			gk2Option("cfg_effect_off", 527, 159, 0),
		}},
		{Key: "liquid", Options: []LegacyValueOption{
			gk2Option("cfg_liquid_off", 527, 219, 0),
			gk2Option("cfg_liquid_on", 652, 219, 1),
		}},
		{Key: "skip unread", Options: []LegacyValueOption{
			gk2Option("cfg_noread_off", 527, 279, 0),
			gk2Option("cfg_noread_on", 652, 279, 1),
		}},
		// 图名与文字相反：cfg_skip_slow 是「通常」(值0)，cfg_skip_normal 是「高速」(值1)。
		{Key: "skip speed", Options: []LegacyValueOption{
			gk2Option("cfg_skip_slow", 527, 325, 0),
			gk2Option("cfg_skip_normal", 652, 325, 1),
			gk2Option("cfg_skip_next", 527, 355, 2),
		}},
		{Key: "voice loop", Options: []LegacyValueOption{
			gk2Option("cfg_loop_on", 130, 491, 1),
			gk2Option("cfg_loop_off", 255, 491, 0),
		}},
	}
}

// gk2VolumeBar 返回一条音量填充条：底图是空槽，cfg_meter1 按音量裁宽度贴上去。
func gk2VolumeBar(y int) LegacyGauge {
	return LegacyGauge{Bar: UIButton{Image: "cfg_meter1", Frames: 1, X: 91, Y: y}, Fill: true, TrackWidth: 276}
}

// gk2Option 的素材都是 3 帧竖排（常态/悬停/选中）；宽 250 的选项同样按 3 帧切。
func gk2Option(image string, x, y, value int) LegacyValueOption {
	return LegacyValueOption{Button: UIButton{Image: image, Frames: 3, X: x, Y: y}, Value: value}
}
