package save

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
)

// GlobalSave 是原引擎的"全局存档"（save/common.dat）。
//
// 它和我们的槽位存档（KIZUNA-SAVE1）不是一回事：槽位存档记录"某一刻的脚本状态"，
// 全局存档记录"跨周目累积的系统数据"——哪些对白读过、哪些 CG / 场景解锁过、
// 音量与显示设置。原引擎把它单独放一个文件，换存档、开新周目都保留；
// 鉴赏（museum）界面读的就是它。所以我们必须读写同一份文件，
// 玩家在原引擎里的进度带过来就能直接看到已解锁的 CG。
//
// 文件结构（全部小端，除摘要头外都是 int32 长度前缀的块）：
//
//	[32 字节] ASCII 摘要头（原引擎写入的 md5 文本，我们不解释内容，原样保留）
//	[前缀块]  设置 + 若干系统表（对白表之前的所有内容，原样保留）
//	[4+N]     对白已读表（逐句 1 字节，1=读过）
//	[间隔块]  对白表与 CG 表之间的若干 int32（原样保留）
//	[4+G]     CG 解锁表（逐张 1 字节）
//	[4+S]     场景解锁表（逐个 1 字节）
//	[尾部]    剩余字节（原样保留）
//
// 前缀块和间隔块的具体内容随作品/版本变化（音量、窗口、系统开关、以及 BGM 鉴赏
// 解锁位等），我们不需要理解它们，只保证原样搬运——这样即使某部作品的设置块
// 结构不同，写回时也不会破坏原引擎的数据。
//
// 定位方式：不写死偏移，而是用"对白数 / CG 数 / 场景数"这三个由游戏资源算出来的
// 期望值去锚定三张表。这三个数分别来自脚本消息表、cg.dat、scene.dat，
// 对不上就说明文件不是本作的（或已损坏），此时宁可不用也不要写坏。
type GlobalSave struct {
	// Header 是开头的 32 字节 ASCII 摘要，原样保留。
	Header []byte
	// Prefix 是摘要头与对白表之间的全部字节（设置 + 系统表）。
	Prefix []byte
	// MessageRead 是逐句的"已读"标志。
	MessageRead []byte
	// Gap 是对白表与 CG 表之间的字节（原引擎写的两个 int32，语义未知）。
	Gap []byte
	// CgUnlocked 是逐张 CG 的解锁标志。
	CgUnlocked []byte
	// SceneUnlocked 是逐个场景的解锁标志。
	SceneUnlocked []byte
	// Tail 是场景表之后的剩余字节，原样保留。
	Tail []byte
	// Ancient 表示这是 Tbxdl 世代（特別病棟）的 common.dat：
	// 32 字节零头 + int32 对白数 + 一个未知 int32 + 对白表 +
	// int32 场景数 + 场景表。这一代没有逐张 CG 解锁表，CG 鉴赏按全部可用处理。
	Ancient bool
	// AncientUnknown 是 ancient 头里对白数后面的那个 int32（实测为 1），
	// 原样保留，写回时不要改掉。
	AncientUnknown int32
}

const (
	// globalHeaderSize 是摘要头长度：原引擎写 32 字节 ASCII（md5 十六进制串）。
	globalHeaderSize = 32
	// globalMaxBlock 挡住损坏文件里天文数字的 int32，避免按几 GB 分配内存。
	globalMaxBlock = 1 << 26
	// globalMaxScan 是锚点搜索范围上限：三张表一定在文件前部，
	// 扫太远只会把"不是本作的文件"误判成有效。
	globalMaxScan = 1 << 20
)

var errGlobalSaveCorrupt = errors.New("bishop global save: corrupt")

// GlobalExpectations 描述"这部作品应该有多大的三张表"。
//
// 三个数字都来自游戏资源本身，不依赖存档：MessageCount 来自脚本消息表，
// CgCount 来自 cg.dat 的名字表，SceneCount 来自 scene.dat。
// 解析时用它们做校验，写新档时用它们定表长。
type GlobalExpectations struct {
	MessageCount int
	CgCount      int
	SceneCount   int
}

// GlobalSavePath 返回某部作品的全局存档路径。
//
// 固定 save/common.dat：位置写死是为了和原引擎共用同一份文件。
func GlobalSavePath(dataDir string) string {
	return filepath.Join(dataDir, "save", "common.dat")
}

