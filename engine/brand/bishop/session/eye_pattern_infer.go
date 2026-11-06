package session

import (
	"sort"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
)

// eye_pattern_infer.go 在 bsxx.cfg 没写 [bustup setting] eye pattern 时，
// 从素材本身推断眼部帧条有几帧。
//
// 背景：三射面談（SM）的配置没有 eye pattern 一项，但立绘的眼部条实际是
// 4 帧。旧实现退回默认 6：
//   - 剧情里按 6 切 164px 的条，每帧 27px，第 0 帧只剩眉毛/半只眼睛；
//   - 鉴赏室的整除检查（164%6 != 0）直接放弃切帧，整条 4 行眼睛一起画出来。
// 这正是"眼睛眉毛消失 / 出现很多行"的根因。SMIE（有 eye pattern=4）不出现
// 多行，也反过来印证了这一点。
//
// 推断方式：抽若干张表情叠加图（BSS-Composition 的第二个部件就是眼部条），
// 取它们高度的最大公约数。正常素材的每帧高度一致，所以 GCD 就是帧数；
// 至少 3 个样本才采信，样本不足或 GCD 超出 2..8 时保持配置默认值。

const (
	eyePatternSampleLimit = 24
	eyePatternMinimum     = 2
	eyePatternMaximum     = 8
)

func inferEyePattern(resources *resource.GameResourceIndex) int {
	if resources == nil {
		return 0
	}
	heights := make([]int, 0, eyePatternSampleLimit)
	for _, name := range eyePatternSampleNames(resources) {
		location, ok := resources.TryGet(name)
		if !ok {
			continue
		}
		sizes, ok := gfx.BsgCompositionPartSizes(location.Read())
		if !ok || len(sizes) < 2 {
			continue
		}
		// 第 1 个部件是眼睛/眉毛帧条；第 0 个是脸型底图，高度不能参与 GCD。
		height := sizes[1][1]
		if height > 1 {
			heights = append(heights, height)
		}
		if len(heights) >= eyePatternSampleLimit {
			break
		}
	}
	if len(heights) < 3 {
		return 0
	}
	divisor := 0
	for _, height := range heights {
		divisor = gcdInt(divisor, height)
	}
	if divisor < eyePatternMinimum || divisor > eyePatternMaximum {
		return 0
	}
	return divisor
}

// eyePatternSampleNames 从资源索引里挑出表情叠加图的基名。
//
// 路径里带 /face/ 的条目最可靠（SM/SMIE 都是 06/face/bu06f0001a03.bsg）。
// 旧作或扁平目录没有该路径段时退回"buNN[fu]…"的命名约定，避免一部作品
// 因为打包目录不同就推断不出帧数。
func eyePatternSampleNames(resources *resource.GameResourceIndex) []string {
	seen := map[string]struct{}{}
	names := []string{}
scan:
	for _, archive := range resources.Archives() {
		for _, entry := range archive.Entries() {
			base := entry.Path
			if slash := strings.LastIndexAny(base, "\\/"); slash >= 0 {
				base = base[slash+1:]
			}
			if dot := strings.LastIndexByte(base, '.'); dot > 0 {
				base = base[:dot]
			}
			lower := strings.ToLower(base)
			if !isBustupFaceSample(lower, entry.Path) {
				continue
			}
			if _, duplicate := seen[lower]; duplicate {
				continue
			}
			seen[lower] = struct{}{}
			names = append(names, base)
			if len(names) >= eyePatternSampleLimit {
				break scan
			}
		}
	}
	sort.Strings(names)
	core.Logf("session: eye pattern candidates=%d", len(names))
	return names
}

func isBustupFaceSample(base, path string) bool {
	if len(base) < 10 || !strings.HasPrefix(base, "bu") {
		return false
	}
	if base[2] < '0' || base[2] > '9' || base[3] < '0' || base[3] > '9' {
		return false
	}
	if base[4] != 'f' && base[4] != 'u' {
		return false
	}
	// 表情叠加图的文件名以两位表情号结尾（…a03）；身体图没有这两位。
	if base[len(base)-1] < '0' || base[len(base)-1] > '9' {
		return false
	}
	if base[len(base)-2] < '0' || base[len(base)-2] > '9' {
		return false
	}
	if strings.Contains(strings.ToLower(path), "/face/") {
		return true
	}
	// 扁平目录：只认"表情号非 0"的名字，避免把 bu06f0011a 这类身体图当样本。
	return base[len(base)-2:] != "00"
}

func gcdInt(left, right int) int {
	if left < 0 {
		left = -left
	}
	if right < 0 {
		right = -right
	}
	for right != 0 {
		left, right = right, left%right
	}
	return left
}
