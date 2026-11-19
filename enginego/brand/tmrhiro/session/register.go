// 品牌注册：capability 层按 Brand 接口发现本引擎，drivers.go 的 blank
// import 是唯一接线点。
//
// Detect 只看 srp.pac + 至少一个资源归档是否同时存在（见 pac.FindGameDir）：
// TMR-Hiro 的归档没有魔数，靠目录结构判定比靠某个文件名更稳，也不会把
// 别的引擎的同名 .pac 误判进来。
package session

import (
	"kizuna/enginego/brand/tmrhiro/pac"
	"kizuna/enginego/core"
)

type brand struct{}

func init() { core.Register(brand{}) }

func (brand) ID() string   { return "tmrhiro" }
func (brand) Name() string { return "TMR-Hiro ADV System" }

func (brand) Detect(dir string) bool {
	if dir == "" {
		return false
	}
	_, ok := pac.FindGameDir(dir)
	return ok
}

func (brand) New() core.Engine { return New() }
