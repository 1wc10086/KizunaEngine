package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/core"
)

var legacyBgmBounds = [][2]float64{
	{0.000000, 63.640000},
	{63.640000, 111.266667},
	{111.266667, 181.773333},
	{181.773333, 221.720000},
	{221.720000, 284.600000},
	{284.600000, 366.440000},
	{366.440000, 396.946667},
	{396.946667, 456.760000},
	{456.760000, 487.440000},
	{487.440000, 520.706667},
	{520.706667, 552.813333},
	{552.813333, 665.173333},
	{665.173333, 703.720000},
	{703.720000, 777.280000},
	{777.280000, 857.133333},
	{857.133333, 1066.973333},
	{1066.973333, 1273.506667},
	{1273.506667, 1334.386667},
	{1334.386667, 1475.560000},
	{1475.560000, 1683.853333},
	{1683.853333, 1856.186667},
	{1856.186667, 1979.040000},
	{1979.040000, 2101.893333},
	{2101.893333, 2165.866667},
	{2165.866667, 2338.253333},
	{2338.253333, 2407.240000},
	{2407.240000, 2445.040000},
	{2445.040000, 2485.906667},
	{2485.906667, 2559.320000},
	{2559.320000, 2592.306667},
	{2592.306667, 2674.186667},
	{2674.186667, 2774.720000},
	{2774.720000, 2845.253333},
	{2845.253333, 2924.146667},
}

var legacyAudioSubdirectories = []string{"audio", "AUDIO"}

type LegacyBgmCatalog struct {
	dataDirectory  string
	cacheDirectory string
	segments       map[int][2]float64
}

func NewLegacyBgmCatalog(dataDirectory, cacheDirectory string) *LegacyBgmCatalog {
	catalog := &LegacyBgmCatalog{
		dataDirectory:  dataDirectory,
		cacheDirectory: cacheDirectory,
		segments:       make(map[int][2]float64, len(legacyBgmBounds)),
	}
	for index, bounds := range legacyBgmBounds {
		catalog.segments[index+2] = bounds
	}
	return catalog
}

func (c *LegacyBgmCatalog) Segments() map[int][2]float64 {
	return c.segments
}

func (c *LegacyBgmCatalog) Resolve(track int) string {
	if track < 2 || track > 35 {
		return ""
	}
	if loose := c.findLooseTrack(track); loose != "" {
		return loose
	}
	bounds, ok := c.segments[track]
	if !ok {
		return ""
	}
	wave := audio.FindCdWave(c.dataDirectory)
	if wave == "" {
		core.Logf("legacy: bgm track %d wave not found", track)
		return ""
	}
	key := fmt.Sprintf("bgm-track%02d", track)
	path := audio.MaterializeCdSlice(wave, bounds[0], bounds[1], c.cacheDirectory, key)
	if path == "" {
		core.Logf("legacy: bgm track %d slice failed", track)
		return ""
	}
	return path
}

const legacyAncestorDepth = 2

// looseRoots 返回数据目录及其上两级祖先，以及它们下的 audio 子目录。
// 解包后的 WAV 可能与 SCNFILE.PFT 不在同一层，所以需要向上查找。
func (c *LegacyBgmCatalog) looseRoots() []string {
	roots := make([]string, 0, (legacyAncestorDepth+1)*(len(legacyAudioSubdirectories)+1))
	dir := filepath.Clean(c.dataDirectory)
	for level := 0; level <= legacyAncestorDepth; level++ {
		roots = append(roots, dir)
		for _, sub := range legacyAudioSubdirectories {
			roots = append(roots, filepath.Join(dir, sub))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return roots
}

func (c *LegacyBgmCatalog) findLooseTrack(track int) string {
	if c.dataDirectory == "" {
		return ""
	}
	name := fmt.Sprintf("track%02d.wav", track)
	for _, root := range c.looseRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.EqualFold(entry.Name(), name) {
				return filepath.Join(root, entry.Name())
			}
		}
	}
	return ""
}
