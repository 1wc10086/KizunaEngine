package session

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// systemui_config.go 是原版扩展设置画面（define.bsa 的 system_config_ex.layout）。
//
// 原引擎把设置画面也做成资源驱动：底图、分类页签、按钮/仪表的状态帧、说明图
// 全在 layout 里，代码只按功能组 ID 决定"这个按钮改哪个设置"。这里沿用同一份
// layout，而不是在 Flutter 里重画一套表单，保证视觉和原版一致；音量也由引擎
// 直接乘进 AudioEvent，宿主不再持有独立音量。
//
// 布局里的功能组很多是作品特有的（换装/演出/角色音量）。我们保留所有控件的
// 绘制，只对当前作品能落地的组做动作；未实现的组点击后不改变设置，玩家不会
// 因为按了一个没接好的按钮而把配置改坏。

type configControl struct {
	// group/state 是 layout 里的功能组和状态名；state 可能是 "on"/"off"、
	// 序号、或 "toggle"/"change"（角色音量）。
	group string
	state string
	// button 是可直接绘制的按钮图；meter 为 true 时它是仪表底图。
	button config.ConfigButton
	meter  bool
	// plate 表示这只是底图（不参与命中）：例如"当前角色"的脸孔底图
	// cfgx_face_base，它只是给角色头像当背景，不能抢走叠在它上面的
	// "试听"按钮的点击。
	plate bool
	// hitX/Y/W/H 是该控件的命中区（按钮按图片真实尺寸，仪表按 hit_region）。
	hitX int
	hitY int
	hitW int
	hitH int
	// maxValue 是仪表的刻度上限：音量 1000、速度 100、底色 255。
	maxValue int
	// description 是悬停说明；空值表示没有。
	description config.ConfigDescription
}

// actorSample 是一个角色在设置画面里的样本资料。
//
// Voice/BGV 是"试听"用的文件（形如 01_01998_001），Thumb 是选中该角色时
// 叠在脸孔底图上的头像图名。三者都来自 system_config_ex.layout 的
// "actor setting" 数组，作品之间可以完全不同，所以运行时读而不是写死。
type actorSample struct {
	Voice string
	BGV   string
	Thumb string
}

// openConfigScreen 打开原版设置画面；作品没有 layout 时返回 false，
// 调用方会退回宿主的设置页。
func (s *Session) openConfigScreen() bool {
	if s.configScreen == nil || len(s.configScreen.Categories) == 0 {
		// define.bsa 没有 system_config_ex.layout 的作品（牝教師3、
		// 特別授業3SLG 等）走 bsxx.cfg 的旧式 [config screen]。
		return s.openLegacyConfigScreen()
	}
	if s.gameState == nil || s.composer == nil || !s.canOpenSystemUI() {
		return false
	}
	s.beginSystemUI(uiConfig)
	s.ui.configPage = 0
	s.buildConfigControls()
	return true
}

// buildConfigControls 根据当前分类重建控件。
//
// 绘制与命中共用这份切片：如果各画各的，按钮图有多帧、仪表 hit_region 与
// 底图不一致时，热区会和看到的画面错位。
func (s *Session) buildConfigControls() {
	ui := s.ui
	if ui == nil || s.configScreen == nil {
		return
	}
	ui.configControls = nil
	if ui.configPage < 0 || ui.configPage >= len(s.configScreen.Categories) {
		ui.configPage = 0
	}
	page, ok := s.configScreen.Pages[s.configScreen.Categories[ui.configPage].Layout]
	if !ok {
		return
	}
	keys := sortedConfigKeys(page.Functions)
	for _, group := range keys {
		s.addConfigGroup(ui, group, page.Functions[group])
	}
	// 仪表先画，按钮后画：底图/刻度和 min/max 按钮可能重叠，后画的按钮在上层。
	sort.SliceStable(ui.configControls, func(a, b int) bool {
		left, right := ui.configControls[a], ui.configControls[b]
		if left.meter != right.meter {
			return left.meter
		}
		if left.hitY != right.hitY {
			return left.hitY < right.hitY
		}
		return left.hitX < right.hitX
	})
}

func sortedConfigKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------- 角色个别音量

