package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/core"
)

// game_settings.go 保存"原引擎设置画面"里的可调项。
//
// 这些值以前散落在 Flutter 的 AppSettings 里（主音量/BGM/语音），但原引擎的设置
// 画面还包含主音量、SE、BGS、BGV、文字速度、窗口底色、演出开关等一整套选项，
// 而且音量必须直接乘进引擎发出的 AudioEvent，Flutter 侧不该再有一份独立音量。
// 因此设置的所有权和持久化都放在引擎侧，宿主只负责把画面画出来。
//
// 文件放在游戏自己的 save 目录下，和存档一起卸载；不同作品互不影响。

const gameSettingsFileName = "kizuna-settings.json"

// GameSettings 是原版设置画面的可调项。
type GameSettings struct {
	// Version 用来识别旧格式：没有版本号的设置文件不覆盖默认值，
	// 避免缺字段时把音量和按角色开关读成 0。
	Version        int     `json:"version"`
	MasterVolume   float64 `json:"masterVolume"`
	BgmVolume      float64 `json:"bgmVolume"`
	SeVolume       float64 `json:"seVolume"`
	BgsVolume      float64 `json:"bgsVolume"`
	VoiceVolume    float64 `json:"voiceVolume"`
	BgvVolume      float64 `json:"bgvVolume"`
	BgmAttenuation float64 `json:"bgmAttenuation"`

	MessageSpeed int `json:"messageSpeed"` // 0..100
	ClickWait    int `json:"clickWait"`    // 0..100

	BgRed   int `json:"bgRed"`   // 0..255
	BgGreen int `json:"bgGreen"` // 0..255
	BgBlue  int `json:"bgBlue"`  // 0..255
	BgAlpha int `json:"bgAlpha"` // 0..255

	ScreenMode string `json:"screenMode"` // window / full
	// SquareMode 是旧式设置里的"スクエアモード"（4:3 显示）。
	SquareMode bool `json:"squareMode"`
	// AutoSelect 是 0 手动 / 1 中出し / 2 外出し；只影响原作脚本读的选项。
	AutoSelect int `json:"autoSelect"`
	// AutoSelectPosition 是旧式设置里"射精位置"的 10 个单选（AFDVD）。
	// 与 AutoSelect 的三态不是一回事：前者是具体位置，后者是内/外射。
	AutoSelectPosition int `json:"autoSelectPosition"`
	// EffectMode 是画面効果：0 なし / 1 コマ数固定 / 2 時間固定。
	EffectMode int `json:"effectMode"`
	// MeasureMode 是計量設定：0 なし / 1 通常 / 2 絶倫。
	MeasureMode int `json:"measureMode"`
	BgmSelect   int `json:"bgmSelect"` // 0 normal / 1 jazz
	VoiceMode   int `json:"voiceMode"`
	// VoiceLoop 是旧式设置顶部的「ボイスループ再生」。
	VoiceLoop bool `json:"voiceLoop"`
	// 多汁設定（学園/GK）：0 なし / 1 少ない / 2 普通 / 3 多い。
	// 引擎当前只负责保存与显示，剧本用同名属性读取原版设置。
	MaleLiquid   int `json:"maleLiquid"`
	FemaleLiquid int `json:"femaleLiquid"`
	MilkLiquid   int `json:"milkLiquid"`
	// SkipMode 是跳过处理：0 通常 / 1 高速 / 2 次の選択肢まで。
	SkipMode   int  `json:"skipMode"`
	CeaseSkip  bool `json:"ceaseSkip"`
	SkipUnread bool `json:"skipUnread"`
	UnderHair  bool `json:"underHair"`
	AllEffect  bool `json:"allEffect"`
	Wipe       bool `json:"wipe"`
	Cutin      bool `json:"cutin"`
	// AntiAlias 是旧式设置画面的「アンチエイリアス」开关。
	// 引擎不直接做抗锯齿，但设置画面的勾选状态要能保存/还原。
	AntiAlias  bool `json:"antiAlias"`
	Hold       bool `json:"hold"`
	Blink      bool `json:"blink"`
	XCountdown bool `json:"xCountdown"`
	Steam      bool `json:"steam"`
	// HairMode 是 満淫電車2IE 设置页的「hair mode」：原版用它决定
	// 立绘是否叠头发图层（GraphicsEV_hair.bsa）。默认开，首次启动与原版一致。
	HairMode bool `json:"hairMode"`
	// AheOff 是「アヘ顔OFF」：开时 CG 鉴赏/立ち絵/剧情里的强烈アヘ顔图层
	// 改用 `<name>_noahe` 素材，A-H-E 入场动画也不再播放。
	// 默认关闭（显示アヘ顔），与原版首次启动一致。
	AheOff bool `json:"aheOff"`
	// 旧式"詳細演出設定"里没有独立 GameSettings 字段的开关。
	// 保留它们是为了让设置画面能显示原版勾选状态，并让剧本侧读取一致。
	LayerAction  bool `json:"layerAction"`
	ForcedAction bool `json:"forcedAction"`
	UnlockNotice bool `json:"unlockNotice"`

	// CharacterVoice / CharacterBgv 是"個別音声設定"里每个角色自己的音量。
	// 它们存的是 0..1 的系数，不是 dB：引擎在发出 AudioEvent 时把它乘进事件音量，
	// 宿主只按事件音量播放，所以这里必须由引擎保管。
	CharacterVoice   [8]float64 `json:"characterVoice"`
	CharacterBgv     [8]float64 `json:"characterBgv"`
	CharacterVoiceOn [8]bool    `json:"characterVoiceOn"`
	// SelectedActor 是设置画面里当前选中的角色（0 起）。
	// 记住它是为了让玩家下次打开设置时，"个别音量"那几行直接是他上次在调的角色。
	SelectedActor int `json:"selectedActor"`
	// MessageWindowPosition 是学園2 对话框的上/中/下三档位置：
	// 0 上、1 中、2 下（默认）。原版用对话框右上角 ▲▼ 切换。
	MessageWindowPosition int `json:"messageWindowPosition"`
	// DressNude 是学園2 老版 DRESS 按钮的着衣状态：false 着衣 / true 全裸。
	// 原引擎把状态拼到 clNNNNN 素材名后面（clNNNNN0 / clNNNNN1），
	// 所以它必须随设置持久化，读档/重启后角色才不会突然穿上衣服。
	DressNude bool `json:"dressNude"`
}

