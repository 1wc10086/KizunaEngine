package gothic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
)

const (
	GothicFirstTrack        = 2
	GothicLastTrack         = 20
	GothicSegmentCount      = 19
	GothicWaveLengthSeconds = 2995.213333
)

var gothicTrackTable = []struct {
	name  string
	track int
}{
	{"平穏な一日", 2},
	{"切れた", 3},
	{"凌辱", 4},
	{"欲望", 5},
	{"コミカル", 6},
	{"シリアス", 7},
	{"夜の街", 8},
	{"愛のテーマ", 9},
	{"エンド", 10},
	{"切ない", 11},
	{"列車内", 12},
	{"時計", 13},
	{"風", 14},
	{"奈々子エンド", 15},
	{"メトロノーム", 16},
	{"仕事場", 17},
	{"雑踏", 18},
	{"雨音", 19},
	{"Ｈのテーマ", 20},
}

var gothicSegmentStarts = []float64{
	2.000000, 133.466667, 307.346667, 485.013333, 658.186667, 847.053333,
	1033.093333, 1193.626667, 1365.960000, 1557.800000, 1759.480000,
	1891.320000, 2016.066667, 2150.506667, 2274.360000, 2406.013333,
	2488.546667, 2639.626667, 2790.933333,
}

var gothicToku1SegmentStarts = []float64{
	2.000000, 158.106667, 290.093333, 432.840000, 467.706667, 662.173333,
	809.640000, 1015.680000, 1200.520000, 1401.973333, 1613.186667,
	1810.040000, 2019.480000,
}

const gothicToku1WaveLengthSeconds = 2216.493333

type GothicAudioProfile struct {
	CacheKeyPrefix    string
	FirstTrack        int
	LastTrack         int
	WaveLengthSeconds float64
	segmentStarts     []float64
	trackOf           func(string) int
}

func newGothicMazeProfile() *GothicAudioProfile {
	return &GothicAudioProfile{
		CacheKeyPrefix:    "maze-track",
		FirstTrack:        GothicFirstTrack,
		LastTrack:         GothicLastTrack,
		WaveLengthSeconds: GothicWaveLengthSeconds,
		segmentStarts:     gothicSegmentStarts,
		trackOf:           mazeGothicTrackOf,
	}
}

func newGothicToku1Profile() *GothicAudioProfile {
	return &GothicAudioProfile{
		CacheKeyPrefix:    "toku1-track",
		FirstTrack:        2,
		LastTrack:         14,
		WaveLengthSeconds: gothicToku1WaveLengthSeconds,
		segmentStarts:     gothicToku1SegmentStarts,
		trackOf:           Toku1GothicTrackOf,
	}
}

func IsGothicToku1Name(normalizedName string) bool {
	if len(normalizedName) != 4 {
		return false
	}
	if len(normalizedName) < 2 || normalizedName[0] != 'g' && normalizedName[0] != 'G' {
		return false
	}
	if normalizedName[1] != 'm' && normalizedName[1] != 'M' {
		return false
	}
	return normalizedName[2] >= '0' && normalizedName[2] <= '9' &&
		normalizedName[3] >= '0' && normalizedName[3] <= '9'
}

func Toku1GothicTrackOf(normalizedName string) int {
	if !IsGothicToku1Name(normalizedName) {
		return 0
	}
	number := (int(normalizedName[2]-'0') * 10) + int(normalizedName[3]-'0')
	if number >= 1 && number <= 13 {
		return number + 1
	}
	return 0
}

func (p *GothicAudioProfile) TrackOf(normalizedName string) int {
	return p.trackOf(normalizedName)
}

func (p *GothicAudioProfile) TrySegment(track int) (float64, float64, bool) {
	start := 0.0
	end := 0.0
	index := track - p.FirstTrack
	if index < 0 || index >= len(p.segmentStarts) {
		return start, end, false
	}
	start = p.segmentStarts[index]
	if index+1 < len(p.segmentStarts) {
		end = p.segmentStarts[index+1]
	} else {
		end = p.WaveLengthSeconds
	}
	return start, end, end > start
}

func (p *GothicAudioProfile) CacheKey(track int) string {
	return fmt.Sprintf("%s%02d", p.CacheKeyPrefix, track)
}

func mazeGothicTrackOf(key string) int {
	for _, entry := range gothicTrackTable {
		if entry.name == key {
			return entry.track
		}
	}
	return 0
}

type GothicAudioCatalog struct {
	dataDirectory  string
	resources      *resource.GameResourceIndex
	cacheDirectory string
	voiceResolved  bool
	voiceData      []byte
	voiceFound     bool
}

func NewGothicAudioCatalog(dataDirectory string, resources *resource.GameResourceIndex, cacheDirectory string) *GothicAudioCatalog {
	return &GothicAudioCatalog{dataDirectory: dataDirectory, resources: resources, cacheDirectory: cacheDirectory}
}

