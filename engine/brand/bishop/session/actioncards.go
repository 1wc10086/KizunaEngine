package session

import (
	"strings"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// actioncards.go 处理学園3/学園3IE 的「行動カード」网格摆位。
//
// 剧本在正常等级下每个角色画一张卡，`#layercount` 也正好每张 +1；
// 但部分等级分支（例如 #redim_lv >= 60）一次画两张卡，脚本只把
// #layercount 加了 1。后面的 LAYER_PARAM 摆位又按 #layercount 选网格，
// 于是实际卡数比脚本认为的多，后几张卡会停在 x=-800 的屏幕外。
//
// 原引擎的网格本身是按“实际卡数”排的（脚本里 4…9 张的分支坐标都在，
// 见 map_main 末尾的 LAYER_PARAM 列表），所以这里在进入选择态前，
// 对行动卡按实际数量重新套用同一张网格表：正常 4/5 张时脚本已经摆好，
// 不动；6…9 张时补齐所有卡。只对存在 #action_point 的作品生效，
// 不影响其它用 custom map 的作品。

// actionCardPoint 是一张行动卡左上角的屏幕坐标。
type actionCardPoint struct{ x, y int }

// actionCardLayout 返回 n 张行动卡时的卡位表。
//
// 坐标全部来自学園3/学園3IE 的 map_main：
//
//	4 张：y=154，x=144+128i
//	5 张：y=154，x=80+128i
//	6 张：两行各 3 张（186/336/486，y=54/254）
//	7 张：上行 3 张，下行 4 张（144/272/400/528，y=254）
//	8 张：两行各 4 张
//	9 张：上行 4 张，下行 5 张（80/208/336/464/592，y=254）
//
// 1…3 张在剧本里没有分支；按 4 张的起点左对齐，保证至少能看见。
func actionCardLayout(count int) []actionCardPoint {
	row := func(xs []int, y int) []actionCardPoint {
		out := make([]actionCardPoint, 0, len(xs))
		for _, x := range xs {
			out = append(out, actionCardPoint{x: x, y: y})
		}
		return out
	}
	three := []int{186, 336, 486}
	four := []int{144, 272, 400, 528}
	five := []int{80, 208, 336, 464, 592}

	switch count {
	case 4:
		return row(four, 154)
	case 5:
		return row(five, 154)
	case 6:
		return append(row(three, 54), row(three, 254)...)
	case 7:
		return append(row(three, 54), row(four, 254)...)
	case 8:
		return append(row(four, 54), row(four, 254)...)
	case 9:
		return append(row(four, 54), row(five, 254)...)
	case 1, 2, 3:
		return row(four[:count], 154)
	}
	return nil
}

// arrangeActionCards 在所有行动卡都画完后，按实际卡数重排卡位。
//
// 只在“卡数 > 5”且存在卡仍停在屏幕外时动手：4/5 张时脚本自己的
// LAYER_PARAM 已经摆好，重排反而可能和动画/顺序不一致。
func (s *Session) arrangeActionCards() {
	if s.script == nil || s.gameState == nil {
		return
	}
	// #action_point 是这一套行动系统的唯一标识；其它 custom map 作品
	// 即使碰巧有 cardXX 图，也不会被这条路径碰到。
	if s.script.GetPropertyIndex("#action_point") < 0 {
		return
	}
	buttons := s.gameState.Buttons()
	cards := make([]state.StageButton, 0, len(buttons))
	offscreen := false
	for _, button := range buttons {
		if button.Layer < 6 || !strings.HasPrefix(strings.ToLower(button.Image), "card") {
			continue
		}
		cards = append(cards, button)
		if button.X < 0 {
			offscreen = true
		}
	}
	if len(cards) <= 5 || !offscreen {
		return
	}
	layout := actionCardLayout(len(cards))
	if len(layout) != len(cards) {
		return
	}
	for index, point := range layout {
		layer := cards[index].Layer
		s.gameState.MoveButtonByLayer(layer, point.x, point.y)
		s.gameState.MoveLayerTo(layer, point.x, point.y)
	}
	core.Logf("script: arranged %d action card(s) on the %d-card grid", len(cards), len(cards))
}
