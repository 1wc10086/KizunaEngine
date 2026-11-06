package gothic

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
)

func (s *Session) dispatchAction(action *vmgothic.GothicAction) core.Step {
	if action == nil {
		s.flushWaveQueue()
		return core.StepContinue
	}
	switch action.Kind {
	case vmgothic.GothicActionText:
		return s.appendText(action.Text)
	case vmgothic.GothicActionPageBreak:
		if action.Value == 2 {
			return s.pageWait()
		}
		return s.segmentBreak()
	case vmgothic.GothicActionPause, vmgothic.GothicActionWait:
		s.gameState.SetWaitMilliseconds(int(action.Value))
		return core.StepWait
	case vmgothic.GothicActionMenu:
		return s.showMenu(action.Menu)
	case vmgothic.GothicActionWaveOpen:
		return s.applyWaveOpen(action.Statement)
	case vmgothic.GothicActionWaveWait:
		return s.applyWaveWait()
	case vmgothic.GothicActionWavePlay, vmgothic.GothicActionWaveChain:
		return s.applyWaveSound(action.Value)
	case vmgothic.GothicActionTilt:
		return s.applyTilt()
	case vmgothic.GothicActionAniToggle:
		return s.applyAni(action.Value)
	case vmgothic.GothicActionRmap:
		return s.applyRmap(action.Statement)
	case vmgothic.GothicActionSmap:
		return s.applySmap(action.Statement)
	case vmgothic.GothicActionDmap:
		return s.applyDmap(action)
	case vmgothic.GothicActionSetControl:
		return s.applySetControl(action.Statement)
	case vmgothic.GothicActionInputName:
		return s.applyInputName()
	case vmgothic.GothicActionCommand:
		s.flushWaveQueue()
		return s.dispatchCommand(action.Statement)
	case vmgothic.GothicActionFinish:
		s.flushWaveQueue()
		if s.profile == gothicProfileMaze {
			// 原版结局/Game Over 后回到标题，而不是把宿主停在空白结束页。
			// 新建 VM，下一次「初めから」从 boot 重新初始化变量；标志让
			// boot 不再闪 logo、也不再停在 startmenu。
			s.machine = vmgothic.NewGothicMachine(s.program)
			s.clearTransientState()
			s.skipStartMenu = true
			s.suppressLogo = true
			return s.showTitle()
		}
		return core.StepFinished
	case vmgothic.GothicActionEmpty:
		s.flushWaveQueue()
		return core.StepContinue
	default:
		s.flushWaveQueue()
		return core.StepContinue
	}
}

func (s *Session) dispatchCommand(statement *vmgothic.GothicStatement) core.Step {
	if statement == nil {
		return core.StepContinue
	}
	return s.dispatchCommandCore(statement)
}

func (s *Session) dispatchCommandCore(statement *vmgothic.GothicStatement) core.Step {
	name := strings.ToLower(statement.Name)
	sub := strings.ToLower(statement.Sub)
	args := statement.Args
	if args == nil {
		args = []string{}
	}
	switch name {
	case "g", "graphic":
		return s.applyGraphics(sub, args)
	case "music":
		return s.applyMusic(sub, args)
	case "t", "text":
		return s.applyTextCommand(sub, args)
	case "game":
		return s.applyGame(sub)
	case "cls":
		return s.applyCls()
	case "logo":
		return s.applyLogo()
	case "startmenu":
		return s.applyStartMenu()
	case "staffroll":
		return s.applyStaffRoll()
	case "restart":
		return s.applyRestart()
	case "init", "pause":
		return core.StepContinue
	default:
		core.Logf("gothic: command '%s' ignored at %d", name, statement.Index)
		return core.StepContinue
	}
}

func (s *Session) applyGraphics(sub string, args []string) core.Step {
	switch sub {
	case "draw":
		if len(args) >= 3 {
			layer := parseGothicInt(args[1])
			image := unquoteGothic(args[2])
			s.stage.LoadBase(layer, image)
			s.backgroundLayer = layer
			s.background = image
			s.markCG(image)
			core.Logf("gothic: draw %d '%s'", layer, image)
		}
		return core.StepContinue
	case "layer":
		return s.applyLayer(args)
	case "map1":
		return s.applyMapRegister(1, args)
	case "map2":
		return s.applyMap2(args)
	case "map3":
		return s.applyMap3(args)
	case "scroll":
		return s.applyScroll(args)
	case "cls":
		if len(args) >= 3 {
			layer := parseGothicInt(args[1])
			if layer == 3 {
				s.clearMessage()
			} else {
				blue, green, red := s.backgroundFill()
				s.stage.FillLayer(layer, blue, green, red)
			}
		}
		return core.StepContinue
	case "copy":
		if len(args) >= 3 {
			// MAZE 的写法是 `copy 0, 4`：第一个是目标层、第二个是源层。
			// 剧本先把背景/立绘画在 4 号工作层，再 copy 到 0 号显示层；
			// 反过来会把空白的 0 层盖到 4 层，背景整块消失。
			s.stage.CopyLayer(parseGothicInt(args[2]), parseGothicInt(args[1]))
		}
		return core.StepContinue
	case "effect":
		if len(args) >= 5 {
			source := parseGothicInt(args[1])
			target := parseGothicInt(args[2])
			effectType := parseGothicInt(args[3])
			parameter := parseGothicInt(args[4])
			if s.global != nil && !s.global.Effects {
				core.Logf("gothic: effect disabled by settings")
				return core.StepContinue
			}
			s.stage.BeginEffect(effectType, parameter, source, target)
			s.effectStamp = time.Now().UTC()
			core.Logf("gothic: effect %d,%d,%d,%d", source, target, effectType, parameter)
		}
		return core.StepContinue
	case "map":
		return s.applyClickMapCommand(args)
	case "page", "print", "printc", "copy2", "level":
		core.Logf("gothic: graphics '%s' ignored", sub)
		return core.StepContinue
	default:
		core.Logf("gothic: graphics '%s' unknown", sub)
		return core.StepContinue
	}
}

