package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const GraphicsCategory = "graphics"

const gameScanDepth = 6

const (
	gameLinkDepthLimit = 6
	gameLooseCategory  = "loose"
	gameMovieCategory  = "movie"
	gameImageRank      = 0
	gameIventRank      = 1
	gameDefaultRank    = 2
	gameThumbRank      = 3
)

var gameCategoryPrefixes = []string{
	"GraphicsUA", "GraphicsBU", "GraphicsEV", "Graphics", "graphics",
	"Voice", "voice", "Bgm", "BGM", "bgm", "Se", "SE", "sesys",
	"define", "embed", "movie", "Movie", "anm", "bgs",
	"GRPFILE", "SCNFILE", "PCMFILE", "BGMFILE", "SOUND",
}

var gameImageExtensions = []string{".bmp", ".png", ".jpg", ".jpeg", ".gif", ".tga"}

var gameIgnoredSuffixes = []string{".bsa", ".snn", ".pk", ".dsk", ".pft", ".inx", ".exe", ".dll"}

type GameResourceIndex struct {
	root              string
	byName            map[string]*ResourceLocation
	priorityOfName    map[string]int
	folded            map[string]*ResourceLocation
	foldedPriority    map[string]int
	foldedLinks       map[string]*ResourceLocation
	categoryOfArchive map[string]string
	snnIndexes        map[string]string
	extraPass         bool
	mainArchives      []Archive
	links             map[string]*ResourceLocation
	nameOrder         []string
	archives          []Archive
	warnings          []string
}

// newGameResourceIndex 创建空索引。大小写折叠表与精确表同时初始化，避免遗漏。
func newGameResourceIndex(root string) *GameResourceIndex {
	return &GameResourceIndex{
		root:              root,
		byName:            make(map[string]*ResourceLocation),
		priorityOfName:    make(map[string]int),
		categoryOfArchive: make(map[string]string),
		links:             make(map[string]*ResourceLocation),
		folded:            make(map[string]*ResourceLocation),
		foldedPriority:    make(map[string]int),
		foldedLinks:       make(map[string]*ResourceLocation),
	}
}

func Build(root string, extraRoots ...string) *GameResourceIndex {
	index := newGameResourceIndex(root)
	if !core.DirExists(root) {
		index.warnings = append(index.warnings, "game directory not found: "+root)
		core.Logf("index: root='%s' MISSING", root)
		return index
	}

	core.Logf("index: root='%s' scanning", root)
	// 同一目录可能经由不同路径（尾部斜杠、大小写）被传进来，按归一化键去重，
	// 否则同一个 Disk 会被扫两遍，归档被重复登记。
	roots := []string{root}
	seenRoots := map[string]struct{}{gameRootKey(root): {}}
	var extras []string
	for _, extra := range extraRoots {
		key := gameRootKey(extra)
		if _, dup := seenRoots[key]; dup || !core.DirExists(extra) {
			continue
		}
		seenRoots[key] = struct{}{}
		extras = append(extras, extra)
		roots = append(roots, extra)
	}
	// 先收集所有根的 .Inx：旧作把 Graphics2.Inx 放在 Disc1、Graphics2.Snn 放在
	// Disc2，只按同目录找索引会把整个图形包判成“不是归档”，标题/立绘全丢。
	index.snnIndexes = collectSnnIndexes(roots)
	index.extraPass = false
	index.scanArchives(root)
	index.addLooseFiles(root)
	index.mainArchives = append([]Archive(nil), index.archives...)

	for _, extra := range extras {
		core.Logf("index: extra='%s' scanning", extra)
		// 额外根只补主根缺失的资源，不能反向覆盖主根同名图。addNamed 在
		// extraPass 下遇到已存在的名字直接跳过，避免 VISUAL.bsa 之类的
		// 文件名 rank 把主目录的标题/对话框素材顶掉。
		index.extraPass = true
		index.scanArchives(extra)
		index.addLooseFiles(extra)
	}
	index.extraPass = false

	core.Logf("index: archives=%d names=%d links=%d warnings=%d", len(index.archives), len(index.byName), len(index.links), len(index.warnings))
	for _, warning := range index.warnings {
		core.Logf("index: warn: %s", warning)
	}

	return index
}

