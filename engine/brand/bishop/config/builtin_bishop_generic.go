package config

import (
	"strings"

	"kizuna/engine/core/binutil"
)

// builtin_bishop_generic.go 是"配置缺段"时的通用兜底。
//
// 旧一代 Bishop 作品把一部分界面编译在 exe 里：bsxx.cfg 可能只写了地图/
// 立绘/状态画面，标题与对话框两段完全不存在。此时引擎会：
//   - 没有标题 → 一进游戏就落进 opening，玩家看不到主界面；
//   - 没有 [message window] → 不画原版对话框，宿主 Flutter 会叠一层
//     完全不同的信息框，工具按钮也全部消失。
//
// 这里只做"从素材本身能确定的事"：标题按钮图自带屏幕坐标（BSG 图头），
// 对话框底图自带尺寸。配置里已经有对应段的作品一律不动，所以不会影响
// 已经适配好的侵蝕/淫獄/MK2IE 等作品。

// BuiltinImage 是解析 BSG 图头得到的只读信息。
type BuiltinImage struct {
	Width   int
	Height  int
	OffsetX int
	OffsetY int
	Frames  int
}

// ParseBuiltinImage 从 BSG 图头取尺寸/偏移/帧数。
//
// 参数只需要文件前 0x40 字节：图头就在开头，不必把整张 1MB 的背景读进来。
func ParseBuiltinImage(head []byte) (BuiltinImage, bool) {
	if len(head) < 0x40 || !strings.HasPrefix(string(head), "BSS-Graphics") {
		return BuiltinImage{}, false
	}
	width := int(binutil.U16(head[0x16:]))
	height := int(binutil.U16(head[0x18:]))
	if width <= 0 || height <= 0 {
		return BuiltinImage{}, false
	}
	frames := int(head[0x2D])
	if frames <= 0 {
		frames = 1
	}
	return BuiltinImage{
		Width:   width,
		Height:  height,
		OffsetX: int(int16(binutil.U16(head[0x20:]))),
		OffsetY: int(int16(binutil.U16(head[0x22:]))),
		Frames:  frames,
	}, true
}

// ApplyBuiltinGenericFallbacks 补齐缺失的标题与对话框。
//
// 返回值只用于日志，调用方不需要据此改流程。
func ApplyBuiltinGenericFallbacks(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || probe.Exists == nil {
		return false
	}
	changed := false
	if cfg.Title.Empty() {
		if applyGenericTitle(cfg, probe) {
			changed = true
		}
	}
	if cfg.Message.Empty() {
		if applyGenericMessage(cfg, probe) {
			changed = true
		}
	}
	return changed
}

// genericTitleMenu 是标题菜单的候选素材名，按"先新后旧"排列。
// 同一个键只取第一张存在的图；坐标优先用图头里的屏幕坐标。
var genericTitleMenu = []struct {
	key    string
	images []string
}{
	{"op", []string{"title_menu_opening", "title_menu_op"}},
	{"start", []string{"title_menu_start", "title_menu_begin"}},
	{"load", []string{"title_menu_load"}},
	{"museum", []string{"title_menu_museum"}},
	{"config", []string{"title_menu_config", "title_menu_setting"}},
	{"quit", []string{"title_menu_quit"}},
}

