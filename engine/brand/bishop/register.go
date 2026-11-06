package bishop

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/brand/bishop/session"
	"kizuna/engine/core"
)

type brand struct{}

func init() {
	core.Register(brand{})
}

func (brand) ID() string {
	return "bishop"
}

func (brand) Name() string {
	return "Bishop"
}

func (brand) Detect(dir string) bool {
	return detect(dir)
}

func (brand) New() core.Engine {
	return session.New()
}

var scriptNames = []string{"bsxx.dat", "bss.dat", "bsx.dat"}

func detect(root string) bool {
	if root == "" {
		return false
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	for _, directory := range descend(root, 6) {
		if hasAnyFold(directory, scriptNames) && !excluded(directory) {
			return true
		}
	}
	return false
}

func excluded(directory string) bool {
	children, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, child := range children {
		if child.IsDir() {
			continue
		}
		name := child.Name()
		if strings.EqualFold(name, "SCNFILE.PFT") ||
			strings.EqualFold(name, "SCENE.PFT") ||
			strings.EqualFold(name, "MAINCON.SCS") ||
			strings.EqualFold(name, "emecfg.ecf") {
			return true
		}
		if strings.EqualFold(filepath.Ext(name), ".bdt") {
			return true
		}
	}
	return false
}

func hasAnyFold(directory string, names []string) bool {
	children, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, child := range children {
		if child.IsDir() {
			continue
		}
		for _, name := range names {
			if strings.EqualFold(child.Name(), name) {
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
