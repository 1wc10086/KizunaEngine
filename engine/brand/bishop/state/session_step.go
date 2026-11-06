package state

type SessionStep int

const (
	SessionContinue SessionStep = iota
	SessionAwaitInput
	SessionAwaitChoice
	SessionAwaitSelect
	SessionAwaitMap
	SessionWait
	SessionFinished
	SessionFaulted
	// SessionAwaitTitle 停在主界面，等待宿主把菜单点击回传（见 Session.SelectTitleItem）。
	SessionAwaitTitle
	// SessionAwaitMovie 停在影片上，等宿主播完回调 OpMovieDone。
	SessionAwaitMovie
	// SessionAwaitTarget 停在原版的目标选择画面（system call "target"）。
	// 与地图一样由引擎自绘，脚本在玩家选中前不推进。
	SessionAwaitTarget
	// SessionAwaitStaffroll 停在片尾字幕；字幕滚完或玩家点击后继续脚本。
	SessionAwaitStaffroll
	// SessionAwaitEyecatch 停在章节间的过场；短动画播完或点击后继续。
	SessionAwaitEyecatch
)
