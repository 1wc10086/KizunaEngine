package config

import (
	"kizuna/engine/core/strutil"
)

// ecstasy_meter.go 解析 bsxx.cfg 的 [ecstasy meter]（満淫電車2 専用）。
//
// 这一代把「言葉責めシステムNEO」的数值全部放在脚本属性里，界面只由引擎
// 按这份配置画：右上角的兴奋/压力/燃烧/连击/剩余时间条，以及每种反应
// 等级对应的数值增量。脚本本身只负责叙事分支。
const EcstasyMeterSection = "ecstasy meter"

// EcstasyMeter 是兴奋度仪表及其数值变量。
type EcstasyMeter struct {
	// Increment 是反应等级 S/A/B/C/D/E 对应的 (兴奋, 压力) 增量。
	Increment map[byte][2]int
	// 变量名均带 #，由 writeOne/readProperty 直接解析。
	ResultVariable    string
	CharacterVariable string
	WeakPointVariable string
	EventVariable     string
	LevelVariable     string
	EcstasyVariable   string
	StressVariable    string
	ComboVariable     string
	IncrementVariable string
	TimeLimitVariable string
	NoWordsVariable   string
	Base              string
	BaseX             int
	BaseY             int
	// BaseWidth/BaseHeight 是 base size；零值表示整张图。
	BaseWidth  int
	BaseHeight int
	Limit      UIButton
	LimitX     int
	LimitY     int
	RedIcon    UIButton
	BlueIcon   UIButton
	RedEnd     UIButton
	BlueEnd    UIButton
	Fire       UIButton
	FireEnd    UIButton
	// FirePos/FireStep 是燃烧槽的落点位置（fire pos / fire next pos），
	// FireEndStep 是 fire graduate next pos。每级一个火焰沿 next 方向叠，
	// 第 4 级用 graduate 帧封顶。Fire/FireEnd 本身是源图/源矩形。
	FirePos     UIPoint
	FireStep    UIPoint
	FireEndStep UIPoint
	Cover       UIButton
	Combo       UIButton
}

// Empty 判断有没有配置这一代仪表。
func (m EcstasyMeter) Empty() bool {
	return m.Base == "" && len(m.Increment) == 0
}

func parseEcstasyMeter(document *IniDocument) EcstasyMeter {
	meter := EcstasyMeter{
		Increment:         map[byte][2]int{},
		ResultVariable:    document.Get(EcstasyMeterSection, "result", "#result"),
		CharacterVariable: document.Get(EcstasyMeterSection, "character no", "#character_no"),
		WeakPointVariable: document.Get(EcstasyMeterSection, "weak point type", "#weak_point"),
		EventVariable:     document.Get(EcstasyMeterSection, "event type", "#event_type"),
		LevelVariable:     document.Get(EcstasyMeterSection, "ecstasy level", "#ecstasy_level"),
		EcstasyVariable:   document.Get(EcstasyMeterSection, "ecstasy point", "#ecstasy"),
		StressVariable:    document.Get(EcstasyMeterSection, "stress point", "#stress"),
		ComboVariable:     document.Get(EcstasyMeterSection, "combo count", "#combo_count"),
		IncrementVariable: document.Get(EcstasyMeterSection, "increment ecstasy", "#increment_ecstasy"),
		TimeLimitVariable: document.Get(EcstasyMeterSection, "time limit", "#time_limit"),
		NoWordsVariable:   document.Get(EcstasyMeterSection, "dont have words", "#dont_have_words"),
		Base:              document.Get(EcstasyMeterSection, "base", ""),
	}
	meter.BaseX, meter.BaseY = parsePoint(document.Get(EcstasyMeterSection, "base pos", ""))
	if size := ParsePositions(document.Get(EcstasyMeterSection, "base size", "")); len(size) >= 2 {
		meter.BaseWidth, meter.BaseHeight = size[0], size[1]
	}
	for _, letter := range []byte{'s', 'a', 'b', 'c', 'd', 'e'} {
		key := "increment value " + string(letter)
		parts := strutil.SplitTrimNonEmpty(document.Get(EcstasyMeterSection, key, ""), ",")
		if len(parts) < 2 {
			continue
		}
		ecstasy, ok1 := parseInt(parts[0])
		stress, ok2 := parseInt(parts[1])
		if ok1 && ok2 {
			meter.Increment[letter-0x20] = [2]int{ecstasy, stress}
		}
	}
	meter.Limit = atlasButtonOr(document, EcstasyMeterSection, "limit number", meter.Base)
	meter.LimitX, meter.LimitY = parsePoint(document.Get(EcstasyMeterSection, "limit pos", ""))
	meter.RedIcon = atlasButtonOr(document, EcstasyMeterSection, "red icon", meter.Base)
	meter.BlueIcon = atlasButtonOr(document, EcstasyMeterSection, "blue icon", meter.Base)
	meter.RedEnd = atlasButtonOr(document, EcstasyMeterSection, "red graduate", meter.Base)
	meter.BlueEnd = atlasButtonOr(document, EcstasyMeterSection, "blue graduate", meter.Base)
	meter.Fire = atlasButtonOr(document, EcstasyMeterSection, "fire icon", meter.Base)
	meter.FireEnd = atlasButtonOr(document, EcstasyMeterSection, "fire graduate", meter.Base)
	// 火焰落点：fire pos 是第 1 级、fire next pos 是每级叠放的偏移、
	// fire graduate next pos 是第 4 级火焰相对第 3 级的偏移。缺省不动
	// 首个落在 atlasButtonOr 给出的 X/Y 上。
	meter.FirePos = UIPoint{X: meter.Fire.X, Y: meter.Fire.Y}
	if point, ok := parseUIPoint(document.Get(EcstasyMeterSection, "fire pos", "")); ok {
		meter.FirePos = point
	}
	if point, ok := parseUIPoint(document.Get(EcstasyMeterSection, "fire next pos", "")); ok {
		meter.FireStep = point
	}
	if point, ok := parseUIPoint(document.Get(EcstasyMeterSection, "fire graduate next pos", "")); ok {
		meter.FireEndStep = point
	}
	meter.Cover = atlasButtonOr(document, EcstasyMeterSection, "cover", meter.Base)
	meter.Combo = atlasButtonOr(document, EcstasyMeterSection, "combo icon", meter.Base)
	return meter
}
