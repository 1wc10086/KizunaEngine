package save

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"

	"kizuna/engine/brand/emon/eme"
)

const (
	SaveDirectoryName   = "SAVEDATA"
	SystemFileName      = "emesys.dat"
	SystemMagicText     = "EMESYS_VER1.20"
	FallbackSaveFileKey = "EMON-SAVEDATA-1"
	MagicSize           = 16
	KeySize             = eme.KeySize
	PrefixSize          = MagicSize + KeySize
	lengthSize          = 4
	methodCount         = 8
	subKeyAdjustAttemps = 4096
)

var keyBlockCore = buildKeyBlock()
var systemMagicCore = buildSystemMagic()
var fallbackMagicCore = buildFallbackMagic()

func KeyBlock() []byte {
	out := make([]byte, len(keyBlockCore))
	copy(out, keyBlockCore)
	return out
}

func SystemMagicBytes() []byte {
	out := make([]byte, len(systemMagicCore))
	copy(out, systemMagicCore)
	return out
}

func SlotPath(directory string, slot int) string {
	return filepath.Join(directory, fmt.Sprintf("Save%03d.dat", slot))
}

func SaveFileMagic(saveFileKey string) []byte {
	if len(saveFileKey) == 0 {
		out := make([]byte, len(fallbackMagicCore))
		copy(out, fallbackMagicCore)
		return out
	}
	magic := make([]byte, MagicSize)
	encoded := EncodeCP932(saveFileKey)
	copy(magic, encoded)
	return magic
}

func Pack(magic []byte, body []byte, lengthPrefixed bool) []byte {
	payload := make([]byte, len(body))
	copy(payload, body)
	key := adjustKeyBlock(len(payload))
	if len(payload) != 0 {
		_ = eme.EncryptRange(payload, 0, len(payload), key)
	}
	size := PrefixSize
	if lengthPrefixed {
		size += lengthSize
	}
	size += len(payload)
	file := make([]byte, size)
	magicLength := len(magic)
	if magicLength > MagicSize {
		magicLength = MagicSize
	}
	copy(file[0:magicLength], magic[0:magicLength])
	copy(file[MagicSize:MagicSize+KeySize], key)
	at := PrefixSize
	if lengthPrefixed {
		binary.LittleEndian.PutUint32(file[at:at+lengthSize], uint32(len(payload)))
		at += lengthSize
	}
	copy(file[at:], payload)
	return file
}

func TryUnpack(file []byte, magic []byte, lengthPrefixed bool) ([]byte, string, bool) {
	prefix := PrefixSize
	if lengthPrefixed {
		prefix += lengthSize
	}
	if len(file) < prefix {
		return nil, fmt.Sprintf("truncated header (%d bytes)", len(file)), false
	}
	if !bytes.Equal(file[0:MagicSize], magic) {
		return nil, "magic mismatch", false
	}
	length := len(file) - prefix
	if lengthPrefixed {
		declared := int(int32(binary.LittleEndian.Uint32(file[PrefixSize : PrefixSize+lengthSize])))
		if declared < 0 || len(file) != prefix+declared {
			return nil, fmt.Sprintf("payload length %d does not match file size %d", declared, len(file)), false
		}
		length = declared
	}
	if length == 0 {
		return []byte{}, "", true
	}
	payload := make([]byte, length)
	copy(payload, file[prefix:prefix+length])
	key := make([]byte, KeySize)
	copy(key, file[MagicSize:MagicSize+KeySize])
	_ = eme.DecryptRange(payload, 0, len(payload), key)
	return payload, "", true
}

func adjustKeyBlock(length int) []byte {
	key := make([]byte, len(keyBlockCore))
	copy(key, keyBlockCore)
	if length < 2 {
		return key
	}
	for i := 0; i < methodCount; i++ {
		if key[i] != 8 {
			continue
		}
		at := 8 + i*4
		sub := binary.LittleEndian.Uint32(key[at : at+4])
		binary.LittleEndian.PutUint32(key[at:at+4], makeSubKeyCoprime(sub, uint32(length)))
	}
	return key
}

func makeSubKeyCoprime(sub uint32, length uint32) uint32 {
	for attempt := 0; attempt < subKeyAdjustAttemps; attempt++ {
		if greatestCommonDivisor(sub, length) == 1 {
			return sub
		}
		if sub == 0xFFFFFFFF {
			sub = 1
		} else {
			sub++
		}
	}
	return sub
}

func greatestCommonDivisor(left uint32, right uint32) uint32 {
	for right != 0 {
		remainder := left % right
		left = right
		right = remainder
	}
	return left
}

func buildKeyBlock() []byte {
	methods := [8]byte{1, 4, 2, 8, 0, 0, 0, 0}
	subKeys := [8]uint32{0x89E73CDB, 0x11, 0x4D1483CC, 7, 0, 0, 0, 0}
	key := make([]byte, KeySize)
	copy(key, methods[:])
	for i := 0; i < len(subKeys); i++ {
		binary.LittleEndian.PutUint32(key[8+i*4:8+i*4+4], subKeys[i])
	}
	return key
}

func buildSystemMagic() []byte {
	magic := make([]byte, MagicSize)
	copy(magic, SystemMagicText)
	return magic
}

func buildFallbackMagic() []byte {
	magic := make([]byte, MagicSize)
	copy(magic, FallbackSaveFileKey)
	return magic
}
