package state

// StageHitMask 是地图按钮（IMAGE/IMAGE_EX 的 type=160）的像素命中掩码。
//
// 原引擎的地图卡不是整块矩形热区：每张卡都带一张同名 <image>_hit 图，
// 只有掩码里的非零像素才算点中。多张卡同时铺在同一屏时（KZK、屈辱2），
// 用图片外框做命中会让所有卡重叠在 (0,0)，点击永远落到列表第一张卡上，
// 表现就是“点别人的卡却进第一个角色的剧情”。
//
// Values 已翻成屏幕坐标系（行 0 = 屏幕顶部），与按钮绘制原点
// (StageButton.X/Y) 的相对坐标一致；非零表示可点。
type StageHitMask struct {
	Width  int
	Height int
	Values []uint8
}

// ValueAt 返回掩码值；越界返回 0（不可点）。
func (m *StageHitMask) ValueAt(x, y int) int {
	if m == nil || x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return 0
	}
	index := (y * m.Width) + x
	if index < 0 || index >= len(m.Values) {
		return 0
	}
	return int(m.Values[index])
}

// Hit 判断按钮局部坐标是否落在可点像素上。
func (m *StageHitMask) Hit(x, y int) bool {
	return m.ValueAt(x, y) != 0
}

// Bounds 返回非零像素的外接矩形（屏幕坐标，相对按钮绘制原点）。
// 没有任何可点像素时 ok=false，调用方应把按钮当成不可点。
func (m *StageHitMask) Bounds() (x, y, width, height int, ok bool) {
	if m == nil || m.Width <= 0 || m.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	minX, minY := m.Width, m.Height
	maxX, maxY := -1, -1
	for row := 0; row < m.Height; row++ {
		base := row * m.Width
		for column := 0; column < m.Width; column++ {
			index := base + column
			if index >= len(m.Values) || m.Values[index] == 0 {
				continue
			}
			if column < minX {
				minX = column
			}
			if row < minY {
				minY = row
			}
			if column > maxX {
				maxX = column
			}
			if row > maxY {
				maxY = row
			}
		}
	}
	if maxX < minX || maxY < minY {
		return 0, 0, 0, 0, false
	}
	return minX, minY, maxX - minX + 1, maxY - minY + 1, true
}
