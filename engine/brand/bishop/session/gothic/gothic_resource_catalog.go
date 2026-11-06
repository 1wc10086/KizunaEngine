package gothic

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/gfx/graphicreader"
	"kizuna/engine/brand/bishop/gfx/gsa"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
)

type gothicCatalogEntry struct {
	basis    *resource.ResourceLocation
	overlays []*resource.ResourceLocation
}

type GothicResourceCatalog struct {
	mu        sync.Mutex
	entries   map[string]*gothicCatalogEntry
	images    map[string]*gfx.BsgImage
	resources *resource.GameResourceIndex
	cache     *rendering.ImageCache
}

func NewGothicResourceCatalog(dataDirectory string, resources *resource.GameResourceIndex, cache *rendering.ImageCache) *GothicResourceCatalog {
	catalog := &GothicResourceCatalog{
		entries:   map[string]*gothicCatalogEntry{},
		images:    map[string]*gfx.BsgImage{},
		resources: resources,
		cache:     cache,
	}
	if len(dataDirectory) > 0 {
		catalog.scan(dataDirectory)
	}
	return catalog
}

func (c *GothicResourceCatalog) Resolve(name string) *gfx.BsgImage {
	if len(name) == 0 {
		return nil
	}
	request := strings.Trim(strings.Trim(strings.TrimSpace(name), "\""), "\"")
	request = strings.TrimSpace(request)
	if len(request) == 0 {
		return nil
	}
	file := filepath.Base(request)
	stem := strings.ToLower(strings.TrimSuffix(file, filepath.Ext(file)))
	if len(stem) == 0 {
		return nil
	}
	c.mu.Lock()
	if hit, ok := c.images[stem]; ok {
		c.mu.Unlock()
		return hit
	}
	if hit, ok := c.images[request]; ok {
		c.mu.Unlock()
		return hit
	}
	entry := c.entries[stem]
	if entry == nil {
		entry = c.entries[file]
	}
	var blended *gfx.BsgImage
	if entry != nil && entry.basis != nil {
		blended = c.loadBlended(entry)
		if blended != nil {
			c.images[stem] = blended
			c.images[request] = blended
		}
	}
	c.mu.Unlock()
	if blended != nil {
		return blended
	}
	if c.cache == nil {
		return nil
	}
	image := c.cache.Get(name)
	if image == nil {
		core.Logf("gothic: resolve '%s' failed", request)
	}
	return image
}

// Names 返回目录里所有资源的 stem（小写、无扩展名），排序后交给
// 鉴赏目录兜底扫描使用。只读快照，不暴露内部 map。
func (c *GothicResourceCatalog) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *GothicResourceCatalog) Clear() {
	c.mu.Lock()
	c.images = map[string]*gfx.BsgImage{}
	c.mu.Unlock()
	if c.cache != nil {
		c.cache.Clear()
	}
}

func (c *GothicResourceCatalog) loadBlended(entry *gothicCatalogEntry) *gfx.BsgImage {
	basisLocation := entry.basis
	if basisLocation == nil {
		return nil
	}
	basis, err := graphicreader.Read(basisLocation.Read())
	if err != nil {
		core.Logf("gothic: basis '%s' failed: %v", basisLocation.Name(), err)
		return nil
	}
	if basis == nil {
		return nil
	}
	overlays := []*gfx.BsgImage{}
	for _, location := range entry.overlays {
		overlay, err := graphicreader.Read(location.Read())
		if err != nil {
			core.Logf("gothic: overlay '%s' failed: %v", location.Name(), err)
			continue
		}
		if overlay != nil {
			overlays = append(overlays, overlay)
		}
	}
	if len(overlays) == 0 {
		return basis
	}
	blended, err := gsa.Blend(basis, overlays)
	if err != nil {
		core.Logf("gothic: blend failed: %v", err)
		return basis
	}
	return blended
}

