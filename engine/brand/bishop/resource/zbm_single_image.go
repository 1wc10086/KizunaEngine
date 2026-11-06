package resource

import (
	"encoding/binary"
	"math"
)

const ZbmSingleImageHeaderSize = 32

func IsZbmSingleImage(data []byte) bool {
	if len(data) < ZbmSingleImageHeaderSize {
		return false
	}
	if data[0] != 'a' || data[1] != 'm' || data[2] != 'p' || data[3] != '_' {
		return false
	}
	if binary.LittleEndian.Uint16(data[4:]) != 1 {
		return false
	}

	unpackedSize := binary.LittleEndian.Uint32(data[6:])
	if unpackedSize == 0 {
		return false
	}

	dataBase := binary.LittleEndian.Uint32(data[10:])
	return dataBase >= ZbmSingleImageHeaderSize && uint64(dataBase) <= uint64(len(data))
}

func UnpackZbmSingleImage(data []byte) []byte {
	if !IsZbmSingleImage(data) {
		return nil
	}

	unpackedSize := binary.LittleEndian.Uint32(data[6:])
	if unpackedSize > math.MaxInt32 {
		return nil
	}

	dataBase := binary.LittleEndian.Uint32(data[10:])
	payload := data[dataBase:]
	if len(payload) == 0 {
		return nil
	}

	unpacked, err := ZbmUnpack(payload, int(unpackedSize))
	if err != nil {
		return nil
	}
	if len(unpacked) > int(unpackedSize) {
		return unpacked[:unpackedSize]
	}
	if len(unpacked) < int(unpackedSize) {
		return nil
	}

	return unpacked
}
