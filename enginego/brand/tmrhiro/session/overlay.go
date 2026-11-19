// 本文件是引擎自绘的模态界面：暂停菜单、存/读档、设置、回想、确认框。
//
// 为什么这些界面必须由引擎画：原版的坐标、按钮图、页签、格子全部来自
// _system 与 AGRD（见 config/ui.go 的 SlotBoard / SystemScreen），Flutter 侧
// 另画一套会出现"按钮图是原版、热区是另一套"的错位。更要紧的是这些界面
// 与原版的状态机绑定（存档写进原版的槽位、设置直接改引擎音量与速度、
// 回想读引擎的已读记录），交给宿主等于承认引擎没有实现原版功能。
//
// 层序：界面底图（sys_save_base / sys_system_base_01 …）→ 格子/控件状态图
// → 文字。底图本身就是深色板，直接贴上即可（不参与舞台的反相混合）。
package session

import (
	"strings"

	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/brand/tmrhiro/grd"
)

// overlayKind 是引擎自绘的模态界面。互斥：同一帧至多一个。
type overlayKind uint8

const (
	overlayNone overlayKind = iota
	overlayMenu
	overlaySave
	overlayLoad
	overlayConfig
	overlayBacklog
	overlayConfirm
)

// composeOverlay 画当前界面。
func (s *Session) composeOverlay() {
	switch s.overlay {
	case overlayMenu:
		s.composeMenu()
	case overlaySave:
		s.composeSlotBoard(true)
	case overlayLoad:
		s.composeSlotBoard(false)
	case overlayConfig:
		s.composeConfig()
	case overlayBacklog:
		s.composeBacklog()
	case overlayConfirm:
		s.composeConfirm()
	}
}

// composeMenu 画暂停菜单（屏幕顶端那排「セーブ / ロード / システム /
// タイトル画面 / ゲーム終了 / 回想終了」）。
//
// 底图是 sys_window_menu（1280x48 的深色条），按钮是 sys_window_button 的
// 第 4..5 行（悬停用 sys_window_button02 的同一区域）。
func (s *Session) composeMenu() {
	bar := s.ui.Menu
	if img := s.cache.Get("sys_window_menu"); img != nil {
		s.stage.DrawImage(img, 0, 0)
	}
	// 按钮图 sys_window_button 是 577x165 的图集：
	//   第 0 行 = 常态（Q-SAVE / Q-LOAD / … 与顶栏菜单的 セーブ / ロード / …），
	//   第 1 行 = 悬停/选中。
	// 每个按钮的源矩形已经在 [MESSAGEWINDOW] 的 TWB_* 里给出（SrcX/SrcY），
	// 悬停时把同一块换成下一行（+H）的图——原版就是这么切的。
	// 以前悬停时换整张 sys_window_button02，那张图的排布与常态图不完全一致，
	// 按钮会串位。
	normal := s.cache.Get("sys_window_button")
	if normal == nil {
		return
	}
	hover := s.cache.Get("sys_window_button02")
	if hover == nil {
		hover = normal
	}
	for index, button := range bar.Buttons {
		if !button.Enabled {
			continue
		}
		src := normal
		srcY := button.SrcY
		if index == s.overlayHover {
			src = hover
		}
		if button.SrcY+button.H <= src.Height {
			srcY = button.SrcY
		}
		s.stage.DrawRegion(src, button.SrcX, srcY, button.W, button.H, button.X, button.Y)
	}
}

// slotRect 返回当前页第 index 格（0 起）的矩形。
func (s *Session) slotRect(board config.SlotBoard, index int) config.Rect {
	col := index % board.Cols
	row := index / board.Cols
	x := board.BaseX + col*board.GapX
	y := board.BaseY + row*board.GapY
	return config.Rect{X: x, Y: y, W: board.TileW, H: board.TileH}
}

// slotCount 返回每页格数。
func slotCount(board config.SlotBoard) int {
	if board.Rows > 0 && board.Cols > 0 {
		return board.Rows * board.Cols
	}
	return 10
}

