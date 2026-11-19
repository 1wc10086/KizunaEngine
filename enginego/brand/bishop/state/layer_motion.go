package state

import "time"

// layer_motion.go 实现 LAYER_PARAM type=6 的关键帧移动/淡入淡出。
//
// 原引擎把 define.bsa 里的 `<name>.txt` 动画脚本挂在刚画好的图层上：
// 牝教師4 的 A-H-E 是「アヘ顔」特写从画面上方滑入并淡入，
// 放課後3 的地图卡则是从上方翻转入场。地图卡在本引擎里是按钮，
// 位置由 type=0 单独给出，所以只有"普通图层 + 脚本"走这条路径。
//
// 这里只保存动画数据，不依赖资源包/渲染器；具体 XML 解析在
// runtime/stage_director.go，绘制在 rendering/scene_composer.go。

// LayerMotionKeyframe 是一帧关键帧。HasPos/HasOpacity 用来区分
// "这一帧没写该属性，沿用上一帧"和"明确写成 0"。
type LayerMotionKeyframe struct {
	At         float64
	X, Y       int
	HasPos     bool
	Opacity    float64
	HasOpacity bool
}

// LayerMotion 是一层的关键帧动画。
//
// Pos 是屏幕坐标里的**中心点**（和按钮动画脚本一致）；绘制时减去图片
// 半宽/半高再叠加到图层原点上。BaseX/BaseY 是该层第一个部件的原始位置，
// 这样只位移动画本身，不会把多部件层的相对位置拆开。
type LayerMotion struct {
	Start     time.Time
	BaseX     int
	BaseY     int
	Width     int
	Height    int
	Keyframes []LayerMotionKeyframe
	// Hidden 表示这层因为「アヘ顔OFF」不该显示。绘制时直接跳过，
	// 不修改图层本身的透明度/可见性，换下一张图时 SetLayerAt 会清掉动画。
	Hidden bool
}

// NewHiddenLayerMotion 创建一个永远不绘制的动画，用于关闭アヘ顔图层。
func NewHiddenLayerMotion() *LayerMotion {
	return &LayerMotion{Hidden: true}
}

// Transform 返回当前时刻图层相对原点的位移与不透明度（0..1）。
//
// ok=false 表示这层没有可绘制内容（例如 Hidden）。动画最后一帧之后
// 保持最终值：A-H-E 的最后一帧 opacity=0，所以アヘ顔会停在"已淡出"，
// 与脚本作者的意图一致。
func (m *LayerMotion) Transform(now time.Time) (dx, dy int, opacity float64, ok bool) {
	if m == nil || m.Hidden || len(m.Keyframes) == 0 || m.Width <= 0 || m.Height <= 0 {
		return 0, 0, 0, false
	}
	elapsed := now.Sub(m.Start).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	// 找 elapsed 所在区间；只保留最近一个已知位置/透明度。
	current := m.Keyframes[0]
	position := struct {
		x, y int
		has  bool
	}{}
	alpha := 1.0
	apply := func(frame LayerMotionKeyframe) {
		if frame.HasPos {
			position.x, position.y, position.has = frame.X, frame.Y, true
		}
		if frame.HasOpacity {
			alpha = frame.Opacity
		}
	}
	apply(current)
	for index := 1; index < len(m.Keyframes); index++ {
		next := m.Keyframes[index]
		if elapsed >= next.At {
			current = next
			apply(current)
			continue
		}
		// 在 current 与 next 之间插值；只有两边都写了 pos 才插值坐标，
		// 否则保持上一个已知位置（A-H-E 的 70 帧只写透明度）。
		span := next.At - current.At
		if span <= 0 {
			apply(next)
			break
		}
		t := (elapsed - current.At) / span
		if current.HasPos && next.HasPos {
			position.x = int(float64(current.X) + float64(next.X-current.X)*t + 0.5)
			position.y = int(float64(current.Y) + float64(next.Y-current.Y)*t + 0.5)
			position.has = true
		}
		if current.HasOpacity && next.HasOpacity {
			alpha = current.Opacity + (next.Opacity-current.Opacity)*t
		} else if next.HasOpacity {
			// 上一帧没写透明度时，原引擎从默认 1 开始淡到目标值。
			alpha = 1 + (next.Opacity-1)*t
		}
		break
	}
	if !position.has {
		return 0, 0, clampOpacity(alpha), true
	}
	dx = position.x - m.Width/2 - m.BaseX
	dy = position.y - m.Height/2 - m.BaseY
	return dx, dy, clampOpacity(alpha), true
}

func clampOpacity(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// SetLayerMotion 给图层挂上关键帧动画。下一次合成按当前时刻计算位移/
// 透明度，静态图仍然由 LayerSurface 正常绘制。
func (s *GameState) SetLayerMotion(plane int, motion *LayerMotion) {
	if plane < 0 || plane >= PlaneCount {
		return
	}
	if s.motions == nil {
		s.motions = make(map[int]*LayerMotion, 2)
	}
	s.motions[plane] = motion
	s.stageRevision++
}

// LayerMotion 返回图层上的关键帧动画；没有则返回 nil。
func (s *GameState) LayerMotion(plane int) *LayerMotion {
	if s.motions == nil {
		return nil
	}
	return s.motions[plane]
}

// ClearLayerMotion 取消图层上的关键帧动画。
func (s *GameState) ClearLayerMotion(plane int) {
	if s.motions == nil {
		return
	}
	if _, ok := s.motions[plane]; !ok {
		return
	}
	delete(s.motions, plane)
	s.stageRevision++
}

// HasLayerMotions 表示当前是否有关键帧动画需要持续重画。
func (s *GameState) HasLayerMotions() bool {
	return len(s.motions) > 0
}

func (s *GameState) clearMotions() {
	if len(s.motions) == 0 {
		return
	}
	clear(s.motions)
	s.stageRevision++
}
