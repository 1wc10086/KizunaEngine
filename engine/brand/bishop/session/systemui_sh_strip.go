package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
)

// systemui_sh_strip.go 是「すくーるヘブン らぶえろハーレム☆ももいろタイフーン
// 初回限定版」的立ち絵鑑賞页。
//
// 为什么需要单独一页：这一代的鉴赏布局全部编译在 sh.exe 里，发行目录没有
// museum_buex_layout，bsxx.cfg（本作没有）也没有 [museum phase - bu]，所以
// openMuseumMode("strip") 会一路退到 KD 的 sd_* 页、再退到脱衣设定，全都
// 不适用——最终"standing gallery unavailable"，玩家点立ち絵页签没有反应。
//
// 素材证据（alphagrid/BISHOP BSG 工具实测）：
//   - museum_ch1..8：237x208 的角色选择卡（椭圆头像+名字，单帧），恰好对应
//     [button object] 里 map_sakura..map_maria 的 8 名角色；
//   - 立绘本体是 cl{c}{w}{p}{d}（着衣 400x600）与 cu{c}{w}{p}{d}（脱衣
//     600x600）：c=角色 1..9、w=服装 1..2、p=姿势 1..2、d=差分 0..6/7；
//   - museum_prev/next（20x32 翻页箭头）、museum_arrow、museum_pageline。
//
// 原引擎的精确坐标在 sh.exe 里没有可核对的常量表，这里按"素材能讲的布局"
// 还原：选择页 2 列 x 4 行卡片网格全可见（不引入猜测的翻页），查看页立绘
// 居中、左右换差分、上下换服装/姿势。两态都复用 museum_menu 页签与戻る。

const (
	// shStripColumns/shStripRows 是选择页的网格。8 张卡按 2x4 全部摆下，
	// 不做翻页：卡片 237x208，800x600 里 2 列 + 4 行（行距 98，轻微重叠
	// 是原代卡片布局的常见样式，上下交叠能看清头像即可）。
	shStripColumns = 2
	shStripRows    = 4
	// shStripCount 是选择卡数量：museum_ch1..8。
	shStripCount = shStripColumns * shStripRows
)

// shStripCharacter 把卡片下标换算成脚本角色编号 1..8。
// museum_ch1..8 的编号与 cl 图的 c 段一一对应。
func shStripCharacter(index int) int {
	return index + 1
}

// shStripRect 返回选择页第 index 张卡的位置（237x208）。
func shStripRect(index int) (x, y, w, h int) {
	column := index % shStripColumns
	row := index / shStripColumns
	w, h = 237, 208
	x = (800 - shStripColumns*w - 40) / 2 * (column + 1) / (column + 1) // 保持可读
	x = 143 + column*277
	y = 88 + row*98
	return x, y, w, h
}

// shStripState 是这一代立绘鉴赏的运行状态。
type shStripState struct {
	selecting bool
	// character 是 1..8 的角色编号（与 museum_chN/clN 的编号一致）。
	character int
	// wear/pose/differential 是 cl{c}{w}{p}{d} 的三段可变下标。
	wear         int
	pose         int
	differential int
}

// shStripImage 拼当前查看的立绘图名。
//
// 命名是 cl{角色}{服装}{姿势}{差分两位}（cl11100..cl11107 = 角色1 服装1
// 姿势1 差分00..07；cl11205 = 姿势2 差分05；cl12100 = 服装2），差分段
// 固定两位数字，材料表里没有单位数写法。cu 是脱衣同构族，鉴赏页按
// cl 走（与选择卡 museum_ch 的编号一致）。
func (st *shStripState) shStripImage() string {
	return fmt.Sprintf("cl%d%d%d%02d", st.character, st.wear+1, st.pose+1, st.differential)
}

// openSHStripMuseum 按素材签名打开这一代专属的立绘鉴赏页。
//
// 只在 museum_ch1 存在（本代签名素材）时生效；其它作品不进入。
//
// 不能只看名字：牝教師3 也有 museum_ch1..7，但那是 CG/場景页的
// 64x267 三帧页签条（page icon0..6 = museum_ch1..7），不是 237x208
// 的角色选择卡。只按名字命中会把页签条整条当卡片画成两列重复头像，
// 就是"立ち絵鉴赏进去后满屏错位头像"的来源。尺寸是这两代唯一可靠的
// 区分：SH 的卡片 >= 200x180，MK3 的页签条只有 64 宽。
func (s *Session) openSHStripMuseum() bool {
	if s.ui == nil || s.composer == nil || !s.resourceExists("museum_ch1") {
		return false
	}
	card := s.composer.Load("museum_ch1")
	if !isSHStripCard(card) {
		return false
	}
	s.ui.kind = uiMuseumStrip
	s.ui.shStrip = &shStripState{selecting: true, character: 1}
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	return true
}

// isSHStripCard 判断 museum_ch1 是不是すくーるヘブン 的角色卡。
//
// SH 的 museum_ch1..8 是 237x208 单帧卡；牝教師3 的同名图是
// 64x267 的三帧页签条。尺寸判断让两条完全不同的界面不会互相误吃。
func isSHStripCard(image *gfx.BsgImage) bool {
	return image != nil && image.Width >= 200 && image.Height >= 180
}

