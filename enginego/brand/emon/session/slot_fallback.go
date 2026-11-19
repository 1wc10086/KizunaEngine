package session

// slot_fallback.go：存档/读档槽位点选的兜底。
//
// 正常流程里点槽位会触发脚本的 1番選択/選択番号倍，再由 ActionWait 后的宏
// 弹确认框。个别设备上点选事件到达的精灵或坐标与桌面版不同（例如点在槽位
// 备注文字、缩略图或缩放后的边缘），宏可能什么都没做，表现就是“点存档没
// 反应/停在读档页”。这里在点击器处理完后检查：如果点的是槽位却没有出现
// 确认框，就直接按槽位编号读档。只在“读档页”生效，存档页仍走原脚本
// （存档需要注释输入，不能静默写入）。

import (
	"strings"

	"kizuna/enginego/core"
)

// slotNumberFromSprite 从 ＳＬ選択１エリア / ＳＬ選択Ａコメント 里取出
// 1..10 的槽位编号；不是槽位精灵时返回 0。
func slotNumberFromSprite(name string) int {
	if !strings.HasPrefix(name, "ＳＬ選択") {
		return 0
	}
	rest := strings.TrimPrefix(name, "ＳＬ選択")
	runes := []rune(rest)
	if len(runes) == 0 {
		return 0
	}
	switch {
	case runes[0] >= '１' && runes[0] <= '９':
		return int(runes[0]-'０') // 全角数字
	case runes[0] >= '1' && runes[0] <= '9':
		return int(runes[0] - '0')
	case runes[0] == 'Ａ' || runes[0] == 'A':
		return 10
	}
	return 0
}

func (s *Session) isLoadPageVisible() bool {
	if s.stage == nil {
		return false
	}
	button := s.stage.Find("ロード画面ボタン")
	return button != nil && button.Visible() && isChainVisible(button)
}

// loadSlotDirect 在读档页把槽位点击直接落到 Load，不经过脚本的确认框。
// 真机日志显示脚本的 GetYesNoMessage 会被宿主在几十毫秒内以“否/空”结束
//（随后跳回读档页），确认框实际上无法使用；这里直接读档保证功能可用。
func (s *Session) loadSlotDirect(spriteName string) bool {
	if s.machine == nil || s.stage == nil || spriteName == "" {
		return false
	}
	if !s.isLoadPageVisible() {
		return false
	}
	slot := slotNumberFromSprite(spriteName)
	if slot == 0 {
		return false
	}
	page := s.machine.SettingInt("SaveLoadPage", 0)
	if page < 0 || page > 9 {
		page = 0
	}
	target := page*10 + slot
	if !s.machine.LoadSlot(target) {
		core.Logf("emon: direct load slot %d was empty", target)
		return false
	}

	s.step = s.machine.Step()
	s.invalidateFrame()
	return true
}

// tryLoadSlotFallback 在点选没有产生确认框时直接读取槽位。
func (s *Session) tryLoadSlotFallback(spriteName string) bool {
	if s.machine == nil || s.stage == nil || spriteName == "" {
		return false
	}
	if !s.isLoadPageVisible() {
		return false
	}
	slot := slotNumberFromSprite(spriteName)
	if slot == 0 {
		return false
	}
	page := 0
	if s.machine != nil {
		page = s.machine.SettingInt("SaveLoadPage", 0)
	}
	if page < 0 || page > 9 {
		page = 0
	}
	target := page*10 + slot
	if !s.machine.LoadSlot(target) {
		core.Logf("emon: load fallback slot %d was empty", target)
		return false
	}
	s.step = s.machine.Step()
	s.invalidateFrame()
	return true
}
