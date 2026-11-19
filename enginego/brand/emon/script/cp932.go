package script

import "kizuna/enginego/core/textcodec"

func DecodeCP932(b []byte) string {
	return textcodec.DecodeCP932(b)
}