// loadActorSettings 解析设置画面 "actor setting" 数组，取出每个角色的
// 样本语音、样本 BGV 和头像图名。
//
// 这些资料来自作品自己的 layout（角色数量、样本文件名、头像图名都可能不同），
// 所以只能运行时读。解析一次就缓存：设置画面每帧都要用到它们，
// 而布局树在整局游戏里不会变。
func (s *Session) loadActorSettings() {
	if s.actorsLoaded {
		return
	}
	s.actorsLoaded = true
	if s.configScreen == nil {
		return
	}
	page, ok := s.configScreen.Pages["sound"]
	if !ok {
		return
	}
	array, ok := config.Array(page.Functions["actor setting"])
	if !ok {
		return
	}
	for _, item := range array {
		object, ok := config.Object(item)
		if !ok {
			continue
		}
		change, ok := config.Object(object["change"])
		if !ok {
			continue
		}
		data, ok := config.Object(change["data"])
		if !ok {
			continue
		}
		sample := actorSample{}
		if values, ok := config.Object(data["sample"]); ok {
			sample.Voice, _ = config.Text(values["voice"])
			sample.BGV, _ = config.Text(values["bgv"])
		}
		if image, ok := config.Object(data["image"]); ok {
			if thumbnail, ok := config.Object(image["thumbnail"]); ok {
				sample.Thumb, _ = config.Text(thumbnail["name"])
			}
		}
		s.actorSamples = append(s.actorSamples, sample)
	}
	if len(s.actorSamples) > 0 {
		core.Logf("settings: actor samples loaded=%d", len(s.actorSamples))
	}
}

// selectedActorIndex 返回当前选中的角色下标。
//
// 设置文件里存的下标可能越界（旧存档、作品角色更少），所以每次用之前
// 都要夹一次；夹完再拿去索引 CharacterVoice/CharacterBgv。
func (s *Session) selectedActorIndex() int {
	s.loadActorSettings()
	count := len(s.settings.CharacterVoice)
	if len(s.actorSamples) > 0 && len(s.actorSamples) < count {
		count = len(s.actorSamples)
	}
	return clampActorIndex(s.settings.SelectedActor, count)
}

// clampActorIndex 把角色下标夹进 [0, count-1]。
func clampActorIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index > count-1 {
		return count - 1
	}
	return index
}

// actorVolumeScale 把设置里的 0..1 音量系数换算成仪表刻度（0..1000）。
//
// 用 math.Round 而不是直接截断：0.29 在二进制浮点里是 0.28999999999999998，
// 截断成整数会变成 289，仪表上的刻度就会差一格。
func actorVolumeScale(volume float64) int {
	return int(math.Round(clampVolumeSetting(volume) * 1000))
}

// actorVolumeValue 把仪表刻度换算回 0..1 的音量系数。
func actorVolumeValue(scale int) float64 {
	return clampVolumeSetting(float64(scale) / 1000)
}

// playActorSample 试听当前角色的样本语音或样本 BGV。
//
// 样本用和正式播放一样的音量链路：通道音量 × 个别音量系数。
// 这样玩家在设置画面里听到的就是剧情里的实际响度。
func (s *Session) playActorSample(voice bool) {
	s.loadActorSettings()
	index := s.selectedActorIndex()
	if index < 0 || index >= len(s.actorSamples) {
		return
	}
	sample := s.actorSamples[index]
	name := sample.BGV
	kind := kindBgv
	if voice {
		name = sample.Voice
		kind = kindVoice
	}
	if name == "" {
		return
	}
	s.audio = append(s.audio, core.AudioEvent{
		Kind:    2,
		Path:    s.materialize(name),
		Volume:  s.volumeGain(kind) * s.characterGain(name, voice),
		Channel: noTarget,
		Group:   noTarget,
	})
}

