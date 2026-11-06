package legacy

const LegacyStandeeCapacity = 5

type LegacyStandeeEntry struct {
	Index int
	Name  string
}

type LegacyStandeeRegistry struct {
	entries []LegacyStandeeEntry
}

func NewLegacyStandeeRegistry() *LegacyStandeeRegistry {
	return &LegacyStandeeRegistry{}
}

func (r *LegacyStandeeRegistry) Entries() []LegacyStandeeEntry {
	return r.entries
}

func (r *LegacyStandeeRegistry) Count() int {
	return len(r.entries)
}

func (r *LegacyStandeeRegistry) Primary() *LegacyStandeeEntry {
	if len(r.entries) == 0 {
		return nil
	}
	return &r.entries[0]
}

func (r *LegacyStandeeRegistry) Secondary() *LegacyStandeeEntry {
	if len(r.entries) < 2 {
		return nil
	}
	return &r.entries[1]
}

func (r *LegacyStandeeRegistry) Register(index int, name string) bool {
	for slot := range r.entries {
		if r.entries[slot].Index == index {
			r.entries[slot] = LegacyStandeeEntry{Index: index, Name: name}
			return true
		}
	}
	if len(r.entries) >= LegacyStandeeCapacity {
		return false
	}
	r.entries = append(r.entries, LegacyStandeeEntry{Index: index, Name: name})
	return true
}

func (r *LegacyStandeeRegistry) Unregister(index int) bool {
	for slot := range r.entries {
		if r.entries[slot].Index == index {
			r.entries = append(r.entries[:slot], r.entries[slot+1:]...)
			return true
		}
	}
	return false
}

func (r *LegacyStandeeRegistry) Clear() {
	r.entries = r.entries[:0]
}
