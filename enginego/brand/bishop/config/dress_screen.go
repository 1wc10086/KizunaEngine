package config

import (
	"fmt"
	"strconv"
	"strings"
)

// dress_screen.go 解析原引擎"立ち絵 / 脱衣"设定画面的布局。
//
// 这块界面和存档画面、鉴赏室一样是原引擎自绘的：底图、左侧的模式/角色/脱衣
// 状态按钮、右侧的服装列表，全部由 bsxx.cfg 的 [dress screen] 给出图片名、
// 帧数和坐标，引擎只负责按坐标贴图并做命中判定。沿用同一份配置，换作品时
// 只需要换素材，不用改 Go 代码。

const DressScreenSection = "dress screen"

const (
	// DressModeLimit 是模式按钮（mode0…）的数量上限。
	DressModeLimit = 8
	// DressTargetLimit 是角色选择按钮（target0…）的数量上限。
	DressTargetLimit = 16
	// DressStateLimit 是通用脱衣状态按钮（common state0…）的数量上限。
	DressStateLimit = 16
	// DressCostumeCountLimit 是配置里服装按钮（dress00…）的总数上限。
	DressCostumeCountLimit = 64
	// DressCostumeLimit 是服装列表同时可见的条目上限。
	// 列表从 (844,127) 起、行距 30，最多 14 条才不会顶到画面下沿。
	DressCostumeLimit = 14
)

const (
	// DressStageCount 是脱衣阶段（ぬぎぬぎ段階）的数量。
	// 和 common state0…6 一一对应：着衣 / 半脱ぎ / トップレス / スカートオフ /
	// 下着＋ストッキング / 下着 / 全裸。配置里每个阶段各占一个数字。
	DressStageCount = 7
	// DressLevelLimit 是 levelN 按钮的编号上限（原配置最多用到三位数）。
	DressLevelLimit = 200
	// DressUnderWearCount 是"下着"（under wear）设置的条目数：ch1…ch5。
	DressUnderWearCount = 5
	// DressMaxStripMinimum / DressMaxStripMaximum 是 [bustup setting] 的
	// max strip 允许范围，范围之外一律按默认值处理。
	DressMaxStripMinimum = 1
	DressMaxStripMaximum = 9
	// DressDefaultMaxStrip 是 max strip 的默认值。
	DressDefaultMaxStrip = 7
)

