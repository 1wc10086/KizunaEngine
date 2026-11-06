package session

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
)

// systemui_dress.go 是"立ち絵 / 脱衣"设定画面的渲染与交互。
//
// 原引擎的 [dress screen] 是一张自绘界面：底图 strip_base / strip_base2，
// 左侧是模式 / 角色 / 通用脱衣状态按钮，右侧一列是当前角色的服装列表，
// 再往下是各脱衣阶段的立绘预览（levelN），屏幕中间显示角色立绘。
// 素材名、帧数和坐标全部来自 bsxx.cfg，这里按配置贴图并做命中判定，
// 和存档画面、鉴赏室走同一套"配置驱动"的规矩。
//
// 画面本身不写回游戏状态：原引擎也不把换装结果存进存档，
// 所以退出这个界面后剧情里的立绘还是原来的样子。

const (
	// dressWholeBodyHeight 是全身立绘的目标高度。
	// 立绘原图有 2350x2990，缩到比屏幕略小一点，免得压住上下边框。
	dressWholeBodyHeight = 748

	// 按钮图的帧约定（模式/服装/阶段按钮）：第 0 帧是白色的命中掩码
	// （只用来判命中，永远不画），第 1 帧是常态，第 2 帧是悬停，
	// 第 3 帧是选中。角色卡另有一套帧序，见 dressTargetNormalFrame。
	// 服装图（costume_*）还有第 5 帧，表示"未确定/锁定"，
	// 这一屏不需要它（选中即刻生效），所以服装也只用 1/2/3 三帧。
	dressFrameNormal   = 1
	dressFrameHover    = 2
	dressFrameSelected = 3

	// dressTargetNormalFrame 是 UKIE 式角色卡的常态帧。
	//
	// UKIE 的 strip_ch0 是「ヒロイン選択」标题 + 1 号卡片的合成图：
	// frame1 是雪花/噪点过渡帧，frame2 才是常态头像，frame3 是选中红框。
	// 原版 DRESS 的角色卡静止时显示头像而不是雪花（见游戏手册截图），
	// 所以这张素材不能复用通用的 dressFrame（它把第 1 帧当常态）。
	dressTargetNormalFrame = 2

	dressModeSingle = 0 // 一角色ずつ：只改当前选中的角色。
	dressModeAll    = 1 // 全体一括設定：一次改动套用到所有角色。
	dressModeView   = 2 // 視点設定：自由视角，显示滑杆与脸/胸/股间按钮。

	// 视角滑杆的取值范围（0…1000）以及三个预置位置。
	dressViewRange = 1000
	dressViewFace  = 0
	dressViewBust  = 350
	dressViewHip   = 700

	// dressWheelStep 是滚轮一次滚过几条服装。
	dressWheelStep = 3

	// dressStateTitleHeight 是「脱衣選択」面板标题占的高度。
	// 面板图（common state0）把标题和第一行"着衣"烘焙在同一张图里，
	// 标题下方才是可点的第一行；点标题不该选中阶段。
	dressStateTitleHeight = 55
)

// dressState 是脱衣画面自己的状态。
type dressState struct {
	// character 是角色下标（对应 target0… 的顺序，0 基）。
	character int
	// mode 是模式按钮下标（mode0…2）。
	mode int
	// showTargets 表示当前停在"角色选择"页。
	//
	// 教育指導 KS 的個別設定是两步：先在 dress_base 的左面板点角色卡，
	// 选完才显示服装/阶段列表（dress_base 的右面板）。旧实现同时画角色卡
	// 和服装列表，两块 UI 在同一块面板上重叠。
	showTargets bool
	// fromMuseum 表示这个画面是从鉴赏室进来的：返回时回鉴赏室而不是关掉界面。
	fromMuseum bool
	// scroll 是服装列表的绝对滚动偏移（列表里第一条可见条目的下标）。
	scroll int
	// viewPos 是自由视角的纵向位置（0…1000），0 是画面最上。
	viewPos int
	// perCharacter 是每个角色各自的服装与脱衣阶段。
	perCharacter []dressPerCharacter
}

// dressPerCharacter 是单个角色当前的服装与脱衣阶段。
//
// "全体一括設定"要把一次改动套用到所有角色，所以不能只记一份当前值：
// 每个角色都要记住自己被套用成了什么样子，切回那个角色时才显示得对。
type dressPerCharacter struct {
	// costume 是该角色服装列表里的绝对下标。
	costume int
	// strip 是该角色当前的脱衣阶段（也就是 common state 的下标）。
	strip int
	// set 表示玩家在本局 DRESS 里明确改过这个角色。
	//
	// 只有 set 的角色才会同步到正常剧情的立绘：否则打开画面时给所有
	// 角色补的默认值会把没被编辑的角色也换掉（個別設定只改当前角色）。
	set bool
}

// openDressScreen 打开立ち絵 / 脱衣设定画面。
//
// 从鉴赏室的"立ち絵"页签进来时，鉴赏室的界面状态要留着（返回时回到鉴赏室）；
// 从游戏内的工具条进来则新开一个系统界面。返回 false 表示当前状态不允许打开
// （地图、选择肢、影片等），调用方要另想办法。
func (s *Session) openDressScreen() bool {
	if s.config == nil || s.config.Dress.Empty() {
		return false
	}
	// 牝教師2 老版没有配置驱动的 [dress screen] 素材，走专用自绘页。
	if s.config.Dress.MK2 {
		return s.openMK2DressScreen()
	}
	if s.config.Dress.Atlas && s.md2DressStage < 0 {
		// 原版首次进入默认"着衣"（状态 0）；只设置高亮，不重写已画好的立绘。
		s.md2DressStage = 0
	}
	if s.ui == nil {
		if !s.canOpenSystemUI() {
			return false
		}
		s.beginSystemUI(uiDress)
		s.ui.dress = &dressState{}
		s.dressReset()
		return true
	}

	if s.ui.dress == nil {
		s.ui.dress = &dressState{}
		s.dressReset()
	}
	// 从鉴赏室进来的：返回按钮要回鉴赏室而不是关掉整个界面。
	if s.ui.kind == uiMuseum {
		s.ui.dress.fromMuseum = true
	}
	s.ui.kind = uiDress
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.dressInvalidate()
	return true
}

// dressReset 把脱衣画面恢复到初始状态。
//
// 初始状态按原引擎的约定：1 号角色、模式 0（一角色ずつ）、
// 阶段取鉴赏室配置的初始阶段（init ng level），服装列表选中第一条。
func (s *Session) dressReset() {
	dress := s.ui.dress
	if dress == nil {
		return
	}
	dress.character = 0
	dress.mode = dressModeSingle
	dress.showTargets = true
	dress.scroll = 0
	dress.viewPos = 0
	dress.perCharacter = nil
	for index := range s.config.Dress.Targets {
		entry := s.dressEntry(index)
		entry.costume = s.dressInitialCostume(index + 1)
		// 初始阶段还要看这件衣服有没有那一级的素材：有就照用，
		// 没有就退到最接近的一级，免得一进来立绘是空的。
		entry.strip = s.dressClampStage(index+1, s.dressWearForCharacter(index), s.dressInitialStage())
		entry.set = false
		// 已经在本局里设置过 DRESS 的角色，重新打开时保持上次的选择。
		if choice, ok := s.dressLive[index+1]; ok {
			list := s.dressCostumeListFor(index + 1)
			entry.costume = dressCostumeIndexForWear(list, choice.wear)
			entry.strip = s.dressClampStage(index+1, s.dressWearForCharacter(index), choice.stage)
			entry.set = true
		}
	}
}

