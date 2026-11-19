// 原版设置面板的引擎自绘实现（sq_setting）。
//
// 布局与原图名全部取 menus.ConfigLayoutFor()；不按“看起来差不多”再调，
// 否则会与 config_bg 里烘死的标签错位。指针命中与绘制同源同一张布局表，
// 保证画出来的按钮与能点中的按钮永远对得上。
package session

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	overaudio "kizuna/enginego/brand/overdose/audio"
	"kizuna/enginego/brand/overdose/menus"
	oversave "kizuna/enginego/brand/overdose/save"
	"kizuna/enginego/core"
)

// composeConfigOverlay 画设置面板：背景、消息窗预览、滑杆柄、所有按钮。
func (s *Session) composeConfigOverlay(canvas *bishopgfx.BgraCanvas) {
	l := menus.ConfigLayoutFor()
	s.drawImage(canvas, l.Background, 0, 0)

	// 消息窗预览：config_mw 按透明度 alpha 画，再在同位置盖 config_txt。
	// alpha 公式与原版一致：255-(100-opacity)*255/100，opacity=100 时不透明。
	op := int(s.settings.WindowOpacity)
	if op < 0 {
		op = 0
	}
	if op > 100 {
		op = 100
	}
	alpha := 255 - (100-op)*255/100
	if s.cache != nil {
		if img := s.cache.Get(l.Window); img != nil {
			canvas.Blit(img, int(l.PreviewRect.X), int(l.PreviewRect.Y), alpha)
		}
	}
	s.drawImage(canvas, l.TextBg, int(l.PreviewRect.X), int(l.PreviewRect.Y))

	// 滑杆柄：x = rect.X + value*(SliderTravel-PinSize)/100。
	s.drawConfigPin(canvas, l, l.WndSliderRect, s.settings.WindowOpacity)
	s.drawConfigPin(canvas, l, l.AudioSliderRects[0], s.settings.MasterVolume)
	s.drawConfigPin(canvas, l, l.AudioSliderRects[1], s.settings.BgmVolume)
	s.drawConfigPin(canvas, l, l.AudioSliderRects[2], s.settings.SeVolume)
	s.drawConfigPin(canvas, l, l.AudioSliderRects[3], s.settings.VoiceVolume)

	// 开关类：两侧/三档都画，当前选中项用 -o 按下图。
	s.drawButton(canvas, l.On, l.FullOnRect, s.settings.FullScreen)
	s.drawButton(canvas, l.Off, l.FullOffRect, !s.settings.FullScreen)
	s.drawButton(canvas, l.On, l.EffectOnRect, s.settings.EffectEnabled)
	s.drawButton(canvas, l.Off, l.EffectOffRect, !s.settings.EffectEnabled)
	s.drawButton(canvas, l.On, l.UnreadOnRect, s.settings.SkipReadOnly)
	s.drawButton(canvas, l.Off, l.UnreadOffRect, !s.settings.SkipReadOnly)

	textImgs := [3]string{l.TextSlow, l.TextNormal, l.TextFast}
	for i, r := range l.TextRects {
		s.drawButton(canvas, textImgs[i], r, s.settings.TextSpeed == int32(i+1))
	}
	autoImgs := [3]string{l.AutoSlow, l.AutoNormal, l.AutoFast}
	for i, r := range l.AutoRects {
		s.drawButton(canvas, autoImgs[i], r, autoDelayTier(s.settings.AutoDelayMs) == i+1)
	}
	clickImgs := [3]string{l.ClickRight1, l.ClickRight2, l.ClickRight3}
	for i, r := range l.ClickRects {
		s.drawButton(canvas, clickImgs[i], r, s.settings.ClickRightMode == int32(i+1))
	}

	// 角色语音：config_voice 是覆盖在 config_bg 已烘死的角色立绘上的 ON/OFF 状态图；
	// 开启时用 -o（ON 变红），关闭时用常态（两个标签都是浅色）。
	// 不要把 198x90 当两帧切片：原图就是一张 198x90 的透明叠图。
	for i, r := range l.CharaRects {
		s.drawButton(canvas, l.Voice, r, s.settings.UseChara[i])
	}

	// 音频试听：两个 config_test。
	for _, r := range l.TestRects {
		s.drawImage(canvas, l.Test, int(r.X), int(r.Y))
	}

	// 底部四按钮。
	s.drawImage(canvas, l.Title, int(l.TitleRect.X), int(l.TitleRect.Y))
	s.drawImage(canvas, l.Return, int(l.ReturnRect.X), int(l.ReturnRect.Y))
	s.drawImage(canvas, l.Exit, int(l.ExitRect.X), int(l.ExitRect.Y))
	s.drawImage(canvas, l.Reset, int(l.ResetRect.X), int(l.ResetRect.Y))
}

