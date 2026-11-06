package bdt

import (
	"strings"

	"kizuna/engine/brand/bishop/resource"
)

var bdtManinSentinels = []string{"keikoku", "sb_win1", "futaxxxx"}

func DetectBdtVariant(resources *resource.GameResourceIndex) BdtVariant {
	if resources == nil {
		return ContainerVariant
	}
	for _, sentinel := range bdtManinSentinels {
		if location, ok := resources.TryGet(sentinel); ok && isLooseBdtCgImage(location) {
			return ManinVariant
		}
	}
	return ContainerVariant
}

func isLooseBdtCgImage(location *resource.ResourceLocation) bool {
	if location == nil {
		return false
	}
	path := location.LooseFile()
	if len(path) == 0 {
		return false
	}
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.EqualFold(segment, "cg") {
			return true
		}
	}
	return false
}
