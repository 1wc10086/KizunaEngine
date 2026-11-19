package config

// config_screen_legacy.go 解析 bsxx.cfg 的旧式 [config screen]。
//
// 这套画面和 define.bsa:system_config_ex.layout 是两条不同的技术路线：
//   - layout 版把每个控件/说明/页签都写成资源树；
//   - 旧式版只给"底图 + 命中表 + cfg_* 素材 + 一组坐标"，控件状态由引擎
//     按设置值自己画。牝教師3、特別授業3SLG 以及大量 2010 年代 Bishop 作品
//     都属于后者，所以必须单独支持，否则标题/工具条的"設定"只能退回宿主。
//
// 坐标全部是游戏坐标系下的绝对像素；`1366 - 36`、`735, 457` 这类写法由
// parseCoordinate 负责算式求值，和 [title phase] 共用同一套解析器。

const (
	ConfigScreenSection     = "config screen"
	DetailedSoundSection    = "detailed sound setting"
	DetailedEffectSection   = "detailed effect setting"
	DetailedLiquidSection   = "detailed liquid setting"
	legacyMaxVolumeChannels = 16
	legacyMaxCharacters     = 16
	legacyMaxColorChannels  = 4
)

// LegacyConfigScreen 是旧式设置画面的完整布局。
type LegacyConfigScreen struct {
	Base     string
	HitTable string
	// Backdrop 是 Base 下面的一层整屏背景。牝教師 MK 的设置内容 cfg_ex 只有
	// 600x404，必须垫在 800x600 的白色装饰底图 config_base 上；其它作品的
	// 设置底图本身就是整屏，留空即可。
	Backdrop string
	// Menu 是 Base/Backdrop 之上的左侧菜单覆盖图（201x600）。零值保持旧行为：
	// 不叠菜单；有值时覆盖整屏之上的左侧竖条（按钮/控件仍在菜单上层绘制）。
	Menu string
	// TitleLabel 是左上角 plaque 里的大号模式名（放課後2 的 cfg_title）。
	// 设置页固定画 CONFIG 那一帧；留空时 plaque 保持底图原样。
	TitleLabel UIButton
	// BaseX/BaseY 是 Base 的屏幕坐标（控件坐标仍是绝对屏幕坐标，不随它偏移）。
	// 零值保持旧行为：Base 画在 (0,0)。
	BaseX       int
	BaseY       int
	CheckMark   string
	RadioButton string
	// CheckMarkAtlas/RadioButtonAtlas 是精灵图世代的勾选/单选图标：
	// Image 仍写入 Base，具体源矩形在这里。零值时沿用独立图片。
	CheckMarkAtlas   UIButton
	RadioButtonAtlas UIButton
	// CheckMarkFrames / RadioFrames 是勾选/单选图标的帧数。
	// 大多数作品是单帧（底图已有空框，只叠勾）；KD 的 cfg_mark 是
	// 100x108 的三帧图（绿/青/黄三色椭圆），必须按 3 帧切，否则一次画三颗。
	CheckMarkFrames int
	RadioFrames     int

	// 这些开关只有坐标：底图上已经画好标签，引擎只负责把勾选/单选图标
	// 贴到对应位置。
	ScreenMode UIPoint
	SquareMode UIPoint
	AllEffect  UIPoint
	CeaseSkip  UIPoint
	SkipUnread UIPoint
	// VoiceLoop 是顶部的「ボイスループ再生」开关。
	VoiceLoop UIPoint
	// VoiceLoopOff 与 SkipUnreadOff 非零时，表示该作品把 ON/OFF 画成两个独立点：
	// VoiceLoop/SkipUnread 只负责「开」的位置，这两个负责「关」的位置，各自按设置值
	// 决定是否画标记。为 (0,0) 时退回单点勾选，原有作品依赖这一行为，零值必须保持不变。
	VoiceLoopOff  UIPoint
	SkipUnreadOff UIPoint
	// AutoSelectMarkFrame / SkipModeMarkFrame 选择标记图的帧：
	// 不同作品的 cfg_mark 含义不同——KD 是绿/青/黄三色椭圆，AFDVD 是
	// 方框/圆点/菱形三形状。零值即第 0 帧，旧作品不受影响。
	AutoSelectMarkFrame int
	SkipModeMarkFrame   int
	// SkipModes 是「スキップ処理」的三个单选项（通常/高速/次の選択肢まで）。
	SkipModes []UIPoint
	// RadioGroups 是旧式设置里除"自动選択/スキップ処理"之外的互斥选项组。
	// AFDVD 有画面効果（なし/コマ数固定/時間固定）、計量設定（なし/通常/絶倫）
	// 和 10 项射精位置；这些在原作里都是单选，键名决定写回哪个 GameSettings 字段。
	RadioGroups []LegacyRadioGroup

	EffectDetail UIButton
	// LiquidDetail 是 [config screen] 的 "liquid detail" 按钮（汁設定の詳細）。
	// 学園2IE 的底图只画标签，按钮/页面都在 [detailed liquid setting] 里。
	LiquidDetail UIButton
	AutoSelect   []UIPoint
	// BgmSelectors 是「BGM曲調」的两个单选项（ノーマル/ハード）。
	// 牝教師4 把它放在 [config screen] 里，而不是 layout 版的功能组；
	// 值写回 GameSettings.BgmSelect，发 BGM 事件时按 bgm_select 表换曲。
	BgmSelectors []UIPoint
	// 以下是 Tbxdl 世代（特別病棟）的设置项选项图：
	// 底图只画标签，cfg_itemN 是不同的"值"图（OFF/ON、通常/高速/…），
	// 引擎按当前设置值取帧贴上去。其它作品留空，不影响原有行为。
	SkipUnreadButton   UIButton
	SkipSpeedButton    UIButton
	VoiceRepeatButton  UIButton
	ScreenModeButton   UIButton
	ScreenEffectButton UIButton
	JuiceButton        UIButton
	// TextSpeedBar / AutoSpeedBar 是 11 帧的横向值条（cfg_item4/6）。
	TextSpeedBar UIButton
	AutoSpeedBar UIButton
	// Speeds[0] 是文字速度，[1] 是自动播放速度。
	Speeds []LegacyGauge
	// VoiceMode 四项互斥：0 只播一次 / 1 循环到点击 / 2 到同角色下一句 /
	// 3 到任何角色下一句。
	VoiceMode []UIPoint
	// Colors 是对话框底色的 RGBA 四条滑杆。
	Colors []LegacyGauge
	// Volumes 顺序固定为 master/BGM/SE/環境音/音声/BGV/BGM減衰率。
	Volumes    []LegacyGauge
	Characters []LegacyCharacter
	// ToggleHitWidth 是勾选/单选控件的横向命中宽度（0/28 表示只命中图标）。
	// AFDVD 的标签文字在图标右侧，玩家会直接点文字；给整行一个更宽的命中区。
	ToggleHitWidth int
	// ExtraToggles 是主设置页上额外的复选框。
	//
	// 旧结构只有固定语义的 ScreenMode/AllEffect 等字段，无法表达
	// 「アンチエイリアス」「カットイン」这种作品专属开关；ヒメドレイ 的
	// cfg_ex 把它们和画面効果画在同一块面板里。键名由 session 侧映射到
	// GameSettings，未知键只显示勾选状态。
	ExtraToggles []LegacyToggle
	// CharacterOffFrame 是角色页签"语音关闭"时画哪一帧；-1 表示不区分。
	// KD 的 cfg_chN 第 0 帧是彩色头像、第 1 帧是灰头像+红叉；旧逻辑把
	// 第 1 帧当"选中"，会把当前角色画成禁用。
	CharacterOffFrame int
	// CharacterSelectedFrame 是当前选中角色的帧号；0 沿用旧行为（选中=第 1 帧）。
	// GK2 用第 2 帧表示选中，与语音关闭的灰帧区分开。
	CharacterSelectedFrame int
	// ValueGroups 是取值型选项：每个按钮代表一个设置值，绘制时当前值取第 2 帧。
	// 与单点勾选、值图互不影响；零值时不参与任何逻辑。
	ValueGroups []LegacyValueGroup

	Reset UIButton
	Back  UIButton
	Title UIButton
	Exit  UIButton
	// ModeSave/ModeLoad/ModeConfig 是设置在存档画面底部的三个页签（AFDVD）。
	// 原版这一排是"記録/呼出/環境設定"，点前两个直接切到对应数据画面。
	ModeSave   UIButton
	ModeLoad   UIButton
	ModeConfig UIButton

	Effect *LegacyEffectPage
	Sound  *LegacySoundPage
	Liquid *LegacyLiquidPage
}

