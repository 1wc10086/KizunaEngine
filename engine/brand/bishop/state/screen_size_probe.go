package state

import (
	"math"
	"sort"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

const (
	screenProbeMaximumSamples  = 1024
	screenProbeMinimumHits     = 3
	screenProbeMinimumOverride = 32
	screenProbeOverrideMargin  = 8
	screenProbeMinimumArea     = 0.2
	screenProbeMinimumAspect   = 1.2
	screenProbeMaximumAspect   = 2.0
	screenProbeMinimumDelta    = 0.1
)

func ScreenSizeResolve(resources ResourceIndex, configured *ScreenSize) *ScreenSize {
	tally := screenSizeSample(resources)
	if len(tally) == 0 {
		core.Logf("screen: probe: no images")
		return nil
	}
	var largest int64
	for size := range tally {
		if area := size.Area(); area > largest {
			largest = area
		}
	}
	detected, ok := screenSizeMostFrequent(tally, largest)
	if !ok {
		core.Logf("screen: probe: no candidate among %d sizes", len(tally))
		return nil
	}
	if configured == nil {
		core.Logf("screen: probe: %dx%d hits=%d", detected.Width, detected.Height, tally[detected])
		result := detected
		return &result
	}
	current := *configured
	if current == detected {
		return nil
	}
	detectedHits := tally[detected]
	configuredHits := tally[current]
	if detectedHits < screenProbeMinimumOverride ||
		configuredHits*screenProbeOverrideMargin > detectedHits ||
		math.Abs(current.Aspect()-detected.Aspect()) < screenProbeMinimumDelta {
		core.Logf("screen: probe: keep configured %dx%d (hits %d vs %d)", current.Width, current.Height, configuredHits, detectedHits)
		return nil
	}
	core.Logf("screen: probe: override %dx%d -> %dx%d (hits %d vs %d)", current.Width, current.Height, detected.Width, detected.Height, configuredHits, detectedHits)
	result := detected
	return &result
}

func screenSizeSample(resources ResourceIndex) map[ScreenSize]int {
	tally := make(map[ScreenSize]int)
	samples := 0
	header := make([]byte, gfx.BsgHeaderSize)
	archives := resources.ArchivesInCategory(GraphicsCategory)
	sort.SliceStable(archives, func(i, j int) bool {
		return strings.ToLower(archives[i].Name()) < strings.ToLower(archives[j].Name())
	})
	for _, archive := range archives {
		if samples >= screenProbeMaximumSamples {
			break
		}
		for _, entry := range archive.Entries() {
			if samples >= screenProbeMaximumSamples {
				break
			}
			if !strings.HasSuffix(strings.ToLower(entry.Path), ".bsg") {
				continue
			}
			samples++
			if archive.Read(entry.Offset, header) != len(header) {
				continue
			}
			width, height, ok := gfx.TryReadBsgDimensions(header)
			if !ok {
				continue
			}
			size := ScreenSize{Width: width, Height: height}
			tally[size]++
		}
	}
	core.Logf("screen: probe: sampled %d images", samples)
	return tally
}

func screenSizeMostFrequent(tally map[ScreenSize]int, largest int64) (ScreenSize, bool) {
	var best ScreenSize
	bestHits := 0
	found := false
	for size, hits := range tally {
		if hits < screenProbeMinimumHits || hits <= bestHits || size.Area() < int64(float64(largest)*screenProbeMinimumArea) {
			continue
		}
		aspect := size.Aspect()
		if aspect < screenProbeMinimumAspect || aspect > screenProbeMaximumAspect {
			continue
		}
		best = size
		bestHits = hits
		found = true
	}
	return best, found
}
