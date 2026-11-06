package session

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
)

// systemui_config_actor_test.go 覆盖设置画面里的"个别音声設定"和 BGM 类型选择。
//
// 这两块都依赖作品自己的布局（system_config_ex.layout 的 "actor setting"、
// define.bsa 的 bgm_select），所以测试里照着真实文件摆一份等价数据：
// 三个角色、各自的样本语音/样本BGV/头像，以及一条 type0 -> type1 的 BGM 映射。

// configTestLayoutText 是 system_config_ex.layout 里声音页的精简版。
const configTestLayoutText = `
{
	base: "cfgx_base"
	"category button":[
		{ layout:"sound", image:{name:"cfgx_category_sound", max_state:4, pos:[367, 127]} }
	]
	"common button":[
		{ no:125, image:{name:"cfgx_back", max_state:3, pos:[0, 0]} }
	]
	"layout":{
		"sound":{
			base: "cfgx_base_sound"
			function:{
				"bgm selector":[
					{ name:"cfgx_bgm_normal", max_state:5, pos:[222, 270] }
					{ name:"cfgx_bgm_jazz", max_state:5, pos:[367, 270] }
				]
				"actor setting":[
					{
						toggle:{ name:"cfgx_toggle", max_state:5, pos:[804, 317] }
						change:{
							name:"cfgx_change_ch1", max_state:5, pos:[879, 317]
							description:{image:"cfgx_change_ch1_desc", pos:[589, 497]}
							data:{
								name:"『キャラ1』"
								image:{ thumbnail:{name:"cfgx_face_ch1"} }
								sample:{ voice:"01_01998_001", bgv:"01_01999_058" }
							}
						}
					}
					{
						toggle:{ name:"cfgx_toggle", max_state:5, pos:[804, 347] }
						change:{
							name:"cfgx_change_ch2", max_state:5, pos:[879, 347]
							description:{image:"cfgx_change_ch2_desc", pos:[589, 497]}
							data:{
								name:"『キャラ2』"
								image:{ thumbnail:{name:"cfgx_face_ch2"} }
								sample:{ voice:"02_02998_001", bgv:"02_02999_058" }
							}
						}
					}
					{
						toggle:{ name:"cfgx_toggle", max_state:5, pos:[804, 377] }
						change:{
							name:"cfgx_change_ch3", max_state:5, pos:[879, 377]
							description:{image:"cfgx_change_ch3_desc", pos:[589, 497]}
							data:{
								name:"『キャラ3』"
								image:{ thumbnail:{name:"cfgx_face_ch3"} }
								sample:{ voice:"03_03998_001" }
							}
						}
					}
				]
				"current actor":{
					thumbnail:{name:"cfgx_face_base", pos:[589, 317]}
					voice:{
						bar:{ name:"cfgx_volume_bar", pos:[743, 540], hit_region:[743, 540, 200, 23] }
						min:{ name:"cfgx_min", max_state:3, pos:[667, 539] }
						max:{ name:"cfgx_max", max_state:3, pos:[949, 539] }
						play:{ name:"cfgx_play_samplevoice", max_state:3, pos:[589, 317] }
					}
					bgv:{
						bar:{ name:"cfgx_text_bar", pos:[743, 588], hit_region:[743, 588, 200, 23] }
						min:{ name:"cfgx_min", max_state:3, pos:[667, 587] }
						max:{ name:"cfgx_max", max_state:3, pos:[949, 587] }
						play:{ name:"cfgx_play_samplebgv", max_state:3, pos:[589, 362] }
					}
				}
			}
		}
	}
}
`

// bgmSelectTestText 是 define.bsa 里 bgm_select 的等价内容。
const bgmSelectTestText = `
{
	set: [
		["bgm07", "bgm13"]
		["bgm08", "bgm14"]
	]
}
`