// drawConfigPin 按 0..100 的值画滑杆柄。
func (s *Session) drawConfigPin(canvas *bishopgfx.BgraCanvas, l menus.ConfigLayout, r menus.Rect, value int32) {
	v := int(value)
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	travel := l.SliderTravel - l.PinSize
	if travel < 0 {
		travel = 0
	}
	x := int(r.X) + v*travel/100
	s.drawImage(canvas, l.Pin, x, int(r.Y))
}

// autoDelayTier 把 AutoDelayMs 换回档位：2000=slow(1)/1000=normal(2)/200=fast(3)。
// 未知值按最接近档处理，按钮至少能标出一个选中项。
func autoDelayTier(ms int32) int {
	switch {
	case ms <= 600:
		return 3
	case ms <= 1500:
		return 2
	default:
		return 1
	}
}

// pointerConfigOverlay 处理设置面板的点按。
func (s *Session) pointerConfigOverlay(x, y int32) (core.Step, bool) {
	l := menus.ConfigLayoutFor()

	// 底部按钮：return 保存关闭；title 保存回标题；exit 保存后请求宿主退出；
	// reset 恢复默认设置。
	if hit(l.ReturnRect, x, y) {
		s.persistSystem()
		return s.closeOverlay(), true
	}
	if hit(l.TitleRect, x, y) {
		s.persistSystem()
		if step, err := s.Start(0); err == nil {
			return step, true
		}
		return s.step, true
	}
	if hit(l.ExitRect, x, y) {
		s.persistSystem()
		s.requestHost("exit")
		return s.step, true
	}
	if hit(l.ResetRect, x, y) {
		s.settings = oversave.DefaultSettings()
		s.applyVolumes()
		s.persistSystem()
		return s.step, true
	}

	// 开关类：点击任意侧即切到该侧状态。
	if hit(l.FullOnRect, x, y) {
		s.settings.FullScreen = true
		s.persistSystem()
		return s.step, true
	}
	if hit(l.FullOffRect, x, y) {
		s.settings.FullScreen = false
		s.persistSystem()
		return s.step, true
	}
	if hit(l.EffectOnRect, x, y) {
		s.settings.EffectEnabled = true
		s.persistSystem()
		return s.step, true
	}
	if hit(l.EffectOffRect, x, y) {
		s.settings.EffectEnabled = false
		s.persistSystem()
		return s.step, true
	}
	if hit(l.UnreadOnRect, x, y) {
		s.settings.SkipReadOnly = true
		s.persistSystem()
		return s.step, true
	}
	if hit(l.UnreadOffRect, x, y) {
		s.settings.SkipReadOnly = false
		s.persistSystem()
		return s.step, true
	}

	// 三档：文本速度 / 自动速度 / 右键模式。
	for i, r := range l.TextRects {
		if hit(r, x, y) {
			s.settings.TextSpeed = int32(i + 1)
			s.persistSystem()
			return s.step, true
		}
	}
	autoMs := [3]int32{2000, 1000, 200}
	for i, r := range l.AutoRects {
		if hit(r, x, y) {
			s.settings.AutoDelayMs = autoMs[i]
			s.persistSystem()
			return s.step, true
		}
	}
	for i, r := range l.ClickRects {
		if hit(r, x, y) {
			s.settings.ClickRightMode = int32(i + 1)
			s.persistSystem()
			return s.step, true
		}
	}

	// 角色语音：点叉切换对应角色。
	for i, r := range l.CharaRects {
		if hit(r, x, y) {
			s.settings.UseChara[i] = !s.settings.UseChara[i]
			s.persistSystem()
			return s.step, true
		}
	}

	// 试听：SE 播 se_001，VOICE 播 syu0010。
	if hit(l.TestRects[0], x, y) {
		if s.audio != nil {
			s.audio.Play(overaudio.PlayRequest{
				Channel: overaudio.Channel{Kind: overaudio.CategorySound, Idx: 0},
				Name:    "se_001",
			})
		}
		return s.step, true
	}
	if hit(l.TestRects[1], x, y) {
		if s.audio != nil {
			s.audio.Play(overaudio.PlayRequest{
				Channel: overaudio.Channel{Kind: overaudio.CategoryVoice, Idx: 0},
				Name:    "syu0010",
			})
		}
		return s.step, true
	}

	// 滑杆：命中轨道时按 x 计算 0..100，并挂起拖动。
	if hit(l.WndSliderRect, x, y) {
		s.configDrag = 1
		s.setConfigSlider(l.WndSliderRect, x)
		return s.step, true
	}
	for i, r := range l.AudioSliderRects {
		if hit(r, x, y) {
			s.configDrag = 2 + i
			s.setConfigSlider(r, x)
			return s.step, true
		}
	}

	return s.step, false
}

