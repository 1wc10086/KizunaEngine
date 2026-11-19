// 品牌注册：Detect 只认 X0WIN 的数据锚点（DAT+LST），不用 AUTORUN.INF
// 或 SETUP.EXE——那些是光盘安装器残留，删掉启动器的绿色版仍然应该能识别。
package session

import (
	"kizuna/enginego/brand/blackpackage/archive"
	"kizuna/enginego/core"
)

type brand struct{}

func init() { core.Register(brand{}) }

func (brand) ID() string   { return "blackpackage" }
func (brand) Name() string { return "BLACKPACKAGE (FFA SYSTEM)" }

func (brand) Detect(dir string) bool {
	_, ok := archive.FindGameDir(dir)
	return ok
}

func (brand) New() core.Engine { return New() }