func applyGenericTitle(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if !probe.Exists("title_base") || probe.Header == nil {
		return false
	}
	entries := make([]TitleMenuEntry, 0, len(genericTitleMenu))
	panelRight := cfg.Width
	panelBottom := cfg.Height
	if probe.Width > 0 {
		panelRight = probe.Width
	}
	if probe.Height > 0 {
		panelBottom = probe.Height
	}
	fallbackY := 0
	for _, candidate := range genericTitleMenu {
		image := ""
		for _, name := range candidate.images {
			if probe.Exists(name) {
				image = name
				break
			}
		}
		if image == "" {
			continue
		}
		info, ok := probe.Header(image)
		if !ok {
			continue
		}
		frames := genericTitleFrames(info)
		entry := TitleMenuEntry{Key: candidate.key, Image: image, Frames: frames, Frame: -1}
		if info.OffsetX != 0 || info.OffsetY != 0 {
			// 图头里的屏幕坐标是原作摆放位置，最可信。
			entry.X, entry.Y = info.OffsetX, info.OffsetY
		} else if panelRight > 0 && panelBottom > 0 {
			// 没有烘焙坐标时退到右下角竖排：旧式 Bishop 标题几乎都把
			// 菜单放在画面右下。按钮图高度按帧切，行距取 8px。
			cellHeight := info.Height / max(1, frames)
			entry.X = panelRight - info.Width - 20
			if fallbackY == 0 {
				fallbackY = panelBottom - 20
			}
			fallbackY -= cellHeight + 8
			entry.Y = fallbackY
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return false
	}
	cfg.Title = TitlePhase{Base: "title_base", Entries: entries}
	if probe.Exists("title_hit") {
		cfg.Title.HitTable = "title_hit"
	}
	return true
}

// genericTitleFrames 猜按钮图是几帧竖排。
//
// 标题按钮的 BSG 头经常写 FrameCount=1，但实际是"常态/悬停"两帧竖排
// （牝教師2 的 120x96 就是两帧 48px）。只在高宽比明显偏竖、且能整除时
// 才按两帧处理，避免把一张正常高度的单图切成两半。
func genericTitleFrames(info BuiltinImage) int {
	if info.Frames > 1 {
		return info.Frames
	}
	if info.Height >= 64 && info.Height%2 == 0 && info.Height/2 >= 24 && info.Height > info.Width/2 {
		return 2
	}
	return 1
}

func applyGenericMessage(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	base := ""
	if probe.Exists("msg_base") {
		base = "msg_base"
	} else if probe.Exists("message") {
		base = "message"
	}
	if base == "" {
		return false
	}
	width := 0
	height := 0
	offsetX := 0
	offsetY := 0
	if probe.Header != nil {
		if info, ok := probe.Header(base); ok {
			width, height = info.Width, info.Height
			offsetX, offsetY = info.OffsetX, info.OffsetY
		}
	}
	if width <= 0 || height <= 0 {
		return false
	}
	screenWidth := cfg.Width
	if screenWidth <= 0 {
		screenWidth = probe.Width
	}
	if screenWidth <= 0 {
		screenWidth = width
	}
	screenHeight := cfg.Height
	if screenHeight <= 0 {
		screenHeight = probe.Height
	}
	if screenHeight <= 0 {
		screenHeight = 600
	}
	baseX, baseY := offsetX, offsetY
	if baseX == 0 && baseY == 0 {
		// 原地作把对话框贴在屏幕底部；只有图头没有坐标时才需要自己算。
		baseY = screenHeight - height
	}
	// 正文区域按底图比例取：大多数作品的文字都从左侧 3% 起、宽度占 78%，
	// 底部 20px 留给工具条。字体 16/行高 22 与 800x600 的比例一致。
	textX := width * 3 / 100
	textY := height * 24 / 100
	textW := width * 78 / 100
	textH := height * 58 / 100
	cfg.Message = MessageWindow{
		Base: base, BasePosSet: true, BaseX: baseX, BaseY: baseY,
		Speed: 100, BgColor: 0,
		NameX:     textX,
		NameY:     6,
		NameW:     textW,
		NameH:     22,
		TextX:     textX,
		TextY:     textY,
		TextW:     textW,
		TextH:     textH,
		NameColor: 0xffff8822,
	}
	if probe.Exists("unagashi") {
		cfg.Message.Unagashi = "unagashi"
		cfg.Message.UnagashiX = width - 100
		cfg.Message.UnagashiY = height - 50
		cfg.Message.UnagashiCellW = 32
		cfg.Message.UnagashiCellH = 32
		cfg.Message.UnagashiColumns = 1
		cfg.Message.UnagashiFrames = 30
		cfg.Message.UnagashiSpeed = 16
	}
	if cfg.Font.Size == 0 {
		cfg.Font = FontSettings{
			Size: 16, Width: 20, Height: 22, Weight: 1000,
			Edge: 1, Shadow: 1, LineLength: max(16, textW/20), Lines: 3,
		}
	}
	return true
}
