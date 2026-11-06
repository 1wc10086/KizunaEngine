package config

import (
	"fmt"
	"strconv"
	"strings"

	"kizuna/engine/core/strutil"
)

// ui_layout.go 解析原引擎"系统界面"的布局：存档/读档画面、游戏内菜单条、
// 鉴赏室（CG / 场景 / BGM）以及 CG 查看器。
//
// 这些界面在原引擎里不是脚本画出来的，而是由 bsxx.cfg 的段驱动：每个界面给出
// 底图、按钮图、帧数与坐标，引擎按这些数据合成并对点击做命中判定。
// 我们沿用同一份配置而不是把坐标写进 Go 代码，换作品时只换素材即可。
//
// 坐标一律保持在"游戏坐标系"（本作 1024x768）里；宿主只看到最终帧，
// 不需要知道这些数字，所以解析层不做缩放。

const (
	DataScreenSection   = "data screen"
	GameMenuSection     = "game menu"
	SystemButtonSection = "system button"
	MuseumSection       = "museum phase"
	MuseumCgSection     = "museum phase - cg"
	MuseumSceneSection  = "museum phase - scene"
	MuseumMusicSection  = "museum phase - bgm"
	// ScenePlaylistSection 是鉴赏室里"播放列表"子画面
	// （[scene playlist]）的布局：42 个槽位的缩略图网格 + 翻页 + 播放/关闭。
	ScenePlaylistSection = "scene playlist"
	CgViewerSection      = "cg viewer"
	SlotViewerSection    = "slot viewer"
	LogScreenSection     = "log screen"
	// StatusScreenSection 是"状態"画面：只给一个 layout 资源名，
	// 具体页面/坐标/显示条件都写在 define.bsa 的布局文件里。
	StatusScreenSection = "status screen"
)

// UIPoint 是配置里的一个屏幕坐标。
type UIPoint struct {
	X int
	Y int
}

// UIButton 是一张"多帧按钮图"（竖排帧）及其左上角坐标。
//
// 原引擎把常态/悬停/按下/不可用画进同一张竖排图，按状态取帧贴图；
// Frames 来自配置而不是图片高度，因为解码器会把整张竖排图当成单帧图像。
type UIButton struct {
	Image  string
	Frames int
	X      int
	Y      int
	// Part 是复合图（BSS-Composition）里的部件号，1 基；0 表示整张图。
	// 旧式作品的页签/按钮常常没有独立图片，只存在于一张 museum_menu 之类的
	// 合成图里；配置用 `image#3` 选中第 3 个部件。
	Part int
	// Start 是绘制时的起始帧。像 museum_num 这种"一张图里排了 12 个数字"的
	// 数字条，用 `image@5` 表示这一项固定画第 5 帧（页码 5）。
	Start int
	// FixedFrame 表示永远画 Start 指定的那一帧，不随悬停/选中切换。
	// 页码数字条/复合条按钮用它：第 3 页就只显示数字 3。
	FixedFrame bool
	// Clip 是从按钮图里裁出的源矩形（X, Y, 宽, 高），零宽/零高表示整张图。
	// 旧式作品的 save_load 把 SAVE/LOAD 两个页签画在同一张 206x36 图里，
	// 只有用源矩形才能让它们各自成为一个按钮，同时保留悬停帧。
	Clip [4]int
	// Hit 是命中矩形（绝对屏幕坐标：X, Y, 宽, 高），零宽/零高表示按图片矩形命中。
	//
	// 有些按钮图是一整条多状态竖排图（museum_menu 的 15 帧、cfg_button 的 6 帧），
	// 图片矩形覆盖整列/整块，不能直接当热区；配置里给的热区（如博物馆页签的
	// 202x36 条）往往比图片窄/高，必须单独描述。见 session.hitButton。
	Hit [4]int
	// FrameOffset/FrameStep 把"状态帧"映射到复合图的某一行：
	// 实际绘制帧 = FrameOffset + 状态帧*FrameStep。museum_menu 是状态优先
	// 排列（0..4 常态、5..9 悬停、10..14 选中），页签与返回键都嵌在同一个
	// UIButton 上，所以字段放在这里而不是 MuseumMode；零值沿用原来的
	// "状态帧 = 帧号"行为。
	FrameOffset int
	FrameStep   int
	// Atlas 表示 Clip 描述的是一张精灵图（sprite sheet）里的单帧矩形，
	// 而不是整张多帧竖排图。BSXScript 1.0 / 満淫電車2 这一代把标题菜单、
	// 存档页、设置页的全部控件烘在一张大图里，配置写的是
	// `0, 600, 250, 46`（源矩形）+ `patterns = 0, 2`（排列方向与帧数）+
	// `pos = 525, 184`（屏幕落点）。没有这个标记时 Clip 的含义仍是旧式
	// "从竖排图里裁一段"；打开时必须走下面的 FrameRect。
	Atlas bool
	// AtlasAxis 是间距图里多帧的排列方向：0 纵向、1 横向。
	// 配置的 `patterns = 方向, 数量`，方向 0 是纵向、1 是横向。
	AtlasAxis int
	// HoverClip 是悬停态的源矩形。多数精灵图把常态/悬停放在同一张图里
	// 但不相邻（存档页的页码按钮每页有独立的悬停块），只靠 Frame 无法
	// 从常态矩形推出悬停矩形，所以单独保存。
	HoverClip [4]int
}

// Empty 表示这一项没有被配置（不同作品可能没有某个按钮）。
func (b UIButton) Empty() bool {
	return len(b.Image) == 0
}

// FrameHeight 返回按钮单帧的高度；只有配置了帧数才可能切帧。
func (b UIButton) FrameHeight(height int) int {
	frames := b.Frames
	if frames <= 1 || height <= 0 {
		return height
	}
	return height / frames
}

// FrameRect 返回精灵图里第 frame 帧的源矩形。
//
// Atlas 关闭时沿用旧的"整图竖排"语义：整宽、按帧数等分高度；调用方仍用
// image.Height/Frames 自己算。只有 Atlas 打开时才使用 Clip + Axis，因为
// 这一代的 Clip[3] 是单帧高度，不是整张条带的高度。
func (b UIButton) FrameRect(imageWidth, imageHeight, frame int) (x, y, width, height int) {
	if !b.Atlas || b.Clip[2] <= 0 || b.Clip[3] <= 0 {
		return 0, 0, 0, 0
	}
	frames := max(1, b.Frames)
	frame = clampIntLocal(frame, 0, frames-1)
	x, y = b.Clip[0], b.Clip[1]
	width, height = b.Clip[2], b.Clip[3]
	if b.AtlasAxis == 1 {
		x += frame * width
	} else {
		y += frame * height
	}
	if x < 0 || y < 0 || x+width > imageWidth || y+height > imageHeight {
		return 0, 0, 0, 0
	}
	return x, y, width, height
}

// clampIntLocal 是 config 包内部的整数夹取；session 包里有同名但不可见的
// 实现，这里不跨包复用，避免把渲染细节泄漏到配置层。
func clampIntLocal(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// parsePatterns 解析 `方向, 数量`。方向 0 纵向、1 横向；缺省按纵向 1 帧。
func parsePatterns(value string) (axis, frames int) {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) == 0 {
		return 0, 1
	}
	if len(parts) >= 1 {
		if value, ok := parseInt(parts[0]); ok && value == 1 {
			axis = 1
		}
	}
	frames = 1
	if len(parts) >= 2 {
		if value, ok := parseInt(parts[1]); ok && value > 0 {
			frames = value
		}
	}
	return axis, frames
}

// parseAtlasButton 解析精灵图按钮。value 是 `x, y, w, h`，配套键是
// `<key> pos`（屏幕坐标）与 `<key> patterns`（方向、帧数）。
//
// 返回 ok=false 表示这不是精灵图写法，调用方应退回 parseUIButton。
func parseAtlasButton(document *IniDocument, section, key, atlasImage string) (UIButton, bool) {
	value := document.Get(section, key, "")
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) < 4 {
		return UIButton{}, false
	}
	rect := [4]int{}
	for index := 0; index < 4; index++ {
		number, ok := parseInt(parts[index])
		if !ok {
			return UIButton{}, false
		}
		rect[index] = parseCoordinate(parts[index])
		if number < 0 {
			return UIButton{}, false
		}
	}
	x, y := parsePoint(document.Get(section, key+" pos", ""))
	axis, frames := parsePatterns(document.Get(section, key+" patterns", ""))
	button := UIButton{
		Image:     atlasImage,
		Frames:    max(1, frames),
		X:         x,
		Y:         y,
		Clip:      rect,
		Atlas:     true,
		AtlasAxis: axis,
	}
	// 显式热区（这一代偶尔把热区写得比画面大/小）。
	if hit := ParsePositions(document.Get(section, key+" hit", "")); len(hit) >= 4 {
		copy(button.Hit[:], hit[:4])
	}
	return button, true
}

// atlasButtonOr 先尝试精灵图写法，失败再退回旧式 `image, frames, x, y`。
func atlasButtonOr(document *IniDocument, section, key, atlasImage string) UIButton {
	if button, ok := parseAtlasButton(document, section, key, atlasImage); ok {
		return button
	}
	return parseUIButton(document.Get(section, key, ""))
}

