package audio

import "kizuna/engine/core/binutil"

func putUint16(b []byte, v uint16) {
	binutil.PutU16(b, v)
}

func putUint32(b []byte, v uint32) {
	binutil.PutU32(b, v)
}

func readUint16(b []byte) uint16 {
	return binutil.U16(b)
}

func readUint32(b []byte) uint32 {
	return binutil.U32(b)
}

func readInt32(b []byte) int32 {
	return binutil.I32(b)
}

func isFourCc(data []byte, offset int, a, b, c, d byte) bool {
	return offset+4 <= len(data) &&
		data[offset] == a &&
		data[offset+1] == b &&
		data[offset+2] == c &&
		data[offset+3] == d
}
