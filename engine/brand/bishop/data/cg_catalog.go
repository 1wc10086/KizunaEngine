package data

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"kizuna/engine/core/binutil"
	"kizuna/engine/core/textcodec"
)

type CgVariant struct {
	Entry   int
	Variant int
	Images  []string
}

// CgEntry 是鉴赏室里的一格：同一个场景的多张变体（正常/服装差分等）。
//
// 原引擎的博物馆按"格"显示缩略图，点进去后用左右箭头切换同一格的变体；
// 因此目录必须保留 entry -> variants 的分组，不能只给一条扁平变体表。
type CgEntry struct {
	Index    int
	Variants []CgVariant
}

type CgGroup struct {
	Index    int
	Variants []CgVariant
	Entries  []CgEntry
}

type CgCatalog struct {
	Groups []CgGroup
	Names  []string
	// nameIndex 是 Names 的反查表（惰性建立，见 IndexOf）。
	nameIndex map[string]int
}

// IndexOf 反查图片名在名字表里的下标，用于全局存档的 CG 解锁位。
//
// 全局存档按"名字表下标"记录解锁状态（原引擎的布局就是这样），
// 所以剧情里出现某张 CG 时必须换成下标才能写进去。
// 名字表里的条目可能带 '*' 前缀（表示该图不是独立 CG 而是合成用的部件），
// cgTrim 在解析时已去掉，这里按去掉后的形式比较。
//
// 内部建反查表：3004 条名字的线性扫描如果挂在"每句对白"上会成为热点，
// 而鉴赏判定一局要跑几万次。表在首次调用时建一次，之后是 O(1)。
func (c *CgCatalog) IndexOf(image string) int {
	if c == nil || len(image) == 0 {
		return -1
	}
	if c.nameIndex == nil {
		c.nameIndex = make(map[string]int, len(c.Names))
		for index, name := range c.Names {
			trimmed := cgTrim(name)
			// 重名时保留第一次出现的下标：原引擎的解锁位也是按首次出现记的。
			if _, exists := c.nameIndex[trimmed]; !exists {
				c.nameIndex[trimmed] = index
			}
		}
	}
	if index, ok := c.nameIndex[image]; ok {
		return index
	}
	return -1
}

func (c *CgCatalog) VariantCount() int {
	total := 0
	for _, group := range c.Groups {
		total += len(group.Variants)
	}
	return total
}

func ParseCgCatalog(catalogData []byte) (*CgCatalog, error) {
	if isTextCgCatalog(catalogData) {
		return parseTextCgCatalog(catalogData)
	}
	position := 0
	first := readCgByte(catalogData, &position)
	second := readCgByte(catalogData, &position)
	third := readCgByte(catalogData, &position)
	fourth := readCgByte(catalogData, &position)
	wide := (first | second | third) != 0
	if !wide && fourth != 0 {
		return nil, errors.New("unsupported cg.dat header")
	}
	if wide {
		position = 0
	}
	groupCount := int(readCgByte(catalogData, &position))
	var indexLists [][]int
	var variantImages [][]int
	var groupOffsets []int
	var groupEntries [][][][]int
	for group := 0; group < groupCount; group++ {
		entryCount := int(readCgByte(catalogData, &position))
		groupOffsets = append(groupOffsets, len(variantImages))
		entries := make([][][]int, 0, entryCount)
		for entry := 0; entry < entryCount; entry++ {
			if !wide {
				readCgUInt16(catalogData, &position)
			}
			variantCount := int(readCgByte(catalogData, &position))
			variants := make([][]int, 0, variantCount)
			for variant := 0; variant < variantCount; variant++ {
				readCgByte(catalogData, &position)
				indexCount := int(readCgByte(catalogData, &position))
				indices := make([]int, 0, indexCount)
				for i := 0; i < indexCount; i++ {
					if wide {
						indices = append(indices, int(readCgUInt32(catalogData, &position)))
					} else {
						indices = append(indices, readCgUInt16(catalogData, &position))
					}
				}
				// 紧随其后的字节是“附加图层组”的数量，不是 1 字节尾部标记。
				// 原引擎把一张 CG 拆成底图 + 若干覆盖层，每层各存一组索引
				// （MK 的立绘/表情差分就是这种结构：2414/2572 个变体带图层）。
				// 旧解析器把它当尾部字节丢掉，只要某一格带图层，后面的流就
				// 全部错位——MK 表现为整张名字表读成 0 条、鉴赏室里全是黑格。
				// 图层索引按原顺序拼进 Images：底图先画，覆盖层后画。
				if wide {
					layerCount := int(readCgByte(catalogData, &position))
					for layer := 0; layer < layerCount; layer++ {
						layerIndexCount := int(readCgByte(catalogData, &position))
						for i := 0; i < layerIndexCount; i++ {
							indices = append(indices, int(readCgUInt32(catalogData, &position)))
						}
					}
				} else {
					// 旧式窄索引目录（u16 索引）目前没有发现带图层的样本，
					// 保持原样读取单字节尾部，避免破坏已适配的旧作。
					readCgByte(catalogData, &position)
				}
				indexLists = append(indexLists, indices)
				variantImages = append(variantImages, indices)
				variants = append(variants, indices)
			}
			entries = append(entries, variants)
		}
		groupEntries = append(groupEntries, entries)
	}
	nameCount := readCgUInt16(catalogData, &position)
	// 编码按整个名字池探测一次：逐条猜会被表头的日文标题带偏（KJ 的
	// 『残酷な契り』），错一次后面所有名字下标漂移一格，封面全部错位。
	nameEncoding := cgNameEncodingOf(catalogData, position)
	names := make([]string, 0, nameCount)
	for i := 0; i < nameCount; i++ {
		names = append(names, readCgName(catalogData, &position, nameEncoding))
	}
	groups := make([]CgGroup, 0, groupCount)
	for group := 0; group < groupCount; group++ {
		start := groupOffsets[group]
		end := len(variantImages)
		if group+1 < groupCount {
			end = groupOffsets[group+1]
		}
		length := end - start
		if length < 0 {
			length = 0
		}
		variants := make([]CgVariant, 0, length)
		for index := start; index < end; index++ {
			images := []string{}
			for _, nameIndex := range variantImages[index] {
				if nameIndex >= 0 && nameIndex < len(names) {
					images = append(images, cgTrim(names[nameIndex]))
				}
			}
			variants = append(variants, CgVariant{Entry: index, Variant: index, Images: images})
		}
		cgEntries := make([]CgEntry, 0, len(groupEntries[group]))
		for entryIndex, entryVariants := range groupEntries[group] {
			entry := CgEntry{Index: entryIndex}
			for variantIndex, indices := range entryVariants {
				images := []string{}
				for _, nameIndex := range indices {
					if nameIndex >= 0 && nameIndex < len(names) {
						images = append(images, cgTrim(names[nameIndex]))
					}
				}
				entry.Variants = append(entry.Variants, CgVariant{
					Entry:   entryIndex,
					Variant: variantIndex,
					Images:  images,
				})
			}
			cgEntries = append(cgEntries, entry)
		}
		groups = append(groups, CgGroup{Index: group, Variants: variants, Entries: cgEntries})
	}
	return &CgCatalog{Groups: groups, Names: names}, nil
}

