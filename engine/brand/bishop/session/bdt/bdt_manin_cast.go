package bdt

import (
	"strconv"
	"strings"

	"kizuna/engine/core/textcodec"
)

// bdt_manin_cast.go 解析 ctbl.rbd（角色别出现点）并回答「某时间带某角色在哪个站」。
//
// 格式（原作注释）：
//
//	<1>遥香
//	1   鎌多 新重 - - 樟南 鎌多
//	15  - - 樟南 樟南 - -
//
// 第一列是进行值门槛，后 6 列依次是朝1/朝2/昼1/昼2/夜1/夜2 的车站；
// 当前进行值取「不超过它的最大门槛」那一行。地图上把 mfN 表情贴到对应
// 车站，就是原作「各駅にカーソルを合わせると顔アイコンが出る」的逻辑。

var maninCharacterRegisters = [6]int{11, 13, 15, 17, 19, 21}

var maninStationOrder = []string{
	"鎌多", "新重", "樟南", "聖城", "桜木町", "新横山", "一ヶ谷", "町屋",
}

func maninStationIDByName(name string) int {
	name = strings.TrimSpace(name)
	if name == "市ヶ谷" {
		// ctbl 用「市ヶ谷」，脚本标签用「一ヶ谷」，是同一站。
		name = "一ヶ谷"
	}
	for index, candidate := range maninStationOrder {
		if candidate == name {
			return index + 1
		}
	}
	return 0
}

func loadManinCharacters(data []byte) []maninCharacter {
	if len(data) == 0 {
		return nil
	}
	decrypted := make([]byte, len(data))
	for i, value := range data {
		decrypted[i] = ^value
	}
	text := textcodec.DecodeCP932(decrypted)
	var out []maninCharacter
	var current *maninCharacter
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "<") {
			close := strings.IndexByte(trimmed, '>')
			if close < 0 {
				continue
			}
			index, err := strconv.Atoi(strings.TrimSpace(trimmed[1:close]))
			if err != nil || index < 1 || index > 7 {
				continue
			}
			name := strings.TrimSpace(trimmed[close+1:])
			out = append(out, maninCharacter{Index: index, Name: name})
			current = &out[len(out)-1]
			continue
		}
		if current == nil {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		threshold, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		row := maninCharRow{Threshold: threshold}
		for slot := 0; slot < 6 && slot+1 < len(fields); slot++ {
			station := fields[slot+1]
			if station == "-" {
				continue
			}
			row.Stations[slot] = station
		}
		current.Rows = append(current.Rows, row)
	}
	return out
}

func (s *Session) maninCharacters() []maninCharacter {
	if s.manin == nil {
		return nil
	}
	if s.manin.charactersLoaded {
		return s.manin.characters
	}
	s.manin.charactersLoaded = true
	if s.resources == nil {
		return nil
	}
	location, ok := s.resources.TryGet("ctbl")
	if !ok {
		location, ok = s.resources.TryGet("ctbl.rbd")
	}
	if !ok || location == nil {
		return nil
	}
	s.manin.characters = loadManinCharacters(location.Read())
	return s.manin.characters
}

// maninCharacterStation 返回角色 index（0..5）在当前时间带的站点 ID（1..8）。
// slot 为 X1-2（0..5）；没有出现点时返回 0。
func (s *Session) maninCharacterStation(index, slot int) int {
	if index < 0 || index >= 6 || slot < 0 || slot >= 6 || s.machine == nil {
		return 0
	}
	characters := s.maninCharacters()
	if len(characters) == 0 {
		return 0
	}
	var character *maninCharacter
	for i := range characters {
		if characters[i].Index == index+1 {
			character = &characters[i]
			break
		}
	}
	if character == nil {
		return 0
	}
	registers := s.machine.X()
	register := maninCharacterRegisters[index]
	if register < 0 || register >= len(registers) {
		return 0
	}
	progress := int(registers[register])
	best := -1
	for rowIndex, row := range character.Rows {
		if progress >= row.Threshold && (best < 0 || row.Threshold >= character.Rows[best].Threshold) {
			best = rowIndex
		}
	}
	if best < 0 {
		return 0
	}
	return maninStationIDByName(character.Rows[best].Stations[slot])
}
