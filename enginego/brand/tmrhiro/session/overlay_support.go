package session

import (
	"time"

	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/brand/tmrhiro/grd"
	"kizuna/enginego/brand/tmrhiro/save"
	"kizuna/enginego/brand/tmrhiro/script"
)

// 本文件补齐 overlay.go 用到的会话级小工具：槽位读取、缩略图、时间格式化、
// 场景封面、确认框上下文。放在一起是因为它们都只是"界面要用的查询"，
// 不含绘制逻辑。

// confirmState 是确认框的上下文。
//
// 原版只在真正需要确认的地方弹框（覆盖已有存档、删除存档、回标题会丢进度、
// 退出游戏、把达成率改成 100%/0%）。上下文用一个结构体而不是裸字符串，
// 是因为确认后要按"当时"的槽位/页面执行，而不是按弹框时的全局状态。
type confirmState struct {
	prompt string
	// action 是确认后的动作（save/load/delete/title/exit/percent100/percent0）。
	action string
	// slot 是相关槽位（存档类动作有效）。
	slot uint32
	// value 是数值参数（percent 动作的百分比）。
	value int
}

// clockBase 是存档时间显示的"基准时刻"（本次打开游戏的时刻）。
//
// 原版日期来自系统时钟；存档里已经有 SavedAt 时直接用存档文本，
// 没有时用它格式化占位。
func (s *Session) clockBase() time.Time {
	if s.openedAt.IsZero() {
		return time.Now()
	}
	return s.openedAt
}

// slotEntry 取槽位元信息；没有存档时返回 ok=false。
func (s *Session) slotEntry(slot uint32) (save.Slot, bool) {
	if s.store == nil || !s.store.Exists(slot) {
		return save.Slot{}, false
	}
	entry, err := s.store.Load(slot)
	if err != nil {
		return save.Slot{}, false
	}
	return entry, true
}

// drawThumb 把缩略图缩放到格子里（最近邻，与界面其它贴图一致）。
func (s *Session) drawThumb(thumb save.Thumb, x, y, w, h int) {
	if thumb.Width <= 0 || thumb.Height <= 0 || len(thumb.Pixels) == 0 {
		return
	}
	img := &grd.Image{
		Width:   thumb.Width,
		Height:  thumb.Height,
		CanvasW: thumb.Width,
		CanvasH: thumb.Height,
		Pixels:  thumb.Pixels,
	}
	s.stage.DrawImageScaled(img, x, y, w, h, 255)
}

// sceneCover 取场景脚本的封面：扫该脚本的前几张 CG 记录，取第一张能解的。
//
// 原版的场景回想页封面就是"该片段的第一张 CG"。脚本很小（几十 KB），
// 现场扫一遍比维护另一份索引表更不容易和剧本不同步。
func (s *Session) sceneCover(scriptName string) *grd.Image {
	if scriptName == "" || s.set == nil || s.stage == nil {
		return nil
	}
	records, err := s.set.Entry(scriptName)
	if err != nil {
		return nil
	}
	parsed, err := script.Parse(scriptName, records)
	if err != nil {
		return nil
	}
	for _, record := range parsed.Records {
		if record.Op != 2 || record.Arg != 0x0000 {
			continue
		}
		name := trimGraphicPayload(string(record.Raw))
		if name == "" {
			continue
		}
		if img := s.stage.ResolveImage(name); img != nil && img.Width > 0 {
			return img
		}
	}
	return nil
}

// trimGraphicPayload 去掉 op2/0x00 载荷里的"层号 + 名字"前缀。
//
// 载荷形如 `01 00 BG0500_a` 或 `14 00 attention`：前两个字节是层号/标志，
// 名字从偏移 2 开始。个别记录只有名字。
func trimGraphicPayload(raw string) string {
	if len(raw) <= 2 {
		return ""
	}
	// 前两个字节是控制字段，且都不是可打印 ASCII（层号 ≤ 0x20 或为 0）。
	head := []byte(raw)
	if head[0] <= 0x20 && head[1] <= 0x20 {
		raw = raw[2:]
	}
	return trimSpace(raw)
}

// padSlot 把槽号补成三位（原版显示 No.001）。
func padSlot(slot uint32) string {
	text := itoa(int(slot))
	for len(text) < 3 {
		text = "0" + text
	}
	return text
}

// trimSpace / itoa 是界面文字用的小工具（避免为一处调用引入 strings 依赖）。
func trimSpace(value string) string {
	start := 0
	end := len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == 0) {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == 0) {
		end--
	}
	return value[start:end]
}

// titleButton 是标题/界面按钮的命中矩形（供 overlay 与 screen 共用）。
func rectFor(button config.ToolbarButton, originX, originY int) config.Rect {
	return config.Rect{X: button.X + originX, Y: button.Y + originY, W: button.W, H: button.H}
}

var _ = time.Now

// 缩略图尺寸：原版的 SAVE_THUM_SIZE 就是 128x72。
const (
	thumbWidth  = 128
	thumbHeight = 72
)

// shrinkFrame 把整帧缩成缩略图（最近邻，与界面其它贴图一致）。
//
// 抽成函数而不是在 Save 里内联：存档路径不该关心采样算法，
// 而缩略图格式（save.Thumb）是存档包的契约。
func shrinkFrame(frame *grd.Image, width, height int) (save.Thumb, bool) {
	if frame == nil || width <= 0 || height <= 0 || frame.Width <= 0 || frame.Height <= 0 {
		return save.Thumb{}, false
	}
	out := make([]byte, width*height*4)
	sw, sh := frame.Width, frame.Height
	for y := 0; y < height; y++ {
		sy := y * sh / height
		srcRow := sy * sw
		dstRow := y * width
		for x := 0; x < width; x++ {
			sx := x * sw / width
			si := (srcRow + sx) * 4
			di := (dstRow + x) * 4
			if si+3 >= len(frame.Pixels) {
				continue
			}
			out[di] = frame.Pixels[si]
			out[di+1] = frame.Pixels[si+1]
			out[di+2] = frame.Pixels[si+2]
			out[di+3] = 255
		}
	}
	return save.Thumb{Width: width, Height: height, Pixels: out}, true
}