// DressScreen 是 [dress screen] 的布局。
type DressScreen struct {
	// MK2 表示牝教師2 老版的ぬぎぬぎ設定：素材里没有 strip_base / costume_*
	// 界面图，引擎按 bu<角色><服装><阶段><表情> 的命名自绘角色卡、阶段
	// 按钮与服装切换（systemui_mk2_dress.go）。
	MK2 bool
	// Atlas 表示 bsxx.cfg 用的是 BSXScript 1.0 的精灵图写法（満淫電車2）：
	// 3 个着衣状态按钮 + 4 个透視模式按钮烘在 dress_base 里，没有角色/
	// 服装列表。状态对所有角色/服装统一生效。
	Atlas bool
	// Pens 是 4 个透視模式按钮：衣服消去/衣服復活/下着消去/下着復活。
	Pens []UIButton
	// Cursors 是拖拽透視用的两个鼠标指针帧。
	Cursors []UIButton
	// Base 是底图（strip_base），Base2 是盖在底图之上的第二层（strip_base2）。
	Base  string
	Base2 string
	// AlternateBase2 表示 base2 不是叠加层，而是"全体設定"页的第二张底图。
	// 教育指導 KS 的 dress_base（個別設定）与 dress_base2（ぬぎぬぎモード）
	// 是两张完整页面；同时画会得到两套重叠的面板。旧式 chNN 表格式
	// 是这一代的可靠指纹。
	AlternateBase2 bool
	// Modes 是模式按钮：一个角色 / 全部 / 仅查看。
	Modes []UIButton
	// Targets 是角色选择按钮。
	Targets []UIButton
	// States 是"通用脱衣状态"按钮：着衣 / 半脱ぎ / トップレス / …。
	States []UIButton
	// Back 是返回按钮。
	Back UIButton
	// Costumes 是服装按钮列表（全部服装 + 每个角色各自的服装）。
	Costumes []UIButton
	// DressPos 是服装列表的起点，DressSpace 是相邻两条之间的间距。
	DressPos   UIPoint
	DressSpace UIPoint
	// LevelPos/LevelSpace 是右侧脱衣阶段标签列（levelN）的起点与间距。
	// 教育指導 KS 只写这一组，不写 levelN 自己的坐标；KSIE 则相反，
	// 每个 levelN 都带绝对坐标。只有 LevelPos 非零时才按列排版。
	LevelPos   UIPoint
	LevelSpace UIPoint
	// WholeBodyPos 是全身立绘相对画面中心的偏移（"画面中央からの差分"）。
	WholeBodyPos UIPoint
	// ModeMenuType 是模式菜单的种类（mode menu type）。
	// 原引擎用它决定模式按钮是排成一列还是别的摆法，0 表示没有这个设定。
	ModeMenuType int
	// ShowBu 是否在画面上显示半身立绘（show bu）。
	ShowBu bool
	// EnableViewControl 是否启用视角操作控件（enable view control）。
	EnableViewControl bool
	// ShowCommonButton 是否显示"通用脱衣状态"按钮（show common button）。
	ShowCommonButton bool
	// EnableInReplay 是否允许在回想（replay）里打开这个画面
	// （enable in replay mode）。
	EnableInReplay bool
	// MaxStrip 是脱衣阶段数（[bustup setting] 的 max strip），
	// 阶段编号是 0…(max strip - 1)。
	MaxStrip int
	// InitNgLevel 是鉴赏室"立ち絵"页签的初始脱衣阶段
	// （[museum phase] 的 init ng level），没配置时是 0。
	InitNgLevel int
	// UnderWear 是每个角色的"下着"设定（ch1 under wear…ch5 under wear），
	// 下标 0 对应 1 号角色。
	UnderWear []int
	// StripConvert 是脱衣阶段 → 档位编号的换算表（ngng convert table）。
	// 表为空时按 0…(max strip - 1) 的恒等映射处理。
	StripConvert []int
	// DressCostumes 是每个角色每件服装的脱衣阶段表，
	// 顺序与配置文件里的书写顺序一致。
	DressCostumes []DressCostume
	// Levels 是 levelN 按钮（脱衣阶段的立绘档位），顺序同配置文件。
	Levels []DressLevel
	// View 是画面上的视角操作控件（滑杆 + 脸 / 胸 / 股间按钮）。
	View DressViewControl
}

// DressCostume 是一条"角色 × 服装"的脱衣阶段表。
//
// 原引擎把"这件衣服在每个脱衣阶段该画哪张图"写在 [dress screen] 的
// chNN dressNN 表里，例如：
//
//	ch01 dress01 = 1, 1, 3, 2, 4, 5, 6, 7, 1
//
// 依次是：服装编号、服装图编号、然后各脱衣阶段各自对应的档位编号。
// 这张表是纯数据驱动的东西——每部作品的阶段顺序和素材编号都不一样，
// 所以这里只原样保存配置里的编号，画的时候再拿编号去查档位按钮。
//
// Stages 的下标是"阶段号"而不是"配置里的第几列"：多数作品是连续的
// 0..6，SL / SLIE 则是 0,1,2,3,5（没有 4 号阶段），解析时按 common state
// 的实际编号对位，见 parseDressCostumeTables。
type DressCostume struct {
	// Character 是角色编号（1 基），来自键里的 chNN。
	Character int
	// Wear 是服装编号（1 基）。0 是"全部服装"这一共用条目：
	// 它对所有角色都成立，选中它表示"所有服装一起看"。
	Wear int
	// DressImage 是这条服装对应的服装按钮编号（dress00… 的序号，0 基），
	// 也就是要画哪一张服装缩略图。0 表示用"全部服装"那张图。
	DressImage int
	// Stages 是 7 个脱衣阶段对应的档位编号（levelN 的 N，1 基）。
	// 0 表示这个阶段没有对应素材（这件衣服在该阶段不适用）。
	Stages [7]int
}

// DressLevel 是一个脱衣阶段档位按钮（levelN）。
type DressLevel struct {
	// Index 是档位编号（levelN 的 N，1 基）。
	Index int
	// Button 是这一档的按钮图（帧数与坐标）。
	Button UIButton
}

