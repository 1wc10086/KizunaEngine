// Package save 是 Kizuna 自己的槽位存档（TMR-Hiro 品牌）。
//
// 原版把进度写在 <root>/save/savedata.NNN（二进制 + 缩略图）；那套布局只在
// 原 EXE 里有读取器，无法安全地逐字节复刻。这里沿用其它品牌的 JSON 方案：
// 只保存“能继续玩”的最小状态（脚本名、记录号、变量、标题、时间），
// 跨平台、可读、坏了也能人工修。代价是与原版存档不互通（原版存档本来也
// 依赖游戏目录里的 SD 卡与注册表）。
package save

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kizuna/enginego/core"
)

// Slot 是落盘格式。
type Slot struct {
	Version int    `json:"version"`
	Title   string `json:"title"`
	SavedAt string `json:"saved_at"`
	Summary string `json:"summary"`
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
	// Script/Index 是剧本名与记录号：读档从这条记录继续。
	Script string `json:"script"`
	Index  int    `json:"index"`
	// CallStack 是跨脚本调用栈（script:index）。
	CallStack []string `json:"call_stack,omitempty"`
	// Vars 是脚本变量（$glo/$tmp/$flg 三类，键是原样变量名）。
	Vars map[string]int32 `json:"vars,omitempty"`
	// Stage 是画面快照：背景、立绘、CG、覆盖色。读档要“原地继续”，
	// 不存这些就得重跑一遍剧本才能画出同一屏，玩家会看到黑屏再闪一下。
	Stage StageState `json:"stage"`
	// Bgm 是存档瞬间正在播的曲子（从标题读档时接着播）。
	Bgm string `json:"bgm,omitempty"`
	// Seen 是周目内已读文本的指纹集合（用于“跳过未读”），
	// 与原版的“已读”标记同义。
	Seen []string `json:"seen,omitempty"`
}

// StageState 是画面快照。
type StageState struct {
	Background string     `json:"background,omitempty"`
	CG         string     `json:"cg,omitempty"`
	Chars      []CharSlot `json:"chars,omitempty"`
	FadeColor  string     `json:"fade_color,omitempty"`
	FadeAlpha  int        `json:"fade_alpha,omitempty"`
}

// CharSlot 是立绘槽快照。
type CharSlot struct {
	Name  string  `json:"name"`
	X     int     `json:"x"`
	Y     int     `json:"y"`
	Scale float64 `json:"scale"`
}

// Store 管理 <game>/kizuna-save/ 下的槽位文件。
type Store struct {
	dir string
}

// New 建存档目录。
func New(dir string) *Store {
	_ = os.MkdirAll(dir, 0o755)
	return &Store{dir: dir}
}

// Dir 返回存档目录。
func (s *Store) Dir() string { return s.dir }

// Path 返回槽位文件路径。
func (s *Store) Path(slot uint32) string {
	return filepath.Join(s.dir, fmt.Sprintf("slot%03d.json", slot))
}

// Save 写一个槽位。
func (s *Store) Save(slot uint32, snapshot Slot) error {
	if slot == 0 {
		return fmt.Errorf("tmrhiro/save: 槽位 0 是自动存档位，不允许直接写")
	}
	// 与原版一致：自动存档位（0）以外的槽位从 1 开始。
	snapshot.Version = 1
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	path := s.Path(slot)
	if err := core.EnsureParentDir(path); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load 读一个槽位。
func (s *Store) Load(slot uint32) (Slot, error) {
	raw, err := os.ReadFile(s.Path(slot))
	if err != nil {
		return Slot{}, fmt.Errorf("tmrhiro/save: 槽位 %d 无档: %w", slot, err)
	}
	var snapshot Slot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return Slot{}, fmt.Errorf("tmrhiro/save: 槽位 %d 损坏: %w", slot, err)
	}
	return snapshot, nil
}

// Exists 报告槽位是否有档。
func (s *Store) Exists(slot uint32) bool { return core.FileExists(s.Path(slot)) }

// Delete 删档。
func (s *Store) Delete(slot uint32) error { return os.Remove(s.Path(slot)) }

