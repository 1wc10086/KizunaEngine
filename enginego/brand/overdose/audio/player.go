package audio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kizuna/enginego/brand/overdose/arc"
	"kizuna/enginego/core"
)

// 宿主侧音频事件的 Kind 编号与两仓库共用协议一致（见 bridge/protocol_consts.dart
// KzAudioKind 与既有品牌 bishop/emon 的使用方式）：
//
//	0 bgm / 1 se / 2 voice / 3 stopBgm / 4 stopVoice / 5 stopSe
//	6 voiceSequence / 7 volume / 8 fadeOut / 9 stopAll
//
// 这里把它们提升为具名常量，避免后续会话层写魔数时写错（如把停语音与
// 停音效互换）而无处可查。本包内部也只产出这些编号。
const (
	KindBGM           = 0
	KindSE            = 1
	KindVoice         = 2
	KindStopBGM       = 3
	KindStopVoice     = 4
	KindStopSE        = 5
	KindVoiceSequence = 6
	KindVolume        = 7
	KindFadeOut       = 8
	KindStopAll       = 9
)

// NoTarget 表示"不绑定具体通道"的哨兵值：与 bishop/emon 的 noTarget 一致，
// 即宿主把 Channel/Group 当作全局目标（BGM 单槽、SE 池、语音主通道共用）。
const NoTarget = uint32(0xFFFFFFFF)

// Category 是逻辑分类：决定去哪个 .arc 里找、以及事件 Kind 用哪一个。
// 为什么分类要独立成类型：ASB 指令按 opcode 天然分三类（0x58 BGM、0x5c SE、
// 0x64 voice），但三类池子上限与 channel 域不同；分类必须在找文件阶段
// 就错位，否则 voice 的 miy_0792 会去 music.arc 查，查不到就误报"文件缺失"。
type Category uint8

const (
	CategoryVoice Category = iota
	CategorySound
	CategoryMusic
)

// Channel 是归一后的通道：idx 是脚本给的池内序号（BGM<4 / SE<8 / voice<32），
// kind 区分池子。会话层直接把这个结构喂给 Player 就能得到宿主事件
// （Play/Stop/Volume/FadeOut 四个入口都只收 Channel，省去 switch）。
type Channel struct {
	Kind Category
	Idx  int32
}

// PlayRequest 是"播一个文件"的归一请求，等价于 ASB 0x58/0x5c/0x64 的
// （idx, file, loop, vol, fade）五元组。Name 不带路径，只写归档内条目名
// （含 .ogg）；是否物化成磁盘文件由 Player 负责，调用方无需关心。
type PlayRequest struct {
	Channel Channel
	Name    string
	Loop    bool
	Volume  int32
	Fade    int32
}

