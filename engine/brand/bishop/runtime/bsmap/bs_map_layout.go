package bsmap

type BsMapLayout struct {
	// Background 是地图底图；Overlay/OverlayHover 是底图之上的路线标记层
	// （base on / base over）。三射面談把表示地点的图标烘焙在 map_on/map_over
	// 里，底图本身没有标记；Overlay 在角色卡之前绘制，不能当成普通角色层。
	Background   string
	Overlay      string
	OverlayHover string
	// FrameImage 是地图边框/DAY 卡片（map_frame），画在所有角色卡之上。
	// FrameX/FrameY 是它的屏幕落点：牝教師2 的 map_frame 是整屏图（0,0），
	// MKIE 的则是一块 240x104 的信息面板，必须按 [map phase] frame 的坐标贴。
	FrameImage string
	FrameX     int
	FrameY     int
	// FrameImage2 是第二块只画不点的边框/信息面板。放課後2 的地图同时需要
	// 右下角的系统菜单底板（map_menu_base）和左上角的周数/曜日卡
	// （map_info_base），单个 FrameImage 放不下。零值表示没有第二块，
	// 其它作品完全不受影响。
	FrameImage2 string
	Frame2X     int
	Frame2Y     int
	Items       []BsMapItem
	// Decor 是只画不点的装饰层（学園2 地图上的周数/星期/残数）。
	// 它不能放进 Items：beginMapSelect 会给每个 item 建热区，装饰图会把
	// 地图上的点击吞掉。渲染时按 DrawX/DrawY + Frame 逐张叠加。
	Decor  []BsMapItem
	Source string
	Mask   *BsMapMaskData
	// MenuImage/MenuX/MenuY/MenuRowHeight/MenuFrameWidth/MenuFrames 描述
	// Tbxdl 世代右侧的楼层列表图：整图里并排放着常态/悬停两列。
	// 渲染层只贴常态列，悬停时按行裁出悬停列叠在同一行上。
	MenuImage      string
	MenuX          int
	MenuY          int
	MenuRowHeight  int
	MenuFrameWidth int
	MenuFrames     int
	// CancelLabel 是右键/取消地图时应该跳回的标签。Tbxdl 的
	// "仕事に戻る" 是列表第 11 行，取消也应当回工作画面而不是标题。
	CancelLabel string
	// SystemButtons 是地图上的系统按钮（学園天堂的 config/log/status/qsave/qload/save/load）。
	// 它们不写回 #character/#place，点击只打开系统界面并保持选择态。零值表示不画，
	// 其他 profile 不填就不受影响。
	SystemButtons []BsMapSystemButton
	// ResultProperty/ResultPropertyKey 是地图选择要写回的结果变量（UK/UKIE 的 #result）。
	// Session 在打开地图时把它清零：脚本在 SELECT_TAIL 之后按 #result == 1..10 分派角色
	// 场景，上一张地图残留的值会被这次 SELECT_TAIL 重新消费。
	ResultProperty    string
	ResultPropertyKey int
	// TimeSlot/TimeValue/TimeSlotValid 是 map.dat 的时段槽。
	// map_time 图只有 4 帧，而脚本的 #time 是 4..7；直接画 #time-1 会越界，
	// 必须用 map.dat 的 TimeValues 把变量值换算成 0 基槽位。
	TimeSlot      int
	TimeValue     int
	TimeSlotValid bool
	// DayVariable 是地图上 DAY 卡片里的日期变量（#day）。
	DayVariable string
	// FixedBackground 表示 profile 已经按自己的规则选好底图/叠加层，
	// Session 不要再套 bsxx.cfg 的 base by time：牝教師2 的 map_lock1/2
	// 是"不可去地点"的整屏暗化图，按 base on 无条件叠上去会把整张地图
	// 染成紫色，而它的可用/不可用是按 map_hit 逐地点裁的。
	FixedBackground bool
	// NextValue 是"时间推进/取消"按钮写回结果变量的值。
	// UK/UKIE 的脚本把 0 当"谁也没见"，牝教師2 的两代则把 9999 当
	// 「時間を進める」；零值保持旧行为，不影响已经适配的作品。
	NextValue int
	// AllowEmpty 表示"没有角色卡也要打开地图"（UK/UKIE 的时段地图）。
	// 原版在所有人都已被拜访过时仍显示底图与「時間を進める」按钮；如果这里
	// 直接返回空布局，Session 会跳过 SELECT_TAIL，脚本拿着上一张图的 #result
	// 继续分派，可能重放已经看过的角色场景。
	AllowEmpty bool
}

// BsMapSystemButton 是地图上一颗只触发系统功能、不改地图结果的按钮。
// Tool 是剥掉 map_ 前缀后的功能名，供 session 侧 customMapToolKey 分派。
type BsMapSystemButton struct {
	Image  string
	X      int
	Y      int
	Width  int
	Height int
	Tool   string
	// Hit 是纯热区按钮的屏幕矩形（X, Y, 宽, 高）。MDDVD 的 Q.SAVE/LOAD
	// 等六颗按钮已经烤在 map_base 里，没有独立按钮图；只登记热区即可。
	// 为 (0,0,0,0) 时保持旧行为，按 Image 的实际尺寸命中。
	Hit [4]int
	// Frames 是竖排帧数；0 表示沿用旧的三帧约定（学園天堂）。
	Frames int
	// Frame 是固定绘制的帧号；-1 表示沿用悬停切帧。
	// 学園（GK）的 map_submenu_button 把 6 个按钮的标签竖排在同一张图里，
	// 每个按钮必须固定画自己那一帧，不能悬停时跳到第 1 帧。
	Frame int
}

// BsMapGkSource 是学園（GK/GKDVD/GKWindows10）的内置地图。
// 这一代的地图不是 MapPrms 的通用左栏列表：12 个地点按钮固定在右栏
// x=620、y=116+38i，顶部还有常驻工具条/日期卡/目标头像，由 Session 的
// drawGkMapOverlay 补画动态部分。
const BsMapGkSource = "gk-map"

var BsMapLayoutEmpty = BsMapLayout{Background: "", Items: []BsMapItem{}, Source: "none", TimeSlot: -1}

func (l BsMapLayout) IsEmpty() bool {
	return len(l.Items) == 0
}

func (l BsMapLayout) HasMask() bool {
	return l.Mask != nil && l.Mask.Width > 0 && l.Mask.Height > 0
}