// List 列出 1..max 的槽位元信息（按槽号升序）。
func (s *Store) List(max uint32) []Slot {
	out := make([]Slot, 0, max)
	for slot := uint32(1); slot <= max; slot++ {
		snapshot, err := s.Load(slot)
		if err != nil {
			out = append(out, Slot{})
			continue
		}
		snapshot.Summary = strings.TrimSpace(snapshot.Summary)
		out = append(out, snapshot)
	}
	return out
}

// Preview 返回槽位预览（含缩略图名）。缩略图按原版命名写到 save/ss/ 下的
// “savedataNNN.bmp”，由 session 在 Save 时落盘；这里只给路径。
func (s *Store) Preview(slot uint32) (Slot, string) {
	snapshot, err := s.Load(slot)
	if err != nil {
		return Slot{}, ""
	}
	thumb := filepath.Join(s.dir, "thumb", fmt.Sprintf("slot%03d.bmp", slot))
	if !core.FileExists(thumb) {
		thumb = ""
	}
	return snapshot, thumb
}

// FormatClock 生成原版风格的存档时间串（"No.%03d : YYYY/MM/DD HH:MM"）。
func FormatClock(slot uint32, at time.Time) string {
	return fmt.Sprintf("No.%03d : %4d/%02d/%02d %02d:%02d", slot, at.Year(), int(at.Month()), at.Day(), at.Hour(), at.Minute())
}

// SlotNumber 把 "slot007.json" 解析成 7（扫目录时用）。
func SlotNumber(name string) (uint32, bool) {
	if !strings.HasPrefix(name, "slot") || !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "slot"), ".json"))
	if err != nil || value <= 0 {
		return 0, false
	}
	return uint32(value), true
}

// Existing 扫描目录里所有槽位号（升序）。
func (s *Store) Existing() []uint32 {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	slots := make([]uint32, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if slot, ok := SlotNumber(entry.Name()); ok {
			slots = append(slots, slot)
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i] < slots[j] })
	return slots
}

// Thumb 是存档缩略图（128x72 的 BGRA，与引擎画布同字节序）。
//
// 为什么要自己存一份：原版存档目录里带 savedataNNN.png，宿主的读档界面直接
// 贴它；我们的存档是 JSON，没有二进制附件。缩略图不参与读档恢复（读完会
// 重放画面），只在存读档界面显示，所以按原始字节落盘最省事——不用引入
// PNG 编码器，也不怕 JSON 体积膨胀。
type Thumb struct {
	Width  int
	Height int
	Pixels []byte
}

// ThumbPath 返回缩略图文件路径。
func (s *Store) ThumbPath(slot uint32) string {
	return filepath.Join(s.dir, "thumb", fmt.Sprintf("slot%03d.bgra", slot))
}

// SaveThumb 写缩略图。宽度/高度写进文件头（小端 u32×2），之后是像素。
func (s *Store) SaveThumb(slot uint32, thumb Thumb) error {
	if slot == 0 || thumb.Width <= 0 || thumb.Height <= 0 || len(thumb.Pixels) < thumb.Width*thumb.Height*4 {
		return fmt.Errorf("tmrhiro/save: 缩略图非法")
	}
	path := s.ThumbPath(slot)
	if err := core.EnsureParentDir(path); err != nil {
		return err
	}
	buf := make([]byte, 8+thumb.Width*thumb.Height*4)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(thumb.Width))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(thumb.Height))
	copy(buf[8:], thumb.Pixels[:thumb.Width*thumb.Height*4])
	return core.AtomicWrite(path, buf)
}

// LoadThumb 读缩略图；没有时返回 ok=false。
func (s *Store) LoadThumb(slot uint32) (Thumb, bool) {
	raw, err := os.ReadFile(s.ThumbPath(slot))
	if err != nil || len(raw) < 8 {
		return Thumb{}, false
	}
	width := int(binary.LittleEndian.Uint32(raw[0:4]))
	height := int(binary.LittleEndian.Uint32(raw[4:8]))
	if width <= 0 || height <= 0 || width > 1024 || height > 1024 {
		return Thumb{}, false
	}
	if len(raw) < 8+width*height*4 {
		return Thumb{}, false
	}
	return Thumb{Width: width, Height: height, Pixels: raw[8 : 8+width*height*4]}, true
}

// DeleteThumb 删缩略图（删档时一起删）。
func (s *Store) DeleteThumb(slot uint32) error {
	err := os.Remove(s.ThumbPath(slot))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
