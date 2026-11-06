package config

import (
	"strconv"
	"strings"
)

// status_legacy.go 解析旧式 [status screen] 的 ITEM 画面（教育指導 KS/KSIE）。
//
// 这一代没有 define.bsa 的 status_layout：底图 status_base 已经把五名角色、
// 每人三组道具的空槽和箭头烘焙在整屏里，配置只用 item111…item533 给出
// 每张道具图的位置。脚本把道具状态写在 #kiyoka_item1 这类属性里：
//   0 = 未取得，1 = 取得第一件，2 = 取得第二件，3 = 完成（itemcheck）。
// 引擎在状态画面打开时按属性值把对应的 itemXYZ 叠到空槽上。
//
// 只解析 item 开头的三位键与返回/标题/退出按钮；其它键（save/load/config
// mode 等）这一屏不用，保留在 document 里不影响其它界面。

// LegacyStatusItem 是一张道具图。
type LegacyStatusItem struct {
	Image string
	// Character 是 1 基角色号（itemXYZ 的 X）。
	Character int
	// Item 是 1 基道具号（Y）。
	Item int
	// Part 是 1 基部件号（Z）：1/2 是小图标，3 是大图。
	Part   int
	Button UIButton
}

// LegacyStatusScreen 是旧式 [status screen] 的 ITEM 页。
type LegacyStatusScreen struct {
	Base  string
	Items []LegacyStatusItem
	Back  UIButton
	Title UIButton
	Exit  UIButton
}

func (s LegacyStatusScreen) Empty() bool {
	return len(s.Base) == 0 || len(s.Items) == 0
}

func parseLegacyStatusScreen(document *IniDocument) LegacyStatusScreen {
	screen := LegacyStatusScreen{
		Base:  document.Get(StatusScreenSection, "base", ""),
		Back:  parseUIButton(document.Get(StatusScreenSection, "back button", "")),
		Title: parseUIButton(document.Get(StatusScreenSection, "title button", "")),
		Exit:  parseUIButton(document.Get(StatusScreenSection, "exit button", "")),
	}
	for _, entry := range document.Section(StatusScreenSection) {
		key := strings.ToLower(strings.TrimSpace(entry.Key))
		if len(key) != 7 || !strings.HasPrefix(key, "item") {
			continue
		}
		character, err1 := strconv.Atoi(key[4:5])
		item, err2 := strconv.Atoi(key[5:6])
		part, err3 := strconv.Atoi(key[6:7])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		if character < 1 || item < 1 || part < 1 {
			continue
		}
		button := parseUIButton(entry.Value)
		if button.Empty() {
			continue
		}
		screen.Items = append(screen.Items, LegacyStatusItem{
			Image: button.Image, Character: character, Item: item, Part: part, Button: button,
		})
	}
	return screen
}
