package config

import (
	"strconv"
	"strings"
)

// chart.go 解析 [main window] chart db 指向的场景图数据库（define.bsa:chart.db）。
//
// CHART 是原引擎给玩家的"流程地图"：剧本每推进到一场，地图上对应的格子就点亮
// 一张缩略图，格与格之间用连线标出分支和去向。画面的底图、按钮、确认框写在
// define.bsa:status_layout 的 chart 页里（见 status_screen.go），本文件只负责
// chart.db 的数据：每页的格子（block）和连线（line）。
//
// 坐标空间有两点必须记住，否则渲染会整体错位：
//   - 场景图按 1024 宽的虚拟画布排版。chart.db 里 line 的坐标属于这个 1024
//     空间，渲染时把 line 的点按 screenWidth/1024 缩放，就落回各作的原生坐标。
//   - block 的 pos 已经是各作的原生屏幕坐标（YK 1366x768、Yakata 1024x576），
//     缩放后的 line 坐标正好落在 block 上。因此 block 的位置绝对不能再缩放，
//     也不要拿 line 的坐标去反推 block 的位置。
//
// 数据里的数字没有需要引擎解释的语义：block id 是原引擎的格子编号，thumb 名
// 直接对应图片资源。这里只搬运和拆分，不重新编号——编号和资源帧是一一对应的。

// defaultChartThumbRatio 是布局没写 thumb_ratio 时缩略图的缩放比例。
// 数值取自原引擎的默认值（YK/Yakata 都写 .094，所以缺省也按它算）。
const defaultChartThumbRatio = 0.094

// ChartDefaultThumbRatio 是上面的导出别名：session 层计算缩略图尺寸时
// 需要同一个默认值，避免两边各写一个 0.094 后逐渐漂移。
const ChartDefaultThumbRatio = defaultChartThumbRatio

// ChartThumb 是格子里的一个缩略图。
type ChartThumb struct {
	Name string
	// OffsetX 是缩略图相对格子左上角的水平偏移，来自 "名,341" 逗号后的数字。
	OffsetX int
	// Special 表示名字前面带 '*'（例如 *chart_next_block）。这类图不是普通
	// 缩略图，而是原引擎的特殊标记（翻页/终点等），渲染时按特殊格子处理。
	Special bool
}

// ChartBlock 是地图上的一个格子：位置、编号文字和缩略图。
type ChartBlock struct {
	ID    int
	X     int
	Y     int
	Label string
	// Thumbs 按书写顺序保存：背景在前、立绘在后，后画的压在前面的上面。
	Thumbs []ChartThumb
}

// ChartAnchor 是连线的端点。
//
// ID 指向的是格子上的接点（例如 0x01000305），不是 block 的 id；pos 是该接点
// 在 1024 空间里的坐标。两者都原样保留，渲染时按 ID 找格子、按 pos 画线。
type ChartAnchor struct {
	ID int
	X  int
	Y  int
}

// ChartLine 是两个格子之间的一条连线。Points 是折点，直线时为空。
type ChartLine struct {
	ID          int
	Source      ChartAnchor
	Destination ChartAnchor
	// Points 是 1024 空间里的折点，按书写顺序连成折线。
	Points []UIPoint
}

// ChartPage 是地图的一页。原引擎按页翻看整张流程图。
type ChartPage struct {
	Blocks []ChartBlock
	Lines  []ChartLine
}

// ChartDatabase 是 chart.db 的顶层结构。
type ChartDatabase struct {
	Pages []ChartPage
}

// ParseChartDatabase 解析 chart.db。缺失的可选字段得到零值，只有语法错误
// 才返回 error——某一格少了 thumb 不该让整个 CHART 画面开不起来。
func ParseChartDatabase(data []byte) (*ChartDatabase, error) {
	root, err := ParseLayoutDocument(data)
	if err != nil {
		return nil, err
	}

	database := &ChartDatabase{}
	pages, _ := Array(root["page"])
	for _, item := range pages {
		object, ok := Object(item)
		if !ok {
			continue
		}
		database.Pages = append(database.Pages, parseChartPage(object))
	}
	return database, nil
}

