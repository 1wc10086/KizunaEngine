package bsmap

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/vm/bs"
)

type BsMapProfileMapPrms struct{}

const (
	BsMapMapPrmsTrayLeft       = 116
	BsMapMapPrmsRowTop         = 116
	BsMapMapPrmsRowStep        = 38
	BsMapMapPrmsRowHeight      = 38
	BsMapMapPrmsTrayWidth      = 644
	BsMapMapPrmsMaximumItems   = 24
	BsMapMapPrmsBaseImage      = "map_base"
	BsMapMapPrmsFrameImage     = "map_frame"
	BsMapMapPrmsRowImagePrefix = "map_place"
)

func (p *BsMapProfileMapPrms) Name() string {
	return "MapPrms"
}

func (p *BsMapProfileMapPrms) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	return context.Dialect == bs.BsScriptRev2 ||
		context.Dialect == bs.BsScriptRev3 ||
		context.Dialect == bs.BsScriptRev4
}

func (p *BsMapProfileMapPrms) Build(context BsMapContext) *BsMapLayout {
	parameters := LoadBsMapParameters(context.GameRoot)
	if parameters == nil {
		return nil
	}
	items := []BsMapItem{}
	seen := map[string]struct{}{}
	for _, index := range parameters.Jumps {
		name := parameters.Name(index)
		if len(name) == 0 || context.Script.FindLabel(name) < 0 {
			continue
		}
		if _, duplicate := seen[strings.ToLower(name)]; duplicate {
			continue
		}
		seen[strings.ToLower(name)] = struct{}{}
		position := len(items)
		if position >= BsMapMapPrmsMaximumItems {
			break
		}
		items = append(items, NewBsMapItem(
			bsMapMapPrmsRowImage(context, position), 0, -1, -1,
			BsMapMapPrmsTrayLeft,
			BsMapMapPrmsRowTop+(BsMapMapPrmsRowStep*position),
			BsMapMapPrmsTrayWidth,
			BsMapMapPrmsRowHeight,
			name))
	}
	if len(items) == 0 {
		return nil
	}
	background := BsMapMapPrmsBaseImage
	if !context.Resources.Exists(background) {
		background = bsMapMapPrmsFrameOrEmpty(context)
	}
	return &BsMapLayout{Background: background, Items: items, Source: BsMapParametersFileName}
}

func bsMapMapPrmsRowImage(context BsMapContext, position int) string {
	candidate := BsMapMapPrmsRowImagePrefix + fmt.Sprintf("%02d", position+1)
	if context.Resources.Exists(candidate) {
		return candidate
	}
	frame := bsMapMapPrmsFrameOrEmpty(context)
	if len(frame) > 0 {
		return frame
	}
	return ""
}

func bsMapMapPrmsFrameOrEmpty(context BsMapContext) string {
	if context.Resources.Exists(BsMapMapPrmsFrameImage) {
		return BsMapMapPrmsFrameImage
	}
	return ""
}
