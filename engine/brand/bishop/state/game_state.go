package state

import (
	"path/filepath"
	"strings"
)

const (
	BackgroundSlot       = 1
	PlaneCount           = 32
	OverlayPlane         = PlaneCount - 1
	NoTransition         = 0
	MaskTransition       = 1
	WhiteFlashTransition = 25
)

type GameState struct {
	speaker string
	text    string
	// playerFamily/playerGiven 是剧本 \A/\B 占位符要替换成的主人公姓名。
	// 原版允许在设置里改名；没有登记默认姓名的作品留空，文本层会把占位符
	// 当作控制码剥掉，至少不会把 \A/\B 直接画进对话框。
	playerFamily string
	playerGiven  string
	// multiSpeaker/multiText 是原版「multi messages = 2」的第二个发言框。
	// 剧本用 window part=2 切到它；两个框可以同时显示（例如两人对话）。
	multiSpeaker string
	// extraMeterProperty 是 [extra window] 的等级变量（剧本 `set levelmeter` 指定）。
	extraMeterProperty string
	multiText          string
	// multiMessageVisible 是第二消息框的显示开关；主框仍用
	// messageWindowVisible。两者分开，window part=1/2 才能独立控制。
	multiMessageVisible bool
	// selectMultiHidden 记录进选择前第二框是不是关的，退出时原样恢复。
	selectMultiHidden   bool
	bgm                 string
	hasBgm              bool
	bgmVolume           float64
	bgmLoop             bool
	soundEffect         string
	hasSoundEffect      bool
	voice               string
	hasVoice            bool
	voiceSequence       []string
	voiceSequenceSerial int
	voiceSequenceLoop   bool
	maskImage           string
	hasMaskImage        bool
	transitionActive    bool
	transitionType      int
	transitionDuration  float64
	transitionSerial    int
	movie               string
	hasMovie            bool
	movieSerial         int
	movieFiles          []string
	voiceSerial         int
	effectSerial        int
	awaitingInput       bool
	awaitingChoice      bool
	// messageId 是"当前这句对白在脚本消息表里的下标"。
	// 全局存档按这个下标记录"已读"，所以必须从对白指令带出来，
	// 不能靠文本内容反查（同一句话可能重复出现）。
	messageId            int
	messageWindowVisible bool
	showMessageFrame     bool
	// menuBarVisible 是常驻工具条的显示开关（学園2 顶部菜单条）。
	// 标题/系统界面以外的剧情与地图画面都显示它。
	menuBarVisible bool
	// viewMode 是原版底部「顔/胸/腰」视点按钮的当前选择：
	// 0=顔（使用 f 立绘裁切）、1=胸、2=腰（使用 u 全身图裁切）。
	viewMode         int
	textSpeed        float64
	waitMilliseconds int
	fillAlpha        float64
	fillRed          uint8
	fillGreen        uint8
	fillBlue         uint8
	stageRevision    int
	selecting        bool
	// selectWindowHidden 记录"进入选择前对话框是否是隐藏的"。
	// 选择结束时要按这个值恢复，而不是无条件显示：脚本可能本来就把
	// 对话框关着（例如地图过渡、纯演出段），强行打开会多出一个空框。
	selectWindowHidden bool
	hoveredButton      int
	counterValue       int
	planes             [PlaneCount]*StageLayer
	layers             []*StageLayer
	// animators 是"挂在某个图层上的逐帧动画"（モーションプラス 的动态 CG）。
	// 有动画时渲染器画动画帧而不是该层的静态图；图层被换图/清除时一起清掉。
	animators map[int]FrameAnimator
	// motions 保存 LAYER_PARAM type=6 的图层关键帧动画（移动/淡入淡出）。
	// 与 animators 分开：前者按墙钟取一张整屏图，后者只改变已经画好的
	// 图层的位移/透明度，两者互不覆盖。
	motions     map[int]*LayerMotion
	buttons     []StageButton
	choices     []string
	selectItems []SelectItem
	// selectHover 是玩家指着/拖到的选项下标；-1 表示没有。
	selectHover int
	// selectRevision 在选项列表或悬停变化时递增，渲染器据此重绘。
	selectRevision int
	bsLayers       []BsLayerEntry
	overlays       []StageOverlay
}

