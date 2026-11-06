package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/disc"
)

const minimumWeight = 1024 * 1024

type EngineKind int

const (
	KindModern EngineKind = iota
	KindLegacy
	KindGothic
	KindTj2
	KindBdt
	KindEmon
)

func (k EngineKind) BrandID() string {
	switch k {
	case KindLegacy:
		return "bishop-legacy"
	case KindGothic:
		return "bishop-gothic"
	case KindTj2:
		return "bishop-tj2"
	case KindBdt:
		return "bishop-bdt"
	case KindEmon:
		return "emon"
	default:
		return "bishop"
	}
}

func (k EngineKind) BrandName() string {
	switch k {
	case KindLegacy:
		return "Bishop Legacy"
	case KindGothic:
		return "Bishop Gothic"
	case KindTj2:
		return "Bishop Tj2"
	case KindBdt:
		return "Bishop Bdt"
	case KindEmon:
		return "EmonEngine"
	default:
		return "Bishop"
	}
}

type GameEntry struct {
	BrandID       string
	BrandName     string
	Name          string
	Directory     string
	DataDirectory string
	CoverPath     string
	ExtraDirs     []string
	ScriptPath    string
	HasScript     bool
	Weight        int64
	Kind          EngineKind
}

func Scan(root string) []GameEntry {
	if root == "" || !core.DirExists(root) {
		return nil
	}
	children, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	found := make([]GameEntry, 0, len(children))
	for _, child := range children {
		path := filepath.Join(root, child.Name())
		if child.IsDir() {
			if entry, ok := Describe(path); ok {
				found = append(found, entry)
				continue
			}
			if disc.IsContainer(path) {
				if entry, ok := describeDisc(path); ok {
					found = append(found, entry)
				}
			}
			continue
		}
		// 用户把游戏根直接指向光盘镜像文件时，镜像本身就是一个“游戏目录”。
		if disc.IsContainer(path) {
			if entry, ok := describeDisc(path); ok {
				found = append(found, entry)
			}
		}
	}
	// 根目录本身可能就是一部游戏（设置里直接选了游戏目录）：只有在上面的
	// 子目录扫描一无所获时才回退，避免把整个游戏库误判成一部作品。
	if len(found) == 0 {
		if entry, ok := Describe(root); ok {
			found = append(found, entry)
		} else if disc.IsContainer(root) {
			if entry, ok := describeDisc(root); ok {
				found = append(found, entry)
			}
		}
	}
	sort.Slice(found, func(i, j int) bool {
		return found[i].Name < found[j].Name
	})
	return found
}

func Describe(directory string) (GameEntry, bool) {
	if !core.DirExists(directory) {
		if disc.IsContainer(directory) {
			return describeDisc(directory)
		}
		return GameEntry{}, false
	}
	if disc.IsContainer(directory) {
		// 目录里同时存在已解包目录与镜像时，优先用已解包内容：用户可以随时
		// 删掉镜像而不影响游戏库。只有没有可用数据目录时才回退到镜像。
		if entry, ok := describeDirectory(directory); ok {
			return entry, true
		}
		return describeDisc(directory)
	}
	return describeDirectory(directory)
}

func describeDirectory(directory string) (GameEntry, bool) {
	name := baseName(directory)
	if name == "" {
		return GameEntry{}, false
	}
	data, ok := FindDataDirectory(directory)
	if !ok {
		core.Logf("library: '%s' has no script, archive or cg.dat", name)
		return GameEntry{}, false
	}
	kind := matchKind(data)
	weight := Measure(data)
	if weight < minimumWeight && kind != KindEmon && !core.FileExists(FindScript(data)) {
		return GameEntry{}, false
	}
	if data != directory {
		core.Logf("library: '%s' data directory resolved to '%s'", name, data)
	}
	// 多光盘安装/已解包的旧作会把资源分散在多个目录：主目录之外还有
	// Graphics2.Snn、Voice.Snn、scene.dat 等。全部作为 extraDirs 交给资源
	// 索引，主目录仍然拥有最高优先级。
	extras := CompanionDataDirs(directory, data)
	if kind == KindTj2 {
		for _, extra := range FindCompanionDataDirectories(directory, data) {
			extras = appendUnique(extras, extra)
		}
	}
	entry := GameEntry{
		BrandID:       kind.BrandID(),
		BrandName:     kind.BrandName(),
		Name:          name,
		Directory:     directory,
		DataDirectory: data,
		CoverPath:     FindCover(data),
		ExtraDirs:     extras,
		Kind:          kind,
		Weight:        weight,
	}
	entry.ScriptPath = scriptPath(kind, data)
	entry.HasScript = kind == KindLegacy || kind == KindGothic || kind == KindTj2 || kind == KindBdt || kind == KindEmon || core.FileExists(entry.ScriptPath)
	return entry, true
}

