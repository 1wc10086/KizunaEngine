package bdt

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
)

type BdtAudioCatalog struct {
	resources *resource.GameResourceIndex
	// allowVQF 为 true 时把 .vqf 交给宿主（fvp/FFmpeg 解 TwinVQ）。
	// manin 的语音/SE 全是 VQF；AFDVD 仍是容器路线，保持原来的跳过行为，
	// 避免改到已适配作品的音频。
	allowVQF bool
}

func NewBdtAudioCatalog(resources *resource.GameResourceIndex, allowVQF bool) *BdtAudioCatalog {
	return &BdtAudioCatalog{resources: resources, allowVQF: allowVQF}
}

func (c *BdtAudioCatalog) ResolveBgm(track int) (string, bool) {
	if track <= 0 {
		core.Logf("bdt: %s", "bgm track out of range")
		return "", false
	}
	stem := "bgm" + bdtTwoDigits(track)
	return c.resolveKey("bgm", []string{stem, stem + ".ogg", stem + ".wav"})
}

func (c *BdtAudioCatalog) ResolveSe(name string) (string, bool) {
	file := strings.TrimSpace(name)
	if len(file) == 0 {
		core.Logf("bdt: %s", "se name is empty")
		return "", false
	}
	if strings.Contains(file, ".") {
		return c.resolveKey("se", []string{file})
	}
	return c.resolveKey("se", []string{file, file + ".ogg"})
}

func (c *BdtAudioCatalog) ResolveVoice(code string) (string, bool) {
	id := strings.TrimSpace(code)
	if len(id) == 0 {
		core.Logf("bdt: %s", "voice code is empty")
		return "", false
	}
	candidates := []string{id}
	if id[0] != 'v' && id[0] != 'V' && id[0] != 'w' && id[0] != 'W' {
		candidates = append(candidates, "v"+id, "w"+id)
	}
	return c.resolveKey("voice", candidates)
}

func (c *BdtAudioCatalog) resolveKey(kind string, candidates []string) (string, bool) {
	unsupported := false
	for _, candidate := range candidates {
		location, ok := c.resources.TryGet(candidate)
		if !ok {
			continue
		}
		if isBdtTwinVq(location) && !c.allowVQF {
			logBdtTwinVqOnce(kind, candidate, location)
			unsupported = true
			continue
		}
		return candidate, true
	}
	if !unsupported && len(candidates) > 0 {
		core.Logf("bdt: %s '%s' not found", kind, candidates[0])
	}
	return "", false
}

func isBdtTwinVq(location *resource.ResourceLocation) bool {
	if location == nil {
		return false
	}
	if strings.EqualFold(filepath.Ext(location.LooseFile()), ".vqf") {
		return true
	}
	if entry, ok := location.Entry(); ok {
		if strings.EqualFold(filepath.Ext(entry.Path), ".vqf") {
			return true
		}
	}
	return strings.EqualFold(filepath.Ext(location.Name()), ".vqf")
}

var (
	bdtTwinVqGate   sync.Mutex
	bdtTwinVqLogged = map[string]bool{}
)

func logBdtTwinVqOnce(kind, candidate string, location *resource.ResourceLocation) {
	key := location.Key()
	bdtTwinVqGate.Lock()
	dup := bdtTwinVqLogged[key]
	if !dup {
		bdtTwinVqLogged[key] = true
	}
	bdtTwinVqGate.Unlock()
	if dup {
		return
	}
	core.Logf("bdt: %s '%s' is TwinVQ, skipped", kind, candidate)
}

func bdtTwoDigits(value int) string {
	return fmt.Sprintf("%02d", value)
}