// gameRootKey 把根目录归一化成去重用的键。
//
// 旧版作品的 Disk 目录可能以不同写法（尾部斜杠、相对/绝对路径）重复传入，
// 不归一化会导致同一个 Disk 被扫两遍，归档被重复登记。
func gameRootKey(directory string) string {
	if directory == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(directory))
}

func (g *GameResourceIndex) Root() string {
	return g.root
}

func (g *GameResourceIndex) Warnings() []string {
	return g.warnings
}

func (g *GameResourceIndex) Archives() []Archive {
	return g.archives
}

func (g *GameResourceIndex) Count() int {
	return len(g.byName)
}

func (g *GameResourceIndex) ContainsExact(name string) bool {
	if _, ok := g.byName[name]; ok {
		return true
	}

	_, ok := g.links[name]
	return ok
}

func (g *GameResourceIndex) TryGet(name string) (*ResourceLocation, bool) {
	if suffix := strings.IndexByte(name, ':'); suffix > 0 {
		name = name[:suffix]
	}

	if location, ok := g.resolve(name, 0); ok {
		return location, true
	}

	stem := pathFileNameWithoutExtension(name)
	if stem != "" && stem != name {
		return g.resolve(stem, 0)
	}

	return nil, false
}

func (g *GameResourceIndex) Require(name string) (*ResourceLocation, error) {
	if location, ok := g.TryGet(name); ok {
		return location, nil
	}

	return nil, fmt.Errorf("resource not found: %s", name)
}

func (g *GameResourceIndex) Exists(name string) bool {
	_, ok := g.TryGet(name)
	return ok
}

func (g *GameResourceIndex) ArchivesInCategory(category string) []Archive {
	var result []Archive
	for _, archive := range g.archives {
		if strings.EqualFold(g.categoryOfArchive[archive.Name()], category) {
			result = append(result, archive)
		}
	}

	return result
}

// MainArchivesInCategory 只返回主数据目录里的归档。
//
// 屏幕尺寸探测用于“没有 [main window] 配置”的作品；额外根是给多盘安装补
// 缺失资源的，里面可能混着别的分辨率素材。用主根结果才能保持内置布局的
// 800x600 坐标不被别的盘带偏。
func (g *GameResourceIndex) MainArchivesInCategory(category string) []Archive {
	var result []Archive
	for _, archive := range g.mainArchives {
		if strings.EqualFold(g.categoryOfArchive[archive.Name()], category) {
			result = append(result, archive)
		}
	}
	return result
}

func (g *GameResourceIndex) NamesInCategory(category string) []string {
	var result []string
	for _, key := range g.nameOrder {
		if item, ok := g.byName[key]; ok && strings.EqualFold(item.Category(), category) {
			result = append(result, item.Name())
		}
	}

	return result
}

func (g *GameResourceIndex) RegisterArchive(archive Archive, category ...string) {
	if archive == nil {
		return
	}

	bucket := GraphicsCategory
	if len(category) > 0 && category[0] != "" {
		bucket = category[0]
	}

	g.archives = append(g.archives, archive)
	g.categoryOfArchive[archive.Name()] = bucket

	for _, entry := range archive.Entries() {
		name := pathFileNameWithoutExtension(entry.Path)
		if name == "" {
			continue
		}

		location := ArchivedResourceLocation(name, bucket, archive, entry)
		rank := gameDefaultRank
		if gameIsImagePath(entry.Path) {
			rank = gameImageRank
		}

		g.addNamed(name, location, rank)
		full := strings.ReplaceAll(entry.Path, "\\", "/")
		g.addNamed(full, location, rank)
		if strings.Contains(full, "/") {
			g.addNamed(strings.ReplaceAll(full, "/", "."), location, rank)
		}
	}
}

func (g *GameResourceIndex) Dispose() {
	for _, archive := range g.archives {
		archive.Dispose()
	}

	g.archives = nil
}

