// 引擎自绘选择肢。原版把 0x1c（简单文本）与 0x1d（adv_select2 带坐标）
// 都画在 ADV 层上，点击按屏幕坐标命中；Flutter 的居中列表会丢掉原版布局，
// 所以这里把两种菜单都画进帧里，Flutter 只负责把 tap 坐标送回来。
package session

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	overscript "kizuna/enginego/brand/overdose/script"
	"kizuna/enginego/core"
)

type choiceMenuItem struct {
	position int // 传给 VM.Choose 的数组下标
	index    int32
	text     string
	image    string
	x, y     int32
	w, h     int32
	textX    int32
	textY    int32
	enabled  bool
}

// setChoiceMenu 把 VM 的 ChoiceEntry 转成绘制项。
// 简单选项（Image 为空）按 sq_adv 的公式居中纵排：
//
//	x = (800-519)/2
//	y = 20 + (600-n*60)/(n+1) + 65*i
//
// 高级选项（0x1d）直接用脚本给的 x/y 与按钮图名。
func (s *Session) setChoiceMenu(entries []overscript.ChoiceEntry) {
	s.choiceMenu = s.choiceMenu[:0]
	n := len(entries)
	for i, e := range entries {
		item := choiceMenuItem{
			position: i,
			index:    e.Index,
			text:     e.Text,
			image:    e.Image,
			enabled:  e.Enabled,
		}
		if item.image == "" {
			item.image = "adv_select"
			item.x = (frameWidth - 519) / 2
			item.y = int32(20 + (600-n*60)/(n+1) + 65*i)
		} else {
			item.x, item.y = e.X, e.Y
		}
		w, h := s.imageSize(item.image, 286, 40)
		if item.image == "adv_select" {
			w, h = s.imageSize(item.image, 519, 40)
		}
		item.w, item.h = int32(w), int32(h)
		// 按钮图左侧是图标区，文字从图标右侧开始；25 号字在 40px 高内
		// 垂直居中（原版 nSelectSize=25）。
		item.textX = item.x + 48
		item.textY = item.y + 7
		s.choiceMenu = append(s.choiceMenu, item)
	}
	s.hoverX, s.hoverY = -1, -1
	advanced := false
	for _, item := range s.choiceMenu {
		if item.image != "" && item.image != "adv_select" {
			advanced = true
			break
		}
	}
	core.Logf("overdose: choice menu n=%d advanced=%v", len(s.choiceMenu), advanced)
}

func (s *Session) imageSize(name string, defW, defH int) (int, int) {
	if s.cache != nil {
		if img := s.cache.Get(name); img != nil && img.Width > 0 && img.Height > 0 {
			return img.Width, img.Height
		}
	}
	return defW, defH
}

// composeChoiceMenu 在 stage/对话框之上画菜单；只有 step==AwaitSelect 才出现。
func (s *Session) composeChoiceMenu(canvas *bishopgfx.BgraCanvas) {
	if len(s.choiceMenu) == 0 {
		return
	}
	for _, item := range s.choiceMenu {
		hovered := item.enabled && s.hoverX >= item.x && s.hoverX < item.x+item.w &&
			s.hoverY >= item.y && s.hoverY < item.y+item.h
		img := item.image
		if hovered && !s.drawImage(canvas, img+"-o", int(item.x), int(item.y)) {
			s.drawImage(canvas, img, int(item.x), int(item.y))
		} else if !hovered {
			s.drawImage(canvas, img, int(item.x), int(item.y))
		}
		color := uint32(0xFFFFFFFF)
		if !item.enabled {
			color = 0xFF808080
		} else if hovered {
			// 原版 clSelectSelect = 16750899 (0xFF9933)。
			color = 0xFFFF9933
		}
		maxW := int(item.w) - 56
		if maxW < 10 {
			maxW = 10
		}
		s.drawText(canvas, item.text, int(item.textX), int(item.textY), 25, 25, maxW, 1, color)
	}
}

// hitChoiceMenu 返回命中的数组下标；disabled 项不返回。
func (s *Session) hitChoiceMenu(x, y int32) (int, bool) {
	for _, item := range s.choiceMenu {
		if !item.enabled {
			continue
		}
		if x >= item.x && x < item.x+item.w && y >= item.y && y < item.y+item.h {
			return item.position, true
		}
	}
	return 0, false
}

// SelectItems 故意返回空：高级菜单由引擎自绘，Flutter 若再画一层会重叠。
// step 仍用 AwaitSelect，宿主据此停止推进对白。
func (s *Session) SelectItems() []core.SelectItem { return nil }
