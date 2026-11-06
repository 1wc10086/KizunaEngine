package config

import (
	"kizuna/engine/core/strutil"
)

// TitlePhaseSection 是 bsxx.cfg 里主界面（标题菜单）的配置段。
const TitlePhaseSection = "title phase"

// TitleMenuEntry 是主界面上的一个菜单项。
//
// 原引擎不把菜单项画进脚本，而是读这一段配置：每项给出"按钮图 + 帧数 + 坐标"，
// 由引擎自己合成标题画面并做命中判定。我们沿用同一份数据，好处是换游戏不用改代码。
type TitleMenuEntry struct {
	// Key 是配置里的键名（special / op / start / load / museum / config / quit），
	// 也是宿主回传点击时的标识。
	Key    string
	Image  string
	Frames int
	X      int
	Y      int
	// Frame 是 HoverOnly 按钮悬停时画的帧号；-1 表示沿用旧行为（不指定帧）。
	Frame int
	// HoverOnly 为真时按钮平时不画，只在悬停时画 Frame 那一帧（Ancient 标题）。
	HoverOnly bool
	// Clip/Atlas/AtlasAxis 是 BSXScript 1.0「満淫電車2」这一代的精灵图写法：
	// 菜单项不再有独立图片，而是 Base 大图里的一个矩形。Atlas 为真时
	// Image 等于 TitlePhase.Base，Clip 是单帧源矩形，帧沿 AtlasAxis 排列
	// （0 纵向、1 横向），Frames 是配置里的 patterns 数量。
	Clip      [4]int
	Atlas     bool
	AtlasAxis int
}

// TitlePhase 是完整的主界面配置。
type TitlePhase struct {
	// BgmName 是标题画面播放的 BGM；为空表示沿用当前 BGM。
	BgmName string
	// Base 是标题底图（title_base）。
	Base string
	// BaseClip 是底图的源矩形。BSXScript 1.0 的 title_base 是 800x784 的
	// 精灵图：上半部分 800x600 是标题画，下半部分是菜单按钮。配置用
	// `base size = 0, 0, 800, 600` 指定只露上半部分；不裁会把按钮图当
	// 标题画的一部分画在屏幕外。
	BaseClip [4]int
	// BaseX/BaseY 是标题底图的屏幕落点。零值表示左上角对齐。
	//
	// ヒメドレイ 的 title_base 是 800x1012 的合成图，原引擎把它放在
	// (0,-412)：屏幕上露出的是图像下半部分的封面和右侧菜单留白区。
	// 如果按 (0,0) 画，菜单会盖在封面上半部分，和原作完全不同。
	BaseX int
	BaseY int
	// HitTable 是可选的命中表图（title_hit）；为空时用按钮图自身的矩形。
	HitTable string
	// Entries 按配置顺序排列，用于绘制与命中。
	Entries []TitleMenuEntry
	// UseSpecialMode 对应 `use special mode`：为真时 special 菜单才有可能出现。
	UseSpecialMode bool
	// UseConditions 是 special 菜单的显示条件（如 `@harem == 0xf`）。
	// 原引擎按脚本变量判断；空字符串表示没有条件。
	UseConditions string
	// ResumeAfterTitleCall 表示 START 不按标签名跳转，而是恢复标题时的
	// VM 快照、从 system call "title" 的下一条指令继续。
	//
	// 学園（GK）的 boot 标签在 `system call "title"` 之后紧接序章标签
	// （prologue 就在 title 后面 29 字节），原版 START 就是继续执行这段。
	// 若按名称 Seek 到 prologue，会跳过 title 之后、prologue 之前的初始化
	// 指令；若重新 Seek 到 boot，又会再次停在同一个 title 调用上形成死循环。
	ResumeAfterTitleCall bool
}

// Empty 判断这份配置是否可用；不可用时调用方应跳过主界面，直接进剧情。
func (t TitlePhase) Empty() bool {
	return len(t.Entries) == 0 && t.Base == ""
}

// titleMenuKeys 固定了菜单项的顺序与识别方式。
//
// 顺序即绘制顺序，也是"默认选中项"的顺序；这里写死而不是遍历 INI 段，
// 是因为配置里还混着 bgm name / base / hit table 这些非菜单键，
// 靠前缀白名单比靠位置更稳。special 放在最前：原作的隐藏菜单按钮在
// 列表顶部（y = 362 + 43 * 0），实际绘制位置由坐标决定，不依赖这个顺序。
var titleMenuKeys = []string{"special", "op", "start", "load", "museum", "museum_music", "museum_cg", "museum_scene", "config", "quit"}

func parseTitlePhase(document *IniDocument) TitlePhase {
	phase := TitlePhase{
		BgmName:        document.Get(TitlePhaseSection, "bgm name", ""),
		Base:           document.Get(TitlePhaseSection, "base", ""),
		HitTable:       document.Get(TitlePhaseSection, "hit table", ""),
		UseSpecialMode: document.GetInt(TitlePhaseSection, "use special mode", 0) != 0,
		UseConditions:  document.Get(TitlePhaseSection, "use conditions", ""),
	}
	// `base size` 是 BSXScript 1.0 的精灵图底图裁剪矩形。旧作品没有这个键，
	// 零值保持"整图铺开"的行为。
	if size := ParsePositions(document.Get(TitlePhaseSection, "base size", "")); len(size) >= 4 &&
		size[2] > 0 && size[3] > 0 {
		copy(phase.BaseClip[:], size[:4])
	}
	for _, key := range titleMenuKeys {
		value := document.Get(TitlePhaseSection, key, "")
		if len(value) == 0 {
			continue
		}
		// BSXScript 1.0 的写法是 `0, 600, 250, 46` + `op patterns = 0, 2`
		// + `op pos = 525, 184`：源矩形、排列、屏幕落点分三个键。它和旧写法
		// 的区别不是"有没有图名"，而是前四段全是数字且存在 pos 键；按这个
		// 特征分流，避免把旧作品的图名 `title_menu_op` 当成矩形。
		if button, ok := parseAtlasButton(document, TitlePhaseSection, key, phase.Base); ok && button.Image != "" {
			phase.Entries = append(phase.Entries, TitleMenuEntry{
				Key:       key,
				Image:     button.Image,
				Frames:    button.Frames,
				X:         button.X,
				Y:         button.Y,
				Frame:     -1,
				Clip:      button.Clip,
				Atlas:     true,
				AtlasAxis: button.AtlasAxis,
			})
			continue
		}
		// 旧写法形如 "title_menu_start, 2, 772, 292"：图片名、帧数、X、Y。
		// 坐标可能是 "62+150*0" 这样的算式（黒の教室），必须走与其它布局共用的
		// parseCoordinate；早期这里用 strconv.Atoi，算式解析失败会被静默当成 0，
		// 六个按钮全部叠在左上角，只剩最后绘制的 quit 可见。
		parts := strutil.SplitTrimNonEmpty(value, ",")
		if len(parts) == 0 {
			continue
		}
		entry := TitleMenuEntry{Key: key, Image: parts[0], Frames: 1, Frame: -1}
		if len(parts) > 1 {
			if frames, ok := parseInt(parts[1]); ok && frames > 0 {
				entry.Frames = frames
			}
		}
		if len(parts) > 2 {
			entry.X = parseCoordinate(parts[2])
		}
		if len(parts) > 3 {
			entry.Y = parseCoordinate(parts[3])
		}
		phase.Entries = append(phase.Entries, entry)
	}
	return phase
}
