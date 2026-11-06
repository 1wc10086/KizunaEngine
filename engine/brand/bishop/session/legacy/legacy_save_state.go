package legacy

import (
	"kizuna/engine/brand/bishop/vm/scf"
)

type LegacySaveState struct {
	Version         int
	ScenarioNumber  int
	Machine         *scf.ScfMachineState
	BackgroundName  string
	MenuVisible     bool
	Standees        []LegacyStandeeEntry
	FaceName        string
	BgmTrack        int
	Speaker         string
	Text            string
	Summary         string
	SavedAtTicks    int64
	Thumbnail       []byte
	ThumbnailWidth  int
	ThumbnailHeight int
	// CommandActive 记录存档时是否停在命令菜单。SCF 的 PC 停在 label 之后，
	// 若只恢复 PC 会直接执行第一条命令块而不是重新弹菜单，所以必须单独记。
	CommandActive bool
}
