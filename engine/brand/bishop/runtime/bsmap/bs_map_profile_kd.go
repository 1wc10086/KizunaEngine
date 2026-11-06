package bsmap

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

type BsMapProfileKd struct{}

const (
	bsMapKdMapBaseImage         = "map_base1"
	bsMapKdMapBaseFallback      = "map_base2"
	bsMapKdExtraImage           = "map_extra"
	bsMapKdMenuBaseImage        = "map_menu_base"
	bsMapKdNextTimeImage        = "map_nexttime"
	bsMapKdNextTimeButtonImage  = "map_menu_button"
	bsMapKdTrayImage            = "sd_tray"
	bsMapKdHitMaskImage         = "map_hit"
	bsMapKdCharacterImagePrefix = "sd_"
	bsMapKdProbeCharacter       = "nodoka"
	bsMapKdResultProperty       = "#result"
	bsMapKdSource               = "kd-rev6"
	bsMapKdResultPropertyKey    = 2
	bsMapKdCharacterCount       = 5
	bsMapKdCharacterFrameCount  = 2
	bsMapKdTrayWidth            = 270
	bsMapKdTrayHeight           = 113
	bsMapKdDecorationHitX       = -4096
	bsMapKdDecorationHitY       = -4096
	bsMapKdLastCharacterBit     = 1 << (bsMapKdCharacterCount - 1)
	bsMapKdFullMask             = (1 << bsMapKdCharacterCount) - 1
)

var bsMapKdCharacterSuffixes = []string{
	"nodoka",
	"hikaru",
	"shizuru",
	"kotono",
	"yukie",
}

var bsMapKdCaseATrayX = []int{117, 415, 35, 495, 265}
var bsMapKdCaseATrayY = []int{408, 408, 266, 266, 206}
var bsMapKdCaseBTrayX = []int{25, 505, 127, 405, 800}
var bsMapKdCaseBTrayY = []int{398, 398, 266, 266, 600}

func (p *BsMapProfileKd) Name() string {
	return "Kd"
}

func (p *BsMapProfileKd) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	return context.Dialect == bs.BsScriptRev6 &&
		context.Resources.Exists(bsMapKdHitMaskImage) &&
		context.Resources.Exists(bsMapKdNextTimeButtonImage) &&
		context.Resources.Exists(bsMapKdCharacterImagePrefix+bsMapKdProbeCharacter)
}

func (p *BsMapProfileKd) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map kd: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapKdLayout(context)
}

func createBsMapKdLayout(context BsMapContext) *BsMapLayout {
	resources := context.Resources
	mask := context.Properties.ByKey(bsMapKdResultPropertyKey)
	caseB := mask >= 0 && (mask&bsMapKdLastCharacterBit) == 0
	enabled := mask
	if mask < 0 {
		enabled = bsMapKdFullMask
	}
	logBsMapKdMask(mask, caseB, bsMapKdDisabledNames(enabled))
	items := make([]BsMapItem, 0, (bsMapKdCharacterCount*2)+3)
	items = addBsMapKdDecoration(items, resources, bsMapKdExtraImage)
	tray := BsMapMaskLoad(resources, bsMapKdTrayImage)
	trayWidth := bsMapKdTrayWidth
	trayHeight := bsMapKdTrayHeight
	if tray != nil && tray.Width > 0 && tray.Height > 0 {
		trayWidth = tray.Width
		trayHeight = tray.Height
	}
	trayX := bsMapKdCaseATrayX
	trayY := bsMapKdCaseATrayY
	if caseB {
		trayX = bsMapKdCaseBTrayX
		trayY = bsMapKdCaseBTrayY
	}
	characters := 0
	for index := bsMapKdCharacterCount - 1; index >= 0; index-- {
		if (enabled & (1 << index)) == 0 {
			continue
		}
		image := bsMapKdCharacterImagePrefix + bsMapKdCharacterSuffixes[index]
		character := BsMapMaskLoad(resources, image)
		if character == nil || character.Width <= 0 || character.Height <= 0 {
			continue
		}
		x := trayX[index]
		y := trayY[index]
		if tray != nil {
			items = append(items, NewBsMapItemDecor(bsMapKdTrayImage, x, y, bsMapKdDecorationHitX, bsMapKdDecorationHitY))
		}
		cellHeight := character.Height / bsMapKdCharacterFrameCount
		if cellHeight < 1 {
			cellHeight = 1
		}
		originX := x + (trayWidth / 2) - (character.Width / 2)
		originY := (trayHeight / 2) + y - cellHeight
		items = append(items, NewBsMapItemWithProperty(
			image, 0, originX, originY, originX, originY, character.Width, cellHeight,
			"", bsMapKdResultProperty, bsMapKdResultPropertyKey, index+1))
		characters++
	}
	if characters == 0 {
		return nil
	}
	items = addBsMapKdDecoration(items, resources, bsMapKdMenuBaseImage)
	items = addBsMapKdDecoration(items, resources, bsMapKdNextTimeImage)
	background := ""
	if resources.Exists(bsMapKdMapBaseImage) {
		background = bsMapKdMapBaseImage
	} else if resources.Exists(bsMapKdMapBaseFallback) {
		background = bsMapKdMapBaseFallback
	}
	return &BsMapLayout{Background: background, Items: items, Source: bsMapKdSource}
}

func bsMapKdDisabledNames(enabled int) string {
	names := make([]string, 0, bsMapKdCharacterCount)
	for index := 0; index < bsMapKdCharacterCount; index++ {
		if (enabled & (1 << index)) == 0 {
			names = append(names, bsMapKdCharacterSuffixes[index])
		}
	}
	return strings.Join(names, ", ")
}

func logBsMapKdMask(mask int, caseB bool, disabled string) {
	if mask < 0 {
		core.Logf("script: map kd: %s (key %d) is unreadable, keeping Case A with all %d characters enabled", bsMapKdResultProperty, bsMapKdResultPropertyKey, bsMapKdCharacterCount)
		return
	}
	selection := ""
	if len(disabled) > 0 {
		selection = "zero bits hide " + disabled
	} else {
		selection = "all " + strconv.Itoa(bsMapKdCharacterCount) + " bits set"
	}
	caseName := "A"
	if caseB {
		caseName = "B"
	}
	core.Logf("script: map kd: %s = 0x%X selects Case %s, %s", bsMapKdResultProperty, uint32(mask), caseName, selection)
}

func addBsMapKdDecoration(items []BsMapItem, resources *resource.GameResourceIndex, image string) []BsMapItem {
	if !resources.Exists(image) {
		return items
	}
	return append(items, NewBsMapItemDecor(image, -1, -1, bsMapKdDecorationHitX, bsMapKdDecorationHitY))
}
