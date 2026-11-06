package bdt

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
)

// bdt_manin_attack.go 实现原作的核心「言葉責め」与「責め言葉整理」。
//
// 言葉責め规则（来自官方手册）：
//   - 每轮从词库选 3 句，逐句播放；
//   - 反应 A..E 对应 +6 兴奋 / +4 / +2+2 / +2+4 / +6 压力；
//   - 兴奋 48 通关；压力 48 或轮数用尽/无词可用失败；
//   - 每次兴奋升级（每 12 点）时压力为 0 才点燃烧条，烧条 4 通关进おねだり。
//
// X30 的结果值与原脚本分支对应：1=通常H，2=おねだりH，3=压力满，
// 4=时间切れ，5=ネタ切れ。脚本 401-02..04 正好处理 3/4/5。

const (
	maninAttackRowsPerPage = 10
	maninOrganizerPageSize = 7
)

func (s *Session) drawManinAttack(canvas *gfx.BgraCanvas) {
	u := s.manin
	attack := &u.attack
	if attack.Phase == maninAttackExecute {
		s.drawManinAttackGauges(canvas)
		return
	}
	// 选词/确认时把场景压暗，突出手机和词表；底下的剧情帧仍保留作背景。
	canvas.FillBlend(0, 0, maninScreenWidth, maninScreenHeight, 8, 8, 16, 120)
	s.drawManinAttackGauges(canvas)
	s.drawManinAttackTarget(canvas)

	// 左：手机模组 + 词表（原作的操作主体）。
	if !s.maninDrawImage(canvas, "mobile", 8, 70) {
		canvas.Fill(8, 70, 208, 365, 60, 40, 20, 235)
	}
	canvas.Fill(24, 108, 176, 300, 40, 30, 20, 235)
	s.maninDrawText(canvas, "責め言葉", 11, 30, 112, 160, 1, 0xffffffff)
	pool := u.attackPool()
	pageCount := (len(pool) + maninAttackRowsPerPage - 1) / maninAttackRowsPerPage
	if pageCount < 1 {
		pageCount = 1
	}
	if attack.Page >= pageCount {
		attack.Page = pageCount - 1
	}
	if attack.Page < 0 {
		attack.Page = 0
	}
	for row := 0; row < maninAttackRowsPerPage; row++ {
		index := attack.Page*maninAttackRowsPerPage + row
		y := 128 + row*26
		if index >= len(pool) {
			break
		}
		selected := false
		for _, id := range attack.Selected {
			if id == pool[index] {
				selected = true
				break
			}
		}
		if selected {
			canvas.Fill(24, y, 176, 22, 60, 90, 40, 235)
		} else if u.hover == row {
			canvas.FillBlend(24, y, 176, 22, 60, 60, 120, 120)
		}
		s.drawManinRank(canvas, u.wordRank(pool[index]), 28, y+3)
		text := u.wordText(pool[index])
		s.maninDrawText(canvas, maninShorten(text, 13), 9, 48, y+5, 150, 1, 0xffeeeeee)
	}
	s.maninDrawText(canvas, strconv.Itoa(attack.Page+1)+"/"+strconv.Itoa(pageCount), 8, 150, 390, 60, 1, 0xffd0d0d0)

	// 右：実行文章 + 按钮。
	if image := s.maninImage("frame"); image != nil {
		canvas.BlitScaled(image, 232, 128, 380, 140, 255)
	} else {
		canvas.Fill(232, 128, 380, 140, 40, 30, 20, 235)
	}
	s.maninDrawText(canvas, "実行文章", 11, 244, 136, 160, 1, 0xffffffff)
	for i, id := range attack.Selected {
		s.maninDrawText(canvas, strconv.Itoa(i+1)+". "+u.wordText(id), 11, 244, 158+i*32, 356, 2, 0xffffb0c8)
	}
	if len(attack.Selected) == 0 {
		s.maninDrawText(canvas, "左の一覧から3つまで選択してください", 10, 244, 162, 350, 2, 0xffd0d0d0)
	}
	s.drawManinButton(canvas, 240, 282, 110, 30, "戻る")
	s.drawManinButton(canvas, 360, 282, 110, 30, "消去")
	s.drawManinButton(canvas, 480, 282, 110, 30, "進む")

	if attack.Phase == maninAttackConfirm {
		canvas.FillBlend(0, 0, 640, 480, 0, 0, 0, 140)
		canvas.Fill(160, 180, 320, 130, 40, 30, 20, 250)
		s.maninDrawText(canvas, "この3つで言葉責めを実行しますか？", 12, 180, 196, 280, 2, 0xffffffff)
		s.drawManinButton(canvas, 190, 260, 110, 34, "はい")
		s.drawManinButton(canvas, 340, 260, 110, 34, "いいえ")
	}
}