func defaultGameSettings(cfg *config.BsxxConfiguration) GameSettings {
	settings := GameSettings{
		// Version 2 增加旧式设置画面的スクエアモード/自動選択/詳細演出开关；
		// Version 3 把按角色音量从 6 人扩到 8 人，并增加 AFDVD 的画面効果/
		// 計量設定/射精位置。旧文件缺这些键时必须保留默认值。
		Version:               4,
		MasterVolume:          1,
		BgmVolume:             0.5,
		SeVolume:              0.75,
		BgsVolume:             0.75,
		VoiceVolume:           1,
		BgvVolume:             0.75,
		BgmAttenuation:        0.5,
		MessageSpeed:          50,
		ClickWait:             50,
		MessageWindowPosition: 2,
		BgRed:                 0,
		BgGreen:               0,
		BgBlue:                0,
		BgAlpha:               128,
		ScreenMode:            "full",
		SquareMode:            false,
		MaleLiquid:            2,
		FemaleLiquid:          2,
		MilkLiquid:            2,
		AutoSelect:            0,
		BgmSelect:             0,
		VoiceMode:             0,
		UnderHair:             true,
		AllEffect:             true,
		EffectMode:            1,
		MeasureMode:           1,
		Wipe:                  true,
		Cutin:                 true,
		AntiAlias:             true,
		Blink:                 true,
		XCountdown:            true,
		Steam:                 true,
		HairMode:              true,
		AheOff:                false,
		LayerAction:           true,
		ForcedAction:          true,
		UnlockNotice:          true,
	}
	if cfg != nil {
		settings.BgmVolume = cfg.BgmVolume
		settings.SeVolume = cfg.EffectVolume
		settings.VoiceVolume = cfg.VoiceVolume
	}
	for index := range settings.CharacterVoice {
		settings.CharacterVoice[index] = 1
		settings.CharacterBgv[index] = 1
		settings.CharacterVoiceOn[index] = true
	}
	// 原引擎允许作品覆盖默认音量；[default volume] 里还有 bgs/bgv 与衰减，
	// 这里按同一份配置读，避免换作品后默认值与原作不一致。
	if cfg != nil {
		if value := cfg.Raw(config.VolumeSection, "bgs", ""); value != "" {
			settings.BgsVolume = clampVolumeSetting(parseFloat(value, settings.BgsVolume))
		}
		if value := cfg.Raw(config.VolumeSection, "bgv", ""); value != "" {
			settings.BgvVolume = clampVolumeSetting(parseFloat(value, settings.BgvVolume))
		}
		if value := cfg.Raw(config.VolumeSection, "attenuated bgm", ""); value != "" {
			settings.BgmAttenuation = clampVolumeSetting(parseFloat(value, settings.BgmAttenuation))
		}
		rawColor := cfg.Raw(config.MessageWindowSection, "bg color", "")
		if rawColor != "" {
			// 配置里写的是 0xAARRGGBB。把它拆进四个分量，
			// 设置画面的滑杆才有正确的初始位置；解析失败保持默认半透明黑。
			if parsed, ok := parseARGB(rawColor); ok {
				settings.BgAlpha = int(parsed >> 24)
				settings.BgRed = int(parsed>>16) & 0xff
				settings.BgGreen = int(parsed>>8) & 0xff
				settings.BgBlue = int(parsed) & 0xff
			}
		} else {
			// 没有 bsxx.cfg 的内置作品（放課後2）把原版窗口色写在 BgColor 里；
			// 不取它的话滑杆会显示通用的 0x80000000，和实际窗口对不上。
			// 其它作品 BgColor 的默认值本来就是 0x80000000，行为不变。
			settings.BgAlpha = int(cfg.Message.BgColor >> 24)
			settings.BgRed = int(cfg.Message.BgColor>>16) & 0xff
			settings.BgGreen = int(cfg.Message.BgColor>>8) & 0xff
			settings.BgBlue = int(cfg.Message.BgColor) & 0xff
		}
	}
	return settings
}