func (s *Session) addConfigGroup(ui *systemUI, group string, value any) {
	object, ok := config.Object(value)
	if !ok {
		if array, ok := config.Array(value); ok {
			for index, item := range array {
				s.addConfigIndexed(ui, group, index, item)
			}
		}
		return
	}
	// 直接按钮。
	if button, ok := config.ParseButton(object); ok {
		s.addConfigControl(ui, group, "", button, object, false)
		return
	}
	// 仪表组：{master:{bar,min,max}, bgm:{...}}。
	if hasConfigKey(object, "bar") || hasConfigKey(object, "min") || hasConfigKey(object, "max") {
		for _, state := range []string{"bar", "min", "max"} {
			if child, ok := config.Object(object[state]); ok {
				s.addConfigState(ui, group, state, child)
			}
		}
		return
	}
	// 状态组：{window:{...}, full:{...}} / {on:{...}, off:{...}}。
	for _, state := range sortedConfigKeys(object) {
		s.addConfigState(ui, group, state, object[state])
	}
}

func hasConfigKey(object map[string]any, key string) bool {
	_, ok := object[key]
	return ok
}

func (s *Session) addConfigState(ui *systemUI, group, state string, value any) {
	object, ok := config.Object(value)
	if !ok {
		if array, ok := config.Array(value); ok {
			for index, item := range array {
				s.addConfigIndexed(ui, group+":"+state, index, item)
			}
		}
		return
	}
	if button, ok := config.ParseButton(object); ok {
		// 仪表的判定看状态名的最后一段：通道音量是 "bar"，而嵌套的
		// "当前角色/个别音量"、"bg color/红绿蓝" 都是 "xxx/bar"。
		// 认成仪表才能用 layout 给的 hit_region 命中，也才会画出当前刻度。
		s.addConfigControl(ui, group, state, button, object, configStateLeaf(state) == "bar")
		return
	}
	if hasConfigKey(object, "bar") || hasConfigKey(object, "min") || hasConfigKey(object, "max") {
		subGroup := group
		subPrefix := state
		// volume 的下一层是通道名（master/bgm/se/...），把它提升为功能组，
		// 这样 master/bar 会变成 group=master、state=bar，音量的读取/写入
		// 才能按通道分派。bg color 的 red/green 是同一组的不同刻度，
		// 不能提升。
		if group == "volume" && !strings.Contains(state, "/") {
			subGroup = state
			subPrefix = ""
		}
		for _, sub := range []string{"bar", "min", "max"} {
			if child, ok := config.Object(object[sub]); ok {
				s.addConfigState(ui, subGroup, joinConfigState(subPrefix, sub), child)
			}
		}
		// 仪表组里还可能有独立按钮（例如"当前角色"的样本试听 play）。
		// 它们不是刻度，但同样是能点的控件，不能被上面的固定列表丢掉。
		for _, sub := range sortedConfigKeys(object) {
			if sub == "bar" || sub == "min" || sub == "max" {
				continue
			}
			child, ok := config.Object(object[sub])
			if !ok {
				continue
			}
			if button, ok := config.ParseButton(child); ok {
				s.addConfigControl(ui, subGroup, joinConfigState(subPrefix, sub), button, child, false)
			}
		}
		return
	}
	for _, sub := range sortedConfigKeys(object) {
		s.addConfigState(ui, group, joinConfigState(state, sub), object[sub])
	}
}

func joinConfigState(prefix, state string) string {
	if prefix == "" {
		return state
	}
	return prefix + "/" + state
}

func (s *Session) addConfigIndexed(ui *systemUI, group string, index int, value any) {
	object, ok := config.Object(value)
	if !ok {
		return
	}
	prefix := group + ":" + strconv.Itoa(index)
	if toggle, ok := config.Object(object["toggle"]); ok {
		s.addConfigState(ui, prefix, "toggle", toggle)
	}
	if change, ok := config.Object(object["change"]); ok {
		s.addConfigState(ui, prefix, "change", change)
	}
	if _, hasToggle := object["toggle"]; hasToggle {
		return
	}
	// layer action 这类数组元素是 {on:{...}, off:{...}}。
	for _, state := range []string{"on", "off"} {
		if child, ok := config.Object(object[state]); ok {
			s.addConfigState(ui, prefix, state, child)
		}
	}
	if button, ok := config.ParseButton(object); ok {
		s.addConfigControl(ui, group, strconv.Itoa(index), button, object, false)
	}
}

