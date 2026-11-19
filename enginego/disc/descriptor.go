package disc

// descriptor.go 解析 CUE / CCD / MDS 三种常见镜像描述符。
//
// 我们只需要三件事：数据轨从哪个 LBA 开始、数据轨的扇区布局、整张盘声明了多少
// 扇区。音轨不参与游戏资源，因此全部忽略。描述符缺失或格式不认识时返回零值，
// 后续的 LBA16 自证探测仍然可以识别纯 ISO / 普通 BIN。

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type cueSheet struct {
	dataFile  string
	dataTrack int64
	hint      *sectorLayout
	tracks    int
}

func readDescriptorHint(path string) (*sectorLayout, int64, uint32, error) {
	directory := filepath.Dir(path)
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	// 数据文件本身可能叫 Disk2.bin，描述符叫 Disc2.cue（名字不完全一致），
	// 所以除了同 stem，还要扫描目录里的描述符，看 FILE/Parent 指向谁。
	if sheet, declared, ok := findCueFor(path); ok {
		sheet.dataTrack = max64(0, sheet.dataTrack)
		// cue 没有 lead-out，通常拿不到声明扇区数；同 stem 的 ccd 往往有，
		// 用它判断镜像是否插入了重复扇区（Disk1.Alpha 就是这种情况）。
		if declared == 0 {
			directory := filepath.Dir(path)
			stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			for _, candidate := range foldCandidates(directory, stem+".ccd") {
				if _, _, ccdDeclared, ok := parseCCD(candidate); ok && ccdDeclared > 0 {
					declared = ccdDeclared
					break
				}
			}
		}
		return sheet.hint, sheet.dataTrack, declared, nil
	}
	for _, ext := range []string{".ccd", ".mds"} {
		for _, candidate := range foldCandidates(directory, stem+ext) {
			if ext == ".ccd" {
				if hint, track, declared, ok := parseCCD(candidate); ok {
					return hint, track, declared, nil
				}
			} else {
				if declared, ok := parseMDS(candidate); ok {
					return nil, 0, declared, nil
				}
			}
		}
	}
	return nil, 0, 0, nil
}

func findCueFor(path string) (*cueSheet, uint32, bool) {
	directory := filepath.Dir(path)
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	candidates := foldCandidates(directory, stem+".cue")
	// 名字不同时（Disk2.bin vs Disc2.cue）扫描全部 cue，只接受 FILE 指向本文件的。
	entries, err := os.ReadDir(directory)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".cue") {
				candidates = append(candidates, filepath.Join(directory, entry.Name()))
			}
		}
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		sheet, declared, ok := parseCue(candidate)
		if !ok {
			continue
		}
		if !strings.EqualFold(filepath.Base(sheet.dataFile), filepath.Base(path)) &&
			!sameFile(sheet.dataFile, path) {
			continue
		}
		return &sheet, declared, true
	}
	return nil, 0, false
}

func parseCue(path string) (cueSheet, uint32, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cueSheet{}, 0, false
	}
	sheet := cueSheet{}
	var current *struct {
		mode  string
		index int64
	}
	var tracks []*struct {
		mode  string
		index int64
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "FILE "):
			name := cueQuoted(line[len("FILE "):])
			if sheet.dataFile == "" && name != "" {
				// CUE 里的 FILE 常用大写（MANIN_1.BIN），而解包/拷贝后的
				// 实际文件可能是小写；必须按目录做一次大小写不敏感回退，
				// 否则 List/Prepare 会报 "disc: no image found"。
				sheet.dataFile = resolveCueDataFile(filepath.Dir(path), name)
			}
		case strings.HasPrefix(upper, "TRACK "):
			fields := strings.Fields(line)
			track := &struct {
				mode  string
				index int64
			}{index: -1}
			if len(fields) >= 3 {
				track.mode = strings.ToUpper(fields[2])
			}
			tracks = append(tracks, track)
			current = track
		case strings.HasPrefix(upper, "INDEX ") && current != nil:
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[1] == "01" {
				current.index = cueMSF(fields[2])
			}
		}
	}
	sheet.tracks = len(tracks)
	for _, track := range tracks {
		if strings.Contains(track.mode, "AUDIO") {
			continue
		}
		sheet.dataTrack = max64(0, track.index)
		switch {
		case strings.Contains(track.mode, "2048"):
			sheet.hint = &sectorLayout{size: 2048, userOffset: 0, name: "cue-mode1-2048"}
		case strings.Contains(track.mode, "MODE2"):
			sheet.hint = &sectorLayout{size: 2352, userOffset: 24, name: "cue-mode2-2352"}
		default:
			sheet.hint = &sectorLayout{size: 2352, userOffset: 16, name: "cue-mode1-2352"}
		}
		break
	}
	if sheet.dataFile == "" {
		return sheet, 0, false
	}
	return sheet, 0, true
}

