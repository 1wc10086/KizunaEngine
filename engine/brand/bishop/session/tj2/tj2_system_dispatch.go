package tj2

import (
	"strings"

	"kizuna/engine/brand/bishop/state"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
)

func HandleTj2System(session *Tj2Session, command vmtj2.Tj2Command) state.SessionStep {
	if command.Kind != vmtj2.Tj2CommandKindFunction {
		core.Logf("tj2: !p unexpected %d tick=%d", int(command.Kind), session.TickCounter())
		return state.SessionContinue
	}
	if command.Value == 0xFFFF {
		return state.SessionContinue
	}
	text := command.Text
	if session.IsClassic() && session.Machine() != nil {
		return handleTj2Classic(session, session.Machine(), command, text)
	}
	switch command.Value {
	case 0:
		core.Logf("tj2: func0 shake tick=%d", session.TickCounter())
	case 5:
		core.Logf("tj2: func5 stub tick=%d", session.TickCounter())
	case 8:
		core.Logf("tj2: func8 map a=%d b=%d tick=%d", tj2At(text, 0), tj2At(text, 1), session.TickCounter())
	case 10:
		return session.OpenMap()
	case 11:
		session.SetMapClock(tj2Atoi(text) != 0)
		clock := "off"
		if tj2Atoi(text) != 0 {
			clock = "on"
		}
		core.Logf("tj2: func11 clock %s tick=%d", clock, session.TickCounter())
	case 12, 13, 14:
		core.Logf("tj2: func%d dose %d tick=%d", command.Value, tj2Atoi(text), session.TickCounter())
	case 16:
		session.SetMenuVisible(true)
		core.Logf("tj2: func16 title menu=%v tick=%d", session.MenuVisible(), session.TickCounter())
	case 21:
		session.UnlockGallery(tj2Atoi(text))
		core.Logf("tj2: func21 unlock-cg %d tick=%d", tj2Atoi(text), session.TickCounter())
	case 23:
		core.Logf("tj2: func23 ending tick=%d", session.TickCounter())
	case 24:
		session.RecordMapDefault(tj2At(text, 0), tj2At(text, 1))
		core.Logf("tj2: func24 cell a=%d b=%d tick=%d", tj2At(text, 0), tj2At(text, 1), session.TickCounter())
	case 25:
		session.RecordMapCells(tj2At(text, 0), tj2At(text, 1), tj2At(text, 2), tj2At(text, 3), tj2At(text, 4))
		core.Logf("tj2: func25 cells %s tick=%d", text, session.TickCounter())
	case 26:
		session.SetTimerVisible(tj2Atoi(text) != 0)
		timer := "off"
		if session.TimerVisible() {
			timer = "on"
		}
		core.Logf("tj2: func26 timer %s tick=%d", timer, session.TickCounter())
	case 27:
		session.AddTick()
		applyTj2Digits(session, tj2At(text, 0), tj2At(text, 1), tj2At(text, 2))
		core.Logf("tj2: func27 digits %d,%d,%d countdown=%d visible=%v", tj2At(text, 0), tj2At(text, 1), tj2At(text, 2), session.CountdownSeconds(), session.CountdownVisible())
	case 28:
		core.Logf("tj2: func28 recall %d,%d,%d tick=%d", tj2At(text, 0), tj2At(text, 1), tj2At(text, 2), session.TickCounter())
	case 29:
		session.AddTick()
		tickTj2Countdown(session)
		core.Logf("tj2: func29 countdown-tick countdown=%d visible=%v", session.CountdownSeconds(), session.CountdownVisible())
	case 30:
		applyTj2Countdown(session, tj2Atoi(text))
		core.Logf("tj2: func30 countdown-set %d countdown=%d visible=%v", tj2Atoi(text), session.CountdownSeconds(), session.CountdownVisible())
	case 31:
		core.Logf("tj2: func31 permit tick=%d", session.TickCounter())
	case 32:
		core.Logf("tj2: func32 logo tick=%d", session.TickCounter())
	case 33:
		session.AddTick()
		applyTj2Countdown(session, tj2At(text, 0))
		core.Logf("tj2: func33 gauge %d,%d,%d,%d tick=%d", tj2At(text, 0), tj2At(text, 1), tj2At(text, 2), tj2At(text, 3), session.TickCounter())
	case 34:
		core.Logf("tj2: func34 anim %s tick=%d", text, session.TickCounter())
	case 35:
		session.SetMenuVisible(tj2Atoi(text) != 0)
		mode := "off"
		if tj2Atoi(text) != 0 {
			mode = "on"
		}
		core.Logf("tj2: func35 h-mode %s menu=%v tick=%d", mode, session.MenuVisible(), session.TickCounter())
	case 36, 37:
		core.Logf("tj2: func%d param %d tick=%d", command.Value, tj2Atoi(text), session.TickCounter())
	default:
		core.Logf("tj2: !p func%d \"%s\" tick=%d", command.Value, text, session.TickCounter())
	}
	return state.SessionContinue
}

