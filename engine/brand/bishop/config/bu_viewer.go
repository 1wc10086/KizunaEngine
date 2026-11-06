package config

import (
	"strconv"
	"strings"

	"kizuna/engine/core/strutil"
)

// bu_viewer.go 解析旧式立绘鉴赏的两段配置：
//
//	[museum phase - bu]  角色选择页：底图 + 每个角色的点击目标
//	[bu viewer]          换装查看器：底图、开关组、服装/表情编号表
//
// 为什么单独一套：新版作品把整套界面写成 define.bsa 的 museum_buex_layout
// （JSON 方言），旧版作品（特別授業3SLG 旧版 tj3slg）的 define.bsa 里没有
// 这个资源，界面只存在于 bsxx.cfg。两套是并行的发行方式，不能互相替代：
// 用 buex 解析器去读 [bu viewer] 会拿到空布局，玩家点"立绘"页签没有反应。
//
// 编号表（wear / under wear / face pose）必须原样保留：它们直接拼进
// bu<NN><view><strip><wear><pose><variant><face> 图名，任何"聪明"的
// 重新编号都会让立绘和按钮错位。

const (
	MuseumBUSection = "museum phase - bu"
	BuViewerSection = "bu viewer"
)

// MuseumBUPage 是旧式立绘鉴赏的角色选择页。
type MuseumBUPage struct {
	Base     string
	HitTable string
	Targets  []MuseumBUTarget
	// 精灵图世代（満淫電車2）：Base 是 1022x726 的拼图，整张画出来会把
	// 全部角色卡和装饰铺到屏幕上。Atlas 打开时只画 CharactersBase、
	// 角色目标和标题条，背景用博物馆主底图。
	Atlas          bool
	Title          UIButton
	Menu           UIButton
	CharactersBase UIButton
	SelectedTarget UIButton
}

// MuseumBUTarget 是一个可点击的角色目标。
//
// Character 来自 `targetN no`，不是 targetN 的 N。tj3slg 的
// target3 no = 2（第 4 个目标仍指向 3 号角色），按下标取会打开错误的人。
type MuseumBUTarget struct {
	Button    UIButton
	Character int
}

// Empty 判断作品是否配了旧式立绘选择页。
func (p MuseumBUPage) Empty() bool {
	return p.Base == "" && len(p.Targets) == 0
}

// BuViewerSetting 是一组等间距排列的小开关。
//
// 与 museum_buex_layout 的 switchs 不同，旧配置用 settings = N 加
// settingN pos/space/repeat x 描述；repeat x 是列数，行数由开关数量推出来。
type BuViewerSetting struct {
	X, Y     int
	SpaceX   int
	SpaceY   int
	Columns  int
	Disabled bool
}

// BuViewerCharacter 是一个角色的服装/表情编号表。
type BuViewerCharacter struct {
	Wear      []int
	UnderWear int
	// Faces 是 face pose1/2/3 三段；下标与姿势 a/b/c 对应。
	Faces [][]int
}

// BuViewer 是旧式换装查看器的布局。
type BuViewer struct {
	Base       string
	BaseX      int
	BaseY      int
	OrderUp    UIButton
	OrderDown  UIButton
	Exit       UIButton
	OnOff      UIButton
	PosPointer UIButton
	// Switch 是开关按钮图（只有名字和帧数，坐标由每组 setting 给出）。
	Switch UIButton
	// PosRegion 是站位滑杆的活动区域 [左, 上, 宽, 高]。
	PosRegion [4]int
	// DefaultOrder 是角色图标的初始顺序（1 基角色编号）。
	DefaultOrder  []int
	Icons         []UIButton
	Settings      []BuViewerSetting
	PoseSignature string
	ViewSignature string
	NgngLevel     int
	NgngConvert   []int
	Characters    []BuViewerCharacter
	// EnableViewControl 对应 `enable view control`：关闭时不显示站位滑杆。
	EnableViewControl bool
	// Atlas 表示这是満淫電車2 的精灵图查看器：panel image 是控制面板，
	// 立绘由引擎按 bu<ch>u<wear><pose>a<stage> 现场拼名。
	Atlas bool
	// TransIcon 是右侧「着衣三段階切替」面板的开关图标。
	TransIcon UIButton
}

