package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/enginego/core"
	"kizuna/enginego/core/strutil"
)

// MaxDepth 6：光盘镜像解包后是 <mount>/iso/program files/BISHOP/TB，
// 数据文件在第 5 层；旧的 4 层会让整个《特別病棟》扫描不到。
const MaxDepth = 6

const defaultConfiguration = "bsxx.cfg"

var scriptCandidates = []string{"bsxx.dat", "bsx.dat", "bss.dat"}

var archiveSuffixes = []string{".bsa", ".snn"}

// dataAnchors 是逐字 FileExists 匹配的表；data.xp3 的后缀/大小写变体由
// isKrkrDirectory 统一处理，所以只把字面量放进 primaryAnchors（EqualFold 匹配）。
var dataAnchors = []string{"cg.dat", "bsxx.dat", "bsx.dat", "bss.dat", "SCNFILE.PFT", "scnfile.pft", "MAINCON.SCS", "maincon.scs", "SCENE.PFT", "scene.pft", "scene00.bdt", "emecfg.ecf", "X0WIN.DAT", "X0WIN.LST", "x0win.dat", "x0win.lst", "srp.pac", "SRP.PAC"}

var primaryAnchors = []string{"SCENE.PFT", "SCNFILE.PFT", "MAINCON.SCS", "cg.dat", "bsxx.dat", "bsx.dat", "bss.dat", "scene00.bdt", "emecfg.ecf", "X0WIN.DAT", "x0win.dat", "data.xp3", "srp.pac", "SRP.PAC"}

var configurationPatterns = []string{"*.cfg", "*.cfg.txt"}

// DetectDataDirectory 是数据目录判定的规范入口：包含 data.xp3（大小写不敏感）
// 或任意 *.xp3、start.tjs、first.ks、scenario/*.ks 的目录都算数据根。
//
// FindDataDirectory 是同一规则在 capi/brand 里的既有名字，保留以免改动调用点。
func DetectDataDirectory(root string) (string, bool) {
	fallback := ""
	hasFallback := false
	for _, directory := range descend(root) {
		if hasPrimaryAnchor(directory) {
			return preferKiriKiriDataArchive(directory), true
		}
		if !hasFallback && isDataDirectory(directory) {
			fallback = directory
			hasFallback = true
		}
	}
	if hasFallback {
		return preferKiriKiriDataArchive(fallback), true
	}
	return "", false
}

func FindDataDirectory(root string) (string, bool) {
	return DetectDataDirectory(root)
}

// preferKiriKiriDataArchive 在 KiriKiri 候选目录内优先挑真正持有 data.xp3 的
// 那一层：父目录里散落的 patch.xp3/voice.xp3 只是补丁，把父目录当数据根会
// 让引擎在错误的位置查找 Config.tjs 与剧本。非 KiriKiri 目录原样返回，
// 避免 .xp3 抢走其它引擎已命中的锚点。
func preferKiriKiriDataArchive(directory string) string {
	// matchKind 不会再调回 FindDataDirectory，这里不存在递归。
	if matchKind(directory) != KindKrkr || hasKiriKiriDataArchive(directory) {
		return directory
	}
	for _, candidate := range descend(directory) {
		if hasKiriKiriDataArchive(candidate) {
			return candidate
		}
	}
	return directory
}

func FindCompanionDataDirectories(directory string, primary string) []string {
	primaryStems := pftStems(primary)
	result := []string{}
	for _, candidate := range descend(directory) {
		if candidate == primary {
			continue
		}
		if hasNewArchivePair(candidate, primaryStems) {
			result = append(result, candidate)
		}
	}
	sort.Strings(result)
	return result
}

func FindScript(root string) string {
	for _, directory := range descend(root) {
		for _, candidate := range scriptCandidates {
			path := filepath.Join(directory, candidate)
			if core.FileExists(path) {
				return path
			}
		}
	}
	return filepath.Join(root, scriptCandidates[0])
}

func FindConfiguration(root string) string {
	for _, directory := range descend(root) {
		for _, pattern := range configurationPatterns {
			if match, ok := matchPattern(directory, pattern); ok {
				return match
			}
		}
	}
	return filepath.Join(root, defaultConfiguration)
}

func FindFile(root string, name string) (string, bool) {
	for _, directory := range descend(root) {
		path := filepath.Join(directory, name)
		if core.FileExists(path) {
			return path, true
		}
	}
	return "", false
}

func FindCover(root string) string {
	candidates := []string{"cover.png", "cover.jpg", "thumb.png", "thumb.jpg", "startup.bmp", "package.png", "menu.kg", "menu.KG"}
	for _, directory := range descend(root) {
		for _, candidate := range candidates {
			path := filepath.Join(directory, candidate)
			if core.FileExists(path) {
				return path
			}
		}
	}
	return ""
}

func isDataDirectory(directory string) bool {
	for _, anchor := range dataAnchors {
		if core.FileExists(filepath.Join(directory, anchor)) {
			return true
		}
	}
	// Over-Dose 没有散装脚本锚点；用 script.arc + system.arc 成对判断，
	// 比把通用 .arc 后缀塞进 archiveSuffixes 更不容易误伤别的引擎。
	if isOverdoseDirectory(directory) {
		return true
	}
	if isKrkrDirectory(directory) {
		return true
	}
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		if strutil.HasAnySuffixFold(name, archiveSuffixes) {
			return true
		}
	}
	return hasLegacyArchivePair(directory)
}