func (s *Session) applyLayer(args []string) core.Step {
	if len(args) < 2 {
		return core.StepContinue
	}
	if strings.EqualFold(args[1], "clear") {
		if len(args) >= 3 {
			s.stage.ClearPlanes(parseGothicInt(args[2]))
		}
		return core.StepContinue
	}
	if strings.EqualFold(args[1], "level") {
		core.Logf("gothic: graphics 'layer level' ignored")
		return core.StepContinue
	}
	if len(args) >= 4 {
		layer := parseGothicInt(args[1])
		plane := parseGothicInt(args[2])
		image := unquoteGothic(args[3])
		x := 0
		y := 0
		if len(args) > 4 {
			x = parseGothicInt(args[4])
		}
		if len(args) > 5 {
			y = parseGothicInt(args[5])
		}
		s.stage.SetPlane(layer, plane, image, x, y)
		s.markCG(image)
		core.Logf("gothic: layer %d,%d '%s' (%d,%d)", layer, plane, image, x, y)
	}
	return core.StepContinue
}

func (s *Session) applyMapRegister(kind int, args []string) core.Step {
	if len(args) < 3 {
		core.Logf("gothic: graphics 'map%d' malformed", kind)
		return core.StepContinue
	}
	image := unquoteGothic(args[1])
	variable := strings.TrimSpace(args[2])
	if kind == 1 {
		s.mapState.Map1Image = image
		s.mapState.Map1Variable = variable
	} else {
		s.mapState.Map2Image = image
		s.mapState.Map2Variable = variable
	}
	core.Logf("gothic: map%d '%s' %s", kind, image, variable)
	return core.StepContinue
}

// applyClickMapCommand 处理 MAZE 的 `g map "oem.gsa" mapvar`。
//
// 与 map2/map3 不同，这里的图片只是点击掩码：剧本先画 oen.gsa 当背景，
// 再让 oem 的四个纯色块做命中区。原引擎不会把 oem 画出来，否则白底会
// 盖住整张 staffroom 图。变量写入 1..4，对应 staff_1/2/3/end。
func (s *Session) applyClickMapCommand(args []string) core.Step {
	if len(args) < 3 || s.machine == nil {
		core.Logf("gothic: graphics 'map' malformed")
		return core.StepContinue
	}
	image := unquoteGothic(args[1])
	variable := strings.TrimSpace(args[2])
	if image == "" || variable == "" {
		return core.StepContinue
	}
	pixels := s.catalog.Resolve(image)
	if pixels == nil {
		core.Logf("gothic: map '%s' unresolved, continuing", image)
		return core.StepContinue
	}
	s.gameState.EndSelect()
	s.mapState.EnterClickMap(image, "", variable, pixels)
	s.mapColorIndex = true
	s.gameState.BeginSelect()
	core.Logf("gothic: click map '%s' -> %s", image, variable)
	return core.StepAwaitSelect
}

func (s *Session) applyMap2(args []string) core.Step {
	if len(args) < 2 {
		core.Logf("gothic: graphics 'map2' malformed")
		return core.StepContinue
	}
	image := unquoteGothic(args[1])
	variable := ""
	if len(args) > 2 {
		variable = strings.TrimSpace(args[2])
	}
	s.mapState.Map2Image = image
	s.mapState.Map2Variable = variable
	if len(image) == 0 || s.machine == nil {
		core.Logf("gothic: graphics 'map2' malformed")
		return core.StepContinue
	}
	pixels := s.catalog.Resolve(image)
	if pixels == nil {
		core.Logf("gothic: map2 '%s' unresolved, continuing", image)
		return core.StepContinue
	}
	s.stage.LoadBase(s.backgroundLayer, image)
	s.background = image
	s.gameState.EndSelect()
	s.mapState.EnterClickMap(image, "", "", pixels)
	s.map2Waiting = true
	s.gameState.BeginSelect()
	core.Logf("gothic: map2 '%s' awaiting click", image)
	return core.StepAwaitSelect
}

