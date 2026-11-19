package script

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// exec 执行一条记录。约定：调用前 v.ip 指向记录起点；返回前必须把 v.ip
// 推到下一条（或由跳转改写）。这样 Run 循环可以统一处理 yield。
func (v *VM) exec(s *Script, rec Record) (Step, error) {
	r := &exprReader{vm: v, pay: rec.Payload}
	next := rec.Off + rec.Len
	step := StepContinue

	switch rec.Op {
	case 0x0000, 0x0001:
		// 原版是错误/警告记录；HUD 里出现的 0x0000 多半是填充。
		// 不阻塞游戏，只记日志。
	case 0x0002:
		// 创建/配置层：idx, flag, ?, ?, ?, width, height, mode
		idx := v.eval(r)
		args := make([]int32, 0, 8)
		for r.more() && len(args) < 8 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil {
			v.stage.CreateLayer(idx, args)
		}
	case 0x0003:
		if v.stage != nil {
			v.stage.FreeLayer(v.eval(r))
		}
	case 0x0006:
		// 0x402e70：在层 E3 上以颜色 E2 填充 (E4,E5,E6,E7)。
		// arg1=E1 是混合标志，arg3 固定 0（调色板模式）。
		args := make([]int32, 0, 7)
		for r.more() && len(args) < 7 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil && len(args) >= 7 {
			v.stage.FillRect(args[2], args[3], args[4], args[5], args[6], args[1])
		}
	case 0x0007:
		// 0x404300：srcE3(sx E4, sy E5, w E6, h E7) → dstE8(E9,E10)。
		args := make([]int32, 0, 10)
		for r.more() && len(args) < 10 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil && len(args) >= 10 {
			v.stage.CopyRect(args[2], args[7], args[3], args[4], args[5], args[6], args[8], args[9])
		}
	case 0x0008:
		ms := v.eval(r)
		if ms > 0 {
			v.WaitMs = uint32(ms)
			step = StepWait
		}
	case 0x0009:
		v.active = v.eval(r)
		if v.stage != nil {
			v.stage.SetActive(v.active)
		}
		raw := r.cstr()
		v.setMessage(raw)
		// 原版 op 0x9 在逐字显示时会被反复调用；buffer 127 里是完整文本，
		// $p127.r<slot> 输出的是“已显示到的前缀”。只有当前输出等于完整
		// 文本（或不再有更长的前缀）时才把这一句交给宿主，否则继续跑。
		full := v.bufferString(127)
		t := v.Text
		if full != "" && t != "" && strings.HasPrefix(full, t) && len([]rune(t)) < len([]rune(full)) {
			step = StepContinue
		} else if t != "" {
			step = StepAwaitInput
		}
	case 0x000a:
		idx := v.eval(r)
		x := v.eval(r)
		y := v.eval(r)
		w := v.eval(r)
		h := v.eval(r)
		if v.stage != nil {
			v.stage.LayerRect(idx, x, y, w, h)
		}
	case 0x000b:
		idx := v.eval(r)
		_ = v.eval(r)
		if v.stage != nil {
			v.stage.LayerRect(idx, 0, 0, -1, -1)
		}
	case 0x000c:
		idx := v.eval(r)
		_ = v.eval(r)
		if v.stage != nil {
			v.stage.LayerRect(idx, 0, 0, -1, -1)
		}
	case 0x000d:
		idx := v.eval(r)
		txt := v.expand(r.cstr())
		if v.stage != nil {
			v.stage.SetActive(idx)
		}
		v.Speaker = ""
		v.Text = txt
	case 0x000e:
		// 0x41b640 复位窗口的裁剪矩形（把 +0x38..+0x54 还原成层边界），
		// 既不是释放层，也不清像素。我们不做裁剪建模，因此只消费参数。
		// 以前当成 FreeLayer 会删掉 0 号桌面层；改成 ClearLayer 又会把
		// 每帧画好的背景抹黑——两者都会让画面全黑。
		_ = v.eval(r)
	case 0x0011:
		v.active = v.eval(r)
		if v.stage != nil {
			v.stage.SetActive(v.active)
		}
		step = v.yieldForMenu()
	case 0x0012:
		v.flag475c04 = true
	case 0x0013:
		v.flag475c04 = false
	case 0x0014:
		// 刷新/重绘
		if v.stage != nil {
			v.stage.Redraw()
		}
	case 0x0015:
		idx := v.eval(r)
		txt := v.expand(r.cstr())
		v.menu = append(v.menu, Choice{Value: idx, Text: txt})
	case 0x0016:
		// 创建窗口：第 1 个参数是**接收窗口句柄的槽**（原版 0x42e920），
		// 后面是父层/尺寸/样式等。它不是“建一个层”——以前把槽号/句柄值
		// 当层号调用 CreateLayer，脚本用 0x16 建立 750x136 的对话框时会把
		// 0 号屏幕层重建、整帧内容被清空。
		//
		// 本作的对话框/存读档界面由引擎自绘，不依赖 Win32 子窗口，因此这里
		// 只把句柄写成 0，保证脚本后续判断“创建成功”即可。
		slot := v.evalSlot(r)
		for i := 0; i < 7 && r.more(); i++ {
			_ = v.eval(r)
		}
		// 返回一个不会与真实层号冲突的假句柄：脚本随后会对它调用
		// FreeLayer/移动窗口，句柄越界时这些操作自然变成 no-op，不会误删
		// 0 号桌面层。真正的对话框由引擎自绘 UI 承担。
		v.winSerial++
		v.setVar(slot, 0x4000+v.winSerial)
	case 0x0017:
		name := v.expand(r.cstr())
		scr, err := v.loadScript(name)
		if err != nil {
			return StepFaulted, err
		}
		v.ctxs = v.ctxs[:0]
		v.ctxs = append(v.ctxs, ctx{name: scr.Name})
		v.cur = 0
		v.ip = 0
		v.finished = false
		return StepContinue, nil
	case 0x0019:
		a := v.eval(r)
		b := v.eval(r)
		v.setVar(b, v.varAt(a))
	case 0x001a:
		// 22 个表达式 + 0。原版是复杂界面/选项设置；保留菜单文本由 0x15 提供。
		for r.more() {
			_ = v.eval(r)
		}
	case 0x001b:
		_ = v.eval(r)
	case 0x001c:
		// flush
	case 0x001d:
		name := v.expand(r.cstr())
		scr, err := v.loadScript(name)
		if err != nil {
			return StepFaulted, err
		}
		if v.cur+1 >= ctxMax {
			return StepFaulted, fmt.Errorf("blackpackage: 调用深度超限")
		}
		v.ctxs[v.cur].ip = next
		v.ctxs = append(v.ctxs, ctx{name: scr.Name})
		v.cur++
		v.ip = 0
		return StepContinue, nil
	case 0x001e:
		if v.cur > 0 {
			v.ctxs = v.ctxs[:v.cur]
			v.cur--
			v.ip = v.ctxs[v.cur].ip
			return StepContinue, nil
		}
		v.finished = true
		return StepFinished, nil
	case 0x001f:
		// 工具栏菜单项（原版 0x42fda0 调 AppendMenuA）：4 个表达式
		// （菜单句柄、flags、命令 id、参数）+ 字符串。
		//
		// 注意这不是对白选择肢：本作在启动时用它一次性注册“ログ/文字消/
		// オート/セーブ/ロード…”整条菜单栏。以前误当成选择肢，YIELD 时
		// 会把 30 多项工具栏当成剧情选项弹给玩家。真正的剧情选项走 0x15。
		for i := 0; i < 4 && r.more(); i++ {
			_ = v.eval(r)
		}
		_ = v.expand(r.cstr())

	// ---- 系统/界面 ----
	case 0x0020:
		_ = v.eval(r)
		_ = v.eval(r)
	case 0x0039:
		// RegOpenKeyEx 的“键是否存在”探针：vars[slot] = 1/0。
		// 本作 INIT 用 `HKLM\...\App Paths\X0WIN.EXE\CDROM` 判断是不是
		// 从安装版启动，并据此用 CDROM 里的文件大小校验 X0WIN.DAT。跨平台
		// 没有注册表，正确答案是“不存在”（0）；如果像以前那样什么都不写，
		// slot 里残留的旧值会让 INIT 误判为安装版并弹“文件已损坏”。
		slot := v.evalSlot(r)
		_ = v.expand(r.cstr())
		v.setVar(slot, 0)
	case 0x0038:
		// RegQueryValueEx：key 不存在时原版不写目标缓冲。这里显式写空串，
		// 避免调用方拿到上一次残留的路径。
		slot := v.evalSlot(r)
		_ = v.expand(r.cstr())
		if slot >= 0 && int(slot)*4 < len(v.vars) {
			v.vars[slot*4] = 0
		}
	case 0x003a:
		// RegDeleteKey：没有注册表，视为成功。
	case 0x0021, 0x0023, 0x0024, 0x0025, 0x0028, 0x002a, 0x002b, 0x002c,
		0x0035, 0x0037,
		0x003c, 0x003d, 0x0040, 0x0042, 0x0043, 0x0047, 0x0048,
		0x0049, 0x004a, 0x004c:
		// 原生菜单/文件/注册表/对话框。对跨平台引擎是 no-op；需要
		// 宿主动作的 op（如 0x33 提示框）单独处理。
		for r.more() {
			_ = v.eval(r)
		}
	case 0x002d:
		// vars[slot] = 原版 FileCheck(path)。原版会先看当前目录，再查
		// 路径表；跨平台下我们同时看用户存档目录和游戏目录。
		slot := v.evalSlot(r)
		name := v.expand(r.cstr())
		v.setVar(slot, boolInt(v.fileExists(name)))
	case 0x002e:
		// 写变量块：start, count(dwords), path, [结果槽]。
		// 原版 0x417020 以 CREATE_ALWAYS 写入 count*4 字节，末尾不给
		// 结果槽时 0x491dc0=1；给了结果槽则在成功时写 1。
		start := v.evalSlot(r)
		count := v.eval(r)
		name := v.expand(r.cstr())
		hasTail := r.more()
		tail := int32(0)
		if hasTail {
			tail = v.evalSlot(r)
		}
		if count < 0 {
			count = 0
		}
		end := start + count
		if start < 0 || end < start || int(end)*4 > len(v.vars) {
			if hasTail {
				v.setVar(tail, 1)
			}
			return step, nil
		}
		// 原版语义：成功写 0，失败写 1（0x430501 / 0x4304e3）。
		ok := v.writeVarBlock(name, v.vars[start*4:end*4])
		if hasTail {
			v.setVar(tail, boolInt(!ok))
		} else if !ok {
			v.lastErr = "blackpackage: 写变量块失败 " + name
		}
	case 0x002f:
		// 读变量块：slot, path。原版 0x416800 直接把文件读进 vars[slot]。
		slot := v.evalSlot(r)
		name := v.expand(r.cstr())
		v.readVarBlock(name, slot)
	case 0x0032:
		// GetLocalTime 的 8 个字段依次写入 8 个槽位。
		slots := make([]int32, 0, 8)
		for i := 0; i < 8 && r.more(); i++ {
			slots = append(slots, v.evalSlot(r))
		}
		now := time.Now()
		vals := []int32{
			int32(now.Year()), int32(now.Month()), int32(now.Weekday()),
			int32(now.Day()), int32(now.Hour()), int32(now.Minute()),
			int32(now.Second()), int32(now.Nanosecond() / 1e6),
		}
		for i, sl := range slots {
			if i < len(vals) {
				v.setVar(sl, vals[i])
			}
		}
	case 0x0034:
		// 窗口表空检查：跨平台没有 HWND，恒为 0。
		slot := v.evalSlot(r)
		_ = v.eval(r)
		v.setVar(slot, 0)
	case 0x0044:
		// FindFirstFileA：有匹配写 1，否则写 0（原版只在失败时写 slot，
		// 但 slot 通常先被清零，这里统一写结果）。
		slot := v.evalSlot(r)
		pat := v.expand(r.cstr())
		p := v.resolvePath(pat)
		ok := false
		if p != "" {
			if m, err := filepath.Glob(p); err == nil && len(m) > 0 {
				ok = true
			}
		}
		v.setVar(slot, boolInt(ok))
	case 0x00af:
		// vars[slot] = 脚本数据中 (raw + Σargs*4) 处的 dword。
		slot := v.evalSlot(r)
		raw := int32(r.readRawU32())
		off := int32(0)
		for r.more() {
			off += v.eval(r)
		}
		pos := int(raw + off*4)
		if pos >= 0 && pos+4 <= len(s.Data) {
			v.setVar(slot, int32(binary.LittleEndian.Uint32(s.Data[pos:pos+4])))
		}
	case 0x0045:
		// CopyFileA(src, dst, FALSE)：失败时原版弹重试框。
		_ = v.eval(r)
		src := v.expand(r.cstr())
		dst := v.expand(r.cstr())
		if r.more() {
			_ = v.eval(r)
		}
		_ = copyFile(v.resolvePath(src), v.resolvePath(dst))
	case 0x0046:
		// SetFileAttributesA(path, FILE_ATTRIBUTE_NORMAL)：Linux 上改权限，
		// 目标不存在时保持原版的“失败可重试”语义但不阻塞剧情。
		name := v.expand(r.cstr())
		_ = os.Chmod(v.resolvePath(name), 0o644)
	case 0x0102:
		// 层/窗口颜色配置：index, color。原版把颜色打包进
		// 0x469c40[idx]/0x475464[idx] 供 0x16 的窗口建立使用；
		// 这里记录到 stage，窗口创建时再取。
		idx := v.eval(r)
		color := v.eval(r)
		if v.stage != nil {
			v.stage.SetWindowColor(idx, color)
		}
	case 0x0033:
		// 0x0033：MessageBox(resultSlot, flags, text)。原版是模态框，
		// 返回按钮 id 写入 slot，并把等待键抬起的动作留到宿主。
		slot := v.evalSlot(r)
		_ = v.eval(r)
		txt := v.expand(r.cstr())
		v.setVar(slot, 1)
		if txt != "" {
			v.Speaker = ""
			v.Text = txt
			step = StepAwaitInput
		}

	// ---- 图形/音频 ----
	case 0x0061:
		// 0x44a3f0：把（展开后的）文件名登记到 id 槽。后续 0x62 按 id 播放。
		id := v.eval(r)
		name := v.expand(r.cstr())
		if name != "" {
			v.sounds[id] = name
		}
	case 0x0062:
		// sndPlaySoundA(handle, flags|4)：按 id 播放登记过的 SE/系统音。
		id := v.eval(r)
		_ = v.eval(r) // flags：原版原样透传，宿主不需要
		if name := v.sounds[id]; name != "" {
			v.emitChannel(1, 0, name, false)
		}
	case 0x0063:
		// sndPlaySoundA(NULL, 0)：停止当前 SE（不是 stopAll！）。
		// 原版只停 SE 通道，BGM/语音由 e1 sel 36/47 单独控制；以前误映射成
		// stopAll 会把刚起的 BGM 一起掐掉，这是“没有背景音乐”的直接原因之一。
		v.Audio = append(v.Audio, Audio{Kind: 5, ID: 1})
	case 0x006b:
		// 0x422750(name, loop)：播放 BGM（原版走 MCI/MIDI 或 CD 音轨）。
		// 本作绝大多数 BGM 名是 `X0$ar<n>_GM.MID` 一类，实际存在的等价音频
		// 是 TR*.WA1 / z0*.WA1；名字解析交给宿主时的 materialize 统一处理，
		// 解析不到就静默跳过（与原版“找不到文件就报错继续”一致）。
		name := v.expand(r.cstr())
		loop := v.eval(r)
		if name != "" {
			v.emitChannel(0, 0, name, loop != 0)
		}
	case 0x006c:
		// 0x4226a0：停止 BGM。
		v.Audio = append(v.Audio, Audio{Kind: 3, ID: 0})
	case 0x00ac:
		// byte[0x48eae0+idx] = val。剧情分支和界面开关都用这张表。
		idx := v.eval(r)
		val := v.eval(r)
		if idx >= 0 && int(idx) < len(v.flags) {
			v.flags[idx] = int8(val)
		}
	case 0x00ad:
		// vars[slot] = (signed byte) byte[0x48eae0+idx]。
		slot := v.evalSlot(r)
		idx := v.eval(r)
		var val int32
		if idx >= 0 && int(idx) < len(v.flags) {
			val = int32(v.flags[idx])
		}
		v.setVar(slot, val)
	case 0x00b4:
		// 消息框可见标志置 1（0x464eb0）。
		v.msgVisible = true
	case 0x00b5:
		v.msgVisible = false
	case 0x00d8:
		// 系统开关表：0xd8(sel, val) 写 0x483224 起的槽。这里只保留值，
		// auto/skip/消息窗这些会在 0xe6 里另有实现；保留是为了脚本回读一致。
		sel := v.eval(r)
		val := v.eval(r)
		if sel >= 0 && int(sel) < len(v.uiFlags) {
			v.uiFlags[sel] = val
			switch sel {
			case 3:
				v.skipFlag = val != 0
			case 4:
				v.autoFlag = val != 0
			}
		}
	case 0x00e6:
		// 消息窗口/AUTO/SKIP 控制。selector 语义（反汇编 0x43cef0 分派）：
		//   0=初始化窗口 1=清 WAIT 4=清 SKIP 请求 8/9=逐行参数 3=AUTO 开 4=...
		// 本作实际用到 0/3/4/8/9/10；保持每个分支消费正确数量的表达式即可。
		sel := int32(r.byte())
		switch sel {
		case 0:
			v.msgVisible = true
		case 1:
			v.msgVisible = false
		case 3:
			v.skipFlag = true
		case 4:
			v.skipFlag = false
		case 8, 9:
			// 8/9：每行文本的属性表（索引, 参数...），4 个表达式。
			for i := 0; i < 4 && r.more(); i++ {
				_ = v.eval(r)
			}
		case 10:
			for i := 0; i < 4 && r.more(); i++ {
				_ = v.eval(r)
			}
		default:
			for r.more() {
				_ = v.eval(r)
			}
		}
	case 0x00e7:
		// 引擎窗口文字接口（INIT 里建立的 X0_fr00 消息窗）。selector：
		//   0=把文本画进图层（同 0x9，但不等待）  1=字体/颜色
		//   6/8=等待（yield 7/8，等玩家点击/定时） 12=把文本写进窗口缓冲
		//   13=把文本追加到窗口缓冲
		// 这组 op 是“对白/菜单文字由游戏自己画”的入口，不能整个跳过。
		sel := int32(r.byte())
		switch sel {
		case 0:
			// 同 0x9：layer, string。文本照常交给宿主显示，但不进入等待。
			v.active = v.eval(r)
			if v.stage != nil {
				v.stage.SetActive(v.active)
			}
			v.setMessage(v.expand(r.cstr()))
		case 1:
			// 字体尺寸/颜色：两个表达式（字号、颜色），只消费。
			for i := 0; i < 2 && r.more(); i++ {
				_ = v.eval(r)
			}
		case 4, 5:
			// 带坐标的文字绘制，实参是 6 个表达式再跟字符串：
			//   (x, y, 色/属性, 对齐, 字体缓冲基址, 文字缓冲基址, "文本")
			// 这是把标签画进图层（存档槽标题之类），不是对白。以前只吞 4 个
			// 表达式，cstr 会从表达式字节中间开始读，结果把 `\x02` 之类的
			// 控制字节当成对白显示（启动时出现乱码）。
			for i := 0; i < 6 && r.more(); i++ {
				_ = v.eval(r)
			}
			_ = v.expand(r.cstr())
		case 6:
			// yield 7：等待玩家点击（原文由 0x42bcf0 推 7 后返回 1）。
			step = StepAwaitInput
		case 8:
			// yield 8：等待定时器（原版把等待时长加进 0x465c24）。
			step = StepWait
			v.WaitMs = 16
		case 12:
			// 文本写入窗口：layer, ?, string。
			_ = v.eval(r)
			_ = v.eval(r)
			v.setMessage(v.expand(r.cstr()))
		case 13:
			// 追加文本：string。
			if txt := v.expand(r.cstr()); txt != "" {
				v.setMessage(txt)
			}
		default:
			// 参数个数因选择子而异；记录总长已经界定负载，消费不完整也不
			// 会影响 IP（exec 结束时统一 ip=next），所以安全地吞掉剩余字节。
		}
	case 0x00e5:
		// 文本引擎配置（INIT 里的窗口/字体/键位初始化）。首个字节是选择子，
		// 只需正确消费参数；不实现也不会画错（文本由 0x9/0xe7 提供）。
		_ = int32(r.byte())
		for r.more() {
			_ = v.eval(r)
		}
	case 0x00e3:
		// 文件搜索库（原版 0x43bc30 起的 42 个选择子）。本作 INIT 用 sel 3
		// 检查 `$s0\VERUP*.SO4` 是否存在来决定要不要跑升级脚本，漏实现会让
		// INIT 走进错误的升级分支。
		sel := int32(r.byte())
		switch sel {
		case 0:
			// 读文件进变量区（同 0x2f）。
			slot := v.evalSlot(r)
			_ = v.eval(r)
			name := v.expand(r.cstr())
			v.readVarBlock(name, slot)
		case 3:
			// slot = 通配符是否有匹配。
			slot := v.evalSlot(r)
			pattern := v.expand(r.cstr())
			v.setVar(slot, boolInt(v.globExists(pattern)))
		case 4:
			// slot = 当前搜索结果的文件大小/句柄（用作“是否有结果”）。
			slot := v.evalSlot(r)
			v.setVar(slot, 0)
		case 10:
			// 两个槽位 = 找到的文件大小（高低 32 位）。INIT@0xc62 拿它和
			// FindFirstFile 的两个字段相加比较，用来校验 X0WIN.DAT 是否完整。
			// 跨平台没有 Win32 的查找结构，统一报 0；对应的两个字段（sel
			// 32/33）也为 0，等式成立，不会误报“文件已损坏”。
			for i := 0; i < 2 && r.more(); i++ {
				slot := v.evalSlot(r)
				v.setVar(slot, 0)
			}
		case 32, 33:
			// FindFirstFile 结构里的两个数值字段：同样没有对应概念，写 0。
			slot := v.evalSlot(r)
			_ = v.expand(r.cstr())
			v.setVar(slot, 0)
		case 5, 6:
			// 关闭搜索/清理。
		default:
			for r.more() {
				_ = v.eval(r)
			}
		}
	case 0x00e0, 0x00e2:
		// 系统库分派（窗口属性/音量/时间）。开头是选择子字节。
		_ = int32(r.byte())
		for r.more() {
			_ = v.eval(r)
		}
	case 0x00e4:
		_ = int32(r.byte())
		for r.more() {
			_ = v.eval(r)
		}
	case 0x00a1, 0x00a4, 0x00a9, 0x00ab, 0x00ae,
		0x00b2, 0x00b6, 0x00b7, 0x00b8,
		0x00b9, 0x00bc, 0x00c0, 0x00c2, 0x00c6, 0x00c9, 0x00cb, 0x00cd,
		0x00ce, 0x00d0, 0x00d5, 0x00d7, 0x00da, 0x00db,
		0x00dc, 0x00dd:
		for r.more() {
			_ = v.eval(r)
		}
	case 0x00e1:
		// 音频子系统。首字节是选择子；语义按原版分派表（0x43920f 的
		// 60 项跳转表）逐支还原，只覆盖本作实际用到的：
		//   34/0x22 播放文件(channel, name, id)
		//   36/0x24 停止通道(channel)
		//   46/0x2e 淡出停止(channel, a, b, frames)
		//   47/0x2f 循环/淡入(channel, a, b, frames)
		//   58/0x3a 带参数的播放(mode, channel, name, vol, loop, fade, ...)
		//   33/0x21 BGM 播放（channel, 文件）
		//   其余（音量/查询/回声等）只消费参数。
		sel := int32(r.byte())
		switch sel {
		case 33:
			// 0x40f3e0(ch, x)：BGM 播放（老式 CD/MIDI 通道）。
			ch := v.eval(r)
			if r.more() {
				_ = v.eval(r)
			}
			// 该选择子只有一个表达式（通道），文件名在别处设置。
			_ = ch
		case 34:
			ch := v.eval(r)
			name := v.expand(r.cstr())
			id := int32(0)
			if r.more() {
				id = v.eval(r)
			}
			if name != "" {
				v.emitChannel(ch, id, name, false)
			}
		case 35:
			// 0x40f5f0(ch, a, b, mode, frames)：带淡入淡出的播放，参数里没有
			// 新文件名，沿用该通道已登记的曲目。
			ch := v.eval(r)
			for i := 0; i < 4 && r.more(); i++ {
				_ = v.eval(r)
			}
			if name := v.chanName[clampCh(ch)]; name != "" {
				v.emitChannel(ch, 0, name, true)
			}
		case 41:
			// 0x40eb80 的封装：原版全局 0x463b38 为 -1 表示音频驱动“已就绪”
			// （-1 是初始化失败的哨兵的反面用法），包装后 vars[slot]=1 表示
			// 正常。INIT@0x2b99 用 `V[ac6c] == 0` 判断失败分支：写 1 才会
			// 跳到 DirectSound 正常路径；写 0 会让 INIT 弹“DirectSound が
			// 使えません”并要求玩家在音乐/语音之间二选一。
			slot := v.evalSlot(r)
			v.setVar(slot, 1)
		case 42:
			// 0x40f260：停止所有播放器（切场景/回标题时用）。
			for ch := int32(0); ch < 4; ch++ {
				v.stopChannel(ch)
			}
		case 43, 44:
			// 0x40ebf0 / 0x40ec60：查询播放位置/总长（毫秒）。脚本只用它们
			// 做进度显示，没有真实播放器时给 0 即可。
			slot := v.evalSlot(r)
			if r.more() {
				_ = v.eval(r)
			}
			v.setVar(slot, 0)
		case 36:
			ch := v.eval(r)
			v.stopChannel(ch)
		case 46, 47:
			// 淡出/淡入停止：channel + 3 个参数。按“停止该通道”处理，
			// 保留通道号以免误停其它类别。
			ch := v.eval(r)
			for i := 0; i < 3 && r.more(); i++ {
				_ = v.eval(r)
			}
			v.stopChannel(ch)
		case 58:
			// mode, channel, filename, volume, loop, fade, ...（共 7 个参数）。
			mode := v.eval(r)
			ch := v.eval(r)
			name := v.expand(r.cstr())
			vol := int32(0)
			if r.more() {
				vol = v.eval(r)
			}
			loop := int32(0)
			if r.more() {
				loop = v.eval(r)
			}
			for r.more() {
				_ = v.eval(r)
			}
			if name != "" {
				// mode=1 是语音播放接口，0 是通用；两者都按文件名分类，
				// loop 参数以脚本为准（CD 音轨/BGM 会传 1）。
				v.emitChannel(ch, vol, name, loop != 0)
			}
			_ = mode
		default:
			// 其余选择子只改播放器内部状态（音量、查询、淡入淡出曲线）。
			for r.more() {
				_ = v.eval(r)
			}
		}
	case 0x00b0:
		// 媒体打开：两段路径 + 9 个布局参数 + 结果槽。
		// .mpg 交给宿主影片层；.wa1/.wav（CD 音轨）按 BGM 循环播放。
		// 原版同一个入口既播 MPEG-1 也播 CD-DA，本作的 TR*.WA1 走这条。
		p1 := v.expand(r.cstr())
		p2 := v.expand(r.cstr())
		vals := make([]int32, 0, 9)
		for i := 0; i < 9 && r.more(); i++ {
			vals = append(vals, v.eval(r))
		}
		slot := int32(0)
		hasSlot := false
		if r.more() {
			slot = v.evalSlot(r)
			hasSlot = true
		}
		name := p1
		if !strings.Contains(strings.ToLower(name), ".mpg") {
			name = p2
		}
		low := strings.ToLower(name)
		switch {
		case strings.Contains(low, ".mpg"):
			if hasSlot {
				v.setVar(slot, 0)
			}
			base := name
			if i := strings.LastIndexAny(base, "\\/"); i >= 0 {
				base = base[i+1:]
			}
			v.movie = base
			return StepAwaitMovie, nil
		case strings.HasSuffix(low, ".wa1"), strings.HasSuffix(low, ".wav"):
			// CD 音轨：取基名（脚本给的是 `$S4\TR02.WA1` 之类路径）。
			base := name
			if i := strings.LastIndexAny(base, "\\/"); i >= 0 {
				base = base[i+1:]
			}
			v.emitChannel(0, 0, base, true)
			if hasSlot {
				v.setVar(slot, 0)
			}
		default:
			if hasSlot {
				v.setVar(slot, 1)
			}
		}
	case 0x0053:
		// vars[slot] = timeGetTime()。原版是 32 位毫秒计数；脚本用它
		// 做动画/演出计时。必须实现，否则 STAFF 等轮询循环永不结束。
		slot := v.evalSlot(r)
		v.setVar(slot, int32(v.tickMs()))
	case 0x0055:
		// 忙等到 GetTickCount() >= target。原版会卡住线程；这里改成
		// 让宿主等待剩余毫秒，然后重新执行本条记录。
		target := uint32(v.eval(r))
		now := v.tickMs()
		if int32(target-now) > 0 {
			v.WaitMs = target - now
			v.ip = rec.Off
			return StepWait, nil
		}
	case 0x0082:
		// vars[slot] = GetDeviceCaps(caps)。ARG 12 是 BITSPIXEL（Win32 的
		// 常量就是 12），INIT 用它决定要不要提示“请改成 16 位色”。本作是
		// 800x600/16 位色时代的游戏，回报 16 可以让影片相关提示按原版的
		// “正常环境”分支走，而不是给玩家弹一个无意义的警告框。
		slot := v.evalSlot(r)
		caps := v.eval(r)
		if caps == 12 {
			v.setVar(slot, 16)
		} else {
			v.setVar(slot, 0)
		}
	case 0x0060, 0x0065, 0x0067, 0x006d, 0x006e, 0x006f,
		0x0071, 0x0072, 0x0073, 0x0075, 0x0076, 0x0077, 0x0079, 0x007e,
		0x007f, 0x0086, 0x008a, 0x008b, 0x008d, 0x008e, 0x008f,
		0x0091, 0x0093, 0x0094, 0x0095, 0x0096:
		for r.more() {
			_ = v.eval(r)
		}
	case 0x0098:
		layer := v.eval(r)
		x := v.eval(r)
		y := v.eval(r)
		w := v.eval(r)
		h := v.eval(r)
		if v.stage != nil {
			v.stage.Fill(layer, x, y, w, h, 0)
		}
	case 0x009a:
		args := make([]int32, 0, 16)
		for r.more() && len(args) < 16 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil {
			v.stage.Chip(args)
		}
	case 0x009b:
		_ = v.eval(r)
	case 0x009c:
		if v.stage != nil {
			v.stage.Redraw()
		}
	case 0x009d:
		_ = v.eval(r)
	case 0x009e:
		_ = v.eval(r)
	case 0x009f:
		_ = v.eval(r)

	case 0x00a2:
		// 与 0x6 同一个 0x402e70，唯一区别是 arg3=1（直接 24 位颜色）。
		args := make([]int32, 0, 7)
		for r.more() && len(args) < 7 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil && len(args) >= 7 {
			v.stage.FillRect(args[2], args[3], args[4], args[5], args[6], args[1])
		}
	case 0x00a6:
		// 与 0x7 同一个 0x404300，arg3=1。
		args := make([]int32, 0, 10)
		for r.more() && len(args) < 10 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil && len(args) >= 10 {
			v.stage.CopyRect(args[2], args[7], args[3], args[4], args[5], args[6], args[8], args[9])
		}
	case 0x00a3:
		// 载入图片到层：layer, ?, ?, x, y, 文件名,
		//   [4 个输出槽 = x, y, w, h], [绘制模式]
		//
		// 原版把立绘先画到 1 号层 800x3000 画布的离屏区（y=600*k），再把
		// 回写的矩形用 0x7 拷到可见位置；x/y 放在第 4/5 个参数里。以前忽略
		// 了这两个坐标和输出槽，所有立绘都画在图片自带偏移上 → 多角色重叠、
		// 换表情时旧图不消失。
		layer := v.eval(r)
		a1 := v.eval(r)
		a2 := v.eval(r)
		a3 := v.eval(r)
		a4 := v.eval(r)
		rawName := r.cstr()
		name := v.expand(rawName)
		v.LastImageRaw, v.LastImageExpanded = rawName, name
		if name != "" {
			v.images = append(v.images, name)
		}
		if v.FirstImageRaw == "" && strings.Contains(rawName, "$sr") {
			v.FirstImageRaw = rawName
			v.FirstImageBase = v.bufBase[127]
		}
		var out [4]int32
		outN := 0
		for outN < 4 && r.more() {
			out[outN] = v.evalSlot(r)
			outN++
		}
		if r.more() {
			// 绘制模式（原版默认 7）：只影响对齐/混合细节，暂不建模。
			_ = v.eval(r)
		}
		args := []int32{layer, a1, a2, a3, a4}
		if v.stage != nil {
			v.stage.LoadImage(layer, name, args)
			if outN == 4 {
				w, h := v.stage.ImageSize(name)
				v.setVar(out[0], a3)
				v.setVar(out[1], a4)
				v.setVar(out[2], int32(w))
				v.setVar(out[3], int32(h))
			}
		}
	case 0x00d6:
		// 等待输入：原版默认 flags=3，清 [0x464054] 后 yield 14。
		if r.more() {
			_ = v.eval(r)
		}
		step = StepAwaitInput
	case 0x00d1:
		sel := int32(r.readRawU32())
		args := make([]int32, 0, 20)
		for r.more() && len(args) < 20 {
			args = append(args, v.eval(r))
		}
		if v.stage != nil {
			v.stage.Blit(sel, args)
		}

	// ---- 表达式/赋值/控制 ----
	case 0x0200:
		slot := v.evalSlot(r)
		v.setVar(slot, v.eval(r))
	case 0x0203:
		slot := v.evalSlot(r)
		a := v.eval(r)
		b := v.eval(r)
		v.setVar(slot, a>>uint(b&31))
	case 0x0204:
		slot := v.evalSlot(r)
		a := v.eval(r)
		b := v.eval(r)
		v.setVar(slot, a<<uint(b&31))
	case 0x0207:
		slot := v.evalSlot(r)
		v.setVar(slot, v.eval(r)&v.eval(r))
	case 0x0208:
		slot := v.evalSlot(r)
		v.setVar(slot, v.eval(r)|v.eval(r))
	case 0x0209:
		slot := v.evalSlot(r)
		v.setVar(slot, v.eval(r)^v.eval(r))
	case 0x020c:
		slot := v.evalSlot(r)
		v.setVar(slot, v.eval(r)+v.eval(r))
	case 0x020d:
		// dst = a - b。除了普通减法，本作还把它当“动画帧”用：OP 的淡出
		// 循环是 `V[x] = V[x] - 0`，原版靠 DirectDraw 回调递减 x 到 0 才
		// 退出；没有回调就会死循环。检测到“减 0 且值不变”时按帧计数，
		// 超过 30 帧把 x 清零（等价于动画走完），避免卡死在 OP。
		slot := v.evalSlot(r)
		a := v.eval(r)
		b := v.eval(r)
		res := a - b
		if b == 0 && res == v.varAt(slot) {
			v.noopTicks[slot]++
			if v.noopTicks[slot] > 30 {
				res = 0
				delete(v.noopTicks, slot)
			}
		} else {
			delete(v.noopTicks, slot)
		}
		v.setVar(slot, res)
	case 0x020e, 0x020f:
		slot := v.evalSlot(r)
		v.setVar(slot, v.eval(r)*v.eval(r))
	case 0x0210:
		slot := v.evalSlot(r)
		a := v.eval(r)
		b := v.eval(r)
		if b != 0 {
			v.setVar(slot, a/b)
		} else {
			v.setVar(slot, 0)
		}
	case 0x0212:
		slot := v.evalSlot(r)
		a := v.eval(r)
		b := v.eval(r)
		if b != 0 {
			v.setVar(slot, a%b)
		}
	case 0x0214:
		dst := v.evalSlot(r)
		idx := int32(0)
		for r.more() {
			idx += v.eval(r)
		}
		v.setVar(dst, v.varAt(idx))
	case 0x0215:
		// 0x43e690：地址由前 n-1 个表达式之和给出，最后一个表达式是值。
		// 典型用法是 vars[bufbase[k] + ofs] = value。
		off := int32(0)
		last := int32(0)
		for r.more() {
			off += last
			last = v.eval(r)
		}
		v.setVar(off, last)
	case 0x0216:
		v.push(v.eval(r))
	case 0x0217:
		// 弹出 0x216 压入的值；记录长度是 4，没有操作数。
		_ = v.pop()
	case 0x021a:
		target := int32(r.readRawU32())
		v.ip = int(target)
		return StepContinue, nil
	case 0x021b:
		target := int32(r.readRawU32())
		v.push(int32(next))
		if v.callDepth >= 0x400 {
			return StepFaulted, fmt.Errorf("blackpackage: 递归过深")
		}
		v.callDepth++
		v.ip = int(target)
		return StepContinue, nil
	case 0x021c:
		ret := v.pop()
		if v.callDepth > 0 {
			v.callDepth--
		}
		v.ip = int(ret)
		return StepContinue, nil
	case 0x0230:
		buf := v.eval(r)
		v.setBufferBase(buf, v.evalSlot(r))
	case 0x0232:
		buf := v.eval(r)
		off := v.eval(r)
		val := v.eval(r)
		if b := v.bufBytes(buf, off); b != nil {
			b[0] = byte(val)
		}
	case 0x0233:
		buf := v.eval(r)
		off := v.eval(r)
		val := v.eval(r)
		if b := v.bufBytes(buf, off); len(b) >= 2 {
			binary.LittleEndian.PutUint16(b, uint16(val))
		}
	case 0x0235:
		buf := v.eval(r)
		off := v.eval(r)
		val := v.eval(r)
		if b := v.bufBytes(buf, off); len(b) >= 4 {
			binary.LittleEndian.PutUint32(b, uint32(val))
		}
	case 0x0237:
		buf := v.eval(r)
		off := v.eval(r)
		s := v.expand(r.cstr())
		v.copyStringToBuf(buf, off, encodeCP932(s))
	case 0x0238:
		dst := v.evalSlot(r)
		buf := v.eval(r)
		off := v.eval(r)
		if b := v.bufBytes(buf, off); len(b) > 0 {
			v.setVar(dst, int32(b[0]))
		}
	case 0x023b:
		dst := v.evalSlot(r)
		buf := v.eval(r)
		off := v.eval(r)
		if b := v.bufBytes(buf, off); len(b) >= 4 {
			v.setVar(dst, int32(binary.LittleEndian.Uint32(b)))
		}
	case 0x023d:
		_ = v.eval(r)
	case 0x023e:
		a := v.evalSlot(r)
		b := v.evalSlot(r)
		if a > b {
			a, b = b, a
		}
		for i := a; i <= b; i++ {
			v.setVar(i, 0)
		}
	case 0x0240:
		v.ip = int(v.eval(r))
		return StepContinue, nil
	case 0x0245:
		_ = v.eval(r)
	case 0x0246:
		slot := v.evalSlot(r)
		value := v.eval(r)
		// 扫描 0x245 记录，找第一个值相等的块，落一个相对偏移到 slot。
		off := rec.Off + rec.Len
		for off < len(s.Data) {
			r2 := s.Recs[off]
			if r2.Op == 0x0245 {
				er := &exprReader{vm: v, pay: r2.Payload}
				if v.eval(er) == value {
					v.setVar(slot, int32(off))
					break
				}
			}
			if r2.Op == 0x0001 {
				break
			}
			off += r2.Len
		}
	case 0x024b:
		// 0x43fd70：把 count 个 dword 写入 vars[slot + i*stride]。
		slot := v.evalSlot(r)
		count := v.eval(r)
		val := int32(0)
		if r.more() {
			val = v.eval(r)
		}
		stride := int32(1)
		if r.more() {
			stride = v.eval(r)
		}
		for i := int32(0); i < count; i++ {
			v.setVar(slot+i*stride, val)
		}
	case 0x024d:
		_ = v.eval(r)
	case 0x024e:
		target := int32(r.readRawU32())
		if v.eval(r) != 0 {
			v.ip = int(target)
			return StepContinue, nil
		}
	case 0x024f:
		target := int32(r.readRawU32())
		if v.eval(r) == 0 {
			v.ip = int(target)
			return StepContinue, nil
		}
	case 0x0252:
		dst := v.evalSlot(r)
		left := int32(0)
		right := int32(0)
		found := false
		for r.more() {
			a := v.eval(r)
			b := v.eval(r)
			if a == b && !found {
				left = 0
				found = true
			} else if !found && a > b {
				left = 1
				found = true
			}
		}
		_ = right
		v.setVar(dst, left)
	case 0x0259:
		buf := v.eval(r)
		off := v.eval(r)
		w := v.eval(r)
		h := v.eval(r)
		_ = w
		_ = h
		_ = buf
		_ = off
	case 0x025c:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)+1)
	case 0x025d:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)-1)
	case 0x025e:
		slot := v.evalSlot(r)
		v.setVar(slot, ^v.varAt(slot))
	case 0x0261:
		slot := v.evalSlot(r)
		n := v.eval(r)
		v.setVar(slot, v.varAt(slot)>>uint(n&31))
	case 0x0264:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)&v.eval(r))
	case 0x0265:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)|v.eval(r))
	case 0x0266:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)^v.eval(r))
	case 0x0267:
		slot := v.evalSlot(r)
		bit := v.eval(r)
		v.setVar(slot, v.varAt(slot)|(1<<uint(bit&31)))
	case 0x0268:
		slot := v.evalSlot(r)
		bit := v.eval(r)
		v.setVar(slot, v.varAt(slot)&^(1<<uint(bit&31)))
	case 0x0269:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)+v.eval(r))
	case 0x026a:
		slot := v.evalSlot(r)
		v.setVar(slot, v.varAt(slot)-v.eval(r))

	// ---- 字符串/文本 ----
	case 0x0300:
		// 文本格式模式；原版取一个字节 mode 后走不同渲染分支。
		// 我们只消费这个字节，真正文本由 op 0x9/0x488 提供。
		if r.more() {
			r.pos++
		}
	case 0x042c:
		dst := v.evalSlot(r)
		ch := byte(v.eval(r))
		if ch >= 0x80 {
			v.setVar(dst, 1)
		} else {
			v.setVar(dst, 0)
		}
	case 0x045d:
		buf := v.eval(r)
		v.copyStringToBuf(buf, 0, encodeCP932(v.expand(r.cstr())))
	case 0x045f:
		buf := v.eval(r)
		v.concatBuffer(buf, 0, encodeCP932(v.expand(r.cstr())))
	case 0x0460:
		dst := v.eval(r)
		dstOff := v.eval(r)
		src := v.eval(r)
		srcOff := v.eval(r)
		v.copyBuffer(dst, dstOff, src, srcOff)
	case 0x0465:
		buf := v.eval(r)
		off := v.eval(r)
		v.copyStringToBuf(buf, off, encodeCP932(v.expand(r.cstr())))
	case 0x0466:
		dst := v.eval(r)
		dstOff := v.eval(r)
		src := v.eval(r)
		srcOff := v.eval(r)
		v.copyBuffer(dst, dstOff, src, srcOff)
	case 0x0467:
		buf := v.eval(r)
		off := v.eval(r)
		v.concatBuffer(buf, off, encodeCP932(v.expand(r.cstr())))
	case 0x0468:
		dst := v.eval(r)
		dstOff := v.eval(r)
		src := v.eval(r)
		srcOff := v.eval(r)
		v.concatBuffer(dst, dstOff, v.readCStringAt(src, srcOff, 0x10000))
	case 0x0472:
		dst := v.evalSlot(r)
		buf := v.eval(r)
		off := v.eval(r)
		want := v.expand(r.cstr())
		got := v.readCString(buf, off)
		if got == want {
			v.setVar(dst, 0)
		} else {
			v.setVar(dst, 1)
		}
	case 0x0473:
		dst := v.evalSlot(r)
		ab := v.eval(r)
		ao := v.eval(r)
		bb := v.eval(r)
		bo := v.eval(r)
		if v.readCString(ab, ao) == v.readCString(bb, bo) {
			v.setVar(dst, 0)
		} else {
			v.setVar(dst, 1)
		}
	case 0x0478:
		dst := v.evalSlot(r)
		buf := v.eval(r)
		off := v.eval(r)
		pat := v.expand(r.cstr())
		if wildcardMatch(v.readCString(buf, off), pat) {
			v.setVar(dst, 0)
		} else {
			v.setVar(dst, 1)
		}
	case 0x047a, 0x047b:
		dst := v.evalSlot(r)
		_ = v.eval(r)
		_ = v.eval(r)
		_ = v.expand(r.cstr())
		v.setVar(dst, 1)
	case 0x0481:
		dst := v.evalSlot(r)
		buf := v.eval(r)
		off := v.eval(r)
		v.setVar(dst, int32(len(v.readCStringAt(buf, off, 0x10000))))
	case 0x0488:
		buf := v.eval(r)
		off := v.eval(r)
		v.copyStringToBuf(buf, off, encodeCP932(r.cstr()))

	default:
		// 未实现的 op 按“无副作用”跳过，保证坏脚本/非主线脚本不会卡死整局。
	}

	v.ip = next
	return step, nil
}

