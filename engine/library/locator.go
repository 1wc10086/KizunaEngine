package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

// MaxDepth 6：光盘镜像解包后是 <mount>/iso/program files/BISHOP/TB，
// 数据文件在第 5 层；旧的 4 层会让整个《特別病棟》扫描不到。
const MaxDepth = 6

const defaultConfiguration = "bsxx.cfg"

var scriptCandidates = []string{"bsxx.dat", "bsx.dat", "bss.dat"}

var archiveSuffixes = []string{".bsa", ".snn"}

var dataAnchors = []string{"cg.dat", "bsxx.dat", "bsx.dat", "bss.dat", "SCNFILE.PFT", "scnfile.pft", "MAINCON.SCS", "maincon.scs", "SCENE.PFT", "scene.pft", "scene00.bdt", "emecfg.ecf"}

var primaryAnchors = []string{"SCENE.PFT", "SCNFILE.PFT", "MAINCON.SCS", "cg.dat", "bsxx.dat", "bsx.dat", "bss.dat", "scene00.bdt", "emecfg.ecf"}

var configurationPatterns = []string{"*.cfg", "*.cfg.txt"}

func FindDataDirectory(root string) (string, bool) {
	fallback := ""
	hasFallback := false
	for _, directory := range descend(root) {
		if hasPrimaryAnchor(directory) {
			return directory, true
		}
		if !hasFallback && isDataDirectory(directory) {
			fallback = directory
			hasFallback = true
		}
	}
	return fallback, hasFallback
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
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		if strutil.HasAnySuffixFold(name, archiveSuffixes) {
			return true
		}
		for _, anchor := range primaryAnchors {
			if strings.EqualFold(name, anchor) {
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
