package archive

import (
	"os"
	"path/filepath"
	"strings"
)

// FindGameDir 在 dir 及其一级子目录里找 X0WIN 的锚点。
//
// 不能只看 EXE：汉化/绿色版经常删掉启动器，但 X0WIN.DAT + X0WIN.LST
// 一定同时存在（数据与索引是配对的）。AUTORUN.INF / SETUP.EXE 是光盘
// 安装器残留，不能作为识别依据。
func FindGameDir(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	candidates := []string{dir}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, de := range entries {
			if de.IsDir() {
				candidates = append(candidates, filepath.Join(dir, de.Name()))
			}
		}
	}
	for _, c := range candidates {
		if isGameDir(c) {
			return c, true
		}
	}
	return "", false
}

func isGameDir(dir string) bool {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	if has("X0WIN.DAT") && has("X0WIN.LST") {
		return true
	}
	// 兼容小写/混合大小写文件名。
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	var dat, lst, so4 bool
	for _, de := range entries {
		n := strings.ToLower(de.Name())
		switch n {
		case "x0win.dat":
			dat = true
		case "x0win.lst":
			lst = true
		}
		if strings.HasSuffix(n, ".so4") {
			so4 = true
		}
	}
	return (dat && lst) || (lst && so4)
}
