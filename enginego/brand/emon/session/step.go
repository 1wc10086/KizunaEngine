package session

import (
	"reflect"
	"time"

	"kizuna/enginego/brand/emon/stage"
	emontext "kizuna/enginego/brand/emon/text"
	"kizuna/enginego/brand/emon/vm"
	"kizuna/enginego/core"
)

func (s *Session) Run(budget uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil || s.ctx == nil {
		return core.StepFaulted, nil
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	steps := int(budget)
	if steps <= 0 {
		steps = commandBudget
	}
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.machine.Run(steps)
	})
	return s.publish(), nil
}

func (s *Session) Advance() (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil || s.ctx == nil {
		return core.StepFaulted, nil
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	if s.machine.MessageLocked() && s.step == vm.EmonStepAwaitInput {
		s.pump()
		return s.publish(), nil
	}
	s.awaitingInput = false
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.machine.Advance()
	})
	return s.publish(), nil
}

func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil || s.ctx == nil {
		return core.StepFaulted, nil
	}
	if s.step != vm.EmonStepAwaitChoice && s.step != vm.EmonStepAwaitAction {
		return core.StepContinue, nil
	}
	if s.machine.SelectLocked() {
		return s.mapped, nil
	}
	position := s.resolveChoicePosition(int(index))
	if position < 0 {
		core.Logf("emon: select index %d rejected", index)
		return s.mapped, nil
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	s.awaitingInput = false
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.selectPosition(position)
	})
	return s.publish(), nil
}

func (s *Session) Pointer(op uint32, x int32, y int32) (core.Step, error) {
	if s.machine == nil || s.ctx == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	switch op {
	case 5:
		return core.StepContinue, nil
	case 0:
		if !s.gate.TryLock() {
			return s.mapped, nil
		}
		s.machine.SetPointer(int(x), int(y))
		s.invalidateFrame()
		mapped := s.mapped
		s.gate.Unlock()
		return mapped, nil
	case 1:
		if s.autoMenu {
			return s.mapped, nil
		}
		return s.dispatchInput(func() vm.EmonStep {
			if id, hit := s.hitPromptButton(int(x), int(y)); hit {
				return s.resolvePromptButton(id)
			}
			// 触屏没有真正的指针悬停：按下/点按前先按“移动到该点”派发一次
			// SPRMOUSEOVER，还原 PC 上“先悬停、再点击”的顺序。BOKUONE 的
			// 对话框工具条就是靠底部热区的 SPRMOUSEOVER 滑入的；缺了这一步，
			// 触屏玩家永远看不到 Q.SAVE/SAVE/LOAD 这些按钮。
			s.machine.SetPointer(int(x), int(y))
			s.machine.PointerPress(int(x), int(y))
			return s.machine.Step()
		})
	case 2:
		if s.autoMenu {
			return s.mapped, nil
		}
		return s.dispatchInput(func() vm.EmonStep {
			s.machine.PointerRelease(int(x), int(y))
			return s.machine.Step()
		})
	case 3:
		if s.autoMenu {
			s.handleAutoMenuClick(int(x), int(y))
			s.invalidateFrame()
			return s.mapped, nil
		}
		// 读档页优先：槽位点击直接 Load。真机日志里脚本确认框会被宿主
		// 自动以“否/空”结束，导致读档永远回到页面；直接读档绕开这个
		// 不可靠的宿主交互，存档页仍走原脚本（需要注释输入）。
		if sprite := s.machine.HitSelect(int(x), int(y)); sprite != nil {
			if s.loadSlotDirect(sprite.Name()) {
				return s.publish(), nil
			}
		}
		hitName := ""
		_, promptBefore := s.machine.PendingPrompt()
		step, err := s.dispatchInput(func() vm.EmonStep {
			if id, hit := s.hitPromptButton(int(x), int(y)); hit {
				return s.resolvePromptButton(id)
			}
			if sprite := s.machine.HitSelect(int(x), int(y)); sprite != nil {
				hitName = sprite.Name()
			}
			// 同 press：触屏一次点按要合并“悬停 + 点击”两步。引擎收到 hover
			// 后若压入宏（如工具条滑入），交给下一次 Run 执行；Tap 仍然处理
			// 本次点击本身，两者互不吞掉。
			s.machine.SetPointer(int(x), int(y))
			return s.machine.Tap(int(x), int(y), false)
		})
		if err == nil && hitName != "" {
			_, hasAfter := s.machine.PendingPrompt()
			if !hasAfter && !promptBefore {
				// 点的是读档槽位却没有弹确认框：按槽位编号直接读，避免设备上
				// 的点击差异让整个读档流程静默失败。
				if s.tryLoadSlotFallback(hitName) {
					return s.publish(), nil
				}
			}
		}
		return step, err
	case 4:
		return s.dispatchInput(func() vm.EmonStep {
			s.machine.RightClick()
			return s.machine.Step()
		})
	case 6:
		// 触屏拖动（宿主 KzPointer.drag）。设置界面的音量滑杆用 SPRMOVE/
		// SPREREA/SPRDRAG 触发器更新旋钮位置，之前 emon 忽略 op 6，滑杆完全
		// 拖不动。PointerMove 会先派发指针/区域触发器；只有按住状态才额外
		// 派发 SPRDRAG，避免普通滑动误触。
		return s.dispatchInput(func() vm.EmonStep {
			s.machine.PointerDrag(int(x), int(y))
			return s.machine.Step()
		})
	default:
		return s.mapped, nil
	}
}

