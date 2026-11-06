package tj2

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/brand/bishop/resource"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	tj2ModernTrackCount    = 18
	tj2ModernSegmentCount  = 19
	tj2ModernMinimumGap    = 1.0
	tj2ClassicSegmentCount = 15
	tj2ClassicMinimumGap   = 4.0
	tj2BgmSearchDepth      = 4
)

var tj2ModernFallbackSegments = []audio.CdSegment{
	{Start: 2.000000, End: 104.910839},
	{Start: 106.910839, End: 204.899048},
	{Start: 206.899048, End: 306.930613},
	{Start: 308.930612, End: 404.875465},
	{Start: 406.875465, End: 496.876009},
	{Start: 498.876009, End: 566.771156},
	{Start: 568.771156, End: 658.678820},
	{Start: 660.678821, End: 758.667030},
	{Start: 760.667029, End: 870.822494},
	{Start: 872.822494, End: 987.807709},
	{Start: 989.807710, End: 1086.588481},
	{Start: 1088.588481, End: 1191.406440},
	{Start: 1193.406440, End: 1267.431655},
	{Start: 1269.431655, End: 1369.277460},
	{Start: 1371.277460, End: 1461.185124},
	{Start: 1463.185125, End: 1488.007257},
	{Start: 1490.007256, End: 1609.822222},
	{Start: 1611.822222, End: 1691.977505},
	{Start: 1693.977506, End: 1866.693333},
}

var tj2ClassicFallbackSegments = []audio.CdSegment{
	{Start: 0, End: 188.5},
	{Start: 190.5, End: 361.9},
	{Start: 363.5, End: 513.2},
	{Start: 515.5, End: 654.0},
	{Start: 656, End: 808.5},
	{Start: 810.5, End: 956.5},
	{Start: 958.5, End: 1089.2},
	{Start: 1091, End: 1274.0},
	{Start: 1276, End: 1448.9},
	{Start: 1451.5, End: 1604.4},
	{Start: 1606, End: 1753.6},
	{Start: 1755.5, End: 1927.8},
	{Start: 1930, End: 2106.7},
	{Start: 2108.5, End: 2137.0},
	{Start: 2139, End: 2317.8},
}

type Tj2BgmCatalog struct {
	gameRoot       string
	dataDirectory  string
	cacheDirectory string
	extraDirs      []string
	trackNames     []string
	classic        bool
}

func NewTj2BgmCatalog(gameRoot, dataDirectory, cacheDirectory string) *Tj2BgmCatalog {
	return &Tj2BgmCatalog{
		gameRoot:       gameRoot,
		dataDirectory:  dataDirectory,
		cacheDirectory: cacheDirectory,
	}
}

func (c *Tj2BgmCatalog) ConfigureTable(table *Tj2DataTable) {
	c.trackNames = append([]string(nil), table.TrackNumbers()...)
	c.classic = table.Dialect().Variant == vmtj2.Tj2VariantClassic
}

func (c *Tj2BgmCatalog) ConfigureExtraDirectories(extraDirs []string) {
	c.extraDirs = c.extraDirs[:0]
	for _, dir := range extraDirs {
		if strings.TrimSpace(dir) != "" {
			c.extraDirs = append(c.extraDirs, dir)
		}
	}
}

func (c *Tj2BgmCatalog) Resolve(trackIndex int) string {
	if len(c.trackNames) > 0 {
		if trackIndex < 0 || trackIndex >= len(c.trackNames) {
			core.Logf("tj2: bgm track index out of range: %d", trackIndex)
			return ""
		}
		return c.resolveName(c.trackNames[trackIndex], segmentIndexFor(c.trackNames[trackIndex]))
	}
	if trackIndex < 0 || trackIndex >= tj2ModernTrackCount {
		core.Logf("tj2: bgm track index out of range: %d", trackIndex)
		return ""
	}
	return c.resolveName(fmt.Sprintf("bgm%04d", trackIndex+1), trackIndex)
}