// LegacyGauge 是一条音量/速度滑杆：底条 + 两端按钮。
type LegacyGauge struct {
	Bar UIButton
	Min UIButton
	Max UIButton
	// Knob 是竖向滑杆的滑块（KD 的 cfg_knob）。Bar 为空时，
	// Knob 的 X/Y 表示轨道左上角，TrackHeight 表示轨道高度。
	Knob UIButton
	// Vertical 表示竖向滑杆（KD 的音量/速度/颜色条）。
	Vertical bool
	// ValueFrame 表示 Bar 是多帧"值图"（Tbxdl 的 cfg_item4/6）：
	// 按当前值取帧，而不是只画第 0 帧再加一条标记线。
	// 其它作品的 Bar 即使有多帧也保持原有行为。
	ValueFrame bool
	// TrackHeight 是竖向轨道的像素高度（不含滑块高度）。
	TrackHeight int
	// Dots 是离散刻度（AFDVD 的"遅・・・・速"十连点）：值按
	// round(value/max*(len(Dots)-1)) 取当前点，点哪个点就设成哪个值。
	// 非空时不再画底条/滑块。
	Dots []UIPoint
	// DotMark 是当前刻度上的标记图（cfg_mark 的第 3 帧菱形）。
	DotMark UIButton
	// MarkerOnly 表示底图已经画好横条，引擎只画当前位置的竖线标记。
	// Knob.X/Y 是轨道左上角，TrackWidth 是轨道宽度，TrackHeight 是标记高度。
	MarkerOnly bool
	TrackWidth int
	// Fill 表示 Bar 是"已填充段"：按值裁剪宽度，底槽由底图画好（GK2 音量条）。
	Fill bool
	// HorizontalKnob 表示 Knob 沿 TrackWidth 横向移动（GK2 速度滑块）。
	HorizontalKnob bool
	// Steps 大于 0 时把值量化到 Steps 档；0 为连续取值。
	Steps int
}

