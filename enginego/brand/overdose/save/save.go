package save

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kizuna/enginego/brand/overdose/script"
)

// 为什么存档用 JSON 而不走原引擎的 save/ 格式：overdose_spec §8 已明确
// 不尝试对齐原版格式，其余品牌统一走 KIZUNA-SAVE1 风格后，再发明一
// 套二进制格式只会多一处需要单独维护的编解码路径。这里延续 JSON 路线，
// 唯一不同是结构按 ASB 的真实运行图收敛：变量是 6 个命名空间的槽位而
// 非“全局/局部两只 map”，舞台是全局字符串 var0/var1 引出的层而非
// emon 的 sprite 表。
const (
	// FormatVersion 是存档 JSON 的结构版本。读取时版本不符直接拒绝，
	// 不做“尽力兼容”——ASB 指令流的偏移坐标依赖脚本版本，跨版本猜测
	// 恢复只会把 PC 落在半条指令上。
	FormatVersion = 1

	// Magic 是文件头前 8 字节的标识（与 emon 的魔字同理，区分本品牌的
	// 槽位档与其它品牌的 slot 文件）。
	Magic = "KIZUNA-SAVE1"

	// MaxSlot 是总槽位上限。原版是 10 bank × 10 slot，session 对外用
	// 1..100（bank*10+slot+1），所以上限必须是 100，不是 99。
	MaxSlot = 100
)

// LayerState 是舞台的一层快照。
// 为什么单列出 layer 而不复用 script.VariableStore：舞台的合成规则
// （哪层在前、哪层是 CG）在 stage 包里，存档只记“当前帧里有什么”，
// 恢复时由 session 重建合成；把 stage 内部的浮点动画时间抄进存档
// 会让存档格式与 stage 的内部调整相互锁死。
type LayerState struct {
	Index    int32   `json:"index,omitempty"`
	Name     string  `json:"name"`
	X        int32   `json:"x,omitempty"`
	Y        int32   `json:"y,omitempty"`
	Alpha    int32   `json:"alpha,omitempty"`
	Scale    float64 `json:"scale,omitempty"`
	Visible  bool    `json:"visible,omitempty"`
	Centered bool    `json:"centered,omitempty"`
}

// StageState 是整个舞台的快照：背景层 + 人物层 + CG + 滤镜。
// bg/人物/CG 的名字都来自全局字符串变量（var0=背景、var1=角色），
// 与脚本里 0x11 写进变量的取值同一口径，恢复时直接塞回变量即可，
// 不需要再单独解释一遍第几层画的是哪张图。
type StageState struct {
	Background     string       `json:"background,omitempty"`
	Characters     []LayerState `json:"characters,omitempty"`
	CG             string       `json:"cg,omitempty"`
	Layers         []LayerState `json:"layers,omitempty"`
	Fade           string       `json:"fade,omitempty"` // "black"/"white"/"none"
	MessageVisible bool         `json:"messageVisible,omitempty"`
}

// AudioState 是音频通道快照（字段与 audio.SaveState 解耦——这里用
// JSON 可直接序列化的普通结构体，save 包对 audio 包不设依赖，
// 只在 session 层把两者接起来）。
type AudioState struct {
	BgmFile string `json:"bgmFile,omitempty"`
	BgmLoop bool   `json:"bgmLoop,omitempty"`
	BgmVol  int32  `json:"bgmVol,omitempty"`
	// SeFile/SEVol 仅记录 SE 池上一条（ASB 的 SE 不跨帧保持，存档
	// 只为读档后回放参考留一笔，不承诺原声恢复）。
	SeFile    string `json:"seFile,omitempty"`
	SeVol     int32  `json:"seVol,omitempty"`
	VoiceFile string `json:"voiceFile,omitempty"`
	VoiceVol  int32  `json:"voiceVol,omitempty"`
}

// MessageState 是当前对话框的快照：读档要落回的那一句。
type MessageState struct {
	Speaker string `json:"speaker,omitempty"`
	Text    string `json:"text,omitempty"`
}