// resolve 先精确查找，再查大小写折叠表。链接目标递归调用 resolve，所以链接解析出的
// 名字同样享受大小写回退；精确命中始终优先，回退只影响查不到的大小写。
func (g *GameResourceIndex) resolve(name string, depth int) (*ResourceLocation, bool) {
	if found, ok := g.byName[name]; ok {
		return found, true
	}
	if found, ok := g.resolveLink(g.links, name, depth); ok {
		return found, true
	}

	folded := strings.ToLower(name)
	if found, ok := g.folded[folded]; ok {
		return found, true
	}

	return g.resolveLink(g.foldedLinks, folded, depth)
}

func (g *GameResourceIndex) resolveLink(table map[string]*ResourceLocation, name string, depth int) (*ResourceLocation, bool) {
	if depth >= gameLinkDepthLimit {
		return nil, false
	}

	link, ok := table[name]
	if !ok {
		return nil, false
	}

	target := readLinkTarget(link)
	if target == "" {
		return nil, false
	}

	return g.resolve(target, depth+1)
}

func readLinkTarget(link *ResourceLocation) string {
	text := strings.TrimSpace(decodeUTF8(link.Read()))
	line, ok := firstLinkLine(text)
	if !ok {
		return ""
	}

	return pathFileNameWithoutExtension(line)
}

func firstLinkLine(text string) (string, bool) {
	begin := 0
	for i := 0; i < len(text); i++ {
		if text[i] != '\r' && text[i] != '\n' {
			continue
		}

		if segment := text[begin:i]; len(segment) > 0 {
			return strings.TrimSpace(segment), true
		}
		begin = i + 1
	}

	if tail := text[begin:]; len(tail) > 0 {
		return strings.TrimSpace(tail), true
	}

	return "", false
}

func (g *GameResourceIndex) scanArchives(root string) {
	seen := map[string]struct{}{}
	for _, pattern := range []string{"*.bsa", "*.BSA"} {
		for _, path := range enumerateFiles(root, pattern) {
			key := strings.ToLower(path)
			if _, taken := seen[key]; taken {
				continue
			}
			seen[key] = struct{}{}
			g.addArchive(path)
		}
	}

	g.scanSnnArchives(root)
	g.scanPackArchives(root)
	g.scanDskArchives(root)
	g.scanZbmArchives(root)
}

