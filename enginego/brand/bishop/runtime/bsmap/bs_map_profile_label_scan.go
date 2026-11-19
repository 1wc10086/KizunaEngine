package bsmap

import (
	"sort"
	"strings"
)

type BsMapProfileLabelScan struct{}

const (
	BsMapLabelScanMaximumItems = 24
	BsMapLabelScanSuffix       = "_MAP"
)

func (p *BsMapProfileLabelScan) Name() string {
	return "labels"
}

func (p *BsMapProfileLabelScan) Matches(context BsMapContext) bool {
	return context.Script.IsBsScript()
}

func (p *BsMapProfileLabelScan) Build(context BsMapContext) *BsMapLayout {
	type candidate struct {
		offset int
		name   string
	}
	candidates := []candidate{}
	for index := 0; index < context.Script.LabelCount(); index++ {
		name := context.Script.GetLabelName(index)
		offset := context.Script.GetLabelOffset(index)
		if len(name) == 0 || offset < 0 {
			continue
		}
		if len(name) < len(BsMapLabelScanSuffix) || !strings.EqualFold(name[len(name)-len(BsMapLabelScanSuffix):], BsMapLabelScanSuffix) {
			continue
		}
		candidates = append(candidates, candidate{offset: offset, name: name})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(a, b int) bool { return candidates[a].offset < candidates[b].offset })
	items := make([]BsMapItem, 0, BsMapLabelScanMaximumItems)
	for _, entry := range candidates {
		if len(items) >= BsMapLabelScanMaximumItems {
			break
		}
		position := len(items)
		items = append(items, NewBsMapItem(
			"", 0, -1, -1,
			BsMapMapPrmsTrayLeft,
			BsMapMapPrmsRowTop+(BsMapMapPrmsRowStep*position),
			BsMapMapPrmsTrayWidth,
			BsMapMapPrmsRowHeight,
			entry.name))
	}
	background := ""
	if context.Resources.Exists(BsMapMapPrmsBaseImage) {
		background = BsMapMapPrmsBaseImage
	}
	return &BsMapLayout{Background: background, Items: items, Source: "label scan"}
}
