// Package archive reads BLACKPACKAGE / FFA SYSTEM resource archives.
//
// 发行版没有把文件塞进单个容器，而是用 .LST 做索引、.DAT 做数据：
// .LST 每条 22 字节（14 字节名 + u32 offset + u32 size），offset 是相对
// 同名 .DAT 起点的字节偏移。X0WIN 是主数据（剧本/图片/语音），X0WIS
// 是安装/卸载程序用的小包。这里把两者都挂到同一个 Set 上，按名字取，
// 同名时主包优先。
package archive

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const lstEntrySize = 0x16

// Entry 是索引里的一条资源。Set 持有对应的 *os.File，Close 时统一关。
type Entry struct {
	Name       string
	Offset     uint32
	Size       uint32
	dat        *os.File
	standalone bool
}

// Set 是同一游戏目录下所有 LST/DAT 的集合。
type Set struct {
	dir     string
	entries map[string]*Entry // key 为小写名
	order   []string
	files   []*os.File
}

// Open 扫描 dir，打开所有成对的 *.LST / *.DAT。主包（X0WIN）优先于
// 安装包（X0WIS），因此按文件名倒序打开：WIN 在 WIS 之前注册。
func Open(dir string) (*Set, error) {
	if dir == "" {
		return nil, fmt.Errorf("blackpackage: 空目录")
	}
	s := &Set{dir: dir, entries: make(map[string]*Entry)}
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var lsts []string
	for _, de := range names {
		if de.IsDir() {
			continue
		}
		n := strings.ToLower(de.Name())
		if strings.HasSuffix(n, ".lst") {
			lsts = append(lsts, de.Name())
		}
	}
	// 主数据包在前。排序用 basename，保证 X0WIN 先于 X0WIS。
	sort.Slice(lsts, func(i, j int) bool {
		return strings.ToLower(lsts[i]) > strings.ToLower(lsts[j])
	})
	for _, lstName := range lsts {
		base := strings.TrimSuffix(lstName, filepath.Ext(lstName))
		datName := base + ".DAT"
		if _, err := os.Stat(filepath.Join(dir, datName)); err != nil {
			// 有些目录扩展名大小写不同，按不区分大小写再找一次。
			if alt := findCI(names, base+".dat"); alt != "" {
				datName = alt
			} else {
				continue
			}
		}
		if err := s.addPair(filepath.Join(dir, lstName), filepath.Join(dir, datName)); err != nil {
			return nil, err
		}
	}
	// 独立文件（TR02.WA1、X0WIN00C.MPG 等）也纳入，名字冲突时仍让 DAT 优先。
	for _, de := range names {
		if de.IsDir() {
			continue
		}
		key := strings.ToLower(de.Name())
		if _, ok := s.entries[key]; ok {
			continue
		}
		if !strings.HasSuffix(key, ".wa1") && !strings.HasSuffix(key, ".mpg") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, de.Name()))
		if err != nil {
			continue
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}
		s.files = append(s.files, f)
		s.entries[key] = &Entry{Name: de.Name(), Size: uint32(st.Size()), dat: f, standalone: true}
		s.order = append(s.order, de.Name())
	}
	if len(s.entries) == 0 {
		return nil, fmt.Errorf("blackpackage: %s 下没有可识别的 LST/DAT", dir)
	}
	return s, nil
}

func findCI(names []os.DirEntry, want string) string {
	want = strings.ToLower(want)
	for _, de := range names {
		if strings.ToLower(de.Name()) == want {
			return de.Name()
		}
	}
	return ""
}

func (s *Set) addPair(lstPath, datPath string) error {
	lst, err := os.ReadFile(lstPath)
	if err != nil {
		return err
	}
	if len(lst)%lstEntrySize != 0 {
		return fmt.Errorf("blackpackage: %s 大小不是 %d 的倍数", lstPath, lstEntrySize)
	}
	dat, err := os.Open(datPath)
	if err != nil {
		return err
	}
	st, err := dat.Stat()
	if err != nil {
		dat.Close()
		return err
	}
	max := uint64(st.Size())
	s.files = append(s.files, dat)
	for off := 0; off+lstEntrySize <= len(lst); off += lstEntrySize {
		rec := lst[off : off+lstEntrySize]
		name := strings.TrimRight(string(rec[:14]), "\x00")
		if name == "" {
			continue
		}
		eo := binary.LittleEndian.Uint32(rec[14:18])
		es := binary.LittleEndian.Uint32(rec[18:22])
		if uint64(eo)+uint64(es) > max {
			return fmt.Errorf("blackpackage: %s 条目越界 (%s)", datPath, name)
		}
		key := strings.ToLower(name)
		if _, ok := s.entries[key]; ok {
			continue
		}
		s.entries[key] = &Entry{Name: name, Offset: eo, Size: es, dat: dat}
		s.order = append(s.order, name)
	}
	return nil
}

// Names 返回全部资源名（保持索引顺序）。
func (s *Set) Names() []string {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

// Has 判断名字是否存在（不区分大小写，允许省略扩展名）。
func (s *Set) Has(name string) bool {
	_, ok := s.find(name)
	return ok
}

func (s *Set) find(name string) (*Entry, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if e, ok := s.entries[key]; ok {
		return e, true
	}
	// 剧本/图片引用有时只给主干名。按主干找第一个同主干的条目。
	base := strings.TrimSuffix(key, filepath.Ext(key))
	for k, e := range s.entries {
		if strings.TrimSuffix(k, filepath.Ext(k)) == base {
			return e, true
		}
	}
	return nil, false
}

// Entry 读取并返回资源原始字节。SO4 会在这里解掉 8 字节 LZSS 头；
// 素材本身不再二次压缩，直接返回。名字不存在返回 error。
func (s *Set) Entry(name string) ([]byte, error) {
	e, ok := s.find(name)
	if !ok {
		return nil, fmt.Errorf("blackpackage: 找不到资源 %q", name)
	}
	if e.standalone {
		return os.ReadFile(e.dat.Name())
	}
	buf := make([]byte, e.Size)
	if _, err := e.dat.ReadAt(buf, int64(e.Offset)); err != nil {
		return nil, err
	}
	if strings.HasSuffix(strings.ToLower(e.Name), ".so4") && len(buf) >= 8 {
		packed := binary.LittleEndian.Uint32(buf[0:4])
		unpacked := binary.LittleEndian.Uint32(buf[4:8])
		if int64(packed)+8 == int64(len(buf)) && unpacked > 0 {
			return Decompress(buf[8:8+packed], int(unpacked))
		}
	}
	return buf, nil
}

// Close 关闭所有底层文件。
func (s *Set) Close() {
	if s == nil {
		return
	}
	for _, f := range s.files {
		if f != nil {
			f.Close()
		}
	}
	s.files = nil
	s.entries = nil
}

// Dir 返回打开时的游戏目录。
func (s *Set) Dir() string { return s.dir }

// Count 返回资源数。
func (s *Set) Count() int { return len(s.entries) }