// isTextCgCatalog 判断 cg.dat 是不是 Tbxdl 世代的 CSV 目录。
//
// 旧版/特別病棟 的 cg.dat 是纯文本 "group,entry,image,flag"，不是二进制
// 名字表；按二进制解析会读到 0x30('0')=48 个组、变体全是空图。
func isTextCgCatalog(data []byte) bool {
	limit := min(len(data), 512)
	if limit == 0 {
		return false
	}
	for index := 0; index < limit; index++ {
		value := data[index]
		if value == 0 {
			return false
		}
	}
	line := strings.TrimSpace(string(data[:limit]))
	if line == "" || line[0] < '0' || line[0] > '9' {
		return false
	}
	return strings.Contains(line, ",") && strings.Contains(line, "\n")
}

// parseTextCgCatalog 解析 "group,entry,image,flag" 文本目录。
//
// 同一 (group, entry) 的多行是同一格的不同变体；原引擎用左右箭头切换。
// 第四列是解锁/路线标记，当前只用于展示，不参与分组。
func parseTextCgCatalog(data []byte) (*CgCatalog, error) {
	type entryKey struct{ group, entry int }
	entries := map[entryKey]*CgEntry{}
	groups := map[int]*CgGroup{}
	var entryOrder []entryKey
	var groupOrder []int
	names := make([]string, 0, 256)
	seenNames := map[string]bool{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		columns := strings.Split(line, ",")
		if len(columns) < 3 {
			continue
		}
		group, errGroup := strconv.Atoi(strings.TrimSpace(columns[0]))
		entry, errEntry := strconv.Atoi(strings.TrimSpace(columns[1]))
		image := strings.TrimSpace(columns[2])
		if errGroup != nil || errEntry != nil || image == "" {
			continue
		}
		key := entryKey{group: group, entry: entry}
		if _, exists := entries[key]; !exists {
			entries[key] = &CgEntry{Index: entry}
			entryOrder = append(entryOrder, key)
		}
		entryValue := entries[key]
		entryValue.Variants = append(entryValue.Variants, CgVariant{
			Entry:   entry,
			Variant: len(entryValue.Variants),
			Images:  []string{cgTrim(image)},
		})
		if _, exists := groups[group]; !exists {
			groups[group] = &CgGroup{Index: group}
			groupOrder = append(groupOrder, group)
		}
		if !seenNames[image] {
			seenNames[image] = true
			names = append(names, cgTrim(image))
		}
	}
	sort.Ints(groupOrder)
	result := make([]CgGroup, 0, len(groupOrder))
	for _, groupIndex := range groupOrder {
		group := groups[groupIndex]
		for _, key := range entryOrder {
			if key.group != groupIndex {
				continue
			}
			entryValue := entries[key]
			group.Entries = append(group.Entries, *entryValue)
			group.Variants = append(group.Variants, entryValue.Variants...)
		}
		result = append(result, *group)
	}
	return &CgCatalog{Groups: result, Names: names}, nil
}