// composeSlotBoard 画存/读档界面。
//
// 原版只画"当前页的 10 格"：两列五行，每格 544x90；左侧缩略图 128x72 在
// (19,9)，右侧从 (200,10) 起三行文字（NO.001 / 日期 / 台词摘要）。
// 空位的格子贴 sys_save_button 里的 "NO DATA" 图块（180x104 那块灰色渐变）。
func (s *Session) composeSlotBoard(saveMode bool) {
	board := s.ui.Save
	base := "sys_save_base"
	button := "sys_save_button"
	label := "SAVE"
	if !saveMode {
		board = s.ui.Load
		base = "sys_load_base"
		button = "sys_load_button"
		label = "LOAD"
	}
	if img := s.cache.Get(base); img != nil {
		s.stage.DrawImage(img, img.OffsetX, img.OffsetY)
	}
	// 页签：SAVE_PAGE_POS 的 10 个按钮，当前页高亮。
	s.drawPageTabs(board)
	// 返回/自动/删档按钮（图集里的裁片）。
	if img := s.cache.Get(button); img != nil {
		s.drawCroppedButton(img, board.ReturnX, board.ReturnY, board.ReturnSrcX, board.ReturnSrcY, board.ReturnW, board.ReturnH)
		s.drawCroppedButton(img, board.AutoX, board.AutoY, board.AutoSrcX, board.AutoSrcY, board.AutoW, board.AutoH)
		s.drawCroppedButton(img, board.DelX, board.DelY, board.DelSrcX, board.DelSrcY, board.DelW, board.DelH)
	}
	perPage := slotCount(board)
	start := s.overlayPage * perPage
	for i := 0; i < perPage; i++ {
		slot := uint32(start + i + 1)
		rect := s.slotRect(board, i)
		entry, has := s.slotEntry(slot)
		s.drawSlotFrame(rect)
		if !has {
			s.drawNoData(button, board, rect)
			s.drawSlotText(board, rect, slot, "", "", false)
			continue
		}
		if thumb, ok := s.store.LoadThumb(slot); ok {
			s.drawThumb(thumb, rect.X+board.ThumbX, rect.Y+board.ThumbY, board.ThumbW, board.ThumbH)
		}
		s.drawSlotText(board, rect, slot, entry.SavedAt, entry.Summary, true)
		if i == s.overlayHover {
			s.drawSlotHover(button, rect)
		}
	}
	s.drawUIRun(label, board.BaseX-60, board.BaseY-46, 24, 0xffffffff, 200)
	s.drawUIRun("ページ "+itoa(s.overlayPage+1)+" / "+itoa(board.PageMax),
		board.PageX-30, board.BaseY-40, 18, 0xffffffff, 300)
}

// drawPageTabs 画页签（1..10），当前页用高亮图。
func (s *Session) drawPageTabs(board config.SlotBoard) {
	for page := 0; page < board.PageMax; page++ {
		x := board.PageX + page*board.PageStepX
		y := board.PageY
		state := uint32(0)
		if page == s.overlayPage {
			state = 1
		}
		s.drawUIRun(itoa(page+1), x+board.PageW/2-6, y+4, 22, stateColor(state), board.PageW)
	}
}

// stateColor 是页签/按钮的常态与选中颜色（原版选中态是亮黄）。
func stateColor(state uint32) uint32 {
	if state == 1 {
		return 0xffffff00
	}
	return 0xffffffff
}

// drawSlotFrame 给一个存档格画底板与描边（原版每格是"深色底 + 1px 白框"）。
//
// 底板不透明：存档界面盖在剧情画面上，半透明会让背景透出来、文字难读；
// 原版 SAVE.JPG 里格子内部就是接近纯黑。
func (s *Session) drawSlotFrame(rect config.Rect) {
	s.stage.Canvas().FillBlend(rect.X, rect.Y, rect.W, rect.H, 0x10, 0x12, 0x14, 0xF2)
	s.stage.Canvas().Fill(rect.X, rect.Y, rect.W, 1, 0xC8, 0xC8, 0xC8, 0xFF)
	s.stage.Canvas().Fill(rect.X, rect.Y+rect.H-1, rect.W, 1, 0xC8, 0xC8, 0xC8, 0xFF)
	s.stage.Canvas().Fill(rect.X, rect.Y, 1, rect.H, 0xC8, 0xC8, 0xC8, 0xFF)
	s.stage.Canvas().Fill(rect.X+rect.W-1, rect.Y, 1, rect.H, 0xC8, 0xC8, 0xC8, 0xFF)
}

// drawNoData 画空位的 "NO DATA" 图块。
//
// 图块是 sys_save_button 里 (0,0)-(180,104)，原版贴在格子左侧的缩略图位置
// （与 SAVE_B_THUM_POS 的 19,9 对齐）；右侧留白接 "No.001" 与日期。
func (s *Session) drawNoData(button string, board config.SlotBoard, rect config.Rect) {
	img := s.cache.Get(button)
	if img == nil {
		return
	}
	// NO DATA 图块在缩略图位置（SAVE_B_THUM_POS），尺寸与缩略图一致（128x72）；
	// 按 180x104 原尺寸贴会溢出 90px 高的格子，压住下面一行（手册里图块
	// 就停在格内）。
	w, h := board.ThumbW, board.ThumbH
	if w <= 0 || h <= 0 {
		w, h = 128, 72
	}
	x := rect.X + board.ThumbX
	if board.ThumbX <= 0 {
		x = rect.X + 19
	}
	y := rect.Y + board.ThumbY
	if board.ThumbY <= 0 {
		y = rect.Y + 9
	}
	s.stage.DrawRegionScaled(img, 0, 0, 180, 104, x, y, w, h, 255)
}

