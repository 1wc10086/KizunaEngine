package scf

import "kizuna/engine/core/textcodec"

func DetectTextEncoding(block []byte) ScriptTextEncoding {
	return textcodec.DetectTextEncoding(block)
}

func DecodeText(text []byte, encoding ScriptTextEncoding) string {
	return textcodec.DecodeText(text, encoding)
}
