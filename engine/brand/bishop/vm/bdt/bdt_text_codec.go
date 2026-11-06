package bdt

import (
	"unicode/utf16"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"kizuna/engine/core/textcodec"
)

type bdtTextEncoding int

const (
	bdtEncodingUtf8 bdtTextEncoding = iota
	bdtEncodingCp932
	bdtEncodingGbk
	bdtEncodingUtf16
)

func bdtDetectEncoding(block []byte) bdtTextEncoding {
	if textcodec.IsUtf8Block(block) {
		return bdtEncodingUtf8
	}
	if textcodec.ScoreCP932(block) >= textcodec.ScoreGBK(block) {
		return bdtEncodingCp932
	}
	return bdtEncodingGbk
}

func bdtDecodeText(text []byte, encoding bdtTextEncoding) string {
	switch encoding {
	case bdtEncodingUtf8:
		return string(text)
	case bdtEncodingGbk:
		decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), text)
		if err != nil {
			return string(text)
		}
		return string(decoded)
	case bdtEncodingCp932:
		decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), text)
		if err != nil {
			return string(text)
		}
		return string(decoded)
	default:
		return bdtDecodeUtf16LE(text)
	}
}

func bdtDecodeUtf16LE(text []byte) string {
	if len(text) == 0 {
		return ""
	}
	units := make([]uint16, 0, len(text)/2+1)
	for i := 0; i+1 < len(text); i += 2 {
		units = append(units, uint16(text[i])|uint16(text[i+1])<<8)
	}
	if len(text)%2 != 0 {
		units = append(units, uint16(text[len(text)-1]))
	}
	return string(utf16.Decode(units))
}
