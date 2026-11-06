package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_buex.go 是「立ち絵鑑賞室」（bust-up gallery）。
//
// 原引擎的鉴赏室不是一个列表，而是一整套用 define.bsa 的 museum_buex_layout
// 描述的自绘画面：顶部主菜单（setting_bgv / bg_select / setting_bu /
// edit_message / data_store / back_to_thumbs）、立ち絵页的六组开关、角色图标、
// 站立位置滑杆、背景选择页、文章编辑页和预设存档页。
//
// 这里做的事和别处一样：布局（底图、按钮、坐标）全部来自 BSA 里的那份文档，
// 引擎只负责按坐标贴图、判命中、维护状态。但是有一处必须自己实现——
// 立绘图名：
//
//	bu<NN><视角><ぬぎぬぎ><服装><姿势><变体>[表情]
//	  NN   两位角色编号（1 起，对应 bu01…bu05）
//	  视角 u = 全身（容器 2350x2990），f = 半身（容器 1024x768）
//	  ぬぎぬぎ 一位脱衣阶段号（0 = 着衣）
//	  服装 两位服装编号（1 起，和角色表 wear 一一对应）
//	  姿势 一位姿势号（1…4）
//	  变体 a / b；表情图按 a=1…12、b=13…24 分组
//	  表情 两位表情编号，可省略
//
// 例：bu01u0011a = 1 号角色、全身、着衣、01 号服、姿势 1、变体 a；
//     bu01u0011a12 = 同一张图加 12 号表情。
//
// 名字拼不出来是常态（不是每个"角色×阶段×服装×姿势×表情"都有素材），
// 所以这里按"精确名字 → 换姿势号 → 换变体字母 → 基础图"的顺序逐个试探，
// 试探不到就退到最基础的 bu<NN>u0011a。

const (
	// buexPageStandee 立ち絵页；buexPageBackground 背景选择页；
	// buexPageEdit 文章编辑页；buexPageStore 预设存档页。
	buexPageStandee    = 0
	buexPageBackground = 1
	buexPageEdit       = 2
	buexPageStore      = 3
)

const (
	// buexViewBust 是左开关的半身（f）图；buexViewClose 是右开关，把同一张
	// 半身图放大，比半身更近。两个档位都用 f 图，不再加载 u 全身图。
	// 旧存档里的 0（全身）由 buexNormalizeView 迁到半身。
	buexViewBust  = 1
	buexViewClose = 2
)

// 开关组在 layout.Setting.Switches 里的固定次序。
//
// 顺序不能改：原文档里六组开关分别是
// ポーズ / 視点 / ぬぎぬぎ / 服装 / 下着 / 表情，
// 而且它们的坐标是分开写的（x=290/195/224/195/429/372），
// 交换顺序会把开关画到别的排布上去。
//
// 旧式 [bu viewer]（SL）还有第 7 组"背景"开关；它排在表情后面，
// 只由 legacy 转换器产生，新版 museum_buex_layout 没有这一组。
const (
	buexSwitchPose = iota
	buexSwitchView
	buexSwitchNgng
	buexSwitchWear
	buexSwitchUnder
	buexSwitchFace
	buexSwitchBackground
	buexSwitchCount
)

const (
	// buexCloseScale 是右开关（近距離）的放大倍数。
	// 原版鉴赏室没有全身档：左开关是半身（f）1:1，右开关把同一张半身图
	// 放大到比半身更近。1.5 倍能明显看出"更近"，又不会把脸切掉一半。
	buexCloseScale = 1.5

	// buexStandeeSway 站位滑杆把立绘中心从画面中线往左右推的最大比例。
	// 站位条是 0…1000 的横向行程，两端对应屏幕左右各 35% 的位置；
	// 再大就会把人物推出画面。
	buexStandeeSway = 0.35

	// buexSwitchWidth / buexSwitchHeight 是开关按钮的兜底尺寸。
	// 素材实际尺寸因作品而异（侵蝕 16x18、Yakata 28x28、YK 36x36），
	// 命中区和换行上限都按解码后的真实帧高算，见 buexSwitchSize；
	// 这里只在图片缺失时使用（测试环境没有资源）。
	buexSwitchWidth  = 16
	buexSwitchHeight = 18

	// buexPresetCount 存档页的预设格数；buexSlotCount 左侧的槽位数。
	buexPresetCount = 36
	buexSlotCount   = 4

	// buexPickerWidth / buexPickerHeight 取色器底图的尺寸。
	buexPickerWidth  = 490
	buexPickerHeight = 200

	// buexNoticeDuration 提示语的显示时长。
	buexNoticeDuration = 1200 * time.Millisecond
)

// buexFontSizes 文章编辑页可以选的字号。
var buexFontSizes = []int{14, 18, 22, 26}

// buexPreset 是一条鉴赏预设。
//
// 旧版本只有 character / background / view / message 四个字段，
// 读旧文件时缺字段按零值处理（view=0 是全身、message 为空），
// 所以老存档不会把鉴赏室读坏。
type buexPreset struct {
	// Character 是角色编号（1 基，与 bu<NN> 的 NN 一致）。
	Character int `json:"character"`
	// Background 是背景图名（空表示没有背景）。
	Background string `json:"background"`
	// View 是距离档位：1 = 半身（f），2 = 放大半身（更近）。
	// 0 是旧文件的全身档，读入时迁到半身。
	View int `json:"view"`
	// Strip 是脱衣阶段（ぬぎぬぎ）。
	Strip int `json:"strip"`
	// Wear 是服装编号（1 基）。
	Wear int `json:"wear"`
	// Pose 是姿势变体字母（a / b）。
	Pose string `json:"pose"`
	// Face 是表情编号（0 = 没有表情图）。
	Face int `json:"face"`
	// Under 是下着档位（0…4）。
	Under int `json:"under"`
	// Message 是配在立绘旁边的留言。
	Message string `json:"message"`
}

// buexPresetFile 是 kizuna-buex.json 的内容。
//
// 旧版本这个文件直接是一个数组（36 条预设），现在多存了左侧的 4 个槽位，
// 所以写成对象；读的时候两种格式都接受（见 UnmarshalJSON）。
type buexPresetFile struct {
	Presets []buexPreset `json:"presets"`
	Slots   []buexPreset `json:"slots"`
}

// UnmarshalJSON 同时接受旧的数组格式和新的对象格式。
//
// 旧文件是玩家已经存下来的东西，不能因为改了格式就读不出来；
// 数组格式按"预设列表"解释，槽位留空。
func (f *buexPresetFile) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var presets []buexPreset
		if err := json.Unmarshal(trimmed, &presets); err != nil {
			return err
		}
		f.Presets = presets
		return nil
	}
	type plain buexPresetFile
	var value plain
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return err
	}
	*f = buexPresetFile(value)
	return nil
}

// buexCharacterState 是一个角色当前的鉴赏状态。
//
// 每个角色各存一份：鉴赏室里可以同时显示多个角色，
// 换角色时不能把上一个角色的服装/表情一起带走。
type buexCharacterState struct {
	// visible 表示这个角色现在是否画出来（on_off_button 开关）。
	visible bool
	// view 是视角：0 = 全身（u），1 = 半身（f）。
	view int
	// strip 是脱衣阶段。
	strip int
	// wear 是服装编号（1 基）。
	wear int
	// pose 是姿势变体字母（a / b）。
	pose string
	// face 是表情编号（0 = 不带表情图）。
	face int
	// under 是下着档位（0…4）。
	under int
	// position 是立ち絵位置バー的值（0…1000，500 = 画面中央）。
	//
	// 原版手册：「任意に選択中のキャラクターの立ち位置を変更する」——
	// 站位属于单个角色，不是全局。多角色同屏（multi bu）时各画各的，
	// 切换角色再拖滑杆不会把别人一起推走。
	position int
}

// buexThumb 是一张背景缩略图。
type buexThumb struct {
	// image 是背景图名。
	image string
	// caption 是缩略图上的标题（EV 页是场景标题，背景页是图名）。
	caption string
}

// buexPageRef 是背景选择页的一个页签按钮。
//
// 命中时只回一个平铺下标，所以这里要记住"第几组、第几页、按钮本身"。
type buexPageRef struct {
	group  int
	page   int
	button config.ConfigButton
	// available 表示这一页有没有内容（没内容的页签既不画也不可点）。
	available bool
}

// buexMuseumState 是鉴赏室一屏的状态。
type buexMuseumState struct {
	layout *config.BuexLayout
	// page 是当前子页（立ち絵 / 背景 / 编辑 / 存档）。
	page int
	// legacy 表示这份状态来自旧式 [bu viewer] 配置（没有 museum_buex_layout）。
	//
	// 旧式查看器只有"立ち絵"这一页：没有背景选择、留言、预设存档，
	// 退出按钮也要回角色选择页而不是鉴赏室主菜单。
	legacy bool
	// legacySelect 表示退出时回旧式角色选择页。
	// 素材缺失时（tj3slg 没有 museum_bu）没有选择页可回，直接回鉴赏室主菜单。
	legacySelect bool
	// multi 表示立ち絵页允许多个角色同屏（[museum phase] multi bu）。
	// 每一名可见角色按自己的 position 画，层序由 order 决定；
	// 非 multi 的布局仍然只画当前选中角色（侵蝕/淫獄 的画面不变）。
	multi bool

	// chars 是每个角色当前的状态，与 layout.Setting.Characters 一一对应。
	chars []buexCharacterState
	// order 是角色的显示顺序（存角色下标，是个排列）。
	order []int
	// selected 是当前选中的角色下标。
	selected int
	// position 是立ち絵位置バー的值（0…1000，500 = 画面中央）。
	// 它不是距离：距离是 setting_bu 里那两个开关（近距離/遠距離），
	// 由每个角色自己的 view 字段保存。把站位条当距离用会让"近/遠"
	// 两个档位都跟着滑杆缩放，正是之前距离看起来不对的原因。
	position int

	// viewTop 是近距離（u 图）视口的上沿，单位是 u 图的源像素 y。
	// 原版的自由視点バー/顔胸腰按钮只移动这条视口，不改图名、不缩放；
	// viewTopSet 为 false 时按 [free-sight screen] 的 fixed position 取默认。
	viewTop    int
	viewTopSet bool

	// background 是当前选中的背景图名（空表示没有背景）。
	background string

	// bgNames 是立ち絵背景页的候选（按名字排序）。
	bgNames []string
	// evPages 是 EV 页：下标是场景分组，内容是这一组的场景缩略图。
	evPages [][]buexThumb
	// evOffset 是"页签分类"到 evPages 的起始偏移。
	// YK / Yakata 只给 5 个 EV 分类，而 scene.dat 有 6 个分组（第 0 组是
	// 共通/序章，不在页签里）；页签分类 i 对应 evPages[evOffset+i]。
	// 侵蝕/淫獄 有 6 个分类，偏移就是 0。
	evOffset int
	// pages 是背景页所有页签按钮的平铺列表（命中下标就是这里的下标）。
	pages []buexPageRef
	// bgGroup / bgPage 是当前所在的页组和页码。
	bgGroup int
	bgPage  int

	// message 是留言文字；editing 表示正在等宿主把文字送回来。
	message string
	editing bool
	// editSub 是编辑页的子菜单（0 = 自由留言，1 = 对话框留言）。
	editSub int
	// fontSize 是字号在 buexFontSizes 里的下标；align 是排版（0 左 / 1 中 / 2 右）。
	fontSize int
	align    int
	// balloon 表示要不要画气泡框。
	balloon bool
	// colorOpen 表示取色器是否展开。
	colorOpen bool
	// colorTarget 是取色器当前编辑的目标（0 = 文字色，1 = 描边色）。
	colorTarget int
	// colorText / colorEdge 是 RGBA 四个分量（0…255）。
	colorText [4]int
	colorEdge [4]int

	// presets 是存档页的 36 条预设；slots 是左侧的 4 个槽位。
	presets []buexPreset
	slots   []buexPreset
	// selectedPreset 是当前选中的预设格；slotSelected 是当前选中的槽位。
	selectedPreset int
	slotSelected   int

	// notice 是提示语；noticeUntil 是它消失的时刻。
	notice      string
	noticeUntil time.Time
}

// ---------------------------------------------------------------- 打开

// openBustupMuseum 打开立ち絵鑑賞室。
//
// 布局来自 BSA 里的 museum_buex_layout：读不到或者语法不对就直接失败，
// 让调用方（鉴赏室主菜单）去退回别的页面，而不是开一个空壳。
func (s *Session) openBustupMuseum() bool {
	if s.ui == nil || s.res == nil || s.composer == nil {
		return false
	}
	// 已经开着就不要重建：页签条就画在鉴赏室画面上，
	// 点回"立ち絵"页签不该把玩家刚调好的服装/背景清空。
	if s.ui.kind == uiMuseumBU && s.ui.buex != nil {
		return true
	}
	location, ok := s.res.TryGet("museum_buex_layout")
	if !ok || location == nil {
		// 旧版作品没有 museum_buex_layout，立绘鉴赏写在 bsxx.cfg 的
		// [museum phase - bu] + [bu viewer] 里（tj3slg 那条技术路线）。
		// 先让玩家选角色，或直接开查看器（没有目标按钮的发行版）。
		core.Logf("ui: museum_buex_layout not found, trying legacy [bu viewer]")
		if s.openLegacyBustupMuseum() {
			return true
		}
		return false
	}
	layout, err := config.LoadBuexLayout(location.Read())
	if err != nil {
		core.Logf("ui: museum_buex_layout parse failed: %v", err)
		if s.openLegacyBustupMuseum() {
			return true
		}
		return false
	}
	state := s.buexNewState(layout)
	s.ui.kind = uiMuseumBU
	s.ui.buex = state
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	core.Logf("ui: bust-up gallery opened (%d characters, %d backgrounds)",
		len(layout.Setting.Characters), len(state.bgNames))
	return true
}

// buexNewState 按布局建一份鉴赏室状态。
//
// 页面数据（背景候选、EV 页、页签）只在这里算一次：
// 它们来自场景表和布局��档，翻页时不需要重算。
func (s *Session) buexNewState(layout *config.BuexLayout) *buexMuseumState {
	state := &buexMuseumState{
		layout:    layout,
		page:      buexPageStandee,
		position:  500,
		multi:     layout.MultiStandee,
		colorText: [4]int{255, 255, 255, 255},
		colorEdge: [4]int{0, 0, 0, 255},
	}
	characters := layout.Setting.Characters
	state.chars = make([]buexCharacterState, len(characters))
	state.order = make([]int, len(characters))
	for index, character := range characters {
		state.order[index] = index
		// 多角色同屏时只默认打开第一个角色。原版每个角色各有 ON/OFF，
		// 全开会让六个人物叠在同一位置；玩家点 ON 再拖站位即可把人摆开。
		// 单角色布局保持原来的 visible=true，绘制逻辑也只取 selected。
		visible := true
		if state.multi {
			visible = index == 0
		}
		state.chars[index] = buexCharacterState{
			visible: visible,
			// 距离开关的左边是「遠距離」：原版打开鉴赏室时左开关就是选中的，
			// 人物以 f 图（屏幕尺寸全身）显示。右边才是「近距離」，
			// 有 u 素材的作品换成高分辨率 u 图，否则放大 f 图。
			view:     buexViewBust,
			strip:    0,
			wear:     buexDefaultWear(character),
			pose:     "a",
			position: 500,
			// 立绘的"身体/头发"图本身没有眼睛和眉毛，表情叠加图才是眼睛所在的层。
			// face=0 表示"不叠加"，鉴赏室一打开就是无眼人物；这里按素材挑一个
			// 默认表情（优先 03，原版剧情的默认表情；没有就取表格第一项）。
			face: buexDefaultFace(character),
			// 下着开关有 5 档，而布局里的 under_wear 是"下着的服装编号"，
			// 不是开关档位，所以开关一律从第 0 档开始。
			under: 0,
		}
	}
	stored := loadBuexPresets(s)
	state.presets = stored.Presets
	state.slots = stored.Slots
	s.buexBuildPages(state)
	// 默认给一张立ち絵背景：buex_bubase 只是一层透明底板，
	// 不铺背景时人物会站在纯黑画面上，和原版的鉴赏室观感差很远。
	// 这里再兜一层：scene.dat 缺失或分组不匹配时（MKIE 的 gb00001a 等
	// 背景就不在 Group>=6 的场景里），依次从资源名和固定名字里挑一张
	// 真能画出来的图。mros 的 10 位背景名曾经整批被旧解析器丢掉，
	// 导致这里为空。
	if state.background == "" {
		state.background = s.buexBackgroundFallback(state)
	}
	if state.background == "" {
		// 一张都挑不出来时立ち絵页只能靠 museum_base 垫底（见
		// drawBuexStandeePage）；这条日志用来区分"素材问题"和"逻辑问题"。
		core.Logf("ui: bust-up gallery has no usable background")
	}
	return state
}

