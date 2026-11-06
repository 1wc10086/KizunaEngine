package runtime

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// layer_motion.go 解析 LAYER_PARAM type=6 的入场动画脚本。
//
// 脚本是 define.bsa 里的 `<name>.txt`，格式和按钮动画一致：
//
//	<layer fps="29.97" begin="1" end="82">
//	    <frame no="1" opacity="20" pos="512 100"/>
//	    <frame no="4.5" opacity="100" pos="512 480"/>
//	</layer>
//
// 只有"普通图层"才挂这条动画。地图卡在本引擎里是 StageButton，
// 位置由紧随其后的 LAYER_PARAM type=0 给出，动画脚本只负责翻转入场；
// 把它当图层动画会把卡片从按钮表里挪走，点击区域也会一起错位。

type layerAnimationDocument struct {
	FPS    float64 `xml:"fps,attr"`
	Begin  float64 `xml:"begin,attr"`
	End    float64 `xml:"end,attr"`
	Frames []struct {
		No      float64  `xml:"no,attr"`
		Opacity *float64 `xml:"opacity,attr"`
		Pos     string   `xml:"pos,attr"`
	} `xml:"frame"`
}

// layerHasButton 判断这一层是否登记成了地图按钮。
func (d *StageDirector) layerHasButton(plane int) bool {
	for _, button := range d.state.Buttons() {
		if button.Layer == plane {
			return true
		}
	}
	return false
}

// applyLayerMotion 尝试给图层挂上关键帧动画；返回 false 表示这不是
// 可直接播放的图层动画，调用方继续走原来的"地图卡/忽略"分支。
func (d *StageDirector) applyLayerMotion(plane int, name string) bool {
	if plane < 0 || plane >= state.PlaneCount || len(name) == 0 {
		return false
	}
	layer := d.state.LayerAt(plane)
	if layer == nil || len(layer.Artworks()) == 0 {
		return false
	}
	// アヘ顔OFF 时不是"不播放动画"，而是这层根本不该出现：
	// A-H-E 特写从上方滑入后覆盖在 CG 上，跳过动画会让它停在屏幕外，
	// 但部分素材的原始位置就在画面内。用 Hidden 动画明确跳过绘制。
	if d.configuration != nil && d.configuration.AheOff && isAheAnimation(name) {
		d.state.SetLayerMotion(plane, state.NewHiddenLayerMotion())
		core.Logf("gfx: layer %d A-H-E suppressed by setting", plane)
		return true
	}
	location, ok := d.resources.TryGet(name + ".txt")
	if !ok || location == nil {
		return false
	}
	document := layerAnimationDocument{}
	if err := xml.Unmarshal(location.Read(), &document); err != nil {
		core.Logf("gfx: layer animation '%s' parse failed: %v", name, err)
		return false
	}
	motion := buildLayerMotion(layer, document, d.resources)
	if motion == nil {
		return false
	}
	d.state.SetLayerMotion(plane, motion)
	core.Logf("gfx: layer %d motion '%s' %d keyframe(s)", plane, name, len(motion.Keyframes))
	return true
}

// buildLayerMotion 把 XML 关键帧换算成相对图层原点的位移/透明度。
//
// pos 在原引擎里是图片中心点；这里减去图片半宽/半高，再减去图层第一个
// 部件自己的 (x,y)，这样多部件图层只整体移动，不会改变内部相对位置。
func buildLayerMotion(layer *state.StageLayer, document layerAnimationDocument, resources state.ResourceIndex) *state.LayerMotion {
	if len(document.Frames) == 0 {
		return nil
	}
	artworks := layer.Artworks()
	if len(artworks) == 0 {
		return nil
	}
	base := artworks[0]
	width, height := artworkPixelSize(resources, base)
	if width <= 0 || height <= 0 {
		return nil
	}
	fps := document.FPS
	if fps <= 0 {
		fps = 30
	}
	begin := document.Begin
	if begin == 0 {
		begin = document.Frames[0].No
	}
	motion := &state.LayerMotion{
		Start:  time.Now(),
		BaseX:  base.X,
		BaseY:  base.Y,
		Width:  width,
		Height: height,
	}
	for _, frame := range document.Frames {
		key := state.LayerMotionKeyframe{
			At: (frame.No - begin) / fps,
		}
		if key.At < 0 {
			key.At = 0
		}
		if frame.Opacity != nil {
			key.Opacity = clampPercent(*frame.Opacity)
			key.HasOpacity = true
		}
		if parts := strings.Fields(frame.Pos); len(parts) >= 2 {
			if x, errX := strconv.Atoi(parts[0]); errX == nil {
				if y, errY := strconv.Atoi(parts[1]); errY == nil {
					key.X, key.Y, key.HasPos = x, y, true
				}
			}
		}
		motion.Keyframes = append(motion.Keyframes, key)
	}
	return motion
}

// artworkPixelSize 取部件图像的像素尺寸；读不到时返回 0。
func artworkPixelSize(resources state.ResourceIndex, artwork state.LayerArtwork) (int, int) {
	if strings.TrimSpace(artwork.Image) == "" || resources == nil {
		return 0, 0
	}
	location, ok := resources.TryGet(artwork.Image)
	if !ok || location == nil {
		return 0, 0
	}
	width, height, ok := gfx.TryReadBsgDimensions(location.Read())
	if !ok {
		return 0, 0
	}
	return width, height
}

// isAheAnimation 判断动画脚本是否就是本作的「アヘ顔」特写。
func isAheAnimation(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return normalized == "a-h-e" || strings.HasPrefix(normalized, "a-h-e_")
}

func clampPercent(value float64) float64 {
	value /= 100
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