// parseARGB 解析 bsxx.cfg 里 0xAARRGGBB 形式的颜色。
func parseARGB(value string) (uint32, bool) {
	text := strings.TrimSpace(value)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
	if text == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(parsed), true
}

// applyDisplaySettings 把设置里影响 HUD/素材解析的项推给渲染器。
//
// 除了射精计数器和对话框底色，还同步アヘ顔OFF：渲染层按后缀换素材，
// 剧情、CG 鉴赏、立ち絵鉴赏共用同一份结果。
func (s *Session) applyDisplaySettings() {
	if s.composer == nil {
		return
	}
	// AHE 后缀与开关由渲染层统一处理：素材加载、剧情图层、CG 鉴赏都走
	// 同一个 ImageCache，避免三处各写一份替换规则而漏掉其中一处。
	suffix := ""
	if s.config != nil {
		s.config.AheOff = s.settings.AheOff
		suffix = s.config.AheSuffix
	}
	s.composer.Images.SetAheSuffix(suffix)
	s.composer.Images.SetAheOff(s.settings.AheOff)
	s.composer.SetCounterEnabled(s.settings.XCountdown)
	s.composer.SetMessageSpeed(s.settings.MessageSpeed)
	s.composer.SetMessageWindowOffset(s.messageWindowOffset())
	s.composer.SetMessageBackground(
		uint32(clampInt(s.settings.BgAlpha, 0, 255))<<24 |
			uint32(clampInt(s.settings.BgRed, 0, 255))<<16 |
			uint32(clampInt(s.settings.BgGreen, 0, 255))<<8 |
			uint32(clampInt(s.settings.BgBlue, 0, 255)))
}

// messageWindowOffset 把三档位置换算成对话框的垂直位移。
//
// 学園2 的对话框底图在 (0,460)；上/中两档不是简单地把整块贴到 0/230，
// 顶部 24px 要留给常驻工具条，所以上档停在 y=24。其它作品没有 ▲▼ 按钮，
// 位置保持默认 2，位移为 0。
func (s *Session) messageWindowOffset() int {
	if s.config == nil || !s.config.Museum.Gakuen2 {
		return 0
	}
	baseY := s.config.Message.BaseY
	position := clampInt(s.settings.MessageWindowPosition, 0, 2)
	switch position {
	case 0:
		return 24 - baseY
	case 1:
		return 230 - baseY
	default:
		return 0
	}
}

func parseFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func clampVolumeSetting(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampSetting(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func (s *Session) gameSettingsPath() string {
	if s.dataDir == "" {
		return filepath.Join(s.cacheDir, gameSettingsFileName)
	}
	return filepath.Join(dataSaveDir(s.dataDir), gameSettingsFileName)
}

// loadGameSettings 先按配置生成默认值，再用磁盘上的用户设置覆盖。
// 文件损坏/不存在都退回默认值：设置文件不该阻止游戏启动。
func (s *Session) loadGameSettings() {
	s.settings = defaultGameSettings(s.config)
	if s.config == nil {
		return
	}
	data, err := os.ReadFile(s.gameSettingsPath())
	if err != nil {
		return
	}
	var stored GameSettings
	if err := json.Unmarshal(data, &stored); err != nil {
		core.Logf("settings: ignore damaged '%s': %v", s.gameSettingsPath(), err)
		return
	}
	if stored.Version < 1 {
		core.Logf("settings: ignore settings without a version")
		return
	}
	s.settings.MasterVolume = clampVolumeSetting(stored.MasterVolume)
	s.settings.BgmVolume = clampVolumeSetting(stored.BgmVolume)
	s.settings.SeVolume = clampVolumeSetting(stored.SeVolume)
	s.settings.BgsVolume = clampVolumeSetting(stored.BgsVolume)
	s.settings.VoiceVolume = clampVolumeSetting(stored.VoiceVolume)
	s.settings.BgvVolume = clampVolumeSetting(stored.BgvVolume)
	s.settings.BgmAttenuation = clampVolumeSetting(stored.BgmAttenuation)
	s.settings.MessageSpeed = clampSetting(stored.MessageSpeed, 0, 100)
	s.settings.ClickWait = clampSetting(stored.ClickWait, 0, 100)
	s.settings.BgRed = clampSetting(stored.BgRed, 0, 255)
	s.settings.BgGreen = clampSetting(stored.BgGreen, 0, 255)
	s.settings.BgBlue = clampSetting(stored.BgBlue, 0, 255)
	s.settings.BgAlpha = clampSetting(stored.BgAlpha, 0, 255)
	if stored.ScreenMode == "window" || stored.ScreenMode == "full" {
		s.settings.ScreenMode = stored.ScreenMode
	}
	s.settings.BgmSelect = clampSetting(stored.BgmSelect, 0, 1)
	s.settings.VoiceMode = clampSetting(stored.VoiceMode, 0, 3)
	s.settings.VoiceLoop = stored.VoiceLoop
	s.settings.SkipMode = clampSetting(stored.SkipMode, 0, 2)
	s.settings.CeaseSkip = stored.CeaseSkip
	s.settings.SkipUnread = stored.SkipUnread
	s.settings.UnderHair = stored.UnderHair
	s.settings.HairMode = stored.HairMode
	s.settings.AllEffect = stored.AllEffect
	s.settings.Wipe = stored.Wipe
	s.settings.Cutin = stored.Cutin
	// Version 4 才写入 antiAlias；旧文件的缺省 false 不能覆盖默认的 true。
	if stored.Version >= 4 {
		s.settings.AntiAlias = stored.AntiAlias
	}
	s.settings.Hold = stored.Hold
	s.settings.Blink = stored.Blink
	s.settings.XCountdown = stored.XCountdown
	s.settings.Steam = stored.Steam
	s.settings.AheOff = stored.AheOff
	if stored.Version >= 2 {
		s.settings.SquareMode = stored.SquareMode
		s.settings.AutoSelect = clampSetting(stored.AutoSelect, 0, 2)
		s.settings.LayerAction = stored.LayerAction
		s.settings.ForcedAction = stored.ForcedAction
		s.settings.UnlockNotice = stored.UnlockNotice
	}
	// Version 1 的按角色音量是可选的（缺键时 JSON 反序列化得到 0），
	// Version 2 起写入完整数组，Version 3 扩到 8 个角色。
	characterLimit := len(stored.CharacterVoice)
	if stored.Version < 3 && characterLimit > 6 {
		characterLimit = 6
	}
	for index := 0; index < characterLimit; index++ {
		if stored.Version >= 2 {
			// 0 是合法的"静音"，不能再按 >0 判断，否则静音角色重开后回到 100%。
			s.settings.CharacterVoice[index] = clampVolumeSetting(stored.CharacterVoice[index])
			s.settings.CharacterBgv[index] = clampVolumeSetting(stored.CharacterBgv[index])
			s.settings.CharacterVoiceOn[index] = stored.CharacterVoiceOn[index]
		} else if stored.CharacterVoice[index] > 0 {
			s.settings.CharacterVoice[index] = clampVolumeSetting(stored.CharacterVoice[index])
			if stored.CharacterBgv[index] > 0 {
				s.settings.CharacterBgv[index] = clampVolumeSetting(stored.CharacterBgv[index])
			}
		}
	}
	if stored.Version >= 3 {
		// AFDVD 旧式设置新增的三项。旧文件没有这些键，保持默认值。
		s.settings.AutoSelectPosition = clampSetting(stored.AutoSelectPosition, 0, 9)
		s.settings.EffectMode = clampSetting(stored.EffectMode, 0, 2)
		s.settings.MeasureMode = clampSetting(stored.MeasureMode, 0, 2)
	}
	s.settings.SelectedActor = clampSetting(stored.SelectedActor, 0, len(s.settings.CharacterVoice)-1)
}

// saveGameSettings 原子落盘：先写临时文件再 rename，避免进程被杀时留下半个 JSON。
func (s *Session) saveGameSettings() {
	path := s.gameSettingsPath()
	if !core.EnsureDir(filepath.Dir(path)) {
		core.Logf("settings: create directory failed: %s", filepath.Dir(path))
		return
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		core.Logf("settings: encode failed: %v", err)
		return
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		core.Logf("settings: write failed: %v", err)
		return
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		core.Logf("settings: commit failed: %v", err)
	}
}

// resetGameSettings 对应设置画面的"初期化"按钮。
func (s *Session) resetGameSettings() {
	s.settings = defaultGameSettings(s.config)
	s.saveGameSettings()
	s.refreshAudioVolumes()
}