// drawSlotHover 给悬停格描一圈原版的选中边框（图集里的白色描边块）。
func (s *Session) drawSlotHover(button string, rect config.Rect) {
	img := s.cache.Get(button)
	if img == nil {
		return
	}
	// 图集里 (0,272) 是"整格白边"版（80x? 的渐变底板），按格子尺寸拉伸。
	s.stage.DrawRegionScaled(img, 0, 272, 544, 78, rect.X, rect.Y, rect.W, rect.H, 255)
}

// drawSlotText 画格子里的三行文字。
func (s *Session) drawSlotText(board config.SlotBoard, rect config.Rect, slot uint32, savedAt, summary string, hasData bool) {
	x := rect.X + board.TextX
	y := rect.Y + board.TextY
	line := board.TextNext
	if line <= 0 {
		line = 20
	}
	s.drawUIRun("No."+padSlot(slot), x, y, 20, 0xffffffff, rect.W-board.TextX-8)
	dateText := "----/--/-- --:--"
	if hasData && savedAt != "" {
		// 存档里记的是 "No.003 : 2026/11/17 09:12"：编号已经单独画了一行，
		// 这里只保留时间，避免"No.003 No.003"重复。
		if idx := strings.Index(savedAt, ": "); idx >= 0 {
			dateText = savedAt[idx+2:]
		} else {
			dateText = savedAt
		}
	}
	s.drawUIRun(dateText, x, y+line, 16, 0xffc0c0c0, rect.W-board.TextX-8)
	if hasData && summary != "" {
		s.drawUIRun(summary, x, y+line*2, 16, 0xffe0e0e0, rect.W-board.TextX-8)
	}
}

// composeConfig 画设置界面。
//
// 底图是 sys_system_base_01..05（一页一张），左侧导航与每个控件都在
// SYSTEM_PARTS_* 里定义；控件状态图是 sys_system_button_01 里的裁片。
// 原版把这些控件写成"种类 + 矩形 + 状态阵列"，这里按种类分派：
//
//	SW_ONE    单击一次切换（ON/OFF）
//	SW_GROUP  N 个互斥状态（模式选择、档位）
//	SW_SLIDE  滑杆（音量、不透明度）
//	SW_PUSH   立绘/角色设定入口
func (s *Session) composeConfig() {
	page := s.overlayPage
	if page < 0 {
		page = 0
	}
	if page >= s.ui.System.PageMax {
		page = s.ui.System.PageMax - 1
	}
	base := "sys_system_base_0" + itoa(page+1)
	if img := s.cache.Get(base); img != nil {
		s.stage.DrawImage(img, img.OffsetX, img.OffsetY)
	}
	// 左侧导航：五个页签。底图 sys_system_base_0N 上已经画好这些文字，
	// 这里只需要在选中页上压一层高亮（原版用 S_PAGE 的选中帧）。
	for index, rect := range s.ui.System.PageButtons {
		if index != page {
			continue
		}
		s.stage.Canvas().FillBlend(rect.X, rect.Y, rect.W, rect.H, 0x60, 0xF0, 0xE0, 0x40)
	}
	// 「戻る / ゲーム終了 / タイトル画面」的文字已经画在 sys_system_base 上，
	// 这里不再叠字（否则会描成双份），只在悬停时压一层高亮。
	for _, rect := range []config.Rect{s.ui.System.GameButton, s.ui.System.TitleButton, s.ui.System.EndButton} {
		if rect.Contains(s.overlayHoverX, s.overlayHoverY) && s.overlayHover >= 0 {
			s.stage.Canvas().FillBlend(rect.X, rect.Y, rect.W, rect.H, 0x60, 0xF0, 0xE0, 0x40)
		}
	}
	// 控件：只画当前页的（PARTS 的编号前两位是页号）。
	for _, part := range s.ui.System.Parts {
		// Page 是 SYSTEM_PARTS_XXYY 的 XX（1 起）；page 是 0 起的当前页。
		if part.Page != page+1 {
			continue
		}
		s.drawSystemPart(part)
	}
}

