package rendering

import (
	"container/list"
	"strings"
	"sync"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/gfx/graphicreader"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
)

const (
	DefaultImageBudgetBytes = int64(128 * 1024 * 1024)
	DefaultImageCapacity    = 96
	imageUnavailableLimit   = 1024
)

type ImageCache struct {
	resources   *resource.GameResourceIndex
	budgetBytes int64
	capacity    int
	images      map[string]*gfx.BsgImage
	aliases     map[string]string
	unavailable map[string]struct{}
	recency     *list.List
	nodes       map[string]*list.Element
	bytes       int64
	mu          sync.Mutex
	// generation 在清空缓存时递增。图层 surface 缓存按 layer.Revision 判断
	// 是否重画，而「アヘ顔OFF」/DRESS 切换只换素材名、不改 layer revision；
	// 不把这个版本算进去，已经画好的立绘要等下一次换层才更新。
	generation uint64
	// dressSuffix 是学園2 老版着衣/全裸切换的图层后缀（"0" 着衣 / "1" 全裸）。
	// 剧本里的 IMAGE 名是 7 位的 cl05001，实际素材是 cl050010 / cl050011；
	// 原引擎在加载时把当前状态拼到名字后面，这里用同一规则解析。
	dressSuffix string
	// aheSuffix/aheOff 是「アヘ顔OFF」的素材替换规则。
	//
	// 牝教師4 的强烈アヘ顔不是独立 CG，而是叠在同一张图上的 `<name>_noahe`
	// 变体；开关打开时原引擎在加载阶段换名，剧情、CG 鉴赏、立ち絵鉴赏共用。
	// 只改缓存键不换图会让三个界面各自看到不同结果，所以放在最底层统一处理。
	aheSuffix string
	aheOff    bool
}

func NewImageCache(resources *resource.GameResourceIndex, budgetBytes int64, capacity int) *ImageCache {
	return &ImageCache{
		resources:   resources,
		budgetBytes: budgetBytes,
		capacity:    capacity,
		images:      map[string]*gfx.BsgImage{},
		aliases:     map[string]string{},
		unavailable: map[string]struct{}{},
		recency:     list.New(),
		nodes:       map[string]*list.Element{},
	}
}

func (c *ImageCache) BudgetBytes() int64 {
	return c.budgetBytes
}

func (c *ImageCache) Capacity() int {
	return c.capacity
}

func (c *ImageCache) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.images)
}

func (c *ImageCache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// SetDressSuffix 切换学園2 的着衣状态并清空图片缓存。
//
// 缓存是按基础名（cl05001）索引的；如果只改后缀不清缓存，下一次 Get
// 会直接命中旧状态的图。清空后重画的成本只在玩家按 DRESS 时发生一次。
func (c *ImageCache) SetDressSuffix(suffix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dressSuffix == suffix {
		return
	}
	c.dressSuffix = suffix
	c.clearLocked()
}

// SetAheSuffix 设置「アヘ顔OFF」的替换后缀（通常是 `_noahe`）。
//
// 后缀变化只影响之后加载的图；清空缓存保证已经画过的アヘ顔不会留在
// 界面上，和 SetDressSuffix 的处理保持一致。
func (c *ImageCache) SetAheSuffix(suffix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.aheSuffix == suffix {
		return
	}
	c.aheSuffix = suffix
	c.clearLocked()
}

// SetAheOff 切换アヘ顔显示。开启时优先加载 `<name><suffix>`。
func (c *ImageCache) SetAheOff(off bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.aheOff == off {
		return
	}
	c.aheOff = off
	c.clearLocked()
}

// aheName 返回当前设置下应该加载的实际素材名。
//
// 只在"开关打开 + 后缀非空 + 请求名没有后缀 + 变体确实存在"时替换；
// 其它作品没有 ahe suffix，aheOff 也不会凭空改变素材名。
func (c *ImageCache) aheName(name string) string {
	if !c.aheOff || c.aheSuffix == "" || c.resources == nil {
		return name
	}
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(c.aheSuffix)) {
		return name
	}
	candidate := name + c.aheSuffix
	if c.resources.ContainsExact(candidate) {
		return candidate
	}
	return name
}

// clearLocked 清空缓存；调用方必须已持锁。
func (c *ImageCache) clearLocked() {
	c.images = map[string]*gfx.BsgImage{}
	c.aliases = map[string]string{}
	c.unavailable = map[string]struct{}{}
	c.recency = list.New()
	c.nodes = map[string]*list.Element{}
	c.bytes = 0
	c.generation++
}

// Generation 返回缓存清空次数；图层 surface 用它判断素材替换是否需要重画。
func (c *ImageCache) Generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

