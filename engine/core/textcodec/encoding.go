package textcodec

import (
	"fmt"
	"strings"
)

type Encoding int

const (
	Utf16 Encoding = iota
	Utf8
	Gbk
	Cp932
)

func CanonicalName(encoding string) Encoding {
	switch normalizeEncoding(encoding) {
	case "utf16", "utf16le":
		return Utf16
	case "gbk", "gb2312", "gb18030":
		return Gbk
	case "cp932", "shiftjis", "sjis", "ms932", "windows31j":
		return Cp932
	default:
		return Utf8
	}
}

func normalizeEncoding(encoding string) string {
	s := strings.ToLower(encoding)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func TextUnitSize(encoding Encoding) int {
	if encoding == Utf16 {
		return 2
	}
	return 1
}

func DecodeText(text []byte, encoding Encoding) string {
	switch encoding {
	case Utf8:
		return DecodeLooseUtf8(text)
	case Gbk:
		return DecodeGBK(text)
	case Cp932:
		return DecodeCP932(text)
	default:
		return DecodeUTF16LE(text)
	}
}

func ReadString(block []byte, offset int, encoding Encoding) string {
	if offset < 0 || offset >= len(block) {
		return ""
	}
	end := FindTerminator(block, offset, encoding)
	if end < offset {
		return ""
	}
	text := block[offset:end]
	if len(text) == 0 {
		return ""
	}
	if encoding != Utf16 && HasHighByte(text) && IsStrictUtf8(text) {
		return DecodeLooseUtf8(text)
	}
	return DecodeText(text, encoding)
}

func FindTerminator(block []byte, offset int, encoding Encoding) int {
	if encoding != Utf16 {
		for index := offset; index < len(block); index++ {
			if block[index] == 0 {
				return index
			}
		}
		return len(block)
	}
	position := offset
	for position+1 < len(block) {
		if block[position] == 0 && block[position+1] == 0 {
			return position
		}
		position += 2
	}
	return len(block)
}

func Decode(encoding string, b []byte) string {
	switch CanonicalName(encoding) {
	case Utf16:
		return DecodeUTF16LE(b)
	case Gbk:
		return DecodeGBK(b)
	case Cp932:
		return DecodeCP932(b)
	default:
		return DecodeUTF8(b)
	}
}

func Encode(encoding string, s string) ([]byte, error) {
	switch CanonicalName(encoding) {
	case Utf16:
		return EncodeUTF16LE(s)
	case Gbk:
		return EncodeGBK(s)
	case Cp932:
		return EncodeCP932(s)
	default:
		return []byte(s), nil
	}
}

func encodeWithMap(m map[rune]uint32, name string, s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for i, r := range s {
		if r < 0x80 {
			out = append(out, byte(r))
			continue
		}
		v, ok := m[r]
		if !ok {
			return nil, fmt.Errorf("textcodec: %s cannot encode U+%04X at offset %d", name, r, i)
		}
		if v>>16 == 1 {
			out = append(out, byte(v))
		} else {
			out = append(out, byte(v>>8), byte(v))
		}
	}
	return out, nil
}
