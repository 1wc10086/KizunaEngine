package config

// applyBuiltinInformation 填 KD（毎日けだものっ!!）的行动信息窗。
//
// KD 的 #day 是两位编码：十位是「周」，个位是「曜日」（0 = 日 … 6 = 土）。
// 例如 30 = 第3週日曜日；60、70 … 110 直接对应各路线名，由 WeekStrip 的帧 6..11 绘制。
// 0 是 オープニング，必须作为合法值显示，所以设置 DayZero。
// #time 的 0 是「朝」，同理设置 TimeZero。
//
// WeekDays = 10、WeekStart = 0 让渲染器按 (day-WeekStart)/WeekDays 得到周数、
// 按 % WeekDays 得到曜日，正好拆开十位与个位。
func applyBuiltinInformation(cfg *BsxxConfiguration) {
	cfg.Information = InformationWindow{
		DayVariable:  "#day",
		TimeVariable: "#time",
		WeekStrip:    DigitStrip{Image: "msg_info_str0", Frames: 12, X: 22, Y: 572},
		WeekDayStrip: DigitStrip{Image: "msg_info_str1", Frames: 7, X: 106, Y: 575},
		// msg_info_time 的图头 OffsetX/OffsetY 是 (183,123)，正好是它相对
		// 对话框原点 (0,450) 的位置。drawStrip 会加图头偏移，所以 X/Y 写
		// 对话框原点而不是 181,572；否则会被再加一次 (183,123) 跑到屏幕外。
		TimeStrip:    DigitStrip{Image: "msg_info_time", Frames: 6, X: 0, Y: 450},
		TimeZoneLow:  0,
		TimeZoneHigh: 5,
		WeekDays:     10,
		WeekStart:    0,
		DayZero:      true,
		TimeZero:     true,
		WeekDayFrom:  10,
	}
}
