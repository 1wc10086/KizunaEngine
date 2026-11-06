package session

import (
	"testing"

	"kizuna/engine/brand/bishop/gfx"
)

// 牝教師3 的 museum_ch1..7 是 CG 页的 64x267 三帧页签条，
// 和すくーるヘブン 的 237x208 角色选择卡同名。只按资源名命中会把
// 页签条当卡片画成两列重复头像，必须用尺寸区分。
func TestSHStripCardSignature(t *testing.T) {
	cases := []struct {
		width, height int
		want          bool
	}{
		{237, 208, true},
		{64, 267, false},
		{88, 150, false},
	}
	for _, item := range cases {
		image := &gfx.BsgImage{Width: item.width, Height: item.height}
		if got := isSHStripCard(image); got != item.want {
			t.Fatalf("isSHStripCard(%dx%d) = %v, want %v", item.width, item.height, got, item.want)
		}
	}
	if isSHStripCard(nil) {
		t.Fatal("nil image must not be a SH strip card")
	}
}