// volumes 钳制到 0..100（原引擎默认 0x64=100 满格）。为什么在这里钳：
// 脚本里出现过 0x14 之类的中间值，宿主侧是 0..1 的 float；不在此地
// 收敛的话，每个通道的换算都要各自写一遍钳制，漏掉一处就是超 1 的增益。
func clampVolume(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func volumeFloat(v int32) float64 {
	return float64(clampVolume(v)) / 100.0
}

// archiveKinds 是归一化表：把 PlayRequest 变成事件 Kind。voiceSequence
// 这里不产出——ASB 没有连续播放多条语音的指令，产出它就是违背契约。
func (c Category) playKind() uint32 {
	switch c {
	case CategoryVoice:
		return KindVoice
	case CategorySound:
		return KindSE
	default:
		return KindBGM
	}
}

func (c Category) stopKind() uint32 {
	switch c {
	case CategoryVoice:
		return KindStopVoice
	case CategorySound:
		return KindStopSE
	default:
		return KindStopBGM
	}
}

// Player 是音频材料的会话级状态机：掌管"每个通道当前在播什么"与事件队列。
// 为什么需要它而不是无状态函数：
//   - fade/stop 的语义依赖上一条 Play（比如 StopSE 必须知道当前文件）；
//   - 同名同通道重复 Play 要幂等：宿主那边已有"同文件只改音量"的优化，
//     如果 Player 每次都发新 Play 事件，宿主会重复启动解码器。
//   - Tick 驱动 fade 计时：照 bishop/emon 的习惯，倒计时不靠墙钟 goroutine，
//     而是会话每帧 Tick(dtMs) 推进，方便暂停、读档后恢复以及测试。
type Player struct {
	set    *arc.Set
	root   string
	events []core.AudioEvent
	// slots 是 idx->当前播放。各通道 idx 的意义：BGM 池 0..3、SE 0..7、
	// voice 0..31（见 opcodes_spec §6）。Key 含 Category，免去不同池 idx 重合。
	slots map[channelKey]slotState
	// nowMs 是 Tick 累加的时间轴（毫秒，单调）。fade 的开始/结束都记在此处，
	// 这样 Tick 不被墙钟打断后还能由存档的剩余时间恢复（见 SaveState）。
	nowMs int64
}

type channelKey struct {
	cat Category
	idx int32
}

type slotState struct {
	name       string // 条目名（含 .ogg），非空 = 在播
	loop       bool
	vol        int32 // 脚本音量 0..100
	fadeMs     int32 // Play 自带的 fade 毫秒（仅 fadeIn 算）；0 = 无淡入
	fadeEnd    int64 // nowMs+fadeMs，fadeIn 完成时刻；fadeMs==0 时无意义
	fadingOut  bool  // 正在 FadeOut：fadeOutMs 到期后发 stop
	fadeOutEnd int64
	// pendingStop 是 Stop 的延迟标记：宿主收到 stop 事件后即停，引擎这里
	// 直接清槽位；保留该字段是为了 fadeOut→stop 的两段式（先 fadeOut 再补 stop）。
	// 这里不引入多余分支：FadeOut 到期时清槽位并置 pendingStop 由 Tick 发出。
}

// New 新建 Player。audioRoot 应为 <cacheDir>/<gamehash>/audio（会话层传入，
// 本包不算 gamehash，避免与 core.GameCacheDir 耦合）。
// gameHash 与 archive 名拼接路径见 Materialize 的说明。
func New(set *arc.Set, audioRoot string) *Player {
	return &Player{
		set:   set,
		root:  strings.TrimSpace(audioRoot),
		slots: make(map[channelKey]slotState),
	}
}

// SetRoot 覆盖音频根目录（会话层在 Open 后注入 cache 配置时用）。
func (p *Player) SetRoot(root string) { p.root = strings.TrimSpace(root) }

// Root 返回当前音频根目录。
func (p *Player) Root() string { return p.root }

// ---------------- 物化 ----------------

// Materialize 把归档条目解回成磁盘上的 .ogg 文件，返回绝对路径。
//   - archive 取 "voice"/"sound"/"music"（大小写不敏感），决定查哪个 .arc；
//   - name 是条目名，可带或不带 .ogg（带 .ogg 优先，与归档原始名一致）；
//   - 若目标文件已存在且大小不为 0，直接返回（复用，不重复写盘）——
//     语音在剧情里连播上千条，若每次都重写，磁盘 I/O 与闪存磨损都不可接受。
//
// 为什么物化而不是直接喂字节：Flutter/Dart 侧播放走 flutter_soloud，
// 它只认文件路径（且现有 bishop/emon 会话都用这条链路）。解包一步到位
// 也是为了让收集到的 .ogg 不必整段进引擎内存。
func (p *Player) Materialize(archive, name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", errors.New("audio: 条目名为空")
	}
	if p.set == nil {
		return "", errors.New("audio: 存档集合为空")
	}
	// archive 名归一：调用方传 "voice"/"Voice"/"music.arc" 都走同一路径，
	// 免得后续传错时靠大小写去翻案。
	archKey := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.ToLower(archive), ".arc")))
	var target *arc.Archive
	switch archKey {
	case "voice":
		target = p.set.Voice
	case "sound", "se":
		target = p.set.Sound
	case "music", "bgm":
		target = p.set.Music
	case "":
		// 为空时按 voice→sound→music 顺序在集合里找（set.Entry 的次序里
		// 他们恰好是最后三类），不给调用方加负担。
		return p.materializeFromSet(clean)
	default:
		return "", fmt.Errorf("audio: 未知归档类 %q（合法 voice/sound/music）", archive)
	}
	if target == nil {
		return "", fmt.Errorf("audio: %s.arc 未打开", archKey)
	}
	// 条目名在归档里含 .ogg，且 arc.find 对无后缀也有兜底；为保证路径
	// 稳定，这里显式带上 .ogg 再查（对方给的是无后缀笔误也不会误判）。
	lookup := clean
	if !strings.HasSuffix(strings.ToLower(lookup), ".ogg") {
		lookup += ".ogg"
	}
	data, err := target.Entry(lookup)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("audio: 条目 %q 为空", lookup)
	}
	return p.writeOgg(archKey, archBase(lookup), data)
}

