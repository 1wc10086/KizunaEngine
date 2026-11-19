package gothic

import "strings"

type GothicVariableTable struct {
	values map[string]int32
	order  []string
}

func NewGothicVariableTable() *GothicVariableTable {
	return &GothicVariableTable{values: make(map[string]int32)}
}

func (t *GothicVariableTable) Get(name string) int32 {
	key := normalizeVariableName(name)
	if len(key) == 0 {
		return 0
	}
	if value, ok := t.values[strings.ToLower(key)]; ok {
		return value
	}
	return 0
}

func (t *GothicVariableTable) Set(name string, value int32) {
	key := normalizeVariableName(name)
	if len(key) == 0 {
		return
	}
	lower := strings.ToLower(key)
	if _, ok := t.values[lower]; !ok {
		t.order = append(t.order, key)
	}
	t.values[lower] = clampVariableValue(value)
}

func (t *GothicVariableTable) IsDeclared(name string) bool {
	key := normalizeVariableName(name)
	if len(key) == 0 {
		return false
	}
	_, ok := t.values[strings.ToLower(key)]
	return ok
}

func (t *GothicVariableTable) Declare(name string, value int32) {
	key := normalizeVariableName(name)
	if len(key) == 0 {
		return
	}
	lower := strings.ToLower(key)
	if _, ok := t.values[lower]; !ok {
		t.order = append(t.order, key)
	}
	t.values[lower] = clampVariableValue(value)
}

type GothicVariableEntry struct {
	Name  string
	Value int32
}

func (t *GothicVariableTable) Snapshot() []GothicVariableEntry {
	result := make([]GothicVariableEntry, 0, len(t.order))
	for _, key := range t.order {
		result = append(result, GothicVariableEntry{Name: key, Value: t.values[strings.ToLower(key)]})
	}
	return result
}

func (t *GothicVariableTable) Restore(values []GothicVariableEntry) {
	t.values = make(map[string]int32)
	t.order = nil
	for _, entry := range values {
		key := normalizeVariableName(entry.Name)
		if len(key) == 0 {
			continue
		}
		lower := strings.ToLower(key)
		if _, ok := t.values[lower]; ok {
			continue
		}
		t.order = append(t.order, key)
		t.values[lower] = clampVariableValue(entry.Value)
	}
}

func normalizeVariableName(name string) string {
	return strings.TrimSpace(name)
}

func clampVariableValue(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}