func (s *Session) PressKey(code uint32) (core.Step, error) {
	if s.machine == nil || s.ctx == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	return s.dispatchInput(func() vm.EmonStep {
		s.machine.PressKey(int(code))
		return s.machine.Step()
	})
}

func (s *Session) Wheel(delta int32) (core.Step, error) {
	if s.machine == nil || s.ctx == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	return s.dispatchInput(func() vm.EmonStep {
		s.machine.Wheel(int(delta))
		return s.machine.Step()
	})
}

// resolvePromptButton 把对话框按钮 id 落为脚本提交语义：
//   - yes/no → machine.SubmitYesNo（1/0 整数落进変数５，覆盖确认的 ifJump 能正确分支）
//   - confirm/cancel → machine.SubmitText（确认用 promptEdit，取消提交空串走缺省名字/评论保留）
//
// 所有分支都不清 promptEdit：提交后 prompt 消失，下一条 GetStringMessage 仍从空输入框开始，
// 这与原版每次弹一次新输入窗的表现一致。
func (s *Session) resolvePromptButton(id string) vm.EmonStep {
	if s.machine == nil {
		return s.step
	}
	yesNo := s.machine.PendingPromptIsYesNo()
	switch id {
	case core.PromptButtonYes:
		if !yesNo {
			return s.step
		}
		s.machine.SubmitYesNo(true)
		s.promptEdit = ""
		return s.guard(func() vm.EmonStep { return s.machine.Run(commandBudget) })
	case core.PromptButtonNo:
		if !yesNo {
			return s.step
		}
		s.machine.SubmitYesNo(false)
		s.promptEdit = ""
		return s.guard(func() vm.EmonStep { return s.machine.Run(commandBudget) })
	case core.PromptButtonConfirm:
		if yesNo {
			return s.step
		}
		edit := s.promptEdit
		s.machine.SubmitText(edit)
		s.promptEdit = ""
		return s.guard(func() vm.EmonStep { return s.machine.Run(commandBudget) })
	case core.PromptButtonCancel:
		if yesNo {
			return s.step
		}
		s.machine.SubmitText("")
		s.promptEdit = ""
		return s.guard(func() vm.EmonStep { return s.machine.Run(commandBudget) })
	default:
		return s.step
	}
}

func (s *Session) dispatchInput(input func() vm.EmonStep) (core.Step, error) {
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	backLog := s.machine.BackLogActive()
	var next vm.EmonStep
	faulted := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				core.Logf("emon: input failed: %v", recovered)
				faulted = true
			}
		}()
		next = input()
	}()
	if faulted {
		return core.StepFaulted, nil
	}
	if backLog && !s.machine.BackLogActive() {
		s.awaitingInput = false
		s.step = s.guard(func() vm.EmonStep {
			return s.machine.Advance()
		})
	} else {
		s.step = next
	}
	return s.publish(), nil
}