func NewGameState() *GameState {
	state := &GameState{
		bgmVolume:            0.8,
		bgmLoop:              true,
		messageWindowVisible: true,
		// 默认由引擎绘制原版对话框；宿主在设置里关掉时才改回 Flutter 的 MessageBox。
		// 放在状态默认值里，避免宿主还没来得及发 option 的第一帧没有对话框。
		showMessageFrame: true,
		textSpeed:        1.0,
		hoveredButton:    -1,
		selectHover:      -1,
		counterValue:     -1,
		movieFiles:       []string{},
	}
	for index := 0; index < PlaneCount; index++ {
		state.planes[index] = NewStageLayer(index, StageLayerScreen)
		state.layers = append(state.layers, state.planes[index])
	}
	return state
}

func (s *GameState) Layers() []*StageLayer {
	return s.layers
}

func (s *GameState) BsLayers() []BsLayerEntry {
	return s.bsLayers
}

func (s *GameState) Overlays() []StageOverlay {
	return s.overlays
}

func (s *GameState) StageRevision() int {
	return s.stageRevision
}

func (s *GameState) BumpStageRevision() {
	s.stageRevision++
}

func (s *GameState) Speaker() string {
	return s.speaker
}

func (s *GameState) SetSpeaker(value string) {
	s.speaker = value
}

func (s *GameState) Text() string {
	return s.text
}

func (s *GameState) SetText(value string) {
	s.text = value
}

// SetPlayerName 设置剧本 \A（姓）/ \B（名）要替换成的主人公姓名。
//
// 空字符串表示这部作品没有默认姓名；ResolveNameCodes 会把占位符去掉，
// 后续 bs.Message/bs.Speaker 再剥其它控制码。
func (s *GameState) SetPlayerName(family, given string) {
	s.playerFamily = family
	s.playerGiven = given
}

// ResolveNameCodes 把正文/角色名里的 \A、\B 换成人名。
//
// 必须在 bs.Message/bs.Speaker 之前调用：那两个函数会把"反斜杠+下一个字符"
// 当控制码整体删掉，先替换才能保住"雅也君"里的名字。\b（小写）是行首
// 强调码，不是姓名，保持原样交给剥码逻辑。
func (s *GameState) ResolveNameCodes(value string) string {
	if len(value) == 0 || (s.playerFamily == "" && s.playerGiven == "") {
		return value
	}
	value = strings.ReplaceAll(value, `\A`, s.playerFamily)
	value = strings.ReplaceAll(value, `\B`, s.playerGiven)
	return value
}

func (s *GameState) Bgm() (string, bool) {
	return s.bgm, s.hasBgm
}

func (s *GameState) SetBgm(name string) {
	s.bgm = name
	s.hasBgm = true
}

func (s *GameState) ClearBgm() {
	s.bgm = ""
	s.hasBgm = false
}

func (s *GameState) BgmVolume() float64 {
	return s.bgmVolume
}

func (s *GameState) SetBgmVolume(value float64) {
	s.bgmVolume = value
}

func (s *GameState) BgmLoop() bool {
	return s.bgmLoop
}

func (s *GameState) SetBgmLoop(value bool) {
	s.bgmLoop = value
}

func (s *GameState) SoundEffect() (string, bool) {
	return s.soundEffect, s.hasSoundEffect
}

func (s *GameState) SetSoundEffect(name string) {
	s.soundEffect = name
	s.hasSoundEffect = true
}

func (s *GameState) ClearSoundEffect() {
	s.soundEffect = ""
	s.hasSoundEffect = false
}

func (s *GameState) Voice() (string, bool) {
	return s.voice, s.hasVoice
}

func (s *GameState) SetVoice(name string) {
	s.voice = name
	s.hasVoice = true
}

func (s *GameState) ClearVoice() {
	s.voice = ""
	s.hasVoice = false
}

func (s *GameState) VoiceSequence() []string {
	return s.voiceSequence
}

func (s *GameState) VoiceSequenceSerial() int {
	return s.voiceSequenceSerial
}

func (s *GameState) SetVoiceSequenceSerial(value int) {
	s.voiceSequenceSerial = value
}

func (s *GameState) VoiceSequenceLoop() bool {
	return s.voiceSequenceLoop
}

func (s *GameState) SetVoiceSequenceLoop(value bool) {
	s.voiceSequenceLoop = value
}

func (s *GameState) MaskImage() (string, bool) {
	return s.maskImage, s.hasMaskImage
}

func (s *GameState) ClearMaskImage() {
	s.maskImage = ""
	s.hasMaskImage = false
}

