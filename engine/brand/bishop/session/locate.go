package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

var scriptCandidates = []string{"bsxx.dat", "bsx.dat", "bss.dat"}

const maxLocateDepth = 6

type probeIndex struct {
	res *resource.GameResourceIndex
}

func (p probeIndex) ContainsExact(name string) bool {
	if p.res == nil {
		return false
	}
	return p.res.ContainsExact(name)
}

func (p probeIndex) TryGet(name string) (state.ResourceLocation, bool) {
	if p.res == nil {
		return nil, false
	}
	location, ok := p.res.TryGet(name)
	if !ok || location == nil {
		return nil, false
	}
	return location, true
}

func (p probeIndex) ArchivesInCategory(category string) []resource.Archive {
	if p.res == nil {
		return nil
	}
	return p.res.MainArchivesInCategory(category)
}

func resolveDataDir(root string) (string, error) {
	for _, directory := range descend(root) {
		if hasNamedFile(directory, scriptCandidates) {
			return directory, nil
		}
	}
	return "", errorString("bishop session: no bs script under " + root)
}

func findScript(root string) (string, error) {
	for _, directory := range descend(root) {
		for _, candidate := range scriptCandidates {
			path := filepath.Join(directory, candidate)
			if core.PathExists(path) {
				return path, nil
			}
		}
	}
	for _, directory := range descend(root) {
		if found, ok := matchFileFold(directory, scriptCandidates); ok {
			return found, nil
		}
	}
	return filepath.Join(root, scriptCandidates[0]), errorString("bishop session: no bs script under " + root)
}

func findConfiguration(root string) string {
	for _, directory := range descend(root) {
		if found, ok := matchGlobFold(directory, "*.cfg"); ok {
			return found
		}
		if found, ok := matchGlobFold(directory, "*.cfg.txt"); ok {
			return found
		}
	}
	return filepath.Join(root, "bsxx.cfg")
}

// findConfigurationInRoots 按 roots 顺序找 bsxx.cfg，dataDir 排第一位。
//
// 旧版的配置可能落在 Disk 目录里而非脚本目录；找不到任何真实文件时返回 roots[0]
// 下的默认路径，交给 config.Load 决定是否使用内置布局。
func findConfigurationInRoots(roots []string) string {
	for _, root := range roots {
		if found := findConfiguration(root); core.PathExists(found) {
			return found
		}
	}
	return findConfiguration(roots[0])
}

func findFile(root string, name string) string {
	for _, directory := range descend(root) {
		path := filepath.Join(directory, name)
		if core.PathExists(path) {
			return path
		}
	}
	for _, directory := range descend(root) {
		if found, ok := matchFileFold(directory, []string{name}); ok {
			return found
		}
	}
	return ""
}

// maxResourceAncestorLevels 限制向上回溯查找兄弟 Disk 目录的层数。
//
// 旧版 dataDir 位于 特別病棟/Disk1.Alpha_extracted/iso/program files/BISHOP/TB，
// 兄弟 Disk2 在特別病棟下，需要向上 5 层；再往上会扫到无关的游戏库目录。
const maxResourceAncestorLevels = 5

// resourceRoots 返回查找资源时使用的目录列表，dataDir 永远排第一位。
//
// 旧版把脚本、图形、语音拆在同级的 Disk1/Disk2/Disk3 里：脚本在 Disk1，
// Graphics2 与 system.prm 在 Disk2。只扫 dataDir 会丢掉这些文件。这里从用户
// 选的 root 向下、从 dataDir 向上收集形如 Disk*/*_extracted 的兄弟目录。
// 祖先目录不能加入：祖先会被递归扫描，dataDir 会被重复登记一遍。
func resourceRoots(root, dataDir string) []string {
	roots := []string{dataDir}
	dataKey := filepath.Clean(dataDir)
	seen := map[string]struct{}{dataKey: {}}
	add := func(directory string) {
		key := filepath.Clean(directory)
		if _, taken := seen[key]; taken {
			return
		}
		if strings.HasPrefix(dataKey, key+string(filepath.Separator)) {
			return
		}
		seen[key] = struct{}{}
		roots = append(roots, directory)
	}
	if root != "" {
		for _, directory := range descend(root) {
			if isDiskDirName(filepath.Base(directory)) {
				add(directory)
			}
		}
	}
	ancestor := dataDir
	for level := 0; level < maxResourceAncestorLevels; level++ {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		if children, err := os.ReadDir(parent); err == nil {
			for _, child := range children {
				if child.IsDir() && isDiskDirName(child.Name()) {
					add(filepath.Join(parent, child.Name()))
				}
			}
		}
		ancestor = parent
	}
	return roots
}

