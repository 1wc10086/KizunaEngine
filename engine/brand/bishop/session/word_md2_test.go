package session

import "testing"

// TestWordRankFor 锁定 MD2 的弱点映射：0..3 取对应属性，
// 4 是愛莉的“全属性”哨兵，取四字母里最高的一级。
// 若这里退回固定 C，愛莉的每句话都会 +2 压力，通关回合会被 stressmax 抢先。
func TestWordRankFor(t *testing.T) {
	s := &Session{words: []md2Word{{Code: [4]byte{'S', 'A', 'B', 'C'}}}}
	cases := []struct {
		weak int
		want byte
	}{
		{0, 'S'},
		{1, 'A'},
		{2, 'B'},
		{3, 'C'},
		{4, 'S'},
		{-1, 'C'},
		{9, 'C'},
	}
	for _, tc := range cases {
		if got := s.wordRankFor(1, tc.weak); got != tc.want {
			t.Fatalf("weak=%d rank=%c, want %c", tc.weak, got, tc.want)
		}
	}
	if got := s.wordRankFor(99, 0); got != 'C' {
		t.Fatalf("out-of-range word rank=%c, want C", got)
	}
}

func TestWordSelectedRejectsDuplicates(t *testing.T) {
	selected := []int{3, 5}
	if !wordSelected(selected, 3) || wordSelected(selected, 4) {
		t.Fatal("wordSelected returned wrong result")
	}
}