// DataScreen 是 [data screen]（存档 / 读档画面）的布局。
type DataScreen struct {
	SaveBase string
	LoadBase string
	// Backdrop 是整屏背景，画在 SaveBase/LoadBase 下面。零值保持旧行为：
	// 不画这层，只画底图。某些 2005 年的半全屏底图需要它垫白/垫黑。
	Backdrop string
	// MenuSave/MenuLoad 是存档/读档时的左侧菜单覆盖图（201x600），画在底图与
	// 页签/槽位装饰之后、交互按钮之前。零值保持旧行为：不叠这层菜单。
	MenuSave string
	MenuLoad string
	// MenuConfig 是设置画面用的左侧菜单覆盖图；存档画面只用 MenuSave/MenuLoad，
	// 这一字段供会话层在配置画面复用同一套菜单时读取。零值不叠图。
	MenuConfig string
	HitTable   string
	// NoBegin 是画面上显示的槽位起始编号：原引擎按编号给玩家看，
	// 我们内部仍用 0 基下标。
	NoBegin int
	// PageMax 是页数，SlotsPerPage 是每页槽位数。
	PageMax      int
	SlotsPerPage int
	// AutoOnly 是"オートセーブ専用"占位图。
	AutoOnly string
	// NoData 是空格子的"NO DATA"占位图（学園2 的 dm_nodata）。
	NoData string
	// AutoOnlyButton/NoDataButton 是精灵图世代的同名占位图：配置给的是
	// 源矩形而不是图片名，旧字段保持字符串以免破坏已有作品。
	AutoOnlyButton UIButton
	NoDataButton   UIButton
	// Slots 是每页各槽位的左上角坐标。
	Slots []UIPoint
	// PageButtons 是页选择按钮（通常是数字 1..N）。
	PageButtons []UIButton
	// PageHits 是"页码画在底图里"时的纯热区（旧式作品 KD 的 Page Jump）。
	// 它和 PageButtons 二选一：有独立按钮图时用按钮图，没有时按坐标命中。
	PageHits []UIPoint
	// PageNumbers 是旧式作品（学園2）的页码数字条：dm_page 把 10 个页码
	// 的「常态 / 当前 / 悬停」三种状态竖排在 33 帧里，引擎按页下标取帧。
	// PageNumberPos 是每页数字的左上角；PageHits 仍然负责命中。
	PageNumbers   UIButton
	PageNumberPos []UIPoint
	// PageNumberStart 是页码数字条的取帧起点：页码 i 画第 i+PageNumberStart 帧。
	// 旧作的数字条常把 0..9 竖排（牝教師 MK 的 dm_number 10 帧），页码 1 对应
	// 第 1 帧；学園2 的 dm_page 是三组各 11 帧（0/11/22 起），起点仍为 0。
	// 零值保持原来的"页码 = 帧号"行为。
	PageNumberStart int
	// SortButtons 是"按编号/新到旧/旧到新排序"三个按钮的坐标。
	SortButtons []UIPoint
	// SelectFrame 是鼠标悬停时套在槽位外的边框。
	SelectFrame UIButton
	// SaveMode / LoadMode 是底部"存档/读档"页签。
	SaveMode UIButton
	LoadMode UIButton
	// Config 是旧式作品在存档页右下角提供的"设置"入口。
	// KD 的 cfg_mode_config 就在这一列，和 save/load 并排。
	Config UIButton
	// Item 是"SYSEM 菜单"第四个页签（持ち物）。MK 的 cfg_title 第 4 帧
	// 写着 ITEM，点击后打开 ItemScreen。
	Item UIButton
	// Back / Title / Exit 是右上角的返回、回标题、退出。
	Back  UIButton
	Title UIButton
	Exit  UIButton
	// Confirm 是覆盖已有存档时的确认窗口（[slot viewer]）。
	Confirm SlotViewer

	// 以下是 Tbxdl 世代（特別病棟 旧版/Win10）的槽位样式。
	// 该世代没有整屏底图，每个槽位是一张 380x160 的圆角框（SaveFrame），
	// 框的左侧写字号/日期、右侧贴 160x120 缩略图。
	// 这些字段为零时不影响其它作品原有的 [data screen] 行为。
	SlotFrame       string
	SlotFrameFrames int
	// SlotBase 是槽位框下面的底板（ヒメドレイ 的 dm_slotbase：金色织纹）。
	// dm_slot 的边框中心透明，空格子没有缩略图时露出的就是这张底板。
	SlotBase string
	// SlotFrameAuto 是第一页第一个槽位（オートセーブ）专用的框图。
	SlotFrameAuto string
	// SlotFrameHover / SlotFrameSelected 是悬停/当前槽位使用的帧号。
	SlotFrameHover    int
	SlotFrameSelected int
	// SlotSize 是槽位框的命中尺寸；零值退回 ThumbnailRatio 的缩略图尺寸。
	SlotSize [2]int
	// SlotThumb 是缩略图相对槽位左上角的矩形（X, Y, 宽, 高）。
	SlotThumb [4]int
	// SlotTextX / SlotTextY / SlotTextLineGap 是槽位文字相对坐标。
	SlotTextX       int
	SlotTextY       int
	SlotTextLineGap int
	SlotTextColor   uint32
	// SlotNumberPrefix/SlotNumberWidth 控制槽号文字：空/0 时沿用旧的
	// "0." 格式；GK 需要 "No. 000" 这种固定三位编号。
	SlotNumberPrefix string
	SlotNumberWidth  int
	// AutoSlotIndex 是"オートセーブ専用"槽位的页内下标；-1 表示没有。
	AutoSlotIndex int
	// AutoSlotGrid 表示这个格子直接显示内部的 AutoSlot（save.AutoSlot），
	// 而不是网格里的普通存档。学園（GK）第 0 页左上角就是自动存档格。
	AutoSlotGrid bool
	// SlotFrameGeneric 强制走通用槽位绘制（drawAncientDataSlot），即使
	// SlotFrame 恰好叫 dm_slot。学園（GK）的 dm_slot 是 320x160 的三态
	// 白框，和学園2 的 170x130 小框不是同一套几何；不打开时旧作品保持
	// 原有的"dm_slot = 学園2 专用绘制"行为。
	SlotFrameGeneric bool
	// SlotFrameOpaque 表示槽位框的 alpha 全 0、形状写在 RGB 里（GK 的
	// dm_slot 白色圆角框），必须忽略 alpha 整块画。
	SlotFrameOpaque bool
	// SlotEmptyFrame 是空格子的框帧号；-1 表示沿用常态帧。
	// 学園（GK）的 dm_slot 第 2 帧是灰框，用来表示没有存档。
	SlotEmptyFrame int
	// PagePrevPoint/PageNextPoint 是"Page No."上下的翻页箭头热区
	// （只有热区，图已烘焙在 save_base 里）。零值表示没有。
	PagePrevPoint UIPoint
	PageNextPoint UIPoint
	// PageNumber 是"当前页"数字条（GK 的 dm_pagenum：0..9 各一帧）。
	// PageNumberX/Y 是两位数页码的左上角，PageNumberGap 是两位之间的间距。
	PageNumber    UIButton
	PageNumberX   int
	PageNumberY   int
	PageNumberGap int
	// ShortcutAuto/ShortcutQuick 是存档页右下的 Auto/Quick 快捷读档热区。
	// 图已经烘焙在 save_base 上，这里只登记命中矩形（屏幕坐标）。
	ShortcutAutoPoint  UIPoint
	ShortcutQuickPoint UIPoint
	// PageSelectFrame 是页选择条底图（PageSelectFrame，432x42）。
	PageSelectFrame UIButton
	// ModeLabelBackdrop 为真时，在 SaveMode/LoadMode/Config 三块标签后面
	// 铺一条深色底。MK 的 cfg_title 是白字、底图 config_base 是白色，
	// 不垫底时"SAVE/LOAD/CONFIG"几乎看不见；其它作品的页签自带底色，
	// 保持 false。
	ModeLabelBackdrop bool
	// TitleLabel 是左上角 plaque 里的大号模式名（放課後2 的 cfg_title 是
	// 300x180 的三帧：SAVE/LOAD/CONFIG）。按当前模式固定画一帧；
	// 留空时 plaque 保持底图原样。
	TitleLabel UIButton
}

// ItemScreen 是旧式系统菜单的「持ち物」页（MK 的 SYSTEM → ITEM）。
//
// 这一页没有独立的 cfg：底图 cfg_item_base 上烘着五名角色的头像/名字，
// 右侧留白处由引擎按 #itemNN 属性画 cfg_item 的 14 个道具图标。
type ItemScreen struct {
	// MK2 表示牝教師2 老版：底图 cfg_item_base 烘着 4 名角色 x 3 组空槽，
	// cfg_item 的 3 个部件分别是"左上/左下/右侧大格"的 12 帧图标条，
	// 持有状态是 #yuk_item1..#skm_item3 的位掩码。绘制见
	// systemui_item.go 的 MK2 分支。
	MK2   bool
	Base  string
	BaseX int
	BaseY int
	// Icon 是 15 帧的 64x64 道具图（第 0..13 帧是道具，最后一帧是 X）。
	Icon       string
	IconFrames int
	// Slots 是道具图标的左上角；与持有状态一一对应，未持有的不画。
	Slots []UIPoint
	// Selected 是可选的选中框（dm_item 56x80）。
	Selected UIButton
	Back     UIButton
	Title    UIButton
	Exit     UIButton
}

// Empty 判断作品有没有这一页。
func (i ItemScreen) Empty() bool {
	return !i.MK2 && i.Base == "" && i.Icon == ""
}

