package tj2

import (
	"strings"

	"kizuna/engine/brand/bishop/state"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
)

var tj2ClassicBuiltinWipes = []int{0, 1, 5, 9, 11, 13, 99}

func (s *Tj2Session) Dispatch(command vmtj2.Tj2Command) state.SessionStep {
	switch command.Kind {
	case vmtj2.Tj2CommandKindBackground:
		return s.applyBackground(command)
	case vmtj2.Tj2CommandKindStandee:
		return s.applyStandee(command)
	case vmtj2.Tj2CommandKindMessage:
		return s.applyMessage(command)
	case vmtj2.Tj2CommandKindChapter:
		return s.applyChapter(command)
	case vmtj2.Tj2CommandKindFunction:
		return HandleTj2System(s, command)
	case vmtj2.Tj2CommandKindPlayBgm:
		return s.applyBgm(command)
	case vmtj2.Tj2CommandKindPlaySound:
		return s.applySound(command)
	case vmtj2.Tj2CommandKindPlayVoice:
		return s.applyVoice(command)
	case vmtj2.Tj2CommandKindLoadScenario:
		return state.SessionContinue
	case vmtj2.Tj2CommandKindWait:
		return s.applyWait(command)
	case vmtj2.Tj2CommandKindWipe:
		return s.applyWipe(command)
	case vmtj2.Tj2CommandKindClearFlags:
		return s.applyClearFlags()
	case vmtj2.Tj2CommandKindName:
		return s.applyName(command)
	case vmtj2.Tj2CommandKindLoadDialog:
		// 只有旧脚本（标题"続きから"、H 场景的读档）会用 !r。
		// 现代 TJ2 没有这条路径，保持原来的"立即继续"，避免多停一次。
		if s.IsToku2UI() {
			return s.quickLoad()
		}
		if s.UsesEngineUI() {
			return s.openClassicLoad(true)
		}
		return state.SessionContinue
	case vmtj2.Tj2CommandKindSaveDialog:
		if s.IsToku2UI() {
			return s.quickSave()
		}
		if s.UsesEngineUI() {
			return s.openClassicSave(true)
		}
		return state.SessionContinue
	case vmtj2.Tj2CommandKindHalt:
		return state.SessionFinished
	case vmtj2.Tj2CommandKindFaulted:
		return s.applyFault(command)
	default:
		return state.SessionContinue
	}
}

func (s *Tj2Session) applyBackground(command vmtj2.Tj2Command) state.SessionStep {
	if s.table == nil {
		return state.SessionContinue
	}
	if command.Index < 0 || command.Index >= len(s.table.GraphicNames()) {
		core.Logf("tj2: background index %d out of range", command.Index)
		return state.SessionContinue
	}
	name := s.resolveGraphicName(command)
	if len(name) == 0 {
		core.Logf("tj2: background index %d has empty name", command.Index)
		return state.SessionContinue
	}
	s.stage.ClearStandees()
	s.standees.Clear()
	image := s.images.Get(name)
	if image == nil {
		core.Logf("tj2: background '%s' missing, keeping current", name)
		return state.SessionContinue
	}
	s.stage.SetBackground(name, image, command.OffsetX, command.OffsetY)
	s.markGalleryByName(name)
	s.menuVisible = false
	core.Logf("tj2: background '%s' %dx%d", name, image.Width, image.Height)
	return state.SessionContinue
}

func (s *Tj2Session) resolveGraphicName(command vmtj2.Tj2Command) string {
	text := strings.TrimSpace(command.Text)
	if len(text) > 0 {
		return text
	}
	if s.table == nil || command.Index < 0 || command.Index >= len(s.table.GraphicNames()) {
		return ""
	}
	return classicGraphicFileName(s.table.GraphicName(command.Index))
}

func classicGraphicFileName(tableName string) string {
	stem := strings.TrimSpace(tableName)
	if len(stem) == 0 {
		return ""
	}
	return strings.ToUpper(stem + ".KG")
}

