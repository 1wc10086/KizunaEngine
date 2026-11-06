package tj2

type Tj2StandeeEntry struct {
	Layer   int
	Index   int
	Name    string
	OffsetX int
	OffsetY int
}

type Tj2StandeeRegistry struct {
	entries []Tj2StandeeEntry
}

const (
	Tj2StandeeMinLayer = 1
	Tj2StandeeMaxLayer = 3
	Tj2StandeeCapacity = 5
	Tj2ClassicMinSlot  = 0
	Tj2ClassicMaxSlot  = 4
)

func (r *Tj2StandeeRegistry) Entries() []Tj2StandeeEntry {
	return r.entries
}

func (r *Tj2StandeeRegistry) Count() int {
	return len(r.entries)
}

func (r *Tj2StandeeRegistry) Register(layer, index int, name string, offsetX, offsetY int) bool {
	for slot := range r.entries {
		if r.entries[slot].Layer == layer {
			r.entries[slot] = Tj2StandeeEntry{Layer: layer, Index: index, Name: name, OffsetX: offsetX, OffsetY: offsetY}
			return true
		}
	}
	if len(r.entries) >= Tj2StandeeCapacity {
		return false
	}
	r.entries = append(r.entries, Tj2StandeeEntry{Layer: layer, Index: index, Name: name, OffsetX: offsetX, OffsetY: offsetY})
	return true
}

func (r *Tj2StandeeRegistry) Hide(layer int) bool {
	for slot := range r.entries {
		if r.entries[slot].Layer == layer {
			r.entries = append(r.entries[:slot], r.entries[slot+1:]...)
			return true
		}
	}
	return false
}

func (r *Tj2StandeeRegistry) TryAllocateSlot(slotCount int) (int, bool) {
	for slot := 0; slot < slotCount; slot++ {
		if !r.hasLayer(slot) {
			return slot, true
		}
	}
	return -1, false
}

func (r *Tj2StandeeRegistry) Unregister(index int) bool {
	for slot := range r.entries {
		if r.entries[slot].Index == index {
			r.entries = append(r.entries[:slot], r.entries[slot+1:]...)
			return true
		}
	}
	return false
}

func (r *Tj2StandeeRegistry) Clear() {
	r.entries = r.entries[:0]
}

func (r *Tj2StandeeRegistry) hasLayer(layer int) bool {
	for _, entry := range r.entries {
		if entry.Layer == layer {
			return true
		}
	}
	return false
}