func (s *GameState) TransitionActive() bool {
	return s.transitionActive
}

func (s *GameState) TransitionType() int {
	return s.transitionType
}

func (s *GameState) TransitionDuration() float64 {
	return s.transitionDuration
}

func (s *GameState) TransitionSerial() int {
	return s.transitionSerial
}

func (s *GameState) Movie() (string, bool) {
	return s.movie, s.hasMovie
}

func (s *GameState) SetMovie(name string) {
	s.movie = name
	s.hasMovie = true
}

func (s *GameState) ClearMovie() {
	s.movie = ""
	s.hasMovie = false
}

func (s *GameState) MovieSerial() int {
	return s.movieSerial
}

func (s *GameState) SetMovieSerial(value int) {
	s.movieSerial = value
}

func (s *GameState) MovieFiles() []string {
	return s.movieFiles
}

func (s *GameState) SetMovieFiles(value []string) {
	s.movieFiles = value
}

func (s *GameState) VoiceSerial() int {
	return s.voiceSerial
}

func (s *GameState) SetVoiceSerial(value int) {
	s.voiceSerial = value
}

func (s *GameState) EffectSerial() int {
	return s.effectSerial
}

func (s *GameState) SetEffectSerial(value int) {
	s.effectSerial = value
}

func (s *GameState) MessageId() int {
	return s.messageId
}

func (s *GameState) SetMessageId(value int) {
	s.messageId = value
}

func (s *GameState) AwaitingInput() bool {
	return s.awaitingInput
}

func (s *GameState) SetAwaitingInput(value bool) {
	s.awaitingInput = value
}

func (s *GameState) AwaitingChoice() bool {
	return s.awaitingChoice
}

func (s *GameState) SetAwaitingChoice(value bool) {
	s.awaitingChoice = value
}

func (s *GameState) MessageWindowVisible() bool {
	return s.messageWindowVisible
}

func (s *GameState) SetMessageWindowVisible(value bool) {
	s.messageWindowVisible = value
	s.multiMessageVisible = value
}

// SetWindowVisible 按 window 命令的 part 控制主框/第二框。
//
// part 1 = 主消息框，part 2 = 多消息的第二框；part 0 在部分作品里表示
// "两个一起"，这里按 1+2 处理。flag 0/1 是隐藏/显示。
func (s *GameState) SetWindowVisible(part int, visible bool) {
	switch part {
	case 1:
		s.messageWindowVisible = visible
	case 2:
		s.multiMessageVisible = visible
	default:
		s.messageWindowVisible = visible
		s.multiMessageVisible = visible
	}
}

func (s *GameState) MultiSpeaker() string {
	return s.multiSpeaker
}

func (s *GameState) SetMultiSpeaker(value string) {
	s.multiSpeaker = value
}

// ExtraMeterProperty 是 [extra window] 的等级变量名。
//
// 剧本用 `set levelmeter #hearts` 告诉引擎"等级看 #hearts"；学園2 老版没有
// 这条指令，因此默认空串表示不画等级帧。
func (s *GameState) ExtraMeterProperty() string {
	return s.extraMeterProperty
}

func (s *GameState) SetExtraMeterProperty(value string) {
	s.extraMeterProperty = value
}

func (s *GameState) MultiText() string {
	return s.multiText
}

func (s *GameState) SetMultiText(value string) {
	s.multiText = value
}

func (s *GameState) MultiMessageVisible() bool {
	return s.multiMessageVisible
}

func (s *GameState) ShowMessageFrame() bool {
	return s.showMessageFrame
}

// MenuBarVisible 表示常驻工具条（学園2 顶部 Q.SAVE…QUIT）当前是否显示。
//
// 它是引擎流程状态而不是剧情状态：标题画面隐藏、进入剧情/地图后显示，
// 由 Session 在 enterTitle/leaveTitle 里切换，Reset 不清除。
func (s *GameState) MenuBarVisible() bool {
	return s.menuBarVisible
}

func (s *GameState) SetMenuBarVisible(value bool) {
	s.menuBarVisible = value
}

func (s *GameState) ViewMode() int {
	return s.viewMode
}

func (s *GameState) SetViewMode(value int) {
	if value < 0 {
		value = 0
	}
	if value > 2 {
		value = 2
	}
	s.viewMode = value
}

func (s *GameState) SetShowMessageFrame(value bool) {
	s.showMessageFrame = value
}

