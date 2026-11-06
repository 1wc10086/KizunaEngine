package session

import (
	"os"

	"kizuna/engine/brand/bishop/data"
)

// probe.go 暴露给外部诊断工具（cmd 下的 bishop-probe、以及本包测试）的
// 只读入口。Session.Open 的路径全部是包内私有函数，这里只做一层转发，
// 不复制逻辑，保证探测结果与真实运行一致。

// ResolveDataDirForProbe 返回游戏数据目录（含脚本的那一层）。
func ResolveDataDirForProbe(root string) (string, error) {
	return resolveDataDir(root)
}

// FindScriptForProbe 返回脚本文件路径。
func FindScriptForProbe(dataDir string) (string, error) {
	return findScript(dataDir)
}

// FindConfigurationForProbe 返回配置文件路径（没有 cfg 时是默认路径，
// 由 config.Load 决定是否用内置布局）。
func FindConfigurationForProbe(dataDir string) string {
	return findConfiguration(dataDir)
}

// CollectMoviesForProbe 收集数据目录下的影片文件（与 Open 的索引同源）。
func CollectMoviesForProbe(dataDir string) []string {
	return collectMovies(dataDir)
}

// ParseCgCatalogForProbe 解析 cg.dat（可能不存在，返回 nil）。
func ParseCgCatalogForProbe(dataDir string) *data.CgCatalog {
	path := findFileInRoots([]string{dataDir}, "cg.dat")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	catalog, err := data.ParseCgCatalog(raw)
	if err != nil {
		return nil
	}
	return catalog
}

// ParseSceneCatalogForProbe 解析 scene.dat（可能不存在，返回 nil）。
func ParseSceneCatalogForProbe(dataDir string) *data.SceneCatalog {
	path := findFileInRoots([]string{dataDir}, "scene.dat")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data.ParseSceneCatalog(raw, nil)
}
