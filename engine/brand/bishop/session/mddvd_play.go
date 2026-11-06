package session

import (
	"fmt"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/runtime/bs"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
	"kizuna/engine/core/textcodec"
)

// mddvd_play.go 还原 MDDVD 的三个「X 系」界面与言葉責め：
//
//   - XA：言葉責め本体。原作 state 0x18，兴奋/压力/燃烧三条计量，
//     兴奋 48 通关、压力 48 失败、时间到 result 4、词用完 result 5；
//   - XR：携帯のデータ整理（state 0x0d），浏览/整理词库，不改变脚本结果；
//   - higoroom：卑語モード（state 0x0b），角色选择 + 每人 CG 缩略图页。
//
// 三个界面共用 mddvdPlayState，由 Pointer/Compose 接管；脚本停在原
// system call 之后，关闭界面后从下一条继续。其它作品不会进入这里。

type mddvdPlayMode int

const (
	mddvdModeAttack mddvdPlayMode = iota
	mddvdModeOrganize
	mddvdModeHigoMenu
	mddvdModeHigoPage
	mddvdModeHigoView
	mddvdModePhone
)

const (
	mddvdAttackSelectLimit = 3
	mddvdGaugeMax          = 48
	mddvdBurningMax        = 4
)

type mddvdWord struct {
	ID      string
	Ratings [6]int
	Text    string
}

type mddvdPlayState struct {
	mode   mddvdPlayMode
	words  []mddvdWord
	target int
	// selected 是本轮选中的词下标；used 记录已经用过的词。原作允许连续
	// 多轮「3 句一组」，直到兴奋/压力到线或词用完。
	selected []int
	used     []bool
	page     int
	hover    int
	// timeUnits 是原作的帧计时器：XT420 表示 420 单位，每执行一句
	// 扣 60（7 句），到 0 时 result 4。它不是墙钟秒数。
	timeUnits int
	// 原作 XA 的三条计量：兴奋 / 压力 / 燃烧。阈值 48，燃烧满 4 时
	// 进入「おねだり」分支（result 2）。
	arousal int
	stress  int
	burning int
	best    int
	good    int
	normal  int
	// higoPage 是卑語モード当前页：0 菜单，1..7 角色页。
	higoPage int
	// organizePage 是データ整理列表的翻页。
	organizePage int
	// phoneText/phoneSpeaker 是 XM 打开携帯时保留的当前对白。
	phoneText    string
	phoneSpeaker string
	// higoCG 是卑語モード里点开的具体 CG 图名。
	higoCG string
}

// installMddvdPlayHandler 只在 MDDVD 内置布局生效时安装钩子。
func (s *Session) installMddvdPlayHandler() {
	if s.config == nil || !s.config.Mddvd {
		return
	}
	if router, ok := s.router.(*bs.BsScriptRouter); ok {
		router.SetSystemCallHandler(s.handleMddvdSystemCall)
	}
}

