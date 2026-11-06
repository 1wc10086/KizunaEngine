package session

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

// fallback_catalog.go 给"没有 cg.dat / scene.dat"的发行版重建鉴赏目录。
//
// 黒の教室 モーションプラス 是这类发行版：EV 图层图（evXX_YYY_1p00 等）
// 都在 GraphicsEV*.bsa 里，但安装包没有带目录文件。原引擎能正常显示鉴赏室，
// 说明它至少能从资源本身得到场景/变体；这里按同样的命名约定重建：
//
//	evXX_YYY_<层>p<姿势>  —— 第几层、第几个姿势的身体
//	evXX_YYY_<层>f<表情>  —— 同一层的表情（数字 = 层号 + 两位序号）
//
// 场景按 evXX 分成 6 页（和 [museum phase - cg] 的 6 个页签一致），
// 每个场景的第一组图层就是缩略图/查看器的默认画面；同一场景的其它姿势
// 作为变体供左右箭头切换。
//
// 因为拿不到原引擎的名字表顺序，common.dat 的逐位解锁无法对到正确的图；
// 这种发行版通常自带「全cg」存档，所以调用方把目录标记成 unlockAll。

const (
	fallbackCgGroupCount = 6
	fallbackMaxVariants  = 64
	fallbackMaxLayers    = 4
)

type fallbackLayerImages struct {
	poses map[string]string
	faces map[string]string
}