// DressViewControl 是画面右侧的视角操作控件。
//
// 原引擎里这套控件（滑杆 + 脸 / 胸 / 股间三个按钮）和自由视角画面
// 用的是同一组图，因此这里按同样的键名解析。
type DressViewControl struct {
	// Enabled 是否显示这套控件（对应 enable view control）。
	Enabled bool
	// MeterX / MeterY 是滑杆轨道左上角，MeterW / MeterH 是轨道尺寸。
	MeterX int
	MeterY int
	MeterW int
	MeterH int
	// Knob 是滑杆上的滑块。
	Knob UIButton
	// Face / Bust / Hip 分别是移到脸 / 胸 / 股间的按钮。
	Face UIButton
	Bust UIButton
	Hip  UIButton
}

// Empty 判断作品有没有配这个画面。
func (d DressScreen) Empty() bool {
	return len(d.Base) == 0 && len(d.Base2) == 0 && len(d.Costumes) == 0 &&
		len(d.States) == 0 && len(d.Pens) == 0
}

// DressCostumeCode 从服装图名解析角色编号与服装编号。
//
// 服装图名是 costume_<角色><编号>：costume_101 是 1 号角色的 01 号服装，
// costume_507 是 5 号角色的 07 号服装。共用的"全部服装"按钮（costume_allcos）
// 没有数字，返回 ok=false。
func DressCostumeCode(image string) (character int, code int, ok bool) {
	const prefix = "costume_"
	if len(image) <= len(prefix) {
		return 0, 0, false
	}

	suffix := image[len(prefix):]
	for index := 0; index < len(suffix); index++ {
		if suffix[index] < '0' || suffix[index] > '9' {
			return 0, 0, false
		}
	}
	if len(suffix) < 3 {
		return 0, 0, false
	}

	character = int(suffix[0] - '0')
	// 编号是后两位：101 → 01、110 → 10。
	for index := len(suffix) - 2; index < len(suffix); index++ {
		code = code*10 + int(suffix[index]-'0')
	}
	return character, code, true
}