// LegacyRadioGroup 是一组坐标固定的单选控件。
//
// Key 必须与 session 的 legacyRadioValue/setLegacyRadioValue 认识的名字一致：
// screen effect / measure / auto select pos。MarkFrame 选择 cfg_mark 的帧
// （0 方框、1 圆点、2 菱形）。
type LegacyRadioGroup struct {
	Key       string
	Points    []UIPoint
	MarkFrame int
	// Labels 是可选的选项文字。底图没有把这些字烤进去的作品（AFDVD 的
	// 射精位置列表）由引擎用内嵌字体画在每个点右侧；其它作品留空。
	Labels []string
	// HitWidth 是本组单选项的横向命中宽度（0/28 表示只命中图标）。
	HitWidth int
}

// LegacyCharacter 是一个角色的个别音量按钮与"詳細"按钮。
type LegacyCharacter struct {
	Button UIButton
	Detail UIButton
}

// LegacyValueGroup 是一组取值型选项：每个按钮代表一个设置值，当前值的按钮高亮。
type LegacyValueGroup struct {
	Key     string
	Options []LegacyValueOption
	// Exclusive 表示这组选项共用同一个显示位置，只有当前值（和悬停项）
	// 需要绘制。MDDVD 的「画面表示 / 未読スキップ / スキップ速度」把
	// ON 与 OFF 做成两张同尺寸的值图叠在同一个命中块上；不打开时所有
	// 选项都会画出来，文字会互相覆盖。其它作品保持 false 不受影响。
	Exclusive bool
	// ExclusiveToggle 表示 Exclusive 组的点击语义是"在选项之间循环"，
	// 而不是按命中矩形精确选择。两个选项叠在同一位置时，点击只能落到
	// 其中一张图上，必须由组自己决定切到另一个值。
	ExclusiveToggle bool
}

