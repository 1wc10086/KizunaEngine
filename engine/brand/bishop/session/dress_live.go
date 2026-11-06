package session

import (
	"fmt"
	"strconv"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
)

// dress_live.go 把 DRESS 画面选好的服装/脱衣阶段应用到正常剧情的立绘。
//
// 手册明确写了「通常画面で表示されるキャラクターの立ち絵を下着姿や全裸に
// 変更」，所以这一屏不能只做预览。原版不把换装结果写进存档（读档后回到
// 剧本原本的立绘），但当前会话里后续再画同一角色时要保持玩家选的状态。
//
// 立绘图名是固定格式：
//
//	bu<NN><u|f><脱衣阶段><服装两位><姿势><变体><表情两位>
//	bu01f0001a01  = 1 号、半身、阶段0、服装00、姿势1、变体a、表情01
//
// DRESS 的服装表给的是服装编号和阶段号，所以只要替换第 5、6-7 位即可；
// 姿势/变体/表情保留原剧本的值，避免把表情换成另一张脸。新名字没有素材时
// 依次退到「阶段0」「服装00」，都不存在就保留原图，绝不把立绘弄丢。

// dressLiveChoice 是某个角色在 DRESS 里选中的服装/脱衣阶段。
type dressLiveChoice struct {
	wear  int
	stage int
}

// sessionResourceIndex 把 *resource.GameResourceIndex 适配成
// state.ResourceIndex。resource 的方法返回 *ResourceLocation，而接口要求
// ResourceLocation，Go 的方法签名不协变，所以需要这层薄适配。
type sessionResourceIndex struct {
	index *resource.GameResourceIndex
}

func (a sessionResourceIndex) ContainsExact(name string) bool {
	if a.index == nil {
		return false
	}
	return a.index.ContainsExact(name)
}

func (a sessionResourceIndex) TryGet(name string) (state.ResourceLocation, bool) {
	if a.index == nil {
		return nil, false
	}
	location, ok := a.index.TryGet(name)
	if !ok || location == nil {
		return nil, false
	}
	return location, true
}

func (a sessionResourceIndex) ArchivesInCategory(category string) []resource.Archive {
	if a.index == nil {
		return nil
	}
	return a.index.ArchivesInCategory(category)
}

// dressStandeeName 按当前选择拼出一个存在的立绘图名。
func (s *Session) dressStandeeName(name string, choice dressLiveChoice) string {
	if len(name) < 12 || name[0] != 'b' || name[1] != 'u' {
		return ""
	}
	stage := clampInt(choice.stage, 0, 9)
	wear := clampInt(choice.wear, 0, 99)
	// 依次尝试：目标组合、阶段0、服装00、两者都0。
	for _, candidate := range []struct{ stage, wear int }{
		{stage, wear},
		{0, wear},
		{stage, 0},
		{0, 0},
	} {
		body := fmt.Sprintf("bu%s%c%d%02d%c%c",
			name[2:4], name[4],
			candidate.stage, candidate.wear,
			name[8], name[9])
		if len(name) > 10 {
			body += name[10:]
		}
		exists := s.composer != nil && s.composer.Load(body[:10]) != nil
		if !exists && s.res != nil {
			_, exists = s.res.TryGet(body[:10])
		}
		if exists {
			return body
		}
	}
	return ""
}

