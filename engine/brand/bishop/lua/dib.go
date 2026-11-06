package lua

import (
	"math"

	lua "github.com/yuin/gopher-lua"

	"kizuna/engine/brand/bishop/gfx"
)

const (
	keyA  = 0x41 // 'A'：确定/执行
	keyB  = 0x42 // 'B'：取消/返回
	keyWU = 0x3c // 滚轮上
	keyWD = 0x3e // 滚轮下
)

// dib 是 Lua 看到的一张图或一块画布。
//
// DIB.Create 返回的图带 **内嵌绘制偏移** (x,y)：脚本写
// `DIB.Create("status_icon1", 2, x, y)` 后统一用 `DIBCopy(target,0,0,img)`，
// 原引擎会把 x/y 加到目标坐标上。忽略这个偏移会让所有带位置创建的图
// 叠在左上角。canvas 非 nil 时表示 session 传进来的目标画布。
type dib struct {
	image  *gfx.BsgImage
	name   string
	canvas *gfx.BgraCanvas
	frames int
	state  int
	x, y   int
	// hasPos 表示脚本显式传了 x/y（哪怕是 0,0）。
	//
	// 旧版 Lua 大量写 `DIB.Create("tra_menu_base")`（不传坐标）再
	// `DIBCopy(target,0,0,dib)`，位置来自图片自身的内嵌偏移（BSG 头部
	// 的 offset_x/offset_y）。把它们当成 (0,0) 会让训练/领地界面所有
	// 面板叠在左上角；显式传了坐标的图则必须用脚本给的坐标。
	hasPos bool
	// view 是 DIBSetView 设置的字形目标尺寸（只用于字体图集）。
	view    [4]int
	hasView bool
	width   int
	height  int
	// loaded 区分"还没解码"和"解码失败"：Lua 在 load 阶段会创建几十个 DIB，
	// 立即解码会占掉上百 MB；真正绘制时再按需加载。
	loaded bool
}

// ensureImage 惰性解码图片。缺图返回 nil，调用方跳过绘制即可。
func (d *dib) ensureImage(host Host) *gfx.BsgImage {
	if d == nil {
		return nil
	}
	if d.image != nil || d.loaded {
		return d.image
	}
	d.loaded = true
	image, ok := host.Image(d.name)
	if !ok || image == nil {
		return nil
	}
	d.image = image
	d.width = image.Width
	d.height = image.Height
	return d.image
}

func newDIB(L *lua.LState, value *dib) lua.LValue {
	if value == nil {
		return lua.LNil
	}
	userdata := L.NewUserData()
	userdata.Value = value
	return userdata
}

func asDIB(L *lua.LState, index int) *dib {
	userdata, ok := L.Get(index).(*lua.LUserData)
	if !ok {
		return nil
	}
	value, _ := userdata.Value.(*dib)
	return value
}

func (d *dib) frameWidth() int {
	if d == nil {
		return 0
	}
	return d.width
}

func (d *dib) frameHeight() int {
	if d == nil || d.frames <= 0 {
		return 0
	}
	return maxInt(1, d.height/d.frames)
}

func (d *dib) sourceRect() (int, int, int, int) {
	if d == nil {
		return 0, 0, 0, 0
	}
	if d.hasView && d.view[2] > 0 && d.view[3] > 0 {
		return d.view[0], d.view[1], d.view[2], d.view[3]
	}
	return 0, d.state * d.frameHeight(), d.frameWidth(), d.frameHeight()
}

// registerDIB 注册 DIB / DIBText 命名空间与全局绘制函数。
func (r *Runtime) registerDIB() {
	L := r.L
	dibTable := L.NewTable()
	L.SetFuncs(dibTable, map[string]lua.LGFunction{
		"Create": r.dibCreate,
	})
	L.SetGlobal("DIB", dibTable)

	textTable := L.NewTable()
	L.SetFuncs(textTable, map[string]lua.LGFunction{
		"CreateFont": r.dibCreateFont,
	})
	L.SetGlobal("DIBText", textTable)

	setGlobals(L, map[string]lua.LGFunction{
		"DIBCopy":              r.dibCopy,
		"DIBFill":              r.dibFill,
		"DIBGradationRect":     r.dibGradationRect,
		"DIBReverseColor":      r.dibReverseColor,
		"DIBSetView":           r.dibSetView,
		"DIBGetWidth":          r.dibGetWidth,
		"DIBGetHeight":         r.dibGetHeight,
		"DIBSetState":          r.dibSetState,
		"DIBInit":              r.dibInit,
		"DIBIfHitThenSetState": r.dibIfHitThenSetState,
		"DrawLine":             r.drawLine,
		"DrawText":             r.drawText,
		"DrawAsciiText":        r.drawAsciiText,
		"DrawObject":           r.drawObject,
		"UpdateObject":         r.updateObject,
	})
}

