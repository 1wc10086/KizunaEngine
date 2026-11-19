package gfx

import (
	"container/list"
	"strings"
	"sync"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/arc"
	"kizuna/enginego/core"
)

// 本包把 CPB 字节摊成 bishopgfx.BsgImage 并做有界 LRU。
// Arch loader 是解码钩子：session 直接给 arc.Set 的图形查表，
// 这里只管“解出来、摊平、缓存、按名复用”，不碰存档的流密码那层。

// Image 是 CPB 解码后的可复用图。别名 bishop 侧类型，原因很直接：
// BgraCanvas 的 Blit 系列只认 *BsgImage，另立类型就得处处做转换。
type Image = bishopgfx.BsgImage

// Loader 按名取 CPB 原始字节。arc.Set.Image 已能给 (w,h,bgra)，
// 但这里保留“给字节”的钩子：测试与未开存档的合成路径只用字节也能跑。
type Loader func(name string) ([]byte, error)

// ArchiveSet 是最小存档面：只需按名取字节。*arc.Set 原样满足，
// 不在 gfx 侧 import 整个 arc 集合之外的东西，避免循环。
type ArchiveSet interface {
	Entry(name string) ([]byte, error)
}

func cacheKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Cache 是按规范化名（小写、无空格）索引的有界 LRU。
// 无界缓存会把 1.1G 的 graphic.arc 全摊出来，内存先爆；
// 所以按字节预算与条目数双限，超限逐出最久未用者。
type Cache struct {
	mu        sync.Mutex
	loader    Loader
	maxBytes  int64
	maxItems  int
	images    map[string]*Image
	nodes     map[string]*list.Element
	order     *list.List // element.Value = key
	bytes     int64
	misses    map[string]struct{} // 记过“解不出”的名，避免每次合成都重试
	hits      int64
	missCount int64
}

// NewCache 建缓存。预算 <=0 时用默认（64MB / 96 张），与 bishop 侧 ImageCache
// 同量级：CPB 多为 800x600x4 ≈ 1.9MB，96 张约 180MB 不够，故默认收紧到 64MB。
// 立绘常驻槽位靠 stage 层的 Cur 立绘名复用缓存命中，不要为它单独留大预算。
func NewCache(loader Loader) *Cache {
	return NewCacheBounded(loader, 64<<20, 96)
}

// NewCacheBounded 建带显式预算的缓存。任一为 0 表示不设该维上限（另一维仍守）。
func NewCacheBounded(loader Loader, maxBytes int64, maxItems int) *Cache {
	return &Cache{
		loader:   loader,
		maxBytes: maxBytes,
		maxItems: maxItems,
		images:   make(map[string]*Image),
		nodes:    make(map[string]*list.Element),
		order:    list.New(),
		misses:   make(map[string]struct{}),
	}
}

// FromArchiveSet 用存档集合作 Loader：无扩展名也能命中（arc 侧已兜底），
// 且 .cpb 后缀会由 arc.Entry 的 find 折叠，这里只做空名拦截。
func FromArchiveSet(set ArchiveSet) *Cache {
	if set == nil {
		return NewCache(nil)
	}
	return NewCache(func(name string) ([]byte, error) {
		return set.Entry(name)
	})
}

func (c *Cache) SetLoader(loader Loader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loader = loader
	c.misses = make(map[string]struct{})
}

// Get 按名取图，命中与未命中都在锁内记账。名字为空或解码失败返回 nil，
// 不报错：合成层宁可跳过一层，也不让一张坏图阻断整帧（原引擎跳层也是此语义）。
func (c *Cache) Get(name string) *Image {
	key := cacheKey(name)
	if key == "" || c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if img, ok := c.images[key]; ok {
		c.hits++
		if el := c.nodes[key]; el != nil {
			c.order.MoveToBack(el)
		}
		return img
	}
	if _, bad := c.misses[key]; bad {
		return nil
	}
	c.missCount++
	img := c.decode(key)
	if img == nil {
		c.misses[key] = struct{}{}
		core.Logf("overdose-gfx: missing/unreadable image %q", key)
		return nil
	}
	c.insertLocked(key, img)
	return img
}

// decode 在已持锁时调用：Loader 内部不得回头调本缓存（会自死锁）。
func (c *Cache) decode(key string) *Image {
	if c.loader == nil {
		return nil
	}
	raw, err := c.loader(key)
	if err != nil || len(raw) == 0 {
		return nil
	}
	w, h, bgra, err := arc.DecodeCPB(raw)
	if err != nil {
		return nil
	}
	if w <= 0 || h <= 0 || len(bgra) != w*h*4 {
		return nil
	}
	return bishopgfx.NewBsgImage(w, h, 0, 0, bgra)
}

// Put 直接塞一张已解码图（测试与合成图注入用）。预算同样适用，
// 避免“Put 绕过预算”把缓存撑爆。
func (c *Cache) Put(name string, img *Image) {
	if img == nil || c == nil {
		return
	}
	key := cacheKey(name)
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.insertLocked(key, img)
}

func (c *Cache) insertLocked(key string, img *Image) {
	if prev, ok := c.images[key]; ok {
		c.bytes -= int64(len(prev.Pixels))
		if el := c.nodes[key]; el != nil {
			c.order.Remove(el)
			delete(c.nodes, key)
		}
	}
	c.images[key] = img
	c.bytes += int64(len(img.Pixels))
	c.nodes[key] = c.order.PushBack(key)
	c.evictLocked(key)
}

func (c *Cache) evictLocked(keep string) {
	for {
		over := false
		if c.maxItems > 0 && len(c.images) > c.maxItems {
			over = true
		}
		if c.maxBytes > 0 && c.bytes > c.maxBytes {
			over = true
		}
		if !over {
			return
		}
		front := c.order.Front()
		if front == nil {
			return
		}
		k, _ := front.Value.(string)
		if k == keep {
			front = front.Next()
			if front == nil {
				return
			}
			k, _ = front.Value.(string)
		}
		if img, ok := c.images[k]; ok {
			c.bytes -= int64(len(img.Pixels))
			delete(c.images, k)
		}
		if el := c.nodes[k]; el != nil {
			c.order.Remove(el)
			delete(c.nodes, k)
		}
	}
}

// Len 返回当前缓存条数（诊断/测试用）。
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.images)
}

// Bytes 返回当前缓存像素字节占用。
func (c *Cache) Bytes() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// Hits/Misses 暴露命中率，供 CI 或诊断日志看缓存是否生效。
func (c *Cache) Hits() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

func (c *Cache) Misses() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.missCount
}

// Clear 清空全部缓存（含 miss 记忆）。换存档或切游戏时必须清：
// 否则旧存档的同名图会冒充新存档的图，错色且极难定位。
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.images = make(map[string]*Image)
	c.nodes = make(map[string]*list.Element)
	c.order.Init()
	c.bytes = 0
	c.misses = make(map[string]struct{})
}

// DecodeCPB 是 arc.DecodeCPB 的本包转出：让调用方不需要同时 import arc。
func DecodeCPB(b []byte) (int, int, []byte, error) {
	return arc.DecodeCPB(b)
}

// NewImage 把已解出的 (w,h,bgra) 包成 *Image。bgra 必须正好 w*h*4，否则 nil。
func NewImage(w, h int, bgra []byte) *Image {
	if w <= 0 || h <= 0 || len(bgra) != w*h*4 {
		return nil
	}
	out := make([]byte, len(bgra))
	copy(out, bgra)
	return bishopgfx.NewBsgImage(w, h, 0, 0, out)
}
