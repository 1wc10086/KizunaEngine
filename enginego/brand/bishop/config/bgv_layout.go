package config

import (
	"sort"
	"strings"
)

// bgv_layout.go 解析 define.bsa 里的 bgv_layout（BGV 鉴赏室的布局）。
//
// 文件格式和 system_config_ex.layout 完全一致：键可以不写引号，键值对之间没有
// 逗号，';' 之后是注释，坐标可以写成 "4+204*0" 这样的算式。因此直接复用
// config_layout.go 里的通用解析器。
//
// 唯一的额外难点是曲目表（list）：它是有序的（"吐息初期１" 必须排在
// "吐息初期２" 前面），而通用解析器把对象存成 map，顺序会丢。顺序错乱会让
// 玩家以为游戏文件被重排过，所以这里在解析之后另外扫一遍 token 流，把每个
// list 的键值对按文档顺序取回来。

// BGVLayout 是 bgv_layout 的顶层结构。
type BGVLayout struct {
	// Base 是底图（overlay_mode.base）。
	Base string
	// Close 是关闭按钮（overlay_mode.close）。
	Close ConfigButton
	// Control 是全局控制条（stop/min/max/volume）。
	Control BGVControl
	// Font 是列表文字的字号/行距。
	Font BGVFont
	// Characters 是各角色分栏，顺序和布局文件里一样。
	Characters []BGVCharacter
}

// BGVFont 是 font 段；Block 是列表一行的字号和行距（[22, 22]）。
type BGVFont struct {
	Block [2]int
}

// BGVControl 是一组控制按钮：全局一组，每个角色各一组。
type BGVControl struct {
	Stop   ConfigButton
	Min    ConfigButton
	Max    ConfigButton
	Volume BGVVolume
	// ScrollBar 只有角色分栏里才有（滚动条）。
	ScrollBar ConfigButton
}

// BGVVolume 是音量条：底图按钮 + 可点击/可拖动的区域。
type BGVVolume struct {
	Button    ConfigButton
	HitX      int
	HitY      int
	HitWidth  int
	HitHeight int
}

// BGVView 是曲目的显示区域。
type BGVView struct {
	// X/Y 是列表左上角；Width 是可用宽度；Height 是每行的高度。
	X      int
	Y      int
	Width  int
	Height int
	// OffsetX/OffsetY 是列表内容的整体偏移。
	OffsetX int
	OffsetY int
	// IntervalStart/IntervalCount 对应 interval:[0, 25]：从第 0 条开始显示 25 条。
	IntervalStart int
	IntervalCount int
	// Lines 是可见行数。
	Lines int
}

// BGVCharacter 是角色的一栏：控制按钮 + 曲目表。
type BGVCharacter struct {
	// Signature 是角色编号（"01".."05"），和音频文件名的前缀一致。
	Signature string
	Control   BGVControl
	View      BGVView
	// Tracks 是按显示顺序排好的曲目。
	Tracks []BGVTrack
}

// BGVTrack 是一条 BGV 曲目：显示名（原文） + 音频文件名。
type BGVTrack struct {
	Name string
	File string
}

// LoadBGVLayout 解析整个 bgv_layout 文档。
//
// 只有语法错误才返回 error；缺失的可选段会得到零值。
func LoadBGVLayout(data []byte) (*BGVLayout, error) {
	root, err := ParseLayoutDocument(data)
	if err != nil {
		return nil, err
	}

	layout := &BGVLayout{}
	if overlay, ok := Object(root["overlay_mode"]); ok {
		layout.Base, _ = Text(overlay["base"])
		if button, ok := ParseButton(overlay["close"]); ok {
			layout.Close = button
		}
	}
	if control, ok := Object(root["control"]); ok {
		layout.Control = parseBGVControl(control)
	}
	if font, ok := Object(root["font"]); ok {
		if block, ok := Array(font["block"]); ok {
			if len(block) > 0 {
				layout.Font.Block[0], _ = Number(block[0])
			}
			if len(block) > 1 {
				layout.Font.Block[1], _ = Number(block[1])
			}
		}
	}

	characters, _ := Array(root["characters"])
	ordered := orderedBGVLists(data)
	for index, item := range characters {
		object, ok := Object(item)
		if !ok {
			continue
		}
		character := BGVCharacter{}
		character.Signature, _ = Text(object["signature"])
		if control, ok := Object(object["control"]); ok {
			character.Control = parseBGVControl(control)
		}
		if view, ok := Object(object["view"]); ok {
			character.View = parseBGVView(view)
		}
		character.Tracks = parseBGVTracks(object["list"], orderedTrackList(ordered, index))
		layout.Characters = append(layout.Characters, character)
	}

	return layout, nil
}

