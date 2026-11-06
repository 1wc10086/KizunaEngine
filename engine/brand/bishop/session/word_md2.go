package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// word_md2.go 实现 満淫電車2 的「言葉責めシステムNEO」。
//
// 剧本把数值和分支都写好了，引擎负责玩家能看见/能操作的三件事：
//  1. 读到带 <U n> 的台词时，把 n 号责め言葉显示出来并记进手机临时记忆；
//  2. SELECT_TAIL "言葉責め" 时打开手机，让玩家从记忆里挑 3 句；
//  3. 按目标弱点属性与每句话的 4 位等级码计算兴奋/压力，把结果写回
//     #result/#ecstasy/#stress/#increment_ecstasy/#time_limit/#combo_count，
//     剧本随后按这些值播放升级/结束分支。
//
// 责め言葉表在游戏根目录的 HumiliationWords.dat：每行是 4 个等级字母
// （S/A/B/C/D/E，对应目标的 4 种弱点）+ 台词。4 个字母分别代表
// 胸/尻/羞恥/被虐；#weak_point 选其中一个字母。

const wordMemoryLimit = 7
const wordSavedPages = 3
const wordSavedPerPage = 7
const wordSelectionLimit = 3

type md2Word struct {
	Code [4]byte
	Text string
}

type wordUIState struct {
	// attack 为真表示正在挑 3 句执行；为假表示夜间整理记忆。
	attack   bool
	selected []int
	hover    int
	page     int
	// savedSel 是整理模式下选中的保存槽（当前页下标），-1 表示自动找空位。
	savedSel int
	// overwrite/pending 是手册的"上書きモード"：当前页 7 个槽都满时，
	// 先记住要保存的临时词，再让玩家在保存页选一个槽覆盖。
	// 不选槽直接按終了 = 中断上書き（临时词仍在，不写入）。
	overwrite bool
	pending   int
}

// wordPlayState 是本回合逐词反应的进行中状态。脚本把每句责め言葉拆成
// words%03d 标签（MSG + MOVE f1=106 + SELECT_TAIL），引擎逐句执行：累加
// 兴奋/压力、跳反应台词、算跨级。只活在一个回合内，finishWordTurn 建、
// endWordAttack 清。
type wordPlayState struct {
	// returnPC 是 SELECT_TAIL "言葉責め" 之后的 PC（分派点）：回合结算跳到
	// 这里，脚本按 #result/#combo_count/#increment_ecstasy 选分支。
	returnPC int
	// loopPC 是 SELECT_TAIL "言葉責め" 本身的 PC：跨级直接回合跳到这里，
	// 下一轮 openWordTurn 弹手机；不走升级场景（升级是跨级回合才有的演出）。
	loopPC int
	// loopLabel 是 loopPC 所属的标签名（Lxx_yyy_words_embarrass）。
	// 回合重开必须 seek 标签而不是 seek loopPC：剧本在 SELECT_TAIL 前用
	// `MOVE f1=2922` 把名字索引写进 frame1，直接跳 SELECT_TAIL 会沿用
	// 反应标签留下的 f1=14951（"次の責め言葉"），结果一回合连扣 8 次时间。
	loopLabel     string
	words         []int
	index         int
	current       int
	ecstasyBefore int
	stressBefore  int
	gainE         int
	gainS         int
}

// ensureWords 惰性加载责め言葉表与记忆槽。文件缺失时返回 false，
// 调用方把它当成"没有这一代系统"，不影响其它作品。
func (s *Session) ensureWords() bool {
	if s.wordsLoaded {
		return len(s.words) > 0
	}
	s.wordsLoaded = true
	// 注意：range 取到的是数组副本，必须用下标写回，否则所有保存槽
	// 会保持零值，整理界面永远认为"页已满"而拒绝保存。
	for page := range s.wordSaved {
		for slot := range s.wordSaved[page] {
			s.wordSaved[page][slot] = -1
		}
	}
	path := ""
	for _, root := range append([]string{s.dataDir, s.dir}, s.lookupRoots...) {
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, "HumiliationWords.dat")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			path = candidate
			break
		}
	}
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), data); err == nil {
		data = decoded
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if len(line) < 5 {
			continue
		}
		word := md2Word{Text: line[4:]}
		copy(word.Code[:], []byte(line[:4]))
		for index := range word.Code {
			if word.Code[index] >= 'a' && word.Code[index] <= 'z' {
				word.Code[index] -= 'a' - 'A'
			}
		}
		s.words = append(s.words, word)
	}
	core.Logf("word: loaded %d humiliation word(s) from %s", len(s.words), path)
	return len(s.words) > 0
}

// wordText 返回 1 基编号对应的台词。
func (s *Session) wordText(number int) string {
	if number < 1 || number > len(s.words) {
		return ""
	}
	return s.words[number-1].Text
}

// resetWordMemoryForNewGame 把手机记忆恢复到"全新周目"。
// 只在标题 START/OPENING 的非保留路径调用；读档路径走 restoreWordMemory。
func (s *Session) resetWordMemoryForNewGame() {
	if s.config == nil || s.config.Ecstasy.Empty() {
		return
	}
	s.ensureWords()
	s.wordTemp = nil
	for page := range s.wordSaved {
		for slot := range s.wordSaved[page] {
			s.wordSaved[page][slot] = -1
		}
	}
	s.wordActive = false
	s.wordPlay = nil
	s.wordUI = nil
	s.wordBurn = 0
	s.wordBurnLocked = false
}

