package rendering

import (
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// bsg_animation.go 播放 モーションプラス 的逐帧 CG。
//
// 这类图像的格式是 "BSS-Composition" 容器：头部 0x11 字节记帧数，
// 0x20 之后连续排列 N 个完整 BSG 帧（每帧自己带 data offset/size，
// 块长 = dataOffset + dataSize）。连带 <name>_3anim_sc.txt 描述播放帧率
// 和帧->层映射。
//
// 文件动辄 100–190MB，不能整份读进内存；这里只读 0x40 字节的头建立帧表，
// 播放时按需 ReadRange 一帧并解码，缓存最近若干帧。

const (
	animationFrameCache = 8
	animationMinFPS     = 1
	animationMaxFPS     = 120
	animationMaxFrames  = 4096
)

var errAnimationFormat = errors.New("unsupported animation container")

// animationLocation 是播放器需要的最小资源接口（由 resource.ResourceLocation 实现）。
type animationLocation interface {
	Name() string
	Size() int64
	ReadRange(offset, length int64) []byte
}

type BsgAnimationPlayer struct {
	location animationLocation
	offsets  []int64
	sizes    []int64

	fps        int
	begin      int
	end        int
	mapping    []int
	frameCount int

	start time.Time

	mu    sync.Mutex
	cache map[int]*gfx.BsgImage
	order []int
}

// NewBsgAnimationPlayer 建立播放器；script 是 <name>_anim_sc.txt 的内容。
func NewBsgAnimationPlayer(location animationLocation, script []byte) (*BsgAnimationPlayer, error) {
	if location == nil {
		return nil, errAnimationFormat
	}
	header := location.ReadRange(0, gfx.BsgCompositionHeaderSize)
	if !gfx.IsBsgComposition(header) {
		return nil, errAnimationFormat
	}
	count := int(header[gfx.BsgPartCountOffset])
	if count <= 0 || count > animationMaxFrames {
		return nil, errAnimationFormat
	}

	offsets := make([]int64, count)
	sizes := make([]int64, count)
	var offset int64 = gfx.BsgCompositionHeaderSize
	for index := 0; index < count; index++ {
		frameHeader := location.ReadRange(offset, gfx.BsgHeaderSize)
		if !gfx.IsBsgGraphics(frameHeader, 0) {
			return nil, errAnimationFormat
		}
		dataOffset := int64(int32(binary.LittleEndian.Uint32(frameHeader[0x32:])))
		dataSize := int64(int32(binary.LittleEndian.Uint32(frameHeader[0x36:])))
		block := dataOffset + dataSize
		if block < gfx.BsgHeaderSize {
			return nil, errAnimationFormat
		}
		offsets[index] = offset
		sizes[index] = block
		offset += block
		if offset > location.Size() {
			return nil, errAnimationFormat
		}
	}

	spec := parseAnimationScript(script, count)
	player := &BsgAnimationPlayer{
		location:   location,
		offsets:    offsets,
		sizes:      sizes,
		fps:        spec.fps,
		begin:      spec.begin,
		end:        spec.end,
		mapping:    spec.mapping,
		frameCount: count,
		start:      time.Now(),
		cache:      make(map[int]*gfx.BsgImage, animationFrameCache),
		order:      make([]int, 0, animationFrameCache),
	}
	core.Logf("gfx: animation '%s' frames=%d fps=%d range=%d..%d", location.Name(), count, player.fps, player.begin, player.end)
	return player, nil
}

type animationScript struct {
	fps     int
	begin   int
	end     int
	mapping []int
}

// parseAnimationScript 解析 XML 的播放参数。
//
// 不用 encoding/xml：这些文件带 Shift-JIS 注释，Unmarshal 会因为非法 UTF-8
// 整体失败。这里只认属性，非 ASCII 文本对它没有影响。
func parseAnimationScript(data []byte, frameCount int) animationScript {
	script := animationScript{fps: 30, begin: 0, end: frameCount - 1, mapping: make([]int, frameCount)}
	for index := range script.mapping {
		script.mapping[index] = index
	}
	text := string(data)

	if value, ok := animationAttribute(text, "fps"); ok {
		script.fps = value
	}
	if value, ok := animationAttribute(text, "begin"); ok {
		script.begin = value
	}
	if value, ok := animationAttribute(text, "end"); ok {
		script.end = value
	}
	if script.fps < animationMinFPS || script.fps > animationMaxFPS {
		script.fps = 30
	}
	if script.begin < 0 {
		script.begin = 0
	}
	if script.end < script.begin {
		script.end = script.begin
	}
	if script.end >= frameCount {
		script.end = frameCount - 1
	}
	if script.begin > script.end {
		script.begin = script.end
	}

	position := 0
	for {
		start := strings.Index(text[position:], "<frame")
		if start < 0 {
			break
		}
		start += position
		end := strings.Index(text[start:], ">")
		if end < 0 {
			break
		}
		end += start
		segment := text[start : end+1]
		position = end + 1

		no, okNo := animationAttribute(segment, "no")
		layer, okLayer := animationAttribute(segment, "layer")
		if !okNo || !okLayer || no < 0 || no >= frameCount || layer < 0 || layer >= frameCount {
			continue
		}
		script.mapping[no] = layer
	}
	return script
}

// animationAttribute 提取 src 里 key="123" / key=123 的十进制值。
func animationAttribute(text, key string) (int, bool) {
	search := key
	position := 0
	for {
		index := strings.Index(text[position:], search)
		if index < 0 {
			return 0, false
		}
		index += position
		position = index + len(search)
		// 必须是完整的属性名，避免把 "no" 匹配到别的单词里。
		if index > 0 {
			previous := text[index-1]
			if previous != ' ' && previous != '\t' && previous != '\r' && previous != '\n' && previous != '<' {
				continue
			}
		}
		rest := strings.TrimLeft(text[position:], " \t\r\n")
		if len(rest) == 0 || rest[0] != '=' {
			continue
		}
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		rest = strings.Trim(rest, "\"'")
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if digits == 0 {
			return 0, false
		}
		value, err := strconv.Atoi(rest[:digits])
		if err != nil {
			return 0, false
		}
		return value, true
	}
}

// Frame 返回当前时刻应该显示的帧。
//
// 动画从挂载时开始计时，按 fps 循环 begin..end；脚本释放图层时播放器随图层一起丢弃。
func (p *BsgAnimationPlayer) Frame(now time.Time) *gfx.BsgImage {
	if p == nil || p.frameCount <= 0 {
		return nil
	}
	total := p.end - p.begin + 1
	if total <= 0 {
		return nil
	}
	elapsed := now.Sub(p.start)
	if elapsed < 0 {
		elapsed = 0
	}
	tick := int(elapsed.Milliseconds() * int64(p.fps) / 1000)
	sequence := p.begin + tick%total

	source := sequence
	if sequence >= 0 && sequence < len(p.mapping) {
		source = p.mapping[sequence]
	}
	if source < 0 || source >= p.frameCount {
		source = sequence
	}
	if source < 0 || source >= p.frameCount {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if image, ok := p.cache[source]; ok {
		return image
	}
	data := p.location.ReadRange(p.offsets[source], p.sizes[source])
	if len(data) == 0 {
		return nil
	}
	image, err := gfx.DecodeBsgImage(data, 0)
	if err != nil || image == nil {
		return nil
	}
	if len(p.order) >= animationFrameCache {
		oldest := p.order[0]
		p.order = p.order[1:]
		delete(p.cache, oldest)
	}
	p.cache[source] = image
	p.order = append(p.order, source)
	return image
}
