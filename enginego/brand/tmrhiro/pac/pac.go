// Package pac 读取 TMR-Hiro ADV System 的 .PAC 资源归档。
//
// 归档没有魔数，布局（小端）：
//
//	u16 count        条目数
//	u8  nameLen     定长名字字段的字节数
//	u32 dataOff     第一条数据相对文件起点的偏移
//	count × entry   nameLen 字节名字（'\0' 填充）+ 偏移 + 大小
//
// 两个发行世代只差目录项大小（见 GARbro 的 ArcFormats/Tmr-Hiro/ArcPAC.cs）：
//   - v1：目录项 = name + u32 offset + u32 size，dataOff 正好等于索引结束；
//   - v2：目录项 = name + u64 offset + u32 size，dataOff 比 v1 索引多 4*count
//     （目录后多一张每项 u32 的表，引擎读它，我们用不到）。
//
// 偏移一律相对 dataOff 起算，绝对偏移 = dataOff + offset。
//
// 名字大小写：原引擎把查询名与目录名都转小写再比较（sub_40E430），
// 这里同样大小写不敏感，避免脚本里混用 Srp/srp。
package pac

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kizuna/enginego/core"
)

const (
	indexHeaderSize = 7
	maxEntries      = 1 << 20
	// version2ExtraPerEntry 是 v2 目录尾部的附加表（每项 4 字节），用于识别世代。
	version2ExtraPerEntry = 4
	// looseCacheLimit 是允许驻留内存的条目上限。剧本/配置只有几十 KB；
	// 图像与音频动辄数 MB，全部缓存会把 1.7G 的归档摊进内存。
	looseCacheLimit = 4 << 20
)

// Entry 是归档里的一条资源。
type Entry struct {
	Name   string
	Offset int64 // 绝对文件偏移
	Size   int64
}

// Archive 是一个已解析的 .PAC。目录常驻内存，数据按次开文件读：
// 并发读取（合成线程与音频线程）各自 seek，不共享 fd。
type Archive struct {
	path    string
	version int
	dataOff int64
	entries map[string]Entry // key 为小写名
	order   []string         // 原始顺序
}

// Open 解析一个归档，只读目录不读数据。
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	header := make([]byte, indexHeaderSize)
	if _, err := f.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("tmrhiro/pac: %s: 读头失败: %w", filepath.Base(path), err)
	}
	count := int(binary.LittleEndian.Uint16(header[0:2]))
	nameLen := int(header[2])
	dataOff := int64(binary.LittleEndian.Uint32(header[3:7]))
	if count <= 0 || count > maxEntries || nameLen <= 0 {
		return nil, fmt.Errorf("tmrhiro/pac: %s: 头字段非法 (count=%d nameLen=%d)", filepath.Base(path), count, nameLen)
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	indexEnd := int64(indexHeaderSize) + int64(nameLen+8)*int64(count)

	version := 0
	switch {
	case dataOff == indexEnd:
		version = 1
	case dataOff == indexEnd+version2ExtraPerEntry*int64(count):
		version = 2
	default:
		// 非标准布局：只接受“数据起点在索引之后”的归档，越过文件尾的是损坏包。
		if dataOff <= indexEnd || dataOff >= info.Size() {
			return nil, fmt.Errorf("tmrhiro/pac: %s: dataOff=%d 与目录不匹配 (%s)", filepath.Base(path), dataOff, path)
		}
		version = 1
	}

	entrySize := int64(nameLen + 8)
	if version == 2 {
		entrySize = int64(nameLen + 12)
	}
	dir := make([]byte, entrySize*int64(count))
	if _, err := f.ReadAt(dir, indexHeaderSize); err != nil {
		return nil, fmt.Errorf("tmrhiro/pac: %s: 读目录失败: %w", filepath.Base(path), err)
	}

	a := &Archive{path: path, version: version, dataOff: dataOff, entries: make(map[string]Entry, count)}
	pos := 0
	for i := 0; i < count; i++ {
		raw := dir[pos : pos+nameLen]
		pos += nameLen
		name := strings.TrimRight(string(raw), "\x00")
		var off, size int64
		if version == 2 {
			off = int64(binary.LittleEndian.Uint64(dir[pos : pos+8]))
			size = int64(binary.LittleEndian.Uint32(dir[pos+8 : pos+12]))
			pos += 12
		} else {
			off = int64(binary.LittleEndian.Uint32(dir[pos : pos+4]))
			size = int64(binary.LittleEndian.Uint32(dir[pos+4 : pos+8]))
			pos += 8
		}
		abs := dataOff + off
		if size <= 0 || abs < dataOff || abs+size > info.Size() {
			// 越界条目跳过而不是整体失败：个别作品归档尾部有被截断的项。
			continue
		}
		key := gameKey(name)
		if _, dup := a.entries[key]; dup {
			continue
		}
		a.entries[key] = Entry{Name: name, Offset: abs, Size: size}
		a.order = append(a.order, name)
	}
	return a, nil
}

