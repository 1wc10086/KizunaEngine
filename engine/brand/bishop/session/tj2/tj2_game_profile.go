package tj2

import (
	"kizuna/engine/brand/bishop/resource"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
)

// Tj2UIFamily 把“字节码方言”和“界面/演出家族”分开。
//
// 特別授業2 的 SCNDAT.TBL 是 220/1（现代方言），但标题、信息框、鉴赏室
// 仍由当年的 TJ2.EXE 自己绘制，和纯现代分支（宿主画对话框）不是一回事。
// 早期实现对这两件事都用 Variant 判断，导致 特別授業2 永远进不了经典 UI。
type Tj2UIFamily int

const (
	// Tj2UIFamilyModern：现代分支，对话框/选项交给宿主。
	Tj2UIFamilyModern Tj2UIFamily = iota
	// Tj2UIFamilyClassic：GK 光盘系（SP09 信息框、MVM01 地图）。
	Tj2UIFamilyClassic
	// Tj2UIFamilyToku2：sp_mes1 信息框 + TJ2.EXE 风格的标题/鉴赏/存读档。
	Tj2UIFamilyToku2
)

type Tj2GameProfile struct {
	Variant vmtj2.Tj2Variant
	// UIFamily 决定对话框、标题、鉴赏室由谁绘制；Variant 只决定字节码/地图语义。
	UIFamily                      Tj2UIFamily
	MessageWindowImage            string
	MessageWindowIsPartsSheet     bool
	MessageWindowSourceX          int
	MessageWindowSourceY          int
	MessageWindowSourceWidth      int
	MessageWindowSourceHeight     int
	MessageWindowDestX            int
	MessageWindowDestY            int
	MessageOriginX                int
	MessageOriginY                int
	MessageColumns                int
	MessageRows                   int
	Advance                       int
	LineHeight                    int
	GlyphWidth                    int
	GlyphHeight                   int
	TextRed                       uint8
	TextGreen                     uint8
	TextBlue                      uint8
	HasSpeakerPlate               bool
	StandeeSlotCount              int
	MenuImageName                 string
	CursorImageName               string
	CursorOffsetX                 int
	CursorOffsetY                 int
	ItemImageName                 string
	MissingWipeFallsBackToInstant bool
	ThumbnailWidth                int
	ThumbnailHeight               int
	SaveThumbnailWidth            int
	SaveThumbnailHeight           int
	SavePanelSlots                int
}

func ModernTj2Profile() Tj2GameProfile {
	return Tj2GameProfile{
		Variant:             vmtj2.Tj2VariantModern,
		UIFamily:            Tj2UIFamilyModern,
		MessageWindowImage:  MessageWindowImage,
		MessageOriginX:      MessageOriginX,
		MessageOriginY:      MessageOriginY,
		MessageColumns:      MessageColumns,
		MessageRows:         MessageRows,
		Advance:             Advance,
		LineHeight:          LineHeight,
		GlyphWidth:          MenuGlyphWidth,
		GlyphHeight:         MenuGlyphHeight,
		TextRed:             255,
		TextGreen:           255,
		TextBlue:            255,
		HasSpeakerPlate:     true,
		StandeeSlotCount:    4,
		CursorImageName:     CursorImage,
		ItemImageName:       ItemImage,
		ThumbnailWidth:      ThumbnailWidth,
		ThumbnailHeight:     ThumbnailHeight,
		SaveThumbnailWidth:  SaveThumbnailWidth,
		SaveThumbnailHeight: SaveThumbnailHeight,
		SavePanelSlots:      MenuSavePanelMaxSlots,
	}
}

func ClassicTj2Profile() Tj2GameProfile {
	return Tj2GameProfile{
		Variant:                       vmtj2.Tj2VariantClassic,
		UIFamily:                      Tj2UIFamilyClassic,
		MessageWindowImage:            "sp09.kg",
		MessageWindowIsPartsSheet:     true,
		MessageWindowSourceX:          0,
		MessageWindowSourceY:          340,
		MessageWindowSourceWidth:      640,
		MessageWindowSourceHeight:     140,
		MessageWindowDestX:            0,
		MessageWindowDestY:            340,
		MessageOriginX:                66,
		MessageOriginY:                358,
		MessageColumns:                48,
		MessageRows:                   4,
		Advance:                       10,
		LineHeight:                    22,
		GlyphWidth:                    20,
		GlyphHeight:                   22,
		TextRed:                       255,
		TextGreen:                     255,
		TextBlue:                      255,
		StandeeSlotCount:              5,
		CursorImageName:               "PCLICK.KG",
		CursorOffsetX:                 20,
		MissingWipeFallsBackToInstant: true,
		ThumbnailWidth:                96,
		ThumbnailHeight:               72,
		SaveThumbnailWidth:            160,
		SaveThumbnailHeight:           120,
		SavePanelSlots:                12,
	}
}

// Toku2Tj2Profile 在现有方言档案上套用 特別授業2 的界面布局。
//
// 为什么不直接 new 一份 ClassicTj2Profile：特別授業2 的字节码是 220/1
// （现代方言），Variant 必须保留，否则 standee/地图/指令解析都会走错分支。
// 这里只改“画在哪、用什么素材、谁负责画”。信息框是独立的 sp_mes1.kg
// 608x112，放在 (16,352)；正文原点 (34,364)，一行 24 字、最多 4 行。
func Toku2Tj2Profile(base Tj2GameProfile) Tj2GameProfile {
	profile := base
	profile.UIFamily = Tj2UIFamilyToku2
	profile.MessageWindowImage = "sp_mes1.kg"
	profile.MessageWindowIsPartsSheet = false
	profile.MessageWindowSourceX = 0
	profile.MessageWindowSourceY = 0
	profile.MessageWindowSourceWidth = 0
	profile.MessageWindowSourceHeight = 0
	profile.MessageWindowDestX = 16
	profile.MessageWindowDestY = 352
	profile.MessageOriginX = 34
	profile.MessageOriginY = 364
	profile.MessageColumns = 24
	profile.MessageRows = 4
	profile.Advance = 19
	profile.LineHeight = 22
	profile.GlyphWidth = 20
	profile.GlyphHeight = 22
	profile.ThumbnailWidth = 80
	profile.ThumbnailHeight = 60
	profile.SaveThumbnailWidth = 160
	profile.SaveThumbnailHeight = 120
	profile.SavePanelSlots = 24
	return profile
}

// ProfileForSession 在方言档案之上叠加界面家族识别。
//
// sp_mes1.kg 是 特別授業2 的信息框；sp09.kg 是 GK 光盘系。两者同时存在时
// 以 sp09 为准，避免把已经适配好的 GK 作品误判成 Toku2。
func ProfileForSession(dialect vmtj2.Tj2Dialect, resources *resource.GameResourceIndex) Tj2GameProfile {
	profile := ProfileForDialect(dialect)
	if resources == nil {
		return profile
	}
	hasMes1 := resources.Exists("sp_mes1.kg") || resources.Exists("sp_mes1")
	hasSp09 := resources.Exists("sp09.kg") || resources.Exists("sp09")
	if hasMes1 && !hasSp09 {
		return Toku2Tj2Profile(profile)
	}
	return profile
}

func ProfileForDialect(dialect vmtj2.Tj2Dialect) Tj2GameProfile {
	if dialect.Variant == vmtj2.Tj2VariantClassic {
		return ClassicTj2Profile()
	}
	return ModernTj2Profile()
}
