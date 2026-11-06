package session

import (
	"strings"

	"kizuna/engine/brand/bishop/state"
)

// simple_display.go 实现 MKIE 工具条上的 VIEW / STRIP 两颗"简易显示"按钮。
//
// 它们是 [system button] 里的 simple view / simple strip，作用域 game，
// 操作手册的说明是：
//   - VIEW：立绘处于放大状态时，按「顔 → 胸 → 股間」循环切换视点；
//   - STRIP：让当前立绘脱衣（着衣 → 下着 → 全裸 循环）。
//
// 这两颗按钮只在 MKIE/MK2IE 这一代出现（MK 的对应按钮是 VIEW/DRESS），
// 所以动作放在会话层，而不是加进所有作品共用的视角/换装分支里：
// 其它作品的 view up / dress 语义不同，混在一起会把 侵蝕/淫獄 的按钮行为改坏。

// simpleViewNext 返回下一档视点。原版是循环的，到头回到「顔」。
func simpleViewNext(mode int) int {
	return (mode + 1) % 3
}

// cycleSimpleView 处理 VIEW：循环顔/胸/股間三档视点。
//
// 优先走 [free-sight screen]（MKIE 配了 mode = 1）：这个模式下立绘会被
// 合成器按 ViewMode 裁切/滚动，和原版的"镜头推近"一致。没有自由视角配置
// 的作品才退回 Rev5 的 cs 视点切换。
func (s *Session) cycleSimpleView() bool {
	if s.gameState == nil {
		return false
	}
	if s.config != nil && s.config.FreeSight.Mode {
		s.freeSightActive = true
		s.gameState.SetViewMode(simpleViewNext(s.gameState.ViewMode()))
		s.composer.Invalidate()
		return true
	}
	if s.bsStage != nil && s.bsStage.CycleLegacyView() {
		s.composer.Invalidate()
		return true
	}
	return false
}

// simpleViewOn 报告 VIEW 按钮是否该显示"已启用"帧。
//
// ViewMode 的 0 是默认的「顔」档，和"没按过 VIEW"同值；只有 1/2（胸/股間）
// 才算玩家真的切过视点，按钮显示第 2 帧。
func (s *Session) simpleViewOn() bool {
	return s.gameState != nil && s.gameState.ViewMode() != 0
}

// nextBUStrip 返回下一个脱衣阶段。
//
// MKIE 的 bu 立绘只存在 0（着衣）/ 3（下着）/ 5（全裸）三档素材
// （统计 1027 个 bu 名，7 个角色全部如此）。不在这三档里的名字（例如
// 鉴赏室遗留的自定义状态）一律回到 0，避免拼出永远不会存在的图名。
func nextBUStrip(stage byte) byte {
	switch stage {
	case '0':
		return '3'
	case '3':
		return '5'
	default:
		return '0'
	}
}

// gameMenuToggleOn 让工具条上的状态按钮画"启用"帧。
//
// 目前只有 VIEW/STRIP 是 4 帧状态按钮；其它按钮（SKIP/VOICE 等）
// 仍按点击/悬停两态画，不能把它们的 frame 加 2。
func (s *Session) gameMenuToggleOn(key string) bool {
	switch key {
	case "simple view":
		return s.simpleViewOn()
	case "simple strip":
		stage, ok := s.currentBUStrip()
		return ok && stage != '0'
	}
	return false
}

// currentBUStrip 返回当前舞台上半身立绘的脱衣阶段。
//
// 只看角色层：地图按钮、背景层也叫 bu* 的概率很低，但用 IsBustupLayerName
// 卡住 bu + 两位数字，避免把无关图层当成立绘去改。
func (s *Session) currentBUStrip() (byte, bool) {
	if s.gameState == nil {
		return 0, false
	}
	for _, layer := range s.gameState.Layers() {
		// 不按 layer.Kind() 过滤：MKIE 的剧本用 IMAGE 把 bu 合成图贴在前景层，
		// 图层种类是 Screen 而不是 Character，按种类筛会漏掉真正的立绘。
		name := layer.Name()
		if len(name) < 10 || !state.IsBustupLayerName(name) {
			continue
		}
		stage := name[5]
		if stage < '0' || stage > '9' {
			continue
		}
		return stage, true
	}
	return 0, false
}

// toggleSimpleStrip 处理 STRIP：把当前 bu 立绘切到下一脱衣阶段。
//
// 只改名字里的脱衣位（第 6 个字符），角色/服装/姿势/表情都不动，
// 然后用 state.BustupResolve 重新拆出身体/头发/表情部件。
// 某一档没有素材时继续试下一档，避免按一次 STRIP 立绘直接消失。
func (s *Session) toggleSimpleStrip() bool {
	if s.gameState == nil {
		return false
	}
	for plane, layer := range s.gameState.Layers() {
		name := layer.Name()
		if len(name) < 10 || !state.IsBustupLayerName(name) {
			continue
		}
		if strings.HasPrefix(name, "buex") {
			continue
		}
		x, y := 0, 0
		if artworks := layer.Artworks(); len(artworks) > 0 {
			x, y = artworks[0].X, artworks[0].Y
		}
		stage := name[5]
		for attempt := 0; attempt < 3; attempt++ {
			stage = nextBUStrip(stage)
			candidate := []byte(name)
			candidate[5] = stage
			body := string(candidate[:10])
			if !s.res.ContainsExact(body) {
				continue
			}
			artworks := state.BustupResolve(string(candidate), x, y,
				s.config.EyePattern, sessionResourceIndex{index: s.res}, s.config.HairSuffix)
			if len(artworks) == 0 {
				continue
			}
			s.gameState.SetLayerAt(plane, string(candidate), layer.Kind(), artworks)
			s.composer.Invalidate()
			return true
		}
	}
	return false
}
