package legacy

import (
	"encoding/json"
	"os"

	"kizuna/engine/core"
)

const legacyConfigFileName = "kizuna-legacy.json"

type legacyConfig struct {
	Clear       bool `json:"clear"`
	InstantText bool `json:"instantText"`
	Bgm         bool `json:"bgm"`
}

func defaultLegacyConfig() legacyConfig {
	return legacyConfig{Bgm: true}
}

func loadLegacyConfig(path string) legacyConfig {
	config := defaultLegacyConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return config
	}
	if err := json.Unmarshal(data, &config); err != nil {
		core.Logf("legacy: config '%s' unreadable, using defaults: %v", path, err)
		return defaultLegacyConfig()
	}
	return config
}

func saveLegacyConfig(path string, config legacyConfig) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		core.Logf("legacy: config encode failed: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		core.Logf("legacy: config write failed: %v", err)
	}
}

func (e *Engine) persistConfig() {
	saveLegacyConfig(e.configPath, e.config)
}