func (c *GothicResourceCatalog) scan(dataDirectory string) {
	candidates := []string{}
	if graphics := findGothicDirectory(dataDirectory, "GRAPHICS"); graphics != "" {
		candidates = append(candidates, graphics)
	}
	if len(candidates) == 0 && isGothicDirectory(dataDirectory) {
		candidates = append(candidates, dataDirectory)
	}
	for _, directory := range candidates {
		files, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			c.addFile(filepath.Join(directory, file.Name()))
		}
	}
	c.scanArchive()
	for _, entry := range c.entries {
		overlays := entry.overlays
		sort.Slice(overlays, func(i, j int) bool {
			return gothicOverlayOrder(overlays[i].Name()) < gothicOverlayOrder(overlays[j].Name())
		})
	}
}

func (c *GothicResourceCatalog) scanArchive() {
	resources := c.resources
	if resources == nil {
		return
	}
	for _, archive := range resources.ArchivesInCategory(resource.GraphicsCategory) {
		for _, archived := range archive.Entries() {
			file := filepath.Base(strings.ReplaceAll(archived.Path, "\\", "/"))
			if len(file) == 0 {
				continue
			}
			if !isGothicOverlayFile(file) && !isGothicBasisExtension(file) {
				continue
			}
			stem := strings.ToLower(strings.TrimSuffix(file, filepath.Ext(file)))
			if len(stem) == 0 {
				continue
			}
			c.addEntry(stem, file, resource.ArchivedResourceLocation(file, resource.GraphicsCategory, archive, archived))
		}
	}
}

func (c *GothicResourceCatalog) addFile(path string) {
	file := filepath.Base(path)
	if len(file) == 0 {
		return
	}
	stem := strings.ToLower(strings.TrimSuffix(file, filepath.Ext(file)))
	if len(stem) == 0 {
		return
	}
	c.addEntry(stem, file, resource.LooseResourceLocation(file, resource.GraphicsCategory, path))
}

func (c *GothicResourceCatalog) addEntry(stem, file string, location *resource.ResourceLocation) {
	entry, ok := c.entries[stem]
	if !ok {
		entry = &gothicCatalogEntry{}
		c.entries[stem] = entry
	}
	if isGothicOverlayFile(file) {
		for _, existing := range entry.overlays {
			if existing.Key() == location.Key() {
				return
			}
		}
		entry.overlays = append(entry.overlays, location)
		return
	}
	if isGothicBasisExtension(file) {
		if entry.basis == nil {
			entry.basis = location
		} else if entry.basis.Key() != location.Key() {
			core.Logf("gothic: duplicate basis '%s' ignored, keeping '%s'", file, entry.basis.Name())
		}
	}
}

func isGothicBasisExtension(fileName string) bool {
	extension := filepath.Ext(fileName)
	return strings.EqualFold(extension, ".gsa") || strings.EqualFold(extension, ".bmp")
}

func isGothicOverlayFile(fileName string) bool {
	if len(fileName) == 0 || len(fileName) < 4 {
		return false
	}
	end := len(fileName)
	if fileName[end-4] != '.' {
		return false
	}
	kind := fileName[end-3]
	if kind != 'G' && kind != 'g' {
		return false
	}
	part := fileName[end-2]
	if part != '0' && part != '1' && part != 'S' && part != 's' {
		return false
	}
	digit := fileName[end-1]
	return digit >= '0' && digit <= '9'
}

func gothicOverlayOrder(fileName string) int {
	if len(fileName) == 0 {
		return int(^uint(0) >> 1)
	}
	extension := filepath.Ext(fileName)
	if len(extension) == 0 || len(extension) < 3 || (extension[1] != 'G' && extension[1] != 'g') {
		return int(^uint(0) >> 1)
	}
	number := extension[2:]
	if len(number) > 0 && (number[0] == 'S' || number[0] == 's') {
		number = number[1:]
	}
	if len(number) == 0 {
		return int(^uint(0) >> 1)
	}
	order := 0
	for i := 0; i < len(number); i++ {
		if number[i] < '0' || number[i] > '9' {
			return int(^uint(0) >> 1)
		}
		order = order*10 + int(number[i]-'0')
	}
	return order
}

func isGothicDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func containsGothicString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
