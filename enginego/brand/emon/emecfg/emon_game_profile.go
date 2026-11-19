package emecfg

import (
	"strings"

	"kizuna/enginego/brand/emon/eme"
	"kizuna/enginego/brand/emon/script"
	"kizuna/enginego/brand/emon/text"
	"kizuna/enginego/core"
	"kizuna/enginego/core/strutil"
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

	// Emon 系作品有两种正文区域键，第一分量语义不同，不能套同一个公式：
	//   文章ウィンドウサイズ   = 宽,高     （TOPPATU / NOZOMI / BOKUONE）
	//   文章ウィンドウＣＧサイズ = 右边界,高（ASENURE，旧适配沿用至今）
	// 原引擎 sub_43AB00 用前者：每行字数 = 宽 / 字号；行高先按 字号*1.1
	// 求行数，再把行高反算成 高/行数（未设固定行間时），使最后一行恰好
	// 落在文本框底部。少了这一步，BOKUONE 会按默认 34 字/4 行排版：
	// 896px 的框里塞 34*27=918px，且第 4 行压到 Q.SAVE 工具条上。
	if area, ok := readPair(symbols, source, "文章ウィンドウサイズ"); ok && style.Size > 0 && area.Y > 0 {
		lineHeight := max(1, int(float64(style.Size)*1.1))
		lines := max(1, area.Y/lineHeight)
		style.MaxLines = lines
		style.LineHeight = max(1, area.Y/lines)
		// 原引擎的换行宽度是“正文区宽 - 字号”：sub_43A680 用
		// GetTextExtentExPointA 的 nMaxExtent = *(a1+4) - *(a1+36)，
		// 其中 +4 是文章ウィンドウサイズ的宽、+36 是初始化成字号的左边距。
		if area.X > style.Size {
			style.MaxCharactersPerLine = max(1, (area.X-style.Size)/style.Size)
		}
	} else if area, ok := readPair(symbols, source, "文章ウィンドウＣＧサイズ"); ok {
		if style.Size > 0 && area.Y > 0 && area.Y%style.Size == 0 {
			// 高度是字号整数倍时（ASENURE 96=24*4）按 1em 行距排满 4 行；
			// 沿用 +8 会只剩 3 行，第 4 行被挤掉。
			style.LineHeight = style.Size
		}
		// X 是正文相对窗口的左内边距：(右边界 - X) / 字号 = 每行字数。
		if style.Size > 0 && area.X > style.X {
			style.MaxCharactersPerLine = (area.X - style.X) / style.Size
		}
		if style.LineHeight > 0 && area.Y > 0 {
			style.MaxLines = area.Y / style.LineHeight
		}
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
		"emon: text window=(%d,%d) body=(%d,%d) name=(%d,%d) size=%d line=%d chars=%d lines=%d",
		windowX, windowY, style.X, style.Y, style.SpeakerX, style.SpeakerY,
		style.Size, style.LineHeight, style.MaxCharactersPerLine, style.MaxLines)
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