func (s *Session) applyMap3(args []string) core.Step {
	if len(args) < 4 {
		core.Logf("gothic: graphics 'map3' malformed")
		return core.StepContinue
	}
	image := unquoteGothic(args[1])
	timeVariable := strings.TrimSpace(args[2])
	mapVariable := strings.TrimSpace(args[3])
	if len(image) == 0 || len(timeVariable) == 0 || len(mapVariable) == 0 || s.machine == nil {
		core.Logf("gothic: graphics 'map3' malformed")
		return core.StepContinue
	}
	pixels := s.catalog.Resolve(image)
	if pixels == nil {
		core.Logf("gothic: map3 '%s' unresolved, continuing", image)
		return core.StepContinue
	}
	s.stage.LoadBase(s.backgroundLayer, image)
	s.background = image
	timeValue := s.machine.Variables().Get(timeVariable) - 1
	s.machine.Variables().Set(timeVariable, timeValue)
	if timeValue <= 0 {
		s.machine.Variables().Set(mapVariable, 0)
		s.mapState.Leave()
		s.gameState.EndSelect()
		core.Logf("gothic: map3 '%s' timer expired, %s=0", image, mapVariable)
		return core.StepContinue
	}
	s.gameState.EndSelect()
	s.mapState.EnterClickMap(image, timeVariable, mapVariable, pixels)
	s.gameState.BeginSelect()
	core.Logf("gothic: map3 '%s' %s=%d awaiting click", image, timeVariable, timeValue)
	return core.StepAwaitSelect
}

func (s *Session) applyScroll(args []string) core.Step {
	if len(args) < 7 {
		core.Logf("gothic: graphics 'scroll' malformed")
		return core.StepContinue
	}
	layer := parseGothicInt(args[1])
	target := parseGothicInt(args[2])
	x := parseGothicInt(args[3])
	y := parseGothicInt(args[4])
	width := parseGothicInt(args[5])
	height := parseGothicInt(args[6])
	core.Logf("gothic: scroll %d,%d (%d,%d) %dx%d ignored", layer, target, x, y, width, height)
	return core.StepContinue
}

func (s *Session) applyMusic(sub string, args []string) core.Step {
	switch sub {
	case "load":
		pending := ""
		if len(args) >= 2 {
			pending = unquoteGothic(args[1])
		}
		if len(pending) == 0 {
			s.hasPendingMusic = false
			s.pendingMusic = ""
		} else {
			s.hasPendingMusic = true
			s.pendingMusic = pending
		}
		return core.StepContinue
	case "on":
		if s.global != nil && !s.global.BgmOn {
			return core.StepContinue
		}
		if s.hasPendingMusic && len(s.pendingMusic) > 0 {
			path := s.audioCatalog.ResolveMusic(s.pendingMusic)
			if len(path) > 0 {
				s.gameState.SetBgm(path)
				s.gameState.SetBgmLoop(true)
				s.bgmName = s.pendingMusic
				core.Logf("gothic: bgm '%s' -> '%s'", s.pendingMusic, path)
			} else {
				core.Logf("gothic: bgm '%s' unresolved", s.pendingMusic)
			}
		} else {
			core.Logf("gothic: music on without pending track")
		}
		return core.StepContinue
	case "off":
		s.gameState.ClearBgm()
		s.bgmName = ""
		return core.StepContinue
	case "effect":
		if s.global != nil && !s.global.SeOn {
			return core.StepContinue
		}
		if len(args) >= 2 {
			file := unquoteGothic(args[1])
			path := s.audioCatalog.ResolveEffect(file)
			if len(path) > 0 {
				s.gameState.SetSoundEffect(path)
				s.gameState.SetEffectSerial(s.gameState.EffectSerial() + 1)
				core.Logf("gothic: se '%s' -> '%s'", file, path)
			} else {
				core.Logf("gothic: se '%s' unresolved", file)
			}
		}
		return core.StepContinue
	default:
		core.Logf("gothic: music '%s' unknown", sub)
		return core.StepContinue
	}
}

func (s *Session) applyTextCommand(sub string, args []string) core.Step {
	switch sub {
	case "frame":
		if len(args) >= 2 {
			if strings.EqualFold(args[1], "on") {
				s.frameMain = true
			} else if strings.EqualFold(args[1], "off") {
				s.frameMain = false
			}
			s.stage.SetMessageVisible(s.frameMain || s.frameUpper || s.frameLower)
		}
		return core.StepContinue
	case "frameu":
		return s.applyFrameFlag(true, args)
	case "framed":
		return s.applyFrameFlag(false, args)
	case "wait", "lwait", "resume", "memo":
		return core.StepContinue
	default:
		core.Logf("gothic: text '%s' ignored", sub)
		return core.StepContinue
	}
}