// FrameState 是调用栈的一帧：脚本名 + 字节偏移（与脚本流的 PC 同口径）。
// 偏移是流内字节位置（opcodes_spec §1），不要存“第几条指令”——
// 0x04/0x05 的 field 就是它，恢复时直接赋值给 VM 栈即可。
type FrameState struct {
	Script string `json:"script"`
	Offset int    `json:"offset"`
}

// VariableSnapshot 是 6 个命名空间的“非默认”槽位表。
// 存储形态为 "ns:index" -> 文本，并非 VariableStore.Snapshot 的 map 形态：
// 后者的 key 是未导出类型 namespacedInt，JSON 化会产生不易读的键；
// 这里与 emon 的 map[string]EmonValue 做同一件事——键可读、可手改，
// 而既有 Snapshot/Restore 在 Load/Save 时由 session 一次性转换。
type VariableSnapshot map[string]string

// Settings 是跨周目保留的设置（写入系统文件，非槽位文件）。
// 字段语义与原版 opt.* 对齐：
//   - TextSpeed 是档位 1=slow(100ms)/2=normal(30ms)/3=fast(0ms)，不是毫秒；
//     原版 sq_setting 的 _GetMessageSpeed 就是按 100/0/其它 三档判断。
//   - AutoDelayMs 是自动播放每句等待毫秒（2000/1000/200）。
//   - ClickRightMode 1=关闭窗口/退出确认，2=设置，3=重播语音。
//   - UseChara 顺序是 0=YUI(3), 1=KYOKO(1), 2=MIYUKI(2)，与脚本的
//     SetUseChara(3/1/2, ...) 一一对应。
//
// Initialized 用来区分“旧系统文件没有设置块”和“玩家真的把音量调到 0”。
type Settings struct {
	Initialized    bool    `json:"initialized,omitempty"`
	FullScreen     bool    `json:"fullScreen,omitempty"`
	TextSpeed      int32   `json:"textSpeed,omitempty"` // 1/2/3 档位
	AutoEnabled    bool    `json:"autoEnabled,omitempty"`
	AutoDelayMs    int32   `json:"autoDelayMs,omitempty"`
	SkipEnabled    bool    `json:"skipEnabled,omitempty"`
	SkipReadOnly   bool    `json:"skipReadOnly,omitempty"`
	MasterVolume   int32   `json:"masterVolume,omitempty"`  // 0..100
	BgmVolume      int32   `json:"bgmVolume,omitempty"`     // 0..100
	SeVolume       int32   `json:"seVolume,omitempty"`      // 0..100
	VoiceVolume    int32   `json:"voiceVolume,omitempty"`   // 0..100
	WindowOpacity  int32   `json:"windowOpacity,omitempty"` // 0..100
	EffectEnabled  bool    `json:"effectEnabled,omitempty"`
	ClickRightMode int32   `json:"clickRightMode,omitempty"` // 1/2/3
	UseChara       [3]bool `json:"useChara,omitempty"`
	WindowVisible  bool    `json:"windowVisible,omitempty"`
}

// DefaultSettings 返回原版首次启动的设置。三档速度与原版 sq_setting 一致。
func DefaultSettings() Settings {
	return Settings{
		Initialized:    true,
		FullScreen:     true,
		TextSpeed:      2,
		AutoDelayMs:    1000,
		MasterVolume:   100,
		BgmVolume:      100,
		SeVolume:       100,
		VoiceVolume:    100,
		WindowOpacity:  100,
		EffectEnabled:  true,
		ClickRightMode: 2,
		UseChara:       [3]bool{true, true, true},
		WindowVisible:  true,
	}
}

