package config

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf16"

	"kizuna/engine/core/textcodec"
)

type IniEntry struct {
	Key   string
	Value string
}

type iniSection struct {
	name    string
	entries []IniEntry
}

type IniDocument struct {
	sections []*iniSection
}

var emptyIniDocument = &IniDocument{sections: []*iniSection{{}}}

func EmptyIniDocument() *IniDocument {
	return emptyIniDocument
}

func ParseIniDocument(data []byte) *IniDocument {
	return ParseIniDocumentText(decodeIniText(data))
}

// ParseIniDocumentText 解析已经解码好的 UTF-8 文本。
//
// 单独暴露文本入口，是因为并非所有 INI 都是 CP932：汉化/移植补丁可能把
// 配置文件存成无 BOM 的 UTF-8，直接按 CP932 解码会得到乱码。调用方可以
// 先用 utf8.Valid 判断编码，再走这里。
func ParseIniDocumentText(text string) *IniDocument {
	document := &IniDocument{sections: []*iniSection{{}}}
	section := document.sections[0]

	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if len(line) == 0 || line[0] == ';' || line[0] == '#' {
			continue
		}

		if line[0] == '[' {
			if close := strings.IndexByte(line, ']'); close > 0 {
				section = document.sectionOrCreate(strings.TrimSpace(line[1:close]))
			}
			continue
		}

		separator := strings.IndexByte(line, '=')
		if separator <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:separator])
		if len(key) == 0 {
			continue
		}

		section.set(key, strings.TrimSpace(stripComment(line[separator+1:])))
	}

	return document
}

func LoadIniDocument(path string) (*IniDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseIniDocument(data), nil
}

func (d *IniDocument) Section(name string) []IniEntry {
	for _, candidate := range d.sections {
		if strings.EqualFold(candidate.name, name) {
			return candidate.entries
		}
	}

	return nil
}

func (d *IniDocument) Get(section, key, fallback string) string {
	for _, candidate := range d.sections {
		if !strings.EqualFold(candidate.name, section) {
			continue
		}

		for _, entry := range candidate.entries {
			if strings.EqualFold(entry.Key, key) {
				return entry.Value
			}
		}

		return fallback
	}

	return fallback
}

func (d *IniDocument) GetInt(section, key string, fallback int) int {
	if value, ok := parseInt(d.Get(section, key, "")); ok {
		return value
	}

	return fallback
}

func (d *IniDocument) GetDouble(section, key string, fallback float64) float64 {
	if value, ok := parseFloat(d.Get(section, key, "")); ok {
		return value
	}

	return fallback
}

func (d *IniDocument) Has(section, key string) bool {
	return len(d.Get(section, key, "")) > 0
}

func (d *IniDocument) sectionOrCreate(name string) *iniSection {
	for _, candidate := range d.sections {
		if strings.EqualFold(candidate.name, name) {
			return candidate
		}
	}

	section := &iniSection{name: name}
	d.sections = append(d.sections, section)

	return section
}

func (s *iniSection) set(key, value string) {
	for index := range s.entries {
		if strings.EqualFold(s.entries[index].Key, key) {
			s.entries[index].Value = value
			return
		}
	}

	s.entries = append(s.entries, IniEntry{Key: key, Value: value})
}

func stripComment(value string) string {
	if index := strings.IndexByte(value, ';'); index >= 0 {
		return value[:index]
	}

	return value
}

func parseInt(value string) (int, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, false
	}

	return int(parsed), true
}

func parseFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}

	return parsed, true
}

func decodeIniText(data []byte) string {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return decodeUTF16Text(data[2:], false)
	}

	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		return decodeUTF16Text(data[2:], true)
	}

	return textcodec.DecodeCP932(data)
}

func decodeUTF16Text(data []byte, bigEndian bool) string {
	if !bigEndian {
		return textcodec.DecodeUTF16LE(data)
	}

	units := make([]uint16, 0, len(data)/2)
	index := 0
	for ; index+1 < len(data); index += 2 {
		units = append(units, uint16(data[index])<<8|uint16(data[index+1]))
	}

	runes := utf16.Decode(units)
	if index < len(data) {
		runes = append(runes, 0xFFFD)
	}

	return string(runes)
}
