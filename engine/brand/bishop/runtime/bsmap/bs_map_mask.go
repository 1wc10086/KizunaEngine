package bsmap

import (
	"sort"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/gfx/graphicreader"
	"kizuna/engine/brand/bishop/resource"
)

const (
	BsMapMaskBlueChannel  = 0
	BsMapMaskGreenChannel = 1
	BsMapMaskRedChannel   = 2
)

type BsMapMaskRegion struct {
	Value  int
	X      int
	Y      int
	Width  int
	Height int
}

func (r BsMapMaskRegion) CenterX() int {
	return r.X + (r.Width / 2)
}

func (r BsMapMaskRegion) CenterY() int {
	return r.Y + (r.Height / 2)
}

func BsMapMaskLoad(resources *resource.GameResourceIndex, name string) *gfx.BsgImage {
	if len(name) == 0 {
		return nil
	}
	location, ok := resources.TryGet(name)
	if !ok {
		return nil
	}
	data := location.Read()
	if len(data) == 0 {
		return nil
	}
	image, err := graphicreader.Read(data)
	if err != nil || image == nil {
		return nil
	}
	return image
}

func BsMapMaskRegions(image *gfx.BsgImage, channel int) []BsMapMaskRegion {
	type box struct {
		minX, minY, maxX, maxY int
	}
	bounds := make(map[int]*box)
	pixels := image.Pixels
	stride := image.Width * 4
	for y := 0; y < image.Height; y++ {
		row := y * stride
		for x := 0; x < image.Width; x++ {
			index := row + (x * 4)
			if index+3 >= len(pixels) || pixels[index+3] == 0 {
				continue
			}
			value := int(pixels[index+channel])
			if value == 0 {
				continue
			}
			if existing, ok := bounds[value]; ok {
				if x < existing.minX {
					existing.minX = x
				}
				if y < existing.minY {
					existing.minY = y
				}
				if x > existing.maxX {
					existing.maxX = x
				}
				if y > existing.maxY {
					existing.maxY = y
				}
				continue
			}
			bounds[value] = &box{x, y, x, y}
		}
	}
	keys := make([]int, 0, len(bounds))
	for key := range bounds {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	regions := make([]BsMapMaskRegion, 0, len(bounds))
	for _, key := range keys {
		b := bounds[key]
		regions = append(regions, BsMapMaskRegion{
			Value:  key,
			X:      b.minX,
			Y:      b.minY,
			Width:  b.maxX - b.minX + 1,
			Height: b.maxY - b.minY + 1,
		})
	}
	return regions
}

type BsMapMaskData struct {
	Width        int
	Height       int
	Values       []byte
	ItemForValue []int
}

func (d *BsMapMaskData) ValueAt(x, y int) int {
	if uint(x) >= uint(d.Width) || uint(y) >= uint(d.Height) {
		return -1
	}
	return int(d.Values[(y*d.Width)+x])
}

func (d *BsMapMaskData) ItemAt(x, y int) int {
	value := d.ValueAt(x, y)
	if value >= 0 && value < len(d.ItemForValue) {
		return d.ItemForValue[value]
	}
	return -1
}
