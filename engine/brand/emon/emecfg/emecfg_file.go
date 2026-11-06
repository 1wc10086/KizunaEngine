package emecfg

import (
	"os"
	"strconv"
	"strings"

	"kizuna/engine/brand/emon/script"
)

const (
	defaultStartScript   = "ASMainScript.txt"
	defaultDataDirectory = "data"
	defaultWindowWidth   = 800
	defaultWindowHeight  = 600
)

type EmecfgFile struct {
	SettingsText    string
	Settings        map[string]string
	MacroList       []script.EmonMacroDefinition
	Symbols         map[string]string
	CdRomCheckFiles []string
	StartScript     string
	DataDirectory   string
	SaveFileKey     string
	WindowWidth     int
	WindowHeight    int
}

func ParseEmecfgFile(fileBytes []byte) (*EmecfgFile, error) {
	image, err := DecodeEcfImage(fileBytes)
	if err != nil {
		return nil, err
	}

	return fromImage(image), nil
}

func LoadEmecfgFile(path string) (*EmecfgFile, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, nil
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseEmecfgFile(fileBytes)
}

func newEmecfgFile(
	settingsText string,
	settings map[string]string,
	macros []script.EmonMacroDefinition,
	symbols map[string]string,
	cdRomCheckFiles []string,
) *EmecfgFile {
	config := &EmecfgFile{
		SettingsText:    settingsText,
		Settings:        settings,
		MacroList:       macros,
		Symbols:         symbols,
		CdRomCheckFiles: cdRomCheckFiles,
	}
	config.StartScript = getSetting(settings, "STARTSCRIPT", defaultStartScript)
	config.DataDirectory = getSetting(settings, "DATADIRECTORY", defaultDataDirectory)
	config.SaveFileKey = getSetting(settings, "SAVEFILEKEY", "")
	config.WindowWidth = getSettingInt(settings, "WINDOWWIDTH", defaultWindowWidth)
	config.WindowHeight = getSettingInt(settings, "WINDOWHEIGHT", defaultWindowHeight)
	return config
}

func fromImage(image *EcfImage) *EmecfgFile {
	settings := map[string]string{}
	cdRomCheckFiles := []string{}
	for _, rawLine := range strings.Split(image.SettingsText, "\n") {
		line := rawLine
		if strings.HasSuffix(line, "\r") {
			line = line[:len(line)-1]
		}

		if len(line) == 0 || line[0] == '#' {
			continue
		}

		separator := strings.IndexByte(line, '=')
		if separator < 0 {
			continue
		}

		key := strings.TrimSpace(line[:separator])
		if len(key) == 0 {
			continue
		}

		value := line[separator+1:]
		if strings.EqualFold(key, "CDROMCHECKFILE") {
			cdRomCheckFiles = append(cdRomCheckFiles, value)
		}

		foldSet(settings, key, value)
	}

	macros := make([]script.EmonMacroDefinition, 0, len(image.Macros))
	for _, record := range image.Macros {
		macros = append(macros, script.EmonMacroDefinition{Name: record.Name, Body: record.Value})
	}

	symbols := map[string]string{}
	for _, record := range image.Symbols {
		foldSet(symbols, record.Name, record.Value)
	}

	return newEmecfgFile(image.SettingsText, settings, macros, symbols, cdRomCheckFiles)
}

func getSetting(settings map[string]string, key, fallback string) string {
	if value, ok := foldLookup(settings, key); ok && len(value) > 0 {
		return value
	}

	return fallback
}

func getSettingInt(settings map[string]string, key string, fallback int) int {
	if value, ok := foldLookup(settings, key); ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32); err == nil {
			return int(parsed)
		}
	}

	return fallback
}

func foldLookup(values map[string]string, key string) (string, bool) {
	if value, ok := values[key]; ok {
		return value, true
	}

	for candidate, value := range values {
		if strings.EqualFold(candidate, key) {
			return value, true
		}
	}

	return "", false
}

func foldSet(values map[string]string, key, value string) {
	if _, ok := values[key]; ok {
		values[key] = value
		return
	}

	for candidate := range values {
		if strings.EqualFold(candidate, key) {
			values[candidate] = value
			return
		}
	}

	values[key] = value
}