func cgTrim(name string) string {
	if strings.HasPrefix(name, "*") {
		return name[1:]
	}
	return name
}

// readCgName 读取名字表里的一条图片名。
//
// 不同代的 cg.dat 混用两种编码：
//   - 现代作品是 UTF-16LE（每个字符两字节，00 00 结尾）；
//   - 旧式作品（KD 毎日けだものっ!!）是 CP932/ASCII 的单字节串。
//
// 只按 UTF-16 读会把 KD 的 "ev01_00_1\0" 解成乱码，鉴赏室缩略图全部
// 找不到资源。只按"字符后一字节是否为 0"逐条猜也不行：KJ 的名字表以
// UTF-16 日文标题『残酷な契り』开头，汉字高字节不是 0，会被误判成
// CP932 并少读 1 字节，之后整张表的名字下标全部漂移一格——鉴赏室封面
// 从此全部指向相邻的错误图片（KJ 实测就是"封面错位、大小不对"）。
// 因此编码由 cgNameEncodingOf 对整个池子探测一次，这里只按结果读。
func readCgName(catalogData []byte, position *int, encoding cgNameEncoding) string {
	if *position < 0 || *position >= len(catalogData) {
		return ""
	}
	if encoding == cgNameUtf16 {
		return readCgWideString(catalogData, position)
	}
	start := *position
	for *position < len(catalogData) && catalogData[*position] != 0 {
		*position++
	}
	raw := catalogData[start:*position]
	if *position < len(catalogData) {
		*position++
	}
	return textcodec.DecodeText(raw, textcodec.Cp932)
}

// cgNameEncoding 是 cg.dat 名字池的整体编码。
type cgNameEncoding int

const (
	cgNameCp932 cgNameEncoding = iota
	cgNameUtf16
)

// cgNameEncodingOf 探测整个名字池的编码。
//
// 各代 cg.dat 的名字池都是"整表同编码"：屈辱/侵蝕/KJ 都是 UTF-16LE（屈辱
// 首名 'ev00_001_1p00' 看着像单字节，其实每字符高字节都是 0），KD 这类
// 旧作才是 CP932。在池内逐条猜编码不可行——UTF-16 里的日文字符高字节
// 非 0，单看前 4 字节和 CP932 无法区分，猜错一次后面全部漂移。
//
// 探测规则：把 [start,len) 按 UTF-16 单元扫一遍，统计"高字节为 0 的 ASCII
// 形字符"与"高字节非 0 字符"的比例。UTF-16LE 池里每个 ASCII 字符的高字节
// 都是 0（图片名 ev…/bg…/cg… 占绝大多数），所以 asciiLike 远多于
// highNonZero；CP932 单字节池按 2 字节对齐去读时，只有串结尾的 00 会让
// 高字节为 0，asciiLike 反而少于 highNonZero（学園 GK 的池是 860:2347）。
// 因此判据必须是"asciiLike 超过一半"，而不是先前的 25%：25% 会把
// CP932 池误判成 UTF-16，名字整体错位成韩文字符，鉴赏室的 CG 全部加载
// 失败（GK 的真实现象）。
func cgNameEncodingOf(catalogData []byte, start int) cgNameEncoding {
	if start < 0 || start >= len(catalogData) {
		return cgNameCp932
	}
	asciiLike := 0
	highNonZero := 0
	for offset := start; offset+1 < len(catalogData); offset += 2 {
		low, high := catalogData[offset], catalogData[offset+1]
		if low == 0 && high == 0 {
			continue // 串结尾的 00 00，不是字符
		}
		if high == 0 && low != 0 {
			asciiLike++
		}
		if high != 0 {
			highNonZero++
		}
	}
	if asciiLike > 0 && asciiLike*2 > asciiLike+highNonZero {
		// ASCII 形字符超过一半：每个字符的高字节都是 0，判定整池 UTF-16LE。
		return cgNameUtf16
	}
	return cgNameCp932
}

func readCgWideString(catalogData []byte, position *int) string {
	var builder strings.Builder
	for *position+2 <= len(catalogData) {
		value := binutil.U16(catalogData[*position:])
		*position += 2
		if value == 0 {
			break
		}
		builder.WriteRune(rune(value))
	}
	return builder.String()
}

func readCgByte(catalogData []byte, position *int) byte {
	if *position < len(catalogData) {
		value := catalogData[*position]
		*position++
		return value
	}
	return 0
}

func readCgUInt16(catalogData []byte, position *int) int {
	if *position+2 > len(catalogData) {
		*position = len(catalogData)
		return 0
	}
	value := binutil.U16(catalogData[*position:])
	*position += 2
	return int(value)
}

func readCgUInt32(catalogData []byte, position *int) uint32 {
	if *position+4 > len(catalogData) {
		*position = len(catalogData)
		return 0
	}
	value := binutil.U32(catalogData[*position:])
	*position += 4
	return value
}