func (s *Session) applyFrameFlag(upper bool, args []string) core.Step {
	if len(args) >= 2 {
		on := strings.EqualFold(args[1], "on")
		off := strings.EqualFold(args[1], "off")
		if on || off {
			if upper {
				s.frameUpper = on
			} else {
				s.frameLower = on
			}
			s.stage.SetMessageVisible(s.frameMain || s.frameUpper || s.frameLower)
			name := "framed"
			if upper {
				name = "frameu"
			}
			value := "off"
			if on {
				value = "on"
			}
			core.Logf("gothic: %s %s", name, value)
		}
	}
	return core.StepContinue
}

func (s *Session) applyWaveOpen(statement *vmgothic.GothicStatement) core.Step {
	file := ""
	if statement != nil {
		args := statement.Args
		if len(args) >= 2 {
			file = unquoteGothic(args[1])
		} else {
			file = strings.TrimSpace(statement.Sub)
		}
	}
	core.Logf("gothic: wave open '%s'", file)
	return core.StepContinue
}

func (s *Session) applyWaveWait() core.Step {
	s.flushWaveQueue()
	s.gameState.SetWaitMilliseconds(500)
	core.Logf("gothic: wave wait 500ms")
	return core.StepWait
}

func (s *Session) applyWaveSound(id int32) core.Step {
	if !s.audioCatalog.SupportsWave() {
		core.Logf("gothic: voice %d skipped, sp.pla unavailable", id)
		return core.StepContinue
	}
	s.pendingWave = append(s.pendingWave, id)
	return core.StepContinue
}

func (s *Session) flushWaveQueue() {
	if len(s.pendingWave) == 0 {
		return
	}
	voices := []string{}
	effects := 0
	skipped := 0
	for _, id := range s.pendingWave {
		path := s.audioCatalog.ResolveVoice(int(id))
		if path == "" {
			core.Logf("gothic: voice %d skipped, resolve failed", id)
			skipped++
			continue
		}
		if int(id) < gothicVoiceEffectThreshold {
			s.gameState.SetVoice(path)
			s.gameState.SetVoiceSerial(s.gameState.VoiceSerial() + 1)
			voices = append(voices, path)
		} else {
			s.gameState.SetSoundEffect(path)
			s.gameState.SetEffectSerial(s.gameState.EffectSerial() + 1)
			effects++
		}
	}
	s.pendingWave = s.pendingWave[:0]
	if len(voices) > 0 {
		s.gameState.SetVoiceSequence(voices, false)
	}
	core.Logf("gothic: wave flush voices=%d effects=%d skipped=%d (decode pending)", len(voices), effects, skipped)
}

func (s *Session) applyTilt() core.Step {
	core.Logf("gothic: tilt ignored")
	return core.StepContinue
}

func (s *Session) applyAni(value int32) core.Step {
	s.aniOn = value != 0
	if s.aniOn {
		core.Logf("gothic: ani on")
	} else {
		core.Logf("gothic: ani off")
	}
	return core.StepContinue
}

func (s *Session) applyRmap(statement *vmgothic.GothicStatement) core.Step {
	args := []string{}
	if statement != nil {
		args = statement.Args
	}
	ds := ""
	ti := ""
	pl := ""
	if len(args) > 0 {
		ds = strings.TrimSpace(args[0])
	}
	if len(args) > 1 {
		ti = strings.TrimSpace(args[1])
	}
	if len(args) > 2 {
		pl = strings.TrimSpace(args[2])
	}
	if s.machine == nil || len(pl) == 0 {
		core.Logf("gothic: rmap without row variable, continuing")
		return core.StepContinue
	}
	mask := s.catalog.Resolve("mmask")
	if mask == nil {
		core.Logf("gothic: rmap 'mmask' unresolved, continuing")
		return core.StepContinue
	}
	page := int(s.machine.Variables().Get(ds))
	col := int(s.machine.Variables().Get(ti))
	s.drawRouteMap(page, col, pl)
	s.gameState.EndSelect()
	s.mapState.EnterRouteMap(page, col, pl, mask, BuildGothicMapColorTable(mask))
	s.hoverMask = mask
	s.hoverPage = -2147483648
	s.hoverCol = -2147483648
	s.hoverRow = -2147483648
	s.gameState.BeginSelect()
	core.Logf("gothic: rmap %s,%s,%s page=%d col=%d", ds, ti, pl, page, col)
	return core.StepAwaitSelect
}

func (s *Session) applySmap(statement *vmgothic.GothicStatement) core.Step {
	args := []string{}
	raw := ""
	if statement != nil {
		args = statement.Args
		raw = statement.Raw
	}
	if len(args) < 5 {
		core.Logf("gothic: smap '%s' malformed", raw)
		return core.StepContinue
	}
	label := unquoteGothic(args[4])
	if len(label) == 0 {
		core.Logf("gothic: smap '%s' malformed", raw)
		return core.StepContinue
	}
	s.mapState.AddZone(parseGothicInt(args[0]), parseGothicInt(args[1]), parseGothicInt(args[2]), parseGothicInt(args[3]), label)
	return core.StepContinue
}

