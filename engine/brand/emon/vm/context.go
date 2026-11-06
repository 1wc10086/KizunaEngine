package vm

import (
	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/emon/eme"
	"kizuna/engine/brand/emon/emecfg"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	emontext "kizuna/engine/brand/emon/text"
)

type EmonConfig interface {
	StartScript() string
	MacroList() []script.EmonMacroDefinition
	Symbols() map[string]string
	WindowWidth() int
	WindowHeight() int
}

type emecfgAdapter struct {
	inner *emecfg.EmecfgFile
}

func (a *emecfgAdapter) StartScript() string {
	return a.inner.StartScript
}

func (a *emecfgAdapter) MacroList() []script.EmonMacroDefinition {
	return a.inner.MacroList
}

func (a *emecfgAdapter) Symbols() map[string]string {
	return a.inner.Symbols
}

func (a *emecfgAdapter) WindowWidth() int {
	return a.inner.WindowWidth
}

func (a *emecfgAdapter) WindowHeight() int {
	return a.inner.WindowHeight
}

type EmonArchives interface {
	Contains(name string) bool
	Read(name string) []byte
	Locate(name string) (*eme.EmeEntry, *eme.EmeArchive)
}

type EmonVariables interface {
	Count() int
	Entries() []script.EmonVariableEntry
	Contains(name string) bool
	Lookup(name string) script.EmonValue
	Set(name string, value script.EmonValue)
	Remove(name string) bool
	Clear()
	LoadFrom(entries map[string]script.EmonValue)
	Clone() *script.EmonVariableStore
	Snapshot() map[string]script.EmonValue
}

type EmonMacros interface {
	Count() int
	Names() []string
	Add(name string, definition script.EmonMacroDefinition)
	TryGet(name string) (script.EmonMacroDefinition, bool)
	Contains(name string) bool
}

type EmonStage interface {
	Root() *stage.EmonSprite
	GetOrCreate(name string, parent *stage.EmonSprite) *stage.EmonSprite
	GetOrCreateKind(name string, parent *stage.EmonSprite, kind stage.EmonSpriteKind) *stage.EmonSprite
	Reorder(sprite *stage.EmonSprite, placement stage.EmonSpriteKind)
	Find(name string) *stage.EmonSprite
	Remove(name string) bool
	Clear()
	All() []*stage.EmonSprite
	SelectSprites() []*stage.EmonSprite
	HitTestSelect(x, y int) *stage.EmonSprite
	HitTest(x, y int) *stage.EmonSprite
}

type EmonComposer interface {
	Width() int
	Height() int
	Current() *bishopgfx.BgraCanvas
	Snapshot()
	SetEffect(effect *stage.EmonEffect)
	SetBackground(b, g, r int)
	BeginEffect(kind stage.EmonEffectKind, frames, param2, param3 int) *stage.EmonEffect
}

type EmonMessage interface {
	Speaker() string
	SetSpeaker(value string)
	Body() string
	SetBody(value string)
	Visible() bool
	SetVisible(value bool)
	FullyRevealed() bool
	RevealAll()
	Append(text string)
	Clear()
}

type EmonAudio interface {
	PlaySound(request EmonSoundRequest)
	NotifyPlaybackCompleted(file string, voice bool)
	HasCompletedSounds() bool
	DequeueCompletion() (EmonSoundCompletion, bool)
	PlayMusic(file string, volume int)
	StopMusic()
	FadeOutMusic(frames int)
	StopGroup(group int)
	StopChannel(channel int)
	SetGroupVolume(group, volume int)
	SetChannelVolume(channel, volume int)
	SetMusicVolume(volume int)
	StopAll()
	Update(deltaSeconds float64)
	IsPlaying() bool
}

type EmonSystemState interface {
	Load(variables *script.EmonVariableStore) bool
	Pump(variables *script.EmonVariableStore)
	Write(variables map[string]script.EmonValue) bool
}

type EmonSaves interface {
	SystemState() EmonSystemState
	Write(data *EmonSaveData) bool
	Read(slot int) *EmonSaveData
	Exists(slot int) bool
	Delete(slot int) bool
}

