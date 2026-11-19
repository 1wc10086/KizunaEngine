package vm

// load_ui_cleanup.go：读档后清掉“存档界面残留”。
//
// 原引擎的存档点是剧情脚本指针，读档后直接回到剧情；存档/读档界面本身是
// 由工具条宏创建的 UI 精灵（セーブロードスプライト 整棵子树）。ASENURE 的
// 存档宏在 *界面显示中* 调用 #Save，于是 SaveFileKey 里会连
// 「ＳＬ選択＊エリア DISPON」这类 UI 状态一起写进 SpriteStates；
// 读档后原样恢复，玩家看到的就是“读档成功但画面还停在读档页”。
// 这里在 RestoreState 之后按名字把存档/读档界面收掉，与回到剧情的
// 原版行为一致。只影响 UI 精灵，不动剧情 sprite。

import (
	"strings"
)

var loadCleanupRoots = []string{
	"セーブロードスプライト",
}

func (m *Machine) clearLoadUi() {
	if m.context == nil || m.context.Stage == nil {
		return
	}
	// 只收根节点：绘制与命中都会检查父级可见性，根一关整棵子树就不画。
	// 不能逐个 SetVisible(false)——脚本再次打开界面时只会把根设回 DISPON，
	// 子精灵（ＳＬ背景 等）的自身标记还停在隐藏，读档页就会“能点但全黑”。
	for _, root := range loadCleanupRoots {
		if sprite := m.context.Stage.Find(root); sprite != nil {
			sprite.SetVisible(false)
		}
	}
	// 子精灵的自身可见性复位成脚本声明时的基线，保证下次打开界面能正常显示。
	m.restoreLoadUiBaseline()
	if m.context.Message != nil {
		m.context.Message.SetVisible(true)
	}
}

// restoreLoadUiBaseline 把存档界面子树的自身可见性复位。
// 界面根节点由脚本的 SpriteSwitch 控制，子节点多数在 #SetSprite 时就是
// DISPON，之后只在翻页/悬停时临时改动；读档时若把它们留在当时的临时状态，
// 下次打开界面就会缺图（全黑但热区仍在）。
func (m *Machine) restoreLoadUiBaseline() {
	stage := m.context.Stage
	if stage == nil {
		return
	}
	for _, sprite := range stage.All() {
		if !isLoadUiName(sprite.Name()) {
			continue
		}
		base, ok := m.spriteBaseVisible.Get(sprite.Name())
		if !ok {
			continue
		}
		sprite.SetVisible(base != 0)
	}
}

// screenRootNames 是决定「当前画面」的根精灵。
// 存档时记录它们的可见性，读档时原样恢复；调教/CG/标题等模式无需再按
// 画面モード 猜，任何作品都适用。
var screenRootNames = []string{
	"ゲームスプライト",
	"ゲーム基本スプライト",
	"フルＣＧ",
	"調教モードスプライト",
	"タイトルスプライト",
	"汗メータースプライト",
	"システムスプライト",
	"回想スプライト",
	"おまけスプライト",
	// ZUBUNURE 的独占 HUD 与 BONYUMANAMI 的会話 HUD 是独立画面根，
	// 不是调教/会話模式的子精灵。存档宏在画面中调用 #Save 时会先关 HUD，
	// 若不记录这里，读档时恢复不到，就会缺 HUD 界面。
	"独占モードスプライト",
	"会話モードスプライト",
	"ミニ選択スプライト",
	// おまけ 調教/独占 是从标题进入的独立界面，也要参与画面根快照，
	// 否则从该界面附近存档再读档会把整层界面收掉。
	"おまけ調教モードスプライト",
	"おまけ独占モードスプライト",
}

// overlayRootNames 是覆盖在游戏画面上、会临时关掉背景的全屏界面根节点。
var overlayRootNames = map[string]bool{
	"セーブロードスプライト":   true,
	"汗メータースプライト":    true,
	"システムスプライト":     true,
	"回想スプライト":       true,
	"おまけスプライト":      true,
	"おまけ調教モードスプライト": true,
}

// noteOverlayVisibility 在界面根节点开关时维护画面快照。
// 存档宏会先关背景、后开界面，所以在“开界面”那一刻拍快照已经太晚。
// 真正的做法是每帧在无界面时更新基线（refreshScreenRootsBaseline），
// 打开界面时把它固定住。
func (m *Machine) noteOverlayVisibility(name string, visible bool) {
	if !overlayRootNames[name] {
		return
	}
	if visible {
		if m.uiScreenSnapshot == nil {
			m.uiScreenSnapshot = m.captureScreenRoots()
		}
		return
	}
	m.uiScreenSnapshot = nil
}

// refreshScreenRootsBaseline 在没有任何全屏 UI 时记录当前画面根可见性，
// 作为下一次存档的画面快照。每帧调用，开销只是几个 map 写。
func (m *Machine) refreshScreenRootsBaseline() {
	if m.context == nil || m.context.Stage == nil {
		return
	}
	for name := range overlayRootNames {
		if sprite := m.context.Stage.Find(name); sprite != nil && sprite.Visible() {
			return
		}
	}
	m.uiScreenSnapshot = m.captureScreenRoots()
}

