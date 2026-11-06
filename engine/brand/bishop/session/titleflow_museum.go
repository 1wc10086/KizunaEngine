package session

import "kizuna/engine/core"

// titleMuseumModes 把标题上的鉴赏入口映射到 [museum phase] 的模式键。
// Ancient 作品的标题有 music/cg/scene 三个直达入口，但鉴赏室本身是否存在仍由 Museum 布局决定。
var titleMuseumModes = map[string]string{
	TitleKeyMuseumMusic: "music",
	TitleKeyMuseumCG:    "cg",
	TitleKeyMuseumScene: "scene",
}

// openTitleMuseumMode 打开鉴赏室并直接切到指定页签。
// 没有鉴赏布局时 openMuseum 返回 false，这里什么都不做，玩家停在标题，不会崩溃。
func (s *Session) openTitleMuseumMode(key string) {
	mode, ok := titleMuseumModes[key]
	if !ok || !s.openMuseum() {
		return
	}
	for index, entry := range s.config.Museum.Modes {
		if entry.Key == mode {
			s.openMuseumMode(index)
			return
		}
	}
	core.Logf("title: museum mode '%s' not configured", mode)
}
