package gothic

import "kizuna/engine/core/textcodec"

type GothicTextEncoding = textcodec.Encoding

const (
	GothicTextUtf16 = textcodec.Utf16
	GothicTextUtf8  = textcodec.Utf8
	GothicTextGbk   = textcodec.Gbk
	GothicTextCp932 = textcodec.Cp932
)

func DetectGothicTextEncoding(block []byte) GothicTextEncoding {
	return textcodec.DetectTextEncoding(block)
}

func DecodeGothicText(block []byte, encoding GothicTextEncoding) string {
	return textcodec.DecodeText(block, encoding)
}