// drawSystemPart 画一个设置控件。
//
// 几何一律来自 config.SystemPart（字段含义见那边的注释），这里不再猜：
//
//   - 第 i 个单元的屏幕位置 = X + i*ColStep，图集源坐标 = SrcX + i*SrcStep；
//     两者都带符号（负 = 向左排）。on/off 组的 ON 在 X-168、图集里在
//     SrcX-160 就是这个原因。
//   - 每行 PerRow 个单元，行距 RowStepY（多行控件：サウンド 8×2、
//     確認メッセージ 8×2）。
//   - 图集里同一单元的三种状态竖排：第 0 行常态、第 1 行选中、第 2 行悬停。
//
// 早期实现把选项一律当成"从 X 往右、从 SrcX 往右"贴，多行控件也当成一行，
// 于是 ON/OFF 左右颠倒、第二行压在第一行上；命中矩形又按整行算，点滑杆
// 会先命中同一行的开关组。
func (s *Session) drawSystemPart(part config.SystemPart) {
	button := s.cache.Get("sys_system_button_01")
	if button == nil || part.W <= 0 || part.H <= 0 || part.SrcX < 0 || part.SrcY < 0 {
		return
	}
	if part.Kind == "SW_SLIDE" {
		s.drawSystemSlide(part, button)
		return
	}
	options := part.Total
	if options <= 0 {
		options = 1
	}
	perRow := part.PerRow
	if perRow <= 0 {
		perRow = 1
	}
	// 图集步进与屏幕步进同符号：负步进时 SrcX 指向**最右一格**（ON|OFF 的
	// SrcX=160 是 OFF、SrcStep=-160 指向 ON=0）。所以第一格要按
	// SrcX + (perRow-1)*SrcStep 定位，与 systemPartCell 的算法保持一致，
	// 否则 ON/OFF 的文字会左右互换（点 ON 高亮 OFF）。
	srcFirst := part.SrcX
	srcStep := part.SrcStep
	if srcStep < 0 {
		srcFirst = part.SrcX + (perRow-1)*srcStep
		srcStep = -srcStep
	}
	for i := 0; i < options; i++ {
		row, col := i/perRow, i%perRow
		x, y := s.systemPartCell(part, i)
		sx := srcFirst + col*srcStep
		if sx < 0 || sx+part.W > button.Width || part.SrcY+part.H > button.Height {
			continue
		}
		// 常态帧（图集第 0 行）。
		s.stage.DrawRegion(button, sx, part.SrcY, part.W, part.H, x, y)
		// 选中帧（图集第 1 行）只贴在当前值那一格。
		if s.systemRowChoice(part, row) == col && part.SrcY+part.H*2 <= button.Height {
			s.stage.DrawRegion(button, sx, part.SrcY+part.H, part.W, part.H, x, y)
		}
	}
}

// systemRowChoice 返回第 row 行当前选中的列号。
//
// 多行控件（サウンド的 8×2、確認メッセージ的 8×2）每一行是**独立的**
// ON/OFF 开关（按键音/打ち付け音/キス音…），不是"一个 8 档选项排成两行"：
// 原版把行号当作第 0..Rows-1 个开关，列号才是 ON/OFF。所以档位按
// "键名+行号"分别保存，绘制时每行各自高亮。
func (s *Session) systemRowChoice(part config.SystemPart, row int) int {
	if part.Rows <= 1 {
		index, _ := s.systemPartState(part)
		if part.Total > 0 && part.PerRow > 0 {
			return index % part.PerRow
		}
		return index
	}
	// 多行控件按"键名#行号"回读；没存过的行用配置的出厂默认：
	// 確認メッセージ四行原版默认全 ON（SYS_DEF_MES_CONFIRM=1,1,1,1），
	// サウンド的按键音/打ち付け音默认 ON、其余 OFF。
	if value, ok := s.systemChoices[part.Key+"#"+itoa(row)]; ok {
		return value
	}
	switch part.Key {
	case "S_MES_CONFIRM":
		return 0
	}
	if lineDefaults, ok := systemRowDefault(part.Key); ok && row < len(lineDefaults) {
		return lineDefaults[row]
	}
	return 0
}

// systemRowDefault 是"多行开关"每组每一行的出厂默认列号（0=ON / 1=OFF）。
//
// 值取自 _system 的 SYS_DEF_*（本作只给了確認メッセージ的 1,1,1,1，
// 其余按原版首次进入设置页的显示：按键音与打ち付け音是 ON、其它 OFF）。
func systemRowDefault(key string) ([]int, bool) {
	switch key {
	case "S_SOUND_HSE":
		// 射精音 / キス・フェラ音 / 受傷音 / 挿入音 —— 四行全 OFF。
		return []int{1, 1, 1, 1}, true
	case "S_SOUND_HSE03":
		// 打ち付け音 / 吐息・喘ぎ声 / 絶頂音 / その他 —— 四行全 OFF。
		return []int{1, 1, 1, 1}, true
	}
	return nil, false
}

