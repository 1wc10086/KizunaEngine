package state

import (
	"time"

	"kizuna/engine/brand/bishop/gfx"
)

// FrameAnimator 是一个按时间取帧的动画源。
//
// 定义在 state 而不是 rendering，是为了让"哪个图层在播动画"跟着 GameState
// 一起被存档/重置；具体怎么解码由 rendering 实现，state 不依赖 resource。
type FrameAnimator interface {
	Frame(now time.Time) *gfx.BsgImage
}

// SetLayerAnimator 给图层挂上动画。下一次合成开始画这个动画的当前帧，
// 原来那张静态图不再绘制。
func (s *GameState) SetLayerAnimator(plane int, anim FrameAnimator) {
	if plane < 0 || plane >= PlaneCount || anim == nil {
		return
	}
	if s.animators == nil {
		s.animators = make(map[int]FrameAnimator, 4)
	}
	s.animators[plane] = anim
	s.stageRevision++
}

// LayerAnimator 返回图层上的动画；没有则返回 nil。
func (s *GameState) LayerAnimator(plane int) FrameAnimator {
	if s.animators == nil {
		return nil
	}
	return s.animators[plane]
}

// ClearLayerAnimator 取消图层上的动画（换图、清层、整场重置时调用）。
func (s *GameState) ClearLayerAnimator(plane int) {
	if s.animators == nil {
		return
	}
	if _, ok := s.animators[plane]; !ok {
		return
	}
	delete(s.animators, plane)
	s.stageRevision++
}

// HasAnimators 表示当前是否有动画在播；渲染器据此每帧强制重画。
func (s *GameState) HasAnimators() bool {
	return len(s.animators) > 0
}

func (s *GameState) clearAnimators() {
	if len(s.animators) == 0 {
		return
	}
	clear(s.animators)
	s.stageRevision++
}