// LoadGlobalSave 读取全局存档。
//
// 文件不存在返回 (nil, nil)：这是"全新安装、还没玩过"的正常状态，
// 调用方据此把全部 CG / 场景当作未解锁，而不是当成错误。
func LoadGlobalSave(path string, want GlobalExpectations) (*GlobalSave, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseGlobalSave(raw, want)
}

// ParseGlobalSave 解析全局存档字节。
//
// 全程边界检查：任何一处长度越界都返回错误而不是 panic——这份文件可能来自
// 别的版本、被截断、或被玩家手工改过。
func ParseGlobalSave(raw []byte, want GlobalExpectations) (*GlobalSave, error) {
	if len(raw) < globalHeaderSize+4 {
		return nil, errGlobalSaveCorrupt
	}
	if parsed, ok := parseAncientGlobalSave(raw, want); ok {
		return parsed, nil
	}
	messageAt := globalFindTable(raw, want.MessageCount, globalHeaderSize)
	if messageAt < 0 {
		return nil, errGlobalSaveCorrupt
	}
	message, messageEnd, ok := globalReadBlock(raw, messageAt)
	if !ok {
		return nil, errGlobalSaveCorrupt
	}
	cgAt := globalFindTable(raw, want.CgCount, messageEnd)
	if cgAt < 0 {
		return nil, errGlobalSaveCorrupt
	}
	cg, cgEnd, ok := globalReadBlock(raw, cgAt)
	if !ok {
		return nil, errGlobalSaveCorrupt
	}
	sceneAt := globalFindTable(raw, want.SceneCount, cgEnd)
	if sceneAt < 0 {
		return nil, errGlobalSaveCorrupt
	}
	scene, sceneEnd, ok := globalReadBlock(raw, sceneAt)
	if !ok {
		return nil, errGlobalSaveCorrupt
	}
	return &GlobalSave{
		Header:        append([]byte(nil), raw[:globalHeaderSize]...),
		Prefix:        append([]byte(nil), raw[globalHeaderSize:messageAt]...),
		MessageRead:   message,
		Gap:           append([]byte(nil), raw[messageEnd:cgAt]...),
		CgUnlocked:    cg,
		SceneUnlocked: scene,
		Tail:          append([]byte(nil), raw[sceneEnd:]...),
	}, nil
}

// globalFindTable 从 from 开始找"长度恰好等于 want 的块"。
//
// 用期望长度当锚点，是因为这几张表的大小由资源决定、几乎不可能撞车：
// 文件里同时出现两个 int32 == 36301 且后面正好跟得下 36301 字节的概率可以忽略。
// want <= 0 表示调用方没有这个期望值（例如缺 cg.dat），此时直接判定失败，
// 让上层退回"没有全局存档"的保守状态，而不是猜一个位置去覆盖玩家数据。
func globalFindTable(raw []byte, want int, from int) int {
	if want <= 0 || want > globalMaxBlock {
		return -1
	}
	limit := min(len(raw)-4, from+globalMaxScan)
	for pos := max(0, from); pos <= limit; pos++ {
		length, next, ok := globalReadInt32(raw, pos)
		if !ok || int(length) != want {
			continue
		}
		if next+want <= len(raw) {
			return pos
		}
	}
	return -1
}

// Encode 把全局存档写回字节，布局与解析严格对称。
func (g *GlobalSave) Encode() []byte {
	if g == nil {
		return nil
	}
	if g.Ancient {
		return g.encodeAncient()
	}
	size := globalHeaderSize + len(g.Prefix)
	size += 4 + len(g.MessageRead)
	size += len(g.Gap)
	size += 4 + len(g.CgUnlocked)
	size += 4 + len(g.SceneUnlocked)
	size += len(g.Tail)

	out := make([]byte, 0, size)
	header := g.Header
	if len(header) != globalHeaderSize {
		// 头长度固定 32；短了补 0、长了截断，保证写出来的文件仍能被原引擎读。
		header = make([]byte, globalHeaderSize)
		copy(header, g.Header)
	}
	out = append(out, header...)
	out = append(out, g.Prefix...)
	out = appendBlock(out, g.MessageRead)
	out = append(out, g.Gap...)
	out = appendBlock(out, g.CgUnlocked)
	out = appendBlock(out, g.SceneUnlocked)
	return append(out, g.Tail...)
}

