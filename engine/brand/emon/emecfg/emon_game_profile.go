package emecfg

import (
	"strings"

	"kizuna/engine/brand/emon/eme"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/text"
	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const (
	ConfigurationFileName    = "emecfg.ecf"
	StartScriptFileName      = "ASMainScript.txt"
	DefaultDataDirectoryName = "data"
	DefaultWindowWidth       = 800
	DefaultWindowHeight      = 600
)

var (
	spriteExtensions = []string{".bmp", ".png", ".jpg", ".jpeg", ".gif", ".anm", ".tga"}
	audioExtensions  = []string{".ogg", ".wav", ".mp3"}
	scriptExtensions = []string{".txt"}

	EmptyEmonGameProfile = newEmonGameProfile(
		"",
		StartScriptFileName,
		DefaultDataDirectoryName,
		nameSet(),
		nameSet(),
		nameSet())
)

type EmonGameProfile struct {
	Name          string
	StartScript   string
	DataDirectory string
	TextStyle     *text.EmonTextStyle
	spriteFiles   map[string]struct{}
	audioFiles    map[string]struct{}
	scriptFiles   map[string]struct{}
}

func newEmonGameProfile(
	name string,
	startScript string,
	dataDirectory string,
	spriteFiles map[string]struct{},
	audioFiles map[string]struct{},
	scriptFiles map[string]struct{},
) *EmonGameProfile {
	return &EmonGameProfile{
		Name:          name,
		StartScript:   startScript,
		DataDirectory: dataDirectory,
		TextStyle:     text.NewEmonTextStyle(),
		spriteFiles:   spriteFiles,
		audioFiles:    audioFiles,
		scriptFiles:   scriptFiles,
	}
}

func (p *EmonGameProfile) SpriteFileCount() int {
	return len(p.spriteFiles)
}

func (p *EmonGameProfile) AudioFileCount() int {
	return len(p.audioFiles)
}

func ForEmonGameProfile(config *EmecfgFile, archives *eme.EmeArchiveSet) *EmonGameProfile {
	sprites := nameSet()
	audio := nameSet()
	scripts := nameSet()

	if archives != nil {
		for _, entryName := range archives.Names {
			classify(entryName, sprites, audio, scripts)
		}
	}

	if config != nil {
		for _, value := range config.Symbols {
			classify(value, sprites, audio, scripts)
		}

		classify(config.StartScript, sprites, audio, scripts)
	}

	title := ""
	if config != nil {
		title = setting(config, "SCRIPTTITLE", "")
	}

	startScript := StartScriptFileName
	if config != nil {
		startScript = config.StartScript
	}

	dataDirectory := DefaultDataDirectoryName
	if config != nil {
		dataDirectory = config.DataDirectory
	}

	return newEmonGameProfile(title, startScript, dataDirectory, sprites, audio, scripts)
}

func ApplyTextSymbols(style *text.EmonTextStyle, symbols map[string]string) {
	source := &symbolSource{symbols: symbols}
	window, hasWindow := readPair(symbols, source, "文章ウィンドウ位置")
	body, hasBody := readPair(symbols, source, "文章文字位置")
	name, hasName := readPair(symbols, source, "文章名前位置")
	size, hasSize := readNumber(symbols, source, "文章文字サイズ")

	if hasBody {
		style.X = body.X
		style.Y = body.Y
	}

	if hasName {
		style.SpeakerX = name.X
		style.SpeakerY = name.Y
	}

	if hasSize && size > 0 {
		style.Size = size
		style.LineHeight = size + 8
	}

	if font, ok := foldLookup(symbols, "文章文字フォント"); ok {
		family := strings.TrimSpace(script.Evaluate(font, source).AsString())
		if len(family) > 0 {
			style.FontFamily = family
		}
	}

	windowX, windowY := 0, 0
	if hasWindow {
		windowX, windowY = window.X, window.Y
	}

	core.Logf(
		"emon: text window=(%d,%d) body=(%d,%d) name=(%d,%d) size=%d",
		windowX, windowY, style.X, style.Y, style.SpeakerX, style.SpeakerY, style.Size)
}

func (p *EmonGameProfile) IsSpriteFile(name string) bool {
	return strings.TrimSpace(name) != "" &&
		(setContains(p.spriteFiles, name) || strutil.HasAnyExtFold(name, spriteExtensions))
}

func (p *EmonGameProfile) IsAudioFile(name string) bool {
	return strings.TrimSpace(name) != "" &&
		(setContains(p.audioFiles, name) || strutil.HasAnyExtFold(name, audioExtensions))
}

func (p *EmonGameProfile) IsScriptFile(name string) bool {
	return strings.TrimSpace(name) != "" &&
		(setContains(p.scriptFiles, name) || strutil.HasAnyExtFold(name, scriptExtensions))
}

func setting(config *EmecfgFile, key, fallback string) string {
	if value, ok := foldLookup(config.Settings, key); ok && len(value) > 0 {
		return value
	}

	return fallback
}

func readPair(symbols map[string]string, source script.EmonVariableSource, key string) (textPoint, bool) {
	raw, ok := foldLookup(symbols, key)
	if !ok {
		return textPoint{}, false
	}

	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return textPoint{}, false
	}

	return textPoint{
		X: int(script.Evaluate(parts[0], source).AsInt()),
		Y: int(script.Evaluate(parts[1], source).AsInt()),
	}, true
}

func readNumber(symbols map[string]string, source script.EmonVariableSource, key string) (int, bool) {
	raw, ok := foldLookup(symbols, key)
	if !ok {
		return 0, false
	}

	return int(script.Evaluate(raw, source).AsInt()), true
}

func classify(value string, sprites, audio, scripts map[string]struct{}) {
	if strings.TrimSpace(value) == "" {
		return
	}

	name := strings.TrimSpace(value)
	if strutil.HasAnyExtFold(name, spriteExtensions) {
		sprites[strings.ToLower(name)] = struct{}{}
		return
	}

	if strutil.HasAnyExtFold(name, audioExtensions) {
		audio[strings.ToLower(name)] = struct{}{}
		return
	}

	if strutil.HasAnyExtFold(name, scriptExtensions) {
		scripts[strings.ToLower(name)] = struct{}{}
	}
}

func nameSet() map[string]struct{} {
	return map[string]struct{}{}
}

func setContains(values map[string]struct{}, name string) bool {
	_, ok := values[strings.ToLower(name)]
	return ok
}

type textPoint struct {
	X int
	Y int
}

type symbolSource struct {
	symbols map[string]string
}

func (s *symbolSource) Lookup(name string) script.EmonValue {
	if raw, ok := foldLookup(s.symbols, name); ok {
		return script.ParseEmonValue(raw)
	}

	return script.EmonValueFromIdentifier(name)
}
