package config

import (
	"strconv"

	"kizuna/engine/core/strutil"
)

// TargetSelectSection 是 [target select] 段名。
const TargetSelectSection = "target select"

// target_select.go 解析 [target select]（教育指導 KS 的放学目标选择）。
//
// 这一代的剧本不在脚本里画目标卡：leaveschool_select 先用 #result 的位掩码
// 标记"今天哪几个人可攻略"，再发 SELECT_TAIL "target"。原引擎按掩码把
// target0…target11 的卡片横排画在 (340,142) 附近，点哪张就把卡片值写回
// #result，脚本随后按 1..9 分派到对应的放学事件。
//
// KSIE 把同一屏改成脚本自绘的 [custom map phase]，因此这份配置只对 KS 生效；
// 段缺失的作品不会进入目标选择分支。
type TargetSelect struct {
	Cards []TargetSelectCard
	// WorkArea 是卡片可用的屏幕区域（x, y, w, h），用于把整排卡片居中。
	WorkArea [4]int
	// CardWidth/CardHeight 是单张卡片的裁切尺寸。
	CardWidth  int
	CardHeight int
	// Max 是原版一屏最多显示的卡片数；超过时靠左右移动查看。
	Max int
	// SelectedX/SelectedY 是选中卡片（单张时）或整排起点的坐标。
	SelectedX int
	SelectedY int
	// OK/Cancel 是确认/取消按钮。KS 的目标卡是"点卡即选"，这两个按钮在
	// 目标选择里不绘制；保留解析结果是为了兼容同代可能出现的确认框。
	OK     UIButton
	Cancel UIButton
}

// TargetSelectCard 是一张目标卡。
type TargetSelectCard struct {
	Image string
	Value int
}

func (t TargetSelect) Empty() bool {
	return len(t.Cards) == 0 || t.CardWidth <= 0 || t.CardHeight <= 0
}

func parseTargetSelect(document *IniDocument) TargetSelect {
	selectScreen := TargetSelect{
		OK:     parseUIButton(document.Get(TargetSelectSection, "ok", "")),
		Cancel: parseUIButton(document.Get(TargetSelectSection, "cancel", "")),
		Max:    document.GetInt(TargetSelectSection, "max", 6),
	}
	if size := ParsePositions(document.Get(TargetSelectSection, "card size", "")); len(size) >= 2 {
		selectScreen.CardWidth, selectScreen.CardHeight = size[0], size[1]
	}
	if area := ParsePositions(document.Get(TargetSelectSection, "work area", "")); len(area) >= 4 {
		copy(selectScreen.WorkArea[:], area[:4])
	}
	selectScreen.SelectedX, selectScreen.SelectedY = parsePoint(
		document.Get(TargetSelectSection, "selected card pos", ""))
	for index := 0; index < 12; index++ {
		value := document.Get(TargetSelectSection, "target"+strconv.Itoa(index), "")
		parts := strutil.SplitTrimNonEmpty(value, ",")
		if len(parts) < 2 {
			continue
		}
		number, ok := parseInt(parts[1])
		if !ok {
			continue
		}
		selectScreen.Cards = append(selectScreen.Cards, TargetSelectCard{
			Image: parts[0],
			Value: number,
		})
	}
	if selectScreen.Max <= 0 {
		selectScreen.Max = 6
	}
	return selectScreen
}