func (s *Session) guard(op func() vm.EmonStep) (step vm.EmonStep) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("emon: run failed: %v", recovered)
			s.record("run failed")
			step = vm.EmonStepFaulted
		}
	}()
	return op()
}

func (s *Session) pump() {
	if s.ctx == nil || s.message == nil || s.stage == nil || s.composer == nil || s.audio == nil || s.machine == nil {
		return
	}
	delta := s.takeDelta()
	s.ctx.FrameDelta = delta
	s.composer.SetFrameDelta(delta)
	s.audio.Update(delta)
	s.advanceMessageText(delta)
	s.syncAudioSettings()
	s.pollAutoMenu()
	s.machine.PumpFrame()
	s.stage.Advance(delta)
}

// advanceMessageText 按原引擎 MessageSpeed 语义推进逐字显示。
// asenure.exe 的 sub_4328A0：0 = 立刻显示全句；1 = 每帧 1 字（60fps 下 60cps）；
// n>1 = 每 n 帧 1 字；n<0 = 每帧 -n 字。旧实现固定 30cps，设置里的
// テキスト速度（速い=0 / 普通=1 / 遅い=4）完全无效。
func (s *Session) advanceMessageText(delta float64) {
	speed := s.machine.SettingInt("MessageSpeed", 1)
	switch {
	case speed == 0:
		s.message.RevealAll()
	case speed > 1:
		s.message.Advance(delta, messageCharsPerSec/speed)
	case speed < 0:
		s.message.Advance(delta, messageCharsPerSec*(-speed))
	default:
		s.message.Advance(delta, messageCharsPerSec)
	}
}

func (s *Session) publish() core.Step {
	s.composeFrame()
	s.mapped = mapEmonStep(s.step, s.machine.BackLogActive(), s.hasSelectLocked())
	return s.mapped
}

func (s *Session) refresh() {
	if s.message == nil {
		return
	}
	s.speaker = s.message.Speaker()
	body := s.message.Body()
	if body == "" {
		s.text = ""
	} else {
		s.text = s.message.RevealedText()
	}
	s.awaitingInput = s.step == vm.EmonStepAwaitInput || s.step == vm.EmonStepAwaitAction || s.step == vm.EmonStepAwaitName
	s.publishSelect()
}

func (s *Session) composeFrame() {
	if s.ctx == nil || s.stage == nil || s.composer == nil || s.frame == nil {
		return
	}
	s.refresh()
	signature := s.pictureSignature()
	if s.composer.Effect() == nil && signature == s.presentSig {
		return
	}
	s.presentSig = signature
	s.composer.SetBackLogActive(s.machine.BackLogActive())
	s.composer.Compose(s.stage, s.frame)
	s.drawMessageText()
	// prompt 对话框最外层：遮罩 + 模态窗盖住对白层，避免存档/读档页的确认窗下还有对话文字。
	s.drawPromptDialog()
	s.drawAutoMenu()
	s.frameSerial++
}

// drawMessageText 把对白文字画进引擎帧。ASENURE 的 メッセージウィンドウ/
// メッセージボックス 是脚本创建的半透明窗口精灵，文字必须由引擎按配置的
// 文章文字位置/字号叠在同一张画布上；宿主再叠 Flutter 的 MessageBox 会
// 出现双层框、字体和位置都与原版不一致。窗口精灵被脚本隐藏时（全屏 CG 等）
// 也不画文字，避免文字浮在画面上。
func (s *Session) drawMessageText() {
	if s.ctx == nil || s.composer == nil || s.frame == nil || s.message == nil {
		return
	}
	if !s.message.Visible() {
		return
	}
	originX, originY := 0, 0
	if window := s.stage.Find("メッセージウィンドウ"); window != nil {
		if !isChainVisible(window) {
			return
		}
		originX, originY = window.AbsoluteX(), window.AbsoluteY()
	}
	glyphs := emontext.CurrentGlyphSource()
	if glyphs == nil {
		glyphs = emontext.ResolveGlyphSource()
	}
	emontext.DrawMessageBoxColorsAt(s.frame, s.message, s.composer.TextStyle(), glyphs,
		s.message.RevealCount(), originX, originY, s.messageColors())
}