// handleMddvdSystemCall 处理 MDDVD 的专属 system call。
func (s *Session) handleMddvdSystemCall(name string) (state.SessionStep, bool) {
	keyword := strings.ToLower(strings.TrimSpace(name))
	switch {
	case keyword == "xa":
		if !s.openMddvdAttack() {
			core.Logf("mddvd: XA has no word catalog, continuing")
			return state.SessionContinue, true
		}
		return state.SessionAwaitInput, true
	case keyword == "xr":
		if !s.openMddvdOrganize() {
			return state.SessionContinue, true
		}
		return state.SessionAwaitInput, true
	case len(name) >= 3 && strings.HasPrefix(strings.ToUpper(name), "XB"):
		// XB1,f01 … XB6,f06：为第 1..6 名角色记录 fNN_00 前缀。
		if index := int(name[2] - '0'); index >= 1 && index <= 6 {
			if comma := strings.IndexByte(name, ','); comma >= 0 && comma+1 < len(name) {
				value := strings.TrimSpace(name[comma+1:])
				if len(value) >= 3 {
					s.mddvdPrefixes[index] = value[:3] + "_00"
					return state.SessionContinue, true
				}
			}
		}
	case keyword == "xm":
		if s.openMddvdPhone() {
			return state.SessionAwaitInput, true
		}
		return state.SessionContinue, true
	case strings.HasPrefix(keyword, "xt"):
		if units := mddvdParseTimer(keyword); units > 0 {
			s.mddvdTimeUnits = units
			return state.SessionContinue, true
		}
		return state.SessionContinue, false
	}
	// Dxxx_xx / op_00 是剧本里的"场景登记"调用：原引擎把它们登记进
	// sceneroom 的解锁表后继续，不是播放（播放由 sceneroom 界面负责）。
	// 只登记 scene.dat 里真实存在的标签，未知名字保持标记语义。
	if s.config != nil && s.config.Mddvd && (strings.HasPrefix(name, "D") || keyword == "op_00") {
		if catalog := s.ensureScenes(); catalog != nil {
			if index := catalog.IndexOfLabel(name); index >= 0 {
				if global := s.ensureGlobal(); global != nil {
					if global.UnlockScene(index) {
						s.globalDirty = true
						core.Logf("mddvd: scene registered '%s' (#%d)", name, index)
					}
				}
			}
		}
		return state.SessionContinue, true
	}
	return state.SessionContinue, false
}

// mddvdParseTimer 把 XT420 解析成原作的帧计时器单位。值被夹在
// 1..540（exe 的 0x413308 也是这么夹的）；每次执行一句扣 60。
func mddvdParseTimer(keyword string) int {
	digits := strings.TrimPrefix(keyword, "xt")
	value, err := strconv.Atoi(digits)
	if err != nil || value <= 0 {
		return 0
	}
	if value > 540 {
		value = 420
	}
	if value < 1 {
		value = 1
	}
	return value
}

func (s *Session) openMddvdAttack() bool {
	words := s.mddvdWords()
	if len(words) == 0 {
		return false
	}
	target := int(s.sessionProperty("#charactor"))
	if target < 1 || target > 6 {
		target = 1
	}
	units := s.mddvdTimeUnits
	if units <= 0 {
		units = 420
	}
	s.mddvd = &mddvdPlayState{
		mode: mddvdModeAttack, words: words, target: target,
		used: make([]bool, len(words)), page: 0, hover: -1,
		timeUnits: units,
	}
	s.mddvdHideMessage()
	core.Logf("mddvd: XA opened target=%d words=%d units=%d", target, len(words), units)
	return true
}

func (s *Session) openMddvdPhone() bool {
	if s.res == nil {
		return false
	}
	if _, ok := s.res.TryGet("aw_mobile"); !ok {
		return false
	}
	state := &mddvdPlayState{mode: mddvdModePhone, hover: -1}
	if s.gameState != nil {
		state.phoneText = s.gameState.Text()
		state.phoneSpeaker = s.gameState.Speaker()
	}
	s.mddvd = state
	s.mddvdHideMessage()
	core.Logf("mddvd: XM phone screen")
	return true
}

func (s *Session) openMddvdOrganize() bool {
	words := s.mddvdWords()
	if len(words) == 0 {
		return false
	}
	s.mddvd = &mddvdPlayState{mode: mddvdModeOrganize, words: words, target: 1, hover: -1}
	s.mddvdHideMessage()
	core.Logf("mddvd: XR opened words=%d", len(words))
	return true
}

// openMddvdHigo 从标题的「卑語モード」或脚本的 higoroom 打开角色选择页。
func (s *Session) openMddvdHigo() bool {
	if s.res == nil {
		return false
	}
	if _, ok := s.res.TryGet("higo_base"); !ok {
		return false
	}
	s.mddvd = &mddvdPlayState{mode: mddvdModeHigoMenu, higoPage: 0, hover: -1}
	s.mddvdHideMessage()
	core.Logf("mddvd: higo gallery opened")
	return true
}