// SlotViewer 是存档槽确认/详情窗口的布局。
type SlotViewer struct {
	Base string
	// BaseX/BaseY 是确认窗口底图的屏幕落点。零值保持旧行为（左上角），
	// 放課後2 的 dm_slotviewer 是 464x208 的居中窗口，必须按 (168,196) 贴。
	BaseX  int
	BaseY  int
	OK     UIButton
	Cancel UIButton
	// 以下是存档确认窗口里的槽位详情：槽号、保存日期/时间、缩略图、
	// 正文预览、游戏内日期、场景名。全部来自 [slot viewer]。
	NoHeader        UIButton
	Numbers         UIButton
	TimeZone        UIButton
	TimeZoneNumbers int
	Thumbnail       UIPoint
	TextArea        [4]int
	TextColor       uint32
	FontSize        [3]int
	FontEdge        int
	SlotNoX         int
	SlotNoY         int
	SlotNoOffset    int
	DateX           int
	DateY           int
	DateOffset      int
	TimeX           int
	TimeY           int
	TimeOffset      int
	DayStrip        UIButton
	DayUnit         UIButton
	DaySpecial      UIButton
	DayNoOffset     int
	DayNoMax        int
	SceneNameSize   int
	SceneNameColor  uint32
	SceneNameEdge   uint32
	SceneNameX      int
	SceneNameY      int
	SceneNameAlign  int
	TimeInGame      UIPoint
	// 六名女主角攻略度条（dm_meter）。MeterImage 为空表示没有这条 UI。
	// 第 i 行的条位于 MeterRect 的 y 加上 i*MeterGap，角色名画在条上方 MeterNameY 处。
	MeterImage      string
	MeterFrames     int
	MeterRect       [4]int
	MeterGap        int
	MeterNameY      int
	MeterNames      [6]string
	MeterProperties [6]string
	MeterMax        int
	MeterTrackFrame int
	MeterFillFrame  int
	MeterNameSize   int
	MeterNameColor  uint32
}

// Empty 判断这个作品是否配了存档画面。
// Tbxdl 世代没有整屏底图，只要配了槽位框就算有存档画面。
func (d DataScreen) Empty() bool {
	return d.SaveBase == "" && d.LoadBase == "" && d.SlotFrame == ""
}

// MenuButton 是游戏内菜单条上的一个按钮。
type MenuButton struct {
	// Key 与 [system button] 的键名一致：save/load/qsave/qload/config/log…
	Key string
	UIButton
	// Width/Height 只给没有图片的热区按钮用（Ancient 工具条），有图片时以图片尺寸为准。
	Width  int
	Height int
	// Persistent 表示这个按钮属于常驻工具条（学園2 顶部的 Q.SAVE…QUIT）。
	// 对话框收起时只画这些按钮，对话框里的 SKIP/AUTO/窗口钮不会飘到地图上。
	Persistent bool
}

// GameMenu 是 [game menu] + [system button] 的布局。
type GameMenu struct {
	Base string
	BaseX int
	BaseY int
	// BaseFill 是「gamemenu_base 只有白色描边、需要垫一层底色」的标记。
	// 只有 BSXScript 1.0（満淫電車2）走内置按钮分支时置真；侵蝕/淫獄 等
	// 也使用 gamemenu_base，但它们自带底色，不能垫，否则会在正文上方
	// 出现一条 800x44 的紫色横条。
	BaseFill   bool
	ShowArea   [4]int
	AutoHideMs int
	Buttons    []MenuButton
	// Overlay 为真时菜单底图画在对话框底图之上（Ancient 的 msgwnd_b 叠在 message 上）。
	Overlay bool
	// Persistent 表示这条菜单条在对话框收起来时仍然常驻（学園2 的顶部
	// Q.SAVE…QUIT 工具条在剧情、地图、存档画面都可见）。其它作品的
	// 工具条跟随对话框显示，保持默认 false 不受影响。
	Persistent bool
	// AutoExIcon/CutInIcon/HoldIcon 是底栏上的开关指示灯（switch_lamp）。
	// 原作把蓝色下划线叠在 AUTO/CUT-IN/HOLD 按钮下方，表示该功能开启；
	// 按钮图本身不带状态，不画灯玩家就分不清开/关。图标坐标相对 [game menu]
	// base pos，`sync button pos = 1` 的作品与按钮使用同一原点。
	AutoExIcon UIButton
	CutInIcon  UIButton
	HoldIcon   UIButton
}

// Empty 判断作品是否配了游戏内菜单条。
func (g GameMenu) Empty() bool {
	return g.Base == "" || len(g.Buttons) == 0
}

// MuseumMode 是鉴赏室顶部的一个模式页签。
type MuseumMode struct {
	Key   string
	State uint32
	// ActiveFrame 是选中页签画哪一帧；0 表示沿用旧行为（第 2 帧）。
	// AFDVD 的 museum_menu 第 2 帧是黑字（压在深色木板上几乎看不见），
	// 原作实际高亮用第 1 帧的蓝色字，所以单独给作品留一个覆盖点。
	ActiveFrame int
	UIButton
}

// Museum 是鉴赏室主界面与各子页面的布局。
type Museum struct {
	Base string
	// DefaultMode 是打开鉴赏室时默认进入的子页（如学園 GK 的 "music"）。
	// 空字符串沿用旧行为：停在主菜单（页签 + 底图）。这个模式可以没有
	// 页签按钮，只作为初始页面存在。
	DefaultMode string
	// StripPortraits 是旧式立绘鉴赏页的整身立绘部件（AFDVD 的 museum_chs）。
	// 有值时 "strip" 页签直接画这些部件，不再走 buex/KD 的换装分支。
	StripPortraits []UIButton
	// MK2Strip 表示牝教師2 老版的立ち絵鉴赏：museum_chs 的 7 张角色卡
	// 是选择页，点卡后进入 bu*/gb* 的组合查看器。交互在
	// systemui_mk2_strip.go 里单独实现（素材坐标/命名都与 AFFDVD 不同）。
	MK2Strip bool
	// MKStrip 表示牝教師初代（MK）的立ち絵鉴赏：museum_ch1..6 是六张
	// 72x528 的整身角色条，点开后用 bum_base 的控制面板浏览 cs 合成立绘
	// （距離/服装/表情/视点/背景/ポーズ）。交互在 systemui_mk_strip.go。
	MKStrip bool
	// StripBackdrop 是角色卡后面的装饰底板（museum_chs 的第 1 个部件）。
	StripBackdrop UIButton
	// Ancient 表示这是 Tbxdl 世代（特別病棟）的内置鉴赏室。
	// 该世代的 CG/Scene 页是固定的 5x4 缩略图网格加右上角翻页按钮，
	// 和 bsxx.cfg 的 [museum phase] 布局模型不同，渲染器需要走专用分支。
	Ancient bool
	// Gakuen2 表示这是学園2（BSScript Rev.2）的内置鉴赏室。
	// 这一代没有 [museum phase] 坐标：组页签是角色剪影（cg_hit 的像素值），
	// 缩略图是 cgN_thumb/sceneN_thumb 竖排条，音乐页的曲名已经烘焙在底图里。
	// 坐标固定在 session/systemui_museum_gk2.go，因为只服务这一部作品。
	Gakuen2 bool
	// MultiBu 对应 [museum phase] multi bu：立ち絵鑑賞一次显示多个角色，
	// 每个角色有自己的显示开关（表示切り替え）与立ち位置，
	// 重なりボタン调整立绘之间的前后层序。
	//
	// 牝教師3 是这一语义，原版手册的截图里同屏站着多名角色；
	// 新版 museum_buex_layout 不写这一项，仍是单角色展示。
	MultiBu bool
	// Modes 是 music/bgv/cg/scene/strip 五个模式页签。
	Modes []MuseumMode
	// Return 是"返回标题"按钮。
	Return UIButton
	CG     MuseumCGPage
	Scene  MuseumScenePage
	Music  MuseumMusicPage
	Viewer CGViewer
	// Bu / BuViewer 是旧式立绘鉴赏（[museum phase - bu] + [bu viewer]）。
	//
	// 新版作品（3SLG/侵蝕/淫獄）把立绘鉴赏写成 define.bsa 里的
	// museum_buex_layout；牝教師3 和特別授業3SLG 旧版没有那个资源，
	// 而是用 bsxx.cfg 里这两段描述"角色选择页 + 换装查看器"。
	// 牝教師3 再额外打开 MultiBu：查看器本身就是多角色同屏的完整界面。
	Bu       MuseumBUPage
	BuViewer BuViewer
}

// Empty 判断作品有没有鉴赏室。
func (m Museum) Empty() bool {
	if m.Gakuen2 {
		// 学園2 老版的底图名/坐标全部内置在渲染器里，配置结构体本身是空的；
		// 不把 Gakuen2 算作有效布局的话，标题的三个鉴赏入口会全部打不开。
		return false
	}
	return m.Base == "" && m.CG.Base == "" && m.Scene.Base == ""
}

