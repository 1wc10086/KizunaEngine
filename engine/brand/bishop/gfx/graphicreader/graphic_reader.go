package graphicreader

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/gfx/bmp"
	"kizuna/engine/brand/bishop/gfx/gsa"
	"kizuna/engine/brand/bishop/gfx/kg"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
)

func Read(data []byte) (*gfx.BsgImage, error) {
	if len(data) == 0 {
		core.Logf("gfx: graphic resource is empty")
		return nil, nil
	}

	if gsa.IsGsa(data) {
		return gsa.TryRead(data), nil
	}

	if gfx.IsBsgGraphic(data) {
		image, err := gfx.ReadBsgGraphic(data)
		if err != nil {
			core.Logf("gfx: BSG decode failed: %v", err)
			return nil, err
		}

		return image, nil
	}

	if kg.IsKg(data) {
		return kg.TryRead(data), nil
	}

	if resource.IsZbmSingleImage(data) {
		return Read(resource.UnpackZbmSingleImage(data))
	}

	if bmp.IsBbm(data) {
		return bmp.TryRead(data), nil
	}

	if bmp.IsBmp(data) {
		image, err := bmp.Decode(data)
		if err != nil {
			core.Logf("gfx: BMP decode failed: %v", err)
			return nil, nil
		}

		return image, nil
	}

	core.Logf("gfx: unknown graphic signature %X", data[:min(len(data), 8)])
	return nil, nil
}
