package gothic

import (
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/core"
)

type gothicBrand struct{}

func init() {
	core.Register(gothicBrand{})
}

func (gothicBrand) ID() string {
	return "bishop-gothic"
}

func (gothicBrand) Name() string {
	return "Bishop Gothic"
}

func (gothicBrand) Detect(dir string) bool {
	return DetectGothicDataDir(dir) != ""
}

func (gothicBrand) New() core.Engine {
	return NewSession()
}

func DetectGothicDataDir(dir string) string {
	if dir == "" {
		return ""
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	type visit struct {
		directory string
		depth     int
	}
	queue := []visit{{directory: dir, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(current.directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.EqualFold(entry.Name(), "MAINCON.SCS") {
				return current.directory
			}
		}
		if current.depth >= 4 {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				queue = append(queue, visit{directory: filepath.Join(current.directory, entry.Name()), depth: current.depth + 1})
			}
		}
	}
	return ""
}