// NormalizeSettings 把缺失/越界的字段收敛到合法值。
// 旧系统文件没有 Initialized 时整块回默认；否则只补越界项，
// 避免把玩家明确设置的 0 音量又改回 100。
func NormalizeSettings(s Settings) Settings {
	if !s.Initialized {
		return DefaultSettings()
	}
	if s.TextSpeed < 1 || s.TextSpeed > 3 {
		s.TextSpeed = 2
	}
	if s.AutoDelayMs <= 0 {
		s.AutoDelayMs = 1000
	}
	for _, v := range []*int32{&s.MasterVolume, &s.BgmVolume, &s.SeVolume, &s.VoiceVolume, &s.WindowOpacity} {
		if *v < 0 {
			*v = 0
		}
		if *v > 100 {
			*v = 100
		}
	}
	if s.ClickRightMode < 1 || s.ClickRightMode > 3 {
		s.ClickRightMode = 2
	}
	return s
}

// SlotData 是槽位档的完整载荷。
type SlotData struct {
	Version    int32            `json:"version"`
	Slot       int32            `json:"slot"`
	Title      string           `json:"title,omitempty"`
	Summary    string           `json:"summary,omitempty"`
	SavedAt    time.Time        `json:"savedAt"`
	Script     string           `json:"script"`
	Stack      []FrameState     `json:"stack,omitempty"`
	PC         int              `json:"pc,omitempty"`         // 当前帧 PC（字节偏移）
	InstrIndex int              `json:"instrIndex,omitempty"` // 当前帧内第几条指令（辅助定位）
	Variables  VariableSnapshot `json:"variables,omitempty"`
	Stage      StageState       `json:"stage,omitempty"`
	Audio      AudioState       `json:"audio,omitempty"`
	Message    MessageState     `json:"message,omitempty"`
}

// SystemData 是系统/全局文件（gallery 解锁 + settings + 已读）的一份快照。
type SystemData struct {
	Version      int32    `json:"version"`
	CgUnlocks    []string `json:"cgUnlocks,omitempty"`
	SceneUnlocks []string `json:"sceneUnlocks,omitempty"`
	MusicUnlocks []string `json:"musicUnlocks,omitempty"`
	// ReadTextKeys 记已读对白的标识（脚本名+行/文本哈希由 session 定，
	// 本包只负责存取这串不透明键）。
	ReadTextKeys []string `json:"readTextKeys,omitempty"`
	Settings     Settings `json:"settings,omitempty"`
}

// -------- 变量快照转换（桥接 script.VariableStore）--------

