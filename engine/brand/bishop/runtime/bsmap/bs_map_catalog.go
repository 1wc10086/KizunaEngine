package bsmap

import (
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/brand/bishop/vm/bs"
)

type BsMapCatalog struct {
	resources *resource.GameResourceIndex
	script    *vm.BsxScriptFile
	gameRoot  string
	empty     bool
	// dialect/hasDialect 记录打开游戏时的 BS 方言判定结果。
	// BSXScript 2.x（UK）是现代签名，BsDialect() 返回 (wide, false)，
	// 但 Uk profile 需要一个"这不是 Rev 系"的明确信号；hasDialect=false
	// 时 Resolve 会带着 wide 方言进 context，所有 Rev 系 profile 的
	// Matches 都不会命中，行为等价于"没有地图目录"。
	dialect    bs.BsScriptDialect
	hasDialect bool
}

var BsMapCatalogEmpty = &BsMapCatalog{empty: true}

var installedBsMapCatalog *BsMapCatalog

func BsMapCatalogInstalled() *BsMapCatalog {
	return installedBsMapCatalog
}

func BsMapCatalogInstall(catalog *BsMapCatalog) {
	installedBsMapCatalog = catalog
}

func (c *BsMapCatalog) IsEmpty() bool {
	return c == nil || c.empty || c == BsMapCatalogEmpty
}

func BuildBsMapCatalog(resources *resource.GameResourceIndex, script *vm.BsxScriptFile, gameRoot string) *BsMapCatalog {
	// BSXScript 2.x（UK 裏教師）是现代签名：hasBsDialect 恒为 false，
	// IsBsScript()/BsDialect() 都不可用——但它的 map.dat 与 BSScript 旧作
	// 完全同构。目录只负责把"资源 + 脚本 + 根目录"打包传给 profile；
	// 方言分流是每个 profile 的 Matches 自己的事（Uk 只认 Rev2..Rev4，
	// 现代剧本没有 BsDialect，Rev 系 profile 也不会误认）。
	// 旧实现要求 IsBsScript()，UK 的 SystemCall("map") 因此被当成普通
	// 标记放行，地图整屏被跳过。
	if script == nil {
		return BsMapCatalogEmpty
	}
	dialect, hasDialect := script.BsDialect()
	return &BsMapCatalog{resources: resources, script: script, gameRoot: gameRoot, dialect: dialect, hasDialect: hasDialect}
}

func (c *BsMapCatalog) Resolve(properties BsPropertyView) BsMapLayout {
	if c.IsEmpty() {
		return BsMapLayoutEmpty
	}
	context := BsMapContext{
		Resources:  c.resources,
		Script:     c.script,
		Dialect:    c.dialect,
		HasDialect: c.hasDialect,
		GameRoot:   c.gameRoot,
		Properties: properties,
	}
	return BuildBsMapProfiles(context)
}
