package arc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 存档集合（overdose_spec §1）：
//
//	graphic.arc .cpb     script.arc .asb/.nut
//	sound.arc / music.arc / voice.arc .ogg
//	system.arc .tbl
//
// Set 只定位不解内容——解耦了按类查表的需求（Session 需要从任意类取条目）。
// 也注意：Graphic 可能有多于 2000 条，若每次都扫全表就慢，
// 这里的 byName 已预建，好过调用方自己轮查。
type Set struct {
	Dir     string
	Graphic *Archive
	Script  *Archive
	Sound   *Archive
	Music   *Archive
	Voice   *Archive
	System  *Archive
}

// OpenSet 在 dir 里打开 *.arc（按文件名分五类），单座坏了不丢全局。
// 允许缺席某些类（同目录也许只有 system.arc），其余类照常可用。
func OpenSet(dir string) (*Set, error) {
	dir = resolveDir(dir) // dir 可以是父目录 YUI/，本函数会自动下钻到優衣
	if dir == "" {
		return nil, fmt.Errorf("arc: 定位不到游戏数据目录（传过来的路径：%s）", dir)
	}
	s := &Set{Dir: dir}
	// 保留扩展名的大小写无关；按固定次序遍历保证确定性
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(de.Name()), ".arc") {
			files = append(files, filepath.Join(dir, de.Name()))
		}
	}
	sort.Strings(files)
	for _, path := range files {
		lower := strings.ToLower(filepath.Base(path))
		arc, err := Open(path)
		if err != nil {
			// 单座坏了不丢全局：其余类别是否可用与它无关。
			_ = err
			continue
		}
		switch lower {
		case "graphic.arc":
			s.Graphic = arc
		case "script.arc":
			s.Script = arc
		case "sound.arc":
			s.Sound = arc
		case "music.arc":
			s.Music = arc
		case "voice.arc":
			s.Voice = arc
		case "system.arc":
			s.System = arc
		}
	}
	if s.Graphic == nil && s.Script == nil && s.System == nil {
		return nil, fmt.Errorf("arc: %s 里一座可用的 .arc 都没有", dir)
	}
	return s, nil
}

// Close 关闭所有已打开的存档。
func (s *Set) Close() {
	for _, a := range []*Archive{s.Graphic, s.Script, s.Sound, s.Music, s.Voice, s.System} {
		if a != nil {
			a.Close()
		}
	}
}

// Entry 在集合里按固定次序去找：先系统（.tbl），再脚本（.asb/.nut），
// 再图形（.cpb），后音频（.ogg）。重名时以这种次序优先。
func (s *Set) Entry(name string) ([]byte, error) {
	for _, a := range []*Archive{s.System, s.Script, s.Graphic, s.Sound, s.Music, s.Voice} {
		if a == nil {
			continue
		}
		if _, ok := a.find(name); ok {
			return a.Entry(name)
		}
	}
	return nil, fmt.Errorf("arc: 集合里没有条目 %q", name)
}

// Names 在集合里找满足 ext 的全部名并保持原序。
func (s *Set) Names(ext string) []string {
	var out []string
	for _, a := range []*Archive{s.System, s.Script, s.Graphic, s.Sound, s.Music, s.Voice} {
		if a == nil {
			continue
		}
		out = append(out, a.Names(ext)...)
	}
	return out
}

// Image 只取 graphic.arc 的图形帧。
func (s *Set) Image(name string) (int, int, []byte, error) {
	if s.Graphic == nil {
		return 0, 0, nil, fmt.Errorf("arc: graphic.arc 未打开")
	}
	return s.Graphic.Image(name)
}

// Table 只取 system.arc 的表。
func (s *Set) Table(name string) ([]Record, error) {
	if s.System == nil {
		return nil, fmt.Errorf("arc: system.arc 未打开")
	}
	return s.System.Table(name)
}

// EntrySize 的查找次序与 Archive.find 保持一致（包含无后缀兜底）。
func (s *Set) EntrySize(name string) (int64, bool) {
	for _, a := range []*Archive{s.System, s.Script, s.Graphic, s.Sound, s.Music, s.Voice} {
		if a == nil {
			continue
		}
		if _, ok := a.find(name); ok {
			return a.EntrySize(name)
		}
	}
	return 0, false
}

// FindGameDir 允许三种输入：数据目录（優衣）、数据目录父级（YUI/）、
// 或者其中任何 arc 本体。逐级向上挖或上两层挖最大一两层。
// 只有到命中的目录才算真：魔数解出来是 ARC\0 才算，别的文件一眼可擦。
func FindGameDir(input string) (string, bool) {
	dir := resolveDir(input)
	return dir, dir != ""
}

// resolveDir 是 FindGameDir 的本体，返回解出的真实目录或空。
func resolveDir(input string) string {
	// 若传的是 arc 本体
	if fi, err := os.Stat(input); err == nil && !fi.IsDir() {
		if strings.EqualFold(filepath.Ext(input), ".arc") {
			return filepath.Dir(input)
		}
		input = filepath.Dir(input)
	}
	// input 自身就是
	if isGameDir(input) {
		return input
	}
	// input/優衣, input/YUI/優衣 两层
	cands := []string{
		filepath.Join(input, "優衣"),
		filepath.Join(input, "YUI", "優衣"),
	}
	for _, c := range cands {
		if isGameDir(c) {
			return c
		}
	}
	// 单层扫描：上一层的数据目录可能落在任何名字里（如中文名不写“優衣”）
	// 这里不要太放量，只看 immediate 子目录的两层，避免全盘乱序（Be aitch）。
	if des, err := os.ReadDir(input); err == nil {
		for _, de := range des {
			if !de.IsDir() {
				continue
			}
			if isGameDir(filepath.Join(input, de.Name())) {
				return filepath.Join(input, de.Name())
			}
			// 再下一层
			subdir := filepath.Join(input, de.Name())
			if des2, err2 := os.ReadDir(subdir); err2 == nil {
				for _, de2 := range des2 {
					if de2.IsDir() && isGameDir(filepath.Join(subdir, de2.Name())) {
						return filepath.Join(subdir, de2.Name())
					}
				}
			}
		}
	}
	return ""
}

// isGameDir 用是否能解出 ARC\0 头或“yui.exe + script.arc 并存”断言。
// *.sum 与任意文件皆无关，不用去打开。
func isGameDir(dir string) bool {
	if dir == "" {
		return false
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	// 任一 *.arc 解出头
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(de.Name()), ".arc") {
			if IsArchive(filepath.Join(dir, de.Name())) {
				return true
			}
		}
	}
	// 兜底：看经可启动 exe 的标记（spec §1）
	_, a := os.Stat(filepath.Join(dir, "yui.exe"))
	_, b := os.Stat(filepath.Join(dir, "script.arc"))
	return a == nil && b == nil
}

// IsGameDir 公开发言器供 Session 在正式打开前单独断言。
func IsGameDir(dir string) bool {
	return isGameDir(dir)
}