// systemPartCell 返回第 index 个单元在屏幕上的左上角。
//
// 单元按"每行 PerRow 个"排：同一行 i%PerRow 个横向递增 ColStep，
// 行号 i/PerRow 竖向递增 RowStepY。ColStep 为负时从锚点向左铺（本作的
// ON/OFF 组就是这样：SrcX 指向 OFF，第一步往左才是 ON）。
func (s *Session) systemPartCell(part config.SystemPart, index int) (int, int) {
	perRow := part.PerRow
	if perRow <= 0 {
		perRow = 1
	}
	row := index / perRow
	col := index % perRow
	step := part.ColStep
	if step == 0 {
		step = part.W
	}
	rowStep := part.RowStepY
	if rowStep <= 0 {
		rowStep = part.H
	}
	// 负步进表示"配置里的 X 是**最右一格**的左沿，选项从它向左铺":
	// S_GRAPHIC=449,-168 的两块板实际是 ON 在 281、OFF 在 449；
	// S_MES_CONFIRM=1013,-168 的 8 格是 845/1013 两列。第一格的 x 因此是
	// X + (perRow-1)*step，行内用 |step| 向右递增，否则 ON/OFF 会左右颠倒
	// （点 ON 却落在 OFF 上、画出来也和原版反着）。
	firstX := part.X
	colStep := step
	if step < 0 {
		firstX = part.X + (perRow-1)*step
		colStep = -step
	}
	return firstX + col*colStep, part.Y + row*rowStep
}

// drawSystemSlide 画一组滑杆。
//
// 图集里只有**滑块**（SrcX,SrcY,W,H 的圆钮），轨道是原引擎自己画的细线：
// 手册截图里是「MIN ————●———— MAX」的细条。以前把图集里的 (SrcX,SrcY)
// 当整条轨道贴出去，结果是一条黑带压在标签上，滑块也只剩一个圆点。
//
// 一组滑杆 = Total 条，每条占一条轨道，行距 RowStepY（不透明度 4 条 33px、
// 音量 3 条 32px）。拖动的命中区由 systemPartAt 按 SlideX 的宽度算，
// 拖动时它会给出"落在第几档"，与这里的 ratio 是同一套换算。
func (s *Session) drawSystemSlide(part config.SystemPart, atlas *grd.Image) {
	trackW := part.SlideX
	if trackW <= 0 {
		trackW = part.W
	}
	knobW := part.W
	if knobW <= 0 {
		knobW = 12
	}
	count := part.Total
	if count <= 0 {
		count = 1
	}
	rowStep := part.RowStepY
	if rowStep <= 0 {
		rowStep = part.H
	}
	for i := 0; i < count; i++ {
		rowIndex, rowMax := s.slideRowState(part, i)
		y := part.Y + i*rowStep
		midY := y + part.H/2
		// 轨道：原版是"细线 + 两端圆点"，这里画 2px 的浅灰细线。
		s.stage.Canvas().Fill(part.X, midY, trackW, 2, 0xC8, 0xC8, 0xC8, 0xFF)
		ratio := 0.0
		if rowMax > 1 {
			ratio = float64(rowIndex) / float64(rowMax-1)
		}
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		// 已滑过的部分略亮，便于看出当前值。
		knobX := part.X + int(ratio*float64(trackW-knobW))
		if knobX > part.X {
			s.stage.Canvas().Fill(part.X, midY, knobX-part.X, 2, 0xFF, 0xFF, 0xFF, 0xFF)
		}
		// 滑块：图集里的圆钮（SrcX,SrcY,W,H）。
		if part.SrcX+knobW <= atlas.Width && part.SrcY+part.H <= atlas.Height {
			s.stage.DrawRegion(atlas, part.SrcX, part.SrcY, knobW, part.H, knobX, y)
		}
	}
}

// slideRowState 返回第 i 条滑杆的当前档位与满档数。
//
// 本作的滑杆分两类：
//   - 一条滑杆但多条值（不透明度：4 条分别是不透明度与 R/G/B，原版按
//     "窗口底色"保存 0..100，这里把同一个值画到 4 条轨道的同一位置）；
//   - 每条滑杆各自一个值（音量：BGM/ボイス/通常SE/H系SE 是 4 个独立音量，
//     配置按"每行一个"排列）。区分依据是键名：音量类按行号取各自的设置。
func (s *Session) slideRowState(part config.SystemPart, row int) (int, int) {
	const maxValue = 100
	// 先看有没有玩家拖动过的值（applySystemSlide 按"键名#行号"保存），
	// 有就按它画滑块；没有才回退到引擎里的实际音量/默认档位。
	if value, ok := s.systemChoices[part.Key+"#"+itoa(row)]; ok {
		return clampIntValue(value, maxValue), maxValue
	}
	switch part.Key {
	case "S_SOUND_VOLUME":
		// BGM 一组（3 条：BGM/语音/SE 由 S_SOUND 管，这一组只有 BGM 的实际音量）。
		return clampIntValue(int(s.bgmVolume*maxValue+0.5), maxValue), maxValue
	case "S_SOUND":
		switch row {
		case 0:
			return clampIntValue(int(s.bgmVolume*maxValue+0.5), maxValue), maxValue
		case 1:
			return clampIntValue(int(s.voiceVolume*maxValue+0.5), maxValue), maxValue
		}
		return clampIntValue(int(s.seVolume*maxValue+0.5), maxValue), maxValue
	case "S_VP_SOUND_DOWN_VOLUME":
		return 100, maxValue
	}
	// 其它滑杆（底色的不透明度与 R/G/B）没有可观察行为：按原版出厂值
	// （手册截图是不透明度 100%、R/G/B 255）画滑块位置。
	return maxValue, maxValue
}