func (s *Session) addConfigControl(ui *systemUI, group, state string, button config.ConfigButton, object map[string]any, meter bool) {
	control := configControl{
		group:    group,
		state:    state,
		button:   button,
		meter:    meter,
		maxValue: s.configControlMax(group, state),
		// "当前角色"的脸孔底图只是背景板：角色头像和试听按钮都叠在它上面，
		// 它自己不响应点击。
		plate: group == "current actor" && state == "thumbnail",
	}
	control.description = parseConfigDescription(object["description"])

	image := s.composer.Load(button.Name)
	width, height := 0, 0
	if image != nil {
		width = image.Width
		frames := max(1, button.MaxState)
		height = image.Height / frames
	}
	if meter {
		if region, ok := config.Array(object["hit_region"]); ok && len(region) >= 4 {
			control.hitX, _ = config.Number(region[0])
			control.hitY, _ = config.Number(region[1])
			control.hitW, _ = config.Number(region[2])
			control.hitH, _ = config.Number(region[3])
		} else {
			control.hitX, control.hitY = button.X, button.Y
			control.hitW, control.hitH = width, height
		}
	} else {
		control.hitX, control.hitY = button.X, button.Y
		control.hitW, control.hitH = width, height
	}
	if control.hitW <= 0 || control.hitH <= 0 {
		return
	}
	ui.configControls = append(ui.configControls, control)
}

func (s *Session) configControlMax(group, state string) int {
	switch group {
	case "master", "bgm", "se", "bgs", "voice", "bgv", "bgm attenuation rate":
		return 1000
	case "message speed", "click wait time":
		return 100
	case "bg color":
		switch configBaseState(state) {
		case "red", "green", "blue", "alpha":
			return 255
		}
	case "current actor":
		// 个别音声音量的刻度和通道音量一样是 0..1000。
		return 1000
	}
	return 100
}

// configBaseState 取出 "red/bar" 这类复合状态的第一段。
// 仪表子状态带父级名（bg color 的 red/green/blue/alpha），
// 按钮状态则可能是 "on"/"off"/序号。
func configBaseState(state string) string {
	if index := strings.IndexByte(state, '/'); index >= 0 {
		return state[:index]
	}
	return state
}

// configStateLeaf 取出复合状态的最后一段："voice/bar" -> "bar"。
// 和 configBaseState 相反：判断"这是不是刻度条"要看的是末段。
func configStateLeaf(state string) string {
	if index := strings.LastIndexByte(state, '/'); index >= 0 {
		return state[index+1:]
	}
	return state
}

func parseConfigDescription(value any) config.ConfigDescription {
	object, ok := config.Object(value)
	if !ok {
		return config.ConfigDescription{}
	}
	description := config.ConfigDescription{}
	if text, ok := config.Text(object["text"]); ok {
		description.Text = text
	}
	if image, ok := config.Text(object["image"]); ok {
		description.Image = image
		description.HasImage = len(image) > 0
	}
	if pos, ok := config.Array(object["pos"]); ok {
		description.X, description.Y = configPositions(pos)
	}
	return description
}

func configPositions(values []any) (int, int) {
	if len(values) < 2 {
		return 0, 0
	}
	x, _ := config.Number(values[0])
	y, _ := config.Number(values[1])
	return x, y
}

// ---------------------------------------------------------------- 绘制

func (s *Session) drawConfigScreen(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || s.configScreen == nil {
		return
	}
	screen := s.configScreen
	s.drawImageAt(canvas, screen.Base, 0, 0)
	if ui.configPage >= 0 && ui.configPage < len(screen.Categories) {
		page := screen.Categories[ui.configPage].Layout
		if entry, ok := screen.Pages[page]; ok {
			s.drawImageAt(canvas, entry.Base, 0, 0)
		}
	}

	for index, control := range ui.configControls {
		hover := ui.hover.kind == hitConfigControl && ui.hover.index == index
		if control.meter {
			s.drawConfigMeter(canvas, control, hover)
			continue
		}
		frame := 1
		if s.configControlSelected(control) {
			frame = 3
		}
		if hover {
			frame = 4
		}
		s.drawConfigFrame(canvas, control.button, frame)
	}

	// 当前角色的头像：脸孔底图 cfgx_face_base 已经作为控件画好了，
	// 选中角色的头像要叠在它上面（原引擎就是这么叠的）。
	if s.configPageLayout() == "sound" {
		s.drawConfigActorThumbnail(canvas)
	}

	for index, category := range screen.Categories {
		frame := 1
		if ui.configPage == index {
			frame = 3
		}
		if ui.hover.kind == hitConfigCategory && ui.hover.index == index {
			frame = 2
		}
		s.drawConfigFrame(canvas, category.Button, frame)
	}
	for index, common := range screen.Common {
		frame := 1
		if ui.hover.kind == hitConfigCommon && ui.hover.index == index {
			frame = 2
		}
		s.drawConfigFrame(canvas, common.Button, frame)
	}

	// 悬停说明：优先画说明图（原版大部分控件都给了图），没有图时留给宿主/
	// 后续文字渲染处理。说明图不参与命中。
	if description, ok := s.configHoverDescription(); ok && description.HasImage {
		s.drawImageAt(canvas, description.Image, description.X, description.Y)
	}
}

