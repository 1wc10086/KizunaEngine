package gfx

func DecodeBsgImage(data []byte, baseOffset int) (*BsgImage, error) {
	header, err := ParseBsgHeader(data, baseOffset)
	if err != nil {
		return nil, err
	}

	source := data[header.DataOffset : header.DataOffset+header.DataSize]
	pixels := make([]byte, header.PixelCount()*4)

	switch header.ColorMode {
	case BsgColorModeBgra32:
		unpackTrueColor(header, source, pixels, 4)
	case BsgColorModeBgr24:
		unpackTrueColor(header, source, pixels, 3)
		makeOpaque(pixels, header.PixelCount())
	default:
		unpackIndexed(header, data, source, pixels)
	}

	flipVertical(pixels, header.Width, header.Height)

	return &BsgImage{
		Width:      header.Width,
		Height:     header.Height,
		OffsetX:    header.OffsetX,
		OffsetY:    header.OffsetY,
		Pixels:     pixels,
		FrameIndex: header.FrameIndex,
		FrameCount: header.FrameCount,
		ClipLeft:   header.ClipLeft,
		ClipTop:    header.ClipTop,
		ClipWidth:  header.ClipWidth,
		ClipHeight: header.ClipHeight,
		Alpha:      header.Alpha,
		BlendMode:  header.BlendMode,
	}, nil
}

func unpackTrueColor(header *BsgHeader, source []byte, pixels []byte, channels int) {
	if header.Compression == BsgCompressionNone {
		if channels == 4 {
			copy(pixels, source)
			return
		}

		count := min(len(source)/3, header.PixelCount())
		for i := 0; i < count; i++ {
			target := i * 4
			origin := i * 3
			pixels[target] = source[origin]
			pixels[target+1] = source[origin+1]
			pixels[target+2] = source[origin+2]
			pixels[target+3] = 255
		}

		return
	}

	position := 0
	for plane := 0; plane < channels && position < len(source); plane++ {
		consumed := 0
		if header.Compression == BsgCompressionLz {
			consumed = UnpackBsgLz(source[position:], pixels, plane, 4)
		} else {
			consumed = UnpackBsgRle(source[position:], pixels, plane, 4)
		}

		if consumed <= 0 {
			break
		}

		position += consumed
	}
}

func unpackIndexed(header *BsgHeader, data []byte, source []byte, pixels []byte) {
	indices := make([]byte, header.PixelCount())
	if header.Compression == BsgCompressionNone {
		copy(indices, source)
	} else if header.Compression == BsgCompressionLz {
		UnpackBsgLz(source, indices, 0, 1)
	} else {
		UnpackBsgRle(source, indices, 0, 1)
	}

	palette := ReadBsgPalette(data, header.PaletteOffset)
	for i := 0; i < len(indices); i++ {
		target := pixels[i*4 : i*4+BsgPaletteEntrySize]
		if palette == nil {
			value := indices[i]
			target[0] = value
			target[1] = value
			target[2] = value
			target[3] = 255
			continue
		}

		palette.Write(int(indices[i]), target)
	}
}

func makeOpaque(pixels []byte, pixelCount int) {
	limit := min(pixelCount*4, len(pixels))
	for i := 3; i < limit; i += 4 {
		pixels[i] = 255
	}
}

func flipVertical(pixels []byte, width, height int) {
	stride := width * 4
	scratch := make([]byte, stride)
	for row := 0; row < height/2; row++ {
		top := row * stride
		bottom := (height - 1 - row) * stride
		copy(scratch, pixels[top:top+stride])
		copy(pixels[top:top+stride], pixels[bottom:bottom+stride])
		copy(pixels[bottom:bottom+stride], scratch)
	}
}
