package glyph

import (
	"kizuna/enginego/brand/emon/text"
	"kizuna/enginego/core/textrender"
)

// EmonGlyphSource 是 brand/emon/text 的单字形源，现在仅作薄适配：实际栅格
// 与缓存都归 core/textrender，不再自带字体。字体家族/粗细参数保留接口但
// 目前不映射到字形（统一内嵌字体），保留是为了不改调用方的接口。
type EmonGlyphSource struct{}

func NewEmonGlyphSource() *EmonGlyphSource {
	return &EmonGlyphSource{}
}

var Instance = NewEmonGlyphSource()

func init() {
	text.SetCurrentGlyphSource(Instance)
}

// Install 把薄适配层装为当前字形源，保持原行为。
func Install() {
	text.SetCurrentGlyphSource(Instance)
}

func (s *EmonGlyphSource) Render(character rune, pixelSize int) *text.EmonGlyphBitmap {
	return s.RenderStyled(character, pixelSize, "", 0, 0)
}

// RenderStyled 把 textrender 的单字形转成 text 的位图；textrender 返回 nil
// 时用内置回退保证仍有推进宽度。family/weight/quality 不再有不同位图，同一
// 字符同一字号只栅格一次。
func (s *EmonGlyphSource) RenderStyled(character rune, pixelSize int, family string, weight int, quality int) *text.EmonGlyphBitmap {
	if pixelSize <= 0 {
		return nil
	}
	bitmap := textrender.GlyphRaster(character, pixelSize)
	if bitmap != nil {
		return &text.EmonGlyphBitmap{Width: bitmap.Width, Height: bitmap.Height, Alpha: bitmap.Alpha}
	}
	return text.EmonFallback.Render(character, pixelSize)
}

// ClearCache 清空 textrender 的单字形缓存。整段 Render 的缓存由 core 持有，
// 这里不清。
func (s *EmonGlyphSource) ClearCache() {
	textrender.ClearGlyphCache()
}

// FamilyList 为向后兼容保留，实际不再用于新字形映射；返回固定家族列表。
const FamilyList = "Noto Sans SC, sans-serif, Noto Sans CJK SC, Noto Sans SC, Droid Sans Fallback"
