package vm

import (
	"testing"
)

// fakeLoader 用内存里的脚本响应名字查询。
type fakeLoader map[string][]byte

func (f fakeLoader) load(name string) ([]byte, error) {
	data, ok := f[name]
	if !ok {
		return nil, errNotFound
	}
	return data, nil
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errNotFound = constErr("脚本不存在")

func TestAtoiOr(t *testing.T) {
	if got := atoiOr("-12", 0); got != -12 {
		t.Fatalf("-12 -> %d", got)
	}
	if got := atoiOr(" 007 ", 0); got != 7 {
		t.Fatalf("007 -> %d", got)
	}
	if got := atoiOr("x", 5); got != 5 {
		t.Fatalf("非法输入应返回默认值，实得 %d", got)
	}
}

func TestAtofOr(t *testing.T) {
	if got := atofOr("0.68", 1); got < 0.679 || got > 0.681 {
		t.Fatalf("0.68 -> %f", got)
	}
	if got := atofOr("-0.5", 1); got != -0.5 {
		t.Fatalf("-0.5 -> %f", got)
	}
}

func TestSlotXLayout(t *testing.T) {
	cases := []struct {
		count, index, want int
	}{
		{1, 0, 0},
		{2, 0, -300},
		{2, 1, 300},
		{3, 0, -300},
		{3, 1, 300},
		{3, 2, 0},
	}
	for _, c := range cases {
		if got := slotX(c.count, c.index); got != c.want {
			t.Fatalf("slotX(%d,%d) = %d，期望 %d", c.count, c.index, got, c.want)
		}
	}
}

func TestSplitPair(t *testing.T) {
	a, b := splitPair("title3,title3_on")
	if a != "title3" || b != "title3_on" {
		t.Fatalf("splitPair = %q, %q", a, b)
	}
	a, b = splitPair("menu")
	if a != "menu" || b != "" {
		t.Fatalf("无逗号时 = %q, %q", a, b)
	}
}
