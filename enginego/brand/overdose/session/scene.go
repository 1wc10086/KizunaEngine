// 鉴赏场景回放：从 scene_pageNN.tbl 的行记录取出脚本 + label，复用已挂了
// macro hook 的同一颗 VM 跳过去继续演，不要为回放重建 VM（重建会丢舞台
// 的 HandleMacroEvent 绑定）。演完由 finishScript 按 sceneReturnExtra
// 回场景页（overlayExtra + galleryScene）。
package session

import (
	"fmt"
	"strings"

	"kizuna/enginego/brand/overdose/arc"
	"kizuna/enginego/core"
)

// scenePageRecords 返回一页 scene_pageNN.tbl 的行记录（页号从 1 起）。
// 供 overlay_extra 画场景网格 / 判已读时取缩略图名与回放入口；
// 表缺失返回 nil，调用方按空页处理，不要让读表失败中断主流程。
func (s *Session) scenePageRecords(page int) []arc.Record {
	if s.archives == nil {
		return nil
	}
	recs, err := s.archives.Table(fmt.Sprintf("scene_page%02d.tbl", page))
	if err != nil {
		return nil
	}
	return recs
}

// replayScene 从场景页表第 row 条记录起回放：
//   - rec.Cells[1] 是脚本文件（带 .asb），rec.Cells[2] 是 label；
//   - 脚本名去掉 .asb 后缀才是 VM loader 的键名，与 loader 侧 TrimSuffix 同口径。
//
// 失败统一回 StepFaulted：索引越界、列缺失、脚本名为空、StartAt 找错 label
// 都会让玩家看到错的剧情，宁可 fault 回宿主报错，也不要猜一个入口跑。
func (s *Session) replayScene(page, row int) (core.Step, error) {
	if s.archives == nil || s.vm == nil {
		s.lastError = errNotOpen.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, errNotOpen
	}
	recs, err := s.archives.Table(fmt.Sprintf("scene_page%02d.tbl", page))
	if err != nil {
		s.lastError = err.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, err
	}
	if row < 0 || row >= len(recs) {
		err := fmt.Errorf("overdose: 场景页 %d 无第 %d 条记录", page, row)
		s.lastError = err.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, err
	}
	rec := recs[row]
	if len(rec.Cells) < 2 {
		err := fmt.Errorf("overdose: 场景页 %d 第 %d 条记录缺脚本列", page, row)
		s.lastError = err.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, err
	}
	script := strings.TrimSuffix(rec.Cells[1], ".asb")
	label := ""
	if len(rec.Cells) >= 3 {
		label = rec.Cells[2]
	}
	if script == "" {
		err := fmt.Errorf("overdose: 场景页 %d 第 %d 条记录脚本名为空", page, row)
		s.lastError = err.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, err
	}
	s.atTitle = false
	s.menu = nil
	s.overlay = overlayNone
	s.sceneReturnExtra = true
	s.currentScript = script
	s.msgPages = nil
	s.msgFull = ""
	s.msgPage = 0
	if err := s.vm.StartAt(script, label); err != nil {
		s.lastError = err.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, err
	}
	return s.pump(commandBudget)
}
