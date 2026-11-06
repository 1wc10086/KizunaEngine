package legacy

import (
	"kizuna/engine/core"
)

type legacyBrand struct{}

func (legacyBrand) ID() string {
	return "bishop-legacy"
}

func (legacyBrand) Name() string {
	return "Bishop Legacy"
}

func (legacyBrand) Detect(dir string) bool {
	if hasLegacyAnchor(dir) {
		return true
	}
	return findLegacyDataDir(dir) != dir
}

func (legacyBrand) New() core.Engine {
	return New()
}

func init() {
	core.Register(legacyBrand{})
}