// Empty 判断作品是否配了旧式换装查看器。
func (v BuViewer) Empty() bool {
	if v.Base != "" || len(v.Icons) > 0 {
		return false
	}
	// parseBuViewer 在没有 [bu viewer] 段时也会按 settings=16 生成 16 个
	// Disabled 占位项；只看 len(Settings) 会把"没有这个界面"误判成有，
	// 随后 openLegacyBustupViewer 用空布局建页并在 groups[1:] 崩溃。
	for _, setting := range v.Settings {
		if !setting.Disabled {
			return false
		}
	}
	return true
}

func parseMuseumBUPage(document *IniDocument) MuseumBUPage {
	base := document.Get(MuseumBUSection, "base", "")
	page := MuseumBUPage{
		Base:     base,
		HitTable: document.Get(MuseumBUSection, "hit table", ""),
		Title:    atlasButtonOr(document, MuseumBUSection, "title", base),
		Menu:     atlasButtonOr(document, MuseumBUSection, "menu", base),
	}
	if characters, ok := parseAtlasButton(document, MuseumBUSection, "characters base", base); ok {
		page.CharactersBase = characters
	}
	if selected, ok := parseAtlasButton(document, MuseumBUSection, "selectedtarget", base); ok {
		page.SelectedTarget = selected
	}
	for index := 0; index < 16; index++ {
		button := atlasButtonOr(document, MuseumBUSection, "target"+strconv.Itoa(index), base)
		if button.Empty() {
			break
		}
		character := document.GetInt(MuseumBUSection, "target"+strconv.Itoa(index)+" no", index)
		page.Targets = append(page.Targets, MuseumBUTarget{Button: button, Character: character})
	}
	page.Atlas = page.CharactersBase.Atlas || len(page.Targets) > 0
	return page
}