func parseDressScreen(document *IniDocument) DressScreen {
	base := document.Get(DressScreenSection, "base", "")
	if state0, ok := parseAtlasButton(document, DressScreenSection, "state0", base); ok && state0.Atlas {
		// 精灵图世代：只解析实际存在的三个状态与四个透視模式。
		screen := DressScreen{Base: base, Atlas: true}
		screen.Back = atlasButtonOr(document, DressScreenSection, "back", base)
		for index := 0; index < 3; index++ {
			if button, ok := parseAtlasButton(document, DressScreenSection, "state"+itoa(index), base); ok {
				screen.States = append(screen.States, button)
			}
		}
		for index := 0; index < 4; index++ {
			if button, ok := parseAtlasButton(document, DressScreenSection, "pen"+itoa(index), base); ok {
				screen.Pens = append(screen.Pens, button)
			}
		}
		for index := 0; index < 2; index++ {
			if button, ok := parseAtlasButton(document, DressScreenSection, "cursor"+itoa(index), base); ok {
				screen.Cursors = append(screen.Cursors, button)
			}
		}
		return screen
	}
	screen := DressScreen{
		Base:  base,
		Base2: document.Get(DressScreenSection, "base2", ""),
		Back:  parseUIButton(document.Get(DressScreenSection, "back button", "")),
	}
	screen.Modes = parseDressButtons(document, "mode", DressModeLimit)
	screen.Targets = parseDressButtons(document, "target", DressTargetLimit)
	states, stateIDs := parseDressStateButtons(document)
	screen.States = states
	screen.Costumes = parseDressCostumes(document)

	if point, ok := parseUIPoint(document.Get(DressScreenSection, "dress pos", "")); ok {
		screen.DressPos = point
	} else {
		screen.DressPos = UIPoint{X: 844, Y: 127}
	}
	if point, ok := parseUIPoint(document.Get(DressScreenSection, "dress space", "")); ok {
		screen.DressSpace = point
	} else {
		screen.DressSpace = UIPoint{X: 0, Y: 30}
	}
	if point, ok := parseUIPoint(document.Get(DressScreenSection, "level pos", "")); ok {
		screen.LevelPos = point
	}
	if point, ok := parseUIPoint(document.Get(DressScreenSection, "level space", "")); ok {
		screen.LevelSpace = point
	}
	if point, ok := parseUIPoint(document.Get(DressScreenSection, "whole body pos", "")); ok {
		screen.WholeBodyPos = point
	}

	// 服装按钮的位置不取各自写在配置里的坐标：只有第一条（全部服装）写了坐标，
	// 那是原引擎给它的特例；整列按钮按 dress pos + 间距依次排开。
	for index := range screen.Costumes {
		screen.Costumes[index].X = screen.DressPos.X + index*screen.DressSpace.X
		screen.Costumes[index].Y = screen.DressPos.Y + index*screen.DressSpace.Y
	}

	// 开关类的键都是 0/1，非 0 即为真：不同作品写的值不统一，
	// 所以统一按"非零"处理。
	screen.EnableInReplay = document.GetInt(DressScreenSection, "enable in replay mode", 0) != 0
	screen.ModeMenuType = document.GetInt(DressScreenSection, "mode menu type", 0)
	// show bu 缺省时按"有立绘素材就显示"处理：教育指導 KS 不写这一项，
	// 但 DRESS 画面的主体就是 200x575 的全身立绘；旧默认 false 会让中间
	// 整块空白。只有明确写 0 的作品才隐藏。
	screen.ShowBu = len(screen.Costumes) > 0 || len(screen.Levels) > 0
	if document.Has(DressScreenSection, "show bu") {
		screen.ShowBu = document.GetInt(DressScreenSection, "show bu", 0) != 0
	}
	screen.EnableViewControl = document.GetInt(DressScreenSection, "enable view control", 0) != 0
	// show common button 在 SL 的配置里根本没写，但同一个段里配了
	// common state0..3/5。原引擎的默认值是"有阶段按钮就显示"：只有明确写
	// 0 才收起面板；完全没有 common state 的作品仍然保持 false。
	screen.ShowCommonButton = len(screen.States) > 0
	if document.Has(DressScreenSection, "show common button") {
		screen.ShowCommonButton = document.GetInt(DressScreenSection, "show common button", 0) != 0
	}

	// max strip 与 init ng level 不在 [dress screen] 段里，但都是这个画面的
	// 数据，所以从同一份配置里按段名取。
	screen.MaxStrip = parseDressMaxStrip(document)
	screen.InitNgLevel = document.GetInt(MuseumSection, "init ng level", 0)
	screen.StripConvert = parseDressStripConvert(document, screen.MaxStrip)
	screen.UnderWear = parseDressUnderWear(document)
	screen.DressCostumes, screen.AlternateBase2 = parseDressCostumeTables(document, stateIDs, screen.Costumes)
	screen.Levels = parseDressLevels(document)
	screen.View = parseDressViewControl(document)
	return screen
}

// parseDressStateButtons 读取 common stateN 按钮，保留编号里的空档。
//
// SL / SLIE 的状态键是 0,1,2,3,5——没有 state4，因为这两作的立绘阶段号
// 本身就是 0,1,2,3,5（bu01u5011a 而不是 u4011a）。旧实现遇到第一个空项
// 就 break，state5 整颗按钮被丢掉，玩家永远选不到最后的"下着"阶段。
//
// 返回的 buttons 用空占位保留编号，stateIDs 记录实际存在的编号：渲染/命中
// 仍然按 layout.States[index] 取，阶段表则按 stateIDs 把 5 个值对到
// 0,1,2,3,5 上。
func parseDressStateButtons(document *IniDocument) ([]UIButton, []int) {
	buttons := make([]UIButton, DressStateLimit)
	ids := make([]int, 0, DressStateLimit)
	last := -1
	for index := 0; index < DressStateLimit; index++ {
		button := parseUIButton(document.Get(DressScreenSection, "common state"+strconv.Itoa(index), ""))
		if button.Empty() {
			continue
		}
		buttons[index] = button
		ids = append(ids, index)
		last = index
	}
	if last < 0 {
		return nil, nil
	}
	return buttons[:last+1], ids
}

// parseDressMaxStrip 读取 [bustup setting] 的 max strip。
//
// 它决定脱衣阶段的个数（0…max strip - 1）。原配置允许写 1～9，
// 没写或写了越界的值都按默认值 7 处理：阶段数错了会让阶段表整体错位，
// 比"少几个阶段"严重得多。
func parseDressMaxStrip(document *IniDocument) int {
	value := document.GetInt(BustupSection, "max strip", 0)
	if value < DressMaxStripMinimum || value > DressMaxStripMaximum {
		return DressDefaultMaxStrip
	}
	return value
}