func (s *GameState) TextSpeed() float64 {
	return s.textSpeed
}

func (s *GameState) SetTextSpeed(value float64) {
	s.textSpeed = value
}

func (s *GameState) WaitMilliseconds() int {
	return s.waitMilliseconds
}

func (s *GameState) SetWaitMilliseconds(value int) {
	s.waitMilliseconds = value
}

func (s *GameState) FillAlpha() float64 {
	return s.fillAlpha
}

func (s *GameState) SetFillAlpha(value float64) {
	s.fillAlpha = value
}

func (s *GameState) FillRed() uint8 {
	return s.fillRed
}

func (s *GameState) SetFillRed(value uint8) {
	s.fillRed = value
}

func (s *GameState) FillGreen() uint8 {
	return s.fillGreen
}

func (s *GameState) SetFillGreen(value uint8) {
	s.fillGreen = value
}

func (s *GameState) FillBlue() uint8 {
	return s.fillBlue
}

func (s *GameState) SetFillBlue(value uint8) {
	s.fillBlue = value
}

func (s *GameState) ScreenCount() int {
	return s.countKind(StageLayerScreen)
}

func (s *GameState) CharacterCount() int {
	return s.countKind(StageLayerCharacter)
}

func (s *GameState) HasAnimatedLayer() bool {
	for _, layer := range s.layers {
		if !layer.IsDrawable() {
			continue
		}
		for _, artwork := range layer.Artworks() {
			if artwork.IsAnimated() {
				return true
			}
		}
	}
	return false
}

func (s *GameState) Buttons() []StageButton {
	return s.buttons
}

func (s *GameState) Selecting() bool {
	return s.selecting
}

func (s *GameState) HoveredButton() int {
	return s.hoveredButton
}

func (s *GameState) CounterValue() int {
	return s.counterValue
}

func (s *GameState) SetCounterValue(value int) {
	s.counterValue = value
}

func (s *GameState) Choices() []string {
	return s.choices
}

func (s *GameState) SelectItems() []SelectItem {
	return s.selectItems
}

func (s *GameState) Background() (string, bool) {
	for _, layer := range s.layers {
		if len(layer.Artworks()) > 0 {
			return layer.Name(), true
		}
	}
	return "", false
}

func (s *GameState) LayerAt(plane int) *StageLayer {
	if plane < 0 {
		plane = 0
	}
	if plane < PlaneCount {
		return s.planes[plane]
	}
	return s.planes[PlaneCount-1]
}

func (s *GameState) SetLayerAt(plane int, name string, kind StageLayerKind, artworks []LayerArtwork) {
	s.LayerAt(plane).Set(name, kind, artworks)
	s.ClearLayerAnimator(plane)
	s.ClearLayerMotion(plane)
	s.stageRevision++
}

// AppendLayerAt 把图追加到指定层（IMAGE_EX 的前景层语义）。
func (s *GameState) AppendLayerAt(plane int, name string, kind StageLayerKind, artworks []LayerArtwork) {
	s.LayerAt(plane).Append(name, kind, artworks)
	s.ClearLayerAnimator(plane)
	s.ClearLayerMotion(plane)
	s.stageRevision++
}

// ReplaceLayerArtwork 原位替换一个部件，用于 CHANGE_IMAGE 更新多层合成层里的单个元素。
func (s *GameState) ReplaceLayerArtwork(plane, index int, artwork LayerArtwork) bool {
	layer := s.LayerAt(plane)
	if !layer.ReplaceArtwork(index, artwork) {
		return false
	}
	s.ClearLayerAnimator(plane)
	s.ClearLayerMotion(plane)
	s.stageRevision++
	return true
}

func (s *GameState) SetBsLayers(entries []BsLayerEntry) {
	count := len(entries)
	if count > PlaneCount {
		count = PlaneCount
	}
	previousCount := len(s.bsLayers)
	upper := previousCount
	if count > upper {
		upper = count
	}
	for index := 0; index < upper; index++ {
		var next *BsLayerEntry
		if index < count {
			next = &entries[index]
		}
		var previous *BsLayerEntry
		if index < previousCount {
			previous = &s.bsLayers[index]
		}
		if next != nil {
			if previous != nil && sameBsLayer(*previous, *next) {
				continue
			}
			s.SetLayerAt(index, next.Name, next.Kind, next.Artworks)
			s.LayerAt(index).SetVisible(next.Visible)
			continue
		}
		if previous != nil {
			s.ClearLayer(index)
		}
	}
	s.bsLayers = append(s.bsLayers[:0], entries[:count]...)
}

