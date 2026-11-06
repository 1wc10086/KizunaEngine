package session

import (
	"strconv"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// eyecatch.go 是学園2 老版章节间的过场（system call "eyecatch"）。
//
// 脚本在这个调用处停下，原版显示一章的标题/LOGO 动画，播完再回 main。
// 发行素材里 logo1/logo2 都是 800x600 整屏图，keikoku 是警告画面；这里
// 用 logo1 做短过场（2.2 秒），点击可跳过。没有素材的作品直接放行。

const (
	eyecatchKindImage = iota
	// eyecatchKindDateMK 是牝教師初代的「第N週 X曜日」卡片：
	// eye_num 是 21 帧 24x24 的单字条（0..9 + 月火水木金土日 + 第週曜日），
	// 由引擎把「第 / 数字 / 週」和「曜日 / 曜 / 日」两列叠出来。
	eyecatchKindDateMK
	// eyecatchKindDateMKIE 是重制版：eye_num1..9 与 eye_wday0..6 已经是
	// 24x72 的整列「第N週 / X曜日」图，直接贴两列。
	eyecatchKindDateMKIE
)

type eyecatchState struct {
	active bool
	until  time.Time
	image  string
	kind   int
	week   int
	wday   int
}

const eyecatchDuration = 2200 * time.Millisecond

func (s *Session) beginEyecatch() state.SessionStep {
	if s.res == nil {
		return state.SessionContinue
	}
	// 牝教師（MK/MKIE）的 eyecatch 是日期卡，不是整屏 logo：
	// 脚本先写 #eyecatch.week / #eyecatch.wday，再 SYSTEM_CALL "eyecatch"。
	// 只按 logo1/logo2 找会得到 MK 的启动 logo，MKIE 则完全找不到素材。
	if s.beginEyecatchDateCard() {
		return state.SessionAwaitInput
	}
	// GK2 老版用 logo1/logo2；GK2IE 用 eyecatch 整屏图。按素材选择，
	// 没有过场素材的作品继续原来的“记录日志后继续”。
	image := ""
	for _, name := range []string{"eyecatch", "logo1", "logo2"} {
		if s.res.Exists(name) && s.eyecatchImage(name) {
			image = name
			break
		}
	}
	if image == "" {
		return state.SessionContinue
	}
	s.eyecatch = eyecatchState{active: true, until: time.Now().Add(eyecatchDuration), image: image}
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.gameState.SetMessageWindowVisible(false)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("ui: eyecatch '%s'", image)
	return state.SessionAwaitInput
}

// beginEyecatchDateCard 识别并启动牝教師两代的日期卡。
func (s *Session) beginEyecatchDateCard() bool {
	if !s.res.Exists("eye_logo") || !s.res.Exists("eyefade") {
		return false
	}
	kind := eyecatchKindImage
	switch {
	case s.res.Exists("eye_num1") && s.res.Exists("eye_wday0"):
		kind = eyecatchKindDateMKIE
	case s.res.Exists("eye_num"):
		kind = eyecatchKindDateMK
	default:
		return false
	}
	day := s.readProperty("#day")
	hasWeek := s.script != nil && s.script.GetPropertyIndex("#eyecatch.week") >= 0
	hasWday := s.script != nil && s.script.GetPropertyIndex("#eyecatch.wday") >= 0
	// 一周天数默认 7（MK 的日期卡按 7 天排）。放課後2 的 Information 布局
	// 用 6 天一周（月～週末），用 Labels 作指纹区分，避免影响其它作品。
	weekDays := 7
	houkago2Week := s.config != nil && s.config.Information.WeekDays == 6 &&
		len(s.config.Information.Labels) > 0
	if houkago2Week {
		weekDays = 6
	}
	week := 1
	if hasWeek {
		week = s.sessionProperty("#eyecatch.week")
	} else if day >= 0 {
		offset := day
		if houkago2Week {
			// #day 从 1 开始数第一天；放課後2 的第 1 天是月曜日。
			offset = max(0, day-1)
		}
		week = offset/weekDays + 1
	}
	wday := 0
	if hasWday {
		wday = clampInt(s.sessionProperty("#eyecatch.wday"), 0, 6)
	} else if kind == eyecatchKindDateMK && day >= 0 {
		// MK 没有 #eyecatch.wday：原作第一天是火曜日，而 eye_num 的
		// 星期帧从月曜日开始，所以 0=月 的素材要 (+1)。
		if houkago2Week {
			wday = max(0, day-1) % weekDays
		} else {
			wday = (day + 1) % 7
		}
	}
	if week < 1 {
		week = 1
	}
	if week > 9 {
		week = 9
	}
	s.eyecatch = eyecatchState{
		active: true, until: time.Now().Add(eyecatchDuration),
		kind: kind, week: week, wday: wday,
	}
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.gameState.SetMessageWindowVisible(false)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("ui: eyecatch date card week=%d wday=%d kind=%d", week, wday, kind)
	return true
}

// eyecatchImage 判断资源名对应的确实是过场整屏图。
//
// 牝教師2 的 bgm.bsa 里有一个同名音频 "eyecatch.ogg"：只判 Exists 会把
// 音频当成图片，过场变成 2.2 秒黑屏。BSG 图有 "BSS-Graphics"/
// "BSS-Composition" 签名，音频没有。
func (s *Session) eyecatchImage(name string) bool {
	location, ok := s.res.TryGet(name)
	if !ok {
		return false
	}
	head := string(location.ReadPrefix(16))
	return strings.HasPrefix(head, "BSS-Graphics") || strings.HasPrefix(head, "BSS-Composition")
}

func (s *Session) eyecatchActive() bool {
	return s.eyecatch.active
}

func (s *Session) finishEyecatch() {
	if !s.eyecatch.active {
		return
	}
	s.eyecatch.active = false
	s.gameState.SetMessageWindowVisible(true)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	s.lastStep = s.finishRun(s.run(commandBudget))
}

func (s *Session) tickEyecatch() {
	if !s.eyecatch.active {
		return
	}
	if time.Now().After(s.eyecatch.until) {
		s.finishEyecatch()
	}
}

func (s *Session) drawEyecatch(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || !s.eyecatch.active {
		return false
	}
	canvas.Clear(0, 0, 0, 255)
	if s.eyecatch.kind == eyecatchKindImage {
		s.drawImageAt(canvas, s.eyecatch.image, 0, 0)
		return true
	}
	s.drawImageAt(canvas, "eyefade", 0, 0)
	s.drawImageAt(canvas, "eye_logo", 80, 120)
	if s.eyecatch.kind == eyecatchKindDateMKIE {
		s.drawImageAt(canvas, "eye_num"+strconv.Itoa(s.eyecatch.week), 420, 150)
		s.drawImageAt(canvas, "eye_wday"+strconv.Itoa(s.eyecatch.wday), 470, 150)
		return true
	}
	// MK 的 eye_num 是 21 帧单字条：0..9 数字，10..16 月..日，
	// 17 第 / 18 週 / 19 曜 / 20 日。把两列竖着叠出来。
	image := s.composer.Load("eye_num")
	if image == nil {
		return true
	}
	frameHeight := image.Height / 21
	if frameHeight <= 0 {
		return true
	}
	draw := func(frame, x, y int) {
		canvas.DrawRegion(image, 0, frame*frameHeight, image.Width, frameHeight, x, y)
	}
	draw(17, 420, 150)
	draw(s.eyecatch.week%10, 420, 174)
	draw(18, 420, 198)
	draw(10+s.eyecatch.wday, 470, 150)
	draw(19, 470, 174)
	draw(20, 470, 198)
	return true
}
