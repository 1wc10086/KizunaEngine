package save

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"kizuna/engine/brand/emon/emecfg"
	"kizuna/engine/core"
	"kizuna/engine/library"
)

const MaxSlot = 999

var slotPattern = regexp.MustCompile(`(?i)^Save(\d{3})\.dat$`)

type EmonSaveStore struct {
	directory string
	magic     []byte
	system    *EmonSystemStateStore
}

func NewEmonSaveStore(directory string) *EmonSaveStore {
	return &EmonSaveStore{
		directory: directory,
		magic:     resolveMagic(directory),
		system:    NewEmonSystemStateStore(directory),
	}
}

func (s *EmonSaveStore) DirectoryPath() string {
	return s.directory
}

func (s *EmonSaveStore) SystemState() *EmonSystemStateStore {
	return s.system
}

func DefaultDirectory(dataDirectory string) string {
	return filepath.Join(dataDirectory, SaveDirectoryName)
}

func (s *EmonSaveStore) Read(slot int) *EmonSaveData {
	if slot < 0 || slot > MaxSlot {
		core.Logf("emon-save: slot %d out of range, ignoring read", slot)
		return nil
	}
	path := s.PathFor(slot)
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return s.readContainer(slot, path)
}

func (s *EmonSaveStore) Write(data *EmonSaveData) bool {
	if data == nil {
		return false
	}
	if data.Slot < 0 || data.Slot > MaxSlot {
		core.Logf("emon-save: slot %d out of range, ignoring write", data.Slot)
		return false
	}
	path := s.PathFor(int(data.Slot))
	payload := data.ToPayload()
	payload.Slot = data.Slot
	body, err := MarshalSavePayload(payload)
	if err != nil {
		core.Logf("emon-save: slot %d write failed: %s", data.Slot, describeError(err))
		return false
	}
	temporary := path + ".tmp"
	file := Pack(s.magic, body, true)
	if err := os.MkdirAll(s.directory, 0755); err != nil {
		core.Logf("emon-save: slot %d write failed: %s", data.Slot, describeError(err))
		return false
	}
	if err := os.WriteFile(temporary, file, 0644); err != nil {
		tryDeleteFile(temporary)
		core.Logf("emon-save: slot %d write failed: %s", data.Slot, describeError(err))
		return false
	}
	return moveOrCopyFile(temporary, path, int(data.Slot))
}

func (s *EmonSaveStore) Exists(slot int) bool {
	if slot < 0 || slot > MaxSlot {
		return false
	}
	_, err := os.Stat(s.PathFor(slot))
	return err == nil
}

func (s *EmonSaveStore) Delete(slot int) bool {
	if slot < 0 || slot > MaxSlot {
		return false
	}
	path := s.PathFor(slot)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	if err := os.Remove(path); err != nil {
		core.Logf("emon-save: slot %d delete failed: %s", slot, describeError(err))
		return false
	}
	return true
}

func (s *EmonSaveStore) Slots() []int {
	found := map[int]struct{}{}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return []int{}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := slotPattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		slot, err := strconv.Atoi(match[1])
		if err != nil || slot < 0 || slot > MaxSlot {
			continue
		}
		found[slot] = struct{}{}
	}
	result := make([]int, 0, len(found))
	for slot := range found {
		result = append(result, slot)
	}
	sort.Ints(result)
	return result
}

func (s *EmonSaveStore) PathFor(slot int) string {
	return SlotPath(s.directory, slot)
}

func (s *EmonSaveStore) readContainer(slot int, path string) *EmonSaveData {
	file, err := os.ReadFile(path)
	if err != nil {
		core.Logf("emon-save: slot %d read failed: %s", slot, describeError(err))
		return nil
	}
	body, reason, ok := TryUnpack(file, s.magic, true)
	if !ok {
		return rejectSave(path, reason)
	}
	payload, err := UnmarshalSavePayload(body)
	if err != nil {
		core.Logf("emon-save: slot %d save payload is not a KizunaEngine save, treating as absent", slot)
		return nil
	}
	if payload == nil {
		return rejectSave(path, "empty payload")
	}
	payload.Slot = int32(slot)
	return EmonSaveDataFromPayload(payload)
}

func resolveMagic(directory string) []byte {
	if len(directory) == 0 {
		return SaveFileMagic("")
	}
	game := filepath.Dir(directory)
	located, ok := library.FindFile(game, emecfg.ConfigurationFileName)
	if !ok {
		return SaveFileMagic("")
	}
	config, err := emecfg.LoadEmecfgFile(located)
	if err != nil {
		core.Logf("emon-save: %s magic read failed: %s", emecfg.ConfigurationFileName, describeError(err))
		return SaveFileMagic("")
	}
	if config == nil {
		return SaveFileMagic("")
	}
	return SaveFileMagic(config.SaveFileKey)
}

func moveOrCopyFile(temporary string, destination string, slot int) bool {
	if err := os.Rename(temporary, destination); err == nil {
		return true
	} else {
		body, readErr := os.ReadFile(temporary)
		if readErr != nil {
			tryDeleteFile(temporary)
			core.Logf("emon-save: slot %d write failed: %s", slot, describeError(readErr))
			return false
		}
		if writeErr := os.WriteFile(destination, body, 0644); writeErr != nil {
			tryDeleteFile(temporary)
			core.Logf("emon-save: slot %d write failed: %s", slot, describeError(writeErr))
			return false
		}
		tryDeleteFile(temporary)
		core.Logf("emon-save: slot %d rename failed, copied instead: %s", slot, describeError(err))
		return true
	}
}

func tryDeleteFile(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	_ = os.Remove(path)
}

func rejectSave(path string, reason string) *EmonSaveData {
	core.Logf("emon-save: ignoring malformed save '%s': %s", path, reason)
	return nil
}

func describeError(err error) string {
	if err == nil {
		return ""
	}
	message := ""
	depth := 0
	for current := err; current != nil; current = unwrapError(current) {
		if depth > 0 {
			message += " <= "
		}
		message += fmt.Sprintf("%T: %s", current, current.Error())
		depth++
		if depth >= 8 {
			break
		}
	}
	return message
}

type unwrapper interface {
	Unwrap() error
}

func unwrapError(err error) error {
	if wrapped, ok := err.(unwrapper); ok {
		return wrapped.Unwrap()
	}
	return nil
}