// LegacyValueOption 是取值组中的一个按钮及其代表的设置值。
type LegacyValueOption struct {
	Button UIButton
	Value  int
}

// LegacyEffectPage 是 [detailed effect setting]（詳細演出設定）。
type LegacyEffectPage struct {
	Base      string
	HitTable  string
	CheckMark string
	Back      UIButton
	// Toggles 保留原键名；动作由 session 按键名映射到 GameSettings。
	Toggles []LegacyToggle
}

// LegacyLiquidPage 是 [detailed liquid setting]（詳細汁設定）。
//
// 学園2IE 把“なし/通常”两个选项画在同一张底图上：引擎只负责贴单选标记。
type LegacyLiquidPage struct {
	Base      string
	HitTable  string
	CheckMark string
	// Options 依次是 type0 pos0（なし）、type1 pos1（通常）…
	Options []UIPoint
	Back    UIButton
}

// LegacySoundPage 是 [detailed sound setting]（詳細音声設定）。
type LegacySoundPage struct {
	Base      string
	HitTable  string
	CheckMark string
	VoicePlay UIPoint
	Volumes   []LegacyGauge
	Back      UIButton
}

// LegacyToggle 是一个只有开关状态的控件。
type LegacyToggle struct {
	Key string
	X   int
	Y   int
}

// Empty 表示作品没有旧式设置画面。
func (l LegacyConfigScreen) Empty() bool {
	return len(l.Base) == 0
}

// HasAnyToggle 用于判断画面里是否真的有可交互内容；只有底图没有坐标的
// 配置（汉化补丁裁剪过）应当退回宿主而不是显示一张死图。
func (l LegacyConfigScreen) HasAnyToggle() bool {
	return len(l.Speeds) > 0 || len(l.Volumes) > 0 || len(l.Characters) > 0 ||
		len(l.ValueGroups) > 0 || len(l.ExtraToggles) > 0 ||
		!l.EffectDetail.Empty() || !l.LiquidDetail.Empty() ||
		l.AllEffect.X != 0 || l.AllEffect.Y != 0
}

