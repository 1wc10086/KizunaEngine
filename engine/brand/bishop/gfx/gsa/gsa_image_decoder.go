package gsa

import (
	"errors"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

func TryRead(data []byte) (image *gfx.BsgImage) {
	defer func() {
		if recover() != nil {
			image = nil
		}
	}()
	image, err := Read(data)
	if err != nil {
		core.Logf("gfx: GSA decode failed: %v", err)
		return nil
	}
	return image
}

func Read(data []byte) (*gfx.BsgImage, error) {
	if len(data) == 0 {
		return nil, errors.New("GSA data is empty")
	}
	header, ok := ParseGsaHeader(data)
	if !ok {
		return nil, errors.New("not a GSA image")
	}
	if len(data) <= 0xD8 {
		return nil, errors.New("GSA bitstream is truncated")
	}
	is24Bit := header.Is24Bit()
	bytesPerPixel := 4
	stride := header.Width * 4
	if is24Bit {
		bytesPerPixel = 3
		stride = (header.Width*3 + 3) &^ 3
	}
	reservedLines := (header.Height + 1) &^ 1
	outputSize := int64(stride) * int64(reservedLines)
	if outputSize <= 0 || outputSize > 64*1024*1024 {
		return nil, errors.New("GSA dimensions are too large")
	}
	output := make([]byte, outputSize)
	bits := NewGsaBitReader(data, 0xD8)
	planes := NewGsaPlaneDecoder(output, stride, header.Width, header.Height, bytesPerPixel)
	if !is24Bit {
		if err := planes.ReadPlane(bits, 3); err != nil {
			return nil, err
		}
	}
	if err := planes.ReadPlane(bits, 0); err != nil {
		return nil, err
	}
	if err := planes.ReadPlane(bits, 1); err != nil {
		return nil, err
	}
	if err := planes.ReadPlane(bits, 2); err != nil {
		return nil, err
	}
	if header.Type&0x80 != 0 {
		applyScale(output, stride, header.Width, header.Height, bytesPerPixel)
	}
	pixels := toTopDownBgra(output, stride, header.Width, header.Height, bytesPerPixel)
	return gfx.NewBsgImage(header.Width, header.Height, header.OffsetX, header.OffsetY, pixels), nil
}

func applyScale(output []byte, stride, width, height, bytesPerPixel int) {
	row := 0
	for y := 0; y < height; y++ {
		dst := row
		row += stride
		for x := 0; x < width; x++ {
			output[dst] <<= 3
			output[dst+1] <<= 2
			output[dst+2] <<= 2
			dst += bytesPerPixel
		}
	}
}

func toTopDownBgra(output []byte, stride, width, height, bytesPerPixel int) []byte {
	pixels := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		src := (height - 1 - y) * stride
		dst := y * width * 4
		if bytesPerPixel == 4 {
			copy(pixels[dst:dst+width*4], output[src:src+width*4])
		} else {
			for x := 0; x < width; x++ {
				pixels[dst] = output[src]
				pixels[dst+1] = output[src+1]
				pixels[dst+2] = output[src+2]
				pixels[dst+3] = 255
				src += 3
				dst += 4
			}
		}
	}
	return pixels
}
