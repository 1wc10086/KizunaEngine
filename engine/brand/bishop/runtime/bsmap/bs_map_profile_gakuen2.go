package bsmap

import (
	"math"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

type BsMapProfileGakuen2 struct{}

const (
	BsMapGakuen2HitMaskImage    = "map_hit"
	BsMapGakuen2BackgroundImage = "map_base"
	BsMapGakuen2Source          = "gakuen2-rev2"
	BsMapGakuen2FirstMaskValue  = 0
	// 0..9 是十个可去的地点；10 是左下角的「時間を渡す」。
	BsMapGakuen2LastMaskValue  = 9
	BsMapGakuen2TimeMaskValue  = 10
	BsMapGakuen2HitPadding     = 3
	BsMapGakuen2MaskValueCount = BsMapGakuen2LastMaskValue - BsMapGakuen2FirstMaskValue + 1
	BsMapGakuen2MaxMaskValue   = 255
	BsMapGakuen2NoItemIndex    = -1
	BsMapGakuen2TimeProperty   = "#time"
	// 时间推进到 10 之后（放課後終了/夜/深夜）地图上的地点不可再点：
	// 剧本的 map_s1f..map_byd 只处理 #time 6..9 的分支，此时再点地点会
	// 顺着标签链落到别的楼层，最后空转回 main。原作这时只留「時間を渡す」。
	BsMapGakuen2ClosedTime = 10
	// 「時間を渡す」把 #time 直接推进到 10；syscall map 返回后剧本用
	// #time==10/11/12 跳到 after_01/night/midnight。若在这里 seek
	// map_tim（它只是 jump main），main 会再 +1 变成 11，跳过 after_01。
	BsMapGakuen2TimeAdvance = 10
)

const bsMapGakuen2IndexedPixelStride = 1

var bsMapGakuen2PlaceLabels = []string{
	"map_s1f",
	"map_s2f",
	"map_s3f",
	"map_c1f",
	"map_c2f",
	"map_c3f",
	"map_rof",
	"map_gym",
	"map_grd",
	"map_byd",
}

// 角色页按此顺序检查，先为 1 的角色优先；下标与 bsMapGakuen2RoleLabels 一一对应。
var bsMapGakuen2RoleProperties = []string{
	"#hrn_map",
	"#mik_map",
	"#kwr_map",
	"#izm_map",
	"#ayn_map",
	"#kyk_map",
}

var bsMapGakuen2RoleLabels = []string{
	"map_hrn",
	"map_mik",
	"map_kwr",
	"map_izm",
	"map_ayn",
	"map_kyk",
}

// 侵攻度属性，下标与 bsMapGakuen2RoleProperties 一一对应。
var bsMapGakuen2RoleSinkouProperties = []string{
	"#hrn_sinkou",
	"#mik_sinkou",
	"#kwr_sinkou",
	"#izm_sinkou",
	"#ayn_sinkou",
	"#kyk_sinkou",
}

func (p *BsMapProfileGakuen2) Name() string {
	return "Gakuen2"
}

func (p *BsMapProfileGakuen2) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	return context.Dialect == bs.BsScriptRev2 &&
		context.Resources.Exists(BsMapGakuen2HitMaskImage) &&
		context.Resources.Exists(BsMapGakuen2BackgroundImage)
}

func (p *BsMapProfileGakuen2) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map gakuen2: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapGakuen2Layout(context)
}

func createBsMapGakuen2Layout(context BsMapContext) *BsMapLayout {
	width, height, mask, ok := tryLoadBsMapGakuen2Mask(context)
	if !ok {
		core.Logf("script: map gakuen2: '%s' is not a readable 8-bit indexed graphic", BsMapGakuen2HitMaskImage)
		return nil
	}
	core.Logf("script: map gakuen2: '%s' %dx%d indexed mask; BsMapMask.Regions cannot read it (the palette sets alpha 0 on every entry, and Regions skips alpha 0, so every channel yields 0 region), so the raw index buffer is decoded and used; mask %d..%d are the ten clickable places, %d is the «時間を渡す» button, other values are edge noise", BsMapGakuen2HitMaskImage, width, height, BsMapGakuen2FirstMaskValue, BsMapGakuen2LastMaskValue, BsMapGakuen2TimeMaskValue)
	places, values, summary := bsMapGakuen2PlaceItems(context, width, height, mask)
	timeItem, timeOK := bsMapGakuen2TimeItem(context, width, height, mask)
	if len(places) == 0 && !timeOK {
		return nil
	}
	if timeOK {
		places = append(places, timeItem)
		values = append(values, BsMapGakuen2TimeMaskValue)
		summary = append(summary, "'時間を渡す' -> #time=10")
	}
	core.Logf("script: map gakuen2: %d item(s) %s", len(places), strings.Join(summary, ", "))
	hitMask := bsMapGakuen2MaskData(width, height, mask, places, values)
	// 目标头像不进装饰层：它要压在 map_on（地点描边）之上、map_site（准星）
	// 之下，而装饰层是舞台图层，绘制顺序和 hover 覆盖层混不到一起。
	// 由 session.drawGK2MapFaces 在合成后按 MAPPRMS.DAT 的坐标表直接画。
	decor := bsMapGakuen2DateDecor(context)
	return &BsMapLayout{Background: BsMapGakuen2BackgroundImage, Items: places, Decor: decor, Source: BsMapGakuen2Source, Mask: hitMask}
}

