package vm

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kizuna/engine/brand/emon/script"
	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const maxScriptCacheEntries = 512

type ScriptLoader struct {
	RootDirectory string

	archives EmonArchives
	cache    map[string]*script.EmonProgram
	bodies   map[string]struct{}
	order    []string
	gate     sync.Mutex
}

func NewScriptLoader(archives EmonArchives, rootDirectory string) *ScriptLoader {
	return &ScriptLoader{
		RootDirectory: rootDirectory,
		archives:      archives,
		cache:         make(map[string]*script.EmonProgram),
		bodies:        make(map[string]struct{}),
	}
}

func NormalizeScriptName(fileName string) string {
	if fileName == "" {
		return ""
	}
	trimmed := strings.Trim(strings.TrimSpace(fileName), "\"")
	normalized := strings.ReplaceAll(trimmed, "/", "\\")
	return strings.TrimLeft(normalized, "\\")
}

func (l *ScriptLoader) Count() int {
	l.gate.Lock()
	defer l.gate.Unlock()
	return len(l.cache)
}

func (l *ScriptLoader) Load(fileName string) *script.EmonProgram {
	name := NormalizeScriptName(fileName)
	if name == "" {
		core.Logf("emon: script name is empty")
		return nil
	}
	l.gate.Lock()
	if cached, exists := l.lookupLocked(name); exists {
		l.touchLocked(name)
		l.gate.Unlock()
		return cached
	}
	l.gate.Unlock()
	content := l.readBytes(name)
	if len(content) == 0 {
		core.Logf("emon: script '%s' was not found", name)
		return nil
	}
	program := script.ParseBytes(name, content)
	l.store(name, program)
	return program
}

func (l *ScriptLoader) ParseBody(name string, body string) *script.EmonProgram {
	program := script.Parse(name, body)
	l.store(name, program)
	return program
}

func (l *ScriptLoader) Contains(fileName string) bool {
	name := NormalizeScriptName(fileName)
	if name == "" {
		return false
	}
	l.gate.Lock()
	_, inCache := l.lookupLocked(name)
	_, inBodies := l.bodies[strings.ToUpper(name)]
	l.gate.Unlock()
	if inCache || inBodies {
		return true
	}
	if l.archives == nil {
		return l.looseFileExists(name)
	}
	return l.archives.Contains(name) || l.looseFileExists(name)
}

func (l *ScriptLoader) readBytes(name string) []byte {
	if l.archives != nil {
		if content := l.archives.Read(name); len(content) != 0 {
			return content
		}
	}
	if l.RootDirectory == "" {
		return nil
	}
	for _, candidate := range scriptCandidates(name) {
		content, err := os.ReadFile(filepath.Join(l.RootDirectory, candidate))
		if err != nil {
			core.Logf("emon: script '%s' read failed: %s", candidate, err.Error())
			continue
		}
		return content
	}
	return nil
}

func (l *ScriptLoader) looseFileExists(name string) bool {
	if l.RootDirectory == "" {
		return false
	}
	for _, candidate := range scriptCandidates(name) {
		if _, err := os.Stat(filepath.Join(l.RootDirectory, candidate)); err != nil {
			core.Logf("emon: script '%s' probe failed: %s", candidate, err.Error())
			continue
		}
		return true
	}
	return false
}

func scriptCandidates(name string) []string {
	if strutil.HasSuffixFold(name, ".txt") {
		return []string{name}
	}
	return []string{name, name + ".txt"}
}

func (l *ScriptLoader) store(key string, program *script.EmonProgram) {
	l.gate.Lock()
	defer l.gate.Unlock()
	if len(l.cache) >= maxScriptCacheEntries {
		if _, exists := l.lookupLocked(key); !exists {
			l.evictOldestLocked()
		}
	}
	l.cache[key] = program
	if l.indexOfLocked(key) < 0 {
		l.order = append(l.order, key)
	}
}

func (l *ScriptLoader) evictOldestLocked() {
	for len(l.cache) >= maxScriptCacheEntries && len(l.order) != 0 {
		oldest := l.order[0]
		l.order = l.order[1:]
		delete(l.cache, oldest)
		delete(l.bodies, strings.ToUpper(oldest))
	}
}

func (l *ScriptLoader) touchLocked(key string) {
	if index := l.indexOfLocked(key); index >= 0 {
		l.order = append(l.order[:index], l.order[index+1:]...)
	}
	l.order = append(l.order, key)
}

func (l *ScriptLoader) indexOfLocked(key string) int {
	for i, existing := range l.order {
		if strings.EqualFold(existing, key) {
			return i
		}
	}
	return -1
}

func (l *ScriptLoader) lookupLocked(key string) (*script.EmonProgram, bool) {
	if program, exists := l.cache[key]; exists {
		return program, true
	}
	for name, program := range l.cache {
		if strings.EqualFold(name, key) {
			return program, true
		}
	}
	return nil, false
}