func (s *Session) applyDmap(action *vmgothic.GothicAction) core.Step {
	args := []string{}
	raw := ""
	value := int32(0)
	if action != nil {
		if action.Statement != nil {
			args = action.Statement.Args
			raw = action.Statement.Raw
		}
		value = action.Value
	}
	label := ""
	if len(args) >= 2 {
		label = unquoteGothic(args[1])
	}
	if value <= 0 || len(label) == 0 {
		core.Logf("gothic: dmap '%s' malformed", raw)
		return core.StepContinue
	}
	s.mapState.AddTarget(int(value), label)
	return core.StepContinue
}

func (s *Session) resolveRouteClick(index int) core.Step {
	if !s.mapState.RouteActive() || s.machine == nil {
		return core.StepContinue
	}
	core.Logf("gothic: rmap choice %d without button, staying", index)
	return core.StepAwaitSelect
}

func (s *Session) resolveRoutePixel(x, y int) core.Step {
	if !s.mapState.RouteActive() || s.machine == nil {
		return core.StepContinue
	}
	row := s.mapState.RouteRowAt(x, y)
	if row <= 0 {
		core.Logf("gothic: rmap click (%d,%d) missed, staying", x, y)
		return core.StepAwaitSelect
	}
	if len(s.mapState.RouteRowVariable) > 0 {
		s.machine.Variables().Set(s.mapState.RouteRowVariable, int32(row))
	}
	kind := s.mapState.LookupKind(s.mapState.RoutePage, s.mapState.RouteColumn, row)
	label := ""
	if kind != 0 {
		label = s.mapState.LookupLabel(s.mapState.RoutePage, s.mapState.RouteColumn, row)
	} else {
		label = s.mapState.FallbackLabel(row)
	}
	if len(label) == 0 {
		label = "error"
	}
	s.stage.SetPlane(s.backgroundLayer, 1, "", 0, 0)
	s.mapState.Leave()
	s.gameState.EndSelect()
	if !s.machine.Call(label) {
		core.Logf("gothic: rmap label '%s' missing, continuing in place", label)
		step, _ := s.run(gothicDefaultRunBudget)
		return step
	}
	core.Logf("gothic: rmap click (%d,%d) row=%d kind=%d -> '%s'", x, y, row, kind, label)
	step, _ := s.run(gothicDefaultRunBudget)
	return step
}

func (s *Session) drawRouteMap(page, col int, rowVariable string) {
	school1 := s.catalog.Resolve("school1")
	if school1 == nil {
		core.Logf("gothic: rmap 'school1' unresolved")
		return
	}
	s.hoverBase1 = school1
	s.stage.LoadBase(s.backgroundLayer, "school1")
	s.hoverBase2 = s.catalog.Resolve("school2")
	if s.hoverBase2 == nil {
		core.Logf("gothic: rmap 'school2' unresolved")
	}
	kind := 0
	if s.machine != nil && len(rowVariable) > 0 {
		row := int(s.machine.Variables().Get(rowVariable))
		if row > 0 {
			kind = s.mapState.LookupKind(page, col, row)
		} else {
			kind = 0
		}
	}
	if kind <= 0 || kind > 7 || s.catalog.Resolve("parts") == nil {
		s.stage.SetPlane(s.backgroundLayer, 1, "", 0, 0)
		return
	}
	s.stage.SetPlaneRegion(s.backgroundLayer, 1, "parts", 130*(kind-1), 100, 112, 112, 520, 8)
}

func (s *Session) resolveMapPixel(x, y int) core.Step {
	if s.mapState.RouteActive() {
		return s.resolveRoutePixel(x, y)
	}
	if s.map2Waiting {
		s.map2Waiting = false
		s.mapState.Leave()
		s.gameState.EndSelect()
		core.Logf("gothic: map2 click (%d,%d) acknowledged, continuing", x, y)
		step, _ := s.run(gothicDefaultRunBudget)
		return step
	}
	if !s.mapState.ClickMapActive() || s.machine == nil {
		return core.StepContinue
	}
	if s.mapState.ClickPixels == nil || len(s.mapState.MapVariable) == 0 {
		core.Logf("gothic: map3 click without pixels, leaving map")
		s.mapState.Leave()
		s.gameState.EndSelect()
		step, _ := s.run(gothicDefaultRunBudget)
		return step
	}
	color := SampleGothicMapColor(s.mapState.ClickPixels, x, y)
	if color <= 0 {
		core.Logf("gothic: readmap click (%d,%d) no pixel, staying", x, y)
		return core.StepAwaitSelect
	}
	value := int32(color)
	if s.mapColorIndex {
		value = gothicMapColorValue(color)
	}
	variable := s.mapState.MapVariable
	s.machine.Variables().Set(variable, value)
	s.mapState.Leave()
	s.mapColorIndex = false
	s.gameState.EndSelect()
	core.Logf("gothic: click map (%d,%d) %s=%d", x, y, variable, value)
	step, _ := s.run(gothicDefaultRunBudget)
	return step
}