func hasPrimaryAnchor(directory string) bool {
	// Over-Dose 必须在 primary 阶段就命中，否则父目录里其它归档的探测顺序
	// 可能先把数据目录判成 fallback。
	if isOverdoseDirectory(directory) {
		return true
	}
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		if strutil.HasAnySuffixFold(name, archiveSuffixes) {
			return true
		}
		if hasKrkrScriptName(name) {
			return true
		}
		for _, anchor := range primaryAnchors {
			if strings.EqualFold(name, anchor) {
				return true
			}
		}
	}
	// 裸的 patch.xp3/voice.xp3 只算 fallback（isDataDirectory），否则父目录里
	// 一个零散的 xp3 会排到真正的 Bishop 子目录前面。
	return hasScenarioScript(directory)
}

// hasKiriKiriDataArchive 只认 data.xp3：它是 KiriKiri 最后挂载的归档。
// patch.xp3 / voice.xp3 这类补丁归档证明不了“这里有一部游戏”。
func hasKiriKiriDataArchive(directory string) bool {
	return hasFile(directory, "data.xp3")
}

// isKrkrDirectory 识别 KiriKiri 的数据目录：任意 *.xp3，或 start.tjs /
// first.ks / scenario/*.ks 这类散装脚本。规则与 enginec 的 krkr::detect 对齐，
// 避免库扫描显示的 brand 与真正打开时的识别结果不同。
func isKrkrDirectory(directory string) bool {
	if hasXP3(directory) {
		return true
	}
	return hasKrkrScriptAnchor(directory)
}

func hasXP3(directory string) bool {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		if core.HasFoldSuffix(name, ".xp3") {
			return true
		}
	}
	return false
}

// hasKrkrScriptAnchor 只认散装脚本锚点（start.tjs / first.ks / scenario/*.ks），
// 不含 xp3：primary 阶段的锚点必须能证明“这里有一部游戏”，而孤立的补丁
// 归档证明不了。
func hasKrkrScriptAnchor(directory string) bool {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		if hasKrkrScriptName(name) {
			return true
		}
	}
	return hasScenarioScript(directory)
}

func hasKrkrScriptName(name string) bool {
	return strings.EqualFold(name, "start.tjs") || strings.EqualFold(name, "first.ks")
}

// hasScenarioScript 检查 scenario/ 子目录里是否有 .ks。目录名按大小写不敏感
// 匹配：解包出来的移植版常见 "Scenario" 这一层大小写差异。
func hasScenarioScript(directory string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.EqualFold(entry.Name(), "scenario") {
			continue
		}
		names, err := core.ListFilesTop(filepath.Join(directory, entry.Name()))
		if err != nil {
			continue
		}
		for _, name := range names {
			if core.HasFoldSuffix(name, ".ks") {
				return true
			}
		}
	}
	return false
}

func pftStems(directory string) map[string]struct{} {
	stems := map[string]struct{}{}
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return stems
	}
	for _, name := range names {
		if core.HasFoldSuffix(name, ".pft") {
			stems[strings.ToLower(trimExt(name))] = struct{}{}
		}
	}
	return stems
}

func hasNewArchivePair(directory string, primaryStems map[string]struct{}) bool {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	var dskStems map[string]struct{}
	var pft []string
	for _, name := range names {
		switch core.LowerExt(name) {
		case ".dsk":
			if dskStems == nil {
				dskStems = map[string]struct{}{}
			}
			dskStems[strings.ToLower(trimExt(name))] = struct{}{}
		case ".pft":
			pft = append(pft, strings.ToLower(trimExt(name)))
		}
	}
	if dskStems == nil {
		return false
	}
	for _, stem := range pft {
		if _, ok := dskStems[stem]; !ok {
			continue
		}
		if _, primary := primaryStems[stem]; !primary {
			return true
		}
	}
	return false
}

func hasLegacyArchivePair(directory string) bool {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	var pftStems map[string]struct{}
	var dskStems []string
	for _, name := range names {
		if core.HasFoldSuffix(name, ".dsk") {
			dskStems = append(dskStems, strings.ToLower(trimExt(name)))
		} else if core.HasFoldSuffix(name, ".pft") {
			if pftStems == nil {
				pftStems = map[string]struct{}{}
			}
			pftStems[strings.ToLower(trimExt(name))] = struct{}{}
		}
	}
	if pftStems == nil {
		return false
	}
	for _, stem := range dskStems {
		if _, ok := pftStems[stem]; ok {
			return true
		}
	}
	return false
}

func matchPattern(directory string, pattern string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(directory, pattern))
	if err != nil {
		return "", false
	}
	files := make([]string, 0, len(matches))
	for _, match := range matches {
		if core.FileExists(match) {
			files = append(files, match)
		}
	}
	if len(files) == 0 {
		return "", false
	}
	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i]) < strings.ToLower(files[j])
	})
	return files[0], true
}

// isOverdoseDirectory 用“script.arc + system.arc 同目录”判断 Over-Dose。
// 单看 .arc 后缀会把别的引擎的通用归档误伤；这两件同时存在才是数据目录。
func isOverdoseDirectory(directory string) bool {
	return core.FileExists(filepath.Join(directory, "script.arc")) &&
		core.FileExists(filepath.Join(directory, "system.arc"))
}

func descend(root string) []string {
	if !core.DirExists(root) {
		return nil
	}
	type node struct {
		path  string
		depth int
	}
	out := []string{}
	queue := []node{{root, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out = append(out, cur.path)
		if cur.depth >= MaxDepth {
			continue
		}
		children, err := os.ReadDir(cur.path)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.IsDir() {
				queue = append(queue, node{filepath.Join(cur.path, child.Name()), cur.depth + 1})
			}
		}
	}
	return out
}
