package tj2

import (
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
)

type Tj2SaveState struct {
	Version           int
	ScenarioNumber    int
	Machine           *vmtj2.Tj2MachineState
	BackgroundName    string
	BackgroundOffsetX int
	BackgroundOffsetY int
	BackgroundHistory []Tj2BackgroundLayer
	Standees          []Tj2StandeeEntry
	BgmTrack          int
	Speaker           string
	Text              string
	Summary           string
	SavedAtTicks      int64
	Thumbnail         []byte
	ThumbnailWidth    int
	ThumbnailHeight   int
	// MessagePages/MessagePage 保存经典分支的分页对白位置。
	// 旧格式没有这两项，读档时按空处理（只恢复 Text 当前页）。
	MessagePages []string
	MessagePage  int
}
