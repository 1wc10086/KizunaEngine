package vm

import (
	"math"
	"strings"
)

func isNaN(value float64) bool {
	return math.IsNaN(value)
}

func roundAwayFromZero(value float64) int {
	return int(math.Floor(value + 0.5))
}

func indexOfToken(text, token string) int {
	for i := 0; i+len(token) <= len(text); i++ {
		match := true
		for j := 0; j < len(token); j++ {
			a := text[i+j]
			b := token[j]
			if a == b {
				continue
			}
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func replaceLineEndings(text, replacement string) string {
	replacer := strings.NewReplacer("\r\n", replacement, "\r", replacement, "\n", replacement)
	return replacer.Replace(text)
}

type foldMap[V any] struct {
	keys  []string
	vals  map[string]V
	index map[string]string
}

func newFoldMap[V any]() *foldMap[V] {
	return &foldMap[V]{vals: make(map[string]V), index: make(map[string]string)}
}

func (m *foldMap[V]) Len() int {
	return len(m.vals)
}

func (m *foldMap[V]) Get(name string) (V, bool) {
	if value, exists := m.vals[name]; exists {
		return value, true
	}
	if original, exists := m.index[strings.ToUpper(name)]; exists {
		if value, ok := m.vals[original]; ok && strings.EqualFold(original, name) {
			return value, true
		}
	}
	var zero V
	return zero, false
}

func (m *foldMap[V]) Contains(name string) bool {
	_, exists := m.Get(name)
	return exists
}

func (m *foldMap[V]) Set(name string, value V) {
	if _, exists := m.vals[name]; exists {
		m.vals[name] = value
		return
	}
	upper := strings.ToUpper(name)
	if original, exists := m.index[upper]; exists {
		if _, ok := m.vals[original]; ok && strings.EqualFold(original, name) {
			m.vals[original] = value
			return
		}
	}
	m.vals[name] = value
	m.keys = append(m.keys, name)
	if _, exists := m.index[upper]; !exists {
		m.index[upper] = name
	}
}

func (m *foldMap[V]) Delete(name string) {
	original := name
	if _, exists := m.vals[name]; exists {
	} else if candidate, exists := m.index[strings.ToUpper(name)]; exists {
		if _, ok := m.vals[candidate]; !ok || !strings.EqualFold(candidate, name) {
			return
		}
		original = candidate
	} else {
		return
	}
	delete(m.vals, original)
	if current, exists := m.index[strings.ToUpper(original)]; exists && current == original {
		delete(m.index, strings.ToUpper(original))
	}
	for i, key := range m.keys {
		if key == original {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

func (m *foldMap[V]) Clear() {
	m.keys = nil
	m.vals = make(map[string]V)
	m.index = make(map[string]string)
}

func (m *foldMap[V]) Keys() []string {
	return append([]string(nil), m.keys...)
}

func (m *foldMap[V]) Snapshot() map[string]V {
	out := make(map[string]V, len(m.vals))
	for _, key := range m.keys {
		if value, exists := m.vals[key]; exists {
			out[key] = value
		}
	}
	return out
}

type foldSet struct {
	inner *foldMap[struct{}]
}

func newFoldSet() *foldSet {
	return &foldSet{inner: newFoldMap[struct{}]()}
}

func (s *foldSet) Add(name string) bool {
	if s.inner.Contains(name) {
		return false
	}
	s.inner.Set(name, struct{}{})
	return true
}

func (s *foldSet) Contains(name string) bool {
	return s.inner.Contains(name)
}

func (s *foldSet) Remove(name string) {
	s.inner.Delete(name)
}

func (s *foldSet) Clear() {
	s.inner.Clear()
}
