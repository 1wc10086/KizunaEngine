// Package config（界面部分）：把 _system 里的界面段解析成结构化布局。
//
// 原引擎的这些段不是"给美术看的配置"，而是逐个像素的布局：
// MESSAGEWINDOW 给出对话框底图位置、正文坐标、字体、催促图标；
// TWB_* / MMB_* 给出底部工具条和暂停菜单的按钮矩形（常用负数表示"仍在同一张
// 图里、向左偏移"）；SAVE/LOAD/SYSTEM/VIEW 给出各自界面的分页与格子。
//
// 这里只做"字符串 → 数字/矩形"的翻译，不解释按钮语义：语义（SAVE 打开存读档、
// SYSTEM 打开设置…）随作品变化，由 session/ui 决定。之所以集中在一个文件，
// 是因为这些段的字段名相互引用（VIEW 的百分比坐标来自 LOAD 的格子、ARCHIVE 的
// 缩略图来自 SAVE 的缩略图尺寸），拆开会把同一套数字抄两遍。
package config

import (
	"strconv"
	"strings"
)

// Rect 是一个界面矩形（原版坐标系，1280x720）。
type Rect struct {
	X, Y, W, H int
}

// Empty 报告矩形是否未定义（原版用 0,0,0,0 表示"本作不显示该按钮"）。
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Contains 判断点是否落在矩形里。
func (r Rect) Contains(x, y int) bool {
	return !r.Empty() && x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// MessageWindow 是 [MESSAGEWINDOW] 的布局。
//
// 底图是 sys_window_base（1280x181 的白色渐变遮罩），原版用反相混合画在
// MW_DRAW_POS_Y 处；MW_DRAW_HEIGHT 是底图高度，剩下的 MW_DRAW_Y.. 部分靠
// 底图自己的垂直渐变保持半透明。
type MessageWindow struct {
	X, Y          int
	Width, Height int
	FrameDraw     bool // MW_FRAM_DRAW：是否画边框（sys_window_frame）
	GraphicCount  int  // MW_GRAPHIC_COUNT：底图变体张数（1..N 对应 sys_window_base..）
	FontName      string
	FontSize      int
	TextColor     uint32
	ReadColor     uint32
	FontWeight    int
	LineMax       int // 正文行数
	TextMax       int // 每行字数
	FrontH        int // 行首字距（TEXT_FRONT_H）
	BackH         int // 行尾字距
	BracketsH     int
	TextX, TextY  int
	TextNext      int // 行距
	VulgarColor   uint32
	// 角色名：NP_DRAW_FLAG 关闭时名字画在 NAME_DRAW_POS 处，
	// NAME_DRAW_PLUS 是微调偏移（原版给个别作品手调）。
	NameDraw   bool
	NameX      int
	NameY      int
	NameWidth  int
	NameHeight int
	NamePlusX  int
	NamePlusY  int
	// [MESSAGE] 段：原版确认框（sys_message_base_0N + sys_message_button）的
	// 位置与 YES/NO 按钮几何。MES_B_NEXT_X 是"第二个按钮相对第一个的横向步进"，
	// 本作是 134（108 宽 + 26 间距）。
	ConfirmX, ConfirmY int
	ConfirmW, ConfirmH int
	ButtonX, ButtonY   int
	ButtonW, ButtonH   int
	ButtonNextX        int
	// 催促图标：CW_DRAW_PAT 帧、CW_DRAW_TIME 每帧毫秒。
	WaitX, WaitY  int
	WaitW, WaitH  int
	WaitTimeMs    int
	WaitFrames    int
	OverlayCount  int
	TextDrawShade int // SYS_DEF_TEXT_DRAW_SHADOW
}

// ToolbarButton 是底部工具条/暂停菜单的一个按钮。
type ToolbarButton struct {
	// Key 是原版键名（TWB_SAVE、MMB_BACKLOG… 去掉前缀）。
	Key string
	// Enabled 为 false 时原版不画也不响应（配置里第一字段 0）。
	Enabled bool
	// Kind 是配置第二字段：0 = 消息窗底部工具条，1 = 暂停菜单。两组的 Y 语义
	// 不同（工具条相对消息窗，菜单是屏幕绝对坐标），所以必须保留。
	Kind int
	// Image 是按钮图名（sys_window_button）；X/Y/W/H 是它在屏幕上的矩形。
	// SrcX/SrcY 是 image 内的源偏移——按钮图是一张横排大图，按字段裁出每一格。
	Image     string
	X, Y      int
	SrcX      int
	SrcY      int
	W, H      int
	FrameW    int // image 里每帧的宽度（0 表示整图一帧）
	FrameH    int
	FramesCnt int
	// Hover 沿用原版 0/1（常态/悬停）。
	Hover bool
}

// Toolbar 是 [MESSAGEWINDOW] 的 TWB_* 组，也就是正文下方那排
// 「Q-SAVE / Q-LOAD / BACKLOG / AUTO / SKIP / VOICE / SYSTEM / CLEAR」。
type Toolbar struct {
	Width, Height int
	Buttons       []ToolbarButton
}

// MenuBar 是 [MENU] / [MAPMENU] / [MAPMENU03] 的按钮组，也就是 ESC 菜单
// 「セーブ / ロード / システム / タイトル画面 / ゲーム終了 / 回想終了」。
type MenuBar struct {
	Kind     int
	Width    int
	Height   int
	Buttons  []ToolbarButton
	PageMax  int
	PageType []int
	Widths   []int
	Heights  []int
}

// SlotBoard 是存读档界面的格子布局（[SAVE] / [LOAD] 共用格式）。
//
// 原版每页 10 格、2 列 5 行，格子 544x90；缩略图 128x72 放在格内 (19,9)，
// 文字从 (200,10) 开始，行距 20。页签在 (318,72) 起，每页 60x27。
type SlotBoard struct {
	Kind int
	// TileW/TileH 是单个格子尺寸（SAVE_BASE_POS 的第 5/6 字段）。
	TileW, TileH int
	// Cols/Rows 是每页列/行数（第 9/8 字段）。
	Cols, Rows int
	// BaseX/BaseY 是第一个格子的左上角。
	BaseX, BaseY int
	// GapX/GapY 是格子间距（第 14/11 字段：列步进/行步进）。
	GapX, GapY int
	// TextX/TextY/TextNext 是格子内正文坐标与行距。
	// TextX 是格子内文字起点（相对格子左上角）。原版的 SAVE_B_TEXT_POS 给的是
	// 200,10,20 —— 200 已经是**格内**坐标，不需要再加上格子 x。
	TextX, TextY, TextNext int
	ThumbX, ThumbY         int
	ThumbW, ThumbH         int
	PageMax, SlotMax       int
	// PageX/PageY 是页签起点，PageW/PageH 是单页尺寸，PageStepX 是列步进。
	PageX, PageY       int
	PageW, PageH       int
	PageStepX          int
	PageSrcX, PageSrcY int
	// 确认框/自动存档/删档/返回按钮：位置 + 图集里的源矩形。
	ReturnX, ReturnY       int
	ReturnSrcX, ReturnSrcY int
	ReturnW, ReturnH       int
	AutoX, AutoY           int
	AutoSrcX, AutoSrcY     int
	AutoW, AutoH           int
	DelX, DelY             int
	DelSrcX, DelSrcY       int
	DelW, DelH             int
	// SaveType 是原版的存档类型位（101 = 有缩略图 + 标题 + 日期 + NO DATA）。
	SaveType int
}

// SystemScreen 是 [SYSTEM] 的设置界面布局。
//
// 原版把每个控件写成一整行数字（SW_GROUP = 多态按钮、SW_SLIDE = 滑杆、
// SW_ONE = 单按钮），这里保存原始字段，由 ui 按 Kind 解释。
type SystemScreen struct {
	PageMax  int
	Parts    []SystemPart
	Defaults map[string][]int
	// LeftNav 是左侧「一般設定 / テキスト設定 / サウンド設定 / 射精設定 /
	// ボイス設定」的按钮区（SYSTEM_PARTS_9901..9904）。
	PageButtons []Rect
	GameButton  Rect
	TitleButton Rect
	EndButton   Rect
}

// SystemPart 是一行设置控件。
//
// 字段顺序逐列核对过 _system（并用 sys_system_base_01..05 里的实心板位置
// 验证过每一行）：这是设置页唯一的真值来源，读错任何一个都会让控件画歪、
// 热区错位。本作的控件分两类：
//
//	SW_GROUP / SW_ONE（一排/多行按钮）
//	  0 保留, 1 键名, 2 种类, 3 屏幕x, 4 屏幕y,
//	  5 图集srcX, 6 图集srcY, 7 单元宽, 8 单元高,
//	  9 单元总数, 10 每行单元数, 11 行数,
//	  12 屏幕横向步进（带符号：负 = 选项从 X 向左铺）,
//	  13 0, 14 图集横向步进（带符号）, 15/16 0, 17 屏幕行距
//
//	SW_SLIDE（滑杆）
//	  9 滑杆条数, 10/11 与按钮组同义, 13 条间行距（33 / 56）, 18 轨道长度
//
// 注意 12/14 的负号：ON|OFF 这类组的图集把 OFF 画在左边、ON 在右边，
// 配置里的 SrcX 指向 ON、SrcStep = -160，屏幕步进也是 -168 且 X 指向 ON。
// 也就是说"负步进 = 配置里的 X 是最右一格、选项向左边铺"，第一格要按
// X + (perRow-1)*step 定位。早期实现把负号当成"取绝对值"的信号，
// 结果 ON/OFF 左右颠倒、第二格画到面板外；命中矩形还错误地按"整行宽度"
// 算，点滑杆会先命中同一行的开关组，滑杆拖不动、开关点不中。
type SystemPart struct {
	Page int    // 页号（1 起）
	Key  string // 键名（S_TEXTSPEED 等）
	Kind string // SW_GROUP / SW_SLIDE / SW_ONE / SW_PUSH
	X, Y int
	W, H int
	SrcX int
	SrcY int
	// Total 是单元总数（SW_SLIDE 的条数）；PerRow 每行单元数；Rows 行数。
	Total, PerRow, Rows int
	// ColStep 是屏幕横向步进（带符号），SrcStep 是图集横向步进（带符号）。
	ColStep, SrcStep int
	// RowStepY 是多行控件的屏幕行距；SW_SLIDE 用它排列多条滑杆。
	RowStepY int
	// SlideX 是滑杆轨道长度（SW_SLIDE）。
	SlideX int
	Raw    []string
}

// ViewScreen 是 [VIEW]（CG 鉴赏）的分页布局。
//
// VIEW_GRAPHIC_COUNT 给出每页 10x10 的格子区；实际每页 4 行 5 列来自
// 原版「CG GALLERY」画面的固定排版，页签由 cgmode座標 的 PREVIOUS/RETURN/NEXT
// 三联按钮承担。
type ViewScreen struct {
	PercentX, PercentY int
	// NumberW/NumberH 是 sys_number 图集里单个数字的显示尺寸（[VIEW] 的
	// VIEW_NUMBER_SIZE=20,26）。号码从 PercentX 起逐位向右贴。
	NumberW, NumberH   int
	CellsX, CellsY     int
	CellW, CellH       int
	CellGapX, CellGapY int
	Rows, Cols         int
	PageMax            int
	Prev, Next         Rect
	Return             Rect
}

// AlbumScreen 是 [ALBUM]（场景回想）的按钮与缩放。
type AlbumScreen struct {
	// Scale 是原版把剧情 CG 缩放进预览框的比例（POSITION 第三字段）。
	Scale    float64
	PosX     int
	PosY     int
	PrevImg  string
	Prev     Rect
	NextImg  string
	Next     Rect
	Return   Rect
	Pager    []Rect
	PageMax  int
	PageText bool
}

// SelectScreen 是选项菜单的排版（[SELECT]）。
type SelectScreen struct {
	Width, Height int
	PosX, PosY    int
	NextX, NextY  int
	FontSize      int
	TextType      int
	TextPlusX     int
	TextPlusY     int
}

// CharaView 是 [CHARAVIEW] 的立绘查看器布局。
//
// 原版的 CHARACTER VIEWER 是"左侧整屏立绘 + 右侧一列按钮"的独立界面：
// [CHARAVIEW] 的 CharaStat/BGStat 给出列表起点与格子尺寸（30x156 的竖条），
// B_Hide/B_End/B_MoveX/MoveY/B_Scale/B_Init 给出操作按钮的屏幕位置与图集源坐标。
type CharaView struct {
	FontSize int
	// ListX/ListY 是右侧列表（角色/服装/表情）的起点，格子 30x156、步进 30。
	ListX, ListY int
	ListW, ListH int
	ListStep     int
	// BGX/BGY 是背景列表（[BG] 段）的位置；BGStep 是它的行步进。
	BGX, BGY         int
	BGW, BGH, BGStep int
	// Hide/End/Init 是单按钮；MoveX/MoveX2（◀/▶）、MoveY/MoveY2（▲/▼）、
	// Scale/Scale2（縮小/拡大）是同一行的两个按钮。每个按钮都带屏幕矩形与图集源矩形。
	//
	// 原版把"同一行第二个按钮"写成相对第一条的偏移（B_Scale=1088,481,776,0,76,30,-80,0,-76,0），
	// 以前只读第一个，于是玩家只看到一个按钮，且 ▲/▼ 被画到同一处（重叠）。
	Hide, End, Init                    Rect
	HideSrc, EndSrc, InitSrc           Rect
	MoveX, MoveX2, MoveXSrc, MoveX2Src Rect
	MoveY, MoveY2, MoveYSrc, MoveY2Src Rect
	Scale, Scale2, ScaleSrc, Scale2Src Rect
	// RowArrowLsrc/RowArrowRsrc 是四行选择条两侧小箭头在图集里的裁片
	// （◀ 157,0,38,30 / ▶ 197,0,38,30）。箭头不是配置项，是固定素材。
	RowArrowLsrc, RowArrowRsrc Rect
	// Characters 是 [CHARAVIEW] 的显示名（マリア,マリア催眠,リサ,…）。
	Characters []string
	// Entries 是每个角色页（[CHARA01..06]）的宏前缀、服装表、表情表。
	// 12 个显示名逐个对应 CHARA01..06 六段配置（同一角色的普通/催眠两套表情）。
	Entries []CharaEntry
	// BGList 是 [BG] 段的「BG01=BG0100_a,コックピット_通常」。
	BGList []ViewEntry
}

// CharaEntry 是一个角色页（[CHARA01..06]）。
//
// 宏名拼法是 Prefix + Costume + Expr：_charafilemacro 的键是
// fm_C01a_01_0_0_0_01，而 CostumeList 是 "01_0_0_0"、ExprList 是 "_01"。
type CharaEntry struct {
	Prefix      string
	Costumes    []string
	CostumeList []string
	Expressions []string
	ExprList    []string
	// Page 是配置段名（CHARA01..06），诊断用。
	Page string
}

// ViewEntry 是鉴赏里的一行（名字 + 显示用标题）。
type ViewEntry struct {
	Name  string
	Label string
}

// Ints 取逗号分隔的整数（解析失败按 0），供 _system 的布局行使用。
func (i *Ini) IntsRaw(section, key string) []string {
	value := i.Get(section, key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

// RectAt 从字段序列里取 x,y,w,h 四个整数（不足的位置补 0）。
func RectAt(fields []string, offset int) Rect {
	if len(fields) < offset+4 {
		return Rect{}
	}
	return Rect{
		X: atoiField(fields[offset]),
		Y: atoiField(fields[offset+1]),
		W: atoiField(fields[offset+2]),
		H: atoiField(fields[offset+3]),
	}
}

func atoiField(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return n
}

// ParseColor 把 "ffffff" / "#ffffff" 解成 0xFFFFFFFF（不透明）。
func ParseColor(value string) uint32 {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	if len(value) != 6 {
		return 0xffffffff
	}
	var rgb uint32
	for i := 0; i < 6; i++ {
		rgb <<= 4
		ch := value[i]
		switch {
		case ch >= '0' && ch <= '9':
			rgb |= uint32(ch - '0')
		case ch >= 'a' && ch <= 'f':
			rgb |= uint32(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			rgb |= uint32(ch-'A') + 10
		default:
			return 0xffffffff
		}
	}
	return 0xFF000000 | rgb
}
