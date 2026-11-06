package bs

import (
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	vmbs "kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

const (
	FacePlane  = state.OverlayPlane
	LayerLimit = state.PlaneCount - 1
)

const (
	bsRemoveSubcode             = 0
	bsBackgroundSubcode         = 1
	bsOverlaySubcode            = 2
	bsBustupSubcode             = 3
	bsInPlaceSubcode            = 4
	bsRenameSubcode             = 5
	bsExtendedBackgroundSubcode = 0x81
)

const (
	bsMessageWindowPart = 0
	bsHitWindowPart     = 1
	bsCloseWindowPart   = 2
)

const (
	bsOverlayMargin  = 4
	bsGroupKeyLength = 4
	bsFilterKindMax  = 14
)

const (
	bsDeleteVisibility = -1
	bsHideVisibility   = 0
	bsShowVisibility   = 1
)

const bsRev4BustupMarker = 'p'

const (
	bsMessageFrameImage = "message"
	bsMessageHitImage   = "message_hit"
	bsInformationImage  = "information"
	bsCloseImage        = "buchg_close"
)

const (
	bsVariantFirst = 0
	bsVariantLast  = 9
)

const (
	bsRev3BustupSuffix = "00"
	bsRev5BustupSuffix = "0"
	bsFilterSuffix     = "filter"
)

// Rev5 的「cs」立绘是"分块合成"而不是一张整图：剧本只给
// cs<角色><场景><距离><表情> 6 字符名，引擎把它拆成
//   - 身体：cs<前5字符>0 + 视点(a/b/c) + 服装(a/b/c)
//   - 表情：原 6 字符名
//
// 视点是官方说明里的「顔から胸 / 胸から股間 / 股間から太もも」，
// 服装是「着衣 / 下着 / 全裸」；距离位是 1（遠い）/2（近い）。
//
// 远距离必须把 a/b/c 三块按原图的拼接偏移叠成全身再缩放；只画 a 块
// 会得到"只有头/上半身"的画面，这正是旧实现里 MK 立绘只剩头的原因。
const (
	bsLegacyCsNameLength = 6
	bsLegacyCsBaseEnd    = 5
	bsLegacyCsDistance   = 4
	bsLegacyCsExpression = 5

	bsLegacyNearDistance = '2'
	bsLegacyViewCount    = 3
	bsLegacyWearCount    = 3

	// 三块身体图的屏幕拼接偏移（原图像素）。用相邻块重叠区的像素差
	// 搜索得到：a/b 最优 376，b/c 最优 274（即 c 相对 a 是 650）。
	bsLegacySectionBOffset = 376
	bsLegacySectionCOffset = 650
	// 远距离缩放/落点：官方截图里全身站姿约占 600 高的 715px 缩放区，
	// 头宽约为原图的 0.6；水平居中后左边为 (800-800*0.6)/2=160。
	bsLegacyFarScale = 0.6
	bsLegacyFarX     = 160
	bsLegacyFarY     = 60
)

var bsFilterNames = []string{"reverse", "grayscale", "blur", "blackout"}

type BsStageDirector struct {
	machine   *vm.BsxMachine
	script    vm.BsMachineCore
	st        *state.GameState
	resources *resource.GameResourceIndex
	frame     *state.StageOverlay
	hit       *state.StageOverlay
	close     *state.StageOverlay
	// messageFrameOverlay 表示对话框底图要由舞台叠层画。
	//
	// 配置了 [message window] base 的作品（KD/AFDVD/学園2）由
	// MessageWindowRenderer 画底图并做底色混合；舞台再叠一张同样的
	// "message" 会画两遍（且叠层不受 bg color 控制）。只有没有配置
	// 底图、但仍需要 band 叠层的作品才保留这个旧路径。
	messageFrameOverlay bool

	// legacyBustup 保存当前 Rev5「cs」立绘的原始名与视点/服装档位。
	//
	// 剧本只在换表情时重发 IMAGE，DRESS/VIEW 按钮却要立即重画同一个人物的
	// 另一套身体图，所以必须记住最后一次的名字和档位；否则按钮只能改设置，
	// 画面要等下一句对白才更新。
	legacyBustup  string
	legacyView    int
	legacyWear    int
	legacyBustupX int
	legacyBustupY int
	// legacyCl 保存同一代「cl」立绘（cl<CG><差分>：身体是 cl1310aa/ab/ac，
	// 表情头是 cl1311…cl1316）。剧本只把表情头当 IMAGE 发过来，身体必须由
	// 引擎按服装档补上；否则画面上只有一颗浮空的头（MK 实测）。
	legacyCl     string
	legacyClWear int
	legacyClX    int
	legacyClY    int
	// rev6Bustup 保存ヒメドレイ的「身体 + 表情 + 眼睛」立绘原始名。
	// DRESS 按钮要在这套服装编号（第 5..7 位）之间循环，表情后缀保持不变。
	rev6Bustup  string
	rev6Dress   int
	rev6BustupX int
	rev6BustupY int
}

func NewBsStageDirector(machine *vm.BsxMachine, script vm.BsMachineCore, st *state.GameState, configuration *config.BsxxConfiguration, resources *resource.GameResourceIndex) *BsStageDirector {
	overlay := true
	if configuration != nil && configuration.Message.Base == bsMessageFrameImage {
		overlay = false
	}
	return &BsStageDirector{machine: machine, script: script, st: st, resources: resources, messageFrameOverlay: overlay}
}

func (d *BsStageDirector) ShowImage() state.SessionStep {
	command := d.machine.Command()
	name := d.script.Name(int(command.First))
	if len(name) > 0 && name[0] == '*' {
		return state.SessionContinue
	}
	if d.isAncient() {
		return d.showAncientImage(name, int(command.Second), int(command.Third), int(command.Fourth))
	}
	x := int(command.Third)
	y := int(command.Fourth)
	switch int(command.Second) {
	case bsRemoveSubcode:
		d.remove(name)
	case bsBackgroundSubcode, bsExtendedBackgroundSubcode:
		d.showBackground(name, int(command.Second), x, y)
	case bsOverlaySubcode:
		d.upsert(name, d.resolve(name, x, y), x, y)
	case bsBustupSubcode:
		if d.isRev() {
			d.showBustup(name, x, y)
		} else {
			d.upsert(name, d.resolve(name, x, y), x, y)
		}
	case bsInPlaceSubcode:
		d.replaceInPlace(name, name, x, y)
	case bsRenameSubcode:
		d.rename(name, d.script.Name(int(command.Third)))
	default:
		core.Logf("gfx: image '%s' subcode %d ignored", name, int(command.Second))
	}
	return state.SessionContinue
}

func (d *BsStageDirector) showAncientImage(name string, mode, x, y int) state.SessionStep {
	switch mode {
	case bsRemoveSubcode:
		d.remove(name)
	case bsBackgroundSubcode, bsExtendedBackgroundSubcode:
		d.showBackground(name, mode, x, y)
	case bsOverlaySubcode, bsBustupSubcode:
		d.upsertToTail(name, d.resolve(name, x, y), x, y)
	default:
		core.Logf("gfx: ancient image '%s' mode %d illegal, fault", name, mode)
		return state.SessionFaulted
	}
	return state.SessionContinue
}

func (d *BsStageDirector) upsertToTail(name string, artworks []state.LayerArtwork, x, y int) {
	table := d.current()
	index := indexOfBsLayer(table, name)
	if index >= 0 {
		existing := table[index]
		existing.X = x
		existing.Y = y
		existing.Artworks = artworks
		table = append(table[:index], table[index+1:]...)
		table = append(table, existing)
		d.commit(table, "layer '"+name+"' moved to front ("+itoaBs(x)+","+itoaBs(y)+")")
		return
	}
	table = d.add(table, name, artworks, x, y)
	d.commit(table, "layer '"+name+"' appended ("+itoaBs(x)+","+itoaBs(y)+")")
}

func (d *BsStageDirector) ApplyLayerControl() state.SessionStep {
	command := d.machine.Command()
	switch vmbs.BsLayerOperation(int(command.Second)) {
	case vmbs.BsLayerSwap:
		d.swapLayers(int(command.First), int(command.Third))
	case vmbs.BsLayerVisibility:
		d.setLayerVisibility(int(command.First), int(command.Third))
	case vmbs.BsLayerPosition:
		d.moveLayer(int(command.First), int(command.Third), int(command.Fourth))
	case vmbs.BsLayerFilter:
		d.applyFilter(int(command.First), int(command.Third), int(command.Fourth))
	default:
		core.Logf("gfx: layer operation %d ignored", int(command.Second))
	}
	return state.SessionContinue
}

func (d *BsStageDirector) ApplyLayerImage() state.SessionStep {
	return d.applyLayerReplacement()
}

func (d *BsStageDirector) ApplyLayerRename() state.SessionStep {
	return d.applyLayerReplacement()
}

func (d *BsStageDirector) ShowMessageWindow() {
	d.st.SetMessageWindowVisible(true)
	if d.messageFrameOverlay {
		overlay := bandBsOverlay(bsMessageFrameImage)
		d.frame = &overlay
	}
	d.syncOverlays()
}

func (d *BsStageDirector) ApplyWindow() state.SessionStep {
	command := d.machine.Command()
	part := int(command.First)
	visible := command.Second != 0
	switch part {
	case bsMessageWindowPart:
		if visible && d.messageFrameOverlay {
			overlay := bandBsOverlay(bsMessageFrameImage)
			d.frame = &overlay
		} else {
			d.frame = nil
		}
		d.st.SetMessageWindowVisible(visible)
		d.syncOverlays()
		core.Logf("ui: window frame visible=%v", visible)
		return state.SessionContinue
	case bsHitWindowPart:
		if visible {
			overlay := d.hitOverlay()
			d.hit = &overlay
		} else {
			d.hit = nil
		}
		d.syncOverlays()
		image := "-"
		if d.hit != nil {
			image = d.hit.Image
		}
		core.Logf("ui: window hit '%s' visible=%v", image, visible)
		return state.SessionContinue
	case bsCloseWindowPart:
		if d.isCompact() {
			if visible {
				overlay := state.StageOverlay{Image: bsCloseImage, Anchor: state.StageOverlayTopRight, X: bsOverlayMargin, Y: bsOverlayMargin}
				d.close = &overlay
			} else {
				d.close = nil
			}
			d.syncOverlays()
			core.Logf("ui: window part=%d '%s' visible=%v", bsCloseWindowPart, bsCloseImage, visible)
			return state.SessionContinue
		}
		core.Logf("ui: window part=%d flag=%d ignored, frame visible=%v", part, int(command.Second), d.st.MessageWindowVisible())
		return state.SessionContinue
	default:
		core.Logf("ui: window part=%d flag=%d ignored, frame visible=%v", part, int(command.Second), d.st.MessageWindowVisible())
		return state.SessionContinue
	}
}

func (d *BsStageDirector) ApplyClearScreen() state.SessionStep {
	effect := int(d.machine.Command().First)
	d.st.StartTransition(effect, 0)
	core.Logf("gfx: clear screen effect=%d", effect)
	return state.SessionContinue
}

func (d *BsStageDirector) ApplyTransition() state.SessionStep {
	command := d.machine.Command()
	mask := ""
	if command.Second >= 0 {
		mask = d.script.Name(int(command.Second))
	}
	effect := int(command.First)
	if len(mask) > 0 {
		d.st.SetMask(mask)
		effect = state.MaskTransition
	}
	d.st.StartTransition(effect, 0)
	core.Logf("gfx: transition effect=%d mask='%s' -> %d", int(command.First), mask, effect)
	return state.SessionContinue
}

func (d *BsStageDirector) ApplyFace(names int) {
	if names < 0 {
		d.clearFace()
		return
	}
	name := d.script.Name(names)
	if state.StageNameIsEmpty(name) {
		d.clearFace()
		return
	}
	if d.st.LayerAt(FacePlane).Name() == name {
		return
	}
	if !d.existsExactly(name) {
		d.clearFace()
		core.Logf("gfx: face '%s' not found, cleared", name)
		return
	}
	d.st.SetLayerAt(FacePlane, name, state.StageLayerCharacter, d.resolve(name, 0, 0))
	core.Logf("gfx: face '%s' -> plane %d", name, FacePlane)
}

func (d *BsStageDirector) clearFace() {
	if len(d.st.LayerAt(FacePlane).Artworks()) > 0 {
		d.st.ClearLayer(FacePlane)
		core.Logf("gfx: face cleared")
	}
}

func (d *BsStageDirector) remove(name string) {
	table := d.current()
	index := indexOfBsLayer(table, name)
	if index < 0 {
		index = indexOfBsLayerOwner(table, name)
	}
	if index < 0 {
		core.Logf("gfx: image '%s' has no layer to remove", name)
		return
	}
	if strings.HasPrefix(table[index].Name, "CS") {
		d.legacyBustup = ""
	}
	if strings.HasPrefix(table[index].Name, "BU") {
		d.legacyCl = ""
	}
	table = append(table[:index], table[index+1:]...)
	d.commit(table, "image '"+name+"' removed")
}

func (d *BsStageDirector) showBackground(name string, subcode, x, y int) {
	if baseName, ok := d.tryRev4Bustup(name); ok {
		bustup := make([]state.BsLayerEntry, 0, 2)
		bustup = d.addEntry(bustup, baseName, d.resolve(baseName, x, y), x, y)
		bustup = d.addEntry(bustup, name, []state.LayerArtwork{patchBsArtwork(name, x, y)}, x, y)
		d.commit(bustup, "bustup background '"+baseName+"' + '"+name+"' ("+itoaBs(x)+","+itoaBs(y)+")")
		return
	}
	layers := make([]state.BsLayerEntry, 0, 2)
	entry := state.NewBsLayerEntry(name, x, y, state.StageLayerScreen, d.resolve(name, x, y))
	if !d.anyArtworkExists(entry.Artworks) {
		core.Logf("gfx: background '%s' not found, kept as a layer entry", name)
	}
	layers = append(layers, entry)
	if subcode == bsExtendedBackgroundSubcode {
		core.Logf("gfx: background '%s' recall layer skipped", name)
	}
	d.commit(layers, "background '"+name+"' ("+itoaBs(x)+","+itoaBs(y)+")")
}

func (d *BsStageDirector) upsert(name string, artworks []state.LayerArtwork, x, y int) {
	table := d.current()
	table = d.update(table, name, artworks, x, y)
	d.commit(table, "layer '"+name+"' ("+itoaBs(x)+","+itoaBs(y)+")")
}

func (d *BsStageDirector) replaceInPlace(lookup, artwork string, x, y int) {
	table := d.current()
	index := indexOfBsLayer(table, lookup)
	if index >= 0 {
		existing := table[index]
		table[index] = existing.WithArtworks(artwork, d.resolve(artwork, existing.X, existing.Y))
		d.commit(table, "layer '"+lookup+"' replaced in place ("+itoaBs(existing.X)+","+itoaBs(existing.Y)+")")
		return
	}
	table = d.add(table, artwork, d.resolve(artwork, x, y), x, y)
	d.commit(table, "layer '"+artwork+"' appended ("+itoaBs(x)+","+itoaBs(y)+")")
}

func (d *BsStageDirector) rename(lookup, artwork string) {
	table := d.current()
	index := indexOfBsLayer(table, lookup)
	if index < 0 {
		core.Logf("gfx: layer '%s' not found, rename to '%s' skipped", lookup, artwork)
		return
	}
	existing := table[index]
	table[index] = existing.WithArtworks(artwork, d.resolve(artwork, existing.X, existing.Y))
	d.commit(table, "layer '"+lookup+"' renamed to '"+artwork+"'")
}

func (d *BsStageDirector) showBustup(name string, x, y int) {
	// Rev6 的「bu + 4 位角色/姿势 + 3 位表情」立绘（ヒメドレイ）是
	// 身体、脸、眼睛三张 800x600 合成图：只画剧本给的表情图会只剩一个头。
	// 必须在旧的 base 推导之前截住。
	if d.showRev6ComposedBustup(name, x, y) {
		return
	}
	d.rev6Bustup = ""
	// Rev5 的 cs 合成立绘必须在旧的 bu 解析之前截住：旧的 base 推导会
	// 得到不存在的 6 字符身体名，最后只剩下表情头被单独画出来。
	if d.showLegacyBustup(name, x, y) {
		return
	}
	if d.showLegacyClBustup(name, x, y) {
		return
	}
	// 剧本换成了别的立绘体系（bu 立绘/普通 CG）时清掉 Rev5 记录，
	// 否则玩家在新场景点 DRESS 会把上一个角色重新贴回舞台。
	d.legacyBustup = ""
	d.legacyCl = ""
	baseName, ok := d.tryDeriveBustupBase(name)
	if !ok {
		d.upsert(name, d.resolve(name, x, y), x, y)
		return
	}
	group := d.groupKey(name)
	var baseImage string
	var patchImage string
	if d.script.Dialect() == vmbs.BsScriptRev6 {
		baseImage = baseName
		patchImage = d.resolveKdPatchName(name)
	} else {
		baseImage = d.resolveVariant(baseName)
		patchImage = d.resolveVariant(name)
	}
	artworks := make([]state.LayerArtwork, 0, 2)
	if d.existsExactly(baseImage) {
		artworks = append(artworks, state.NewLayerArtwork(baseImage, x, y))
	}
	if d.existsExactly(patchImage) && patchImage != baseImage {
		artworks = append(artworks, patchBsArtwork(patchImage, x, y))
	}
	if len(artworks) == 0 {
		core.Logf("gfx: bustup '%s' has no drawable artwork, skipped", name)
		return
	}
	table := d.current()
	index := indexOfBsLayer(table, group)
	entry := state.NewBsLayerEntry(group, x, y, state.StageLayerCharacter, artworks)
	if index >= 0 {
		table[index] = entry
	} else {
		table = append(table, entry)
	}
	note := "bustup '" + group + "' '" + artworks[0].Image + "' only (" + itoaBs(x) + "," + itoaBs(y) + ")"
	if len(artworks) > 1 {
		note = "bustup '" + group + "' '" + baseImage + "' + '" + patchImage + "' (" + itoaBs(x) + "," + itoaBs(y) + ")"
	}
	d.commit(table, note)
}

// showRev6ComposedBustup 处理 Rev6 里「身体 + 表情 + 眼睛」的三层立绘。
//
// ヒメドレイ 的素材命名是 9 字符：
//
//	bu2000108      表情头（130x120，合成图里只有脸）
//	bu2000108eye   眼睛条（120x192，4 帧眨眼）
//	bu2000100      身体（完整人物，头是空白）
//
// 原引擎把三张按顺序叠起来；旧实现把 bu2000108 当成整张立绘，画面上
// 就只剩一个头。条件必须同时满足"9 字符 + 前 7 字符 + 00 的身体存在"，
// 其它 Rev6 作品（KD/牝教師2 老版）的命名不满足，不会被误吃。
func (d *BsStageDirector) showRev6ComposedBustup(name string, x, y int) bool {
	if d.script.Dialect() != vmbs.BsScriptRev6 || len(name) != 9 {
		return false
	}
	if name[0] != 'b' || name[1] != 'u' {
		return false
	}
	for index := 2; index < len(name); index++ {
		if !isBsAsciiDigit(name[index]) {
			return false
		}
	}
	body := name[:7] + "00"
	if body == name || !d.existsExactly(body) || !d.existsExactly(name) {
		return false
	}
	artworks := make([]state.LayerArtwork, 0, 3)
	artworks = append(artworks, state.NewLayerArtwork(body, x, y))
	artworks = append(artworks, state.NewLayerArtwork(name, x, y))
	eye := name + "eye"
	if d.existsExactly(eye) {
		// 眼睛条是 4 帧竖排图。必须标成动画部件，LayerSurface 才会按
		// FrameCount 只裁一帧；用普通 LayerArtwork 会把 120x192 整条贴上去，
		// 画面上就是四只眼睛从上到下排成一列。
		artworks = append(artworks, state.AnimatedLayerArtwork(eye, x, y, 4, 0))
	}
	group := d.groupKey(name)
	table := d.current()
	index := indexOfBsLayer(table, group)
	entry := state.NewBsLayerEntry(group, x, y, state.StageLayerCharacter, artworks)
	if index >= 0 {
		table[index] = entry
	} else {
		table = append(table, entry)
	}
	d.commit(table, "rev6 bustup '"+group+"' body='"+body+"' face='"+name+"' eye='"+eye+"' ("+itoaBs(x)+","+itoaBs(y)+")")
	d.rev6Bustup, d.rev6BustupX, d.rev6BustupY = name, x, y
	if variant, ok := bsRev6BustupVariant(name); ok {
		d.rev6Dress = variant
	}
	return true
}

// bsRev6BustupVariant 取「bu<角色2位><服装3位><表情2位>」里的服装编号。
func bsRev6BustupVariant(name string) (int, bool) {
	if len(name) != 9 {
		return 0, false
	}
	value := 0
	for index := 4; index < 7; index++ {
		if !isBsAsciiDigit(name[index]) {
			return 0, false
		}
		value = value*10 + int(name[index]-'0')
	}
	if value <= 0 {
		return 0, false
	}
	return value, true
}

// pad3 把 1..999 写成三位十进制（服装编号）。
func pad3(value int) string {
	if value < 0 {
		value = 0
	}
	text := itoaBs(value)
	for len(text) < 3 {
		text = "0" + text
	}
	if len(text) > 3 {
		text = text[len(text)-3:]
	}
	return text
}

// ToggleRev6Dress 在 1..8 号服装之间循环，表情后缀保持不变。
//
// ヒメドレイ 的身体图是 bu<角色><服装3位>00，表情图是
// bu<角色><服装3位><表情2位>；原版的 DRESS 按钮换的就是这一段。
// 找不到下一套（个别角色服装更少）就继续试，最多绕一圈。
func (d *BsStageDirector) ToggleRev6Dress() bool {
	if len(d.rev6Bustup) != 9 || d.rev6Dress <= 0 {
		return false
	}
	prefix := d.rev6Bustup[:4]
	expression := d.rev6Bustup[7:9]
	for step := 1; step <= 8; step++ {
		variant := ((d.rev6Dress - 1 + step) % 8) + 1
		candidate := prefix + pad3(variant) + expression
		body := prefix + pad3(variant) + "00"
		if !d.existsExactly(body) || !d.existsExactly(candidate) {
			continue
		}
		d.rev6Dress = variant
		d.showRev6ComposedBustup(candidate, d.rev6BustupX, d.rev6BustupY)
		core.Logf("ui: rev6 dress variant=%d", variant)
		return true
	}
	return false
}

// clBustupWearSuffixes 是「cl」立绘的三档服装后缀。
//
// 原版把同一具身体做成三张 800x600 的整图：aa 着衣、ab 下着、ac 全裸。
// 表情头（cl1311…cl1316）只画五官，身体必须按档位补在下面。
var clBustupWearSuffixes = [...]string{"aa", "ab", "ac"}

// isLegacyClBustup 判断名字是不是 Rev5 的 cl 表情头。
//
// 形如 cl1311：cl + 3 位 CG 编号 + 1 位表情号（1…6）。
// 只看名字不够——同前缀也可能是一张独立 CG，所以还要求 cl1310aa/ab/ac
// 至少存在一张身体图；不存在就交回通用解析，避免误伤普通 CG。
func (d *BsStageDirector) isLegacyClBustup(name string) bool {
	if d.script.Dialect() != vmbs.BsScriptRev5 || len(name) != 6 {
		return false
	}
	if name[0] != 'c' || name[1] != 'l' {
		return false
	}
	for index := 2; index < len(name); index++ {
		if name[index] < '0' || name[index] > '9' {
			return false
		}
	}
	if name[5] < '1' || name[5] > '6' {
		return false
	}
	body := name[:5] + "0"
	for _, suffix := range clBustupWearSuffixes {
		if d.existsExactly(body + suffix) {
			return true
		}
	}
	return false
}

// showLegacyClBustup 按「身体 + 表情头」合成 cl 立绘。
//
// 身体三档依次是着衣/下着/全裸；缺某一档时退回第一张存在的身体图，
// 表情头永远叠在最上面。返回 true 表示这个名字已由本函数接管。
func (d *BsStageDirector) showLegacyClBustup(name string, x, y int) bool {
	if !d.isLegacyClBustup(name) {
		return false
	}
	d.legacyCl, d.legacyClX, d.legacyClY = name, x, y
	d.legacyBustup = ""
	artworks := d.buildLegacyClBustup(name, x, y)
	if len(artworks) == 0 {
		core.Logf("gfx: rev5 cl bustup '%s' has no drawable body, kept old layer", name)
		return true
	}
	group := d.groupKey(name)
	table := d.current()
	index := indexOfBsLayer(table, group)
	entry := state.NewBsLayerEntry(group, x, y, state.StageLayerCharacter, artworks)
	if index >= 0 {
		table[index] = entry
	} else {
		table = append(table, entry)
	}
	d.commit(table, "rev5 cl bustup '"+group+"' '"+name+"' wear="+itoaBs(d.legacyClWear)+" ("+itoaBs(x)+","+itoaBs(y)+")")
	return true
}

func (d *BsStageDirector) buildLegacyClBustup(name string, x, y int) []state.LayerArtwork {
	body := name[:5] + "0"
	artworks := make([]state.LayerArtwork, 0, 2)
	chosen := ""
	for offset := 0; offset < len(clBustupWearSuffixes); offset++ {
		wear := (d.legacyClWear + offset) % len(clBustupWearSuffixes)
		candidate := body + clBustupWearSuffixes[wear]
		if d.existsExactly(candidate) {
			chosen = candidate
			break
		}
	}
	if len(chosen) > 0 {
		artworks = append(artworks, state.NewLayerArtwork(chosen, x, y))
	}
	if d.existsExactly(name) {
		artworks = append(artworks, state.NewLayerArtwork(name, x, y))
	}
	return artworks
}

// isLegacyCsBustup 判断名字是不是 Rev5 的 cs 合成立绘。
//
// 只认 6 字符的 cs + 4 位 [0-9a-z]，避免把现代作品的 bu*、背景图或
// 特殊色名（black/white）误当成这套命名。其它 Rev5 作品若不使用 cs
// 命名（KD/AFDVD 等）也不会进入这条分支。
func (d *BsStageDirector) isLegacyCsBustup(name string) bool {
	if d.script.Dialect() != vmbs.BsScriptRev5 || len(name) != bsLegacyCsNameLength {
		return false
	}
	if name[0] != 'c' || name[1] != 's' {
		return false
	}
	for index := 2; index < len(name); index++ {
		value := name[index]
		if (value >= '0' && value <= '9') || (value >= 'a' && value <= 'z') {
			continue
		}
		return false
	}
	return true
}

// showLegacyBustup 按 Rev5 的 cs 规则合成并替换图层。
//
// 返回值只表示"这个名字由本函数接管"，不表示资源一定齐全：即使身体图
// 缺失也不能退回旧的 bustup 解析，否则会把只有头部的表情图单独画出来，
// 就是玩家看到的"立绘只剩头"。
func (d *BsStageDirector) showLegacyBustup(name string, x, y int) bool {
	if !d.isLegacyCsBustup(name) {
		return false
	}
	d.legacyCl = ""
	d.legacyBustup, d.legacyBustupX, d.legacyBustupY = name, x, y
	artworks, group := d.buildLegacyBustup(name, x, y)
	if len(artworks) == 0 {
		core.Logf("gfx: rev5 bustup '%s' has no drawable artwork, kept old layer", name)
		return true
	}
	table := d.current()
	index := indexOfBsLayer(table, group)
	entry := state.NewBsLayerEntry(group, x, y, state.StageLayerCharacter, artworks)
	if index >= 0 {
		table[index] = entry
	} else {
		table = append(table, entry)
	}
	d.commit(table, "rev5 bustup '"+group+"' '"+name+"' view="+itoaBs(d.legacyView)+" wear="+itoaBs(d.legacyWear)+" ("+itoaBs(x)+","+itoaBs(y)+")")
	return true
}

// buildLegacyBustup 生成 Rev5 立绘的图层名与部件列表。
//
// 近距离（距离位 2）是"镜头推进"的一屏：只画当前视点的 a/b/c 一块，
// 再加表情头。远距离（距离位 1）是全身：a/b/c 三块按拼接偏移叠起来，
// 再整体缩放到原版站位；表情头同样缩放后盖在最上面。
func (d *BsStageDirector) buildLegacyBustup(name string, x, y int) ([]state.LayerArtwork, string) {
	group := "CS" + name[2:3]
	base := name[:bsLegacyCsBaseEnd] + "0"
	far := name[bsLegacyCsDistance] != bsLegacyNearDistance
	artworks := make([]state.LayerArtwork, 0, 4)
	add := func(image string, artworkX, artworkY int, scale float64) {
		if !d.existsExactly(image) {
			return
		}
		artwork := state.NewLayerArtwork(image, artworkX, artworkY)
		if scale > 0 && scale != 1 {
			artwork.ScaleX, artwork.ScaleY = scale, scale
		}
		artworks = append(artworks, artwork)
	}
	section := func(index int) string {
		return string(rune('a' + core.Clamp(index, 0, bsLegacyViewCount-1)))
	}
	wear := string(rune('a' + core.Clamp(d.legacyWear, 0, bsLegacyWearCount-1)))
	if far {
		offsets := [...]int{0, bsLegacySectionBOffset, bsLegacySectionCOffset}
		for index, offset := range offsets {
			image := base + section(index) + wear
			add(image, x+bsLegacyFarX, y+bsLegacyFarY+int(float64(offset)*bsLegacyFarScale), bsLegacyFarScale)
		}
		add(name, x+bsLegacyFarX, y+bsLegacyFarY, bsLegacyFarScale)
	} else {
		add(base+section(d.legacyView)+wear, x, y, 1)
		// 视点 b/c 是胸/腰特写，头已经不在框内；这时再叠表情头会变成
		// 画面顶部飘着一颗头。只有视点 a（顔から胸）才叠表情。
		if d.legacyView == 0 {
			add(name, x, y, 1)
		}
	}
	if len(artworks) == 0 {
		core.Logf("gfx: rev5 bustup '%s' missing body/face in '%s'", name, base)
	}
	return artworks, group
}

// LegacyBustupActive 报告当前场景是否有 Rev5 cs 立绘。
// Session 用它决定 DRESS/VIEW 按钮走原版的换装/视点逻辑还是旧界面。
func (d *BsStageDirector) LegacyBustupActive() bool {
	return len(d.legacyBustup) > 0
}

// ToggleLegacyDress 依次切换 着衣→下着→全裸→着衣，并立即重画。
func (d *BsStageDirector) ToggleLegacyDress() bool {
	if len(d.legacyCl) > 0 {
		d.legacyClWear = (d.legacyClWear + 1) % len(clBustupWearSuffixes)
		artworks := d.buildLegacyClBustup(d.legacyCl, d.legacyClX, d.legacyClY)
		if len(artworks) > 0 {
			group := d.groupKey(d.legacyCl)
			table := d.current()
			entry := state.NewBsLayerEntry(group, d.legacyClX, d.legacyClY, state.StageLayerCharacter, artworks)
			if index := indexOfBsLayer(table, group); index >= 0 {
				table[index] = entry
			} else {
				table = append(table, entry)
			}
			d.commit(table, "rev5 cl dress wear="+itoaBs(d.legacyClWear))
		}
		core.Logf("ui: rev5 cl dress wear=%d", d.legacyClWear)
		return true
	}
	if len(d.legacyBustup) == 0 {
		return false
	}
	d.legacyWear = (d.legacyWear + 1) % bsLegacyWearCount
	d.showLegacyBustup(d.legacyBustup, d.legacyBustupX, d.legacyBustupY)
	core.Logf("ui: rev5 dress wear=%d", d.legacyWear)
	return true
}

// CycleLegacyView 切换「顔から胸 / 胸から股間 / 股間から太もも」三档视点。
//
// 官方说明里视点按钮只在近距离可用；远距离时保留了按钮但忽略点击，
// 返回 true 表示"按钮已经消费"，避免落到其它作品的视点逻辑上。
func (d *BsStageDirector) CycleLegacyView() bool {
	if len(d.legacyBustup) == 0 {
		return false
	}
	if d.legacyBustup[bsLegacyCsDistance] != bsLegacyNearDistance {
		core.Logf("ui: rev5 view ignored in far distance")
		return true
	}
	d.legacyView = (d.legacyView + 1) % bsLegacyViewCount
	d.showLegacyBustup(d.legacyBustup, d.legacyBustupX, d.legacyBustupY)
	core.Logf("ui: rev5 view=%d", d.legacyView)
	return true
}

func (d *BsStageDirector) resolveKdPatchName(name string) string {
	if d.existsExactly(name) {
		return name
	}
	if len(name) < 9 {
		return name
	}
	first := []byte(name)
	first[5] = '0'
	candidate := string(first)
	if d.existsExactly(candidate) {
		return candidate
	}
	first[6] = '0'
	first[7] = '1'
	first[8] = '1'
	candidate = string(first)
	if d.existsExactly(candidate) {
		return candidate
	}
	return name
}

func (d *BsStageDirector) resolveVariant(name string) string {
	if len(name) == 0 || isBsSpecialName(name) || d.existsExactly(name) {
		return name
	}
	for digit := bsVariantFirst; digit <= bsVariantLast; digit++ {
		candidate := name + string(rune('0'+digit))
		if d.existsExactly(candidate) {
			return candidate
		}
	}
	return name
}

func (d *BsStageDirector) tryDeriveBustupBase(name string) (string, bool) {
	if !d.isRev() || len(name) < 2 || !isBsAsciiDigit(name[len(name)-1]) {
		return name, false
	}
	var baseName string
	switch d.script.Dialect() {
	case vmbs.BsScriptRev3:
		baseName = name[:len(name)-2] + bsRev3BustupSuffix
	case vmbs.BsScriptRev4:
		if name[len(name)-2] == bsRev4BustupMarker {
			baseName = name[:len(name)-2]
		} else {
			baseName = name[:len(name)-1] + bsRev5BustupSuffix
		}
	case vmbs.BsScriptRev5:
		baseName = name[:len(name)-1] + bsRev5BustupSuffix
	case vmbs.BsScriptRev6:
		baseName = name[:len(name)-2]
	default:
		return name, false
	}
	if len(baseName) == 0 || baseName == name {
		return name, false
	}
	return baseName, true
}

func (d *BsStageDirector) tryRev4Bustup(name string) (string, bool) {
	if d.script.Dialect() != vmbs.BsScriptRev4 || len(name) < 3 || name[len(name)-2] != bsRev4BustupMarker || !isBsAsciiDigit(name[len(name)-1]) {
		return name, false
	}
	baseName := name[:len(name)-2]
	if !d.exists(baseName) {
		return name, false
	}
	return baseName, true
}

func (d *BsStageDirector) groupKey(name string) string {
	switch d.script.Dialect() {
	case vmbs.BsScriptRev6:
		if len(name) >= 4 {
			return "BU" + name[2:4]
		}
		return name
	case vmbs.BsScriptRev5:
		// cs 立绘用独立的 CS 前缀，避免和同作品里可能存在的 bu 图层
		// 共用 "BU0x" 键而互相顶掉。
		if d.isLegacyCsBustup(name) && len(name) >= 3 {
			return "CS" + name[2:3]
		}
		if len(name) >= 3 {
			return "BU0" + name[2:3]
		}
		return name
	case vmbs.BsScriptRev3:
		if derived, ok := d.tryDeriveBustupBase(name); ok {
			return derived
		}
		return name
	default:
		if len(name) <= bsGroupKeyLength {
			return name
		}
		return name[:bsGroupKeyLength]
	}
}

func (d *BsStageDirector) swapLayers(first, second int) {
	left, ok := d.tryResolveLayer(first)
	if !ok {
		return
	}
	right, ok := d.tryResolveLayer(second)
	if !ok {
		return
	}
	if !d.st.SwapBsLayers(left, right) {
		core.Logf("gfx: layer %d and %d not swapped", first, second)
		return
	}
	core.Logf("gfx: layer %d <-> %d swapped depth=%d", first, second, len(d.st.BsLayers()))
}

func (d *BsStageDirector) setLayerVisibility(ordinal, mode int) {
	slot, ok := d.tryResolveLayer(ordinal)
	if !ok {
		return
	}
	name := d.st.BsLayers()[slot].Name
	switch mode {
	case bsDeleteVisibility:
		if d.st.RemoveBsLayer(slot) {
			core.Logf("gfx: layer %d '%s' removed depth=%d", ordinal, name, len(d.st.BsLayers()))
		}
	case bsHideVisibility:
		d.st.SetBsLayerVisible(slot, false)
		core.Logf("gfx: layer %d '%s' hidden", ordinal, name)
	case bsShowVisibility:
		d.st.SetBsLayerVisible(slot, true)
		core.Logf("gfx: layer %d '%s' visible", ordinal, name)
	default:
		core.Logf("gfx: layer %d '%s' visibility mode %d ignored", ordinal, name, mode)
	}
}

func (d *BsStageDirector) moveLayer(ordinal, x, y int) {
	slot, ok := d.tryResolveLayer(ordinal)
	if !ok {
		return
	}
	entry := d.st.BsLayers()[slot]
	if !d.st.SetBsLayer(slot, entry.Moved(x, y)) {
		core.Logf("gfx: layer %d '%s' not moved", ordinal, entry.Name)
		return
	}
	core.Logf("gfx: layer %d '%s' position (%d,%d)", ordinal, entry.Name, x, y)
}

func (d *BsStageDirector) applyFilter(ordinal, kind, parameter int) {
	if kind < 0 || kind > bsFilterKindMax {
		core.Logf("gfx: filter kind %d on layer %d ignored", kind, ordinal)
		return
	}
	name := bsFilterSuffix + itoaBs(kind)
	if kind < len(bsFilterNames) {
		name = bsFilterNames[kind]
	}
	core.Logf("gfx: filter '%s' layer=%d kind=%d param=%d not supported, ignored", name, ordinal, kind, parameter)
}

func (d *BsStageDirector) applyLayerReplacement() state.SessionStep {
	command := d.machine.Command()
	name := ""
	if command.Second >= 0 {
		name = d.script.Name(int(command.Second))
	}
	slot, ok := d.tryResolveLayer(int(command.First))
	if len(name) == 0 || !ok {
		return state.SessionContinue
	}
	entry := d.st.BsLayers()[slot]
	artworks := d.resolve(name, entry.X, entry.Y)
	if !d.exists(name) {
		core.Logf("gfx: layer %d '%s' image '%s' not found", int(command.First), entry.Name, name)
	}
	d.st.SetBsLayer(slot, entry.WithArtworks(name, artworks))
	core.Logf("gfx: layer %d '%s' -> '%s' (%d,%d)", int(command.First), entry.Name, name, entry.X, entry.Y)
	return state.SessionContinue
}

func (d *BsStageDirector) tryResolveLayer(ordinal int) (int, bool) {
	if d.script.Dialect() == vmbs.BsScriptRev4 {
		name := ""
		if ordinal >= 0 {
			name = d.script.Name(ordinal)
		}
		if slot := d.st.IndexOfBsLayer(name); slot >= 0 {
			return slot, true
		}
	}
	if ordinal >= 0 && ordinal < len(d.st.BsLayers()) {
		return ordinal, true
	}
	core.Logf("gfx: layer index %d out of range (count=%d)", ordinal, len(d.st.BsLayers()))
	return -1, false
}

func (d *BsStageDirector) update(table []state.BsLayerEntry, name string, artworks []state.LayerArtwork, x, y int) []state.BsLayerEntry {
	index := indexOfBsLayer(table, name)
	if index >= 0 {
		entry := table[index]
		entry.X = x
		entry.Y = y
		entry.Artworks = artworks
		table[index] = entry
		return table
	}
	return d.add(table, name, artworks, x, y)
}

func (d *BsStageDirector) add(table []state.BsLayerEntry, name string, artworks []state.LayerArtwork, x, y int) []state.BsLayerEntry {
	return d.addEntry(table, name, d.resolveArtworks(artworks), x, y)
}

func (d *BsStageDirector) addEntry(table []state.BsLayerEntry, name string, artworks []state.LayerArtwork, x, y int) []state.BsLayerEntry {
	resolved := artworks
	if !d.anyArtworkExists(resolved) {
		core.Logf("gfx: layer '%s' not found, skipped", name)
		return table
	}
	return append(table, state.NewBsLayerEntry(name, x, y, state.StageLayerCharacter, resolved))
}

func (d *BsStageDirector) resolveArtworks(artworks []state.LayerArtwork) []state.LayerArtwork {
	if !d.usesVariantSuffix() || len(artworks) == 0 {
		return artworks
	}
	changed := false
	resolved := make([]state.LayerArtwork, len(artworks))
	for index, artwork := range artworks {
		image := d.resolveVariant(artwork.Image)
		if image != artwork.Image {
			changed = true
		}
		artwork.Image = image
		resolved[index] = artwork
	}
	if !changed {
		return artworks
	}
	return resolved
}

func patchBsArtwork(name string, x, y int) state.LayerArtwork {
	return state.StripLayerArtwork(name, x, y, state.BustupEyePartIndex)
}

func (d *BsStageDirector) commit(table []state.BsLayerEntry, note string) {
	d.st.SetBsLayers(table)
	core.Logf("gfx: %s depth=%d", note, len(table))
}

func (d *BsStageDirector) current() []state.BsLayerEntry {
	table := make([]state.BsLayerEntry, 0, len(d.st.BsLayers())+1)
	return append(table, d.st.BsLayers()...)
}

func indexOfBsLayer(table []state.BsLayerEntry, name string) int {
	for index := range table {
		if table[index].Name == name {
			return index
		}
	}
	return -1
}

func indexOfBsLayerOwner(table []state.BsLayerEntry, image string) int {
	for index := range table {
		if table[index].Name == image {
			return index
		}
		for _, artwork := range table[index].Artworks {
			if artwork.Image == image {
				return index
			}
		}
	}
	return -1
}

func (d *BsStageDirector) resolve(name string, x, y int) []state.LayerArtwork {
	image := name
	if d.usesVariantSuffix() {
		image = d.resolveVariant(name)
	}
	return []state.LayerArtwork{state.NewLayerArtwork(image, x, y)}
}

func (d *BsStageDirector) usesVariantSuffix() bool {
	switch d.script.Dialect() {
	case vmbs.BsScriptAncientWide, vmbs.BsScriptAncientCompact, vmbs.BsScriptRev2:
		return true
	}
	return false
}

func (d *BsStageDirector) isRev() bool {
	return vmbs.RevIsRev(d.script.Dialect())
}

func (d *BsStageDirector) isAncient() bool {
	switch d.script.Dialect() {
	case vmbs.BsScriptAncientWide, vmbs.BsScriptAncientCompact:
		return true
	}
	return false
}

func (d *BsStageDirector) exists(name string) bool {
	if len(name) == 0 {
		return false
	}
	return d.resources.Exists(name) || isBsSpecialName(name)
}

func (d *BsStageDirector) existsExactly(name string) bool {
	return len(name) > 0 && (d.resources.ContainsExact(name) || isBsSpecialName(name))
}

func isBsSpecialName(name string) bool {
	if strings.EqualFold(name, state.StageNameBlack) || strings.EqualFold(name, state.StageNameWhite) || strings.EqualFold(name, state.StageNameTransparent) || strings.EqualFold(name, state.StageNameReserved) {
		return true
	}
	_, _, _, _, ok := state.StageNameTryParseColor(name)
	return ok
}

func (d *BsStageDirector) isCompact() bool {
	return d.script.Dialect() == vmbs.BsScriptAncientCompact
}

func bandBsOverlay(image string) state.StageOverlay {
	return state.StageOverlay{Image: image, Anchor: state.StageOverlayBottomBand}
}

func (d *BsStageDirector) hitOverlay() state.StageOverlay {
	if d.isCompact() {
		return state.StageOverlay{Image: bsInformationImage, Anchor: state.StageOverlayTopLeft, X: bsOverlayMargin, Y: bsOverlayMargin}
	}
	return bandBsOverlay(bsMessageHitImage)
}

func (d *BsStageDirector) syncOverlays() {
	overlays := make([]state.StageOverlay, 0, 3)
	if d.frame != nil && d.st.ShowMessageFrame() {
		overlays = append(overlays, *d.frame)
	}
	if d.hit != nil {
		overlays = append(overlays, *d.hit)
	}
	if d.close != nil {
		overlays = append(overlays, *d.close)
	}
	d.st.SetOverlays(overlays)
}

func isBsAsciiDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func (d *BsStageDirector) anyArtworkExists(artworks []state.LayerArtwork) bool {
	for _, artwork := range artworks {
		if d.existsArtwork(artwork.Image) {
			return true
		}
	}
	return false
}

func (d *BsStageDirector) existsArtwork(image string) bool {
	return len(image) > 0 && (d.resources.Exists(image) || isBsSpecialName(image))
}

func itoaBs(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		position--
		buffer[position] = '-'
	}
	return string(buffer[position:])
}