func (c *GothicAudioCatalog) SupportsWave() bool {
	return c.voiceContainer() != nil
}

func (c *GothicAudioCatalog) TrackOf(name string) int {
	if len(name) == 0 {
		return 0
	}
	key := normalizeGothicAudioName(name)
	return selectGothicAudioProfile(key).TrackOf(key)
}

func (c *GothicAudioCatalog) TrySegment(track int) (float64, float64, bool) {
	index := track - GothicFirstTrack
	if index < 0 || index >= len(gothicSegmentStarts) {
		return 0, 0, false
	}
	start := gothicSegmentStarts[index]
	var end float64
	if index+1 < len(gothicSegmentStarts) {
		end = gothicSegmentStarts[index+1]
	} else {
		end = GothicWaveLengthSeconds
	}
	return start, end, end > start
}

func (c *GothicAudioCatalog) ResolveMusic(name string) string {
	if len(name) == 0 {
		return ""
	}
	key := normalizeGothicAudioName(name)
	profile := selectGothicAudioProfile(key)
	track := profile.TrackOf(key)
	if track < profile.FirstTrack || track > profile.LastTrack {
		return ""
	}
	if loose := c.findLoose(fmt.Sprintf("track%02d.wav", track)); loose != "" {
		return loose
	}
	if loose := c.findLoose(fmt.Sprintf("track%03d.wav", track)); loose != "" {
		return loose
	}
	wave := findGothicCdWave(c.dataDirectory)
	if wave == "" {
		core.Logf("gothic-audio: music '%s' track %d wave not found", name, track)
		return ""
	}
	start, end, ok := profile.TrySegment(track)
	if !ok {
		return ""
	}
	if len(c.cacheDirectory) > 0 {
		_ = os.MkdirAll(c.cacheDirectory, 0o755)
	}
	path := audio.MaterializeCdSlice(wave, start, end, c.cacheDirectory, profile.CacheKey(track))
	if path == "" {
		core.Logf("gothic-audio: music '%s' track %d resolve failed", name, track)
	}
	return path
}

// ResolveTrack 直接按 CD 音轨号切出一段，供音楽鑑賞室使用。
// 鉴赏室显示的是曲名而不是脚本标签，不能走 ResolveMusic 的名字表。
func (c *GothicAudioCatalog) ResolveTrack(track int) string {
	profile := newGothicMazeProfile()
	if track < profile.FirstTrack || track > profile.LastTrack {
		return ""
	}
	wave := findGothicCdWave(c.dataDirectory)
	if wave == "" {
		core.Logf("gothic-audio: track %d wave not found", track)
		return ""
	}
	start, end, ok := profile.TrySegment(track)
	if !ok {
		return ""
	}
	if len(c.cacheDirectory) > 0 {
		_ = os.MkdirAll(c.cacheDirectory, 0o755)
	}
	path := audio.MaterializeCdSlice(wave, start, end, c.cacheDirectory, profile.CacheKey(track))
	if path == "" {
		core.Logf("gothic-audio: track %d materialize failed", track)
	}
	return path
}

func (c *GothicAudioCatalog) ResolveEffect(name string) string {
	if len(name) == 0 {
		return ""
	}
	file := filepath.Base(strings.Trim(strings.Trim(strings.TrimSpace(name), "\""), "\""))
	file = strings.TrimSpace(file)
	if len(file) == 0 {
		return ""
	}
	if direct := c.findLoose(file); direct != "" {
		return direct
	}
	stem := strings.TrimSuffix(file, filepath.Ext(file))
	if len(stem) == 0 {
		return ""
	}
	return c.findLoose(stem + ".wav")
}

func (c *GothicAudioCatalog) ResolveVoice(id int) string {
	data := c.voiceContainer()
	if data == nil {
		core.Logf("gothic-audio: voice %d unavailable: SP.PLA not found", id)
		return ""
	}
	if len(c.cacheDirectory) == 0 {
		core.Logf("gothic-audio: voice %d has no cache directory", id)
		return ""
	}
	target := filepath.Join(c.cacheDirectory, fmt.Sprintf("voice%04d.wav", id))
	wave := audio.DecodeSpPlaResource(data, id)
	if len(wave) == 0 {
		core.Logf("gothic-audio: voice %d decode failed", id)
		return ""
	}
	if info, err := os.Stat(target); err == nil && info.Size() == int64(len(wave)) {
		return target
	}
	_ = os.MkdirAll(c.cacheDirectory, 0o755)
	if info, err := os.Stat(target); err == nil && info.Size() == int64(len(wave)) {
		return target
	}
	if err := os.WriteFile(target, wave, 0o644); err != nil {
		core.Logf("gothic-audio: voice %d resolve failed: %v", id, err)
		return ""
	}
	return target
}