// ExportVariables 把 VariableStore 的非默认槽位导出为可 JSON 化的表。
// 为什么只存非默认：600 个槽位里绝大多数永远是 0/""，全存的话存档
// 是空变量的 10 倍大；读入时对照默认 0/"" 即可，无需任何标志位补齐。
func ExportVariables(vs *script.VariableStore) VariableSnapshot {
	if vs == nil {
		return nil
	}
	// script.VariableStore.Snapshot 返回 map[namespacedInt]string，
	// key 是未导出类型，save 包无法引用。这里按公开的 GetVar 逐槽
	// 展开，语义与 Snapshot 等价（同样只取非默认），但不依赖内部布局。
	out := make(VariableSnapshot)
	for ns := uint16(0); ns < 6; ns++ {
		kind, ok := scriptKindOf(ns)
		if !ok {
			continue
		}
		for idx := uint16(0); idx < 100; idx++ {
			v, err := vs.GetVar(ns, idx)
			if err != nil {
				continue
			}
			s := v.Display()
			switch kind {
			case "bool":
				if !v.AsBool() {
					continue
				}
				s = "1"
			case "int":
				if v.AsInt() == 0 {
					continue
				}
				s = itoa(int(v.AsInt()))
			default:
				if s == "" {
					continue
				}
			}
			out[varKey(ns, idx)] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ImportVariables 把存档里的表写回 VariableStore。
// 未存的槽位不触碰：调用方需先用一个新 VariableStore 再导入，
// 不能把部分存档写进正在运行的 VM（会残留已存档不存在的值）。
func ImportVariables(vs *script.VariableStore, snap VariableSnapshot) {
	if vs == nil || len(snap) == 0 {
		return
	}
	// script.VariableStore.Restore 收的是 map[namespacedInt]string，
	// 该 key 类型未导出，构造不出。为不依赖未导出标识，这里按
	// Set* 方法逐条写入，语义与 Restore 一致。
	for key, val := range snap {
		ns, idx, ok := parseVarKey(key)
		if !ok {
			continue
		}
		kind, ok := scriptKindOf(ns)
		if !ok {
			continue
		}
		switch kind {
		case "bool":
			_ = vs.SetBool(ns, idx, val == "1")
		case "int":
			_ = vs.SetInt(ns, idx, parseInt(val))
		default:
			_ = vs.SetString(ns, idx, val)
		}
	}
}

func varKey(ns, idx uint16) string {
	return itoa(int(ns)) + ":" + itoa(int(idx))
}

func parseVarKey(key string) (uint16, uint16, bool) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	ns := parseInt(parts[0])
	idx := parseInt(parts[1])
	if ns < 0 || ns > 5 || idx < 0 || idx > 99 {
		return 0, 0, false
	}
	return uint16(ns), uint16(idx), true
}

// scriptKindOf 固定 6 个命名空间的种类（与 script 包 namesOf 的约定一致）。
func scriptKindOf(ns uint16) (string, bool) {
	switch ns {
	case 0, 3:
		return "bool", true
	case 1, 4:
		return "int", true
	case 2, 5:
		return "string", true
	}
	return "", false
}

func parseInt(s string) int32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	var n int32
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int32(r-'0')
	}
	if neg {
		return -n
	}
	return n
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [12]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// -------- Store：槽位 + 系统文件 --------

// Store 统一管理槽位目录与系统文件。
// 为什么槽位与系统文件共用一个 Store：session.Close 时只需落一份
// 系统文件；若两者各持目录，退出时极易只写其一。
type Store struct {
	dir string
	sys *SystemStore
}

// New 新建 Store。dir 是槽位目录（如 <gameDir>/save 或缓存下的 save 目录），
// 系统文件一并落在该目录下（与 emon 的 SAVEDATA 目录同理）。
func New(dir string) *Store {
	return &Store{dir: dir, sys: NewSystemStore(dir)}
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) System() *SystemStore { return s.sys }

// SlotPath 返回第 slot 个槽位的文件路径。
func (s *Store) SlotPath(slot int) string {
	return filepath.Join(s.dir, slotFileName(slot))
}

func slotFileName(slot int) string {
	return "slot" + zeroPad3(slot) + ".json"
}

func zeroPad3(v int) string {
	s := itoa(v)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// Save 把 SlotData 写进第 slot 个槽位（原子落盘：临时文件 + rename）。
func (s *Store) Save(slot int, data *SlotData) error {
	if slot < 0 || slot > MaxSlot {
		return errors.New("save: 槽位越界")
	}
	if data == nil {
		return errors.New("save: 数据为空")
	}
	cp := *data
	cp.Version = FormatVersion
	cp.Slot = int32(slot)
	if cp.SavedAt.IsZero() {
		cp.SavedAt = time.Now()
	}
	body, err := marshalSlot(&cp)
	if err != nil {
		return err
	}
	if err := ensureDir(s.dir); err != nil {
		return err
	}
	return atomicWrite(s.SlotPath(slot), body)
}

// Load 读第 slot 个槽位；不存在返回 (nil, nil)——“新玩家还没存过”是
// 正常状态，不是错误。
func (s *Store) Load(slot int) (*SlotData, error) {
	if slot < 0 || slot > MaxSlot {
		return nil, errors.New("save: 槽位越界")
	}
	raw, err := os.ReadFile(s.SlotPath(slot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return unmarshalSlot(raw)
}

// Delete 删除第 slot 个槽位；不存在返回 nil（幂等）。
func (s *Store) Delete(slot int) error {
	if slot < 0 || slot > MaxSlot {
		return errors.New("save: 槽位越界")
	}
	err := os.Remove(s.SlotPath(slot))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Exists 判断槽位是否已有档。
func (s *Store) Exists(slot int) bool {
	if slot < 0 || slot > MaxSlot {
		return false
	}
	_, err := os.Stat(s.SlotPath(slot))
	return err == nil
}

// Slots 列举已有档位的槽位号（升序）。
func (s *Store) Slots() []int {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n, ok := parseSlotFileName(e.Name())
		if !ok {
			continue
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func parseSlotFileName(name string) (int, bool) {
	if !strings.HasPrefix(name, "slot") || !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, "slot"), ".json")
	if len(mid) != 3 {
		return 0, false
	}
	n := 0
	for _, r := range mid {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if n < 0 || n > MaxSlot {
		return 0, false
	}
	return n, true
}

// SlotsWithMeta 列举全部槽位并附摘要（供 session.Slots() 用）。
func (s *Store) SlotsWithMeta() []SlotMeta {
	var out []SlotMeta
	for slot := 0; slot <= MaxSlot; slot++ {
		data, err := s.Load(slot)
		if err != nil || data == nil {
			continue
		}
		out = append(out, SlotMeta{
			Slot:    slot,
			Title:   data.Title,
			Summary: data.Summary,
			SavedAt: data.SavedAt,
		})
	}
	return out
}

// SlotMeta 是槽位摘要。
type SlotMeta struct {
	Slot    int
	Title   string
	Summary string
	SavedAt time.Time
}

// -------- 系统文件 --------

// SystemStore 管理系统文件（CG/场景/音乐解锁 + 设置 + 已读）。
type SystemStore struct {
	dir  string
	path string
}

const systemFileName = "system.json"

// NewSystemStore 新建。
func NewSystemStore(dir string) *SystemStore {
	return &SystemStore{dir: dir, path: filepath.Join(dir, systemFileName)}
}

func (s *SystemStore) Path() string { return s.path }

// Load 读系统文件；不存在返回 (nil, nil)（全新安装）。
func (s *SystemStore) Load() (*SystemData, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var d SystemData
	if err := json.Unmarshal(stripMagic(raw), &d); err != nil {
		return nil, err
	}
	if d.Version != FormatVersion {
		return nil, errors.New("save: 系统文件版本不符")
	}
	return &d, nil
}

// Save 原子写系统文件。
func (s *SystemStore) Save(d *SystemData) error {
	if d == nil {
		return errors.New("save: 系统数据为空")
	}
	cp := *d
	cp.Version = FormatVersion
	body, err := json.Marshal(&cp)
	if err != nil {
		return err
	}
	// 头部魔字：与槽位文件同约定，便于按文件头识别品牌。
	full := append(append([]byte{}, []byte(Magic)...), body...)
	if err := ensureDir(s.dir); err != nil {
		return err
	}
	return atomicWrite(s.path, full)
}

// -------- 编解码小工具 --------

func marshalSlot(d *SlotData) ([]byte, error) {
	body, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return append(append([]byte{}, []byte(Magic)...), body...), nil
}

func unmarshalSlot(raw []byte) (*SlotData, error) {
	body := stripMagic(raw)
	var d SlotData
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	if d.Version != FormatVersion {
		return nil, errors.New("save: 槽位版本不符")
	}
	return &d, nil
}

// stripMagic 剥去头部魔字；缺魔字也不算错（为将来格式过渡留容错，
// 但 JSON 结构仍必须对），这里是“文件头可识别”而非“硬校验”。
func stripMagic(raw []byte) []byte {
	if len(raw) > len(Magic) && string(raw[:len(Magic)]) == Magic {
		return raw[len(Magic):]
	}
	return raw
}

func ensureDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// atomicWrite 临时文件 + rename 落盘（与 audio 的 atomicWriteFile 同理：
// 中断只留下 .tmp，不留下半写的目标文件被“已存在”误判）。
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
