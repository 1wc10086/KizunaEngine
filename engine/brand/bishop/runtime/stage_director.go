package runtime

import (
	"math"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

const (
	stageMapButtonType      = 160
	stageBustupType         = 3
	stageOverlayType        = 2
	stageBackgroundType     = 1
	stageMapBaseType        = 15
	stageExtendedBackground = 0x81
)

type resourceIndexAdapter struct {
	index *resource.GameResourceIndex
}

func (a resourceIndexAdapter) ContainsExact(name string) bool {
	if a.index == nil {
		return false
	}
	return a.index.ContainsExact(name)
}

func (a resourceIndexAdapter) TryGet(name string) (state.ResourceLocation, bool) {
	if a.index == nil {
		return nil, false
	}
	location, ok := a.index.TryGet(name)
	if !ok {
		return nil, false
	}
	return location, true
}

func (a resourceIndexAdapter) ArchivesInCategory(category string) []resource.Archive {
	if a.index == nil {
		return nil
	}
	return a.index.ArchivesInCategory(category)
}

type StageDirector struct {
	machine       *vm.BsxMachine
	script        *vm.BsxScriptFile
	state         *state.GameState
	configuration *config.BsxxConfiguration
	resources     state.ResourceIndex
	// foregroundSlots 记录当前舞台已经铺了几个 IMAGE_EX 前景部件。
	//
	// 原引擎把 IMAGE_EX type=2 的每个部件放在独立的层上（第 1 个=层 1，
	// 第 2 个=层 2…），剧本随后用 CHANGE_IMAGE/ SHOW_LAYER 的层号精确地
	// 换图或隐藏。黒の教室 的地图就是靠这个机制控制"哪些地点有事件/搜索"：
	// 全部 11 个条先画出来，再把当前不可用的层 SHOW_LAYER(-1) 隐藏。
	// 换背景（IMAGE 层 1）会清空舞台，计数同时归零。
	foregroundSlots int
	// mapMasks 缓存地图卡的 _hit 掩码：整块 1024x768 索引图解压不便宜，
	// 而地图每次重画都会重新登记同一批卡。掩码是不可变数据，可以长期复用。
	mapMasks map[string]*state.StageHitMask
	// customMapButtonID 是 [custom map phase] result table 的登记序号。
	//
	// OH 的 [button object] 单列只写帧数，按钮值要按"本屏第几个 IMAGE_EX
	// type=160"查 result table。背景重画时归零，保证每张地图从头计数。
	customMapButtonID int
	// mapLayerPlacements 记录 LAYER_PARAM type=0 给地图卡层下发的位置。
	//
	// 放課後3 的顺序是"先 CHAR 登记卡（坐标 0,0），最后统一用层号摆位"。
	// 如果只把位置写到已经存在的按钮上，遇到"先摆位、后登记卡"或下一轮
	// 重画时就会丢位置；按层号记住目标点，登记卡时直接套用，两种顺序都正确。
	mapLayerPlacements map[int]mapLayerPlacement
	// bustupOverride 是 DRESS 画面选好的服装/脱衣阶段对剧本立绘图名的
	// 覆盖函数。剧本后续再画同一个角色时也要用新状态，所以在解析前调用。
	// nil 表示没有设置过（其它作品/没打开过 DRESS），行为完全不变。
	bustupOverride func(string) string
}

// SetBustupOverride 设置 DRESS 的立绘图名覆盖函数。
func (d *StageDirector) SetBustupOverride(override func(string) string) {
	d.bustupOverride = override
}

// applyBustupOverride 在解析前套用 DRESS 覆盖。
func (d *StageDirector) applyBustupOverride(name string) string {
	if d.bustupOverride == nil || len(name) == 0 {
		return name
	}
	return d.bustupOverride(name)
}

// mapLayerPlacement 是一层地图卡的目标屏幕坐标（左上角）。
type mapLayerPlacement struct {
	x int
	y int
}

func NewStageDirector(machine *vm.BsxMachine, script *vm.BsxScriptFile, gameState *state.GameState, configuration *config.BsxxConfiguration, resources *resource.GameResourceIndex) *StageDirector {
	return &StageDirector{
		machine:            machine,
		script:             script,
		state:              gameState,
		configuration:      configuration,
		resources:          resourceIndexAdapter{index: resources},
		mapMasks:           map[string]*state.StageHitMask{},
		mapLayerPlacements: map[int]mapLayerPlacement{},
	}
}

// ShowImage 处理 IMAGE（0x1E）。
//
// 现代 BSXScript 的第二个参数不是“层号”，而是图片类型/用途：
//
//	1     背景（清台后铺到层 0）
//	2     前景叠图（每张占一个独立层，可多层同时可见）
//	15    全屏底图（KZK / 屈辱2 的地图底图、过场黑幕）
//	160   地图按钮（与 IMAGE_EX type=160 同义）
//	其他  仍按原来的“层号 - 1”放置（16/17/18/11 等分层事件 CG）
//
// 旧实现一律当层号用：KZK 的地图卡（type=160）被放到越界的最后一层，
// 后一张覆盖前一张，且从未登记成按钮，于是地图只剩最后一张卡、点击
// 完全无效；type=2 的“已完成”标记也复用同一层互相覆盖。
// KZK / 屈辱2 的底图用 type=15；若仍按层 14 画，会盖住 CHAR 铺的
// 角色状态条（层 2..8），地图显示就是错的。
func (d *StageDirector) ShowImage() state.SessionStep {
	index := d.machine.Frame(1)
	name := d.script.GetName(int(index))
	if d.script.Dialect().IsAncient() {
		sub := d.machine.Frame(3)
		if sub == 0 || len(name) == 0 {
			d.state.ReleaseScreen(int(d.machine.Frame(2)))
			core.Logf("gfx: ancient layer %d released '%s'", d.machine.Frame(2), name)
			return state.SessionContinue
		}
		core.Logf("gfx: ancient layer %d <- '%s' sub=%d", d.machine.Frame(2), name, sub)
		d.applyImage(int(d.machine.Frame(2)), name)
		return state.SessionContinue
	}
	subcode := int(d.machine.Frame(2))
	switch subcode {
	case stageMapButtonType:
		if len(name) > 0 {
			d.addMapButton(name, 0, 0, -1)
		}
		return state.SessionContinue
	case stageOverlayType:
		// 普通 IMAGE 没有坐标语义：KZK 的卡图/已完成标记都是整屏叠图，
		// 位置由图片内容决定；frame[3]/[4] 在多条命令之间会残留旧值，
		// 不能拿来当绘制坐标。
		return d.appendScreenImage(stageOverlayType, name, 0, 0)
	case stageBackgroundType, stageMapBaseType, stageExtendedBackground:
		d.applyImage(int(state.BackgroundSlot), name)
		return state.SessionContinue
	}
	d.applyImage(subcode, name)
	return state.SessionContinue
}

func (d *StageDirector) ShowImageExtended() state.SessionStep {
	name := d.script.GetName(int(d.machine.Frame(1)))
	imageType := d.machine.Frame(2)
	x := int(d.machine.Frame(3))
	y := int(d.machine.Frame(4))
	if imageType == stageMapButtonType && len(name) > 0 {
		if x >= d.screenWidth() {
			x, y = d.animatedButtonPosition(name, 11)
		}
		// 负坐标是"先把按钮停在屏幕外，稍后用 LAYER_PARAM type=0 摆位"
		// 的哨兵写法。学園3/学園3IE 的行动卡正是 `IMAGE_EX type=160,
		// x=-800`，随后按 #layercount（6、7…）逐张摆到 144/272/400… 的
		// 卡位上。按钮必须和普通前景图共用同一个自动层号，否则
		// MoveButtonByLayer 找不到它，五张卡会永远停在屏幕外——画面上
		// 只剩 "Action slot" 底板，玩家无法选择任何行动。
		layer := -1
		// 教育指導 IE 的目标卡在 (0,0) 登记，随后用 LAYER_PARAM 按
		// #target.no（1 基）摆位。必须给每张卡分配连续层号，否则
		// MoveButtonByLayer 找不到按钮，所有卡都会叠在左上角。
		if x < 0 || d.customMapTargetCards() || d.mapLayeredButtons() {
			// OHIE 先用 IMAGE_EX 登记卡，再用 LAYER_PARAM type=0 subtype=5/6
			// 按层号把卡摆到最终位置。没有层号的按钮无法被移动，卡片会
			// 停在脚本给的粗略坐标（时间推进按钮甚至停在 y=-116 屏幕外）。
			layer = d.nextForegroundPlane()
		}
		d.addMapButton(name, x, y, layer)
		return state.SessionContinue
	}
	if imageType == stageBustupType && state.BustupIsBustupName(name) {
		return d.PlaceCharacter(int(imageType)-1, name, x, y)
	}
	if len(name) > 0 {
		return d.appendScreenImage(int(imageType), name, x, y)
	}
	d.applyImage(int(imageType), name)
	return state.SessionContinue
}

// appendScreenImage 处理 IMAGE_EX 的前景部件（type 2 等）。
//
// 原引擎的语义是"每个前景部件占一个独立层"：黒の教室 的地图先 IMAGE 铺底，
// 再连续 IMAGE_EX 画 6 条事件条、5 条搜索条、日期卡、时间卡、周数卡和星期卡，
// 依次落在层 1、2、3…；之后剧本用 CHANGE_IMAGE <层号> 换某个条的差分，
// 用 SHOW_LAYER <层号>, -1 把当前不可用的地点条隐藏。
//
// 之前把全部部件堆在同一层，层号信息全丢，SHOW_LAYER 也无处可施，
// 结果地图上所有地点都挂着"Talk & Event / Idea Search"小字，且换图会清掉整层。
// 现在按顺序分配层，每个部件保留自己的坐标。
func (d *StageDirector) appendScreenImage(slot int, name string, x, y int) state.SessionStep {
	if slot <= 0 {
		return state.SessionContinue
	}
	if state.BustupIsBustupName(name) {
		name = d.applyBustupOverride(name)
	}
	artworks := state.BustupResolve(name, x, y, d.configuration.EyePattern, d.resources, d.configuration.HairSuffix)
	if slot == state.BackgroundSlot {
		// 背景层 1 仍是"换整张背景"：清掉舞台再铺，前景部件计数归零。
		d.state.ClearStage()
		d.foregroundSlots = 0
		d.customMapButtonID = 0
		d.state.SetLayerAt(0, name, state.StageLayerScreen, artworks)
		core.Logf("gfx: image_ex background '%s' at (%d,%d)", name, x, y)
		return state.SessionContinue
	}
	plane := d.nextForegroundPlane()
	d.state.SetLayerAt(plane, name, state.StageLayerScreen, artworks)
	core.Logf("gfx: image_ex slot %d <- '%s' at (%d,%d)", plane, name, x, y)
	return state.SessionContinue
}

// nextForegroundPlane 给前景叠图分配一个不会覆盖角色的层。
//
// 前景部件按添加顺序占 1、2、3…，剧本随后用 CHANGE_IMAGE/SHOW_LAYER
// 引用这些层号，所以顺序本身不能改。但 KZK 的地图会把角色状态条
// （CHAR）放在层 2..8，紧接着又把整屏“已完成”标记（IMAGE type=2）
// 叠上来；如果直接占用 2、3…，标记会把状态条整层替换掉。
// 跳过当前仍放着角色图的层即可：既保持原有层号语义，又不破坏角色条。
func (d *StageDirector) nextForegroundPlane() int {
	d.foregroundSlots++
	plane := d.foregroundSlots
	for plane < state.OverlayPlane {
		layer := d.state.LayerAt(plane)
		if len(layer.Artworks()) == 0 || layer.Kind() != state.StageLayerCharacter {
			d.foregroundSlots = plane
			return plane
		}
		plane++
	}
	// 正常剧本只用到十几层；损坏的脚本不能把背景/覆盖层挤掉。
	d.foregroundSlots = state.OverlayPlane - 1
	return state.OverlayPlane - 1
}

// mapButtonMask 取（并缓存）地图卡的 _hit 掩码；没有掩码时缓存 nil。
func (d *StageDirector) mapButtonMask(name string) *state.StageHitMask {
	if d.mapMasks == nil {
		d.mapMasks = map[string]*state.StageHitMask{}
	}
	if mask, ok := d.mapMasks[name]; ok {
		return mask
	}
	mask := loadMapButtonHitMask(d.resources, name)
	d.mapMasks[name] = mask
	return mask
}

// addMapButton 登记一张地图卡。
//
// value 来自 bsxx.cfg 的 [button object]；frames 决定竖排图是 2 帧
// （常态 / 悬停大图）。命中优先用同名 _hit 掩码，没有掩码的作品
// （部分旧作只有矩形）仍走图片外框。
//
// layer >= 0 表示这是 CHAR type=160 登记的分层地图卡。放課後3 的剧本
// 先登记卡、再在同一个 map_main 末尾用 LAYER_PARAM type=0 摆位；把层号
// 记在按钮上，后面的 LAYER_PARAM 才能找到要移动的卡。已经收到过该层
// 位置的（重画/先摆位后登记）直接套用，避免按钮先闪一下左上角。
func (d *StageDirector) addMapButton(name string, x, y, layer int) {
	if layer >= 0 {
		if placement, ok := d.mapLayerPlacements[layer]; ok {
			x, y = placement.x, placement.y
		}
	}
	value := d.configuration.MapButtonValue(name)
	frames := d.configuration.MapButtonFrames(name)
	if d.configuration.CustomMap.HasResultTable() {
		// OH/OHIE：按钮值由 result table 按 ID 查表得到，帧数走这一代
		// 自己的单列/双列约定。OH 的单列 `map_ch1_norm = 5` 是"5 帧"，
		// 旧逻辑把它当值、帧数回退 2，1210px 竖排图会被画成两张半卡。
		d.customMapButtonID++
		id := d.customMapButtonID
		if explicit, ok := d.configuration.MapButtonResultID(name); ok {
			id = explicit
		} else if named, ok := d.configuration.MapButtonNamedID(name); ok {
			// OH 的 hot 卡只画"今天有事件"的角色，登记序号不能当槽位；
			// 图名里的 chN 才是原版的按钮 ID。
			id = named
		}
		if mapped, ok := d.configuration.CustomMap.ResultValue(id); ok {
			value = mapped
		}
		if frames = d.configuration.MapButtonTableFrames(name); frames <= 0 {
			frames = d.configuration.MapButtonFrames(name)
		}
	}
	button := state.NewStageButtonWithFrames(name, x, y, value, frames)
	if hover := d.configuration.MapButtonHoverFrame(name); hover >= 0 {
		button.HoverFrame = hover
		button.HasHoverFrame = true
	}
	button.Layer = layer
	maskNote := ""
	if mask := d.mapButtonMask(name); mask != nil {
		applyMapButtonHitMask(&button, mask)
		maskNote = " hit=" + strconv.Itoa(button.HitWidth) + "x" + strconv.Itoa(button.HitHeight)
	}
	d.state.AddButton(button)
	core.Logf("gfx: map button '%s' (%d,%d) value=%d frames=%d layer=%d%s", name, x, y, value, frames, layer, maskNote)
}

// ShowLayerImage 处理 SHOW_LAYER(0x22)：显示/隐藏指定层。
//
// 地图把每个前景部件放在独立层上，然后在同一条脚本里按当前可用的地点
// 调用 SHOW_LAYER(层号, -1) 隐藏其余部件。Frame(1) 是层号（与 CHANGE_IMAGE
// 一致），Frame(2) < 0 表示隐藏、>= 0 表示显示；正数同时可作为图片名下标，
// 能解析出资源时顺带把该层换成那张图（部分剧本用它显示"已选"状态）。
// 隐藏只影响绘制，内容保留，之后 SHOW_LAYER(层号, 0) 可原样恢复。
func (d *StageDirector) ShowLayerImage() state.SessionStep {
	plane := int(d.machine.Frame(1))
	value := int(d.machine.Frame(2))
	if plane < 0 || plane >= state.PlaneCount {
		return state.SessionContinue
	}
	if value < 0 {
		d.state.SetLayerVisible(plane, false)
		core.Logf("gfx: layer %d hidden", plane)
		return state.SessionContinue
	}
	d.state.SetLayerVisible(plane, true)
	// value==0 只表示"显示"；0 号名字表项常常是 black，直接换图会把层刷黑。
	// 只有正数才当图片名下标处理。
	if value > 0 {
		if name := d.script.GetName(value); len(name) > 0 {
			if state.BustupIsBustupName(name) {
				name = d.applyBustupOverride(name)
			}
			layer := d.state.LayerAt(plane)
			x, y := 0, 0
			if pieces := layer.Artworks(); len(pieces) > 0 {
				x, y = pieces[0].X, pieces[0].Y
			}
			artworks := state.BustupResolve(name, x, y, d.configuration.EyePattern, d.resources, d.configuration.HairSuffix)
			d.state.SetLayerAt(plane, name, layer.Kind(), artworks)
		}
	}
	core.Logf("gfx: layer %d shown", plane)
	return state.SessionContinue
}

func (d *StageDirector) ApplySelect() state.SessionStep {
	if len(d.state.Buttons()) == 0 {
		return state.SessionContinue
	}
	d.state.BeginSelect()
	core.Logf("select: %d map target(s)", len(d.state.Buttons()))
	return state.SessionAwaitSelect
}

// AttachAnimation 把 *_anim_sc 播放脚本挂到刚设置的动态 CG 图层上。
//
// モーションプラス 的脚本顺序是：
//
//	Image(subcode=2, "ev01_001_3_anim")   // 图层 1 放动画容器
//	Attach(layer=1, "ev01_001_3anim_sc")  // 同一层挂播放脚本
//
// 容器本身是"上百张全屏帧"的特殊 BSS-Composition，不能当静态图展开；
// 这里建立按帧读取的播放器，渲染器每帧取当前画面。
func (d *StageDirector) AttachAnimation() state.SessionStep {
	plane := int(d.machine.Frame(1))
	scriptName := d.script.GetName(int(d.machine.Frame(2)))
	if plane < 0 || plane >= len(d.state.Layers()) || scriptName == "" {
		return state.SessionContinue
	}
	name := animationArtworkName(d.state.LayerAt(plane))
	if name == "" {
		core.Logf("gfx: attach '%s' to layer %d ignored (no *_anim image)", scriptName, plane)
		return state.SessionContinue
	}
	image, ok := d.resources.TryGet(name)
	if !ok || image == nil {
		core.Logf("gfx: animation image '%s' missing", name)
		return state.SessionContinue
	}
	script, ok := d.resources.TryGet(scriptName)
	if !ok || script == nil {
		core.Logf("gfx: animation script '%s' missing", scriptName)
		return state.SessionContinue
	}
	player, err := rendering.NewBsgAnimationPlayer(image, script.Read())
	if err != nil {
		core.Logf("gfx: animation '%s' failed: %v", name, err)
		return state.SessionContinue
	}
	d.state.SetLayerAnimator(plane, player)
	core.Logf("gfx: animation layer %d <- '%s' script='%s'", plane, name, scriptName)
	return state.SessionContinue
}

// animationArtworkName 取图层上最后一个 *_anim 图名。
func animationArtworkName(layer *state.StageLayer) string {
	if layer == nil {
		return ""
	}
	name := ""
	for _, artwork := range layer.Artworks() {
		if strings.HasSuffix(strings.ToLower(artwork.Image), "_anim") {
			name = artwork.Image
		}
	}
	return name
}

func (d *StageDirector) ChangeImage() state.SessionStep {
	plane := d.machine.Frame(1)
	name := d.script.GetName(int(d.machine.Frame(2)))
	d.applyChange(int(plane), name)
	return state.SessionContinue
}

func (d *StageDirector) ReleaseLayer() state.SessionStep {
	plane := d.machine.Frame(1)
	if plane < 0 {
		return state.SessionContinue
	}
	d.state.ClearLayer(int(plane))
	core.Logf("gfx: layer %d released", plane)
	return state.SessionContinue
}

// ShowCharacter 处理 CHAR(0x39)。
//
// CHAR 的第三个参数是图片用途，和 IMAGE/IMAGE_EX 的 type 同义：
//
//	2    前景叠图
//	160  地图/选项按钮
//	其它 立绘
//
// mros 的事件三选一把选项条写成 `CHAR type=160, x=1024, y=0`：x=1024
// 是“屏幕外”哨兵，真实落点来自 define.bsa 的 select1/2/3.txt 动画脚本。
// 旧实现一律当立绘放到 (1024,0)，选项条既不可见也不可点，事件结束后
// 地图会停在 0 个热区上，玩家永久卡死。这里按用途分流：160 注册成
// 按钮，2 叠到前景层，其余仍走立绘。
func (d *StageDirector) ShowCharacter() state.SessionStep {
	plane := int(d.machine.Frame(1))
	name := d.script.GetName(int(d.machine.Frame(2)))
	subcode := int(d.machine.Frame(3))
	x := int(d.machine.Frame(4))
	y := int(d.machine.Frame(5))
	if len(name) > 0 && (subcode == stageMapButtonType || subcode == stageOverlayType) {
		if x >= d.screenWidth() {
			x, y = d.animatedButtonPosition(name, plane)
		}
		if subcode == stageMapButtonType {
			d.addMapButton(name, x, y, plane)
			return state.SessionContinue
		}
		return d.appendScreenImage(subcode, name, x, y)
	}
	return d.PlaceCharacter(plane, name, x, y)
}

func (d *StageDirector) screenWidth() int {
	if d.configuration != nil && d.configuration.Width > 0 {
		return d.configuration.Width
	}
	return 1024
}

// animatedButtonPosition 解析“从屏幕外滑入”的按钮最终位置。
//
// 参数 x 只是为了表明调用方已经判定它是哨兵值（>= 屏幕宽）；真正的
// 落点由同名 txt（selectok/selectcancel）或 select1/2/3.txt 决定。
// 脚本里的 pos 是按钮中心点，所以要减去图片半宽/半高换算成左上角。
// 找不到脚本时水平居中、顶部对齐，至少不会把按钮丢到屏幕外。
func (d *StageDirector) animatedButtonPosition(name string, plane int) (int, int) {
	width, height := d.buttonImageSize(name)
	centerX, centerY, ok := d.animationFinalPosition(name, plane)
	if !ok {
		return (d.screenWidth() - width) / 2, 0
	}
	return centerX - width/2, centerY - height/2
}

func (d *StageDirector) mapButtonFrames(name string) int {
	if d.configuration == nil {
		return 0
	}
	return d.configuration.MapButtonFrames(name)
}

// buttonImageSize 取按钮图一帧的像素尺寸；读不到时返回 0。
func (d *StageDirector) buttonImageSize(name string) (int, int) {
	if d.resources == nil {
		return 0, 0
	}
	location, ok := d.resources.TryGet(name)
	if !ok || location == nil {
		return 0, 0
	}
	width, height, ok := gfx.TryReadBsgDimensions(location.Read())
	if !ok {
		return 0, 0
	}
	if frames := d.mapButtonFrames(name); frames > 1 {
		height /= frames
	}
	return width, height
}

// animationFinalPosition 读取按钮演出脚本最后一帧的中心坐标。
//
// 脚本是 `<layer ...><frame pos="512 200"/></layer>` 的简化 XML；只取
// 最后一个带 pos 的 frame。找不到同名脚本时，select_ 选项条回退到
// select1/2/3（同一屏的三行）。
func (d *StageDirector) animationFinalPosition(name string, plane int) (int, int, bool) {
	for _, script := range d.animationScriptCandidates(name, plane) {
		if x, y, ok := d.readAnimationPosition(script); ok {
			return x, y, true
		}
	}
	return 0, 0, false
}

func (d *StageDirector) animationScriptCandidates(name string, plane int) []string {
	candidates := []string{name}
	if strings.HasPrefix(strings.ToLower(name), "select_") {
		row := 0
		if plane >= 11 {
			row = (plane - 11) % 3
		}
		candidates = append(candidates, "select"+strconv.Itoa(row+1))
	}
	return candidates
}

func (d *StageDirector) readAnimationPosition(name string) (int, int, bool) {
	if d.resources == nil {
		return 0, 0, false
	}
	location, ok := d.resources.TryGet(name + ".txt")
	if !ok || location == nil {
		return 0, 0, false
	}
	text := string(location.Read())
	index := strings.LastIndex(text, `pos="`)
	if index < 0 {
		return 0, 0, false
	}
	rest := text[index+len(`pos="`):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return 0, 0, false
	}
	parts := strings.Fields(rest[:end])
	if len(parts) < 2 {
		return 0, 0, false
	}
	x, errX := strconv.Atoi(parts[0])
	y, errY := strconv.Atoi(parts[1])
	if errX != nil || errY != nil {
		return 0, 0, false
	}
	return x, y, true
}

func (d *StageDirector) PlaceCharacter(plane int, name string, x, y int) state.SessionStep {
	if state.StageNameIsEmpty(name) {
		d.state.ClearLayer(plane)
		core.Logf("gfx: character %d cleared", plane)
		return state.SessionContinue
	}
	resolvedY := y
	if state.BustupIsStanding(name) {
		resolvedY = d.configuration.StandingOffsetFor(name)
	}
	name = d.applyBustupOverride(name)
	artworks := state.BustupResolve(name, x, resolvedY, d.configuration.EyePattern, d.resources, d.configuration.HairSuffix)
	d.state.SetLayerAt(plane, name, state.StageLayerCharacter, artworks)
	core.Logf("gfx: character %d <- '%s' (%d part(s) y=%d)", plane, name, len(artworks), resolvedY)
	return state.SessionContinue
}

func (d *StageDirector) ApplyMask() state.SessionStep {
	name := d.script.GetName(int(d.machine.Frame(3)))
	maskType := d.machine.Frame(1)
	if len(name) == 0 {
		return state.SessionContinue
	}
	d.state.SetMask(name)
	core.Logf("gfx: mask '%s' type=%d", name, maskType)
	return state.SessionContinue
}

func (d *StageDirector) ApplyTransition() state.SessionStep {
	transitionType := int(d.machine.Frame(1))
	if transitionType == state.NoTransition {
		d.state.EndTransition()
		return state.SessionContinue
	}
	duration := float64(d.machine.Frame(2)) * 0.1
	d.state.StartTransition(transitionType, duration)
	mask, _ := d.state.MaskImage()
	core.Logf("gfx: transition type=%d %gs mask='%s'", transitionType, duration, mask)
	return state.SessionContinue
}

func (d *StageDirector) ApplyLayerParameter() state.SessionStep {
	switch d.machine.Frame(2) {
	case 0:
		// type=0 的第三参数是子类型：
		//   放課後3 用 0 + (x,y) 摆地图卡；
		//   教育指導 IE 的目标卡用 5 + (x,y) 把选中的卡移到中央；
		//   SMIE / UKIE 用 3/5/8 做层演出（只给一个偏移量），语义不是
		//   "移到屏幕坐标"。只在作品明确配置了目标卡（[custom map phase]
		//   result 指向 #target）时才把子类型 5 当摆位，其它作品保持忽略。
		subtype := int(d.machine.Frame(3))
		plane := int(d.machine.Frame(1))
		x := int(d.machine.Frame(4))
		y := int(d.machine.Frame(5))
		switch {
		case subtype == 0:
			d.applyLayerPlacement(plane, x, y)
		case subtype == 5 && d.customMapTargetCards():
			d.applyLayerPlacement(plane, x, y)
		case d.mapLayeredButtons() && (subtype == 5 || subtype == 6):
			// OH/OHIE 用 subtype 5/6 给地图卡摆位：subtype 5 是常态卡，
			// 6 是「時間を進める」从上方滑入后的最终位置。只有确实是
			// map_ 图层或按钮层才处理——同一部作品的 H 场景也用 subtype 5
			// 做层演出，那些层不是地图卡，不能把人物整层挪走。
			if d.layerHasButton(plane) || d.layerIsMapCard(plane) {
				d.applyLayerPlacement(plane, x, y)
			}
		}
		return state.SessionContinue
	case 1, 2:
		d.applyAlpha(int(d.machine.Frame(1)), toScalar(d.machine.Frame(6)))
		return state.SessionContinue
	case 6:
		// type=6 是"给这一层挂入场动画"，参数 frame[3] 是 define.bsa 里的
		// 动画脚本名。地图卡在本引擎里是按钮（位置由 type=0 给出），动画
		// 只负责翻转入场；普通图层则按脚本做移动/淡入淡出（牝教師4 的
		// A-H-E アヘ顔特写就是这一种）。
		name := ""
		if d.script != nil {
			name = d.script.GetName(int(d.machine.Frame(3)))
		}
		plane := int(d.machine.Frame(1))
		if len(name) > 0 && !d.layerHasButton(plane) && d.applyLayerMotion(plane, name) {
			return state.SessionContinue
		}
		if len(name) > 0 {
			core.Logf("gfx: layer %d entrance animation '%s' ignored (map card is a button)", plane, name)
		}
		// 目标卡的 type=6 同时带最终坐标 frame[4]/[5]；动画可以忽略，
		// 但卡片必须落到目标位置。其它作品的 type=6 不带坐标语义，
		// 只在 TargetCards 作品里套用。
		if d.customMapTargetCards() {
			d.applyLayerPlacement(int(d.machine.Frame(1)), int(d.machine.Frame(4)), int(d.machine.Frame(5)))
		}
		return state.SessionContinue
	case 7:
		// 这一支的 frame[4] 是 IEEE-754 float 位模式（其它作品的 alpha）。
		// 放課後3 也用 type=7，但 frame[4]=3 表示卡片状态，3 的 float 位
		// 是 denormal（约 4e-45），按 alpha 处理会把层设成几乎全透明。
		// 只有 0 或指数位非零的正常 float 才当 alpha，避免误伤。
		bits := d.machine.Frame(4)
		if bits == 0 || (uint32(bits)>>23)&0xFF != 0 {
			d.applyAlpha(int(d.machine.Frame(1)), toScalar(bits))
		}
		return state.SessionContinue
	default:
		return state.SessionContinue
	}
}

func (d *StageDirector) customMapTargetCards() bool {
	return d.configuration != nil && d.configuration.CustomMap.TargetCards
}

// mapLayeredButtons 表示这一代作品的地图卡需要在登记时就绑定舞台层号。
//
// OH/OHIE 的流程都是 IMAGE_EX 先登记卡、随后 LAYER_PARAM type=0
// subtype=5/6 按层号给出最终坐标（OH 的时间推进按钮从底部移到右上角，
// OHIE 的卡还要从初始的粗略坐标再校正一次）。没有层号（Layer=-1）的
// 按钮无法被移动。result table 是这两代作品独有的标志，用它把行为
// 限制在 OH/OHIE，其它作品不变。
func (d *StageDirector) mapLayeredButtons() bool {
	return d.configuration != nil && d.configuration.CustomMap.HasResultTable()
}

// layerIsMapCard 判断该舞台层是不是脚本画的地图卡/不可用卡。
//
// OH/OHIE 对"今天没有事件"的角色不登记按钮，而是用 IMAGE_EX type=2
// 画一张灰度卡（map_chN_dis）。剧本随后同样用 LAYER_PARAM subtype=5
// 给这个层摆位；只移动按钮会让灰卡留在较低的初始坐标，和原版对齐不上。
func (d *StageDirector) layerIsMapCard(plane int) bool {
	if plane < 0 || plane >= state.PlaneCount {
		return false
	}
	pieces := d.state.LayerAt(plane).Artworks()
	if len(pieces) == 0 {
		return false
	}
	return strings.HasPrefix(strings.ToLower(pieces[0].Image), "map_")
}

// applyLayerPlacement 执行 LAYER_PARAM type=0：把层号 plane 摆到 (x,y)。
//
// 放課後3 的地图流程是：
//
//	CHAR type=160, 层号 6..17      // 登记角色卡（坐标 0,0）
//	LAYER_PARAM 6, 0, _, mapXa, mapYa   // 再按角色/卡片位置摆位
//
// 只移动舞台层不够——卡片本身是按钮，不移动按钮的话所有卡仍叠在
// 左上角，点击也只会命中第一张。这里同时更新两者，并把位置按层号记下来，
// 供同一次重画里稍后登记的按钮直接使用。
func (d *StageDirector) applyLayerPlacement(plane, x, y int) {
	if plane < 0 {
		return
	}
	if d.mapLayerPlacements == nil {
		d.mapLayerPlacements = map[int]mapLayerPlacement{}
	}
	d.mapLayerPlacements[plane] = mapLayerPlacement{x: x, y: y}
	movedButton := d.state.MoveButtonByLayer(plane, x, y)
	movedLayer := d.state.MoveLayerTo(plane, x, y)
	core.Logf("gfx: layer %d placed at (%d,%d) button=%t layer=%t", plane, x, y, movedButton, movedLayer)
}

func (d *StageDirector) ApplyFill() state.SessionStep {
	d.state.SetFillBlue(uint8(d.machine.Frame(1) & 0xFF))
	d.state.SetFillGreen(uint8(d.machine.Frame(2) & 0xFF))
	d.state.SetFillRed(uint8(d.machine.Frame(3) & 0xFF))
	d.state.SetFillAlpha(1.0)
	return state.SessionContinue
}

func (d *StageDirector) applyImage(slot int, name string) {
	if slot <= 0 {
		return
	}
	plane := slot - 1
	if state.StageNameIsEmpty(name) {
		d.state.ClearLayer(plane)
		core.Logf("gfx: layer %d <- reserved", plane)
		return
	}
	if state.BustupIsBustupName(name) {
		name = d.applyBustupOverride(name)
	}
	artworks := state.BustupResolve(name, 0, 0, d.configuration.EyePattern, d.resources, d.configuration.HairSuffix)
	if slot == state.BackgroundSlot {
		d.state.ClearStage()
		// 换背景意味着上一屏的所有前景部件作废，IMAGE_EX 的层号重新从 1 开始。
		d.foregroundSlots = 0
		d.customMapButtonID = 0
		// 层号到屏幕位置的映射也属于上一屏：新地图/新场景的 6 号层可能是
		// 完全不同的卡，沿用旧坐标会把新卡摆到旧地点。
		if len(d.mapLayerPlacements) > 0 {
			clear(d.mapLayerPlacements)
		}
		d.state.SetLayerAt(plane, name, state.StageLayerScreen, artworks)
		core.Logf("gfx: stage cleared, layer %d <- '%s'", plane, name)
		return
	}
	d.state.SetLayerAt(plane, name, state.StageLayerScreen, artworks)
	for _, artwork := range artworks {
		core.Logf("gfx: layer %d <- '%s' at (%d,%d)", plane, artwork.Image, artwork.X, artwork.Y)
	}
}

func (d *StageDirector) applyChange(plane int, name string) {
	if plane < 0 {
		return
	}
	if state.StageNameIsEmpty(name) {
		d.state.ClearLayer(plane)
		core.Logf("gfx: layer %d cleared", plane)
		return
	}
	layer := d.state.LayerAt(plane)
	pieces := layer.Artworks()
	if len(pieces) == 0 {
		// CHANGE_IMAGE 的语义是"把这一层已有的图换成另一张"：层是空的就没有
		// 可换的目标。旧实现会在 (0,0) 新建一张图，地图上某些 CHANGE_IMAGE
		// 因此把多余的条画到左上角。
		core.Logf("gfx: change image on empty layer %d ignored ('%s')", plane, name)
		return
	}
	x := pieces[0].X
	y := pieces[0].Y
	name = d.applyBustupOverride(name)
	artworks := state.BustupResolve(name, x, y, d.configuration.EyePattern, d.resources, d.configuration.HairSuffix)
	// CHANGE_IMAGE 的常见用法是"换掉这一层的那张图"：单图图层直接替换即可。
	// 但地图的 UI 前景层是 IMAGE_EX 累积出来的多层合成（日期、周数、各地点条），
	// 只换其中一个地点条时不能把同层其它部件一起清掉——否则地图上的小字/周卡片
	// 就在切状态的一瞬间消失或跑到别处。此时按"共同前缀"找到要换的那个部件原位替换。
	if len(pieces) > 1 && len(artworks) > 0 {
		if index := bestChangeTarget(pieces, name); index >= 0 {
			replacement := artworks[0]
			replacement.X = pieces[index].X
			replacement.Y = pieces[index].Y
			if d.state.ReplaceLayerArtwork(plane, index, replacement) {
				core.Logf("gfx: layer %d part %d <- '%s' at (%d,%d)", plane, index, name, replacement.X, replacement.Y)
				return
			}
		}
	}
	d.state.SetLayerAt(plane, name, layer.Kind(), artworks)
	core.Logf("gfx: layer %d <- '%s'", plane, name)
}

// bestChangeTarget 在多部件图层里找 CHANGE_IMAGE 要替换的那一个。
//
// 地图状态条命名是 `placeN_event_barX` / `placeN_idea_barX`，换状态时只有
// 末尾的 bar 编号变化；按最长公共前缀匹配能稳定命中同一个地点的条，
// 又不会误伤 `map_week1`、`map_date_base` 这些同类前缀很短的部件。
// 返回 -1 表示没有足够可信的匹配，调用方退回"替换整层"。
func bestChangeTarget(pieces []state.LayerArtwork, name string) int {
	best := -1
	bestLength := 0
	for index, piece := range pieces {
		length := commonPrefixLength(piece.Image, name)
		if length > bestLength {
			bestLength = length
			best = index
		}
	}
	// 至少共享到 `_bar` 这一级（10 个字符以上）才认为同一个部件；
	// 只共享 `place`/`map_` 的属于不同卡片，不能互相替换。
	if bestLength < 10 {
		return -1
	}
	return best
}

func commonPrefixLength(a, b string) int {
	limit := min(len(a), len(b))
	length := 0
	for length < limit && a[length] == b[length] {
		length++
	}
	return length
}

func (d *StageDirector) applyAlpha(plane int, value float64) {
	if math.IsNaN(value) || value < 0 || value > 1 {
		return
	}
	d.state.SetAlpha(plane, value)
}

func toScalar(bits int32) float64 {
	return float64(math.Float32frombits(uint32(bits)))
}
