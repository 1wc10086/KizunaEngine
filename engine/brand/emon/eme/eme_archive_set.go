package eme

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/core"
)

type emeArchiveSlot struct {
	Archive *EmeArchive
	Entry   *EmeEntry
}

type EmeArchiveSet struct {
	Archives []*EmeArchive
	Names    []string
	index    map[string]emeArchiveSlot
	disposed bool
}

func OpenDirectory(directory string) *EmeArchiveSet {
	set := &EmeArchiveSet{index: make(map[string]emeArchiveSlot)}
	set.load(directory)
	return set
}

func (s *EmeArchiveSet) IsDisposed() bool {
	return s.disposed
}

func (s *EmeArchiveSet) Contains(name string) bool {
	if name == "" {
		return false
	}

	slot, ok := s.resolve(name)
	if !ok {
		return false
	}

	return !slot.Archive.IsDisposed()
}

func (s *EmeArchiveSet) Read(name string) []byte {
	data, ok := s.TryRead(name)
	if !ok {
		return nil
	}

	return data
}

func (s *EmeArchiveSet) TryRead(name string) ([]byte, bool) {
	if name == "" {
		return nil, false
	}

	slot, ok := s.resolve(name)
	if !ok || slot.Archive.IsDisposed() {
		return nil, false
	}

	data, err := slot.Archive.ReadEntry(slot.Entry)
	if err != nil {
		core.Logf("eme: failed to read '%s': %v", name, err)
		return nil, false
	}

	return data, true
}

func (s *EmeArchiveSet) Locate(name string) (*EmeEntry, *EmeArchive) {
	if name == "" {
		return nil, nil
	}

	slot, ok := s.resolve(name)
	if !ok || slot.Archive.IsDisposed() {
		return nil, nil
	}

	return slot.Entry, slot.Archive
}

func (s *EmeArchiveSet) resolve(name string) (emeArchiveSlot, bool) {
	if slot, ok := s.index[foldKey(name)]; ok {
		return slot, true
	}

	for _, candidate := range s.Names {
		if strings.EqualFold(candidate, name) {
			if slot, ok := s.index[foldKey(candidate)]; ok {
				return slot, true
			}
		}
	}

	return emeArchiveSlot{}, false
}

func (s *EmeArchiveSet) Dispose() {
	if s.disposed {
		return
	}

	s.disposed = true
	for _, archive := range s.Archives {
		archive.Dispose()
	}

	s.Archives = nil
	s.index = make(map[string]emeArchiveSlot)
	s.Names = nil
}

func (s *EmeArchiveSet) load(directory string) {
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return
	}

	dirEntries, err := os.ReadDir(directory)
	if err != nil {
		core.Logf("eme: failed to list '%s': %v", directory, err)
		return
	}

	files := make([]string, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		files = append(files, filepath.Join(directory, dirEntry.Name()))
	}

	sort.Slice(files, func(i, j int) bool { return foldKey(files[i]) < foldKey(files[j]) })
	for _, file := range files {
		if !strings.EqualFold(filepath.Ext(file), ".eme") {
			continue
		}

		archive, err := OpenArchive(file)
		if err != nil {
			core.Logf("eme: failed to open '%s': %v", file, err)
			continue
		}

		s.add(archive)
	}
}

func (s *EmeArchiveSet) add(archive *EmeArchive) {
	s.Archives = append(s.Archives, archive)
	for i := range archive.Directory {
		entry := &archive.Directory[i]
		key := foldKey(entry.Name)
		if _, exists := s.index[key]; !exists {
			s.index[key] = emeArchiveSlot{Archive: archive, Entry: entry}
			s.Names = append(s.Names, entry.Name)
		}

		stem := foldKey(stripExtension(entry.Name))
		if _, exists := s.index[stem]; !exists {
			s.index[stem] = emeArchiveSlot{Archive: archive, Entry: entry}
		}
	}
}
