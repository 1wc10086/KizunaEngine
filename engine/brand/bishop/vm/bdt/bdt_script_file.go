package bdt

import (
	"os"
	"strings"
)

type BdtScriptFile struct {
	Name   string
	Length int
	Lines  []BdtLine
	Labels []BdtLine

	labelIndex map[string]int
}

func LoadBdtScript(encrypted []byte) *BdtScriptFile {
	decrypted := make([]byte, len(encrypted))
	for index, value := range encrypted {
		decrypted[index] = ^value
	}
	return bdtScriptFromDecrypted(decrypted, "")
}

func LoadBdtScriptFile(path string) (*BdtScriptFile, error) {
	encrypted, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(encrypted))
	for index, value := range encrypted {
		decrypted[index] = ^value
	}
	return bdtScriptFromDecrypted(decrypted, bdtBaseName(path)), nil
}

func (f *BdtScriptFile) TryGetLabel(id string) (int, bool) {
	index, ok := f.labelIndex[strings.ToUpper(NormalizeBdtLabelId(id))]
	return index, ok
}

func bdtScriptFromDecrypted(decrypted []byte, name string) *BdtScriptFile {
	lines := []BdtLine{}
	labels := []BdtLine{}
	labelIndex := map[string]int{}
	encoding := bdtDetectEncoding(decrypted)
	start := 0
	for position := 0; position <= len(decrypted); position++ {
		if position < len(decrypted) && decrypted[position] != 10 {
			continue
		}
		end := position
		if end > start && decrypted[end-1] == 13 {
			end--
		}
		text := bdtDecodeText(decrypted[start:end], encoding)
		line := BdtLine{Index: len(lines), Offset: start, Kind: ClassifyBdtLine(text), Text: text}
		lines = append(lines, line)
		if line.Kind == BdtLineKindLabel {
			labels = append(labels, line)
			id := ""
			if len(text) > 0 {
				id = text[1:]
			}
			if _, exists := labelIndex[strings.ToUpper(id)]; !exists {
				labelIndex[strings.ToUpper(id)] = line.Index
			}
		}
		start = position + 1
	}
	return &BdtScriptFile{Name: name, Length: len(decrypted), Lines: lines, Labels: labels, labelIndex: labelIndex}
}

func bdtBaseName(path string) string {
	name := path
	if index := strings.LastIndexByte(name, '/'); index >= 0 {
		name = name[index+1:]
	}
	if index := strings.LastIndexByte(name, '\\'); index >= 0 {
		name = name[index+1:]
	}
	return name
}