// MuseumCGPage 是 [museum phase - cg] 的布局。
type MuseumCGPage struct {
	Base string
	// Bases 是"每组一张整屏底图"的旧作（MDDVD 的 cg0_base..cg4_base）：
	// 每个分组的底图分别画着该组的分页高亮和装饰，不能只画 Base 一张。
	// 非空时按当前 cgGroup 取对应底图；为空时保持只画 Base 的旧行为。
	Bases     []string
	UnderBase string
	// PageBase 是缩略图下方"達成率/ページ選択"标签条（GK 的 museum_pageline）。
	PageBase UIButton
	HitTable string
	// PageButtons 是左侧的角色/分组页签；点击切换当前组。
	PageButtons []UIButton
	// Thumbs 是每页最多 36 个缩略图的左上角坐标。
	Thumbs []UIPoint
	// ThumbStrips 是"每个页签一张竖排缩略图条"的素材名（KD 的 cg0_thumb…）。
	// 有值时直接切条里的帧，不再从完整 CG 合成缩略图——旧作的完整 CG
	// 很大，逐格合成会明显卡顿，且原版本来就用预制缩略图。
	ThumbStrips []string
	// ThumbX/ThumbY 是第一个格子的原点，ThumbW/ThumbH 是格子的实际尺寸。
	// 这两个值由连续的 thumbN pos 差值算出：不同作品的格子是 224x123、
	// 207x127 等，写死 126x96 会让缩略图只占格子的一半。
	ThumbX int
	ThumbY int
	ThumbW int
	ThumbH int
	// Percent 是"达成率"数字条：图片、帧数、坐标与字距。
	Percent      UIButton
	PercentSpace int
	// Parts 是缩略图编号数字条。
	Parts      UIButton
	PartsSpace int
	PartsX     int
	PartsY     int
	// 以下是 Tbxdl 世代的固定网格与右上角翻页按钮。
	Rows, Columns int
	NavPrev       UIButton
	NavTop        UIButton
	NavNext       UIButton
	NavBack       UIButton
	ThumbFrame    UIButton
	Config        UIButton
	// 以下是 BSXScript 1.0 精灵图世代（満淫電車2）的装饰与缩略图框。
	// Title/Menu 是标题条与左栏底板；ThumbFrames 是每格缩略图的花纹框，
	// ThumbOffset 是缩略图相对框左上角的偏移（配置 thumb = 2,2）。
	Title       UIButton
	Menu        UIButton
	ThumbFrames []UIButton
	ThumbOffset UIPoint
	// ThumbInfo 是缩略图上「已解锁/总数」的小数字条，ThumbInfoSpace 是字距。
	ThumbInfo      UIButton
	ThumbInfoSpace int
	// Atlas 表示 Base 是精灵图而不是整屏底图：不能整张画出来，
	// 只画配置里的各个矩形。満淫電車2 的 museum_cg/museum_scene 都是
	// 875x376 的拼图，直接铺开会在屏幕上出现一整张素材表。
	Atlas bool
}

// MuseumScenePage 是 [museum phase - scene] 的布局。
type MuseumScenePage struct {
	Base string
	// Bases 同 MuseumCGPage.Bases：每个分页一张整屏底图（MDDVD 的
	// sc0_base / sc1_base），按当前 ui.scenePage 取图。
	Bases     []string
	UnderBase string
	// PageBase 同 CG 页：museum_pageline 的标签条。
	PageBase     UIButton
	HitTable     string
	PageButtons  []UIButton
	Thumbs       []UIPoint
	ThumbStrips  []string
	ThumbX       int
	ThumbY       int
	ThumbW       int
	ThumbH       int
	Percent      UIButton
	PercentSpace int
	// PlayNow / AddPlaylist 是场景页顶部的两个"选择模式"按钮：
	// 点缩略图是立即播放，还是加入播放列表。
	PlayNow     UIButton
	AddPlaylist UIButton
	// PlaylistButton 打开播放列表子画面。
	PlaylistButton UIButton
	Playlist       ScenePlaylist
	// ClickMode 是旧式场景页的"选择模式"指示图（8x8），
	// ClickModePoints 是它的两个落点：0 = そのまま再生，1 = 再生リストへ追加。
	// 新版把这些写成 play now / add playlist 两个按钮，旧版只画一个小标记。
	ClickMode       UIButton
	ClickModePoints []UIPoint
	// 以下是 Tbxdl 世代的固定网格与右上角翻页按钮（与 CG 页同构）。
	Rows, Columns int
	NavPrev       UIButton
	NavTop        UIButton
	NavNext       UIButton
	NavBack       UIButton
	ThumbFrame    UIButton
	Config        UIButton
	// グラフィック精灵图世代（満淫電車2）的页面装饰与缩略图框。
	Title          UIButton
	Menu           UIButton
	ThumbFrames    []UIButton
	ThumbOffset    UIPoint
	ThumbInfo      UIButton
	ThumbInfoSpace int
	Atlas          bool
}

// ScenePlaylist 是 [scene playlist] 的布局。
//
// 播放列表是玩家自己挑的场景集合：7 列 × 6 行 = 42 格，最多 10 页。
// 缩略图坐标不是逐格写在配置里的，而是给一个基准 + 行列数，按格算出来。
type ScenePlaylist struct {
	Base      string
	HitTable  string
	ScrollBar UIButton
	Play      UIButton
	Close     UIButton
	// ThumbX/Y/W/H 是网格第一格的左上角和单格尺寸。
	ThumbX int
	ThumbY int
	ThumbW int
	ThumbH int
	// Columns 是每行几格；SlotMax 是每页最多几格。
	Columns int
	SlotMax int
	// PageButtons 是底部的数字页签。
	PageButtons []UIButton
}

// MuseumMusicPage 是 [museum phase - bgm] 的布局。
type MuseumMusicPage struct {
	Base     string
	HitTable string
	// PageBase 是可选的分页/乐谱底板部件（AFDVD 的 museum_titlelist 第 1 个部件）。
	// 它不是整张复合图：直接画整图会把 20 首曲名重复叠一遍。
	PageBase UIButton
	Play     UIButton
	Stop     UIButton
	Prev     UIButton
	Next     UIButton
	Max      UIButton
	Min      UIButton
	// Volume 是音量滑块的图（KD 的 museum_bgmctrl 第 8 个部件）。
	// 滑块的实际 Y 由 VolumeTrackHeight 和当前 BGM 音量算出，不能直接用
	// 图里的 OffsetY：合成图部件坐标是“初始摆放位置”，不是运行时状态。
	Volume UIButton
	// VolumeKnob 表示 Volume 是“滑块”而不是整条音量条。
	// 新版作品（侵蝕/淫獄/KDIE）的 volume 键给的是完整横条，只有 KD 是滑块。
	// 音量滑块的绘制方式。KD 是竖向滑块（VolumeKnob），AFDVD 是底部一条
	// 横向电平条；后者按当前 BGM 音量截取条宽，视觉上与原作一致。
	VolumeKnob bool
	// VolumeFill 表示 Volume 是一条横向电平条：按 BGM 音量截取左侧一段。
	VolumeFill bool
	// ControlBase 是左侧控制盒的底图（KD 的 museum_bgmctrl 第 1 个部件）。
	// 它包含四个按钮的空槽和竖向音量轨道；旧作没有这个底图时留空。
	ControlBase UIButton
	// VolumeTrackHeight 是滑块从最大音量走到静音的可移动像素。
	VolumeTrackHeight int
	// VolumeRegion 是音量轨道的可点击区域：x、y、宽、高。
	// 没有单独 region 时用 x/y + 宽高推算。
	VolumeRegion [4]int
	Tracks       []UIButton
	Names        []string
	// BakedTracks 表示曲名已经烘焙在 PageBase/底图里，Tracks 只用于命中，
	// 不能逐条绘制（否则整张列表会全部画成高亮色）。绘制时只补画
	// hover/current 的 TrackSelect。
	BakedTracks bool
	// Tbxdl 世代：曲目名已经烘焙在 music_base 里，TrackSelect 是
	// 270x484 的竖排高亮条（22 帧 x 270x22），TrackCount 是曲目数。
	TrackSelect UIButton
	TrackCount  int
	// 精灵图世代（満淫電車2）的标题条、左栏底板与控制盒花纹。
	Title       UIButton
	Menu        UIButton
	ButtonFrame UIButton
	VolumeFrame UIButton
	// Atlas 表示 Base 是精灵图，只画配置矩形。
	Atlas bool
}

// CGViewer 是 [cg viewer] 的布局。
type CGViewer struct {
	Base   UIButton
	PanelX int
	PanelY int
	Prev   UIButton
	Next   UIButton
	Exit   UIButton
	// Config 是底栏上的「CFG」按钮（KD 用）。点它打开设置画面，
	// 关闭后应回到 CG 查看器而不是标题/剧情。
	Config UIButton
	Number UIButton
	// ButtonBase/HoverOffset 是底栏按钮的帧约定。
	//
	// 新版作品（3SLG/侵蝕/淫獄）的按钮图第 0 帧是命中掩码，常态从第 1 帧
	// 开始、悬停 +1；KD 的 cgm_btn 是 10 帧竖排：0..4 常态、5..9 红色悬停，
	// 所以 ButtonBase=0、HoverOffset=5。旧配置没有这两个键时按 1/1 处理。
	ButtonBase  int
	HoverOffset int
	// ToggleHoverOffset 非 0 时表示演出开关用“常态帧/高亮帧”两态，
	// 而不是新版作品的 5 帧（掩码/关/关悬停/开/开悬停）。KD 为 5。
	ToggleHoverOffset int
	// TogglePanel/ClickAdvance 是 KD 的交互约定：右键显示/隐藏控制盒，
	// 左键点画面空白处切换下一张差分。新版作品保持原来的按钮式交互。
	TogglePanel  bool
	ClickAdvance bool
	// NumberX/NumberY/NumberSpace 是底栏当前序号的位置；序号用数字条画。
	// 它们和下面"NowNo*"读的是同一个键（now cg no pos / now cg no space），
	// 保留旧字段是为了不破坏已有调用点。
	NumberX     int
	NumberY     int
	NumberSpace int
	// Atlas 表示満淫電車2 的精灵图查看器：Base 是 136x124 的控制盒拼图，
	// 只画 panel base 矩形，箭头/退出/数字各用自己的源矩形。
	Atlas bool
	// NumberPatterns 是序号数字条的帧数（0-9 共 10 帧）。
	NumberPatterns int
	// CutIn/Hair/Steam 是底栏上的三个演出开关（切入/头发/水汽）。
	//
	// 这三张按钮图有 5 帧：0 是命中掩码（只用来判定点击，永不显示），
	// 1 是"关"的常态，2 是"关"的悬停，3 是"开"的常态，4 是"开"的悬停。
	CutIn UIButton
	Hair  UIButton
	Steam UIButton
	// Ahe 是「アヘ顔OFF」按钮。牝教師4 把它和切入/头发/水汽放在同一排，
	// 帧约定相同（0 掩码 / 1 关 / 2 关悬停 / 3 开 / 4 开悬停）。
	Ahe UIButton
	// BGVVolume 是 BGV 音量条的底图（单帧）；BGVDetail 是展开/收起这条
	// 音量条的按钮（3 帧：0 命中掩码、1 常态、2 悬停）。
	BGVVolume UIButton
	BGVDetail UIButton
	// BGVVolumeRegion 是音量条的可点击/可标记区域：x、y、宽、高。
	BGVVolumeRegion [4]int
	// NowNoX/NowNoY 是"当前 CG 编号"的坐标，NowNoSpaceX 是数字之间的字距。
	NowNoX      int
	NowNoY      int
	NowNoSpaceX int
	// AllThumbsX/AllThumbsY 是"全部缩略图"的坐标，AllThumbsSpaceX 是字距。
	AllThumbsX      int
	AllThumbsY      int
	AllThumbsSpaceX int
}