// isDiskDirName 识别旧版安装包的光盘目录名（Disk1、Disk2.Alpha_extracted 等）。
func isDiskDirName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "disk") || strings.HasSuffix(lower, "_extracted")
}

// findFileInRoots 按 roots 的顺序查找文件，前面的根优先（dataDir 永远第一）。
func findFileInRoots(roots []string, name string) string {
	for _, root := range roots {
		if root == "" {
			continue
		}
		if found := findFile(root, name); found != "" {
			return found
		}
	}
	return ""
}

// collectMoviesInRoots 合并所有根下的影片，去重后排序。
//
// 旧版影片可能放在 Disk2 的 movie 目录，只扫 dataDir 会漏掉。
func collectMoviesInRoots(roots []string) []string {
	var files []string
	seen := make(map[string]struct{})
	for _, root := range roots {
		for _, path := range collectMovies(root) {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

// resourceLookupRoots 返回 Session 的资源查找根；旧会话未设置时退回 dataDir。
func (s *Session) resourceLookupRoots() []string {
	if len(s.lookupRoots) > 0 {
		return s.lookupRoots
	}
	return []string{s.dataDir}
}

func descend(root string) []string {
	var out []string
	type entry struct {
		path  string
		depth int
	}
	queue := []entry{{path: root, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		info, err := os.Stat(current.path)
		if err != nil || !info.IsDir() {
			continue
		}
		out = append(out, current.path)
		if current.depth >= maxLocateDepth {
			continue
		}
		children, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		var names []string
		for _, child := range children {
			if child.IsDir() {
				names = append(names, child.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			queue = append(queue, entry{path: filepath.Join(current.path, name), depth: current.depth + 1})
		}
	}
	return out
}

func hasNamedFile(directory string, names []string) bool {
	children, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, child := range children {
		if child.IsDir() {
			continue
		}
		for _, name := range names {
			if child.Name() == name {
				return true
			}
		}
	}
	return false
}

func matchFileFold(directory string, names []string) (string, bool) {
	children, err := os.ReadDir(directory)
	if err != nil {
		return "", false
	}
	wants := make([]string, 0, len(names))
	for _, name := range names {
		wants = append(wants, name)
	}
	sort.Strings(wants)
	for _, want := range wants {
		for _, child := range children {
			if child.IsDir() {
				continue
			}
			if equalFold(child.Name(), want) {
				return filepath.Join(directory, child.Name()), true
			}
		}
	}
	return "", false
}

func matchGlobFold(directory string, pattern string) (string, bool) {
	children, err := os.ReadDir(directory)
	if err != nil {
		return "", false
	}
	var names []string
	for _, child := range children {
		if !child.IsDir() {
			names = append(names, child.Name())
		}
	}
	sort.Strings(names)
	suffix := pattern[1:]
	for _, name := range names {
		if len(name) >= len(suffix) && equalFold(name[len(name)-len(suffix):], suffix) {
			return filepath.Join(directory, name), true
		}
	}
	return "", false
}

func equalFold(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// collectMovies 收集 root 下的影片文件：movie 子目录里的，以及直接放在 root 根目录的
// （旧作 SH/op.avi、staffroll.avi 就是这种布局，没有 movie 目录）。
//
// 不能只查 <dataDir>/movie：实际发行版的结构是 <root>/Files/movie，而
// 翻译补丁会把一份 bsxx.dat 放到 <root> 根目录。resolveDataDir 以"目录里有
// 脚本"为准，于是 dataDir 会选中 <root>，此时 <root>/movie 并不存在，
// 结果就是一个影片都索引不到，标题 OP / 片尾被静默跳过（engine.log 里是
// "session: 0 movie file(s)"）。这里用现成的 descend（BFS，最多 4 层）
// 找出所有名为 movie 的目录，兼容根目录、Files 子目录和未来的其他布局。
func collectMovies(root string) []string {
	var files []string
	seen := make(map[string]struct{})
	for _, directory := range movieDirectories(root) {
		children, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.IsDir() || !isMovieFile(child.Name()) {
				continue
			}
			if directory == root && !isRootTitleMovie(child.Name()) {
				continue
			}
			path := filepath.Join(directory, child.Name())
			// 统一在引擎侧把影片路径收口成绝对路径：
			// 宿主通过 op 40 直接拿绝对路径交给播放器，而游戏根目录是设置里的自由文本，
			// 可能是相对路径；如果这里保留相对路径，宿主工作目录一变，
			// File(relative) 就打不开影片。取绝对路径失败时保留原路径，不丢影片。
			if absolute, err := filepath.Abs(path); err == nil {
				path = absolute
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

// movieDirectories 返回 root 下可能放影片的目录：root 自身（旧作 op.avi、staffroll.avi
// 直接放在数据目录根部）以及所有名为 movie 的子目录。
func movieDirectories(root string) []string {
	var out []string
	for _, directory := range descend(root) {
		if directory == root || strings.EqualFold(filepath.Base(directory), "movie") {
			out = append(out, directory)
		}
	}
	return out
}

// isRootTitleMovie 判断 root 根目录里的文件是不是标题用影片。
//
// 根目录只收 op / staffroll 两个固定名字：数据目录里还混着其他资源，整体收进来
// 会让同名或相近的文件抢走 MoviePath 的匹配，影响其他作品。
func isRootTitleMovie(name string) bool {
	stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	return stem == "op" || stem == "staffroll"
}

// isMovieFile 只把影片扩展名收进索引。
//
// movie 目录里可能混着 readme、字幕或缓存文件；把非影片也收进来不会播错，
// 但会污染 MoviePath 的主干匹配（例如 readme.txt 的 stem 恰好和剧本名相同）。
func isMovieFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mpg", ".mpeg", ".m2v", ".mp4", ".m4v", ".avi", ".wmv", ".mkv", ".webm", ".mov":
		return true
	default:
		return false
	}
}

// dataSaveDir 返回作品实际使用的存档目录。
//
// 多数作品是 <dataDir>/save；但有些发行包把存档放在数据目录之外——
// 牝教師3 的发行版是 `Game/data` + 游戏根 `Save/save`，原引擎把
// common.dat 写在根目录那份里。只查 <dataDir>/save 会导致：
//   - 读不到玩家原有存档（读档画面全空）；
//   - 新写的设置/成就落在一个原引擎不会看的位置。
//
// 判据是 common.dat：它有才是真正的存档目录；空目录（比如发行包里
// 预先建好的 Game/data/save）不能抢走根目录那份。
func dataSaveDir(dataDir string) string {
	direct := filepath.Join(dataDir, "save")
	if core.PathExists(filepath.Join(direct, "common.dat")) {
		return direct
	}
	parent := filepath.Dir(dataDir)
	grandparent := filepath.Dir(parent)
	for _, candidate := range []string{
		filepath.Join(parent, "save"),
		filepath.Join(parent, "Save", "save"),
		filepath.Join(grandparent, "Save", "save"),
	} {
		if core.PathExists(filepath.Join(candidate, "common.dat")) {
			return candidate
		}
	}
	return direct
}

func defaultCacheDir() string {
	return filepath.Join(os.TempDir(), "kizuna-bishop")
}

func roundHalfEven(value float64) int {
	floor := int(value)
	frac := value - float64(floor)
	if frac < 0 {
		frac = -frac
	}
	switch {
	case frac < 0.5:
		return floor
	case frac > 0.5:
		if value >= 0 {
			return floor + 1
		}
		return floor - 1
	default:
		if floor%2 == 0 {
			return floor
		}
		if value >= 0 {
			return floor + 1
		}
		return floor - 1
	}
}
