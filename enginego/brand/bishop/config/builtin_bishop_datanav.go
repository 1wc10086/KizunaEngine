package config

// applyBuiltinDataNav 补齐存读档页右下角的导航按钮：Config / SaveMode / LoadMode / Back / Title / Exit。
// 坐标对应 KD manual/imgs/dm.png 右侧一列，每格 128x24，纵向间隔 28。
//
// cfg_button 是一张 128x144 的竖排图，共 6 帧，单帧高 144/6 = 24，正好是一格按钮的高度。
// Back / Title / Exit 三个文字按钮都取自这张图的不同帧，所以用 Start 指定起始帧（0 基：Back=3、Title=4、Exit=5）。
// FixedFrame 让按钮始终画 Start 指定的那一帧，不随悬停切换，否则鼠标经过时会跳到其他文字。
// Frames 必须写成 6：FrameHeight 靠它把整张图切成单帧高度。
// cfg_mode_* 是 3 帧竖排图，只需按状态取帧，不设置 Start / FixedFrame。
func applyBuiltinDataNav(cfg *BsxxConfiguration) {
	nav := &cfg.DataScreen
	nav.Config = UIButton{Image: "cfg_mode_config", Frames: 3, X: 632, Y: 404}
	// cfg_mode_save/load 的第 2 帧是"不可用"的叉号，而通用存档页把
	// frame=2 当作"当前模式"。直接按状态取帧会让正在看的页显示成被禁用；
	// 这里固定画第 1 帧（常态文字），当前模式由画面顶部的 SAVE/LOAD 标识。
	nav.SaveMode = UIButton{Image: "cfg_mode_save", Frames: 3, X: 632, Y: 432, Start: 1, FixedFrame: true}
	nav.LoadMode = UIButton{Image: "cfg_mode_load", Frames: 3, X: 632, Y: 460, Start: 1, FixedFrame: true}
	nav.Back = UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 488, Start: 3, FixedFrame: true}
	nav.Title = UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 516, Start: 4, FixedFrame: true}
	nav.Exit = UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 544, Start: 5, FixedFrame: true}
}
