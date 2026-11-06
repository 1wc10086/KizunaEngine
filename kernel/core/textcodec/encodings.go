package textcodec

import (
	"fmt"
	"strings"
)

func DecodeUTF8(b []byte) string {
	return string(b)
}

func normalizeEncoding(encoding string) string {
	s := strings.ToLower(encoding)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func CanonicalName(encoding string) string {
	switch normalizeEncoding(encoding) {
	case "utf8":
		return "utf8"
	case "utf16", "utf16le":
		return "utf16le"
	case "gbk", "gb2312", "gb18030":
		return "gbk"
	case "cp932", "shiftjis", "sjis", "ms932", "windows31j":
		return "cp932"
	default:
		return ""
	}
}

func UnitSize(encoding string) int {
	switch normalizeEncoding(encoding) {
	case "utf16", "utf16le":
		return 2
	default:
		return 1
	}
}

func Decode(encoding string, b []byte) string {
	switch normalizeEncoding(encoding) {
	case "utf16", "utf16le":
		return DecodeUTF16LE(b)
	case "gbk", "gb2312", "gb18030":
		return DecodeGBK(b)
	case "cp932", "shiftjis", "sjis", "ms932", "windows31j":
		return DecodeCP932(b)
	default:
		return DecodeUTF8(b)
	}
}

func Encode(encoding string, s string) ([]byte, error) {
	switch normalizeEncoding(encoding) {
	case "", "utf8":
		return []byte(s), nil
	case "utf16", "utf16le":
		return EncodeUTF16LE(s)
	case "gbk", "gb2312", "gb18030":
		return EncodeGBK(s)
	case "cp932", "shiftjis", "sjis", "ms932", "windows31j":
		return EncodeCP932(s)
	default:
		return nil, fmt.Errorf("textcodec: unknown encoding %q", encoding)
	}
}

func buildEncodeMap(single func(byte) (rune, bool), double func(byte, byte) (rune, bool)) map[rune]uint32 {
	m := make(map[rune]uint32, 24000)
	put := func(r rune, code uint32) {
		if _, dup := m[r]; !dup {
			m[r] = code
		}
	}
	for b := 0x80; b < 0x100; b++ {
		if r, ok := single(byte(b)); ok {
			put(r, 0x10000|uint32(b))
		}
	}
	for lead := 0x80; lead < 0x100; lead++ {
		for trail := 0x40; trail < 0x100; trail++ {
			if r, ok := double(byte(lead), byte(trail)); ok {
				put(r, 0x20000|uint32(lead<<8|trail))
			}
		}
	}
	return m
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
