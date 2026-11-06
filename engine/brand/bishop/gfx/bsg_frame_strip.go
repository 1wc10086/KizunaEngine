package gfx

import "kizuna/engine/core"

// BsgFrameStripFallbackFrames 是"没有任何帧信息可用时"的最后兜底。
// 只有在调用方既没有给出图片自带的帧数、也没有给出期望帧数时才会用到它，
// 例如某些旧作的 4 连表情图。
const BsgFrameStripFallbackFrames = 4

// TryResolveBsgFrameStrip 决定一张竖排帧条该切成几帧。
//
// 优先级（顺序很重要，改错会直接导致立绘出现"多只眼睛"）：
//  1. part.FrameCount：BSG 头里显式写了帧数，最可信；
//  2. requestedFrames：调用方按 bsxx.cfg 的 eye pattern 算出来的帧数。立绘的
//     表情/眨眼条就是走这条路——cfg 写 `eye pattern = 6`，图片 834px 高时
//     每帧 139px。旧实现把"部件数 > 1"的兜底排在 requestedFrames 前面，
//     于是 834px 被切成 4 帧（每帧 208px），一帧里塞进一只半眼睛，画面上就是
//     好几只眼睛和眉毛叠在一起；
//  3. partCount > 1 的 4 帧兜底：只有前两者都拿不到时才用。
//
// partCount 兜底放最后，是因为它描述的是"这张合成图有几个部件"，和"部件内部
// 有几帧"是两件不同的事，把它当帧数只是历史猜测。
func TryResolveBsgFrameStrip(part *BsgImage, partCount int, requestedFrames int) (frames int, frameHeight int, ok bool) {
	frames = 0
	if part.FrameCount > 1 {
		frames = part.FrameCount
	}

	if frames <= 1 && requestedFrames > 1 {
		frames = requestedFrames
	}

	if frames <= 1 && partCount > 1 {
		frames = BsgFrameStripFallbackFrames
	}

	if frames <= 1 {
		frames = 1
	}

	frames = core.Clamp(frames, 1, max(1, part.Height))
	frameHeight = core.Clamp(part.Height/frames, 1, max(1, part.Height))
	return frames, frameHeight, frames > 1
}

func SourceTopBsgFrameStrip(part *BsgImage, frames int, frameHeight int, frame int) int {
	index := 0
	if frames > 0 {
		index = ((frame % frames) + frames) % frames
	}

	return min(index*frameHeight, max(0, part.Height-frameHeight))
}
