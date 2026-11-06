package session

import (
	"fmt"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// staffroll.go 是学園2 老版的片尾字幕（system call "staffroll"）。
//
// 原版素材有两组：
//   - staffroll_01..08 是各结局的整屏背景；
//   - staffroll_monitor00/01/…/16 是按 #end 值区分的显示器结局背景；
//   - staffroll_list 是 260x1280 的字幕文字图，竖向上滚。
//
// 引擎按 #end 选背景（先 monitor%02d，再 %02d），把字幕从屏幕底部滚到顶部；
// 滚完或玩家点击后继续脚本。字幕图的黑底按色键跳过，只把文字叠在背景上。

type staffrollState struct {
	active     bool
	started    time.Time
	background string
	// list 是字幕图名。学園2 是 staffroll_list（260x1280）；
	// 牝教師初代把整段制作人员名单烘成 ed_roll（256x5805）。
	list string
	// speed 是滚动速度（像素/秒）。长图用更快速度，避免玩家等两分钟。
	speed float64
	// opaque 表示字幕图自带完整背景（MKIE 的 staffroll 是 800x5805 的白底
	// 名单），不能再按"黑底文字"做色键，否则白底会被当成背景透明掉。
	opaque bool
}

const (
	staffrollScrollSpeed = 36 // 像素/秒，约 35 秒滚完 1280px
	staffrollLongSpeed   = 80 // ed_roll 5805px，约 73 秒
)

// beginStaffroll 打开片尾字幕。
func (s *Session) beginStaffroll() state.SessionStep {
	if s.composer == nil || s.res == nil {
		return state.SessionContinue
	}
	// 以素材为准而不是作品开关：有 staffroll_list 或 ed_roll 的作品才需要
	// 阻塞字幕，没有的保持原来的“记录日志后继续”。
	list := "staffroll_list"
	if !s.res.Exists(list) {
		if s.res.Exists("ed_roll") {
			list = "ed_roll"
		} else if image := s.composer.Load("staffroll"); image != nil && image.Height > s.height*2 {
			// MKIE 没有 staffroll_list，只有一张 800x5805 的整幅名单。
			list = "staffroll"
		} else if s.requestStaffrollMovie() {
			return state.SessionAwaitMovie
		} else {
			return state.SessionContinue
		}
	}
	end := s.sessionProperty("#end")
	background := ""
	for _, name := range []string{
		fmt.Sprintf("staffroll_monitor%02d", end),
		fmt.Sprintf("staffroll_%02d", end),
	} {
		if s.res.Exists(name) {
			background = name
			break
		}
	}
	speed := float64(staffrollScrollSpeed)
	opaque := false
	if list == "ed_roll" || list == "staffroll" {
		// ed_roll 是黑底白字，staffroll 是白底黑字，两者都自带背景。
		speed = staffrollLongSpeed
		background = ""
		opaque = list == "staffroll"
	}
	s.staffroll = staffrollState{
		active:     true,
		started:    time.Now(),
		background: background,
		list:       list,
		speed:      speed,
		opaque:     opaque,
	}
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.gameState.SetMessageWindowVisible(false)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("ui: staffroll start background='%s' end=%d", background, end)
	return state.SessionAwaitInput
}

// staffrollActive 供 Pointer 判断是否要吞掉点击。
func (s *Session) staffrollActive() bool {
	return s.staffroll.active
}

// tickStaffroll 在宿主心跳里推进字幕；滚完自动收尾。
func (s *Session) tickStaffroll() {
	if !s.staffroll.active {
		return
	}
	listHeight := s.staffrollListHeight()
	if listHeight <= 0 {
		return
	}
	elapsed := time.Since(s.staffroll.started).Seconds()
	y := float64(s.height) - elapsed*s.staffroll.speed
	if y+float64(listHeight) < 0 {
		s.finishStaffroll()
	}
}

func (s *Session) staffrollListHeight() int {
	if s.composer == nil {
		return 0
	}
	if s.staffroll.list == "" {
		return 0
	}
	image := s.composer.Load(s.staffroll.list)
	if image == nil {
		return 0
	}
	return image.Height
}

// finishStaffroll 关闭字幕并让脚本从 SYSCALL 的下一条继续。
func (s *Session) finishStaffroll() {
	if !s.staffroll.active {
		return
	}
	s.staffroll.active = false
	s.gameState.SetMessageWindowVisible(true)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("ui: staffroll finished")
	s.lastStep = s.finishRun(s.run(commandBudget))
}

// drawStaffroll 把背景和滚动字幕画到当前帧；返回是否画了东西。
func (s *Session) drawStaffroll(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || !s.staffroll.active || s.composer == nil {
		return false
	}
	if s.staffroll.background != "" {
		s.drawImageAt(canvas, s.staffroll.background, 0, 0)
	} else {
		canvas.Clear(0, 0, 0, 255)
	}
	if s.staffroll.list == "" {
		return true
	}
	image := s.composer.Load(s.staffroll.list)
	if image == nil {
		return true
	}
	elapsed := time.Since(s.staffroll.started).Seconds()
	listY := s.height - int(elapsed*s.staffroll.speed)
	stride := image.Width * 4
	if stride <= 0 || len(image.Pixels) < stride*image.Height {
		return true
	}
	left := (s.width - image.Width) / 2
	for row := 0; row < image.Height; row++ {
		targetY := listY + row
		if targetY < 0 || targetY >= canvas.Height {
			continue
		}
		sourceRow := row * stride
		targetRow := targetY * canvas.Width * 4
		for x := 0; x < image.Width; x++ {
			targetX := left + x
			if targetX < 0 || targetX >= canvas.Width {
				continue
			}
			source := sourceRow + x*4
			// 字幕图是黑底文字；只复制非黑像素，背景才透得出来。
			// MKIE 的 staffroll 自带白底，必须整行复制。
			if !s.staffroll.opaque &&
				image.Pixels[source] < 8 && image.Pixels[source+1] < 8 && image.Pixels[source+2] < 8 {
				continue
			}
			target := targetRow + targetX*4
			copy(canvas.Buffer[target:target+4], image.Pixels[source:source+4])
		}
	}
	return true
}