func cueQuoted(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") {
		if end := strings.Index(value[1:], "\""); end >= 0 {
			return value[1 : end+1]
		}
	}
	fields := strings.Fields(value)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func cueMSF(value string) int64 {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0
	}
	minute, _ := strconv.Atoi(parts[0])
	second, _ := strconv.Atoi(parts[1])
	frame, _ := strconv.Atoi(parts[2])
	return int64((minute*60+second)*75 + frame)
}

func parseCCD(path string) (*sectorLayout, int64, uint32, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, false
	}
	sections := map[string]map[string]string{}
	current := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if sections[current] == nil {
				sections[current] = map[string]string{}
			}
			continue
		}
		if current == "" || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		sections[current][strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	hint := &sectorLayout{size: 2352, userOffset: 16, name: "ccd-mode1-2352"}
	track := int64(0)
	declared := uint32(0)
	foundData := false
	for name, section := range sections {
		if strings.HasPrefix(name, "Entry ") {
			point := parseHexInt(section["Point"])
			if point < 0x64 {
				control := parseHexInt(section["Control"])
				if control&0x04 == 0 {
					continue
				}
				if !foundData {
					track = int64(parseInt(section["PLBA"]))
					foundData = true
				}
				continue
			}
			if point == 0xA2 {
				declared = uint32(parseInt(section["PLBA"]))
			}
			continue
		}
		if strings.HasPrefix(name, "TRACK ") {
			if strings.TrimSpace(section["MODE"]) == "2" {
				hint = &sectorLayout{size: 2352, userOffset: 24, name: "ccd-mode2-2352"}
			}
		}
	}
	if !foundData {
		return nil, 0, declared, false
	}
	return hint, max64(0, track), declared, true
}

func parseMDS(path string) (uint32, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 0x60 {
		return 0, false
	}
	if !strings.HasPrefix(string(data), "MEDIA DESCRIPTOR") {
		return 0, false
	}
	offset := int(binary.LittleEndian.Uint32(data[0x50:]))
	if offset <= 0 || offset+8 > len(data) {
		return 0, false
	}
	declared := binary.LittleEndian.Uint32(data[offset+4:])
	return declared, declared > 0
}

func parseHexInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		parsed, _ := strconv.ParseInt(value[2:], 16, 64)
		return int(parsed)
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// foldCandidates 返回目录中与 name 大小写不敏感匹配的路径。
// Android 的 exFAT/外部存储对大小写不敏感，但解包到内部存储后是敏感的，
// 描述符里写 "Disk2.BIN"、实际文件叫 Disk2.bin 时不能直接拼路径。
func foldCandidates(directory, name string) []string {
	direct := filepath.Join(directory, name)
	if _, err := os.Stat(direct); err == nil {
		return []string{direct}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			out = append(out, filepath.Join(directory, entry.Name()))
		}
	}
	return out
}

// resolveCueDataFile 把 CUE 的 FILE 名解析成真实文件。CUE 常写大写
// （MANIN_1.BIN），实际文件可能是小写；先精确命中，再按 basename 折叠。
func resolveCueDataFile(directory, name string) string {
	if candidates := foldCandidates(directory, name); len(candidates) > 0 {
		return candidates[0]
	}
	base := filepath.Base(name)
	if candidates := foldCandidates(directory, base); len(candidates) > 0 {
		return candidates[0]
	}
	return filepath.Join(directory, name)
}

func sameFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}
