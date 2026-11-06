package tj2

import (
	"os"

	"kizuna/engine/core"
)

type tj2Brand struct{}

func init() {
	core.Register(tj2Brand{})
}

func (tj2Brand) ID() string {
	return "bishop-tj2"
}

func (tj2Brand) Name() string {
	return "Bishop Tj2"
}

func (tj2Brand) Detect(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	// 解包后的 TJ2 常在 `游戏根/TJ2/` 下，锚文件不在入口目录本身。
	// 复用数据目录解析（最多 4 层 BFS），与 OpenExtras 找数据目录的规则保持一致。
	return hasTj2Anchor(resolveTj2DataDir(dir))
}

func (tj2Brand) New() core.Engine {
	return &Engine{}
}