func (v *VM) push(x int32) {
	if len(v.stack) >= valStackMax {
		return
	}
	v.stack = append(v.stack, x)
}

func (v *VM) pop() int32 {
	if len(v.stack) == 0 {
		return 0
	}
	x := v.stack[len(v.stack)-1]
	v.stack = v.stack[:len(v.stack)-1]
	return x
}

func (v *VM) yieldForMenu() Step {
	if len(v.menu) > 0 {
		v.Choices = append([]Choice(nil), v.menu...)
		return StepAwaitChoice
	}
	return StepAwaitInput
}

// Choose 把选择结果写回 vars 并清空菜单。原版把结果放在 #select_value，
// 但 X0 的选择肢具体槽位由脚本决定；这里返回 index 让 session 记录。
func (v *VM) Choose(index int) (Step, error) {
	if index < 0 || index >= len(v.Choices) {
		return StepFaulted, fmt.Errorf("blackpackage: 选择项越界")
	}
	v.ChooseValue(v.Choices[index].Value)
	return StepContinue, nil
}

// ChooseValue 按脚本给的选择值落结果槽并清空菜单。引擎自绘选择肢直接
// 用值回调，避免再经过索引映射。
func (v *VM) ChooseValue(value int32) {
	// 0x15 条目携带的 value 是脚本给选择项的编号；原版的点击回调
	// 还会同步窗口/菜单状态（e6 子系统）。这里至少把编号落进常用槽位，
	// 让依赖该槽的脚本分支能读到玩家的选择。
	v.setVar(0xac4e, value)
	v.menu = nil
	v.Choices = nil
}

