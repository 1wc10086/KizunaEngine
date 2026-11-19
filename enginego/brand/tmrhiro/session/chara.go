// 本文件是 CHARACTER VIEWER（立绘鉴赏器）：原版 [CHARAVIEW] + [CHARA01..06] + [BG]。
//
// 原版布局（_charaview，全部是 1280x720 游戏坐标）：
//
//	右侧 x=1008 一列四行，每行 156x30、行距 44：
//	  第 1 行 角色（CharaStat  y=40）    ◀ [マリア]         ▶
//	  第 2 行 服装（CharaCostume y=84）   ◀ [裸]             ▶
//	  第 3 行 表情（CharaExpression y=128）◀ [微笑]           ▶
//	  第 4 行 背景（BGStat       y=172）  ◀ [コックピット_通常] ▶
//	其下是 上下左右 + 拡大/縮小 + デフォルト + 隠す + 戻る：
//	  ▲(1008,352) ▼(1008,438) ◀(1008,395) ▶(1088,395)
//	  拡大(1008,481) 縮小(1088,481) デフォルト(1008,524) 隠す(1008,567) 戻る(1008,649)
//
// 为什么行是"当前值 + 左右箭头"而不是列表：原版每一行只显示一个当前项，
// 点 ◀/▶ 在该类的候选项之间循环。以前的实现把 6 个角色、30 个表情整列铺出来，
// 行高 30、步进 30 直接叠在一起，命中与绘制全错位（玩家看到按钮重叠）。
package session

import (
	"fmt"

	"kizuna/enginego/brand/tmrhiro/config"
)

// charaView 是立绘鉴赏器的运行态。
type charaView struct {
	// 四行的当前选择。
	character int
	costume   int
	expr      int
	bg        int
	// zoom 是相对"整身铺满屏幕高度"的倍率；offset 是玩家用上下左右挪的像素。
	zoom     float64
	offsetX  int
	offsetY  int
	hidden   bool
	dragging bool
	dragX    int
	dragY    int
}

// charaViewActive 报告是否停在立绘鉴赏器。
func (s *Session) charaViewActive() bool {
	if s.vm == nil || !s.vm.CharaViewActive() {
		return false
	}
	// 没有 _charaview 的作品不会有这个界面；此时退回脚本界面，
	// 避免玩家停在一条没有返回按钮的黑屏上。
	return s.ui != nil && s.ui.Chara.ListW > 0
}

// charaEntry 返回当前选中角色对应的配置段。
//
// [CHARAVIEW] 的显示名在 CHARA01..06 里是 1:1 对应的（本作 6 个名字 = 6 段），
// 只有多于段数时才按比例折算（兼容把普通/催眠写成 12 个名字的其它作品）。
func (s *Session) charaEntry() (config.CharaEntry, bool) {
	view := s.ui.Chara
	if len(view.Entries) == 0 {
		return config.CharaEntry{}, false
	}
	index := s.chara.character
	if index < 0 {
		index = 0
	}
	if index >= len(view.Characters) {
		index = len(view.Characters) - 1
	}
	if index < 0 {
		index = 0
	}
	per := 1
	if len(view.Characters) > len(view.Entries) {
		per = len(view.Characters) / len(view.Entries)
	}
	entryIndex := index / per
	if entryIndex >= len(view.Entries) {
		entryIndex = len(view.Entries) - 1
	}
	return view.Entries[entryIndex], true
}

// charaCostumes 返回当前角色页的服装显示名。
func (s *Session) charaCostumes() []string {
	if entry, ok := s.charaEntry(); ok {
		return entry.Costumes
	}
	return nil
}

// charaExpressions 返回当前角色页的表情显示名。
func (s *Session) charaExpressions() []string {
	if entry, ok := s.charaEntry(); ok {
		return entry.Expressions
	}
	return nil
}

// charaMacro 返回当前选中组合对应的宏名。
//
// _charafilemacro 的键形如 fm_C01a_01_0_0_0_07 = 前缀 + 服装 + 表情后缀，
// 其中 CostumeList 已经是 "01_0_0_0"（含下划线），ExprList 是 "_01"。
func (s *Session) charaMacro() string {
	entry, ok := s.charaEntry()
	if !ok {
		return ""
	}
	costume := ""
	if len(entry.CostumeList) > 0 {
		c := s.chara.costume
		if c < 0 || c >= len(entry.CostumeList) {
			c = 0
		}
		costume = entry.CostumeList[c]
	}
	suffix := ""
	if len(entry.ExprList) > 0 {
		e := s.chara.expr
		if e < 0 || e >= len(entry.ExprList) {
			e = 0
		}
		suffix = entry.ExprList[e]
	}
	if entry.Prefix == "" || costume == "" || suffix == "" {
		return ""
	}
	return entry.Prefix + costume + suffix
}

