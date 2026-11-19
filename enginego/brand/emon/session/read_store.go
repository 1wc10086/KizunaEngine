package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"kizuna/enginego/core"
)

// emonReadStore 是既読/未読数据库。
//
// 原引擎用一套“已读行”标志驱动 SkipModeFlag 0x10（既読アイコン）和
// 「未読文章もスキップする」的停点。键取“脚本名:源行号”：译文/脚本补丁会改
// 正文，但行号稳定；循环重放同一行时也仍能命中。存盘放在游戏 SAVEDATA 下
// （与 emesys.dat 同级），这样换存档、重启应用都保留；写不进去（只读目录）
// 时退化为纯内存，不影响游玩。
type emonReadStore struct {
	mu      sync.Mutex
	path    string
	files   map[string]map[int]struct{}
	pending int
	closed  bool
}

const (
	readStoreVersion    = 1
	readStoreFlushEvery = 64
	readStoreFileName   = "KizunaRead.dat"
)

type readStoreFile struct {
	Version int              `json:"version"`
	Files   map[string][]int `json:"files"`
}

func newEmonReadStore(directory string) *emonReadStore {
	store := &emonReadStore{files: map[string]map[int]struct{}{}}
	if strings.TrimSpace(directory) == "" {
		return store
	}
	store.path = filepath.Join(directory, readStoreFileName)
	data, err := os.ReadFile(store.path)
	if err != nil {
		if !os.IsNotExist(err) {
			core.Logf("emon-read: read '%s' failed: %v", store.path, err)
		}
		return store
	}
	var file readStoreFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != readStoreVersion {
		core.Logf("emon-read: ignoring malformed read data '%s'", store.path)
		return store
	}
	for name, lines := range file.Files {
		set := map[int]struct{}{}
		for _, line := range lines {
			set[line] = struct{}{}
		}
		store.files[name] = set
	}
	return store
}

func (s *emonReadStore) IsRead(key string) bool {
	name, line, ok := splitReadKey(key)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set, exists := s.files[name]
	if !exists {
		return false
	}
	_, read := set[line]
	return read
}

func (s *emonReadStore) MarkRead(key string) {
	name, line, ok := splitReadKey(key)
	if !ok {
		return
	}
	s.mu.Lock()
	set, exists := s.files[name]
	if !exists {
		set = map[int]struct{}{}
		s.files[name] = set
	}
	if _, read := set[line]; read {
		s.mu.Unlock()
		return
	}
	set[line] = struct{}{}
	s.pending++
	needFlush := s.pending >= readStoreFlushEvery
	path := s.path
	if needFlush {
		s.pending = 0
	}
	s.mu.Unlock()
	if needFlush && path != "" {
		s.Flush()
	}
}

func (s *emonReadStore) Flush() {
	if s == nil {
		return
	}
	s.mu.Lock()
	path := s.path
	if path == "" || s.closed {
		s.mu.Unlock()
		return
	}
	s.pending = 0
	file := readStoreFile{Version: readStoreVersion, Files: map[string][]int{}}
	for name, set := range s.files {
		lines := make([]int, 0, len(set))
		for line := range set {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		file.Files[name] = lines
	}
	s.mu.Unlock()
	body, err := json.Marshal(file)
	if err != nil {
		core.Logf("emon-read: marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		core.Logf("emon-read: create directory failed: %v", err)
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o644); err != nil {
		core.Logf("emon-read: write '%s' failed: %v", temporary, err)
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		core.Logf("emon-read: commit '%s' failed: %v", path, err)
	}
}

func (s *emonReadStore) Close() {
	if s == nil {
		return
	}
	s.Flush()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func splitReadKey(key string) (string, int, bool) {
	idx := strings.LastIndex(key, ":")
	if idx <= 0 || idx == len(key)-1 {
		return "", 0, false
	}
	line, err := strconv.Atoi(key[idx+1:])
	if err != nil {
		return "", 0, false
	}
	return key[:idx], line, true
}