// configPageLayout 返回当前分类页的 layout 名（例如 "sound"）。
func (s *Session) configPageLayout() string {
	ui := s.ui
	if ui == nil || s.configScreen == nil {
		return ""
	}
	if ui.configPage < 0 || ui.configPage >= len(s.configScreen.Categories) {
		return ""
	}
	return s.configScreen.Categories[ui.configPage].Layout
}

// drawConfigActorThumbnail 把当前选中角色的头像画在脸孔底图上。
//
// 位置取自 layout 里 "current actor"/"thumbnail" 的 pos，
// 不写死坐标：作品可以换底图位置。
func (s *Session) drawConfigActorThumbnail(canvas *gfx.BgraCanvas) {
	s.loadActorSettings()
	index := s.selectedActorIndex()
	if index < 0 || index >= len(s.actorSamples) {
		return
	}
	thumb := s.actorSamples[index].Thumb
	if thumb == "" {
		return
	}
	x, y := s.configActorThumbnailPos()
	s.drawImageAt(canvas, thumb, x, y)
}

// configActorThumbnailPos 找出脸孔底图的位置，作为角色头像的落点。
//
// 位置直接取自 layout（"current actor"/"thumbnail"/pos），不写死坐标：
// 作品可以换底图位置，而且底图没加载出来时也不该把头像画到别处。
func (s *Session) configActorThumbnailPos() (int, int) {
	if s.configScreen == nil {
		return 0, 0
	}
	page, ok := s.configScreen.Pages["sound"]
	if !ok {
		return 0, 0
	}
	current, ok := config.Object(page.Functions["current actor"])
	if !ok {
		return 0, 0
	}
	thumbnail, ok := config.Object(current["thumbnail"])
	if !ok {
		return 0, 0
	}
	if pos, ok := config.Array(thumbnail["pos"]); ok && len(pos) >= 2 {
		x, _ := config.Number(pos[0])
		y, _ := config.Number(pos[1])
		return x, y
	}
	return 0, 0
}

func (s *Session) configHoverDescription() (config.ConfigDescription, bool) {
	ui := s.ui
	if ui == nil || s.configScreen == nil {
		return config.ConfigDescription{}, false
	}
	switch ui.hover.kind {
	case hitConfigCategory:
		if ui.hover.index >= 0 && ui.hover.index < len(s.configScreen.Categories) {
			return s.configScreen.Categories[ui.hover.index].Description, true
		}
	case hitConfigCommon:
		if ui.hover.index >= 0 && ui.hover.index < len(s.configScreen.Common) {
			return s.configScreen.Common[ui.hover.index].Description, true
		}
	case hitConfigControl:
		if ui.hover.index >= 0 && ui.hover.index < len(ui.configControls) {
			return ui.configControls[ui.hover.index].description, true
		}
	}
	return config.ConfigDescription{}, false
}

func (s *Session) drawConfigFrame(canvas *gfx.BgraCanvas, button config.ConfigButton, frame int) {
	if button.Name == "" {
		return
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return
	}
	frames := max(1, button.MaxState)
	frame = clampInt(frame, 0, frames-1)
	height := image.Height / frames
	if height <= 0 {
		return
	}
	canvas.DrawRegion(image, 0, frame*height, image.Width, height, button.X, button.Y)
}

