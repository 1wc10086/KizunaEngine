package gothic

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
)

// gothic_toku1_ui.go 放 Toku1（特別病棟）专用的标题与界面几何。
//
// Toku1 的标题按钮烘焙在 title01 里，坐标与 MAZE 不同，所以这里单独给出
// 矩形；MAZE 路径仍走 titleItemRects，不经过这里的分支。

// isToku1Title 判断是否走 Toku1 的烘焙标题。title01 缺失时（部分解包目录）
// 退回普通菜单路径，避免标题界面一片空白。
func (s *Session) isToku1Title() bool {
	return s.profile == gothicProfileToku1 && s.catalog != nil && s.catalog.Resolve("title01") != nil
}

// titleRects 是标题按钮的命中与高亮矩形，和 title02 的红色下划线对齐。
func (s *Session) titleRects() [][4]int {
	if s.profile != gothicProfileToku1 {
		return titleItemRects()
	}
	rects := make([][4]int, 5)
	for i := range rects {
		rects[i] = [4]int{456, 145 + 50*i, 170, 36}
	}
	return rects
}

// toku1TitleItems 返回 Toku1 标题的五个按钮。读档项只在存在存档时可用，
// 与 MAZE 的 anySave 判断保持一致。
func toku1TitleItems(anySave bool) []core.TitleMenuItem {
	labels := []string{"はじめから", "つづきから", "環境設定", "鑑賞室", "終了"}
	keys := []string{"start", "load", "config", "museum", "quit"}
	items := make([]core.TitleMenuItem, 0, len(keys))
	for i := range keys {
		items = append(items, core.TitleMenuItem{
			Key:     keys[i],
			Label:   labels[i],
			X:       456,
			Y:       int32(145 + 50*i),
			Width:   170,
			Height:  36,
			Enabled: keys[i] != "load" || anySave,
		})
	}
	return items
}

// ---- Toku1 鉴赏室/设置布局 ----

const toku1GalleryPerPage = 20

// toku1GalleryCellRect 是 album0/kaisou1 的 5x4 网格。
// 实测黑格：x=15+105*c，y=40+85*r，94x68。
func toku1GalleryCellRect(index int) [4]int {
	column := index % 5
	row := index / 5
	return [4]int{15 + column*105, 40 + row*85, 94, 68}
}

func (s *Session) galleryPerPage() int {
	if s.profile == gothicProfileToku1 {
		return toku1GalleryPerPage
	}
	return 12
}

func (s *Session) galleryCellRect(index int) [4]int {
	if s.profile == gothicProfileToku1 {
		return toku1GalleryCellRect(index)
	}
	return albumCellRect(index)
}

// toku1SceneLabels 返回剧本里可回放的 hNscene 标签。
//
// Toku1 的回想不是固定 H1..H21，而是脚本自己定义的 h1scene/h10scene…
// 直接扫描标签表，避免对不存在的场景调用 Call 把 VM 打停。
func (s *Session) toku1SceneLabels() []string {
	if s.program == nil {
		return nil
	}
	var out []string
	for _, statement := range s.program.Statements() {
		if statement.Kind != vmgothic.GothicStatementLabel {
			continue
		}
		lower := strings.ToLower(statement.Name)
		if strings.HasPrefix(lower, "h") && strings.HasSuffix(lower, "scene") {
			out = append(out, lower)
		}
	}
	// 标签顺序按脚本出现顺序，原版回想也是这个顺序。
	return out
}

func (s *Session) kaisouPageCount() int {
	if s.profile == gothicProfileToku1 {
		labels := s.toku1SceneLabels()
		if len(labels) == 0 {
			return 1
		}
		return (len(labels) + toku1GalleryPerPage - 1) / toku1GalleryPerPage
	}
	return 2
}

// toku1MusicCellRect 是 music0 的 13 根竖条。x=30+39*i，y=59，27x278。
// slotCellRectForCount 给 24 槽的 Toku1 用 6x4 紧凑网格；
// MAZE 仍是 15 槽的 3x5 大格。原实现固定 3x8，24 槽会掉出屏幕底部。
func (s *Session) slotCellRect(index int) [4]int {
	if s.saveSlotCount > 15 {
		column := index % 6
		row := index / 6
		return [4]int{10 + column*104, 50 + row*96, 100, 92}
	}
	return slotCellRect(index)
}

func toku1MusicCellRect(index int) [4]int {
	return [4]int{30 + index*39, 59, 27, 278}
}