func (s *Session) drawManinAttackGauges(canvas *gfx.BgraCanvas) {
	attack := &s.manin.attack
	s.maninDrawText(canvas, "残り時間 "+strconv.Itoa(attack.TurnsLeft), 12, 250, 18, 180, 1, 0xffffffff)
	bars := []struct {
		label string
		value int
		max   int
		blue  byte
		green byte
		red   byte
	}{
		{"興奮", attack.Arousal, maninGaugeMax, 80, 80, 240},
		{"ストレス", attack.Stress, maninGaugeMax, 220, 80, 60},
		{"バーニング", attack.Burning, 4, 40, 200, 220},
	}
	for i, bar := range bars {
		y := 46 + i*30
		s.maninDrawText(canvas, bar.label, 10, 360, y, 90, 1, 0xffffffff)
		canvas.Fill(440, y, 180, 16, 20, 20, 20, 230)
		width := 0
		if bar.max > 0 {
			width = bar.value * 176 / bar.max
		}
		if width > 176 {
			width = 176
		}
		canvas.Fill(442, y+2, width, 12, bar.blue, bar.green, bar.red, 255)
	}
}

func (s *Session) drawManinAttackTarget(canvas *gfx.BgraCanvas) {
	u := s.manin
	name := u.face
	if name == "" {
		name = "mf" + strconv.Itoa(u.attack.Target)
	}
	if image := s.maninImage(name); image != nil {
		canvas.Draw(image, 232, 24)
	}
	s.maninDrawText(canvas, "対象: "+maninTargetName(u.attack.Target), 10, 232, 122, 160, 1, 0xffffffff)
}

func maninTargetName(target int) string {
	if target < 1 || target > 6 {
		return "?"
	}
	return []string{"遥香", "柚", "沙也香", "真澄", "晶", "千尋"}[target-1]
}

func (s *Session) drawManinRank(canvas *gfx.BgraCanvas, rank, x, y int) {
	name := "rank_e"
	switch rank {
	case 5:
		name = "rank_a"
	case 4:
		name = "rank_b"
	case 3:
		name = "rank_c"
	case 2:
		name = "rank_d"
	}
	if !s.maninDrawImage(canvas, name, x, y) {
		canvas.Fill(x, y, 12, 12, 100, 100, 100, 255)
	}
}

func (s *Session) drawManinButton(canvas *gfx.BgraCanvas, x, y, w, h int, label string) {
	canvas.Fill(x, y, w, h, 90, 60, 30, 240)
	canvas.Fill(x, y, w, 2, 220, 220, 220, 255)
	canvas.Fill(x, y+h-2, w, 2, 220, 220, 220, 255)
	s.maninDrawText(canvas, label, 12, x+8, y+8, w-16, 1, 0xffffffff)
}