func (s *Session) drawConfigMeter(canvas *gfx.BgraCanvas, control configControl, hover bool) {
	image := s.composer.Load(control.button.Name)
	if image == nil || control.meter == false {
		return
	}
	canvas.Draw(image, control.button.X, control.button.Y)
	value := s.configControlValue(control)
	if control.maxValue <= 0 {
		return
	}
	fraction := float64(value) / float64(control.maxValue)
	fraction = max(0, min(1, fraction))
	x := control.hitX + int(float64(control.hitW-3)*fraction)
	// 用一条 3px 的白色标记表示当前刻度：原版仪表底图本身没有指针，
	// 玩家需要看到自己拖到了哪里；不重画整条底图，悬停/改值都很便宜。
	alpha := byte(200)
	if hover {
		alpha = 255
	}
	canvas.Fill(x, control.hitY, 3, control.hitH, 255, 255, 255, alpha)
}

func (s *Session) configControlValue(control configControl) int {
	switch control.group {
	case "master":
		return int(s.settings.MasterVolume * 1000)
	case "bgm":
		return int(s.settings.BgmVolume * 1000)
	case "se":
		return int(s.settings.SeVolume * 1000)
	case "bgs":
		return int(s.settings.BgsVolume * 1000)
	case "voice":
		return int(s.settings.VoiceVolume * 1000)
	case "bgv":
		return int(s.settings.BgvVolume * 1000)
	case "bgm attenuation rate":
		return int(s.settings.BgmAttenuation * 1000)
	case "message speed":
		return s.settings.MessageSpeed
	case "click wait time":
		return s.settings.ClickWait
	case "current actor":
		// 个别音量：读的是"当前选中角色"自己的系数。
		// 系数存在设置里，播放时由引擎乘进 AudioEvent 的音量（见 audio_volume.go）。
		switch {
		case strings.HasPrefix(control.state, "voice/"):
			return actorVolumeScale(s.settings.CharacterVoice[s.selectedActorIndex()])
		case strings.HasPrefix(control.state, "bgv/"):
			return actorVolumeScale(s.settings.CharacterBgv[s.selectedActorIndex()])
		}
	case "bg color":
		switch configBaseState(control.state) {
		case "red":
			return s.settings.BgRed
		case "green":
			return s.settings.BgGreen
		case "blue":
			return s.settings.BgBlue
		case "alpha":
			return s.settings.BgAlpha
		}
	}
	return 0
}

func (s *Session) setConfigControlValue(control configControl, value int) {
	value = clampInt(value, 0, control.maxValue)
	switch control.group {
	case "master":
		s.settings.MasterVolume = clampVolumeSetting(float64(value) / 1000)
	case "bgm":
		s.settings.BgmVolume = clampVolumeSetting(float64(value) / 1000)
	case "se":
		s.settings.SeVolume = clampVolumeSetting(float64(value) / 1000)
	case "bgs":
		s.settings.BgsVolume = clampVolumeSetting(float64(value) / 1000)
	case "voice":
		s.settings.VoiceVolume = clampVolumeSetting(float64(value) / 1000)
	case "bgv":
		s.settings.BgvVolume = clampVolumeSetting(float64(value) / 1000)
	case "bgm attenuation rate":
		s.settings.BgmAttenuation = clampVolumeSetting(float64(value) / 1000)
	case "message speed":
		s.settings.MessageSpeed = value
		s.applyDisplaySettings()
	case "click wait time":
		s.settings.ClickWait = value
	case "current actor":
		// 只写"当前选中角色"的那一条；别的角色的音量不受影响。
		// bar/min/max 都落到同一个音量上：min/max 按钮只是把刻度设成 0/1000。
		index := s.selectedActorIndex()
		switch {
		case strings.HasPrefix(control.state, "voice/"):
			s.settings.CharacterVoice[index] = actorVolumeValue(value)
		case strings.HasPrefix(control.state, "bgv/"):
			s.settings.CharacterBgv[index] = actorVolumeValue(value)
		}
	case "bg color":
		switch configBaseState(control.state) {
		case "red":
			s.settings.BgRed = value
		case "green":
			s.settings.BgGreen = value
		case "blue":
			s.settings.BgBlue = value
		case "alpha":
			s.settings.BgAlpha = value
		}
		// 拖动底色滑杆时立刻重画对话框；渲染器内部只用最终 ARGB 做缓存键，
		// 不会每帧重排文字。
		s.applyDisplaySettings()
	}
}

