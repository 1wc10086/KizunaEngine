package bdt

import (
	"strings"

	"kizuna/engine/brand/bishop/gfx"
)

// bdt_manin_name.go 还原「名前変更」画面。
//
// 原作的 s_name01 是软键盘示意图，s_name03 只把「決定/戻る/キーボード入力」
// 三块标成命中区——真正的输入依赖 Windows IME。Android 没有日文物理键盘，
// 这里把 s_name01 的 5x10 假名格做成可点区域，保留原图布局，同时给触屏
// 一个可用的输入方式；姓/名分栏与 5 字上限按原作。

type maninKanaKey struct {
	text string
	row  int
	x    int
	y    int
	w    int
	h    int
}

var maninKanaRows = [][]string{
	{"あ", "い", "う", "え", "お"},
	{"か", "き", "く", "け", "こ"},
	{"さ", "し", "す", "せ", "そ"},
	{"た", "ち", "つ", "て", "と"},
	{"な", "に", "ぬ", "ね", "の"},
	{"は", "ひ", "ふ", "へ", "ほ"},
	{"ま", "み", "む", "め", "も"},
	{"や", "", "ゆ", "", "よ"},
	{"ら", "り", "る", "れ", "ろ"},
	{"わ", "", "を", "", "ん"},
}

func maninKanaKeys() []maninKanaKey {
	keys := make([]maninKanaKey, 0, 50)
	for row, values := range maninKanaRows {
		for col, value := range values {
			if value == "" {
				continue
			}
			keys = append(keys, maninKanaKey{
				text: value,
				row:  row,
				x:    34 + col*32,
				y:    100 + row*31,
				w:    30,
				h:    28,
			})
		}
	}
	return keys
}

func (s *Session) maninNameFamily() string {
	if s.manin == nil {
		return ""
	}
	return s.manin.nameFamily
}

func (s *Session) maninNameGiven() string {
	if s.manin == nil {
		return ""
	}
	return s.manin.nameGiven
}

func (s *Session) maninInitNameScreen() {
	if s.manin == nil {
		return
	}
	name := strings.TrimSpace(s.manin.global.Name)
	parts := strings.Fields(name)
	s.manin.nameFamily = ""
	s.manin.nameGiven = ""
	s.manin.nameField = 0
	if len(parts) > 0 {
		s.manin.nameFamily = parts[0]
	}
	if len(parts) > 1 {
		s.manin.nameGiven = strings.Join(parts[1:], "")
	}
}

func (s *Session) maninNameAppend(text string) {
	if s.manin == nil {
		return
	}
	var current *string
	if s.manin.nameField == 0 {
		current = &s.manin.nameFamily
	} else {
		current = &s.manin.nameGiven
	}
	if len([]rune(*current)) >= 5 {
		return
	}
	*current += text
}

func (s *Session) maninNameBackspace() {
	if s.manin == nil {
		return
	}
	var current *string
	if s.manin.nameField == 0 {
		current = &s.manin.nameFamily
	} else {
		current = &s.manin.nameGiven
	}
	runes := []rune(*current)
	if len(runes) == 0 {
		return
	}
	*current = string(runes[:len(runes)-1])
}

func (s *Session) maninCommitName() {
	if s.manin == nil {
		return
	}
	name := strings.TrimSpace(s.manin.nameFamily + " " + s.manin.nameGiven)
	if name == "" {
		return
	}
	runes := []rune(name)
	if len(runes) > 11 {
		runes = runes[:11]
	}
	s.manin.global.Name = string(runes)
	saveManinGlobal(s.maninSaveDir(), s.manin.global)
	s.maninExitScreen()
}

func (s *Session) drawManinName(canvas *gfx.BgraCanvas) {
	if s.manin == nil || canvas == nil {
		return
	}
	canvas.Clear(0, 0, 0, 255)
	s.maninDrawImage(canvas, "s_name01", 0, 0)
	family := s.manin.nameFamily
	given := s.manin.nameGiven
	if family == "" {
		family = "＿＿"
	}
	if given == "" {
		given = "＿＿"
	}
	s.maninDrawText(canvas, family, 20, 14, 6, 240, 1, 0xffffffff)
	s.maninDrawText(canvas, given, 20, 14, 42, 240, 1, 0xffffffff)
	underlineY := 34
	if s.manin.nameField == 1 {
		underlineY = 70
	}
	canvas.Fill(10, underlineY, 220, 2, 80, 200, 255, 255)
	for _, key := range maninKanaKeys() {
		if s.manin.hover >= 0 && s.manin.hover == key.row {
			canvas.FillBlend(key.x, key.y, key.w, key.h, 80, 80, 160, 90)
		}
	}
	// 決定/戻る/かな文字は s_name01 已经烘焙；这里不再重绘，避免双影，
	// 只保留姓名栏和悬停高亮。
}

func (s *Session) pointerManinName(x, y int) bool {
	if s.manin == nil {
		return false
	}
	// 決定 / 戻る（s_name03 的命中块）
	if x >= 384 && x < 495 && y >= 16 && y < 72 {
		s.maninCommitName()
		return true
	}
	if x >= 495 && x < 624 && y >= 16 && y < 72 {
		s.maninExitScreen()
		return true
	}
	// 姓 / 名 栏
	if x >= 0 && x < 260 && y >= 0 && y < 38 {
		s.manin.nameField = 0
		return true
	}
	if x >= 0 && x < 260 && y >= 38 && y < 78 {
		s.manin.nameField = 1
		return true
	}
	if x >= 34 && x < 84 && y >= 410 && y < 436 {
		s.maninNameBackspace()
		return true
	}
	if x >= 90 && x < 150 && y >= 410 && y < 436 {
		s.maninNameAppend("　")
		return true
	}
	for _, key := range maninKanaKeys() {
		if x >= key.x && x < key.x+key.w && y >= key.y && y < key.y+key.h {
			s.maninNameAppend(key.text)
			return true
		}
	}
	return true
}