// bsMapGakuen2TimeItem 生成「時間を渡す」按钮。
//
// 它不是地点：点击后要把 #time 推进到 10 并让 syscall map 正常返回，
// 剧本的后续分支才会跳 after_01。早期的实现把它当成 map_tim 标签，
// 那个标签只是 JUMP main，main 再把 #time +1 成 11，直接跳过傍晚剧情，
// 而且 #time 很快超过 12，地图变成死循环。
func bsMapGakuen2TimeItem(context BsMapContext, width, height int, mask []byte) (BsMapItem, bool) {
	box, ok := bsMapGakuen2MaskBox(mask, width, height, BsMapGakuen2TimeMaskValue)
	if !ok {
		return BsMapItem{}, false
	}
	hitX, hitY, hitWidth, hitHeight := bsMapGakuen2HitBox(box.minX, box.minY, box.maxX-box.minX+1, box.maxY-box.minY+1, width, height)
	item := NewBsMapItem("", 0, box.minX, box.minY, hitX, hitY, hitWidth, hitHeight, "")
	item.Property = BsMapGakuen2TimeProperty
	item.PropertyKey = -1
	item.Value = BsMapGakuen2TimeAdvance
	return item, true
}

// bsMapGakuen2MaskBox 求某个掩码值的屏幕外接矩形。
func bsMapGakuen2MaskBox(mask []byte, width, height, value int) (bsMapGakuen2Box, bool) {
	box := bsMapGakuen2Box{math.MaxInt32, math.MaxInt32, -1, -1, 0}
	for row := 0; row < height; row++ {
		offset := row * width
		screenY := (height - 1) - row
		for column := 0; column < width; column++ {
			if int(mask[offset+column]) != value {
				continue
			}
			box.minX = min(box.minX, column)
			box.minY = min(box.minY, screenY)
			box.maxX = max(box.maxX, column)
			box.maxY = max(box.maxY, screenY)
			box.count++
		}
	}
	return box, box.count > 0
}

// bsMapGakuen2DateDecor 生成地图左上角日期栏的覆盖层。
//
// map_base 只画了「第 週 （ ） 放課後-残」的固定字，数字/星期/残数要由
// 引擎按 #day/#time 取 map_date/mw_date 的帧贴上去：
//   week      = (day-1)/7+1     -> map_date 第 0..4 帧（1..5）
//   weekday   = (day-1)%7       -> map_date 第 5..11 帧（月..日）
//   remaining = 10-time         -> mw_date  第 0..9 帧
//
// 坐标对齐 map_base 上的空白：第_週 的空格在 x≈32、（）之间在 x≈104、
// 残 后面的数字在 x≈230。只有 day>0 才画周数/星期（序章日 #day=0）。
func bsMapGakuen2DateDecor(context BsMapContext) []BsMapItem {
	day := context.Properties.ByName("#day")
	time := context.Properties.ByName(BsMapGakuen2TimeProperty)
	decor := make([]BsMapItem, 0, 3)
	if day > 0 {
		week := (day - 1) / 7
		weekday := (day - 1) % 7
		if week >= 0 && week < 5 && context.Resources.Exists("map_date") {
			decor = append(decor, newBsMapGakuen2Decor("map_date", week, 32, 30))
		}
		if weekday >= 0 && weekday < 7 && context.Resources.Exists("map_date") {
			decor = append(decor, newBsMapGakuen2Decor("map_date", 5+weekday, 104, 30))
		}
	}
	if time >= 0 && time <= 10 && context.Resources.Exists("mw_date") {
		remaining := 10 - time
		if remaining >= 0 && remaining <= 9 {
			decor = append(decor, newBsMapGakuen2Decor("mw_date", remaining, 230, 26))
		}
	}
	return decor
}