func parseBuViewer(document *IniDocument) BuViewer {
	viewer := BuViewer{
		Base:       document.Get(BuViewerSection, "base", ""),
		OrderUp:    parseUIButton(document.Get(BuViewerSection, "order up", "")),
		OrderDown:  parseUIButton(document.Get(BuViewerSection, "order down", "")),
		Exit:       parseUIButton(document.Get(BuViewerSection, "exit button", "")),
		OnOff:      parseUIButton(document.Get(BuViewerSection, "on/off button", "")),
		PosPointer: parseUIButton(document.Get(BuViewerSection, "pos pointer", "")),
		PosRegion:  [4]int{0, 0, 0, 0},
	}
	viewer.BaseX, viewer.BaseY = parsePoint(document.Get(BuViewerSection, "base pos", ""))
	if panel := document.Get(BuViewerSection, "panel image", ""); panel != "" {
		viewer.Base = panel
		viewer.Atlas = true
		viewer.BaseX, viewer.BaseY = parsePoint(document.Get(BuViewerSection, "panel pos", ""))
	}
	if trans := document.Get(BuViewerSection, "trans icon image", ""); trans != "" {
		viewer.TransIcon = UIButton{Image: trans, Frames: max(1, document.GetInt(BuViewerSection, "trans icon max state", 2))}
		viewer.TransIcon.X, viewer.TransIcon.Y = parsePoint(document.Get(BuViewerSection, "trans icon pos", ""))
	}
	// `switch image = dress_view_switch, 3` 只给图名和帧数，没有坐标。
	if switchImage := parseUIButton(document.Get(BuViewerSection, "switch image", "")); !switchImage.Empty() {
		if states := document.GetInt(BuViewerSection, "switch max state", 0); states > 0 {
			switchImage.Frames = states
		}
		viewer.Switch = switchImage
	}
	if region := ParsePositions(document.Get(BuViewerSection, "pos region", "")); len(region) >= 4 {
		copy(viewer.PosRegion[:], region[:4])
	}
	if order := parseIntList(document.Get(BuViewerSection, "default order", "")); len(order) > 0 {
		viewer.DefaultOrder = order
	}
	viewer.EnableViewControl = document.GetInt(BuViewerSection, "enable view control", 0) != 0

	for index := 0; index < 16; index++ {
		icon := parseUIButton(document.Get(BuViewerSection, "character icon"+strconv.Itoa(index), ""))
		if icon.Empty() {
			break
		}
		viewer.Icons = append(viewer.Icons, icon)
	}

	// settings = 7 是数量；每一项的 pos 为负数（如 -16,-16）表示原作者
	// 把它移出画面（禁用），解析时保留哨兵值，由渲染层跳过。
	count := document.GetInt(BuViewerSection, "settings", 0)
	if count <= 0 {
		count = 16
	}
	limit := min(count, 16)
	// 按 setting0..N 的原始下标保存（缺项也占位）。
	//
	// 后面的渲染层用"组下标"取语义：0 ポーズ、1 視点、2 ぬぎぬぎ、
	// 3 服装、4 下着、5 表情。中间缺一项就跳过会让后面的组整体前移，
	// 开关会画到别的标签上。
	for index := 0; index < limit; index++ {
		prefix := "setting" + strconv.Itoa(index)
		value := document.Get(BuViewerSection, prefix+" pos", "")
		setting := BuViewerSetting{Disabled: true}
		if value != "" {
			setting.Disabled = false
			setting.X, setting.Y = parsePoint(value)
			if space := ParsePositions(document.Get(BuViewerSection, prefix+" space", "")); len(space) >= 2 {
				setting.SpaceX, setting.SpaceY = space[0], space[1]
			}
			setting.Columns = document.GetInt(BuViewerSection, prefix+" repeat x", 1)
			setting.Disabled = setting.X < 0 || setting.Y < 0
		}
		viewer.Settings = append(viewer.Settings, setting)
	}

	viewer.PoseSignature = document.Get(BuViewerSection, "pose signature", "ab")
	viewer.ViewSignature = document.Get(BuViewerSection, "view signature", "fu")
	viewer.NgngLevel = document.GetInt(BuViewerSection, "ngng level", 0)
	viewer.NgngConvert = parseIntList(document.Get(BuViewerSection, "ngng convert table", ""))

	for index := 1; index <= 16; index++ {
		prefix := "ch" + strconv.Itoa(index)
		// 必须有 `chN wear` 才算这个角色存在：不能只看 under wear，
		// 因为它的默认值是 1，会让读取循环一路生成十几套空角色。
		rawWear := document.Get(BuViewerSection, prefix+" wear", "")
		if rawWear == "" {
			break
		}
		wear := parseIntList(rawWear)
		under := document.GetInt(BuViewerSection, prefix+" under wear", 1)
		character := BuViewerCharacter{Wear: wear, UnderWear: under}
		for pose := 1; pose <= 3; pose++ {
			row := parseIntList(document.Get(BuViewerSection, prefix+" face pose"+strconv.Itoa(pose), ""))
			if len(row) == 0 {
				break
			}
			character.Faces = append(character.Faces, row)
		}
		viewer.Characters = append(viewer.Characters, character)
	}
	return viewer
}

// parseIntList 解析 "1, 2, 3, 0" 这类整数表，并去掉末尾的 0。
//
// 末尾的 0 在旧配置里表示"没有这一件"（ch1 wear = 1,2,3,0），
// 保留它会让服装开关多出一颗点不到的空白。
func parseIntList(value string) []int {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) == 0 {
		return nil
	}
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		out = append(out, number)
	}
	for len(out) > 0 && out[len(out)-1] == 0 {
		out = out[:len(out)-1]
	}
	return out
}