// parseDressStripConvert 读取脱衣阶段 → 档位编号的换算表。
//
// 表没写时用恒等映射（阶段 i 对应档位 i）：这是最常见的默认关系，
// 写死一个"空表 = 没有换算"会让阶段表整体指向不存在的档位。
func parseDressStripConvert(document *IniDocument, maxStrip int) []int {
	convert := ParsePositions(document.Get(DressScreenSection, "ngng convert table", ""))
	if len(convert) > 0 {
		return convert
	}

	identity := make([]int, 0, maxStrip)
	for index := 0; index < maxStrip; index++ {
		identity = append(identity, index)
	}
	return identity
}

// parseDressUnderWear 读取每个角色的"下着"设定（ch1 under wear…ch5 under wear）。
//
// 键的下标从 1 开始（ch1 对应 1 号角色），这里转成 0 基数组，
// 调用方按角色编号 - 1 取值。
func parseDressUnderWear(document *IniDocument) []int {
	underWear := make([]int, DressUnderWearCount)
	for index := 0; index < DressUnderWearCount; index++ {
		key := "ch" + strconv.Itoa(index+1) + " under wear"
		underWear[index] = document.GetInt(DressScreenSection, key, 0)
	}
	return underWear
}

// parseDressCostumeTables 读取 chNN dressNN 表。
//
// 按配置文件里的书写顺序收集：每个角色自己的服装表是一段，
// 顺序错了会把某个角色的服装画到别的角色身上。
//
// 值的第 3 项起是各脱衣阶段对应的档位编号。多数作品的阶段号是连续的
// 0..6，直接按顺序放；SL / SLIE 的阶段号是 0,1,2,3,5（没有 4），而配置
// 只写 5 个值，必须按实际存在的 common state 编号对位，否则最后的
// 阶段会被放到不存在的 4 号上。
func parseDressCostumeTables(document *IniDocument, stateIDs []int, costumes []UIButton) ([]DressCostume, bool) {
	result := make([]DressCostume, 0, DressCostumeCountLimit)
	legacy := false
	for _, entry := range document.Section(DressScreenSection) {
		character, dress, ok := parseDressCostumeKey(entry.Key)
		if !ok {
			continue
		}

		values := ParsePositions(entry.Value)
		if len(values) < 2 {
			continue
		}

		costume := DressCostume{Character: character}
		var stageValues []int
		switch {
		case len(stateIDs) > 1 && len(values) == len(stateIDs)+2:
			// 新式（KSIE / UKIE）：服装编号, 服装图编号, 阶段0…N。
			costume.Wear = values[0]
			costume.DressImage = values[1]
			stageValues = values[2:]
		case len(stateIDs) == 6 && len(values) == len(stateIDs)+1 && values[1] == 1 && values[0] >= 0 && values[0] < len(costumes):
			// 旧式（教育指導 KS）：键里的 dressNN 本身就是服装编号，
			// 首值是服装图编号，后面直接跟 6 个阶段的档位。
			// 例：ch01 dress01 = 18, 1, 0, 0, 9, 10, 11
			//     -> 服装 1（制服）, 图 dress18, 阶段 1/0/0/9/10/11。
			// 旧实现把 18 当成服装编号、1 当成图号，整列服装都变成同一张图，
			// 立绘也找不到对应服装。
			legacy = true
			costume.Wear = dress
			costume.DressImage = values[0]
			stageValues = values[1:]
			if dressCostumeIsAll(costumes, costume.DressImage) {
				// "全服装一括"在原版里是 wear 0，不是列表末尾的编号。
				costume.Wear = 0
			}
		default:
			costume.Wear = values[0]
			costume.DressImage = values[1]
			stageValues = values[2:]
		}

		// 阶段值按 common state 的实际编号对位；数量对不上时按 0..n-1。
		stages := stateIDs
		if !dressStageIDsUsable(stages, len(stageValues)) && len(stageValues) != len(stages) {
			// 数量对不上（缺项/旧表）时退回"第 i 个值就是第 i 阶段"。
			stages = identityDressStages(len(stageValues))
		}
		for index := 0; index < len(stages) && index < len(stageValues); index++ {
			stage := stages[index]
			if stage < 0 || stage >= DressStageCount {
				continue
			}
			costume.Stages[stage] = stageValues[index]
		}
		result = append(result, costume)
	}
	return result, legacy
}