func parseChartPage(object map[string]any) ChartPage {
	page := ChartPage{}
	if blocks, ok := Array(object["block"]); ok {
		page.Blocks = make([]ChartBlock, 0, len(blocks))
		for _, item := range blocks {
			entry, ok := Object(item)
			if !ok {
				continue
			}
			block := ChartBlock{}
			block.ID, _ = Number(entry["id"])
			block.Label, _ = Text(entry["label"])
			if pos, ok := Array(entry["pos"]); ok {
				block.X, block.Y = layoutPosition(pos)
			}
			if property, ok := Object(entry["property"]); ok {
				block.Thumbs = parseChartThumbs(property["thumb"])
			}
			page.Blocks = append(page.Blocks, block)
		}
	}
	if lines, ok := Array(object["line"]); ok {
		page.Lines = make([]ChartLine, 0, len(lines))
		for _, item := range lines {
			entry, ok := Object(item)
			if !ok {
				continue
			}
			line := ChartLine{}
			line.ID, _ = Number(entry["id"])
			line.Source = parseChartAnchor(entry["source"])
			line.Destination = parseChartAnchor(entry["destination"])
			line.Points = parseChartPoints(entry["points"])
			page.Lines = append(page.Lines, line)
		}
	}
	return page
}

// parseChartThumbs 解析 property.thumb。
//
// 原数据有两种写法：单个字符串，或若干字符串组成的数组（一个格子里叠一张
// 背景和若干立绘）。数组顺序就是绘制顺序，必须保留。
func parseChartThumbs(value any) []ChartThumb {
	switch typed := value.(type) {
	case string:
		if thumb, ok := parseChartThumb(typed); ok {
			return []ChartThumb{thumb}
		}
	case []any:
		thumbs := make([]ChartThumb, 0, len(typed))
		for _, item := range typed {
			text, ok := Text(item)
			if !ok {
				continue
			}
			if thumb, ok := parseChartThumb(text); ok {
				thumbs = append(thumbs, thumb)
			}
		}
		return thumbs
	}
	return nil
}

// parseChartThumb 拆分一个缩略图名："bu01f0011a02,341" -> 名字 + 水平偏移。
//
// 偏移只影响这张图相对格子左上角的位置，正负都出现过（立绘分列两侧）。
// 是个画在名字前的 '*' 表示特殊标记，星号本身不是资源名的一部分，要剥掉。
func parseChartThumb(text string) (ChartThumb, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ChartThumb{}, false
	}

	thumb := ChartThumb{}
	if strings.HasPrefix(text, "*") {
		thumb.Special = true
		text = strings.TrimPrefix(text, "*")
	}
	if comma := strings.IndexByte(text, ','); comma >= 0 {
		// 偏移解不出来时按 0 处理：图仍然要画，不能因为一个坏数字整格消失。
		if offset, err := strconv.Atoi(strings.TrimSpace(text[comma+1:])); err == nil {
			thumb.OffsetX = offset
		}
		text = text[:comma]
	}
	thumb.Name = strings.TrimSpace(text)
	if thumb.Name == "" {
		return ChartThumb{}, false
	}
	return thumb, true
}

func parseChartAnchor(value any) ChartAnchor {
	anchor := ChartAnchor{}
	object, ok := Object(value)
	if !ok {
		return anchor
	}
	anchor.ID, _ = Number(object["id"])
	if pos, ok := Array(object["pos"]); ok {
		anchor.X, anchor.Y = layoutPosition(pos)
	}
	return anchor
}

// parseChartPoints 解析连线的折点。大多数连线是直的，没有 points；
// 有折点时每个点是 [x, y]，同样属于 1024 空间。
func parseChartPoints(value any) []UIPoint {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	points := make([]UIPoint, 0, len(array))
	for _, item := range array {
		pair, ok := Array(item)
		if !ok || len(pair) < 2 {
			continue
		}
		x, _ := Number(pair[0])
		y, _ := Number(pair[1])
		points = append(points, UIPoint{X: x, Y: y})
	}
	return points
}
