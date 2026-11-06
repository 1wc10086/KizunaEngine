package stage

import (
	"strconv"
	"strings"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
)

type MaskPatternSource interface {
	Get(pattern int) *bishopgfx.BsgImage
}

const (
	MaskPatternPrefix = "mask_"
	MaskPatternSuffix = ".bmp"
)

func MaskPatternFileName(pattern int) string {
	return MaskPatternPrefix + formatMaskPattern(pattern) + MaskPatternSuffix
}

func formatMaskPattern(pattern int) string {
	digits := strconv.Itoa(pattern)
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = digits[1:]
	}
	if len(digits) < 2 {
		digits = strings.Repeat("0", 2-len(digits)) + digits
	}
	if negative {
		return "-" + digits
	}
	return digits
}

func Luminance(pixels []byte, offset int) int {
	blue := pixels[offset]
	green := pixels[offset+1]
	red := pixels[offset+2]
	if red == green && green == blue {
		return int(red)
	}

	return (int(red)*77 + int(green)*151 + int(blue)*28) >> 8
}

type EmonCachedMaskPatterns struct {
	source MaskPatternSource
	cache  map[int]*bishopgfx.BsgImage
}

func NewEmonCachedMaskPatterns(source MaskPatternSource) *EmonCachedMaskPatterns {
	return &EmonCachedMaskPatterns{source: source, cache: make(map[int]*bishopgfx.BsgImage)}
}

func (c *EmonCachedMaskPatterns) Count() int {
	return len(c.cache)
}

func (c *EmonCachedMaskPatterns) Get(pattern int) *bishopgfx.BsgImage {
	if cached, ok := c.cache[pattern]; ok {
		return cached
	}

	image := c.source.Get(pattern)
	c.cache[pattern] = image
	return image
}

func (c *EmonCachedMaskPatterns) Clear() {
	clear(c.cache)
}