// configControlSelected 判断按钮当前是否处于"选中/ON"状态。
func (s *Session) configControlSelected(control configControl) bool {
	state := control.state
	group := control.group
	switch {
	case group == "screen mode":
		return strings.EqualFold(state, s.settings.ScreenMode)
	case group == "under hair":
		return (state == "on") == s.settings.UnderHair
	case group == "all effect":
		return (state == "on") == s.settings.AllEffect
	case group == "wipe":
		return (state == "on") == s.settings.Wipe
	case group == "cutin":
		return (state == "on") == s.settings.Cutin
	case group == "blink":
		return (state == "on") == s.settings.Blink
	case group == "x countdown":
		return (state == "on") == s.settings.XCountdown
	case group == "steam":
		return (state == "on") == s.settings.Steam
	case group == "cease skip":
		return (state == "on") == s.settings.CeaseSkip
	case group == "skip unread":
		return (state == "on") == s.settings.SkipUnread
	case group == "bgm selector":
		index, err := strconv.Atoi(state)
		return err == nil && index == s.settings.BgmSelect
	case group == "voice mode":
		index, err := strconv.Atoi(state)
		return err == nil && index == s.settings.VoiceMode
	case strings.HasPrefix(group, "actor setting:"):
		character := configCharacterIndex(group)
		if character < 0 || character >= len(s.settings.CharacterVoiceOn) {
			return false
		}
		if state == "change" {
			// 角色切换按钮：当前选中的那个高亮。
			return character == s.selectedActorIndex()
		}
		return state == "toggle" && s.settings.CharacterVoiceOn[character]
	}
	return false
}

func configCharacterIndex(group string) int {
	index := strings.LastIndexByte(group, ':')
	if index < 0 {
		return -1
	}
	character, err := strconv.Atoi(group[index+1:])
	if err != nil {
		return -1
	}
	return character
}

// ---------------------------------------------------------------- 命中