// CompanionDataDirs 找出同一游戏目录下其它含脚本/归档的子目录。
// 排除主目录本身、主目录的后代，以及 save/movie 这类明显不是资源根的位置；
// 返回顺序固定，避免同一份资源索引在两次扫描里优先级漂移。
func CompanionDataDirs(root, primary string) []string {
	primary = filepath.Clean(primary)
	separator := string(filepath.Separator)
	var candidates []string
	for _, candidate := range descend(root) {
		candidate = filepath.Clean(candidate)
		if candidate == primary {
			continue
		}
		// 主目录的后代（save/movie 等）和主目录的祖先（游戏根本身）都不是
		// 额外资源根；把祖先加进去会让同一个归档被扫描两次。
		if strings.HasPrefix(candidate, primary+separator) {
			continue
		}
		if strings.HasPrefix(primary, candidate+separator) {
			continue
		}
		switch strings.ToLower(filepath.Base(candidate)) {
		case "save", "movie", "thumb", "manual":
			continue
		}
		if !hasResourceAnchor(candidate) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i]) < len(candidates[j])
	})
	// 剪掉被更上层 extra 包含的目录：KD/Disc2 与 KD/Disc2/backup 同时命中时，
	// 只保留上层，避免同一个 .Snn 打开两份。
	var extras []string
	for _, candidate := range candidates {
		nested := false
		for _, accepted := range extras {
			if strings.HasPrefix(candidate, accepted+separator) {
				nested = true
				break
			}
		}
		if !nested {
			extras = append(extras, candidate)
		}
	}
	sort.Strings(extras)
	return extras
}

// HasPlayableData 判断目录里是否已经有一套“能直接打开”的游戏数据。
// opOpen 用它决定是否需要解包光盘镜像：只有脚本真的存在（或非 Modern 引擎
// 的锚点存在）才跳过解包，避免残留的半成品解包目录把镜像挡掉。
func HasPlayableData(root string) bool {
	data, ok := FindDataDirectory(root)
	if !ok || Measure(data) <= 0 {
		return false
	}
	if core.FileExists(FindScript(data)) {
		return true
	}
	return matchKind(data) != KindModern
}

func hasResourceAnchor(directory string) bool {
	if isDataDirectory(directory) {
		return true
	}
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		switch core.LowerExt(name) {
		case ".bsa", ".snn", ".dsk", ".pft", ".pk", ".zbm":
			return true
		}
	}
	return false
}

func appendUnique(list []string, value string) []string {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return list
		}
	}
	return append(list, value)
}

