package script

import "kizuna/engine/core/textcodec"

func Decode(data []byte) string {
	return textcodec.Decode("cp932", data)
}

func Encode(text string) []byte {
	return EncodeCP932(text)
}

func EncodeCP932(text string) []byte {
	out := make([]byte, 0, len(text))
	for _, symbol := range text {
		if symbol < 0xA0 {
			out = append(out, byte(symbol))
			continue
		}

		encoded, err := textcodec.EncodeCP932(string(symbol))
		if err != nil {
			out = append(out, '?')
			continue
		}

		out = append(out, encoded...)
	}

	return out
}

func DecodeScript(data []byte) string {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return string(data[3:])
	}

	if IsUtf8(data) {
		return string(data)
	}

	return DecodeCP932(data)
}

func IsUtf8(data []byte) bool {
	return textcodec.IsStrictUtf8(data)
}

func SplitLines(text string) []string {
	lines := make([]string, 0, 16)
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\r' {
			lines = append(lines, text[start:i])
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}

			start = i + 1
		} else if text[i] == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}

	lines = append(lines, text[start:])
	return lines
}