// gameKey 是大写不敏感的查表键。cga 等归档里有全大写名字，
// 统一转小写后一张表就能覆盖。
func gameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Path 返回归档文件路径。
func (a *Archive) Path() string { return a.path }

// Version 返回识别出的世代（1 或 2）。
func (a *Archive) Version() int { return a.version }

// Count 返回有效条目数。
func (a *Archive) Count() int { return len(a.entries) }

// Names 返回按字典序排列的条目名。
func (a *Archive) Names() []string {
	out := append([]string(nil), a.order...)
	sort.Strings(out)
	return out
}

// Lookup 按名字（大小写不敏感）查条目。
func (a *Archive) Lookup(name string) (Entry, bool) {
	if a == nil {
		return Entry{}, false
	}
	e, ok := a.entries[gameKey(name)]
	return e, ok
}

// Read 读出条目全部字节。
func (a *Archive) Read(name string) ([]byte, error) {
	entry, ok := a.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("tmrhiro/pac: %s: 没有条目 %q", filepath.Base(a.path), name)
	}
	f, err := os.Open(a.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, entry.Size)
	if _, err := f.ReadAt(buf, entry.Offset); err != nil {
		return nil, fmt.Errorf("tmrhiro/pac: %s: 读 %q 失败: %w", filepath.Base(a.path), entry.Name, err)
	}
	return buf, nil
}

// Set 是同一游戏目录下所有 .PAC 的集合，外加散装文件回退。
//
// 名字空间是扁平的：脚本只写 "BG0500_a"、"SEH_I_101" 这样的名字，
// 从不写归档名，因此 Set 按名查即可。分桶只用来决定“先查哪一类”，
// 让同名的图像优先于音频。
type Set struct {
	dir      string
	archives []*Archive
	images   []*Archive // Agrd/Bgrd/Cgrd/Dgrd/…：图像与界面
	scripts  []*Archive // srp*：剧本
	audio    []*Archive // mus/se/sse/vic/mov：音频
	mu       sync.Mutex
	cache    map[string][]byte
}

// OpenSet 扫描 dir 里所有 *.pac。解析失败的杂项包跳过，不让整部游戏打不开。
//
// 与 Open（单归档）分名：同一包里既要有“打开一个归档”，也要有“打开目录下
// 全部归档”，两者签名不同但语义差一层，用两个名字比让调用方看参数猜更清楚。
func OpenSet(dir string) (*Set, error) {
	if dir == "" {
		return nil, fmt.Errorf("tmrhiro/pac: 空目录")
	}
	names, err := core.ListFilesTop(dir)
	if err != nil {
		return nil, err
	}
	s := &Set{dir: dir, cache: make(map[string][]byte)}
	for _, name := range names {
		if !core.HasFoldSuffix(name, ".pac") {
			continue
		}
		a, err := Open(filepath.Join(dir, name))
		if err != nil {
			core.Logf("tmrhiro/pac: 跳过 %s: %v", name, err)
			continue
		}
		s.archives = append(s.archives, a)
		lower := strings.ToLower(core.Stem(name))
		switch {
		case strings.Contains(lower, "srp"):
			s.scripts = append(s.scripts, a)
		case strings.Contains(lower, "mus"), strings.Contains(lower, "se"),
			strings.Contains(lower, "vic"), strings.Contains(lower, "mov"):
			s.audio = append(s.audio, a)
		default:
			s.images = append(s.images, a)
		}
	}
	if len(s.archives) == 0 {
		return nil, fmt.Errorf("tmrhiro/pac: %s 里没有可用的 .PAC", dir)
	}
	return s, nil
}

// Dir 返回游戏目录。
func (s *Set) Dir() string { return s.dir }

// Archives 返回全部已打开的归档（诊断用）。
func (s *Set) Archives() []*Archive { return append([]*Archive(nil), s.archives...) }

// TotalEntries 返回所有归档条目数之和。
func (s *Set) TotalEntries() int {
	total := 0
	for _, a := range s.archives {
		total += a.Count()
	}
	return total
}

