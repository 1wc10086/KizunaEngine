package core

import "sync"

type Brand interface {
	ID() string
	Name() string
	Detect(dir string) bool
	New() Engine
}

var (
	brandMu   sync.RWMutex
	brandList []Brand
	brandByID = make(map[string]Brand)
)

func Register(b Brand) {
	if b == nil {
		return
	}
	id := b.ID()
	if id == "" {
		return
	}
	brandMu.Lock()
	defer brandMu.Unlock()
	if _, dup := brandByID[id]; dup {
		return
	}
	brandByID[id] = b
	brandList = append(brandList, b)
}

func Brands() []Brand {
	brandMu.RLock()
	defer brandMu.RUnlock()
	out := make([]Brand, len(brandList))
	copy(out, brandList)
	return out
}

func Find(id string) (Brand, bool) {
	brandMu.RLock()
	defer brandMu.RUnlock()
	b, ok := brandByID[id]
	return b, ok
}

func Detect(dir string) (Brand, bool) {
	brandMu.RLock()
	defer brandMu.RUnlock()
	for _, b := range brandList {
		if b.Detect(dir) {
			return b, true
		}
	}
	return nil, false
}
