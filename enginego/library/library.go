package library

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/enginego/core"
	"kizuna/enginego/disc"
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
	KindOverdose
	KindBlackPackage
	KindTmrHiro
	KindKrkr
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
	case KindOverdose:
		return "overdose"
	case KindBlackPackage:
		return "blackpackage"
	case KindTmrHiro:
		return "tmrhiro"
	case KindKrkr:
		return "krkr"
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
	case KindOverdose:
		return "Over-Dose Moon"
	case KindBlackPackage:
		return "BLACKPACKAGE (FFA SYSTEM)"
	case KindTmrHiro:
		return "TMR-Hiro ADV System"
	case KindKrkr:
		return "KiriKiri"
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
	// KiriKiri 的资源几乎全在 xp3 里：用归档体积作权重才反映真实体量，只有
	// 启动器的目录量出来接近 0，会被下面的门槛当成空目录。散装 first.ks 的
	// 移植版没有 xp3，仍按目录大小算。
	if kind == KindKrkr {
		if archiveWeight := xp3Weight(data); archiveWeight > 0 {
			// 存在 xp3 就说明是完整安装：权重必须高于门槛，否则归档被裁剪过的
			// 作品会在游戏库里被当成空目录隐藏。
			weight = max(archiveWeight, minimumWeight+1)
		}
	}
	// Over-Dose 的数据目录只有若干 .arc + yui.exe，脚本锚点在归档内，
	// FindScript 找不到散装脚本；它和 Emon 一样按 kind 直接放行。
	if weight < minimumWeight && kind != KindEmon && kind != KindOverdose && kind != KindBlackPackage && kind != KindKrkr && kind != KindTmrHiro && !core.FileExists(FindScript(data)) {
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
	entry.HasScript = kind == KindTmrHiro || kind == KindLegacy || kind == KindGothic || kind == KindTj2 || kind == KindBdt || kind == KindEmon || kind == KindOverdose || kind == KindBlackPackage || kind == KindKrkr || core.FileExists(entry.ScriptPath)
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
		case ".bsa", ".snn", ".dsk", ".pft", ".pk", ".zbm", ".xp3":
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
			// Over-Dose Moon：script.arc 是剧本，graphic.arc 是资源，yui.exe 是启动器。
			// 只认 script.arc 会误伤别的 .arc 引擎，所以再加 graphic.arc 或 yui.exe 作旁证。
			case "script.arc", "graphic.arc", "yui.exe":
				score += 100
				hasScript = true
			// 满淫电车这类光盘版剧本在 inst/ 子目录：scene00.bdt/system.bdt 是 BDT 剧本锚点，
			// mden.exe 是该作的启动器；三者给出高分与脚本锚点，否则只含这三个文件的
			// 镜像（后文还有 cg/*.zbm、pcm/**/*.vqf 等散装资源）会被整盘拒绝。
			case "scene00.bdt", "system.bdt", "mden.exe":
				score += 100
				hasScript = true
			// KiriKiri 光盘版：data.xp3 是本体归档（脚本都在里面），patch.xp3 /
			// voice.xp3 只是补丁与语音，不能单独充当脚本锚点。
			case "data.xp3":
				score += 100
				hasScript = true
			// TMR-Hiro：srp.pac 是剧本归档；资源归档只是旁证，
			// 单凭 mus.pac 会把别的手工工具盘误判成一部作品。
			case "srp.pac":
				score += 100
				hasScript = true
			// atktbl.bdt / indexwww.dat 不是剧本入口，只是同一批 BDT 資源的旁证；
			// 单个旁证不算脚本锚点，避免普通工具盘被误判。
			case "atktbl.bdt", "indexwww.dat":
				score += 40
			case "cg.dat":
				score += 40
			case "mus.pac", "se.pac", "vic.pac", "agrd.pac", "bgr.pac", "cgrd.pac":
				// TMR-Hiro 的数据归档：与 srp.pac 同目录才算完整安装。
				score += 20
			}
			switch strings.ToLower(filepath.Ext(name)) {
			case ".bsa", ".snn", ".pft", ".dsk", ".pk", ".arc":
				score += 10
			case ".xp3":
				// 其它 xp3（patch/voice）让目录成为 KiriKiri 候选；脚本锚点只有
				// data.xp3，单凭补丁归档不足以判定镜像里有一部作品。
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
		case "script.arc", "graphic.arc", "yui.exe":
			kind = KindOverdose
		case "data.xp3":
			kind = KindKrkr
		case "srp.pac":
			kind = KindTmrHiro
		}
		if kind != KindModern {
			break
		}
	}
	// KiriKiri 镜像里即使混进 .bdt 旁证文件也不能被改判成 BDT：.xp3 已经
	// 明确是另一套引擎。
	if kind != KindKrkr {
		for _, name := range names {
			if strings.HasSuffix(name, ".bdt") {
				kind = KindBdt
				break
			}
		}
	}
	// 只有 patch.xp3 / voice.xp3、没有 data.xp3 的镜像同样是 KiriKiri（例如
	// 只压了补丁盘），但优先级低于上面按锚点判定的结果。
	if kind == KindModern {
		for _, name := range names {
			if core.HasFoldSuffix(name, ".xp3") {
				kind = KindKrkr
				break
			}
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
	// KiriKiri 的入口在 data.xp3 内部，扫描阶段读不到；用 KAG 约定的
	// start.ks 与目录扫描保持一致，其它引擎保持镜像路径。
	if kind == KindKrkr {
		entry.ScriptPath = "start.ks"
	}
	entry.HasScript = true
	return entry, true
}

func matchKind(data string) EngineKind {
	// TMR-Hiro：srp.pac 是剧本归档，mus/se/vic 至少一个资源归档才认。
	// 它的归档没有魔数，只能靠这组文件名；放在最前面是因为 srp.pac 不会
	// 与其它引擎的锚点撞名，先判不会误伤。
	if hasTmrHiroAnchors(data) {
		return KindTmrHiro
	}
	// Over-Dose：script.arc/graphic.arc 同目录，yui.exe 是启动器。
	// 归档后缀是通用的 .arc，不能只靠后缀判断；script.arc 与 graphic.arc
	// 并存、或 yui.exe 存在才认。
	if hasFile(data, "yui.exe") || (hasFile(data, "script.arc") && hasFile(data, "graphic.arc")) {
		return KindOverdose
	}
	if hasFile(data, "SCNFILE.PFT") {
		return KindLegacy
	}
	// data.xp3 是 KiriKiri 本体归档，命中就不该再被旁证的 .bdt 改判
	// （enginec 的 krkr::detect 同样把 data.xp3 当作决定性标记）。
	if hasKiriKiriDataArchive(data) {
		return KindKrkr
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
	if hasFile(data, "X0WIN.DAT") || hasFile(data, "X0WIN.LST") || hasFile(data, "x0win.dat") || hasFile(data, "x0win.lst") {
		return KindBlackPackage
	}
	// Bishop 现代作没有专门的后缀锚点，默认落到 Modern；显式返回一次，免得
	// 目录里顺带的 .xp3 把整部作品抢成 KiriKiri。
	if hasFile(data, "bsxx.dat") || hasFile(data, "bsx.dat") || hasFile(data, "bss.dat") || hasFile(data, "cg.dat") {
		return KindModern
	}
	// KiriKiri 放在最后：xp3 后缀和 general .ks 太通用，只有前面所有 Bishop/
	// Emon 锚点都没命中时才认，避免把别的引擎的移植目录抢过来。
	if isKrkrDirectory(data) {
		return KindKrkr
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
	case KindOverdose:
		// 剧本在 script.arc 里；这里的路径只用于“有剧本”标记与界面显示。
		return filepath.Join(data, "script.arc")
	case KindBlackPackage:
		// 剧本在 X0WIN.DAT 里，LST 是索引；路径同样只用于界面显示。
		return filepath.Join(data, "X0WIN.LST")
	case KindTmrHiro:
		// 剧本在 srp.pac 里，这里同样只用于界面显示。
		return filepath.Join(data, "srp.pac")
	case KindKrkr:
		// 真正的入口在 data.xp3 内部，扫描阶段读不到；这里给 KAG 约定的入口名，
		// 和 krkr::detect 返回的 storage name 一致（相对数据根，'/' 分隔）。
		// 路径不存在也没关系：只用于“有剧本”标记与界面显示。
		return "start.ks"
	default:
		return FindScript(data)
	}
}

// hasTmrHiroAnchors 判断目录是不是 TMR-Hiro 安装：srp.pac + 至少一个资源归档。
// 发行版把数据放在 <root>/________ 这类随机名子目录里，调用方通常已经在
// 数据根上调用，所以这里只看当前目录。
func hasTmrHiroAnchors(directory string) bool {
	rules := []string{"srp.pac", "mus.pac", "se.pac", "vic.pac", "agrd.pac", "bgr.pac", "cgrd.pac"}
	seen := make(map[string]bool, len(rules))
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return false
	}
	for _, name := range names {
		seen[strings.ToLower(name)] = true
	}
	if !seen["srp.pac"] {
		return false
	}
	for _, name := range rules[1:] {
		if seen[name] {
			return true
		}
	}
	return false
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

// xp3Weight 返回目录里 xp3 的总大小。KiriKiri 的游戏数据几乎全在归档里，
// 用归档体积而不是目录体积，能让只有 data.xp3 的目录过 minimumWeight 门槛。
func xp3Weight(directory string) int64 {
	names, err := core.ListFilesTop(directory)
	if err != nil {
		return 0
	}
	var total int64
	for _, name := range names {
		if !core.HasFoldSuffix(name, ".xp3") {
			continue
		}
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
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