// buexSafeBackgroundNames 是各代作品里固定存在的背景名（最后一道兜底）。
//
// 只在资源索引和场景表都给不出候选时使用。名字本身不能证明文件存在，
// 所以下面的 buexBackgroundFallback 仍要逐个加载并检查像素，
// 验证失败的名字绝不能写进 state.background。
var buexSafeBackgroundNames = []string{
	// 牝教師（MKIE/MK2）这一代的 [main window] bg signature 是 gb。
	"gb00001a", "gb00002a", "gb00003a",
	// 其余作品沿用 bg 前缀的常见起始编号。
	"bg00001a", "bg01001a",
}

// buexBackgroundFallback 选一张"确实能画出来"的背景图。
//
// 只信 bgNames 不够：有的发行版没有 scene.dat，或者背景不在场景分组里
// （MKIE 的 gb00001a 等 47 张图在 Graphics.bsa 的 bg/ 下，scene.dat 里
// 的图名却是 g10110 这种，buexBackgroundNames 一张都筛不到）。候选顺序：
//  1. state.bgNames（场景表或资源名筛出来的）；
//  2. fallbackBackgroundNames(resourceImageNames())（资源索引再筛一遍）；
//  3. buexSafeBackgroundNames（前两步都挑不出可用图时才试的固定名字）。
//
// 名字能查到、解码也成功，并不等于画得出来：BsgImage 可能整张全透明
// （只剩容器）或纯黑，贴上去和没贴一样。所以每张都要过
// buexBackgroundImageUsable，避免"有背景名却还是黑屏"。
func (s *Session) buexBackgroundFallback(state *buexMuseumState) string {
	if s.composer == nil {
		return ""
	}
	if state != nil {
		for _, name := range state.bgNames {
			if s.buexBackgroundLoads(name) {
				return name
			}
		}
	}
	for _, name := range fallbackBackgroundNames(s.resourceImageNames()) {
		if s.buexBackgroundLoads(name) {
			return name
		}
	}
	for _, name := range buexSafeBackgroundNames {
		if s.buexBackgroundLoads(name) {
			return name
		}
	}
	return ""
}

// buexBackgroundLoads 判断一个背景名能不能真的铺出画面。
func (s *Session) buexBackgroundLoads(name string) bool {
	if s.composer == nil || name == "" {
		return false
	}
	return buexBackgroundImageUsable(s.composer.Load(name))
}

// buexBackgroundImageUsable 判断解码后的图有没有"看得见"的像素。
//
// 合成图（BSG Composition）在解码时已经摊平成容器自己的 Pixels，
// 那就是玩家实际看到的画面，优先看它；只有容器没有像素（例如部件图）
// 时才下探到 Parts——有一个部件可见就算这张图可见。
func buexBackgroundImageUsable(image *gfx.BsgImage) bool {
	if image == nil {
		return false
	}
	if len(image.Pixels) > 0 {
		return buexPixelsVisible(image.Pixels)
	}
	for _, part := range image.Parts {
		if buexBackgroundImageUsable(part) {
			return true
		}
	}
	return false
}

// buexPixelsVisible 抽样判断 BGRA 像素是不是"空白"（全透明或纯黑）。
//
// 全透明/纯黑的图和没画一样，玩家看到的仍是黑屏，所以要按不可用处理。
// 抽样而不是全扫：1024x768 是 78 万像素，打开鉴赏室时要筛几十张合成图，
// 全扫会明显拖慢进入画面。大图每 97 个像素取一个（97 与 1024/800/1280
// 都互质，采样点不会总落在同一列）；小图（<=4096 像素）全扫，因为采样太稀
// 时一个透明的边角就可能把整张小图误判成空白。
// 可见采样点不到 1% 就算空白：宁可留一张很暗的图，也不能把玩家丢回黑屏。
func buexPixelsVisible(pixels []byte) bool {
	total := len(pixels) / 4
	if total == 0 {
		return false
	}
	step := 97
	if total <= 4096 {
		step = 1
	}
	// 三色和的下限，约等于 (8,8,8)：纯黑和几乎全黑的画布都算空。
	const visibleThreshold = 24
	samples := 0
	visible := 0
	for index := 0; index < total; index += step {
		offset := index * 4
		samples++
		// BGRA：第 4 个字节是 alpha，小于 8 就是看不见的像素。
		if pixels[offset+3] < 8 {
			continue
		}
		if int(pixels[offset])+int(pixels[offset+1])+int(pixels[offset+2]) >= visibleThreshold {
			visible++
		}
	}
	return visible*100 >= samples
}

// buexDefaultWear 角色默认穿的服装：取服装表的第一件，没有表就用 1 号。
func buexDefaultWear(character config.BuexCharacter) int {
	if len(character.Wear) > 0 && character.Wear[0] > 0 {
		return character.Wear[0]
	}
	return 1
}

// buexBuildPages 按布局和场景表把背景页的数据备好。
//
// 页签结构在作品之间不一致，不能按"内层数组下标"直接当页签：
//   - 侵蝕/淫獄：第一组是背景（2 页），第二组是 6 个 EV 分类，每类 1 页；
//   - YK/Yakata：第一组是背景（2 页），后面每组是"一个分类的 page1/page2"，
//     Yakata 甚至把 5 个分类的 10 颗按钮都塞进同一个内层数组。
//
// 共同规律是：背景分类的按钮各自是一页；EV 按钮按 page1/page2 成对出现，
// 一对就是一个分类的两页。这里按按钮图名里的页码识别配对，再按分类数
// 推断要跳过 scene.dat 开头几个共通分组（YK/Yakata 的 5 分类对 6 组）。
func (s *Session) buexBuildPages(state *buexMuseumState) {
	catalog := s.ensureScenes()
	state.bgNames = buexBackgroundNames(catalog)
	if len(state.bgNames) == 0 {
		// 没有 scene.dat 的作品（黒の教室）不会把背景放进场景表；
		// 「立ち絵背景」页按 bgNNNNNab 的命名直接从资源取。
		state.bgNames = fallbackBackgroundNames(s.resourceImageNames())
	}
	state.evPages = buexEventPages(catalog)
	groups := state.layout.Bg.PageGroups
	perPage := s.buexThumbCount(state)

	// 背景分类：每个按钮是一页。
	if len(groups) > 0 {
		for page, button := range groups[0] {
			state.pages = append(state.pages, buexPageRef{
				group:     0,
				page:      page,
				button:    button,
				available: page*perPage < len(state.bgNames),
			})
		}
	}

	// EV 分类：把其余组的按钮摊平后按页码配对。
	evButtons := make([]config.ConfigButton, 0, 16)
	if len(groups) > 1 {
		for _, group := range groups[1:] {
			evButtons = append(evButtons, group...)
		}
	}
	pagesPerCategory := buexPagesPerCategory(evButtons)
	categories := 0
	if pagesPerCategory > 0 {
		categories = len(evButtons) / pagesPerCategory
	}
	state.evOffset = buexEventPageOffset(state.evPages, categories)
	for index, button := range evButtons {
		category := index / pagesPerCategory
		page := index % pagesPerCategory
		evIndex := state.evOffset + category
		available := evIndex >= 0 && evIndex < len(state.evPages) &&
			page*perPage < len(state.evPages[evIndex])
		state.pages = append(state.pages, buexPageRef{
			group:     1 + category,
			page:      page,
			button:    button,
			available: available,
		})
	}
}

// buexPagesPerCategory 判断 EV 页签是"每类几页"。
//
// 按钮图名里的页码是唯一可靠的信号：buex_page1/buex_page2 是同一分类的
// 两页；museum_page43 这类名字没有页码，每个按钮就是独立分类的一页。
// 识别失败时按 1 页处理，保持侵蝕/淫獄 的行为不变。
func buexPagesPerCategory(buttons []config.ConfigButton) int {
	if len(buttons) < 2 || len(buttons)%2 != 0 {
		return 1
	}
	for index := 0; index < len(buttons); index += 2 {
		first, okFirst := buexPageTabNumber(buttons[index].Name)
		second, okSecond := buexPageTabNumber(buttons[index+1].Name)
		if !okFirst || !okSecond || first != 1 || second != 2 {
			return 1
		}
	}
	return 2
}

// buexPageTabNumber 从按钮图名末尾读取页码（buex_page1 -> 1）。
//
// 只接受"page"后面恰好一位数字的名字。museum_page43 末尾是 43，
// 不是页码，不能把 6 个角色分类误配成两页一组。
func buexPageTabNumber(name string) (int, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return 0, false
	}
	index := len(lower)
	for index > 0 && lower[index-1] >= '0' && lower[index-1] <= '9' {
		index--
	}
	if index == len(lower) || len(lower)-index != 1 || index == 0 {
		return 0, false
	}
	if !strings.HasSuffix(lower[:index], "page") {
		return 0, false
	}
	number := int(lower[index] - '0')
	if number <= 0 {
		return 0, false
	}
	return number, true
}