// drawSHStripMuseum 绘制选择页/查看页。
func (s *Session) drawSHStripMuseum(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.shStrip == nil {
		return
	}
	s.drawImageAt(canvas, s.config.Museum.Base, 0, 0)
	state := ui.shStrip
	if state.selecting {
		for index := 0; index < shStripCount; index++ {
			x, y, _, _ := shStripRect(index)
			s.drawFrameButton(canvas, config.UIButton{
				Image:  fmt.Sprintf("museum_ch%d", shStripCharacter(index)),
				Frames: 1, X: x, Y: y,
			}, 0)
			if shStripCharacter(index) == state.character {
				// 选中卡描一圈青色边框（与 KD 页同一套画法）。
				canvas.Fill(x-2, y-2, 241, 2, 0, 200, 255, 255)
				canvas.Fill(x-2, y+206, 241, 2, 0, 200, 255, 255)
				canvas.Fill(x-2, y-2, 2, 208, 0, 200, 255, 255)
				canvas.Fill(x+237, y-2, 2, 208, 0, 200, 255, 255)
			}
		}
	} else {
		s.drawSHStripViewer(canvas, state)
	}
	s.drawMuseumTabs(canvas)
}

// drawSHStripMuseum 画查看页：立绘居中 + 差分/服装/姿势指示。
func (s *Session) drawSHStripViewer(canvas *gfx.BgraCanvas, state *shStripState) {
	image := s.composer.Load(state.shStripImage())
	if image == nil {
		// 差分不存在（有的角色只有 6 张差分）：显示 museum_empty 占位，
		// 玩家按箭头继续翻，不会卡死。
		s.drawFrameButton(canvas, config.UIButton{
			Image: "museum_empty", Frames: 1,
			X: (canvas.Width - 120) / 2, Y: (canvas.Height - 90) / 2,
		}, 0)
	} else {
		x := (canvas.Width - image.Width) / 2
		y := max(24, (canvas.Height-image.Height)/2)
		canvas.Blit(image, x, y, 255)
	}
	// 左右翻差分、上下换服装/姿势的箭头：museum_prev/next 是 20x32 单帧箭头。
	navY := canvas.Height - 44
	s.drawFrameButton(canvas, config.UIButton{Image: "museum_prev", Frames: 1, X: 24, Y: navY}, 0)
	s.drawFrameButton(canvas, config.UIButton{Image: "museum_next", Frames: 1, X: canvas.Width - 44, Y: navY}, 0)
	// 底部状态字（角色/服装/姿势/差分）：用引擎字体，绘制走 rendering.DrawText
	// 的公共管线（与 BGV 鉴赏的曲目列表同一套）。
	label := fmt.Sprintf("char %d  wear %d  pose %d  diff %d",
		state.character, state.wear+1, state.pose+1, state.differential+1)
	rendering.DrawText(canvas, s.textFont, label, 24, 24, 18, 1, 1, 22, 400, 2, 0xFFFFFFFF)
}

// hitSHStrip 命中检测：选择页的卡片、查看页的箭头、公共页签。
func (s *Session) hitSHStrip(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.shStrip == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if hit, ok := s.hitMuseumTabs(x, y); ok {
		return hit
	}
	state := ui.shStrip
	if state.selecting {
		for index := 0; index < shStripCount; index++ {
			bx, by, bw, bh := shStripRect(index)
			if x >= bx && y >= by && x < bx+bw && y < by+bh {
				return uiHit{kind: hitSHStripCard, index: index}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	navY := s.height - 44
	if y >= navY && y < navY+32 {
		if x >= 24 && x < 44 {
			return uiHit{kind: hitSHStripNav, index: 0}
		}
		if x >= s.width-44 && x < s.width-24 {
			return uiHit{kind: hitSHStripNav, index: 1}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateSHStrip 处理立绘鉴赏页的点击。
func (s *Session) activateSHStrip(hit uiHit) {
	ui := s.ui
	if ui == nil || ui.shStrip == nil {
		return
	}
	state := ui.shStrip
	switch hit.kind {
	case hitSHStripCard:
		// 点卡进查看态；再点同一张卡回选择态（原代没有独立返回按钮时的
		// 常见约定，触屏上也能用页签回主菜单）。
		character := shStripCharacter(hit.index)
		if !state.selecting && state.character == character {
			state.selecting = true
		} else {
			state.character = character
			state.selecting = false
			state.wear, state.pose, state.differential = 0, 0, 0
		}
	case hitSHStripNav:
		if hit.index == 0 {
			state.differential--
			if state.differential < 0 {
				state.differential = shStripMaxDifferential(s, state)
			}
		} else {
			state.differential++
			if s.composer.Load(fmt.Sprintf("cl%d%d%d%02d", state.character, state.wear+1, state.pose+1, state.differential)) == nil {
				state.differential = 0
			}
		}
	case hitMuseumMode:
		// 页签切换交给公共分支（activateSystemUI 的 hitMuseumMode 已处理）。
		return
	case hitMuseumReturn:
		if !state.selecting {
			state.selecting = true
		}
		return
	}
	s.composer.Invalidate()
}

// shStripMaxDifferential 返回当前角色/服装/姿势下可用的最大差分下标。
//
// 差分编号从 0 递增，遇到第一个不存在的图回退一步；素材缺失时钳到 0。
func shStripMaxDifferential(s *Session, state *shStripState) int {
	for d := 8; d >= 0; d-- {
		if s.composer.Load(fmt.Sprintf("cl%d%d%d%02d", state.character, state.wear+1, state.pose+1, d)) != nil {
			return d
		}
	}
	return 0
}
