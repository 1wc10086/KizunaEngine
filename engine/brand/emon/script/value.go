package script

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type EmonValue struct {
	number       int32
	text         string
	hasText      bool
	isIdentifier bool
}

func EmonValueFromInt(value int32) EmonValue {
	return EmonValue{number: value}
}

func EmonValueFromString(value string) EmonValue {
	return EmonValue{text: value, hasText: true}
}

func EmonValueFromIdentifier(name string) EmonValue {
	return EmonValue{text: name, hasText: true, isIdentifier: true}
}

func EmonValueFromToken(raw string) EmonValue {
	return ParseEmonValue(raw)
}

func ParseEmonValue(literal string) EmonValue {
	runes := []rune(literal)
	if len(runes) >= 2 && isQuoteRune(runes[0]) && isQuoteRune(runes[len(runes)-1]) {
		return EmonValueFromString(string(runes[1 : len(runes)-1]))
	}

	trimmed := strings.TrimSpace(literal)
	if len(trimmed) > 2 && hasHexPrefix(trimmed) {
		if hex, ok := parseHexUint32(trimmed[2:]); ok {
			return EmonValueFromInt(int32(hex))
		}
	}

	if value, ok := parseIntInvariant(trimmed); ok {
		return EmonValueFromInt(value)
	}

	return EmonValueFromString(literal)
}

func (v EmonValue) IsInteger() bool {
	return !v.hasText
}

func (v EmonValue) IsIdentifier() bool {
	return v.isIdentifier
}

func (v EmonValue) AsInt() int32 {
	if !v.hasText {
		return v.number
	}

	text := strings.TrimSpace(v.text)
	if text == "" {
		return 0
	}

	if len(text) > 2 && hasHexPrefix(text) {
		if hex, ok := parseHexUint32(text[2:]); ok {
			return int32(hex)
		}

		return 0
	}

	if value, ok := parseIntInvariant(text); ok {
		return value
	}

	return 0
}

func (v EmonValue) AsString() string {
	if v.hasText {
		return v.text
	}

	return strconv.FormatInt(int64(v.number), 10)
}

func (v EmonValue) TryGetNumber() (int32, bool) {
	if !v.hasText {
		return v.number, true
	}

	if v.isIdentifier {
		return 0, true
	}

	text := strings.TrimSpace(v.text)
	if text == "" {
		return 0, true
	}

	if len(text) > 2 && hasHexPrefix(text) {
		if hex, ok := parseHexUint32(text[2:]); ok {
			return int32(hex), true
		}
	}

	if value, ok := parseIntInvariant(text); ok {
		return value, true
	}

	return 0, false
}

func (v EmonValue) Add(other EmonValue) EmonValue {
	if !v.hasText && !other.hasText {
		return EmonValueFromInt(v.number + other.number)
	}

	return EmonValueFromString(v.AsString() + other.AsString())
}

func (v EmonValue) Subtract(other EmonValue) EmonValue {
	return EmonValueFromInt(v.AsInt() - other.AsInt())
}

func (v EmonValue) Multiply(other EmonValue) EmonValue {
	return EmonValueFromInt(v.AsInt() * other.AsInt())
}

func (v EmonValue) Divide(other EmonValue) EmonValue {
	divisor := other.AsInt()
	if divisor == 0 {
		return EmonValueFromInt(0)
	}

	dividend := v.AsInt()
	if divisor == -1 && dividend == minInt32 {
		return EmonValueFromInt(minInt32)
	}

	return EmonValueFromInt(dividend / divisor)
}

func (v EmonValue) Modulo(other EmonValue) EmonValue {
	divisor := other.AsInt()
	if divisor == 0 {
		return EmonValueFromInt(0)
	}

	return EmonValueFromInt(v.AsInt() % divisor)
}

func (v EmonValue) Equals(other EmonValue) bool {
	if !v.hasText || !other.hasText {
		return !v.hasText && !other.hasText && v.number == other.number
	}

	return v.text == other.text
}

func (v EmonValue) MarshalJSON() ([]byte, error) {
	switch {
	case v.isIdentifier:
		return marshalValueField("id", v.AsString())
	case !v.hasText:
		return []byte(`{"i":` + strconv.FormatInt(int64(v.number), 10) + `}`), nil
	default:
		return marshalValueField("s", v.AsString())
	}
}

func (v *EmonValue) UnmarshalJSON(data []byte) error {
	if jsonKind(data) != '{' {
		return errors.New("emon value must be an object")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return errors.New("emon value must be an object")
	}

	var identifier string
	var text string
	hasIdentifier := false
	var number int32
	hasNumber := false

	if raw, exists := fields["id"]; exists && jsonKind(raw) == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return errors.New("emon value object malformed")
		}

		identifier = value
		hasIdentifier = true
	}

	if raw, exists := fields["i"]; exists {
		kind := jsonKind(raw)
		if kind == '-' || (kind >= '0' && kind <= '9') {
			var value int32
			if err := json.Unmarshal(raw, &value); err != nil {
				return errors.New("emon value object malformed")
			}

			number = value
			hasNumber = true
		}
	}

	if raw, exists := fields["s"]; exists && jsonKind(raw) == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return errors.New("emon value object malformed")
		}

		text = value
	}

	switch {
	case hasIdentifier:
		*v = EmonValueFromIdentifier(identifier)
	case hasNumber:
		*v = EmonValueFromInt(number)
	default:
		*v = EmonValueFromString(text)
	}

	return nil
}

func marshalValueField(name string, value string) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	field := make([]byte, 0, len(name)+len(encoded)+4)
	field = append(field, '{', '"')
	field = append(field, name...)
	field = append(field, '"', ':')
	field = append(field, encoded...)
	field = append(field, '}')
	return field, nil
}

func jsonKind(raw json.RawMessage) byte {
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return raw[i]
		}
	}

	return 0
}
