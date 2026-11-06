package session

import "kizuna/engine/brand/bishop/gfx"

// standee_render.go 处理"立绘合成图里的一张图可能是竖排帧条"这件事。
//
// 原引擎的立绘（bu*）是 BSS-Composition：一个 1024x768 或 2350x2990 的容器，
// 里面放若干部件。很多部件本身又是指令头里写了 FrameCount 的竖排帧条——
// 最常见的是眨眼/表情帧。如果直接把整个部件贴上去，画面上会看到同一张脸
// 从上到下重复 6~8 次（真机截图里就是一条"人柱"）。
//
// 契约：合成图的每个部件只画它的第 0 帧。动画在剧情里由 LayerSurface 按
// 时间推进；鉴赏室/脱衣画面是静态展示，不需要眨眼，也不该让帧条露出来。

// standeePartFrame 取立绘部件的第一帧；没有帧信息时原样返回。
//
// partCount 固定传 1：这里的"部件数"是合成图的图层数，不是帧数。
// 把图层数当帧数是历史上"多只眼睛"的根因，见 gfx.TryResolveBsgFrameStrip 的注释。
func standeePartFrame(part *gfx.BsgImage) *gfx.BsgImage {
	return standeePartFrameN(part, 1)
}

// standeePartFrameN 取部件的第 0 帧；frames 是调用方知道的帧数。
//
// 表情/眨眼部件（a/f/face/bu01f0001a12 这类）的 BSG 头没有写帧数，
// 但图像高度正好是 eye pattern 的整数倍。调用方把 bsxx.cfg 的
// eye pattern 传进来，才能把它切成一张脸而不是一整条"人柱"。
// 高度除不尽时说明它不是帧条，保持原样，避免把普通部件切坏。
func standeePartFrameN(part *gfx.BsgImage, frames int) *gfx.BsgImage {
	if part == nil || part.Width <= 0 || part.Height <= 0 {
		return part
	}
	// 不再要求"高度能被 frames 整除"：眼睛条的实际帧高未必整除（SMIE 的
	// bu01u0001a05 是 411px / 4 帧），旧检查会把整条 4 行眼睛原样画出来。
	// 调用方现在只对"确认是帧条的部件"传 frames>1，普通部件一律传 1，
	// 所以这里可以放心按 frames 均分。
	frames, frameHeight, ok := gfx.TryResolveBsgFrameStrip(part, 1, frames)
	if !ok || frames <= 1 || frameHeight <= 0 || frameHeight >= part.Height {
		return part
	}

	top := gfx.SourceTopBsgFrameStrip(part, frames, frameHeight, 0)
	stride := part.Stride()
	offset := top * stride
	size := part.Width * frameHeight * 4
	if offset < 0 || offset+size > len(part.Pixels) {
		return part
	}
	pixels := make([]byte, size)
	copy(pixels, part.Pixels[offset:offset+size])
	// 偏移量要保留：合成图的部件坐标是相对容器的，丢了偏移就会错位。
	return gfx.NewBsgImage(part.Width, frameHeight, part.OffsetX, part.OffsetY, pixels)
}