// wordRankFor 返回某个词在指定弱点属性下的等级字母。
//
// 0..3 是四种属性；4 是愛莉的“全属性弱点”哨兵值（L01_100/L01_307 写 4，
// 其它角色写 0..3）。四个字母里取最高一级：这样全属性角色对任何词都能
// 得到该词最强的反应，也解释了教程角色为何能稳定通关。
func (s *Session) wordRankFor(number, weak int) byte {
	if number < 1 || number > len(s.words) {
		return 'C'
	}
	code := s.words[number-1].Code
	if weak >= 0 && weak < 4 {
		return code[weak]
	}
	if weak == 4 {
		const order = "SABCDE"
		best := len(order)
		for _, letter := range code {
			if index := strings.IndexByte(order, letter); index >= 0 && index < best {
				best = index
			}
		}
		if best >= 0 && best < len(order) {
			return order[best]
		}
	}
	return 'C'
}

// applyWordCodes 把台词里的责め言葉占位符换成原文。
//
// 剧本有两种写法：
//   - <R n>：剧情里"想到一句好台词"，显示成粉色并记进手机临时记忆；
//   - <U n>：言葉责め时执行栏里显示第 n 句（不重复记忆）。
//
// 两种都必须换成 HumiliationWords.dat 的原文，否则玩家看到的是控制码。
//
// 实际文本里编号前可能带资源号（<R 300054>）或空白，所以不能只读
// 紧跟在字母后的第一串数字：取尖括号内最后一串数字，再按词表长度
// 截取后三位还原。
func (s *Session) applyWordCodes() {
	if s.gameState == nil || !s.ensureWords() {
		return
	}
	text := s.gameState.Text()
	if !strings.Contains(text, "<R") && !strings.Contains(text, "<U") {
		return
	}
	var collected []int
	var builder strings.Builder
	for {
		start := -1
		for _, marker := range []string{"<R", "<U"} {
			if index := strings.Index(text, marker); index >= 0 && (start < 0 || index < start) {
				start = index
			}
		}
		if start < 0 {
			builder.WriteString(text)
			break
		}
		builder.WriteString(text[:start])
		remember := text[start+1] == 'R'
		close := strings.IndexByte(text[start:], '>')
		if close < 0 {
			builder.WriteString(text[start:])
			break
		}
		close += start
		inner := text[start+2 : close]
		number := 0
		digits := 0
		for index := len(inner) - 1; index >= 0; index-- {
			ch := inner[index]
			if ch < '0' || ch > '9' {
				if digits > 0 {
					break
				}
				continue
			}
			multiplier := 1
			for step := 0; step < digits; step++ {
				multiplier *= 10
			}
			number += int(ch-'0') * multiplier
			digits++
		}
		if digits > 3 && number > len(s.words) {
			// 300054 → 054 → 54；只取后三位，因为词表只有 130 句。
			number %= 1000
		}
		if word := s.wordText(number); word != "" {
			builder.WriteString(word)
			if remember {
				collected = append(collected, number)
			}
		} else {
			builder.WriteString(text[start : close+1])
		}
		text = text[close+1:]
	}
	if builder.Len() == 0 {
		return
	}
	s.gameState.SetText(builder.String())
	for _, number := range collected {
		s.rememberWord(number)
	}
}

// rememberWord 把一句话放进临时记忆；满了就丢掉最旧的一句。
func (s *Session) rememberWord(number int) {
	for _, existing := range s.wordTemp {
		if existing == number {
			return
		}
	}
	s.wordTemp = append(s.wordTemp, number)
	if len(s.wordTemp) > wordMemoryLimit {
		s.wordTemp = append([]int(nil), s.wordTemp[len(s.wordTemp)-wordMemoryLimit:]...)
	}
}

func (s *Session) savedWordAt(page, slot int) int {
	if page < 0 || page >= wordSavedPages || slot < 0 || slot >= wordSavedPerPage {
		return -1
	}
	return s.wordSaved[page][slot]
}

func (s *Session) setSavedWord(page, slot, value int) {
	if page < 0 || page >= wordSavedPages || slot < 0 || slot >= wordSavedPerPage {
		return
	}
	s.wordSaved[page][slot] = value
}

// availableWords 返回攻击时能选的句子：临时记忆 + 3 页保存。
func (s *Session) availableWords() []int {
	out := make([]int, 0, len(s.wordTemp)+wordSavedPages*wordSavedPerPage)
	out = append(out, s.wordTemp...)
	for page := 0; page < wordSavedPages; page++ {
		for slot := 0; slot < wordSavedPerPage; slot++ {
			if value := s.wordSaved[page][slot]; value > 0 {
				out = append(out, value)
			}
		}
	}
	return out
}

// beginWordAttack 对应 SELECT_TAIL "言葉責め開始"：打开仪表，重置本轮的
// 选择状态。脚本随后会进入循环，在每一轮发 "言葉責め"。
func (s *Session) beginWordAttack() {
	if s.config == nil || s.config.Ecstasy.Empty() {
		return
	}
	s.wordActive = true
	s.wordUI = nil
	// 新一轮言葉責め从零开始：上一次事件残留的 gain/burn 不能带进来。
	s.wordPlay = nil
	s.wordBurn = 0
	s.wordBurnLocked = false
	// 剧本在「言葉責め開始」之后按 #dont_have_words 决定是否直接跳到
	// ネタ切れ分支（label 778）。必须在这里就判断手机里有没有词，
	// 不能等第一次「言葉責め」时才判断——那时剧本已经进了主循环。
	noWords := 0
	if len(s.availableWords()) == 0 {
		noWords = 1
	}
	s.writeOne(s.config.Ecstasy.NoWordsVariable, -1, noWords)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("word: attack started")
}

