package tj2

import "fmt"

const (
	ScreenWidth  = 640
	ScreenHeight = 480

	MessageOriginX = 34
	MessageOriginY = 364
	MessageColumns = 52
	MessageRows    = 4
	LineHeight     = 22
	Advance        = 19

	ThumbnailWidth      = 80
	ThumbnailHeight     = 60
	SaveThumbnailWidth  = 160
	SaveThumbnailHeight = 120

	EndingWaitMilliseconds = 3000

	TableName          = "SCNDAT.TBL"
	MessageWindowImage = "sp_mes1.kg"
	CursorImage        = "PCLICK.KG"
	ItemImage          = "sp_item.KG"
)

func ScenarioName(number int) string {
	return fmt.Sprintf("SCN%03d.SCF", number)
}

func ThumbnailName(stem string) string {
	return stem + ".THM"
}

func SoundArchiveName(stem string) string {
	return stem + ".ADP"
}
