package script

import (
	"strings"
)

type EmonMacroRegistry struct {
	macros map[string]EmonMacroDefinition
	order  []string
}

func NewEmonMacroRegistry() *EmonMacroRegistry {
	return &EmonMacroRegistry{macros: make(map[string]EmonMacroDefinition)}
}

func (r *EmonMacroRegistry) Count() int {
	return len(r.order)
}

func (r *EmonMacroRegistry) Names() []string {
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}

func (r *EmonMacroRegistry) Add(name string, definition EmonMacroDefinition) {
	if name == "" {
		return
	}

	if key, exists := r.key(name); exists {
		r.macros[key] = definition
		return
	}

	r.macros[name] = definition
	r.order = append(r.order, name)
}

func (r *EmonMacroRegistry) TryGet(name string) (EmonMacroDefinition, bool) {
	if key, exists := r.key(name); exists {
		return r.macros[key], true
	}

	return EmonMacroDefinition{}, false
}

func (r *EmonMacroRegistry) Contains(name string) bool {
	_, exists := r.key(name)
	return exists
}

func (r *EmonMacroRegistry) key(name string) (string, bool) {
	if _, exists := r.macros[name]; exists {
		return name, true
	}

	for _, existing := range r.order {
		if strings.EqualFold(existing, name) {
			return existing, true
		}
	}

	return "", false
}
