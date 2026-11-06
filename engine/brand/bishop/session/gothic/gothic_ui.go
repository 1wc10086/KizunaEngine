package gothic

// gothic_ui.go 是 MAZE 的系统界面状态机。
//
// 原版把标题、设置、鉴赏、存读档都画在游戏自己的 640x480 画布上，
// 只有右键菜单是 Windows 原生菜单。这里保留同一套坐标/素材：
//   - 标题：title01 底图，title02 的蓝色条作为逐项高亮；
//   - 设置：option0 底图；
//   - 汁设定：mode0 底图；
//   - CG 鉴赏：album 底图 + sumbnail 缩略图册；
//   - 回想：kaisou1 底图；
//   - 音乐：music0 底图。
// Flutter 只拿到透明的 SelectItems 热区，不重画任何界面。

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
)

type gothicUIMode int

const (
	uiNone gothicUIMode = iota
	uiTitle
	uiOption
	uiJuice
	uiGalleryMenu
	uiAlbum
	uiKaisou
	uiMusic
	uiPreview
	uiSlots
	uiIngame
	uiStaffRoll
)

type gothicUI struct {
	mode  gothicUIMode
	hover int
	// page 是鉴赏页（0 起）。album 12 格/页，kaisou 12 格/页。
	page int
	// slotSave 区分存/读；slotReturn 是关闭存读档后回到哪里。
	slotSave   bool
	slotReturn gothicUIMode
	// preview 是查看中的大图名；previewReturn 是查看前的页面。
	preview       string
	previewReturn gothicUIMode
	// staffIndex 是片尾第几张（0..7）。
	staffIndex int
	// playingTrack 是音乐鉴赏里正在播的格子，-1 表示没有。
	playingTrack int
	// optionReturn 是设置页的来源（标题/游戏内/鉴赏）。
	optionReturn gothicUIMode
	// 存读档界面打开时一次性读取 15 个槽位的信息/缩略图，
	// 避免每帧 Compose 都去读磁盘。
	slotInfos    []GothicSaveSlotInfo
	slotPreviews []*GothicSavePreview
	// titleLoadEnabled 缓存“有没有存档”，标题界面每帧都会问 TitleMenu。
	titleLoadEnabled bool
	// pageSelect 表示 Toku1 的 CG 鉴赏正停在 album1 的页码选择页。
	pageSelect bool
}

type gothicUIHit struct {
	action string
	label  string
	x      int
	y      int
	w      int
	h      int
}

func (s *Session) uiOpen(mode gothicUIMode) {
	if s.ui == nil {
		s.ui = &gothicUI{playingTrack: -1}
	}
	s.ui.mode = mode
	s.ui.hover = -1
	s.stage.Invalidate()
}

func (s *Session) uiClose() {
	if s.ui == nil {
		return
	}
	// 音乐鉴赏退出时停掉试听，回到剧情后不会残留 BGM。
	if s.ui.mode == uiMusic && s.ui.playingTrack >= 0 {
		s.gameState.ClearBgm()
	}
	s.ui = nil
	s.stage.Invalidate()
}

// currentUIMode 让外部（State/step）不用解引用 nil。
func (s *Session) currentUIMode() gothicUIMode {
	if s.ui == nil {
		return uiNone
	}
	return s.ui.mode
}

func (s *Session) uiStep() core.Step {
	switch s.currentUIMode() {
	case uiTitle:
		return core.StepAwaitTitle
	case uiNone:
		return core.StepContinue
	case uiStaffRoll:
		return core.StepAwaitInput
	default:
		return core.StepAwaitSelect
	}
}