func parseUIButton(value string) UIButton {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) == 0 || len(parts[0]) == 0 {
		return UIButton{}
	}
	image := parts[0]
	button := UIButton{Frames: 1}
	// 复合图部件：image#N（N 从 1 开始）。先剥 # 再剥 @。
	if hash := strings.LastIndexByte(image, '#'); hash > 0 {
		if part, err := strconv.Atoi(image[hash+1:]); err == nil && part > 0 {
			button.Part = part
			image = image[:hash]
		}
	}
	// 固定帧：image@N。只用于数字条/复合条里"一项占一帧"的按钮。
	if at := strings.LastIndexByte(image, '@'); at > 0 {
		if start, err := strconv.Atoi(image[at+1:]); err == nil && start >= 0 {
			button.Start = start
			button.FixedFrame = true
			image = image[:at]
		}
	}
	button.Image = image
	if len(parts) > 1 {
		if frames, ok := parseInt(parts[1]); ok && frames > 0 {
			button.Frames = frames
		}
	}
	if len(parts) > 2 {
		button.X = parseCoordinate(parts[2])
	}
	if len(parts) > 3 {
		button.Y = parseCoordinate(parts[3])
	}
	return button
}

func parseUIPoint(value string) (UIPoint, bool) {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) < 2 {
		return UIPoint{}, false
	}
	return UIPoint{X: parseCoordinate(parts[0]), Y: parseCoordinate(parts[1])}, true
}

func parseDataScreen(document *IniDocument) DataScreen {
	screen := DataScreen{
		SaveBase:     document.Get(DataScreenSection, "save base", ""),
		LoadBase:     document.Get(DataScreenSection, "load base", ""),
		HitTable:     document.Get(DataScreenSection, "hit table", ""),
		NoBegin:      document.GetInt(DataScreenSection, "no begin", 0),
		PageMax:      document.GetInt(DataScreenSection, "page max", 10),
		SlotsPerPage: document.GetInt(DataScreenSection, "data slots", 28),
		AutoOnly:     document.Get(DataScreenSection, "auto only", ""),
		NoData:       document.Get(DataScreenSection, "no data", ""),
		Confirm:      parseSlotViewer(document),
	}
	// 精灵图世代：底图本身是 1024x1024 的拼图，按钮只是图里的矩形。
	// 用 save base 作为所有控件的源图（load base 的按钮区域与它逐像素
	// 相同，差别只在左上角的 SAVE/LOAD 标题）。
	atlas := screen.SaveBase
	screen.SelectFrame = atlasButtonOr(document, DataScreenSection, "select frame", atlas)
	screen.SaveMode = atlasButtonOr(document, DataScreenSection, "save mode", atlas)
	screen.LoadMode = atlasButtonOr(document, DataScreenSection, "load mode", atlas)
	// 这一代把设置页签写成 `config mode`，旧作品写 `config button`。
	if button, ok := parseAtlasButton(document, DataScreenSection, "config mode", atlas); ok {
		screen.Config = button
	} else if button, ok := parseAtlasButton(document, DataScreenSection, "config button", atlas); ok {
		screen.Config = button
	} else {
		screen.Config = parseUIButton(document.Get(DataScreenSection, "config button", ""))
	}
	screen.Back = atlasButtonOr(document, DataScreenSection, "back button", atlas)
	screen.Title = atlasButtonOr(document, DataScreenSection, "title button", atlas)
	screen.Exit = atlasButtonOr(document, DataScreenSection, "exit button", atlas)
	if button, ok := parseAtlasButton(document, DataScreenSection, "auto only", atlas); ok {
		screen.AutoOnlyButton = button
	}
	if button, ok := parseAtlasButton(document, DataScreenSection, "no data", atlas); ok {
		screen.NoDataButton = button
	}
	// 精灵图世代的槽位首格是自动存档：只读，显示 AUTO ONLY 占位图。
	if screen.AutoOnlyButton.Atlas {
		screen.AutoSlotIndex = 0
		screen.AutoSlotGrid = true
	}
	if screen.PageMax <= 0 {
		screen.PageMax = 10
	}
	if screen.SlotsPerPage <= 0 {
		screen.SlotsPerPage = 28
	}
	for index := 0; index < screen.SlotsPerPage; index++ {
		point, ok := parseUIPoint(document.Get(DataScreenSection, "slot"+strconv.Itoa(index)+" pos", ""))
		if !ok {
			break
		}
		screen.Slots = append(screen.Slots, point)
	}
	for index := 0; index < screen.PageMax; index++ {
		suffix := strconv.Itoa(index)
		// 精灵图写法：`page button` 是所有页码共用的常态矩形，
		// `page buttonN pos` 是落点，`page buttonN on` 是悬停/当前页矩形。
		if normal, ok := parseAtlasButton(document, DataScreenSection, "page button", atlas); ok {
			pos, posOK := parseUIPoint(document.Get(DataScreenSection, "page button"+suffix+" pos", ""))
			if !posOK {
				break
			}
			normal.X, normal.Y = pos.X, pos.Y
			if hover, hoverOK := parseAtlasButton(document, DataScreenSection, "page button"+suffix+" on", atlas); hoverOK {
				normal.HoverClip = hover.Clip
			}
			screen.PageButtons = append(screen.PageButtons, normal)
			continue
		}
		button := parseUIButton(document.Get(DataScreenSection, "page button"+suffix, ""))
		if button.Empty() {
			break
		}
		screen.PageButtons = append(screen.PageButtons, button)
	}
	// 页码热区：底图已经画好数字时只登记坐标，绘制层不会重复画数字。
	for index := 0; index < 16; index++ {
		point, ok := parseUIPoint(document.Get(DataScreenSection, "page hit"+strconv.Itoa(index), ""))
		if !ok {
			break
		}
		screen.PageHits = append(screen.PageHits, point)
	}
	// 排序按钮的键号在原配置里是 0/2/3（1 缺失），按实际出现的键收集，
	// 而不是假设 0..2 连续。
	for _, key := range []string{"sort button0", "sort button1", "sort button2", "sort button3"} {
		if point, ok := parseUIPoint(document.Get(DataScreenSection, key, "")); ok {
			screen.SortButtons = append(screen.SortButtons, point)
		}
	}
	return screen
}

