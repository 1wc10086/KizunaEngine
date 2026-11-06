package script

import (
	"math"
	"strconv"
	"strings"
)

const minInt32 = -1 << 31

const maxInt32 = 1<<31 - 1

func isQuoteRune(c rune) bool {
	return c == '"' || c == '＂'
}

func hasHexPrefix(text string) bool {
	if len(text) < 2 {
		return false
	}

	return text[0] == '0' && (text[1] == 'x' || text[1] == 'X')
}

func parseHexUint32(text string) (uint32, bool) {
	value, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, false
	}

	return uint32(value), true
}

func parseIntInvariant(text string) (int32, bool) {
	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return 0, false
	}

	return int32(value), true
}

func parseInt64Invariant(text string) (int64, bool) {
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}

	return value, true
}

func parseFloatInvariant(text string) (float64, bool) {
	if strings.IndexByte(text, '_') >= 0 {
		return 0, false
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}

	return value, true
}

func truncateReal(value float64) int32 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	if value >= float64(maxInt32) {
		return maxInt32
	}

	if value <= float64(minInt32) {
		return minInt32
	}

	return int32(value)
}

func formatReal(value float64) string {
	if math.IsNaN(value) {
		return "NaN"
	}

	if math.IsInf(value, 1) {
		return "Infinity"
	}

	if math.IsInf(value, -1) {
		return "-Infinity"
	}

	mantissa, exponent := splitExponent(strconv.FormatFloat(value, 'e', -1, 64))
	if exponent < -4 || exponent >= 16 {
		return mantissa + "E" + formatExponent(exponent)
	}

	return strconv.FormatFloat(value, 'f', -1, 64)
}

func splitExponent(text string) (string, int) {
	index := strings.IndexByte(text, 'e')
	if index < 0 {
		return text, 0
	}

	exponent, err := strconv.Atoi(text[index+1:])
	if err != nil {
		return text, 0
	}

	return text[:index], exponent
}

func formatExponent(exponent int) string {
	sign := "+"
	if exponent < 0 {
		sign = "-"
		exponent = -exponent
	}

	digits := strconv.Itoa(exponent)
	if len(digits) < 2 {
		digits = "0" + digits
	}

	return sign + digits
}
