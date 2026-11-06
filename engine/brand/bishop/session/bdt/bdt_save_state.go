package bdt

import (
	bdtvm "kizuna/engine/brand/bishop/vm/bdt"
)

type BdtSpriteEntry struct {
	Slot int
	Name string
	X    int
	Y    int
	Mode int
}

type BdtSaveState struct {
	Version         int
	Label           string
	Machine         *bdtvm.BdtMachineState
	X               []int32
	Z               []int32
	BackgroundName  string
	Sprites         []BdtSpriteEntry
	BgmTrack        int
	Speaker         string
	Text            string
	Summary         string
	SavedAtTicks    int64
	Thumbnail       []byte
	ThumbnailWidth  int
	ThumbnailHeight int
}
