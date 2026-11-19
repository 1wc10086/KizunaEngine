package config

import (
	"strconv"
	"strings"
)

// status2.go 解析 [status2 screen]——Bishop 的「ケダモノカルテ」（角色病历卡）画面。
//
// 这是 OH / OHIE 独有的界面：Karte 工具按钮打开一页四名女主角的选择卡，
// 点进某个角色后显示她的最新病历（等级 1..10 的心情留言、医生诊断和小人图）。
// 界面数据全部来自 bsxx.cfg：
//
//	base      = karte_select_base       ; 选择页整屏底图
//	target0.. = karte_select_0, 3, ...  ; 四张角色卡（3 帧：无数据/常态/悬停）
//	back button = karte_back, 3, ...    ; 返回
//	register  = 10, 11, 12, 13          ; 各角色等级所在的系统参数键
//	ch0 base / ch0 lvN message / ...    ; 详情页素材
//
// register 里放的不是脚本属性下标，而是 BSXScript 头部的系统参数键：
// 键 10 在 OH/OHIE 的脚本里映射到 #miya_karte，11..13 依次是其余三人。
// 解析由 vm.BsxScriptFile.ResolveSlot 完成，引擎只负责按键取值。
const Status2ScreenSection = "status2 screen"

// KarteMaxLevel 是 [status2 screen] 配置里一代作品最多写到的等级。
// 解析时按实际出现的 lvN 扩展切片，绘制时再按这个上限兜底。
const KarteMaxLevel = 10

// Status2Screen 是 [status2 screen] 的完整布局。
type Status2Screen struct {
	Base    string
	Targets []UIButton
	Back    UIButton
	// Register 是每名角色的等级来源（系统参数键），顺序与 Targets 一致。
	Register   []int
	Characters []Status2Character
}

// Status2Character 是单个角色的病历页素材。
type Status2Character struct {
	Base string
	// MessagePos / DiagnosisPos / SDPos 是三类覆盖图的左上角。
	MessagePos   UIPoint
	DiagnosisPos UIPoint
	SDPos        UIPoint
	// 下标 = 等级-1。配置里最多 lv10；没人更新的角色等级为 0，此时
	// 选择页画第 0 帧（黑色空卡），详情页只画底图。
	Messages        []string
	MessageVoices   []string
	Diagnoses       []string
	DiagnosisVoices []string
	SDPictures      []string
	// VoiceButton / DiagnosisButton 是两段文本右侧的「Voice」按钮。
	VoiceButton     UIButton
	DiagnosisButton UIButton
}

// HasLayout 判断这一代作品是否真的配置了 Karte 画面。
func (s Status2Screen) HasLayout() bool {
	return len(s.Base) > 0 && len(s.Targets) > 0 && len(s.Characters) > 0
}

func parseStatus2Screen(document *IniDocument) Status2Screen {
	screen := Status2Screen{
		Base: document.Get(Status2ScreenSection, "base", ""),
		Back: parseUIButton(document.Get(Status2ScreenSection, "back button", "")),
	}
	for _, part := range strings.Split(document.Get(Status2ScreenSection, "register", ""), ",") {
		if value, ok := parseInt(strings.TrimSpace(part)); ok {
			screen.Register = append(screen.Register, value)
		}
	}
	// targetN 不一定连续：老配置可能缺一张卡。逐个探测比"遇到空就停"稳。
	for index := 0; index < 16; index++ {
		button := parseUIButton(document.Get(Status2ScreenSection,
			"target"+strconv.Itoa(index), ""))
		if button.Empty() {
			continue
		}
		screen.Targets = append(screen.Targets, button)
	}
	screen.Characters = parseStatus2Characters(document)
	return screen
}

// parseStatus2Characters 扫描整个段，按 `chN ...` 键建角色表。
//
// 键的形状固定，所以按空格拆词即可，不需要正则：
//
//	ch0 base / ch0 message pos / ch0 lv1 message / ch0 lv1 message voice
//	ch0 voice button / ch0 dr voice button
//
// 等级键可能超过 10（补丁把 lv10 写成 lv0 的情况见文件里的注释），
// 所以切片按实际出现的最大等级扩容，绘制时再 clamp。
func parseStatus2Characters(document *IniDocument) []Status2Character {
	entries := document.Section(Status2ScreenSection)
	maxIndex := -1
	for _, entry := range entries {
		character, ok := status2CharacterIndex(entry.Key)
		if !ok {
			continue
		}
		if character > maxIndex {
			maxIndex = character
		}
	}
	if maxIndex < 0 {
		return nil
	}
	characters := make([]Status2Character, maxIndex+1)
	for _, entry := range entries {
		character, ok := status2CharacterIndex(entry.Key)
		if !ok {
			continue
		}
		fields := strings.Fields(strings.ToLower(entry.Key))
		if len(fields) < 2 {
			continue
		}
		target := &characters[character]
		switch {
		case len(fields) == 2 && fields[1] == "base":
			target.Base = strings.TrimSpace(entry.Value)
		case len(fields) == 3 && fields[1] == "message" && fields[2] == "pos":
			target.MessagePos, _ = parseUIPoint(entry.Value)
		case len(fields) == 3 && fields[1] == "diagnosis" && fields[2] == "pos":
			target.DiagnosisPos, _ = parseUIPoint(entry.Value)
		case len(fields) == 3 && fields[1] == "sd" && fields[2] == "pos":
			target.SDPos, _ = parseUIPoint(entry.Value)
		case len(fields) == 3 && fields[1] == "voice" && fields[2] == "button":
			target.VoiceButton = parseUIButton(entry.Value)
		case len(fields) == 4 && fields[1] == "dr" && fields[2] == "voice" && fields[3] == "button":
			target.DiagnosisButton = parseUIButton(entry.Value)
		default:
			level, ok := status2Level(fields[1])
			if !ok {
				continue
			}
			index := level - 1
			value := strings.TrimSpace(entry.Value)
			switch {
			case len(fields) == 3 && fields[2] == "message":
				target.Messages = growString(target.Messages, index, value)
			case len(fields) == 4 && fields[2] == "message" && fields[3] == "voice":
				target.MessageVoices = growString(target.MessageVoices, index, value)
			case len(fields) == 3 && fields[2] == "diagnosis":
				target.Diagnoses = growString(target.Diagnoses, index, value)
			case len(fields) == 4 && fields[2] == "diagnosis" && fields[3] == "voice":
				target.DiagnosisVoices = growString(target.DiagnosisVoices, index, value)
			case len(fields) == 3 && fields[2] == "sd":
				target.SDPictures = growString(target.SDPictures, index, value)
			}
		}
	}
	return characters
}

func growString(values []string, index int, value string) []string {
	if index < 0 {
		return values
	}
	for len(values) <= index {
		values = append(values, "")
	}
	values[index] = value
	return values
}

// status2CharacterIndex 从 `chN ...` 键里取出 N。
func status2CharacterIndex(key string) (int, bool) {
	fields := strings.Fields(strings.ToLower(key))
	if len(fields) == 0 || len(fields[0]) < 3 || !strings.HasPrefix(fields[0], "ch") {
		return 0, false
	}
	value, err := strconv.Atoi(fields[0][2:])
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// status2Level 从 `lvN` 取出 N（1 基）。
func status2Level(token string) (int, bool) {
	if len(token) < 3 || !strings.HasPrefix(token, "lv") {
		return 0, false
	}
	value, err := strconv.Atoi(token[2:])
	if err != nil || value < 1 {
		return 0, false
	}
	return value, true
}