// systemRowChoice 返回第 row 行当前选中的列号。
//
// 多行控件（サウンド的 8×2、確認メッセージ的 8×2）每一行是**独立的**
// ON/OFF 开关（按键音/打ち付け音/キス音…），不是"一个 8 档选项排成两行"：
// 原版把行号当作第 0..Rows-1 个开关，列号才是 ON/OFF。所以档位按
// "键名+行号"分别保存，绘制时每行各自高亮。
// systemPartState 返回控件当前选中的单元下标与总单元数。
//
// 每个 PART 的单元排布是"每行 PerRow 个、共 Total 个"，所以下标同时用于
// 绘制（选中帧）与命中（点击落在哪一格）。不同键的取值来源不同：
//   - 文本/自动速度：引擎里的 1..10 档 → 0..9；
//   - 音量：0..1 的比例 → 按条数折算；
//   - 开关：引擎里的布尔值（skipUnread/effects/…）；
//   - 其余：systemChoices 里保存的单元下标（原版的选中档位）。
func (s *Session) systemPartState(part config.SystemPart) (int, int) {
	options := part.Total
	if options <= 0 {
		options = 1
	}
	index, ok := s.systemPartValue(part, options)
	if !ok {
		return 0, options
	}
	if index < 0 {
		index = 0
	}
	if index >= options {
		index = options - 1
	}
	return index, options
}

// systemPartValue 按控件键取"当前是第几格"。
func (s *Session) systemPartValue(part config.SystemPart, options int) (int, bool) {
	key := part.Key
	switch key {
	case "S_TEXTSPEED":
		return clampIntValue(s.textSpeed-1, 9), true
	case "S_AUTOSPEED":
		return clampIntValue(s.autoSpeed-1, 9), true
	case "S_SOUND", "S_SOUND_VOLUME":
		volume := s.seVolume
		if key == "S_SOUND_VOLUME" {
			volume = s.bgmVolume
		}
		if options <= 1 {
			return 0, true
		}
		return clampIntValue(int(volume*float64(options-1)+0.5), options-1), true
	case "S_SKIP", "S_SKIP_H_STOP", "S_GRAPHIC", "S_AUTO_VOICE_CUT",
		"S_READ_TEXT_COLOR", "S_VULGAR_WORD", "S_WINDOW_BASE_DRAW",
		"S_SKIP_S_STOP", "S_SKIP_V_STOP", "S_SKIP_E_STOP":
		if s.boolSetting(key) {
			return 1, true
		}
		return 0, true
	}
	if s.systemChoices != nil {
		if value, ok := s.systemChoices[key]; ok {
			return value, true
		}
	}
	// 没有保存过档位的控件停在第一格（与原版首次进入设置页一致）。
	return 0, true
}

// boolSetting 把 SW_ONE 的键映射到引擎里的开关值。
func (s *Session) boolSetting(key string) bool {
	switch key {
	case "S_SKIP":
		return s.skipUnread
	case "S_SKIP_H_STOP":
		return s.ceaseSkip
	case "S_GRAPHIC":
		return s.effects
	case "S_AUTO_VOICE_CUT":
		return s.autoVoiceCut
	case "S_READ_TEXT_COLOR":
		return s.readColorOn
	case "S_VULGAR_WORD":
		return s.vulgarWord
	case "S_WINDOW_BASE_DRAW":
		return s.windowBaseDraw
	}
	return false
}

// intSetting 把滑杆/多态控件的键映射到（当前值, 档位数）。
//
// 选项数用配置的 Cols（本作 S_TEXTSPEED/S_AUTOSPEED/音量都是 10/10/4 档），
// 以前滑杆恒按 100 档换算，滑块位置与"点哪一格"都对不上。

// systemChoice 读取/回显"设备类选项"的选择（右键行为、窗口模式…）。
//
// 这些选项在触屏移植里没有可观察行为，但原版的选中状态必须能回显，
// 否则玩家点完看不到任何变化。存在内存里，并随设置文件落盘。
func (s *Session) systemChoice(key string, options int) int {
	if s.systemChoices == nil {
		return 0
	}
	value, ok := s.systemChoices[key]
	if !ok || value < 0 || value >= options {
		return 0
	}
	return value
}