func maninShorten(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func (s *Session) pointerManinAttack(x, y int) bool {
	u := s.manin
	attack := &u.attack
	if attack.Phase == maninAttackConfirm {
		if x >= 190 && x < 300 && y >= 260 && y < 294 {
			attack.Phase = maninAttackExecute
			attack.Queue = append([]int{}, attack.Selected...)
			attack.QueueIndex = 0
			attack.Selected = nil
			s.gameState.SetWaitMilliseconds(650)
			s.composerInvalidate()
			return true
		}
		if x >= 340 && x < 450 && y >= 260 && y < 294 {
			attack.Phase = maninAttackSelect
			s.composerInvalidate()
		}
		return true
	}
	if attack.Phase != maninAttackSelect {
		return true
	}
	pool := u.attackPool()
	// 词表行
	if x >= 24 && x < 200 && y >= 126 && y < 128+maninAttackRowsPerPage*26 {
		row := (y - 128) / 26
		index := attack.Page*maninAttackRowsPerPage + row
		if index >= 0 && index < len(pool) && len(attack.Selected) < 3 {
			id := pool[index]
			already := false
			for _, existing := range attack.Selected {
				if existing == id {
					already = true
					break
				}
			}
			if !already {
				attack.Selected = append(attack.Selected, id)
			}
			s.composerInvalidate()
			return true
		}
	}
	switch {
	case x >= 240 && x < 350 && y >= 282 && y < 312:
		if len(attack.Selected) == 0 {
			s.finishManinAttack(5)
		}
	case x >= 360 && x < 470 && y >= 282 && y < 312:
		if len(attack.Selected) > 0 {
			attack.Selected = attack.Selected[:len(attack.Selected)-1]
			s.composerInvalidate()
		}
	case x >= 480 && x < 590 && y >= 282 && y < 312:
		if len(attack.Selected) == 0 {
			if len(pool) == 0 {
				s.finishManinAttack(5)
			}
			return true
		}
		attack.Phase = maninAttackConfirm
		s.composerInvalidate()
	}
	return true
}

// ------------------------------------------------------------------ 整理

func (s *Session) drawManinOrganize(canvas *gfx.BgraCanvas) {
	u := s.manin
	org := &u.org
	canvas.Clear(8, 8, 16, 255)
	s.maninDrawText(canvas, "責め言葉整理", 16, 20, 18, 300, 1, 0xffffffff)
	// 左：临时记忆（拾ったネタ）。
	canvas.Fill(16, 60, 290, 350, 28, 20, 12, 240)
	s.maninDrawText(canvas, "メモリー（未保存）", 11, 24, 66, 260, 1, 0xffffffff)
	for row := 0; row < 12; row++ {
		index := row
		if index >= len(u.memory) {
			break
		}
		y := 88 + row*26
		if u.hover == row {
			canvas.FillBlend(20, y, 282, 22, 60, 60, 120, 120)
		}
		s.drawManinRank(canvas, u.wordRank(u.memory[index]), 24, y+4)
		s.maninDrawText(canvas, maninShorten(u.wordText(u.memory[index]), 22), 9, 44, y+5, 250, 1, 0xffeeeeee)
	}
	// 右：データ欄（3 页 ×7）。
	canvas.Fill(320, 60, 300, 350, 20, 24, 30, 240)
	s.maninDrawText(canvas, "データ欄 "+strconv.Itoa(org.Page+1)+"/3", 11, 328, 66, 260, 1, 0xffffffff)
	for row := 0; row < maninOrganizerPageSize; row++ {
		index := org.Page*maninOrganizerPageSize + row
		if index >= len(u.words) {
			continue
		}
		y := 88 + row*40
		if org.Pending > 0 {
			canvas.FillBlend(324, y, 292, 34, 120, 60, 40, 120)
		}
		s.drawManinRank(canvas, u.wordRank(u.words[index]), 328, y+8)
		s.maninDrawText(canvas, maninShorten(u.wordText(u.words[index]), 24), 9, 348, y+8, 260, 1, 0xffeeeeee)
	}
	if org.Pending > 0 {
		s.maninDrawText(canvas, "上書きする言葉を選んでください", 11, 320, 372, 300, 1, 0xffff9090)
	}
	s.drawManinButton(canvas, 16, 420, 110, 32, "戻る")
	s.drawManinButton(canvas, 136, 420, 110, 32, "登録")
	s.drawManinButton(canvas, 256, 420, 110, 32, "進む")
	s.drawManinButton(canvas, 376, 420, 110, 32, "終了")
}

func (s *Session) pointerManinOrganize(x, y int) bool {
	u := s.manin
	org := &u.org
	// 记忆行：点一条就写入数据欄；满 21 条进入覆盖模式。
	if x >= 20 && x < 302 && y >= 88 && y < 88+12*26 {
		row := (y - 88) / 26
		if row >= 0 && row < len(u.memory) {
			id := u.memory[row]
			if u.saveWord(id) {
				u.removeMemory(id)
				s.composerInvalidate()
			} else {
				org.Pending = id
				org.Mode = 1
				s.composerInvalidate()
			}
			return true
		}
	}
	// 数据欄行：覆盖模式下替换选中项。
	if x >= 324 && x < 616 && y >= 88 && y < 88+maninOrganizerPageSize*40 {
		row := (y - 88) / 40
		index := org.Page*maninOrganizerPageSize + row
		if row >= 0 && row < maninOrganizerPageSize && index >= 0 && index < len(u.words) {
			if org.Pending > 0 {
				u.words[index] = org.Pending
				u.removeMemory(org.Pending)
				org.Pending = 0
				org.Mode = 0
				s.composerInvalidate()
			}
			return true
		}
	}
	switch {
	case x >= 16 && x < 126 && y >= 420 && y < 452:
		s.maninCloseOrganizer()
	case x >= 136 && x < 246 && y >= 420 && y < 452:
		org.Mode = 1
	case x >= 256 && x < 366 && y >= 420 && y < 452:
		if org.Page < 2 {
			org.Page++
		} else {
			org.Page = 0
		}
		s.composerInvalidate()
	case x >= 376 && x < 486 && y >= 420 && y < 452:
		s.maninCloseOrganizer()
	}
	return true
}

func (s *Session) maninCloseOrganizer() {
	if s.manin == nil {
		return
	}
	s.manin.org = maninOrganizerState{Overwrite: -1}
	s.manin.screen = maninScreenNone
	s.manin.prevScreen = maninScreenNone
	if s.machine != nil {
		s.machine.CloseManinStop()
	}
	saveManinGlobal(s.maninSaveDir(), s.manin.global)
	s.composerInvalidate()
}