// materializeFromSet 在未指定归档类时依次尝试三座音频归档。
// 为什么保留这个兜底：ASB 脚本里出现的名字没有归档类信息（如 se_006 与
// yui_0184——看起来像不同类别），会话层只给 name 时也得能找到。
func (p *Player) materializeFromSet(name string) (string, error) {
	lookup := name
	if !strings.HasSuffix(strings.ToLower(lookup), ".ogg") {
		lookup += ".ogg"
	}
	for _, a := range []*arc.Archive{p.set.Voice, p.set.Sound, p.set.Music} {
		if a == nil {
			continue
		}
		data, err := a.Entry(lookup)
		if err != nil || len(data) == 0 {
			continue
		}
		return p.writeOgg(archiveLabel(a), archBase(lookup), data)
	}
	return "", fmt.Errorf("audio: 集合里没有音频条目 %q", name)
}

func archiveLabel(a *arc.Archive) string {
	base := strings.ToLower(filepath.Base(a.Path))
	return strings.TrimSuffix(base, ".arc")
}

// writeOgg 落盘：路径为 <root>/<archive>/<name>.ogg，name 已经过清除。
// 为什么不像 emon 那样把内容哈希拼进文件名：原引擎的语音/音效文件都是
// 按固定名播的（miy_0792.ogg），物化后的文件名若带哈希，读档后探针
// 按路径对比当前文件时需要再反查一次，而"稳定路径"能让“已物化”判断
// 仅靠 Stat 即可完成；代价是一旦归档内容变了路径不变（同名覆盖风险），
// 但 Over-Dose 的归档是发行物、不会热更。
func (p *Player) writeOgg(archive, base string, data []byte) (string, error) {
	safe := SanitizeName(base)
	if safe == "" {
		safe = "unnamed.ogg"
	}
	dir := filepath.Join(p.root, archive)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("audio: 建目录 %s: %w", dir, err)
	}
	target := filepath.Join(dir, safe)
	if info, err := os.Stat(target); err == nil && !info.IsDir() && info.Size() > 0 {
		// 已存在：直接复用，不重写。iov 校验（大小>0）避免零字节的半成品
		// 被误认为成功而永久顶住正确内容。
		return target, nil
	}
	if err := atomicWriteFile(target, data); err != nil {
		return "", err
	}
	return target, nil
}

// SanitizeName 清除文件名字符：归档里的名字本应干净（字母/数字/_/.），
// 但 CP932 解码或手工存档注入的恶意名可能含 "../" 或 "\"。这里把目录
// 分隔符与非法字符一律换成 "_"，并去掉首位圆点，避免路径穿越写出
// <root>/<archive> 之外。返回空串表示原名全是非法字符。
func SanitizeName(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return ""
	}
	// 取最后一个分隔符之后的部分：调用方若传 "foo\bar.ogg" 也按一个文件名处理。
	if idx := strings.LastIndexAny(clean, "/\\"); idx >= 0 {
		clean = clean[idx+1:]
	}
	clean = strings.TrimLeft(clean, ".")
	var b strings.Builder
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := b.String()
	// 隐藏文件首位保护：清除后首位不能是 '.'；也截断超长，避免 ENAMETOOLONG。
	out = strings.TrimLeft(out, ".")
	if len(out) > 120 {
		// 保留扩展名：超长截断时只砍主名部分，扩展名不能丢。
		ext := filepath.Ext(out)
		if ext != "" {
			out = out[:120-len(ext)] + ext
		} else {
			out = out[:120]
		}
	}
	return out
}

func archBase(name string) string {
	base := name
	if idx := strings.LastIndexAny(base, "/\\"); idx >= 0 {
		base = base[idx+1:]
	}
	return base
}