// dressCostumeIsAll 判断服装图是不是"全服装一括"。
//
// KS 的旧式表没有显式 wear 0；只有图名里带 ikkatsu/allcos 的那一条是
// 一括项。KSIE 的 costume_allcos 也是同一语义。
func dressCostumeIsAll(costumes []UIButton, index int) bool {
	if index < 0 || index >= len(costumes) {
		return false
	}
	name := strings.ToLower(costumes[index].Image)
	return strings.Contains(name, "ikkatsu") || strings.Contains(name, "allcos")
}

// dressStageIDsUsable 判断"状态编号表"能否直接拿来对位阶段值。
//
// 只有"存在空档（如 0,1,2,3,5）且数量恰好等于阶段值个数"时才用；
// 连续编号或数量对不上都按 0..n-1 处理，避免把老配置的值挪到别的阶段。
func dressStageIDsUsable(ids []int, values int) bool {
	if len(ids) == 0 || values <= 0 || len(ids) != values {
		return false
	}
	for index, id := range ids {
		if id != index {
			return true
		}
	}
	return false
}

func identityDressStages(count int) []int {
	stages := make([]int, 0, count)
	for index := 0; index < count && index < DressStageCount; index++ {
		stages = append(stages, index)
	}
	return stages
}

// parseDressCostumeKey 解析 "ch01 dress12" 这样的键。
//
// 键里的 chNN 是角色编号（1 基），dressNN 是这个角色自己的第几条服装；
// 真正的服装编号与服装图在值里。
func parseDressCostumeKey(key string) (character int, dress int, matched bool) {
	text := strings.ToLower(strings.TrimSpace(key))
	if !strings.HasPrefix(text, "ch") {
		return 0, 0, false
	}

	rest := text[2:]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, 0, false
	}
	parsed, ok := parseInt(rest[:digits])
	if !ok || parsed < 1 {
		return 0, 0, false
	}
	character = parsed

	rest = strings.TrimSpace(rest[digits:])
	if !strings.HasPrefix(rest, "dress") {
		return 0, 0, false
	}
	rest = rest[len("dress"):]
	digits = 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, 0, false
	}
	parsed, ok = parseInt(rest[:digits])
	if !ok {
		return 0, 0, false
	}
	return character, parsed, true
}

// parseDressLevels 读取 levelN 按钮（脱衣阶段的立绘档位）。
//
// 编号和按钮图都来自配置：档位图是竖排帧条，坐标也各不相同，
// 不能按固定位置画。
func parseDressLevels(document *IniDocument) []DressLevel {
	levels := make([]DressLevel, 0, DressLevelLimit)
	for _, entry := range document.Section(DressScreenSection) {
		index, ok := parseDressLevelKey(entry.Key)
		if !ok {
			continue
		}

		button := parseUIButton(entry.Value)
		if button.Empty() {
			continue
		}
		levels = append(levels, DressLevel{Index: index, Button: button})
	}
	return levels
}

// parseDressLevelKey 解析 "level12" 这样的键，返回档位编号。
func parseDressLevelKey(key string) (int, bool) {
	text := strings.ToLower(strings.TrimSpace(key))
	if !strings.HasPrefix(text, "level") {
		return 0, false
	}

	rest := text[len("level"):]
	if len(rest) == 0 {
		return 0, false
	}
	for index := 0; index < len(rest); index++ {
		if rest[index] < '0' || rest[index] > '9' {
			return 0, false
		}
	}

	index, ok := parseInt(rest)
	if !ok || index < 1 || index > DressLevelLimit {
		return 0, false
	}
	return index, true
}

// parseDressViewControl 读取画面上的视角操作控件。
//
// [dress screen] 里没有写这些键时沿用 [free-sight screen] 的：
// 视角操作本来就是自由视角那套控件，脱衣画面复用同一组图与坐标。
func parseDressViewControl(document *IniDocument) DressViewControl {
	view := DressViewControl{
		Enabled: document.GetInt(DressScreenSection, "enable view control", 0) != 0,
		Knob:    parseUIButton(dressViewValue(document, "knob")),
		Face:    parseUIButton(dressViewValue(document, "face button")),
		Bust:    parseUIButton(dressViewValue(document, "bust button")),
		Hip:     parseUIButton(dressViewValue(document, "hip button")),
	}
	view.MeterX, view.MeterY, view.MeterW, view.MeterH = dressMeterRegion(document)
	return view
}