type EmonScripts interface {
	Load(fileName string) *script.EmonProgram
	Contains(fileName string) bool
	ParseBody(name, body string) *script.EmonProgram
}

type RuntimeContext struct {
	Config          EmonConfig
	Archives        EmonArchives
	SystemVariables EmonVariables
	Variables       EmonVariables
	Symbols         EmonVariables
	Macros          EmonMacros
	Stage           EmonStage
	Composer        EmonComposer
	Message         EmonMessage
	Audio           EmonAudio
	Saves           EmonSaves
	Scripts         EmonScripts
	RootDirectory   string
	FrameDelta      float64
	OpenUrl         func(string)
}

func NewRuntimeContext(
	config EmonConfig,
	archives EmonArchives,
	systemVariables EmonVariables,
	variables EmonVariables,
	symbols EmonVariables,
	macros EmonMacros,
	stage EmonStage,
	composer EmonComposer,
	message EmonMessage,
	audio EmonAudio,
	saves EmonSaves,
	scripts EmonScripts,
	rootDirectory string,
) *RuntimeContext {
	if scripts == nil {
		scripts = NewScriptLoader(archives, rootDirectory)
	}
	context := &RuntimeContext{
		Config:          config,
		Archives:        archives,
		SystemVariables: systemVariables,
		Variables:       variables,
		Symbols:         symbols,
		Macros:          macros,
		Stage:           stage,
		Composer:        composer,
		Message:         message,
		Audio:           audio,
		Saves:           saves,
		Scripts:         scripts,
		RootDirectory:   rootDirectory,
	}
	if context.RootDirectory == "" {
		if loader, ok := scripts.(*ScriptLoader); ok {
			context.RootDirectory = loader.RootDirectory
		}
	}
	context.SeedSymbols()
	return context
}

func CreateRuntimeContext(config *emecfg.EmecfgFile, archives *eme.EmeArchiveSet, audio EmonAudio, saves EmonSaves, rootDirectory string) *RuntimeContext {
	gameStage := stage.NewEmonStage()
	return NewRuntimeContext(
		&emecfgAdapter{inner: config},
		archives,
		script.NewEmonVariableStore(),
		script.NewEmonVariableStore(),
		script.NewEmonVariableStore(),
		script.NewEmonMacroRegistry(),
		gameStage,
		stage.NewEmonStageComposer(config.WindowWidth, config.WindowHeight),
		emontext.NewEmonMessageBox(),
		audio,
		saves,
		nil,
		rootDirectory,
	)
}

func (c *RuntimeContext) Width() int {
	return c.Composer.Width()
}

func (c *RuntimeContext) Height() int {
	return c.Composer.Height()
}

func (c *RuntimeContext) Clone() *RuntimeContext {
	context := NewRuntimeContext(
		c.Config,
		c.Archives,
		c.SystemVariables.Clone(),
		script.NewEmonVariableStore(),
		script.NewEmonVariableStore(),
		c.Macros,
		stage.NewEmonStage(),
		stage.NewEmonStageComposer(c.Width(), c.Height()),
		emontext.NewEmonMessageBox(),
		c.Audio,
		c.Saves,
		NewScriptLoader(c.Archives, c.RootDirectory),
		c.RootDirectory,
	)
	for _, macro := range c.Config.MacroList() {
		if !context.Macros.Contains(macro.Name) {
			context.Macros.Add(macro.Name, macro)
		}
	}
	context.OpenUrl = c.OpenUrl
	return context
}

func (c *RuntimeContext) SeedSymbols() {
	if c.Symbols.Count() != 0 {
		return
	}
	for key, value := range c.Config.Symbols() {
		c.Symbols.Set(key, script.ParseEmonValue(value))
	}
	seeded := c.Symbols.Snapshot()
	for name, value := range seeded {
		if len(name) <= len(appearSuffix) {
			continue
		}
		tail := name[len(name)-len(appearSuffix):]
		if tail != appearSuffix {
			continue
		}
		alias := name[:len(name)-len(appearSuffix)] + disappearSuffix
		if _, exists := seeded[alias]; !exists {
			c.Symbols.Set(alias, value)
		}
	}
}

const appearSuffix = "現"

const disappearSuffix = "消"