// Save 原子写入全局存档。
//
// 先写同目录临时文件再 rename：鉴赏解锁是玩家玩几小时才攒出来的数据，
// 写到一半被杀进程会直接丢档；rename 在同一个文件系统上是原子的。
func (g *GlobalSave) Save(path string) error {
	encoded := g.Encode()
	if len(encoded) == 0 {
		return errGlobalSaveCorrupt
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, encoded, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// CgUnlockedAt 查询第 index 张 CG 是否解锁。
//
// 表比索引短时视为未解锁：通常意味着游戏更新加了新 CG，而玩家的 common.dat
// 还是旧版本——那种情况下新 CG 本来就该是锁着的。
func (g *GlobalSave) CgUnlockedAt(index int) bool {
	if g == nil {
		return false
	}
	if g.Ancient {
		// 这一代没有逐张 CG 解锁表，原引擎也不会锁 CG；鉴赏页应全部可用。
		return index >= 0
	}
	return globalFlagAt(g.CgUnlocked, index)
}

// SceneUnlockedAt 查询第 index 个场景是否解锁。
func (g *GlobalSave) SceneUnlockedAt(index int) bool {
	if g == nil {
		return false
	}
	return globalFlagAt(g.SceneUnlocked, index)
}

// MessageReadAt 查询第 index 句对白是否读过。
func (g *GlobalSave) MessageReadAt(index int) bool {
	if g == nil {
		return false
	}
	return globalFlagAt(g.MessageRead, index)
}

// UnlockCg 标记第 index 张 CG 已解锁，返回是否发生了状态变化（供调用方决定要不要落盘）。
func (g *GlobalSave) UnlockCg(index int) bool {
	if g == nil || g.Ancient || globalFlagAt(g.CgUnlocked, index) {
		return false
	}
	g.CgUnlocked = globalSetFlag(g.CgUnlocked, index)
	return true
}

// UnlockScene 标记第 index 个场景已解锁。
func (g *GlobalSave) UnlockScene(index int) bool {
	if g == nil || globalFlagAt(g.SceneUnlocked, index) {
		return false
	}
	g.SceneUnlocked = globalSetFlag(g.SceneUnlocked, index)
	return true
}

// MarkMessageRead 标记第 index 句对白已读。
func (g *GlobalSave) MarkMessageRead(index int) bool {
	if g == nil || globalFlagAt(g.MessageRead, index) {
		return false
	}
	g.MessageRead = globalSetFlag(g.MessageRead, index)
	return true
}

// NewGlobalSave 为"还没有 common.dat"的作品造一份空档。
//
// 摘要头留空（写 0）而不是编一个 md5：原引擎只在读档时用它做作品校验，
// 我们自己不校验，留空不会让原引擎误判成别的作品的数据。
// NewAncientGlobalSave 为 Tbxdl 世代造一份空的 common.dat。
// 布局与 parseAncientGlobalSave 严格对称，写回不会破坏原引擎文件。
func NewAncientGlobalSave(want GlobalExpectations) *GlobalSave {
	return &GlobalSave{
		Header:         make([]byte, globalHeaderSize),
		Ancient:        true,
		AncientUnknown: 1,
		MessageRead:    make([]byte, max(0, want.MessageCount)),
		SceneUnlocked:  make([]byte, max(0, want.SceneCount)),
	}
}

func NewGlobalSave(want GlobalExpectations) *GlobalSave {
	return &GlobalSave{
		Header:        make([]byte, globalHeaderSize),
		MessageRead:   make([]byte, max(0, want.MessageCount)),
		CgUnlocked:    make([]byte, max(0, want.CgCount)),
		SceneUnlocked: make([]byte, max(0, want.SceneCount)),
	}
}

// EnsureSize 把三张表补到期望长度。
//
// 游戏更新后 cg.dat 会变长，而玩家手里的 common.dat 还是旧的；补长（而不是重建）
// 能保住已有解锁状态，新增的条目保持 0（未解锁）。
func (g *GlobalSave) EnsureSize(want GlobalExpectations) {
	if g == nil {
		return
	}
	g.MessageRead = globalGrow(g.MessageRead, want.MessageCount)
	if !g.Ancient {
		g.CgUnlocked = globalGrow(g.CgUnlocked, want.CgCount)
	}
	g.SceneUnlocked = globalGrow(g.SceneUnlocked, want.SceneCount)
}

func globalGrow(table []byte, size int) []byte {
	if size <= len(table) {
		return table
	}
	grown := make([]byte, size)
	copy(grown, table)
	return grown
}

func globalFlagAt(table []byte, index int) bool {
	if index < 0 || index >= len(table) {
		return false
	}
	return table[index] != 0
}

// globalSetFlag 置位并把表扩到 index+1；表本身用 0 填充（0 = 未解锁）。
func globalSetFlag(table []byte, index int) []byte {
	if index < 0 {
		return table
	}
	if index >= len(table) {
		grown := make([]byte, index+1)
		copy(grown, table)
		table = grown
	}
	table[index] = 1
	return table
}

// globalReadBlock 读"int32 长度 + 内容"。长度非法或越界时返回 ok=false。
func globalReadBlock(raw []byte, pos int) ([]byte, int, bool) {
	length, next, ok := globalReadInt32(raw, pos)
	if !ok || length < 0 || length > globalMaxBlock {
		return nil, pos, false
	}
	end := next + int(length)
	if end > len(raw) || end < next {
		return nil, pos, false
	}
	return raw[next:end], end, true
}

func globalReadInt32(raw []byte, pos int) (int32, int, bool) {
	if pos < 0 || pos+4 > len(raw) {
		return 0, pos, false
	}
	return int32(binary.LittleEndian.Uint32(raw[pos : pos+4])), pos + 4, true
}

// parseAncientGlobalSave 解析 Tbxdl 世代的 common.dat。
//
// 实测布局（檔案 27199 字节）：
//
//	[32 字节 0] [int32 对白数] [int32 1] [对白表] [int32 场景数] [场景表]
//
// 没有 CG 表。用"头部全 0 + 对白数/场景数与资源期望值一致"识别，
// 避免把现代作品的 md5 头误判成这种布局。
func parseAncientGlobalSave(raw []byte, want GlobalExpectations) (*GlobalSave, bool) {
	if len(raw) < globalHeaderSize+12 {
		return nil, false
	}
	for index := 0; index < globalHeaderSize; index++ {
		if raw[index] != 0 {
			return nil, false
		}
	}
	messageCount := int(int32(binary.LittleEndian.Uint32(raw[32:36])))
	if messageCount <= 0 || messageCount > globalMaxBlock {
		return nil, false
	}
	messageEnd := 40 + messageCount
	if messageEnd+4 > len(raw) {
		return nil, false
	}
	sceneCount := int(int32(binary.LittleEndian.Uint32(raw[messageEnd : messageEnd+4])))
	if sceneCount < 0 || sceneCount > globalMaxBlock {
		return nil, false
	}
	sceneStart := messageEnd + 4
	if sceneStart+sceneCount > len(raw) {
		return nil, false
	}
	if want.MessageCount > 0 && messageCount != want.MessageCount {
		return nil, false
	}
	if want.SceneCount > 0 && sceneCount != want.SceneCount {
		return nil, false
	}
	return &GlobalSave{
		Header:         append([]byte(nil), raw[:globalHeaderSize]...),
		Ancient:        true,
		AncientUnknown: int32(binary.LittleEndian.Uint32(raw[36:40])),
		MessageRead:    append([]byte(nil), raw[40:messageEnd]...),
		SceneUnlocked:  append([]byte(nil), raw[sceneStart:sceneStart+sceneCount]...),
		Tail:           append([]byte(nil), raw[sceneStart+sceneCount:]...),
	}, true
}

func (g *GlobalSave) encodeAncient() []byte {
	size := globalHeaderSize + 8 + len(g.MessageRead) + 4 + len(g.SceneUnlocked) + len(g.Tail)
	out := make([]byte, 0, size)
	header := g.Header
	if len(header) != globalHeaderSize {
		header = make([]byte, globalHeaderSize)
		copy(header, g.Header)
	}
	out = append(out, header...)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(len(g.MessageRead)))
	out = append(out, buf[:]...)
	unknown := g.AncientUnknown
	if unknown == 0 {
		unknown = 1
	}
	binary.LittleEndian.PutUint32(buf[:], uint32(unknown))
	out = append(out, buf[:]...)
	out = append(out, g.MessageRead...)
	binary.LittleEndian.PutUint32(buf[:], uint32(len(g.SceneUnlocked)))
	out = append(out, buf[:]...)
	out = append(out, g.SceneUnlocked...)
	return append(out, g.Tail...)
}

func appendBlock(dst, block []byte) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(len(block)))
	return append(append(dst, buf[:]...), block...)
}