func (s *Session) applySetControl(statement *vmgothic.GothicStatement) core.Step {
	control := ""
	args := []string{}
	raw := ""
	if statement != nil {
		control = strings.TrimSpace(statement.Sub)
		args = statement.Args
		raw = statement.Raw
	}
	if len(control) == 0 || len(args) < 5 {
		core.Logf("gothic: set '%s' malformed", raw)
		return core.StepContinue
	}
	values := [4]int32{}
	values[0] = int32(s.evalOperand(args[1]))
	values[1] = int32(s.evalOperand(args[2]))
	values[2] = int32(parseGothicInt(args[3]))
	values[3] = int32(parseGothicInt(args[4]))
	s.controls[strings.ToLower(control)] = values
	if strings.EqualFold(control, "date1") {
		s.dateWeek = int(values[0])
		s.dateDay = int(values[1])
		s.dateHour = int(values[2])
		s.dateMinute = int(values[3])
		s.dateVisible = true
		s.stage.Invalidate()
	}
	core.Logf("gothic: set %s %d,%d,%d,%d", control, values[0], values[1], values[2], values[3])
	return core.StepContinue
}

func (s *Session) evalOperand(operand string) int {
	text := strings.TrimSpace(operand)
	if len(text) == 0 {
		return 0
	}
	if literal, err := strconv.Atoi(text); err == nil {
		return literal
	}
	if text[0] == '$' && len(text) > 1 {
		if value, err := strconv.ParseInt(text[1:], 16, 32); err == nil {
			return int(value)
		}
		core.Logf("gothic: operand '%s' invalid", text)
		return 0
	}
	if s.machine == nil {
		return 0
	}
	return int(s.machine.Variables().Get(text))
}

func (s *Session) applyInputName() core.Step {
	if s.nameResolved {
		return core.StepContinue
	}
	s.awaitingName = true
	s.nameWaitIssued = true
	core.Logf("gothic: inputname waiting, default '%s'", s.playerName)
	return core.StepAwaitInput
}

func (s *Session) applyGame(sub string) core.Step {
	if !strings.EqualFold(sub, "clear") {
		core.Logf("gothic: game '%s' unknown", sub)
		return core.StepContinue
	}
	if s.machine != nil {
		s.machine.Variables().Set("cleargame", 1)
	}
	if s.profile == gothicProfileMaze && s.global != nil {
		s.global.Cleared = true
		s.globalDirty = true
		s.flushGlobal()
	}
	s.clearMessage()
	s.stage.Reset()
	s.gameState.SetSpeaker("")
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearBgm()
	s.gameState.ClearSoundEffect()
	s.background = ""
	s.clearTransientState()
	core.Logf("gothic: game clear")
	return core.StepContinue
}

func (s *Session) applyCls() core.Step {
	s.clearMessage()
	blue, green, red := s.backgroundFill()
	s.stage.FillLayer(0, blue, green, red)
	return core.StepContinue
}

// backgroundFill 把设置里的「背景色設定」映射成画布清屏色。
// 原版的选项是 黒/赤/青/セピア，对应 layout.go 里的四个常量。
func (s *Session) backgroundFill() (byte, byte, byte) {
	if s.global == nil || s.profile != gothicProfileMaze {
		return 0, 0, 0
	}
	switch s.global.Background {
	case 1:
		return GothicRedBlue, GothicRedGreen, GothicRedRed
	case 2:
		return GothicBlueBlue, GothicBlueGreen, GothicBlueRed
	case 3:
		return GothicSepiaBlue, GothicSepiaGreen, GothicSepiaRed
	default:
		return 0, 0, 0
	}
}

func (s *Session) applyLogo() core.Step {
	if s.suppressLogo {
		s.suppressLogo = false
		return core.StepContinue
	}
	if s.profile == gothicProfileMaze {
		s.stage.Reset()
		s.stage.LoadBase(4, "black.gsa")
		if s.catalog.Resolve("logo.gsa") != nil {
			s.stage.SetPlane(4, 1, "logo.gsa", (GothicScreenWidth-512)/2, (GothicScreenHeight-256)/2)
		}
		s.backgroundLayer = 4
		s.background = "logo.gsa"
		s.gameState.SetWaitMilliseconds(1200)
		core.Logf("gothic: logo (MAZE) shown")
		return core.StepWait
	}
	if title := s.findTitleImage(); len(title) > 0 {
		s.stage.LoadBase(4, title)
		s.backgroundLayer = 4
		s.background = title
		core.Logf("gothic: logo '%s'", title)
		return core.StepContinue
	}
	if archived := s.findArchiveTitle(); len(archived) > 0 {
		s.stage.LoadBase(4, archived)
		s.backgroundLayer = 4
		s.background = archived
		s.stage.BeginEffect(3, 1, 4, 4)
		s.effectStamp = time.Now().UTC()
		core.Logf("gothic: logo '%s' fade", archived)
	} else {
		core.Logf("gothic: logo without title art, continuing")
	}
	return core.StepContinue
}

