package textrender

import (
	"container/list"
	"sync"
)

const glyphCacheCapacity = 4096

// GlyphBitmap 是一个字符的格子位图，行优先，len(Alpha) == Width*Height。
// 画布合成端按半角 1 格/全角 2 格安排位置时，Width 就是推进距离，因此这里
// 必须按“半角 pixelSize/2、全角 pixelSize”给出。
//
// 引擎不再内置字体，GlyphRaster 返回的位图恒为全零：它只提供推进宽度和
// 高度，真正的字形由宿主用平台字体通过 core.TextRun 绘制。保留这个类型是
// 因为 emon 与 blackpackage 的旧合成代码按 cell 推进，改动会连带改它们的
// 绘制循环。
type GlyphBitmap struct {
	Width  int
	Height int
	Alpha  []byte
}

type glyphKey struct {
	character rune
	pixelSize int
}

var (
	glyphGate  sync.Mutex
	glyphCache = map[glyphKey]*GlyphBitmap{}
	glyphNodes = map[glyphKey]*list.Element{}
	glyphOrder = list.New()
)

// GlyphRaster 返回一个字符的格子位图（cell 宽度：半角 pixelSize/2、全角
// pixelSize；高度 pixelSize），pixelSize <= 0 返回 nil。
//
// 位图没有墨：字体已从二进制里移除，字形由宿主绘制。调用方仍可用 Width
// 推进光标、用 Height 对齐行盒。返回的位图由进程级缓存持有，调用方只应
// 读取，不要修改 Alpha。
func GlyphRaster(character rune, pixelSize int) *GlyphBitmap {
	if pixelSize <= 0 {
		return nil
	}
	key := glyphKey{character: character, pixelSize: pixelSize}

	glyphGate.Lock()
	if cached, ok := glyphCache[key]; ok {
		glyphTouch(key)
		glyphGate.Unlock()
		return cached
	}
	glyphGate.Unlock()

	width := glyphCellWidth(character, pixelSize)
	bitmap := &GlyphBitmap{Width: width, Height: pixelSize, Alpha: make([]byte, width*pixelSize)}

	// 极小概率两个协程同时生成同一键时谁先插谁赢，未赢者丢弃自己结果。
	glyphGate.Lock()
	if cached, ok := glyphCache[key]; ok {
		glyphTouch(key)
		glyphGate.Unlock()
		return cached
	}
	glyphCache[key] = bitmap
	glyphTouch(key)
	for glyphOrder.Len() > glyphCacheCapacity {
		oldest := glyphOrder.Front()
		if oldest == nil {
			break
		}
		glyphOrder.Remove(oldest)
		glyphOldKey := oldest.Value.(glyphKey)
		delete(glyphNodes, glyphOldKey)
		delete(glyphCache, glyphOldKey)
	}
	glyphGate.Unlock()

	return bitmap
}

// ClearGlyphCache 只清单字形缓存，不清整段 Render 的排版缓存。
func ClearGlyphCache() {
	glyphGate.Lock()
	defer glyphGate.Unlock()
	glyphCache = map[glyphKey]*GlyphBitmap{}
	glyphNodes = map[glyphKey]*list.Element{}
	glyphOrder.Init()
}

func glyphTouch(key glyphKey) {
	if node, ok := glyphNodes[key]; ok && node != nil {
		glyphOrder.Remove(node)
	}
	glyphNodes[key] = glyphOrder.PushBack(key)
}

func glyphCellWidth(character rune, pixelSize int) int {
	if glyphIsHalfWidth(character) {
		width := pixelSize / 2
		if width < 1 {
			width = 1
		}
		return width
	}
	return pixelSize
}

func glyphIsHalfWidth(character rune) bool {
	return character < 0x80 || (character >= 0xFF61 && character <= 0xFF9F)
}
