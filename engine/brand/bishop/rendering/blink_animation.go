package rendering

import (
	"time"

	"kizuna/engine/core/dotnet"
)

const (
	blinkShortPauseMinimum = 200.0
	blinkShortPauseRange   = 800.0
	blinkLongPauseMinimum  = 4000.0
	blinkLongPauseRange    = 2000.0
	blinkShortPauseChance  = 0.8
)

type BlinkAnimation struct {
	frames            int
	frameMilliseconds float64
	random            *dotnet.Random
	frame             int
	next              float64
}

func NewBlinkAnimation(frames int, frameMilliseconds float64) *BlinkAnimation {
	if frames < 1 {
		frames = 1
	}
	if frameMilliseconds < 1.0 {
		frameMilliseconds = 1.0
	}
	b := &BlinkAnimation{
		frames:            frames,
		frameMilliseconds: frameMilliseconds,
		random:            dotnet.NewRandomSeed(int32(time.Now().UnixNano())),
	}
	b.next = b.pause()
	return b
}

func (b *BlinkAnimation) Frame() int {
	return b.frame
}

func (b *BlinkAnimation) Advance(milliseconds float64) {
	if b.frames <= 1 {
		b.frame = 0
		return
	}
	guard := 0
	for milliseconds >= b.next && guard < 64 {
		guard++
		b.frame++
		if b.frame >= b.frames {
			b.frame = 0
			b.next = milliseconds + b.pause()
			continue
		}
		b.next = milliseconds + b.frameMilliseconds
	}
}

func (b *BlinkAnimation) pause() float64 {
	if b.random.NextDouble() > blinkShortPauseChance {
		return blinkShortPauseMinimum + b.random.NextDouble()*blinkShortPauseRange
	}
	return blinkLongPauseMinimum + b.random.NextDouble()*blinkLongPauseRange
}