// MAPPRMS.DAT 0x68 起的 28 组 (int16 x, int16 y) 是原引擎绘制目标头像的
// 坐标表：1..7 人各一组，y 恒为 250，x 以 360 为起点、每人间隔 100，取的是
// 头像图左上角（官方地图截图里头像中心落在 400，正是 360+80/2）。
// 1..6 是六名女主角，第 7 个位置留给「?」（map_face 的 6/13 帧）。
var bsMapGakuen2FaceSlots = [7][7][2]int{
	{{360, 250}},
	{{310, 250}, {410, 250}},
	{{260, 250}, {360, 250}, {460, 250}},
	{{210, 250}, {310, 250}, {410, 250}, {510, 250}},
	{{160, 250}, {260, 250}, {360, 250}, {460, 250}, {560, 250}},
	{{110, 250}, {210, 250}, {310, 250}, {410, 250}, {510, 250}, {610, 250}},
	{{60, 250}, {160, 250}, {260, 250}, {360, 250}, {460, 250}, {560, 250}, {660, 250}},
}

const (
	// BsMapGakuen2FaceImage 是地图上的目标头像条图（80x1204，14 帧）。
	BsMapGakuen2FaceImage = "map_face"
	// BsMapGakuen2FaceCellHeight 是头像条的帧高。BSG 头写的是 FrameCount=1，
	// 不能靠 FrameCount 推导；整条 1204px 画出去会滚到屏幕外。
	BsMapGakuen2FaceCellHeight = 86
	// BsMapGakuen2FaceMaxCount 是坐标表能容纳的最大人数。
	BsMapGakuen2FaceMaxCount = 7
	// BsMapGakuen2RoleCount 是地图上可能出现的目标人数（六名女主角）。
	BsMapGakuen2RoleCount = 6
)

// BsMapGakuen2RoleProperty 返回第 index 名女主角的「今天在地图上」标记。
func BsMapGakuen2RoleProperty(index int) string {
	if index < 0 || index >= len(bsMapGakuen2RoleProperties) {
		return ""
	}
	return bsMapGakuen2RoleProperties[index]
}

// BsMapGakuen2RoleSinkouProperty 返回第 index 名女主角的侵攻度属性。
//
// MAPPRMS.DAT 的条件程序同时读 #X_sinkou 和 #X_map：前者是 0..6 的路线进度，
// 后者是剧本在事件里置 1、选完地点清 0 的「今天还有事件」。只按 #X_map 判断，
// 序章和没触发事件的时段整排头像都是空的；原版只要路线已经开始（sinkou>=1）
// 就会把人列在地图上。
func BsMapGakuen2RoleSinkouProperty(index int) string {
	if index < 0 || index >= len(bsMapGakuen2RoleSinkouProperties) {
		return ""
	}
	return bsMapGakuen2RoleSinkouProperties[index]
}

// BsMapGakuen2FaceSlot 返回第 slot 个头像的左上角坐标（共 count 人时）。
func BsMapGakuen2FaceSlot(count, slot int) (int, int, bool) {
	if count < 1 || count > len(bsMapGakuen2FaceSlots) {
		return 0, 0, false
	}
	row := bsMapGakuen2FaceSlots[count-1]
	if slot < 0 || slot >= len(row) {
		return 0, 0, false
	}
	return row[slot][0], row[slot][1], true
}


func newBsMapGakuen2Decor(image string, frame, x, y int) BsMapItem {
	item := NewBsMapItem(image, frame, x, y, 0, 0, 0, 0, "")
	// 学園2 的条图头部 FrameCount 都是 1，渲染层拿不到帧数；这里按素材
	// 实际高度/帧数写死帧高，否则 map_face 会把整条 1204px 画到屏幕上。
	switch image {
	case "map_face":
		item.CellHeight = 86
	case "map_date":
		item.CellHeight = 27
	case "mw_date":
		item.CellHeight = 32
	case "date_info":
		item.CellHeight = 18
	}
	return item
}

