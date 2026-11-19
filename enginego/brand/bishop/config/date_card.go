package config

// DateCard 是剧情画面左上角的日期/时刻卡（学園 GK/GKDVD/GKWindows10）。
//
// 它不是 [message window] 的附属小窗：原版在对话、选择肢和地图上都把
// 同一张 date_base 贴在 (0,0)，只是地图另外还有目标头像和工具条。
// 卡里的年/周/曜日/AM-PM/时/分是独立帧图，引擎按 #GKR_* 属性每帧取帧。
type DateCard struct {
	Base string
	X    int
	Y    int
	// WeekNo/Week/AMPM/Hour/Minute 是各自帧图的屏幕坐标；
	// 对应的帧号来自 #GKR_week_no/#GKR_week/#GKR_ampm/#GKR_hour/#GKR_minute。
	WeekNo UIPoint
	Week   UIPoint
	AMPM   UIPoint
	Hour   UIPoint
	Minute UIPoint
}

// Empty 表示作品没有内置日期卡。
func (d DateCard) Empty() bool {
	return d.Base == ""
}