func (v *VM) setMessage(raw string) {
	txt := v.expand(raw)
	v.Speaker, v.Text = splitSpeaker(txt)
}

func splitSpeaker(s string) (string, string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "_「"); i > 0 {
		return strings.TrimSpace(s[:i]), s[i+1:]
	}
	if i := strings.Index(s, "「"); i > 0 {
		return strings.TrimSpace(s[:i]), s[i:]
	}
	return "", s
}

func wildcardMatch(s, pat string) bool {
	if pat == "" {
		return s == ""
	}
	// 只支持 * 与 ?，够原版文件通配使用。
	var dp func(i, j int) bool
	memo := map[[2]int]bool{}
	seen := map[[2]int]bool{}
	dp = func(i, j int) bool {
		k := [2]int{i, j}
		if seen[k] {
			return memo[k]
		}
		seen[k] = true
		var ok bool
		switch {
		case j == len(pat):
			ok = i == len(s)
		case pat[j] == '*':
			ok = dp(i, j+1) || (i < len(s) && dp(i+1, j))
		case i < len(s) && (pat[j] == '?' || pat[j] == s[i]):
			ok = dp(i+1, j+1)
		}
		memo[k] = ok
		return ok
	}
	return dp(0, 0)
}

// ---- 文件/音频辅助 ----

func boolInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// resolvePath 把原版相对当前目录的文件名映射到可写目录。
// 存档/全局数据统一以 X0WIN 开头（X0WIN00.DAT、X0WINRE.DAT…），
// 放到 storeDir 既不污染游戏目录，也便于宿主做备份。
func (v *VM) resolvePath(name string) string {
	p := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if p == "" {
		return ""
	}
	base := p
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if v.storeDir != "" && strings.HasPrefix(strings.ToUpper(base), "X0WIN") {
		return filepath.Join(v.storeDir, base)
	}
	if v.storeDir != "" && !filepath.IsAbs(p) && !strings.Contains(p, "/") {
		return filepath.Join(v.storeDir, p)
	}
	return p
}

// fileExists 复刻原版 FileCheck（0x417eb0）：先查当前目录，再查引擎的
// 路径表。本作的“路径表”就是 X0WIN.DAT/LST 归档 —— 语音、图片、音效
// 全在 DAT 里而不是磁盘上，所以只 stat 磁盘一定失败。
//
// 本作大量用 `0x2d slot, "b0266.wa1"` + 条件跳转决定“这句有没有语音”；
// 这里漏查归档会让所有语音分支都被跳过（表现为完全没有角色语音）。
func (v *VM) fileExists(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	// 归档内条目（语音/音效/图片/剧本）。
	if v.set != nil && v.set.Has(trimmed) {
		return true
	}
	// 只给主干名时也要能命中（脚本里两种写法都有）。
	if i := strings.LastIndexAny(trimmed, "\\/"); i >= 0 {
		base := trimmed[i+1:]
		if v.set != nil && v.set.Has(base) {
			return true
		}
	}
	return false
}