func bsMapGakuen2MaskData(width, height int, mask []byte, items []BsMapItem, values []int) (result *BsMapMaskData) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map gakuen2: pixel hit mask build failed: %s, pixel hit disabled, the bounding boxes stay in use", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	if width <= 0 || height <= 0 || len(mask) != width*height {
		core.Logf("script: map gakuen2: '%s' declares %dx%d but holds %d bytes, pixel hit disabled, the bounding boxes stay in use", BsMapGakuen2HitMaskImage, width, height, len(mask))
		return nil
	}
	screen := make([]byte, len(mask))
	for row := 0; row < height; row++ {
		source := row * width
		target := ((height - 1) - row) * width
		for column := 0; column < width; column++ {
			screen[target+column] = mask[source+column]
		}
	}
	itemForValue := make([]int, BsMapGakuen2MaxMaskValue+1)
	for i := range itemForValue {
		itemForValue[i] = BsMapGakuen2NoItemIndex
	}
	clickable := 0
	for index := range items {
		value := values[index]
		// #time>=10 时地点被收成 0x0 热区，但像素掩码仍然会把这些值映射到
		// item；不跳过的话，玩家的点击会绕过“只剩時間を渡す”的日终状态。
		if items[index].HitWidth <= 0 || items[index].HitHeight <= 0 {
			continue
		}
		if value < BsMapGakuen2FirstMaskValue || value > BsMapGakuen2MaxMaskValue || itemForValue[value] != BsMapGakuen2NoItemIndex {
			continue
		}
		itemForValue[value] = index
		clickable++
	}
	core.Logf("script: map gakuen2: pixel hit mask %dx%d in screen coordinates ('%s' flipped so that row 0 is the screen top, the frame the mouse and BsMapItem.HitY use), %d value(s) of %d..%d resolve to a place, every other value is not clickable", width, height, BsMapGakuen2HitMaskImage, clickable, BsMapGakuen2FirstMaskValue, BsMapGakuen2LastMaskValue)
	return &BsMapMaskData{Width: width, Height: height, Values: screen, ItemForValue: itemForValue}
}

type bsMapGakuen2Box struct {
	minX, minY, maxX, maxY, count int
}

func bsMapGakuen2PlaceItems(context BsMapContext, width, height int, mask []byte) ([]BsMapItem, []int, []string) {
	bounds := make([]bsMapGakuen2Box, BsMapGakuen2MaskValueCount)
	for index := range bounds {
		bounds[index] = bsMapGakuen2Box{math.MaxInt32, math.MaxInt32, -1, -1, 0}
	}
	for row := 0; row < height; row++ {
		offset := row * width
		screenY := (height - 1) - row
		for column := 0; column < width; column++ {
			value := int(mask[offset+column])
			if value > BsMapGakuen2LastMaskValue {
				continue
			}
			box := &bounds[value]
			if column < box.minX {
				box.minX = column
			}
			if screenY < box.minY {
				box.minY = screenY
			}
			if column > box.maxX {
				box.maxX = column
			}
			if screenY > box.maxY {
				box.maxY = screenY
			}
			box.count++
		}
	}
	items := make([]BsMapItem, 0, BsMapGakuen2MaskValueCount)
	values := make([]int, 0, BsMapGakuen2MaskValueCount)
	summary := make([]string, 0, BsMapGakuen2MaskValueCount)
	// #time>=10 是放課後結束/夜/深夜：地点剧情只覆盖 6..9，此时把地点
	// 的命中框收成 0，玩家只能按「時間を渡す」继续，和原作的日终地图一致。
	closed := context.Properties.ByName(BsMapGakuen2TimeProperty) >= BsMapGakuen2ClosedTime
	// 角色属性在 build 时一次性读取，避免每个地点重复查询。
	roleValues := bsMapGakuen2RoleValues(context)
	core.Logf("script: map gakuen2: 角色属性 %s", bsMapGakuen2RoleSummary(roleValues))
	for value := BsMapGakuen2FirstMaskValue; value <= BsMapGakuen2LastMaskValue; value++ {
		placeLabel := bsMapGakuen2PlaceLabels[value-BsMapGakuen2FirstMaskValue]
		label := placeLabel
		box := bounds[value]
		if box.count == 0 {
			core.Logf("script: map gakuen2: mask value %d ('%s') has no pixel, skipped", value, placeLabel)
			continue
		}
		if context.Script.FindLabel(placeLabel) < 0 {
			core.Logf("script: map gakuen2: label '%s' is missing, mask value %d skipped", placeLabel, value)
			continue
		}
		label, role := bsMapGakuen2RouteLabel(context, roleValues, placeLabel)
		if routeIndex := bsMapGakuen2RoleIndex(role); routeIndex >= 0 {
			core.Logf("script: map gakuen2: mask value %d place '%s' -> '%s' (%s=1)", value, placeLabel, label, role)
		} else {
			core.Logf("script: map gakuen2: mask value %d place '%s' -> '%s' (no role map flag)", value, placeLabel, label)
		}
		boxWidth := box.maxX - box.minX + 1
		boxHeight := box.maxY - box.minY + 1
		hitX, hitY, hitWidth, hitHeight := bsMapGakuen2HitBox(box.minX, box.minY, boxWidth, boxHeight, width, height)
		if closed {
			hitX, hitY, hitWidth, hitHeight = 0, 0, 0, 0
		}
		items = append(items, NewBsMapItem("", 0, box.minX, box.minY, hitX, hitY, hitWidth, hitHeight, label))
		values = append(values, value)
		summary = append(summary, "mask "+strconv.Itoa(value)+" '"+label+"' bbox ("+strconv.Itoa(box.minX)+","+strconv.Itoa(box.minY)+") "+strconv.Itoa(boxWidth)+"x"+strconv.Itoa(boxHeight)+" hit ("+strconv.Itoa(hitX)+","+strconv.Itoa(hitY)+","+strconv.Itoa(hitWidth)+","+strconv.Itoa(hitHeight)+")")
	}
	return items, values, summary
}

