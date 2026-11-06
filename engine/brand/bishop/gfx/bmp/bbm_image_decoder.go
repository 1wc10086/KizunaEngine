package bmp

import (
	"errors"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

const bbmEncryptedHeaderSize = 100

func IsBbm(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xBD && data[1] == 0xB2
}

func TryRead(data []byte) *bishopgfx.BsgImage {
	image, err := ReadBbm(data)
	if err != nil {
		core.Logf("gfx: BBM decode failed: %v", err)
		return nil
	}

	return image
}

func ReadBbm(data []byte) (*bishopgfx.BsgImage, error) {
	if len(data) == 0 {
		return nil, errors.New("BBM data is empty")
	}

	span := data
	if IsBbm(span) {
		span = DecryptBbm(span)
	} else if !IsBmp(span) {
		return nil, errors.New("not a BBM image")
	}

	return Decode(span)
}

func DecryptBbm(data []byte) []byte {
	output := make([]byte, len(data))
	copy(output, data)
	count := min(bbmEncryptedHeaderSize, len(output))
	for i := 0; i < count; i++ {
		output[i] ^= 0xFF
	}

	return output
}