// resourceImageNames 枚举索引里的全部图片名（去掉扩展名、去重、排序）。
//
// 只在没有目录文件时调用一次，结果缓存在 Session 上：EV 归档有几千个条目，
// 每次打开鉴赏室都重新遍历会拖慢界面。
func (s *Session) resourceImageNames() []string {
	if s.imageNames != nil {
		return s.imageNames
	}
	if s.res == nil {
		s.imageNames = []string{}
		return s.imageNames
	}
	seen := make(map[string]bool, 8192)
	names := make([]string, 0, 8192)
	for _, archive := range s.res.Archives() {
		for _, entry := range archive.Entries() {
			name := path.Base(strings.ReplaceAll(entry.Path, "\\", "/"))
			name = strings.TrimSuffix(name, path.Ext(name))
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	s.imageNames = names
	return names
}

// fallbackBases 返回按脚本回放标签顺序排列的 EV 图前缀，以及前缀→标签映射。
//
// 场景页和 CG 页共用同一套顺序，这样"第几格是哪个场景"在两页之间一致。
func (s *Session) fallbackBases(byBase map[string][]string) ([]string, map[string]string) {
	if s.sceneLabels == nil {
		s.sceneLabels = s.probeSceneLabels(byBase)
	}
	labelFor := s.sceneLabels
	bases := make([]string, 0, len(byBase))
	for base := range byBase {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	if len(labelFor) == 0 {
		return bases, labelFor
	}
	sort.SliceStable(bases, func(a, b int) bool {
		offA := s.script.FindLabel(labelFor[bases[a]])
		offB := s.script.FindLabel(labelFor[bases[b]])
		if offA != offB {
			return offA < offB
		}
		return bases[a] < bases[b]
	})
	return bases, labelFor
}

// fallbackSceneCatalog 按 EV 图名重建场景表。
//
// 场景标签不能用"EV 编号相同"来猜：L01_008 实际播放的可能是 ev01_002。
// 原 scene.dat 的每一行来自脚本里真实存在的回放标签，所以这里先枚举标签，
// 用一次"只读探测"（不派发绘图命令）找出每个标签第一次引用的 EV 图，
// 再按标签在脚本里的顺序生成场景表。
func (s *Session) fallbackSceneCatalog() *data.SceneCatalog {
	names := s.resourceImageNames()
	byBase := map[string][]string{}
	for _, name := range names {
		if base, _, _, ok := evSceneBase(name); ok {
			byBase[base] = append(byBase[base], name)
		}
	}

	bases, labelFor := s.fallbackBases(byBase)

	scenes := make([]data.ReplayScene, 0, len(bases))
	indexByGroup := map[int]int{}
	for _, base := range bases {
		_, evGroup, _, _ := evSceneBase(base)
		group := s.fallbackEvPage(evGroup, false)
		// 鉴赏室只列真正能回放的场景：探测不到标签的图（例如纯 logo/过场）
		// 还在 CG 页里，但放进场景页只会点了没反应。
		if len(labelFor) > 0 && labelFor[base] == "" {
			continue
		}
		variants := fallbackSceneVariants(base, byBase[base])
		if len(variants) == 0 {
			continue
		}
		sceneID := labelFor[base]
		if sceneID == "" {
			sceneID = s.fallbackReplayLabel(evGroup, indexOfScene(base), base)
		}
		index := indexByGroup[group]
		indexByGroup[group]++
		images := variants[0]
		if cover := s.sceneCovers[base]; len(cover) > 0 {
			images = cover
		}
		scenes = append(scenes, data.ReplayScene{
			Group:   group,
			Index:   index,
			SceneId: sceneID,
			Label:   sceneID,
			Images:  images,
			Title:   base,
		})
	}

	core.Logf("catalog: rebuilt scene table from resources: %d scene(s)", len(scenes))
	return &data.SceneCatalog{Scenes: scenes, Titles: map[string]string{}}
}

// probeSceneLabels 找出每个 EV 图对应的真实回放标签。
//
// 直接跑脚本会解码图像、改舞台状态，代价很大；这里只调用 machine.Fetch()
// 推进 VM，遇到 Image 命令就读它的名字表参数，一旦命中 evXX_YYY 就停。
// 探测前后保存/恢复机器状态，不影响玩家正在玩的进度。
func (s *Session) probeSceneLabels(bases map[string][]string) map[string]string {
	if s.script == nil || s.machine == nil || len(bases) == 0 {
		return nil
	}
	saved := s.machine.CaptureState()
	restart := s.machine.RestartBoot
	defer func() {
		s.machine.RestoreState(saved)
		s.machine.RestartBoot = restart
	}()

	type sceneLabel struct {
		name   string
		offset int
	}
	labels := make([]sceneLabel, 0, 160)
	for index := 0; index < s.script.LabelCount(); index++ {
		name := s.script.GetLabelName(index)
		if !isSceneReplayLabel(name) {
			continue
		}
		labels = append(labels, sceneLabel{name: name, offset: s.script.GetLabelOffset(index)})
	}
	sort.Slice(labels, func(a, b int) bool { return labels[a].offset < labels[b].offset })

	result := map[string]string{}
	covers := make(map[string][]string, len(bases))
	for _, label := range labels {
		if len(result) >= len(bases) {
			break
		}
		base, cover := s.probeSceneBase(label.offset)
		if base == "" {
			continue
		}
		if _, ok := bases[base]; !ok {
			continue
		}
		existing, seen := result[base]
		prefer := !seen || (strings.HasSuffix(label.name, "_replay") && !strings.HasSuffix(existing, "_replay"))
		if prefer {
			result[base] = label.name
			if len(cover) > 0 {
				covers[base] = cover
			}
		} else if len(covers[base]) == 0 && len(cover) > 0 {
			covers[base] = cover
		}
	}
	s.sceneCovers = covers
	return result
}

// probeSceneBase 从某个标签开始推进 VM，返回它第一次引用的 EV 图前缀，
// 以及该场景在第一条对白之前实际绘制的同前缀图层。
//
// 后者就是原引擎展示这张 CG/场景时的初始构图：多人场景会依次画
// ev06_001_1p00 + 1f100 + 1f200 + 1f300…，只按命名规则取"身体+最小表情"
// 会漏掉其他角色，封面看起来就像少人/换人。这里只做 Fetch，不派发绘制，
// 所以不会真的解码图片。
func (s *Session) probeSceneBase(offset int) (string, []string) {
	s.machine.RestoreState(vm.EntryState(offset))
	s.machine.RestartBoot = false
	base := ""
	images := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for step := 0; step < 4000; step++ {
		command := s.machine.Fetch()
		if s.machine.Faulted() || s.machine.Halted() {
			return "", nil
		}
		// 第一条对白出现时画面已经稳定，后面的图属于剧情演出，不是封面。
		if command == vm.DisplayMessage && base != "" {
			break
		}
		name := ""
		switch command {
		case vm.Image, vm.ImageExtended:
			name = s.script.GetName(int(s.machine.Frame(1)))
		case vm.ChangeImage:
			name = s.script.GetName(int(s.machine.Frame(2)))
		}
		found, _, _, ok := evSceneBase(name)
		if !ok {
			continue
		}
		if base == "" {
			base = found
		}
		// 只收同一场景的部件；遇到别的 EV（过场闪现）不并入封面。
		if found != base || len(images) >= 16 {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		images = append(images, name)
	}
	return base, images
}

// isSceneReplayLabel 只接受 L01_001 或 L01_001_replay 这种完整场景标签。
func isSceneReplayLabel(name string) bool {
	if len(name) < 8 || name[0] != 'L' || name[3] != '_' {
		return false
	}
	if !allDigits(name[1:3]) || !allDigits(name[4:7]) {
		return false
	}
	return len(name) == 7 || (len(name) == 14 && name[7:] == "_replay")
}

// indexOfScene 从 ev01_001 取 1。
func indexOfScene(base string) int {
	if len(base) < 8 {
		return 0
	}
	value, _ := strconv.Atoi(base[5:8])
	return value
}

// museumCGPageCount 返回 CG 鉴赏的页数（配置里的 page icon 数量）。
func (s *Session) museumCGPageCount() int {
	pages := 0
	if s.config != nil {
		pages = len(s.config.Museum.CG.PageButtons)
	}
	if pages < 1 {
		pages = fallbackCgGroupCount
	}
	if pages > 12 {
		pages = 12
	}
	return pages
}

// fallbackEvPage 把 EV 场景编号映射到鉴赏页。
//
// 没有 cg.dat 的发行版（黒の教室 モーションプラス）的页签是
// 「5 名女主角 + その他」：ev01…ev05 依次是角色页，ev00（序章/共通场景）
// 和 ev06（多人场景）都属于最后一页。旧实现按 evGroup-1 直接夹取，
// ev00 被塞进第 1 页（菜菜子），于是角色页里会冒出别的角色的封面。
func (s *Session) fallbackEvPage(evGroup int, cg bool) int {
	pages := s.museumScenePageCount()
	if cg {
		pages = s.museumCGPageCount()
	}
	last := max(0, pages-1)
	if evGroup >= 1 && evGroup <= last {
		return evGroup - 1
	}
	return last
}

// fallbackCgCatalog 按 EV 图名重建 CG 目录（页数取鉴赏页签，每场景多姿势变体）。
func (s *Session) fallbackCgCatalog() *data.CgCatalog {
	names := s.resourceImageNames()
	byBase := map[string][]string{}
	for _, name := range names {
		if base, _, _, ok := evSceneBase(name); ok {
			byBase[base] = append(byBase[base], name)
		}
	}

	pageCount := s.museumCGPageCount()
	ordered, _ := s.fallbackBases(byBase)
	groups := make([]data.CgGroup, pageCount)
	usedNames := make([]string, 0, 2048)
	usedSeen := map[string]bool{}
	for group := 0; group < pageCount; group++ {
		bases := make([]string, 0, 64)
		for _, base := range ordered {
			_, evGroup, _, _ := evSceneBase(base)
			if s.fallbackEvPage(evGroup, true) == group {
				bases = append(bases, base)
			}
		}

		entries := make([]data.CgEntry, 0, len(bases))
		variants := make([]data.CgVariant, 0, len(bases)*4)
		for _, base := range bases {
			built := fallbackSceneVariants(base, byBase[base])
			// 剧本探测到的初始构图优先作为第一个变体：它是原引擎实际显示
			// 的画面（含多人场景的全部角色部件），比按图名猜的构图完整。
			if cover := s.sceneCovers[base]; len(cover) > 0 {
				built = prependCoverVariant(cover, built)
			}
			if len(built) == 0 {
				continue
			}
			entryIndex := len(entries)
			entry := data.CgEntry{Index: entryIndex}
			for variantIndex, images := range built {
				item := data.CgVariant{Entry: entryIndex, Variant: variantIndex, Images: images}
				entry.Variants = append(entry.Variants, item)
				variants = append(variants, item)
				for _, image := range images {
					if usedSeen[image] {
						continue
					}
					usedSeen[image] = true
					usedNames = append(usedNames, image)
				}
			}
			entries = append(entries, entry)
		}
		groups[group] = data.CgGroup{Index: group, Variants: variants, Entries: entries}
	}

	core.Logf("catalog: rebuilt cg table from resources: %d group(s), %d name(s)",
		len(groups), len(usedNames))
	return &data.CgCatalog{Groups: groups, Names: usedNames}
}

// fallbackReplayLabel 找这个场景在脚本里的回放标签。
//
// 多数场景两种写法之一存在：L01_001_replay（标准回放入口）或 L01_001。
// 两个都没有时退回图名，至少缩略图还能显示，只是点击不能回放。
func (s *Session) fallbackReplayLabel(group, index int, base string) string {
	if s.script != nil {
		for _, name := range []string{
			fmt.Sprintf("L%02d_%03d_replay", group, index),
			fmt.Sprintf("L%02d_%03d", group, index),
		} {
			if s.script.FindLabel(name) >= 0 {
				return name
			}
		}
	}
	return base
}

// fallbackBackgroundNames 从图名里挑"立ち絵背景"候选。
//
// 两代作品的命名不同：侵蝕/淫獄の放課後2 是 bg + 5 位编号 + 时段(a/b) + 差分(b/c/d)，
// 例如 bg01001ab；SLIE 只有 bg + 4 位编号 + 一个差分字母，例如 bg0100b。
// KDIE（BSXScript 3.2，无 scene.dat）更旧：bg + 4 位编号、没有时段/差分后缀，
// 例如 bg0011、bg0012。它们是不同的背景（同一地点的不同镜头），不能像带后缀的
// 变体那样折叠成一个地点，否则鉴赏室只剩 20 张背景、玩家看不到其余镜头。
//
// 立ち絵鉴赏室必须有一张默认背景，否则人物会站在纯黑画面上；带后缀的命名按
// 地点去重，有 a（白天）版本就优先它，没有则保留第一张；无后缀的名字各自独立。
func fallbackBackgroundNames(names []string) []string {
	picked := make(map[string]string, 64)
	for _, name := range names {
		base, suffix, ok := parseBackgroundName(name)
		if !ok {
			continue
		}
		if existing, found := picked[base]; found {
			// suffix 为空表示"这个名字本身就是完整背景"，base 也是完整名字，
			// 正常不会与带后缀的 base 相同；保留现有 a 后缀优先规则即可。
			if strings.HasPrefix(existing[len(base):], "a") || !strings.HasPrefix(suffix, "a") {
				continue
			}
		}
		picked[base] = name
	}
	result := make([]string, 0, len(picked))
	for _, name := range picked {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// parseBackgroundName 把背景图名拆成"地点 base"与"变体后缀"。
//
// 返回的 base 是去重键：带后缀的命名把后缀去掉，无后缀的命名用完整名字。
// 这样 bg01001ab 与 bg01001ac 会合并成一个地点，而 bg0011 与 bg0012
// （KDIE 的不同镜头）会各自保留。长度检查先做，避免 name[2:6] 越界。
func parseBackgroundName(name string) (base, suffix string, ok bool) {
	// Bishop 有两种背景前缀：多数作品是 bg（bg01001ab），牝教師2 这一代
	// 的 [main window] bg signature 写的是 gb（gb00001a）。只认 bg 会让
	// 这一代的鉴赏室候选为空，立ち絵页打开就是纯黑背景。
	prefixLen := 0
	switch {
	case strings.HasPrefix(name, "bg"), strings.HasPrefix(name, "gb"):
		prefixLen = 2
	default:
		return "", "", false
	}
	prefix := name[:prefixLen]
	rest := name[prefixLen:]
	// 无后缀：4 位编号（KDIE 的 bg0011）或 5 位编号（bg01001）。
	// 5 位名字与"4 位编号+1 位差分字母"同长，必须先看剩余部分是不是纯数字，
	// 否则 bg01001 会被误拆成 base=bg0100。
	if (len(rest) == 4 || len(rest) == 5) && allDigits(rest) {
		return name, "", true
	}
	// 5 位编号 + 1/2 位字母（bg01001b / bg01001ab / gb00001a）。
	if (len(rest) == 6 || len(rest) == 7) && allDigits(rest[:5]) && allLetters(rest[5:]) {
		return prefix + rest[:5], rest[5:], true
	}
	// 4 位编号 + 1 位字母（bg0100b）。
	if len(rest) == 5 && allDigits(rest[:4]) && allLetters(rest[4:]) {
		return prefix + rest[:4], rest[4:], true
	}
	// mros 的背景命名：5 位编号 + a/b + 1 位差分字母 + 1 位时间/光线号，
	// 例如 bg01001aa6。前 7 位是地点，后面 3 位是同一地点的差分；不合并
	// 这些差分的话，鉴赏室的背景候选为空，立ち絵页会退化成纯黑背景。
	// [main window] 的 bg signature 写的就是这一族命名。
	if len(rest) == 8 && allDigits(rest[:5]) &&
		(rest[5] == 'a' || rest[5] == 'b') && allLetters(rest[6:7]) && allDigits(rest[7:8]) {
		return prefix + rest[:5], rest[5:], true
	}
	return "", "", false
}

// allLetters 判断字符串是否全为 ASCII 字母（背景变体后缀 a/b/c…）。
func allLetters(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') {
			return false
		}
	}
	return true
}

// fallbackSceneVariants 生成一个场景的若干 CG 组合。
//
// 关键：层号不是"同时叠加的多个人物"，而是同一场景里不同角色的差分。
// 原引擎的 cg.dat 也是按层分别列变体（一个变体只含一层的 p/f 图），
// 把 1p/2p/3p 叠在一起会得到原版里根本不存在的多人合成图，封面自然全错。
//
// 每一层内按姿势配对表情：表情号从该层最小的号码开始递增（ev01 是 100+，
// ev00_007 是 200+），而不是固定 100。
func fallbackSceneVariants(base string, names []string) [][]string {
	layers := map[int]*fallbackLayerImages{}
	for _, name := range names {
		layer, kind, number, ok := fallbackLayerPart(base, name)
		if !ok {
			continue
		}
		images := layers[layer]
		if images == nil {
			images = &fallbackLayerImages{poses: map[string]string{}, faces: map[string]string{}}
			layers[layer] = images
		}
		switch kind {
		case 'p':
			images.poses[number] = name
		case 'f':
			images.faces[number] = name
		}
	}
	if len(layers) == 0 {
		return nil
	}

	order := make([]int, 0, len(layers))
	for layer := range layers {
		order = append(order, layer)
	}
	sort.Ints(order)

	variants := make([][]string, 0, fallbackMaxVariants)
	seen := map[string]bool{}
	appendVariant := func(images []string) {
		if len(images) == 0 || len(variants) >= fallbackMaxVariants {
			return
		}
		key := strings.Join(images, "|")
		if seen[key] {
			return
		}
		seen[key] = true
		variants = append(variants, images)
	}
	for _, layer := range order {
		spec := layers[layer]
		if spec == nil || len(spec.poses) == 0 {
			continue
		}
		minFace, hasFace := fallbackMinFace(spec.faces)
		poseKeys := make([]string, 0, len(spec.poses))
		for key := range spec.poses {
			poseKeys = append(poseKeys, key)
		}
		sort.Strings(poseKeys)
		for _, poseKey := range poseKeys {
			images := []string{spec.poses[poseKey]}
			if hasFace {
				offset, _ := strconv.Atoi(poseKey)
				face := spec.faces[strconv.Itoa(minFace+offset)]
				if face == "" {
					face = spec.faces[strconv.Itoa(minFace)]
				}
				if face != "" {
					images = append(images, face)
				}
			}
			appendVariant(images)
		}
	}
	return variants
}

// prependCoverVariant 把剧本探测到的初始构图放到变体表第一位。
//
// 同一个场景的封面和"按命名规则拼出的变体"常常是同一组图（顺序可能不同），
// 这里按集合去重，避免查看器里出现两张一模一样的第一个变体。
func prependCoverVariant(cover []string, variants [][]string) [][]string {
	if len(cover) == 0 {
		return variants
	}
	out := make([][]string, 0, len(variants)+1)
	out = append(out, append([]string(nil), cover...))
	for _, variant := range variants {
		if sameImageSet(variant, cover) {
			continue
		}
		out = append(out, variant)
	}
	return out
}

func sameImageSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	used := make(map[string]int, len(a))
	for _, name := range a {
		used[name]++
	}
	for _, name := range b {
		if used[name] == 0 {
			return false
		}
		used[name]--
	}
	return true
}

// fallbackMinFace 找表情表里最小的号码（100/200/…）作为配对基准。
func fallbackMinFace(faces map[string]string) (int, bool) {
	best := -1
	for key := range faces {
		value, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		if best < 0 || value < best {
			best = value
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// fallbackLayerPart 拆出 "ev01_001_2p00" → (2, 'p', "00", true)。
//
// 只接受四位以上的部件名，并跳过 p00-02-05t1 这类合成容器名：
// 它们不是独立的身体/表情图，放进鉴赏变体只会得到错位的图层。
func fallbackLayerPart(base, name string) (int, byte, string, bool) {
	prefix := base + "_"
	if !strings.HasPrefix(name, prefix) {
		return 0, 0, "", false
	}
	rest := name[len(prefix):]
	if len(rest) < 3 {
		return 0, 0, "", false
	}
	layer := int(rest[0] - '0')
	if layer < 1 || layer > fallbackMaxLayers {
		return 0, 0, "", false
	}
	kind := rest[1]
	if kind != 'p' && kind != 'f' {
		return 0, 0, "", false
	}
	number := rest[2:]
	digits := 0
	for digits < len(number) && number[digits] >= '0' && number[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, 0, "", false
	}
	// 只要纯数字的基础部件（1p00 / 1f100）。p02c、p01m、p00_nohair 这些
	// 是叠加差分/无头发版本，混进目录会让不同图层拼错，直接略过。
	if digits != len(number) {
		return 0, 0, "", false
	}
	return layer, kind, number[:digits], true
}

// evSceneBase 判断图名是否属于 EV 场景并拆分 ev01_001。
func evSceneBase(name string) (string, int, int, bool) {
	if len(name) < 8 || !strings.HasPrefix(name, "ev") || name[4] != '_' {
		return "", 0, 0, false
	}
	if !allDigits(name[2:4]) || !allDigits(name[5:8]) {
		return "", 0, 0, false
	}
	if len(name) > 8 && name[8] != '_' {
		return "", 0, 0, false
	}
	group, _ := strconv.Atoi(name[2:4])
	index, _ := strconv.Atoi(name[5:8])
	return name[:8], group, index, true
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