func parseGameMenu(document *IniDocument) GameMenu {
	menu := GameMenu{
		Base: document.Get(GameMenuSection, "base", ""),
	}
	menu.BaseX, menu.BaseY = parsePoint(document.Get(GameMenuSection, "base pos", ""))
	menu.AutoHideMs = document.GetInt(GameMenuSection, "auto hide", 0)
	menu.AutoExIcon = parseMenuLamp(document, "autoex icon", "autoex pos")
	menu.CutInIcon = parseMenuLamp(document, "cut-in icon", "cut-in pos")
	menu.HoldIcon = parseMenuLamp(document, "hold icon", "hold pos")
	if area := ParsePositions(document.Get(GameMenuSection, "show area", "")); len(area) >= 4 {
		copy(menu.ShowArea[:], area[:4])
	} else {
		menu.ShowArea = [4]int{0, menu.BaseY, 1024, 149}
	}
	syncPosition := document.GetInt(GameMenuSection, "sync button pos", 0) != 0
	// [system button] 的值是"图片, 帧数, x, y, 作用域"，作用域为 game 的才会
	// 出现在游戏内菜单条上（system 的属于标题/确认对话框等其它界面）。
	for _, entry := range document.Section(SystemButtonSection) {
		button := parseUIButton(entry.Value)
		if button.Empty() {
			continue
		}
		parts := strutil.SplitTrimNonEmpty(entry.Value, ",")
		if len(parts) >= 5 && !strings.EqualFold(parts[4], "game") {
			continue
		}
		if syncPosition {
			// sync button pos = 1 表示 [system button] 的坐标相对
			// [game menu] base pos。SL 的 base pos 是 (452,412)：只加 Y
			// 会把 skip/config 等按钮留在左边、和正文重叠，auto/cutin/hold
			// 也会跑到画面中间。侵蝕/淫獄 的 BaseX 是 0，行为不变。
			button.X += menu.BaseX
			button.Y += menu.BaseY
		}
		menu.Buttons = append(menu.Buttons, MenuButton{Key: strings.ToLower(entry.Key), UIButton: button})
	}
	// BSXScript 1.0（満淫電車2）没有 [system button]：原引擎把 8 颗
	// 菜单按钮的位置硬编码在 exe 里。按素材宽度 86x32（4 帧竖排）排成
	// 顶部一排；只有 gamemenu_base + title phase base size 同时成立时补，
	// 不会影响任何已有作品。
	if len(menu.Buttons) == 0 && menu.Base == "gamemenu_base" &&
		document.Has(TitlePhaseSection, "base size") {
		// 只有这一代需要紫色垫底；其它 gamemenu_base 作品的底图自带颜色。
		menu.BaseFill = true
		defaults := []struct {
			key   string
			image string
			x     int
		}{
			{"qsave", "btn_qsave", 0},
			{"qload", "btn_qload", 86},
			{"save", "btn_save", 172},
			{"load", "btn_load", 258},
			{"config", "btn_config", 344},
			{"log", "btn_log", 430},
			{"dress", "btn_dress", 516},
			{"cut in", "btn_cutin", 604},
		}
		for _, item := range defaults {
			menu.Buttons = append(menu.Buttons, MenuButton{
				Key: item.key,
				UIButton: UIButton{
					Image: item.image, Frames: 4, X: item.x, Y: 6,
				},
			})
		}
		// 手册截图底部的 SKIP / AUTO / VOICE 三颗按钮在配置里被注释掉了，
		// 原引擎照样按固定坐标画。位置取自 800x600 手册图：三颗 170x32。
		for index, item := range []struct {
			key   string
			image string
		}{
			{"skip", "btn_skip"},
			{"auto", "btn_auto"},
			{"voice", "btn_voice"},
		} {
			menu.Buttons = append(menu.Buttons, MenuButton{
				Key: item.key,
				UIButton: UIButton{
					Image: item.image, Frames: 4, X: index * 170, Y: 568,
				},
			})
		}
		menu.ShowArea = [4]int{0, 0, 800, 50}
	}
	return menu
}

// parseMenuLamp 解析底栏开关指示灯：`<key> icon` 给图片名，`<key> pos`
// 给相对 [game menu] base pos 的坐标。没有配图时返回零值。
func parseMenuLamp(document *IniDocument, iconKey, posKey string) UIButton {
	button := parseUIButton(document.Get(GameMenuSection, iconKey, ""))
	if button.Empty() {
		return UIButton{}
	}
	button.X, button.Y = parsePoint(document.Get(GameMenuSection, posKey, ""))
	return button
}

func parseMuseum(document *IniDocument) Museum {
	museum := Museum{
		Base:    document.Get(MuseumSection, "base", ""),
		Return:  atlasButtonOr(document, MuseumSection, "return to title", document.Get(MuseumSection, "base", "")),
		MultiBu: document.GetInt(MuseumSection, "multi bu", 0) != 0,
	}
	// 页签的语义必须从按钮图名判断，不能按 modeN 的下标硬编码。
	//
	// 特別授業3SLG 旧版（tj3slg）只有 4 个页签：music / cg / scene / strip，
	// 没有 bgv。按 "music,bgv,cg,scene,strip" 取下标会把 CG 当成 BGV、
	// 回想当成 CG、立绘当成回想，玩家看到的就是"CG 页打不开、回想显示的是
	// 另一页"。新版 3.3 恰好是完整的 5 个，才让这个错误长期没有被发现。
	//
	// 图片名在历代作品里都带用途词（museum_mode_cg / museum_cg_mode /
	// museum_bgv 等），所以按词匹配最稳；无法识别时才退回下标。
	fallbackKeys := []string{"music", "bgv", "cg", "scene", "strip"}
	// 精灵图世代的页签图名是底图名（museum_base），无法从图名认出页面；
	// 这一代的顺序固定是 BGM/CG/Scene/立ち絵，没有单独的 BGV 页。
	atlasKeys := []string{"music", "cg", "scene", "strip"}
	atlasMuseum := false
	if button, ok := parseAtlasButton(document, MuseumSection, "mode0", museum.Base); ok {
		atlasMuseum = button.Image != ""
	}
	for index := 0; index < len(fallbackKeys); index++ {
		button := atlasButtonOr(document, MuseumSection, "mode"+strconv.Itoa(index), museum.Base)
		if button.Empty() {
			continue
		}
		key := museumModeKey(button.Image)
		if key == "" {
			if atlasMuseum && index < len(atlasKeys) {
				key = atlasKeys[index]
			} else {
				key = fallbackKeys[index]
			}
		}
		museum.Modes = append(museum.Modes, MuseumMode{
			Key:      key,
			State:    uint32(document.GetInt(MuseumSection, "mode"+strconv.Itoa(index)+" state", 0)),
			UIButton: button,
		})
	}
	museum.CG = parseMuseumCGPage(document)
	museum.Scene = parseMuseumScenePage(document)
	museum.Music = parseMuseumMusicPage(document)
	museum.Viewer = parseCGViewer(document)
	museum.Bu = parseMuseumBUPage(document)
	museum.BuViewer = parseBuViewer(document)
	return museum
}

// museumModeKey 从页签按钮图名推断页面种类。
//
// 先匹配更具体的 bgv，避免把 museum_bgv_mode 误判成 music 之类的短词；
// strip 也放在 scene 之前（个别作品写作 museum_scene_strip）。
//
// `dress` / `bu` 是同一页的旧称：SL（BSXScript 2.0）的页签图叫
// museum_dress_mode，内容就是立ち絵鑑賞。不能让它掉进"无法识别"的
// 下标回退——fallbackKeys[3] 是 scene，玩家点立绘鉴赏会打开回想页，
// 看起来就像按钮没反应。
func museumModeKey(image string) string {
	name := strings.ToLower(image)
	for _, key := range []string{"bgv", "strip", "dress", "bu", "music", "scene", "cg"} {
		if strings.Contains(name, key) {
			// dress/bu 与 strip 是同一种页面，统一成 strip，后面的
			// switch 与绘制逻辑只认这一个语义。
			if key == "dress" || key == "bu" {
				return "strip"
			}
			return key
		}
	}
	return ""
}

// parseCGViewer 解析 [cg viewer] 段：CG 查看器的底图、前后箭头、退出键，
// 底栏的演出开关（切入/头发/水汽）与 BGV 音量条。
func parseCGViewer(document *IniDocument) CGViewer {
	viewer := CGViewer{
		Base:        parseUIButton(document.Get(CgViewerSection, "base", "")),
		Prev:        parseUIButton(document.Get(CgViewerSection, "prev arrow", "")),
		Next:        parseUIButton(document.Get(CgViewerSection, "next arrow", "")),
		Exit:        parseUIButton(document.Get(CgViewerSection, "exit", "")),
		Config:      parseUIButton(document.Get(CgViewerSection, "config", "")),
		Number:      parseUIButton(document.Get(CgViewerSection, "number", "")),
		ButtonBase:  1,
		HoverOffset: 1,
		// 三个演出开关：5 帧（命中掩码 / 关 / 关的悬停 / 开 / 开的悬停）。
		CutIn: parseUIButton(document.Get(CgViewerSection, "cut_in", "")),
		Hair:  parseUIButton(document.Get(CgViewerSection, "hair", "")),
		Steam: parseUIButton(document.Get(CgViewerSection, "steam", "")),
		Ahe:   parseUIButton(document.Get(CgViewerSection, "ahe", "")),
		// BGV 音量条底图与详情按钮。
		BGVVolume: parseUIButton(document.Get(CgViewerSection, "bgv volume", "")),
		BGVDetail: parseUIButton(document.Get(CgViewerSection, "bgv detail", "")),
	}
	// KD 用 10 帧 cgm_btn：0..4 常态、5..9 红色悬停；开关同样是
	// 常态/高亮两态。其它作品没有这些键，保持 1/1 与 5 帧开关约定。
	viewer.ButtonBase = document.GetInt(CgViewerSection, "button base", viewer.ButtonBase)
	viewer.HoverOffset = document.GetInt(CgViewerSection, "hover offset", viewer.HoverOffset)
	viewer.ToggleHoverOffset = document.GetInt(CgViewerSection, "toggle hover offset", 0)
	viewer.TogglePanel = document.GetInt(CgViewerSection, "toggle panel", 0) != 0
	viewer.ClickAdvance = document.GetInt(CgViewerSection, "click advance", 0) != 0
	viewer.PanelX, viewer.PanelY = parsePoint(document.Get(CgViewerSection, "panel pos", ""))
	// "now cg no pos" 是当前编号的左上角坐标，"now cg no space" 是字距
	// （负数表示高位数字往左排）。
	viewer.NumberX, viewer.NumberY = parsePoint(document.Get(CgViewerSection, "now cg no pos", ""))
	viewer.NowNoX, viewer.NowNoY = viewer.NumberX, viewer.NumberY
	if space := ParsePositions(document.Get(CgViewerSection, "now cg no space", "")); len(space) > 0 {
		viewer.NumberSpace = space[0]
		viewer.NowNoSpaceX = space[0]
	}
	// "all thumbs pos" 是"全部缩略图"列表的左上角坐标，字距同理。
	viewer.AllThumbsX, viewer.AllThumbsY = parsePoint(document.Get(CgViewerSection, "all thumbs pos", ""))
	if space := ParsePositions(document.Get(CgViewerSection, "all thumbs space", "")); len(space) > 0 {
		viewer.AllThumbsSpaceX = space[0]
	}
	// 序号数字条的帧数（0-9）。
	viewer.NumberPatterns = document.GetInt(CgViewerSection, "number patterns", 10)
	// 精灵图世代：base 是素材表，panel base 是实际面板；按钮坐标相对
	// panel pos，需要在解析时换算成屏幕绝对坐标。
	baseName := document.Get(CgViewerSection, "base", "")
	if panel, ok := parseAtlasButton(document, CgViewerSection, "panel base", baseName); ok {
		panel.Image = baseName
		viewer.Atlas = true
		viewer.Base = panel
		viewer.PanelX, viewer.PanelY = parsePoint(document.Get(CgViewerSection, "panel base pos", ""))
		for _, item := range []struct {
			key    string
			target *UIButton
		}{
			{"prev arrow", &viewer.Prev}, {"next arrow", &viewer.Next},
			{"exit", &viewer.Exit}, {"cut_in", &viewer.CutIn},
		} {
			if button, ok := parseAtlasButton(document, CgViewerSection, item.key, baseName); ok {
				button.X += viewer.PanelX
				button.Y += viewer.PanelY
				*item.target = button
			}
		}
		if number, ok := parseAtlasButton(document, CgViewerSection, "number", baseName); ok {
			viewer.Number = number
			viewer.NowNoX, viewer.NowNoY = parsePoint(document.Get(CgViewerSection, "now cg no pos", ""))
			viewer.NowNoX += viewer.PanelX
			viewer.NowNoY += viewer.PanelY
			if space := ParsePositions(document.Get(CgViewerSection, "now cg no space", "")); len(space) > 0 {
				viewer.NowNoSpaceX = space[0]
			}
		}
		viewer.ToggleHoverOffset = 1
	}
	viewer.Number.Frames = viewer.NumberPatterns
	// 音量条区域：x、y、宽、高。
	if region := ParsePositions(document.Get(CgViewerSection, "bgv volume region", "")); len(region) >= 4 {
		copy(viewer.BGVVolumeRegion[:], region[:4])
	}
	return viewer
}

