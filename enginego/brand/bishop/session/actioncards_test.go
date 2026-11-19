package session

import "testing"

// TestActionCardLayoutMatchesScript 锁住学園3/学園3IE 的 4…9 张卡网格。
//
// 这些坐标直接来自 map_main 末尾的 LAYER_PARAM 列表；行动卡双卡分支
// 只加 1 次 #layercount 时，引擎要靠这张表把实际多出来的卡补到正确卡位。
func TestActionCardLayoutMatchesScript(t *testing.T) {
	cases := []struct {
		count int
		want  []actionCardPoint
	}{
		{4, []actionCardPoint{{144, 154}, {272, 154}, {400, 154}, {528, 154}}},
		{5, []actionCardPoint{{80, 154}, {208, 154}, {336, 154}, {464, 154}, {592, 154}}},
		{6, []actionCardPoint{
			{186, 54}, {336, 54}, {486, 54},
			{186, 254}, {336, 254}, {486, 254},
		}},
		{7, []actionCardPoint{
			{186, 54}, {336, 54}, {486, 54},
			{144, 254}, {272, 254}, {400, 254}, {528, 254},
		}},
		{8, []actionCardPoint{
			{144, 54}, {272, 54}, {400, 54}, {528, 54},
			{144, 254}, {272, 254}, {400, 254}, {528, 254},
		}},
		{9, []actionCardPoint{
			{144, 54}, {272, 54}, {400, 54}, {528, 54},
			{80, 254}, {208, 254}, {336, 254}, {464, 254}, {592, 254},
		}},
	}
	for _, test := range cases {
		got := actionCardLayout(test.count)
		if len(got) != len(test.want) {
			t.Fatalf("layout(%d) = %d points, want %d", test.count, len(got), len(test.want))
		}
		for index := range got {
			if got[index] != test.want[index] {
				t.Fatalf("layout(%d)[%d] = %+v, want %+v", test.count, index, got[index], test.want[index])
			}
		}
	}
}

func TestActionCardLayoutUnknownCount(t *testing.T) {
	if got := actionCardLayout(10); got != nil {
		t.Fatalf("layout(10) = %d points, want nil", len(got))
	}
}
