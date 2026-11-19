package session

// slgScreenState 是剧本 `state training / territory / day_result` 激活的
// SLG 子界面状态。原引擎把这三个名字映射到 embed.bsa 里的 Lua 全局表，
// 由 Lua 负责布局与交互；引擎侧只保存"当前是哪个界面、上一次指针位置"，
// 具体绘制在 slg_lua.go 里。
//
// 这一层单独存在是为了让训练/领地/日结果共用同一套进入/退出规则：
// 进入时暂停脚本、退出时恢复文本与鼠标语义，避免三个界面各写一份。
type slgScreenState struct {
	name string
	// hover 是最近一次指针位置，Lua 的 update(mx,my) 每帧都要。
	pointerX int
	pointerY int
	// dirty 表示 Lua 侧要求重绘（例如动画/滚动）。
	dirty bool
}
