package state

import (
	"kizuna/engine/core"
)

const (
	bustupFullLength    = 12
	bustupBodyLength    = 10
	bustupStateIndex    = 4
	bustupWearIndex     = 5
	bustupPoseIndex     = 6
	bustupStripIndex    = 8
	bustupVariantIndex  = 9
	bustupWidthField    = 0x16
	bustupHeightField   = 0x18
	bustupMeasurePrefix = 0x20
	bustupStandingState = 'u'
	bustupDigitFirst    = '0'
	bustupDigitLast     = '9'
	bustupVariantFirst  = 'a'
	bustupVariantLast   = 'c'
)

const BustupEyePartIndex = 1

const BustupDefaultHairSuffix = "_hair"

func BustupIsBustupName(name string) bool {
	return len(name) >= bustupFullLength &&
		name[0] == 'b' &&
		name[1] == 'u' &&
		name[2] >= '0' && name[2] <= '9' &&
		name[3] >= '0' && name[3] <= '9'
}

func BustupIsStanding(name string) bool {
	return BustupIsBustupName(name) && name[bustupStateIndex] == bustupStandingState
}

// IsBustupLayerName 判断图层名是不是立绘（buNN + 状态字符）。
//
// BustupIsBustupName 要求完整 12 字符名（含表情/服装后缀），但渲染层拿到的
// 身体图只有 10 字符（例如 bu01u0011a）。视点切换只关心前 5 个字符。
func IsBustupLayerName(name string) bool {
	return len(name) >= 5 &&
		name[0] == 'b' && name[1] == 'u' &&
		name[2] >= bustupDigitFirst && name[2] <= bustupDigitLast &&
		name[3] >= bustupDigitFirst && name[3] <= bustupDigitLast
}

// BustupWithState 返回把状态字符（第 5 个字符）替换后的立绘名。
// 原版用 f 表示脸部特写、u 表示全身；胸/腰视点是在 u 图上按偏移裁切。
func BustupWithState(name string, state byte) string {
	if !IsBustupLayerName(name) {
		return name
	}
	bytes := []byte(name)
	bytes[bustupStateIndex] = state
	return string(bytes)
}

func BustupResolve(name string, x, y, frames int, resources ResourceIndex, hairSuffix string) []LayerArtwork {
	if !BustupIsBustupName(name) {
		return []LayerArtwork{NewLayerArtwork(name, x, y)}
	}
	body, bodyFound := bustupBodyName(name, resources)
	if !bodyFound {
		// 本作（すくーるヘブン）的部分立绘没有 10 字符身体图，只有 12 字符合成图，
		// 其中第 1 部件是竖排眼睛帧条。这里必须把它标成动画部件，否则会整条画出，
		// 画面上就是一列重叠的眼睛和眉毛。
		if _, whole := bustupTryMeasure(resources, name); whole {
			return []LayerArtwork{AnimatedLayerArtwork(name, x, y, frames, BustupEyePartIndex)}
		}
		return []LayerArtwork{NewLayerArtwork(name, x, y)}
	}
	canvas, _ := bustupTryMeasure(resources, body)
	face := bustupFindFace(name, resources)
	faceCanvas, _ := bustupTryMeasure(resources, face)
	// b 组全身身体图是 1600x2300，同组的表情图却按 2350x2990 绘制；
	// 原引擎把身体图按表情容器的比例放大后再叠表情。比例只在宽度不同时
	// 启用：SM 的 u 身体图 1600x2300 + 表情 1600x1650 宽度相同，不能缩放
	// （表情高度只是可视区域，不是坐标系）。
	scaleX, scaleY := 1.0, 1.0
	if canvas != nil && faceCanvas != nil && canvas.Width != faceCanvas.Width && canvas.Width > 0 && canvas.Height > 0 {
		scaleX = float64(faceCanvas.Width) / float64(canvas.Width)
		scaleY = float64(faceCanvas.Height) / float64(canvas.Height)
	}
	artworks := []LayerArtwork{{
		Image: body, X: x, Y: y, Frames: 1, AnimatedPart: -1,
		ScaleX: scaleX, ScaleY: scaleY,
	}}
	suffix := hairSuffix
	if len(suffix) == 0 {
		suffix = BustupDefaultHairSuffix
	}
	hair := body + suffix
	if _, ok := bustupTryMeasure(resources, hair); ok {
		artworks = append(artworks, LayerArtwork{
			Image: hair, X: x, Y: y, Frames: 1, AnimatedPart: -1,
			ScaleX: scaleX, ScaleY: scaleY,
		})
	}
	if len(face) > 0 {
		artworks = append(artworks, AnimatedLayerArtwork(face, x, y, frames, BustupEyePartIndex))
	}
	if canvas != nil {
		core.Logf("gfx: bustup '%s' body='%s' hair='%s' face='%s' canvas=%dx%d", name, body, hair, bustupFaceName(face), canvas.Width, canvas.Height)
	} else {
		core.Logf("gfx: bustup '%s' body='%s' hair='%s' face='%s' canvas=unknown", name, body, hair, bustupFaceName(face))
	}
	return artworks
}