func (c *GothicAudioCatalog) voiceContainer() []byte {
	if c.voiceResolved {
		if c.voiceFound {
			return c.voiceData
		}
		return nil
	}
	c.voiceResolved = true
	path := c.findSpPla()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || !audio.IsSpPla(data) {
		return nil
	}
	c.voiceData = data
	c.voiceFound = true
	return c.voiceData
}

func (c *GothicAudioCatalog) findSpPla() string {
	if len(c.dataDirectory) == 0 {
		return ""
	}
	for _, file := range enumerateGothicFiles(c.dataDirectory) {
		if !strings.EqualFold(filepath.Base(file), "SP.PLA") {
			continue
		}
		if strings.EqualFold(filepath.Base(filepath.Dir(file)), "MUSIC") {
			return file
		}
	}
	return ""
}

func (c *GothicAudioCatalog) findLoose(fileName string) string {
	if len(fileName) == 0 {
		return ""
	}
	if indexed := c.indexedPath(fileName); indexed != "" {
		return indexed
	}
	for _, directory := range c.searchDirectories() {
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.EqualFold(entry.Name(), fileName) {
				return filepath.Join(directory, entry.Name())
			}
		}
	}
	return ""
}

func (c *GothicAudioCatalog) indexedPath(fileName string) string {
	if c.resources == nil {
		return ""
	}
	location, ok := c.resources.TryGet(fileName)
	if !ok || location == nil || !location.IsLoose() {
		return ""
	}
	return location.LooseFile()
}

func (c *GothicAudioCatalog) searchDirectories() []string {
	if len(c.dataDirectory) == 0 {
		return nil
	}
	directories := []string{c.dataDirectory}
	entries, err := os.ReadDir(c.dataDirectory)
	if err != nil {
		return directories
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(name, "music") || strings.EqualFold(name, "audio") {
			directories = append(directories, filepath.Join(c.dataDirectory, name))
		}
	}
	return directories
}

func selectGothicAudioProfile(normalizedName string) *GothicAudioProfile {
	if IsGothicToku1Name(normalizedName) {
		return newGothicToku1Profile()
	}
	return newGothicMazeProfile()
}

func normalizeGothicAudioName(name string) string {
	value := strings.Trim(strings.Trim(strings.TrimSpace(name), "\""), "\"")
	value = strings.TrimSpace(value)
	if comma := strings.Index(value, ","); comma > 0 {
		value = strings.TrimSpace(value[:comma])
	}
	if dot := strings.LastIndex(value, "."); dot > 0 {
		extension := value[dot:]
		if strings.EqualFold(extension, ".wav") {
			value = value[:dot]
		}
	}
	return strings.TrimSpace(value)
}

func enumerateGothicFiles(root string) []string {
	type visit struct {
		directory string
		depth     int
	}
	var result []string
	queue := []visit{{directory: root, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(current.directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(current.directory, entry.Name())
			if entry.IsDir() {
				if current.depth < 4 {
					queue = append(queue, visit{directory: path, depth: current.depth + 1})
				}
				continue
			}
			result = append(result, path)
		}
	}
	return result
}

// ResolveToku1Track 按 Toku1 的 13 段音轨表切出音乐鉴赏曲目。
//
// ResolveTrack 固定使用 MAZE 的 19 段表；Toku1 的 SP.PLA 长度和分段点
// 都不同，直接套用会切到错误的区间（甚至越过文件尾）。
func (c *GothicAudioCatalog) ResolveToku1Track(track int) string {
	profile := newGothicToku1Profile()
	if track < profile.FirstTrack || track > profile.LastTrack {
		return ""
	}
	wave := findGothicCdWave(c.dataDirectory)
	if wave == "" {
		core.Logf("gothic-audio: toku1 track %d wave not found", track)
		return ""
	}
	start, end, ok := profile.TrySegment(track)
	if !ok {
		return ""
	}
	if len(c.cacheDirectory) > 0 {
		_ = os.MkdirAll(c.cacheDirectory, 0o755)
	}
	path := audio.MaterializeCdSlice(wave, start, end, c.cacheDirectory, profile.CacheKey(track))
	if path == "" {
		core.Logf("gothic-audio: toku1 track %d materialize failed", track)
	}
	return path
}

// findGothicCdWave 先看数据目录顶层，再向下找两层。
//
// 光盘镜像解包时，CD 音轨被导出到 <dataDir>/audio/；只查顶层会得到
// 空字符串，Toku1/MAZE 的 BGM 整条静音。
func findGothicCdWave(directory string) string {
	if wave := audio.FindCdWave(directory); wave != "" {
		return wave
	}
	return audio.FindCdWaveRecursive(directory, 2)
}