func parseLegacyConfigScreen(document *IniDocument) LegacyConfigScreen {
	screen := LegacyConfigScreen{
		Base:              document.Get(ConfigScreenSection, "config base", ""),
		HitTable:          document.Get(ConfigScreenSection, "hit table", ""),
		CheckMark:         document.Get(ConfigScreenSection, "check mark", ""),
		RadioButton:       document.Get(ConfigScreenSection, "radio button", ""),
		CheckMarkFrames:   document.GetInt(ConfigScreenSection, "check mark frames", 1),
		RadioFrames:       document.GetInt(ConfigScreenSection, "radio frames", 1),
		EffectDetail:      parseUIButton(document.Get(ConfigScreenSection, "effect detail", "")),
		LiquidDetail:      parseUIButton(document.Get(ConfigScreenSection, "liquid detail", "")),
		Reset:             atlasButtonOr(document, ConfigScreenSection, "reset button", document.Get(ConfigScreenSection, "config base", "")),
		Back:              atlasButtonOr(document, ConfigScreenSection, "back button", document.Get(ConfigScreenSection, "config base", "")),
		Title:             atlasButtonOr(document, ConfigScreenSection, "title button", document.Get(ConfigScreenSection, "config base", "")),
		Exit:              atlasButtonOr(document, ConfigScreenSection, "exit button", document.Get(ConfigScreenSection, "config base", "")),
		CharacterOffFrame: document.GetInt(ConfigScreenSection, "character off frame", -1),
		// 精灵图世代（満淫電車2）的 SAVE/LOAD/CONFIG 页签。
		ModeSave:   atlasButtonOr(document, ConfigScreenSection, "save mode", document.Get(ConfigScreenSection, "config base", "")),
		ModeLoad:   atlasButtonOr(document, ConfigScreenSection, "load mode", document.Get(ConfigScreenSection, "config base", "")),
		ModeConfig: atlasButtonOr(document, ConfigScreenSection, "config mode", document.Get(ConfigScreenSection, "config base", "")),
	}
	// 精灵图世代的勾选/单选图标是 config_base 里的一块 16x16 矩形，
	// 配置只给矩形不给图片名；旧作品仍是独立图片。
	if button, ok := parseAtlasButton(document, ConfigScreenSection, "check mark", screen.Base); ok {
		button.Frames = 1
		screen.CheckMark = button.Image
		screen.CheckMarkAtlas = button
	}
	if button, ok := parseAtlasButton(document, ConfigScreenSection, "radio button", screen.Base); ok {
		button.Frames = 1
		screen.RadioButton = button.Image
		screen.RadioButtonAtlas = button
	}
	screen.ScreenMode = legacyPoint(document, ConfigScreenSection, "screen mode")
	screen.SquareMode = legacyPoint(document, ConfigScreenSection, "square mode")
	screen.AllEffect = legacyPoint(document, ConfigScreenSection, "all effect")
	// 満淫電車2 把「画面効果」写成 effect mode；旧作品用 all effect。
	if screen.AllEffect.X == 0 && screen.AllEffect.Y == 0 {
		screen.AllEffect = legacyPoint(document, ConfigScreenSection, "effect mode")
	}
	screen.CeaseSkip = legacyPoint(document, ConfigScreenSection, "cease skip")
	screen.SkipUnread = legacyPoint(document, ConfigScreenSection, "enable skip unread")
	screen.VoiceLoop = legacyPoint(document, ConfigScreenSection, "voice loop")
	if screen.VoiceLoop.X == 0 && screen.VoiceLoop.Y == 0 {
		// 満淫電車2 的「音声をループ再生させる」键名是 repeat voice。
		screen.VoiceLoop = legacyPoint(document, ConfigScreenSection, "repeat voice")
	}
	screen.VoiceLoopOff = legacyPoint(document, ConfigScreenSection, "voice loop off")
	screen.SkipUnreadOff = legacyPoint(document, ConfigScreenSection, "enable skip unread off")
	screen.AutoSelectMarkFrame = document.GetInt(ConfigScreenSection, "auto select mark frame", 0)
	screen.SkipModeMarkFrame = document.GetInt(ConfigScreenSection, "skip mode mark frame", 0)
	for index := 0; index < 3; index++ {
		if point, ok := parseUIPoint(document.Get(ConfigScreenSection, "skip mode pos"+itoa(index), "")); ok {
			screen.SkipModes = append(screen.SkipModes, point)
		}
	}

	for index := 0; index < 3; index++ {
		key := "auto select pos" + itoa(index)
		if point, ok := parseUIPoint(document.Get(ConfigScreenSection, key, "")); ok {
			screen.AutoSelect = append(screen.AutoSelect, point)
		}
	}
	for index := 0; index < 2; index++ {
		if gauge, ok := parseLegacyGauge(document, ConfigScreenSection, "speed"+itoa(index)); ok {
			screen.Speeds = append(screen.Speeds, gauge)
		}
	}
	// 満淫電車2 把两条速度滑杆写成 `message speed` / `auto speed`，
	// `type = state` 表示值条本身就是一棵填充条。
	for _, key := range []string{"message speed", "auto speed"} {
		if gauge, ok := parseAtlasGauge(document, ConfigScreenSection, key, screen.Base); ok {
			screen.Speeds = append(screen.Speeds, gauge)
		}
	}
	// BGM曲調（ノーマル/ハード）是互斥的两点，键名与 layout 版一致。
	// 只认实际写了的点，没写的作品不会凭空多出两个热区。
	for index := 0; index < 2; index++ {
		if point, ok := parseUIPoint(document.Get(ConfigScreenSection, "bgm selector pos"+itoa(index), "")); ok {
			screen.BgmSelectors = append(screen.BgmSelectors, point)
		}
	}
	for index := 0; index < 4; index++ {
		key := "voice mode pos" + itoa(index)
		if point, ok := parseUIPoint(document.Get(ConfigScreenSection, key, "")); ok {
			screen.VoiceMode = append(screen.VoiceMode, point)
		}
	}
	for index := 0; index < legacyMaxColorChannels; index++ {
		if gauge, ok := parseAtlasGauge(document, ConfigScreenSection, "color"+itoa(index), screen.Base); ok {
			screen.Colors = append(screen.Colors, gauge)
		} else if gauge, ok := parseLegacyGauge(document, ConfigScreenSection, "color"+itoa(index)); ok {
			screen.Colors = append(screen.Colors, gauge)
		}
	}

	channels := document.GetInt(ConfigScreenSection, "volume channels", 7)
	if channels <= 0 || channels > legacyMaxVolumeChannels {
		channels = 7
	}
	for index := 0; index < channels; index++ {
		if gauge, ok := parseAtlasGauge(document, ConfigScreenSection, "volume"+itoa(index), screen.Base); ok {
			screen.Volumes = append(screen.Volumes, gauge)
		} else if gauge, ok := parseLegacyGauge(document, ConfigScreenSection, "volume"+itoa(index)); ok {
			screen.Volumes = append(screen.Volumes, gauge)
		}
	}

	characters := document.GetInt(ConfigScreenSection, "characters", 0)
	if characters < 0 || characters > legacyMaxCharacters {
		characters = 0
	}
	for index := 0; index < characters; index++ {
		character := LegacyCharacter{
			Button: atlasButtonOr(document, ConfigScreenSection, "character"+itoa(index), screen.Base),
			Detail: parseUIButton(document.Get(ConfigScreenSection, "character"+itoa(index)+" detail", "")),
		}
		if character.Button.Empty() && character.Detail.Empty() {
			continue
		}
		screen.Characters = append(screen.Characters, character)
	}

	// 満淫電車2IE 在设置页多了一项「hair mode」；它和「画面効果」一样
	// 只有坐标，勾选状态写进 GameSettings 的同名字段。
	if point, ok := parseUIPoint(document.Get(ConfigScreenSection, "hair mode", "")); ok {
		screen.ExtraToggles = append(screen.ExtraToggles, LegacyToggle{Key: "hair mode", X: point.X, Y: point.Y})
	}
	screen.Sound = parseLegacySoundPage(document)
	screen.Effect = parseLegacyEffectPage(document)
	screen.Liquid = parseLegacyLiquidPage(document)
	return screen
}

