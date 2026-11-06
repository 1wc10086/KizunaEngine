package gfx

import "errors"

func IsBsgGraphic(data []byte) bool {
	return IsBsgComposition(data) || IsBsgGraphics(data, 0)
}

func ReadBsgGraphic(data []byte) (*BsgImage, error) {
	if IsBsgComposition(data) {
		composition, err := ParseBsgComposition(data)
		if err != nil {
			return nil, err
		}

		frame := composition.Flatten()
		first := composition.Parts[0]
		return &BsgImage{
			Width:            frame.Width,
			Height:           frame.Height,
			OffsetX:          frame.OffsetX,
			OffsetY:          frame.OffsetY,
			Pixels:           frame.Pixels,
			Parts:            composition.Parts,
			ContainerOffsetX: BsgCompositionContainerOffsetX,
			ContainerOffsetY: BsgCompositionContainerOffsetY,
			FrameIndex:       first.FrameIndex,
			FrameCount:       first.FrameCount,
			ClipLeft:         first.ClipLeft,
			ClipTop:          first.ClipTop,
			ClipWidth:        first.ClipWidth,
			ClipHeight:       first.ClipHeight,
			Alpha:            first.Alpha,
			BlendMode:        first.BlendMode,
		}, nil
	}

	if IsBsgGraphics(data, 0) {
		return DecodeBsgImage(data, 0)
	}

	return nil, errors.New("not a BSG graphic")
}