// newConfigTestSession 造一个打开着设置画面的会话。
func newConfigTestSession(t *testing.T) *Session {
	t.Helper()
	root := t.TempDir()
	resources := resource.Build(root)
	composer := rendering.NewSceneComposer(resources, 1024, 768, 0, 0, nil,
		config.InformationWindow{}, config.GameMenu{}, config.MessageWindow{},
		config.FontSettings{Size: 24})
	screen, err := config.LoadConfigScreen([]byte(configTestLayoutText))
	if err != nil {
		t.Fatal(err)
	}
	// 没有 bsxx.cfg 的作品：用空配置，音量走默认值。
	settings, err := config.Load(filepath.Join(root, "absent.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{
		config:       settings,
		res:          resources,
		composer:     composer,
		width:        1024,
		height:       768,
		dataDir:      root,
		configScreen: screen,
	}
	s.settings = defaultGameSettings(s.config)
	s.ui = &systemUI{kind: uiConfig, hover: uiHit{kind: hitNone, index: -1}}
	return s
}

func TestLoadActorSettingsReadsSamples(t *testing.T) {
	s := newConfigTestSession(t)
	s.loadActorSettings()
	if len(s.actorSamples) != 3 {
		t.Fatalf("actor samples = %d, want 3", len(s.actorSamples))
	}
	first := s.actorSamples[0]
	if first.Voice != "01_01998_001" || first.BGV != "01_01999_058" {
		t.Fatalf("sample 1 = %+v", first)
	}
	if first.Thumb != "cfgx_face_ch1" {
		t.Fatalf("sample 1 thumb = %q", first.Thumb)
	}
	// 第三个角色没有样本 BGV。
	if s.actorSamples[2].BGV != "" {
		t.Fatalf("sample 3 bgv = %q, want empty", s.actorSamples[2].BGV)
	}
	// 再读一次不会重复追加。
	s.loadActorSettings()
	if len(s.actorSamples) != 3 {
		t.Fatalf("actor samples after reload = %d, want 3", len(s.actorSamples))
	}
}

func TestSelectedActorIndexClamps(t *testing.T) {
	s := newConfigTestSession(t)
	s.loadActorSettings()
	// 越界下标必须夹回合法范围，否则会索引到不存在的角色。
	s.settings.SelectedActor = 99
	if got := s.selectedActorIndex(); got != 2 {
		t.Fatalf("selected actor = %d, want 2", got)
	}
	s.settings.SelectedActor = -5
	if got := s.selectedActorIndex(); got != 0 {
		t.Fatalf("selected actor = %d, want 0", got)
	}
	if got := clampActorIndex(4, 6); got != 4 {
		t.Fatalf("clampActorIndex(4,6) = %d, want 4", got)
	}
	if got := clampActorIndex(9, 6); got != 5 {
		t.Fatalf("clampActorIndex(9,6) = %d, want 5", got)
	}
	if got := clampActorIndex(0, 0); got != 0 {
		t.Fatalf("clampActorIndex(0,0) = %d, want 0", got)
	}
}

func TestActorVolumeScaleRoundTrip(t *testing.T) {
	if got := actorVolumeScale(1); got != 1000 {
		t.Fatalf("scale(1) = %d, want 1000", got)
	}
	if got := actorVolumeScale(0.5); got != 500 {
		t.Fatalf("scale(0.5) = %d, want 500", got)
	}
	// 0.29 在浮点里是 0.28999999999999998，截断会差一格。
	if got := actorVolumeScale(0.29); got != 290 {
		t.Fatalf("scale(0.29) = %d, want 290", got)
	}
	// 越界的系数要被夹进 0..1。
	if got := actorVolumeScale(2.5); got != 1000 {
		t.Fatalf("scale(2.5) = %d, want 1000", got)
	}
	if got := actorVolumeValue(250); got != 0.25 {
		t.Fatalf("value(250) = %v, want 0.25", got)
	}
	if got := actorVolumeValue(-10); got != 0 {
		t.Fatalf("value(-10) = %v, want 0", got)
	}
}

func TestConfigActorVolumeControl(t *testing.T) {
	s := newConfigTestSession(t)
	s.buildConfigControls()

	var voiceBar, bgvBar *configControl
	for index := range s.ui.configControls {
		control := &s.ui.configControls[index]
		if control.group != "current actor" {
			continue
		}
		switch control.state {
		case "voice/bar":
			voiceBar = control
		case "bgv/bar":
			bgvBar = control
		}
	}
	if voiceBar == nil || bgvBar == nil {
		t.Fatalf("current actor meters missing: %+v", s.ui.configControls)
	}
	if voiceBar.maxValue != 1000 {
		t.Fatalf("voice bar max = %d, want 1000", voiceBar.maxValue)
	}
	// 默认音量是 1.0，刻度就是 1000。
	if got := s.configControlValue(*voiceBar); got != 1000 {
		t.Fatalf("voice bar value = %d, want 1000", got)
	}
	s.setConfigControlValue(*voiceBar, 400)
	if got := s.settings.CharacterVoice[0]; got != 0.4 {
		t.Fatalf("character voice = %v, want 0.4", got)
	}
	if got := s.configControlValue(*voiceBar); got != 400 {
		t.Fatalf("voice bar value = %d, want 400", got)
	}
	// 换到 2 号角色后，写入的是 2 号角色那一条。
	s.settings.SelectedActor = 1
	s.setConfigControlValue(*bgvBar, 750)
	if got := s.settings.CharacterBgv[1]; got != 0.75 {
		t.Fatalf("character bgv[1] = %v, want 0.75", got)
	}
	if got := s.settings.CharacterBgv[0]; got != 1 {
		t.Fatalf("character bgv[0] = %v, want 1 (unchanged)", got)
	}
	// min/max 按钮落到同一个音量上。
	s.setConfigControlValue(configControl{group: "current actor", state: "voice/min", maxValue: 1000}, 0)
	if got := s.settings.CharacterVoice[1]; got != 0 {
		t.Fatalf("character voice[1] = %v, want 0", got)
	}
	s.setConfigControlValue(configControl{group: "current actor", state: "voice/max", maxValue: 1000}, 1000)
	if got := s.settings.CharacterVoice[1]; got != 1 {
		t.Fatalf("character voice[1] = %v, want 1", got)
	}
}

func TestConfigActorSelectionStoresActor(t *testing.T) {
	s := newConfigTestSession(t)
	s.activateConfigControl(configControl{group: "actor setting:2", state: "change"}, 0, 0)
	if got := s.settings.SelectedActor; got != 2 {
		t.Fatalf("selected actor = %d, want 2", got)
	}
	// 选中的角色在按钮上是高亮状态。
	if !s.configControlSelected(configControl{group: "actor setting:2", state: "change"}) {
		t.Fatal("selected actor button is not highlighted")
	}
	if s.configControlSelected(configControl{group: "actor setting:0", state: "change"}) {
		t.Fatal("unselected actor button is highlighted")
	}
	// 越界的角色下标要被夹住（设置里最多存 0..7；AFDVD 的旧式设置
	// 有 7 个角色，所以数组从 6 扩到 8）。
	s.activateConfigControl(configControl{group: "actor setting:9", state: "change"}, 0, 0)
	if got := s.settings.SelectedActor; got != 7 {
		t.Fatalf("selected actor = %d, want 7", got)
	}
	// 作品只有 3 个角色时，实际生效的下标要夹到最后一个角色。
	if got := s.selectedActorIndex(); got != 2 {
		t.Fatalf("effective actor = %d, want 2", got)
	}
}

func TestConfigActorSamplePlayback(t *testing.T) {
	s := newConfigTestSession(t)
	// 试听样本语音：发出的是一条 voice 事件，路径是样本文件名。
	s.activateConfigControl(configControl{group: "current actor", state: "voice/play"}, 0, 0)
	if len(s.audio) != 1 {
		t.Fatalf("audio events = %d, want 1", len(s.audio))
	}
	event := s.audio[0]
	if event.Kind != 2 {
		t.Fatalf("event kind = %d, want 2", event.Kind)
	}
	if event.Path != "01_01998_001" {
		t.Fatalf("event path = %q", event.Path)
	}
	// 样本走和正式播放一样的音量链路：通道音量 × 个别音量。
	if event.Volume != 1 {
		t.Fatalf("event volume = %v, want 1", event.Volume)
	}
	// 切到 2 号角色后试听的是 2 号角色的样本。
	s.audio = nil
	s.activateConfigControl(configControl{group: "actor setting:1", state: "change"}, 0, 0)
	s.activateConfigControl(configControl{group: "current actor", state: "voice/play"}, 0, 0)
	if len(s.audio) != 1 || s.audio[0].Path != "02_02998_001" {
		t.Fatalf("second sample = %+v", s.audio)
	}
	// 3 号角色没有样本 BGV，不该发出任何事件。
	s.audio = nil
	s.activateConfigControl(configControl{group: "actor setting:2", state: "change"}, 0, 0)
	s.activateConfigControl(configControl{group: "current actor", state: "bgv/play"}, 0, 0)
	if len(s.audio) != 0 {
		t.Fatalf("audio events = %d, want 0", len(s.audio))
	}
}

func TestRemapBgmName(t *testing.T) {
	table := map[string]string{"bgm07": "bgm13", "bgm08": "bgm14"}
	// type0 是原曲。
	if got := remapBgmName(table, "bgm07", 0); got != "bgm07" {
		t.Fatalf("mode 0 = %q, want bgm07", got)
	}
	// type1 用映射后的曲子。
	if got := remapBgmName(table, "bgm07", 1); got != "bgm13" {
		t.Fatalf("mode 1 = %q, want bgm13", got)
	}
	// 没有对应版本的曲子保持原样。
	if got := remapBgmName(table, "bgm01", 1); got != "bgm01" {
		t.Fatalf("unmapped = %q, want bgm01", got)
	}
	// 没有映射表时也保持原样。
	if got := remapBgmName(nil, "bgm07", 1); got != "bgm07" {
		t.Fatalf("no table = %q, want bgm07", got)
	}
	if got := remapBgmName(table, "", 1); got != "" {
		t.Fatalf("empty name = %q", got)
	}
}

func TestLoadBgmSelectAndRemap(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bgm_select"), []byte(bgmSelectTestText), 0o644); err != nil {
		t.Fatal(err)
	}
	resources := resource.Build(root)
	settings, err := config.Load(filepath.Join(root, "absent.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{res: resources, config: settings}
	s.settings = defaultGameSettings(s.config)
	s.loadBgmSelect()
	if len(s.bgmSelectTable) != 2 {
		t.Fatalf("bgm table = %v", s.bgmSelectTable)
	}
	// 默认 type0：原曲。
	if got := s.bgmForMode("bgm07"); got != "bgm07" {
		t.Fatalf("mode 0 = %q, want bgm07", got)
	}
	// 切到 type1：换成映射后的曲子。
	s.settings.BgmSelect = 1
	if got := s.bgmForMode("bgm07"); got != "bgm13" {
		t.Fatalf("mode 1 = %q, want bgm13", got)
	}
	// 音乐鉴赏室里播放的是玩家自己挑的曲子，不做映射。
	s.ui = &systemUI{kind: uiMuseumMusic}
	if got := s.bgmForMode("bgm07"); got != "bgm07" {
		t.Fatalf("museum = %q, want bgm07", got)
	}
}
