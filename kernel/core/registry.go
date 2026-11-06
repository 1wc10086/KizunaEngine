package core

import "sync"

type HandlerRef struct {
	FeatureID    string   `json:"featureId"`
	FeatureName  string   `json:"featureName"`
	Action       string   `json:"action"`
	ActionLabel  string   `json:"actionLabel"`
	Label        string   `json:"label"`
	Target       Target   `json:"target"`
	Suffixes     []string `json:"suffixes"`
	OutputSuffix string   `json:"outputSuffix"`
}

var (
	registryMu   sync.RWMutex
	registryList []Feature
	registryByID = make(map[string]Feature)
)

func Register(f Feature) {
	if f == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registryByID[f.ID()]; dup {
		return
	}
	registryByID[f.ID()] = f
	registryList = append(registryList, f)
}

func Features() []Feature {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Feature, len(registryList))
	copy(out, registryList)
	return out
}

func Find(id string) (Feature, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registryByID[id]
	return f, ok
}

func Handlers() []HandlerRef {
	registryMu.RLock()
	defer registryMu.RUnlock()
	var out []HandlerRef
	for _, f := range registryList {
		for _, h := range f.Handlers() {
			out = append(out, HandlerRef{
				FeatureID:    f.ID(),
				FeatureName:  f.Name(),
				Action:       h.Action,
				ActionLabel:  h.Label,
				Label:        f.Name() + h.Label,
				Target:       f.Target(),
				Suffixes:     h.Suffixes,
				OutputSuffix: h.OutputSuffix,
			})
		}
	}
	return out
}
