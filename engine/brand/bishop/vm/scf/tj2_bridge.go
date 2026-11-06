package scf

import "kizuna/engine/brand/bishop/vm/tj2"

type tj2ScfFile struct {
	file    *ScfScriptFile
	entries []tj2.ScfEntry
}

var _ tj2.ScfFile = (*tj2ScfFile)(nil)

func (f *ScfScriptFile) AsTj2File() tj2.ScfFile {
	entries := make([]tj2.ScfEntry, len(f.entries))
	for i := range entries {
		entries[i] = tj2.ScfEntry{
			Head:     f.entries[i].Head,
			Next:     f.entries[i].Next,
			Variable: f.entries[i].Variable,
		}
	}
	return &tj2ScfFile{file: f, entries: entries}
}

func (f *tj2ScfFile) Code() []byte {
	return f.file.Code()
}

func (f *tj2ScfFile) Data() []byte {
	return f.file.Data()
}

func (f *tj2ScfFile) EntryCount() int {
	return f.file.EntryCount()
}

func (f *tj2ScfFile) Entries() []tj2.ScfEntry {
	return f.entries
}

func (f *tj2ScfFile) PoolOffset(index int) int {
	return f.file.PoolOffset(index)
}

func (f *tj2ScfFile) PoolText(offset int) (string, error) {
	return f.file.PoolText(offset)
}

func (f *tj2ScfFile) Match(transfer int) byte {
	return f.file.Match(transfer)
}

func (f *tj2ScfFile) Set(transfer int) byte {
	return f.file.Set(transfer)
}

func (f *tj2ScfFile) TransferBase(transfer int) int {
	return f.file.TransferBase(transfer)
}

func (f *tj2ScfFile) TransferCount() int {
	return f.file.TransferCount()
}

func (f *tj2ScfFile) JumpTargetCount() int {
	return f.file.JumpTargetCount()
}

func (f *tj2ScfFile) JumpTarget(index int) int {
	return f.file.JumpTarget(index)
}