// uiItems 是当前界面所有可点区域。坐标全部用引擎的 640x480 坐标系，
// 宿主只按画面缩放换算，不猜任何布局。
func (s *Session) uiItems() []gothicUIHit {
	if s.ui == nil {
		return nil
	}
	switch s.ui.mode {
	case uiOption:
		return s.optionItems()
	case uiJuice:
		return s.juiceItems()
	case uiGalleryMenu:
		items := []gothicUIHit{
			{action: "gallery.cg", label: "CG鑑賞", x: 180, y: 185, w: 280, h: 42},
			{action: "gallery.kaisou", label: "回想", x: 180, y: 227, w: 280, h: 42},
			{action: "gallery.music", label: "音楽", x: 180, y: 269, w: 280, h: 42},
		}
		// STAFFROOM 是通关后的隐藏标签；原版在 omake 里才会出现。
		if s.global != nil && s.global.Cleared {
			items = append(items, gothicUIHit{action: "gallery.staffroom", label: "スタッフルーム", x: 180, y: 311, w: 280, h: 42})
		}
		items = append(items, gothicUIHit{action: "gallery.back", label: "戻る", x: 180, y: 353, w: 280, h: 42})
		return items
	case uiAlbum:
		if s.profile == gothicProfileToku1 {
			if s.ui.pageSelect {
				items := make([]gothicUIHit, 0, 12)
				pages := (len(s.cgNames) + toku1GalleryPerPage - 1) / toku1GalleryPerPage
				if pages < 1 {
					pages = 1
				}
				for page := 1; page <= pages; page++ {
					rect := toku1PageButtonRect(page)
					items = append(items, gothicUIHit{action: fmt.Sprintf("album.goto.%d", page-1), label: fmt.Sprintf("%d/%d", page, pages), x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
				}
				items = append(items, gothicUIHit{action: "album.pages.exit", label: "戻る", x: 500, y: 425, w: 90, h: 40})
				return items
			}
			// album0 的底部按钮：左「後」=下一页，右「前」=上一页，右边「戻る」=退出。
			items := []gothicUIHit{
				{action: "album.pages", label: "ページ選択", x: 45, y: 426, w: 130, h: 30},
				{action: "album.next", label: "後", x: 215, y: 425, w: 105, h: 40},
				{action: "album.prev", label: "前", x: 330, y: 425, w: 90, h: 40},
				{action: "album.exit", label: "戻る", x: 500, y: 425, w: 90, h: 40},
			}
			start := s.ui.page * toku1GalleryPerPage
			for i := 0; i < toku1GalleryPerPage && start+i < len(s.cgNames); i++ {
				rect := toku1GalleryCellRect(i)
				items = append(items, gothicUIHit{action: fmt.Sprintf("album.cell.%d", i), label: s.cgNames[start+i], x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
			}
			return items
		}
		items := []gothicUIHit{
			{action: "album.prev", label: "前のページ", x: 30, y: 22, w: 150, h: 42},
			{action: "album.next", label: "次のページ", x: 450, y: 22, w: 150, h: 42},
			{action: "album.exit", label: "EXIT", x: 515, y: 432, w: 105, h: 40},
		}
		start := s.ui.page * 12
		for i := 0; i < 12 && start+i < len(s.cgNames); i++ {
			rect := albumCellRect(i)
			items = append(items, gothicUIHit{action: fmt.Sprintf("album.cell.%d", i), label: s.cgNames[start+i], x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
		}
		return items
	case uiKaisou:
		if s.profile == gothicProfileToku1 {
			items := []gothicUIHit{{action: "kaisou.exit", label: "戻る", x: 560, y: 427, w: 65, h: 38}}
			start := s.ui.page * toku1GalleryPerPage
			labels := s.toku1SceneLabels()
			for i := 0; i < toku1GalleryPerPage && start+i < len(labels); i++ {
				rect := toku1GalleryCellRect(i)
				items = append(items, gothicUIHit{action: fmt.Sprintf("kaisou.cell.%d", i), label: labels[start+i], x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
			}
			return items
		}
		items := []gothicUIHit{
			{action: "kaisou.prev", label: "前のページ", x: 30, y: 22, w: 150, h: 42},
			{action: "kaisou.next", label: "次のページ", x: 450, y: 22, w: 150, h: 42},
			{action: "kaisou.exit", label: "EXIT", x: 515, y: 432, w: 105, h: 40},
		}
		start := s.ui.page * 12
		for i := 0; i < 12 && start+i < 21; i++ {
			rect := albumCellRect(i)
			items = append(items, gothicUIHit{action: fmt.Sprintf("kaisou.cell.%d", i), label: fmt.Sprintf("H%d", start+i+1), x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
		}
		return items
	case uiMusic:
		if s.profile == gothicProfileToku1 {
			items := []gothicUIHit{{action: "music.exit", label: "戻る", x: 520, y: 430, w: 70, h: 36}}
			for i := 0; i < 13; i++ {
				rect := toku1MusicCellRect(i)
				items = append(items, gothicUIHit{action: fmt.Sprintf("music.cell.%d", i), label: fmt.Sprintf("BGM %d", i+1), x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
			}
			return items
		}
		items := []gothicUIHit{{action: "music.exit", label: "EXIT", x: 510, y: 430, w: 110, h: 40}}
		for i := 0; i < 10; i++ {
			rect := musicCellRect(i)
			items = append(items, gothicUIHit{action: fmt.Sprintf("music.cell.%d", i), label: fmt.Sprintf("BGM %d", i+1), x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
		}
		return items
	case uiSlots:
		items := make([]gothicUIHit, 0, s.saveSlotCount+1)
		for i := 0; i < s.saveSlotCount; i++ {
			rect := s.slotCellRect(i)
			items = append(items, gothicUIHit{action: fmt.Sprintf("slots.cell.%d", i), label: fmt.Sprintf("SLOT %d", i+1), x: rect[0], y: rect[1], w: rect[2], h: rect[3]})
		}
		items = append(items, gothicUIHit{action: "slots.exit", label: "EXIT", x: 536, y: 444, w: 90, h: 30})
		return items
	case uiIngame:
		if s.profile == gothicProfileToku1 {
			// 原版右クリック菜单有「次の選択肢まで進む」，且 CONFIG 按钮
			// 常驻在信息框下方。Toku1 用 7 项紧凑菜单。
			labels := []string{"スキップ", "セーブ", "ロード", "オプション", "タイトルへ", "終了", "閉じる"}
			actions := []string{"ingame.skip", "ingame.save", "ingame.load", "ingame.option", "ingame.title", "ingame.quit", "ingame.close"}
			items := make([]gothicUIHit, 0, len(labels))
			for i := range labels {
				items = append(items, gothicUIHit{action: actions[i], label: labels[i], x: 200, y: 128 + i*40, w: 240, h: 34})
			}
			return items
		}
		labels := []string{"セーブ", "ロード", "オプション", "タイトルへ", "終了", "閉じる"}
		actions := []string{"ingame.save", "ingame.load", "ingame.option", "ingame.title", "ingame.quit", "ingame.close"}
		items := make([]gothicUIHit, 0, len(labels))
		for i := range labels {
			items = append(items, gothicUIHit{action: actions[i], label: labels[i], x: 200, y: 160 + i*44, w: 240, h: 38})
		}
		return items
	case uiPreview:
		return []gothicUIHit{{action: "preview.back", label: "戻る", x: 0, y: 0, w: GothicScreenWidth, h: GothicScreenHeight}}
	case uiStaffRoll:
		return []gothicUIHit{{action: "staff.next", label: "次へ", x: 0, y: 0, w: GothicScreenWidth, h: GothicScreenHeight}}
	}
	return nil
}

func (s *Session) SelectItems() []core.SelectItem {
	if s.ui == nil && s.choiceActive {
		out := make([]core.SelectItem, 0, len(s.choiceItems))
		for index := range s.choiceItems {
			out = append(out, core.SelectItem{
				Index:  uint32(index),
				Text:   s.choiceItems[index],
				X:      74,
				Y:      int32(120 + index*25),
				Width:  GothicScreenWidth - 148,
				Height: 25,
			})
		}
		return out
	}
	hits := s.uiItems()
	out := make([]core.SelectItem, 0, len(hits))
	for index, hit := range hits {
		out = append(out, core.SelectItem{
			Index:  uint32(index),
			Text:   hit.label,
			X:      int32(hit.x),
			Y:      int32(hit.y),
			Width:  uint32(hit.w),
			Height: uint32(hit.h),
		})
	}
	return out
}

// choiceIndexAt 把画面坐标换算成选择肢行号。原版选项文字居中，
// 命中区覆盖整行宽度，点击文字附近都能选中。
func (s *Session) choiceIndexAt(x, y int) int {
	if !s.choiceActive || y < 120 {
		return -1
	}
	index := (y - 120) / 25
	if index < 0 || index >= len(s.choiceItems) {
		return -1
	}
	if x < 74 || x > GothicScreenWidth-74 {
		return -1
	}
	return index
}

func (s *Session) uiHitAt(x, y int) *gothicUIHit {
	hits := s.uiItems()
	for i := range hits {
		hit := &hits[i]
		if x >= hit.x && x < hit.x+hit.w && y >= hit.y && y < hit.y+hit.h {
			return hit
		}
	}
	return nil
}

func (s *Session) uiHover(x, y int) core.Step {
	if s.ui == nil {
		return core.StepContinue
	}
	if s.ui.mode == uiTitle {
		hover := -1
		for index, rect := range s.titleRects() {
			if x >= rect[0] && x < rect[0]+rect[2] && y >= rect[1] && y < rect[1]+rect[3] {
				hover = index
				break
			}
		}
		if hover != s.ui.hover {
			s.ui.hover = hover
			s.stage.Invalidate()
		}
		return core.StepAwaitTitle
	}
	hover := -1
	hits := s.uiItems()
	for i := range hits {
		hit := &hits[i]
		if x >= hit.x && x < hit.x+hit.w && y >= hit.y && y < hit.y+hit.h {
			hover = i
			break
		}
	}
	if hover != s.ui.hover {
		s.ui.hover = hover
		s.stage.Invalidate()
	}
	return s.uiStep()
}

func (s *Session) uiClick(x, y int) core.Step {
	hit := s.uiHitAt(x, y)
	if hit == nil {
		return s.uiStep()
	}
	return s.uiAction(hit.action)
}

func (s *Session) uiBack() core.Step {
	if s.ui == nil {
		return core.StepContinue
	}
	if s.ui.mode == uiAlbum && s.ui.pageSelect {
		s.ui.pageSelect = false
		s.ui.hover = -1
		s.stage.Invalidate()
		return s.uiStep()
	}
	switch s.ui.mode {
	case uiPreview:
		s.ui.mode = s.ui.previewReturn
		s.ui.preview = ""
		s.stage.Invalidate()
		return s.uiStep()
	case uiAlbum, uiKaisou, uiMusic:
		s.ui.mode = uiGalleryMenu
		s.ui.page = 0
		s.stage.Invalidate()
		return s.uiStep()
	case uiSlots:
		s.ui.mode = s.ui.slotReturn
		if s.ui.mode == uiNone {
			s.uiClose()
			return s.currentStep()
		}
		if s.ui.mode == uiTitle {
			s.showTitleArt()
			return core.StepAwaitTitle
		}
		s.stage.Invalidate()
		return s.uiStep()
	case uiOption:
		s.ui.mode = s.ui.optionReturn
		if s.ui.mode == uiNone {
			s.uiClose()
			return s.currentStep()
		}
		if s.ui.mode == uiTitle {
			s.showTitleArt()
			return core.StepAwaitTitle
		}
		s.stage.Invalidate()
		return s.uiStep()
	case uiJuice:
		s.ui.mode = uiOption
		s.stage.Invalidate()
		return s.uiStep()
	case uiGalleryMenu:
		s.ui.mode = uiTitle
		s.showTitleArt()
		return core.StepAwaitTitle
	case uiIngame:
		s.uiClose()
		return s.currentStep()
	case uiStaffRoll:
		return s.uiAction("staff.next")
	default:
		s.uiClose()
		return s.currentStep()
	}
}

func (s *Session) uiAction(action string) core.Step {
	switch {
	case action == "gallery.cg":
		s.openAlbum()
		return core.StepAwaitSelect
	case action == "gallery.kaisou":
		s.openKaisou()
		return core.StepAwaitSelect
	case action == "gallery.music":
		s.openMusic()
		return core.StepAwaitSelect
	case action == "gallery.staffroom":
		s.uiClose()
		s.stage.Reset()
		s.clearMessage()
		if s.machine != nil && s.machine.Seek("STAFFROOM") {
			step, _ := s.run(gothicDefaultRunBudget)
			return step
		}
		return s.showTitle()
	case action == "gallery.back":
		s.ui.mode = uiTitle
		s.showTitleArt()
		return core.StepAwaitTitle
	case strings.HasPrefix(action, "album."):
		return s.albumAction(action)
	case strings.HasPrefix(action, "kaisou."):
		return s.kaisouAction(action)
	case strings.HasPrefix(action, "music."):
		return s.musicAction(action)
	case strings.HasPrefix(action, "slots."):
		return s.slotsAction(action)
	case strings.HasPrefix(action, "opt."), strings.HasPrefix(action, "juice."):
		return s.optionAction(action)
	case strings.HasPrefix(action, "ingame."):
		return s.ingameAction(action)
	case action == "preview.back":
		return s.uiBack()
	case action == "staff.next":
		return s.staffNext()
	}
	return s.uiStep()
}

func (s *Session) requestHost(action string) {
	s.hostRequest = action
	s.hostRequestSerial++
}

// openSlots 打开 15 槽存/读档。Toku1 仍走原来的宿主兜底面板，
// 这里只服务 MAZE（saveSlotCount==15）。
func (s *Session) openSlots(save bool, returnTo gothicUIMode) {
	s.uiOpen(uiSlots)
	s.ui.slotSave = save
	s.ui.slotReturn = returnTo
	s.refreshSlotCache()
}

func (s *Session) refreshSlotCache() {
	if s.ui == nil {
		return
	}
	s.ui.slotInfos = make([]GothicSaveSlotInfo, s.saveSlotCount)
	s.ui.slotPreviews = make([]*GothicSavePreview, s.saveSlotCount)
	any := false
	for slot := 0; slot < s.saveSlotCount; slot++ {
		s.ui.slotInfos[slot] = s.saves.Inspect(slot)
		if s.ui.slotInfos[slot].Exists {
			any = true
			s.ui.slotPreviews[slot] = s.saves.PreviewSlot(slot)
		}
	}
	s.ui.titleLoadEnabled = any
}

func (s *Session) refreshTitleLoadState() {
	if s.ui == nil {
		return
	}
	any := false
	for slot := 0; slot < s.saveSlotCount; slot++ {
		if s.saves.Exists(slot) {
			any = true
			break
		}
	}
	s.ui.titleLoadEnabled = any
}

func (s *Session) openOption(returnTo gothicUIMode) {
	s.uiOpen(uiOption)
	s.ui.optionReturn = returnTo
}

func (s *Session) openJuice() {
	s.uiOpen(uiJuice)
}

func (s *Session) openGalleryMenu() {
	s.uiOpen(uiGalleryMenu)
}

func (s *Session) openAlbum() {
	s.uiOpen(uiAlbum)
	s.ui.page = 0
	s.ui.pageSelect = false
}

func (s *Session) openKaisou() {
	s.uiOpen(uiKaisou)
	s.ui.page = 0
}

func (s *Session) openMusic() {
	s.uiOpen(uiMusic)
	s.ui.playingTrack = -1
}

func (s *Session) openIngame() {
	s.uiOpen(uiIngame)
}

// showTitleArt 把标题图铺到舞台。title01 本身带 LOGO 与菜单文字，
// title02 只多出五个蓝色选中条，用来做逐项高亮。
func (s *Session) showTitleArt() {
	s.stage.Reset()
	base := s.catalog.Resolve("title01")
	if base != nil {
		s.stage.LoadBase(4, "title01")
		s.backgroundLayer = 4
		s.background = "title01"
	}
	s.clearMessage()
	s.gameState.SetAwaitingInput(false)
	s.stage.Invalidate()
}

func (s *Session) showTitle() core.Step {
	s.uiOpen(uiTitle)
	s.refreshTitleLoadState()
	s.showTitleArt()
	s.titleMode = true
	core.Logf("gothic: title menu presented")
	return core.StepAwaitTitle
}

func (s *Session) albumAction(action string) core.Step {
	switch action {
	case "album.pages":
		if s.profile == gothicProfileToku1 {
			s.ui.pageSelect = true
			s.ui.hover = -1
			s.stage.Invalidate()
			return s.uiStep()
		}
	case "album.pages.exit":
		s.ui.pageSelect = false
		s.ui.hover = -1
		s.stage.Invalidate()
		return s.uiStep()
	case "album.prev":
		if s.ui.page > 0 {
			s.ui.page--
		}
		s.stage.Invalidate()
	case "album.next":
		if (s.ui.page+1)*s.galleryPerPage() < len(s.cgNames) {
			s.ui.page++
		}
		s.stage.Invalidate()
	case "album.exit":
		s.ui.mode = uiGalleryMenu
		s.stage.Invalidate()
	}
	if strings.HasPrefix(action, "album.goto.") {
		page := parseGothicInt(strings.TrimPrefix(action, "album.goto."))
		if page >= 0 {
			s.ui.page = page
		}
		s.ui.pageSelect = false
		s.ui.hover = -1
		s.stage.Invalidate()
		return s.uiStep()
	}
	if strings.HasPrefix(action, "album.cell.") {
		index := parseGothicInt(strings.TrimPrefix(action, "album.cell."))
		position := s.ui.page*s.galleryPerPage() + index
		if position >= 0 && position < len(s.cgNames) {
			name := s.cgNames[position]
			// Toku1 原版按 USR0.DAT 的解锁位过滤；我们还没有该格式，
			// 用同目录的 global.json 记录“看过”。没有任何记录时退回全显示，
			// 避免首次游玩鉴赏室全黑。MAZE 仍按 prof.dat 的解锁位过滤。
			unlocked := s.global != nil && s.global.CG[name]
			if s.profile == gothicProfileToku1 && s.global != nil && len(s.global.CG) == 0 {
				unlocked = true
			}
			if unlocked {
				s.ui.previewReturn = uiAlbum
				s.ui.preview = name
				s.ui.mode = uiPreview
				s.stage.Invalidate()
			}
		}
	}
	return s.uiStep()
}

func (s *Session) kaisouAction(action string) core.Step {
	switch action {
	case "kaisou.prev":
		if s.ui.page > 0 {
			s.ui.page--
		}
		s.stage.Invalidate()
	case "kaisou.next":
		if s.profile == gothicProfileToku1 {
			if (s.ui.page+1)*toku1GalleryPerPage < len(s.toku1SceneLabels()) {
				s.ui.page++
			}
		} else if (s.ui.page+1)*12 < 21 {
			s.ui.page++
		}
		s.stage.Invalidate()
	case "kaisou.exit":
		s.ui.mode = uiGalleryMenu
		s.stage.Invalidate()
	}
	if strings.HasPrefix(action, "kaisou.cell.") {
		index := parseGothicInt(strings.TrimPrefix(action, "kaisou.cell."))
		if s.profile == gothicProfileToku1 {
			labels := s.toku1SceneLabels()
			position := s.ui.page*toku1GalleryPerPage + index
			if position >= 0 && position < len(labels) {
				if step, ok := s.toku1ReplayScene(labels[position]); ok {
					return step
				}
			}
			return s.uiStep()
		}
		scene := s.ui.page*12 + index + 1
		if scene >= 1 && scene <= 21 && s.hSceneUnlocked(scene) {
			// 回想是脚本标签回放：保存 VM/变量快照后 Call 到 HnSCENE。
			// H 场景播完的 #return 会把调用深度降回基线，run 检测到后
			// 调用 finishReplay 恢复快照并退回网格，避免继续跑主线。
			if s.machine != nil {
				// 快照必须在 Call 之前抓：Call 会把返回地址压进调用栈，
				// 抓晚了恢复出来的 PC 会停在 H 标签上而不是主线。
				s.replaySnapshot = s.machine.Snapshot()
				s.replayVars = s.machine.Variables().Snapshot()
				s.replayDepth = s.machine.CallDepth()
			}
			if s.machine != nil && s.machine.Call(fmt.Sprintf("H%dSCENE", scene)) {
				s.replayMode = true
				s.replayPage = s.ui.page
				s.ui = nil
				s.stage.Reset()
				s.gameState.Reset()
				s.clearMessage()
				step, _ := s.run(gothicDefaultRunBudget)
				return step
			}
		}
	}
	return s.uiStep()
}

func (s *Session) musicAction(action string) core.Step {
	switch action {
	case "music.exit":
		s.gameState.ClearBgm()
		s.bgmName = ""
		s.ui.mode = uiGalleryMenu
		s.stage.Invalidate()
		return s.uiStep()
	}
	if strings.HasPrefix(action, "music.cell.") {
		index := parseGothicInt(strings.TrimPrefix(action, "music.cell."))
		count := s.musicCellCount()
		if index >= 0 && index < count {
			path := ""
			if s.profile == gothicProfileToku1 {
				path = s.audioCatalog.ResolveToku1Track(index + 2)
			} else {
				path = s.audioCatalog.ResolveTrack(index + 2)
			}
			if path != "" {
				s.gameState.SetBgm(path)
				s.gameState.SetBgmLoop(true)
				s.bgmName = fmt.Sprintf("track%02d", index+2)
				s.ui.playingTrack = index
				s.stage.Invalidate()
			}
		}
	}
	return s.uiStep()
}

func (s *Session) slotsAction(action string) core.Step {
	if action == "slots.exit" {
		return s.uiBack()
	}
	if strings.HasPrefix(action, "slots.cell.") {
		index := parseGothicInt(strings.TrimPrefix(action, "slots.cell."))
		if index < 0 || index >= s.saveSlotCount {
			return s.uiStep()
		}
		if s.ui.slotSave {
			if err := s.Save(uint32(index), ""); err != nil {
				core.Logf("gothic: slot %d save failed: %v", index, err)
			}
			s.flushGlobal()
			s.refreshSlotCache()
			returnTo := s.ui.slotReturn
			s.uiClose()
			if returnTo == uiTitle {
				s.showTitle()
				return core.StepAwaitTitle
			}
			return s.currentStep()
		}
		if s.saves.Load(index) == nil {
			return s.uiStep()
		}
		s.uiClose()
		step, err := s.Load(uint32(index))
		if err != nil {
			core.Logf("gothic: slot %d load failed: %v", index, err)
			return core.StepFaulted
		}
		s.stage.Invalidate()
		return step
	}
	return s.uiStep()
}

func (s *Session) ingameAction(action string) core.Step {
	switch action {
	case "ingame.skip":
		s.uiClose()
		return s.Skip()
	case "ingame.save":
		if s.choiceActive {
			return s.uiStep()
		}
		s.openSlots(true, uiIngame)
		return core.StepAwaitSelect
	case "ingame.load":
		s.openSlots(false, uiIngame)
		return core.StepAwaitSelect
	case "ingame.option":
		s.openOption(uiIngame)
		return core.StepAwaitSelect
	case "ingame.title":
		s.uiClose()
		return s.returnToTitle()
	case "ingame.quit":
		s.requestHost("exit")
		return s.uiStep()
	case "ingame.close":
		s.uiClose()
		return s.currentStep()
	}
	return s.uiStep()
}

// returnToTitle 回到脚本 boot 状态。MAZE 的 boot 会先 #logo 再 #startmenu，
// 所以这里等价于原版菜单的「タイトルへ戻る」。
func (s *Session) returnToTitle() core.Step {
	s.clearMessage()
	s.stage.Reset()
	s.gameState.Reset()
	s.gameState.ClearBgm()
	s.gameState.ClearSoundEffect()
	s.background = ""
	s.pendingMusic = ""
	s.hasPendingMusic = false
	if s.program != nil {
		s.machine = vmgothic.NewGothicMachine(s.program)
	}
	s.clearTransientState()
	s.uiClose()
	// 回标题后「初めから」要重新跑 boot；抑制 logo/startmenu 二次停顿。
	s.skipStartMenu = true
	s.suppressLogo = true
	return s.showTitle()
}

func (s *Session) staffNext() core.Step {
	if s.ui == nil || s.ui.mode != uiStaffRoll {
		return core.StepContinue
	}
	s.ui.staffIndex++
	if s.ui.staffIndex >= 8 {
		s.uiClose()
		return core.StepContinue
	}
	s.stage.Invalidate()
	return core.StepAwaitInput
}

// optionItems 的坐标按 option0.gsa 的按钮位置给出。
// 原图分辨率就是 640x480，不需要按素材比例换算。
func (s *Session) optionItems() []gothicUIHit {
	if s.profile == gothicProfileToku1 {
		return s.toku1OptionItems()
	}
	return []gothicUIHit{
		{action: "opt.name", label: "名前変更", x: 50, y: 80, w: 200, h: 36},
		{action: "opt.name", label: "姓", x: 245, y: 80, w: 180, h: 36},
		{action: "opt.name", label: "名", x: 430, y: 80, w: 180, h: 36},
		{action: "opt.res.full", label: "FULL", x: 245, y: 124, w: 180, h: 36},
		{action: "opt.res.window", label: "WINDOW", x: 430, y: 124, w: 180, h: 36},
		{action: "opt.effect.on", label: "ON", x: 245, y: 170, w: 180, h: 36},
		{action: "opt.effect.off", label: "OFF", x: 430, y: 170, w: 180, h: 36},
		{action: "opt.text.0", label: "ページ単位", x: 245, y: 218, w: 125, h: 36},
		{action: "opt.text.1", label: "高速", x: 375, y: 218, w: 80, h: 36},
		{action: "opt.text.2", label: "標準", x: 460, y: 218, w: 80, h: 36},
		{action: "opt.text.3", label: "低速", x: 545, y: 218, w: 75, h: 36},
		{action: "opt.bg.0", label: "黒", x: 245, y: 266, w: 80, h: 36},
		{action: "opt.bg.1", label: "赤", x: 330, y: 266, w: 60, h: 36},
		{action: "opt.bg.2", label: "青", x: 395, y: 266, w: 60, h: 36},
		{action: "opt.bg.3", label: "セピア", x: 460, y: 266, w: 140, h: 36},
		{action: "opt.bgm.on", label: "BGM ON", x: 245, y: 316, w: 180, h: 36},
		{action: "opt.bgm.off", label: "BGM OFF", x: 430, y: 316, w: 180, h: 36},
		{action: "opt.se.on", label: "SE ON", x: 245, y: 358, w: 180, h: 36},
		{action: "opt.se.off", label: "SE OFF", x: 430, y: 358, w: 180, h: 36},
		{action: "opt.juice", label: "多汁モード設定", x: 50, y: 424, w: 230, h: 40},
		{action: "opt.shiru", label: "汁コンフィグ", x: 245, y: 424, w: 190, h: 40},
		{action: "opt.exit", label: "EXIT", x: 515, y: 424, w: 110, h: 40},
	}
}

func (s *Session) juiceItems() []gothicUIHit {
	items := make([]gothicUIHit, 0, 11)
	xs := []int{222, 370, 518}
	ys := []int{163, 245, 327}
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			items = append(items, gothicUIHit{
				action: fmt.Sprintf("juice.%d.%d", row, col),
				label:  fmt.Sprintf("%d", col),
				x:      xs[col] - 26, y: ys[row] - 26, w: 52, h: 52,
			})
		}
	}
	items = append(items,
		gothicUIHit{action: "juice.ok", label: "OK", x: 345, y: 418, w: 130, h: 40},
		gothicUIHit{action: "juice.cancel", label: "キャンセル", x: 490, y: 418, w: 130, h: 40},
	)
	return items
}

func (s *Session) optionAction(action string) core.Step {
	if s.global == nil {
		return s.uiStep()
	}
	switch {
	case action == "opt.name":
		s.nameForOption = true
		s.awaitingName = true
		s.nameWaitIssued = true
		return core.StepAwaitName
	case action == "opt.res.full", action == "opt.res.window":
		// Android 上没有窗口/全屏切换的语义，保留设置值即可。
	case action == "opt.effect.on":
		s.global.Effects = true
		s.globalDirty = true
	case action == "opt.effect.off":
		s.global.Effects = false
		s.globalDirty = true
	case strings.HasPrefix(action, "opt.text."):
		s.global.TextSpeed = parseGothicInt(strings.TrimPrefix(action, "opt.text."))
		s.globalDirty = true
	case strings.HasPrefix(action, "opt.bg."):
		s.global.Background = parseGothicInt(strings.TrimPrefix(action, "opt.bg."))
		s.globalDirty = true
	case action == "opt.bgm.on":
		s.global.BgmOn = true
		s.globalDirty = true
	case action == "opt.bgm.off":
		s.global.BgmOn = false
		s.gameState.ClearBgm()
		s.globalDirty = true
	case action == "opt.se.on":
		s.global.SeOn = true
		s.globalDirty = true
	case action == "opt.se.off":
		s.global.SeOn = false
		s.globalDirty = true
	case action == "opt.voice.on":
		s.global.VoiceOn = true
		s.globalDirty = true
	case action == "opt.voice.off":
		s.global.VoiceOn = false
		s.globalDirty = true
	case action == "opt.skip.on":
		s.global.SkipRead = true
		s.globalDirty = true
	case action == "opt.skip.off":
		s.global.SkipRead = false
		s.globalDirty = true
	case action == "opt.quit":
		s.requestHost("exit")
		return s.uiStep()
	case action == "opt.juice", action == "opt.shiru":
		s.openJuice()
		return core.StepAwaitSelect
	case action == "opt.exit":
		s.flushGlobal()
		return s.uiBack()
	case strings.HasPrefix(action, "juice."):
		rest := strings.TrimPrefix(action, "juice.")
		switch rest {
		case "ok":
			s.flushGlobal()
			s.ui.mode = uiOption
			s.stage.Invalidate()
		case "cancel":
			s.ui.mode = uiOption
			s.stage.Invalidate()
		default:
			parts := strings.Split(rest, ".")
			if len(parts) == 2 {
				row := parseGothicInt(parts[0])
				col := parseGothicInt(parts[1])
				if row >= 0 && row < 3 && col >= 0 && col < 3 {
					s.global.Siru[row] = col
					s.globalDirty = true
					s.stage.Invalidate()
				}
			}
		}
	}
	return s.uiStep()
}

func (s *Session) hSceneUnlocked(index int) bool {
	if s.global == nil {
		return false
	}
	return s.global.H[hSceneKey(index)]
}

func (s *Session) hSceneImage(index int) string {
	if s.hImages == nil {
		s.hImages = map[int]string{}
	}
	if name, ok := s.hImages[index]; ok {
		return name
	}
	name := "gb01"
	if s.program != nil {
		if found := s.program.FindScenePreviewImage(fmt.Sprintf("H%dSCENE", index)); found != "" {
			name = found
		}
	}
	s.hImages[index] = name
	return name
}

// albumCellRect 返回 4x3 网格中第 index 格（0..11）的矩形。
// 数值来自 album.gsa/kaisou1.gsa 的格子边框：首格 (20,76)，
// 列距 153，行距 123，格子 126x99（内框约 118x91）。
func albumCellRect(index int) [4]int {
	column := index % 4
	row := index / 4
	x := 20 + column*153
	y := 76 + row*123
	return [4]int{x, y, 126, 99}
}

func musicCellRect(index int) [4]int {
	column := index % 2
	row := index / 2
	x := 80 + column*250
	y := 95 + row*68
	return [4]int{x, y, 230, 56}
}

func slotCellRect(index int) [4]int {
	column := index % 3
	row := index / 3
	x := 14 + column*206
	y := 66 + row*76
	return [4]int{x, y, 196, 70}
}

// syncGlobalFlags 把剧本里的 h1..h21 变量同步到跨周目 H 场景解锁。
// MAZE 的主线通过 `#hN = 1` 标记该 H 场景已看；只在变量从 0 变正时
// 写一次，避免每帧都碰磁盘。
func (s *Session) syncGlobalFlags() {
	if s.profile != gothicProfileMaze || s.global == nil || s.machine == nil {
		return
	}
	for index := 1; index <= 21; index++ {
		name := fmt.Sprintf("h%d", index)
		if s.machine.Variables().Get(name) > 0 {
			s.markHScene(index)
		}
	}
}

// galleryFileFor 把鉴赏目录名变成图片文件名。prof.dat 里的名字本身
// 就是 stem，个别条目带 _f 后缀（差分图），直接拼 .gsa 即可。
func galleryFileFor(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".gsa") {
		return name
	}
	return name + ".gsa"
}

func (s *Session) galleryImage(name string) *gfx.BsgImage {
	if s.catalog == nil {
		return nil
	}
	return s.catalog.Resolve(galleryFileFor(name))
}

// finishReplay 把 VM 恢复到进入回想前的状态，并重建回想网格。
// 这里必须恢复变量而不仅是 PC：H 场景里的 refvar/gosub 会改写 var0..9，
// 直接回到主线会让后续选择肢读到剧透过的变量。
func (s *Session) finishReplay() {
	if s.machine != nil {
		s.machine.Restore(s.replaySnapshot)
		s.machine.Variables().Restore(s.replayVars)
	}
	s.replayMode = false
	s.clearMessage()
	s.stage.Reset()
	s.gameState.Reset()
	s.ui = &gothicUI{mode: uiKaisou, page: s.replayPage, playingTrack: -1}
	s.stage.Invalidate()
}