func parseMuseumCGPage(document *IniDocument) MuseumCGPage {
	base := document.Get(MuseumCgSection, "base", "")
	page := MuseumCGPage{
		Base:         base,
		UnderBase:    document.Get(MuseumCgSection, "under base", ""),
		HitTable:     document.Get(MuseumCgSection, "hit table", ""),
		Percent:      parseUIButton(document.Get(MuseumCgSection, "percent number", "")),
		PercentSpace: document.GetInt(MuseumCgSection, "percent number space", -25),
		Parts:        parseUIButton(document.Get(MuseumCgSection, "thumb number", "")),
		PartsSpace:   document.GetInt(MuseumCgSection, "thumb number space", -13),
		Title:        atlasButtonOr(document, MuseumCgSection, "title", base),
		Menu:         atlasButtonOr(document, MuseumCgSection, "menu", base),
	}
	// `percent number` 是达成率数字条；旧作品写 thumb number。
	if number, ok := parseAtlasButton(document, MuseumCgSection, "percent number", base); ok {
		page.Percent = number
	} else if number, ok := parseAtlasButton(document, MuseumCgSection, "thumb number", base); ok {
		page.Parts = number
	}
	if info, ok := parseAtlasButton(document, MuseumCgSection, "thumb info number", base); ok {
		page.ThumbInfo = info
		page.ThumbInfoSpace = document.GetInt(MuseumCgSection, "thumb info space", 0)
	}
	// 页签上限必须覆盖配置里的最大编号。特別授業3SLG 有 page icon0..12
	// 共 13 个（第 13 个是 FINALE 页），旧上限 12 会让最后一页既画不出
	// 也点不到，cg.dat 里对应的整组 CG 永远无法浏览。
	page.PageButtons = parseLoopButtonsWithAtlas(document, MuseumCgSection, "page icon", base, 16)
	page.Thumbs = parseLoopPoints(document, MuseumCgSection, "thumb", " pos", 64)
	page.ThumbX, page.ThumbY, page.ThumbW, page.ThumbH = deriveThumbGrid(page.Thumbs)
	// 精灵图世代的缩略图框与尺寸是分开写的：`thumb0 frame` 给框的源矩形，
	// `thumbnail size` 给缩略图的实际像素（120x90），`thumb = 2,2` 是
	// 缩略图相对框左上角的偏移。
	for index := 0; index < len(page.Thumbs); index++ {
		key := "thumb" + strconv.Itoa(index) + " frame"
		frame, ok := parseAtlasButton(document, MuseumCgSection, key, base)
		if !ok {
			break
		}
		page.ThumbFrames = append(page.ThumbFrames, frame)
	}
	if size := ParsePositions(document.Get(MuseumCgSection, "thumbnail size", "")); len(size) >= 2 && size[0] > 0 && size[1] > 0 {
		page.ThumbW, page.ThumbH = size[0], size[1]
	}
	if offset := ParsePositions(document.Get(MuseumCgSection, "thumb", "")); len(offset) >= 2 {
		page.ThumbOffset = UIPoint{X: offset[0], Y: offset[1]}
	}
	page.Atlas = page.Title.Atlas || len(page.ThumbFrames) > 0
	return page
}

// parseLoopButtonsWithAtlas 先按精灵图写法解析一组按钮，失败时退回旧格式。
func parseLoopButtonsWithAtlas(document *IniDocument, section, prefix, atlasImage string, limit int) []UIButton {
	result := make([]UIButton, 0, limit)
	for index := 0; index < limit; index++ {
		button := atlasButtonOr(document, section, prefix+strconv.Itoa(index), atlasImage)
		if button.Empty() {
			break
		}
		result = append(result, button)
	}
	return result
}

// deriveThumbGrid 从 thumbN pos 推导格子原点与尺寸。
//
// 配置只给每个格子的左上角；列间距是"同一行相邻格子的 X 差"，行间距是
// "同一列相邻格子的 Y 差"。取最小正值可以避免 4 列/5 列混排时算成整行宽。
func deriveThumbGrid(points []UIPoint) (x, y, width, height int) {
	if len(points) == 0 {
		return 0, 0, 126, 96
	}
	x, y = points[0].X, points[0].Y
	bestX, bestY := 0, 0
	for _, point := range points {
		if delta := point.X - x; delta > 0 && (bestX == 0 || delta < bestX) {
			bestX = delta
		}
		if delta := point.Y - y; delta > 0 && (bestY == 0 || delta < bestY) {
			bestY = delta
		}
	}
	if bestX <= 0 {
		bestX = 126
	}
	if bestY <= 0 {
		bestY = 96
	}
	return x, y, bestX, bestY
}

// parseSlotViewer 解析 [slot viewer]：存档覆盖确认窗口的详情布局。
func parseSlotViewer(document *IniDocument) SlotViewer {
	viewer := SlotViewer{
		Base:           document.Get(SlotViewerSection, "base", ""),
		OK:             parseUIButton(document.Get(SlotViewerSection, "ok", "")),
		Cancel:         parseUIButton(document.Get(SlotViewerSection, "cancel", "")),
		NoHeader:       parseUIButton(document.Get(SlotViewerSection, "no header", "")),
		Numbers:        parseUIButton(document.Get(SlotViewerSection, "numbers", "")),
		TimeZone:       parseUIButton(document.Get(SlotViewerSection, "time zone", "")),
		TextColor:      parseColor(document.Get(SlotViewerSection, "text color", "0xffffffff"), 0xffffffff),
		FontEdge:       document.GetInt(SlotViewerSection, "font edge", 0),
		DayUnit:        parseUIButton(document.Get(SlotViewerSection, "day unit", "")),
		DaySpecial:     parseUIButton(document.Get(SlotViewerSection, "day sp", "")),
		DayNoOffset:    document.GetInt(SlotViewerSection, "day no offset", 0),
		DayNoMax:       document.GetInt(SlotViewerSection, "day no max", 99),
		SceneNameSize:  document.GetInt(SlotViewerSection, "scene name size", 14),
		SceneNameColor: parseColor(document.Get(SlotViewerSection, "scene name color", "0xffffffff"), 0xffffffff),
		SceneNameEdge:  parseColor(document.Get(SlotViewerSection, "scene name edge", "0x64000000"), 0x64000000),
		SceneNameAlign: document.GetInt(SlotViewerSection, "scene name align", 0),
	}
	viewer.TimeZoneNumbers = document.GetInt(SlotViewerSection, "time zone numbers", 0)
	if point, ok := parseUIPoint(document.Get(SlotViewerSection, "thumbnail", "")); ok {
		viewer.Thumbnail = point
	}
	if values := ParsePositions(document.Get(SlotViewerSection, "text area", "")); len(values) >= 4 {
		viewer.TextArea = [4]int{values[0], values[1], values[2], values[3]}
	}
	if values := ParsePositions(document.Get(SlotViewerSection, "font size", "")); len(values) >= 3 {
		viewer.FontSize = [3]int{values[0], values[1], values[2]}
	}
	if values := ParsePositions(document.Get(SlotViewerSection, "slot no pos", "")); len(values) >= 2 {
		viewer.SlotNoX, viewer.SlotNoY = values[0], values[1]
	}
	viewer.SlotNoOffset = document.GetInt(SlotViewerSection, "slot no offset", 0)
	if values := ParsePositions(document.Get(SlotViewerSection, "date pos", "")); len(values) >= 2 {
		viewer.DateX, viewer.DateY = values[0], values[1]
	}
	viewer.DateOffset = document.GetInt(SlotViewerSection, "date offset", 0)
	if values := ParsePositions(document.Get(SlotViewerSection, "time pos", "")); len(values) >= 2 {
		viewer.TimeX, viewer.TimeY = values[0], values[1]
	}
	viewer.TimeOffset = document.GetInt(SlotViewerSection, "time offset", 0)
	if values := ParsePositions(document.Get(SlotViewerSection, "scene name pos", "")); len(values) >= 2 {
		viewer.SceneNameX, viewer.SceneNameY = values[0], values[1]
	}
	if values := ParsePositions(document.Get(SlotViewerSection, "time in game pos", "")); len(values) >= 2 {
		viewer.TimeInGame = UIPoint{X: values[0], Y: values[1]}
	}
	viewer.DayStrip = parseUIButton(document.Get(SlotViewerSection, "day no", ""))
	// slotviewer_font 是 12 帧竖排字条：0-9、'/'、':'。
	// 配置只给了名字没给帧数，这里补上，否则会把 288px 切成 10 帧、每帧 28px。
	if viewer.Numbers.Image != "" && viewer.Numbers.Frames <= 1 {
		viewer.Numbers.Frames = 12
	}
	return viewer
}