func parseLegacySoundPage(document *IniDocument) *LegacySoundPage {
	page := &LegacySoundPage{
		Base:      document.Get(DetailedSoundSection, "base", ""),
		HitTable:  document.Get(DetailedSoundSection, "hit table", ""),
		CheckMark: document.Get(DetailedSoundSection, "check mark", ""),
		VoicePlay: legacyPoint(document, DetailedSoundSection, "voice play"),
		Back:      parseUIButton(document.Get(DetailedSoundSection, "back button", "")),
	}
	for index := 0; index < 2; index++ {
		if gauge, ok := parseLegacyGauge(document, DetailedSoundSection, "volume"+itoa(index)); ok {
			page.Volumes = append(page.Volumes, gauge)
		}
	}
	if len(page.Base) == 0 {
		return nil
	}
	return page
}

func parseLegacyEffectPage(document *IniDocument) *LegacyEffectPage {
	page := &LegacyEffectPage{
		Base:      document.Get(DetailedEffectSection, "base", ""),
		HitTable:  document.Get(DetailedEffectSection, "hit table", ""),
		CheckMark: document.Get(DetailedEffectSection, "check mark", ""),
		Back:      parseUIButton(document.Get(DetailedEffectSection, "back button", "")),
	}
	// 键名就是原引擎的功能名；顺序固定为画面上的阅读顺序，
	// 让绘制/命中共用同一份切片。
	for _, key := range []string{
		"effect mode", "layer action", "forced action0", "unlock notice",
		"eye anime", "cutin", "x count", "hair mode", "steam mode", "ahe mode",
	} {
		point, ok := parseUIPoint(document.Get(DetailedEffectSection, key, ""))
		if !ok {
			continue
		}
		page.Toggles = append(page.Toggles, LegacyToggle{Key: key, X: point.X, Y: point.Y})
	}
	if len(page.Base) == 0 {
		return nil
	}
	return page
}

