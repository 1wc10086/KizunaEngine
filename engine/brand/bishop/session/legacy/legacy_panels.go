package legacy

import (
	"fmt"

	"kizuna/engine/brand/bishop/vm/scf"
)

func buildCommandPanel(options []scf.ScfChoice) LegacyPanel {
	items := make([]string, 0, len(options))
	for index, option := range options {
		items = append(items, fmt.Sprintf("%2d : %s", index+1, CleanText(option.Text)))
	}
	title := "第二コマンド選択"
	if isRootOptions(options) {
		title = "第一コマンド選択"
	}
	return LegacyPanel{Kind: LegacyPanelCommand, Title: title, Items: items}
}

// isRootOptions 判断是否为第一层命令：第一层的首项 Entry 必为 0。
func isRootOptions(options []scf.ScfChoice) bool {
	for _, option := range options {
		if option.Entry == 0 {
			return true
		}
	}
	return false
}

func choiceTexts(options []scf.ScfChoice) []string {
	texts := make([]string, 0, len(options))
	for _, option := range options {
		texts = append(texts, CleanText(option.Text))
	}
	return texts
}

func (e *Engine) slotPanel(kind LegacyPanelKind) LegacyPanel {
	infos := e.legacySaves.List()
	items := make([]string, 0, len(infos))
	for _, info := range infos {
		summary := "---"
		if info.Exists {
			summary = info.Summary
		}
		// slotPanel 只提供摘要；编号由 drawSaveSlots 统一加，避免出现
		// “ 1 :  1 : ...” 的双重编号。
		items = append(items, summary)
	}
	return LegacyPanel{Kind: kind, Title: "データの選択", Items: items}
}

func (e *Engine) systemPanel() LegacyPanel {
	return LegacyPanel{
		Kind:        LegacyPanelSystem,
		Title:       "SYSTEM 設定",
		InstantText: e.config.InstantText,
		BgmEnabled:  e.config.Bgm,
	}
}
