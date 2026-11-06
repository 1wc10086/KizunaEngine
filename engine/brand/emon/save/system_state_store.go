package save

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kizuna/engine/brand/emon/script"
	"kizuna/engine/core"
)

const FormatVersion = 1

const systemPollInterval = time.Second
const systemWriteInterval = 5 * time.Second

type EmonSystemStateStore struct {
	mu        sync.Mutex
	path      string
	stamp     uint64
	nextPoll  time.Time
	nextWrite time.Time
	warned    bool
}

func NewEmonSystemStateStore(directory string) *EmonSystemStateStore {
	return &EmonSystemStateStore{path: filepath.Join(directory, SystemFileName)}
}

func (s *EmonSystemStateStore) FilePath() string {
	return s.path
}

func (s *EmonSystemStateStore) Load(variables *script.EmonVariableStore) bool {
	values := s.read()
	if values != nil {
		variables.LoadFrom(values)
	}
	s.mu.Lock()
	s.stamp = stampOf(variables.Entries())
	s.mu.Unlock()
	return values != nil
}

func (s *EmonSystemStateStore) Pump(variables *script.EmonVariableStore) {
	now := time.Now()
	s.mu.Lock()
	if now.Before(s.nextPoll) {
		s.mu.Unlock()
		return
	}
	s.nextPoll = now.Add(systemPollInterval)
	stamp := stampOf(variables.Entries())
	if stamp == s.stamp || now.Before(s.nextWrite) {
		s.mu.Unlock()
		return
	}
	body, ok := trySerializeSystem(variables.Snapshot())
	s.mu.Unlock()
	if !ok {
		return
	}
	if !s.commit(body) {
		return
	}
	s.mu.Lock()
	s.stamp = stamp
	s.nextWrite = time.Now().Add(systemWriteInterval)
	s.mu.Unlock()
}

func (s *EmonSystemStateStore) Write(variables map[string]script.EmonValue) bool {
	body, ok := trySerializeSystem(variables)
	if !ok || !s.commit(body) {
		return false
	}
	s.mu.Lock()
	s.stamp = stampOfMap(variables)
	s.nextWrite = time.Now().Add(systemWriteInterval)
	s.mu.Unlock()
	return true
}

func (s *EmonSystemStateStore) read() map[string]script.EmonValue {
	file, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			s.warn("read failed: " + describeError(err))
		}
		return nil
	}
	payload, reason, ok := TryUnpack(file, SystemMagicBytes(), false)
	if !ok {
		s.warn("malformed system file ignored: " + reason)
		return nil
	}
	if !looksLikeJSON(payload) {
		return s.readLegacy(payload)
	}
	state, err := UnmarshalSystemStatePayload(payload)
	if err != nil {
		s.warn("payload is not a system state block: " + describeError(err))
		return nil
	}
	if state == nil || state.Version != FormatVersion {
		version := int32(-1)
		if state != nil {
			version = state.Version
		}
		s.warn(fmt.Sprintf("payload version %d is not %d", version, FormatVersion))
		return nil
	}
	return state.SystemVariables
}

func (s *EmonSystemStateStore) readLegacy(payload []byte) map[string]script.EmonValue {
	if variables, ok := TryReadLegacySystemFile(payload); ok {
		s.backup()
		core.Logf("emon-save: legacy system file imported variables=%d", len(variables))
		return variables
	}
	s.backup()
	s.warn("payload is neither a system state block nor a legacy system file")
	return nil
}

func looksLikeJSON(payload []byte) bool {
	for _, value := range payload {
		if value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			continue
		}
		return value == '{'
	}
	return false
}

func trySerializeSystem(variables map[string]script.EmonValue) ([]byte, bool) {
	copied := make(map[string]script.EmonValue, len(variables))
	for name, value := range variables {
		copied[name] = value
	}
	body, err := MarshalSystemStatePayload(&EmonSystemStatePayload{
		Version:         FormatVersion,
		SystemVariables: copied,
	})
	if err != nil {
		core.Logf("emon-save: system state serialize failed: %s", describeError(err))
		return nil, false
	}
	return body, true
}

func (s *EmonSystemStateStore) commit(body []byte) bool {
	temporary := s.path + ".tmp"
	directory := filepath.Dir(s.path)
	if len(directory) > 0 {
		if err := os.MkdirAll(directory, 0755); err != nil {
			core.Logf("emon-save: system state write failed: %s", describeError(err))
			return false
		}
	}
	file := Pack(SystemMagicBytes(), body, false)
	if err := os.WriteFile(temporary, file, 0644); err != nil {
		tryDeleteFile(temporary)
		core.Logf("emon-save: system state write failed: %s", describeError(err))
		return false
	}
	if err := os.Rename(temporary, s.path); err == nil {
		return true
	} else {
		body, readErr := os.ReadFile(temporary)
		if readErr != nil {
			tryDeleteFile(temporary)
			core.Logf("emon-save: system state write failed: %s", describeError(readErr))
			return false
		}
		if writeErr := os.WriteFile(s.path, body, 0644); writeErr != nil {
			tryDeleteFile(temporary)
			core.Logf("emon-save: system state write failed: %s", describeError(writeErr))
			return false
		}
		tryDeleteFile(temporary)
		core.Logf("emon-save: system state rename failed, copied instead: %s", describeError(err))
		return true
	}
}

func (s *EmonSystemStateStore) backup() {
	target := s.path + ".legacy"
	if _, err := os.Stat(target); err == nil {
		return
	}
	body, err := os.ReadFile(s.path)
	if err != nil {
		core.Logf("emon-save: legacy system file backup failed: %s", describeError(err))
		return
	}
	if err := os.WriteFile(target, body, 0644); err != nil {
		core.Logf("emon-save: legacy system file backup failed: %s", describeError(err))
	}
}

func stampOf(entries []script.EmonVariableEntry) uint64 {
	hash := fnv.New64a()
	var count uint64
	for _, entry := range entries {
		count++
		hash.Write([]byte(strings.ToLower(entry.Name)))
		hash.Write([]byte{0xFF})
		if entry.Value.IsIdentifier() {
			hash.Write([]byte{2})
			hash.Write([]byte(entry.Value.AsString()))
		} else if entry.Value.IsInteger() {
			hash.Write([]byte{0})
			number := entry.Value.AsInt()
			hash.Write([]byte{byte(number), byte(number >> 8), byte(number >> 16), byte(number >> 24)})
		} else {
			hash.Write([]byte{1})
			hash.Write([]byte(entry.Value.AsString()))
		}
	}
	hash.Write([]byte{byte(count), byte(count >> 8), byte(count >> 16), byte(count >> 24), byte(count >> 32), byte(count >> 40), byte(count >> 48), byte(count >> 56)})
	return hash.Sum64()
}

func stampOfMap(variables map[string]script.EmonValue) uint64 {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]script.EmonVariableEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, script.EmonVariableEntry{Name: name, Value: variables[name]})
	}
	return stampOf(entries)
}

func (s *EmonSystemStateStore) warn(message string) {
	s.mu.Lock()
	if s.warned {
		s.mu.Unlock()
		return
	}
	s.warned = true
	s.mu.Unlock()
	core.Logf("emon-save: system state ignored, defaults kept, %s", message)
}