func parseLegacyLiquidPage(document *IniDocument) *LegacyLiquidPage {
	page := &LegacyLiquidPage{
		Base:      document.Get(DetailedLiquidSection, "base", ""),
		HitTable:  document.Get(DetailedLiquidSection, "hit table", ""),
		CheckMark: document.Get(DetailedLiquidSection, "check mark", ""),
		Back:      parseUIButton(document.Get(DetailedLiquidSection, "back button", "")),
	}
	// type0 pos0 / type0 pos1 是“なし/通常”两个单选点；键名来自原配置。
	for index := 0; index < 8; index++ {
		point, ok := parseUIPoint(document.Get(DetailedLiquidSection, "type0 pos"+itoa(index), ""))
		if !ok {
			break
		}
		page.Options = append(page.Options, point)
	}
	if len(page.Base) == 0 {
		return nil
	}
	return page
}

// parseAtlasGauge 解析精灵图世代的一条填充滑杆。
//
// 配置形如：
//
//	volume0 = 574, 794, 200, 18
//	volume0 pos = 421, 139
//	volume0 min = 574, 884, 42, 46
//	volume0 min pos = 387, 124
//
// Bar/Min/Max 都是底图里的矩形，Val 是 0..100 的填充宽度。
func parseAtlasGauge(document *IniDocument, section, prefix, atlasImage string) (LegacyGauge, bool) {
	bar, ok := parseAtlasButton(document, section, prefix, atlasImage)
	if !ok || bar.Clip[2] <= 0 || bar.Clip[3] <= 0 {
		return LegacyGauge{}, false
	}
	gauge := LegacyGauge{
		Bar:        bar,
		Fill:       true,
		TrackWidth: bar.Clip[2],
	}
	if min, ok := parseAtlasButton(document, section, prefix+" min", atlasImage); ok {
		gauge.Min = min
	}
	if max, ok := parseAtlasButton(document, section, prefix+" max", atlasImage); ok {
		gauge.Max = max
	}
	return gauge, true
}

// legacyPoint 读一个"只有坐标"的开关；允许该键不存在。
func legacyPoint(document *IniDocument, section, key string) UIPoint {
	point, _ := parseUIPoint(document.Get(section, key, ""))
	return point
}

func parseLegacyGauge(document *IniDocument, section, prefix string) (LegacyGauge, bool) {
	gauge := LegacyGauge{
		Bar:         parseUIButton(document.Get(section, prefix+" bar", "")),
		Min:         parseUIButton(document.Get(section, prefix+" min", "")),
		Max:         parseUIButton(document.Get(section, prefix+" max", "")),
		Knob:        parseUIButton(document.Get(section, prefix+" knob", "")),
		Vertical:    document.GetInt(section, prefix+" vertical", 0) != 0,
		TrackHeight: document.GetInt(section, prefix+" track height", 0),
	}
	// 竖向滑杆可以只有 knob（底图上已经画好轨道），没有 bar。
	if gauge.Bar.Empty() && gauge.Min.Empty() && gauge.Max.Empty() && gauge.Knob.Empty() {
		return gauge, false
	}
	return gauge, true
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [4]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}