// atomicWriteFile 写临时文件再 rename：为什么必须原子——会话可能在
// 物化中途被杀（Android 进程回收），半写的文件留在目标路径会被
// "已存在复用"误判；rename 是原子操作，半写只留在 .tmp 上，下次能重写。
func atomicWriteFile(path string, data []byte) error {
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

// ---------------- 播放 API ----------------

// Play 挂一条播放请求：物化文件后把 core.AudioEvent（Kind=0/1/2）入队。
// 同通道同文件重复 Play 时仍发事件（宿主侧有“同文件只改音量”优化），
// 但 fadeIn 的倒计时从本次 Play 重新起算——脚本意图是“重新淡入”，
// 不沿用上一次的剩余时间。
func (p *Player) Play(req PlayRequest) {
	name := strings.TrimSpace(req.Name)
	key := channelKey{cat: req.Channel.Kind, idx: req.Channel.Idx}
	if req.Channel.Idx < 0 {
		key.idx = 0
	}
	if name == "" {
		// 空名 = 脚本的停播暗号（文件为空时退为停止该通道）：与其它品牌
		// “改名为空即停”的习惯保持一致，而不是无声地忽略。
		p.Stop(req.Channel)
		return
	}
	path := name
	if p.root != "" {
		if resolved, err := p.Materialize(categoryArchive(req.Channel.Kind), name); err == nil {
			path = resolved
		} else {
			core.Logf("overdose-audio: 物化 %q 失败，透传名: %v", name, err)
		}
	}
	vol := clampVolume(req.Volume)
	loop := req.Loop
	fadeMs := req.Fade
	if fadeMs < 0 {
		fadeMs = 0
	}
	st := slotState{name: name, loop: loop, vol: vol, fadeMs: fadeMs}
	if fadeMs > 0 {
		st.fadeEnd = p.nowMs + int64(fadeMs)
	}
	p.slots[key] = st
	p.events = append(p.events, core.AudioEvent{
		Kind:    req.Channel.Kind.playKind(),
		Path:    path,
		Volume:  volumeFloat(vol),
		Loop:    loop,
		Channel: uint32(req.Channel.Idx),
		Group:   NoTarget,
	})
}

// categoryArchive 把 Category 翻成归档类名（给 Materialize 用）。
func categoryArchive(c Category) string {
	switch c {
	case CategoryVoice:
		return "voice"
	case CategorySound:
		return "sound"
	default:
		return "music"
	}
}

// Stop 停止指定通道。无条件发事件（宿主 stop 是幂等的），槽位清掉；
// 通道本来就没在播时也发——事件队列只代表“宿主侧要执行什么”，
// 而不是“引擎侧什么变化了”，省去调用方先查状态的来回。
func (p *Player) Stop(ch Channel) {
	key := channelKey{cat: ch.Kind, idx: ch.Idx}
	if ch.Idx < 0 {
		key.idx = 0
	}
	delete(p.slots, key)
	p.events = append(p.events, core.AudioEvent{
		Kind:    ch.Kind.stopKind(),
		Channel: uint32(ch.Idx),
		Group:   NoTarget,
	})
}

// SetVolume 改指定通道的增益（Kind=7）。先在槽位里记下，Tick 时不干预；
// 若通道没在播也照常发事件（宿主记增益，下一次 Play 时生效）。
func (p *Player) SetVolume(ch Channel, vol int32) {
	key := channelKey{cat: ch.Kind, idx: ch.Idx}
	if ch.Idx < 0 {
		key.idx = 0
	}
	v := clampVolume(vol)
	if st, ok := p.slots[key]; ok {
		st.vol = v
		p.slots[key] = st
	}
	p.events = append(p.events, core.AudioEvent{
		Kind:    KindVolume,
		Volume:  volumeFloat(v),
		Channel: uint32(ch.Idx),
		Group:   NoTarget,
	})
}

// FadeOut 淡出指定通道：Kind=8（Frames=毫秒数，宿主按 60fps 把 Frames 转
// 毫秒，见 game_audio.dart _fadeEvent）随后由 Tick 在倒计时到期时补发 stop
// 事件。宿主 fade 完成后会自行停止，补 stop 是保险——若宿主不关，声音
// 会在静音后仍占着槽位。
// 为什么用帧数而非毫秒：宿主一个参数要在两端都接（core.AudioEvent.Frames），
// 和 emon 一致；这里把毫秒值直接填 Frames。
func (p *Player) FadeOut(ch Channel, fadeMs int32) {
	key := channelKey{cat: ch.Kind, idx: ch.Idx}
	if ch.Idx < 0 {
		key.idx = 0
	}
	if fadeMs < 0 {
		fadeMs = 0
	}
	st, ok := p.slots[key]
	if !ok {
		st = slotState{}
	}
	st.fadingOut = true
	st.fadeOutEnd = p.nowMs + int64(fadeMs)
	p.slots[key] = st
	p.events = append(p.events, core.AudioEvent{
		Kind:    KindFadeOut,
		Frames:  uint32(fadeMs),
		Channel: uint32(ch.Idx),
		Group:   NoTarget,
	})
}

// StopAll 停全部通道（Kind=9）。facade 语义：宿主一次清 BGM/voice/effects；
// 槽位全清，fadeOut 的待补 stop 也不再发（已含在 stopAll 里）。
func (p *Player) StopAll() {
	for k := range p.slots {
		delete(p.slots, k)
	}
	p.events = append(p.events, core.AudioEvent{
		Kind:    KindStopAll,
		Channel: NoTarget,
		Group:   NoTarget,
	})
}

// Tick 推进时间轴并结算到期的 fadeOut（补发 stop 事件）。
// dtMs 为本帧增量（毫秒）。调用方负责调频，本函数不记墙钟；这样
// 单元测试可以 Tick(1000) 一步跨过任意长 fade。
func (p *Player) Tick(dtMs int64) {
	if dtMs < 0 {
		dtMs = 0
	}
	p.nowMs += dtMs
	for key, st := range p.slots {
		if !st.fadingOut {
			continue
		}
		if p.nowMs < st.fadeOutEnd {
			continue
		}
		// fadeOut 到期：清槽并补 stop。若同通道已 Stop 又 Play 新文件，
		// 槽位里归属已是新文件，此处若按旧 key 无条件发 stopping，
		// 会把新文件误杀；因此只在槽位仍是 fadingOut 状态且 name 与
		// 发起 FadeOut 时相同才补发。当前实现把 fadeOut 视为不起
		// 槽位替换，Play 会整条覆盖 slotState（fadingOut=false），
		// 所以键址上仍是 fadingOut 即意味着该文件仍是 FadeOut 目标。
		delete(p.slots, key)
		p.events = append(p.events, core.AudioEvent{
			Kind:    key.cat.stopKind(),
			Channel: uint32(key.idx),
			Group:   NoTarget,
		})
	}
}

// Events 取出并清空事件队列（会话 Audio() 直接转发）。
func (p *Player) Events() []core.AudioEvent {
	out := p.events
	p.events = nil
	return out
}

// ---------------- 存档/快照 ----------------

// SaveState 是可序列化的通道快照（给 save 包用）。
type SaveState struct {
	BgmFile  string         `json:"bgmFile,omitempty"`
	BgmLoop  bool           `json:"bgmLoop,omitempty"`
	BgmVol   int32          `json:"bgmVol,omitempty"`
	BgmIdx   int32          `json:"bgmIdx,omitempty"`
	Channels []ChannelState `json:"channels,omitempty"`
}

// ChannelState 是单个通道的快照：包含 loop/vol，fade 剩余时间能由
// Tick 的时间轴恢复（存 State 的 FadeLeftMs 而非绝对时刻，免得
// nowMs 在不同会话长度下对不齐）。
type ChannelState struct {
	Category Category `json:"category"`
	Idx      int32    `json:"idx"`
	Name     string   `json:"name"`
	Loop     bool     `json:"loop"`
	Vol      int32    `json:"vol"`
	FadeMs   int32    `json:"fadeMs,omitempty"`
}

// Snapshot 导出当前各通道状态（BGM 单列，方便会话直接对 core.State.Bgm）。
func (p *Player) Snapshot() SaveState {
	var out SaveState
	for key, st := range p.slots {
		if st.name == "" {
			continue
		}
		cs := ChannelState{Category: key.cat, Idx: key.idx, Name: st.name, Loop: st.loop, Vol: st.vol, FadeMs: st.fadeMs}
		if key.cat == CategoryMusic && out.BgmFile == "" && key.idx == 0 {
			out.BgmFile = st.name
			out.BgmLoop = st.loop
			out.BgmVol = st.vol
			out.BgmIdx = key.idx
		}
		out.Channels = append(out.Channels, cs)
	}
	return out
}

// Restore 从快照恢复槽位（不重发 Play 事件——读档后由会话决定是否补播；
// 恢复的 fadeIn 一律重新起算，与 Play 语义一致）。
func (p *Player) Restore(s SaveState) {
	p.slots = make(map[channelKey]slotState)
	for _, cs := range s.Channels {
		if cs.Name == "" {
			continue
		}
		st := slotState{name: cs.Name, loop: cs.Loop, vol: clampVolume(cs.Vol), fadeMs: cs.FadeMs}
		if cs.FadeMs > 0 {
			st.fadeEnd = p.nowMs + int64(cs.FadeMs)
		}
		p.slots[channelKey{cat: cs.Category, idx: cs.Idx}] = st
	}
}

// NowMs 返回当前时间轴（测试与日志用）。
func (p *Player) NowMs() int64 { return p.nowMs }