func (s *Session) hitConfigScreen(x, y int) uiHit {
	ui := s.ui
	if ui == nil || s.configScreen == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	for index, category := range s.configScreen.Categories {
		if hitConfigButton(s, category.Button, x, y) {
			return uiHit{kind: hitConfigCategory, index: index}
		}
	}
	for index, common := range s.configScreen.Common {
		if hitConfigButton(s, common.Button, x, y) {
			return uiHit{kind: hitConfigCommon, index: index}
		}
	}
	for index, control := range ui.configControls {
		// 底图（脸孔底板之类）不响应点击：角色头像和试听按钮都叠在它上面，
		// 让它参与命中会把那些按钮的点击吃掉。
		if control.plate {
			continue
		}
		if x >= control.hitX && y >= control.hitY && x < control.hitX+control.hitW && y < control.hitY+control.hitH {
			return uiHit{kind: hitConfigControl, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func hitConfigButton(s *Session, button config.ConfigButton, x, y int) bool {
	if button.Name == "" {
		return false
	}
	image := s.composer.Load(button.Name)
	if image == nil {
		return false
	}
	frames := max(1, button.MaxState)
	height := image.Height / frames
	return x >= button.X && y >= button.Y && x < button.X+image.Width && y < button.Y+height
}

// ---------------------------------------------------------------- 交互

func (s *Session) activateConfigScreen(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil || s.configScreen == nil {
		return
	}
	switch hit.kind {
	case hitConfigCategory:
		if hit.index >= 0 && hit.index < len(s.configScreen.Categories) {
			ui.configPage = hit.index
			s.buildConfigControls()
			s.composer.Invalidate()
		}
	case hitConfigCommon:
		if hit.index >= 0 && hit.index < len(s.configScreen.Common) {
			s.activateConfigCommon(s.configScreen.Common[hit.index].No)
		}
	case hitConfigControl:
		if hit.index >= 0 && hit.index < len(ui.configControls) {
			s.activateConfigControl(ui.configControls[hit.index], x, y)
		}
	}
}

func (s *Session) activateConfigCommon(no int) {
	switch no {
	case 124: // 初期化
		s.resetGameSettings()
		s.composer.Invalidate()
	case 125: // 戻る：回到打开设置前的画面（标题或剧情）
		s.closeSystemUI()
		// 从 CG 查看器的 CFG 进来的，要回到同一张差分；普通设置没有快照。
		s.resumeCGViewerIfNeeded()
	case 126: // タイトルへ
		s.cgResume = nil
		s.closeSystemUI()
		s.enterTitle()
	case 127: // 終了
		s.cgResume = nil
		s.setHostRequest("exit")
		s.closeSystemUI()
	}
}

func (s *Session) activateConfigControl(control configControl, x, y int) {
	_ = y
	// 仪表：按点击位置换算刻度。
	if control.meter {
		s.setConfigControlMeter(control, x)
		return
	}
	// min/max 是按钮状态，state 里带 "/min"、"/max"。
	switch {
	case strings.HasSuffix(control.state, "/min") || control.state == "min":
		s.setConfigControlValue(configControl{group: control.group, state: control.state, maxValue: control.maxValue}, 0)
		s.finishConfigChange()
		return
	case strings.HasSuffix(control.state, "/max") || control.state == "max":
		s.setConfigControlValue(configControl{group: control.group, state: control.state, maxValue: control.maxValue}, control.maxValue)
		s.finishConfigChange()
		return
	}

	group := control.group
	state := control.state
	// 样本试听：只播一次，不改任何设置，所以不走 finishConfigChange。
	if group == "current actor" {
		switch state {
		case "voice/play":
			s.playActorSample(true)
			return
		case "bgv/play":
			s.playActorSample(false)
			return
		}
	}
	switch {
	case group == "screen mode":
		s.settings.ScreenMode = state
	case group == "under hair":
		s.settings.UnderHair = state == "on"
	case group == "all effect":
		s.settings.AllEffect = state == "on"
	case group == "wipe":
		s.settings.Wipe = state == "on"
	case group == "cutin":
		s.settings.Cutin = state == "on"
	case group == "blink":
		s.settings.Blink = state == "on"
	case group == "x countdown":
		s.settings.XCountdown = state == "on"
		s.applyDisplaySettings()
	case group == "steam":
		s.settings.Steam = state == "on"
	case group == "cease skip":
		s.settings.CeaseSkip = state == "on"
	case group == "skip unread":
		s.settings.SkipUnread = state == "on"
	case group == "bgm selector":
		index, err := strconv.Atoi(state)
		if err == nil {
			s.settings.BgmSelect = clampSetting(index, 0, 1)
		}
	case group == "voice mode":
		index, err := strconv.Atoi(state)
		if err == nil {
			s.settings.VoiceMode = clampSetting(index, 0, 3)
		}
	case strings.HasPrefix(group, "actor setting:") && state == "toggle":
		character := configCharacterIndex(group)
		if character >= 0 && character < len(s.settings.CharacterVoiceOn) {
			s.settings.CharacterVoiceOn[character] = !s.settings.CharacterVoiceOn[character]
		}
	case strings.HasPrefix(group, "actor setting:") && state == "change":
		// 切换"当前角色"：个别音量那几行、试听按钮和头像都跟着换成这个角色。
		character := configCharacterIndex(group)
		if character < 0 {
			return
		}
		s.settings.SelectedActor = clampActorIndex(character, len(s.settings.CharacterVoice))
	default:
		return
	}
	s.finishConfigChange()
}

func (s *Session) finishConfigChange() {
	s.saveGameSettings()
	s.refreshAudioVolumes()
	s.composer.Invalidate()
	core.Logf("settings: updated")
}

// setConfigControlMeter 由 pointerSystemUI 在点击/拖动仪表时调用。
func (s *Session) setConfigControlMeter(control configControl, x int) {
	if control.hitW <= 0 {
		return
	}
	fraction := float64(x-control.hitX) / float64(control.hitW)
	fraction = max(0, min(1, fraction))
	value := int(float64(control.maxValue) * fraction)
	s.setConfigControlValue(control, value)
	s.finishConfigChange()
}