// charaRow 是四行里的一行。
type charaRow struct {
	label    string
	rect     config.Rect
	arrowL   config.Rect
	arrowR   config.Rect
	hasLeft  bool
	hasRight bool
}

// charaRows 返回四行的几何与当前显示名。
//
// 行的位置：CharaStat 给第 1 行的 x/y/宽/高/步进，第 2..4 行沿用同样的 x/宽，
// y 按原版配置里的 84/128/172 递增（步进 44 = 格高 30 + 间隔 14）。
// 箭头不在配置里：图集 sys_charaview_button 的 (157,0,38,30) 是 ◀、(197,0,38,30) 是 ▶，
// 原版把箭头贴在每行格子的左右外侧（箭头宽 38、行高 30），这里按同样的相对位置放。
func (s *Session) charaRows() []charaRow {
	view := s.ui.Chara
	x, y := view.ListX, view.ListY
	w, h, step := view.ListW, view.ListH, view.ListStep
	if x <= 0 {
		x = 1008
	}
	if y <= 0 {
		y = 40
	}
	if w <= 0 {
		w = 156
	}
	if h <= 0 {
		h = 30
	}
	if step <= 0 {
		step = 44
	}
	// 四行的行距：配置里第 2..4 行的 y 是 84/128/172（相对第 1 行的 40 递增 44）。
	// 用 44 而不是 CharaStat 的第 7 字段（那个是列表内步进 30），
	// 否则四行会各压住下面 14px。
	rowStep := 44
	if step > h {
		rowStep = step
	} else {
		rowStep = h + 14
	}
	arrowW := 38
	arrowSrcL := view.RowArrowLsrc
	arrowSrcR := view.RowArrowRsrc
	if arrowSrcL.W <= 0 {
		arrowSrcL = config.Rect{X: 157, Y: 0, W: arrowW, H: h}
		arrowSrcR = config.Rect{X: 197, Y: 0, W: arrowW, H: h}
	}
	_ = arrowSrcR

	rows := make([]charaRow, 0, 4)
	labels := []string{"", "", ""}
	if entry, ok := s.charaEntry(); ok {
		_ = entry
	}
	if len(view.Characters) > 0 {
		c := clampIndex(s.chara.character, len(view.Characters))
		labels[0] = view.Characters[c]
	}
	costumes := s.charaCostumes()
	if len(costumes) > 0 {
		labels[1] = costumes[clampIndex(s.chara.costume, len(costumes))]
	}
	exprs := s.charaExpressions()
	if len(exprs) > 0 {
		labels[2] = exprs[clampIndex(s.chara.expr, len(exprs))]
	}
	bgLabel := ""
	if len(view.BGList) > 0 {
		bgLabel = view.BGList[clampIndex(s.chara.bg, len(view.BGList))].Label
	}
	for i, label := range append(labels, bgLabel) {
		rect := config.Rect{X: x, Y: y + i*rowStep, W: w, H: h}
		rows = append(rows, charaRow{
			label:    label,
			rect:     rect,
			arrowL:   config.Rect{X: x - arrowW - 4, Y: rect.Y, W: arrowW, H: h},
			arrowR:   config.Rect{X: x + w + 4, Y: rect.Y, W: arrowW, H: h},
			hasLeft:  true,
			hasRight: true,
		})
	}
	return rows
}

