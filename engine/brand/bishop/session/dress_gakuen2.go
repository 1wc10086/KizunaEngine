package session

import "kizuna/engine/core"

// dress_gakuen2.go 是学園2 老版 DRESS 按钮的着衣/全裸切换。
//
// 原引擎的 clNNNNN 图层不是完整名字：剧本写 cl05001，素材实际是
// cl050010（着衣）/ cl050011（全裸）。按钮只切换一个 0/1 状态，然后按新
// 后缀重新加载当前舞台上的角色图层。这里复现同一语义：
//   - 状态存在 GameSettings.DressNude，重启/读档后保持；
//   - 切换时清空图片缓存并让下一次合成按新后缀解析。
func (s *Session) applyDressSuffix() {
	if s.composer == nil || s.config == nil || !s.config.Museum.Gakuen2 {
		// 只有学園2 老版把 clNNNNN 的着衣状态拼进素材名；其它作品保持
		// 精确名字，不能让 "0" 后缀误命中同名素材。
		return
	}
	suffix := "0"
	if s.settings.DressNude {
		suffix = "1"
	}
	s.composer.Images.SetDressSuffix(suffix)
}

// toggleGakuen2Dress 切换着衣状态并立即重绘；只对学園2 老版生效。
func (s *Session) toggleGakuen2Dress() bool {
	if s.config == nil || !s.config.Museum.Gakuen2 {
		return false
	}
	s.settings.DressNude = !s.settings.DressNude
	s.applyDressSuffix()
	s.saveGameSettings()
	if s.composer != nil {
		s.composer.ReloadImages()
	}
	suffix := "0"
	if s.settings.DressNude {
		suffix = "1"
	}
	core.Logf("ui: gakuen2 dress suffix '%s'", suffix)
	return true
}