func (s *Session) musicCellCount() int {
	if s.profile == gothicProfileToku1 {
		return 13
	}
	return 10
}

func (s *Session) musicCellRectFor(index int) [4]int {
	if s.profile == gothicProfileToku1 {
		return toku1MusicCellRect(index)
	}
	return musicCellRect(index)
}

// toku1OptionItems 用 option0 的实测单选点坐标。
func (s *Session) toku1OptionItems() []gothicUIHit {
	return []gothicUIHit{
		{action: "opt.bgm.on", label: "音楽あり", x: 66, y: 62, w: 34, h: 34},
		{action: "opt.bgm.off", label: "音楽なし", x: 66, y: 174, w: 34, h: 34},
		{action: "opt.se.on", label: "効果音あり", x: 130, y: 62, w: 34, h: 34},
		{action: "opt.se.off", label: "効果音なし", x: 130, y: 174, w: 34, h: 34},
		{action: "opt.voice.on", label: "音声あり", x: 195, y: 62, w: 34, h: 34},
		{action: "opt.voice.off", label: "音声なし", x: 195, y: 174, w: 34, h: 34},
		{action: "opt.skip.on", label: "既読あり", x: 260, y: 62, w: 34, h: 34},
		{action: "opt.skip.off", label: "既読なし", x: 260, y: 174, w: 34, h: 34},
		{action: "opt.text.0", label: "速い", x: 330, y: 62, w: 34, h: 34},
		{action: "opt.text.1", label: "普通", x: 330, y: 230, w: 34, h: 34},
		{action: "opt.text.2", label: "遅い", x: 330, y: 311, w: 34, h: 34},
		{action: "opt.effect.on", label: "エフェクトあり", x: 395, y: 62, w: 34, h: 34},
		{action: "opt.effect.off", label: "エフェクトなし", x: 395, y: 174, w: 34, h: 34},
		{action: "opt.res.full", label: "フルスクリーン", x: 461, y: 62, w: 34, h: 34},
		{action: "opt.res.window", label: "ウィンドウ", x: 461, y: 254, w: 34, h: 34},
		{action: "opt.juice", label: "汁設定", x: 41, y: 392, w: 124, h: 61},
		{action: "opt.quit", label: "ゲーム終了", x: 350, y: 419, w: 121, h: 34},
		{action: "opt.exit", label: "戻る", x: 491, y: 419, w: 90, h: 34},
	}
}

// toku1ReplayScene 回放 hNscene 标签。
//
// 和 MAZE 的 HnSCENE 一样：先抓 VM/变量快照，再 Call；场景播完的 #return
// 会把调用深度降回基线，run 检测到后调用 finishReplay 恢复主线。
func (s *Session) toku1ReplayScene(label string) (core.Step, bool) {
	if s.machine == nil || strings.TrimSpace(label) == "" {
		return core.StepContinue, false
	}
	s.replaySnapshot = s.machine.Snapshot()
	s.replayVars = s.machine.Variables().Snapshot()
	s.replayDepth = s.machine.CallDepth()
	if !s.machine.Call(label) {
		return core.StepContinue, false
	}
	s.replayMode = true
	s.replayPage = s.ui.page
	s.ui = nil
	s.stage.Reset()
	s.gameState.Reset()
	s.clearMessage()
	step, _ := s.run(gothicDefaultRunBudget)
	return step, true
}

