package gfx

import (
	"encoding/binary"
	"errors"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/gfx/bmp"
	"kizuna/engine/core"
)

const fileHeaderSize = 54

func ToBmpFile(header *EmeImageHeader, payload []byte, frameSize, frameInitPos int) ([]byte, error) {
	image := decodeParsed(header, payload, frameSize, frameInitPos)
	if image == nil {
		return nil, errors.New("EME image payload is inconsistent")
	}

	rowBytes := image.Width * 4
	bytes := make([]byte, fileHeaderSize+rowBytes*image.Height)
	bytes[0] = 'B'
	bytes[1] = 'M'
	binary.LittleEndian.PutUint32(bytes[2:], uint32(len(bytes)))
	binary.LittleEndian.PutUint32(bytes[10:], fileHeaderSize)
	binary.LittleEndian.PutUint32(bytes[14:], 40)
	binary.LittleEndian.PutUint32(bytes[18:], uint32(int32(image.Width)))
	binary.LittleEndian.PutUint32(bytes[22:], uint32(int32(image.Height)))
	binary.LittleEndian.PutUint16(bytes[26:], 1)
	binary.LittleEndian.PutUint16(bytes[28:], 32)
	binary.LittleEndian.PutUint32(bytes[34:], uint32(rowBytes*image.Height))

	for y := 0; y < image.Height; y++ {
		source := image.Pixels[(image.Height-1-y)*rowBytes : (image.Height-1-y)*rowBytes+rowBytes]
		target := bytes[fileHeaderSize+y*rowBytes : fileHeaderSize+y*rowBytes+rowBytes]
		copy(target, source)
	}

	return bytes, nil
}

func LoadBmp(bytes []byte) *bishopgfx.BsgImage {
	if len(bytes) == 0 {
		return nil
	}

	image, err := bmp.Decode(bytes)
	if err == nil {
		return image
	}

	core.Logf("gfx: EME BMP decode failed: %v", err)
	return nil
}

func Blank(width, height int, b, g, r uint8) *bishopgfx.BsgImage {
	canvas := bishopgfx.NewBgraCanvas(width, height)
	canvas.Clear(b, g, r, 255)
	return canvas.ToImage(0, 0)
}
