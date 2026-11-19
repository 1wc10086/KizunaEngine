// 鉴赏覆盖层（CG/音乐/场景 + CG 全屏）的自绘与命中。
//
// 为什么由引擎自绘：页签 cg_btn1/2/3、cd_return、cg_base/cg_page、
// 缩略图 thumb_、音乐 bgm_btn、场景 scene_base/sce_bg 都来自原版
// sq_extra_*.nut（见 tmpwork/ui_exact_spec.md）。Flutter 只送点击坐标，
// 热区与贴图必须用同一份 ExtraLayout 几何，否则错位点不到。
//
// 模态不变式：命中一律返回 hit=true，点空也 true——覆盖层盖住剧情，
// 点击绝不穿透到推进对白的 Advance。
package session

import (
	"fmt"
	"strings"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/arc"
	overaudio "kizuna/enginego/brand/overdose/audio"
	"kizuna/enginego/brand/overdose/menus"
	"kizuna/enginego/core"
)

// composeExtraOverlay 按当前页签画背景、页签/返回与对应网格。
func (s *Session) composeExtraOverlay(canvas *bishopgfx.BgraCanvas) {
	layout := menus.ExtraLayoutFor()
	switch s.extraTab {
	case galleryMusic:
		s.drawImage(canvas, layout.MusicBackground, 0, 0)
	case galleryScene:
		// 场景背景按章节换整图 sce_bg%d（extraChara+1），缺图时退回 scene_base 框架。
		name := fmt.Sprintf("sce_bg%d", s.extraChara+1)
		if !s.drawImage(canvas, name, 0, 0) {
			s.drawImage(canvas, layout.SceneBase, 0, 0)
		}
	default:
		s.drawImage(canvas, layout.CgBackground, 0, 0)
	}

	// 三页签：当前页签用 -o 选中态（BUTTON_SELECT）。
	s.drawButton(canvas, layout.CgTab.Image, layout.CgTab.Rect, s.extraTab == galleryCG)
	s.drawButton(canvas, layout.SceneTab.Image, layout.SceneTab.Rect, s.extraTab == galleryScene)
	s.drawButton(canvas, layout.MusicTab.Image, layout.MusicTab.Rect, s.extraTab == galleryMusic)
	s.drawImage(canvas, layout.Return, layout.ReturnRect.X, layout.ReturnRect.Y)

	switch s.extraTab {
	case galleryMusic:
		s.composeExtraMusic(canvas, layout)
	case galleryScene:
		s.composeExtraScene(canvas, layout)
	default:
		s.composeExtraCG(canvas, layout)
	}
}

// cgCellRect 是 CG 网格第 i 格的命中/底图矩形：56+140*(i%5), 82+110*(i/5)。
// 尺寸 132x100 与底图同：命中只认看得见的区域，不含格间留白。
func cgCellRect(layout menus.ExtraLayout, i int) menus.Rect {
	return menus.Rect{
		X: layout.CGGridX + (i%layout.CGCols)*layout.CGCellW,
		Y: layout.CGGridY + (i/layout.CGCols)*layout.CGCellH,
		W: 132, H: 100,
	}
}

// composeExtraCG 画人物页签与 20 格底图/缩略图。
func (s *Session) composeExtraCG(canvas *bishopgfx.BgraCanvas, layout menus.ExtraLayout) {
	// 人物页签 cg_page1..5：当前人物（extraChara）用 -o。
	for i := 0; i < 5; i++ {
		s.drawButton(canvas, fmt.Sprintf("%s%d", layout.PagePrefix, i+1), layout.PageRects[i], s.extraChara == i)
	}
	names := s.cgPageNames(s.extraChara + 1)
	for i := 0; i < layout.CGPerPage; i++ {
		r := cgCellRect(layout, i)
		s.drawImage(canvas, fmt.Sprintf("%s%02d", layout.BasePrefix, i+1), r.X, r.Y)
		if i >= len(names) {
			continue
		}
		name := names[i]
		if !s.CgUnlockedByName(name) {
			continue
		}
		// 缩略图 128x96 偏移 (+2,+2)，名即 thumb_cga01-01 这类。
		s.drawImageAt(canvas, name, r.X+2, r.Y+2, 128, 96, 255)
	}
}