// drawToku1Album 画 album0 的 5x4 CG 鉴赏网格。
func (s *Session) drawToku1Album(canvas *gfx.BgraCanvas) {
	if s.ui.pageSelect {
		s.drawUIFullImage(canvas, "album1")
		if s.ui.hover >= 0 {
			hits := s.uiItems()
			if s.ui.hover < len(hits) {
				h := hits[s.ui.hover]
				s.drawUIBorder(canvas, h.x, h.y, h.w, h.h, true)
			}
		}
		return
	}
	s.drawUIFullImage(canvas, "album0")
	start := s.ui.page * toku1GalleryPerPage
	for i := 0; i < toku1GalleryPerPage; i++ {
		position := start + i
		rect := toku1GalleryCellRect(i)
		if position >= len(s.cgNames) {
			canvas.Fill(rect[0]+3, rect[1]+3, rect[2]-6, rect[3]-6, 0, 0, 0, 255)
			continue
		}
		image := s.galleryImage(s.cgNames[position])
		if image != nil {
			canvas.BlitScaled(image, rect[0]+3, rect[1]+3, rect[2]-6, rect[3]-6, 255)
		} else {
			canvas.Fill(rect[0]+3, rect[1]+3, rect[2]-6, rect[3]-6, 0, 0, 0, 255)
		}
		if s.ui.hover == i+3 {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
	}
	// album0 左下角烘焙的是“1 / 11”；翻页由页码选择页完成，
	// 底图上的数字保持原样（原版用同一张按钮图，不做动态重绘）。
}

// drawToku1Kaisou 画 kaisou1 的 5x4 回想网格。
//
// 格子来自脚本里真实存在的 hNscene 标签；没有对应标签的格子保持黑底，
// 避免点击后 Call 失败停在半路。
func (s *Session) drawToku1Kaisou(canvas *gfx.BgraCanvas) {
	s.drawUIFullImage(canvas, "kaisou1")
	labels := s.toku1SceneLabels()
	start := s.ui.page * toku1GalleryPerPage
	for i := 0; i < toku1GalleryPerPage; i++ {
		rect := toku1GalleryCellRect(i)
		position := start + i
		if position >= len(labels) {
			canvas.Fill(rect[0]+3, rect[1]+3, rect[2]-6, rect[3]-6, 0, 0, 0, 255)
			continue
		}
		preview := ""
		if s.program != nil {
			preview = s.program.FindScenePreviewImage(labels[position])
		}
		if preview == "" && position < len(s.cgNames) {
			preview = s.cgNames[position]
		}
		if image := s.galleryImage(preview); image != nil {
			canvas.BlitScaled(image, rect[0]+3, rect[1]+3, rect[2]-6, rect[3]-6, 255)
		} else {
			canvas.Fill(rect[0]+3, rect[1]+3, rect[2]-6, rect[3]-6, 0, 0, 0, 255)
		}
		if s.ui.hover == i+1 {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
	}
}

// drawToku1Music 画 music0 的 13 根竖条高亮。
func (s *Session) drawToku1Music(canvas *gfx.BgraCanvas) {
	s.drawUIFullImage(canvas, "music0")
	for i := 0; i < 13; i++ {
		rect := toku1MusicCellRect(i)
		if s.ui.playingTrack == i || s.ui.hover == i+1 {
			s.drawUIBorder(canvas, rect[0], rect[1], rect[2], rect[3], true)
		}
	}
}

// drawToku1Option 在 option0 的空心单选点上补实心标记。
func (s *Session) drawToku1Option(canvas *gfx.BgraCanvas) {
	s.drawUIFullImage(canvas, "option0")
	if s.global == nil {
		return
	}
	mark := func(x, y int) {
		canvas.Fill(x+4, y+4, 9, 9, 230, 220, 90, 255)
	}
	if s.global.BgmOn {
		mark(74, 70)
	} else {
		mark(74, 182)
	}
	if s.global.SeOn {
		mark(138, 70)
	} else {
		mark(138, 182)
	}
	if s.global.VoiceOn {
		mark(203, 70)
	} else {
		mark(203, 182)
	}
	if s.global.SkipRead {
		mark(268, 70)
	} else {
		mark(268, 182)
	}
	switch s.global.TextSpeed {
	case 0:
		mark(338, 70)
	case 1:
		mark(338, 238)
	default:
		mark(338, 319)
	}
	if s.global.Effects {
		mark(403, 70)
	} else {
		mark(403, 182)
	}
	// 原版没有保存"全屏/窗口"偏好，这里固定标フルスクリーン。
	mark(469, 70)
	if s.ui.hover >= 0 {
		hits := s.optionItems()
		if s.ui.hover < len(hits) {
			h := hits[s.ui.hover]
			s.drawUIBorder(canvas, h.x, h.y, h.w, h.h, true)
		}
	}
}

// toku1PageButtonRect 是 album1 页码选择页的 11 个按钮。
// 左列 6/11..1/11（从上到下），右列 11/11..7/11。
func toku1PageButtonRect(page int) [4]int {
	if page >= 1 && page <= 6 {
		return [4]int{45, 66 + (6-page)*65, 120, 35}
	}
	if page >= 7 && page <= 11 {
		return [4]int{195, 66 + (11-page)*65, 120, 35}
	}
	return [4]int{45, 66, 120, 35}
}

// drawDateCard 按 p_date1 的烘焙标签把日期/时间精灵填进空格。
//
// 位置来自 136x56 底图的空白区：第[ ]週 [ ]曜日 / [午前] [ ]時 [ ]分。
// 缺精灵时退回 GOTHIC2 字体，避免某些补丁删掉素材后整块空白。
func (s *Session) drawDateCard(canvas *gfx.BgraCanvas) {
	if canvas == nil || s.catalog == nil {
		return
	}
	// 地图底图（school1/school2）已经把 p_date1 的框和“第/週/曜日/時/分”
	// 烘焙在 (16,8) 处，所以这里只填精灵，不能再画一次底板（会错位成两层）。
	if s.dateWeek >= 1 && s.dateWeek <= 4 {
		if sprite := s.catalog.Resolve(fmt.Sprintf("p_week%d", s.dateWeek)); sprite != nil {
			canvas.Draw(sprite, 46, 12)
		}
	} else if s.dateWeek > 0 {
		s.drawUIText(canvas, fmt.Sprintf("%d", s.dateWeek), 44, 10, 1)
	}
	if s.dateDay >= 1 && s.dateDay <= 6 {
		if sprite := s.catalog.Resolve(fmt.Sprintf("p_day%d", s.dateDay)); sprite != nil {
			canvas.Draw(sprite, 88, 12)
		}
	} else if s.dateDay == 7 {
		s.drawUIText(canvas, "日", 88, 10, 1)
	}
	hour, minute, pm, clockOK := s.toku1Clock()
	if !clockOK {
		// 没有时间变量时退回 set date1 的后两个参数，避免整块空白。
		hour = s.dateHour
		minute = 0
		switch {
		case s.dateMinute == 1:
			minute = 20
		case s.dateMinute == 2:
			minute = 40
		default:
			minute = 0
		}
		pm = hour <= 5
	}
	// 12 小时制：上午用 p_am，下午/晚上用 p_pm。
	if pm {
		if sprite := s.catalog.Resolve("p_pm"); sprite != nil {
			canvas.Draw(sprite, 20, 39)
		}
	} else {
		if sprite := s.catalog.Resolve("p_am"); sprite != nil {
			canvas.Draw(sprite, 20, 39)
		}
	}
	if sprite := s.catalog.Resolve(fmt.Sprintf("p_hour%02d", hour)); sprite != nil {
		canvas.Draw(sprite, 66, 39)
	} else {
		s.drawUIText(canvas, fmt.Sprintf("%02d", hour), 64, 36, 2)
	}
	if sprite := s.catalog.Resolve(fmt.Sprintf("p_min%02d", minute)); sprite != nil {
		canvas.Draw(sprite, 109, 39)
	} else {
		s.drawUIText(canvas, fmt.Sprintf("%02d", minute), 105, 36, 2)
	}
}

// toku1Clock 把剧本的 ti（时间带）换算成 12 小时制。
//
// 说明书里的可移动时间带：11:40 / 12:00 / 12:20 / 12:40 /
// 15:20 / 15:40 / 16:00 / 16:20 / 16:40 / 17:00。p_hour/p_min
// 精灵正好覆盖这些值（00/01/03/04/05/08/10/11 与 00/20/40）。
func (s *Session) toku1Clock() (hour, minute int, pm, ok bool) {
	if s.machine == nil {
		return 0, 0, false, false
	}
	timeVariable := s.mapState.TimeVariable
	if timeVariable == "" {
		timeVariable = "ti"
	}
	slot := int(s.machine.Variables().Get(timeVariable))
	if slot <= 0 {
		return 0, 0, false, false
	}
	type slotTime struct{ hour, minute int }
	table := []slotTime{
		{11, 40}, {0, 0}, {0, 20}, {0, 40}, {3, 20},
		{3, 40}, {4, 0}, {4, 20}, {4, 40}, {5, 0},
		{5, 20}, {5, 40},
	}
	if slot > len(table) {
		slot = len(table)
	}
	entry := table[slot-1]
	return entry.hour, entry.minute, entry.hour <= 5, true
}

// Skip 推进到下一个需要玩家决定的节点（原版「次の選択肢まで進む」）。
// 停在选择肢、地图、标题、影片或结束时；上限 600 步防止坏脚本死循环。
func (s *Session) Skip() core.Step {
	if s.ui != nil || s.titleMode || s.mapState.Active != GothicMapNone {
		return s.currentStep()
	}
	step := core.StepAwaitInput
	for i := 0; i < 600; i++ {
		step, _ = s.Advance()
		switch step {
		case core.StepAwaitInput, core.StepWait:
			continue
		default:
			core.Logf("gothic: skip stopped at %d after %d steps", int(step), i+1)
			return step
		}
	}
	core.Logf("gothic: skip hit budget")
	return step
}