func (s *Session) storeSystemChoice(key string, index int) {
	if s.systemChoices == nil {
		s.systemChoices = make(map[string]int)
	}
	s.systemChoices[key] = index
}

func clampIntValue(value, max int) int {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

// composeBacklog 画回想界面。
//
// 底图是 sys_backlog_base（1280x720 的深色板 + 右侧滚动条）；正文按
// _system 的 [BACKLOG] 段排版（BACKLOG_MAX 行、行距、起点）。滚动位置由
// 宿主滚轮/拖动改 s.overlayScroll。
func (s *Session) composeBacklog() {
	if img := s.cache.Get("sys_backlog_base"); img != nil {
		s.stage.DrawImage(img, 0, 0)
	}
	// 底图右下角的「VOICE / ▲ / ▼ / return」按钮图集只取需要的几格：
	// sys_backlog_button 是 302x120 的整块图，直接整张贴会把三行按钮
	// 全铺在左下角（原版只画一行，其余是悬停/按下帧）。
	if img := s.cache.Get("sys_backlog_button"); img != nil {
		// 这条是"底部条带"：原版按 header 的 top（600）直接贴。用 OffsetY
		// （= screenH-bottom = 0）会把它画到屏幕顶端——回想页的
		// VOICE/▲/▼/return 全跑到左上角就是这个原因。
		top := img.Top
		if top <= 0 {
			top = img.CanvasH - img.Height
		}
		// 图集是 3 行 × 30px 的按钮（VOICE / ▲▼ / return），只画第一行。
		rowH := 30
		if img.Height >= 90 {
			rowH = img.Height / 4
		}
		s.stage.DrawRegion(img, 0, 0, img.Width, rowH, img.OffsetX, top)
	}
	layout := s.backlogLayout()
	visible := layout.maxLines
	if visible <= 0 {
		visible = 5
	}
	height := layout.lineHeight
	if height <= 0 {
		height = 30
	}
	x := layout.x
	y := layout.y
	start := s.overlayScroll
	if start < 0 {
		start = 0
	}
	if len(s.backlog)-start < visible {
		start = len(s.backlog) - visible
	}
	if start < 0 {
		start = 0
	}
	for i := 0; i < visible; i++ {
		index := start + i
		if index >= len(s.backlog) {
			break
		}
		s.drawUIRun(s.backlog[index].Text, x, y+i*height, layout.fontSize, 0xffffffff, layout.width)
	}
	s.drawUIRun("回想 (ホイールでスクロール / 右クリックで戻る)", x, 20, 20, 0xffffffff, 600)
}

// backlogLayout 汇总 [BACKLOG] 段。
type backlogLayout struct {
	maxLines   int
	lineHeight int
	fontSize   int
	x, y       int
	width      int
}

func (s *Session) backlogLayout() backlogLayout {
	out := backlogLayout{maxLines: 5, lineHeight: 124, fontSize: 24, x: 94, y: 100, width: 1060}
	if s.system == nil {
		return out
	}
	out.maxLines = s.system.Int("BACKLOG", "BACKLOG_MAX", 5)
	out.lineHeight = s.system.Int("BACKLOG", "BACKLOG_NEXT_Y", 124)
	out.fontSize = s.system.Int("BACKLOG", "BACKLOG_FONT_SIZE", 24)
	out.x = s.system.Int("BACKLOG", "BACKLOG_POS_X", 94)
	out.y = s.system.Int("BACKLOG", "BACKLOG_POS_Y", 100)
	return out
}

// composeConfirm 画确认框（存/读/删档、回标题、退出游戏）。
//
// 原版没有"自己画的对话框"：它直接贴 sys_message_base_01..05 这五张
// **已经把文案画在图里**的底图（ゲームを終了します。/ タイトル画面に戻ります。/
// セーブデータを上書きします。/ ロードします。/ データを削除します。），
// 再贴 sys_message_button（2 列 × 3 行的 YES/NO 图集）。
// 位置与按钮几何全部来自 _system 的 [MESSAGE]（MES_POS / MES_B_*）。
//
// 以前这里用代码画黑底 + Flutter 文字 + sys_system_button_02（那是"クイック
// セーブしました"的提示条，不是按钮），于是弹窗里出现两行被压扁的文字和
// 一张完全不相干的图——"按钮和文字错乱"就是这么来的。
func (s *Session) composeConfirm() {
	if s.confirm == nil {
		return
	}
	box := s.confirmBox()
	// 底图：按动作选原版那张已经画好文案的图。
	if img := s.cache.Get(confirmBaseImage(s.confirm.action)); img != nil {
		s.stage.DrawImage(img, box.X, box.Y)
	} else {
		// 没有素材时退回半透明黑底，至少文字可读（不改变几何）。
		s.stage.Canvas().FillBlend(box.X, box.Y, box.W, box.H, 0x10, 0x12, 0x14, 0xD8)
	}
	yes, no := s.confirmButtons()
	buttons := s.cache.Get("sys_message_button")
	if buttons != nil {
		// 图集是 2 列（YES / NO）× 3 行（常态 / 悬停 / 按下），每格
		// 108x30；本作的按钮尺寸正好是 108x30，所以按格原尺寸贴。
		cellW, cellH := 108, 30
		if buttons.Width%2 == 0 {
			cellW = buttons.Width / 2
		}
		if buttons.Height >= 3*cellH {
			cellH = buttons.Height / 3
		}
		drawCell := func(rect config.Rect, col, row int) {
			s.stage.DrawRegionScaled(buttons, col*cellW, row*cellH, cellW, cellH,
				rect.X, rect.Y, rect.W, rect.H, 255)
		}
		row := 0
		if s.overlayHover == 0 {
			row = 1
		}
		drawCell(yes, 0, row)
		row = 0
		if s.overlayHover == 1 {
			row = 1
		}
		drawCell(no, 1, row)
		return
	}
	// 没有按钮图集：退化为文字按钮（仅供缺素材的作品兜底）。
	s.drawUIRun("YES", yes.X+30, yes.Y+6, 20, 0xffffffff, yes.W)
	s.drawUIRun("NO", no.X+38, no.Y+6, 20, 0xffffffff, no.W)
}

// confirmBaseImage 按确认动作选原版底图（文案已经画在图上）。
func confirmBaseImage(action string) string {
	switch action {
	case "exit":
		return "sys_message_base_01" // ゲームを終了します。
	case "title":
		return "sys_message_base_02" // タイトル画面に戻ります。
	case "save":
		return "sys_message_base_03" // セーブデータを上書きします。
	case "load":
		return "sys_message_base_04" // ロードします。
	case "delete":
		return "sys_message_base_05" // データを削除します。
	case "percent100", "percent0":
		// 达成率确认由脚本自绘（menu100/menu000 里已经画好了整块对话框），
		// 这里只用一张通用提示图兜底，实际不会走到。
		return "sys_message_base_04"
	}
	return "sys_message_base_01"
}

// confirmBox 返回确认框底图的屏幕矩形（[MESSAGE] 的 MES_POS/MES_WIDTH）。
func (s *Session) confirmBox() config.Rect {
	layout := s.ui.Message
	box := config.Rect{
		X: layout.ConfirmX,
		Y: layout.ConfirmY,
		W: layout.ConfirmW,
		H: layout.ConfirmH,
	}
	if box.W <= 0 || box.H <= 0 {
		// 缺 [MESSAGE] 时按本作手册的 380x166 兜底。
		box = config.Rect{X: 450, Y: 267, W: 380, H: 166}
	}
	return box
}

// confirmButtons 返回 YES/NO 两个按钮的矩形。
//
// 几何来自 [MESSAGE]：MES_B_POS_X/Y 是第一个按钮在底图内的偏移、
// MES_B_NEXT_X 是第二个按钮相对第一个的横向步进（本作 134）。
func (s *Session) confirmButtons() (config.Rect, config.Rect) {
	layout := s.ui.Message
	box := s.confirmBox()
	w, h := layout.ButtonW, layout.ButtonH
	if w <= 0 || h <= 0 {
		w, h = 108, 30
	}
	nextX := layout.ButtonNextX
	if nextX <= 0 {
		nextX = w + 26
	}
	yes := config.Rect{X: box.X + layout.ButtonX, Y: box.Y + layout.ButtonY, W: w, H: h}
	no := config.Rect{X: yes.X + nextX, Y: yes.Y, W: w, H: h}
	return yes, no
}

// drawCroppedButton 从图集里裁一块贴到屏幕（存档界面的返回/自动/删档按钮）。
//
// 这四组按钮的图集源坐标来自 SAVE_RETURN_POS 等的第 3/4 字段；宽高是第 5/6
// 字段。图集里同一个按钮画了三帧（常态/悬停/按下），源坐标已经指向第一帧。
func (s *Session) drawCroppedButton(img *grd.Image, x, y, srcX, srcY, w, h int) {
	if img == nil || w <= 0 || h <= 0 {
		return
	}
	if srcX < 0 || srcY < 0 || srcX+w > img.Width || srcY+h > img.Height {
		// 源矩形越界：退回整图左上角（至少按钮还看得见）。
		srcX, srcY = 0, 0
	}
	s.stage.DrawRegion(img, srcX, srcY, w, h, x, y)
}