func (s *Tj2Session) applyStandee(command vmtj2.Tj2Command) state.SessionStep {
	if s.IsClassic() {
		return s.applyClassicStandee(command)
	}
	if command.Flag == 0 {
		if s.standees.Unregister(command.Index) {
			s.rebuildStandees()
		}
		core.Logf("tj2: standee hide %d", command.Index)
		return state.SessionContinue
	}
	if command.Flag < Tj2StandeeMinLayer || command.Flag > Tj2StandeeMaxLayer {
		core.Logf("tj2: standee layer %d ignored", command.Flag)
		return state.SessionContinue
	}
	name := strings.TrimSpace(command.Text)
	if len(name) == 0 {
		if s.standees.Hide(command.Flag) {
			s.rebuildStandees()
		}
		core.Logf("tj2: standee layer %d cleared", command.Flag)
		return state.SessionContinue
	}
	if !s.standees.Register(command.Flag, command.Index, name, command.OffsetX, command.OffsetY) {
		core.Logf("tj2: standee registry full, ignoring layer %d", command.Flag)
		return state.SessionContinue
	}
	s.rebuildStandees()
	core.Logf("tj2: standee layer %d show %d '%s'", command.Flag, command.Index, name)
	return state.SessionContinue
}

func (s *Tj2Session) applyClassicStandee(command vmtj2.Tj2Command) state.SessionStep {
	if command.Flag == 0 {
		if s.standees.Unregister(command.Index) {
			s.rebuildStandees()
		}
		core.Logf("tj2: standee hide %d", command.Index)
		return state.SessionContinue
	}
	name := s.resolveGraphicName(command)
	if len(name) == 0 {
		core.Logf("tj2: standee index %d has empty name", command.Index)
		return state.SessionContinue
	}
	slot, ok := s.standees.TryAllocateSlot(s.profile.StandeeSlotCount)
	if !ok {
		core.Logf("tj2: standee registry full, ignoring show")
		return state.SessionContinue
	}
	if !s.standees.Register(slot, command.Index, name, command.OffsetX, command.OffsetY) {
		core.Logf("tj2: standee slot %d busy, ignoring show", slot)
		return state.SessionContinue
	}
	s.rebuildStandees()
	core.Logf("tj2: standee slot %d show %d '%s'", slot, command.Index, name)
	return state.SessionContinue
}

func (s *Tj2Session) rebuildStandees() {
	s.stage.ClearStandees()
	for _, entry := range s.standees.Entries() {
		image := s.images.Get(entry.Name)
		if image == nil {
			core.Logf("tj2: standee '%s' missing", entry.Name)
			continue
		}
		s.stage.SetStandee(entry.Layer, image, entry.OffsetX, entry.OffsetY)
	}
}

func (s *Tj2Session) applyMessage(command vmtj2.Tj2Command) state.SessionStep {
	s.gameState.SetSpeaker("")
	if s.UsesEngineUI() {
		// 旧脚本大量使用只有 _r 的 !s 作为"清空文字/过场"，不是真的要
		// 等玩家点一下。若把空消息也当成 AwaitInput，启动 LOGO 之后的
		// 标题会先出现一个空白信息框，必须点一下才看到菜单。
		plain := CleanTj2Text(command.Text)
		if strings.TrimSpace(plain) == "" {
			s.gameState.SetText("")
			s.gameState.SetMessageWindowVisible(false)
			s.messagePages = nil
			s.messagePage = 0
			s.stage.Invalidate()
			return state.SessionContinue
		}
	}
	if s.UsesEngineUI() {
		// _t 是"等玩家点击后继续"的分页符，不是普通换行。按原版拆成多页，
		// 由 Advance 逐页推进；这样长对白不会一次堆满/溢出信息框。
		limit := tj2ClassicLineRunes
		if s.IsToku2UI() {
			// textrender 内部的硬上限是 25 字，这里给 25 才不会在
			// 引擎分页和最终绘制之间出现第二次换行。
			limit = 25
		}
		pages := splitTj2MessagePagesWithLimit(command.Text, limit)
		if len(pages) == 0 {
			pages = []string{CleanTj2Text(command.Text)}
		}
		s.messagePages = pages
		s.messagePage = 0
		s.messageHover = 0
		s.gameState.SetText(pages[0])
		s.appendMessageHistory(pages[0])
	} else {
		s.gameState.SetText(CleanTj2Text(command.Text))
	}
	s.gameState.SetMessageWindowVisible(true)
	s.gameState.SetAwaitingInput(true)
	s.stage.Invalidate()
	return state.SessionAwaitInput
}

func (s *Tj2Session) applyChapter(command vmtj2.Tj2Command) state.SessionStep {
	core.Logf("tj2: chapter index=%d text='%s'", command.Index, command.Text)
	return state.SessionContinue
}

