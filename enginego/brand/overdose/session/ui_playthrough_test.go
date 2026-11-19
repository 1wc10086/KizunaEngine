// 引擎自绘 UI 的端到端测试：标题 → 设置/鉴赏/存读档/回想 → 剧情对话。
//
// 这些断言只走真实数据（GAME_DIR），没有游戏目录时 Skip。
// 目的不是覆盖率，而是锁住“原版坐标/模态不穿透/触屏 tap=3 能推进”三条
// 容易在重构里悄悄坏掉的契约。
package session

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/core"
)

func openUISession(t *testing.T) *Session {
	t.Helper()
	cands := []string{os.Getenv("GAME_DIR"), "/root/code/engine/游戏/YUI/優衣"}
	dir := ""
	for _, c := range cands {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "script.arc")); err == nil {
			dir = c
			break
		}
	}
	if dir == "" {
		t.Skip("未找到游戏数据")
	}
	s := New()
	if _, err := s.OpenExtras(dir, nil, t.TempDir()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func composeOK(t *testing.T, s *Session) []byte {
	t.Helper()
	frame := make([]byte, frameWidth*frameHeight*4)
	if ok, err := s.Compose(unsafePointer(&frame[0]), len(frame)); !ok || err != nil {
		t.Fatalf("Compose = %v, %v", ok, err)
	}
	return frame
}

// pixelNonBlack 检查 800x600 BGRA 帧的某点不是全 0（透明/纯黑底）。
func pixelNonBlack(frame []byte, x, y int) bool {
	off := (y*frameWidth + x) * 4
	if off+2 >= len(frame) {
		return false
	}
	return frame[off] != 0 || frame[off+1] != 0 || frame[off+2] != 0
}

func TestEngineDrawnUI(t *testing.T) {
	s := openUISession(t)
	if step, err := s.Start(0); err != nil || step != core.StepAwaitTitle {
		t.Fatalf("Start(0) = %v, %v", step, err)
	}

	// 标题坐标必须是 sq_menu 的 582/341，step 50。
	items := s.TitleMenu()
	if len(items) != 5 {
		t.Fatalf("标题项 = %d", len(items))
	}
	wantY := []int32{341, 391, 441, 491, 541}
	for i, it := range items {
		if it.X != 582 || it.Y != wantY[i] || it.Width != 218 || it.Height != 38 {
			t.Fatalf("标题第 %d 项 = %+v", i, it)
		}
	}
	titleFrame := composeOK(t, s)
	if !pixelNonBlack(titleFrame, 595, 360) {
		t.Fatal("标题按钮没有画进帧")
	}

	// 设置：打开 → 点“快”文本速度 → 返回关闭。
	s.SelectTitleItem("config")
	if s.overlay != overlayConfig {
		t.Fatalf("config overlay = %v", s.overlay)
	}
	cfgFrame := composeOK(t, s)
	if !pixelNonBlack(cfgFrame, 130, 215) {
		t.Fatal("设置页的消息窗预览没有画进帧")
	}
	if _, err := s.Pointer(3, 712+1, 64+1); err != nil {
		t.Fatalf("text fast: %v", err)
	}
	if s.settings.TextSpeed != 3 {
		t.Fatalf("TextSpeed = %d, want 3", s.settings.TextSpeed)
	}
	// 后续要验证打字机，改回 normal（30ms/字）；fast=0 会瞬间全显。
	s.settings.TextSpeed = 2
	s.persistSystem()
	if _, err := s.Pointer(3, 582+1, 501+1); err != nil {
		t.Fatalf("config return: %v", err)
	}
	if s.overlay != overlayNone {
		t.Fatalf("config 未关闭: %v", s.overlay)
	}

	// 鉴赏：CG → 音乐 → 播放第 1 首 → 场景页 → 返回。
	s.SelectTitleItem("extra")
	if s.overlay != overlayExtra || s.extraTab != galleryCG {
		t.Fatalf("extra = %v/%v", s.overlay, s.extraTab)
	}
	composeOK(t, s)
	if _, err := s.Pointer(3, 417+1, 538+1); err != nil {
		t.Fatalf("music tab: %v", err)
	}
	if s.extraTab != galleryMusic {
		t.Fatalf("extraTab = %v", s.extraTab)
	}
	composeOK(t, s)
	if _, err := s.Pointer(3, 36+1, 149+1); err != nil {
		t.Fatalf("play bgm: %v", err)
	}
	if s.extraPlaying != 1 {
		t.Fatalf("extraPlaying = %d", s.extraPlaying)
	}
	seenBGM := false
	for _, ev := range s.Audio() {
		if ev.Kind == 0 {
			seenBGM = true
		}
	}
	if !seenBGM {
		t.Fatal("点击音乐没有 BGM 事件")
	}
	if _, err := s.Pointer(3, 217+1, 538+1); err != nil {
		t.Fatalf("scene tab: %v", err)
	}
	if s.extraTab != galleryScene {
		t.Fatalf("extraTab = %v", s.extraTab)
	}
	composeOK(t, s)
	if _, err := s.Pointer(3, 637+1, 552+1); err != nil {
		t.Fatalf("extra return: %v", err)
	}
	if s.overlay != overlayNone {
		t.Fatalf("extra 未关闭: %v", s.overlay)
	}

	// 进剧情：推进到第一句对白。
	if _, err := s.SelectTitleItem("start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < 300 && s.Text().Text == ""; i++ {
		if _, err := s.Advance(); err != nil {
			t.Fatalf("advance %d: %v", i, err)
		}
	}
	if s.Text().Text == "" {
		t.Fatal("没有推进到对白")
	}
	if !s.State().EngineMessageWindow {
		t.Fatal("EngineMessageWindow 应为 true")
	}
	msgFrame := composeOK(t, s)
	if !pixelNonBlack(msgFrame, 10, 430) {
		t.Fatal("对话框底图没有画进帧")
	}

	// 触屏 tap(op=3) 必须能推进：第一下补完打字机，第二下换句。
	first := s.Text().Text
	if _, err := s.Pointer(3, 400, 500); err != nil {
		t.Fatalf("tap reveal: %v", err)
	}
	if s.Text().Text != first {
		t.Fatalf("第一下点击不应换句: %q -> %q", first, s.Text().Text)
	}
	for i := 0; i < 40 && s.Text().Text == first; i++ {
		if _, err := s.Pointer(3, 400, 500); err != nil {
			t.Fatalf("tap advance: %v", err)
		}
	}
	if s.Text().Text == first {
		t.Fatal("触屏 tap 没有推进对白")
	}

	// 存档：开界面 → bank 9 → slot 10（100 号，避免撞已有档）→ 直接保存。
	defer func() { _ = s.saves.Delete(100) }()
	if _, err := s.Pointer(3, 477+1, 574+1); err != nil {
		t.Fatalf("adv save: %v", err)
	}
	if s.overlay != overlaySave {
		t.Fatalf("save overlay = %v", s.overlay)
	}
	saveFrame := composeOK(t, s)
	if !pixelNonBlack(saveFrame, 270, 90) {
		t.Fatal("存/读档槽位没有画进帧")
	}
	if _, err := s.Pointer(3, 265+9*46+1, 15+1); err != nil {
		t.Fatalf("bank9: %v", err)
	}
	if s.saveBank != 9 {
		t.Fatalf("saveBank = %d", s.saveBank)
	}
	if _, err := s.Pointer(3, 257+258+1, 80+4*92+1); err != nil {
		t.Fatalf("slot100: %v", err)
	}
	if data, err := s.saves.Load(100); err != nil || data == nil {
		t.Fatalf("槽 100 未写入: %+v, %v", data, err)
	}
	if _, err := s.Pointer(3, 603+1, 541+1); err != nil {
		t.Fatalf("save return: %v", err)
	}

	// 读档：开界面 → bank 9 → slot 10 → 确认 yes。
	before := s.Text()
	if _, err := s.Pointer(3, 556+1, 574+1); err != nil {
		t.Fatalf("adv load: %v", err)
	}
	if s.overlay != overlayLoad {
		t.Fatalf("load overlay = %v", s.overlay)
	}
	composeOK(t, s)
	if _, err := s.Pointer(3, 265+9*46+1, 15+1); err != nil {
		t.Fatalf("load bank9: %v", err)
	}
	if _, err := s.Pointer(3, 257+258+1, 80+4*92+1); err != nil {
		t.Fatalf("load slot100: %v", err)
	}
	if s.overlay != overlayConfirm {
		t.Fatalf("load 未弹确认框: %v", s.overlay)
	}
	if _, err := s.Pointer(3, 285+1, 335+1); err != nil {
		t.Fatalf("load yes: %v", err)
	}
	if s.overlay != overlayNone {
		t.Fatalf("读档后覆盖层未关: %v", s.overlay)
	}
	if got := s.Text(); got.Speaker != before.Speaker || got.Text != before.Text {
		t.Fatalf("读档文本不一致: %q/%q vs %q/%q", got.Speaker, got.Text, before.Speaker, before.Text)
	}

	// 回想：打开 → 滚轮 → return 关闭。
	if _, err := s.Pointer(3, 398+1, 574+1); err != nil {
		t.Fatalf("adv log: %v", err)
	}
	if s.overlay != overlayBacklog {
		t.Fatalf("backlog overlay = %v", s.overlay)
	}
	composeOK(t, s)
	if _, err := s.Wheel(1); err != nil {
		t.Fatalf("wheel: %v", err)
	}
	if _, err := s.Pointer(3, 754+1, 6+1); err != nil {
		t.Fatalf("backlog return: %v", err)
	}
	if s.overlay != overlayNone {
		t.Fatalf("回想未关闭: %v", s.overlay)
	}
}

// TestSceneReplay 验证场景鉴赏点开后真的跳到 tbl 第 1/2 列脚本与 label。
func TestSceneReplay(t *testing.T) {
	s := openUISession(t)
	if _, err := s.Start(0); err != nil {
		t.Fatalf("Start(0): %v", err)
	}
	// 解锁 sop3：正常玩法里由 VM 加载脚本时写 script: 键。
	s.unlockSets.ensure()
	s.unlockSets.set["script:sop3"] = true
	s.openExtraOverlay(galleryScene)
	s.extraChara = 0
	if _, err := s.Pointer(3, 335+1, 162+1); err != nil {
		t.Fatalf("scene click: %v", err)
	}
	if s.overlay != overlayNone || s.atTitle {
		t.Fatalf("场景回放未进入剧情: overlay=%v atTitle=%v", s.overlay, s.atTitle)
	}
	if s.currentScript != "sop3" {
		t.Fatalf("回放脚本 = %q", s.currentScript)
	}
}

// TestSettingsPersistence 确认设置落盘后重新 Open 仍然生效。
func TestSettingsPersistence(t *testing.T) {
	s := openUISession(t)
	orig := s.settings
	s.settings.TextSpeed = 3
	s.settings.WindowOpacity = 40
	s.persistSystem()
	s2 := openUISession(t)
	if s2.settings.TextSpeed != 3 || s2.settings.WindowOpacity != 40 {
		t.Fatalf("设置未恢复: %+v", s2.settings)
	}
	// 还原，避免污染玩家的系统文件。
	s2.settings = orig
	s2.persistSystem()
}