// dibCreate(name, frames?, x?, y?)
func (r *Runtime) dibCreate(L *lua.LState) int {
	name := L.CheckString(1)
	frames := L.OptInt(2, 1)
	if frames < 1 {
		frames = 1
	}
	// 只查存在性，不解码：Lua 在 boot 阶段会创建所有界面的 DIB。
	if !r.host.HasImage(name) {
		L.Push(lua.LNil)
		return 1
	}
	value := &dib{
		name:   name,
		frames: frames,
		x:      L.OptInt(3, 0),
		y:      L.OptInt(4, 0),
	}
	if L.GetTop() >= 4 && L.Get(3).Type() == lua.LTNumber && L.Get(4).Type() == lua.LTNumber {
		value.hasPos = true
	}
	L.Push(newDIB(L, value))
	return 1
}

// DIBCopy(target, x, y, source, view?)
func (r *Runtime) dibCopy(L *lua.LState) int {
	target := asDIB(L, 1)
	source := asDIB(L, 4)
	if target == nil || target.canvas == nil || source == nil {
		return 0
	}
	sourceImage := source.ensureImage(r.host)
	if sourceImage == nil {
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	sx, sy, sw, sh := source.sourceRect()
	if view, ok := L.Get(5).(*lua.LTable); ok {
		sx = tableInt(view, 1, sx)
		sy = tableInt(view, 2, sy)
		sw = tableInt(view, 3, sw)
		sh = tableInt(view, 4, sh)
	}
	if sw <= 0 || sh <= 0 {
		return 0
	}
	dx, dy := dibDrawOffset(source, sourceImage)
	target.canvas.DrawRegion(sourceImage, sx, sy, sw, sh, x+dx, y+dy)
	return 0
}

// DIBFill(target, r, g, b, a)
func (r *Runtime) dibFill(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil || target.canvas == nil {
		return 0
	}
	red := clampColor(L.CheckNumber(2))
	green := clampColor(L.CheckNumber(3))
	blue := clampColor(L.CheckNumber(4))
	alpha := clampColor(L.CheckNumber(5))
	blendRect(target.canvas, 0, 0, target.canvas.Width, target.canvas.Height,
		byte(red), byte(green), byte(blue), byte(alpha))
	return 0
}

// DIBGradationRect(target,x,y,w,h, x2,y2,x3,y3, color1, color2)
func (r *Runtime) dibGradationRect(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil || target.canvas == nil {
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	width := L.CheckInt(4)
	height := L.CheckInt(5)
	left := uint32(L.OptNumber(10, 0))
	right := uint32(L.OptNumber(11, 0))
	if width <= 0 || height <= 0 {
		return 0
	}
	canvas := target.canvas
	for column := 0; column < width; column++ {
		fraction := float64(column) / float64(maxInt(width-1, 1))
		color := lerpARGB(left, right, fraction)
		blendRect(canvas, x+column, y, 1, height,
			byte(color), byte(color>>8), byte(color>>16), byte(color>>24))
	}
	return 0
}

// DIBReverseColor(target,x,y,w,h)
func (r *Runtime) dibReverseColor(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil || target.canvas == nil {
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	width := L.CheckInt(4)
	height := L.CheckInt(5)
	canvas := target.canvas
	left := maxInt(0, x)
	top := maxInt(0, y)
	right := minInt(canvas.Width, x+width)
	bottom := minInt(canvas.Height, y+height)
	for row := top; row < bottom; row++ {
		offset := (row*canvas.Width + left) * 4
		for column := left; column < right; column++ {
			canvas.Buffer[offset] = 255 - canvas.Buffer[offset]
			canvas.Buffer[offset+1] = 255 - canvas.Buffer[offset+1]
			canvas.Buffer[offset+2] = 255 - canvas.Buffer[offset+2]
			offset += 4
		}
	}
	return 0
}

func (r *Runtime) dibSetView(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil {
		return 0
	}
	target.view[0] = L.CheckInt(2)
	target.view[1] = L.CheckInt(3)
	target.view[2] = L.CheckInt(4)
	target.view[3] = L.CheckInt(5)
	target.hasView = true
	return 0
}

func (r *Runtime) dibGetWidth(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil {
		L.Push(lua.LNumber(0))
		return 1
	}
	if target.hasView && target.view[2] > 0 {
		L.Push(lua.LNumber(target.view[2]))
		return 1
	}
	if target.canvas != nil {
		L.Push(lua.LNumber(target.canvas.Width))
		return 1
	}
	if image := target.ensureImage(r.host); image != nil {
		L.Push(lua.LNumber(image.Width))
		return 1
	}
	L.Push(lua.LNumber(target.frameWidth()))
	return 1
}

func (r *Runtime) dibGetHeight(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil {
		L.Push(lua.LNumber(0))
		return 1
	}
	if target.hasView && target.view[3] > 0 {
		L.Push(lua.LNumber(target.view[3]))
		return 1
	}
	if target.canvas != nil {
		L.Push(lua.LNumber(target.canvas.Height))
		return 1
	}
	if image := target.ensureImage(r.host); image != nil {
		L.Push(lua.LNumber(image.Height / maxInt(target.frames, 1)))
		return 1
	}
	L.Push(lua.LNumber(target.frameHeight()))
	return 1
}

func (r *Runtime) dibSetState(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil {
		return 0
	}
	state := L.CheckInt(2)
	if state < 0 {
		state = 0
	}
	if target.frames > 0 && state >= target.frames {
		state = target.frames - 1
	}
	target.state = state
	return 0
}

func (r *Runtime) dibInit(L *lua.LState) int {
	target := asDIB(L, 1)
	name := L.CheckString(2)
	if target == nil {
		return 0
	}
	target.name = name
	target.image = nil
	target.loaded = false
	target.frames = 1
	target.state = 0
	target.width = 0
	target.height = 0
	target.ensureImage(r.host)
	return 0
}

// DIBIfHitThenSetState(button, hitMask, mx, my)
func (r *Runtime) dibIfHitThenSetState(L *lua.LState) int {
	button := asDIB(L, 1)
	hit := asDIB(L, 2)
	mx := L.CheckInt(3)
	my := L.CheckInt(4)
	if button == nil || hit == nil {
		L.Push(lua.LFalse)
		return 1
	}
	image := hit.ensureImage(r.host)
	if image == nil {
		L.Push(lua.LFalse)
		return 1
	}
	inside := mx >= hit.x && my >= hit.y &&
		mx < hit.x+image.Width && my < hit.y+hit.frameHeight()
	if inside {
		button.state = 1
		L.Push(lua.LTrue)
		return 1
	}
	button.state = 0
	L.Push(lua.LFalse)
	return 1
}

// DrawLine(target,x1,y1,x2,y2,color)
func (r *Runtime) drawLine(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil || target.canvas == nil {
		return 0
	}
	x1 := L.CheckInt(2)
	y1 := L.CheckInt(3)
	x2 := L.CheckInt(4)
	y2 := L.CheckInt(5)
	color := uint32(L.CheckNumber(6))
	canvas := target.canvas
	steps := maxInt(absInt(x2-x1), absInt(y2-y1))
	if steps == 0 {
		blendRect(canvas, x1, y1, 1, 1, byte(color), byte(color>>8), byte(color>>16), byte(color>>24))
		return 0
	}
	for step := 0; step <= steps; step++ {
		x := x1 + (x2-x1)*step/steps
		y := y1 + (y2-y1)*step/steps
		blendRect(canvas, x, y, 1, 1, byte(color), byte(color>>8), byte(color>>16), byte(color>>24))
	}
	return 0
}

// DrawObject/UpdateObject 在原引擎里驱动共享的"对象层"（鼠标光标、演出）。
// 这些界面自己会画所有素材；返回 false 表示没有额外的强制刷新。
func (r *Runtime) drawObject(L *lua.LState) int   { return 0 }
func (r *Runtime) updateObject(L *lua.LState) int { L.Push(lua.LFalse); return 1 }

// ------------------------------------------------------------------ 像素工具

func blendRect(canvas *gfx.BgraCanvas, x, y, width, height int, blue, green, red, alpha byte) {
	if canvas == nil || alpha == 0 {
		return
	}
	left := maxInt(0, x)
	top := maxInt(0, y)
	right := minInt(canvas.Width, x+width)
	bottom := minInt(canvas.Height, y+height)
	if left >= right || top >= bottom {
		return
	}
	sourceAlpha := int(alpha)
	inverse := 255 - sourceAlpha
	for row := top; row < bottom; row++ {
		offset := (row*canvas.Width + left) * 4
		for column := left; column < right; column++ {
			canvas.Buffer[offset] = byte((int(blue)*sourceAlpha + int(canvas.Buffer[offset])*inverse) / 255)
			canvas.Buffer[offset+1] = byte((int(green)*sourceAlpha + int(canvas.Buffer[offset+1])*inverse) / 255)
			canvas.Buffer[offset+2] = byte((int(red)*sourceAlpha + int(canvas.Buffer[offset+2])*inverse) / 255)
			canvas.Buffer[offset+3] = 255
			offset += 4
		}
	}
}

func lerpARGB(from, to uint32, fraction float64) uint32 {
	fraction = math.Max(0, math.Min(1, fraction))
	mix := func(shift uint) uint32 {
		left := float64((from >> shift) & 0xff)
		right := float64((to >> shift) & 0xff)
		return uint32(left + (right-left)*fraction)
	}
	return mix(24)<<24 | mix(16)<<16 | mix(8)<<8 | mix(0)
}

func clampColor(value lua.LNumber) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return int(value)
}

func tableInt(table *lua.LTable, index int, fallback int) int {
	value := table.RawGetInt(index)
	if number, ok := value.(lua.LNumber); ok {
		return int(number)
	}
	return fallback
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// dibDrawOffset 返回 DIB 在 DIBCopy 目标坐标之上的额外偏移。
//
// 没有显式坐标的 DIB 用图片自身的内嵌偏移；这是旧版 Lua 的定位方式
// （training/territory 的面板都是 `DIB.Create(name)` + `DIBCopy(0,0,...)`）。
// 显式坐标优先，避免覆盖脚本写死的布局。
func dibDrawOffset(source *dib, image *gfx.BsgImage) (int, int) {
	if source == nil {
		return 0, 0
	}
	dx, dy := source.x, source.y
	if source.hasPos || image == nil {
		return dx, dy
	}
	return dx + image.OffsetX, dy + image.OffsetY
}
