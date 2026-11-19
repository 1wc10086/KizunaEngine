package config

// builtin_bishop_gk2_museum.go 给学園2 老版（BSScript Rev.2）打开内置鉴赏室。
//
// 这一代没有 [museum phase]：组页签、缩略格、曲目行和播放器全是固定坐标，
// 素材也直接命名成 cg_base/music_base 等。与其在配置模型里伪造一套只服务
// 这一部作品的 UIButton 表，不如只打一个 Gakuen2 标记，坐标集中在
// session/systemui_museum_gk2.go，渲染器一眼就能看出这是专用分支。
func applyBuiltinGK2Museum(cfg *BsxxConfiguration) {
	if cfg == nil {
		return
	}
	cfg.Museum.Gakuen2 = true
	// 标题的三个入口直接打开对应页面；Modes 只用于 openTitleMuseumMode 的
	// 键查找和 drawMuseumTabs 的遍历，按钮图留空（老版每一页都是独立全屏图，
	// 没有跨页的公共页签）。
	cfg.Museum.Modes = []MuseumMode{
		{Key: "music"},
		{Key: "cg"},
		{Key: "scene"},
	}
}