// composeExtraMusic 画 10 条 bgm_btn，当前播放用 -o。
func (s *Session) composeExtraMusic(canvas *bishopgfx.BgraCanvas, layout menus.ExtraLayout) {
	for i := 0; i < 10 && i < len(layout.MusicRects); i++ {
		img := fmt.Sprintf("%s%d", layout.MusicBtnPref, i+1)
		s.drawButton(canvas, img, layout.MusicRects[i], s.extraPlaying == i+1)
	}
}

// sceneRecords 返回当前章节页表行记录。scenePageRecords 在 scene.go；
// 表缺失时返回 nil，调用方按空页画。
func (s *Session) sceneRecords() []arc.Record {
	return s.scenePageRecords(s.extraChara + 1)
}

// sceneUnlockedByName 场景解锁查表：先用原名，失败去掉 thumb_ 前缀再查。
func (s *Session) sceneUnlockedByName(name string) bool {
	if s.unlockSets.scenes[name] {
		return true
	}
	return s.unlockSets.scenes[fullCGName(name)]
}

// sceneRecordUnlocked 按原版 _CheckSceneRead 判定：页表第 3 列起是“需要读完的
// 脚本文件”，全部跑过才解锁。没有检查列时退回按图名的解锁表。
func (s *Session) sceneRecordUnlocked(rec arc.Record) bool {
	checks := rec.Cells
	if len(checks) > 3 {
		any := false
		all := true
		for _, fname := range checks[3:] {
			if fname == "" {
				continue
			}
			any = true
			stem := strings.TrimSuffix(fname, ".asb")
			if !s.unlockSets.set["script:"+stem] {
				all = false
				break
			}
		}
		if any {
			return all
		}
	}
	return s.sceneUnlockedByName(rec.Name)
}

// composeExtraScene 画章节背景已由页签分支完成，这里画翻页与 12 格。
func (s *Session) composeExtraScene(canvas *bishopgfx.BgraCanvas, layout menus.ExtraLayout) {
	// 左右翻页：原版 _IsFirstPage 只在 chara==0 时隐藏左键，
	// _IsLastPage 只在 18 时隐藏右键；中间章节两个都画。
	if s.extraChara > 0 {
		s.drawImage(canvas, layout.Prev, layout.PrevRect.X, layout.PrevRect.Y)
	}
	if s.extraChara < 18 {
		s.drawImage(canvas, layout.Next, layout.NextRect.X, layout.NextRect.Y)
	}
	recs := s.sceneRecords()
	for i := 0; i < layout.ScenePerPage; i++ {
		x := layout.SceneGridX + (i%layout.SceneCols)*layout.SceneCellW
		y := layout.SceneGridY + (i/layout.SceneCols)*layout.SceneCellH
		s.drawImage(canvas, layout.SceneBase, x, y)
		if i >= len(recs) {
			continue
		}
		name := recs[i].Name
		if name == "" || !s.sceneRecordUnlocked(recs[i]) {
			continue
		}
		// 场景缩略图 132x100 原尺寸（与底图同尺寸，不缩放）。
		s.drawImage(canvas, name, x, y)
	}
}

// pointerExtraOverlay 处理鉴赏页点击。所有点击命中返回 true（模态不穿透）。
func (s *Session) pointerExtraOverlay(x, y int32) (core.Step, bool) {
	layout := menus.ExtraLayoutFor()

	// return 回标题：closeOverlay 按 atTitle/overlayReturn 回位。
	if hit(layout.ReturnRect, x, y) {
		return s.closeOverlay(), true
	}
	// 三页签切换只改 extraTab/extraPage（人物游标保持，不重置）。
	if hit(layout.CgTab.Rect, x, y) {
		s.extraTab = galleryCG
		s.extraPage = 0
		return s.step, true
	}
	if hit(layout.SceneTab.Rect, x, y) {
		s.extraTab = galleryScene
		s.extraPage = 0
		return s.step, true
	}
	if hit(layout.MusicTab.Rect, x, y) {
		s.extraTab = galleryMusic
		s.extraPage = 0
		return s.step, true
	}

	switch s.extraTab {
	case galleryMusic:
		s.pointerExtraMusic(layout, x, y)
	case galleryScene:
		if step, ok := s.pointerExtraScene(layout, x, y); ok {
			return step, true
		}
	default:
		s.pointerExtraCG(layout, x, y)
	}
	return s.step, true
}