func (s *Session) applyStartMenu() core.Step {
	if s.skipStartMenu {
		s.skipStartMenu = false
		return core.StepContinue
	}
	if s.profile == gothicProfileMaze || s.isToku1Title() {
		return s.showTitle()
	}
	if !s.hasTitleMenu {
		core.Logf("gothic: startmenu bypassed, new game path")
		return core.StepContinue
	}
	return s.presentStartMenu()
}

func (s *Session) presentStartMenu() core.Step {
	texts := append([]string{}, gothicTitleMenuLabels...)
	s.gameState.SetChoices(texts)
	s.stage.SetMenuItems(texts)
	s.startMenuOpen = true
	core.Logf("gothic: startmenu presented")
	return core.StepAwaitChoice
}

func (s *Session) chooseStartMenu(index int) core.Step {
	if index == 0 {
		s.closeStartMenu()
		core.Logf("gothic: startmenu new game")
		step, _ := s.run(gothicDefaultRunBudget)
		return step
	}
	if index == len(gothicTitleMenuLabels)-1 {
		s.closeStartMenu()
		core.Logf("gothic: startmenu quit")
		return core.StepFinished
	}
	if index > 0 && index < len(gothicTitleMenuLabels)-1 {
		core.Logf("gothic: startmenu '%s' unavailable, re-presented", gothicTitleMenuLabels[index])
	} else {
		core.Logf("gothic: startmenu choice %d rejected", index)
	}
	return s.presentStartMenu()
}

func (s *Session) closeStartMenu() {
	s.startMenuOpen = false
	s.gameState.ClearChoices()
	s.stage.ClearMenuItems()
}

func (s *Session) applyStaffRoll() core.Step {
	if s.profile == gothicProfileMaze {
		s.uiOpen(uiStaffRoll)
		s.ui.staffIndex = 0
		s.clearMessage()
		s.stage.Invalidate()
		core.Logf("gothic: staffroll started")
		return core.StepAwaitInput
	}
	s.gameState.SetWaitMilliseconds(3000)
	core.Logf("gothic: staffroll simplified to wait")
	return core.StepWait
}

func (s *Session) applyRestart() core.Step {
	s.mapColorIndex = false
	s.uiClose()
	s.replayMode = false
	s.titleMode = false
	s.clearMessage()
	s.stage.Reset()
	s.gameState.Reset()
	s.gameState.ClearBgm()
	s.gameState.ClearSoundEffect()
	s.background = ""
	s.pendingMusic = ""
	s.hasPendingMusic = false
	if s.profile == gothicProfileMaze && s.program != nil {
		// MAZE 的 restart 是“回标题重开”：必须重建变量表，
		// 否则上一周目的 hN/varN 会带进新周目。Toku1 保持旧的 Reset 行为。
		s.machine = vmgothic.NewGothicMachine(s.program)
	} else if s.machine != nil {
		s.machine.Reset()
	}
	s.clearTransientState()
	core.Logf("gothic: restart")
	return core.StepContinue
}