func (s *Session) mddvdHideMessage() {
	if s.gameState != nil {
		s.gameState.SetAwaitingInput(false)
		s.gameState.SetMessageWindowVisible(false)
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

func (s *Session) mddvdClose() {
	s.mddvd = nil
	// 标题画面的消息框本来就是隐藏的；从标题打开卑語モード再关闭时
	// 不能把它打开，否则标题底图下会多出一条空对话框。
	if s.gameState != nil && !s.titleMode {
		s.gameState.SetMessageWindowVisible(true)
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// mddvdWords 读取 awords.xdt（整份 0xFF 异或）。
//
// 格式：首行混着 18 字节头 + 首条记录号，之后每条是 "aNN\n" + 6 个
// 0..4 评级字节 + CP932 正文 + "\n"。评级第 i 字节对应第 i 名角色。
func (s *Session) mddvdWords() []mddvdWord {
	if s.res == nil {
		return nil
	}
	location, ok := s.res.TryGet("awords.xdt")
	if !ok {
		location, ok = s.res.TryGet("awords")
	}
	if !ok || location == nil {
		return nil
	}
	data := location.Read()
	if len(data) == 0 {
		return nil
	}
	for index := range data {
		data[index] ^= 0xFF
	}
	lines := strings.Split(string(data), "\n")
	words := make([]mddvdWord, 0, 96)
	for index := 0; index+1 < len(lines); index++ {
		line := strings.TrimSpace(strings.TrimSuffix(lines[index], "\r"))
		line = line[strings.LastIndexAny(line, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f")+1:]
		if len(line) < 3 || line[0] != 'a' {
			continue
		}
		if _, err := strconv.Atoi(line[1:]); err != nil {
			continue
		}
		next := strings.TrimSuffix(lines[index+1], "\r")
		if len(next) < 7 {
			continue
		}
		ratings := [6]int{}
		valid := true
		for i := 0; i < 6; i++ {
			if next[i] < 0 || next[i] > 4 {
				valid = false
				break
			}
			ratings[i] = int(next[i])
		}
		if !valid {
			continue
		}
		words = append(words, mddvdWord{
			ID: line, Ratings: ratings,
			Text: strings.TrimSpace(textcodec.DecodeCP932([]byte(next[6:]))),
		})
	}
	return words
}

func (u *mddvdPlayState) wordRank(index int) int {
	if u == nil || index < 0 || index >= len(u.words) {
		return 0
	}
	character := u.target - 1
	if character < 0 || character >= 6 {
		return 0
	}
	return u.words[index].Ratings[character]
}

func (u *mddvdPlayState) pageCount() int {
	if len(u.words) == 0 {
		return 1
	}
	return (len(u.words) + 10) / 11
}

func (u *mddvdPlayState) unusedCount() int {
	count := 0
	for _, used := range u.used {
		if !used {
			count++
		}
	}
	return count
}

// mddvdApplySelection 执行本轮选中的 3 句：按当前角色的评级增加兴奋/
// 压力，每跨过 12 点兴奋且压力为 0 时燃烧 +1。返回是否已经出结果。
func (s *Session) mddvdApplySelection() (int, bool) {
	u := s.mddvd
	if u == nil {
		return 0, true
	}
	if len(u.selected) == 0 {
		return 0, false
	}
	for _, index := range u.selected {
		if index < 0 || index >= len(u.words) || u.used[index] {
			continue
		}
		u.used[index] = true
		rank := u.wordRank(index)
		before := u.arousal / 12
		switch rank {
		case 4:
			u.arousal += 6
			u.best++
		case 3:
			u.arousal += 4
			u.good++
		case 2:
			u.arousal += 2
			u.stress += 2
			u.normal++
		case 1:
			u.arousal += 2
			u.stress += 4
			u.normal++
		default:
			u.stress += 6
		}
		for level := before + 1; level <= u.arousal/12; level++ {
			if u.stress == 0 {
				u.burning++
			}
		}
	}
	u.selected = u.selected[:0]
	// 每个「3 句一组」的回合消耗 60 单位；XT420 因此是 7 回合。
	u.timeUnits -= 60
	switch {
	case u.stress >= mddvdGaugeMax:
		return 3, true
	case u.arousal >= mddvdGaugeMax:
		if u.burning >= mddvdBurningMax {
			return 2, true
		}
		return 1, true
	case u.timeUnits <= 0:
		return 4, true
	case u.unusedCount() == 0:
		return 5, true
	}
	return 0, false
}

func (s *Session) finishMddvdAttack(result int) {
	u := s.mddvd
	if u == nil {
		return
	}
	s.writeOne("#atk_best", -1, u.best)
	s.writeOne("#atk_good", -1, u.good)
	s.writeOne("#atk_normal", -1, u.normal)
	used := 0
	for _, flag := range u.used {
		if flag {
			used++
		}
	}
	s.writeOne("#atk_number", -1, used)
	s.writeOne("#atk_result", -1, result)
	s.mddvdClose()
	core.Logf("mddvd: attack result=%d arousal=%d stress=%d burning=%d", result, u.arousal, u.stress, u.burning)
}

func (s *Session) runMddvdContinue() (core.Step, error) {
	if s.machine == nil {
		return s.lastStep, nil
	}
	return s.finishRun(s.run(commandBudget)), nil
}

// pointerMddvd 处理三个界面的点击。
func (s *Session) pointerMddvd(op uint32, x, y int) (core.Step, error) {
	u := s.mddvd
	if u == nil {
		return s.lastStep, nil
	}
	if op != 1 && op != 6 {
		return s.lastStep, nil
	}
	switch u.mode {
	case mddvdModeAttack:
		return s.pointerMddvdAttack(u, op, x, y)
	case mddvdModeOrganize:
		return s.pointerMddvdOrganize(u, op, x, y)
	case mddvdModeHigoMenu:
		return s.pointerMddvdHigoMenu(u, op, x, y)
	case mddvdModeHigoPage:
		return s.pointerMddvdHigoPage(u, op, x, y)
	case mddvdModeHigoView:
		if op == 1 && x >= 690 && x < 780 && y >= 500 && y < 543 {
			u.mode = mddvdModeHigoPage
			u.higoCG = ""
			s.composer.Invalidate()
		}
		return s.lastStep, nil
	case mddvdModePhone:
		if op == 1 && x >= 690 && x < 780 && y >= 400 && y < 444 {
			title := s.titleMode
			s.mddvdClose()
			if !title {
				return s.runMddvdContinue()
			}
		}
		return s.lastStep, nil
	}
	return s.lastStep, nil
}

func (s *Session) pointerMddvdAttack(u *mddvdPlayState, op uint32, x, y int) (core.Step, error) {
	if u.timeUnits <= 0 {
		s.finishMddvdAttack(4)
		return s.runMddvdContinue()
	}
	if x >= 36 && x < 256 && y >= 108 && y < 108+11*28 {
		row := (y - 108) / 28
		index := u.page*11 + row
		if index >= 0 && index < len(u.words) && !u.used[index] {
			if op == 6 {
				if u.hover != index {
					u.hover = index
					s.composer.Invalidate()
				}
				return s.lastStep, nil
			}
			if !mddvdContains(u.selected, index) && len(u.selected) < mddvdAttackSelectLimit {
				u.selected = append(u.selected, index)
			}
			s.composer.Invalidate()
		}
		return s.lastStep, nil
	}
	if op != 1 {
		return s.lastStep, nil
	}
	switch {
	case x >= 690 && x < 780 && y >= 250 && y < 294:
		s.finishMddvdAttack(5)
		return s.runMddvdContinue()
	case x >= 690 && x < 780 && y >= 300 && y < 344:
		result, done := s.mddvdApplySelection()
		if done {
			s.finishMddvdAttack(result)
			return s.runMddvdContinue()
		}
		s.composer.Invalidate()
	case x >= 690 && x < 780 && y >= 350 && y < 394:
		u.selected = u.selected[:0]
		s.composer.Invalidate()
	case x >= 690 && x < 780 && y >= 400 && y < 444:
		s.finishMddvdAttack(5)
		return s.runMddvdContinue()
	case x >= 300 && x < 660 && y >= 430 && y < 470:
		if x < 480 {
			if u.page > 0 {
				u.page--
			}
		} else if u.page+1 < u.pageCount() {
			u.page++
		}
		s.composer.Invalidate()
	}
	return s.lastStep, nil
}

func (s *Session) pointerMddvdOrganize(u *mddvdPlayState, op uint32, x, y int) (core.Step, error) {
	if op != 1 {
		return s.lastStep, nil
	}
	switch {
	case x >= 690 && x < 780 && y >= 250 && y < 294,
		x >= 690 && x < 780 && y >= 400 && y < 444:
		s.mddvdClose()
		return s.runMddvdContinue()
	case x >= 690 && x < 780 && y >= 300 && y < 344:
		u.organizePage++
		if u.organizePage > 2 {
			u.organizePage = 0
		}
		s.composer.Invalidate()
	}
	return s.lastStep, nil
}

func (s *Session) pointerMddvdHigoMenu(u *mddvdPlayState, op uint32, x, y int) (core.Step, error) {
	if op != 1 {
		return s.lastStep, nil
	}
	// 底部右侧「タイトルへ / 戻る」：关闭界面；标题模式保持标题，
	// 剧情中的 higoroom 则回到脚本下一条。
	if y >= 500 && x >= 531 {
		title := s.titleMode
		s.mddvdClose()
		if !title {
			return s.runMddvdContinue()
		}
		return s.lastStep, nil
	}
	if y >= 500 && x < 504 {
		// その他のシーン → 第 7 页。
		u.mode = mddvdModeHigoPage
		u.higoPage = 7
		s.composer.Invalidate()
		return s.lastStep, nil
	}
	if y < 500 {
		page := clampInt(x/(800/6), 1, 6)
		u.mode = mddvdModeHigoPage
		u.higoPage = page
		s.composer.Invalidate()
	}
	return s.lastStep, nil
}

func (s *Session) pointerMddvdHigoPage(u *mddvdPlayState, op uint32, x, y int) (core.Step, error) {
	if op != 1 {
		return s.lastStep, nil
	}
	// 每页的 戻る 都在右下 (463,536,179,40)。
	if x >= 450 && x < 660 && y >= 530 {
		u.mode = mddvdModeHigoMenu
		u.higoPage = 0
		s.composer.Invalidate()
		return s.lastStep, nil
	}
	if u.higoPage == 7 {
		if x >= 452 && x < 652 && y >= 336 && y < 486 {
			u.higoCG = mddvdHigoCG(7, 0)
			u.mode = mddvdModeHigoView
			s.composer.Invalidate()
		}
		return s.lastStep, nil
	}
	// 六个缩略图格：左右两列 x=394/560，三行 y=160/280/400，150x113。
	// 原引擎的 higoN_hit 只给已解锁格；这里按同一几何命中，再从
	// higo_gallery_cg 表取对应 CG 名。
	for slot := 0; slot < 6; slot++ {
		boxX, boxY := 394, 160
		if slot%2 == 1 {
			boxX = 560
		}
		boxY = 160 + (slot/2)*120
		if x < boxX || x >= boxX+150 || y < boxY || y >= boxY+113 {
			continue
		}
		if name := mddvdHigoCG(u.higoPage, slot); name != "" {
			u.higoCG = name
			u.mode = mddvdModeHigoView
			s.composer.Invalidate()
		}
		return s.lastStep, nil
	}
	return s.lastStep, nil
}

func mddvdContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// drawMddvdPlay 把当前界面画到帧上。没有界面时返回 false。
func (s *Session) drawMddvdPlay(canvas *gfx.BgraCanvas) bool {
	u := s.mddvd
	if u == nil || canvas == nil {
		return false
	}
	switch u.mode {
	case mddvdModeAttack:
		return s.drawMddvdAttack(canvas, u)
	case mddvdModeOrganize:
		return s.drawMddvdOrganize(canvas, u)
	case mddvdModeHigoMenu:
		return s.drawMddvdHigoMenu(canvas, u)
	case mddvdModeHigoPage:
		return s.drawMddvdHigoPage(canvas, u)
	case mddvdModeHigoView:
		return s.drawMddvdHigoView(canvas, u)
	case mddvdModePhone:
		return s.drawMddvdPhone(canvas, u)
	}
	return false
}

func (s *Session) drawMddvdAttack(canvas *gfx.BgraCanvas, u *mddvdPlayState) bool {
	if u.timeUnits <= 0 {
		s.finishMddvdAttack(4)
		return true
	}
	canvas.FillBlend(0, 0, s.width, s.height, 4, 6, 14, 150)
	s.drawImageAt(canvas, "aw_mobile", 20, 60)
	s.drawImageAt(canvas, "aw_frame", 300, 80)
	s.drawMddvdText(canvas, "責め言葉", 15, 40, 86, 200, 1, 0xffffffff)
	s.drawMddvdText(canvas, "対象: "+mddvdTargetName(u.target), 13, 310, 88, 300, 1, 0xffeeeeee)
	remaining := (u.timeUnits + 59) / 60
	if remaining < 0 {
		remaining = 0
	}
	s.drawMddvdText(canvas, "残り "+strconv.Itoa(remaining), 14, 560, 88, 180, 1, 0xffffd060)
	s.drawMddvdText(canvas, "選択 "+strconv.Itoa(len(u.selected))+"/3", 14, 560, 112, 180, 1, 0xffffffff)

	start := u.page * 11
	for row := 0; row < 11; row++ {
		index := start + row
		if index >= len(u.words) {
			break
		}
		y := 108 + row*28
		if u.used[index] {
			canvas.FillBlend(34, y, 222, 24, 20, 20, 20, 140)
		} else if mddvdContains(u.selected, index) {
			canvas.Fill(34, y, 222, 24, 70, 105, 55, 220)
		} else if u.hover == index {
			canvas.FillBlend(34, y, 222, 24, 70, 70, 140, 120)
		}
		s.drawMddvdRank(canvas, u.wordRank(index), 38, y+4)
		s.drawMddvdText(canvas, mddvdShorten(u.words[index].Text, 12), 11, 58, y+5, 190, 1, 0xffffffff)
	}
	s.drawMddvdText(canvas, strconv.Itoa(u.page+1)+"/"+strconv.Itoa(u.pageCount()), 12, 100, 430, 100, 1, 0xffd0d0d0)

	s.drawMddvdText(canvas, "実行文章", 15, 310, 112, 160, 1, 0xffffffff)
	if len(u.selected) == 0 {
		s.drawMddvdText(canvas, "左の一覧から3つまで選択してください", 12, 310, 140, 330, 2, 0xffd0d0d0)
	}
	for i, index := range u.selected {
		if index < 0 || index >= len(u.words) {
			continue
		}
		s.drawMddvdText(canvas, strconv.Itoa(i+1)+". "+mddvdShorten(u.words[index].Text, 24), 12, 310, 140+i*32, 330, 2, 0xffffb0c8)
	}
	s.drawMddvdGauges(canvas, u)
	for index, frame := range []struct {
		start int
	}{{0}, {1}, {2}, {3}} {
		y := 250 + index*50
		s.drawFrameButton(canvas, config.UIButton{
			Image: "aw_button", Frames: 4, Start: frame.start, FixedFrame: true, X: 690, Y: y,
		}, 0)
	}
	return true
}

// drawMddvdGauges 画兴奋/压力/燃烧三条计量，位置取自手机右侧的信息框。
func (s *Session) drawMddvdGauges(canvas *gfx.BgraCanvas, u *mddvdPlayState) {
	bars := []struct {
		label string
		value int
		max   int
		blue  byte
		green byte
		red   byte
	}{
		{"興奮", u.arousal, mddvdGaugeMax, 80, 80, 240},
		{"ストレス", u.stress, mddvdGaugeMax, 220, 80, 60},
		{"バーニング", u.burning, mddvdBurningMax, 40, 200, 220},
	}
	for i, bar := range bars {
		y := 270 + i*36
		s.drawMddvdText(canvas, bar.label, 11, 310, y, 100, 1, 0xffffffff)
		canvas.Fill(400, y, 180, 16, 20, 20, 20, 230)
		width := 0
		if bar.max > 0 {
			width = bar.value * 176 / bar.max
		}
		width = clampInt(width, 0, 176)
		canvas.Fill(402, y+2, width, 12, bar.blue, bar.green, bar.red, 255)
	}
}

func (s *Session) drawMddvdOrganize(canvas *gfx.BgraCanvas, u *mddvdPlayState) bool {
	canvas.FillBlend(0, 0, s.width, s.height, 4, 6, 14, 150)
	s.drawImageAt(canvas, "aw_mobile", 20, 60)
	s.drawImageAt(canvas, "aw_frame", 300, 80)
	s.drawMddvdText(canvas, "データ整理", 15, 40, 86, 220, 1, 0xffffffff)
	s.drawMddvdText(canvas, "メモリー", 12, 40, 112, 200, 1, 0xffffffff)
	s.drawMddvdText(canvas, "データ欄", 12, 310, 100, 200, 1, 0xffffffff)
	start := u.organizePage * 10
	for row := 0; row < 10; row++ {
		index := start + row
		if index >= len(u.words) {
			break
		}
		y := 132 + row*28
		canvas.Fill(34, y, 222, 24, 35, 45, 35, 160)
		s.drawMddvdRank(canvas, u.wordRank(index), 38, y+4)
		s.drawMddvdText(canvas, mddvdShorten(u.words[index].Text, 12), 11, 58, y+5, 190, 1, 0xffffffff)
	}
	for row := 0; row < 10; row++ {
		index := start + row
		if index >= len(u.words) {
			break
		}
		y := 132 + row*28
		canvas.Fill(310, y, 330, 24, 30, 35, 50, 160)
		s.drawMddvdRank(canvas, u.wordRank(index), 314, y+4)
		s.drawMddvdText(canvas, mddvdShorten(u.words[index].Text, 22), 11, 334, y+5, 300, 1, 0xffffffff)
	}
	for index, frame := range []struct{ start int }{{0}, {1}, {2}, {3}} {
		y := 250 + index*50
		s.drawFrameButton(canvas, config.UIButton{
			Image: "aw_button", Frames: 4, Start: frame.start, FixedFrame: true, X: 690, Y: y,
		}, 0)
	}
	return true
}

func (s *Session) drawMddvdPhone(canvas *gfx.BgraCanvas, u *mddvdPlayState) bool {
	canvas.FillBlend(0, 0, s.width, s.height, 4, 6, 14, 150)
	s.drawImageAt(canvas, "aw_mobile", 260, 70)
	s.drawImageAt(canvas, "aw_frame", 300, 100)
	s.drawMddvdText(canvas, "携帯", 16, 300, 110, 320, 1, 0xffffffff)
	if u.phoneSpeaker != "" {
		s.drawMddvdText(canvas, u.phoneSpeaker, 13, 310, 145, 330, 1, 0xffffb0c8)
	}
	s.drawMddvdText(canvas, mddvdShorten(u.phoneText, 120), 12, 310, 170, 330, 6, 0xffffffff)
	s.drawFrameButton(canvas, config.UIButton{
		Image: "aw_button", Frames: 4, Start: 3, FixedFrame: true, X: 690, Y: 400,
	}, 0)
	return true
}

func (s *Session) drawMddvdHigoMenu(canvas *gfx.BgraCanvas, u *mddvdPlayState) bool {
	if !s.drawImageAt(canvas, "higo_base", 0, 0) {
		canvas.Clear(0, 0, 0, 255)
	}
	s.drawImageAt(canvas, "higo_on", 0, 0)
	s.drawMddvdText(canvas, "卑語モード", 24, 180, 12, 440, 1, 0xffffffff)
	return true
}

// mddvdHigoCG 是 higo 角色页缩略图到完整 CG 的对应表。
// 由原版 higoN_on 的每个 150x113 缩略图与 Graphics1 的 g### 图做
// 归一化互相关匹配得到（匹配度均 >0.87）；页 6 第 6 格是空框，留空。
func mddvdHigoCG(page, slot int) string {
	table := map[int][6]string{
		1: {"g103", "g104", "g108", "g109", "g111", "g701"},
		2: {"g203", "g204", "g208", "g209", "g211", "g701"},
		3: {"g303", "g304", "g308", "g309", "g311", "g704"},
		4: {"g403", "g404", "g408", "g409", "g411", "g704"},
		5: {"g503", "g504", "g508", "g509", "g511", "g707"},
		6: {"g601", "g604", "g609p2", "g610", "g707", ""},
		7: {"g807", "", "", "", "", ""},
	}
	if row, ok := table[page]; ok && slot >= 0 && slot < 6 {
		return row[slot]
	}
	return ""
}

func (s *Session) drawMddvdHigoView(canvas *gfx.BgraCanvas, u *mddvdPlayState) bool {
	canvas.Clear(0, 0, 0, 255)
	image := s.composer.Load(u.higoCG)
	if image != nil {
		scale := min(float64(s.width)/float64(image.Width), float64(s.height)/float64(image.Height))
		width := int(float64(image.Width) * scale)
		height := int(float64(image.Height) * scale)
		canvas.BlitScaled(image, (s.width-width)/2, (s.height-height)/2, width, height, 255)
	}
	s.drawFrameButton(canvas, config.UIButton{
		Image: "aw_button", Frames: 4, Start: 0, FixedFrame: true, X: 690, Y: 500,
	}, 0)
	return true
}

func (s *Session) drawMddvdHigoPage(canvas *gfx.BgraCanvas, u *mddvdPlayState) bool {
	page := clampInt(u.higoPage, 1, 7)
	base := fmt.Sprintf("higo%d_base", page)
	on := fmt.Sprintf("higo%d_on", page)
	if !s.drawImageAt(canvas, base, 0, 0) {
		canvas.Clear(0, 0, 0, 255)
	}
	s.drawImageAt(canvas, on, 0, 0)
	return true
}

func (s *Session) drawMddvdRank(canvas *gfx.BgraCanvas, rank, x, y int) {
	image := s.composer.Load("aw_rank")
	if image == nil {
		return
	}
	frames := max(1, image.FrameCount)
	if frames <= 1 && image.Height >= 5 {
		frames = 5
	}
	cell := image.Height / frames
	if cell <= 0 {
		return
	}
	rank = clampInt(rank, 0, frames-1)
	canvas.DrawRegion(image, 0, rank*cell, image.Width, cell, x, y)
}

func (s *Session) drawMddvdText(canvas *gfx.BgraCanvas, text string, size, x, y, width, lines int, color uint32) {
	if s.textFont == nil || strings.TrimSpace(text) == "" {
		return
	}
	lineHeight := size + 5
	image := s.textFont.Render(text, size, 1, 1, lineHeight, width, lines, color)
	if image != nil {
		rendering.DrawTextImage(canvas, image, x, y)
	}
}

func mddvdTargetName(target int) string {
	if target < 1 || target > 6 {
		return "?"
	}
	return []string{"遥香", "柚", "沙也香", "真澄", "晶", "千尋"}[target-1]
}

func mddvdShorten(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
