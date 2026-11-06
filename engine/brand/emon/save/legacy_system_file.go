package save

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"

	"kizuna/engine/brand/emon/script"
)

const (
	legacyNameSize       = 64
	legacyValueSize      = 4
	legacyStringTypeFlag = 0x10
	legacyFloatType      = 4
	legacyMinRecords     = 1
	legacyMaxRecords     = 8192
	legacyScanSize       = 0x10000
	legacyTrailerSize    = 19
)

var legacyPadPattern = [4]byte{0xC4, 0xF2, 0x36, 0xB4}

func TryReadLegacySystemFile(payload []byte) (map[string]script.EmonValue, bool) {
	if len(payload) < legacyValueSize+legacyValueSize {
		return nil, false
	}
	limit := len(payload) - legacyValueSize - legacyValueSize
	start := len(payload) - legacyScanSize
	if start < 0 {
		start = 0
	}
	bestCount := 0
	var best map[string]script.EmonValue
	for offset := start; offset <= limit; offset++ {
		count := int(int32(binary.LittleEndian.Uint32(payload[offset : offset+legacyValueSize])))
		if count < legacyMinRecords || count > legacyMaxRecords {
			continue
		}
		candidate := make(map[string]script.EmonValue)
		folds := make(map[string]struct{})
		if !tryReadLegacyRecords(payload, offset+legacyValueSize, count, candidate, folds) || count <= bestCount {
			continue
		}
		bestCount = count
		best = candidate
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

func tryReadLegacyRecords(payload []byte, offset int, count int, variables map[string]script.EmonValue, folds map[string]struct{}) bool {
	cursor := offset
	for index := 0; index < count; index++ {
		if cursor+legacyNameSize+legacyValueSize > len(payload) {
			return false
		}
		name, ok := tryReadLegacyName(payload, cursor)
		if !ok {
			return false
		}
		fold := strings.ToLower(name)
		if _, exists := folds[fold]; exists {
			return false
		}
		folds[fold] = struct{}{}
		cursor += legacyNameSize
		recordType := int(int32(binary.LittleEndian.Uint32(payload[cursor : cursor+legacyValueSize])))
		cursor += legacyValueSize
		if recordType&legacyStringTypeFlag != 0 {
			if cursor+legacyValueSize > len(payload) {
				return false
			}
			length := int(int32(binary.LittleEndian.Uint32(payload[cursor : cursor+legacyValueSize])))
			cursor += legacyValueSize
			if length < 0 || cursor+length > len(payload) {
				return false
			}
			variables[name] = script.EmonValueFromString(DecodeCP932Range(payload, cursor, length))
			cursor += length
			continue
		}
		if cursor+legacyValueSize > len(payload) {
			return false
		}
		value := int32(binary.LittleEndian.Uint32(payload[cursor : cursor+legacyValueSize]))
		cursor += legacyValueSize
		if recordType == legacyFloatType {
			variables[name] = script.EmonValueFromString(strconv.FormatFloat(float64(math.Float32frombits(uint32(value))), 'G', -1, 32))
		} else {
			variables[name] = script.EmonValueFromInt(value)
		}
	}
	return matchesLegacyTrailer(payload, cursor)
}

func tryReadLegacyName(payload []byte, offset int) (string, bool) {
	terminator := -1
	for i := 0; i < legacyNameSize; i++ {
		if payload[offset+i] == 0 {
			terminator = i
			break
		}
	}
	if terminator <= 0 {
		return "", false
	}
	name := DecodeCP932Range(payload, offset, terminator)
	if len(name) == 0 {
		return "", false
	}
	return name, true
}

func matchesLegacyTrailer(payload []byte, cursor int) bool {
	if cursor > len(payload) || cursor < len(payload)-legacyTrailerSize {
		return false
	}
	for position := cursor; position < len(payload); position++ {
		if payload[position] != legacyPadPattern[(position-cursor)%len(legacyPadPattern)] {
			return false
		}
	}
	return true
}
