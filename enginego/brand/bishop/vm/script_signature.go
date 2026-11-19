package vm

import (
	"strings"
)

const ScriptSignatureSize = 16

const (
	ScriptSignatureModern  = "BSXScript"
	ScriptSignatureClassic = "BSScript Rev."
	ScriptSignatureAncient = "BSScript"
)

func DetectSignature(data []byte) (string, bool) {
	if len(data) < ScriptSignatureSize {
		return "", false
	}
	header := data[:ScriptSignatureSize]
	if startsWithBytes(header, []byte(ScriptSignatureClassic)) {
		return ScriptSignatureClassic + versionDigits(header, len(ScriptSignatureClassic)), true
	}
	if startsWithBytes(header, []byte(ScriptSignatureModern)) {
		return ScriptSignatureModern + " " + versionDigits(header, len(ScriptSignatureModern)), true
	}
	if startsWithBytes(header, []byte(ScriptSignatureAncient)) {
		return ScriptSignatureAncient, true
	}
	return "", false
}

func LayoutKindForSignature(signature string) ScriptLayoutKind {
	if strings.HasPrefix(signature, ScriptSignatureAncient) {
		if strings.HasPrefix(signature, ScriptSignatureClassic) {
			return LayoutClassic
		}
		return LayoutAncient
	}
	if strings.HasPrefix(signature, ScriptSignatureModern) {
		if strings.Contains(signature, " 3.") {
			return LayoutModern
		}
		return LayoutClassic
	}
	return LayoutModern
}

func IsModernSignature(signature string) bool {
	return LayoutKindForSignature(signature) == LayoutModern
}

func startsWithBytes(header, prefix []byte) bool {
	if len(header) < len(prefix) {
		return false
	}
	for index := range prefix {
		if header[index] != prefix[index] {
			return false
		}
	}
	return true
}

func versionDigits(header []byte, start int) string {
	var builder strings.Builder
	for index := start; index < len(header); index++ {
		value := header[index]
		if value == 0 {
			break
		}
		if (value >= '0' && value <= '9') || value == '.' {
			builder.WriteByte(value)
		}
	}
	return builder.String()
}
