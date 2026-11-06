package bdt

type BdtScriptLoader interface {
	Load(scriptId string) *BdtScriptFile
}
