package runtime

import (
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/runtime/bs"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

const (
	messageVoicePayloadOffsetForTyped    = 14
	messageVoicePayloadOffsetForExtended = 18
)

type MessageDirector struct {
	machine *vm.BsxMachine
	script  *vm.BsxScriptFile
	state   *state.GameState
	layout  config.MessageWindow
}

func NewMessageDirector(machine *vm.BsxMachine, script *vm.BsxScriptFile, gameState *state.GameState, layout config.MessageWindow) *MessageDirector {
	return &MessageDirector{machine: machine, script: script, state: gameState, layout: layout}
}

func (d *MessageDirector) Show() state.SessionStep {
	dialect := d.script.Dialect()
	var messageType byte = 1
	if dialect.HasMessageType() {
		messageType = d.machine.Code()[d.machine.InstructionStart()+1]
	}
	messageId := d.machine.Operand(dialect.MessageIdField())
	// 剧本正文里的 "\n" 是原引擎的换行转义，不是两个可见字符；
	// 引擎消息框和回想都按真实换行排版，宿主收到的文本也应是展开后的。
	// 之后先把 \A/\B 姓名占位符替换成默认姓名（角色表第 0 项就是 "\B"），
	// 再交给 bs.Message 剥掉 \b 等其余控制码；直接剥会把"雅也君"里的名字
	// 一起删掉。先展开换行再剥码，否则 "\n" 会被当成"反斜杠 + n"整段删掉。
	rawText := d.script.GetMessage(int(messageId))
	// visibleText 不替换姓名占位符：它只用来判断"这一条是不是纯控制码"。
	// 正文只有 \B（本作 msg#35985）时，替换后会变成"雅也"，看起来像一句
	// 正常对白，其实原版这里是一个名字/控制消息，不该停成对话框。
	visibleText := bs.Message(normalizeDialogue(rawText))
	text := bs.Message(normalizeDialogue(d.state.ResolveNameCodes(rawText)))
	speaker := ""
	characterField := dialect.MessageCharacterField()
	if messageType >= 1 && characterField >= 0 {
		// 角色名同样可能是 "\B"（主人公占位符）。先用默认姓名替换，
		// 再由 bs.Speaker 剥掉其它控制码；原版显示的是名字，不是码字。
		rawSpeaker := d.state.ResolveNameCodes(d.script.GetCharacterName(int(d.machine.Operand(characterField))))
		speaker = bs.Speaker(rawSpeaker)
	}

	// 语音先处理：只有控制码、没有可见正文的消息也可能带语音，
	// 不能因为正文为空就把声音一起吞掉。
	if dialect.HasMessageType() && messageType >= 2 {
		if messageType == 2 {
			d.applyVoice(messageVoicePayloadOffsetForTyped)
		} else {
			d.applyVoice(messageVoicePayloadOffsetForExtended)
		}
	}

	// 纯控制码消息（本作里 msg#35985 的正文就是 "\B"）在原引擎里不会
	// 停成一个只有名字、没有正文的空对话框，而应直接继续执行下一条指令。
	// 之前这里无条件 SetAwaitingInput，玩家看到的就是"只显示 \B，点一下
	// 才跳到下一句"。
	if strings.TrimSpace(visibleText) == "" {
		d.state.SetText("")
		d.state.SetSpeaker("")
		d.state.SetMultiText("")
		d.state.SetMultiSpeaker("")
		d.machine.ApplyDeferredJump()
		core.Logf("text: msg#%d skipped (only text codes: %q)", messageId, rawText)
		return state.SessionContinue
	}

	// 正文永远写主框。剧本的 window part=0/1/2 是消息框的三个显示档
	// （上/中/下位置变体，换景时三个一起关、再按场景需要开），不是
	// "写哪只框"的路由开关——旧实现把它当第二框路由后，屈辱/KJ 的剧本
	// 只要开过一次 part=2（例如 14:40:43 那次场景，连开 46 句），之后所有
	// 正文都进了不可见的第二框，主框就一直挂着上一句；原引擎里这些对白
	// 全部显示在主框。multi messages 的第二发言框由 mm_* 布局描述、
	// 只有真正写过多框文本时才会绘制（当前没有指令产生），这里一并清空，
	// 保持与未引入第二框前完全一致的行为。
	d.state.SetText(text)
	d.state.SetSpeaker(speaker)
	d.state.SetMultiText("")
	d.state.SetMultiSpeaker("")
	d.state.SetMessageId(int(messageId))
	d.state.SetAwaitingInput(true)
	core.Logf("text: msg#%d char='%s' '%s'", messageId, d.state.Speaker(), trimMessage(d.state.Text()))
	d.machine.ApplyDeferredJump()
	return state.SessionAwaitInput
}

func (d *MessageDirector) ApplyWindowVisibility() state.SessionStep {
	part := int(d.machine.Frame(1))
	flag := d.machine.Frame(2) != 0
	core.Logf("ui: window part=%d flag=%v", part, flag)
	// part 0/1/2 是消息框的三个显示档（上/中/下位置变体）：换景时三个一起
	// 关掉，进入新场景后再由剧本决定开哪几档（屈辱的日志里既有开 0+1 的
	// 场景，也有三档全开的场景）。它们只切换可见性，不路由 message 的
	// 写入目标——正文永远写主框，见 Show。
	// part=1 时主框标记随动，transition 期间整屏关窗的表现与原版一致；
	// part=2 只影响 mm 第二框的可见标记，多框文本不会被写入，绘制端自然
	// 不会画出第二框。
	d.state.SetWindowVisible(part, flag)
	return state.SessionContinue
}

func (d *MessageDirector) ApplyOption() state.SessionStep {
	name := d.script.GetName(int(d.machine.Frame(1)))
	value := d.machine.Frame(2)
	if strings.EqualFold(name, "mesg") && value > 0 {
		d.state.SetTextSpeed(float64(value))
	}
	return state.SessionContinue
}

func (d *MessageDirector) applyVoice(payloadOffset int) {
	if d.machine.Operand(10) <= 0 {
		return
	}
	voice := d.script.GetName(int(d.machine.Operand(payloadOffset)))
	if len(voice) == 0 {
		return
	}
	d.state.SetVoice(voice)
	d.state.SetVoiceSerial(d.state.VoiceSerial() + 1)
	core.Logf("voice: '%s'", voice)
}

func trimMessage(value string) string {
	runes := []rune(value)
	if len(runes) <= 32 {
		return value
	}
	return string(runes[:32])
}

// normalizeDialogue 展开剧本里的常见反斜杠转义。
// 只处理换行/制表，其它以反斜杠开头的文本（颜色、语音等）保持原样，
// 避免误伤游戏自己的控制码。
func normalizeDialogue(value string) string {
	if !strings.ContainsRune(value, '\\') {
		return value
	}
	value = strings.ReplaceAll(value, "\\n", "\n")
	value = strings.ReplaceAll(value, "\\r", "\r")
	value = strings.ReplaceAll(value, "\\t", "\t")
	return value
}
