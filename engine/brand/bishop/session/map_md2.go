package session

import (
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// map_md2.go 实现 BSXScript 1.0「満淫電車2」这一代的精灵图地图。
//
// 和 bsmap 的 map.dat 地图不同，这一代的脚本自己把 map_base（或按 #time
// 换成 map_base1/2）铺成背景，然后在 map 标签里发 SELECT_TAIL "map"：
//
//	地图背景已经由 IMAGE 指令画好；
//	引擎补画 6 个站名牌、每个站上的角色头像、悬停箭头和「時間を進める」；
//	玩家点站牌写 #map_result = 0..5，点时间按钮写 #map_result = 6；
//	脚本按 #map_result 分派 map_station1..6，否则进入 next_time。
//
// 因此这里不能走 showMapStage/clearMapStage：那套会替换脚本画好的舞台层。
// 站点按钮和普通舞台按钮共用一张按钮表，靠 Atlas 的 Clip 只画精灵图里
// 的站牌矩形；角色头像/箭头是纯装饰，画在合成后的帧上。

// beginMD2MapSelect 为 SELECT_TAIL "map" 注册精灵图地图按钮。
// 返回 false 表示当前作品不是这一代地图，调用方继续走 bsmap/通用路径。
func (s *Session) beginMD2MapSelect() bool {
	if s.config == nil || s.gameState == nil {
		return false
	}
	phase := s.config.MapPhase
	if !phase.AtlasMap() {
		return false
	}
	// 只清按钮表，不动舞台层：底图是脚本画的。
	s.gameState.EndSelect()
	s.md2Map = true
	s.mapSource = "md2"
	s.mapItems = nil
	s.mapSystemCount = 0
	s.mapSystemTools = nil
	for index, place := range phase.Places {
		button := place.Icon
		button.X = place.BasePos.X
		button.Y = place.BasePos.Y
		s.gameState.AddButton(state.StageButton{
			Image:             phase.Background,
			X:                 button.X,
			Y:                 button.Y,
			Value:             index,
			Frames:            max(1, button.Frames),
			Width:             button.Clip[2],
			Height:            button.Clip[3],
			Layer:             -1,
			Clip:              button.Clip,
			Atlas:             true,
			AtlasAxis:         button.AtlasAxis,
			IgnoreImageOffset: true,
		})
	}
	// 「時間を進める」排在站点之后；值 1000 只用于区分，写回时统一按
	// 脚本约定写成 6（>=6 会落入 next_time 分支）。
	if phase.NextTime.Atlas {
		s.gameState.AddButton(state.StageButton{
			Image:             phase.Background,
			X:                 phase.NextTimePos.X,
			Y:                 phase.NextTimePos.Y,
			Value:             1000,
			Frames:            max(1, phase.NextTime.Frames),
			Width:             phase.NextTime.Clip[2],
			Height:            phase.NextTime.Clip[3],
			Layer:             -1,
			Clip:              phase.NextTime.Clip,
			Atlas:             true,
			AtlasAxis:         phase.NextTime.AtlasAxis,
			IgnoreImageOffset: true,
		})
		s.mapSystemCount = 1
		s.mapSystemTools = []string{"next"}
	}
	s.gameState.BeginSelect()
	// 手册：进入地图/选择肢之前自动存档。内部 AutoSlot 不占用手动格，
	// 读档时按 MapSource 重建这张地图。
	s.autoSaveOnce()
	core.Logf("script: md2 map select %d station(s)", len(phase.Places))
	return true
}

// pointerMD2Map 处理精灵图地图上的移动/点击。
func (s *Session) pointerMD2Map(op uint32, x, y int) (core.Step, error) {
	switch op {
	case 0:
		index := s.hitButton(x, y)
		s.gameState.SetHoveredButton(index)
		if s.composer != nil {
			s.composer.Invalidate()
		}
		return s.lastStep, nil
	case 1, 3:
		// 宿主对一次点按会先发 press(1) 再发 release(3)：选中站点后剧情
		// 立刻开始，紧随其后的 release 不能当成第二次点击。
		if op == 3 && !s.choicePickedAt.IsZero() {
			elapsed := time.Since(s.choicePickedAt)
			s.choicePickedAt = time.Time{}
			if elapsed < 250*time.Millisecond {
				return s.lastStep, nil
			}
		}
		index := s.hitButton(x, y)
		if index < 0 {
			return s.lastStep, nil
		}
		s.choicePickedAt = time.Now()
		return s.finishRun(s.chooseMD2MapButton(index)), nil
	case 4, 5:
		// 右键在车站地图上没有语义；保持在当前画面，避免误触退出。
		return s.lastStep, nil
	}
	return s.lastStep, nil
}

func (s *Session) chooseMD2MapButton(index int) state.SessionStep {
	phase := s.config.MapPhase
	places := len(phase.Places)
	value := 6 // 「時間を進める」：脚本按 >=6 进 next_time
	if index < places {
		value = index
	}
	if index < 0 || (index >= places && index >= places+s.mapSystemCount) {
		return state.SessionContinue
	}
	s.md2Map = false
	s.mapSource = ""
	s.lastAutoSavePC = -1
	s.mapSystemCount = 0
	s.mapSystemTools = nil
	s.mapHoverRow = -1
	s.gameState.EndSelect()
	s.gameState.SetHoveredButton(-1)
	s.writeOne(phase.ResultVariable, -1, value)
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("script: md2 map result %d", value)
	return s.run(commandBudget)
}

// drawMD2MapOverlay 画站点上的角色头像和悬停箭头。
func (s *Session) drawMD2MapOverlay(canvas *gfx.BgraCanvas) bool {
	if !s.md2Map || canvas == nil || s.composer == nil || s.config == nil {
		return false
	}
	phase := s.config.MapPhase
	image := s.composer.Load(phase.Background)
	if image == nil {
		return false
	}
	drawn := false
	// 每个站的 #map_placeN 是角色在位掩码：bit0 愛莉、bit1 美里、
	// bit2 かおる、bit3 唯子、bit4 沙絵。头像数量决定用哪一组
	// `iconN/M pos` 相对坐标。
	for _, place := range phase.Places {
		bits := s.readProperty(place.StateVar)
		if bits <= 0 {
			continue
		}
		order := make([]int, 0, len(phase.Characters))
		for bit := 0; bit < len(phase.Characters); bit++ {
			if bits&(1<<bit) != 0 {
				order = append(order, bit)
			}
		}
		count := len(order)
		if count == 0 {
			continue
		}
		if count > 5 {
			count = 5
			order = order[:count]
		}
		positions := phase.IconPositions[count]
		for slot, character := range order {
			if slot >= len(positions) || character >= len(phase.Characters) {
				break
			}
			icon := phase.Characters[character]
			srcX, srcY, width, height := icon.Clip[0], icon.Clip[1], icon.Clip[2], icon.Clip[3]
			if width <= 0 || height <= 0 ||
				srcX < 0 || srcY < 0 || srcX+width > image.Width || srcY+height > image.Height {
				continue
			}
			canvas.DrawRegion(image, srcX, srcY, width, height,
				place.BasePos.X+positions[slot].X, place.BasePos.Y+positions[slot].Y)
			drawn = true
		}
	}
	// 有角色/事件的车站画一个箭头标记；悬停的那一个按 15 帧循环动画，
	// 其余停在第一帧。原版地图上所有可去站点都带红色箭头。
	hovered := -1
	if s.gameState != nil {
		hovered = s.gameState.HoveredButton()
	}
	if phase.Arrow.Atlas {
		for index, place := range phase.Places {
			bits := s.readProperty(place.StateVar)
			if bits <= 0 && index != hovered {
				continue
			}
			arrow := phase.Arrow
			frames := max(1, arrow.Frames)
			frame := 0
			if index == hovered {
				frame = int(time.Now().UnixMilli()/70) % frames
			}
			srcX, srcY, width, height := arrow.Clip[0], arrow.Clip[1], arrow.Clip[2], arrow.Clip[3]
			if arrow.AtlasAxis == 1 {
				srcX += frame * width
			} else {
				srcY += frame * height
			}
			if width <= 0 || height <= 0 ||
				srcX < 0 || srcY < 0 || srcX+width > image.Width || srcY+height > image.Height {
				continue
			}
			canvas.DrawRegion(image, srcX, srcY, width, height,
				place.BasePos.X+phase.ArrowPos.X, place.BasePos.Y+phase.ArrowPos.Y)
			drawn = true
		}
	}
	return drawn
}

// autoSaveOnce 在同一个 PC 上只写一次内部自动档，避免每帧 Run/Compose
// 都把自动档重写一遍（也会反复覆盖玩家刚写进去的手动档）。
func (s *Session) autoSaveOnce() {
	if s.config == nil || s.config.Ecstasy.Empty() || !s.config.DataScreen.AutoSlotGrid ||
		s.saves == nil || s.ui != nil || s.titleMode || s.replay {
		return
	}
	pc := s.machine.InstructionStart()
	if pc == s.lastAutoSavePC {
		return
	}
	s.lastAutoSavePC = pc
	if err := s.Save(uint32(save.AutoSlot), s.summarize()); err != nil {
		core.Logf("save: auto-save failed: %v", err)
		return
	}
	core.Logf("save: auto-save at pc=%d", pc)
}
