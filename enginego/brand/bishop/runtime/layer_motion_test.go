package runtime

import "testing"

func TestIsAheAnimation(t *testing.T) {
	cases := map[string]bool{
		"A-H-E":     true,
		"a-h-e.txt": false, // 后缀由调用方补，这里只认脚本名
		"a-h-e_01":  true,
		"deal":      false,
		"turn":      false,
	}
	for name, want := range cases {
		if got := isAheAnimation(name); got != want {
			t.Fatalf("isAheAnimation(%q) = %v, want %v", name, got, want)
		}
	}
}
