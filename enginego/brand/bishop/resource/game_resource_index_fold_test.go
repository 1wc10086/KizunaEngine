package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFoldedFallbackFindsUppercaseEntry(t *testing.T) {
	g := newGameResourceIndex("")
	logo := LooseResourceLocation("LOGO1", GraphicsCategory, "LOGO1.bmp")
	g.addNamed("LOGO1", logo, gameDefaultRank)

	got, ok := g.TryGet("logo1")
	if !ok || got != logo {
		t.Fatalf("TryGet(logo1) = %v, %v; want uppercase entry", got, ok)
	}
}

func TestFoldedFallbackKeepsExactPriority(t *testing.T) {
	g := newGameResourceIndex("")
	upper := LooseResourceLocation("Voice", GraphicsCategory, "Voice.bmp")
	lower := LooseResourceLocation("voice", GraphicsCategory, "voice.bmp")
	g.addNamed("Voice", upper, gameDefaultRank)
	g.addNamed("voice", lower, gameDefaultRank)

	if got, _ := g.TryGet("voice"); got != lower {
		t.Fatalf("exact voice should win, got %v", got)
	}
	if got, _ := g.TryGet("Voice"); got != upper {
		t.Fatalf("exact Voice should win, got %v", got)
	}
	if got, _ := g.TryGet("VOICE"); got != upper {
		t.Fatalf("folded fallback should keep first registered entry, got %v", got)
	}
}

func TestFoldedFallbackRespectsMainRoot(t *testing.T) {
	g := newGameResourceIndex("")
	main := LooseResourceLocation("LOGO1", GraphicsCategory, "main/LOGO1.bmp")
	g.addNamed("LOGO1", main, gameDefaultRank)

	g.extraPass = true
	extra := LooseResourceLocation("Logo1", GraphicsCategory, "extra/Logo1.bmp")
	g.addNamed("Logo1", extra, gameImageRank)
	g.extraPass = false

	if got, ok := g.TryGet("logo1"); !ok || got != main {
		t.Fatalf("extra root overrode main root folded entry, got %v", got)
	}
}

func TestFoldedLinkResolvesTarget(t *testing.T) {
	dir := t.TempDir()
	linkPath := filepath.Join(dir, "Alias.link")
	if err := os.WriteFile(linkPath, []byte("logo1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := newGameResourceIndex("")
	logo := LooseResourceLocation("LOGO1", GraphicsCategory, "LOGO1.bmp")
	g.addNamed("LOGO1", logo, gameDefaultRank)
	g.addLink("Alias", LooseResourceLocation("Alias", GraphicsCategory, linkPath))

	if got, ok := g.TryGet("ALIAS"); !ok || got != logo {
		t.Fatalf("folded link did not resolve to target, got %v, %v", got, ok)
	}
}