func (g *GameResourceIndex) scanSnnArchives(root string) {
	seen := make(map[string]struct{})
	var paths []string
	for _, path := range enumerateFiles(root, "*.Snn") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range enumerateFiles(root, "*"+SnnExtension) {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range enumerateFiles(root, "*.SNN") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range dedupeSnnDuplicates(paths) {
		g.addArchive(path)
	}
}

// dedupeSnnDuplicates 去掉解包工具留下的 "_1" 副本。
//
// 同一个 ISO 目录被重复解包时会出现 Graphics2.Snn 与 Graphics2_1.Snn（或
// GRAPHI~1.SNN / GRAPHI~1_1.SNN），两者大小相同。它们会配对到同一个 .Inx，
// 多出来的那份不仅浪费一次索引，还可能被短名回退误当成另一个归档。
func dedupeSnnDuplicates(paths []string) []string {
	seen := make(map[string]string, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		normalized := strings.ToLower(pathFileNameWithoutExtension(path))
		normalized = strings.TrimSuffix(normalized, "_1")
		key := strings.ToLower(pathDirectoryName(path)) + "|" + normalized + "|" + strconv.FormatInt(info.Size(), 10)
		if existing, ok := seen[key]; ok {
			// 保留名字更短的那份（通常是原始名，不带 _1）。
			if len(pathFileName(path)) < len(pathFileName(existing)) {
				seen[key] = path
			}
			continue
		}
		seen[key] = path
	}
	result := make([]string, 0, len(seen))
	for _, path := range seen {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func (g *GameResourceIndex) scanPackArchives(root string) {
	seen := make(map[string]struct{})
	var paths []string
	for _, path := range enumerateFiles(root, "*.PK") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range enumerateFiles(root, "*"+PackPkExtension) {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		g.addArchive(path)
	}
}

func (g *GameResourceIndex) scanDskArchives(root string) {
	seen := make(map[string]struct{})
	var paths []string
	for _, path := range enumerateFiles(root, "*.DSK") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range enumerateFiles(root, "*.dsk") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		g.addArchive(path)
	}
}

func (g *GameResourceIndex) scanZbmArchives(root string) {
	seen := make(map[string]struct{})
	var paths []string
	for _, path := range enumerateFiles(root, "*.Zbm") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range enumerateFiles(root, "*.zbm") {
		key := strings.ToLower(path)
		if _, taken := seen[key]; !taken {
			seen[key] = struct{}{}
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		if ZbmIsArchive(path) {
			g.addArchive(path)
		}
	}
}

func (g *GameResourceIndex) addArchive(path string) {
	archive, err := ArchiveOpenWithIndex(path, g.snnIndexes)
	if err != nil {
		g.warnings = append(g.warnings, pathFileName(path)+": "+err.Error())
		core.Logf("index: OPEN FAILED %s: %s", pathFileName(path), err.Error())
		return
	}

	g.archives = append(g.archives, archive)
	core.Logf("index: %s v%d entries=%d size=%d", archive.Name(), archive.Version(), len(archive.Entries()), archive.Length())

	stem := pathFileNameWithoutExtension(path)
	category := gameCategoryFor(stem)
	g.categoryOfArchive[archive.Name()] = category
	rank := gameArchiveRank(stem)

	for _, entry := range archive.Entries() {
		name := pathFileNameWithoutExtension(entry.Path)
		if name == "" {
			continue
		}

		location := ArchivedResourceLocation(name, category, archive, entry)
		full := strings.ReplaceAll(entry.Path, "\\", "/")
		if strutil.HasSuffixFold(full, ".link") {
			g.addLink(name, location)
			if alias := full[:len(full)-len(".link")]; alias != "" {
				g.addLink(alias, location)
			}
			continue
		}

		g.addNamed(name, location, rank)
		g.addNamed(full, location, rank)
		if strings.Contains(full, "/") {
			g.addNamed(strings.ReplaceAll(full, "/", "."), location, rank)
		}
	}
}

func (g *GameResourceIndex) addNamed(key string, location *ResourceLocation, rank int) {
	if _, taken := g.byName[key]; taken {
		// 额外根永不覆盖主根；主根内部仍按 rank 选择更合适的归档。
		if g.extraPass {
			return
		}
	}
	if _, taken := g.byName[key]; !taken {
		g.byName[key] = location
		g.priorityOfName[key] = rank
		g.nameOrder = append(g.nameOrder, key)
		g.addFolded(key, location, rank)
		return
	}

	if current, ok := g.priorityOfName[key]; ok && rank < current {
		g.byName[key] = location
		g.priorityOfName[key] = rank
		g.addFolded(key, location, rank)
	}
}

func (g *GameResourceIndex) addLink(key string, location *ResourceLocation) {
	if _, taken := g.links[key]; !taken {
		g.links[key] = location
	}
	folded := strings.ToLower(key)
	if _, taken := g.foldedLinks[folded]; !taken {
		g.foldedLinks[folded] = location
	}
}

// addFolded 登记大小写折叠表，规则与 addNamed 对齐。
//
// BDT 的 grp.zbm（LOGO1）和 voice.Snn（V0521001）条目名全大写，而脚本文本是小写
// （CFlogo1、v0521001）。精确表查不到时才回退到这里，所以它只影响原本会 not found
// 的大小写，不改变精确命中。额外根不覆盖主根已有的折叠名，主根内部仍按 rank 选择。
func (g *GameResourceIndex) addFolded(key string, location *ResourceLocation, rank int) {
	folded := strings.ToLower(key)
	if _, taken := g.folded[folded]; taken {
		if g.extraPass {
			return
		}
		if current, ok := g.foldedPriority[folded]; !ok || rank >= current {
			return
		}
	}
	g.folded[folded] = location
	g.foldedPriority[folded] = rank
}

// addFoldedIfAbsent 给松散文件登记折叠名。松散文件没有 rank，只在尚未登记时写入，
// 与精确表的 first-wins 行为保持一致。
func (g *GameResourceIndex) addFoldedIfAbsent(key string, location *ResourceLocation) {
	folded := strings.ToLower(key)
	if _, taken := g.folded[folded]; !taken {
		g.folded[folded] = location
	}
}

func (g *GameResourceIndex) addLooseFiles(root string) {
	for _, path := range enumerateFiles(root, "*") {
		name := pathFileName(path)
		if gameIsIgnoredFile(name) {
			continue
		}

		key := pathFileNameWithoutExtension(name)
		category := gameLooseCategory
		if gameIsMovieFile(path) {
			category = gameMovieCategory
		}
		if _, taken := g.byName[key]; !taken {
			location := LooseResourceLocation(key, category, path)
			g.byName[key] = location
			g.addFoldedIfAbsent(key, location)
			g.nameOrder = append(g.nameOrder, key)
		}
		if _, taken := g.byName[name]; !taken {
			location := LooseResourceLocation(name, category, path)
			g.byName[name] = location
			g.addFoldedIfAbsent(name, location)
			g.nameOrder = append(g.nameOrder, name)
		}
	}
}

func gameArchiveRank(archiveStem string) int {
	if strutil.HasPrefixFold(archiveStem, "VISUAL") {
		return gameImageRank
	}
	if strings.EqualFold(archiveStem, "IVENT") {
		return gameIventRank
	}
	if strings.EqualFold(archiveStem, "THUMB") {
		return gameThumbRank
	}

	return gameDefaultRank
}

func gameIsImagePath(path string) bool {
	extension := pathExtension(path)
	for _, candidate := range gameImageExtensions {
		if strings.EqualFold(extension, candidate) {
			return true
		}
	}

	return false
}

func gameIsIgnoredFile(name string) bool {
	for _, suffix := range gameIgnoredSuffixes {
		if strutil.HasSuffixFold(name, suffix) {
			return true
		}
	}

	return false
}

func gameIsMovieFile(path string) bool {
	directory := pathDirectoryName(path)
	return directory != "" && strings.EqualFold(pathFileName(directory), gameMovieCategory)
}

func gameCategoryFor(archiveName string) string {
	for _, prefix := range gameCategoryPrefixes {
		if !strutil.HasPrefixFold(archiveName, prefix) {
			continue
		}

		switch strings.ToLower(prefix) {
		case "voice":
			return "voice"
		case "bgm":
			return "bgm"
		case "se", "sesys":
			return "se"
		case "scnfile":
			return "define"
		case "pcmfile", "bgmfile", "sound":
			return "se"
		case "define":
			return "define"
		case "embed":
			return "embed"
		case "movie":
			return "movie"
		case "bgs":
			return "bgs"
		default:
			return GraphicsCategory
		}
	}

	return GraphicsCategory
}

func enumerateFiles(root, pattern string) []string {
	type visit struct {
		directory string
		depth     int
	}

	var result []string
	queue := []visit{{directory: root, depth: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, path := range listFilesTop(current.directory) {
			if matchFilePattern(pathFileName(path), pattern) {
				result = append(result, path)
			}
		}

		if current.depth >= gameScanDepth {
			continue
		}

		for _, child := range listDirsTop(current.directory) {
			queue = append(queue, visit{directory: child, depth: current.depth + 1})
		}
	}

	return result
}

func matchFilePattern(name, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(name, pattern[1:])
	}

	return name == pattern
}

func listFilesTop(directory string) []string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}

	var result []string
	for _, item := range entries {
		if item.IsDir() {
			continue
		}

		result = append(result, core.Join(directory, item.Name()))
	}

	return result
}

func listDirsTop(directory string) []string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}

	var result []string
	for _, item := range entries {
		if !item.IsDir() {
			continue
		}

		result = append(result, core.Join(directory, item.Name()))
	}

	return result
}
