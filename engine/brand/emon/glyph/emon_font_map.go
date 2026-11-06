package glyph

import (
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"kizuna/engine/core"
)

const (
	BundledFontFamily = "Noto Sans SC"
	BundledSans       = BundledFontFamily
	BundledSerif      = BundledFontFamily
	fontMapLogLimit   = 64
	FontFamilyList    = "Noto Sans SC, sans-serif, Noto Sans CJK SC, Noto Sans SC, Droid Sans Fallback"
)

var Faces = map[string]string{
	"msgothic":     BundledSans,
	"msゴシック":       BundledSans,
	"mspgothic":    BundledSans,
	"mspゴシック":      BundledSans,
	"msuigothic":   BundledSans,
	"ゴシック":         BundledSans,
	"yugothic":     BundledSans,
	"游ゴシック":        BundledSans,
	"meiryo":       BundledSans,
	"メイリオ":         BundledSans,
	"notosanssc":   BundledSans,
	"notosansjp":   BundledSans,
	"notosanscjk":  BundledSans,
	"msmincho":     BundledSerif,
	"ms明朝":         BundledSerif,
	"mspmincho":    BundledSerif,
	"msp明朝":        BundledSerif,
	"mincho":       BundledSerif,
	"明朝":           BundledSerif,
	"yumincho":     BundledSerif,
	"游明朝":          BundledSerif,
	"notoserifsc":  BundledSerif,
	"notoserifjp":  BundledSerif,
	"notoserifcjk": BundledSerif,
}

var defaultFontKey = CanonicalKey("MS Gothic")

var (
	fontMapLogGate sync.Mutex
	fontMapLogged  = map[string]struct{}{}
)

func CanonicalKey(family string) string {
	if strings.TrimSpace(family) == "" {
		return ""
	}

	folded := strings.TrimSpace(family)
	folded = strings.Trim(folded, "\"")
	folded = norm.NFKC.String(folded)

	var builder strings.Builder
	builder.Grow(len(folded))
	for _, character := range folded {
		if unicode.IsSpace(character) || character == '-' || character == '_' || character == '.' {
			continue
		}

		builder.WriteRune(unicode.ToLower(character))
	}

	return builder.String()
}

func ResolveFamilyList(canonicalKey string, requested string) string {
	if len(canonicalKey) == 0 {
		canonicalKey = defaultFontKey
	}

	if face, ok := Faces[canonicalKey]; ok {
		return face + ", " + FontFamilyList
	}

	LogUnmapped(strings.TrimSpace(requested))
	return requested + ", " + FontFamilyList
}

func LogUnmapped(requested string) {
	if len(requested) == 0 {
		return
	}

	fontMapLogGate.Lock()
	if len(fontMapLogged) >= fontMapLogLimit {
		fontMapLogGate.Unlock()
		return
	}

	if _, ok := fontMapLogged[requested]; ok {
		fontMapLogGate.Unlock()
		return
	}

	fontMapLogged[requested] = struct{}{}
	fontMapLogGate.Unlock()

	core.Logf("font: no mapping for font family '%s', falling back to the compatibility list", requested)
}
