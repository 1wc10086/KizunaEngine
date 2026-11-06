package gsa

import "kizuna/engine/core/binutil"

const GsaSignatureValue = uint32(0x4D428E8C)

func IsGsa(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return binutil.U32(data) == GsaSignatureValue
}