func (s *GameState) RemoveBsLayer(index int) bool {
	if index < 0 || index >= len(s.bsLayers) {
		return false
	}
	table := s.copyBsLayers()
	table = append(table[:index], table[index+1:]...)
	s.SetBsLayers(table)
	return true
}

func (s *GameState) MoveBsLayer(from, to int) bool {
	if from < 0 || to < 0 || from >= len(s.bsLayers) || to >= len(s.bsLayers) || from == to {
		return false
	}
	table := s.copyBsLayers()
	entry := table[from]
	table = append(table[:from], table[from+1:]...)
	table = append(table[:to], append([]BsLayerEntry{entry}, table[to:]...)...)
	s.SetBsLayers(table)
	return true
}

func (s *GameState) SwapBsLayers(first, second int) bool {
	if first < 0 || second < 0 || first >= len(s.bsLayers) || second >= len(s.bsLayers) || first == second {
		return false
	}
	table := s.copyBsLayers()
	table[first], table[second] = table[second], table[first]
	s.SetBsLayers(table)
	return true
}

func (s *GameState) SetBsLayer(index int, entry BsLayerEntry) bool {
	if index < 0 || index >= len(s.bsLayers) {
		return false
	}
	table := s.copyBsLayers()
	table[index] = entry
	s.SetBsLayers(table)
	return true
}

func (s *GameState) SetBsLayerArtworks(index int, artworks []LayerArtwork) bool {
	if index < 0 || index >= len(s.bsLayers) {
		return false
	}
	entry := s.bsLayers[index]
	entry.Artworks = artworks
	return s.SetBsLayer(index, entry)
}

func (s *GameState) SetBsLayerVisible(index int, visible bool) bool {
	if index < 0 || index >= len(s.bsLayers) {
		return false
	}
	entry := s.bsLayers[index]
	if entry.Visible == visible {
		return true
	}
	entry.Visible = visible
	return s.SetBsLayer(index, entry)
}

func (s *GameState) IndexOfBsLayer(name string) int {
	for index, entry := range s.bsLayers {
		if entry.Name == name {
			return index
		}
	}
	return -1
}

func (s *GameState) copyBsLayers() []BsLayerEntry {
	table := make([]BsLayerEntry, 0, len(s.bsLayers)+1)
	table = append(table, s.bsLayers...)
	return table
}

func sameBsLayer(left, right BsLayerEntry) bool {
	return left.X == right.X &&
		left.Y == right.Y &&
		left.Visible == right.Visible &&
		left.Kind == right.Kind &&
		left.Name == right.Name &&
		sameArtworks(left.Artworks, right.Artworks)
}

func sameArtworks(left, right []LayerArtwork) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *GameState) SetOverlays(overlays []StageOverlay) {
	if s.sameOverlays(overlays) {
		return
	}
	s.overlays = append(s.overlays[:0], overlays...)
	s.stageRevision++
}

func (s *GameState) sameOverlays(overlays []StageOverlay) bool {
	if len(overlays) != len(s.overlays) {
		return false
	}
	for index := range overlays {
		if overlays[index] != s.overlays[index] {
			return false
		}
	}
	return true
}

func (s *GameState) ReleaseScreen(plane int) {
	if plane <= 0 || plane >= PlaneCount {
		return
	}
	s.LayerAt(plane).Reset()
	s.stageRevision++
}

func (s *GameState) ClearLayer(plane int) {
	s.LayerAt(plane).Reset()
	s.ClearLayerAnimator(plane)
	s.ClearLayerMotion(plane)
	s.stageRevision++
}

func (s *GameState) ClearStage() {
	for _, layer := range s.layers {
		layer.Reset()
	}
	s.clearAnimators()
	s.clearMotions()
	s.buttons = s.buttons[:0]
	s.bsLayers = s.bsLayers[:0]
	s.overlays = s.overlays[:0]
	s.stageRevision++
}

func (s *GameState) AddButton(button StageButton) {
	s.buttons = append(s.buttons, button)
	s.stageRevision++
}

