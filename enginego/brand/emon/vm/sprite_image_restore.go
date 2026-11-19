package vm

// sprite_image_restore.go：读档时重建精灵的位图。
//
// 原引擎的存档把精灵表（含像素）整块写进档里；我们只存状态，若不记位图来源，
// 读档后每个 sprite 都有坐标/可见性但没有图像，整屏就是全黑（只有文字层
// 还在画）。这里按存档里记录的文件名（或链到的精灵名）重新从 archives
// 加载位图并重建裁剪/遮罩，与存档时的画面一致。

import (
	"strings"

	"kizuna/enginego/brand/emon/save"
	"kizuna/enginego/brand/emon/stage"
	emontext "kizuna/enginego/brand/emon/text"
	"kizuna/enginego/core"
)

func (m *Machine) restoreSpriteImage(sprite *stage.EmonSprite, state *save.EmonSpriteState) {
	if sprite == nil || state == nil {
		return
	}
	// 先重建遮罩（独立于位图来源）。存档里没有遮罩时必须清掉旧遮罩，
	// 否则读档后精灵还戴着读档前的 mask，透明/裁剪形状会错。
	if state.MaskFile != "" {
		if mask := m.ResolveSpriteImage(state.MaskFile); mask != nil {
			sprite.SetMask(mask)
			m.spriteMasks.Set(sprite.Name(), state.MaskFile)
		} else {
			sprite.SetMask(nil)
			m.spriteMasks.Delete(sprite.Name())
		}
	} else {
		sprite.SetMask(nil)
		m.spriteMasks.Delete(sprite.Name())
	}
	switch {
	case state.ImageSourceSprite != "":
		// 链到别的精灵：保持链接，让被链精灵的图/裁剪继续跟随。
		if source := m.context.Stage.Find(state.ImageSourceSprite); source != nil {
			sprite.SetImageSource(source)
			sprite.SetImage(nil)
			m.spriteImages.Set(sprite.Name(), "sprite:"+source.Name())
		} else {
			sprite.SetImageSource(nil)
			sprite.SetImage(nil)
			m.spriteImages.Delete(sprite.Name())
			core.Logf("emon: load sprite '%s' source '%s' was not found", sprite.Name(), state.ImageSourceSprite)
		}
	case state.ImageFile != "":
		image := m.ResolveSpriteImage(state.ImageFile)
		if image == nil {
			// 旧图不能留着，否则读档后该精灵显示的是读档前的图。
			sprite.SetImageSource(nil)
			sprite.SetImage(nil)
			m.spriteImages.Delete(sprite.Name())
			core.Logf("emon: load sprite '%s' image '%s' was not found", sprite.Name(), state.ImageFile)
			return
		}
		sprite.SetImageSource(nil)
		sprite.SetImage(image)
		m.StoreSource(sprite.Name(), image)
		m.spriteImages.Set(sprite.Name(), state.ImageFile)
	default:
		// 存档里这个精灵没有位图：清掉读档前的旧图/旧链接。
		sprite.SetImageSource(nil)
		sprite.SetImage(nil)
		m.spriteImages.Delete(sprite.Name())
		m.sourceImages.Delete(sprite.Name())
	}
	// 恢复裁剪矩形与显示尺寸。存档里没有 SourceRect 时必须清掉旧裁剪，
	// 否则 linked follower 会继续用读档前的裁剪。
	if state.SourceRect != nil && state.SourceRect.Width > 0 && state.SourceRect.Height > 0 {
		sprite.SetSourceRect(int(state.SourceRect.X), int(state.SourceRect.Y),
			int(state.SourceRect.Width), int(state.SourceRect.Height))
	} else {
		sprite.ClearSourceRect()
		if state.Width > 0 && state.Height > 0 {
			sprite.SetSize(int(state.Width), int(state.Height))
		}
	}
	// 文字（选项/槽位备注/名字）不走位图，但要一起恢复，否则读档后选项空白。
	if strings.TrimSpace(state.Text) != "" {
		text := sprite.Text()
		if text == nil {
			text = emontext.NewEmonSpriteText()
			sprite.SetText(text)
		}
		if text.Content() != state.Text {
			text.Clear()
			for _, segment := range strings.Split(state.Text, "\n") {
				text.Append(segment)
			}
			sprite.InvalidateText()
			m.SetSpriteText(sprite.Name(), text.Content())
		}
	}
}
