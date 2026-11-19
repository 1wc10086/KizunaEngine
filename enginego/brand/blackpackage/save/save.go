// Package save 是 Kizuna 自己的槽位存档。
//
// 原版把存档写成 X0WINnn.DAT 并带一张缩略图；那套二进制布局只在原
// EXE 内有读取器，无法安全地逐字节复刻。这里用 JSON 保存“能继续玩”
// 的最小状态：脚本名、IP、变量/buffer、调用栈、标题和时间。跨平台、
// 可读、坏了也能人工修，代价是与原版存档不互通（原版存档本来也需要
// 注册表和安装目录）。
package save

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kizuna/enginego/brand/blackpackage/script"
	"kizuna/enginego/core"
)

// Slot 是落盘格式。VM 的 vars 用 base64 存，避免 JSON 数字把 int32 变 float。
type Slot struct {
	Version int             `json:"version"`
	Title   string          `json:"title"`
	SavedAt string          `json:"saved_at"`
	Summary string          `json:"summary"`
	Speaker string          `json:"speaker"`
	Text    string          `json:"text"`
	// Bgm 是存档瞬间正在播的 BGM 名。原版读档会接着播当时那首曲子；
	// 不存这一项，从标题读档（新进程，宿主音频全停）就是一片安静。
	Bgm      string          `json:"bgm,omitempty"`
	VM       script.Snapshot `json:"vm"`
	VarsB64  string          `json:"vars_b64"`
	// ScreenB64 是存档瞬间的 800x600 RGBA 画面（zlib + base64）。
	// 原版读档后会沿用当时的画面，脚本不会重画；不存这张图，从标题读档
	// 就会看到标题图配着信息框。压缩后通常只有几十 KB。
	ScreenB64 string `json:"screen_b64,omitempty"`
}

// Meta 是存档里“给人看/给宿主用”的展示信息，与 VM 快照分开传，
// 免得 Save 的参数列表长到看不清哪一项对应什么。
//
// Summary/Text 只用于存档列表和读档后立即显示的台词；Bgm 用于读档后
// 恢复 BGM。它们都不参与脚本执行。
type Meta struct {
	Title   string
	Summary string
	Speaker string
	Text    string
	Bgm     string
}

// Store 管理 <game>/kizuna-save/ 下的槽位文件。
type Store struct {
	dir string
}

// New 建存档目录。
func New(dir string) *Store {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "kizuna-blackpackage", "save")
	}
	_ = os.MkdirAll(dir, 0o755)
	return &Store{dir: dir}
}

func (s *Store) path(slot uint32) string {
	return filepath.Join(s.dir, fmt.Sprintf("slot-%03d.json", slot))
}

// Path 返回槽位文件路径，供 session 镜像成 X0WINnn.DAT。
func (s *Store) Path(slot uint32) string { return s.path(slot) }

// LoadBytes 从 JSON 字节解析槽位（用于 X0WINnn.DAT 镜像）。
func (s *Store) LoadBytes(raw []byte) (script.Snapshot, Slot, error) {
	var rec Slot
	if err := json.Unmarshal(raw, &rec); err != nil {
		return script.Snapshot{}, rec, err
	}
	vars, err := base64.StdEncoding.DecodeString(rec.VarsB64)
	if err != nil {
		return script.Snapshot{}, rec, err
	}
	rec.VM.Vars = vars
	return rec.VM, rec, nil
}

// Save 写一个槽位。Vars 大（约 512KB），base64 后约 700KB，单槽可接受。
// meta 是展示信息（标题/摘要/台词/BGM），screen 是存档瞬间的 RGBA 画面，
// 可为 nil（旧调用方/测试）。
func (s *Store) Save(slot uint32, meta Meta, snap script.Snapshot, screen []byte) error {
	if slot == 0 {
		slot = 1
	}
	if len(snap.Vars) == 0 {
		return fmt.Errorf("blackpackage: 空变量表")
	}
	rec := Slot{
		Version: 1,
		Title:   meta.Title,
		SavedAt: time.Now().Format(time.RFC3339),
		Summary: meta.Summary,
		Speaker: meta.Speaker,
		Text:    meta.Text,
		Bgm:     meta.Bgm,
		VM:      snap,
		VarsB64: base64.StdEncoding.EncodeToString(snap.Vars),
	}
	rec.VM.Vars = nil // 只保留一份
	if len(screen) > 0 {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(screen); err == nil && zw.Close() == nil {
			rec.ScreenB64 = base64.StdEncoding.EncodeToString(buf.Bytes())
		}
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(slot) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(slot))
}

// Screen 解码存档里的画面；没有或损坏时返回 nil。
func (r *Slot) Screen() []byte {
	if r.ScreenB64 == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(r.ScreenB64)
	if err != nil {
		return nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil
	}
	return out
}

// Load 读一个槽位。
func (s *Store) Load(slot uint32) (script.Snapshot, Slot, error) {
	var rec Slot
	raw, err := os.ReadFile(s.path(slot))
	if err != nil {
		return script.Snapshot{}, rec, err
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return script.Snapshot{}, rec, err
	}
	vars, err := base64.StdEncoding.DecodeString(rec.VarsB64)
	if err != nil {
		return script.Snapshot{}, rec, err
	}
	rec.VM.Vars = vars
	return rec.VM, rec, nil
}

// List 返回 1..count 的槽位快照。没有档的槽 Exists=false。
func (s *Store) List(count int) []core.Slot {
	out := make([]core.Slot, 0, count)
	for i := 1; i <= count; i++ {
		cs := core.Slot{Index: uint32(i)}
		var rec Slot
		if raw, err := os.ReadFile(s.path(uint32(i))); err == nil {
			if json.Unmarshal(raw, &rec) == nil {
				cs.Exists = true
				cs.Title = rec.Title
				cs.SavedAt = rec.SavedAt
				cs.Summary = rec.Summary
			}
		}
		out = append(out, cs)
	}
	return out
}

// Clean 删掉损坏/过旧的临时文件；Open 时调用一次即可。
func (s *Store) Clean() {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, de := range ents {
		if strings.HasSuffix(de.Name(), ".tmp") {
			_ = os.Remove(filepath.Join(s.dir, de.Name()))
		}
	}
}

// SortedSlotNumbers 供日志/排序使用。
func (s *Store) SortedSlotNumbers() []int {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var out []int
	for _, de := range ents {
		n := strings.TrimSuffix(strings.TrimPrefix(de.Name(), "slot-"), ".json")
		if v, err := strconv.Atoi(n); err == nil {
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}
