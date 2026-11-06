package save

import (
	"time"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

const CurrentVersion = 5

// 版本 5 在缩略图后追加一段 JSON 扩展：SLG 作品的数值表、Lua 的
// PrivateData/CommonData 与成就解锁。旧版本存档没有这段，读的时候留空，
// 不影响剧情/舞台状态的兼容。
const saveExtensionVersion = 5

type SaveArtwork struct {
	Image        string
	X            int
	Y            int
	Frames       int
	AnimatedPart int
}

type SaveLayer struct {
	Slot      int
	Image     string
	Alpha     float64
	Character bool
	Artworks  []SaveArtwork
}

type SaveData struct {
	Version         int
	GameName        string
	SavedAt         time.Time
	Machine         vm.MachineState
	Layers          []SaveLayer
	Bgm             *string
	Speaker         string
	Text            string
	Thumbnail       []byte
	ThumbnailWidth  int
	ThumbnailHeight int
	Summary         string

	// ScriptDB 是 define.bsa 的 var_db 数值表快照（SLG 的训练/日志数据）。
	// 用"表名 -> 行名 -> 列名 -> 值"保存，CSV 增删行列后旧档仍能对齐。
	ScriptDB map[string]map[string]map[string]int32
	// PrivateData/CommonData 对应 Lua 的同名全局表；解锁成就是纯内存副本，
	// 已在 common.dat 落盘，这里再存一份保证换设备/删全局档后本槽仍可读。
	PrivateData          map[string]any
	CommonData           map[string]any
	UnlockedAchievements map[string]bool

	// MapSource/MapButtons 保存脚本自绘地图的当前屏幕：地图卡只在
	// GameState.buttons 里，舞台层快照不会带上它们。读档时没有这份数据
	// 会停在一张没有热区的空地图上。
	MapSource  string
	MapButtons []SaveMapButton

	// WordTemp/WordSaved 是満淫電車2 的责め言葉记忆：临时 7 格、
	// 保存 3 页 x 7 格。HasWordMemory 区分"旧档没有这份数据"和"确实是空记忆"。
	WordTemp      []int
	WordSaved     [3][7]int
	HasWordMemory bool
	// WordActive 表示存档时正处于言葉責め事件中：读档后右上角仪表要恢复。
	WordActive bool

	// ChoiceOptions 是停在选择肢时已经 FETCH 出来的选项。读档时 PC 已经在
	// SELECT 之后，必须把选项恢复回来，否则玩家会跳过这次选择。
	ChoiceOptions []SaveChoiceOption
}

// SaveChoiceOption 是 vm.SelectOption 的存储层形态。
type SaveChoiceOption struct {
	Index     int
	MessageId int
	LabelId   int
	Value     int
	Kind      int
}

// SaveMapButton 是脚本自绘地图上一个可点按钮的最小快照。
// 掩码类地图（KZK/屈辱2）的 1024x768 掩码不适合整张序列化；这类地图
// 存档时由 Session 拒绝，避免读档后所有卡退化成重叠的矩形热区。
type SaveMapButton struct {
	Image    string
	X        int
	Y        int
	Value    int
	Frames   int
	Width    int
	Height   int
	Layer    int
	Disabled bool
	// HoverFrame/HasHoverFrame 记录地图卡的悬停帧（OH 的 5 帧卡用第 4 帧）。
	// 读档后不重跑剧本，必须把这份表现数据一起恢复，否则悬停会跳到第 1 帧。
	HoverFrame    int  `json:"hoverFrame,omitempty"`
	HasHoverFrame bool `json:"hasHoverFrame,omitempty"`
}

func Capture(gameState *state.GameState, machine vm.MachineState, gameName, summary string, thumbnail []byte, thumbnailWidth, thumbnailHeight int) *SaveData {
	layers := make([]SaveLayer, 0, len(gameState.Layers()))
	for _, layer := range gameState.Layers() {
		if len(layer.Artworks()) == 0 {
			continue
		}
		artworks := make([]SaveArtwork, 0, len(layer.Artworks()))
		for _, artwork := range layer.Artworks() {
			artworks = append(artworks, SaveArtwork{
				Image:        artwork.Image,
				X:            artwork.X,
				Y:            artwork.Y,
				Frames:       artwork.Frames,
				AnimatedPart: artwork.AnimatedPart,
			})
		}
		layers = append(layers, SaveLayer{
			Slot:      layer.Id(),
			Image:     layer.Name(),
			Alpha:     layer.Alpha(),
			Character: layer.Kind() == state.StageLayerCharacter,
			Artworks:  artworks,
		})
	}
	var bgm *string
	if name, ok := gameState.Bgm(); ok {
		value := name
		bgm = &value
	}
	return &SaveData{
		Version:         CurrentVersion,
		GameName:        gameName,
		SavedAt:         time.Now(),
		Machine:         machine,
		Layers:          layers,
		Bgm:             bgm,
		Speaker:         gameState.Speaker(),
		Text:            gameState.Text(),
		Thumbnail:       thumbnail,
		ThumbnailWidth:  thumbnailWidth,
		ThumbnailHeight: thumbnailHeight,
		Summary:         summary,
	}
}

func (d *SaveData) ApplyTo(gameState *state.GameState) {
	gameState.ClearStage()
	for _, layer := range d.Layers {
		restoreLayer(gameState, layer)
	}
	if d.Bgm != nil {
		gameState.SetBgm(*d.Bgm)
	} else {
		gameState.ClearBgm()
	}
	gameState.SetSpeaker(d.Speaker)
	gameState.SetText(d.Text)
	gameState.SetAwaitingInput(true)
	gameState.SetMessageWindowVisible(true)
	gameState.ClearChoices()
}

func restoreLayer(gameState *state.GameState, layer SaveLayer) {
	kind := state.StageLayerScreen
	if layer.Character {
		kind = state.StageLayerCharacter
	}
	artworks := make([]state.LayerArtwork, 0, len(layer.Artworks)+1)
	for _, artwork := range layer.Artworks {
		artworks = append(artworks, state.LayerArtwork{
			Image:        artwork.Image,
			X:            artwork.X,
			Y:            artwork.Y,
			Frames:       max(1, artwork.Frames),
			AnimatedPart: artwork.AnimatedPart,
		})
	}
	if len(artworks) == 0 && len(layer.Image) > 0 {
		artworks = append(artworks, state.LayerArtworkAt(layer.Image))
	}
	gameState.SetLayerAt(layer.Slot, layer.Image, kind, artworks)
	gameState.SetAlpha(layer.Slot, layer.Alpha)
}