// bsMapGakuen2RoleValues 一次性读取六个角色的 #X_map 标记。
//
// 原引擎的 MapPrms 块 B 对每个地点都会比较这些属性；属性不存在时
// ByName 返回 0，正好等价于“该角色今天不在地图上”。
func bsMapGakuen2RoleValues(context BsMapContext) []int {
	values := make([]int, len(bsMapGakuen2RoleProperties))
	for index, name := range bsMapGakuen2RoleProperties {
		values[index] = context.Properties.ByName(name)
	}
	return values
}

// bsMapGakuen2RoleSummary 只用于日志，列出六个角色标记的当前值。
func bsMapGakuen2RoleSummary(values []int) string {
	parts := make([]string, 0, len(values))
	for index, value := range values {
		if value == 0 {
			continue
		}
		parts = append(parts, bsMapGakuen2RoleProperties[index]+"="+strconv.Itoa(value))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// bsMapGakuen2RouteLabel 按原版块 B 的顺序选择地点目标。
//
// 第一个 #X_map == 1 的角色把地点改写成 map_hrn…map_kyk；没有角色时
// 退回地点自身的标签。标签不存在时继续看下一个角色，避免跳进空标签。
func bsMapGakuen2RouteLabel(context BsMapContext, values []int, fallback string) (string, string) {
	for index, value := range values {
		if value != 1 {
			continue
		}
		if context.Script.FindLabel(bsMapGakuen2RoleLabels[index]) < 0 {
			continue
		}
		return bsMapGakuen2RoleLabels[index], bsMapGakuen2RoleProperties[index]
	}
	return fallback, ""
}

// bsMapGakuen2RoleIndex 把属性名反查成下标；空名返回 -1。
func bsMapGakuen2RoleIndex(property string) int {
	if property == "" {
		return -1
	}
	for index, name := range bsMapGakuen2RoleProperties {
		if name == property {
			return index
		}
	}
	return -1
}

func bsMapGakuen2HitBox(x, y, width, height, limitWidth, limitHeight int) (int, int, int, int) {
	left := max(0, x-BsMapGakuen2HitPadding)
	top := max(0, y-BsMapGakuen2HitPadding)
	right := min(limitWidth-1, x+width-1+BsMapGakuen2HitPadding)
	bottom := min(limitHeight-1, y+height-1+BsMapGakuen2HitPadding)
	return left, top, right - left + 1, bottom - top + 1
}

func tryLoadBsMapGakuen2Mask(context BsMapContext) (int, int, []byte, bool) {
	location, ok := context.Resources.TryGet(BsMapGakuen2HitMaskImage)
	if !ok {
		return 0, 0, nil, false
	}
	data := location.Read()
	if len(data) == 0 {
		return 0, 0, nil, false
	}
	if !gfx.IsBsgGraphics(data, 0) {
		return 0, 0, nil, false
	}
	header, err := gfx.ParseBsgHeader(data, 0)
	if err != nil {
		return 0, 0, nil, false
	}
	if header.ColorMode != gfx.BsgColorModeIndexed8 {
		return 0, 0, nil, false
	}
	buffer := make([]byte, header.PixelCount())
	source := data[header.DataOffset : header.DataOffset+header.DataSize]
	switch header.Compression {
	case gfx.BsgCompressionNone:
		copy(buffer, source[:min(len(source), len(buffer))])
	case gfx.BsgCompressionLz:
		gfx.UnpackBsgLz(source, buffer, 0, bsMapGakuen2IndexedPixelStride)
	default:
		gfx.UnpackBsgRle(source, buffer, 0, bsMapGakuen2IndexedPixelStride)
	}
	return header.Width, header.Height, buffer, true
}