// applyDressToBustupName 是给 StageDirector 的覆盖函数。
func (s *Session) applyDressToBustupName(name string) string {
	// 満淫電車2：立绘名的最后一位是着衣阶段（0 着衣/1 下着/2 全裸），
	// 例如 bu01f0011a0。它和 dressLive 的服装/阶段位不是同一套命名，
	// 必须优先按最后一位替换。
	if s.md2DressStage >= 0 && len(name) >= 11 && (name[0] == 'b' && name[1] == 'u') {
		last := name[len(name)-1]
		if last >= '0' && last <= '9' && int(last-'0') != s.md2DressStage {
			candidate := name[:len(name)-1] + string(byte('0'+s.md2DressStage))
			if s.resourceExists(candidate) {
				return candidate
			}
		}
	}
	if len(s.dressLive) == 0 {
		return name
	}
	// 牝教師2 老版的立绘名是 bu<角色2><服装2><阶段2><表情2>[部件2]，
	// 没有 UKIE 的 'u/f' 段。只替换服装/阶段两段，保留表情与部件。
	if len(name) >= 10 && name[0] == 'b' && name[1] == 'u' && allDigits(name[2:10]) {
		character, err := strconv.Atoi(name[2:4])
		if err == nil {
			if choice, ok := s.dressLive[character]; ok {
				suffix := name[10:]
				for _, candidate := range []struct{ stage, wear int }{
					{choice.stage, choice.wear},
					{0, choice.wear},
					{choice.stage, 0},
					{0, 0},
				} {
					next := fmt.Sprintf("bu%02d%02d%02d%s",
						character,
						clampInt(candidate.wear, 0, 99),
						clampInt(candidate.stage, 0, 3)*10,
						suffix)
					if s.resourceExists(next) {
						return next
					}
				}
			}
		}
		return name
	}
	if len(name) < 12 {
		return name
	}
	character, err := strconv.Atoi(name[2:4])
	if err != nil || character < 1 {
		return name
	}
	choice, ok := s.dressLive[character]
	if !ok {
		return name
	}
	if candidate := s.dressStandeeName(name, choice); candidate != "" {
		return candidate
	}
	return name
}

// syncDressLive 把 DRESS 画面当前状态同步到会话，并刷新已经在舞台上的立绘。
func (s *Session) syncDressLive() {
	if s.ui == nil || s.ui.dress == nil {
		return
	}
	live := make(map[int]dressLiveChoice, len(s.ui.dress.perCharacter))
	for index, entry := range s.ui.dress.perCharacter {
		if !entry.set {
			continue
		}
		live[index+1] = dressLiveChoice{
			wear:  s.dressWearForCharacter(index),
			stage: entry.strip,
		}
	}
	s.dressLive = live
	s.applyDressLiveToStage()
}

// applyDressLiveToStage 重新解析舞台上的角色层。
func (s *Session) applyDressLiveToStage() {
	if s.gameState == nil || s.composer == nil || s.config == nil || s.res == nil {
		return
	}
	resources := sessionResourceIndex{index: s.res}
	changed := false
	for _, layer := range s.gameState.Layers() {
		if layer.Kind() != state.StageLayerCharacter {
			continue
		}
		name := layer.Name()
		next := s.applyDressToBustupName(name)
		if next == name {
			continue
		}
		pieces := layer.Artworks()
		x, y, frames := 0, 0, 1
		if len(pieces) > 0 {
			x, y = pieces[0].X, pieces[0].Y
			if pieces[0].Frames > 0 {
				frames = pieces[0].Frames
			}
		}
		artworks := state.BustupResolve(next, x, y, frames, resources, s.config.HairSuffix)
		s.gameState.SetLayerAt(layer.Id(), next, state.StageLayerCharacter, artworks)
		changed = true
	}
	if changed {
		s.composer.Invalidate()
	}
}

// applyMD2DressStage 把全局着衣阶段应用到已经在舞台上的立绘。
//
// 满淫电车2 的立绘名末位是阶段号；只改最后一个字符比重新解析整套
// bustup 更快，而且不会动表情/姿势/服装。找不到对应素材时保留原图，
// 绝不把立绘弄丢。
func (s *Session) applyMD2DressStage() {
	if s.gameState == nil || s.md2DressStage < 0 {
		return
	}
	stage := byte('0' + s.md2DressStage)
	for _, layer := range s.gameState.Layers() {
		artworks := layer.Artworks()
		for index, artwork := range artworks {
			name := artwork.Image
			if len(name) < 11 || name[0] != 'b' || name[1] != 'u' {
				continue
			}
			last := name[len(name)-1]
			if last < '0' || last > '9' || last == stage {
				continue
			}
			candidate := name[:len(name)-1] + string(stage)
			if !s.resourceExists(candidate) {
				continue
			}
			artwork.Image = candidate
			layer.ReplaceArtwork(index, artwork)
		}
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}