// dressViewValue 先看 [dress screen]，没有再看 [free-sight screen]。
func dressViewValue(document *IniDocument, key string) string {
	if value := document.Get(DressScreenSection, key, ""); len(value) > 0 {
		return value
	}
	return document.Get(FreeSightSection, key, "")
}

// dressMeterRegion 把 "meter region" 换算成滑杆轨道的左上角与尺寸。
//
// 配置里写的是"左, 上, 右, 下"四个值（原引擎用它裁剪轨道），
// 这里统一换算成 x / y / 宽 / 高，界面层不用再自己做一次减法。
func dressMeterRegion(document *IniDocument) (x int, y int, width int, height int) {
	region := ParsePositions(dressViewValue(document, "meter region"))
	if len(region) < 4 {
		return 0, 0, 0, 0
	}

	left, top, right, bottom := region[0], region[1], region[2], region[3]
	width = right - left
	height = bottom - top
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return left, top, width, height
}

// CostumeFor 查某个角色某件服装的脱衣阶段表。
//
// wear 为 0 时找的是"全部服装"这一共用条目。每个角色各有一条 wear 为 0 的
// 条目（服装编号那一列写 0，原配置里注释成"一括登録"），但它们指向的
// 阶段表并不相同，所以先找这个角色自己那一条；角色自己没有时再退回任意
// 一条共用的条目。注意查询的 wear 也必须是 0：共用条目不能顶替具体的
// 服装号。找不到返回 ok=false。
func (d DressScreen) CostumeFor(character, wear int) (DressCostume, bool) {
	for index := range d.DressCostumes {
		costume := d.DressCostumes[index]
		if costume.Wear == wear && costume.Character == character {
			return costume, true
		}
	}

	if wear != 0 {
		return DressCostume{}, false
	}

	for index := range d.DressCostumes {
		costume := d.DressCostumes[index]
		if costume.Wear == 0 {
			return costume, true
		}
	}
	return DressCostume{}, false
}

// LevelButton 取档位编号对应的按钮（levelN 的 N，1 基）。
func (d DressScreen) LevelButton(index int) (UIButton, bool) {
	for _, level := range d.Levels {
		if level.Index == index {
			return level.Button, true
		}
	}
	return UIButton{}, false
}

// StageLevel 返回某个角色某件服装在第 stage 个脱衣阶段对应的档位编号。
//
// 阶段编号是 0 基的（和 common state0…6 对应）。这一阶段没有素材时返回 0，
// 调用方据此跳过该阶段，而不是去画一张不存在的图。
func (d DressScreen) StageLevel(character, wear, stage int) int {
	if stage < 0 || stage >= DressStageCount {
		return 0
	}

	costume, ok := d.CostumeFor(character, wear)
	if !ok {
		return 0
	}
	return costume.Stages[stage]
}

// StageAvailable 判断这件服装在这个脱衣阶段上有没有对应素材。
func (d DressScreen) StageAvailable(character, wear, stage int) bool {
	return d.StageLevel(character, wear, stage) != 0
}

// parseDressButtons 读取一组编号按钮（mode0、target0、common state0…）。
func parseDressButtons(document *IniDocument, prefix string, limit int) []UIButton {
	buttons := make([]UIButton, 0, limit)
	for index := 0; index < limit; index++ {
		button := parseUIButton(document.Get(DressScreenSection, prefix+strconv.Itoa(index), ""))
		if button.Empty() {
			break
		}
		buttons = append(buttons, button)
	}
	return buttons
}

// parseDressCostumes 读取服装按钮（dress00…dress50）。
//
// 键是两位补零的（dress00、dress01…），但不同作品的编号位数不一定一样，
// 所以补零的键找不到时再按不补零的键试一次。
func parseDressCostumes(document *IniDocument) []UIButton {
	buttons := make([]UIButton, 0, DressCostumeCountLimit)
	for index := 0; index < DressCostumeCountLimit; index++ {
		button := parseUIButton(document.Get(DressScreenSection, fmt.Sprintf("dress%02d", index), ""))
		if button.Empty() {
			button = parseUIButton(document.Get(DressScreenSection, "dress"+strconv.Itoa(index), ""))
		}
		if button.Empty() {
			break
		}
		buttons = append(buttons, button)
	}
	return buttons
}