// pointerExtraCG 处理 CG 页点击：人物页签切换 / 已解锁格开全屏。
func (s *Session) pointerExtraCG(layout menus.ExtraLayout, x, y int32) {
	for i := 0; i < 5; i++ {
		if hit(layout.PageRects[i], x, y) {
			s.extraChara = i
			s.extraPage = 0
			return
		}
	}
	names := s.cgPageNames(s.extraChara + 1)
	for i := 0; i < layout.CGPerPage && i < len(names); i++ {
		if !hit(cgCellRect(layout, i), x, y) {
			continue
		}
		name := names[i]
		if !s.CgUnlockedByName(name) {
			return
		}
		s.extraViewNames = []string{fullCGName(name)}
		s.extraViewIndex = 0
		s.overlay = overlayCGView
		return
	}
}

// pointerExtraMusic 点击播放 BGM（循环满量），并记解锁。
// 按钮 i 对应 bgm%02d(i+1)，不按 music.arc 的条目顺序——原版就是这么映射的，
// 档案索引顺序（bgm04 在前）会把标签和实际曲目对错。
func (s *Session) pointerExtraMusic(layout menus.ExtraLayout, x, y int32) {
	for i := 0; i < 10; i++ {
		if !hit(layout.MusicRects[i], x, y) {
			continue
		}
		name := fmt.Sprintf("bgm%02d", i+1)
		if s.audio != nil {
			s.audio.Play(overaudio.PlayRequest{
				Channel: overaudio.Channel{Kind: overaudio.CategoryMusic, Idx: 0},
				Name:    name,
				Loop:    true,
				Volume:  100,
			})
		}
		s.extraPlaying = i + 1
		s.unlock("bgm", name)
		return
	}
}

// pointerExtraScene 处理场景页点击：翻页 / 已解锁格回放。
func (s *Session) pointerExtraScene(layout menus.ExtraLayout, x, y int32) (core.Step, bool) {
	if s.extraChara > 0 {
		if hit(layout.PrevRect, x, y) {
			s.extraChara--
			return s.step, true
		}
	}
	if s.extraChara < 18 {
		if hit(layout.NextRect, x, y) {
			s.extraChara++
			return s.step, true
		}
	}
	recs := s.sceneRecords()
	for i := 0; i < layout.ScenePerPage && i < len(recs); i++ {
		r := menus.Rect{
			X: layout.SceneGridX + (i%layout.SceneCols)*layout.SceneCellW,
			Y: layout.SceneGridY + (i/layout.SceneCols)*layout.SceneCellH,
			W: 132, H: 100,
		}
		if !hit(r, x, y) {
			continue
		}
		name := recs[i].Name
		if name == "" || !s.sceneRecordUnlocked(recs[i]) {
			break
		}
		// 调用前先退覆盖层，让回放脚本接管画面。
		s.overlay = overlayNone
		step, err := s.replayScene(s.extraChara+1, i)
		if err != nil {
			s.lastError = err.Error()
			return core.StepFaulted, true
		}
		return step, true
	}
	return s.step, true
}

// composeCGViewOverlay 把当前全屏 CG 缩放到 800x600 黑底。
func (s *Session) composeCGViewOverlay(canvas *bishopgfx.BgraCanvas) {
	canvas.Clear(0, 0, 0, 255)
	if s.extraViewIndex < 0 || s.extraViewIndex >= len(s.extraViewNames) {
		return
	}
	name := s.extraViewNames[s.extraViewIndex]
	if name == "" {
		return
	}
	s.drawImageAt(canvas, name, 0, 0, frameWidth, frameHeight, 255)
}

// pointerCGViewOverlay 左键下一张；越界回 CG 鉴赏页（原版 task.Pop），
// 而不是回标题。右键由全局 rightClickAt 直接关闭覆盖层。
func (s *Session) pointerCGViewOverlay(x, y int32) (core.Step, bool) {
	s.extraViewIndex++
	if s.extraViewIndex >= len(s.extraViewNames) {
		s.overlay = overlayExtra
		s.extraTab = galleryCG
		s.step = core.StepAwaitTitle
		return s.step, true
	}
	return s.step, true
}
