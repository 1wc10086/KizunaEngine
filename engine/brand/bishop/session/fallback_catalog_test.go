package session

import (
	"reflect"
	"testing"

	"kizuna/engine/brand/bishop/config"
)

func TestEvSceneBase(t *testing.T) {
	base, group, index, ok := evSceneBase("ev01_001_1p00")
	if !ok || base != "ev01_001" || group != 1 || index != 1 {
		t.Fatalf("got %q %d %d %v", base, group, index, ok)
	}
	if _, _, _, ok := evSceneBase("bg01001ab"); ok {
		t.Fatal("background must not be parsed as an EV scene")
	}
}

func TestFallbackSceneVariants(t *testing.T) {
	names := []string{
		"ev01_001_1p00", "ev01_001_1f100",
		"ev01_001_1p01", "ev01_001_1f101",
		"ev01_001_2p00", "ev01_001_2f100",
		"ev01_001_3p00", "ev01_001_3f100",
		"ev01_001_2p00m", // 差分/混合图，不能进目录
	}
	variants := fallbackSceneVariants("ev01_001", names)
	if len(variants) != 4 {
		t.Fatalf("%d variants, want 4", len(variants))
	}
	// 一个变体只含一层的身体+表情；不同层是不同角色的差分，不能叠在一起。
	want := []string{"ev01_001_1p00", "ev01_001_1f100"}
	if !reflect.DeepEqual(variants[0], want) {
		t.Fatalf("variant 0 = %v, want %v", variants[0], want)
	}
	wantSecond := []string{"ev01_001_1p01", "ev01_001_1f101"}
	if !reflect.DeepEqual(variants[1], wantSecond) {
		t.Fatalf("variant 1 = %v, want %v", variants[1], wantSecond)
	}
}

// TestFallbackEvPage 验证无目录发行版的页签映射：
// 5 名女主角 + その他，ev00（序章/共通）与 ev06（多人场景）都落在最后一页。
func TestFallbackEvPage(t *testing.T) {
	s := &Session{}
	cases := map[int]int{0: 5, 1: 0, 2: 1, 5: 4, 6: 5, 7: 5}
	for evGroup, want := range cases {
		if got := s.fallbackEvPage(evGroup, true); got != want {
			t.Fatalf("ev%02d -> page %d, want %d", evGroup, got, want)
		}
	}
}

// TestPrependCoverVariant 验证探测到的初始构图排在第一位，
// 并且和命名规则拼出的同内容变体去重。
func TestPrependCoverVariant(t *testing.T) {
	cover := []string{"ev06_001_1p00", "ev06_001_1f100", "ev06_001_1f200"}
	built := [][]string{
		{"ev06_001_1f200", "ev06_001_1p00", "ev06_001_1f100"}, // 同集合、不同顺序
		{"ev06_001_1p01", "ev06_001_1f101"},
	}
	got := prependCoverVariant(cover, built)
	if len(got) != 2 {
		t.Fatalf("%d variants, want 2 (cover + one distinct)", len(got))
	}
	if !reflect.DeepEqual(got[0], cover) {
		t.Fatalf("cover = %v, want %v", got[0], cover)
	}
	if !reflect.DeepEqual(got[1], built[1]) {
		t.Fatalf("second variant = %v, want %v", got[1], built[1])
	}
}

func TestFallbackBackgroundNames(t *testing.T) {
	names := []string{"bg01001ab", "bg01001ac", "bg01001bb", "bg02001ab", "bg02001bc", "buex_bgbase"}
	got := fallbackBackgroundNames(names)
	want := []string{"bg01001ab", "bg02001ab"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgrounds = %v, want %v", got, want)
	}
}

// TestFallbackBackgroundNamesShort 验证 SLIE 的 7 字符命名（bg0100b）也能被识别：
// 同一地点只保留一张，没有 a 版本时取第一张。
func TestFallbackBackgroundNamesShort(t *testing.T) {
	names := []string{"bg0100b", "bg0100c", "bg0100d", "bg0200c", "bg0200a", "buex_bgbase"}
	got := fallbackBackgroundNames(names)
	want := []string{"bg0100b", "bg0200a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgrounds = %v, want %v", got, want)
	}
}

// TestFallbackBackgroundNamesKdie 验证 KDIE 的"bg + 4 位编号、无后缀"命名。
//
// KDIE 没有 scene.dat，鉴赏室的背景只能从资源名推导；旧实现只认带后缀的
// 7/9 字符名字，这里会返回空列表，导致立ち絵页没有背景（纯黑）。
func TestFallbackBackgroundNamesKdie(t *testing.T) {
	names := []string{
		"bg0011", "bg0012", "bg0013", "bg0021", "bg0310",
		"buex_bgbase", "bgm01", "bg123", "bg123456",
	}
	got := fallbackBackgroundNames(names)
	want := []string{"bg0011", "bg0012", "bg0013", "bg0021", "bg0310"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgrounds = %v, want %v", got, want)
	}
}

// TestFallbackBackgroundNamesPrefersDayVariant 确认无后缀分支没有破坏
// 带后缀命名的"a 优先"规则，也确认两种命名混在一起时不会把彼此折叠掉。
func TestFallbackBackgroundNamesPrefersDayVariant(t *testing.T) {
	names := []string{"bg01001bc", "bg01001ab", "bg0011", "bg0012"}
	got := fallbackBackgroundNames(names)
	want := []string{"bg0011", "bg0012", "bg01001ab"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgrounds = %v, want %v", got, want)
	}
}

func TestBuexSwitchPositionWrapsByColumns(t *testing.T) {
	spec := config.BuexSwitchGroup{X: 4, Y: 100, SpaceX: 32, SpaceY: 32, Count: 2, Columns: 2}
	if x, y := buexSwitchPosition(spec, 2, 1); x != 36 || y != 100 {
		t.Fatalf("second switch = %d,%d; want 36,100", x, y)
	}
	// 5 颗、每行 2 颗：第 3 颗（index=2）应该换到下一行。
	if x, y := buexSwitchPosition(spec, 5, 2); x != 4 || y != 132 {
		t.Fatalf("wrapped switch = %d,%d; want 4,132", x, y)
	}
}

// TestFallbackBackgroundNamesMros 验证 mros 的 10 位背景名
// （bg + 5 位编号 + a/b + 差分字母 + 时间号）也能进鉴赏室候选。
//
// 这些名字在旧解析器里长度不匹配，全部被丢弃，立ち絵鉴赏页因此没有
// 默认背景，打开就是黑屏。
func TestFallbackBackgroundNamesMros(t *testing.T) {
	names := []string{
		"bg01001aa6", "bg01001ab6", "bg01001ac5", "bg01001ba6",
		"bg01002aa4", "bg01002ba6",
	}
	got := fallbackBackgroundNames(names)
	want := []string{"bg01001aa6", "bg01002aa4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgrounds = %v, want %v", got, want)
	}
}

// TestFallbackBackgroundNamesGbPrefix 验证牝教師2 这一代的 gb 前缀背景名。
//
// MK2/MK2IE 的 [main window] bg signature 是 gb[0-9]{5}[a-z]，旧实现只认
// bg 前缀，鉴赏室候选为空，立ち絵页打开是纯黑背景。
func TestFallbackBackgroundNamesGbPrefix(t *testing.T) {
	names := []string{
		"gb00001a", "gb00001b", "gb00002a", "gb00002b",
		"gbm01", "gb123", "buex_bgbase",
	}
	got := fallbackBackgroundNames(names)
	want := []string{"gb00001a", "gb00002a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgrounds = %v, want %v", got, want)
	}
}