func (s *Session) endWordAttack() {
	s.wordActive = false
	s.wordUI = nil
	// 回合状态只活在回合内：清掉再回标题/读档不会把上一回合的 gain 带回来。
	s.wordPlay = nil
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("word: attack ended")
}

// openWordTurn 对应 SELECT_TAIL "言葉責め"：弹手机让玩家选 3 句。
func (s *Session) openWordTurn() bool {
	if s.config == nil || s.config.Ecstasy.Empty() || !s.ensureWords() {
		return false
	}
	if len(s.availableWords()) == 0 {
		// 没有句子时不弹空手机：写 #result=2（ネタ切れ）让脚本走 lackofmaterial。
		// #increment_ecstasy 同时清零，combo 判定（>12）不会假触发。
		s.writeOne(s.config.Ecstasy.IncrementVariable, -1, 0)
		s.writeOne(s.config.Ecstasy.ResultVariable, -1, 2)
		return true
	}
	s.wordActive = true
	s.wordUI = &wordUIState{attack: true, savedSel: -1}
	s.gameState.SetAwaitingInput(false)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return true
}

// openWordOrganize 对应 SELECT_TAIL "言葉整理"：夜间整理临时记忆。
func (s *Session) openWordOrganize() bool {
	if !s.ensureWords() {
		return false
	}
	s.wordUI = &wordUIState{attack: false, savedSel: -1, page: 0}
	s.gameState.SetAwaitingInput(false)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return true
}

// wordUIRect 返回手机界面的绘制/命中几何。坐标全部相对 mb_base 的左上角。
func (s *Session) wordUIPanel() (int, int) {
	meter := s.config.Ecstasy
	_ = meter
	// mb_base 的 base pos 固定在配置里；这里用同一组坐标，避免依赖
	// [mobile phone] 的解析结果（这一代只有满淫电车2 用到它）。
	return 26, 118
}

type wordHitRect struct {
	x, y, w, h int
	kind       int // 1=exec, 2=memory, 3=confirm, 4=cancel, 5=prev, 6=next, 7=saved
	index      int
}

func (s *Session) wordHitRects() []wordHitRect {
	baseX, baseY := s.wordUIPanel()
	rects := make([]wordHitRect, 0, 20)
	// 実行文章 3 行（相对 10,114/138/162，行高 24）。
	for index := 0; index < wordSelectionLimit; index++ {
		rects = append(rects, wordHitRect{baseX + 10, baseY + 114 + index*24, 228, 24, 1, index})
	}
	// 记忆/曲目 7 行（相对 10,242/266/...）。整理模式进入上書き后，
	// 这 7 行改为保存槽（kind 7），点击即选覆盖目标。
	kind := 2
	if s.wordUI != nil && !s.wordUI.attack && s.wordUI.overwrite {
		kind = 7
	}
	for index := 0; index < wordMemoryLimit; index++ {
		rects = append(rects, wordHitRect{baseX + 10, baseY + 242 + index*24, 228, 24, kind, index})
	}
	if s.wordUI != nil && s.wordUI.attack {
		rects = append(rects,
			wordHitRect{baseX + 60, baseY + 414, 76, 24, 3, 0},
			wordHitRect{baseX + 150, baseY + 414, 76, 24, 4, 0},
			wordHitRect{baseX + 10, baseY + 442, 74, 24, 5, 0},
			wordHitRect{baseX + 164, baseY + 442, 74, 24, 6, 0},
		)
	} else {
		rects = append(rects,
			wordHitRect{baseX + 10, baseY + 414, 74, 24, 5, 0},
			wordHitRect{baseX + 86, baseY + 414, 76, 24, 4, 0},
			wordHitRect{baseX + 164, baseY + 414, 74, 24, 6, 0},
		)
	}
	return rects
}

func (s *Session) wordHitAt(x, y int) wordHitRect {
	for _, rect := range s.wordHitRects() {
		if x >= rect.x && y >= rect.y && x < rect.x+rect.w && y < rect.y+rect.h {
			return rect
		}
	}
	return wordHitRect{kind: 0, index: -1}
}

// pointerWordUI 处理手机界面的点击。
func (s *Session) pointerWordUI(op uint32, x, y int) (core.Step, error) {
	if s.wordUI == nil {
		return s.lastStep, nil
	}
	switch op {
	case 0:
		hit := s.wordHitAt(x, y)
		s.wordUI.hover = hit.kind*100 + hit.index
		if s.composer != nil {
			s.composer.Invalidate()
		}
		return s.lastStep, nil
	case 1, 3:
		if op == 3 && !s.choicePickedAt.IsZero() {
			elapsed := time.Since(s.choicePickedAt)
			s.choicePickedAt = time.Time{}
			if elapsed < 250*time.Millisecond {
				return s.lastStep, nil
			}
		}
		hit := s.wordHitAt(x, y)
		if hit.kind == 0 {
			return s.lastStep, nil
		}
		s.choicePickedAt = time.Now()
		return s.activateWordHit(hit), nil
	}
	return s.lastStep, nil
}

// wordSelected 判断一句词是否已经在执行栏里。
func wordSelected(selected []int, number int) bool {
	for _, value := range selected {
		if value == number {
			return true
		}
	}
	return false
}