// MoveButtonByLayer 把绑定在指定舞台层上的按钮移到 (x,y)。
//
// 放課後3 的地图卡是两层信息分开给的：CHAR type=160 只登记图片/层号，
// 真正的屏幕位置在稍后的 LAYER_PARAM type=0 里按层号下发。按钮必须
// 跟着层号移动，否则所有角色卡都留在登记时的 (0,0)，点谁都命中同一处。
// 层号 -1（IMAGE/IMAGE_EX 直接给坐标的按钮）不参与匹配。
func (s *GameState) MoveButtonByLayer(layer, x, y int) bool {
	if layer < 0 {
		return false
	}
	moved := false
	for index := range s.buttons {
		if s.buttons[index].Layer != layer {
			continue
		}
		if s.buttons[index].X == x && s.buttons[index].Y == y {
			continue
		}
		dx := x - s.buttons[index].X
		dy := y - s.buttons[index].Y
		s.buttons[index].X = x
		s.buttons[index].Y = y
		// 带 _hit 掩码的按钮，导出的热区外接矩形也要跟着走；
		// 放課後3 的卡没有掩码，这里是给同一条 LAYER_PARAM 的其它作品兜底。
		if s.buttons[index].HitMask != nil {
			s.buttons[index].HitX += dx
			s.buttons[index].HitY += dy
		}
		moved = true
	}
	if moved {
		s.stageRevision++
	}
	return moved
}

// MoveLayerTo 把舞台层移动到 (x,y)，保持同一层内多个部件的相对位置。
//
// 只有真的画过内容的层才移动；地图卡在引擎里是按钮而不是图层，所以
// 卡片场景会走 MoveButtonByLayer，这里只兜住其它用 LAYER_PARAM 摆图的
// 作品，行为与"整层平移到目标原点"一致。
func (s *GameState) MoveLayerTo(plane, x, y int) bool {
	if plane < 0 || plane >= PlaneCount {
		return false
	}
	layer := s.LayerAt(plane)
	pieces := layer.Artworks()
	if len(pieces) == 0 {
		return false
	}
	dx := x - pieces[0].X
	dy := y - pieces[0].Y
	if dx == 0 && dy == 0 {
		return false
	}
	layer.Translate(dx, dy)
	s.stageRevision++
	return true
}

// BeginSelect 进入选择（地图/分歧）状态。
//
// 选择期间必须隐藏对话框：地图是把可点地点铺满整屏，保留下方对话框会让
// 玩家看到"上一句台词还挂在地图上"，和原引擎的表现不一致（原引擎进地图前
// 就把消息窗收掉了）。这里记下原状态，退出时按原样恢复。
func (s *GameState) BeginSelect() {
	if s.selecting {
		return
	}
	s.selecting = true
	s.selectWindowHidden = !s.messageWindowVisible
	s.selectMultiHidden = !s.multiMessageVisible
	s.messageWindowVisible = false
	s.multiMessageVisible = false
	s.stageRevision++
}

func (s *GameState) SetHoveredButton(index int) {
	if s.hoveredButton == index {
		return
	}
	s.hoveredButton = index
	s.stageRevision++
}

func (s *GameState) EndSelect() {
	if s.selecting {
		// 只有真的在"选择中"才恢复：Reset/ClearStage 也会调 EndSelect，
		// 那时对话框可见性由调用方自己设定，不该被这里覆盖。
		s.messageWindowVisible = !s.selectWindowHidden
		s.multiMessageVisible = !s.selectMultiHidden
	}
	s.selecting = false
	s.selectWindowHidden = false
	s.selectMultiHidden = false
	s.hoveredButton = -1
	s.buttons = s.buttons[:0]
	s.stageRevision++
}

// SetLayerVisible 控制单个层是否绘制（SHOW_LAYER）。
//
// 地图会把每个前景部件放在独立层上，再用 SHOW_LAYER(层号, -1) 隐藏
// 当前没有事件/搜索选项的地点条；隐藏只影响绘制，图层内容保留，
// 后续 SHOW_LAYER(层号, 0) 可以原样恢复。
func (s *GameState) SetLayerVisible(plane int, visible bool) {
	if plane >= 0 && plane < PlaneCount {
		s.planes[plane].SetVisible(visible)
		s.stageRevision++
	}
}

func (s *GameState) SetAlpha(plane int, value float64) {
	if plane >= 0 && plane < PlaneCount {
		s.planes[plane].SetAlpha(value)
		s.stageRevision++
	}
}

