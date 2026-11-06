package script

import (
	"strings"
	"unicode"

	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

type EmonVariableStore struct {
	values map[string]EmonValue
	order  []string
}

func NewEmonVariableStore() *EmonVariableStore {
	return &EmonVariableStore{values: make(map[string]EmonValue)}
}

func (s *EmonVariableStore) Count() int {
	return len(s.values)
}

type EmonVariableEntry struct {
	Name  string
	Value EmonValue
}

func (s *EmonVariableStore) Entries() []EmonVariableEntry {
	entries := make([]EmonVariableEntry, 0, len(s.order))
	for _, name := range s.order {
		value, exists := s.values[name]
		if !exists {
			continue
		}

		entries = append(entries, EmonVariableEntry{Name: name, Value: value})
	}

	return entries
}

func (s *EmonVariableStore) Contains(name string) bool {
	if name == "" {
		return false
	}

	_, exists := s.keyOf(name)
	return exists
}

func (s *EmonVariableStore) Lookup(name string) EmonValue {
	if key, exists := s.keyOf(name); exists {
		return s.values[key]
	}

	return EmonValueFromIdentifier(name)
}

func (s *EmonVariableStore) Set(name string, value EmonValue) {
	if name == "" {
		return
	}

	if key, exists := s.keyOf(name); exists {
		s.values[key] = value
		return
	}

	s.values[name] = value
	s.order = append(s.order, name)
}

func (s *EmonVariableStore) Remove(name string) bool {
	if name == "" {
		return false
	}

	key, exists := s.keyOf(name)
	if !exists {
		return false
	}

	delete(s.values, key)
	for i := 0; i < len(s.order); i++ {
		if s.order[i] == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}

	return true
}

func (s *EmonVariableStore) Clear() {
	s.values = make(map[string]EmonValue)
	s.order = nil
}

func (s *EmonVariableStore) LoadFrom(entries map[string]EmonValue) {
	if entries == nil {
		return
	}

	s.Clear()
	for name, value := range entries {
		if name == "" {
			continue
		}

		s.Set(name, value)
	}
}

func (s *EmonVariableStore) Clone() *EmonVariableStore {
	clone := NewEmonVariableStore()
	for _, name := range s.order {
		value, exists := s.values[name]
		if !exists {
			continue
		}

		clone.values[name] = value
		clone.order = append(clone.order, name)
	}

	return clone
}

func (s *EmonVariableStore) Snapshot() map[string]EmonValue {
	snapshot := make(map[string]EmonValue, len(s.values))
	for name, value := range s.values {
		snapshot[name] = value
	}

	return snapshot
}

func (s *EmonVariableStore) SetFromText(assignments string) {
	if assignments == "" {
		return
	}

	for _, part := range strutil.SplitTopLevel(assignments, ',') {
		name, expression, compound, ok := trySplitAssignment(part)
		if !ok {
			continue
		}

		var value EmonValue
		switch {
		case expression == "":
			value = EmonValueFromString("")
		case isWhiteSpaceOnly(expression):
			value = EmonValueFromString(expression)
		case compound == 0:
			value = s.evaluateAssignment(name, expression)
		default:
			value = s.evaluateCompoundAssignment(name, expression, compound)
		}

		s.Set(name, value)
	}
}

func (s *EmonVariableStore) evaluateAssignment(name string, expression string) (value EmonValue) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("EmonVariableStore: SetFromText failed for '%s': %s", name, panicMessage(r))
			value = EmonValueFromString(expression)
		}
	}()

	return Evaluate(expression, s)
}

func (s *EmonVariableStore) evaluateCompoundAssignment(name string, expression string, compound rune) (value EmonValue) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("EmonVariableStore: SetFromText failed for '%s': %s", name, panicMessage(r))
			value = EmonValueFromString(expression)
		}
	}()

	left := EmonValueFromInt(s.Lookup(name).AsInt())
	right := EmonValueFromInt(Evaluate(expression, s).AsInt())
	switch compound {
	case '+':
		return left.Add(right)
	case '-':
		return left.Subtract(right)
	case '*':
		return left.Multiply(right)
	case '/':
		return left.Divide(right)
	case '%':
		return left.Modulo(right)
	case '&':
		return EmonValueFromInt(left.AsInt() & right.AsInt())
	case '|':
		return EmonValueFromInt(left.AsInt() | right.AsInt())
	case '^':
		return EmonValueFromInt(left.AsInt() ^ right.AsInt())
	}

	return Evaluate(expression, s)
}

func (s *EmonVariableStore) keyOf(name string) (string, bool) {
	if _, exists := s.values[name]; exists {
		return name, true
	}

	for _, existing := range s.order {
		if strings.EqualFold(existing, name) {
			return existing, true
		}
	}

	return "", false
}

func trySplitAssignment(part string) (string, string, rune, bool) {
	runes := []rune(part)
	depth := 0
	quote := rune(0)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}

			continue
		}

		if c == '"' || c == '＂' {
			quote = c
			continue
		}

		switch c {
		case '(', '（', '[', '［', '{', '｛':
			depth++
			continue
		case ')', '）', ']', '］', '}', '｝':
			if depth > 0 {
				depth--
			}

			continue
		}

		if depth != 0 || c != '=' {
			continue
		}

		prev := rune(0)
		if i > 0 {
			prev = runes[i-1]
		}

		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if next == '=' || prev == '=' || prev == '!' || prev == '<' || prev == '>' {
			continue
		}

		nameRunes := runes[:i]
		valueRunes := runes[i+1:]
		compound := rune(0)
		if prev == '&' || prev == '|' || prev == '^' || prev == '+' ||
			prev == '-' || prev == '*' || prev == '/' || prev == '%' {
			compound = prev
			nameRunes = nameRunes[:len(nameRunes)-1]
		}

		name := trimASCIISpaces(string(nameRunes))
		expression := trimASCIISpaces(string(valueRunes))
		return name, expression, compound, name != ""
	}

	return "", "", 0, false
}

func trimASCIISpaces(text string) string {
	return strings.Trim(text, " \t\r\n")
}

func isWhiteSpaceOnly(text string) bool {
	for _, c := range text {
		if !unicode.IsSpace(c) {
			return false
		}
	}

	return text != ""
}
