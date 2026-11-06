package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/brand/emon/emecfg"
	"kizuna/engine/core"
)

type brand struct{}

func init() {
	core.Register(brand{})
}

func (brand) ID() string {
	return "emon"
}

func (brand) Name() string {
	return "EmonEngine"
}

func (brand) Detect(dir string) bool {
	return detect(dir)
}

func (brand) New() core.Engine {
	return New()
}

func detect(root string) bool {
	if root == "" {
		return false
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	for _, directory := range descend(root, 4) {
		children, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, child := range children {
			if !child.IsDir() && strings.EqualFold(child.Name(), emecfg.ConfigurationFileName) {
				return true
			}
		}
	}
	return false
}

func descend(root string, maxDepth int) []string {
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
		if current.depth >= maxDepth {
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
