package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"kizuna/enginego/brand/tmrhiro/pac"
	"kizuna/enginego/brand/tmrhiro/save"
	"kizuna/enginego/core"
)

// 真数据回归：用实际安装目录走一遍“标题 → 开始 → 前几十句 → 选项”。
//
// 这个测试锁住三件事：标题菜单能起来、对白能连续推进、选项能出现并被
// 选择。缺一不可——它们分别是“进不去游戏”“卡在第一句”“分支按钮按不动”
// 的直接症状。CI 没有游戏目录时跳过。
func TestRealDataPlaythrough(t *testing.T) {
	dir := os.Getenv("TMRHIRO_GAME_DIR")
	if dir == "" {
		dir = "/root/code/Lune/游戏/巨乳女士官洗脳催眠"
	}
	// 发行版把数据放在 <root>/________ 之类的随机名子目录，用 FindGameDir
	// 统一探测，避免测试把某个作品的具体目录名写死。
	if _, ok := pac.FindGameDir(dir); !ok {
		t.Skip("未找到游戏数据")
	}
	s := New()
	if _, err := s.OpenExtras(dir, nil, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// 接管存档目录：OpenExtras 默认把存档写进游戏目录（与其它品牌一致），
	// 测试改成临时目录，并把刚被创建的空目录删掉，玩家的游戏目录保持干净。
	gameDir, _ := pac.FindGameDir(dir)
	_ = os.RemoveAll(filepath.Join(gameDir, "kizuna-save"))
	s.store = save.New(t.TempDir())
	if _, err := s.Start(0); err != nil {
		t.Fatal(err)
	}
	if menu := s.TitleMenu(); len(menu) != 5 {
		t.Fatalf("标题菜单项数 = %d，期望 5", len(menu))
	}
	step, err := s.SelectTitleItem("START")
	if err != nil {
		t.Fatal(err)
	}
	// 原版「任務開始」的第一步是姓名输入（sys_name_change + 系统对话框），
	// 确认之后才进正篇。测试要按同样的流程走一遍：确认默认名，再看停点。
	if s.nameEntryActive() {
		if _, err := s.Pointer(3, int32(nameConfirmBtn.X+10), int32(nameConfirmBtn.Y+10)); err != nil {
			t.Fatalf("name entry: %v", err)
		}
	}
	if !s.nameEntryActive() {
		// 玩家确认的名字必须落到剧本变量 ＄α 上（正文里的名字就是它）。
		if name := s.vm.StrVar("＄α"); name == "" {
			t.Fatalf("姓名输入没有写进 ＄α")
		}
	}
	if step = s.CurrentStep(); step != core.StepAwaitInput {
		t.Fatalf("开始后 step = %d，期望等输入", step)
	}
	// 主角名必须已经替换进正文/说话人：不能出现字面的「＄α」。
	if s.Text().Speaker == "＄α" || strings.Contains(s.Text().Text, "＄α") {
		t.Fatalf("正文里的名字变量没有被替换: %q", s.Text().Speaker)
	}
	seen := 0
	// 选项在第四章（03_00_select）才会出现，实测约 2990 句之后
	// （前几章是一整段连续的日常演出）。上限给足余量，超了就是回归。
	//
	// 中途停在第 40 句上抓一帧：那里一定停在等输入（对话框可见），
	// 用来锁住"消息框真的画进帧位图"这条契约。
	var shot []byte
	for i := 0; i < 4000; i++ {
		if s.Text().Text != "" {
			seen++
		}
		if i == 40 {
			shot = make([]byte, frameWidth*frameHeight*4)
			if ok, err := s.Compose(unsafe.Pointer(&shot[0]), len(shot)); !ok || err != nil {
				t.Fatalf("compose: %v %v", ok, err)
			}
		}
		step, err = s.Advance()
		if err != nil {
			t.Fatalf("advance: %v", err)
		}
		if step == core.StepAwaitChoice {
			break
		}
		if step == core.StepFinished || step == core.StepFaulted {
			t.Fatalf("意外结束 step=%d err=%s", step, s.LastError())
		}
	}
	if seen < 10 {
		t.Fatalf("只看到 %d 句对白", seen)
	}
	if len(s.Choices()) == 0 {
		t.Fatal("没走到选项")
	}
	// 界面层（消息框/工具条）必须真的画进帧里：那个帧是在 awaitInput
	// 时抓的，消息窗底图把下半屏压暗，正文作为 TextRun 交给宿主。
	if len(shot) == 0 {
		t.Fatal("没有抓到对话框帧")
	}
	// 窗口底部（y=700）一定被压成近黑：底图的白渐变在 MW_DRAW_HEIGHT 底部
	// alpha 接近 255。取屏幕左侧 x=20，避开工具条与正文。
	off := (700*frameWidth + 20) * 4
	if shot[off] > 90 || shot[off+1] > 90 || shot[off+2] > 90 {
		t.Fatalf("消息窗区域没有压暗: %v", shot[off:off+4])
	}
	if _, err := s.Choose(0); err != nil {
		t.Fatalf("choose: %v", err)
	}
	// 选择后必须继续出对白。
	if s.Text().Text == "" {
		t.Fatal("选完没有新对白")
	}
	// 存档/读档往返。
	buf := make([]byte, 1280*720*4)
	if _, err := s.Compose(unsafe.Pointer(&buf[0]), len(buf)); err != nil {
		t.Fatalf("compose: %v", err)
	}
	if err := s.Save(1, "auto"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := s.Load(1); err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Text().Text == "" {
		t.Fatal("读档后没有对白")
	}
	// 存档目录默认落在游戏目录里（与其它品牌一致）。真数据测试要能写，
	// 否则归档所在的只读挂载会让 Save/Load 失败；这里显式建在临时目录，
	// 跑完由 t.TempDir 自动清理，玩家的游戏目录保持干净。

}