func (s *Session) activateWordHit(hit wordHitRect) core.Step {
	ui := s.wordUI
	if ui == nil {
		return s.lastStep
	}
	switch hit.kind {
	case 1: // 実行文章：点一下取消这句
		if ui.attack && hit.index < len(ui.selected) {
			ui.selected = append(ui.selected[:hit.index], ui.selected[hit.index+1:]...)
		}
	case 2: // 记忆列表
		if ui.attack {
			words := s.availableWords()
			offset := ui.page * wordMemoryLimit
			if offset+hit.index < len(words) && len(ui.selected) < wordSelectionLimit {
				number := words[offset+hit.index]
				// 同一句不能在一轮里重复选三次：原版 3 个执行栏是
				// 三句不同的责め言葉，重复会白占栏位。
				if !wordSelected(ui.selected, number) {
					ui.selected = append(ui.selected, number)
				}
			}
		} else {
			// 整理模式：点临时记忆里的句子，保存到当前页第一个空位；
			// 没有空位时进入手册的"上書きモード"：先记住这句，等玩家在
			// 保存页点一个槽作为覆盖目标，再真正写入。
			if hit.index < len(s.wordTemp) {
				number := s.wordTemp[hit.index]
				slot := ui.savedSel
				if slot < 0 {
					for index := 0; index < wordSavedPerPage; index++ {
						if s.wordSaved[ui.page][index] < 0 {
							slot = index
							break
						}
					}
				}
				if slot >= 0 {
					s.setSavedWord(ui.page, slot, number)
					s.wordTemp = append(s.wordTemp[:hit.index], s.wordTemp[hit.index+1:]...)
					ui.savedSel = -1
				} else {
					ui.overwrite = true
					ui.pending = number
					ui.savedSel = -1
				}
			}
		}
	case 3: // 決定
		if ui.attack {
			// 手册：执行栏填满 3 句，或已经没有可选句子时才出现确认。
			// 提前确认会让脚本只播 1-2 句就结算，和原版回合不符。
			remaining := 0
			for _, number := range s.availableWords() {
				if !wordSelected(ui.selected, number) {
					remaining++
				}
			}
			if len(ui.selected) < wordSelectionLimit && remaining > 0 {
				break
			}
			if len(ui.selected) == 0 {
				break
			}
		}
		return s.finishWordTurn()
	case 4: // 戻す / 終了
		if ui.attack {
			// 攻击轮的「戻す」撤掉最后选择的一句，和手册一致；
			// 要全部重选就连续点。
			if len(ui.selected) > 0 {
				ui.selected = ui.selected[:len(ui.selected)-1]
			}
			break
		}
		if ui.overwrite {
			// 手册：上書きモード中按終了 = 中断上書き，回到"登録"画面，
			// 刚才选中的临时词不写入保存记忆。
			ui.overwrite = false
			ui.pending = 0
			ui.savedSel = -1
			break
		}
		s.wordUI = nil
		if s.composer != nil {
			s.composer.Invalidate()
		}
		return s.finishRun(s.run(commandBudget))
	case 5: // 上一页
		if ui.page > 0 {
			ui.page--
			ui.savedSel = -1
		}
	case 6: // 下一页
		if ui.attack {
			words := s.availableWords()
			if (ui.page+1)*wordMemoryLimit < len(words) {
				ui.page++
			}
		} else if ui.page+1 < wordSavedPages {
			ui.page++
			ui.savedSel = -1
		}
	case 7: // 保存槽（上書き模式）：点哪个槽就覆盖哪个
		ui.savedSel = hit.index
		if ui.overwrite && ui.pending > 0 {
			s.setSavedWord(ui.page, hit.index, ui.pending)
			// 临时记忆里被选中的那句随之消失；找不到也安全（可能已翻页）。
			for index, number := range s.wordTemp {
				if number == ui.pending {
					s.wordTemp = append(s.wordTemp[:index], s.wordTemp[index+1:]...)
					break
				}
			}
			ui.overwrite = false
			ui.pending = 0
			ui.savedSel = -1
		}
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return s.lastStep
}

// finishWordTurn 打开的回合不做聚合结算——剧本已经把每句拆成 words%03d
// 标签在逐句执行。引擎只把玩家选的词装进 wordPlay、跳到第一句，让词标签
// 的 MSG 正常显示并停在 AwaitInput。数值全在 wordReaction/advanceWord 里
// 逐句写回；#combo_count/#ecstasy_level 仍由升级场景（lv2..lv5combo）自己
// INC，引擎绝不能写。
func (s *Session) finishWordTurn() core.Step {
	ui := s.wordUI
	if ui == nil {
		return s.lastStep
	}
	meter := s.config.Ecstasy
	// 首句的词标签直接执行，不经 openWordTurn 的"回合开始"钩子，所以
	// ecstasyBefore/stressBefore 在这里抓。
	play := &wordPlayState{
		returnPC:      s.machine.ProgramCounter(),
		loopPC:        s.machine.InstructionStart(),
		loopLabel:     s.labelNameAt(s.machine.InstructionStart()),
		words:         append([]int(nil), ui.selected...),
		ecstasyBefore: s.readProperty(meter.EcstasyVariable),
		stressBefore:  s.readProperty(meter.StressVariable),
	}
	s.wordPlay = play
	s.wordUI = nil
	s.gameState.SetAwaitingInput(false)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	first := play.words[0]
	play.current = first
	if !s.machine.SeekLabel(fmt.Sprintf("words%03d", first)) {
		core.Logf("word: label words%03d not found", first)
		return s.finishRun(s.run(commandBudget))
	}
	core.Logf("word: turn opened with %d word(s), first=words%03d", len(play.words), first)
	return s.finishRun(s.run(commandBudget))
}

// wordReaction 对应 SELECT_TAIL "責め言葉の反応処理"：本词的数值已经
// 在词标签 MSG 之后到达这里，把它写回 #ecstasy/#stress 并累计
// #increment_ecstasy；按当前等级跳到对应的 response_* 标签。
// 剧本的 response_* 标签自己 MOVE f1=14951 再 SELECT_TAIL，所以这里
// 跳过去之后玩家看到的就是反应台词，而不是"回合结算"。
func (s *Session) wordReaction() (state.SessionStep, bool) {
	play := s.wordPlay
	if play == nil || s.machine == nil {
		return state.SessionContinue, false
	}
	meter := s.config.Ecstasy
	number := play.current
	// ai(airi) 的 weak_point=4 是“全属性弱点”：脚本只写 4，没有第 5 个
	// 字母，所以取四个属性里最高的一级。不能用 C 兜底——C 级每词固定
	// +2 压力，她会在兴奋到 48 的同一回合先触发 stressmax，永远无法通关。
	rank := s.wordRankFor(number, s.readProperty(meter.WeakPointVariable))
	increment, ok := meter.Increment[rank]
	if !ok {
		increment, ok = meter.Increment['C']
	}
	var gainE, gainS int
	if ok {
		gainE, gainS = increment[0], increment[1]
	}
	play.gainE += gainE
	play.gainS += gainS
	ecstasy := s.readProperty(meter.EcstasyVariable) + gainE
	if ecstasy > 48 {
		ecstasy = 48
	}
	stress := s.readProperty(meter.StressVariable) + gainS
	if stress > 48 {
		stress = 48
	}
	s.writeOne(meter.EcstasyVariable, -1, ecstasy)
	s.writeOne(meter.StressVariable, -1, stress)
	s.writeOne(meter.IncrementVariable, -1, play.gainE)
	// 本词产生压力就清零烧膛并锁死：燃烧ゲージ只在 stress==0 且升级时 +1，
	// 一旦有压力就永久锁死（本局不再增长）。
	if gainS > 0 {
		s.wordBurn = 0
		s.wordBurnLocked = true
	}
	// 反应台词按 "#character_no 的角色 + 等级档 + 本轮升级还是日常" 命名：
	// response_<角色>_<1st|2nd>_<low|high>_<s|a|b|c|d|e>，其中 1st/2nd
	// 由 #event_type 决定（0=1st，1=2nd），low/high 由 #ecstasy_level
	// 决定（<=2=low，>=3=high），字母由本词弱点等级在 s/a/b/c 轮换里
	// 取（d/e 各只有一句）。完整标签表在 bsxx.dat 的 1437 起连续段。
	//（airi,misato,kaoru,yuiko,sae 顺序）。
	character := ""
	switch s.readProperty(meter.CharacterVariable) {
	case 0:
		character = "airi"
	case 1:
		character = "misato"
	case 2:
		character = "kaoru"
	case 3:
		character = "yuiko"
	case 4:
		character = "sae"
	}
	ordinal := "1st"
	if s.readProperty(meter.EventVariable) == 1 {
		ordinal = "2nd"
	}
	level := s.readProperty(meter.LevelVariable)
	highArg := "low"
	if level >= 3 {
		highArg = "high"
	}
	letterText := ""
	switch rank {
	case 'S', 's':
		letterText = "s"
	case 'A', 'a':
		letterText = "a"
	case 'B', 'b':
		letterText = "b"
	case 'C', 'c':
		letterText = "c"
	case 'D', 'd':
		letterText = "d"
	case 'E', 'e':
		letterText = "e"
	default:
		letterText = "c"
	}
	label := ""
	if character != "" {
		label = fmt.Sprintf("response_%s_%s_%s_%s", character, ordinal, highArg, letterText)
	}
	if label != "" && s.machine.SeekLabel(label) {
		return state.SessionContinue, true
	}
	// 找不到反应标签（如词号超出对应角色台词表）时直接走下一句，不停机。
	return s.advanceWord()
}

// advanceWord 对应 SELECT_TAIL "次の責め言葉"：若 #stress>=48 回合中途
// 立即结束并按 #result=1 分派到 stressmax；否则推下一句。回合计次在回合
// 结束时统一处理：time_limit 减 1、判定通关/跨级/超时。
func (s *Session) advanceWord() (state.SessionStep, bool) {
	play := s.wordPlay
	if play == nil || s.machine == nil {
		return state.SessionContinue, false
	}
	meter := s.config.Ecstasy
	if s.readProperty(meter.StressVariable) >= 48 {
		// 压力到顶：本回合直接收尾，#result=1 走 stressmax。
		s.writeOne(meter.ResultVariable, -1, 1)
		s.machine.Jump(play.returnPC)
		return state.SessionContinue, true
	}
	play.index++
	if play.index < len(play.words) {
		play.current = play.words[play.index]
		if s.machine.SeekLabel(fmt.Sprintf("words%03d", play.current)) {
			return state.SessionContinue, true
		}
		// 词标签不存在则这句跳过：递归推下一句（深度不超过 3）。
		return s.advanceWord()
	}
	// 本回合词已说完：结算顺序与剧本一致——先扣 time_limit，再判通关，
	// 再判超时，最后判跨过 12 的整数倍。
	timeLimit := s.readProperty(meter.TimeLimitVariable) - 1
	if timeLimit < 0 {
		timeLimit = 0
	}
	s.writeOne(meter.TimeLimitVariable, -1, timeLimit)
	ecstasy := s.readProperty(meter.EcstasyVariable)
	if ecstasy >= 48 {
		// 手册：ストレス0のままクリアするとバーニングゲージはレベル4。
		// 升级场景只发生 3 次（lv2/lv3/lv4），第 4 格是在通关时补满的；
		// 若中途涨过压力则 locked，保持 0。
		if s.readProperty(meter.StressVariable) == 0 && !s.wordBurnLocked {
			s.wordBurn = 4
		}
		// 通关直接跳 chk_h_type：由当前标签名把 _words_embarrass 换成
		// _chk_h_type，不用按角色硬编码。当前 PC 在 response_* 标签里，
		// 必须从 loopPC 的所属标签（embarrass 主循环）反推。
		if play.loopLabel != "" {
			if chk := strings.Replace(play.loopLabel, "_words_embarrass", "_chk_h_type", 1); chk != play.loopLabel && s.machine.SeekLabel(chk) {
				return state.SessionContinue, true
			}
		}
		if chk := s.wordChkHTypeLabel(); chk != "" && s.machine.SeekLabel(chk) {
			return state.SessionContinue, true
		}
	}
	if timeLimit <= 0 {
		s.writeOne(meter.ResultVariable, -1, 3)
		s.machine.Jump(play.returnPC)
		return state.SessionContinue, true
	}
	if ecstasy/12 > play.ecstasyBefore/12 {
		// 跨级：stress==0 且未锁定时 burn+1；跳到 returnPC 让脚本按新
		// #ecstasy_level 播普通/lv* 或 combo 升级场景。
		if s.readProperty(meter.StressVariable) == 0 && !s.wordBurnLocked && s.wordBurn < 4 {
			s.wordBurn++
		}
		s.machine.Jump(play.returnPC)
		return state.SessionContinue, true
	}
	// 普通回合：回到主循环标签开头（含 MOVE f1=2922），重新弹手机。
	// 不能直接 Jump(loopPC)：那会跳过写 frame1 的 MOVE，SELECT_TAIL 会把
	// 上一句反应标签的 f1=14951 当成命令，导致连续结算并瞬间耗尽时间。
	if play.loopLabel != "" && s.machine.SeekLabel(play.loopLabel) {
		return state.SessionContinue, true
	}
	if index := s.wordNameIndex("言葉責め"); index >= 0 {
		s.machine.SetFrame(1, int32(index))
	}
	s.machine.Jump(play.loopPC)
	return state.SessionContinue, true
}

// labelNameAt 返回覆盖 offset 的标签名（偏移不超过 offset 的最后一个
// DefineLabel）。SELECT_TAIL 位于标签内部而不是标签起点，所以不能只比较相等。
func (s *Session) labelNameAt(offset int) string {
	bestOffset := -1
	name := ""
	for index := 0; index < s.script.LabelCount(); index++ {
		if labelOffset := s.script.GetLabelOffset(index); labelOffset >= 0 && labelOffset <= offset && labelOffset > bestOffset {
			bestOffset = labelOffset
			name = s.script.GetLabelName(index)
		}
	}
	return name
}

// wordNameIndex 在名字表里找 SELECT_TAIL 使用的名字索引。
// 仅在标签丢失的兜底路径里用：直接跳 SELECT_TAIL 时必须先恢复 frame1。
func (s *Session) wordNameIndex(name string) int {
	for index := 0; index < s.script.NameCount(); index++ {
		if s.script.GetName(index) == name {
			return index
		}
	}
	return -1
}

// wordChkHTypeLabel 按 #character_no/#event_type 拼 chk_h_type 标签名。
// 主循环 loopPC 已足以推（见 advanceWord），这里是兜底：embarrass 标签
// 查不到时，按角色/事件拼出来的 L%02d_%03d_chk_h_type 仍然能命中。
func (s *Session) wordChkHTypeLabel() string {
	meter := s.config.Ecstasy
	charNo := s.readProperty(meter.CharacterVariable)
	eventType := s.readProperty(meter.EventVariable)
	if charNo < 0 || charNo > 4 || eventType < 0 || eventType > 1 {
		return ""
	}
	ordinal := 100
	if eventType == 1 {
		ordinal = 307
	}
	return fmt.Sprintf("L%02d_%03d_chk_h_type", charNo+1, ordinal)
}

// ---------------------------------------------------------------- 绘制

// wordSavedContains 判断一句话是否已经进入保存记忆；只有保存过的词
// 才显示 4 位属性等级，临时记忆按原版不显示 rank。
func (s *Session) wordSavedContains(number int) bool {
	for page := 0; page < wordSavedPages; page++ {
		for slot := 0; slot < wordSavedPerPage; slot++ {
			if s.wordSaved[page][slot] == number {
				return true
			}
		}
	}
	return false
}

// drawWordUI 把手机界面画在合成帧上。它不是 [mobile phone] 的逐像素复刻，
// 但复用同一块 mb_base 和同样的三行执行栏/七行记忆栏几何，保证点击位置
// 和原版一致；文字用引擎字体画，避免因缺少素材而整屏空白。
func (s *Session) drawWordUI(canvas *gfx.BgraCanvas) bool {
	ui := s.wordUI
	if canvas == nil || ui == nil || s.composer == nil {
		return false
	}
	baseX, baseY := s.wordUIPanel()
	// 手机底图 + 半透明遮罩：底图本身是 248x482 的机身。
	if image := s.composer.Load("mb_base"); image != nil {
		canvas.Draw(image, baseX, baseY)
	}
	canvas.FillBlend(0, 0, canvas.Width, canvas.Height, 0, 0, 0, 96)
	title := "言葉責め"
	if !ui.attack {
		title = "言葉整理"
		if ui.overwrite {
			title = "言葉整理（上書き）"
		}
	}
	s.drawWordText(canvas, title, baseX+16, baseY+12, 210, 0xffffffff, 18)

	execHints := []string{"", "", ""}
	if ui.attack {
		for index, number := range ui.selected {
			if index < len(execHints) {
				execHints[index] = s.wordText(number)
			}
		}
	} else {
		for index := 0; index < wordSelectionLimit; index++ {
			if number := s.savedWordAt(ui.page, index); number > 0 {
				execHints[index] = s.wordText(number)
			}
		}
	}
	for index := 0; index < wordSelectionLimit; index++ {
		x, y := baseX+10, baseY+114+index*24
		canvas.FillBlend(x, y, 228, 22, 20, 20, 40, 160)
		label := execHints[index]
		if label == "" {
			label = "－"
		}
		s.drawWordText(canvas, label, x+4, y+3, 220, 0xffff9ad4, 12)
	}

	words := s.availableWords()
	offset := ui.page * wordMemoryLimit
	if !ui.attack {
		if ui.overwrite {
			// 上書き模式显示保存页：玩家在这一页点要覆盖的槽。
			words = make([]int, 0, wordSavedPerPage)
			for index := 0; index < wordSavedPerPage; index++ {
				words = append(words, s.wordSaved[ui.page][index])
			}
		} else {
			words = s.wordTemp
		}
		offset = 0
	}
	focus := 0
	for index := 0; index < wordMemoryLimit; index++ {
		x, y := baseX+10, baseY+242+index*24
		selected := ui.hover == 2*100+index
		if selected {
			canvas.FillBlend(x, y, 228, 22, 60, 60, 120, 180)
		} else {
			canvas.FillBlend(x, y, 228, 22, 10, 10, 24, 160)
		}
		label := ""
		number := 0
		if offset+index < len(words) {
			number = words[offset+index]
			label = s.wordText(number)
		}
		if label != "" {
			if selected || focus == 0 {
				focus = number
			}
			s.drawWordText(canvas, label, x+4, y+3, 150, 0xffe8e8e8, 11)
			// 4 位属性等级：橙/绿/青/粉，与手册图例一致。临时记忆不显示。
			if s.wordSavedContains(number) && number >= 1 && number <= len(s.words) {
				for slot := 0; slot < 4; slot++ {
					letter := s.words[number-1].Code[slot]
					s.drawWordText(canvas, string(letter), x+160+slot*15, y+4,
						14, wordRankColor(slot), 11)
				}
			}
		}
	}
	// 右侧详情面板：显示选中句子的全文与图例。
	if focus >= 1 && focus <= len(s.words) {
		panelX, panelY := baseX+270, baseY+104
		canvas.FillBlend(panelX, panelY, 300, 250, 8, 8, 18, 210)
		s.drawWordText(canvas, s.words[focus-1].Text, panelX+8, panelY+8, 284, 0xffffffff, 12)
		if s.wordSavedContains(focus) {
			legend := []string{"胸系", "お尻系", "羞恥系", "被虐系"}
			for slot, text := range legend {
				s.drawWordText(canvas, text, panelX+30+slot*66, panelY+180, 60,
					wordRankColor(slot), 13)
				s.drawWordText(canvas, string(s.words[focus-1].Code[slot]),
					panelX+58+slot*66, panelY+200, 20, wordRankColor(slot), 13)
			}
		}
	}
	if ui.attack {
		s.drawWordText(canvas, fmt.Sprintf("選択 %d/3", len(ui.selected)), baseX+10, baseY+390, 220, 0xffffffff, 12)
		s.drawWordText(canvas, "決定", baseX+60, baseY+416, 76, 0xffffff88, 14)
		s.drawWordText(canvas, "戻す", baseX+150, baseY+416, 76, 0xffffff88, 14)
		s.drawWordText(canvas, "前頁", baseX+10, baseY+444, 74, 0xffffff88, 13)
		s.drawWordText(canvas, "次頁", baseX+164, baseY+444, 74, 0xffffff88, 13)
	} else {
		s.drawWordText(canvas, fmt.Sprintf("ページ %d/3  保存 %d/21", ui.page+1, s.savedWordCount()),
			baseX+10, baseY+390, 228, 0xffffffff, 12)
		s.drawWordText(canvas, "前頁", baseX+10, baseY+416, 74, 0xffffff88, 14)
		s.drawWordText(canvas, "終了", baseX+86, baseY+416, 76, 0xffffff88, 14)
		s.drawWordText(canvas, "次頁", baseX+164, baseY+416, 74, 0xffffff88, 14)
	}
	return true
}

// wordRankColor 是 4 种属性等级的颜色：胸橙、尻绿、羞恥青、被虐粉。
func wordRankColor(slot int) uint32 {
	switch slot {
	case 0:
		return 0xffffa020
	case 1:
		return 0xff40d040
	case 2:
		return 0xff30d0ff
	default:
		return 0xffff70c0
	}
}

func (s *Session) savedWordCount() int {
	count := 0
	for page := 0; page < wordSavedPages; page++ {
		for slot := 0; slot < wordSavedPerPage; slot++ {
			if s.wordSaved[page][slot] > 0 {
				count++
			}
		}
	}
	return count
}

// color 是 0xAARRGGBB；用 uint32 而不是 int，arm32 上 int 只有 32 位，
// 0xffffffff 这类颜色常量会直接溢出（arm64/amd64 上不会暴露这个问题）。
func (s *Session) drawWordText(canvas *gfx.BgraCanvas, text string, x, y, width int, color uint32, size int) {
	if s.textFont == nil || text == "" {
		return
	}
	if width <= 0 {
		width = 220
	}
	image := s.textFont.Render(text, size, 1, 1, size+4, width, 1, color)
	if image != nil {
		rendering.DrawTextImage(canvas, image, x, y)
	}
}

// drawWordMeter 画右上角的兴奋度仪表。底图是 ecstasy_meter 的裁剪区，
// 红/蓝条按 #ecstasy/#stress（0..48）填充，剩余时间用数字条显示。
func (s *Session) drawWordMeter(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || !s.wordActive || s.config == nil {
		return false
	}
	meter := s.config.Ecstasy
	if meter.Empty() || meter.Base == "" || s.composer == nil {
		return false
	}
	image := s.composer.Load(meter.Base)
	if image == nil {
		return false
	}
	width, height := image.Width, image.Height
	if meter.BaseWidth > 0 && meter.BaseHeight > 0 {
		width, height = meter.BaseWidth, meter.BaseHeight
	}
	if width > image.Width {
		width = image.Width
	}
	if height > image.Height {
		height = image.Height
	}
	canvas.DrawRegion(image, 0, 0, width, height, meter.BaseX, meter.BaseY)
	// 红色/蓝色液柱：底图的 cover 区域是 38x204。按 48 点上限画两条
	// 竖向半透明色条，位置与图标列对齐（红 8、蓝 42）。
	ecstasy := s.readProperty(meter.EcstasyVariable)
	stress := s.readProperty(meter.StressVariable)
	drawBar := func(offsetX int, value int, r, g, b byte) {
		if value <= 0 {
			return
		}
		if value > 48 {
			value = 48
		}
		barHeight := 204 * value / 48
		canvas.FillBlend(meter.BaseX+offsetX, meter.BaseY+64+(204-barHeight), 22, barHeight, r, g, b, 150)
	}
	drawBar(8, ecstasy, 220, 40, 40)
	drawBar(42, stress, 40, 100, 230)
	// 剩余时间：limit number 是 0..9 的竖排数字条。
	if !meter.Limit.Empty() && meter.Limit.Frames > 0 {
		value := s.readProperty(meter.TimeLimitVariable)
		frame := clampInt(value, 0, meter.Limit.Frames-1)
		button := meter.Limit
		button.X, button.Y = meter.BaseX+meter.LimitX, meter.BaseY+meter.LimitY
		s.drawFrameButton(canvas, button, frame)
	}
	// 连击槽：combo_count 是 4 位掩码，点亮对应数量的火焰图标。
	combo := s.readProperty(meter.ComboVariable)
	if combo > 0 && !meter.Combo.Empty() {
		s.drawFrameButton(canvas, meter.Combo, 0)
	}
	// バーニングゲージ：引擎自己记的 burn（上限 4）。每级一小团火，拱形
	// 叠放，第 4 级用 fire graduate 帧；位置相对仪表 base。
	if s.wordActive && !meter.Fire.Empty() {
		burn := s.wordBurn
		if burn > 4 {
			burn = 4
		}
		if burn > 0 {
			fireX := meter.BaseX + meter.FirePos.X
			fireY := meter.BaseY + meter.FirePos.Y
			stepX, stepY := meter.FireStep.X, meter.FireStep.Y
			if meter.FireStep.X == 0 && meter.FireStep.Y == 0 {
				stepX, stepY = 0, -48
			}
			for level := 1; level <= burn; level++ {
				button := meter.Fire
				if !meter.FireEnd.Empty() && level == 4 {
					button = meter.FireEnd
					// 第 4 级落在第 3 级上 + graduate next 偏移。
					button.X = fireX + 3*stepX + meter.FireEndStep.X
					button.Y = fireY + 3*stepY + meter.FireEndStep.Y
				} else {
					button.X = fireX + (level-1)*stepX
					button.Y = fireY + (level-1)*stepY
				}
				// atlas 火焰按级取帧：帧 0..3 对应 4 级。
				frame := level - 1
				s.drawFrameButton(canvas, button, frame)
			}
		}
	}
	return true
}

// restoreWordMemory 把存档里的责め言葉记忆恢复回来。旧档没有这一段时
// 清空临时/保存记忆，避免把上一局的词带进新读的存档。
func (s *Session) restoreWordMemory(stored *save.SaveData) {
	if stored == nil {
		return
	}
	s.ensureWords()
	if stored.HasWordMemory {
		s.wordTemp = append([]int(nil), stored.WordTemp...)
		s.wordSaved = stored.WordSaved
		return
	}
	s.wordTemp = nil
	for page := range s.wordSaved {
		for slot := range s.wordSaved[page] {
			s.wordSaved[page][slot] = -1
		}
	}
}