// isGakuen2CharacterLayer 判断是不是剧本里的 7 位角色图层名（cl05001）。
func isGakuen2CharacterLayer(name string) bool {
	if len(name) != 7 {
		return false
	}
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "cl") && !strings.HasPrefix(lower, "cu") {
		return false
	}
	for i := 2; i < len(lower); i++ {
		if lower[i] < '0' || lower[i] > '9' {
			return false
		}
	}
	return true
}

// dressName 把基础图层名解析成当前着衣状态的素材名。
func (c *ImageCache) dressName(name string) string {
	if c.dressSuffix == "" || !isGakuen2CharacterLayer(name) {
		return name
	}
	// 先试当前状态；全裸素材缺失时退回着衣，避免角色整块消失。
	for _, candidate := range []string{name + c.dressSuffix, name + "0", name + "1"} {
		if c.resources != nil && c.resources.ContainsExact(candidate) {
			return candidate
		}
	}
	return name
}

func (c *ImageCache) Get(name string) *gfx.BsgImage {
	request := strings.TrimSpace(name)
	if len(request) == 0 {
		return nil
	}
	// モーションプラス 的动态 CG 是"多帧容器"，由 BsgAnimationPlayer 按帧读取。
	// 这里整份解码会把上百帧展平进一张 1024x768 画布（既慢又失去动画），
	// 所以图像缓存直接拒绝这类名字。
	if strings.HasSuffix(strings.ToLower(request), "_anim") {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := strings.ToLower(request)
	if alias, ok := c.aliases[key]; ok {
		if cached, ok := c.images[alias]; ok {
			c.touch(alias)
			return cached
		}
	}
	if _, ok := c.unavailable[key]; ok {
		c.noteUnavailable(key)
		return nil
	}
	resolved := c.aheName(request)
	resolved = c.dressName(resolved)
	location, ok := c.resources.TryGet(resolved)
	if !ok {
		c.noteUnavailable(key)
		return nil
	}
	canonical := strings.ToLower(location.Key())
	if existing, ok := c.images[canonical]; ok {
		c.aliases[key] = canonical
		c.touch(canonical)
		return existing
	}
	image := decodeImage(location, request)
	if image == nil {
		c.noteUnavailable(key)
		return nil
	}
	c.insert(canonical, image)
	c.aliases[key] = canonical
	return image
}

func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearLocked()
}

func (c *ImageCache) noteUnavailable(request string) {
	if len(c.unavailable) >= imageUnavailableLimit {
		c.unavailable = map[string]struct{}{}
	}
	c.unavailable[request] = struct{}{}
}

func decodeImage(location *resource.ResourceLocation, request string) *gfx.BsgImage {
	image, err := graphicreader.Read(location.Read())
	if err != nil {
		core.Logf("image: '%s' failed: %v", request, err)
		return nil
	}
	if image == nil {
		core.Logf("image: '%s' failed: unsupported graphic", request)
		return nil
	}
	core.Logf("image: '%s' %dx%d @(%d,%d) %dKB", request, image.Width, image.Height, image.OffsetX, image.OffsetY, image.ByteCount()/1024)
	return image
}

func (c *ImageCache) insert(key string, image *gfx.BsgImage) {
	if previous, ok := c.images[key]; ok {
		c.bytes -= int64(previous.ByteCount())
	}
	c.unlink(key)
	c.images[key] = image
	c.bytes += int64(image.ByteCount())
	c.nodes[key] = c.recency.PushBack(key)
	for len(c.images) > c.capacity || c.bytes > c.budgetBytes {
		oldest := c.recency.Front()
		if oldest == nil {
			break
		}
		if oldest.Value.(string) == key {
			oldest = oldest.Next()
			if oldest == nil {
				break
			}
		}
		c.evict(oldest.Value.(string))
	}
}

func (c *ImageCache) touch(key string) {
	if node, ok := c.nodes[key]; ok {
		c.recency.MoveToBack(node)
	}
}

func (c *ImageCache) unlink(key string) {
	if node, ok := c.nodes[key]; ok {
		delete(c.nodes, key)
		c.recency.Remove(node)
	}
}

func (c *ImageCache) evict(key string) {
	if image, ok := c.images[key]; ok {
		delete(c.images, key)
		c.bytes -= int64(image.ByteCount())
	}
	c.unlink(key)
	c.dropAliases(key)
}

func (c *ImageCache) dropAliases(key string) {
	if len(c.aliases) == 0 {
		return
	}
	var stale []string
	for name, target := range c.aliases {
		if strings.EqualFold(target, key) {
			stale = append(stale, name)
		}
	}
	for _, name := range stale {
		delete(c.aliases, name)
	}
}
