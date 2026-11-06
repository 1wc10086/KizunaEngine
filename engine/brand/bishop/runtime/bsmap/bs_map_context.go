package bsmap

import (
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/brand/bishop/vm/bs"
)

type BsPropertyView interface {
	ByName(name string) int
	ByKey(key int) int
}

type BsNullPropertyView struct{}

var BsNullPropertyViewInstance = &BsNullPropertyView{}

func (v *BsNullPropertyView) ByName(name string) int {
	return -1
}

func (v *BsNullPropertyView) ByKey(key int) int {
	return -1
}

type BsMapContext struct {
	Resources  *resource.GameResourceIndex
	Script     *vm.BsxScriptFile
	Dialect    bs.BsScriptDialect
	GameRoot   string
	Properties BsPropertyView
	// HasDialect 区分"真的检测到了 Rev 方言"和"现代签名回落的默认值"。
	// BSXScript 2.x/3.x 没有 Bs 方言，DetectBsDialect 返回 (wide, false)；
	// Rev/Ancient 系 profile 必须在 HasDialect=false 时全部拒绝，否则
	// 现代剧本会被旧系 profile 误吃。LabelScan（兜底行列表）不受限。
	HasDialect bool
}
