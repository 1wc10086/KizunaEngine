package session

import (
	"math"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// karteDesignWidth/karteDesignHeight 是 [status2 screen] 坐标系的设计分辨率。
//
// OH 是 800x600，OHIE（1024x768）沿用同一份坐标配置，素材本身已经按
// 1024/800 = 1.28 重新出图（karte_select_0 从 206px 变成 264px、
// 留言图从 342px 变成 436px）。所以两张屏幕使用同一条坐标规则：
// 把配置坐标按屏幕/设计尺寸缩放，图片本身不再缩放。不缩放时 OHIE 的
// 角色卡会整体偏左上、留言框会压在上面的角色资料上。
const (
	karteDesignWidth  = 800.0
	karteDesignHeight = 600.0
)

// systemui_karte.go 实现 OH/OHIE 的「ケダモノカルテ」（Karte）画面。
//
// 它不是 define.bsa 的 status_layout，而是 bsxx.cfg 的 [status2 screen]：
//
//	选择页：karte_select_base + 四张 3 帧角色卡（黑=无病历 / 常态 / 悬停）
//	详情页：karte_baseN + 当前等级的 message / diagnosis / sd 覆盖图
//	        + 两段文本右侧的 Voice 按钮
//
// 等级来自 register 里的系统参数键。原引擎把等级直接写进脚本属性
// （OH/OHIE 是 #miya_karte…#umi_karte），Karte 只读不改；玩家的 H 事件
// 推进等级后，下一次打开就能看到新内容。
//
// 画面状态放在 systemUI 里而不是 Session：它和存读档/回想一样是临时
// 系统界面，关闭后应当原样回到剧情，不应该影响剧本变量或地图流程。

// openKarteScreen 打开 Karte。配置缺失时返回 false，调用方可以退回宿主角色的
// 设置页（和以前"按钮没反应"的行为区分开）。
func (s *Session) openKarteScreen() bool {
	if s.ui != nil || s.menuVisible {
		return false
	}
	if s.config == nil || !s.config.Status2.HasLayout() {
		return false
	}
	s.beginSystemUI(uiKarte)
	if s.ui != nil {
		s.ui.karteSelected = -1
		s.ui.hover = uiHit{kind: hitNone, index: -1}
	}
	core.Logf("ui: karte screen opened (%d target(s))", len(s.config.Status2.Targets))
	return true
}

// karteScale 返回配置坐标到实际屏幕坐标的比例。
//
// 取宽/高比例的较小值，避免非 4:3 分辨率把卡片拉变形；OH（800x600）得到
// 1.0，OHIE（1024x768）得到 1.28。
func (s *Session) karteScale() float64 {
	if s.width <= 0 || s.height <= 0 {
		return 1
	}
	scaleX := float64(s.width) / karteDesignWidth
	scaleY := float64(s.height) / karteDesignHeight
	return math.Min(scaleX, scaleY)
}

// karteButton 把配置里的按钮坐标换算到屏幕坐标。图片尺寸/帧数不变。
func (s *Session) karteButton(button config.UIButton) config.UIButton {
	scale := s.karteScale()
	if scale == 1 || button.Empty() {
		return button
	}
	button.X = int(math.Round(float64(button.X) * scale))
	button.Y = int(math.Round(float64(button.Y) * scale))
	return button
}

// kartePoint 把配置里的坐标点换算到屏幕坐标。
func (s *Session) kartePoint(point config.UIPoint) (int, int) {
	scale := s.karteScale()
	return int(math.Round(float64(point.X) * scale)), int(math.Round(float64(point.Y) * scale))
}

// karteLevel 读取角色当前病历等级。
//
// register 存的是系统参数键（OH/OHIE 是 10..13），不是属性表下标；
// 用 BsxScriptFile.ResolveSlot 转成属性/变量槽。等级 0 表示还没有病历：
// 选择页画黑色空卡，详情页只显示底图。
func (s *Session) karteLevel(character int) int {
	if s.config == nil || s.script == nil || s.machine == nil {
		return 0
	}
	screen := s.config.Status2
	if character < 0 || character >= len(screen.Register) {
		return 0
	}
	isProperty, slot, ok := s.script.ResolveSlot(screen.Register[character])
	if !ok {
		return 0
	}
	value := 0
	if isProperty {
		value = int(s.machine.Property(slot))
	} else if variables := s.machine.Variables(); uint(slot) < uint(len(variables)) {
		value = int(variables[slot])
	}
	return clampInt(value, 0, config.KarteMaxLevel)
}

// karteLevelIndex 把 1 基等级换算成素材切片下标；越界返回 -1。
func karteLevelIndex(level int, values []string) int {
	index := level - 1
	if index < 0 || index >= len(values) || len(values[index]) == 0 {
		return -1
	}
	return index
}

// karteBackFrame 返回 BACK 按钮的状态帧。
//
// karte_back 是 3 帧竖排：第 0 帧整块纯黑（原版用来隐藏按钮），
// 第 1 帧才是常态 BACK，第 2 帧是悬停/按下态。画第 0 帧会让按钮
// 完全看不见，所以这里从 1 开始。
func (s *Session) karteBackFrame() int {
	if s.ui != nil && s.ui.hover.kind == hitKarteBack {
		return 2
	}
	return 1
}

func (s *Session) drawKarteScreen(canvas *gfx.BgraCanvas) {
	if s.config == nil || canvas == nil {
		return
	}
	screen := s.config.Status2
	selected := -1
	if s.ui != nil {
		selected = s.ui.karteSelected
	}
	if selected < 0 || selected >= len(screen.Characters) || selected >= len(screen.Targets) {
		s.drawKarteSelection(canvas, screen)
		return
	}
	s.drawKarteDetail(canvas, screen, selected)
}

func (s *Session) drawKarteSelection(canvas *gfx.BgraCanvas, screen config.Status2Screen) {
	s.drawImageAt(canvas, screen.Base, 0, 0)
	for index, target := range screen.Targets {
		if index >= len(screen.Characters) {
			break
		}
		level := s.karteLevel(index)
		frame := 1
		if level <= 0 {
			// 没有病历的角色在原版里是一张黑色空卡，点进去也没有内容。
			frame = 0
		} else if s.ui != nil && s.ui.hover.kind == hitKarteTarget && s.ui.hover.index == index {
			frame = 2
		}
		s.drawFrameButton(canvas, s.karteButton(target), frame)
	}
	s.drawFrameButton(canvas, s.karteButton(screen.Back), s.karteBackFrame())
}

func (s *Session) drawKarteDetail(canvas *gfx.BgraCanvas, screen config.Status2Screen, character int) {
	value := screen.Characters[character]
	s.drawImageAt(canvas, value.Base, 0, 0)
	level := s.karteLevel(character)
	if level > 0 {
		messageX, messageY := s.kartePoint(value.MessagePos)
		diagnosisX, diagnosisY := s.kartePoint(value.DiagnosisPos)
		sdX, sdY := s.kartePoint(value.SDPos)
		if index := karteLevelIndex(level, value.Messages); index >= 0 {
			s.drawImageAt(canvas, value.Messages[index], messageX, messageY)
		}
		if index := karteLevelIndex(level, value.Diagnoses); index >= 0 {
			s.drawImageAt(canvas, value.Diagnoses[index], diagnosisX, diagnosisY)
		}
		if index := karteLevelIndex(level, value.SDPictures); index >= 0 {
			s.drawImageAt(canvas, value.SDPictures[index], sdX, sdY)
		}
		// 语音按钮只在对应等级真的配了音时画：某些等级（lv10 的"仮処置"）
		// 只有文本没有语音，画一颗点了没反应的按钮反而误导玩家。
		if karteLevelIndex(level, value.MessageVoices) >= 0 && !value.VoiceButton.Empty() {
			// karte_voice 是 4 帧：0=黑色隐藏态，1=常态，2=悬停，3=按下。
			frame := 1
			if s.ui != nil && s.ui.hover.kind == hitKarteVoice && s.ui.hover.index == 0 {
				frame = 2
			}
			s.drawFrameButton(canvas, s.karteButton(value.VoiceButton), frame)
		}
		if karteLevelIndex(level, value.DiagnosisVoices) >= 0 && !value.DiagnosisButton.Empty() {
			frame := 1
			if s.ui != nil && s.ui.hover.kind == hitKarteVoice && s.ui.hover.index == 1 {
				frame = 2
			}
			s.drawFrameButton(canvas, s.karteButton(value.DiagnosisButton), frame)
		}
	}
	s.drawFrameButton(canvas, s.karteButton(screen.Back), s.karteBackFrame())
}

func (s *Session) hitKarteScreen(x, y int) uiHit {
	if s.config == nil || s.ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	screen := s.config.Status2
	// 返回键在选择页和详情页都存在，先判它，避免和卡片/语音区重叠。
	if hitButton(s, s.karteButton(screen.Back), x, y) {
		return uiHit{kind: hitKarteBack, index: -1}
	}
	selected := s.ui.karteSelected
	if selected < 0 || selected >= len(screen.Characters) {
		for index, target := range screen.Targets {
			if index >= len(screen.Characters) {
				break
			}
			if s.karteLevel(index) <= 0 {
				continue
			}
			if hitButton(s, s.karteButton(target), x, y) {
				return uiHit{kind: hitKarteTarget, index: index}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	value := screen.Characters[selected]
	level := s.karteLevel(selected)
	if level > 0 {
		// 两个语音按钮的命中顺序与绘制顺序一致：留言在上、诊断在下。
		if karteLevelIndex(level, value.MessageVoices) >= 0 && !value.VoiceButton.Empty() &&
			hitButton(s, s.karteButton(value.VoiceButton), x, y) {
			return uiHit{kind: hitKarteVoice, index: 0}
		}
		if karteLevelIndex(level, value.DiagnosisVoices) >= 0 && !value.DiagnosisButton.Empty() &&
			hitButton(s, s.karteButton(value.DiagnosisButton), x, y) {
			return uiHit{kind: hitKarteVoice, index: 1}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateKarteScreen(hit uiHit) {
	ui := s.ui
	if ui == nil || s.config == nil {
		return
	}
	switch hit.kind {
	case hitKarteTarget:
		if hit.index >= 0 && hit.index < len(s.config.Status2.Characters) {
			ui.karteSelected = hit.index
		}
	case hitKarteBack:
		if ui.karteSelected >= 0 {
			// 详情页的返回先回选择页，玩家可以继续看别的角色。
			ui.karteSelected = -1
			return
		}
		s.closeSystemUI()
	case hitKarteVoice:
		s.playKarteVoice(ui.karteSelected, hit.index)
	}
}

// playKarteVoice 播放 Karte 文本对应的语音。
//
// 走和剧情/回想相同的音量链路（语音通道 × 角色个别音量），这样设置里的
// 角色音量对 Karte 同样生效。名字为空或资源不存在时静默跳过，不弹错误。
func (s *Session) playKarteVoice(character, kind int) {
	if s.config == nil || character < 0 || character >= len(s.config.Status2.Characters) {
		return
	}
	value := s.config.Status2.Characters[character]
	level := s.karteLevel(character)
	if level <= 0 {
		return
	}
	names := value.MessageVoices
	if kind == 1 {
		names = value.DiagnosisVoices
	}
	index := karteLevelIndex(level, names)
	if index < 0 {
		return
	}
	name := names[index]
	if len(name) == 0 {
		return
	}
	s.audio = append(s.audio, core.AudioEvent{
		Kind:    2,
		Path:    s.materialize(name),
		Volume:  s.volumeGain(kindVoice) * s.characterGain(name, true),
		Channel: noTarget,
		Group:   noTarget,
	})
}