// describeDisc 在不落盘的情况下读取 ISO 目录，判断这是哪部作品。
// Directory 仍然保留镜像/镜像目录本身：真正打开时 opOpen 会调用 disc.Prepare
// 解包到缓存，宿主不需要知道缓存路径。
func describeDisc(root string) (GameEntry, bool) {
	files, ok, err := disc.List(root)
	if err != nil {
		core.Logf("library: disc '%s' list failed: %v", baseName(root), err)
		return GameEntry{}, false
	}
	if !ok || len(files) == 0 {
		return GameEntry{}, false
	}
	directories := make(map[string][]string)
	for _, file := range files {
		normalized := strings.ReplaceAll(file.Path, "\\", "/")
		index := strings.LastIndex(normalized, "/")
		directory := ""
		name := normalized
		if index >= 0 {
			directory = normalized[:index]
			name = normalized[index+1:]
		}
		directories[directory] = append(directories[directory], strings.ToLower(name))
	}
	bestDir := ""
	bestScore := -1
	bestSize := int64(0)
	for directory, names := range directories {
		score := 0
		hasScript := false
		for _, name := range names {
			switch name {
			case "bss.dat", "bsx.dat", "bsxx.dat", "scnfile.pft", "scene.pft", "maincon.scs", "emecfg.ecf":
				score += 100
				hasScript = true
			// 满淫电车这类光盘版剧本在 inst/ 子目录：scene00.bdt/system.bdt 是 BDT 剧本锚点，
			// mden.exe 是该作的启动器；三者给出高分与脚本锚点，否则只含这三个文件的
			// 镜像（后文还有 cg/*.zbm、pcm/**/*.vqf 等散装资源）会被整盘拒绝。
			case "scene00.bdt", "system.bdt", "mden.exe":
				score += 100
				hasScript = true
			// atktbl.bdt / indexwww.dat 不是剧本入口，只是同一批 BDT 資源的旁证；
			// 单个旁证不算脚本锚点，避免普通工具盘被误判。
			case "atktbl.bdt", "indexwww.dat":
				score += 40
			case "cg.dat":
				score += 40
			}
			switch strings.ToLower(filepath.Ext(name)) {
			case ".bsa", ".snn", ".pft", ".dsk", ".pk":
				score += 10
			case ".bdt":
				// 老的光盘只按名称比对会漏掉 scene00.bdt 这类小写变体：
				// .bdt 后缀本身也是 BDT 剧本的旁证。
				score += 10
			}
		}
		if !hasScript && score == 0 {
			continue
		}
		if score > bestScore || (score == bestScore && (bestDir == "" || len(directory) < len(bestDir))) {
			bestDir = directory
			bestScore = score
		}
	}
	if bestDir == "" && bestScore < 0 {
		return GameEntry{}, false
	}
	// 用虚拟数据目录里的文件名判断引擎类型，规则与真实目录一致。
	kind := KindModern
	names := directories[bestDir]
	for _, name := range names {
		switch name {
		case "scnfile.pft":
			kind = KindLegacy
		case "scene.pft":
			kind = KindTj2
		case "maincon.scs":
			kind = KindGothic
		case "emecfg.ecf":
			kind = KindEmon
		}
		if kind != KindModern {
			break
		}
	}
	for _, name := range names {
		if strings.HasSuffix(name, ".bdt") {
			kind = KindBdt
			break
		}
	}
	name := baseName(root)
	if name == "" {
		name = "disc"
	}
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case ".iso", ".bin", ".img", ".mdf", ".nrg", ".cue", ".ccd", ".mds":
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	for _, file := range files {
		bestSize += file.Size
	}
	entry := GameEntry{
		BrandID:       kind.BrandID(),
		BrandName:     kind.BrandName(),
		Name:          name,
		Directory:     root,
		DataDirectory: root,
		ExtraDirs:     nil,
		Kind:          kind,
		Weight:        bestSize,
	}
	entry.ScriptPath = root
	entry.HasScript = true
	return entry, true
}

func matchKind(data string) EngineKind {
	if hasFile(data, "SCNFILE.PFT") {
		return KindLegacy
	}
	if hasExtension(data, ".bdt") {
		return KindBdt
	}
	if hasFile(data, "SCENE.PFT") {
		return KindTj2
	}
	if hasFile(data, "MAINCON.SCS") {
		return KindGothic
	}
	if hasFile(data, "emecfg.ecf") {
		return KindEmon
	}
	return KindModern
}

func scriptPath(kind EngineKind, data string) string {
	switch kind {
	case KindLegacy:
		return filepath.Join(data, "SCNFILE.PFT")
	case KindTj2:
		return filepath.Join(data, "SCENE.PFT")
	case KindGothic:
		return filepath.Join(data, "MAINCON.SCS")
	case KindBdt:
		return filepath.Join(data, "scene00.bdt")
	case KindEmon:
		return filepath.Join(data, "ASMainScript.txt")
	default:
		return FindScript(data)
	}
}

func hasFile(directory string, name string) bool {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, candidate := range names {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}

func hasExtension(directory string, extension string) bool {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	want := strings.ToLower(extension)
	for _, name := range names {
		if core.LowerExt(name) == want {
			return true
		}
	}
	return false
}

func Measure(directory string) int64 {
	var total int64
	entries, err := os.ReadDir(directory)
	if err != nil {
		return total
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}

func baseName(path string) string {
	trimmed := strings.TrimRight(path, "/\\")
	if trimmed == "" {
		return ""
	}
	index := strings.LastIndexAny(trimmed, "/\\")
	return trimmed[index+1:]
}

func trimExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}