func (s *Tj2Session) applyBgm(command vmtj2.Tj2Command) state.SessionStep {
	if !s.bgmOn {
		s.gameState.ClearBgm()
		return state.SessionContinue
	}
	if command.Index < 0 {
		s.bgmTrack = -1
		s.gameState.ClearBgm()
		core.Logf("tj2: bgm stop")
		return state.SessionContinue
	}
	s.bgmTrack = command.Index
	path := s.bgm.Resolve(command.Index)
	if path == "" {
		s.gameState.ClearBgm()
		core.Logf("tj2: bgm track %d no audio file", command.Index)
	} else {
		s.gameState.SetBgm(path)
		core.Logf("tj2: bgm track %d '%s'", command.Index, path)
	}
	s.gameState.SetBgmLoop(true)
	return state.SessionContinue
}

func (s *Tj2Session) applySound(command vmtj2.Tj2Command) state.SessionStep {
	if !s.seOn {
		return state.SessionContinue
	}
	file := s.soundFileName(command)
	if len(file) == 0 {
		return state.SessionContinue
	}
	if !s.resources.Exists(file) {
		core.Logf("tj2: se '%s' missing, skipped", file)
		return state.SessionContinue
	}
	s.gameState.SetSoundEffect(file)
	s.gameState.SetEffectSerial(s.gameState.EffectSerial() + 1)
	core.Logf("tj2: se '%s'", file)
	return state.SessionContinue
}

func (s *Tj2Session) applyVoice(command vmtj2.Tj2Command) state.SessionStep {
	if !s.voiceOn {
		return state.SessionContinue
	}
	file := s.soundFileName(command)
	if len(file) == 0 {
		return state.SessionContinue
	}
	if !s.resources.Exists(file) {
		core.Logf("tj2: voice '%s' missing, skipped", file)
		return state.SessionContinue
	}
	s.gameState.SetVoice(file)
	s.gameState.SetVoiceSerial(s.gameState.VoiceSerial() + 1)
	core.Logf("tj2: voice '%s'", file)
	return state.SessionContinue
}

func (s *Tj2Session) soundFileName(command vmtj2.Tj2Command) string {
	name := strings.TrimSpace(command.Text)
	if len(name) > 0 {
		return name
	}
	if s.table != nil && command.Index >= 0 && command.Index < len(s.table.SoundNames()) {
		return s.table.SoundName(command.Index) + ".ADP"
	}
	return ""
}

func (s *Tj2Session) applyWait(command vmtj2.Tj2Command) state.SessionStep {
	if s.machine == nil {
		return state.SessionContinue
	}
	s.gameState.SetWaitMilliseconds(s.machine.PendingWaitMilliseconds())
	s.machine.ClearPendingWait()
	return state.SessionWait
}

func (s *Tj2Session) applyWipe(command vmtj2.Tj2Command) state.SessionStep {
	s.RecordWipe(command.Value)
	if s.profile.MissingWipeFallsBackToInstant && !containsTj2Int(tj2ClassicBuiltinWipes, command.Value) && !s.resources.Exists("WIPE.PFT") {
		core.Logf("tj2: wipe %d missing archive, using instant", command.Value)
		return state.SessionContinue
	}
	core.Logf("tj2: wipe %d", command.Value)
	return state.SessionContinue
}

func (s *Tj2Session) applyClearFlags() state.SessionStep {
	if s.machine == nil {
		return state.SessionContinue
	}
	clear(s.machine.Flags())
	core.Logf("tj2: flags cleared")
	return state.SessionContinue
}

func (s *Tj2Session) applyName(command vmtj2.Tj2Command) state.SessionStep {
	core.Logf("tj2: name index=%d text='%s'", command.Index, command.Text)
	return state.SessionContinue
}

func (s *Tj2Session) applyFault(command vmtj2.Tj2Command) state.SessionStep {
	core.Logf("tj2: script fault: %s", command.Text)
	return state.SessionFaulted
}

func (s *Tj2Session) PublishChoices(options []vmtj2.Tj2Choice) state.SessionStep {
	if s.machine == nil {
		return state.SessionContinue
	}
	texts := make([]string, 0, len(options))
	for _, choice := range options {
		texts = append(texts, CleanTj2Text(choice.Text))
	}
	s.gameState.SetChoices(texts)
	if s.IsClassic() && s.classicMap.Active() {
		if len(texts) > Tj2ClassicRowCount {
			s.classicMap.SetActive(false)
		} else {
			core.Logf("tj2: classic map select choices=%d tick=%d", len(texts), s.tickCounter)
			return state.SessionAwaitSelect
		}
	}
	return state.SessionAwaitChoice
}

func containsTj2Int(values []int, value int) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
