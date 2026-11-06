package legacy

type LegacyFunctionSlot struct {
	Number string
	Name   string
}

const (
	legacySlotWait       = 0
	legacySlotFadeOut    = 1
	legacySlotTimeSet    = 2
	legacySlotTimeGet    = 3
	legacySlotEnding     = 4
	legacySlotEndingRoll = 5
)
