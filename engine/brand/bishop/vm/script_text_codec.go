package vm

import "kizuna/engine/core/textcodec"

func DetectTextEncoding(block []byte) ScriptTextEncoding {
	return textcodec.DetectTextEncoding(block)
}

func TextUnitSize(encoding ScriptTextEncoding) int {
	return textcodec.TextUnitSize(encoding)
}

func DecodeText(text []byte, encoding ScriptTextEncoding) string {
	return textcodec.DecodeText(text, encoding)
}

func ReadString(block []byte, offset int, encoding ScriptTextEncoding) string {
	return textcodec.ReadString(block, offset, encoding)
}

func FindTerminator(block []byte, offset int, encoding ScriptTextEncoding) int {
	return textcodec.FindTerminator(block, offset, encoding)
}

func IsUtf8Block(block []byte) bool {
	return textcodec.IsUtf8Block(block)
}