// parseBGVControl 解析一组控制按钮。
func parseBGVControl(object map[string]any) BGVControl {
	control := BGVControl{}
	if button, ok := ParseButton(object["stop"]); ok {
		control.Stop = button
	}
	if button, ok := ParseButton(object["min"]); ok {
		control.Min = button
	}
	if button, ok := ParseButton(object["max"]); ok {
		control.Max = button
	}
	if button, ok := ParseButton(object["scroll_bar"]); ok {
		control.ScrollBar = button
	}
	control.Volume = parseBGVVolume(object["volume"])
	return control
}

// parseBGVVolume 解析音量条：按钮图 + hit_region。
func parseBGVVolume(value any) BGVVolume {
	volume := BGVVolume{}
	object, ok := Object(value)
	if !ok {
		return volume
	}
	if button, ok := ParseButton(object); ok {
		volume.Button = button
	}
	if region, ok := Array(object["hit_region"]); ok {
		if len(region) > 0 {
			volume.HitX, _ = Number(region[0])
		}
		if len(region) > 1 {
			volume.HitY, _ = Number(region[1])
		}
		if len(region) > 2 {
			volume.HitWidth, _ = Number(region[2])
		}
		if len(region) > 3 {
			volume.HitHeight, _ = Number(region[3])
		}
	}
	return volume
}

// parseBGVView 解析列表的显示区域。
func parseBGVView(object map[string]any) BGVView {
	view := BGVView{}
	if pos, ok := Array(object["pos"]); ok {
		if len(pos) > 0 {
			view.X, _ = Number(pos[0])
		}
		if len(pos) > 1 {
			view.Y, _ = Number(pos[1])
		}
		if len(pos) > 2 {
			view.Width, _ = Number(pos[2])
		}
		if len(pos) > 3 {
			view.Height, _ = Number(pos[3])
		}
	}
	if offset, ok := Array(object["offset"]); ok {
		if len(offset) > 0 {
			view.OffsetX, _ = Number(offset[0])
		}
		if len(offset) > 1 {
			view.OffsetY, _ = Number(offset[1])
		}
	}
	if interval, ok := Array(object["interval"]); ok {
		if len(interval) > 0 {
			view.IntervalStart, _ = Number(interval[0])
		}
		if len(interval) > 1 {
			view.IntervalCount, _ = Number(interval[1])
		}
	}
	view.Lines, _ = Number(object["lines"])
	return view
}

// parseBGVTracks 把曲目表解析成有序切片。
//
// ordered 是从 token 流里取回的文档顺序；它和 map 对不上时（作品换了写法、
// 某个角色没有 list）退回按文件名排序，至少保证同一份文件每次的顺序一致。
func parseBGVTracks(value any, ordered []bgvTrackPair) []BGVTrack {
	object, ok := Object(value)
	if !ok {
		return nil
	}

	tracks := make([]BGVTrack, 0, len(object))
	if len(ordered) == len(object) {
		for _, pair := range ordered {
			file, exists := object[pair.name]
			if !exists {
				tracks = nil
				break
			}
			text, _ := Text(file)
			tracks = append(tracks, BGVTrack{Name: pair.name, File: text})
		}
	}
	if tracks != nil {
		return tracks
	}

	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Slice(names, func(left, right int) bool {
		leftFile, _ := Text(object[names[left]])
		rightFile, _ := Text(object[names[right]])
		if leftFile != rightFile {
			return leftFile < rightFile
		}
		return names[left] < names[right]
	})
	for _, name := range names {
		file, _ := Text(object[name])
		tracks = append(tracks, BGVTrack{Name: name, File: file})
	}
	return tracks
}