// buexEventPageOffset 计算页签分类到 evPages 的起始偏移。
//
// scene.dat 的分组数和页签分类数不一定相等（YK/Yakata 是 6 组对 5 个分类，
// 其中最后一组是空的）。这里不猜"要跳过开头几个"，而是滑动窗口找
// "非空分组最多"的一段：这样开头的共通分组或末尾的空分组都能自动跳过。
// 分数相同时取偏移 0，保持侵蝕/淫獄 这类一一对应的作品不变。
func buexEventPageOffset(pages [][]buexThumb, categories int) int {
	if categories <= 0 || len(pages) <= categories {
		return 0
	}
	bestOffset := 0
	bestScore := -1
	for offset := 0; offset+categories <= len(pages); offset++ {
		score := 0
		for index := 0; index < categories; index++ {
			if len(pages[offset+index]) > 0 {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestOffset = offset
		}
	}
	return bestOffset
}

// buexBackgroundNames 收集"立ち絵背景"页的候选图。
//
// 只取场景表里 Group>=6 的场景（那些是背景专用场景）里名字以 bg 开头的图，
// 去重后按名字排序——排序是为了每次打开鉴赏室时的顺序一致。
func buexBackgroundNames(catalog *data.SceneCatalog) []string {
	if catalog == nil {
		return nil
	}
	names := make([]string, 0, 128)
	seen := map[string]bool{}
	for _, scene := range catalog.Scenes {
		if scene.Group < 6 {
			continue
		}
		for _, image := range scene.Images {
			if seen[image] || !buexIsBackgroundImage(image) {
				continue
			}
			seen[image] = true
			names = append(names, image)
		}
	}
	sort.Strings(names)
	return names
}

// buexIsBackgroundImage 判断一张图是不是背景图。
//
// 不直接判 "bg" 前缀：牝教師2 这一代的背景是 gb00001a 这种命名
// （[main window] bg signature = gb[0-9]{5}[a-z]）。统一交给
// parseBackgroundName 判断，两种前缀、以及 4/5 位编号与差分的组合都覆盖。
func buexIsBackgroundImage(image string) bool {
	base := path.Base(image)
	base = strings.TrimSuffix(base, path.Ext(base))
	_, _, ok := parseBackgroundName(base)
	return ok
}

// buexEventPages 把场景表按分组整理成 EV 页。
//
// 下标就是场景的 Group（0…5），每一组一页。
func buexEventPages(catalog *data.SceneCatalog) [][]buexThumb {
	pages := make([][]buexThumb, 6)
	if catalog == nil {
		return pages
	}
	for _, scene := range catalog.Scenes {
		if scene.Group < 0 || scene.Group >= len(pages) {
			continue
		}
		image := buexSceneBackground(scene)
		if image == "" {
			continue
		}
		pages[scene.Group] = append(pages[scene.Group], buexThumb{image: image, caption: scene.Title})
	}
	return pages
}

// buexSceneBackground 取场景里最适合当背景的那张图。
//
// 场景图里有角色立绘（"…_1p00"）也有纯背景，_1p00 是原引擎给"一个人+背景"
// 的那张，用它当缩略图背景最接近玩家在游戏里看到的画面。
func buexSceneBackground(scene data.ReplayScene) string {
	for _, image := range scene.Images {
		if strings.Contains(image, "_1p00") {
			return image
		}
	}
	if len(scene.Images) > 0 {
		return scene.Images[0]
	}
	return ""
}

// buexThumbCount 一页最多放多少张缩略图（布局给的坐标数量）。
func (s *Session) buexThumbCount(state *buexMuseumState) int {
	if state == nil {
		return buexPresetCount
	}
	count := len(state.layout.Bg.ThumbPos)
	if count <= 0 {
		return buexPresetCount
	}
	return count
}

// ---------------------------------------------------------------- 绘制

func (s *Session) drawBustupMuseum(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	if ui.kdStrip {
		s.drawKDStripMuseum(canvas)
		return
	}
	if ui.md2Bu != nil {
		s.drawMD2BuViewer(canvas)
		return
	}
	if ui.buex == nil {
		s.drawLegacyBustupSelect(canvas)
		return
	}
	state := ui.buex
	switch state.page {
	case buexPageBackground:
		s.drawBuexBackgroundPage(canvas, state)
	case buexPageEdit:
		s.drawBuexEditPage(canvas, state)
	case buexPageStore:
		s.drawBuexStorePage(canvas, state)
	default:
		s.drawBuexStandeePage(canvas, state)
	}
	// 鉴赏室页有自己的顶部主菜单（含 back_to_thumbs）：鉴赏室模式页签条
	// 不再画在这里。
	//
	// 原版把页签条留在画面上当兜底，但那一排按钮和鉴赏室自己的菜单
	// 完全重合：Yakata 的页签条在左侧面板上，正好压住换装开关；
	// YK / 侵蝕 / 淫獄 的 BUEX 菜单本来就盖住同一行，还自带返回入口。
	// 留着的实际效果只有"点到重叠位置的空白会莫名切页"，所以不放。
	// 旧式查看器（legacy）本来就是独立全屏页，不画页签条。
	s.drawBuexMenu(canvas, state)
	s.drawBuexNotice(canvas, state)
}

// drawBuexStandeePage 画立ち絵页。
func (s *Session) drawBuexStandeePage(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	// 层的次序不能反：buex_bubase 是一张"下半屏底板 + 开关标签"的
	// 透明覆盖层（实机素材里中间和顶部是全透明的），它必须盖在背景和
	// 立绘之上，底板才会挡住立绘的脚。以前先画底图再画背景，背景会把
	// 标签和底板整块盖掉，只剩一排没有说明的开关。
	if state.background == "" || s.composer.Load(state.background) == nil {
		state.background = s.buexBackgroundFallback(state)
	}
	// backdrop 记录"已经用 museum_base 垫过底"。它是张不透明底图，
	// 只在背景为空的分支里画在立绘下面；下面 base 缺失时不能再画一遍，
	// 否则会把刚画好的立绘整个盖住。
	backdrop := false
	if state.background != "" {
		s.drawBuexBackground(canvas, state.background)
	} else if s.drawImageAt(canvas, "museum_base", 0, 0) {
		backdrop = true
	}
	s.drawBuexStandees(canvas, state)
	base := state.layout.Setting.Base
	if !s.drawImageAt(canvas, base, 0, 0) {
		// 布局里没有底图（或图丢了）时用鉴赏室底图兜底，
		// 否则这一页只剩控件浮在黑屏上。
		if !backdrop {
			s.drawImageAt(canvas, "museum_base", 0, 0)
		}
		s.drawImageAt(canvas, "buex_bubase", 0, 0)
	}
	s.drawBuexSwitches(canvas, state)
	s.drawBuexCharacters(canvas, state)
	s.drawBuexOrderButtons(canvas, state)
	s.drawBuexPointer(canvas, state)
	s.drawBuexToggles(canvas, state)
	s.drawBuexFreeSight(canvas, state)
}

// drawBuexBackground 把一张背景图铺满画面。
//
// 背景图和立绘一样是合成图：容器 + 若干部件，
// 部件要按容器偏移画，否则会错位。
func (s *Session) drawBuexBackground(canvas *gfx.BgraCanvas, name string) {
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	if len(image.Parts) > 0 {
		for _, part := range image.Parts {
			canvas.Draw(part, image.ContainerOffsetX+part.OffsetX, image.ContainerOffsetY+part.OffsetY)
		}
		return
	}
	canvas.Draw(image, 0, 0)
}

// drawBuexStandees 画立ち絵页上所有可见角色。
//
// 原版手册：每名角色有独立的「表示切り替えボタン」和「立ち位置バー」，
// 同屏可以站多名角色；「重なりボタン」调整的是人物之间的前后层序。
// order[0] 是手前（最上层），所以倒着画，最后画的才是手前那一个。
//
// 只有 [museum phase] multi bu = 1 的作品（牝教師3）走这条多角色路径；
// 新版 museum_buex_layout（侵蝕/淫獄）没有这个开关，仍然只展示选中角色，
// 避免把已经调好的画面改成多人叠在一起。
func (s *Session) drawBuexStandees(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	if state == nil {
		return
	}
	if !state.multi {
		index := state.selected
		if index < 0 || index >= len(state.chars) || !state.chars[index].visible {
			return
		}
		s.drawBuexStandee(canvas, state, index)
		return
	}
	for orderIndex := len(state.order) - 1; orderIndex >= 0; orderIndex-- {
		index := state.order[orderIndex]
		if index < 0 || index >= len(state.chars) || !state.chars[index].visible {
			continue
		}
		s.drawBuexStandee(canvas, state, index)
	}
}

// drawBuexStandee 画一个角色的立绘：身体 + 头发 + 表情叠加图。
//
// 三张图必须用同一套变换（x、y、scale）：表情图是单独的合成图，容器和身体
// 图一样大，只是里面只有眼睛/嘴的部件；各算各的缩放会让表情飘到脸外面。
//
// 距離切换在两代作品里含义不同：
//   - 新版 museum_buex_layout（侵蝕/淫獄）的素材只有 f 图，近距離用 1.5 倍
//     放大同一张 f 图；
//   - 牝教師3 同时提供 f（屏幕尺寸全身）和 u（高分辨率全身），近距離直接
//     换 u 图，并用 [free-sight screen] 的 fixed position 决定裁切视口。
//
// 后者才是原版手册里的"近距離表示の立ち絵を頭の上から足の先まで"。
func (s *Session) drawBuexStandee(canvas *gfx.BgraCanvas, state *buexMuseumState, index int) {
	character := state.chars[index]
	name, up := s.buexStandeeName(state, index, character)
	if name == "" {
		return
	}
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	scale := 1.0
	x, y := 0, 0
	if up {
		// u 图是 2048x2600 的高分辨率整身像，按"人物横向居中 + 视口纵向裁切"
		// 贴到屏幕上；不能用缩放把整张图塞进画面，否则原版的近距離观感就没了。
		anchorX := image.ContainerOffsetX + image.Width/2
		if len(image.Parts) > 0 && image.Parts[0] != nil {
			part := image.Parts[0]
			anchorX = image.ContainerOffsetX + part.OffsetX + part.Width/2
		}
		x = s.width/2 - anchorX
		y = -s.buexViewTop(state, image)
	} else {
		if character.view == buexViewClose {
			scale = buexCloseScale
		}
		anchorX, anchorY := buexStandeeAnchor(image)
		x = int(float64(anchorX) * (1 - scale))
		y = int(float64(anchorY) * (1 - scale))
	}
	// 站位条对所有距离档位都生效：它移动的是人物在画面里的左右位置。
	// 多角色模式下每名角色读自己的 position。
	x += s.buexStandeeSwayX(state, index)
	s.drawBuexComposedFrames(canvas, image, x, y, scale, 1)

	if s.settings.UnderHair {
		s.drawBuexHair(canvas, name, x, y, scale)
	}
	if character.face > 0 {
		if faceName := s.buexFaceName(state, index, character, up); faceName != "" {
			if face := s.composer.Load(faceName); face != nil {
				// 表情图里是眨眼/口型的竖排帧条，只画第 0 帧。
				s.drawBuexComposedFrames(canvas, face, x, y, scale, s.buexEyePattern())
			}
		}
	}
}

// buexViewTop 返回近距離 u 图的视口上沿（源像素）。
//
// 原版 [free-sight screen] 的 fixed position 是"顔/胸/股間"三个基准值，
// 视口上沿 = 基准值 - 画面高（配置注释里的"指定の値から画面の高さを
// 引いた位置から表示される"）。玩家拖过自由視点バー后以 viewTop 为准。
func (s *Session) buexViewTop(state *buexMuseumState, image *gfx.BsgImage) int {
	height := 0
	if image != nil {
		height = image.Height
	}
	maxTop := max(0, height-s.height)
	if state != nil && state.viewTopSet {
		return clampInt(state.viewTop, 0, maxTop)
	}
	if s.config != nil && len(s.config.FreeSight.FixedPositions) > 0 {
		return clampInt(s.config.FreeSight.FixedPositions[0]-s.height, 0, maxTop)
	}
	return maxTop / 2
}

// buexViewTopFor 把 0（顔）/1（胸）/2（股間）换算成视口上沿。
func (s *Session) buexViewTopFor(image *gfx.BsgImage, mode int) (int, bool) {
	if s.config == nil || len(s.config.FreeSight.FixedPositions) == 0 {
		return 0, false
	}
	mode = clampInt(mode, 0, len(s.config.FreeSight.FixedPositions)-1)
	height := 0
	if image != nil {
		height = image.Height
	}
	maxTop := max(0, height-s.height)
	return clampInt(s.config.FreeSight.FixedPositions[mode]-s.height, 0, maxTop), true
}

// drawBuexHair 画立绘的头发叠加图。
//
// 头发图和身体图共用同一张容器画布，所以直接用身体算好的变换，
// 再按部件偏移贴上去。
func (s *Session) drawBuexHair(canvas *gfx.BgraCanvas, name string, x, y int, scale float64) {
	hair := s.composer.Load(name + "_hair")
	if hair == nil {
		return
	}
	s.drawBuexComposedFrames(canvas, hair, x, y, scale, 1)
}

// buexEyePattern 返回 bsxx.cfg 的眼部帧数（eye pattern）。
//
// 表情/眨眼部件的 BSG 头没写 FrameCount，只能靠这个值切帧；
// 配置缺失时返回 1，表示"不要切"。
func (s *Session) buexEyePattern() int {
	if s.config != nil && s.config.EyePattern > 1 {
		return s.config.EyePattern
	}
	return 1
}

// drawBuexComposedFrames 画一张合成图（容器 + 部件）。
//
// scale 是缩放系数：容器偏移和部件偏移都要一起缩放，
// 只缩放部件像素会让部件错位。
// frames 是部件内部的帧数（1 = 普通部件，eye pattern = 表情/眨眼帧条），
// 每个部件只取第 0 帧——鉴赏室是静态展示，不该让整条帧条一次画出来。
func (s *Session) drawBuexComposedFrames(canvas *gfx.BgraCanvas, image *gfx.BsgImage, x, y int, scale float64, frames int) {
	if image == nil || image.Width <= 0 || image.Height <= 0 {
		return
	}
	if len(image.Parts) == 0 {
		part := standeePartFrameN(image, frames)
		if scale == 1 {
			canvas.Draw(part, x, y)
			return
		}
		canvas.BlitScaled(part, x, y, int(float64(part.Width)*scale), int(float64(part.Height)*scale), 255)
		return
	}
	for index, part := range image.Parts {
		if part == nil {
			continue
		}
		// 只有第二个及之后的部件才是眼睛/眉毛帧条；第 0 个部件是脸型底图。
		// 旧实现把 frames 传给所有部件：u 脸底图高度 216 恰好能被 4 整除，
		// 底图会被切成 4 条，表情切换时就会出现"眼睛错位/多行"。
		partFrames := 1
		if frames > 1 && len(image.Parts) > 1 && index > 0 {
			partFrames = frames
		}
		part = standeePartFrameN(part, partFrames)
		if scale == 1 {
			canvas.Draw(part, x+image.ContainerOffsetX+part.OffsetX, y+image.ContainerOffsetY+part.OffsetY)
			continue
		}
		px := x + int(float64(image.ContainerOffsetX+part.OffsetX)*scale)
		py := y + int(float64(image.ContainerOffsetY+part.OffsetY)*scale)
		canvas.BlitScaled(part, px, py, int(float64(part.Width)*scale), int(float64(part.Height)*scale), 255)
	}
}

// buexStandeeAnchor 返回半身图里人物的锚点（部件中心 x、部件顶边 y）。
//
// 近距離档把同一张 f 图放大时围绕这个点缩放：头的位置保持不变，
// 人物不会突然跳到画面角落，也不会因为缩放把脸切掉。
func buexStandeeAnchor(image *gfx.BsgImage) (int, int) {
	if image == nil {
		return 0, 0
	}
	part := image
	if len(image.Parts) > 0 && image.Parts[0] != nil {
		part = image.Parts[0]
	}
	x := image.ContainerOffsetX + part.OffsetX + part.Width/2
	y := image.ContainerOffsetY + part.OffsetY
	return x, y
}

// buexStandeeSwayX 站位滑杆（0…1000）对应的水平偏移。
//
// 站位条是横向的立て絵位置バー：500 是画面中央，0/1000 分别把
// 人物中心推到中线左右各 buexStandeeSway 个屏宽的位置；
// 再大就会把人物推出画面。
//
// multi 时 index 是角色下标，取该角色自己的 position；否则读全局 position。
// 站位条在两种布局下都存在，只是"作用对象"不同。
func (s *Session) buexStandeeSwayX(state *buexMuseumState, index int) int {
	if state == nil {
		return 0
	}
	position := state.position
	if state.multi && index >= 0 && index < len(state.chars) {
		position = state.chars[index].position
	}
	fraction := float64(clampInt(position, 0, 1000))/1000 - 0.5
	return int(fraction * 2 * float64(s.width) * buexStandeeSway)
}

// buexSelectedPosition 返回当前选中角色的站位值。
//
// 画指示器和点击滑杆都走这里，保证"拖的是谁"与"画的是谁"一致。
func buexSelectedPosition(state *buexMuseumState) int {
	if state == nil {
		return 500
	}
	if state.multi && state.selected >= 0 && state.selected < len(state.chars) {
		return state.chars[state.selected].position
	}
	return state.position
}

// drawBuexSwitches 画开关组。
//
// 新版布局是六组（ポーズ/視点/ぬぎぬぎ/服装/下着/表情）；
// 旧式 [bu viewer] 还有第 7 组"背景"，buexSwitchCount 会一起处理。
// 开关图 buex_view_switch 是 16x54 的竖排图，3 帧分别对应：
// 0 = 空格（关）、1 = 打勾（开）、2 = 悬停高亮。
// 注意这里没有"命中掩码"帧：早期实现误把第 0 帧当成掩码而画第 1 帧，
// 结果所有开关看起来都是打开的，玩家分不清当前选中的是哪一颗。
func (s *Session) drawBuexSwitches(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	button := state.layout.Setting.Buttons["switch"]
	if button.Name == "" {
		return
	}
	for group := 0; group < buexSwitchCount && group < len(state.layout.Setting.Switches); group++ {
		spec := state.layout.Setting.Switches[group]
		if buexSwitchDisabled(spec, s.width, s.height) {
			continue
		}
		count := s.buexSwitchCount(state, group)
		if count <= 0 {
			continue
		}
		active := s.buexSwitchActive(state, group)
		width, height := s.buexSwitchSize(state)
		for index := 0; index < count; index++ {
			x, y := buexSwitchPosition(spec, count, index)
			frame := 0
			if index == active {
				frame = 1
			}
			if s.ui.hover.kind == buexSwitchHit(group) && s.ui.hover.index == index {
				// 悬停用第 2 帧（高亮框），再叠一层亮色，
				// 玩家一眼能看出鼠标指着哪颗开关。
				frame = 2
				canvas.Fill(x, y, width, height, 255, 255, 255, 60)
			}
			s.drawBuexButtonAt(canvas, button, frame, x, y)
		}
	}
}

// buexSwitchHit 第 group 组开关对应的命中种类。
func buexSwitchHit(group int) uiHitKind {
	switch group {
	case buexSwitchPose:
		return hitBUPose
	case buexSwitchView:
		return hitBUView
	case buexSwitchNgng:
		return hitBUNgng
	case buexSwitchWear:
		return hitBUWear
	case buexSwitchUnder:
		return hitBUUnder
	case buexSwitchFace:
		return hitBUFace
	case buexSwitchBackground:
		return hitBUBackground
	}
	return hitNone
}

// buexSwitchPosition 计算第 index 颗开关的位置。
//
// 配置的 space 是"列间距/行间距"，repeat_x 是每行列数；超过一行要换行，
// 而不是把 X/Y 同时加上 index 倍间距（那会排成一条斜线）。
func buexSwitchPosition(spec config.BuexSwitchGroup, count, index int) (int, int) {
	columns := spec.Columns
	if columns <= 0 || columns > count {
		columns = count
	}
	if columns <= 0 {
		columns = 1
	}
	return spec.X + (index%columns)*spec.SpaceX, spec.Y + (index/columns)*spec.SpaceY
}

// buexSwitchDisabled 判断一组开关是否被原作者移出画面。
//
// 旧配置用负坐标（-16,-16）表示"这一组不显示"；新版的
// museum_buex_layout 用画面右下角以外的坐标（如 1366,768）。
// 两种都要跳过绘制和命中，否则会在左上角画出一排不该出现的开关。
func buexSwitchDisabled(spec config.BuexSwitchGroup, width, height int) bool {
	if spec.X < 0 || spec.Y < 0 {
		return true
	}
	if width > 0 && spec.X >= width {
		return true
	}
	if height > 0 && spec.Y >= height {
		return true
	}
	return false
}

// buexSwitchSize 返回单颗开关按钮的显示/命中尺寸。
//
// 不能写死 16x18：YK 的 buex_view_switch 是 36x108（每帧 36x36），
// Yakata 是 28x84（每帧 28x28）。用素材尺寸做命中框，否则只有按钮左上角
// 一小块能点，玩家会觉得"开关错位/点不动"；换行上限也要用真实帧高，
// 否则 YK 的两行会按 18px 算成三行，仍旧压到下一组控件上。
func (s *Session) buexSwitchSize(state *buexMuseumState) (int, int) {
	if state == nil {
		return buexSwitchWidth, buexSwitchHeight
	}
	button := state.layout.Setting.Buttons["switch"]
	image := s.composer.Load(button.Name)
	if image != nil && image.Width > 0 && image.Height > 0 {
		frames := max(1, button.MaxState)
		if height := image.Height / frames; height > 0 {
			return image.Width, height
		}
	}
	return buexSwitchWidth, buexSwitchHeight
}

// buexSwitchCount 某一组开关有多少颗。
//
// 数量按语义定：服装开关的数量等于当前角色的服装表长度，脱衣开关是
// 换算表长度（不是 max strip：3SLG 的 max strip=6 但换装表只有 0/3/4/5
// 四档，画 6 颗就会有 2 颗点到不存在的阶段），表情开关跟着当前姿势那一页
// 的表情表走（见下面的分支）。
// 布局里的 repeat_x 只决定每行几颗（Columns），不直接当总颗数用；
// 只有下着这种"开关本身就是全部档位"的组例外。
func (s *Session) buexSwitchCount(state *buexMuseumState, group int) int {
	count := 0
	switch group {
	case buexSwitchPose:
		count = 2
	case buexSwitchView:
		count = 2
	case buexSwitchNgng:
		count = len(state.layout.Setting.StripConvert)
		if count <= 0 {
			count = s.buexMaxStrip()
		}
	case buexSwitchWear:
		if state.selected < len(state.layout.Setting.Characters) {
			count = len(state.layout.Setting.Characters[state.selected].Wear)
		}
		if count == 0 {
			count = 1
		}
	case buexSwitchUnder:
		// 下着开关的颗数就是布局里的 repeat_x：YK / Yakata 只有 2 颗，
		// 画 5 颗会有 3 颗点到不存在的档位（看起来像"开关多出来的空位"）。
		// 旧式 [bu viewer] 的配置没有这个含义，仍然按 5 档。
		count = 5
		if !state.legacy && group < len(state.layout.Setting.Switches) {
			if columns := state.layout.Setting.Switches[group].Columns; columns > 0 {
				count = columns
			}
		}
	case buexSwitchFace:
		// 旧式 [bu viewer] 的两排表情开关合并时由 Count 收窄；
		// 牝教師3 的 setting5/6 是独立组（Count=0），按姿势页铺开，
		// 否则 13/14 号表情永远点不到。
		// 新版布局里：声明了行距的组（YK / Yakata / SLIE）也按姿势分页，
		// 一页＝1 颗"无表情"＋当前页的表情数；没写行距的组
		// （侵蝕 / 淫獄の放課後2 的 space:[17,0]）是一整排 13 颗。
		count = 13
		if s.buexFacePaged(state) && state.selected >= 0 && state.selected < len(state.layout.Setting.Characters) {
			count = 1
			faces := state.layout.Setting.Characters[state.selected].Faces
			if page := buexFacePage(state); page < len(faces) {
				count += len(faces[page])
			}
		}
	case buexSwitchBackground:
		// 旧式查看器的背景开关是一整块网格：repeat_x 只是每行几颗，
		// 不是总数。牝教師3 有几十张背景，全都要能点到；
		// 非 multi 的旧作保持原来的 7 颗上限，避免把别的界面顶出屏幕。
		count = len(state.bgNames)
		if !state.multi && count > 7 {
			count = 7
		}
	}
	// 旧式配置用 repeat_x 的合计表达"表情只有两排 12 颗"，
	// Count>0 时按它收窄；新版布局 Count=0，不设上限。
	if group < len(state.layout.Setting.Switches) {
		if limit := state.layout.Setting.Switches[group].Count; limit > 0 && limit < count {
			count = limit
		}
	}
	// 多行开关组最多排到下一组控件之前，否则会盖到别的开关上面。
	if limit := s.buexSwitchRowLimit(state, group); limit > 0 && count > limit {
		count = limit
	}
	return count
}

// buexSwitchRowLimit 一组多行开关最多能排多少颗（0 表示不设上限）。
//
// 布局只写了第一颗的坐标、行距和每行几颗，组与组之间靠坐标分开。
// 表情表很长（YK 每页 9 个表情 + "无表情"）时按颗数铺会折行到下面那组
// 开关上，玩家看到两排开关叠在一起。所以按"到下一组开关的距离"算能排
// 几行，再乘每行的列数当作上限。
// 旧式 [bu viewer] 的坐标是另一套，不适用；单行组也谈不上折行。
func (s *Session) buexSwitchRowLimit(state *buexMuseumState, group int) int {
	if state == nil || state.legacy {
		return 0
	}
	if group < 0 || group >= len(state.layout.Setting.Switches) {
		return 0
	}
	spec := state.layout.Setting.Switches[group]
	if spec.SpaceY <= 0 || spec.Columns <= 0 {
		return 0
	}
	// 下面最近的一组开关；下面没有了就以画面底边为界。
	bottom := s.height
	for index, other := range state.layout.Setting.Switches {
		if index == group || other.Y <= spec.Y {
			continue
		}
		if buexSwitchDisabled(other, s.width, s.height) {
			continue
		}
		if other.Y < bottom {
			bottom = other.Y
		}
	}
	gap := bottom - spec.Y
	_, switchHeight := s.buexSwitchSize(state)
	rows := (gap-switchHeight)/spec.SpaceY + 1
	if rows < 1 {
		rows = 1
	}
	return rows * spec.Columns
}

// buexSwitchActive 当前落在第几颗开关上（用来画"开"那一帧）。
func (s *Session) buexSwitchActive(state *buexMuseumState, group int) int {
	character := state.selected < len(state.layout.Setting.Characters) &&
		state.selected < len(state.chars)
	if !character {
		return 0
	}
	switch group {
	case buexSwitchPose:
		if state.chars[state.selected].pose == "b" {
			return 1
		}
		return 0
	case buexSwitchView:
		// 左开关 = 半身/遠距離（f），右开关 = 近距離（多 u 图，缺 u 时放大 f）。
		// 旧预设里的 full 值按半身处理，不能让左开关空着。
		if state.chars[state.selected].view == buexViewClose {
			return 1
		}
		return 0
	case buexSwitchNgng:
		for index := 0; index < s.buexSwitchCount(state, group); index++ {
			if s.buexStripValue(state, index) == state.chars[state.selected].strip {
				return index
			}
		}
		return 0
	case buexSwitchWear:
		if !character {
			return 0
		}
		wear := state.layout.Setting.Characters[state.selected].Wear
		for index, value := range wear {
			if value == state.chars[state.selected].wear {
				return index
			}
		}
		return 0
	case buexSwitchUnder:
		// 当前档位要落在开关颗数之内：YK / Yakata 只有 2 颗，
		// 旧存档里的 3/4 档没有对应的开关，画出来会没有一颗亮着。
		return clampInt(state.chars[state.selected].under, 0, s.buexSwitchCount(state, buexSwitchUnder)-1)
	case buexSwitchFace:
		// 颗数跟着当前角色的表情表走，第 13 颗之后的表情才点得到。
		for index := 0; index < s.buexSwitchCount(state, buexSwitchFace); index++ {
			if s.buexFaceValue(state, index) == state.chars[state.selected].face {
				return index
			}
		}
		return 0
	case buexSwitchBackground:
		if len(state.background) == 0 {
			return 0
		}
		for index, name := range state.bgNames {
			if index >= s.buexSwitchCount(state, group) {
				break
			}
			if name == state.background {
				return index
			}
		}
		return 0
	}
	return 0
}

// buexStripValue 第 index 颗脱衣开关对应的阶段号。
//
// 原文档给了一张换算表（ngng_convert_table）：开关下标不一定是阶段号，
// 要查表；表里没有的下标就按自己算。
func (s *Session) buexStripValue(state *buexMuseumState, index int) int {
	convert := state.layout.Setting.StripConvert
	if index >= 0 && index < len(convert) {
		value := convert[index]
		if value >= 0 && value < s.buexMaxStrip() {
			return value
		}
	}
	return clampInt(index, 0, s.buexMaxStrip()-1)
}

// buexFaceValue 第 index 颗表情开关对应的表情编号。
//
// 第 0 颗是"不要表情"。多行表情组（YK / Yakata 这类按姿势分页的布局，
// 以及牝教師3 的 setting5）里，第 index 颗对应当前姿势那一页的第 index-1 个
// 表情；单行组（侵蝕 / 淫獄の放課後2）仍然把整张表情表摊平后取第 index-1 个。
// 旧实现只看 faces[0]（第一页 10 个），第二页之后的表情（11..30）永远选不到；
// 而图名里的变体字母正是按 1..10/11..19/20..30 分段的。
func (s *Session) buexFaceValue(state *buexMuseumState, index int) int {
	if index <= 0 {
		return 0
	}
	if state.selected < len(state.layout.Setting.Characters) {
		faces := state.layout.Setting.Characters[state.selected].Faces
		if s.buexFacePaged(state) {
			// 翻到不存在的页（姿势 c 而没有第三页）时只剩"无表情"一颗，
			// 其余下标没有对应的图，按无表情处理。
			if page := buexFacePage(state); page < len(faces) && index-1 < len(faces[page]) {
				return faces[page][index-1]
			}
			return 0
		}
		if flat := buexFlatFaces(faces); index-1 < len(flat) {
			return flat[index-1]
		}
	}
	return index
}

// buexFacePaged 表情组是否按姿势分页。
//
// 带行距的表情组才分页：YK / Yakata / SLIE 的 space:[38,37]、space:[20,20]
// 表示"一行放不下就换行"，一行就是姿势的一页。
//
// 旧式 [bu viewer] 有两种写法：
//   - 牝教師3：setting5（表情）与 setting6（背景）是两组独立的开关，
//     表情组自己 repeat_x=6、space_y=16，角色表也有 face pose1/pose2 两页，
//     必须按姿势分页，否则 13/14 号表情永远点不到；
//   - tj3slg：setting5/6 是同一组表情的上下两排，legacySwitchGroups 会把
//     它们合并成一个 Count>0 的组，此时仍按一整张摊平的表处理。
func (s *Session) buexFacePaged(state *buexMuseumState) bool {
	if state == nil || buexSwitchFace >= len(state.layout.Setting.Switches) {
		return false
	}
	spec := state.layout.Setting.Switches[buexSwitchFace]
	if spec.SpaceY <= 0 {
		return false
	}
	// 新版布局保持原来的判断：写了行距就按页取，不改已经调好的作品。
	if !state.legacy {
		return true
	}
	// 旧式配置只有在"表情是独立组（Count=0）且角色确有多页表情"时才分页；
	// setting5/6 合并成一组的两排（Count>0）仍是摊平表。
	if spec.Count > 0 {
		return false
	}
	if state.selected >= 0 && state.selected < len(state.layout.Setting.Characters) {
		return len(state.layout.Setting.Characters[state.selected].Faces) > 1
	}
	return false
}

// buexFacePage 当前选中角色的姿势对应表情表的第几页。
func buexFacePage(state *buexMuseumState) int {
	if state == nil || state.selected < 0 || state.selected >= len(state.chars) {
		return 0
	}
	switch state.chars[state.selected].pose {
	case "b":
		return 1
	case "c":
		return 2
	}
	return 0
}

// buexFaceOnPage 表情编号 face 是否在当前姿势那一页上。
//
// 换姿势时用它判断旧表情要不要清掉：同一张脸不一定在每一页上
// （YK 的 1 号角色第一页只有 1..9，第二页是 10..17）。
func buexFaceOnPage(state *buexMuseumState, face int) bool {
	if state == nil || state.selected < 0 || state.selected >= len(state.layout.Setting.Characters) {
		return false
	}
	faces := state.layout.Setting.Characters[state.selected].Faces
	page := buexFacePage(state)
	if page >= len(faces) {
		return false
	}
	for _, value := range faces[page] {
		if value == face {
			return true
		}
	}
	return false
}

// buexFlatFaces 把 [][]int 的表情表摊平成一维。
func buexFlatFaces(rows [][]int) []int {
	total := 0
	for _, row := range rows {
		total += len(row)
	}
	out := make([]int, 0, total)
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

// buexMaxStrip 脱衣阶段的最大档数。
//
// 脱衣设置画面和鉴赏室共用同一份阶段表（[bustup setting] 的 max strip）；
// 拿不到就用原文档换算表的长度，再兜底成 7。
func (s *Session) buexMaxStrip() int {
	if s.config != nil && s.config.Dress.MaxStrip > 0 {
		return s.config.Dress.MaxStrip
	}
	if s.ui != nil && s.ui.buex != nil && len(s.ui.buex.layout.Setting.StripConvert) > 0 {
		return len(s.ui.buex.layout.Setting.StripConvert)
	}
	return config.DressDefaultMaxStrip
}

// drawBuexCharacters 画角色图标（按 order 排）和显示开关。
//
// 多角色同屏（牝教師3）时每颗图标右侧都有自己的 ON/OFF；原版截图里
// 六张角色卡各带一个「表示切り替えボタン」，不是只有选中那一张有。
func (s *Session) drawBuexCharacters(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	characters := state.layout.Setting.Characters
	for _, index := range state.order {
		if index < 0 || index >= len(characters) {
			continue
		}
		s.drawBuexCharacterIcon(canvas, state, index)
		if state.multi {
			s.drawBuexAppearance(canvas, state, index)
		}
	}
	if state.multi {
		return
	}
	// 显示开关画在当前选中角色的图标下面：
	// 布局里它只给了"相对角色图标左上角的偏移"，没有绝对坐标。
	s.drawBuexAppearance(canvas, state, state.selected)
}

// drawBuexCharacterIcon 画一张角色卡，并处理悬停/选中帧。
func (s *Session) drawBuexCharacterIcon(canvas *gfx.BgraCanvas, state *buexMuseumState, index int) {
	characters := state.layout.Setting.Characters
	if index < 0 || index >= len(characters) {
		return
	}
	icon := characters[index].Icon
	frame := 1
	if s.ui.hover.kind == hitBUCharacter && s.ui.hover.index == index {
		frame = 2
	}
	if state.selected == index {
		frame = 3
	}
	s.drawBuexButtonAt(canvas, icon, frame, icon.X, icon.Y)
}

// drawBuexAppearance 画一个角色图标旁的 ON/OFF 开关。
func (s *Session) drawBuexAppearance(canvas *gfx.BgraCanvas, state *buexMuseumState, index int) {
	appearance := buexAppearanceButton(state)
	if appearance.Name == "" {
		return
	}
	if index < 0 || index >= len(state.chars) {
		return
	}
	x, y, ok := buexAppearancePositionOf(state, index)
	if !ok {
		return
	}
	frame := 1
	if !state.chars[index].visible {
		frame = 3
	}
	if s.ui.hover.kind == hitBUAppearance && s.ui.hover.index == index {
		frame = 2
	}
	s.drawBuexButtonAt(canvas, appearance, frame, x, y)
}

// buexAppearanceButton 显示开关（on_off_button）的按钮定义。
//
// 新旧布局用同一个键名；个别旧配置把它写成 appearance，两个都认。
func buexAppearanceButton(state *buexMuseumState) config.ConfigButton {
	if state == nil {
		return config.ConfigButton{}
	}
	appearance := state.layout.Setting.Buttons["on_off_button"]
	if appearance.Name == "" {
		appearance = state.layout.Setting.Buttons["appearance"]
	}
	return appearance
}

// buexAppearancePosition 显示开关的位置：当前角色图标左上角 + 配置偏移。
//
// 偏移是相对角色图标的，不能写死：图标本身是 48x54 的一帧，但各作品的
// 图标尺寸和偏移不同（YK / Yakata 是 0,37，侵蝕是 2,63，淫獄是 0,73），
// 写死会把开关画到图标外面，看起来像"选中框错位"。
func buexAppearancePosition(state *buexMuseumState) (int, int, bool) {
	if state == nil {
		return 0, 0, false
	}
	return buexAppearancePositionOf(state, state.selected)
}

// buexAppearancePositionOf 指定角色的显示开关位置。
//
// 多角色同屏时每个 ON/OFF 都跟着自己的角色卡，不能共用 selected 的坐标。
func buexAppearancePositionOf(state *buexMuseumState, index int) (int, int, bool) {
	if state == nil {
		return 0, 0, false
	}
	characters := state.layout.Setting.Characters
	if index < 0 || index >= len(characters) {
		return 0, 0, false
	}
	appearance := buexAppearanceButton(state)
	if appearance.Name == "" {
		return 0, 0, false
	}
	icon := characters[index].Icon
	return icon.X + appearance.X, icon.Y + appearance.Y, true
}

// drawBuexOrderButtons 画排序按钮（把选中角色往前/往后挪一格）。
func (s *Session) drawBuexOrderButtons(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	for _, item := range []struct {
		kind   uiHitKind
		button config.ConfigButton
	}{
		{hitBUOrderUp, state.layout.Setting.Buttons["order_up"]},
		{hitBUOrderDown, state.layout.Setting.Buttons["order_down"]},
	} {
		if item.button.Name == "" {
			continue
		}
		frame := 1
		if s.ui.hover.kind == item.kind {
			frame = 2
		}
		s.drawBuexButtonAt(canvas, item.button, frame, item.button.X, item.button.Y)
	}
}

// drawBuexPointer 画立ち絵位置バー（站位滑杆）的指示器。
//
// pos_region 是横向活动范围：指示器跟着 position（0…1000）在条上滑动。
// 它不参与距离缩放；距离由 setting_bu 里的两个开关切换近/遠。
func (s *Session) drawBuexPointer(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	button := state.layout.Setting.Buttons["pos_pointer"]
	if button.Name == "" {
		return
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return
	}
	left, top, width := buexPosRegion(state.layout.Setting.PosRegion)
	if width <= image.Width {
		width = image.Width
	}
	fraction := float64(clampInt(buexSelectedPosition(state), 0, 1000)) / 1000
	x := left + int(fraction*float64(width-image.Width))
	y := top
	if y == 0 {
		y = button.Y
	}
	s.drawBuexButtonAt(canvas, button, 1, x, y)
}

// drawBuexFreeSight 画自由視点バー和顔/胸/腰按钮。
//
// 只在旧式 [bu viewer] 明确写了 enable view control = 1 且作品有
// [free-sight screen] 时出现（牝教師3）。新版 buex 布局自己排按钮，
// 这里不能再画一套，否则会和它叠在一起。
func (s *Session) drawBuexFreeSight(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	if state == nil || !state.multi || !state.layout.EnableViewControl {
		return
	}
	if s.config == nil || !s.config.FreeSight.Mode {
		return
	}
	view := s.config.FreeSight
	left, top, width, height := view.Meter[0], view.Meter[1], view.Meter[2], view.Meter[3]
	if width <= 0 || height <= 0 {
		return
	}
	image := s.buexFocusImage(state)
	maxTop := 0
	if image != nil {
		maxTop = max(0, image.Height-s.height)
	}
	fraction := 0.0
	if maxTop > 0 {
		fraction = float64(clampInt(s.buexViewTop(state, image), 0, maxTop)) / float64(maxTop)
	}
	// 轨道本身烘焙在 dress_view_base 上，这里只画滑块。
	if !view.Knob.Empty() {
		if knob := s.composer.Load(view.Knob.Image); knob != nil {
			knobWidth, knobHeight := knob.Width, knob.Height
			if frames := max(1, view.Knob.Frames); frames > 1 && knobHeight%frames == 0 {
				knobHeight /= frames
			}
			x := left + (width-knobWidth)/2
			y := top + int(fraction*float64(max(0, height-knobHeight)))
			canvas.DrawRegion(knob, 0, 0, knobWidth, knobHeight, x, y)
		}
	}
	for index, button := range []config.UIButton{view.Face, view.Bust, view.Hip} {
		if button.Empty() {
			continue
		}
		frame := 0
		if s.ui.hover.kind == hitBUFocus && s.ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
}

// buexFocusImage 取当前角色在近距離档会用的 u 图，用于计算视口上限。
//
// 只做只读探测，找不到就返回 nil（自由視点条仍画，但滑块停在顶部）。
func (s *Session) buexFocusImage(state *buexMuseumState) *gfx.BsgImage {
	if state == nil || state.selected < 0 || state.selected >= len(state.chars) {
		return nil
	}
	character := state.chars[state.selected]
	number := state.selected + 1
	strip := clampInt(character.strip, 0, 9)
	wear := clampInt(character.wear, 0, 99)
	variants := []string{character.pose}
	if character.pose != "a" {
		variants = append(variants, "a")
	}
	for _, variant := range variants {
		for pose := 1; pose <= 4; pose++ {
			name := s.buexTryName(number, "u", strip, wear, pose, variant, 0)
			if name == "" {
				continue
			}
			return s.composer.Load(name)
		}
	}
	return nil
}

// buexFocus 处理顔/胸/腰按钮：切到近距離并按对应基准值定位视口。
func (s *Session) buexFocus(state *buexMuseumState, mode int) {
	if state == nil || state.selected < 0 || state.selected >= len(state.chars) {
		return
	}
	state.chars[state.selected].view = buexViewClose
	image := s.buexFocusImage(state)
	if top, ok := s.buexViewTopFor(image, mode); ok {
		state.viewTop = top
		state.viewTopSet = true
	}
}

// buexFocusAt 按鼠标在自由視点轨道上的纵向位置定位视口。
func (s *Session) buexFocusAt(state *buexMuseumState, y int) {
	if state == nil || s.config == nil || !s.config.FreeSight.Mode {
		return
	}
	view := s.config.FreeSight
	if view.Meter[3] <= 0 {
		return
	}
	image := s.buexFocusImage(state)
	maxTop := 0
	if image != nil {
		maxTop = max(0, image.Height-s.height)
	}
	fraction := float64(y-view.Meter[1]) / float64(view.Meter[3])
	fraction = clampFloat(fraction, 0, 1)
	state.viewTop = clampInt(int(fraction*float64(maxTop)), 0, maxTop)
	state.viewTopSet = true
	if state.selected >= 0 && state.selected < len(state.chars) {
		state.chars[state.selected].view = buexViewClose
	}
}

// buexPosRegion 解析 pos_region。
//
// 原文档写的是 [0, 743, 1019, 25]：左边、上边、右边、宽度。
// 第四个数不是"下边界"——743+25 正好是画面高度 768，
// 所以它是这段活动区域的高度，这里按高度处理。
func buexPosRegion(region [4]int) (left, top, width int) {
	left, top = region[0], region[1]
	width = region[2] - region[0]
	if width <= 0 {
		width = 1024
	}
	return left, top, width
}

// drawBuexToggles 画头发/切入/水汽三个开关。
//
// 这三个按钮图有 5 帧：0 命中掩码、1 关、2 关的悬停、3 开、4 开的悬停。
func (s *Session) drawBuexToggles(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	for _, item := range []struct {
		key  string
		kind uiHitKind
		on   bool
	}{
		{"hair_mode", hitBUHair, s.settings.UnderHair},
		{"cutin_mode", hitBUCutin, s.settings.Cutin},
		{"steam_mode", hitBUSteam, s.settings.Steam},
		{"ahe_mode", hitBUAhe, s.settings.AheOff},
	} {
		button := state.layout.Setting.Buttons[item.key]
		if button.Name == "" {
			continue
		}
		frame := 1
		if item.on {
			frame = 3
		}
		if s.ui.hover.kind == item.kind {
			frame++
		}
		s.drawBuexButtonAt(canvas, button, frame, button.X, button.Y)
	}
}

// ---------------------------------------------------------------- 背景页

// drawBuexBackgroundPage 画背景选择页。
func (s *Session) drawBuexBackgroundPage(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	base := state.layout.Bg.Base
	if base == "" {
		base = "buex_bgbase"
	}
	s.drawImageAt(canvas, base, 0, 0)
	// 页签按钮：两组都在同一张底图上（上一组是立ち絵背景，下面几颗是各个 EV）。
	for index, ref := range state.pages {
		if !ref.available {
			continue
		}
		frame := 1
		if s.ui.hover.kind == hitBUPage && s.ui.hover.index == index {
			frame = 2
		}
		if ref.group == state.bgGroup && ref.page == state.bgPage {
			frame = 3
		}
		s.drawBuexButtonAt(canvas, ref.button, frame, ref.button.X, ref.button.Y)
	}
	thumbs := s.buexPageThumbs(state, state.bgGroup, state.bgPage)
	cellW, cellH := buexThumbCell(state.layout.Bg.ThumbPos)
	for index, point := range state.layout.Bg.ThumbPos {
		if index >= len(thumbs) {
			break
		}
		thumb := thumbs[index]
		if image := s.thumbnailCanvasSized([]string{thumb.image}, cellW, cellH); image != nil {
			canvas.BlitCanvasRegion(image, 0, 0, image.Width, image.Height, point.X, point.Y, 255)
		}
		s.drawBuexCaption(canvas, state, thumb.caption, point.X, point.Y)
		if thumb.image == state.background {
			s.drawBuexFrame(canvas, point.X, point.Y)
		}
	}
}

// drawBuexCaption 画缩略图的标题。
//
// 位置用布局里的 subject 偏移（相对缩略图左上角），
// 原文档给的是 [2, -15]，也就是标题在缩略图上方 15 像素。
func (s *Session) drawBuexCaption(canvas *gfx.BgraCanvas, state *buexMuseumState, caption string, x, y int) {
	if caption == "" || s.textFont == nil || s.config == nil {
		return
	}
	font := s.config.Font
	size := max(10, font.Size/2)
	rendering.DrawText(canvas, s.textFont, caption,
		x+state.layout.Bg.Subject.X, y+state.layout.Bg.Subject.Y,
		size, font.Edge, font.Shadow, font.Height, thumbWidth, 1, 0xffffffff)
}

// drawBuexFrame 画缩略图外面的选中框。
func (s *Session) drawBuexFrame(canvas *gfx.BgraCanvas, x, y int) {
	image := s.composer.Load("data_frame")
	if image == nil {
		return
	}
	canvas.Draw(image, x-2, y-1)
}

// buexPageThumbs 取某一页的缩略图列表。
func (s *Session) buexPageThumbs(state *buexMuseumState, group, page int) []buexThumb {
	perPage := s.buexThumbCount(state)
	switch group {
	case 0:
		start := page * perPage
		if start < 0 || start >= len(state.bgNames) {
			return nil
		}
		end := min(start+perPage, len(state.bgNames))
		thumbs := make([]buexThumb, 0, end-start)
		for _, name := range state.bgNames[start:end] {
			thumbs = append(thumbs, buexThumb{image: name, caption: buexImageCaption(name)})
		}
		return thumbs
	default:
		// group 1..N 是 EV 分类；每个分类可以有多页（YK/Yakata 的 page1/page2）。
		index := state.evOffset + group - 1
		if index < 0 || index >= len(state.evPages) {
			return nil
		}
		start := page * perPage
		if start < 0 || start >= len(state.evPages[index]) {
			return nil
		}
		end := min(start+perPage, len(state.evPages[index]))
		return state.evPages[index][start:end]
	}
}

// buexImageCaption 背景图页的标题就是图名（去掉路径和扩展名）。
func buexImageCaption(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}

// buexPageCount 某一组里有多少页（没有内容的页不算）。
func (s *Session) buexPageCount(state *buexMuseumState, group int) int {
	count := 0
	for _, ref := range state.pages {
		if ref.group != group || !ref.available {
			continue
		}
		count++
	}
	return count
}

// ---------------------------------------------------------------- 编辑页

// drawBuexEditPage 画文章编辑页。
func (s *Session) drawBuexEditPage(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	base := state.layout.Edit.Base
	if base == "" {
		base = state.layout.Bg.Base
	}
	if base == "" {
		base = state.layout.Store.Base
	}
	if base == "" {
		base = "buex_dsbase"
	}
	s.drawImageAt(canvas, base, 0, 0)
	// 底图上盖当前的背景和立绘，让玩家看得见留言配在什么画面上。
	if state.background == "" || s.composer.Load(state.background) == nil {
		state.background = s.buexBackgroundFallback(state)
	}
	if state.background != "" {
		s.drawBuexBackground(canvas, state.background)
	}
	s.drawBuexStandees(canvas, state)
	s.drawBuexEditSubMenu(canvas, state)
	s.drawBuexPreview(canvas, state)
	s.drawBuexEditCommands(canvas, state)
	if state.colorOpen {
		s.drawBuexColorPicker(canvas, state)
	}
}

// drawBuexEditSubMenu 画编辑页的两个子菜单（自由留言 / 对话框留言）。
func (s *Session) drawBuexEditSubMenu(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	for index, button := range state.layout.Edit.SubMenu {
		frame := 1
		if s.ui.hover.kind == hitBUEditSub && s.ui.hover.index == index {
			frame = 2
		}
		if state.editSub == index {
			frame = 3
		}
		s.drawBuexButtonAt(canvas, button, frame, button.X, button.Y)
	}
}

// buexEditCommandOrder 编辑命令的固定次序。
//
// 顺序写在代码里：布局里这些按钮是按名字取的（map），
// 而原引擎画它们时是按这个次序横排的。
var buexEditCommandOrder = []string{"fontsize", "fontcolor", "typesetting", "balloon", "delete"}

// drawBuexEditCommands 画五个编辑命令按钮。
//
// 布局里这五个按钮没写坐标（只有图片名和帧数），
// 原引擎是按固定间距横排的；这里沿用同一排法：x = 42 + i*54，y = 640。
func (s *Session) drawBuexEditCommands(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	for index, key := range buexEditCommandOrder {
		button := state.layout.Edit.Commands[key]
		if button.Name == "" {
			continue
		}
		x := 42 + index*54
		y := 640
		frame := 1
		if s.ui.hover.kind == hitBUEditCommand && s.ui.hover.index == index {
			frame = 2
		}
		s.drawBuexButtonAt(canvas, button, frame, x, y)
	}
}

// buexPreviewBox 留言预览框的位置和大小。
func buexPreviewBox() (x, y, width, height int) {
	return 180, 150, 620, 220
}

// drawBuexPreview 画留言的预览。
func (s *Session) drawBuexPreview(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	x, y, width, height := buexPreviewBox()
	if state.balloon {
		// 气泡框：先铺一层半透明白底，再画边。
		canvas.Fill(x, y, width, height, 255, 255, 255, 40)
		canvas.Fill(x, y, width, 1, 255, 255, 255, 160)
		canvas.Fill(x, y+height-1, width, 1, 255, 255, 255, 160)
		canvas.Fill(x, y, 1, height, 255, 255, 255, 160)
		canvas.Fill(x+width-1, y, 1, height, 255, 255, 255, 160)
	}
	if state.message == "" || s.textFont == nil || s.config == nil {
		return
	}
	font := s.config.Font
	size := buexFontSizes[clampInt(state.fontSize, 0, len(buexFontSizes)-1)]
	color := buexColorValue(state.colorText)
	image := s.textFont.Render(state.message, size, font.Edge, font.Shadow, font.Height, width, 4, color)
	if image == nil {
		return
	}
	// 排版：0 左对齐、1 居中、2 右对齐。
	textWidth := image.Bounds().Dx()
	drawX := x
	switch state.align {
	case 1:
		drawX = x + (width-textWidth)/2
	case 2:
		drawX = x + width - textWidth
	}
	rendering.DrawTextImage(canvas, image, drawX, y)
}

// buexColorValue 把 RGBA 分量拼成文字渲染器要的 0xRRGGBBAA。
func buexColorValue(color [4]int) uint32 {
	return uint32(clampInt(color[0], 0, 255))<<16 |
		uint32(clampInt(color[1], 0, 255))<<8 |
		uint32(clampInt(color[2], 0, 255)) |
		uint32(clampInt(color[3], 0, 255))<<24
}

// drawBuexColorPicker 画取色器浮层。
func (s *Session) drawBuexColorPicker(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	picker := state.layout.Edit.Picker
	originX, originY := s.buexPickerOrigin()
	s.drawImageAt(canvas, picker.Base, originX, originY)
	// 当前选中的目标上画单选标记。
	if state.colorTarget < len(picker.Targets) {
		target := picker.Targets[state.colorTarget]
		s.drawImageAt(canvas, picker.Radio, originX+target.X, originY+target.Y)
		// 高亮框按单选图自己的尺寸画；用 target.X 当宽高会把框拉成一大块。
		if radio := s.composer.Load(picker.Radio); radio != nil {
			canvas.Fill(originX+target.X-2, originY+target.Y-2,
				radio.Width+4, radio.Height+4, 255, 255, 255, 60)
		}
	}
	// 关闭按钮。
	s.drawBuexButtonAt(canvas, picker.Close, 1, originX+picker.Close.X, originY+picker.Close.Y)
	color := s.buexActiveColor(state)
	for index, bar := range picker.Bars {
		x := originX + bar.Bar.X
		y := originY + bar.Bar.Y
		s.drawBuexButtonAt(canvas, bar.Bar, 0, x, y)
		s.drawBuexButtonAt(canvas, bar.Min, 1, originX+bar.Min.X, originY+bar.Min.Y)
		s.drawBuexButtonAt(canvas, bar.Max, 1, originX+bar.Max.X, originY+bar.Max.Y)
		// 当前值在色带上的位置：一条 3 像素宽的白线。
		width, height := s.buexBarSize(bar.Bar)
		if width > 0 && index < len(color) {
			fraction := float64(clampInt(color[index], 0, 255)) / 255
			marker := x + int(fraction*float64(width))
			canvas.Fill(marker, y, 3, height, 255, 255, 255, 255)
		}
	}
}

// buexPickerOrigin 取色器底图的左上角（居中）。
func (s *Session) buexPickerOrigin() (int, int) {
	return (s.width - buexPickerWidth) / 2, (s.height - buexPickerHeight) / 2
}

// buexImageSize 取一张按钮图的尺寸；取不到时返回 0，调用方要自己兜底。
func buexImageSize(s *Session, name string) (int, int) {
	if s == nil || s.composer == nil || name == "" {
		return 0, 0
	}
	image := s.composer.Load(name)
	if image == nil {
		return 0, 0
	}
	return image.Width, image.Height
}

// buexBarSize 色带图片的尺寸：宽用来把 0…255 换算成像素位置，高用来画刻度线。
func (s *Session) buexBarSize(button config.ConfigButton) (int, int) {
	image := s.composer.Load(button.Name)
	if image == nil {
		return 0, 0
	}
	return image.Width, image.Height
}

// buexActiveColor 当前正在编辑的颜色（按目标选文字色或描边色）。
func (s *Session) buexActiveColor(state *buexMuseumState) [4]int {
	if state.colorTarget == 0 {
		return state.colorText
	}
	return state.colorEdge
}

// ---------------------------------------------------------------- 存档页

// buexStoreCommandOrder 存档命令的固定次序。
var buexStoreCommandOrder = []string{"save", "load", "toslot", "output", "clear"}

// drawBuexStorePage 画预设存档页。
func (s *Session) drawBuexStorePage(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	base := state.layout.Store.Base
	if base == "" {
		base = "buex_dsbase"
	}
	s.drawImageAt(canvas, base, 0, 0)
	for index, point := range state.layout.Store.ThumbPos {
		preset := buexPreset{}
		if index < len(state.presets) {
			preset = state.presets[index]
		}
		s.drawBuexPresetThumb(canvas, preset, point.X, point.Y)
		if index == state.selectedPreset {
			s.drawBuexFrame(canvas, point.X, point.Y)
		}
	}
	for index, point := range state.layout.Store.SlotPos {
		preset := buexPreset{}
		if index < len(state.slots) {
			preset = state.slots[index]
		}
		s.drawBuexPresetThumb(canvas, preset, point.X, point.Y)
		if index == state.slotSelected {
			s.drawBuexFrame(canvas, point.X, point.Y)
		}
	}
	s.drawBuexStoreCommands(canvas, state)
}

// drawBuexPresetThumb 画一张预设缩略图（没内容时是一块深色底）。
func (s *Session) drawBuexPresetThumb(canvas *gfx.BgraCanvas, preset buexPreset, x, y int) {
	if preset.Background == "" {
		canvas.Fill(x, y, thumbWidth, thumbHeight, 20, 18, 28, 255)
		return
	}
	image := s.thumbnailCanvas([]string{preset.Background})
	if image == nil {
		canvas.Fill(x, y, thumbWidth, thumbHeight, 20, 18, 28, 255)
		return
	}
	canvas.BlitCanvasRegion(image, 0, 0, image.Width, image.Height, x, y, 255)
}

// drawBuexStoreCommands 画存档命令按钮。
//
// save / load / toslot 三个按钮布局里没有坐标（作者是按固定间距排的），
// 这里沿用同一排法：x = 42，y = 404 + 50*i。
// output / clear 有坐标，直接用配置里的。
func (s *Session) drawBuexStoreCommands(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	for index, key := range buexStoreCommandOrder {
		button := state.layout.Store.Commands[key]
		if button.Name == "" {
			continue
		}
		x, y := button.X, button.Y
		switch key {
		case "save":
			x, y = 42, 404
		case "load":
			x, y = 42, 454
		case "toslot":
			// 布局没给坐标；output/clear 占了 504/554，
			// 放到它们下面，避免三个按钮叠在同一个位置。
			x, y = 42, 604
		}
		frame := 1
		if s.buexStoreDisabled(state, key) {
			frame = 3
		} else if s.ui.hover.kind == hitBUStoreCommand && s.ui.hover.index == index {
			frame = 2
		}
		s.drawBuexButtonAt(canvas, button, frame, x, y)
	}
}

// buexStoreDisabled 某个存档命令现在能不能用（不能用的画成灰的，也不响应点击）。
func (s *Session) buexStoreDisabled(state *buexMuseumState, key string) bool {
	switch key {
	case "load", "clear":
		return state.selectedPreset >= len(state.presets) || state.presets[state.selectedPreset].Background == ""
	}
	return false
}

// ---------------------------------------------------------------- 菜单与提示

// drawBuexMenu 画顶部主菜单。
//
// 菜单按钮图的第 0 帧是白色命中掩码（只用来判命中，永远不画），
// 第 1 帧是常态、第 2 帧是悬停、第 3 帧是当前页。
func (s *Session) drawBuexMenu(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	for index, entry := range state.layout.Menu {
		frame := 1
		if s.ui.hover.kind == hitBUMenu && s.ui.hover.index == index {
			frame = 2
		}
		if buexMenuPage(entry.Key) == state.page {
			frame = 3
		}
		s.drawBuexButtonAt(canvas, entry.Button, frame, entry.Button.X, entry.Button.Y)
	}
	// 布局里没有"返回"入口时补一个，玩家才出得去。
	if state.layout.Menu == nil || !buexHasMenuKey(state.layout.Menu, "back_to_thumbs") {
		s.drawBuexButtonAt(canvas, config.ConfigButton{Name: "buex_main_back", MaxState: 3, X: 904, Y: 5}, 1, 904, 5)
	}
}

// buexMenuPage 主菜单项对应哪个子页（-1 表示不是子页入口）。
func buexMenuPage(key string) int {
	switch key {
	case "setting_bu":
		return buexPageStandee
	case "bg_select":
		return buexPageBackground
	case "edit_message":
		return buexPageEdit
	case "data_store":
		return buexPageStore
	}
	return -1
}

// buexHasMenuKey 主菜单里有没有某个入口。
func buexHasMenuKey(menu []config.BuexMenuEntry, key string) bool {
	for _, entry := range menu {
		if entry.Key == key {
			return true
		}
	}
	return false
}

// drawBuexNotice 画提示语。
//
// 原引擎的 notice 是几张整屏的说明图（buex_notice_*），
// 那些图讲的是操作说明，不是"这个操作被拒绝了"，
// 所以这里不用它们，只画一行短文字提示，过一会儿自动消失。
func (s *Session) drawBuexNotice(canvas *gfx.BgraCanvas, state *buexMuseumState) {
	if state.notice == "" {
		return
	}
	if time.Now().After(state.noticeUntil) {
		state.notice = ""
		return
	}
	if s.textFont == nil || s.config == nil {
		return
	}
	font := s.config.Font
	rendering.DrawText(canvas, s.textFont, state.notice, 30, 700,
		font.Size, font.Edge, font.Shadow, font.Height, 900, 1, 0xffffffff)
}

// buexNotice 让鉴赏室显示一条提示。
func (s *Session) buexNotice(state *buexMuseumState, text string) {
	state.notice = text
	state.noticeUntil = time.Now().Add(buexNoticeDuration)
}

// ---------------------------------------------------------------- 命中

func (s *Session) hitBustupMuseum(x, y int) uiHit {
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if ui.kdStrip {
		return s.hitKDStrip(x, y)
	}
	if ui.md2Bu != nil {
		return s.md2BuHit(x, y)
	}
	if ui.buex == nil {
		return s.hitLegacyBustupSelect(x, y)
	}
	state := ui.buex
	// 鉴赏室自己的主菜单优先判：它是这一页唯一的导航入口
	// （背景选择 / 立绘设定 / 文章設定 / 存档 / 返回）。
	for index, entry := range state.layout.Menu {
		if hitBuexButton(s, entry.Button, x, y) {
			return uiHit{kind: hitBUMenu, index: index}
		}
	}
	if !buexHasMenuKey(state.layout.Menu, "back_to_thumbs") {
		back := config.ConfigButton{Name: "buex_main_back", MaxState: 3, X: 904, Y: 5}
		if hitBuexButton(s, back, x, y) {
			return uiHit{kind: hitBUClose}
		}
	}
	// 注意：这里不再兜底判鉴赏室模式页签条。页签条不再画（见 drawBustupMuseum），
	// 再留命中会让 Yakata 左侧面板上那排页签的透明区域抢走换装开关的点击。
	switch state.page {
	case buexPageBackground:
		return s.hitBuexBackgroundPage(state, x, y)
	case buexPageEdit:
		return s.hitBuexEditPage(state, x, y)
	case buexPageStore:
		return s.hitBuexStorePage(state, x, y)
	}
	return s.hitBuexStandeePage(state, x, y)
}

// hitBuexStandeePage 立ち絵页的命中判定。
func (s *Session) hitBuexStandeePage(state *buexMuseumState, x, y int) uiHit {
	// 开关组
	width, height := s.buexSwitchSize(state)
	for group := 0; group < buexSwitchCount && group < len(state.layout.Setting.Switches); group++ {
		spec := state.layout.Setting.Switches[group]
		if buexSwitchDisabled(spec, s.width, s.height) {
			continue
		}
		count := s.buexSwitchCount(state, group)
		for index := 0; index < count; index++ {
			sx, sy := buexSwitchPosition(spec, count, index)
			if x >= sx && y >= sy && x < sx+width && y < sy+height {
				return uiHit{kind: buexSwitchHit(group), index: index}
			}
		}
	}
	// 显示开关画在角色卡上面，命中也要优先于角色卡：
	// ON/OFF 的矩形和图标下沿有重叠，先判图标会让开关点不动。
	// 多角色同屏时每张角色卡各有一个 ON/OFF，单角色布局只命中选中角色的那一个。
	if appearance := buexAppearanceButton(state); appearance.Name != "" {
		indices := []int{state.selected}
		if state.multi {
			indices = state.order
		}
		for _, index := range indices {
			ax, ay, ok := buexAppearancePositionOf(state, index)
			if !ok {
				continue
			}
			if hitBuexButton(s, config.ConfigButton{
				Name: appearance.Name, MaxState: appearance.MaxState, X: ax, Y: ay,
			}, x, y) {
				return uiHit{kind: hitBUAppearance, index: index}
			}
		}
	}
	// 角色图标
	characters := state.layout.Setting.Characters
	for _, index := range state.order {
		if index < 0 || index >= len(characters) {
			continue
		}
		if hitBuexButton(s, characters[index].Icon, x, y) {
			return uiHit{kind: hitBUCharacter, index: index}
		}
	}
	// 排序按钮
	for _, item := range []struct {
		kind   uiHitKind
		button config.ConfigButton
	}{
		{hitBUOrderUp, state.layout.Setting.Buttons["order_up"]},
		{hitBUOrderDown, state.layout.Setting.Buttons["order_down"]},
	} {
		if hitBuexButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	// 三个开关按钮
	for _, item := range []struct {
		key  string
		kind uiHitKind
	}{
		{"hair_mode", hitBUHair},
		{"cutin_mode", hitBUCutin},
		{"steam_mode", hitBUSteam},
		{"ahe_mode", hitBUAhe},
	} {
		if hitBuexButton(s, state.layout.Setting.Buttons[item.key], x, y) {
			return uiHit{kind: item.kind}
		}
	}
	// 自由視点：顔/胸/腰按钮和纵向轨道（旧式 [bu viewer] enable view control）。
	if state.multi && state.layout.EnableViewControl && s.config != nil && s.config.FreeSight.Mode {
		for index, button := range []config.UIButton{s.config.FreeSight.Face, s.config.FreeSight.Bust, s.config.FreeSight.Hip} {
			if !button.Empty() && hitButton(s, button, x, y) {
				return uiHit{kind: hitBUFocus, index: index}
			}
		}
		meter := s.config.FreeSight.Meter
		if meter[2] > 0 && meter[3] > 0 &&
			x >= meter[0] && x < meter[0]+meter[2] &&
			y >= meter[1] && y < meter[1]+meter[3] {
			return uiHit{kind: hitBUFocusBar, index: -1}
		}
	}
	// 站立位置滑杆：活动区域里点哪，人物就移到哪。
	if hit, ok := s.hitBuexPointer(state, x, y); ok {
		return hit
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitBuexPointer 判断是否点在站立位置滑杆的活动区域里。
func (s *Session) hitBuexPointer(state *buexMuseumState, x, y int) (uiHit, bool) {
	region := state.layout.Setting.PosRegion
	left, top, width := buexPosRegion(region)
	// 第四个数是这段区域的高度（见 buexPosRegion 的说明）。
	height := region[3]
	if height <= 0 {
		height = 25
	}
	if x < left || x >= left+width || y < top || y >= top+height {
		return uiHit{}, false
	}
	return uiHit{kind: hitBUPointer}, true
}

// buexThumbCell 从 thumb_pos 的间距推导格子尺寸。
//
// buex 的背景页坐标是逐格写出的（如 421+224*n, 26+123*n），格子尺寸等于
// 最小正间距；写死 126x96 会让缩略图只占格子的一半，命中区也少一半。
func buexThumbCell(points []config.UIPoint) (int, int) {
	if len(points) == 0 {
		return thumbWidth, thumbHeight
	}
	origin := points[0]
	bestX, bestY := 0, 0
	for _, point := range points {
		if delta := point.X - origin.X; delta > 0 && (bestX == 0 || delta < bestX) {
			bestX = delta
		}
		if delta := point.Y - origin.Y; delta > 0 && (bestY == 0 || delta < bestY) {
			bestY = delta
		}
	}
	if bestX <= 0 {
		bestX = thumbWidth
	}
	if bestY <= 0 {
		bestY = thumbHeight
	}
	return bestX, bestY
}

// hitBuexBackgroundPage 背景选择页的命中判定。
func (s *Session) hitBuexBackgroundPage(state *buexMuseumState, x, y int) uiHit {
	for index, ref := range state.pages {
		if !ref.available {
			continue
		}
		if hitBuexButton(s, ref.button, x, y) {
			return uiHit{kind: hitBUPage, index: index}
		}
	}
	thumbs := s.buexPageThumbs(state, state.bgGroup, state.bgPage)
	cellW, cellH := buexThumbCell(state.layout.Bg.ThumbPos)
	for index, point := range state.layout.Bg.ThumbPos {
		if index >= len(thumbs) {
			break
		}
		if x >= point.X && y >= point.Y && x < point.X+cellW && y < point.Y+cellH {
			return uiHit{kind: hitBUThumb, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitBuexEditPage 文章编辑页的命中判定。
func (s *Session) hitBuexEditPage(state *buexMuseumState, x, y int) uiHit {
	// 取色器是浮层，压在其它控件上面。
	if state.colorOpen {
		return s.hitBuexColorPicker(state, x, y)
	}
	for index, button := range state.layout.Edit.SubMenu {
		if hitBuexButton(s, button, x, y) {
			return uiHit{kind: hitBUEditSub, index: index}
		}
	}
	for index, key := range buexEditCommandOrder {
		button := state.layout.Edit.Commands[key]
		if button.Name == "" {
			continue
		}
		if hitBuexButton(s, config.ConfigButton{Name: button.Name, MaxState: button.MaxState, X: 42 + index*54, Y: 640}, x, y) {
			return uiHit{kind: hitBUEditCommand, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitBuexColorPicker 取色器的命中判定。
func (s *Session) hitBuexColorPicker(state *buexMuseumState, x, y int) uiHit {
	picker := state.layout.Edit.Picker
	originX, originY := s.buexPickerOrigin()
	radioW, radioH := buexImageSize(s, picker.Radio)
	if radioW <= 0 {
		radioW, radioH = 24, 24
	}
	for index, target := range picker.Targets {
		tx := originX + target.X
		ty := originY + target.Y
		if x >= tx-2 && y >= ty-2 && x < tx+radioW && y < ty+radioH {
			return uiHit{kind: hitBUColorTarget, index: index}
		}
	}
	if hitBuexButton(s, config.ConfigButton{Name: picker.Close.Name, MaxState: picker.Close.MaxState, X: originX + picker.Close.X, Y: originY + picker.Close.Y}, x, y) {
		return uiHit{kind: hitBUColorClose}
	}
	for index, bar := range picker.Bars {
		width, height := s.buexBarSize(bar.Bar)
		bx := originX + bar.Bar.X
		by := originY + bar.Bar.Y
		if width > 0 && x >= bx && x < bx+width && y >= by && y < by+height {
			return uiHit{kind: hitBUColorBar, index: index}
		}
		if hitBuexButton(s, config.ConfigButton{Name: bar.Min.Name, MaxState: bar.Min.MaxState, X: originX + bar.Min.X, Y: originY + bar.Min.Y}, x, y) {
			return uiHit{kind: hitBUColorMin, index: index}
		}
		if hitBuexButton(s, config.ConfigButton{Name: bar.Max.Name, MaxState: bar.Max.MaxState, X: originX + bar.Max.X, Y: originY + bar.Max.Y}, x, y) {
			return uiHit{kind: hitBUColorMax, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// hitBuexStorePage 存档页的命中判定。
func (s *Session) hitBuexStorePage(state *buexMuseumState, x, y int) uiHit {
	for index, point := range state.layout.Store.ThumbPos {
		if x >= point.X && y >= point.Y && x < point.X+thumbWidth && y < point.Y+thumbHeight {
			return uiHit{kind: hitBUSlot, index: index}
		}
	}
	for index, point := range state.layout.Store.SlotPos {
		if x >= point.X && y >= point.Y && x < point.X+thumbWidth && y < point.Y+thumbHeight {
			return uiHit{kind: hitBUSlot, index: buexPresetCount + index}
		}
	}
	for index, key := range buexStoreCommandOrder {
		button := state.layout.Store.Commands[key]
		if button.Name == "" || s.buexStoreDisabled(state, key) {
			continue
		}
		x, y := button.X, button.Y
		switch key {
		case "save":
			x, y = 42, 404
		case "load":
			x, y = 42, 454
		case "toslot":
			// 布局没给坐标；output/clear 占了 504/554，
			// 放到它们下面，避免三个按钮叠在同一个位置。
			x, y = 42, 604
		}
		if hitBuexButton(s, config.ConfigButton{Name: button.Name, MaxState: button.MaxState, X: x, Y: y}, x, y) {
			return uiHit{kind: hitBUStoreCommand, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// ---------------------------------------------------------------- 交互

func (s *Session) activateBustupMuseum(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil {
		return
	}
	if ui.kdStrip {
		s.activateKDStrip(hit)
		return
	}
	if ui.md2Bu != nil {
		s.activateMD2Bu(hit, x, y)
		return
	}
	if ui.buex == nil {
		s.activateLegacyBustupSelect(hit, x, y)
		return
	}
	state := ui.buex
	switch hit.kind {
	case hitBUClose:
		s.buexLeave()
	case hitBUMenu:
		if hit.index >= 0 && hit.index < len(state.layout.Menu) {
			s.buexMenu(state, state.layout.Menu[hit.index].Key)
		}
	case hitBUCharacter:
		if hit.index >= 0 && hit.index < len(state.chars) {
			state.selected = hit.index
		}
	case hitBUAppearance:
		index := state.selected
		if state.multi && hit.index >= 0 && hit.index < len(state.chars) {
			index = hit.index
		}
		if index >= 0 && index < len(state.chars) {
			state.chars[index].visible = !state.chars[index].visible
		}
	case hitBUOrderUp:
		s.buexMoveOrder(state, -1)
	case hitBUOrderDown:
		s.buexMoveOrder(state, 1)
	case hitBUPose:
		s.buexSetSwitch(state, buexSwitchPose, hit.index)
	case hitBUView:
		s.buexSetSwitch(state, buexSwitchView, hit.index)
	case hitBUNgng:
		s.buexSetSwitch(state, buexSwitchNgng, hit.index)
	case hitBUWear:
		s.buexSetSwitch(state, buexSwitchWear, hit.index)
	case hitBUUnder:
		s.buexSetSwitch(state, buexSwitchUnder, hit.index)
	case hitBUFace:
		s.buexSetSwitch(state, buexSwitchFace, hit.index)
	case hitBUHair:
		s.settings.UnderHair = !s.settings.UnderHair
		s.saveGameSettings()
	case hitBUCutin:
		s.settings.Cutin = !s.settings.Cutin
		s.saveGameSettings()
	case hitBUSteam:
		s.settings.Steam = !s.settings.Steam
		s.saveGameSettings()
	case hitBUAhe:
		s.settings.AheOff = !s.settings.AheOff
		s.saveGameSettings()
		s.applyDisplaySettings()
		s.invalidateUI()
	case hitBUBackground:
		s.buexSetSwitch(state, buexSwitchBackground, hit.index)
	case hitBUPointer:
		s.buexSetPosition(state, x)
	case hitBUFocus:
		s.buexFocus(state, hit.index)
	case hitBUFocusBar:
		s.buexFocusAt(state, y)
	case hitBUPage:
		s.buexSetPage(state, hit.index)
	case hitBUThumb:
		s.buexPickBackground(state, hit.index)
	case hitBUEditSub:
		s.buexSetEditSub(state, hit.index)
	case hitBUEditCommand:
		s.buexEditCommand(state, hit.index)
	case hitBUColorTarget:
		if hit.index >= 0 {
			state.colorTarget = hit.index
		}
	case hitBUColorClose:
		state.colorOpen = false
	case hitBUColorBar:
		s.buexSetColor(state, hit.index, x, true)
	case hitBUColorMin:
		s.buexSetColor(state, hit.index, 0, false)
	case hitBUColorMax:
		s.buexSetColor(state, hit.index, 255, false)
	case hitBUSlot:
		s.buexSelectSlot(state, hit.index)
	case hitBUStoreCommand:
		s.buexStoreCommand(state, hit.index)
	}
	s.composer.Invalidate()
}

// buexLeave 离开鉴赏室，回鉴赏室主菜单。
func (s *Session) buexLeave() {
	ui := s.ui
	if ui == nil {
		return
	}
	// 旧式查看器是从"角色选择页"进来的，退出要回到那一页；
	// 新版 buex 是鉴赏室主菜单的一页，退出回主菜单。
	legacy := ui.buex != nil && ui.buex.legacy
	legacySelect := ui.buex != nil && ui.buex.legacySelect
	ui.kind = uiMuseum
	if legacy && legacySelect {
		ui.kind = uiMuseumBU
	}
	ui.buex = nil
	ui.hover = uiHit{kind: hitNone, index: -1}
}

// buexMenu 处理顶部主菜单的点击。
func (s *Session) buexMenu(state *buexMuseumState, key string) {
	switch key {
	case "setting_bu":
		state.page = buexPageStandee
	case "bg_select":
		state.page = buexPageBackground
	case "setting_bgv":
		if !s.openBGVMuseum() {
			s.buexNotice(state, "BGV データが見つかりません")
		}
	case "edit_message":
		state.page = buexPageEdit
		state.editing = true
		s.setHostRequest("edit_message")
	case "data_store":
		state.page = buexPageStore
	case "back_to_thumbs":
		s.buexLeave()
		return
	}
	s.ui.hover = uiHit{kind: hitNone, index: -1}
}

// buexSetSwitch 处理开关组的点击。
func (s *Session) buexSetSwitch(state *buexMuseumState, group, index int) {
	// 背景开关不依赖当前角色：即使角色表为空也要能换背景。
	if group == buexSwitchBackground {
		if index >= 0 && index < len(state.bgNames) {
			state.background = state.bgNames[index]
			s.invalidateUI()
		}
		return
	}
	if state.selected < 0 || state.selected >= len(state.chars) {
		return
	}
	character := &state.chars[state.selected]
	switch group {
	case buexSwitchPose:
		character.pose = "a"
		if index > 0 {
			character.pose = "b"
		}
		// 表情表按姿势分页时（YK / Yakata / 牝教師3），换姿势后原来那张脸
		// 可能不在新的一页上，否则画的是"姿势 b + 姿势 a 的脸"。
		// 多角色同屏的旧式查看器改为落到新页的第一张表情：原版不会让人物
		// 变成没有眼睛的空白脸；新版 buex 保持原来的"无表情"行为。
		if s.buexFacePaged(state) && character.face != 0 && !buexFaceOnPage(state, character.face) {
			character.face = 0
			if state.multi && state.selected < len(state.layout.Setting.Characters) {
				character.face = buexPageDefaultFace(state.layout.Setting.Characters[state.selected], buexFacePage(state))
			}
		}
	case buexSwitchView:
		// 左开关（index 0）= 遠距離 f 图；右开关（index 1）= 近距離。
		// 有 u 素材的作品换 u 图，没有的（侵蝕/淫獄）仍然放大 f 图。
		character.view = buexViewBust
		if index > 0 {
			character.view = buexViewClose
		}
	case buexSwitchNgng:
		character.strip = s.buexStripValue(state, index)
	case buexSwitchWear:
		character.wear = index + 1
		if state.selected < len(state.layout.Setting.Characters) {
			wear := state.layout.Setting.Characters[state.selected].Wear
			if index >= 0 && index < len(wear) {
				character.wear = wear[index]
			}
		}
	case buexSwitchUnder:
		// 档位上限跟着开关颗数走：YK / Yakata 只有 2 颗，写死 5 会越界。
		character.under = clampInt(index, 0, s.buexSwitchCount(state, buexSwitchUnder)-1)
		// 换下着要顺带把脱衣阶段推到能露出下着的档位。
		character.strip = min(2+character.under, s.buexMaxStrip()-1)
	case buexSwitchFace:
		character.face = s.buexFaceValue(state, index)
		if s.buexFacePaged(state) {
			// 表情是按姿势分页的：选表情不换姿势，姿势由上面的开关决定。
			break
		}
		// 表情编号跨 1..30，图名里的变体字母跟着表情分段：
		// 1..10 → a，11..19 → b，20..30 → c。旧实现只分 a/b，
		// 12 号之后的表情（…c20）永远拼不出来。
		switch {
		case character.face >= 20:
			character.pose = "c"
		case character.face >= 11:
			character.pose = "b"
		default:
			character.pose = "a"
		}
	}
}

// buexMoveOrder 把选中的角色往前/往后挪一格。
func (s *Session) buexMoveOrder(state *buexMuseumState, delta int) {
	position := -1
	for index, value := range state.order {
		if value == state.selected {
			position = index
			break
		}
	}
	if position < 0 {
		return
	}
	target := position + delta
	if target < 0 || target >= len(state.order) {
		return
	}
	state.order[position], state.order[target] = state.order[target], state.order[position]
}

// buexSetPosition 按点击位置设置站立位置（0…1000，500 = 中央）。
func (s *Session) buexSetPosition(state *buexMuseumState, x int) {
	left, _, width := buexPosRegion(state.layout.Setting.PosRegion)
	if width <= 0 {
		return
	}
	fraction := float64(x-left) / float64(width)
	position := clampInt(int(fraction*1000), 0, 1000)
	// 多角色同屏时只移动当前选中的角色；原版手册明确写的是
	// 「選択中のキャラクターの立ち位置」。
	if state.multi && state.selected >= 0 && state.selected < len(state.chars) {
		state.chars[state.selected].position = position
		return
	}
	state.position = position
}

// buexSetPage 切换背景页的页签。
func (s *Session) buexSetPage(state *buexMuseumState, index int) {
	if index < 0 || index >= len(state.pages) {
		return
	}
	ref := state.pages[index]
	if !ref.available {
		return
	}
	state.bgGroup = ref.group
	state.bgPage = ref.page
}

// buexPickBackground 选中一张背景并回到立ち絵页。
func (s *Session) buexPickBackground(state *buexMuseumState, index int) {
	thumbs := s.buexPageThumbs(state, state.bgGroup, state.bgPage)
	if index < 0 || index >= len(thumbs) {
		return
	}
	state.background = thumbs[index].image
	state.page = buexPageStandee
}

// buexSetEditSub 切换编辑页的子菜单。
func (s *Session) buexSetEditSub(state *buexMuseumState, index int) {
	if index < 0 {
		return
	}
	state.editSub = index
	// 自由留言那一个需要真的弹出输入框；对话框留言不用再输入一次。
	if index == 0 {
		state.editing = true
		s.setHostRequest("edit_message")
	}
}

// buexEditCommand 执行一条编辑命令。
func (s *Session) buexEditCommand(state *buexMuseumState, index int) {
	switch index {
	case 0:
		state.fontSize = (state.fontSize + 1) % len(buexFontSizes)
	case 1:
		state.colorOpen = true
	case 2:
		state.align = (state.align + 1) % 3
	case 3:
		state.balloon = !state.balloon
	case 4:
		state.message = ""
	}
}

// buexSetColor 改颜色的一个通道。
//
// byPosition 为真时按鼠标在色带上的横向位置取值（0…255），
// 否则直接用 value（最小/最大按钮给的是 0 或 255）。
func (s *Session) buexSetColor(state *buexMuseumState, index, value int, byPosition bool) {
	picker := state.layout.Edit.Picker
	if index < 0 || index >= len(picker.Bars) {
		return
	}
	color := s.buexActiveColor(state)
	if byPosition {
		bar := picker.Bars[index].Bar
		width, _ := s.buexBarSize(bar)
		if width <= 0 {
			return
		}
		originX, _ := s.buexPickerOrigin()
		fraction := float64(value-(originX+bar.X)) / float64(width)
		color[index] = clampInt(int(fraction*255), 0, 255)
	} else {
		color[index] = clampInt(value, 0, 255)
	}
	if state.colorTarget == 0 {
		state.colorText = color
	} else {
		state.colorEdge = color
	}
}

// buexSelectSlot 选中存档格（0…35 是预设格，之后是左侧的槽位）。
func (s *Session) buexSelectSlot(state *buexMuseumState, index int) {
	if index < buexPresetCount {
		state.selectedPreset = clampInt(index, 0, max(0, len(state.presets)-1))
		return
	}
	state.slotSelected = clampInt(index-buexPresetCount, 0, max(0, len(state.slots)-1))
}

// buexStoreCommand 执行一条存档命令。
func (s *Session) buexStoreCommand(state *buexMuseumState, index int) {
	if index < 0 || index >= len(buexStoreCommandOrder) {
		return
	}
	switch buexStoreCommandOrder[index] {
	case "save":
		state.presets[state.selectedPreset] = s.buexCurrentPreset(state)
		saveBuexPresets(s, state.presets, state.slots)
		s.buexNotice(state, "保存しました")
	case "load":
		if state.selectedPreset >= len(state.presets) || state.presets[state.selectedPreset].Background == "" {
			s.buexNotice(state, "空きスロットです")
			return
		}
		s.buexApplyPreset(state, state.presets[state.selectedPreset])
		s.buexNotice(state, "読み込みました")
	case "toslot":
		state.slots[state.slotSelected] = s.buexCurrentPreset(state)
		saveBuexPresets(s, state.presets, state.slots)
		s.buexNotice(state, "スロットに書き込みました")
	case "output":
		if !s.buexExportPreset(state) {
			s.buexNotice(state, "書き出しに失敗しました")
			return
		}
		s.buexNotice(state, "書き出しました")
	case "clear":
		state.presets[state.selectedPreset] = buexPreset{}
		saveBuexPresets(s, state.presets, state.slots)
		s.buexNotice(state, "消去しました")
	}
}

// buexCurrentPreset 把当前状态打包成一条预设。
func (s *Session) buexCurrentPreset(state *buexMuseumState) buexPreset {
	preset := buexPreset{
		// 角色编号用 1 基，和 bu<NN> 里的 NN 对得上。
		Character:  state.selected + 1,
		Background: state.background,
		Message:    state.message,
	}
	if state.selected >= 0 && state.selected < len(state.chars) {
		character := state.chars[state.selected]
		preset.View = character.view
		preset.Strip = character.strip
		preset.Wear = character.wear
		preset.Pose = character.pose
		preset.Face = character.face
		preset.Under = character.under
	}
	return preset
}

// buexApplyPreset 把一条预设套回当前状态。
func (s *Session) buexApplyPreset(state *buexMuseumState, preset buexPreset) {
	state.background = s.buexPresetBackground(state, preset.Background)
	state.message = preset.Message
	index := clampInt(preset.Character-1, 0, max(0, len(state.chars)-1))
	state.selected = index
	if index >= len(state.chars) {
		return
	}
	character := &state.chars[index]
	character.view = buexNormalizeView(preset.View)
	character.strip = clampInt(preset.Strip, 0, max(0, s.buexMaxStrip()-1))
	character.wear = max(1, preset.Wear)
	character.pose = preset.Pose
	if character.pose != "a" && character.pose != "b" {
		character.pose = "a"
	}
	character.face = max(0, preset.Face)
	character.under = clampInt(preset.Under, 0, 4)
}

// buexPresetBackground 决定套用预设后该铺哪张背景。
//
// 预设里的 background 可能是空的（旧存档、空槽位），也可能指向这台机器上
// 不存在的图（换过发行版/素材包）。直接赋值会把当前能用的背景换成空字符串，
// 玩家读一次预设就回到黑屏。所以：预设的图能用就用；不能用时保留当前能用的
// 背景；当前也废了才重新兜一张；连兜底都找不到才保留预设里的名字
// （绘制时还会再兜一次，不会比空字符串更糟）。
func (s *Session) buexPresetBackground(state *buexMuseumState, requested string) string {
	if s.buexBackgroundLoads(requested) {
		return requested
	}
	if s.buexBackgroundLoads(state.background) {
		return state.background
	}
	if fallback := s.buexBackgroundFallback(state); fallback != "" {
		return fallback
	}
	return requested
}

// ---------------------------------------------------------------- 立绘图名

// buexStandeeName 拼出并试探当前该显示哪张身体图。
//
// 名字格式：bu<NN><u|f><脱衣阶段><服装两位><姿势一位><变体>。
// 大多数作品的鉴赏室只有 f 图；牝教師3 额外有 u 图，近距離（view=close）
// 优先换 u，取不到时退回 f 再按 buexCloseScale 放大。
//
// 表情是**另一张**叠加图（bu01f0001a12 这种），不能拼进身体图名里：
// 那张图的部件是竖排的眨眼帧条，直接当身体贴会得到一条重复的人脸。
// 不是每个组合都有素材，所以按下面的顺序逐个试探：
//  1. 当前变体字母下的精确名字，姿势号 1…4；
//  2. 变体字母退到 a 再试一遍；
//  3. 基础图 bu<NN>f0011a。
func (s *Session) buexStandeeName(state *buexMuseumState, index int, character buexCharacterState) (string, bool) {
	number := index + 1
	useUp := state != nil && state.multi && character.view == buexViewClose
	views := []string{"f"}
	if useUp {
		views = []string{"u", "f"}
	}
	letter := character.pose
	if letter != "a" && letter != "b" {
		letter = "a"
	}
	strip := clampInt(character.strip, 0, 9)
	wear := clampInt(character.wear, 0, 99)
	// 变体字母：先用当前的，用不到再退到 a。
	variants := []string{letter}
	if letter != "a" {
		variants = append(variants, "a")
	}
	for _, view := range views {
		for _, variant := range variants {
			for pose := 1; pose <= 4; pose++ {
				if name := s.buexTryName(number, view, strip, wear, pose, variant, 0); name != "" {
					return name, view == "u"
				}
			}
		}
	}
	// 基础图：只试 f。u 的基础图在屏幕上会显示成一张过大的半身像，
	// 找不到精确状态时宁可用 f，画面至少完整。
	if candidate := fmt.Sprintf("bu%02df0011a", number); s.composer.Load(candidate) != nil {
		return candidate, false
	}
	return "", false
}

// buexDefaultFace 选鉴赏室打开时的默认表情。
//
// 身体图的眼睛位置是空的，必须叠一张表情图；03 是原版剧情的默认表情
// （SM/SMIE 的初遇场景直接用 bu06f0011a03）。刻意不选 0：0 在文件命名里
// 没有对应素材，等于"不叠眼睛"。表格里没有 03 的作品退回第一项。
func buexDefaultFace(character config.BuexCharacter) int {
	fallback := 0
	for _, page := range character.Faces {
		for _, face := range page {
			if face <= 0 {
				continue
			}
			if fallback == 0 {
				fallback = face
			}
			if face == 3 {
				return 3
			}
		}
	}
	return fallback
}

// buexPageDefaultFace 取某一页表情表的默认表情。
//
// 优先 3（系列默认表情），没有就取该页第一张。只用于"换姿势后脸不在
// 当前页"的多角色查看器，避免人物变成没有眼睛的空白脸。
func buexPageDefaultFace(character config.BuexCharacter, page int) int {
	if page < 0 || page >= len(character.Faces) {
		return 0
	}
	first := 0
	for _, face := range character.Faces[page] {
		if face <= 0 {
			continue
		}
		if first == 0 {
			first = face
		}
		if face == 3 {
			return 3
		}
	}
	return first
}

// buexFaceName 拼出表情叠加图的名字。
//
// 表情图固定是"strip=0、wear=00、pose=1"那一套：
// bu01f0001a12 = 1 号角色、半身、12 号表情、变体 a。
// 它是独立的一张合成图，叠在身体图上，不是身体图名的后缀。
//
// up 为真时必须是同一套 u 载体（bu01u0001a12）：表情图是带容器坐标的
// 合成图，f 表情叠到 u 身体上会整块飘到脸外面。
func (s *Session) buexFaceName(state *buexMuseumState, index int, character buexCharacterState, up bool) string {
	if character.face <= 0 {
		return ""
	}
	number := index + 1
	view := "f"
	if up {
		view = "u"
	}
	letter := character.pose
	if letter != "a" && letter != "b" && letter != "c" {
		letter = "a"
	}
	// 表情图的变体字母不一定和阶段分组一致：3SLG 的 20+ 是 c 组，
	// SLIE 的 20/21 仍然在 b 组（bu02f0001b21）。先试当前字母，再依次
	// 试 a/b/c，避免某部作品的第 20 张表情永远拼不出名字。
	variants := []string{letter}
	for _, candidate := range []string{"a", "b", "c"} {
		if candidate != letter {
			variants = append(variants, candidate)
		}
	}
	for _, variant := range variants {
		name := buexStandeeFileName(number, view, 0, 0, 1, variant, character.face)
		if s.composer.Load(name) != nil {
			return name
		}
	}
	return ""
}

// buexStandeeFileName 按约定拼出立绘/表情图名。
//
// bu<NN><u|f><脱衣阶段><服装两位><姿势一位><变体>[表情两位]
// 例：bu01u0011a = 1 号角色全身、着衣、01 号服、姿势 1、变体 a；
//
//	bu01f0001a12 = 同一角色半身的 12 号表情叠加图（strip=0、wear=00）。
func buexStandeeFileName(number int, view string, strip, wear, pose int, variant string, face int) string {
	if face > 0 {
		return fmt.Sprintf("bu%02d%s%d%02d%d%s%02d", number, view, strip, wear, pose, variant, face)
	}
	return fmt.Sprintf("bu%02d%s%d%02d%d%s", number, view, strip, wear, pose, variant)
}

// buexTryName 试探一个立绘名字是否存在。
func (s *Session) buexTryName(number int, view string, strip, wear, pose int, variant string, face int) string {
	name := buexStandeeFileName(number, view, strip, wear, pose, variant, face)
	if s.composer.Load(name) == nil {
		return ""
	}
	return name
}

// ---------------------------------------------------------------- 预设存取

func buexPresetPath(s *Session) string {
	return filepath.Join(dataSaveDir(s.dataDir), "kizuna-buex.json")
}

// buexExportPath 导出文件的位置：和预设文件放在一起。
func buexExportPath(s *Session) string {
	return filepath.Join(dataSaveDir(s.dataDir), "kizuna-buex-export.json")
}

// loadBuexPresets 读预设。
//
// 读不到、文件坏了都返回空表：鉴赏室不能因为存档文件坏了就开不起来。
func loadBuexPresets(s *Session) buexPresetFile {
	stored := buexPresetFile{
		Presets: make([]buexPreset, buexPresetCount),
		Slots:   make([]buexPreset, buexSlotCount),
	}
	data, err := os.ReadFile(buexPresetPath(s))
	if err != nil {
		return stored
	}
	var file buexPresetFile
	if err := json.Unmarshal(data, &file); err != nil {
		core.Logf("buex: ignore damaged presets: %v", err)
		return stored
	}
	for index := range stored.Presets {
		if index < len(file.Presets) {
			stored.Presets[index] = buexNormalizePreset(file.Presets[index])
		}
	}
	for index := range stored.Slots {
		if index < len(file.Slots) {
			stored.Slots[index] = buexNormalizePreset(file.Slots[index])
		}
	}
	return stored
}

// buexNormalizeView 规整距离档位。
//
// 旧预设里 view=0 表示已经不再使用的全身档（原版鉴赏室没有全身），
// 读到时按半身处理；view=2 是新增的放大半身档；其它值一律回到半身。
func buexNormalizeView(view int) int {
	if view == buexViewClose {
		return buexViewClose
	}
	return buexViewBust
}

// buexNormalizePreset 修正预设里的非法值。
//
// 旧文件里的 character 是 0 基的，读进来要当 1 基用；
// 空的 pose 一律当 "a"。
func buexNormalizePreset(preset buexPreset) buexPreset {
	if preset.Character <= 0 {
		preset.Character = 1
	}
	if preset.Pose != "a" && preset.Pose != "b" {
		preset.Pose = "a"
	}
	if preset.Wear <= 0 {
		preset.Wear = 1
	}
	preset.View = buexNormalizeView(preset.View)
	if preset.Face < 0 {
		preset.Face = 0
	}
	if preset.Under < 0 {
		preset.Under = 0
	}
	return preset
}

// saveBuexPresets 写预设（先写临时文件再改名，避免写到一半断电留下半个文件）。
func saveBuexPresets(s *Session, presets, slots []buexPreset) {
	file := buexPresetFile{Presets: presets, Slots: slots}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return
	}
	path := buexPresetPath(s)
	if !core.EnsureDir(filepath.Dir(path)) {
		core.Logf("buex: cannot create preset directory")
		return
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		core.Logf("buex: write presets failed: %v", err)
		return
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		core.Logf("buex: commit presets failed: %v", err)
	}
}

// buexExportPreset 把当前状态导出成独立的 JSON 文件。
func (s *Session) buexExportPreset(state *buexMuseumState) bool {
	preset := s.buexCurrentPreset(state)
	data, err := json.MarshalIndent(preset, "", "  ")
	if err != nil {
		core.Logf("buex: encode export failed: %v", err)
		return false
	}
	path := buexExportPath(s)
	if !core.EnsureDir(filepath.Dir(path)) {
		return false
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		core.Logf("buex: write export failed: %v", err)
		return false
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		core.Logf("buex: commit export failed: %v", err)
		return false
	}
	return true
}

// SubmitName 接收宿主文本框送回来的留言（文章编辑页）。
//
// 只有鉴赏室开着的时候才收：别的界面送来的名字和这里无关。
func (s *Session) SubmitName(name string) bool {
	ui := s.ui
	if ui == nil || ui.kind != uiMuseumBU || ui.buex == nil {
		return false
	}
	ui.buex.message = strings.TrimSpace(name)
	ui.buex.editing = false
	s.composer.Invalidate()
	return true
}

// ---------------------------------------------------------------- 滚轮

// wheelBuex 处理鉴赏室里的滚轮。
//
// 滚轮事件只有增量、没有坐标，所以只能滚"当前这一栏"：
// 背景页翻页、存档页换选中的格子，其它页不响应。
func (s *Session) wheelBuex(delta int32) {
	ui := s.ui
	if ui == nil || delta == 0 {
		return
	}
	// MD2 立绘鉴赏：滚轮滚缩放（手册最多 2 倍），先于旧 buex 分派。
	if ui.md2Bu != nil && s.md2BuWheel(delta) {
		return
	}
	if ui.buex == nil {
		return
	}
	state := ui.buex
	switch state.page {
	case buexPageBackground:
		count := s.buexPageCount(state, state.bgGroup)
		if count <= 0 {
			return
		}
		page := clampInt(state.bgPage+int(delta), 0, count-1)
		if page == state.bgPage {
			return
		}
		state.bgPage = page
	case buexPageStore:
		selected := clampInt(state.selectedPreset+int(delta), 0, max(0, len(state.presets)-1))
		if selected == state.selectedPreset {
			return
		}
		state.selectedPreset = selected
	default:
		return
	}
	s.composer.Invalidate()
}

// ---------------------------------------------------------------- 按钮绘制

// drawBuexButtonAt 按状态帧画鉴赏室的按钮。
//
// 按钮图是竖排的若干帧，MaxState 就是帧数；
// 第 0 帧通常是白色命中掩码，只用来判命中，调用方不要画它。
func (s *Session) drawBuexButtonAt(canvas *gfx.BgraCanvas, button config.ConfigButton, frame, x, y int) {
	if button.Name == "" {
		return
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return
	}
	frames := max(1, button.MaxState)
	height := image.Height / frames
	if height <= 0 {
		return
	}
	frame = clampInt(frame, 0, frames-1)
	canvas.DrawRegion(image, 0, frame*height, image.Width, height, x, y)
}

// hitBuexButton 判断点是否落在按钮上。
func hitBuexButton(s *Session, button config.ConfigButton, x, y int) bool {
	if button.Name == "" {
		return false
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return false
	}
	frames := max(1, button.MaxState)
	height := image.Height / frames
	if height <= 0 {
		return false
	}
	return x >= button.X && y >= button.Y && x < button.X+image.Width && y < button.Y+height
}