// captureScreenRoots 记录当前画面根节点的可见性。
// 必须在存档界面宏改动它们 *之前* 的状态下取值——调用点就在 #Save 当时。
func (m *Machine) captureScreenRoots() map[string]bool {
	stage := m.context.Stage
	if stage == nil {
		return nil
	}
	out := map[string]bool{}
	for _, name := range screenRootNames {
		if sprite := stage.Find(name); sprite != nil {
			out[name] = sprite.Visible()
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ScreenRootsSnapshot 返回用于入档的画面根可见性：
// 有全屏 UI 时用打开前的基线，否则用当前实际状态。
func (m *Machine) ScreenRootsSnapshot() map[string]bool {
	if len(m.uiScreenSnapshot) > 0 {
		out := make(map[string]bool, len(m.uiScreenSnapshot))
		for name, visible := range m.uiScreenSnapshot {
			out[name] = visible
		}
		return out
	}
	return m.captureScreenRoots()
}

// restoreScreenRoots 按存档记录恢复画面根节点。
// restoreGameScreen 只在旧档（没有记录）时兜底。
func (m *Machine) restoreScreenRoots(roots map[string]bool) bool {
	stage := m.context.Stage
	if stage == nil || len(roots) == 0 {
		return false
	}
	for name, visible := range roots {
		if sprite := stage.Find(name); sprite != nil {
			sprite.SetVisible(visible)
		}
	}
	// 既不在记录里、也不是界面自身的画面根，一律关掉，避免叠加出多套界面。
	for _, name := range screenRootNames {
		if _, ok := roots[name]; ok {
			continue
		}
		if sprite := stage.Find(name); sprite != nil {
			sprite.SetVisible(false)
		}
	}
	return true
}

// restoreGameScreen 旧存档（无 ScreenRoots）兜底：按 画面モード 打开画面。
func (m *Machine) restoreGameScreen() {
	stage := m.context.Stage
	if stage == nil {
		return
	}
	mode := strings.TrimSpace(m.Lookup("画面モード").AsString())
	cg := mode == "ＣＧ" || mode == "CG"
	trainingBG := mode == "調教選択背景"
	trainingCG := mode == "調教選択ＣＧ"
	title := mode == "タイトル"
	// 复刻脚本里各「…画面復帰」分支的可见性：
	//   背景       → ゲームスプライト + ゲーム基本スプライト
	//   ＣＧ       → フルＣＧ + ゲームスプライト
	//   調教選択背景 → 調教モードスプライト + ゲーム基本スプライト
	//   調教選択ＣＧ → 調教モードスプライト + フルＣＧ
	if sprite := stage.Find("ゲームスプライト"); sprite != nil {
		sprite.SetVisible(!trainingBG && !trainingCG && !title)
	}
	if sprite := stage.Find("ゲーム基本スプライト"); sprite != nil {
		sprite.SetVisible(trainingBG || (!cg && !trainingCG && !title))
	}
	if sprite := stage.Find("フルＣＧ"); sprite != nil {
		sprite.SetVisible(cg || trainingCG)
	}
	if sprite := stage.Find("調教モードスプライト"); sprite != nil {
		sprite.SetVisible(trainingBG || trainingCG)
	}
	if sprite := stage.Find("タイトルスプライト"); sprite != nil {
		sprite.SetVisible(title)
	}
	for _, name := range []string{"メッセージウィンドウ", "メッセージボックス"} {
		if sprite := stage.Find(name); sprite != nil {
			sprite.SetVisible(!trainingBG && !trainingCG)
		}
	}
}

// restoreModeHud 按变量补开系统 HUD，只开不关。
// 替代原引擎「ロード画面終了」的復帰分支：旧存档没有 ScreenRoots、或新存档是在
// HUD 被隐藏时存的，读档后 restoreScreenRoots 恢复不到 HUD，而原版读档后会按
// 当前变量把 HUD 补开，所以这里在 RestoreState 之后（由 onLoad/LoadSlot 调用）
// 按变量补开，避免读档缺 HUD。只开不关，避免在非对应模式下把脚本本应隐藏的 HUD
// 强行打开。
func (m *Machine) restoreModeHud() {
	if m.context == nil || m.context.Stage == nil {
		return
	}
	// m.Lookup 未命中时返回 identifier 本身，AsString() 就是变量名，AsInt() 为 0，
	// 不会误触发。
	mode := strings.TrimSpace(m.Lookup("画面モード").AsString())
	training := mode == "調教選択背景" || mode == "調教選択ＣＧ"
	if training {
		if sprite := m.context.Stage.Find("調教モードスプライト"); sprite != nil {
			sprite.SetVisible(true)
		}
		// 独占 HUD 在调教选择画面下也存在（ZUBUNURE），独立画面根。
		if sprite := m.context.Stage.Find("独占モードスプライト"); sprite != nil {
			sprite.SetVisible(true)
		}
	}
	if m.Lookup("会話モード会話中").AsInt() != 0 {
		if sprite := m.context.Stage.Find("会話モードスプライト"); sprite != nil {
			sprite.SetVisible(true)
		}
		if sprite := m.context.Stage.Find("ミニ選択スプライト"); sprite != nil {
			sprite.SetVisible(true)
		}
	}
}

func isLoadUiName(name string) bool {
	switch {
	case strings.HasPrefix(name, "ＳＬ選択"), strings.HasPrefix(name, "ＳＬＮＯ"), strings.HasPrefix(name, "ＳＬサム"),
		strings.HasPrefix(name, "Ｓ画面"), strings.HasPrefix(name, "Ｌ画面"),
		strings.HasPrefix(name, "セーブ画面"), strings.HasPrefix(name, "ロード画面"),
		name == "ＳＬ背景", name == "ＳＬサムネイル", name == "Ｓもどる", name == "Ｌもどる":
		return true
	}
	return false
}