// bustupBodyName 找出与 12 字符表情图配套的 10 字符身体图。
//
// 绝大多数作品的身体图就是表情图的前 10 个字符。但 CS 的主人公（ch6）
// 只有一套服装，身体图写的是 `bu06f1001a`（服装位 1），而剧本给的表情图
// 是 `bu06f0001a01`（服装位 0）；按前 10 字符找不到身体图时，旧实现会
// 退回"12 字符合成图"，把只画了头部的表情图当成整个人物，画面上就只剩
// 一个头。这里在服装位（第 6 个字符）上扫 0..9，只替换这一位，角色号、
// 状态、姿势、差分保持不变，所以不会把别的角色/表情配错。
//
// 扫不到时返回 false，调用方仍走原来的整图/空图回退（すくーるヘブン 的
// 立绘确实没有 10 字符身体图，不能强行匹配）。
func bustupBodyName(name string, resources ResourceIndex) (string, bool) {
	if len(name) < bustupBodyLength {
		return "", false
	}
	candidate := name[:bustupBodyLength]
	if _, ok := bustupTryMeasure(resources, candidate); ok {
		return candidate, true
	}
	bytes := []byte(candidate)
	for wear := byte(bustupDigitFirst); wear <= bustupDigitLast; wear++ {
		bytes[bustupWearIndex] = wear
		current := string(bytes)
		if current == candidate {
			continue
		}
		if _, ok := bustupTryMeasure(resources, current); ok {
			core.Logf("gfx: bustup body '%s' resolved from '%s' by wear digit", current, candidate)
			return current, true
		}
	}
	return "", false
}

func bustupFaceName(face string) string {
	if len(face) == 0 {
		return "-"
	}
	return face
}

func bustupFindFace(name string, resources ResourceIndex) string {
	candidate := []byte(name)
	if bustupMatches(resources, candidate) {
		return string(candidate)
	}
	candidate[bustupPoseIndex] = bustupDigitFirst
	candidate[bustupPoseIndex+1] = bustupDigitFirst
	if bustupMatches(resources, candidate) {
		return string(candidate)
	}
	for wear := byte(bustupDigitFirst); wear <= bustupDigitLast; wear++ {
		for strip := byte(bustupDigitFirst); strip <= bustupDigitLast; strip++ {
			candidate[bustupWearIndex] = wear
			candidate[bustupStripIndex] = strip
			if bustupMatches(resources, candidate) {
				return string(candidate)
			}
		}
	}
	candidate[bustupWearIndex] = bustupDigitFirst
	candidate[bustupStripIndex] = bustupDigitFirst
	for variant := byte(bustupVariantFirst); variant <= bustupVariantLast; variant++ {
		candidate[bustupVariantIndex] = variant
		if bustupMatches(resources, candidate) {
			return string(candidate)
		}
	}
	return ""
}

// bustupMatches 只判断候选表情图是否存在。
//
// 不能要求容器宽高完全相等：三射面談IE 的 b 组全身身体图是 1600x2300，
// 表情图是 2350x2990；SM 的 u 身体图 1600x2300、表情图 1600x1650。
// 候选名由请求名逐字符改造而来，角色号/状态位不变，所以"存在即同一角色"；
// 尺寸差异由 BustupResolve 换算成缩放，而不是拒绝这张图。
func bustupMatches(resources ResourceIndex, candidate []byte) bool {
	_, ok := bustupTryMeasure(resources, string(candidate))
	return ok
}

type bustupCanvasSize struct {
	Width  int
	Height int
}

func bustupTryMeasure(resources ResourceIndex, name string) (*bustupCanvasSize, bool) {
	if len(name) == 0 || !resources.ContainsExact(name) {
		return nil, false
	}
	location, ok := resources.TryGet(name)
	if !ok {
		return nil, true
	}
	head := location.ReadPrefix(bustupMeasurePrefix)
	if len(head) < bustupHeightField+2 {
		return nil, true
	}
	width := int(uint16(head[bustupWidthField]) | uint16(head[bustupWidthField+1])<<8)
	height := int(uint16(head[bustupHeightField]) | uint16(head[bustupHeightField+1])<<8)
	if width > 0 && height > 0 {
		return &bustupCanvasSize{Width: width, Height: height}, true
	}
	return nil, true
}

func BustupIsAsciiDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
