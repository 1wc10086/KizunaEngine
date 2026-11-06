package bdt

import (
	"strings"

	"kizuna/engine/brand/bishop/resource"
	bdtvm "kizuna/engine/brand/bishop/vm/bdt"
	"kizuna/engine/core"
)

type BdtCatalog struct {
	resources *resource.GameResourceIndex
	main      *bdtvm.BdtScriptFile
}

func NewBdtCatalog(resources *resource.GameResourceIndex) *BdtCatalog {
	return &BdtCatalog{resources: resources}
}

func (c *BdtCatalog) LoadMain() *bdtvm.BdtScriptFile {
	if c.main != nil {
		return c.main
	}
	location, ok := c.resources.TryGet(ScenarioName)
	if !ok {
		core.Logf("bdt: scenario '%s' not found", ScenarioName)
		return nil
	}
	data := location.Read()
	if len(data) == 0 {
		core.Logf("bdt: scenario '%s' load failed: empty", ScenarioName)
		return nil
	}
	file := bdtvm.LoadBdtScript(data)
	if file == nil {
		core.Logf("bdt: scenario '%s' load failed", ScenarioName)
		return nil
	}
	c.main = file
	core.Logf("bdt: scenario '%s' loaded labels=%d", ScenarioName, len(file.Labels))
	return c.main
}

func (c *BdtCatalog) Load(scriptId string) *bdtvm.BdtScriptFile {
	if isBdtMainScript(scriptId) {
		return c.LoadMain()
	}
	core.Logf("bdt: unknown script '%s'", scriptId)
	return nil
}

func isBdtMainScript(scriptId string) bool {
	if len(strings.TrimSpace(scriptId)) == 0 {
		return false
	}
	trimmed := strings.ReplaceAll(strings.TrimSpace(scriptId), "\\", "/")
	stem := trimmed
	if index := strings.LastIndex(stem, "/"); index >= 0 {
		stem = stem[index+1:]
	}
	if index := strings.LastIndex(stem, "."); index >= 0 {
		stem = stem[:index]
	}
	main := ScenarioName
	if index := strings.LastIndex(main, "."); index >= 0 {
		main = main[:index]
	}
	return strings.EqualFold(stem, main)
}
