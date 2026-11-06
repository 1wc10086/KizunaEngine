package emecfg

import "kizuna/engine/core/textcodec"

func DecodeCP932(b []byte) string {
	return textcodec.DecodeCP932(b)
}
