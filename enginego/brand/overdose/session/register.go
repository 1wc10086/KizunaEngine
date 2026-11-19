// 品牌注册：capability 层按 Brand 接口发现本引擎，drivers.go 的 blank
// import 是唯一接线点——不在这里做任何自注册副作用以外的事。
//
// Detect 只看 script.arc + yui.exe 是否同时存在：arc.FindGameDir 的魔数
// 检查是权威的，但 Detect 的调用方要在 Open 前先拿到 Brand 实例，
// 不能等 Open 失败再回头。两者口径一致即可（见 arc.IsGameDir）。
package session

import (
	"os"
	"path/filepath"

	"kizuna/enginego/brand/overdose/arc"
	"kizuna/enginego/core"
)

type brand struct{}

func init() {
	core.Register(brand{})
}

func (brand) ID() string {
	return "overdose"
}

func (brand) Name() string {
	return "Over-Dose Moon"
}

func (brand) Detect(dir string) bool {
	if dir == "" {
		return false
	}
	gameDir, ok := arc.FindGameDir(dir)
	if !ok {
		return false
	}
	// 用数据锚点而不是 yui.exe：很多汉化/绿色版会删掉或改名启动器，
	// 但 script.arc + system.arc 一定同时存在。graphic.arc 只作弱旁证，
	// 因为 resource-only 目录也可能只有它。
	for _, name := range []string{"script.arc", "system.arc"} {
		if _, err := os.Stat(filepath.Join(gameDir, name)); err != nil {
			return false
		}
	}
	return true
}

func (brand) New() core.Engine {
	return New()
}
