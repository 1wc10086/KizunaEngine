package bmp

var BmpMarker = []byte{'B', 'M'}

func IsBmp(data []byte) bool {
	if len(data) < BmpFileHeaderSize {
		return false
	}

	return isBmpMarker(data[0], data[1])
}

// CD 版 BMP 的首字节 B 会被写成 0，所以只放宽首字节；第二字节必须仍是 M，
// 否则任意两字节都会被当成 BMP，误伤非图片数据。
func isBmpMarker(first, second byte) bool {
	return second == BmpMarker[1] && (first == BmpMarker[0] || first == 0)
}