func (v *VM) writeVarBlock(name string, data []byte) bool {
	p := v.resolvePath(name)
	if p == "" {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false
	}
	return os.WriteFile(p, data, 0o644) == nil
}

func (v *VM) readVarBlock(name string, slot int32) {
	p := v.resolvePath(name)
	if p == "" || slot < 0 {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	off := int(slot) * 4
	if off < 0 || off >= len(v.vars) {
		return
	}
	n := len(data)
	if off+n > len(v.vars) {
		n = len(v.vars) - off
	}
	copy(v.vars[off:off+n], data[:n])
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// Audio 事件的类别沿用 core：0=BGM，1=SE，2=语音，3/4/5=对应停止。

// audioKindForName 按文件名判定音频类别。FFA 的命名规则（本作 6356 个 WA1
// 全部核对过）：
//
//	z0*/bgm*、TR*.WA1（CD 音轨）        → BGM
//	a0*/b0*/c0*/d0*/a1..d1（角色语音）   → 语音
//	y0*/effect*（系统音/效果音）         → SE
//
// 不能只按通道号判断：本作通道 1 同时承载语音和音效，通道 0 才是 BGM。
func audioKindForName(name string) uint32 {
	low := strings.ToLower(name)
	switch {
	case strings.HasPrefix(low, "z0"), strings.HasPrefix(low, "bgm"),
		strings.HasPrefix(low, "tr"):
		return 0
	case strings.HasPrefix(low, "a0"), strings.HasPrefix(low, "a1"),
		strings.HasPrefix(low, "b0"), strings.HasPrefix(low, "b1"),
		strings.HasPrefix(low, "c0"), strings.HasPrefix(low, "c1"),
		strings.HasPrefix(low, "d0"), strings.HasPrefix(low, "d1"):
		return 2
	default:
		return 1
	}
}

// defaultChanKind 是通道的初始类别：0=BGM、1=SE、2=语音、3=SE。
// 停止后必须回到默认值，否则下一次停止会继承上一次的类别（表现是一连串
// “停止 BGM”事件把音乐掐掉）。
func defaultChanKind(ch int) uint32 {
	switch ch {
	case 0:
		return 0
	case 2:
		return 2
	default:
		return 1
	}
}

func clampCh(ch int32) int {
	if ch < 0 {
		return 0
	}
	if ch > 3 {
		return 3
	}
	return int(ch)
}

// emitChannel 把“在通道 ch 上播放 name”翻译成宿主事件，并记住该通道的
// 类别与曲目名。停止时要用同一份记忆决定发哪种 stop（见 stopChannel）。
func (v *VM) emitChannel(ch, volume int32, name string, loop bool) {
	kind := audioKindForName(name)
	idx := clampCh(ch)
	// 音量参数在原版是 0..255 的字节；只有少数脚本传 0..100，按比例归一。
	vol := 1.0
	if volume > 0 {
		vol = float64(volume) / 100.0
		if vol > 1 {
			vol = float64(volume) / 255.0
		}
	}
	if vol > 1 {
		vol = 1
	}
	v.chanKind[idx] = kind
	v.chanName[idx] = name
	v.Audio = append(v.Audio, Audio{
		Kind: kind, Name: name, ID: int32(idx), Flags: volume,
		Loop: loop || kind == 0, Volume: vol,
	})
}

// stopChannel 停止通道。原版 0x40eda0/0x40efc0 都是“停这一条通道”，
// 类别由这条通道上最后播放的内容决定，不能按通道号硬编码成停 SE。
func (v *VM) stopChannel(ch int32) {
	idx := clampCh(ch)
	kind := v.chanKind[idx]
	var stopKind uint32
	switch kind {
	case 0:
		stopKind = 3
	case 2:
		stopKind = 4
	default:
		stopKind = 5
	}
	v.chanKind[idx] = defaultChanKind(idx)
	v.chanName[idx] = ""
	v.Audio = append(v.Audio, Audio{Kind: stopKind, ID: int32(idx)})
}

// globExists 按原版 FindFirstFile 的语义检查通配符是否有匹配：
// 先查归档（X0WIN.DAT 里的名字），再查磁盘（存档/升级文件）。
func (v *VM) globExists(pattern string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false
	}
	base := p
	if i := strings.LastIndexAny(base, "\\/"); i >= 0 {
		base = base[i+1:]
	}
	lowPat := strings.ToLower(base)
	if v.set != nil && strings.ContainsAny(lowPat, "*?") {
		for _, n := range v.set.Names() {
			if wildcardMatch(strings.ToLower(n), lowPat) {
				return true
			}
		}
	} else if v.set != nil && v.set.Has(base) {
		return true
	}
	// 脚本里的路径常带 `$s3\` 这类由 COPY_EXE_DIR 写入的盘符/目录前缀。
	// 跨平台没有“安装盘”，我们把它等价成游戏目录：前缀可能为空，所以除
	// 原始路径外还要单独拿基名到游戏目录里找一次（X0WIN.DAT 之类在
	// 游戏目录下真实存在，靠这一步 INIT 才不会误判安装损坏）。
	if m, err := filepath.Glob(v.resolvePath(p)); err == nil && len(m) > 0 {
		return true
	}
	if v.set != nil {
		dir := v.set.Dir()
		candidate := filepath.Join(dir, base)
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
		if strings.ContainsAny(lowPat, "*?") {
			if m, err := filepath.Glob(filepath.Join(dir, base)); err == nil && len(m) > 0 {
				return true
			}
		}
	}
	return false
}

// tickMs 返回从 VM 启动起的毫秒数。用单调差值而不是 Unix 时间，
// 脚本里不会出现 int32 溢出导致的比较错误。
func (v *VM) tickMs() uint32 {
	if v.start.IsZero() {
		return 0
	}
	return uint32(time.Since(v.start) / time.Millisecond)
}