func parseMuseumScenePage(document *IniDocument) MuseumScenePage {
	base := document.Get(MuseumSceneSection, "base", "")
	page := MuseumScenePage{
		Base:           base,
		UnderBase:      document.Get(MuseumSceneSection, "under base", ""),
		HitTable:       document.Get(MuseumSceneSection, "hit table", ""),
		Percent:        parseUIButton(document.Get(MuseumSceneSection, "percent number", "")),
		PercentSpace:   document.GetInt(MuseumSceneSection, "percent number space", -25),
		PlayNow:        parseUIButton(document.Get(MuseumSceneSection, "play now", "")),
		AddPlaylist:    parseUIButton(document.Get(MuseumSceneSection, "add playlist", "")),
		PlaylistButton: parseUIButton(document.Get(MuseumSceneSection, "playlist button", "")),
		Playlist:       parseScenePlaylist(document),
		Title:          atlasButtonOr(document, MuseumSceneSection, "title", base),
		Menu:           atlasButtonOr(document, MuseumSceneSection, "menu", base),
	}
	if number, ok := parseAtlasButton(document, MuseumSceneSection, "percent number", base); ok {
		page.Percent = number
	}
	if info, ok := parseAtlasButton(document, MuseumSceneSection, "thumb info number", base); ok {
		page.ThumbInfo = info
		page.ThumbInfoSpace = document.GetInt(MuseumSceneSection, "thumb info space", 0)
	}
	page.PageButtons = parseLoopButtonsWithAtlas(document, MuseumSceneSection, "page icon", base, 16)
	page.Thumbs = parseLoopPoints(document, MuseumSceneSection, "thumb", " pos", 64)
	page.ClickMode = parseUIButton(document.Get(MuseumSceneSection, "click mode", ""))
	for index := 0; index < 8; index++ {
		point, ok := parseUIPoint(document.Get(MuseumSceneSection, "click mode"+strconv.Itoa(index), ""))
		if !ok {
			break
		}
		page.ClickModePoints = append(page.ClickModePoints, point)
	}
	page.ThumbX, page.ThumbY, page.ThumbW, page.ThumbH = deriveThumbGrid(page.Thumbs)
	for index := 0; index < len(page.Thumbs); index++ {
		key := "thumb" + strconv.Itoa(index) + " frame"
		frame, ok := parseAtlasButton(document, MuseumSceneSection, key, base)
		if !ok {
			break
		}
		page.ThumbFrames = append(page.ThumbFrames, frame)
	}
	if size := ParsePositions(document.Get(MuseumSceneSection, "thumbnail size", "")); len(size) >= 2 && size[0] > 0 && size[1] > 0 {
		page.ThumbW, page.ThumbH = size[0], size[1]
	}
	if offset := ParsePositions(document.Get(MuseumSceneSection, "thumb", "")); len(offset) >= 2 {
		page.ThumbOffset = UIPoint{X: offset[0], Y: offset[1]}
	}
	page.Atlas = page.Title.Atlas || len(page.ThumbFrames) > 0
	return page
}

// parseScenePlaylist 解析 [scene playlist]。
func parseScenePlaylist(document *IniDocument) ScenePlaylist {
	playlist := ScenePlaylist{
		Base:      document.Get(ScenePlaylistSection, "base", ""),
		HitTable:  document.Get(ScenePlaylistSection, "hit table", ""),
		ScrollBar: parseUIButton(document.Get(ScenePlaylistSection, "scroll bar", "")),
		Play:      parseUIButton(document.Get(ScenePlaylistSection, "play button", "")),
		Close:     parseUIButton(document.Get(ScenePlaylistSection, "close button", "")),
		Columns:   document.GetInt(ScenePlaylistSection, "thumb x count", 7),
		SlotMax:   document.GetInt(ScenePlaylistSection, "slot max", 42),
	}
	// thumb = x, y, width, height（注意是四元组，不是普通按钮）。
	if values := ParsePositions(document.Get(ScenePlaylistSection, "thumb", "")); len(values) >= 4 {
		playlist.ThumbX, playlist.ThumbY, playlist.ThumbW, playlist.ThumbH = values[0], values[1], values[2], values[3]
	}
	if playlist.Columns <= 0 {
		playlist.Columns = 7
	}
	if playlist.SlotMax <= 0 {
		playlist.SlotMax = 42
	}
	playlist.PageButtons = parseLoopButtons(document, ScenePlaylistSection, "page button", 10)
	return playlist
}

func parseMuseumMusicPage(document *IniDocument) MuseumMusicPage {
	base := document.Get(MuseumMusicSection, "base", "")
	page := MuseumMusicPage{
		Base:        base,
		HitTable:    document.Get(MuseumMusicSection, "hit table", ""),
		Play:        atlasButtonOr(document, MuseumMusicSection, "play icon", base),
		Stop:        atlasButtonOr(document, MuseumMusicSection, "stop icon", base),
		Prev:        atlasButtonOr(document, MuseumMusicSection, "prev icon", base),
		Next:        atlasButtonOr(document, MuseumMusicSection, "next icon", base),
		Max:         atlasButtonOr(document, MuseumMusicSection, "max button", base),
		Min:         atlasButtonOr(document, MuseumMusicSection, "min button", base),
		Volume:      atlasButtonOr(document, MuseumMusicSection, "volume", base),
		ControlBase: parseUIButton(document.Get(MuseumMusicSection, "control base", "")),
		Title:       atlasButtonOr(document, MuseumMusicSection, "title", base),
		Menu:        atlasButtonOr(document, MuseumMusicSection, "menu", base),
		ButtonFrame: atlasButtonOr(document, MuseumMusicSection, "button frame", base),
		VolumeFrame: atlasButtonOr(document, MuseumMusicSection, "volume frame", base),
		// volume knob / volume track height 是 KD 专用的滑块描述：
		// 其它作品给的是完整横条，没有这两个键。
		VolumeKnob:        document.GetInt(MuseumMusicSection, "volume knob", 0) != 0,
		VolumeTrackHeight: document.GetInt(MuseumMusicSection, "volume track height", 0),
	}
	if region := ParsePositions(document.Get(MuseumMusicSection, "volume region", "")); len(region) >= 4 {
		copy(page.VolumeRegion[:], region[:4])
	}
	if page.Volume.Atlas && page.Volume.Clip[2] > 0 && page.Volume.Clip[3] > 0 && page.VolumeRegion[2] <= 0 {
		// 満淫電車2 没有单独写 volume region：竖条本身就是轨道，
		// 用 volume pos + 源矩形尺寸补出可点击区域，触屏才能拖音量。
		page.VolumeRegion = [4]int{page.Volume.X, page.Volume.Y, page.Volume.Clip[2], page.Volume.Clip[3]}
	}
	for index := 1; index <= 32; index++ {
		// 曲目键是 track01…track20（两位补零），不是 track1。
		key := "track" + fmt.Sprintf("%02d", index)
		button := atlasButtonOr(document, MuseumMusicSection, key, base)
		if button.Empty() {
			break
		}
		page.Tracks = append(page.Tracks, button)
		page.Names = append(page.Names, document.Get(MuseumMusicSection, key+" name", ""))
	}
	page.Atlas = page.Title.Atlas || (len(page.Tracks) > 0 && page.Tracks[0].Atlas)
	return page
}

func parseLoopButtons(document *IniDocument, section, prefix string, limit int) []UIButton {
	result := make([]UIButton, 0, limit)
	for index := 0; index < limit; index++ {
		button := parseUIButton(document.Get(section, prefix+strconv.Itoa(index), ""))
		if button.Empty() {
			break
		}
		result = append(result, button)
	}
	return result
}

func parseLoopPoints(document *IniDocument, section, prefix, suffix string, limit int) []UIPoint {
	result := make([]UIPoint, 0, limit)
	for index := 0; index < limit; index++ {
		point, ok := parseUIPoint(document.Get(section, prefix+strconv.Itoa(index)+suffix, ""))
		if !ok {
			break
		}
		result = append(result, point)
	}
	return result
}
