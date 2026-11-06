package bdt

import (
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/core"
)

type brand struct{}

func init() {
	core.Register(brand{})
}

func (brand) ID() string {
	return "bishop-bdt"
}

func (brand) Name() string {
	return "Bishop Bdt"
}

// Detect 在目录树里找 .bdt 脚本。光盘版（满淫电车等）把剧本放在 inst/ 子目录
// （inst/scene00.bdt），只看顶层会漏数整盘；AFDVD 这类解包目录剧本在顶层，
// 递归同样先命中顶层，行为不变。深度取 4：足够覆盖"挂载点/inst/剧本"
// 这类两到三级布局，又不会把整棵游戏库树都翻一遍。
func (brand) Detect(dir string) bool {
	if dir == "" {
		return false
	}
	stat, err := os.Stat(dir)
	if err != nil || !stat.IsDir() {
		return false
	}
	return hasBdtFile(dir, 4)
}

func hasBdtFile(dir string, depth int) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".bdt") {
			return true
		}
	}
	if depth <= 0 {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if hasBdtFile(filepath.Join(dir, entry.Name()), depth-1) {
			return true
		}
	}
	return false
}

func (brand) New() core.Engine {
	return NewSession()
}