// dragConfig 按当前拖动中的滑杆更新值（Pointer 的 op=6 触屏拖动入口）。
func (s *Session) dragConfig(x int32) core.Step {
	l := menus.ConfigLayoutFor()
	switch s.configDrag {
	case 1:
		s.setConfigSlider(l.WndSliderRect, x)
	case 2, 3, 4, 5:
		s.setConfigSlider(l.AudioSliderRects[s.configDrag-2], x)
	default:
		return s.step
	}
	return s.step
}

// setConfigSlider 按 x 位置把滑杆换算成 0..100 写回设置，并落盘、推音量。
func (s *Session) setConfigSlider(r menus.Rect, x int32) {
	l := menus.ConfigLayoutFor()
	travel := l.SliderTravel - l.PinSize
	v := 0
	if travel > 0 {
		v = int(x) - int(r.X)
		v = v * 100 / travel
	}
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	switch s.configDrag {
	case 1:
		s.settings.WindowOpacity = int32(v)
	case 2:
		s.settings.MasterVolume = int32(v)
		s.applyVolumes()
	case 3:
		s.settings.BgmVolume = int32(v)
		s.applyVolumes()
	case 4:
		s.settings.SeVolume = int32(v)
		s.applyVolumes()
	case 5:
		s.settings.VoiceVolume = int32(v)
		s.applyVolumes()
	default:
		return
	}
	s.persistSystem()
}

// applyVolumes 按 master 缩放推各通道音量：实际值 = 通道值 × master / 100。
func (s *Session) applyVolumes() {
	if s.audio == nil {
		return
	}
	master := s.settings.MasterVolume
	s.audio.SetVolume(overaudio.Channel{Kind: overaudio.CategoryMusic, Idx: 0}, s.settings.BgmVolume*master/100)
	s.audio.SetVolume(overaudio.Channel{Kind: overaudio.CategorySound, Idx: 0}, s.settings.SeVolume*master/100)
	s.audio.SetVolume(overaudio.Channel{Kind: overaudio.CategoryVoice, Idx: 0}, s.settings.VoiceVolume*master/100)
}