func handleTj2Classic(session *Tj2Session, machine *vmtj2.Tj2Machine, command vmtj2.Tj2Command, text string) state.SessionStep {
	switch command.Value {
	case 0:
		session.SetShake(tj2At(text, 0), tj2At(text, 1))
		core.Logf("tj2: func0 shake %d,%d tick=%d", tj2At(text, 0), tj2At(text, 1), session.TickCounter())
	case 1:
		// 参数是 r,g,b,outMs,inMs：旧脚本的"眼前发黑/白光一闪"。
		// 第 4/5 段是淡出/淡入时长，合起来就是这次效果的持续时间。
		duration := tj2At(text, 3) + tj2At(text, 4)
		session.ApplyScreenFillTimed(tj2At(text, 0), tj2At(text, 1), tj2At(text, 2), duration)
		core.Logf("tj2: func1 fill %d,%d,%d dur=%d tick=%d", tj2At(text, 0), tj2At(text, 1), tj2At(text, 2), duration, session.TickCounter())
	case 2:
		session.SetMenuVisible(true)
		core.Logf("tj2: func2 menu-show menu=%v tick=%d", session.MenuVisible(), session.TickCounter())
	case 3:
		session.SetMenuVisible(false)
		session.Stage().Invalidate()
		core.Logf("tj2: func3 menu-hide menu=%v tick=%d", session.MenuVisible(), session.TickCounter())
	case 4:
		core.Logf("tj2: func4 menu-item %d tick=%d", tj2Atoi(text), session.TickCounter())
	case 5:
		core.Logf("tj2: func5 flag-set tick=%d", session.TickCounter())
	case 6:
		flag0, _ := machine.ReadFlag(0)
		speed := int(flag0)
		if speed > 5 {
			speed = 5
		}
		session.State().SetTextSpeed(float64(speed))
		core.Logf("tj2: func6 text-speed %d tick=%d", speed, session.TickCounter())
	case 7:
		flag0, _ := machine.ReadFlag(0)
		flag1, _ := machine.ReadFlag(1)
		flag2, _ := machine.ReadFlag(2)
		// 旧脚本这三个字节是"角色/台词色索引"，不是 RGB。按 RGB 直用会
		// 得到 (1,10,6) 这类近黑色，正文在棕色信息框上几乎看不清；
		// 原作的对白是白字，这里保持默认色不变。
		core.Logf("tj2: func7 text-color-index %d,%d,%d tick=%d", int(flag0), int(flag1), int(flag2), session.TickCounter())
	case 8:
		flag0, _ := machine.ReadFlag(0)
		flag1, _ := machine.ReadFlag(1)
		session.SetWindowStyle(int(flag0), int(flag1))
		core.Logf("tj2: func8 window-style %d,%d tick=%d", int(flag0), int(flag1), session.TickCounter())
	case 9:
		index := tj2Atoi(text)
		name := ""
		if comma := strings.Index(text, ","); comma >= 0 {
			name = text[comma+1:]
		}
		session.SetNameEntry(index, name)
		session.ClassicMap().SetLabel(index, name)
		core.Logf("tj2: func9 name %d '%s' tick=%d", index, name, session.TickCounter())
	case 10:
		session.generalDword = tj2Atoi(text)
		session.ClassicMap().SetActive(session.generalDword != 0)
		if session.generalDword != 0 {
			// 地图是全屏界面，信息框不能压在 MVM01 的地点按钮上。
			session.State().SetMessageWindowVisible(false)
			session.Stage().Invalidate()
		}
		core.Logf("tj2: func10 dword %d tick=%d", session.generalDword, session.TickCounter())
	case 11:
		session.overlayValue = tj2Atoi(text)
		session.ReloadMessageWindow()
		core.Logf("tj2: func11 overlay %d tick=%d", session.overlayValue, session.TickCounter())
	case 12:
		// 训练场景里 func12/13/14 每次传 1，语义是"给某个计数器加一"，
		// 不是把值设成 1。旧实现反复赋值会把 8 次喂食记成 1 次。
		session.counterA += tj2Atoi(text)
		core.Logf("tj2: func12 counter %d tick=%d", session.counterA, session.TickCounter())
	case 13:
		session.counterB += tj2Atoi(text)
		core.Logf("tj2: func13 counter %d tick=%d", session.counterB, session.TickCounter())
	case 14:
		session.counterC += tj2Atoi(text)
		core.Logf("tj2: func14 counter %d tick=%d", session.counterC, session.TickCounter())
	case 15:
		session.audioDword = tj2Atoi(text)
		core.Logf("tj2: func15 audio %d tick=%d", session.audioDword, session.TickCounter())
	case 16:
		// SCN001 在 func16 之后用 flag[0] 分派四个标题选项。这里必须停住，
		// 否则会直接跑进"初めから"分支（警告画面 + 序章），玩家看不到标题。
		session.titleMenuVisible = true
		return session.enterClassicTitle()
	case 17:
		// 鉴赏室根菜单：GALLARY.KG 已由前面的 !g 铺好，这里只叠菜单热区。
		session.galleryVisible = true
		core.Logf("tj2: func17 gallery root tick=%d", session.TickCounter())
		return session.openClassicGallery()
	case 18:
		session.soundMenuVisible = true
		core.Logf("tj2: func18 sound gallery tick=%d", session.TickCounter())
		return session.openClassicSound()
	case 19:
		session.cgModeVisible = true
		core.Logf("tj2: func19 cg gallery tick=%d", session.TickCounter())
		return session.openClassicCG()
	case 20:
		session.sceneRecallVisible = true
		core.Logf("tj2: func20 scene gallery tick=%d", session.TickCounter())
		return session.openClassicScene()
	case 21:
		session.UnlockGallery(tj2Atoi(text))
		core.Logf("tj2: func21 unlock-cg %d tick=%d", tj2Atoi(text), session.TickCounter())
	case 22:
		core.Logf("tj2: func22 close tick=%d", session.TickCounter())
		return state.SessionFinished
	case 23:
		// SCN178 自己也会写 flag[3]=1；这里再写一次，结局后从标题进
		// 鉴赏室不会因为存档/读档把 flag 清掉而失效。
		session.writeClassicFlag(3, 1)
		core.Logf("tj2: func23 ending tick=%d", session.TickCounter())
	default:
		core.Logf("tj2: !p func%d \"%s\" tick=%d", command.Value, text, session.TickCounter())
	}
	return state.SessionContinue
}

func tj2ToByte(value int) uint8 {
	return clampTj2Byte(value)
}

func applyTj2Digits(session *Tj2Session, hundreds, tens, ones int) {
	applyTj2Countdown(session, hundreds*100+tens*10+ones)
}

func applyTj2Countdown(session *Tj2Session, seconds int) {
	if seconds <= 0 {
		session.HideCountdown()
		return
	}
	session.ShowCountdown(seconds)
}

func tickTj2Countdown(session *Tj2Session) {
	next := session.CountdownSeconds() - 1
	if next <= 0 {
		session.HideCountdown()
		return
	}
	session.ShowCountdown(next)
}

func tj2At(text string, index int) int {
	parts := strings.Split(text, ",")
	if index >= len(parts) {
		return 0
	}
	return tj2Atoi(parts[index])
}