// readEntry 读一个已知条目，小条目走内存缓存。
func (s *Set) readEntry(a *Archive, entry Entry) ([]byte, error) {
	key := gameKey(entry.Name)
	s.mu.Lock()
	if data, ok := s.cache[key]; ok && int64(len(data)) == entry.Size {
		s.mu.Unlock()
		return data, nil
	}
	s.mu.Unlock()

	data, err := a.Read(entry.Name)
	if err != nil {
		return nil, err
	}
	if entry.Size <= looseCacheLimit {
		s.mu.Lock()
		s.cache[key] = data
		s.mu.Unlock()
	}
	return data, nil
}

func (s *Set) readLoose(name string) ([]byte, bool) {
	candidates := []string{name}
	if filepath.Ext(name) == "" {
		// 绿色版/补丁把个别资源以原扩展名放在目录里。
		candidates = append(candidates, name+".grd", name+".sct", name+".txt", name+".bmp")
	}
	for _, candidate := range candidates {
		path := filepath.Join(s.dir, candidate)
		if !core.FileExists(path) {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			return data, true
		}
	}
	names, err := core.ListFilesTop(s.dir)
	if err != nil {
		return nil, false
	}
	want := gameKey(name)
	for _, candidate := range names {
		if gameKey(core.Stem(candidate)) != want {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(s.dir, candidate)); err == nil {
			return data, true
		}
	}
	return nil, false
}

// Entry 按名取资源：先查图像归档，再查剧本，再查音频，最后散装文件。
func (s *Set) Entry(name string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("tmrhiro/pac: 空名字")
	}
	key := gameKey(name)
	for _, group := range [][]*Archive{s.images, s.scripts, s.audio} {
		for _, a := range group {
			if entry, ok := a.entries[key]; ok {
				return s.readEntry(a, entry)
			}
		}
	}
	for _, a := range s.archives {
		if entry, ok := a.entries[key]; ok {
			return s.readEntry(a, entry)
		}
	}
	if data, ok := s.readLoose(name); ok {
		return data, nil
	}
	return nil, fmt.Errorf("tmrhiro/pac: 找不到资源 %q", name)
}

// EntryOrEmpty 是 Entry 的“取不到给空”版本，供可选资源（界面图、BGM）使用。
func (s *Set) EntryOrEmpty(name string) []byte {
	if data, err := s.Entry(name); err == nil {
		return data
	}
	return nil
}

// Has 报告该名字是否存在。
func (s *Set) Has(name string) bool {
	key := gameKey(name)
	for _, a := range s.archives {
		if _, ok := a.entries[key]; ok {
			return true
		}
	}
	_, ok := s.readLoose(name)
	return ok
}

// Size 返回条目字节数；0 表示不存在。
func (s *Set) Size(name string) int64 {
	key := gameKey(name)
	for _, a := range s.archives {
		if entry, ok := a.entries[key]; ok {
			return entry.Size
		}
	}
	names, err := core.ListFilesTop(s.dir)
	if err != nil {
		return 0
	}
	want := key
	for _, candidate := range names {
		if gameKey(core.Stem(candidate)) != want {
			continue
		}
		if info, err := os.Stat(filepath.Join(s.dir, candidate)); err == nil {
			return info.Size()
		}
	}
	return 0
}

// Names 返回“归档 → 名字列表”，供目录/鉴赏枚举（保留原始大小写）。
func (s *Set) Names() map[string][]string {
	out := make(map[string][]string, len(s.archives))
	for _, a := range s.archives {
		out[filepath.Base(a.Path())] = a.Names()
	}
	return out
}

// Close 释放缓存。归档不持有文件句柄，因此只需清缓存。
func (s *Set) Close() {
	s.mu.Lock()
	s.cache = make(map[string][]byte)
	s.mu.Unlock()
}

// FindGameDir 从 dir 本身或它的一级子目录里找出含 srp.pac 的游戏目录。
//
// 发行版把数据放在 <root>/________ 这种随机名目录（光盘安装器的输出名），
// 扫描游戏库时数据库只看得到上一级，因此必须向下探测一层。
func FindGameDir(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	candidates := []string{dir}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				candidates = append(candidates, filepath.Join(dir, entry.Name()))
			}
		}
	}
	for _, candidate := range candidates {
		if isGameDir(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func isGameDir(dir string) bool {
	names, err := core.ListFilesTop(dir)
	if err != nil {
		return false
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[strings.ToLower(name)] = true
	}
	// srp.pac 是剧本归档；再要求至少一个资源归档，避免只压了剧本的残包
	// 被识别成完整安装（那种包装不装都一样，但报错位置会更清楚）。
	if !seen["srp.pac"] {
		return false
	}
	for _, name := range []string{"mus.pac", "se.pac", "vic.pac", "bgr.pac", "agrd.pac", "cgrd.pac"} {
		if seen[name] {
			return true
		}
	}
	return false
}