// MoviePath 把剧本/标题里的影片名解析成实际文件路径。
//
// 剧本里的名字通常是文件名主干（op / staffroll），但这里也接受带扩展名的
// 文件名、带目录的路径和 "*op" 通配写法：手改脚本或不同作品的写法不一致时，
// 不应因为多一个 ".mpg" 就静默跳过影片。比较一律大小写不敏感。
func (s *GameState) MoviePath(name string) (string, bool) {
	wanted := strings.TrimSpace(name)
	if wanted == "" {
		return "", false
	}
	// "*op" 里星号只表示"任意目录"，不参与文件名比较。
	candidate := strings.TrimPrefix(wanted, "*")
	candidate = filepath.Base(candidate)
	extension := filepath.Ext(candidate)
	stem := strings.TrimSuffix(candidate, extension)
	for _, path := range s.movieFiles {
		file := filepath.Base(path)
		if strings.EqualFold(candidate, file) {
			return path, true
		}
		// 只有在调用方没写扩展名时才按主干比较：
		// 否则 "foo.bar" 会误命中 "foo.mpg"。
		if extension == "" && strings.EqualFold(stem, movieStem(path)) {
			return path, true
		}
	}
	return "", false
}

func movieStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

func (s *GameState) SetMask(image string) {
	s.maskImage = image
	s.hasMaskImage = true
}

func (s *GameState) StartTransition(transitionType int, durationSeconds float64) {
	s.transitionType = transitionType
	if durationSeconds > 0 {
		s.transitionDuration = durationSeconds
	} else {
		s.transitionDuration = 0.35
	}
	s.transitionActive = transitionType != NoTransition
	s.transitionSerial++
}

func (s *GameState) EndTransition() {
	s.transitionActive = false
}

func (s *GameState) SetVoiceSequence(voices []string, loop bool) {
	s.voiceSequence = append(s.voiceSequence[:0], voices...)
	s.voiceSequenceLoop = loop
	s.voiceSequenceSerial++
}

func (s *GameState) ClearVoiceSequence() {
	s.voiceSequence = s.voiceSequence[:0]
	s.voiceSequenceLoop = false
	s.voiceSequenceSerial++
}

func (s *GameState) SetChoices(choices []string) {
	s.choices = append(s.choices[:0], choices...)
	s.awaitingChoice = len(s.choices) > 0
}

func (s *GameState) SetSelectItems(items []SelectItem) {
	s.selectItems = append(s.selectItems[:0], items...)
	s.selectHover = -1
	s.selectRevision++
}

// SelectHover 返回当前悬停的选项下标（-1 表示无）。
func (s *GameState) SelectHover() int {
	return s.selectHover
}

// SetSelectHover 更新悬停项；只有变化时才递增 revision。
func (s *GameState) SetSelectHover(index int) {
	if index < 0 || index >= len(s.selectItems) {
		index = -1
	}
	if s.selectHover == index {
		return
	}
	s.selectHover = index
	s.selectRevision++
}

// SelectRevision 用于渲染器判断选项区是否需要重画。
func (s *GameState) SelectRevision() int {
	return s.selectRevision
}

func (s *GameState) ClearChoices() {
	s.choices = s.choices[:0]
	s.selectItems = s.selectItems[:0]
	s.selectHover = -1
	s.selectRevision++
	s.awaitingChoice = false
}

func (s *GameState) Reset() {
	s.speaker = ""
	s.text = ""
	s.multiSpeaker = ""
	s.multiText = ""
	s.fillAlpha = 0
	s.awaitingInput = false
	s.messageWindowVisible = true
	s.ClearChoices()
	s.ClearStage()
	s.EndSelect()
	s.EndTransition()
}

func (s *GameState) ResetForRestart() {
	s.Reset()
	s.ClearBgm()
	s.bgmVolume = 0.8
	s.bgmLoop = true
	s.ClearSoundEffect()
	s.ClearVoice()
	s.voiceSerial++
	s.effectSerial++
	s.ClearVoiceSequence()
	s.ClearMovie()
	s.ClearMaskImage()
	s.textSpeed = 1.0
	s.waitMilliseconds = 0
	s.counterValue = -1
	s.fillRed = 0
	s.fillGreen = 0
	s.fillBlue = 0
}

func (s *GameState) countKind(kind StageLayerKind) int {
	count := 0
	for _, layer := range s.layers {
		if layer.Kind() == kind && len(layer.Artworks()) > 0 {
			count++
		}
	}
	return count
}