// bgvTrackPair 是曲目表里的一条"显示名 → 文件名"。
type bgvTrackPair struct {
	name string
	file string
}

// orderedTrackList 取第 index 个角色的曲目顺序。
func orderedTrackList(lists [][]bgvTrackPair, index int) []bgvTrackPair {
	if index < 0 || index >= len(lists) {
		return nil
	}
	return lists[index]
}

// orderedBGVLists 按文档顺序取出每个角色的曲目表。
//
// 通用解析器返回 map，顺序信息全丢了；这里重新扫一遍 token 流，只看
// characters 数组里的 "list" 段，保留键值对的先后。
func orderedBGVLists(data []byte) [][]bgvTrackPair {
	source := strings.TrimPrefix(string(data), "\ufeff")
	tokens, err := tokenizeLayout(source)
	if err != nil {
		return nil
	}

	for index := 0; index+2 < len(tokens); index++ {
		if tokens[index].kind != layoutTokenWord || tokens[index].text != "characters" {
			continue
		}
		cursor := index + 1
		if tokens[cursor].kind != layoutTokenColon {
			continue
		}
		cursor++
		if tokens[cursor].kind != layoutTokenLBracket {
			continue
		}
		cursor++

		lists := [][]bgvTrackPair{}
		for cursor < len(tokens) && tokens[cursor].kind != layoutTokenRBracket {
			if tokens[cursor].kind != layoutTokenLBrace {
				cursor++
				continue
			}
			end := matchingLayoutBrace(tokens, cursor)
			if end <= cursor {
				break
			}
			lists = append(lists, findBGVListPairs(tokens[cursor+1:end]))
			cursor = end + 1
		}
		return lists
	}
	return nil
}

// findBGVListPairs 在一个角色段里找出 "list" 段并按顺序取出键值对。
func findBGVListPairs(element []layoutToken) []bgvTrackPair {
	for index := 0; index+2 < len(element); index++ {
		if element[index].kind != layoutTokenWord || element[index].text != "list" {
			continue
		}
		if element[index+1].kind != layoutTokenColon || element[index+2].kind != layoutTokenLBrace {
			continue
		}
		return collectBGVListPairs(element, index+3)
	}
	return nil
}

// collectBGVListPairs 从 '{' 之后开始收集 "键: 值" 对，直到对应的 '}'。
func collectBGVListPairs(tokens []layoutToken, index int) []bgvTrackPair {
	pairs := []bgvTrackPair{}
	depth := 0
	for index < len(tokens) {
		token := tokens[index]
		switch token.kind {
		case layoutTokenLBrace:
			depth++
		case layoutTokenRBrace:
			if depth == 0 {
				return pairs
			}
			depth--
		case layoutTokenString, layoutTokenWord:
			if index+2 < len(tokens) && tokens[index+1].kind == layoutTokenColon {
				value := tokens[index+2]
				if value.kind == layoutTokenString || value.kind == layoutTokenWord {
					pairs = append(pairs, bgvTrackPair{name: token.text, file: value.text})
					index += 2
				}
			}
		}
		index++
	}
	return pairs
}

// matchingLayoutBrace 找到与 index 处 '{' 配对的 '}' 的下标。
func matchingLayoutBrace(tokens []layoutToken, index int) int {
	depth := 0
	for cursor := index; cursor < len(tokens); cursor++ {
		switch tokens[cursor].kind {
		case layoutTokenLBrace:
			depth++
		case layoutTokenRBrace:
			depth--
			if depth == 0 {
				return cursor
			}
		case layoutTokenEOF:
			return -1
		}
	}
	return -1
}
