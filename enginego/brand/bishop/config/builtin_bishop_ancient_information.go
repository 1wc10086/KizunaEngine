package config

// builtin_bishop_ancient_information.go 给特別病棟（Ancient 世代）补内置的行动信息窗与状態画面设置。
//
// info_font 是 240x20 的横向字形条：前 10 帧是 0-9，后 10 帧是 月..日，必须按宽度切帧，
// 所以周数取 Start=0、曜日取 Start=10，两者都设 Horizontal。
// info_time 是 108x63 竖排 3 帧（朝/昼/夕），按普通竖排切帧即可。
//
// 注意：#day / #time 变量名与 7 日一周的换算是按同系列作品推定的，尚未对照脚本核对；
// 若剧本实际使用别的变量名，这里是唯一需要改的地方。
func applyBuiltinAncientInformation(cfg *BsxxConfiguration) {
	cfg.Information = InformationWindow{
		Base:         "information",
		BaseX:        0,
		BaseY:        0,
		DayVariable:  "#day",
		TimeVariable: "#time",
		// info_font 是变宽字形：0-9 每格 10px，月..日从 x=100 起每格 20px。
		// 周数显示 week+1（Start=1），曜日显示 月..日（Start=10，对应字形 10..16）。
		WeekStrip: DigitStrip{
			Image: "info_font", Frames: 17, X: 31, Y: 8, Start: 1, Horizontal: true,
			AtlasDigits: 10, AtlasDigitWidth: 10, AtlasWideStart: 100, AtlasWideWidth: 20,
		},
		WeekDayStrip: DigitStrip{
			Image: "info_font", Frames: 17, X: 63, Y: 8, Start: 10, Horizontal: true,
			AtlasDigits: 10, AtlasDigitWidth: 10, AtlasWideStart: 100, AtlasWideWidth: 20,
		},
		TimeStrip:    DigitStrip{Image: "info_time", Frames: 3, X: 16, Y: 34},
		TimeZoneLow:  0,
		TimeZoneHigh: 2,
		WeekDays:     7,
		WeekStart:    1,
		// 日期达到 100 只会命中 DaySpecial 分支；这里未配 DaySpecial，所以实际不会替换，保留是为了与原版哨兵值对齐。
		DayBorder: 100,
		// 时段 0 表示朝，是合法值，必须显示。
		TimeZero: true,
	}
}

// applyBuiltinAncientStatus 打开 Tbxdl 世代的内置状態画面。
//
// 这一代没有 status_layout 资源，状態画面由 session/systemui_status_ancient.go 原生绘制；
// Ancient 标记就是让 openStatusScreen 走那条分支。
func applyBuiltinAncientStatus(cfg *BsxxConfiguration) {
	cfg.Status.Ancient = true
	cfg.Status.Base = "status_base"
}