// dressInvalidate 让画面重画。
//
// 换装只是预览，所有状态都只存在这一屏里，所以每次改动都要显式让合成器重画一帧。
func (s *Session) dressInvalidate() {
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// ---------------------------------------------------------------- 状态存取

// dressEntry 返回某个角色当前的服装/阶段，必要时先补上默认值。
func (s *Session) dressEntry(index int) *dressPerCharacter {
	if s.ui == nil {
		return &dressPerCharacter{}
	}
	dress := s.ui.dress
	if dress == nil {
		return &dressPerCharacter{}
	}
	if index < 0 {
		index = 0
	}
	for len(dress.perCharacter) <= index {
		next := len(dress.perCharacter)
		dress.perCharacter = append(dress.perCharacter, dressPerCharacter{
			costume: s.dressInitialCostume(next + 1),
			strip:   s.dressInitialStage(),
		})
	}
	return &dress.perCharacter[index]
}

// dressCurrentStage 是当前角色的脱衣阶段。
func (s *Session) dressCurrentStage() int {
	if s.ui == nil || s.ui.dress == nil {
		return 0
	}
	return s.dressEntry(s.ui.dress.character).strip
}

// dressInitialStage 是打开画面时的初始脱衣阶段。
//
// 鉴赏室配置里的 init ng level 决定了"立ち絵"一开始显示到第几阶段；
// 超出范围的阶段没有素材，按 0（着衣）处理。
func (s *Session) dressInitialStage() int {
	layout := s.config.Dress
	stage := layout.InitNgLevel
	if stage < 0 {
		stage = 0
	}
	if stage >= config.DressStageCount {
		stage = config.DressStageCount - 1
	}
	if layout.MaxStrip > 0 && stage >= layout.MaxStrip {
		stage = layout.MaxStrip - 1
	}
	return stage
}

// dressCharacterNumber 返回当前角色的编号（1 基，与 bu<NN> 里的 NN 一致）。
//
// 角色按钮 target0 是 1 号角色，而配置里的 chNN 表也用 1 基编号，两边差一。
func (s *Session) dressCharacterNumber() int {
	if s.ui == nil || s.ui.dress == nil {
		return 0
	}
	targets := s.config.Dress.Targets
	if len(targets) == 0 {
		return 0
	}
	index := clampInt(s.ui.dress.character, 0, len(targets)-1)
	return index + 1
}

// ---------------------------------------------------------------- 服装列表

// dressCostumeListFor 返回某个角色的服装条目。
//
// 配置里每个角色的 chNN 表按书写顺序排列，"全部服装"（wear 0）通常写在
// 最后一条；原版 DRESS 的列表把它放在最上面（手册里第一条就是
// 「全服装一括」）。这里用稳定分区把它提到最前，其余顺序保持不变。
// 已经在配置里把 wear0 写在前面的作品顺序不变。
func (s *Session) dressCostumeListFor(character int) []config.DressCostume {
	layout := s.config.Dress
	list := make([]config.DressCostume, 0, len(layout.DressCostumes))
	// 先收 wear 0，再收其余，保证"全服装一括"永远排第一。
	for _, entry := range layout.DressCostumes {
		if entry.Character == character && entry.Wear == 0 {
			list = append(list, entry)
		}
	}
	for _, entry := range layout.DressCostumes {
		if entry.Character == character && entry.Wear != 0 {
			list = append(list, entry)
		}
	}
	return list
}

// dressInitialCostume 是打开画面时该角色默认选中的服装。
//
// 原版默认选第一条具体服装（制服），不是「全服装一括」；列表重排后
// wear0 在 0 号位，所以取第一个 Wear != 0 的条目。只有全服装条目时取 0。
func (s *Session) dressInitialCostume(character int) int {
	return dressDefaultCostumeIndex(s.dressCostumeListFor(character))
}

// dressDefaultCostumeIndex 返回列表里第一条具体服装的下标。
func dressDefaultCostumeIndex(list []config.DressCostume) int {
	for index, entry := range list {
		if entry.Wear != 0 {
			return index
		}
	}
	return 0
}

// dressCostumeList 是当前角色的服装列表。
func (s *Session) dressCostumeList() []config.DressCostume {
	return s.dressCostumeListFor(s.dressCharacterNumber())
}

// dressCostumeButton 取一条服装对应的按钮图。
//
// 阶段表里的 DressImage 是服装按钮（dress00…）的 0 基下标，
// 越界说明配置写坏了，这时跳过这一条而不是画别的图。
func (s *Session) dressCostumeButton(entry config.DressCostume) (config.UIButton, bool) {
	buttons := s.config.Dress.Costumes
	if entry.DressImage < 0 || entry.DressImage >= len(buttons) {
		return config.UIButton{}, false
	}
	return buttons[entry.DressImage], true
}

// dressWearForCharacter 是某个角色当前服装的编号（1 基；0 是"全部服装"）。
func (s *Session) dressWearForCharacter(index int) int {
	list := s.dressCostumeListFor(index + 1)
	entry := s.dressEntry(index)
	if entry.costume < 0 || entry.costume >= len(list) {
		return 0
	}
	return list[entry.costume].Wear
}

// dressWearNumber 是当前角色当前服装的编号。
func (s *Session) dressWearNumber() int {
	if s.ui == nil || s.ui.dress == nil {
		return 0
	}
	return s.dressWearForCharacter(s.ui.dress.character)
}

// dressVisibleRows 是服装列表能同时显示多少条。
//
// 列表的下边界不是固定的：侵蝕 / 淫獄の放課後2 把服装列表和右侧的
// 阶段预览列排在同一列（dress pos 与 levelN 的 X 相同），列表必须停在
// 阶段预览的第一行，否则两列会叠在一起；SL / SLIE 则把阶段预览放在
// 另一列（SLIE: 列表 x=266、预览 x=897），列表可以一直排到画面下沿。
// 旧实现无条件拿"最上面的阶段预览 Y"当底边：这两作的预览起点正好与
// 列表起点同高，算出 0 行，max(1) 后列表只剩一行，玩家几乎看不到服装。
func (s *Session) dressVisibleRows() int {
	layout := s.config.Dress
	top := 0
	topX := 0
	found := false
	for _, level := range layout.Levels {
		if !found || level.Button.Y < top {
			top = level.Button.Y
			topX = level.Button.X
			found = true
		}
	}
	if !found {
		// 没有阶段预览时按配置的默认值给一屏。
		return config.DressCostumeLimit
	}
	space := layout.DressSpace.Y
	if space <= 0 {
		space = 1
	}
	bottom := s.height
	if dressAbs(topX-layout.DressPos.X) <= 8 {
		// 同一列：列表不能压到阶段预览上。
		bottom = top
	} else if bottom <= 0 {
		// 没有屏幕高度（测试或探测失败）时按"一屏默认条数"兜底。
		bottom = layout.DressPos.Y + config.DressCostumeLimit*space
	}
	if bottom <= layout.DressPos.Y {
		return 1
	}
	return max(1, (bottom-layout.DressPos.Y)/space)
}

// dressClampScroll 保证滚动位置合法，并且选中的那一行在可见范围内。
func (s *Session) dressClampScroll() {
	dress := s.ui.dress
	if dress == nil {
		return
	}
	list := s.dressCostumeList()
	visible := s.dressVisibleRows()
	maxScroll := max(0, len(list)-visible)
	dress.scroll = clampInt(dress.scroll, 0, maxScroll)
	selected := s.dressEntry(dress.character).costume
	if selected < dress.scroll {
		dress.scroll = selected
	}
	if selected >= dress.scroll+visible {
		dress.scroll = selected - visible + 1
	}
	dress.scroll = clampInt(dress.scroll, 0, maxScroll)
}

// ---------------------------------------------------------------- 阶段换算

// dressStageFromState 把通用状态按钮的下标换算成脱衣阶段。
//
// common state0…6 的下标本身就是阶段号；配置里的 ngng convert table
// 是阶段 → 档位的换算表，没写时是恒等映射，所以这里只在下标落在表内时查表，
// 表外的下标按原值处理（越界的阶段本来就没有素材）。
func (s *Session) dressStageFromState(index int) int {
	layout := s.config.Dress
	if index < 0 || index >= len(layout.States) {
		return index
	}
	if index < len(layout.StripConvert) {
		mapped := layout.StripConvert[index]
		if mapped >= 0 && mapped < config.DressStageCount {
			return mapped
		}
	}
	return index
}

// dressClampStage 把阶段收敛到这件服装真正有素材的阶段上。
//
// 换衣服之后原来那一级可能没有对应素材（每件衣服的脱衣阶段表都不一样），
// 这时要退到最接近的一级；一样近时取靠前的那一级（0 就是"着衣"，是永远存在的底线），
// 一级都没有时退回 0。
func (s *Session) dressClampStage(character, wear, stage int) int {
	layout := s.config.Dress
	if layout.StageAvailable(character, wear, stage) {
		return stage
	}
	best := -1
	for candidate := 0; candidate < config.DressStageCount; candidate++ {
		if !layout.StageAvailable(character, wear, candidate) {
			continue
		}
		if best < 0 || dressAbs(candidate-stage) < dressAbs(best-stage) {
			best = candidate
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

// dressAbs 是整数的绝对值（没有内置的整数版 abs）。
func dressAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// ---------------------------------------------------------------- 绘制

// drawDressScreen 画整个脱衣设定画面。
func (s *Session) drawDressScreen(canvas *gfx.BgraCanvas) {
	if s.ui == nil {
		return
	}
	if s.ui.mk2Dress != nil {
		s.drawMK2DressScreen(canvas)
		return
	}
	layout := s.config.Dress
	if layout.Atlas {
		s.drawMD2DressScreen(canvas, layout)
		return
	}
	mode := dressModeSingle
	if s.ui != nil && s.ui.dress != nil {
		mode = s.ui.dress.mode
	}
	// KS 的 dress_base / dress_base2 是"個別設定 / 全体設定"两张完整页面，
	// 不能像其它作品那样叠在一起：两张都画会出现两套重叠面板。base2 的
	// 标题「ぬぎぬぎモード」就是全体設定页。
	//
	// 其它作品（SL/UKIE/侵蝕）仍是两层：base 整屏背景、base2 界面框/按钮
	// 底板；立绘夹在两层之间，让右侧半透明阴影压在人物上。
	showTargets := layout.AlternateBase2 && mode == dressModeSingle && s.ui.dress.showTargets
	if layout.AlternateBase2 {
		if mode == dressModeAll && len(layout.Base2) > 0 {
			s.drawImageAt(canvas, layout.Base2, 0, 0)
		} else if len(layout.Base) > 0 {
			s.drawImageAt(canvas, layout.Base, 0, 0)
		} else {
			s.drawImageAt(canvas, layout.Base2, 0, 0)
		}
		// 角色选择页只画卡片；选定角色后才把立绘、服装和阶段铺到面板上。
		if !showTargets {
			s.drawDressBody(canvas)
		}
	} else if s.dressBase2IsBackground(layout.Base2) {
		// 牝教師2 世代把 base2 做成"整屏不透明背景"、base 才是透明装饰层：
		// 按通用的 base→立绘→base2 顺序画，黑色 base2 会把立绘整块盖掉，
		// DRESS 画面只剩底图按钮。这里先铺不透明的 base2，再画立绘，
		// 最后叠透明的 base（STRIP 标题与右侧面板）。
		s.drawImageAt(canvas, layout.Base2, 0, 0)
		s.drawDressBody(canvas)
		s.drawImageAt(canvas, layout.Base, 0, 0)
	} else {
		s.drawImageAt(canvas, layout.Base, 0, 0)
		s.drawDressBody(canvas)
		s.drawImageAt(canvas, layout.Base2, 0, 0)
	}
	s.drawDressModes(canvas)
	if showTargets {
		s.drawDressTargets(canvas)
		s.drawFrameButton(canvas, layout.Back, s.dressFrame(hitDressBack, -1, -1))
		return
	}
	s.drawDressStates(canvas)
	// 視点設定只调镜头：衣装/阶段列表会和右侧的视角按钮叠在一起，暂时收起。
	viewMode := s.ui != nil && s.ui.dress != nil && s.ui.dress.mode == dressModeView
	if !viewMode {
		s.drawDressCostumes(canvas)
		s.drawDressLevels(canvas)
	}
	s.drawDressView(canvas)
	// 其它作品仍是"角色卡常驻"的布局；KS 的個別/全体页不画角色卡。
	if !layout.AlternateBase2 {
		s.drawDressTargets(canvas)
	}
	s.drawFrameButton(canvas, layout.Back, s.dressFrame(hitDressBack, -1, -1))
}

// dressBase2IsBackground 判断 base2 是不是"整屏不透明背景"。
//
// 只看画面中心像素的 alpha：真正的界面框/按钮底板在中心是透明的
// （立绘要能透出来），而牝教師2 的 strip_base2 中心是不透明黑。
// 读的是已解码的缓存图，判断本身是 O(1)。
func (s *Session) dressBase2IsBackground(name string) bool {
	if len(name) == 0 || s.composer == nil {
		return false
	}
	image := s.composer.Load(name)
	if image == nil || image.Width <= 0 || image.Height <= 0 {
		return false
	}
	offset := ((image.Height/2)*image.Width + image.Width/2) * 4
	if offset+3 >= len(image.Pixels) {
		return false
	}
	return image.Pixels[offset+3] >= 250
}

// dressFrame 决定按钮画第几帧。
//
// 悬停优先于选中：鼠标正指着的那一个总要给出反馈。
func (s *Session) dressFrame(kind uiHitKind, index, selected int) int {
	ui := s.ui
	if ui != nil && ui.hover.kind == kind && ui.hover.index == index {
		return dressFrameHover
	}
	if selected >= 0 && selected == index {
		return dressFrameSelected
	}
	return dressFrameNormal
}

// drawDressModes 画模式按钮（一人 / 全体 / 視点）。
func (s *Session) drawDressModes(canvas *gfx.BgraCanvas) {
	layout := s.config.Dress
	mode := 0
	if s.ui != nil && s.ui.dress != nil {
		mode = s.ui.dress.mode
	}
	for index, button := range layout.Modes {
		s.drawFrameButton(canvas, button, s.dressFrame(hitDressMode, index, mode))
	}
}

// drawDressTargets 画左侧的「ヒロイン選択」列表。
//
// 这一栏只在個別設定/視点設定显示；全体一括設定时同一块位置换成
// 「脱衣選択」面板（见 drawDressStates）。
//
// target0 在 UKIE 是「ヒロイン選択」标题 + 1 号角色卡片的合成图，
// 在侵蝕 / 淫獄 等作品里是「标题 + 第一行名字」的竖排条；target1..4
// 是其余角色。它们都按配置坐标直立摆放，互不重叠，直接用配置坐标即可
// ——不要重新排版，也不要把标题当成"打开浮层"的按钮。
func (s *Session) drawDressTargets(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	if ui.dress.mode == dressModeAll {
		return
	}
	layout := s.config.Dress
	for index, button := range layout.Targets {
		s.drawFrameButton(canvas, button, s.dressTargetFrame(index, ui.dress.character))
	}
}

// dressTargetFrame 返回角色卡应画的帧。
//
// 两种布局的素材帧序不同：
//   - UKIE 式「标题 + 1 号卡片」合成面板：frame1 是雪花过渡、frame2 是常态
//     头像、frame3 是选中红框（见游戏手册截图）；
//   - 其它作品（侵蝕 / 淫獄 等）是五张独立卡片：frame1 就是常态，
//     frame2 悬停、frame3 选中。
//
// 用"target0 是否包住 target1"这个结构特征区分两者，避免改 UKIE 时
// 把其它作品的卡片画成雪花/悬停帧。
func (s *Session) dressTargetFrame(index, selected int) int {
	if selected >= 0 && selected == index {
		return dressFrameSelected
	}
	if _, _, _, _, ok := s.dressFirstTargetCardRect(); ok {
		return dressTargetNormalFrame
	}
	return dressFrameNormal
}

// drawDressStates 画左侧的「脱衣選択」面板。
//
// 只在全体一括設定（mode1）显示：面板图 strip_cstate0 本身带标题和 7 行
// 占位（着衣 + 選択不可），state1..6 是叠在占位上的行。
// 個別設定时左侧必须是「ヒロイン選択」，否则玩家会看到两个"脱衣選択"标题，
// 而且角色选择会被阶段列表盖住。
func (s *Session) drawDressStates(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	if ui.dress.mode != dressModeAll {
		return
	}
	layout := s.config.Dress
	// 原配置可以用 show common button 关掉这一组按钮。
	if !layout.ShowCommonButton || len(layout.States) == 0 {
		return
	}
	character := s.dressCharacterNumber()
	wear := s.dressWearNumber()
	stage := s.dressCurrentStage()

	// 面板本体：frame 1 常态、2 悬停着衣、3 选中着衣。
	panel := layout.States[0]
	panelFrame := dressFrameNormal
	if s.ui.hover.kind == hitDressState && s.ui.hover.index == 0 {
		panelFrame = dressFrameHover
	}
	if stage == 0 {
		panelFrame = dressFrameSelected
	}
	s.drawFrameButton(canvas, panel, panelFrame)

	// state1..6 是面板里"着衣"以下的六行。
	for index := 1; index < len(layout.States); index++ {
		button := layout.States[index]
		if !layout.StageAvailable(character, wear, index) {
			s.drawFrameButton(canvas, button, dressFrameNormal)
			s.dressDarken(canvas, button)
			continue
		}
		s.drawFrameButton(canvas, button, s.dressFrame(hitDressState, index, stage))
	}
}

// dressDarken 在不可用的按钮上压一层暗色。
//
// 原引擎用"变暗"表示不可点；这里用半透明黑覆盖按钮区域，视觉上就是灰掉。
func (s *Session) dressDarken(canvas *gfx.BgraCanvas, button config.UIButton) {
	if s.composer == nil {
		return
	}
	image := s.composer.Load(button.Image)
	if image == nil {
		return
	}
	height := button.FrameHeight(image.Height)
	canvas.Fill(button.X, button.Y, image.Width, height, 0, 0, 0, 160)
}

// drawDressCostumes 画右侧的服装列表。
//
// 列表只显示"可见条数"那么多，剩下的靠滚动；坐标按 dress pos + 间距依次排开，
// 不取各按钮自己写的坐标（那一列本来就是排出来的）。
func (s *Session) drawDressCostumes(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	layout := s.config.Dress
	list := s.dressCostumeList()
	if len(list) == 0 {
		return
	}
	visible := s.dressVisibleRows()
	selected := s.dressEntry(ui.dress.character).costume
	for row := 0; row < visible; row++ {
		index := ui.dress.scroll + row
		if index >= len(list) {
			break
		}
		button, ok := s.dressCostumeButton(list[index])
		if !ok {
			continue
		}
		button.X = layout.DressPos.X + row*layout.DressSpace.X
		button.Y = layout.DressPos.Y + row*layout.DressSpace.Y
		s.drawFrameButton(canvas, button, s.dressFrame(hitDressCostume, index, selected))
	}
}

// drawDressLevels 画右侧的阶段预览列。
//
// 每一行是"当前角色 + 当前服装"在某个脱衣阶段上的立绘缩略图（levelN 按钮）。
// 用哪一张由阶段表决定：档位号为 0 表示这一级没有素材，就不画那一行。
func (s *Session) drawDressLevels(canvas *gfx.BgraCanvas) {
	layout := s.config.Dress
	if layout.AlternateBase2 && s.ui != nil && s.ui.dress != nil && s.ui.dress.mode == dressModeAll {
		// 全体設定用 common state0..5 作为统一阶段列表，不再画每件服装的
		// levelN 列，否则两组 204px 宽的标签会叠在一起。
		return
	}
	character := s.dressCharacterNumber()
	wear := s.dressWearNumber()
	stage := s.dressCurrentStage()
	row := 0
	for index := 0; index < config.DressStageCount; index++ {
		level := layout.StageLevel(character, wear, index)
		if level == 0 {
			continue
		}
		button, ok := layout.LevelButton(level)
		if !ok {
			continue
		}
		if button, ok = s.dressLevelButtonAt(button, row); !ok {
			continue
		}
		s.drawFrameButton(canvas, button, s.dressFrame(hitDressLevel, index, stage))
		row++
	}
}

// dressLevelButtonAt 把阶段标签列按 level pos/space 排成一行。
//
// 教育指導 KS 的 level1…12 只给图名和帧数，坐标全靠 `level pos = 564,208`
// 与 `level space = 0,40`；旧实现直接用按钮自身的 (0,0)，整列标签叠在
// 屏幕左上角。KSIE 的每个 levelN 都带绝对坐标，LevelPos 为零，保持原坐标。
func (s *Session) dressLevelButtonAt(button config.UIButton, row int) (config.UIButton, bool) {
	layout := s.config.Dress
	if layout.LevelPos.X == 0 && layout.LevelPos.Y == 0 {
		return button, true
	}
	space := layout.LevelSpace
	if space.Y == 0 {
		space.Y = 30
	}
	button.X = layout.LevelPos.X + row*space.X
	button.Y = layout.LevelPos.Y + row*space.Y
	return button, true
}

// dressViewControlsActive 判断自由视角控件当前是否显示。
//
// 有第三个模式按钮（mode2=視点設定）的作品仍只在那个模式显示，避免
// 挡住服装列表。学園3/学園3IE 只配了 mode0/mode1，但手册里「顔/胸/腰」
// 和自由視点バー 是常驻的；旧实现要求 mode==2，玩家永远看不到这套控件。
func (s *Session) dressViewControlsActive() bool {
	if s.ui == nil || s.ui.dress == nil {
		return false
	}
	if !s.config.Dress.View.Enabled {
		return false
	}
	return s.ui.dress.mode == dressModeView ||
		len(s.config.Dress.Modes) <= dressModeView
}

// drawDressView 画自由视角控件（滑杆 + 脸/胸/股间按钮）。
//
// 只有"視点設定"模式才显示：那才是自由视角的画面。
func (s *Session) drawDressView(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	view := s.config.Dress.View
	if !s.dressViewControlsActive() {
		return
	}
	// 轨道：半透明白色，指示可以拖动的范围。
	canvas.Fill(view.MeterX, view.MeterY, view.MeterW, view.MeterH, 255, 255, 255, 60)
	s.drawDressKnob(canvas)
	for index, button := range []config.UIButton{view.Face, view.Bust, view.Hip} {
		s.drawFrameButton(canvas, button, s.dressFrame(hitDressView, index, s.dressViewPresetSelected(index)))
	}
}

// drawDressKnob 画滑杆上的滑块。
//
// 滑块跟着视角位置走：位置 0 在轨道顶端，1000 在底端。
func (s *Session) drawDressKnob(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	view := s.config.Dress.View
	knob := view.Knob
	if knob.Empty() || s.composer == nil {
		return
	}
	image := s.composer.Load(knob.Image)
	if image == nil {
		return
	}
	height := knob.FrameHeight(image.Height)
	fraction := float64(ui.dress.viewPos) / float64(dressViewRange)
	knob.Y = view.MeterY + int(fraction*float64(view.MeterH-height))
	s.drawFrameButton(canvas, knob, dressFrameNormal)
}

// dressViewPresetSelected 判断三个预置按钮里哪一个处于选中状态。
//
// 滑杆停在某个预置位置上时，对应的按钮高亮。
func (s *Session) dressViewPresetSelected(index int) int {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return -1
	}
	if ui.dress.viewPos == s.dressViewPreset(index) {
		return index
	}
	return -1
}

// dressViewPreset 是三个预置按钮对应的视角位置。
func (s *Session) dressViewPreset(index int) int {
	switch index {
	case 0:
		return dressViewFace
	case 1:
		return dressViewBust
	case 2:
		return dressViewHip
	}
	return dressViewFace
}

// ---------------------------------------------------------------- 立绘

// drawDressBody 画屏幕中间的立绘。
//
// 立绘是合成图（容器比里面的画大一圈），缩放要按里面的那一部分算：
// 直接缩放整张合成图，人物会比原版小一截。
func (s *Session) drawDressBody(canvas *gfx.BgraCanvas) {
	layout := s.config.Dress
	// show bu 关闭时原引擎不画立绘，画面只剩底图和按钮。
	if !layout.ShowBu {
		return
	}
	// KS 的全体設定页是"给所有人统一设阶段"的列表页，画面中间没有立绘；
	// 画上去会压住右侧的 common state 列表。
	if layout.AlternateBase2 && s.ui != nil && s.ui.dress != nil && s.ui.dress.mode == dressModeAll {
		return
	}
	name := s.dressBodyName()
	if len(name) == 0 || s.composer == nil {
		return
	}
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	// 教育指導 KS 的全身立绘 co.... 是 BSS-Composition：Parts 里分别是
	// 身体、衣服、头发等图层，flatten 后的 Pixels 才是完整人物。
	// 旧实现无条件取 Parts[0]，只剩身体/内衣，衣服和头发都丢了。
	wholeBody := strings.HasPrefix(name, "co")
	source := image
	originX, originY := 0, 0
	if !wholeBody && len(image.Parts) > 0 && image.Parts[0] != nil {
		// 立绘部件可能是竖排帧条（眨眼/表情），只取第 0 帧。
		source = standeePartFrame(image.Parts[0])
		originX, originY = source.OffsetX, source.OffsetY
	}
	width, height := source.Width, source.Height
	if width <= 0 || height <= 0 {
		return
	}
	// 只缩不放：脸部特写本来就比全身图小，放大反而会糊。目标高度按屏幕
	// 高度与 whole body pos 的纵向偏移计算：立绘中心在屏幕中心 + offset，
	// 顶边落在 0 时高度 = height + 2*offset（offset 为负）。学園3/学園3IE
	// 是 800x600、offset=-16，原实现固定 748 会把 1400x2300 的全身图缩到
	// 748 高，头部被裁掉近 90px。
	targetHeight := s.dressBodyTargetHeight()
	scale := 1.0
	if height > targetHeight {
		scale = float64(targetHeight) / float64(height)
	}
	scaledWidth := int(float64(width) * scale)
	scaledHeight := int(float64(height) * scale)
	// whole body pos 是相对画面中心的偏移。
	offset := layout.WholeBodyPos
	x := (s.width-scaledWidth)/2 + offset.X
	y := (s.height-scaledHeight)/2 + offset.Y - s.dressViewShift(scaledHeight)
	canvas.BlitScaled(source, x, y, scaledWidth, scaledHeight, 255)
	if !wholeBody {
		s.drawDressHair(canvas, name, originX, originY, scale, x, y)
	}
}

// dressBodyTargetHeight 是全身立绘的目标高度。
//
// whole body pos 的 Y 是"相对画面中心的偏移"：立绘中心下移/上移这么多。
// 让立绘顶边正好落在屏幕顶边时，高度就是 height + 2*offsetY（offsetY<0）。
// 这样 800x600 的作品得到 568px，1024x768 的作品得到 736px；都保住了头部，
// 同时底部还能露出工具按钮。屏幕高度不可用时退回旧常量。
func (s *Session) dressBodyTargetHeight() int {
	height := s.height
	if height <= 0 {
		return dressWholeBodyHeight
	}
	offset := s.config.Dress.WholeBodyPos.Y
	if offset < 0 {
		height += offset * 2
	}
	if height <= 0 {
		return s.height
	}
	return height
}

// applyDressViewMode 把脱衣画面的视角同步到正常画面的立绘。
//
// 原版说明明确"视点位置是所有角色共享的"；只改预览图、退出后正常画面
// 仍按旧视角显示，会让玩家觉得設置没有生效。没有 [free-sight screen]
// 的作品（FreeSight.Mode=0）保持原来的纯预览行为。
func (s *Session) applyDressViewMode(mode int) {
	if s.config == nil || !s.config.FreeSight.Mode || s.gameState == nil {
		return
	}
	s.freeSightActive = true
	s.gameState.SetViewMode(clampInt(mode, 0, 2))
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// dressViewModeForPosition 把自由视角条的连续位置折算成三档视点。
//
// 舞台层只支持 0/1/2（顔/胸/腰），滑杆是连续的；按三等分取最近的一档，
// 和原版"拖到大致位置后立绘跳到对应裁切"的观感一致。
func (s *Session) dressViewModeForPosition() int {
	if s.ui == nil || s.ui.dress == nil {
		return 0
	}
	switch {
	case s.ui.dress.viewPos < dressViewRange/3:
		return 0
	case s.ui.dress.viewPos < dressViewRange*2/3:
		return 1
	default:
		return 2
	}
}

// dressViewShift 是视角模式把立绘往上抬的像素数。
//
// 原引擎的"視点設定"是自由视角：滑杆往下拉就往下看，画面里的立绘相应上移。
// 这里以立绘高度的一半为满量程，和原版上下留的余量大致相当。
func (s *Session) dressViewShift(imageHeight int) int {
	if s.ui == nil || s.ui.dress == nil {
		return 0
	}
	if !s.dressViewControlsActive() {
		return 0
	}
	return s.ui.dress.viewPos * imageHeight / (2 * dressViewRange)
}

// drawDressHair 画立绘的头发叠加图。
//
// 头发是单独的一张图（<立绘名>_hair），它和身体图共用同一张合成画布，
// 只是画布上的一小块。所以要把它的画布坐标换算成屏幕坐标再贴上去，
// 否则头发会跑到人物外面去。
// 设置里关掉"下卷きの头发显示"时就不画头发。
func (s *Session) drawDressHair(canvas *gfx.BgraCanvas, body string, originX, originY int, scale float64, x, y int) {
	// co.... 的全身立绘已经把头发合进 composition，没有再拆一张 _hair。
	if strings.HasPrefix(body, "co") {
		return
	}
	if !s.settings.UnderHair || s.composer == nil {
		return
	}
	suffix := s.config.HairSuffix
	if len(suffix) == 0 {
		suffix = state.BustupDefaultHairSuffix
	}
	hair := s.composer.Load(body + suffix)
	if hair == nil {
		return
	}
	source := hair
	hairX, hairY := 0, 0
	if len(hair.Parts) > 0 && hair.Parts[0] != nil {
		source = standeePartFrame(hair.Parts[0])
		hairX, hairY = source.OffsetX, source.OffsetY
	}
	if source.Width <= 0 || source.Height <= 0 {
		return
	}
	dx := x + int(float64(hairX-originX)*scale)
	dy := y + int(float64(hairY-originY)*scale)
	canvas.BlitScaled(source, dx, dy, int(float64(source.Width)*scale), int(float64(source.Height)*scale), 255)
}

// dressBodyName 返回当前应该显示的立绘图名。
//
// 原引擎的立绘图名是定长的：bu<两位角色>u<脱衣阶段><两位服装><姿态><变体>
//   - bu01u0011a：1 号角色、全身、脱衣阶段 0（着衣）、服装 01、姿态 1、变体 a。
//
// 不是每个组合都有素材：UKIE 的 1 号角色在阶段 5（全裸）只有
// bu01u5001a（服装 00）和 bu01u5065a（服装 06），别的服装要退回 00；
// 阶段 3 的服装 03/04 也没有对应图，需要用相邻服装的立绘代替。
// 所以这里先试"当前服装的各个姿态/变体"，再按距离尝试相邻服装，
// 最后才退回最基础的着衣图——旧实现只试当前服装，缺图时直接显示着衣，
// 玩家在 DRESS 里选"下着/全裸"看到的还是穿好的立绘。
func (s *Session) dressBodyName() string {
	character := s.dressCharacterNumber()
	if character <= 0 {
		return ""
	}
	// 教育指導 KS 的全身立绘是 wholebody/<角色>/co....，命名规则与
	// bu<角色>u<阶段><服装>… 完全不同；先走这条分支，找不到再回退。
	if name := s.dressWholeBodyName(character); len(name) > 0 {
		return name
	}
	// 牝教師2 世代（MK2/MK2IE）的 bu 是另一种定长格式：
	// bu<角色2><服装2><阶段2><表情2>（bu01000011 = 1 号角色、服装 00、
	// 阶段 00、表情 11），没有 UKIE 的 'u' 段。不识别时整段立绘为空，
	// DRESS 画面只剩底图与按钮。
	if name := s.mk2DressBodyName(character); len(name) > 0 {
		return name
	}
	stage := clampInt(s.dressCurrentStage(), 0, 9)
	wear := clampInt(s.dressWearNumber(), 0, 99)

	if name := s.findDressBody(character, stage, wear); len(name) > 0 {
		return name
	}

	// 同一阶段下找相邻服装：先往编号小的方向（全裸会落到 00），再往大的方向。
	maxWear := 0
	for _, entry := range s.config.Dress.DressCostumes {
		if entry.Character == character && entry.Wear > maxWear {
			maxWear = entry.Wear
		}
	}
	for delta := 1; delta <= maxWear; delta++ {
		for _, candidate := range []int{wear - delta, wear + delta} {
			if candidate < 0 || candidate > maxWear {
				continue
			}
			if name := s.findDressBody(character, stage, candidate); len(name) > 0 {
				return name
			}
		}
	}
	return fmt.Sprintf("bu%02du0011a", character)
}

// mk2DressBodyName 解析牝教師2 世代的半身立绘名。
//
// 命名是 bu + 角色2位 + 服装2位 + 阶段2位 + 表情2位：
//   - 服装 00..13 与 [dress screen] 的 chNN dressNN 表里的服装号一致；
//   - 阶段只有 00/10/20/30 四档（着衣 / 下着+ストッキング / 下着 / 全裸），
//     由 ngng convert table 把界面上的 0/1/3/5 换算过来；
//   - 表情 11 是默认脸，缺失时逐级回退，最后用着衣默认图。
//
// 只有 bu01000011 存在时才走这条分支：UKIE 的 bu01u…、KS 的 wholebody
// 都不会命中。
func (s *Session) mk2DressBodyName(character int) string {
	if s.res == nil || s.ui == nil || s.ui.dress == nil || character <= 0 {
		return ""
	}
	base := fmt.Sprintf("bu%02d000011", character)
	if !s.resourceExists(base) {
		return ""
	}
	wear := clampInt(s.dressWearForCharacter(s.ui.dress.character), 0, 13)
	stage := clampInt(s.dressCurrentStage(), 0, 3) * 10
	for _, tryWear := range []int{wear, 0} {
		for _, tryStage := range []int{stage, 0} {
			name := fmt.Sprintf("bu%02d%02d%02d11", character, tryWear, tryStage)
			if s.resourceExists(name) {
				return name
			}
		}
	}
	return base
}

// dressWholeBodyName 解析教育指導 KS 的全身立绘名。
//
// wholebody/<角色>/co<角色><姿势><服装><变体>：
//
//	co010011 = 1 号角色、姿势 00、服装 1（制服）、变体 1。
//
// 服装号不是 dressNN 的图号，而是该角色 chNN dressNN 表里的第几条：
// ch01 dress01=制服、dress02=バスケ部ユニフォーム、dress03=水着……
// 与原版 DRESS 的服装列表顺序一一对应。stage 只影响变体；某些服装
// 没有对应变体（例如私服只有变体 4），按距离回退到最近的可用图。
func (s *Session) dressWholeBodyName(character int) string {
	if s.config == nil || s.res == nil || s.ui == nil || s.ui.dress == nil || character <= 0 {
		return ""
	}
	entry := s.dressEntry(s.ui.dress.character)
	list := s.dressCostumeListFor(character)
	group := 1
	if entry.costume >= 0 && entry.costume < len(list) {
		group = list[entry.costume].Wear
	}
	if group < 1 || group > 9 {
		group = 1
	}
	stage := s.dressCurrentStage()
	variants := dressVariantOrder(stage + 1)
	groups := []int{group}
	for candidate := 1; candidate <= 7; candidate++ {
		if candidate != group {
			groups = append(groups, candidate)
		}
	}
	// 姿势 00 是所有服装组的默认站姿；其它 10/30/40/50 是事件姿态，
	// 个别服装只在这些姿态里有图，作为后备。
	for _, current := range groups {
		for _, pose := range []int{0, 10, 30, 40, 50} {
			for _, variant := range variants {
				name := fmt.Sprintf("co%02d%02d%d%d", character, pose, current, variant)
				if _, ok := s.res.TryGet(name); ok {
					return name
				}
			}
		}
	}
	return ""
}

func dressVariantOrder(preferred int) []int {
	if preferred < 1 || preferred > 9 {
		preferred = 1
	}
	order := make([]int, 0, 9)
	order = append(order, preferred)
	for distance := 1; distance <= 8; distance++ {
		for _, candidate := range []int{preferred - distance, preferred + distance} {
			if candidate >= 1 && candidate <= 9 {
				order = append(order, candidate)
			}
		}
	}
	return order
}

// findDressBody 按"角色 + 阶段 + 服装"枚举姿态与变体，返回第一张存在的图。
//
// 姿态编号在素材里不连续（1…5 都有，个别服装只有 5），所以不能只试 1…4。
func (s *Session) findDressBody(character, stage, wear int) string {
	if s.composer == nil {
		return ""
	}
	for pose := 1; pose <= 9; pose++ {
		// 变体在素材里是 a/b/c（bustupVariantFirst…Last），只试 a/b 会让
		// 少数只有 c 图的姿态永远找不到，DRESS 里表现为该阶段没有立绘。
		for _, variant := range []string{"a", "b", "c"} {
			name := fmt.Sprintf("bu%02du%d%02d%d%s", character, stage, wear, pose, variant)
			if s.composer.Load(name) != nil {
				return name
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------- 命中

// hitDressScreen 判定鼠标点在脱衣画面的哪个控件上。
func (s *Session) hitDressScreen(x, y int) uiHit {
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	layout := s.config.Dress
	if layout.Atlas {
		if hitButton(s, layout.Back, x, y) {
			return uiHit{kind: hitDressBack}
		}
		for index, button := range layout.States {
			if hitButton(s, button, x, y) {
				return uiHit{kind: hitDressState, index: index}
			}
		}
		for index, button := range layout.Pens {
			if hitButton(s, button, x, y) {
				return uiHit{kind: hitDressPen, index: index}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	if hitButton(s, layout.Back, x, y) {
		return uiHit{kind: hitDressBack}
	}
	// 教育指導 KS 的個別設定先停在角色选择页：这时只有角色卡、模式和返回
	// 可点，服装/阶段列表还没出现。
	if layout.AlternateBase2 && ui.dress.mode == dressModeSingle && ui.dress.showTargets {
		for index := range layout.Targets {
			if s.hitDressTarget(index, x, y) {
				return uiHit{kind: hitDressTarget, index: index}
			}
		}
		for index, button := range layout.Modes {
			if hitButton(s, button, x, y) {
				return uiHit{kind: hitDressMode, index: index}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	// 视角控件压在画面最上层，先命中它们。
	if hit := s.hitDressView(x, y); hit.kind != hitNone {
		return hit
	}
	// 左栏随模式切换：個別/視点 = ヒロイン選択，全体一括 = 脱衣選択。
	// 两组在配置里占同一块坐标，只能二选一，不能同时命中。
	if ui.dress.mode == dressModeAll {
		if layout.ShowCommonButton && len(layout.States) > 0 {
			panel := layout.States[0]
			// 面板上半部分是标题；"着衣"是标题下方、第一颗可见状态按钮上方的那一行。
			// 注意不能拿 States[1].Y 当底边：UKIE 的 state1/state4 坐标在屏幕外
			// (y=768)，真正可见的下一行是 state2 (y=454)。旧实现算出 436…768
			// 一整块，点"アンスコ/下着/全裸"都会先命中"着衣"。
			top := panel.Y + dressStateTitleHeight
			bottom := s.dressStatePanelBottom(panel)
			if bottom <= top {
				bottom = s.dressStateButtonBottom(panel)
			}
			if x >= panel.X && x < panel.X+s.dressButtonWidth(panel) && y >= top && y < bottom {
				if layout.StageAvailable(s.dressCharacterNumber(), s.dressWearNumber(), 0) {
					return uiHit{kind: hitDressState, index: 0}
				}
			}
			for index := 1; index < len(layout.States); index++ {
				if !layout.StageAvailable(s.dressCharacterNumber(), s.dressWearNumber(), index) {
					continue
				}
				if hitButton(s, layout.States[index], x, y) {
					return uiHit{kind: hitDressState, index: index}
				}
			}
		}
	} else if !layout.AlternateBase2 || ui.dress.showTargets {
		for index := range layout.Targets {
			if s.hitDressTarget(index, x, y) {
				return uiHit{kind: hitDressTarget, index: index}
			}
		}
	}
	if ui.dress.mode != dressModeView {
		if hit := s.hitDressCostume(x, y); hit.kind != hitNone {
			return hit
		}
		if hit := s.hitDressLevel(x, y); hit.kind != hitNone {
			return hit
		}
	}
	for index, button := range layout.Modes {
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitDressMode, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitDressTarget 判定角色卡片。
//
// UKIE 的 target0 不是一张卡片，而是「ヒロイン選択」标题 + 1 号角色卡片的
// 合成图（289x172），后面 target1…4 才是其余四张卡片。如果直接按整张
// target0 命中，它的矩形会盖住全部卡片，点谁都先选中 1 号角色——表现就是
// "点角色卡片没反应"。这里在 target0 明显包住后续卡片时，把它的热区收到
// 第一张卡片的位置（由 target1/2 的横向间距反推），其余 target 仍按整张图。
func (s *Session) hitDressTarget(index int, x, y int) bool {
	if index < 0 || index >= len(s.config.Dress.Targets) {
		return false
	}
	button := s.config.Dress.Targets[index]
	if button.Empty() {
		return false
	}
	if index == 0 {
		if left, top, width, height, ok := s.dressFirstTargetCardRect(); ok {
			return x >= left && y >= top && x < left+width && y < top+height
		}
	}
	return hitButton(s, button, x, y)
}

// dressFirstTargetCardRect 从 target1/target2 反推 target0 里第一张角色卡的热区。
//
// 配置里五张卡是等距横排的：target0 的第一张卡与 target1…4 同一行，
// 间距可以从 target2.X - target1.X 得到。只有 target0 的矩形确实包住
// target1 时才启用这个收窄逻辑，避免影响"target0 本来就是一张普通卡片"的作品。
func (s *Session) dressFirstTargetCardRect() (left, top, width, height int, ok bool) {
	targets := s.config.Dress.Targets
	if len(targets) < 2 {
		return 0, 0, 0, 0, false
	}
	panel := targets[0]
	first := targets[1]
	firstImage := s.buttonImage(first)
	if firstImage == nil {
		return 0, 0, 0, 0, false
	}
	cardWidth := firstImage.Width
	cardHeight := first.FrameHeight(firstImage.Height)
	if cardWidth <= 0 || cardHeight <= 0 {
		return 0, 0, 0, 0, false
	}

	cardX := first.X
	if len(targets) >= 3 {
		if pitch := targets[2].X - first.X; pitch > 0 {
			cardX = first.X - pitch
		}
	}
	cardY := first.Y

	// 只处理 target0 把 target1 整张卡片包在里面的情况。
	panelImage := s.buttonImage(panel)
	if panelImage == nil {
		return 0, 0, 0, 0, false
	}
	panelWidth := panelImage.Width
	panelHeight := panel.FrameHeight(panelImage.Height)
	if first.X < panel.X || first.Y < panel.Y ||
		first.X+firstImage.Width > panel.X+panelWidth ||
		first.Y+first.FrameHeight(firstImage.Height) > panel.Y+panelHeight {
		return 0, 0, 0, 0, false
	}
	return cardX, cardY, cardWidth, cardHeight, true
}

// dressStatePanelBottom 返回「脱衣選択」面板里第一颗可见状态按钮的上沿。
//
// UKIE 的 state1/state4 坐标在屏幕外（y=768），不能直接拿 state1 当底边；
// 取"大于面板原点、且仍在屏幕内"的最小 Y。没有可用行时退回面板帧高。
func (s *Session) dressStatePanelBottom(panel config.UIButton) int {
	bottom := 0
	for index := 1; index < len(s.config.Dress.States); index++ {
		y := s.config.Dress.States[index].Y
		if y <= panel.Y {
			continue
		}
		if s.height > 0 && y >= s.height {
			continue
		}
		if bottom == 0 || y < bottom {
			bottom = y
		}
	}
	if bottom > panel.Y {
		return bottom
	}
	return s.dressStateButtonBottom(panel)
}

// dressStateButtonBottom 是面板帧的下沿，作为没有后续可见行时的兜底。
func (s *Session) dressStateButtonBottom(panel config.UIButton) int {
	if image := s.buttonImage(panel); image != nil {
		return panel.Y + panel.FrameHeight(image.Height)
	}
	return panel.Y + dressStateTitleHeight
}

// dressButtonWidth 返回按钮图的宽度；没有图片时退回 224（旧实现的硬编码）。
func (s *Session) dressButtonWidth(button config.UIButton) int {
	if image := s.buttonImage(button); image != nil && image.Width > 0 {
		return image.Width
	}
	return 224
}

func (s *Session) hitDressCostume(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	layout := s.config.Dress
	list := s.dressCostumeList()
	for row := 0; row < s.dressVisibleRows(); row++ {
		index := ui.dress.scroll + row
		if index >= len(list) {
			break
		}
		button, ok := s.dressCostumeButton(list[index])
		if !ok {
			continue
		}
		button.X = layout.DressPos.X + row*layout.DressSpace.X
		button.Y = layout.DressPos.Y + row*layout.DressSpace.Y
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitDressCostume, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitDressLevel 命中阶段预览列，返回的是脱衣阶段号。
func (s *Session) hitDressLevel(x, y int) uiHit {
	layout := s.config.Dress
	if layout.AlternateBase2 && s.ui != nil && s.ui.dress != nil && s.ui.dress.mode == dressModeAll {
		return uiHit{kind: hitNone, index: -1}
	}
	character := s.dressCharacterNumber()
	wear := s.dressWearNumber()
	row := 0
	for index := 0; index < config.DressStageCount; index++ {
		level := layout.StageLevel(character, wear, index)
		if level == 0 {
			continue
		}
		button, ok := layout.LevelButton(level)
		if !ok {
			continue
		}
		if button, ok = s.dressLevelButtonAt(button, row); !ok {
			continue
		}
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitDressLevel, index: index}
		}
		row++
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitDressView 命中视角控件（滑杆与三个预置按钮）。
func (s *Session) hitDressView(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	view := s.config.Dress.View
	if !s.dressViewControlsActive() {
		return uiHit{kind: hitNone, index: -1}
	}
	for index, button := range []config.UIButton{view.Face, view.Bust, view.Hip} {
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitDressView, index: index}
		}
	}
	if view.MeterW > 0 && view.MeterH > 0 &&
		x >= view.MeterX && x < view.MeterX+view.MeterW &&
		y >= view.MeterY && y < view.MeterY+view.MeterH {
		return uiHit{kind: hitDressViewBar}
	}
	return uiHit{kind: hitNone, index: -1}
}

// ---------------------------------------------------------------- 交互

// activateDressScreen 处理脱衣画面上的一次点击。
//
// 换装/脱衣阶段只作为预览，不写进存档；但视角位置按原版是"全角色共享"
// 的，所以点顔/胸/腰或拖动自由视角条时同步更新舞台的 ViewMode，
// 关闭界面后正常画面的立绘仍保持这个裁切。
func (s *Session) activateDressScreen(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	layout := s.config.Dress
	if layout.Atlas {
		switch hit.kind {
		case hitDressBack:
			s.md2DressPerspReset()
			s.dressBack()
		case hitDressState:
			s.md2DressStage = clampInt(hit.index, 0, 2)
			// 切换立绘阶段时透視清零（手册：切换立绘/关闭画面时重置）。
			s.md2DressPerspReset()
			s.applyMD2DressStage()
		case hitDressPen:
			s.md2DressPen = clampInt(hit.index, 0, 3)
		}
		s.dressInvalidate()
		return
	}
	switch hit.kind {
	case hitDressBack:
		// KS 個別設定在编辑页按返回先回角色选择页，再按一次才退出。
		if layout.AlternateBase2 && ui.dress.mode == dressModeSingle && !ui.dress.showTargets {
			ui.dress.showTargets = true
		} else {
			s.dressBack()
		}
	case hitDressTarget:
		if hit.index >= 0 && hit.index < len(layout.Targets) {
			ui.dress.character = hit.index
			// 换角色后服装列表整个变了，滚动位置回到顶部。
			ui.dress.scroll = 0
			ui.dress.showTargets = false
			s.dressClampStages()
		}
	case hitDressCostume:
		s.dressSelectCostume(hit.index)
	case hitDressState:
		s.dressSelectStage(s.dressStageFromState(hit.index))
	case hitDressLevel:
		s.dressSelectStage(hit.index)
	case hitDressMode:
		if hit.index >= 0 && hit.index < len(layout.Modes) {
			ui.dress.mode = hit.index
			// 切到個別設定时回到角色选择页；全体設定直接显示服装/阶段。
			ui.dress.showTargets = layout.AlternateBase2 && hit.index == dressModeSingle
		}
	case hitDressView:
		ui.dress.viewPos = s.dressViewPreset(hit.index)
		s.applyDressViewMode(hit.index)
	case hitDressViewBar:
		s.dressSetViewByY(y)
		s.applyDressViewMode(s.dressViewModeForPosition())
	}
	s.dressInvalidate()
}

// dressBack 返回上一层。
//
// 从鉴赏室进来的要回鉴赏室主菜单（鉴赏室的状态留着，返回时还在原来的页签）；
// 从游戏内工具条进来的直接关掉这个系统界面，回到剧情。
func (s *Session) dressBack() {
	ui := s.ui
	if ui == nil {
		return
	}
	if ui.dress != nil && ui.dress.fromMuseum {
		ui.kind = uiMuseum
		ui.hover = uiHit{kind: hitNone, index: -1}
		return
	}
	s.closeSystemUI()
}

// dressSelectCostume 选中列表里的一条服装。
func (s *Session) dressSelectCostume(index int) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	list := s.dressCostumeList()
	if index < 0 || index >= len(list) {
		return
	}
	if ui.dress.mode == dressModeAll {
		// 全体一括設定：这一件衣服套用到所有角色。
		s.dressApplyWearAll(list[index].Wear)
		s.dressClampStages()
		for index := range ui.dress.perCharacter {
			s.dressEntry(index).set = true
		}
	} else {
		entry := s.dressEntry(ui.dress.character)
		entry.costume = index
		entry.set = true
		// 换了衣服以后当前的脱衣阶段可能没有素材，退到最接近的有素材的阶段。
		s.dressClampStageOf(ui.dress.character)
	}
	s.dressClampScroll()
	s.syncDressLive()
}

// dressApplyWearAll 把选中的服装套用到所有角色（全体一括設定）。
//
// 各角色的服装编号是对应的（1 号服装就是每个角色的"私服"），
// 所以按服装编号去找对应的那一条；某个角色没有这件衣服时退回它列表的第一条。
func (s *Session) dressApplyWearAll(wear int) {
	dress := s.ui.dress
	if dress == nil {
		return
	}
	for index := 0; index < len(dress.perCharacter); index++ {
		list := s.dressCostumeListFor(index + 1)
		s.dressEntry(index).costume = dressCostumeIndexForWear(list, wear)
	}
}

// dressCostumeIndexForWear 在服装列表里找服装编号对应的那一条。
//
// 列表已经按"全服装一括在前"重排，0 号是 wear0，所以找不到时不能直接
// 返回 0（那会变成"全服装一括"），应退回第一条具体服装。
func dressCostumeIndexForWear(list []config.DressCostume, wear int) int {
	for index, entry := range list {
		if entry.Wear == wear {
			return index
		}
	}
	return dressDefaultCostumeIndex(list)
}

// dressSelectStage 切到某个脱衣阶段。
func (s *Session) dressSelectStage(stage int) {
	ui := s.ui
	if ui == nil || ui.dress == nil {
		return
	}
	if stage < 0 || stage >= config.DressStageCount {
		return
	}
	if ui.dress.mode == dressModeAll {
		// 全体一括設定：一次改动套用到所有角色。
		// 每个角色能到的阶段不一样（服装表不同），所以各自夹到最近的有素材的阶段。
		for index := 0; index < len(ui.dress.perCharacter); index++ {
			character := index + 1
			wear := s.dressWearForCharacter(index)
			entry := s.dressEntry(index)
			entry.strip = s.dressClampStage(character, wear, stage)
			entry.set = true
		}
		s.syncDressLive()
		return
	}
	character := ui.dress.character
	wear := s.dressWearForCharacter(character)
	// 单个角色：这件衣服没有这一阶段的素材时忽略这次点击。
	if !s.config.Dress.StageAvailable(character+1, wear, stage) {
		return
	}
	entry := s.dressEntry(character)
	entry.strip = stage
	entry.set = true
	s.syncDressLive()
}

// dressClampStages 把每个角色当前的阶段收敛到有素材的阶段上。
//
// 只有"全体一括設定"才需要动所有人的阶段：单角色模式下别的角色没换衣服，
// 它们的阶段本来就是对的。
func (s *Session) dressClampStages() {
	dress := s.ui.dress
	if dress == nil {
		return
	}
	for index := 0; index < len(dress.perCharacter); index++ {
		s.dressClampStageOf(index)
	}
}

// dressClampStageOf 把某个角色当前的阶段收敛到有素材的阶段上。
func (s *Session) dressClampStageOf(index int) {
	character := index + 1
	wear := s.dressWearForCharacter(index)
	entry := s.dressEntry(index)
	entry.strip = s.dressClampStage(character, wear, entry.strip)
}

// dressSetViewByY 按鼠标在滑杆上的位置设置视角。
func (s *Session) dressSetViewByY(y int) {
	view := s.config.Dress.View
	if view.MeterH <= 0 {
		return
	}
	fraction := float64(y-view.MeterY) / float64(view.MeterH)
	fraction = clampFloat(fraction, 0, 1)
	s.ui.dress.viewPos = int(fraction * float64(dressViewRange))
}

// clampFloat 把浮点数限制在区间内。
func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// ---------------------------------------------------------------- 滚轮

// wheelDress 用滚轮滚动服装列表。
//
// 滚轮事件只带增量、没有坐标，所以只能滚"当前这一栏"：
// 这个画面上会滚动的就是服装列表，直接调列表的绝对滚动偏移。
// 一次滚三条（行距 30、一屏 12 条，三条刚好能看出滚到哪儿了）。
func (s *Session) wheelDress(delta int32) {
	ui := s.ui
	if ui == nil || ui.dress == nil || s.config == nil {
		return
	}
	list := s.dressCostumeList()
	visible := s.dressVisibleRows()
	maxScroll := max(0, len(list)-visible)
	scroll := clampInt(ui.dress.scroll+int(delta)*dressWheelStep, 0, maxScroll)
	if scroll == ui.dress.scroll {
		return
	}
	ui.dress.scroll = scroll
	s.dressInvalidate()
}

// drawMD2DressScreen 画 满淫电车2 的ぬぎぬぎ设定：透明的 dress_base 叠在
// 当前剧情画面上，下面是 3 个着衣状态和 4 个透視模式按钮。
func (s *Session) drawMD2DressScreen(canvas *gfx.BgraCanvas, layout config.DressScreen) {
	// 透視：先把 dress_base 透明覆盖层画上（剧情立绘已由合成器垫底），
	// 再画透视结果；顺序不能反，透視图要压在立绘上但不能盖住面板。
	if layout.Atlas {
		if s.md2DressPersp != nil {
			s.md2DressPersp = s.md2DressPerspEnsure(s.md2DressPersp, canvas.Width, canvas.Height)
			s.md2DressDrawPerspSolve(canvas, s.md2DressPersp)
		}
	}
	s.drawImageAt(canvas, layout.Base, 0, 0)
	for index, button := range layout.States {
		frame := 0
		if index == s.md2DressStage {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
	for index, button := range layout.Pens {
		frame := 0
		if index == s.md2DressPen {
			frame = 2
		} else if s.ui != nil && s.ui.hover.kind == hitDressPen && s.ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
	s.drawFrameButton(canvas, layout.Back, s.dressFrame(hitDressBack, 0, 1))
}