func (s *Session) findTitleImage() string {
	names := []string{}
	graphicsDir := findGothicDirectory(s.dataDir, "GRAPHICS")
	entries, err := os.ReadDir(graphicsDir)
	if err != nil {
		core.Logf("gothic: logo scan failed: %v", err)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		file := entry.Name()
		lower := strings.ToLower(file)
		if strings.HasPrefix(lower, "title") || strings.HasPrefix(lower, "gstart") {
			names = append(names, file)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names[0]
}

func (s *Session) findArchiveTitle() string {
	for _, name := range []string{"title01", "title02", "title03", "white"} {
		if s.catalog.Resolve(name) != nil {
			return name
		}
	}
	return ""
}

func (s *Session) chooseMapPixel(x, y int) core.Step {
	if !s.mapState.ClickMapActive() && !s.mapState.RouteActive() {
		return core.StepContinue
	}
	return s.resolveMapPixel(x, y)
}

func (s *Session) cancelMapSelect() core.Step {
	if s.mapState.Active == GothicMapNone {
		return core.StepContinue
	}
	s.mapColorIndex = false
	s.stage.SetPlane(s.backgroundLayer, 1, "", 0, 0)
	s.map2Waiting = false
	s.mapState.Leave()
	s.resetHoverCache()
	s.gameState.EndSelect()
	core.Logf("gothic: map cancelled, continuing in place")
	step, _ := s.run(gothicDefaultRunBudget)
	return step
}

func (s *Session) setHoveredButton(index int) {
	s.gameState.SetHoveredButton(index)
}

func (s *Session) setMapHover(x, y int) {
	if s.disposed || !s.mapState.RouteActive() || s.machine == nil {
		return
	}
	row := s.mapState.RouteRowAt(x, y)
	page := s.mapState.RoutePage
	col := s.mapState.RouteColumn
	if page == s.hoverPage && col == s.hoverCol && row == s.hoverRow {
		return
	}
	s.hoverPage = page
	s.hoverCol = col
	s.hoverRow = row
	if row <= 0 {
		s.restoreRouteBase()
		s.stage.SetPlane(s.backgroundLayer, 1, "", 0, 0)
		return
	}
	s.updateRouteHover(x, y, row)
}

func (s *Session) resetHoverCache() {
	s.hoverBase1 = nil
	s.hoverBase2 = nil
	s.hoverMask = nil
	s.hoverPage = -2147483648
	s.hoverCol = -2147483648
	s.hoverRow = -2147483648
}

func (s *Session) restoreRouteBase() {
	if s.hoverBase1 != nil {
		s.stage.SetBaseImage(s.backgroundLayer, s.hoverBase1)
		return
	}
	s.stage.LoadBase(s.backgroundLayer, "school1")
}

func (s *Session) updateRouteHover(x, y, row int) {
	kind := s.mapState.LookupKind(s.mapState.RoutePage, s.mapState.RouteColumn, row)
	if !s.blendRouteHover(x, y) {
		s.restoreRouteBase()
	}
	if kind >= 1 && kind <= 7 {
		sourceX := 130 * (kind - 1)
		sourceWidth := 112
		sourceHeight := 112
		parts := s.catalog.Resolve("parts")
		if parts != nil {
			sourceWidth = min(sourceWidth, parts.Width-sourceX)
			sourceHeight = min(sourceHeight, parts.Height-100)
		}
		if sourceWidth > 0 && sourceHeight > 0 {
			s.stage.SetPlaneRegion(s.backgroundLayer, 1, "parts", sourceX, 100, sourceWidth, sourceHeight, 520, 8)
			return
		}
	}
	s.stage.SetPlane(s.backgroundLayer, 1, "", 0, 0)
}

func (s *Session) blendRouteHover(x, y int) bool {
	base1 := s.hoverBase1
	base2 := s.hoverBase2
	mask := s.hoverMask
	if base1 == nil || base1.Pixels == nil || base2 == nil || base2.Pixels == nil || mask == nil || mask.Pixels == nil {
		return false
	}
	width := base1.Width
	height := base1.Height
	if width <= 0 || height <= 0 || base2.Width != width || base2.Height != height || mask.Width != width || mask.Height != height {
		return false
	}
	count := width * height * 4
	if len(base1.Pixels) < count || len(base2.Pixels) < count || len(mask.Pixels) < count {
		return false
	}
	maskPixels := mask.Pixels
	maskOffset := ((y * width) + x) * 4
	if maskOffset < 0 || maskOffset+2 >= len(maskPixels) {
		return false
	}
	target0 := maskPixels[maskOffset]
	target1 := maskPixels[maskOffset+1]
	target2 := maskPixels[maskOffset+2]
	if s.hoverPixels == nil || len(s.hoverPixels) != count {
		s.hoverPixels = make([]byte, count)
		s.hoverComposite = nil
	}
	blended := s.hoverPixels
	copy(blended, base1.Pixels[:count])
	hoverPixels := base2.Pixels
	for offset := 0; offset < count; offset += 4 {
		if maskPixels[offset] == target0 && maskPixels[offset+1] == target1 && maskPixels[offset+2] == target2 {
			blended[offset] = hoverPixels[offset]
			blended[offset+1] = hoverPixels[offset+1]
			blended[offset+2] = hoverPixels[offset+2]
			blended[offset+3] = hoverPixels[offset+3]
		}
	}
	if s.hoverComposite == nil {
		s.hoverComposite = gfx.NewBsgImage(width, height, 0, 0, blended)
	}
	s.stage.SetBaseImage(s.backgroundLayer, s.hoverComposite)
	return true
}

func (s *Session) chooseButton(index int) core.Step {
	if !s.mapState.RouteActive() {
		return core.StepContinue
	}
	return s.resolveRouteClick(index)
}

func parseGothicInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func unquoteGothic(value string) string {
	text := strings.TrimSpace(value)
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		return text[1 : len(text)-1]
	}
	return text
}

// gothicMapColorValue 把 oem.gsa 的四块纯色换算成剧本约定的 1..4。
// 顺序来自原版 staffroom：青(1) 藍(2) 緑(3) 赤(4)，红块在右上角，
// 是“退出/结束”那一块。
func gothicMapColorValue(color int) int32 {
	switch color & 0xFFFFFF {
	case 0x00FFFF: // cyan
		return 1
	case 0x0000FF: // blue
		return 2
	case 0x00FF00: // green
		return 3
	case 0xFF0000: // red
		return 4
	}
	return 0
}