func clampIndex(value, count int) int {
	if count <= 0 {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value >= count {
		return count - 1
	}
	return value
}

// charaRowAt 返回命中的行下标（0..3），-1 表示没命中。
//
// 命中范围是"◀ + 格子 + ▶"整段：原版点箭头与点格子是同一条切换逻辑，
// 点格子本体等于"下一项"，与箭头等价。
func (s *Session) charaRowAt(x, y int) int {
	for i, row := range s.charaRows() {
		zh := row.rect.H + 14
		rect := config.Rect{X: row.arrowL.X, Y: row.rect.Y, W: row.arrowR.X + row.arrowR.W - row.arrowL.X, H: zh}
		if x >= rect.X && x < rect.X+rect.W && y >= rect.Y && y < rect.Y+zh {
			return i
		}
	}
	return -1
}

// composeCharaView 画立绘鉴赏器。
//
// 层序：背景（[BG] 选中的整屏图）→ 立绘 → 右侧四行 + 操作按钮。
func (s *Session) composeCharaView() {
	view := s.ui.Chara
	// 背景是整屏 GRD；[BG] 为空时原版就是黑底。
	s.stage.Clear(0, 0, 0, 0xFF)
	s.stage.SetCG("")
	if len(view.BGList) > 0 {
		if img := s.cache.Get(view.BGList[clampIndex(s.chara.bg, len(view.BGList))].Name); img != nil {
			s.stage.DrawImage(img, img.OffsetX, img.OffsetY)
		}
	}
	if !s.chara.hidden {
		if macro := s.charaMacro(); macro != "" {
			if img := s.stage.ResolveImage(macro); img != nil {
				// 立绘的虚拟屏幕是 1000x2048，整身缩进屏幕高度才是原版的默认比例。
				frame := float64(frameHeight) / float64(img.Height)
				zoom := s.chara.zoom
				if zoom <= 0 {
					zoom = 1
				}
				w := int(float64(img.Width) * frame * zoom)
				h := int(float64(img.Height) * frame * zoom)
				x := (frameWidth-w)/2 + s.chara.offsetX
				y := (frameHeight-h)/2 + s.chara.offsetY
				s.stage.DrawImageScaled(img, x, y, w, h, 255)
			}
		}
	}
	s.drawCharaRows()
	s.drawCharaButtons()
}

// drawCharaRows 画四行：◀ [名称] ▶。
//
// 名称区用图集第 0 格（156x30 的深色描边框）当底，箭头用 (157,0) / (197,0) 的裁片。
//
// 注意不能把名称框"按行宽拉伸"：图集里的框只有 156x30，行宽也是 156，
// 直接贴即可；拉伸会在半像素处把 1px 边框抹成 2px 灰边，看起来像按钮粘连。
// 名称里的长文本（コックピット_通常 这类 9 个全角字）会超出 156px，
// 原版是压缩字号贴进去，这里同样按可用宽度缩一档。
func (s *Session) drawCharaRows() {
	img := s.cache.Get("sys_charaview_button")
	if img == nil {
		return
	}
	view := s.ui.Chara
	arrowSrcL := view.RowArrowLsrc
	arrowSrcR := view.RowArrowRsrc
	if arrowSrcL.W <= 0 {
		arrowSrcL = config.Rect{X: 157, Y: 0, W: 38, H: view.ListH}
		arrowSrcR = config.Rect{X: 197, Y: 0, W: 38, H: view.ListH}
	}
	fieldSrc := config.Rect{X: 0, Y: 0, W: img.Width, H: 0}
	fieldSrc.W = view.ListW
	fieldSrc.H = view.ListH
	if img.Width >= 156 && fieldSrc.W <= 0 {
		fieldSrc = config.Rect{X: 0, Y: 0, W: 156, H: 30}
	}
	for _, row := range s.charaRows() {
		// 名称框：图集第 0 格原尺寸贴（行宽与格宽同为 156）。
		if fieldSrc.W > 0 && fieldSrc.H > 0 {
			s.stage.DrawRegion(img, fieldSrc.X, fieldSrc.Y, fieldSrc.W, fieldSrc.H,
				row.rect.X, row.rect.Y)
		}
		s.stage.DrawRegion(img, arrowSrcL.X, arrowSrcL.Y, arrowSrcL.W, arrowSrcL.H, row.arrowL.X, row.arrowL.Y)
		s.stage.DrawRegion(img, arrowSrcR.X, arrowSrcR.Y, arrowSrcR.W, arrowSrcR.H, row.arrowR.X, row.arrowR.Y)
		if row.label == "" {
			continue
		}
		// 名称：先按配置字号贴，放不下就按可用宽度缩字号（原版同样会缩）。
		size := view.FontSize
		if size <= 0 {
			size = 14
		}
		avail := row.rect.W - 12
		for size > 8 && runeAdvance(row.label, size) > avail {
			size--
		}
		textW := runeAdvance(row.label, size)
		tx := row.rect.X + (row.rect.W-textW)/2
		if tx < row.rect.X+6 {
			tx = row.rect.X + 6
		}
		s.drawUIRun(row.label, tx, row.rect.Y+(row.rect.H-size)/2, size, 0xffffffff, avail)
	}
}

// runeAdvance 估算一行文字的像素宽度（全角按字号、半角按字号一半），
// 只用于把名称在 156px 的框里居中。
func runeAdvance(text string, size int) int {
	width := 0
	for _, r := range text {
		if r < 0x80 || (r >= 0xFF61 && r <= 0xFF9F) {
			width += size / 2
			continue
		}
		width += size
	}
	return width
}

// drawCharaButtons 画操作按钮（图集 sys_charaview_button 的裁片）。
//
// 每个按钮的屏幕矩形与图集源矩形都来自 [CHARAVIEW] 的 B_* 行；
// 一行的第二个按钮是相对第一个的偏移（见 config.parseCharaView）。
func (s *Session) drawCharaButtons() {
	img := s.cache.Get("sys_charaview_button")
	if img == nil {
		return
	}
	draw := func(rect, src config.Rect) {
		if rect.Empty() || src.W <= 0 || src.H <= 0 {
			return
		}
		s.stage.DrawRegion(img, src.X, src.Y, src.W, src.H, rect.X, rect.Y)
	}
	view := s.ui.Chara
	// 上下左右：▲ 在上、▼ 在下、◀ 在左、▶ 在右。
	draw(view.MoveY, view.MoveYSrc)
	draw(view.MoveY2, view.MoveY2Src)
	draw(view.MoveX, view.MoveXSrc)
	draw(view.MoveX2, view.MoveX2Src)
	// 拡大 / 縮小（拡大在左、縮小在右）。
	draw(view.Scale2, view.Scale2Src)
	draw(view.Scale, view.ScaleSrc)
	draw(view.Init, view.InitSrc)
	draw(view.Hide, view.HideSrc)
	draw(view.End, view.EndSrc)
}

// pointerCharaView 处理立绘鉴赏器的点击。
//
// 返回值 handled=true 表示这次点击属于本界面，调用方不能再推进剧情。
func (s *Session) pointerCharaView(op uint32, x, y int) (bool, error) {
	view := s.ui.Chara
	if view.ListW <= 0 && view.End.Empty() {
		return false, nil
	}
	// 拖动立绘：触屏上没有"按住立绘"的概念，这里按 pan 的坐标增量平移。
	if op == 6 {
		if s.chara.dragging {
			s.chara.offsetX += x - s.chara.dragX
			s.chara.offsetY += y - s.chara.dragY
		}
		s.chara.dragging = true
		s.chara.dragX, s.chara.dragY = x, y
		s.dirty = true
		return true, nil
	}
	s.chara.dragging = false
	if op != 1 && op != 3 {
		// 移动/悬停不改变状态，但属于本界面（不能落到剧情推进）。
		return op == 0, nil
	}
	// 四行：点箭头向左/右循环，点格子本体向右循环。
	if index := s.charaRowAt(x, y); index >= 0 {
		rows := s.charaRows()
		row := rows[index]
		delta := 1
		if x < row.rect.X {
			delta = -1
		}
		s.stepCharaRow(index, delta)
		s.dirty = true
		return true, nil
	}
	switch {
	case view.Scale2.Contains(x, y):
		// 拡大（左侧那个）。
		s.chara.zoom *= 1.15
		if s.chara.zoom > 4 {
			s.chara.zoom = 4
		}
	case view.Scale.Contains(x, y):
		// 縮小。
		s.chara.zoom /= 1.15
		if s.chara.zoom < 0.25 {
			s.chara.zoom = 0.25
		}
	case view.Init.Contains(x, y):
		s.chara.zoom = 1
		s.chara.offsetX = 0
		s.chara.offsetY = 0
	case view.Hide.Contains(x, y):
		s.chara.hidden = !s.chara.hidden
	case view.MoveY.Contains(x, y):
		s.chara.offsetY -= 40
	case view.MoveY2.Contains(x, y):
		s.chara.offsetY += 40
	case view.MoveX.Contains(x, y):
		s.chara.offsetX -= 40
	case view.MoveX2.Contains(x, y):
		s.chara.offsetX += 40
	case view.End.Contains(x, y):
		// 戻る：view 脚本里 0xe3 之后那条 0x10 的目标就是 MENU。
		s.vm.LeaveCharaView()
		step := s.vm.SelectLabel("MENU")
		s.absorb()
		s.step = s.mapStep(step)
		s.atTitle = false
		return true, nil
	default:
		return false, nil
	}
	s.dirty = true
	return true, nil
}

// stepCharaRow 把第 row 行（0..3）的选择项循环移动 delta。
func (s *Session) stepCharaRow(row, delta int) {
	switch row {
	case 0:
		s.chara.character = cycleIndex(s.chara.character, len(s.ui.Chara.Characters), delta)
		// 换角色时服装/表情表整段换掉，下标必须回到 0：
		// 上一角色的第 3 套衣服在新角色页里可能不存在，宏名会拼出空串。
		s.chara.costume = 0
		s.chara.expr = 0
	case 1:
		s.chara.costume = cycleIndex(s.chara.costume, len(s.charaCostumes()), delta)
	case 2:
		s.chara.expr = cycleIndex(s.chara.expr, len(s.charaExpressions()), delta)
	case 3:
		s.chara.bg = cycleIndex(s.chara.bg, len(s.ui.Chara.BGList), delta)
	}
}

func cycleIndex(value, count, delta int) int {
	if count <= 0 {
		return 0
	}
	value += delta
	for value < 0 {
		value += count
	}
	return value % count
}

// DebugCharaState 是诊断用：立绘鉴赏器的选中项（日志/单测用）。
func (s *Session) DebugCharaState() string {
	return fmt.Sprintf("char=%d costume=%d expr=%d bg=%d zoom=%.2f off=(%d,%d) hidden=%v",
		s.chara.character, s.chara.costume, s.chara.expr, s.chara.bg,
		s.chara.zoom, s.chara.offsetX, s.chara.offsetY, s.chara.hidden)
}
