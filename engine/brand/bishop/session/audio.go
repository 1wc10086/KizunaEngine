package session

import (
	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/core"
)

func (s *Session) pollAudio() {
	if s.gameState == nil || s.res == nil {
		return
	}
	if bgm, ok := s.gameState.Bgm(); ok != s.lastBgmOn || bgm != s.lastBgm {
		stopped := s.lastBgmOn && !ok
		s.lastBgm = bgm
		s.lastBgmOn = ok
		if stopped {
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    3, // stopBgm
				Channel: noTarget,
				Group:   noTarget,
			})
		}
		if ok {
			s.audio = append(s.audio, core.AudioEvent{
				Kind: 0,
				// 脚本给的是 type0 的曲名；按设置里的 BGM 类型换成对应版本。
				Path:    s.materialize(s.bgmForMode(bgm)),
				Volume:  s.gameState.BgmVolume() * s.volumeGain(kindBgm),
				Loop:    s.gameState.BgmLoop(),
				Channel: noTarget,
				Group:   noTarget,
			})
		}
	}
	if serial := s.gameState.EffectSerial(); serial != s.lastEffectN {
		s.lastEffectN = serial
		if name, ok := s.gameState.SoundEffect(); ok {
			s.lastEffect = name
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    1,
				Path:    s.materialize(name),
				Volume:  s.volumeGain(kindSe),
				Channel: noTarget,
				Group:   noTarget,
			})
		} else if s.lastEffect != "" {
			// 同 voice：音效通道的显式停止也要落成一个事件，否则一次性音效会被
			// 脚本 StopSound 之后继续播放。
			s.lastEffect = ""
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    5, // stopSe
				Channel: noTarget,
				Group:   noTarget,
			})
		}
	}
	if serial := s.gameState.VoiceSerial(); serial != s.lastVoiceN {
		s.lastVoiceN = serial
		if name, ok := s.gameState.Voice(); ok {
			s.lastVoice = name
			s.voicePlaying = true
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    2,
				Path:    s.materialize(name),
				Volume:  s.volumeGain(kindVoice) * s.characterGain(name, true),
				Channel: noTarget,
				Group:   noTarget,
				// 「音声をループ再生させる」：原版会把当前句语音循环到下一句/
				// 点击为止；宿主按 Loop 重新播放同一文件。
				Loop: s.settings.VoiceLoop,
			})
			// 原引擎在语音播放时把 BGM 压低到 attenuated bgm，播完恢复。
			// 这里用 kind 7（引擎音乐增益）乘在 BGM 事件音量上，不重启动 BGM。
			s.appendBgmAttenuation(true)
		} else if s.lastVoice != "" {
			// 序列号变了但已经没有 voice：脚本发的是"停止语音"（StopSound 语音通道，
			// 或下一句对白没有配音）。旧实现只处理"有 voice"的分支，这里的停止请求
			// 被静默丢弃，Dart 侧那份仍在播放的语音就会一直响下去——玩家看到的现象是
			// 上一句还没说完就点下一句，两句话的声音叠在一起。这里必须显式发停止事件。
			s.lastVoice = ""
			s.voicePlaying = false
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    4, // stopVoice
				Channel: noTarget,
				Group:   noTarget,
			})
			s.appendBgmAttenuation(false)
		}
	}
	if serial := s.gameState.VoiceSequenceSerial(); serial != s.lastSeqN {
		s.lastSeqN = serial
		names := append([]string(nil), s.gameState.VoiceSequence()...)
		if len(names) > 0 {
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    6,
				Names:   s.materializeAll(names),
				Volume:  s.volumeGain(kindVoice),
				Loop:    s.gameState.VoiceSequenceLoop(),
				Channel: noTarget,
				Group:   noTarget,
			})
		}
	}
}

// stopVoiceForAdvance 在"玩家点了下一句"时立刻掐掉正在播的语音。
//
// 原引擎的行为是：推进对白 = 放弃上一句的语音，只播当前句。我们的音频播放在 Dart
// 侧（flutter_soloud），脚本状态里 voice 只是"当前句应该有哪条语音"，光靠
// VoiceSerial 变化通知不到"这一句被跳过了"——下一句如果同样没有配音，序列号不变，
// 宿主永远收不到停止信号，两条语音就叠在一起。
//
// 因此把"推进"这个动作本身当成停止点：只要有语音在播，就先发一条 stopVoice。
// 与 BGM 不同，语音本来就不跨句，停掉不会影响后续表现。
func (s *Session) stopVoiceForAdvance() {
	if s.gameState == nil {
		return
	}
	// 只在"我们确实发出过一条 voice、且还没停过"时才补停止事件。每句都发会白白
	// 占满宿主的事件队列，也会让 Dart 侧不停做无意义的 stop 调用。
	if !s.voicePlaying {
		s.lastVoiceN = s.gameState.VoiceSerial()
		return
	}
	s.voicePlaying = false
	s.lastVoice = ""
	s.lastVoiceN = s.gameState.VoiceSerial()
	s.audio = append(s.audio, core.AudioEvent{
		Kind:    4, // stopVoice
		Channel: noTarget,
		Group:   noTarget,
	})
	s.appendBgmAttenuation(false)
}

func (s *Session) resetAudioCache() {
	s.lastBgm = ""
	s.lastBgmOn = false
	s.lastEffect = ""
	s.lastEffectN = 0
	if s.gameState != nil {
		s.lastEffectN = s.gameState.EffectSerial()
	}
	s.lastVoice = ""
	s.voicePlaying = false
	s.lastVoiceN = 0
	if s.gameState != nil {
		s.lastVoiceN = s.gameState.VoiceSerial()
	}
	s.lastSeqN = 0
	if s.gameState != nil {
		s.lastSeqN = s.gameState.VoiceSequenceSerial()
	}
}

func (s *Session) materialize(name string) string {
	path := audio.Materialize(name, s.res, s.cacheDir)
	if path == "" {
		return name
	}
	return path
}

func (s *Session) materializeAll(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, s.materialize(name))
	}
	return out
}