// messageColors 读取脚本通过 SetSprTextColor 写在 メッセージボックス / 名前
// 精灵上的正文颜色与 SHADOW 标志。
//
// 原引擎把台词颜色挂在文本对象上，BOKUONE 的 文章表示 宏会按说话人先
// SetSprTextColor(メッセージボックス,キャラＮ基本文字色) 再显示正文；
// 只用固定白色会把粉/蓝/青三色角色配色全部丢掉。名字精灵单独设色，
// 因此分开取；SHADOW 也来自同一份 文章文字フラグ。
func (s *Session) messageColors() emontext.MessageColors {
	colors := emontext.MessageColors{Body: 0xFFFFFF, Speaker: 0xFFFFFF}
	if s.stage == nil {
		return colors
	}
	if box := s.stage.Find("メッセージボックス"); box != nil {
		if setting := box.TextSetting(); setting != nil {
			colors.Body = setting.Color()
			colors.Shadow = setting.ShadowColor()
			colors.ShadowEnabled = setting.Flags()&emontext.EmonSpriteShadow != 0
		}
	}
	if name := s.stage.Find("名前"); name != nil {
		if setting := name.TextSetting(); setting != nil {
			colors.Speaker = setting.Color()
			if !colors.ShadowEnabled && setting.Flags()&emontext.EmonSpriteShadow != 0 {
				colors.Shadow = setting.ShadowColor()
				colors.ShadowEnabled = true
			}
		}
	}
	return colors
}

func (s *Session) invalidateFrame() {
	s.presentSig = -1
}

func (s *Session) pictureSignature() int {
	hash := 17
	hash = hash*31 + s.stageSignatureLocked()
	hash = hash*31 + int(s.step)
	// prompt 对话框可变：提示文字本身不变，但 Edit 变化（IME 同步）必须触发重合成。
	// Edit 不走 stage/message 签名，需要单独打进去，否则宿主改字后画面不刷新。
	if s.machine != nil {
		if _, ok := s.machine.PendingPrompt(); ok {
			hash = hash*31 + 1
			hash = hash*31 + len(s.promptEdit)
			for _, r := range s.promptEdit {
				hash = hash*31 + int(r)
			}
		}
	}
	if s.selectShown {
		hash = hash*31 + 1
	}
	// オートモード設定面板不进 stage/变量签名：面板开关与三个数值必须自己
	// 参与签名，否则 composeFrame 会因“画面没变”跳过重绘，面板永远不出现。
	if s.autoMenu {
		hash = hash*31 + 1
		hash = hash*31 + s.autoMenuValue("AUTOWAIT", 200)
		hash = hash*31 + s.autoMenuValue("AUTOMIN", 2)
		hash = hash*31 + s.autoMenuValue("AUTOMAX", 15)
	}
	hash = hash*31 + s.selectSig
	if s.composer != nil {
		hash = hash*31 + s.composer.Revision()
	}
	if s.machine != nil {
		hash = hash*31 + s.machine.BackLogRevision()
	}
	if s.message != nil {
		hash = hash*31 + s.message.Revision()
		hash = hash*31 + s.message.RevealCount()
		hash = hash*31 + len(s.message.Body())
		if s.composer != nil {
			if effect := s.composer.Effect(); effect != nil {
				hash = hash*31 + int(effect.Kind())
			}
		}
	}
	return hash
}

