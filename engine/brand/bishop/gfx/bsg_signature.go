package gfx

const (
	BsgHeaderSize            = 0x40
	BsgCompositionHeaderSize = 0x20
	BsgPartCountOffset       = 0x11
)

var (
	BsgGraphicsSignature    = []byte("BSS-Graphics")
	BsgCompositionSignature = []byte("BSS-Composition")
)

// CD 版光盘把签名里的部分非零字节写成了 0，这里只放宽这一种差异，并限制数量。
// 超过 2 个或出现其它差异都视为非 BSG，避免把随机数据当图片解码。
const bsgSignatureZeroTolerance = 2

func matchesTolerantSignature(data []byte, offset int, signature []byte) bool {
	if len(signature) == 0 || offset < 0 || len(data) < offset+len(signature) {
		return false
	}

	zeros := 0
	for i, want := range signature {
		got := data[offset+i]
		if got == want {
			continue
		}
		if got != 0 || want == 0 {
			return false
		}
		zeros++
		if zeros > bsgSignatureZeroTolerance {
			return false
		}
	}

	return true
}

func IsBsgGraphics(data []byte, offset int) bool {
	return offset >= 0 &&
		len(data) >= offset+BsgHeaderSize &&
		matchesTolerantSignature(data, offset, BsgGraphicsSignature)
}

func IsBsgComposition(data []byte) bool {
	return len(data) >= BsgCompositionHeaderSize &&
		matchesTolerantSignature(data, 0, BsgCompositionSignature)
}
