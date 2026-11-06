package session

import (
	"sort"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/runtime/bsmap"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// systemui.go 是引擎自绘系统界面的状态机。
//
// 设计边界：
//   - 引擎负责画面、布局、命中判定、存档/读档/鉴赏数据；
//   - 宿主只负责把 Compose 的帧画出来，并把指针事件原样转发回来。
//
// 之所以不把这些界面做成 Flutter 路由：底图、按钮、槽位坐标都来自游戏自己的
// bsxx.cfg / BSA 包，宿主重画一份既会重复实现，也会和原版视觉/命中错位。
// 这条边界也是"Go 管核心逻辑、Flutter 只管显示"的具体落点。

type screenKind int

const (
	uiNone screenKind = iota
	// uiDataSave / uiDataLoad 是同一个存档画面的两种模式，共用 [data screen] 布局。
	uiDataSave
	uiDataLoad
	// uiDataItem 是 SYSTEM 菜单的「持ち物」页（MK 的 ITEM 页签）。
	uiDataItem
	uiMuseum
	// uiMuseumStrip 是旧式立绘鉴赏页（AFDVD 的 museum_chs 整身立绘）。
	uiMuseumStrip
	uiMuseumCG
	uiMuseumScene
	// uiScenePlaylist 是场景鉴赏里的"播放列表"子画面。
	uiScenePlaylist
	uiMuseumMusic
	uiCGViewer
	// uiConfig 是原版扩展设置画面（system_config_ex.layout）。
	uiConfig
	// uiMuseumBGV / uiDress 是 BGV 鉴赏室与立ち絵/脱衣设定画面。
	uiMuseumBGV
	uiMuseumBU
	uiDress
	// uiLog 是引擎自绘的回想/バックログ界面。
	uiLog
	// uiStatus 是"状態"画面（[status screen] + define.bsa:status_layout）。
	uiStatus
	// uiLegacyConfig 是 bsxx.cfg 旧式 [config screen] 的设置画面。
	// 与 uiConfig（define.bsa:system_config_ex.layout）并存：后者是新版资源
	// 驱动布局，前者是 cfg_* 素材 + 坐标的老格式，两套作品都在发行。
	uiLegacyConfig
	// uiStatusSLG 是脚本驱动（[status screen] script name = status_screen）
	// 的状态画面，页面和数据来自 var_db 表而不是布局资源。
	uiStatusSLG
	// uiStatusAncient 是特別病棟（Tbxdl 世代）的内置状態画面：底图与面板都由引擎写死，
	// 没有 status_layout 资源，见 systemui_status_ancient.go。
	uiStatusAncient
	// uiStatusLegacy 是旧式 [status screen] 的 ITEM 页（教育指導 KS/KSIE），
	// 道具图标位置来自 item111…item533，见 systemui_status_legacy.go。
	uiStatusLegacy
	// uiKarte 是 [status2 screen] 的「ケダモノカルテ」（OH/OHIE）。
	// 它与 uiStatus 是两种不同画面：status 按钮在多数作品里打开
	// status_layout，在这两代作品里打开的是角色病历选择/详情页。
	uiKarte
	// uiTarget 是学園2 周结的目标选择画面（system call "target"）。
	uiTarget
	// uiTargetCard 是教育指導（KS）的放学目标卡选择：剧本只给 #result
	// 位掩码，卡片由 [target select] 布局绘制，点卡即写回 #result。
	uiTargetCard
)

type uiHitKind int

const (
	hitNone uiHitKind = iota
	hitSlot
	hitPage
	// hitPagePrev/hitPageNext 是"Page No."上下箭头的翻页热区（GK）。
	hitPagePrev
	hitPageNext
	// hitShortcutAuto/hitShortcutQuick 是存档页右下的 Auto/Quick 快捷读档热区。
	hitShortcutAuto
	hitShortcutQuick
	hitSaveMode
	hitLoadMode
	hitDataConfig
	// hitDataItem 是存档页/系统菜单的 ITEM 页签。
	hitDataItem
	// hitItemSlot 是持ち物页的一个道具格。
	hitItemSlot
	hitBack
	hitTitle
	hitExit
	hitConfirmOK
	hitConfirmCancel
	hitMuseumMode
	hitMuseumReturn
	hitCGPage
	hitCGThumb
	hitCGPrev
	hitCGNext
	hitCGExit
	hitScenePage
	hitSceneThumb
	// 场景页顶部的"立即播放/加入列表"模式和播放列表子画面。
	hitScenePlayNow
	hitSceneAddPlaylist
	hitSceneOpenPlaylist
	hitPlaylistPage
	hitPlaylistSlot
	hitPlaylistPlay
	hitPlaylistClose
	hitMusicTrack
	hitMusicPlay
	hitMusicStop
	hitMusicPrev
	hitMusicNext
	// BGM 鉴赏左侧控制盒的音量轨道与两端按钮。
	hitMusicVolume
	hitMusicMin
	hitMusicMax
	hitCGConfig
	hitConfigCategory
	hitConfigCommon
	hitConfigControl
	hitBGVClose
	hitBGVTrack
	hitBGVStop
	hitBGVVolume
	hitBUClose
	hitBUMenu
	hitBUCharacter
	// hitBUTarget 是旧式 [museum phase - bu] 角色选择页上的目标按钮。
	hitBUTarget
	hitTargetCard
	hitBUView
	hitBUThumb
	hitBUPreset
	hitBUAction
	hitLogVoice
	// 旧式设置画面：分类页签、通用按钮/滑杆/开关、返回/标题/退出。
	hitLegacyConfigCategory
	hitLegacyConfigControl
	// hitLegacyOption 是 Tbxdl 设置画面的"值图"按钮（OFF/ON、通常/高速/…）。
	hitLegacyOption
	hitLegacyConfigReset
	hitLegacyConfigBack
	hitLegacyConfigTitle
	hitLegacyConfigExit
	// hitTargetChoice 是学園2 目标选择卡片的命中。
	hitTargetChoice
	// 牝教師2 老版立ち絵鉴赏：角色卡、戻る、左右差分、背景切换。
	hitMK2Char
	hitMK2Back
	hitMK2Prev
	hitMK2Next
	hitMK2BG
	// 牝教師2 老版ぬぎぬぎ設定：阶段按钮与服装左右切换。
	hitMK2DressStage
	hitMK2DressPrev
	hitMK2DressNext
	// 脚本驱动状态画面：顶部模式按钮与滚动条。
	hitStatusMode
	hitStatusScroll
	// Karte（[status2 screen]）：角色卡、返回、两段文本的语音按钮。
	hitKarteTarget
	hitKarteBack
	hitKarteVoice
	hitDressBack
	hitDressTarget
	hitDressCostume
	hitDressState
	hitDressMode
	hitDressPen
	// 立ち絵鑑賞室（museum_buex_layout）setting_bu 页的六个开关组、
	// 角色排序/显示开关、距離滑块、bg_select 页签，以及
	// edit_message / data_store 两个工具页的控件。
	hitBUOrderUp
	hitBUOrderDown
	hitBUAppearance
	hitBUPose
	hitBUFace
	hitBUWear
	hitBUUnder
	hitBUNgng
	hitBUHair
	hitBUCutin
	hitBUSteam
	hitBUAhe
	// hitBUBackground 是旧式 [bu viewer] 的"背景"开关组（setting6）。
	hitBUBackground
	hitBUPointer
	// hitBUFocus / hitBUFocusBar 是旧式立ち絵查看器的自由視点バー：
	// 顔/胸/股間三个按钮，以及可点击的纵向轨道。
	hitBUFocus
	hitBUFocusBar
	hitBUPage
	hitBUEditSub
	hitBUEditCommand
	hitBUColorTarget
	hitBUColorClose
	hitBUColorBar
	hitBUColorMin
	hitBUColorMax
	hitBUStoreCommand
	hitBUSlot
	// 脱衣設定（[dress screen]）的脱衣段阶预览行、右側视点控件与量尺。
	hitDressLevel
	hitDressView
	hitDressViewBar
	// CG 查看器底栏的演出开关与 BGV 音量/详情。
	hitCGCutin
	hitCGHair
	hitCGSteam
	hitCGAhe
	hitCGBGVVolume
	hitCGBGVDetail
	// 存档画面的三个排序按钮。
	hitDataSort
	// 回想的滚动条轨道、上下翻页箭头，以及 BGV 鉴赏室各栏的滚动条轨道。
	hitLogBar
	hitLogUp
	hitLogDown
	hitBGVBar
	// 状態画面的固定按钮（inventory_back 等）。
	hitStatusButton
	// CHART 流程图：页码按钮、格子（场景）、"显示当前场景"和确认框。
	// 确认框是独立浮层，和存档画面的确认框分开，避免共用一个布尔值时
	// 两套界面的 OK/Cancel 命中互相串线。
	hitChartPage
	hitChartBlock
	hitChartNowScene
	hitChartConfirmOK
	hitChartConfirmCancel
	// すくーるヘブン 初回限定版的立ち絵页：角色卡与差分翻页箭头。
	hitSHStripCard
	hitSHStripNav
)

type uiHit struct {
	kind  uiHitKind
	index int
}

type systemUI struct {
	kind screenKind

	// page 是存档画面的当前页；cgGroup/scenePage 复用同一个字段更易错，
	// 所以鉴赏室各子页面各自保存自己的页码。
	page int
	// sortMode 是存档画面的排序方式：0 按槽位号、1 新到旧、2 旧到新。
	// slotOrder 是排序后的全局槽位顺序；为 nil 时按槽位号。
	sortMode      int
	slotOrder     []int
	slotOrderMode int
	// hover 是当前鼠标命中的控件；只用于画悬停帧。
	hover uiHit
	// pressHit/pressHandled 用来吃掉同一次点按的 release。
	// 宿主把一次点击拆成 press(1) + release(3)，两个事件都会走到
	// activateSystemUI；不挡 release 的话，按钮会被按两次、开关会翻回原值，
	// 存档槽会被保存/删除两次。press 已经处理过同一个命中时，release 只清状态。
	pressHit     uiHit
	pressHandled bool
	// confirm 表示存档覆盖确认窗口；confirmSlot 是等待确认的槽位。
	confirm     bool
	confirmSlot int
	// backdrop 是打开界面前抓下的游戏画面，用来生成存档缩略图：
	// 保存时画面已经被界面盖住，直接抓 composer 帧会把存档画面存进缩略图。
	backdrop *gfx.BgraCanvas
	// slotPage/slotThumbs 是当前页的存档缩略图缓存，换页或写档后重建。
	slotPage   int
	slotThumbs map[int]*gfx.BgraCanvas
	// slotSaved 是当前页槽位的存档元数据，只用于画保存时间，与 slotThumbs 同步刷新。
	slotSaved map[int]*save.SaveData
	// deleteMode 为真时点击槽位删除存档；DELETE 工具条按钮进入，切换存/读模式或关闭界面时清除。
	deleteMode bool
	// savedText/savedSpeaker/savedWindow 用于关闭界面后恢复对话框。
	savedText    string
	savedSpeaker string
	savedWindow  bool
	savedMulti   bool
	// fromTitle 表示界面是从主界面打开的：关闭后回标题，而不是回剧情。
	fromTitle bool
	// savedStep 是打开界面前引擎的 step。系统界面期间对外报告 AwaitInput，
	// 否则宿主会按 AwaitSelect/AwaitChoice 画地图/选择肢覆盖在存档画面上。
	savedStep core.Step

	// 鉴赏室状态。
	cgGroup   int
	cgEntry   int
	cgVariant int
	cgCanvas  *gfx.BgraCanvas
	// gk2Page 是学園2 老版鉴赏页的组页码与掩码缓存，只在 Museum.Gakuen2 时使用。
	gk2Page gk2MuseumPage
	// targetOptions/targetHover 是学園2 周结目标选择的候选卡片与悬停下标。
	targetOptions []targetChoice
	targetHover   int
	// targetCards 是教育指導（KS）放学目标选择：卡片、坐标与悬停下标。
	targetCards *targetCardState
	scenePage   int
	// sceneMode 是场景页顶部的选择模式：0 = 点击立即播放，1 = 加入播放列表。
	sceneMode int
	// playlistPage / playlistSelected 是播放列表子画面的页码与选中槽位。
	playlistPage     int
	playlistSelected int
	musicTrack       int
	musicPlaying     bool
	// musicVolumeDrag 表示当前拖动是在 BGM 鉴赏的音量轨道上。
	// 只有按过轨道才置 true，避免在曲目列表上滑动时误调音量。
	musicVolumeDrag bool
	// cgPanelShown 是 KD CG 查看器右下控制盒的显示状态；右键切换。
	cgPanelShown bool
	// cgZoom 是 CG 查看器的放大倍率（1..2）；cgPanX/cgPanY 是放大后
	// 以画面中心为基准的偏移（单位：屏幕像素）。右键拖拽只在 zoom>1 时平移。
	cgZoom float64
	// cgPanX/cgPanY 在 zoom=1 时恒为 0，所以渲染仍走整屏路径。
	cgPanX int
	cgPanY int
	// cgDrag 是右键拖拽平移的状态：右键按下后先记录起点，移动超过
	// 阈值才算真的拖动；没有移动的松开按原逻辑 toggle 控制盒。
	cgDrag      bool
	cgDragMoved bool
	// cgDragX/cgDragY 是右键拖拽的上一次指针位置（起点/中间帧）。
	cgDragX int
	cgDragY int

	// ancientStatus 记录特別病棟内置状態画面四个面板当前显示的评论轮换步数。
	ancientStatus [ancientStatusPanels]int
	// ancientStatusSelected 是当前展开评论的面板下标；-1 表示只显示整屏资料。
	ancientStatusSelected int

	// karteSelected 是 [status2 screen] 当前查看的角色下标；-1 表示停在
	// 四张角色卡的选择页。详情页点返回先回到选择页，再点一次才关闭。
	karteSelected int

	// CG 查看器底栏的三个演出开关与 BGV 音量条的展开状态。
	//
	// 这三个开关和剧情共用同一份设置（s.settings.Cutin / UnderHair / Steam），
	// 这里保存的是打开查看器那一刻的镜像，方便绘制时不必回读设置。
	cgCutin         bool
	cgHair          bool
	cgSteam         bool
	cgBGVolumeShown bool
	// cgPointerX/cgPointerY 是最近一次命中判定用到的坐标。
	// BGV 音量条按点击位置取比例，激活时需要知道点在了条的哪一段。
	cgPointerX int
	cgPointerY int

	// configPage 是当前分类下标；configControls 是该分类的功能控件。
	// 控件在切换分类时重建，绘制与命中用同一份数据，避免两边几何漂移。
	configPage     int
	configControls []configControl

	// BGV 鉴赏室 / 立ち絵（脱衣）设定。
	bgv  *bgvMuseumState
	buex *buexMuseumState
	// kdStrip 是 KD 专用的立ち絵页：没有 buex/legacy 布局，直接用 sd_* 整身图。
	kdStrip      bool
	kdStripIndex int
	// shStrip 是すくーるヘブン 初回限定版（BSScript Rev.3）的立ち絵页：
	// museum_ch1..8 角色卡选择 + cl{c}{w}{p}{d} 立绘查看。没有 buex/legacy
	// 布局可复用，状态单独存放。
	shStrip *shStripState
	// mk2Strip 是牝教師2 老版的立ち絵页：museum_chs 角色卡 + bu*/gb* 组合。
	mk2Strip *mk2StripState
	// mkStrip 是牝教師初代的立ち絵页：museum_ch1..6 + bum_base 控制面板。
	mkStrip *mkStripState
	// itemSlot 是持ち物页当前选中的道具格，-1 表示没有选中。
	itemSlot int
	// itemFromData/itemFromSave 记录 ITEM 页是从存档页（SAVE 还是 LOAD）
	// 切进来的：BACK 应该回存档页，而不是直接回剧情。
	itemFromData bool
	itemFromSave bool
	// mk2Dress 是牝教師2 老版的ぬぎぬぎ設定：角色卡 + 阶段/服装切换。
	mk2Dress *mk2DressState
	// md2Bu 是満淫電車2 的精灵图立ち絵查看器；nil 表示停在角色选择页。
	md2Bu *md2BuState
	dress *dressState
	log   *logState
	// chart 是"状態"画面里的 CHART 流程图（status_layout 的 chart 页）。
	// 它和 buex 一样是可交互子画面，所以状态放在 ui 上，关界面即释放。
	chart *chartUIState
	// legacy / slg 是两套新增界面的运行状态，分别由
	// systemui_config_legacy.go 与 systemui_status_slg.go 定义。
	legacy *legacyConfigState
	slg    *slgStatusState

	// 触屏拖动（Pointer op 6）的状态。lastDragY = -1 表示还没有上一次位置：
	// 第一帧拖动只记录起点，否则手势开始的瞬间会跳一大段。
	// chart 是二维平移，需要独立的 lastDragX；列表类界面只用 Y。
	// remainder 保存不足一行的位移，慢速拖动时不会因为每次取整而完全不动。
	lastDragX        int
	lastDragY        int
	logDragRemainder int
	bgvDragRemainder int
	bgvDragColumn    int
	// lastPointerOp 记录上一次 pointer 子操作：新手势的第一帧拖动只对起点，
	// 不把"上一手势结束位置 → 这一手势开始位置"算成一次滚动。
	lastPointerOp int
	// hoverX/hoverY 是最近一次悬停/点击的坐标。BGV 的 min/max 共用
	// hitBGVVolume 一种命中类型，画悬停帧时要靠坐标分辨具体是哪一个按钮。
	hoverX int
	hoverY int
}

// cgZoomMax 是 CG 查看器滚轮放大的上限（MD2 手册：最多放大到 2 倍）。
const cgZoomMax = 2.0

// cgZoomStep 是滚轮每格的放大步进（任务指定 0.25）。
const cgZoomStep = 0.25

// cgDragThreshold 是右键拖拽判定为"移动"的像素阈值：小于它视为点击
// （toggle 控制盒），大于等于它才算平移，避免按下时的抖动误触。
const cgDragThreshold = 4

// openDataScreen 打开存档/读档画面。
//
// 只有停在"等玩家点击"（或主界面）时才允许打开：这两个状态脚本不会在后台推进，
// 关闭界面后恢复原来的文本/对话框即可。地图/选择画面有自己的暂停语义，
// 贸然叠加会让宿主同时收到 mapButtons 和系统界面两套热区。
func (s *Session) openDataScreen(saveMode bool) bool {
	layout := s.config.DataScreen
	if layout.Empty() || s.gameState == nil || s.composer == nil {
		return false
	}
	if !s.canOpenSystemUI() {
		return false
	}
	kind := uiDataLoad
	if saveMode {
		kind = uiDataSave
	}
	s.beginSystemUI(kind)
	return true
}

// openDataScreenDelete 打开存档画面并进入删除模式。
//
// 结构上复用读档画面（6 槽/页的同一个界面），点击槽位时只删除、不读档。
// 返回 false 表示作品没有存档画面，调用方应退回宿主动作/日志。
func (s *Session) openDataScreenDelete() bool {
	if !s.openDataScreen(false) {
		return false
	}
	if s.ui != nil {
		s.ui.deleteMode = true
	}
	return true
}

func (s *Session) openMuseum() bool {
	layout := s.config.Museum
	if layout.Empty() || s.gameState == nil || s.composer == nil {
		return false
	}
	if !s.canOpenSystemUI() {
		return false
	}
	s.beginSystemUI(uiMuseum)
	if layout.DefaultMode != "" {
		for index, mode := range layout.Modes {
			if mode.Key == layout.DefaultMode {
				s.openMuseumMode(index)
				break
			}
		}
	}
	return true
}

// handleBsScreenRequest 处理脚本主动请求的系统界面。
//
// 旧式 BSScript 的 `system call "sceneroom"` 是"打开シーン鑑賞"，
// 不是标记：原引擎在章节结束/标题鉴赏入口会用它。只记日志会让玩家
// 点完场景回想后什么都没发生。
func (s *Session) handleBsScreenRequest(keyword string) state.SessionStep {
	switch keyword {
	case "higoroom":
		if s.config != nil && s.config.Mddvd {
			if s.openMddvdHigo() {
				return state.SessionAwaitInput
			}
			core.Logf("script: higoroom has no gallery assets, continuing")
		}
		return state.SessionContinue
	case "sceneroom":
		// MDDVD 的剧本在每条路线结尾用 `system call "sceneroom"` 标记
		// "进入场景回放登记"，随后立刻用 Dxxx_xx 继续场景；它不是
		// 阻塞式鉴赏界面。若在这里打开博物馆，玩家一关界面就会被
		// enterTitle 拉回标题，整条路线中断。场景鉴赏仍可从标题进入。
		if s.config != nil && s.config.Mddvd {
			// 原引擎的 sceneroom 是 state 0xa 的场景鉴赏界面；关闭后
			// 回到脚本下一条（Dxxx_xx 场景/路线尾），不能回标题。
			if s.openMuseumFromScript() {
				for index, mode := range s.config.Museum.Modes {
					if mode.Key == "scene" {
						s.openMuseumMode(index)
						break
					}
				}
				s.mddvdMuseumFromScript = true
				return state.SessionAwaitInput
			}
			core.Logf("script: sceneroom has no scene gallery, continuing")
			return state.SessionContinue
		}
		if !s.openMuseumFromScript() {
			core.Logf("script: sceneroom requested but the museum layout is empty")
			return state.SessionContinue
		}
		for index, mode := range s.config.Museum.Modes {
			if mode.Key == "scene" {
				s.openMuseumMode(index)
				core.Logf("script: sceneroom -> museum scene page")
				return state.SessionContinue
			}
		}
		core.Logf("script: sceneroom requested but the scene mode is missing")
	}
	return state.SessionContinue
}

// openMuseumFromScript 绕过 canOpenSystemUI 的限制：
// 脚本系统调用是原版明确要求打开界面，此时 lastStep 可能还是 Wait/Continue。
func (s *Session) openMuseumFromScript() bool {
	if s.config == nil || s.config.Museum.Empty() || s.gameState == nil || s.composer == nil {
		return false
	}
	if s.ui != nil {
		s.closeSystemUI()
	}
	s.beginSystemUI(uiMuseum)
	return true
}

func (s *Session) canOpenSystemUI() bool {
	// 言葉責め界面进行中不允许叠存档/设置：两边状态会互相穿插。
	if s.mddvd != nil {
		return false
	}
	if s.slgState != nil {
		// Lua 训练/领地界面是模态的：叠上存档/设置会让两边状态互相穿插，
		// 读档后也无法恢复到界面中间态。
		return false
	}
	if s.titleMode {
		return true
	}
	// 地图选择态也允许打开系统界面：原版的 map.dat 地图底部常驻
	// CONFIG/LOG/STATUS/SAVE/LOAD 按钮，点击后画面保持在地图上。
	// 存档时 Session.Save 会把 PC 倒回 SELECT_TAIL 指令头，读档后会重新
	// 打开同一张地图，所以不会产生"跳过这一格"的坏档。
	if s.mapSource == bsmap.BsMapDataFileName && s.gameState != nil && s.gameState.Selecting() {
		return true
	}
	// 学園（GK）的地图顶部常驻 Q.SAVE/Q.LOAD/SAVE/LOAD/CONFIG/LOG 六颗按钮，
	// 原版在地图上就能打开这些界面；关掉后仍停在同一张地图。
	if s.mapSource == bsmap.BsMapGkSource && s.gameState != nil && s.gameState.Selecting() {
		return true
	}
	// 脚本自绘地图同样有常驻工具条（牝教師4 的 FACE…HOLD 一排）。
	// 存档/读档会把 PC 倒回铺地图的指令，读档后重铺地图；CONFIG/LOG/
	// STRIP 只是盖一层界面，关掉后仍停在同一张地图上。
	if s.mapSource == "custom" && s.gameState != nil && s.gameState.Selecting() {
		return true
	}
	// 満淫電車2 的精灵图地图、以及任意停在选择肢的画面：存档扩展段已经
	// 保存 ChoiceOptions/MapSource，读档能恢复同一组选项/同一张地图，
	// 因此可以安全地打开 SAVE/LOAD/CONFIG/LOG。
	if s.mapSource == "md2" && s.gameState != nil && s.gameState.Selecting() {
		return true
	}
	if s.config != nil && !s.config.Ecstasy.Empty() && s.gameState != nil && s.gameState.AwaitingChoice() {
		// 只对満淫電車2 放开：这一代的 ChoiceOptions 会随存档写出，
		// 读档能恢复同一组选项；其它作品的选项状态还没进 SaveData。
		return true
	}
	// 只允许停在"稳定对白"上打开存读档/鉴赏。
	//
	// 地图/选择肢虽然也不在引擎内部跑，但它们的状态（mapItems、mapMask、
	// selectOptions、deferred jump）不在 SaveData 里：从这里存档，读回来会
	// 直接跳过这次选择，甚至停在一张没有热区的背景上。与其给玩家一个坏档，
	// 不如按原引擎的"对话中才能存档"约束住；要支持地图存档，得先把
	// mapflow 的状态一并写进 SaveData。
	//
	// 影片/等待/结束阶段同样不允许，避免状态交叉。
	return s.lastStep == core.StepAwaitInput
}

func (s *Session) beginSystemUI(kind screenKind) {
	if s.ui != nil {
		if s.ui.kind == kind {
			return
		}
		s.closeSystemUI()
	}
	ui := &systemUI{
		kind:          kind,
		hover:         uiHit{kind: hitNone, index: -1},
		slotPage:      -1,
		fromTitle:     s.titleMode,
		savedText:     s.gameState.Text(),
		savedSpeaker:  s.gameState.Speaker(),
		savedWindow:   s.gameState.MessageWindowVisible(),
		savedMulti:    s.gameState.MultiMessageVisible(),
		savedStep:     s.lastStep,
		lastDragX:     -1,
		lastDragY:     -1,
		lastPointerOp: -1,
		karteSelected: -1,
	}
	if s.titleMode {
		ui.savedStep = core.StepAwaitTitle
	} else {
		// 界面期间报告"等点击"：宿主据此收起地图/选择肢覆盖层，
		// 并把点击继续转发给引擎。
		s.lastStep = core.StepAwaitInput
	}
	// 抓一份底层画面：转场/立绘都已经画好，缩略图就是玩家看到的这一屏。
	if frame := s.composer.Frame(); frame != nil && frame.Width > 0 && frame.Height > 0 {
		ui.backdrop = gfx.NewBgraCanvas(frame.Width, frame.Height)
		ui.backdrop.CopyFrom(frame)
	}
	s.ui = ui
	// 系统界面盖住整屏，对话框必须收起来：Flutter 的 MessageBox 是独立图层，
	// 不关掉的话会浮在存档/鉴赏画面上面，还会拦住点击。
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.gameState.SetMessageWindowVisible(false)
	s.gameState.ClearChoices()
	s.menuVisible = false
	s.messageHover = -1
	s.composer.Invalidate()
	// ITEM（持ち物）页会把当前场景垫在面板下面；打开界面前抓的那一帧
	// 还带着对话框与工具条，直接铺上去会让工具条浮在面板上方。这里在
	// 隐藏对话框之后重新合成一次，只留场景层。
	if kind == uiDataItem && s.composer != nil && s.gameState != nil {
		s.composer.SetMessageWindowState(false, -1, false, "")
		s.composer.Compose(s.gameState)
		if frame := s.composer.Frame(); frame != nil && frame.Width > 0 && frame.Height > 0 {
			ui.backdrop = gfx.NewBgraCanvas(frame.Width, frame.Height)
			ui.backdrop.CopyFrom(frame)
		}
	}
	core.Logf("ui: open screen %d (title=%v)", int(kind), s.titleMode)
}

// closeSystemUI 关闭系统界面并恢复剧情文本。
//
// 从主界面进来的只回标题：标题画面没有"上一句对白"，恢复文本会把剧情第一句
// 泄漏到标题上。
func (s *Session) closeSystemUI() {
	ui := s.ui
	if ui == nil {
		return
	}
	// BGV 鉴赏室的声音不属于剧情语音，关界面时不能留着继续响；否则回到
	// 剧情后玩家会听到"没有停掉的喘息声"，而且再进 BGV 时 stop 按钮
	// 会因为新状态里 bgvPlaying=false 而变成空操作。
	if ui.kind == uiMuseumBGV && ui.bgv != nil && ui.bgv.bgvPlaying {
		s.stopBGVTrack(0)
	}
	s.ui = nil
	s.mddvdMuseumFromScript = false
	if !ui.fromTitle {
		s.lastStep = ui.savedStep
	}
	if !ui.fromTitle && s.gameState != nil {
		s.gameState.SetText(ui.savedText)
		s.gameState.SetSpeaker(ui.savedSpeaker)
		s.gameState.SetMessageWindowVisible(ui.savedWindow)
		s.gameState.SetWindowVisible(2, ui.savedMulti)
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("ui: close screen %d", int(ui.kind))
}

// pointerSystemUI 处理系统界面上的鼠标/触摸事件。
func (s *Session) pointerSystemUI(op uint32, x, y int32) (core.Step, error) {
	ui := s.ui
	if ui == nil {
		return s.lastStep, nil
	}
	prevOp := ui.lastPointerOp
	ui.lastPointerOp = int(op)
	// MD2 CG 右键拖拽平移：宿主只发 op 4（rightClick）表示右键
	// 松开/点击，没有"右键按下/移动"的 op。原作手册要求"按住右键拖拽
	// 平移、右键点击切换控制盒"，宿主侧无对应事件，引擎在 op 4 到达
	// 时以"本次右键是否移动过"无法得知为由，不能伪造 drag。
	// 因此这里的 cgDrag/cgDragMoved 只能覆盖将来宿主增加 op 时的
	// 内部路径；当前行为完全保持原有 toggle 逻辑（见 case 4）。
	_ = prevOp
	// 新手势的第一帧拖动只用来对齐起点：上一手势的结束位置和这一手势的
	// 开始位置之间没有实际滑动，直接算差值会跳一大段。
	if op == 6 && prevOp != 6 {
		ui.lastDragX = int(x)
		ui.lastDragY = int(y)
		ui.bgvDragColumn = -1
		// 触屏拖动的第一帧只对齐起点，但可以据此判断手势是不是从 BGM
		// 音量轨道开始的；从列表开始的手势绝不能顺手把音量拖掉。
		ui.musicVolumeDrag = ui.kind == uiMuseumMusic && s.hitMuseumMusic(int(x), int(y)).kind == hitMusicVolume
		return s.lastStep, nil
	}
	switch op {
	case 0:
		ui.hoverX = int(x)
		ui.hoverY = int(y)
		hit := s.hitSystemUI(int(x), int(y))
		// 左键按住拖动时的 hover 也要刷透視格（原版边拖边透），普通悬停不刷。
		if ui.pressHandled || ui.pressHit.kind == hitDressPen {
			s.md2DressBrushAt(int(x), int(y))
		}
		if hit != ui.hover {
			ui.hover = hit
			s.composer.Invalidate()
		}
		ui.lastDragY = int(y)
	case 1:
		ui.hoverX = int(x)
		ui.hoverY = int(y)
		hit := s.hitSystemUI(int(x), int(y))
		ui.hover = hit
		ui.pressHit = hit
		ui.pressHandled = hit.kind != hitNone
		s.activateSystemUI(hit, int(x), int(y))
		s.md2DressBrushAt(int(x), int(y))
		s.composer.Invalidate()
	case 3:
		ui.hoverX = int(x)
		ui.hoverY = int(y)
		hit := s.hitSystemUI(int(x), int(y))
		ui.hover = hit
		if ui.pressHandled && hit == ui.pressHit {
			// press 已经执行过这次动作，release 只是抬手指。
			ui.pressHandled = false
			s.md2DressBrushAt(int(x), int(y))
			return s.lastStep, nil
		}
		ui.pressHandled = false
		s.activateSystemUI(hit, int(x), int(y))
		s.md2DressBrushAt(int(x), int(y))
		s.composer.Invalidate()
	case 6:
		// 触屏拖动只滚动回想/BGV 列表，不能落到推进对白的分支。
		if s.dragSystemUI(int(x), int(y)) {
			s.composer.Invalidate()
		}
	case 4:
		// 右键：确认窗口先取消；CG 查看器只收起/展开控制盒；否则直接关闭界面。
		if ui.confirm {
			ui.confirm = false
			s.composer.Invalidate()
			return s.lastStep, nil
		}
		if ui.kind == uiCGViewer && s.config != nil && s.config.Museum.Viewer.TogglePanel {
			ui.cgPanelShown = !ui.cgPanelShown
			s.composer.Invalidate()
			return s.lastStep, nil
		}
		// MD2 立绘鉴赏：右键点击隐藏/显示底部按钮（手册），在关闭/回上层之前拦截，
		// 否则右键直接把画面关掉，面板按钮就再也点不到。
		if ui.kind == uiMuseumBU && ui.md2Bu != nil && s.md2BuTogglePanel() {
			return s.lastStep, nil
		}
		// 牝教師2 / 牝教師初代的立ち絵查看页：右键先回角色选择页，不回标题。
		if s.backMKStripViewer() {
			return s.lastStep, nil
		}
		if s.backMK2StripViewer() {
			return s.lastStep, nil
		}
		if s.backMK2DressViewer() {
			return s.lastStep, nil
		}
		// Karte 的详情页右键先回选择页，避免玩家一按返回就直接退出。
		if ui.kind == uiKarte && ui.karteSelected >= 0 {
			ui.karteSelected = -1
			s.composer.Invalidate()
			return s.lastStep, nil
		}
		s.closeSystemUI()
		s.resumeCGViewerIfNeeded()
	}
	return s.lastStep, nil
}

// dragSystemUI 处理触屏拖动（Pointer op 6）。
//
// 协议约定拖动只用于滚动列表和拖动滑杆：回想和 BGV 用"行"做步长，余量不足
// 一行时留到下一次，这样慢速拖动不会因为每次取整而纹丝不动。手指向上（y 变小）
// 应露出更晚的条目，所以是 scroll -= steps。返回是否真的改变了画面。
func (s *Session) dragSystemUI(x, y int) bool {
	ui := s.ui
	if ui == nil {
		return false
	}
	lastY := ui.lastDragY
	changed := false
	switch ui.kind {
	case uiCGViewer:
		// CG 查看器没有配置底栏按钮时，用横向滑动切换上一张/下一张：
		// 右滑回上一变体，左滑到下一变体；每 60px 触发一次，避免一格跳多张。
		// 放大后（cgZoom>1）触屏拖动同样平移视图，不切换差分：原作
		// 的"拖动=平移"语义优先于横滑切图（MD2 CG 章）。
		if ui.cgZoom > 1 {
			if ui.lastDragX < 0 {
				ui.lastDragX = x
				break
			}
			ui.cgPanX = clampInt(ui.cgPanX+(x-ui.lastDragX), -s.cgPanLimitX(), s.cgPanLimitX())
			ui.cgPanY = clampInt(ui.cgPanY+(y-ui.lastDragY), -s.cgPanLimitY(), s.cgPanLimitY())
			ui.lastDragX = x
			ui.lastDragY = y
			changed = true
			break
		}
		if ui.lastDragX >= 0 {
			dx := x - ui.lastDragX
			switch {
			case dx >= 60:
				s.stepCGViewer(-1)
				ui.lastDragX = x
				changed = true
			case dx <= -60:
				s.stepCGViewer(1)
				ui.lastDragX = x
				changed = true
			}
		} else {
			ui.lastDragX = x
		}
	case uiLegacyConfig:
		if s.dragLegacyConfig(x, y) {
			changed = true
		}
	case uiMuseumMusic:
		// 只有从音量轨道开始的手势才调音量；在曲名列表上滑动不能误触。
		if ui.musicVolumeDrag {
			s.setMuseumBgmVolumeAt(s.config.Museum.Music, x, y)
			changed = true
		}
	case uiMuseumBU:
		// MD2 立绘鉴赏：放大后触屏拖动平移视图（手册），走 md2BuState 的 pan。
		if ui.md2Bu != nil {
			// op 6 的拖动在本文件末尾已更新 lastDrag，这里直接按位移平移。
			if ui.md2Bu.zoom > 1 {
				if ui.lastDragX < 0 {
					ui.lastDragX = x
					ui.lastDragY = y
					break
				}
				limitX := int(float64(s.width) * (ui.md2Bu.zoom - 1) / 2)
				limitY := int(float64(s.height) * (ui.md2Bu.zoom - 1) / 2)
				ui.md2Bu.panX = clampInt(ui.md2Bu.panX+(x-ui.lastDragX), -limitX, limitX)
				ui.md2Bu.panY = clampInt(ui.md2Bu.panY+(y-ui.lastDragY), -limitY, limitY)
				ui.lastDragX = x
				ui.lastDragY = y
				changed = true
			}
			break
		}
		// 旧式立ち絵查看器（牝教師3）的两条滑杆在触屏上要能拖：
		// 纵向自由視点バー只改近距離视口，横向立ち位置バー只移动选中角色。
		state := ui.buex
		if state == nil || !state.multi || s.config == nil {
			break
		}
		view := s.config.FreeSight
		if view.Mode && view.Meter[2] > 0 && view.Meter[3] > 0 &&
			x >= view.Meter[0] && x < view.Meter[0]+view.Meter[2] &&
			y >= view.Meter[1] && y < view.Meter[1]+view.Meter[3] {
			s.buexFocusAt(state, y)
			changed = true
			break
		}
		region := state.layout.Setting.PosRegion
		left, top, width := buexPosRegion(region)
		height := region[3]
		if height <= 0 {
			height = 25
		}
		if x >= left && x < left+width && y >= top && y < top+height {
			s.buexSetPosition(state, x)
			changed = true
		}
	case uiStatusSLG:
		if s.dragSLGStatus(y) {
			changed = true
		}
	case uiStatus:
		// CHART 是二维地图，可以左右/上下拖；普通"状態"页没有拖动。
		if ui.chart != nil && s.dragChart(x, y) {
			changed = true
		}
	case uiLog:
		if s.config == nil || ui.log == nil {
			break
		}
		layout := s.config.Log
		// 按在滚动条轨道上时直接定位，不再按行累积。
		if !layout.Bar.Empty() {
			if image := s.composer.Load(layout.Bar.Image); image != nil {
				if barX, barY, trackH, _, _, ok := s.logBarGeometry(); ok &&
					x >= barX && y >= barY && x < barX+image.Width && y < barY+trackH {
					before := ui.log.scroll
					s.scrollLogTo(y)
					changed = ui.log.scroll != before
					break
				}
			}
		}
		if lastY >= 0 {
			rowHeight := max(1, s.height/max(1, layout.Lines))
			total := ui.logDragRemainder + (y - lastY)
			steps := total / rowHeight
			ui.logDragRemainder = total - steps*rowHeight
			maxScroll := max(0, len(ui.log.entries)-layout.Lines)
			scroll := clampInt(ui.log.scroll-steps, 0, maxScroll)
			if scroll != ui.log.scroll {
				ui.log.scroll = scroll
				changed = true
			}
		}
	case uiMuseumBGV:
		if ui.bgv == nil || ui.bgv.layout == nil {
			break
		}
		layout := ui.bgv.layout
		barHit := false
		for index := range layout.Characters {
			if index >= bgvCharacterCount {
				break
			}
			if barX, barY, barW, trackH, _, _, ok := s.bgvBarGeometry(index); ok &&
				x >= barX && y >= barY && x < barX+barW && y < barY+trackH {
				before := ui.bgv.scroll[index]
				s.scrollBGVTo(index, y)
				changed = ui.bgv.scroll[index] != before
				barHit = true
				break
			}
		}
		if !barHit && lastY >= 0 {
			// 手势开始时锁定栏位：手指滑出列表纵向范围时不能突然跳回第 1 栏。
			// -1 是"本手势还没定栏"，由第一次拖动按指针位置决定。
			column := ui.bgvDragColumn
			if column < 0 || column >= bgvCharacterCount || column >= len(layout.Characters) {
				column = s.bgvWheelColumn()
				if column < 0 || column >= bgvCharacterCount || column >= len(layout.Characters) {
					column = 0
				}
				ui.bgvDragColumn = column
				ui.bgvDragRemainder = 0
			}
			character := layout.Characters[column]
			lineHeight := max(1, character.View.Height)
			visible := bgvVisibleLines(character.View)
			total := ui.bgvDragRemainder + (y - lastY)
			steps := total / lineHeight
			ui.bgvDragRemainder = total - steps*lineHeight
			maxScroll := max(0, len(character.Tracks)-visible)
			scroll := clampInt(ui.bgv.scroll[column]-steps, 0, maxScroll)
			if scroll != ui.bgv.scroll[column] {
				ui.bgv.scroll[column] = scroll
				s.buildBGVRows()
				changed = true
			}
		}
	}
	ui.lastDragY = y
	return changed
}

// cgPanLimitX/Y 返回当前缩放下 pan 的最大偏移，保证图像始终盖满画面。
//
// 缩放后的图像宽 = width*zoom，平移范围是 ±(宽-画面宽)/2 = ±width*(zoom-1)/2。
// zoom=1 时为 0，pan 恒为 0，所以渲染仍走整屏路径。
func (s *Session) cgPanLimitX() int {
	ui := s.ui
	if ui == nil || ui.cgZoom <= 1 || s.width <= 0 {
		return 0
	}
	return int(float64(s.width) * (ui.cgZoom - 1) / 2)
}

func (s *Session) cgPanLimitY() int {
	ui := s.ui
	if ui == nil || ui.cgZoom <= 1 || s.height <= 0 {
		return 0
	}
	return int(float64(s.height) * (ui.cgZoom - 1) / 2)
}

// resetCGZoom 在换 CG/换差分/Exit 时把缩放与平移清零（任务要求）。
func (s *Session) resetCGZoom() {
	ui := s.ui
	if ui == nil {
		return
	}
	ui.cgZoom = 1
	ui.cgPanX = 0
	ui.cgPanY = 0
	ui.cgDrag = false
	ui.cgDragMoved = false
	ui.cgDragX = 0
	ui.cgDragY = 0
}

func (s *Session) activateSystemUI(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil {
		return
	}
	if ui.confirm {
		switch hit.kind {
		case hitConfirmOK:
			s.confirmSave(ui.confirmSlot)
			ui.confirm = false
		case hitConfirmCancel:
			ui.confirm = false
		}
		return
	}
	// 所属页签在主菜单和三个鉴赏子页上都能点：命中后直接切换，
	// 不需要先"返回主菜单"。放在按 kind 分派之前，避免每个子页各写一遍。
	switch hit.kind {
	case hitMuseumMode:
		s.openMuseumMode(hit.index)
		return
	case hitMuseumReturn:
		// MDDVD 的 sceneroom 是脚本请求的场景鉴赏，关闭后回到剧情；
		// 标题/菜单进入的鉴赏仍然回标题。
		if s.mddvdMuseumFromScript {
			s.mddvdMuseumFromScript = false
			s.closeSystemUI()
			if !s.titleMode {
				s.lastStep = s.finishRun(s.run(commandBudget))
			}
			return
		}
		s.closeSystemUI()
		s.enterTitle()
		return
	}
	switch ui.kind {
	case uiDataSave, uiDataLoad:
		s.activateDataScreen(hit)
	case uiDataItem:
		s.activateItemScreen(hit)
	case uiMuseum:
		// 主菜单只剩"返回标题"需要处理，已在上面的公共分支里。
	case uiMuseumStrip:
		// 立绘页只有页签/返回，公共分支已经处理。
		// すくーるヘブン / 牝教師2 / 牝教師初代各有自己的卡片/查看器交互。
		if ui.shStrip != nil {
			s.activateSHStrip(hit)
		}
		if ui.mkStrip != nil {
			s.activateMKStrip(hit)
		}
		if ui.mk2Strip != nil {
			s.activateMK2Strip(hit)
		}
	case uiMuseumCG:
		s.activateMuseumCG(hit)
	case uiMuseumScene:
		s.activateMuseumScene(hit)
	case uiScenePlaylist:
		s.activateScenePlaylist(hit)
	case uiMuseumMusic:
		s.activateMuseumMusic(hit, x, y)
	case uiCGViewer:
		s.activateCGViewer(hit)
	case uiConfig:
		s.activateConfigScreen(hit, x, y)
	case uiLegacyConfig:
		s.activateLegacyConfigScreen(hit, x, y)
	case uiMuseumBGV:
		s.activateBGVMuseum(hit, x, y)
	case uiMuseumBU:
		s.activateBustupMuseum(hit, x, y)
	case uiDress:
		if ui := s.ui; ui != nil && ui.mk2Dress != nil {
			s.activateMK2Dress(hit)
			return
		}
		s.activateDressScreen(hit, x, y)
	case uiLog:
		s.activateLogScreen(hit, x, y)
	case uiStatus:
		s.activateStatusScreen(hit)
	case uiStatusSLG:
		s.activateSLGStatusScreen(hit, x, y)
	case uiStatusAncient:
		s.activateAncientStatusScreen(hit)
	case uiStatusLegacy:
		s.activateLegacyStatusScreen(hit)
	case uiKarte:
		s.activateKarteScreen(hit)
	case uiTarget:
		if hit.kind == hitTargetChoice {
			s.activateTargetSelect(hit.index)
		}
	case uiTargetCard:
		if hit.kind == hitTargetCard {
			s.activateTargetCard(hit.index)
		}
	}
}

// ---------------------------------------------------------------- 命中判定

func (s *Session) hitSystemUI(x, y int) uiHit {
	ui := s.ui
	if ui == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if ui.confirm {
		layout := s.config.DataScreen.Confirm
		if hitButton(s, layout.OK, x, y) {
			return uiHit{kind: hitConfirmOK}
		}
		if hitButton(s, layout.Cancel, x, y) {
			return uiHit{kind: hitConfirmCancel}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	switch ui.kind {
	case uiDataSave, uiDataLoad:
		return s.hitDataScreen(x, y)
	case uiDataItem:
		return s.hitItemScreen(x, y)
	case uiMuseum:
		if hit, ok := s.hitMuseumTabs(x, y); ok {
			return hit
		}
	case uiMuseumStrip:
		if ui := s.ui; ui != nil && ui.shStrip != nil {
			return s.hitSHStrip(x, y)
		}
		if hit, ok := s.hitMuseumTabs(x, y); ok {
			return hit
		}
		if ui := s.ui; ui != nil && ui.mkStrip != nil {
			return s.hitMKStrip(x, y)
		}
		if ui := s.ui; ui != nil && ui.mk2Strip != nil {
			return s.hitMK2Strip(x, y)
		}
	case uiMuseumCG:
		if hit, ok := s.hitMuseumTabs(x, y); ok {
			return hit
		}
		return s.hitMuseumCG(x, y)
	case uiMuseumScene:
		if hit, ok := s.hitMuseumTabs(x, y); ok {
			return hit
		}
		return s.hitMuseumScene(x, y)
	case uiScenePlaylist:
		return s.hitScenePlaylist(x, y)
	case uiMuseumMusic:
		if hit, ok := s.hitMuseumTabs(x, y); ok {
			return hit
		}
		return s.hitMuseumMusic(x, y)
	case uiCGViewer:
		return s.hitCGViewer(x, y)
	case uiConfig:
		return s.hitConfigScreen(x, y)
	case uiLegacyConfig:
		return s.hitLegacyConfigScreen(x, y)
	case uiMuseumBGV:
		return s.hitBGVMuseum(x, y)
	case uiMuseumBU:
		return s.hitBustupMuseum(x, y)
	case uiDress:
		if ui := s.ui; ui != nil && ui.mk2Dress != nil {
			return s.hitMK2Dress(x, y)
		}
		return s.hitDressScreen(x, y)
	case uiLog:
		return s.hitLogScreen(x, y)
	case uiStatus:
		return s.hitStatusScreen(x, y)
	case uiStatusSLG:
		return s.hitSLGStatusScreen(x, y)
	case uiStatusAncient:
		return s.hitAncientStatusScreen(x, y)
	case uiStatusLegacy:
		return s.hitLegacyStatusScreen(x, y)
	case uiKarte:
		return s.hitKarteScreen(x, y)
	case uiTarget:
		index := s.hitTargetSelect(x, y)
		ui.targetHover = index
		if index >= 0 {
			return uiHit{kind: hitTargetChoice, index: index}
		}
	case uiTargetCard:
		return s.hitTargetCardSelect(x, y)
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) hitMuseumTabs(x, y int) (uiHit, bool) {
	layout := s.config.Museum
	for index, mode := range layout.Modes {
		if hitButton(s, mode.UIButton, x, y) {
			return uiHit{kind: hitMuseumMode, index: index}, true
		}
	}
	if hitButton(s, layout.Return, x, y) {
		return uiHit{kind: hitMuseumReturn}, true
	}
	return uiHit{}, false
}

// hitButton 用图片的真实尺寸做命中，而不是配置里的帧高：
// 按钮图带透明边距，尺寸只有解码后才知道；配置只给帧数。
func hitButton(s *Session, button config.UIButton, x, y int) bool {
	// 纯热区按钮（GK 的 BGM 曲目行）：没有图片，只给了 Hit 矩形。
	// 旧实现先判 Empty 会把这些行全部当成"不存在"，BGM 列表点不动。
	if button.Empty() && (button.Hit[2] <= 0 || button.Hit[3] <= 0) {
		return false
	}
	// 配置给了显式热区时优先用它：复合条/整列图（museum_menu）的图片矩形
	// 覆盖整列，不能用来判定单个页签；热区也比透明边距更贴近原版。
	if button.Hit[2] > 0 && button.Hit[3] > 0 {
		return x >= button.Hit[0] && y >= button.Hit[1] &&
			x < button.Hit[0]+button.Hit[2] && y < button.Hit[1]+button.Hit[3]
	}
	image := s.buttonImage(button)
	if image == nil {
		return false
	}
	height := button.FrameHeight(image.Height)
	width := image.Width
	// Clip 命中只覆盖裁出的那一块，否则 SAVE 页签会把 LOAD 那半也当成自己的热区。
	if button.Clip[2] > 0 && button.Clip[3] > 0 {
		width = button.Clip[2]
		height = button.Clip[3]
	}
	return x >= button.X && y >= button.Y && x < button.X+width && y < button.Y+height
}

func (s *Session) hitDataScreen(x, y int) uiHit {
	layout := s.config.DataScreen
	// 顶部按钮优先于槽位：它们和第一行槽位有重叠的可能，先命中更符合直觉。
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitBack, layout.Back},
		{hitTitle, layout.Title},
		{hitExit, layout.Exit},
		{hitSaveMode, layout.SaveMode},
		{hitLoadMode, layout.LoadMode},
		{hitDataConfig, layout.Config},
		{hitDataItem, layout.Item},
	} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	// 三个排序按钮在底图上只有坐标、没有单独图片；按固定的小按钮区域命中。
	for index, point := range layout.SortButtons {
		if x >= point.X && y >= point.Y && x < point.X+120 && y < point.Y+24 {
			return uiHit{kind: hitDataSort, index: index}
		}
	}
	for index, button := range layout.PageButtons {
		if hitButton(s, button, x, y) {
			return uiHit{kind: hitPage, index: index}
		}
	}
	// GK 的页码只有上下箭头 + 当前页数字，没有逐页按钮。
	if layout.PagePrevPoint.X != 0 || layout.PagePrevPoint.Y != 0 {
		if pointHit(layout.PagePrevPoint, 40, 30, x, y) {
			return uiHit{kind: hitPagePrev}
		}
	}
	if layout.PageNextPoint.X != 0 || layout.PageNextPoint.Y != 0 {
		if pointHit(layout.PageNextPoint, 40, 30, x, y) {
			return uiHit{kind: hitPageNext}
		}
	}
	if layout.ShortcutAutoPoint.X != 0 || layout.ShortcutAutoPoint.Y != 0 {
		if pointHit(layout.ShortcutAutoPoint, 148, 44, x, y) {
			return uiHit{kind: hitShortcutAuto}
		}
	}
	if layout.ShortcutQuickPoint.X != 0 || layout.ShortcutQuickPoint.Y != 0 {
		if pointHit(layout.ShortcutQuickPoint, 148, 44, x, y) {
			return uiHit{kind: hitShortcutQuick}
		}
	}
	// 旧式作品（KD）把 Page Jump 的 1…10 数字直接画进 dm_save/dm_load 底图，
	// 没有独立的页码按钮图。此时只能用配置给的热区；尺寸沿用 dm_pagenum
	// 的单格 28x28（数字条是 28x84、3 帧）。
	if len(layout.PageButtons) == 0 {
		for index, point := range layout.PageHits {
			if x >= point.X && y >= point.Y && x < point.X+28 && y < point.Y+28 {
				return uiHit{kind: hitPage, index: index}
			}
		}
	}
	for index, slot := range layout.Slots {
		// 槽位命中区固定为缩略图大小；按钮图/边框图不参与，
		// 否则命中会随悬停边框的透明边距漂移。
		width, height := slotWidth(s), slotHeight(s)
		if layout.SlotSize[0] > 0 && layout.SlotSize[1] > 0 {
			width, height = layout.SlotSize[0], layout.SlotSize[1]
		}
		if x >= slot.X && y >= slot.Y && x < slot.X+width && y < slot.Y+height {
			return uiHit{kind: hitSlot, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// pointHit 判断点是否落在以 point 为左上角的固定矩形内。
func pointHit(point config.UIPoint, width, height, x, y int) bool {
	return x >= point.X && y >= point.Y && x < point.X+width && y < point.Y+height
}

func slotWidth(s *Session) int {
	return max(1, roundHalfEven(float64(s.width)*s.config.ThumbnailRatio))
}

func slotHeight(s *Session) int {
	return max(1, roundHalfEven(float64(s.height)*s.config.ThumbnailRatio))
}

// ---------------------------------------------------------------- 游戏内菜单条

// supportedMenuKeys 是 [system button] 里所有会显示在游戏内菜单条的键。
// 具体动作统一交给 activateMessageButton：对话框工具条和 ESC 菜单共用同一套
// 语义，避免两处按钮行为不一致。
var supportedMenuKeys = map[string]bool{
	"voice": true, "skip": true, "quick save": true, "quick load": true,
	"qsave": true, "qload": true, "save": true, "load": true,
	"config": true, "dress": true, "log": true, "status": true,
	"view up": true, "view middle": true, "view down": true,
	// MKIE/MK2IE 把 VIEW/STRIP 写成 simple view / simple strip（作用域 game）。
	// 缺了这两项，工具条右侧两块按钮会被 supportedMenuKeys 过滤掉，
	// 玩家在原版能看到的 顔/胸/股間 切换和脱衣按钮就消失了。
	"simple view": true, "simple strip": true,
	"auto ex": true, "auto": true, "cut in": true, "hold menu": true,
	"menu display": true,
	// 放課後3 的 [system button] 有 fire（発射）：地图/对白工具条上都画着
	// 这颗按钮。没有专门动作时也不能从工具条上消失，否则和原版截图对不上；
	// 命中后交给 activateMessageButton 的空分支（保持无副作用）。
	"fire": true,
}

func (s *Session) toggleGameMenu() {
	if s.titleMode || s.ui != nil || s.replay {
		return
	}
	// 引擎对话框模式下工具条常驻；ESC/宿主菜单按钮改为显示/隐藏工具条，
	// 和原版「メニュー表示」按钮同一语义。
	if s.gameState != nil && s.gameState.ShowMessageFrame() {
		s.toolbarVisible = !s.toolbarVisible
		s.messageHover = -1
		s.composer.Invalidate()
		return
	}
	// 菜单条里能用的入口（存/读/快存/快读）都要求"稳定对白"；在地图、
	// 选择肢、影片和转场上展开菜单只会让玩家点到被拒绝的按钮，所以直接
	// 不展开。右键取消地图仍走 Pointer(4)，不受这里影响。
	if s.lastStep != core.StepAwaitInput && s.lastStep != core.StepWait {
		return
	}
	s.menuVisible = !s.menuVisible
	s.menuHover = -1
	if s.menuVisible {
		s.gameState.SetMessageWindowVisible(false)
	} else {
		s.gameState.SetMessageWindowVisible(true)
	}
	s.composer.Invalidate()
	core.Logf("ui: game menu visible=%v", s.menuVisible)
}

// showGameMenuOnHover 在鼠标进入底部区域时展开菜单条。
//
// 触屏没有 hover，所以它只在桌面/带鼠标的设备上生效；移动端由宿主工具栏的
// 菜单按钮发 ESC 切换。
func (s *Session) showGameMenuOnHover(x, y int) bool {
	if s.titleMode || s.ui != nil || s.replay || s.menuVisible {
		return false
	}
	if s.gameState != nil && s.gameState.ShowMessageFrame() {
		return false
	}
	if !s.canPersistStoryState() {
		return false
	}
	area := s.config.GameMenu.ShowArea
	if x < area[0] || y < area[1] || x >= area[0]+area[2] || y >= area[1]+area[3] {
		return false
	}
	s.menuVisible = true
	s.menuHover = -1
	s.gameState.SetMessageWindowVisible(false)
	s.composer.Invalidate()
	return true
}

func (s *Session) pointerGameMenu(op uint32, x, y int32) (core.Step, bool) {
	// 自定义地图上的 [game menu] 是常驻条，不依赖 menuVisible；否则玩家
	// 点得到按钮图却没有热区，地图底部的 SAVE/LOAD 等入口全部失效。
	mapMenu := s.customMapToolbarVisible()
	if (!s.menuVisible && !mapMenu) || s.titleMode || s.ui != nil {
		return s.lastStep, false
	}
	switch op {
	case 0:
		hover := s.hitGameMenu(int(x), int(y))
		if hover != s.menuHover {
			s.menuHover = hover
			s.composer.Invalidate()
		}
		return s.lastStep, false
	case 1, 3:
		index := s.hitGameMenu(int(x), int(y))
		if index >= 0 {
			s.activateGameMenu(index)
			return s.lastStep, true
		}
		// 自定义地图上的角色卡/工作面板按钮可能落在 [game menu] 的
		// show area 里（放課後3 的"帰宅する"在 y=670，四张 Skip 卡在
		// 550..670，全部越过 show area 的 y=592）。这些是真正的游戏
		// 按钮，必须先让地图拿点击；旧实现只要 y 越过 show area 就吞掉，
		// 表现是 Skip/帰宅按钮图看得见却点不动，玩家只能等到时间耗尽。
		if mapMenu && s.hitButton(int(x), int(y)) >= 0 {
			return s.lastStep, false
		}
		// 点在菜单条范围内但没命中按钮：吞掉这次点击，避免把菜单点掉的同时
		// 又推进一句对白。
		area := s.config.GameMenu.ShowArea
		if int(y) >= area[1] {
			return s.lastStep, true
		}
	case 4:
		if !mapMenu {
			s.toggleGameMenu()
		}
		return s.lastStep, true
	}
	return s.lastStep, false
}

func (s *Session) hitGameMenu(x, y int) int {
	buttons := s.config.GameMenu.Buttons
	for index, button := range buttons {
		if !supportedMenuKeys[button.Key] {
			continue
		}
		if hitButton(s, button.UIButton, x, y) {
			return index
		}
	}
	return -1
}

func (s *Session) activateGameMenu(index int) {
	if s.config == nil || index < 0 || index >= len(s.config.GameMenu.Buttons) {
		return
	}
	// 和对话框底部工具条走同一条分派：存档/读档/设置/鉴赏/视点/脱衣等
	// 都只维护一份实现。
	s.activateMessageButton(index)
}

func (s *Session) setHostRequest(kind string) {
	s.hostRequest = kind
	s.hostRequestSerial++
	core.Logf("ui: host request '%s' #%d", kind, s.hostRequestSerial)
}

// ---------------------------------------------------------------- 键盘

// PressKey 处理桌面键盘快捷键，语义尽量贴近原引擎：
// ESC 关界面/开菜单条，F1 快速存档，F2 快速读档，F3 存档画面，F4 读档画面。
// 移动端没有硬件键盘，这些键位不会影响触屏操作。
func (s *Session) PressKey(code uint32) (core.Step, error) {
	if s.slgState != nil {
		// Lua 自绘界面：ESC/方向键转成原脚本的 KEY_B/WU/WD。
		key := 0
		switch code {
		case 27:
			key = 0x42
		case 38:
			key = 0x3c
		case 40:
			key = 0x3e
		}
		if key != 0 && s.slgVM != nil && s.slgVM.Key(key) {
			return s.finishRun(s.finishSLGState()), nil
		}
		return s.lastStep, nil
	}
	if s.ui != nil {
		switch code {
		case 27:
			if s.ui.confirm {
				s.ui.confirm = false
				s.composer.Invalidate()
			} else {
				s.closeSystemUI()
			}
			return s.lastStep, nil
		case 112, 113, 114, 115:
			// 界面里按 F 键不叠加第二个界面。
			return s.lastStep, nil
		}
		return s.lastStep, nil
	}
	switch code {
	case 27:
		s.toggleGameMenu()
	case 112:
		s.quickSave()
	case 113:
		s.quickLoad()
	case 114:
		s.openDataScreen(true)
	case 115:
		s.openDataScreen(false)
	}
	return s.lastStep, nil
}

// ---------------------------------------------------------------- 存档/快捷存档

// slotIndex 把"页 + 页内槽位"换算成内部槽位下标。
func (s *Session) slotIndex(page, index int) int {
	layout := s.config.DataScreen
	if page < 0 {
		page = 0
	}
	return page*layout.SlotsPerPage + index
}

// visibleSlotIndex 在当前排序下取"页 + 页内槽位"对应的真实槽位。
//
// 排序改变的是整张网格的顺序（新到旧/旧到新），不是只重排当前页；
// 否则翻页时又会回到槽位号顺序，玩家会觉得排序只生效了一页。
func (s *Session) visibleSlotIndex(page, index int) int {
	ui := s.ui
	if ui == nil || ui.sortMode == 0 || len(ui.slotOrder) == 0 {
		return s.slotIndex(page, index)
	}
	layout := s.config.DataScreen
	if page < 0 {
		page = 0
	}
	position := page*layout.SlotsPerPage + index
	if position < 0 || position >= len(ui.slotOrder) {
		return s.slotIndex(page, index)
	}
	return ui.slotOrder[position]
}

// ensureSlotOrder 按当前排序方式重建全局槽位顺序。
//
// 只有用户切到"新到旧/旧到新"时才读 280 个存档的 SavedAt；排序结果缓存，
// 翻页/重绘不会反复读盘。存档写入后把 slotOrderMode 置 -1 强制刷新。
func (s *Session) ensureSlotOrder() {
	ui := s.ui
	if ui == nil {
		return
	}
	if ui.sortMode == 0 {
		ui.slotOrder = nil
		ui.slotOrderMode = 0
		return
	}
	if ui.slotOrderMode == ui.sortMode && len(ui.slotOrder) > 0 {
		return
	}
	order := make([]int, save.GridSlotCount)
	for index := range order {
		order[index] = index
	}
	if s.saves != nil {
		// 先一次性读齐 280 个槽的元数据再做比较：如果比较函数里每次 Read，
		// 排序会读几千次文件，真机上点一下排序会卡好几百毫秒。
		saved := make([]*save.SaveData, len(order))
		for index := range saved {
			saved[index] = s.saves.Read(index)
		}
		sort.SliceStable(order, func(a, b int) bool {
			left := saved[order[a]]
			right := saved[order[b]]
			// 空槽永远排在后面：否则"新到旧"会把一整页空槽顶到第一页。
			if left == nil {
				return false
			}
			if right == nil {
				return true
			}
			if ui.sortMode == 1 {
				return left.SavedAt.After(right.SavedAt)
			}
			return left.SavedAt.Before(right.SavedAt)
		})
	}
	ui.slotOrder = order
	ui.slotOrderMode = ui.sortMode
}

func (s *Session) activateDataScreen(hit uiHit) {
	ui := s.ui
	layout := s.config.DataScreen
	switch hit.kind {
	case hitPage:
		if hit.index >= 0 && hit.index < layout.PageMax {
			ui.page = hit.index
		}
	case hitPagePrev:
		if ui.page > 0 {
			ui.page--
			ui.slotPage = -1
			if s.composer != nil {
				s.composer.Invalidate()
			}
		}
	case hitPageNext:
		if ui.page+1 < max(1, layout.PageMax) {
			ui.page++
			ui.slotPage = -1
			if s.composer != nil {
				s.composer.Invalidate()
			}
		}
	case hitShortcutAuto:
		// 原版 Auto 快捷键读取自动存档槽；读档失败只记日志。
		s.closeSystemUI()
		if step, err := s.Load(uint32(save.AutoSlot)); err != nil {
			core.Logf("ui: shortcut auto load failed: %v", err)
		} else {
			s.lastStep = step
		}
	case hitShortcutQuick:
		s.closeSystemUI()
		s.quickLoad()
	case hitDataSort:
		if hit.index >= 0 && hit.index <= 2 {
			ui.sortMode = hit.index
			ui.slotOrderMode = -1
			ui.page = 0
			ui.slotPage = -1
			s.ensureSlotOrder()
		}
	case hitSaveMode:
		ui.kind = uiDataSave
		ui.confirm = false
		ui.deleteMode = false
	case hitLoadMode:
		ui.kind = uiDataLoad
		ui.confirm = false
		ui.deleteMode = false
	case hitDataConfig:
		// 存档页的"设置"入口：和标题/工具条走同一个配置画面。
		if !s.openConfigScreen() {
			s.setHostRequest("settings")
		}
	case hitDataItem:
		if !s.openItemScreen() {
			core.Logf("ui: item screen unavailable")
		}
	case hitBack:
		s.closeSystemUI()
	case hitTitle:
		s.closeSystemUI()
		s.enterTitle()
	case hitExit:
		s.setHostRequest("exit")
		s.closeSystemUI()
	case hitSlot:
		slot := s.visibleSlotIndex(ui.page, hit.index)
		if slot >= save.GridSlotCount {
			return
		}
		if ui.deleteMode {
			// DELETE 模式只删不读：删完立刻刷新缩略图/排序缓存，玩家能马上看到空槽。
			if s.saves != nil {
				s.saves.Delete(slot)
			}
			ui.slotPage = -1
			ui.slotOrderMode = -1
			ui.slotSaved = nil
			ui.deleteMode = false
			s.composer.Invalidate()
			core.Logf("ui: deleted save slot %d", slot)
			return
		}
		if ui.kind == uiDataLoad {
			if layout.AutoSlotGrid && ui.page == 0 && hit.index == layout.AutoSlotIndex {
				slot = save.AutoSlot
			}
			s.loadFromSystemUI(slot)
			return
		}
		// 第 1 页第 1 格是「AUTO SAVE ONLY」：只允许读，不允许玩家手动覆盖。
		// 原版这个格子由每日自动存档写入，手动存档会毁掉自动档。
		if ui.page == 0 && hit.index == layout.AutoSlotIndex {
			core.Logf("ui: auto-save slot is read-only")
			return
		}
		if s.saves != nil && s.saves.Read(slot) != nil {
			ui.confirm = true
			ui.confirmSlot = slot
			return
		}
		s.confirmSave(slot)
	}
}

func (s *Session) confirmSave(slot int) {
	if err := s.Save(uint32(slot), s.summarize()); err != nil {
		core.Logf("ui: save slot %d failed: %v", slot, err)
	}
}

// loadFromSystemUI 从系统界面读档：先关界面再 Load，Load 会恢复文本与舞台。
func (s *Session) loadFromSystemUI(slot int) {
	s.closeSystemUI()
	step, err := s.Load(uint32(slot))
	if err != nil {
		core.Logf("ui: load slot %d failed: %v", slot, err)
		return
	}
	s.lastStep = step
}

func (s *Session) quickSave() {
	// 快速存档不占用手动槽位，读取时也不会出现在存档画面里。
	//
	// 和存档画面共用同一条"只能停在稳定对白上"的约束：地图/选择肢的
	// UI 状态不落盘，在这里写快速存档同样会得到一个读不回来的档。
	if !s.canPersistStoryState() || s.replay || s.wordPlay != nil {
		core.Logf("ui: quick save ignored outside stable dialogue")
		return
	}
	if err := s.Save(uint32(save.QuickSlot), s.summarize()); err != nil {
		core.Logf("ui: quick save failed: %v", err)
		return
	}
	s.menuVisible = false
	s.gameState.SetMessageWindowVisible(true)
	s.composer.Invalidate()
}

// autoSave 写原版的自动存档槽（存档画面的"auto only"格）。
//
// 和 quickSave 不同，它由剧本的 `auto save` 命令在行动/选择画面之前触发：
// 那时 PC 已经越过 SELECT_TAIL，读档后脚本会从下一条指令继续并重新铺出
// 行动面板，因此不依赖 lastStep == AwaitInput。写盘失败只记日志，不能
// 让剧情卡在命令上。
func (s *Session) autoSave() {
	if s.saves == nil || s.machine == nil || s.composer == nil || s.titleMode {
		return
	}
	if s.slgState != nil {
		return
	}
	if err := s.Save(uint32(save.AutoSlot), s.summarize()); err != nil {
		core.Logf("ui: auto save failed: %v", err)
		return
	}
	core.Logf("ui: auto save written at pc %d", s.machine.ProgramCounter())
}

// canPersistStoryState 判断当前是否处在"存档能完整恢复"的稳定状态。
func (s *Session) canPersistStoryState() bool {
	if s.titleMode {
		return false
	}
	if s.lastStep == core.StepAwaitInput {
		return true
	}
	// 脚本自绘地图的按钮已经写进 SaveData 扩展段，地图上也能安全快存/快读。
	if s.mapSource == "custom" && s.gameState != nil && s.gameState.Selecting() {
		return true
	}
	// map.dat 地图（MK/MK2/放課後2 等）的 Save 会把 PC 倒回 SELECT_TAIL，
	// 读档后由 profile 重新铺同一张地图；原版也能在地图上 QSAVE/QLOAD。
	if s.mapSource == bsmap.BsMapDataFileName && s.gameState != nil && s.gameState.Selecting() {
		return true
	}
	// GK 的地图由 MapPrms + profile 重建，Save 会把 PC 倒回 SELECT_TAIL，
	// 读档后重新铺同一张地图，所以地图上也能存档。
	return s.mapSource == bsmap.BsMapGkSource && s.gameState != nil && s.gameState.Selecting()
}

func (s *Session) quickLoad() {
	if s.replay {
		return
	}
	s.menuVisible = false
	step, err := s.Load(uint32(save.QuickSlot))
	if err != nil {
		core.Logf("ui: quick load failed: %v", err)
		s.gameState.SetMessageWindowVisible(true)
		s.composer.Invalidate()
		return
	}
	s.lastStep = step
}

// HostRequestState 暴露给主机状态快照（由 State() 调用）。
func (s *Session) hostRequestState() (string, uint32) {
	return s.hostRequest, s.hostRequestSerial
}