func (c *Tj2BgmCatalog) ResolveByName(name string) string {
	if strings.TrimSpace(name) == "" {
		core.Logf("tj2: bgm name is empty")
		return ""
	}
	stem := strings.TrimSpace(name)
	base := filepath.Base(stem)
	if dot := strings.LastIndex(base, "."); dot >= 0 {
		base = base[:dot]
	}
	if len(base) == 0 {
		core.Logf("tj2: bgm name is not a file name: '%s'", name)
		return ""
	}
	return c.resolveName(base, segmentIndexFor(base))
}

func (c *Tj2BgmCatalog) segmentCount() int {
	if c.classic {
		return tj2ClassicSegmentCount
	}
	return tj2ModernSegmentCount
}

func (c *Tj2BgmCatalog) minimumGap() float64 {
	if c.classic {
		return tj2ClassicMinimumGap
	}
	return tj2ModernMinimumGap
}

func (c *Tj2BgmCatalog) fallbackSegments() []audio.CdSegment {
	if c.classic {
		return tj2ClassicFallbackSegments
	}
	return tj2ModernFallbackSegments
}

func (c *Tj2BgmCatalog) trimStem(segmentIndex int) string {
	if c.classic {
		return fmt.Sprintf("b%02d", segmentIndex+1)
	}
	return fmt.Sprintf("bgm%04d", segmentIndex+1)
}

func (c *Tj2BgmCatalog) cacheKey(segmentIndex int) string {
	if c.classic {
		return fmt.Sprintf("gk1-b%02d", segmentIndex+1)
	}
	return fmt.Sprintf("tj2-bgm%04d", segmentIndex+1)
}

func (c *Tj2BgmCatalog) resolveName(stem string, segmentIndex int) string {
	fileName := stem + ".ADP"
	if loose := c.findLooseFile(fileName); loose != "" {
		return loose
	}
	if len(c.dataDirectory) > 0 {
		index := resource.Build(c.dataDirectory, c.extraDirs...)
		if _, ok := index.TryGet(fileName); ok {
			index.Dispose()
			return fileName
		}
		index.Dispose()
	}
	segmentCount := c.segmentCount()
	if segmentIndex < 0 || segmentIndex >= segmentCount {
		core.Logf("tj2: bgm '%s' not found and has no cd segment", stem)
		return ""
	}
	wave := c.findCdWave()
	if wave == "" {
		core.Logf("tj2: bgm '%s' not found and cd wave is missing", stem)
		return ""
	}
	segments := audio.CdSegments(wave, c.minimumGap(), c.fallbackSegments())
	bounds := c.trimSegmentEnd(c.pickSegment(segments, segmentIndex, segmentCount), segmentIndex)
	path := audio.MaterializeCdSlice(wave, bounds.Start, bounds.End, c.cacheDirectory, c.cacheKey(segmentIndex))
	if path == "" {
		core.Logf("tj2: bgm '%s' cd slice failed", stem)
		return ""
	}
	return path
}

func (c *Tj2BgmCatalog) pickSegment(segments []audio.CdSegment, segmentIndex, segmentCount int) audio.CdSegment {
	if len(segments) == segmentCount+1 &&
		segments[0].End-segments[0].Start <= 5.0 &&
		segmentIndex+1 < len(segments) {
		return segments[segmentIndex+1]
	}
	if segmentIndex < len(segments) {
		return segments[segmentIndex]
	}
	return c.fallbackSegments()[segmentIndex]
}

func (c *Tj2BgmCatalog) findCdWave() string {
	if direct := audio.FindCdWave(c.gameRoot); direct != "" {
		return direct
	}
	for _, extra := range c.extraDirs {
		if candidate := audio.FindCdWaveRecursive(extra, tj2BgmSearchDepth); candidate != "" {
			return candidate
		}
	}
	if len(c.gameRoot) == 0 {
		return ""
	}
	if info, err := os.Stat(c.gameRoot); err != nil || !info.IsDir() {
		return ""
	}
	type entry struct {
		path  string
		depth int
	}
	queue := []entry{{path: c.gameRoot, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if isTj2CdCandidate(current.path) {
			if candidate := audio.FindCdWave(current.path); candidate != "" {
				return candidate
			}
		}
		if current.depth >= tj2BgmSearchDepth {
			continue
		}
		children, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.IsDir() {
				queue = append(queue, entry{path: filepath.Join(current.path, child.Name()), depth: current.depth + 1})
			}
		}
	}
	return ""
}

