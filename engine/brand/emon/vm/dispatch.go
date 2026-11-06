package vm

import (
	"fmt"
	"strings"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const maxInlineExpansions = 256

var handlers = map[string]func(*Machine, *script.EmonStatement) EmonStep{
	"Start":               (*Machine).onStart,
	"Null":                (*Machine).onNull,
	"End":                 (*Machine).onEnd,
	"ScriptEnd":           (*Machine).onScriptEnd,
	"ScriptReset":         (*Machine).onScriptReset,
	"Jump":                (*Machine).onJump,
	"IfJump":              (*Machine).onIfJump,
	"RandomJump":          (*Machine).onRandomJump,
	"SceneName":           (*Machine).onSceneName,
	"Macro":               (*Machine).onMacro,
	"EndMacro":            (*Machine).onEndMacro,
	"Define":              (*Machine).onDefine,
	"Lock":                (*Machine).onLock,
	"Unlock":              (*Machine).onUnlock,
	"LockSelect":          (*Machine).onLockSelect,
	"UnlockSelect":        (*Machine).onUnlockSelect,
	"CursolMoveLock":      (*Machine).onCursolMoveLock,
	"NoSaveOn":            (*Machine).onNoSaveOn,
	"NoSaveOff":           (*Machine).onNoSaveOff,
	"SetNumber":           (*Machine).onSetNumber,
	"Number":              (*Machine).onNumber,
	"IfNumber":            (*Machine).onIfNumber,
	"RandomNumber":        (*Machine).onRandomNumber,
	"SetSysNumber":        (*Machine).onSetSysNumber,
	"SetMacroNumber":      (*Machine).onSetMacroNumber,
	"SetSymbol":           (*Machine).onSetSymbol,
	"GetStrLen":           (*Machine).onGetStrLen,
	"String":              (*Machine).onString,
	"SetOnListStr":        (*Machine).onSetOnListStr,
	"SetOnCalListNumber":  (*Machine).onSetOnCalListNumber,
	"Text":                (*Machine).onTextCommand,
	"Page":                (*Machine).onPage,
	"Return":              (*Machine).onReturn,
	"Wait":                (*Machine).onWait,
	"Time":                (*Machine).onTime,
	"SkipOn":              (*Machine).onSkipOn,
	"SkipOff":             (*Machine).onSkipOff,
	"SetSkipMode":         (*Machine).onSetSkipMode,
	"AutoOn":              (*Machine).onAutoOn,
	"AutoOff":             (*Machine).onAutoOff,
	"ClearMessage":        (*Machine).onClearMessage,
	"Message":             (*Machine).onMessage,
	"SetSprite":           (*Machine).onSetSprite,
	"SetSprPos":           (*Machine).onSetSprPos,
	"SetSprMsk":           (*Machine).onSetSprMsk,
	"SetSprColor":         (*Machine).onSetSprColor,
	"SetSprTrance":        (*Machine).onSetSprTrance,
	"SetSprRect":          (*Machine).onSetSprRect,
	"SetSprReRect":        (*Machine).onSetSprReRect,
	"SetSprTileRect":      (*Machine).onSetSprTileRect,
	"ReRectDraw":          (*Machine).onReRectDraw,
	"GetSprPos":           (*Machine).onGetSprPos,
	"GetOnSpriteName":     (*Machine).onGetOnSpriteName,
	"SpriteSwitch":        (*Machine).onSpriteSwitch,
	"ReLoadSprite":        (*Machine).onReLoadSprite,
	"ClearSprite":         (*Machine).onClearSprite,
	"ClearScreen":         (*Machine).onClearScreen,
	"SwapScreen":          (*Machine).onSwapScreen,
	"SetEffectReady":      (*Machine).onSetEffectReady,
	"Effect":              (*Machine).onEffect,
	"SetSprAnm":           (*Machine).onSetSprAnm,
	"SetSprMtn":           (*Machine).onSetSprMtn,
	"SetSprMsg":           (*Machine).onSetSprMsg,
	"SetSprF3D":           (*Machine).onSetSprF3D,
	"SetMsgHit":           (*Machine).onSetMsgHit,
	"SetMsgPage":          (*Machine).onSetMsgPage,
	"SetSprText":          (*Machine).onSetSprText,
	"SetSprTextFont":      (*Machine).onSetSprTextFont,
	"SetSprTextFontFlag":  (*Machine).onSetSprTextFontFlag,
	"SetSprTextColor":     (*Machine).onSetSprTextColor,
	"SetSpriteMsgSelect":  (*Machine).onSetSpriteMsgSelect,
	"SprWriteText":        (*Machine).onSprWriteText,
	"SetEdit":             (*Machine).onSetEdit,
	"Unrag":               (*Machine).onUnrag,
	"SetCursol":           (*Machine).onSetCursol,
	"Movie":               (*Machine).onMovie,
	"SetMaskSelects":      (*Machine).onSetMaskSelects,
	"SetMaskSelectSprite": (*Machine).onSetMaskSelects,
	"Click":               (*Machine).onClick,
	"AppearMenu":          (*Machine).onAppearMenu,
	"ApearMenu":           (*Machine).onAppearMenu,
	"Sound":               (*Machine).onSound,
	"StopSound":           (*Machine).onStopSound,
	"SetSoundInit":        (*Machine).onSetSoundInit,
	"SetSoundChGr":        (*Machine).onSetSoundChGr,
	"SoundCh":             (*Machine).onSoundCh,
	"SoundGr":             (*Machine).onSoundGr,
	"PlayCh":              (*Machine).onPlayCh,
	"SetSndVolCh":         (*Machine).onSetSndVolCh,
	"SetSndVolGr":         (*Machine).onSetSndVolGr,
	"WaitSound":           (*Machine).onWaitSound,
	"Music":               (*Machine).onMusic,
	"StopMusic":           (*Machine).onStopMusic,
	"FadeOutMusic":        (*Machine).onFadeOutMusic,
	"GetPlayMusicInfo":    (*Machine).onGetPlayMusicInfo,
	"Run":                 (*Machine).onRun,
	"BackLog":             (*Machine).onBackLog,
	"SetBackLog":          (*Machine).onSetBackLog,
	"BackLogMessage":      (*Machine).onBackLogMessage,
	"MakeSave":            (*Machine).onMakeSave,
	"Save":                (*Machine).onSave,
	"Load":                (*Machine).onLoad,
	"SaveSystem":          (*Machine).onSaveSystem,
	"CheckSaveData":       (*Machine).onCheckSaveData,
	"ReSetSaveData":       (*Machine).onReSetSaveData,
	"MessageBox":          (*Machine).onMessageBox,
	"GetStringMessage":    (*Machine).onGetStringMessage,
	"GetYesNoMessage":     (*Machine).onGetYesNoMessage,
	"RunAction":           (*Machine).onRunAction,
	"DelRunAction":        (*Machine).onDelRunAction,
	"SetAction":           (*Machine).onSetAction,
	"DelAction":           (*Machine).onDelAction,
	"SetRunAction":        (*Machine).onSetRunAction,
	"ActionWait":          (*Machine).onActionWait,
	"ClearIntersept":      (*Machine).onClearIntersept,
	"ClearInterseptMacro": (*Machine).onClearIntersept,
}

func IsKnownCommand(name string) bool {
	if name == "" {
		return false
	}
	for key := range handlers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func (m *Machine) lookupHandler(name string) (func(*Machine, *script.EmonStatement) EmonStep, bool) {
	if handler, exists := handlers[name]; exists {
		return handler, true
	}
	for key, handler := range handlers {
		if strings.EqualFold(key, name) {
			return handler, true
		}
	}
	return nil, false
}

func (m *Machine) executeNext() EmonStep {
	m.pumpMotions()
	m.applyMessageIndicators(m.step)
	if m.pendingJump != "" {
		jump := m.pendingJump
		m.pendingJump = ""
		return m.doJump(jump)
	}
	if len(m.frames) == 0 {
		if m.finished {
			return EmonStepFinished
		}
		if !m.bootstrap() {
			return EmonStepFaulted
		}
	}
	frame := m.frames[len(m.frames)-1]
	if frame.Index >= int32(len(frame.Program.Statements)) {
		finishedProgram := frame.Program.Name
		finishedIndex := frame.Index
		m.popFileFrame()
		if len(m.frames) == 0 {
			m.finished = true
			m.logOnce(fmt.Sprintf("machine finished: bottom frame ran past end in '%s' at index %d", finishedProgram, finishedIndex))
			return EmonStepFinished
		}
		return EmonStepContinue
	}
	statement := frame.Program.Statements[frame.Index]
	frame.Index++
	if m.replayFrame != nil {
		contains := false
		for _, candidate := range m.frames {
			if candidate == m.replayFrame {
				contains = true
				break
			}
		}
		if !contains {
			m.finishReplay()
		} else if frame == m.replayFrame && frame.Index-1 >= int32(m.replayTarget) {
			m.finishReplay()
		}
	}
	if statement.IsText {
		return m.onTextStatement(statement)
	}
	if statement.Command != "" {
		if handler, exists := m.lookupHandler(statement.Command); exists {
			outcome := handler(m, statement)
			m.noteFinished(statement, frame, outcome)
			m.noteTextCommand(statement, frame)
			m.noteSpriteImage(statement)
			return outcome
		}
	}
	if statement.Command != "" {
		if macro, exists := m.context.Macros.TryGet(statement.Command); exists {
			if m.isReplaying() && isChoiceMacro(statement.Command) {
				return EmonStepContinue
			}
			return m.invokeMacro(statement, macro)
		}
	}
	if statement.Command != "" {
		m.logOnce(fmt.Sprintf("unknown command '%s' in '%s' line %d", statement.Command, frame.Program.Name, statement.Line))
	}
	return EmonStepContinue
}

func (m *Machine) bootstrap() bool {
	m.ensureMacrosRegistered()
	start := m.context.Config.StartScript()
	if strings.TrimSpace(start) == "" {
		core.Logf("emon: start script is empty")
		return false
	}
	program := m.context.Scripts.Load(start)
	if program == nil {
		core.Logf("emon: start script '%s' was not found", start)
		return false
	}
	m.pushFileFrame(program)
	return true
}

func (m *Machine) ensureMacrosRegistered() {
	for _, macro := range m.context.Config.MacroList() {
		if !m.context.Macros.Contains(macro.Name) {
			m.context.Macros.Add(macro.Name, macro)
		}
	}
}

func (m *Machine) invokeMacro(call *script.EmonStatement, macro script.EmonMacroDefinition) EmonStep {
	if len(m.frames) >= maxFrames {
		core.Logf("emon: macro '%s' exceeds max frames, ignored", macro.Name)
		return EmonStepContinue
	}
	program, exists := m.macroPrograms.Get(macro.Name)
	if !exists {
		program = script.Parse("macro:"+macro.Name, macro.Body)
		m.macroPrograms.Set(macro.Name, program)
	}
	m.pushMacroFrame(macro.Name, program, call)
	return EmonStepContinue
}

func (m *Machine) pushMacroFrame(name string, program *script.EmonProgram, call *script.EmonStatement) {
	if len(m.frames) >= maxFrames {
		core.Logf("emon: macro '%s' exceeds max frames, ignored", name)
		return
	}
	frame := script.NewEmonMacroFrame(name)
	if call != nil {
		for i := 0; i < maxMacroArguments; i++ {
			arg := call.Argument(int32(i))
			if arg.Raw == "" {
				continue
			}
			value := m.valueOf(arg.Raw, arg.WasQuoted)
			frame.Arguments.Set(macroArgumentName(i), value)
			frame.Arguments.Set(macroArgumentAlias(i), value)
		}
	}
	m.frames = append(m.frames, &EmonFrame{Program: program, Macro: frame})
}

func macroArgumentName(index int) string {
	switch index + 1 {
	case 1:
		return "引数１"
	case 2:
		return "引数２"
	case 3:
		return "引数３"
	case 4:
		return "引数４"
	case 5:
		return "引数５"
	case 6:
		return "引数６"
	case 7:
		return "引数７"
	case 8:
		return "引数８"
	case 9:
		return "引数９"
	default:
		return "引数１０"
	}
}

func macroArgumentAlias(index int) string {
	return "引数" + fmt.Sprintf("%d", index+1)
}

func tryMacroArgumentIndex(name string) (int, bool) {
	if !strings.HasPrefix(name, "引数") {
		return -1, false
	}
	switch name[len("引数"):] {
	case "１", "1":
		return 0, true
	case "２", "2":
		return 1, true
	case "３", "3":
		return 2, true
	case "４", "4":
		return 3, true
	case "５", "5":
		return 4, true
	case "６", "6":
		return 5, true
	case "７", "7":
		return 6, true
	case "８", "8":
		return 7, true
	case "９", "9":
		return 8, true
	case "１０", "10":
		return 9, true
	}
	return -1, false
}

func (m *Machine) getMacroProgram(macro script.EmonMacroDefinition) *script.EmonProgram {
	if program, exists := m.macroPrograms.Get(macro.Name); exists {
		return program
	}
	program := script.Parse("macro:"+macro.Name, macro.Body)
	m.macroPrograms.Set(macro.Name, program)
	return program
}

func isChoiceMacro(name string) bool {
	return strings.EqualFold(name, "Select")
}

func (m *Machine) doJump(target string) EmonStep {
	destination := strings.Trim(strings.TrimSpace(target), "\"")
	if destination == "" || strings.EqualFold(destination, "NULL") {
		return EmonStepContinue
	}
	if strutil.HasSuffixFold(destination, ".txt") {
		if len(m.frames) >= maxFrames {
			core.Logf("emon: jump to '%s' exceeds max frames, ignored", destination)
			return EmonStepContinue
		}
		fileProgram := m.context.Scripts.Load(destination)
		if fileProgram == nil {
			m.logOnce("jump target file '" + destination + "' was not found")
			return EmonStepContinue
		}
		m.logOnce("jump '" + target + "' -> '" + destination + "': script file '" + fileProgram.Name + "'")
		m.pushFileFrame(fileProgram)
		return EmonStepContinue
	}
	if len(m.frames) == 0 {
		return EmonStepContinue
	}
	frame := m.frames[len(m.frames)-1]
	index := frame.Program.FindLabel(destination)
	if index >= 0 {
		m.logOnce(fmt.Sprintf("jump '%s' -> '%s': label at statement %d in program '%s'", target, destination, index, frame.Program.Name))
		frame.Index = index
		return EmonStepContinue
	}
	if program := m.context.Scripts.Load(destination); program != nil {
		if len(m.frames) >= maxFrames {
			core.Logf("emon: jump to '%s' exceeds max frames, ignored", destination)
			return EmonStepContinue
		}
		m.logOnce("jump '" + target + "' -> '" + destination + "': script file '" + program.Name + "'")
		m.pushFileFrame(program)
		return EmonStepContinue
	}
	if frame.Macro != nil {
		m.logOnce("jump '" + target + "' -> '" + destination + "': macro return from '" + frame.Macro.Name + "'")
		m.frames = m.frames[:len(m.frames)-1]
		return EmonStepContinue
	}
	if m.unresolvedJumps.Add(destination) {
		core.Logf("emon: jump '%s' -> '%s': unresolved in program '%s'", target, destination, frame.Program.Name)
	}
	return EmonStepContinue
}

func (m *Machine) pushFileFrame(program *script.EmonProgram) {
	m.frames = append(m.frames, &EmonFrame{Program: program})
}

func (m *Machine) popFileFrame() {
	if len(m.frames) == 0 {
		return
	}
	m.frames = m.frames[:len(m.frames)-1]
}

func (m *Machine) onTextStatement(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	text := m.expandInline(statement.Text)
	if text != "" {
		m.context.Message.Append(text)
		m.context.Message.SetVisible(true)
		m.logFirstStoryText(m.CurrentProgram(), max(0, int(m.CurrentStatement())-1), text)
	}
	return EmonStepContinue
}

func (m *Machine) noteFinished(statement *script.EmonStatement, frame *EmonFrame, outcome EmonStep) {
	if outcome != EmonStepFinished {
		return
	}
	m.logOnce(fmt.Sprintf("machine finished: #%s in '%s' at index %d", statement.Command, frame.Program.Name, max(0, int(frame.Index)-1)))
}

func (m *Machine) noteTextCommand(statement *script.EmonStatement, frame *EmonFrame) {
	if m.storyLogged || m.isReplaying() {
		return
	}
	if !strings.EqualFold(statement.Command, "Text") || statement.ArgumentCount() == 0 {
		return
	}
	m.logFirstStoryText(frame.Program.Name, max(0, int(frame.Index)-1), m.text(statement, 0))
}

func (m *Machine) logFirstStoryText(program string, statementIndex int, text string) {
	if m.storyLogged || text == "" {
		return
	}
	m.storyLogged = true
	flat := replaceLineEndings(text, " ")
	core.Logf("emon: first story text in '%s' at index %d: %s", program, statementIndex, shorten(flat))
}

func (m *Machine) noteSpriteImage(statement *script.EmonStatement) {
	command := statement.Command
	isSet := strings.EqualFold(command, "SetSprite")
	isReload := strings.EqualFold(command, "ReLoadSprite")
	if !isSet && !isReload {
		return
	}
	name := m.raw(statement, 0)
	if name == "" || isMainSprite(name) {
		return
	}
	if isReload {
		var file string
		if statement.ArgumentCount() > 1 {
			file = m.text(statement, 1)
		}
		if file == "" || strings.EqualFold(file, "NULL") || m.context.Stage.Find(file) == nil {
			return
		}
	}
	sprite := m.context.Stage.Find(name)
	if sprite == nil || sprite.IsGroup() {
		return
	}
	if sprite.Image() != nil || sprite.ResolveImage() != nil {
		return
	}
	link := sprite.ImageSource()
	if link == nil {
		return
	}
	key := name + "<-" + link.Name()
	if !m.unresolvedLinks.Add(key) {
		verb := "set"
		if isReload {
			verb = "reload"
		}
		m.logOnce(verb + " sprite '" + name + "' source '" + link.Name() + "' is still unresolved")
	}
}

func (m *Machine) expandInline(text string) string {
	if text == "" || !strings.Contains(text, "#") {
		return text
	}
	result := text
	offset := 0
	steps := 0
	for {
		hash := indexOfToken(result[offset:], "#String(")
		if hash < 0 {
			break
		}
		hash += offset
		close := strings.Index(result[hash+8:], ")")
		if close < 0 {
			break
		}
		close += hash + 8
		name := strings.TrimSpace(result[hash+8 : close])
		steps++
		if steps > maxInlineExpansions {
			core.Logf("emon: inline #String stopped after %d expansions at '%s' in '%s'", steps, name, shorten(text))
			break
		}
		value := m.Lookup(name).AsString()
		result = result[:hash] + value + result[close+1:]
		offset = hash + len(value)
	}
	offset = 0
	steps = 0
	for {
		rel := indexOfToken(result[offset:], "#Return")
		if rel < 0 {
			break
		}
		hash := rel + offset
		steps++
		if steps > maxInlineExpansions {
			core.Logf("emon: inline #Return stopped after %d expansions at offset %d in '%s'", steps, hash, shorten(text))
			break
		}
		rest := hash + 7
		for rest < len(result) && isWhiteSpaceByte(result[rest]) {
			rest++
		}
		if rest < len(result) && (result[rest] == '(' || strings.HasPrefix(result[rest:], "（")) {
			fullwidth := result[rest] != '('
			openLen := 1
			closeMark := ")"
			if fullwidth {
				openLen = len("（")
				closeMark = "）"
			}
			index := strings.Index(result[rest+openLen:], closeMark)
			if index < 0 {
				break
			}
			close := rest + openLen + index
			inner := result[rest+openLen : close]
			result = result[:hash] + inner + result[close+len(closeMark):]
			offset = hash + len(inner)
		} else if rest < len(result) && strings.HasPrefix(result[rest:], "『") {
			index := strings.Index(result[rest+1:], "』")
			if index < 0 {
				break
			}
			close := rest + 1 + index
			inner := result[rest : close+len("』")]
			result = result[:hash] + inner + result[close+len("』"):]
			offset = hash + len(inner)
		} else {
			offset = rest
		}
	}
	return result
}

func isWhiteSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\v' || b == '\f'
}

func (m *Machine) EvalIntegers(statement *script.EmonStatement, start int32) []int {
	result := []int{}
	for i := start; i < statement.ArgumentCount(); i++ {
		arg := statement.Argument(i)
		if arg.Raw == "" {
			result = append(result, 0)
			continue
		}
		if !arg.WasQuoted {
			expanded := m.LookupArgument(arg.Raw).AsString()
			if strings.Contains(expanded, ",") {
				for _, part := range strings.Split(expanded, ",") {
					result = append(result, m.parseIntPart(part))
				}
				continue
			}
		}
		result = append(result, int(m.valueOf(arg.Raw, arg.WasQuoted).AsInt()))
	}
	return result
}

func (m *Machine) EvalInt(statement *script.EmonStatement, index int32, fallback int) int {
	arg := statement.Argument(index)
	if arg.Raw == "" {
		return fallback
	}
	if !arg.WasQuoted {
		expanded := m.LookupArgument(arg.Raw).AsString()
		if strings.Contains(expanded, ",") {
			return m.parseIntPart(strings.Split(expanded, ",")[0])
		}
	}
	return int(m.valueOf(arg.Raw, arg.WasQuoted).AsInt())
}

func (m *Machine) parseIntPart(text string) int {
	part := strings.TrimSpace(text)
	if part == "" {
		return 0
	}
	if direct, ok := parseIntInvariant(part); ok {
		return int(direct)
	}
	value := script.Evaluate(part, m.resolver)
	return int(value.AsInt())
}

func (m *Machine) ExpandToken(statement *script.EmonStatement, index int32) string {
	arg := statement.Argument(index)
	if arg.Raw == "" {
		return ""
	}
	if arg.WasQuoted {
		return arg.Raw
	}
	return m.Lookup(arg.Raw).AsString()
}

func (m *Machine) ResolveSpriteImage(name string) *bishopgfx.BsgImage {
	if image := m.LoadImage(name); image != nil {
		return image
	}
	if sprite := m.context.Stage.Find(name); sprite != nil {
		if resolved := sprite.ResolveImage(); resolved != nil {
			return resolved
		}
	}
	return m.SourceImage(name)
}

func (m *Machine) ApplyRect(sprite *stage.EmonSprite, rect EmonRect) {
	source := sprite.ResolveImage()
	if source == nil {
		source = m.SourceImage(sprite.Name())
	}
	if source == nil {
		width := max(0, rect.X2-rect.X1)
		height := max(0, rect.Y2-rect.Y1)
		if width == 0 || height == 0 {
			sprite.SetSourceRect(rect.X1, rect.Y1, 0, 0)
			return
		}
		sprite.ClearSourceRect()
		sprite.SetSize(width, height)
		return
	}
	left := core.Clamp(rect.X1, 0, source.Width)
	top := core.Clamp(rect.Y1, 0, source.Height)
	right := core.Clamp(rect.X2, 0, source.Width)
	bottom := core.Clamp(rect.Y2, 0, source.Height)
	columns := right - left
	rows := bottom - top
	if columns <= 0 || rows <= 0 {
		sprite.SetImage(nil)
		sprite.SetSourceRect(left, top, 0, 0)
		return
	}
	sprite.SetSourceRect(left, top, columns, rows)
}

func IsDefaultRectToken(token string) bool {
	return strings.EqualFold(token, "Default")
}

func IsMessageSizeRectToken(token string) bool {
	return strings.EqualFold(token, "MSGSIZE")
}

func RectEdge(value, origin int) int {
	if value < 0 {
		return origin - value
	}
	return value
}

func OrderRect(x1, y1, x2, y2 int) EmonRect {
	return EmonRect{X1: min(x1, x2), Y1: min(y1, y2), X2: max(x1, x2), Y2: max(y1, y2)}
}

func ResolveRect(values []int, source *bishopgfx.BsgImage) EmonRect {
	width := 0
	height := 0
	if source != nil {
		width = source.Width
		height = source.Height
	}
	if len(values) == 0 {
		return EmonRect{X1: 0, Y1: 0, X2: width, Y2: height}
	}
	if len(values) == 1 {
		return OrderRect(0, 0, RectEdge(values[0], 0), height)
	}
	if len(values) == 2 {
		return OrderRect(0, 0, RectEdge(values[0], 0), RectEdge(values[1], 0))
	}
	if len(values) == 3 {
		return OrderRect(values[0], values[1], RectEdge(values[2], values[0]), height)
	}
	return OrderRect(values[0], values[1], RectEdge(values[2], values[0]), RectEdge(values[3], values[1]))
}

func (m *Machine) ResetToFullImage(sprite *stage.EmonSprite) {
	source := sprite.ResolveImage()
	if source == nil {
		source = m.SourceImage(sprite.Name())
	}
	if source == nil {
		return
	}
	if sprite.ImageSource() != nil {
		sprite.ClearSourceRect()
		sprite.SetSize(source.Width, source.Height)
		m.spriteRects.Delete(sprite.Name())
		return
	}
	sprite.SetImage(source)
	sprite.ClearSourceRect()
	sprite.SetSize(source.Width, source.Height)
	m.spriteRects.Delete(sprite.Name())
}

func (m *Machine) isChainVisible(sprite *stage.EmonSprite) bool {
	for current := sprite.Parent(); current != nil; current = current.Parent() {
		if !current.Visible() {
			return false
		}
	}
	return sprite.Visible()
}

func shorten(text string) string {
	runes := []rune(text)
	if len(runes) > 80 {
		return string(runes[:80]) + "…"
	}
	return text
}

func (m *Machine) logOnce(message string) {
	if m.loggedNames.Add(message) {
		core.Logf("emon: %s", message)
	}
}