func (s *Session) stageSignatureLocked() int {
	if s.stage == nil {
		return 0
	}
	hash := 17
	for _, sprite := range s.stage.All() {
		for _, c := range sprite.Name() {
			hash = hash*31 + int(c)
		}
		hash = hash*31 + sprite.X()
		hash = hash*31 + sprite.Y()
		hash = hash*31 + sprite.Alpha()
		if sprite.Visible() {
			hash = hash*31 + 1
		}
		hash = hash*31 + sprite.TextRevision()
		// 绘制还会消费这些字段：着色、裁剪、绘制分带、遮罩和实际位图。
		// 只哈希宽高会漏掉“同尺寸换图/改色/改裁剪”，composeFrame 会误判画面
		// 没变而跳过重绘（设置页的透过率试验块、同尺寸 CG 切换都会中招）。
		hash = hash*31 + sprite.Tint()
		hash = hash*31 + sprite.ColorAmount()
		hash = hash*31 + int(sprite.Kind())
		if sprite.HasSourceRect() {
			hash = hash*31 + 1
			hash = hash*31 + sprite.SourceX()
			hash = hash*31 + sprite.SourceY()
			hash = hash*31 + sprite.SourceWidth()
			hash = hash*31 + sprite.SourceHeight()
		} else {
			hash = hash*31 + 2
		}
		if image := sprite.ResolveImage(); image != nil {
			hash = hash*31 + image.Width
			hash = hash*31 + image.Height
			hash = hash*31 + int(reflect.ValueOf(image).Pointer())
		} else {
			hash = hash*31 + 7
		}
		if mask := sprite.ResolveMask(); mask != nil {
			hash = hash*31 + int(reflect.ValueOf(mask).Pointer())
		}
	}
	s.stageSignature = hash
	return hash
}

func (s *Session) takeDelta() float64 {
	now := time.Since(s.clock).Seconds()
	delta := now - s.lastSeconds
	s.lastSeconds = now
	if delta <= 0 {
		return 0
	}
	if delta > maximumDelta {
		return maximumDelta
	}
	return delta
}

func mapEmonStep(step vm.EmonStep, backLogActive bool, hasSelect bool) core.Step {
	if backLogActive {
		return core.StepWait
	}
	if (step == vm.EmonStepAwaitChoice || step == vm.EmonStepAwaitAction) && hasSelect {
		return core.StepAwaitSelect
	}
	switch step {
	case vm.EmonStepContinue:
		return core.StepContinue
	case vm.EmonStepAwaitInput:
		return core.StepAwaitInput
	case vm.EmonStepAwaitName:
		return core.StepAwaitName
	case vm.EmonStepAwaitChoice:
		return core.StepAwaitChoice
	case vm.EmonStepAwaitAction:
		return core.StepAwaitSelect
	case vm.EmonStepWait:
		return core.StepWait
	case vm.EmonStepFinished:
		return core.StepFinished
	default:
		return core.StepFaulted
	}
}

func (s *Session) isAnimating() bool {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return false
	}
	if s.step == vm.EmonStepFinished || s.step == vm.EmonStepFaulted {
		return false
	}
	if s.step == vm.EmonStepContinue || s.step == vm.EmonStepWait {
		return true
	}
	if s.composer != nil && s.composer.Effect() != nil {
		return true
	}
	if s.machine.MotionsActive() {
		return true
	}
	return s.message != nil && s.message.Visible() && !s.message.FullyRevealed()
}

func (s *Session) hasSelectLocked() bool {
	if s.disposed || s.ctx == nil || s.stage == nil {
		return false
	}
	if s.step != vm.EmonStepAwaitChoice && s.step != vm.EmonStepAwaitAction {
		return false
	}
	if s.machine != nil && s.machine.BackLogActive() {
		return false
	}
	return countVisibleSelectSprites(s.stage) > 0
}

func countVisibleSelectSprites(stg *stage.EmonStage) int {
	count := 0
	for _, sprite := range stg.SelectSprites() {
		if sprite.Visible() && isChainVisible(sprite) && !isMessageWindowSelect(sprite) {
			count++
		}
	}
	return count
}

func isChainVisible(sprite *stage.EmonSprite) bool {
	for current := sprite.Parent(); current != nil; current = current.Parent() {
		if !current.Visible() {
			return false
		}
	}
	return true
}