func isTj2CdCandidate(directory string) bool {
	name := filepath.Base(directory)
	if len(name) > 0 && (strings.EqualFold(name, "audio") || strings.EqualFold(name, "music")) {
		return true
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, item := range entries {
		if !item.IsDir() && strings.HasSuffix(strings.ToLower(item.Name()), ".wav") {
			return true
		}
	}
	return false
}

func (c *Tj2BgmCatalog) trimSegmentEnd(bounds audio.CdSegment, segmentIndex int) audio.CdSegment {
	if duration, ok := c.adpDuration(segmentIndex); ok {
		trimmed := bounds.Start + duration
		if trimmed > bounds.Start && trimmed < bounds.End {
			return audio.CdSegment{Start: bounds.Start, End: trimmed}
		}
	}
	return bounds
}

func (c *Tj2BgmCatalog) adpDuration(segmentIndex int) (float64, bool) {
	if segmentIndex < 0 || segmentIndex >= c.segmentCount() {
		return 0, false
	}
	path := c.findLooseFile(c.trimStem(segmentIndex) + ".ADP")
	if path == "" {
		return 0, false
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() < 196 {
		return 0, false
	}
	stream, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer stream.Close()
	header := make([]byte, 196)
	read := 0
	for read < len(header) {
		n, err := stream.Read(header[read:])
		if n <= 0 || err != nil {
			return 0, false
		}
		read += n
	}
	sampleRate := binutil.U32(header[0:4])
	samples := int32(binutil.U32(header[188:192]))
	if sampleRate == 0 || samples <= 0 {
		return 0, false
	}
	seconds := float64(samples) / float64(sampleRate)
	if seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

func segmentIndexFor(stem string) int {
	if len(stem) == 7 && strings.EqualFold(stem[:3], "bgm") {
		if modern, ok := tj2TryParse(stem[3:7]); ok && modern >= 1 && modern <= tj2ModernSegmentCount {
			return modern - 1
		}
	}
	if len(stem) == 3 && (stem[0] == 'b' || stem[0] == 'B') {
		if classic, ok := tj2TryParse(stem[1:3]); ok && classic >= 1 && classic <= tj2ClassicSegmentCount {
			return classic - 1
		}
	}
	return -1
}

func tj2TryParse(text string) (int, bool) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return 0, false
	}
	index := 0
	sign := 1
	if trimmed[0] == '+' || trimmed[0] == '-' {
		if trimmed[0] == '-' {
			sign = -1
		}
		index++
	}
	if index >= len(trimmed) {
		return 0, false
	}
	value := 0
	for ; index < len(trimmed); index++ {
		if trimmed[index] < '0' || trimmed[index] > '9' {
			return 0, false
		}
		value = value*10 + int(trimmed[index]-'0')
	}
	return sign * value, true
}

func (c *Tj2BgmCatalog) findLooseFile(fileName string) string {
	if len(c.gameRoot) > 0 {
		if info, err := os.Stat(c.gameRoot); err == nil && info.IsDir() {
			if found := findLooseFileUnder(fileName, c.gameRoot); found != "" {
				return found
			}
		}
	}
	for _, extra := range c.extraDirs {
		if info, err := os.Stat(extra); err != nil || !info.IsDir() {
			continue
		}
		if found := findLooseFileUnder(fileName, extra); found != "" {
			return found
		}
	}
	return ""
}

func findLooseFileUnder(fileName, root string) string {
	type entry struct {
		path  string
		depth int
	}
	queue := []entry{{path: root, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		subdirs := []string{}
		for _, item := range entries {
			if item.IsDir() {
				subdirs = append(subdirs, filepath.Join(current.path, item.Name()))
			} else if strings.EqualFold(item.Name(), fileName) {
				return filepath.Join(current.path, item.Name())
			}
		}
		if current.depth >= tj2BgmSearchDepth {
			continue
		}
		for _, sub := range subdirs {
			queue = append(queue, entry{path: sub, depth: current.depth + 1})
		}
	}
	return ""
}
