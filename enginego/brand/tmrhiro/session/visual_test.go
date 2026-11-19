package session

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"unsafe"
)

// dumpFrame 把当前帧导出成 PNG（RGB 直读，便于人工核对原版布局）。
func dumpFrame(t *testing.T, s *Session, path string) []byte {
	t.Helper()
	buf := make([]byte, frameWidth*frameHeight*4)
	if ok, err := s.Compose(unsafe.Pointer(&buf[0]), len(buf)); !ok || err != nil {
		t.Fatalf("compose: %v %v", ok, err)
	}
	img := image.NewRGBA(image.Rect(0, 0, frameWidth, frameHeight))
	for i := 0; i < frameWidth*frameHeight; i++ {
		o := i * 4
		img.Set(i%frameWidth, i/frameWidth, color.RGBA{R: buf[o+2], G: buf[o+1], B: buf[o], A: 255})
	}
	f, _ := os.Create(path)
	png.Encode(f, img)
	f.Close()
	return buf
}

// TestDbgVisual 是人工核对用的截图脚本（不是断言测试）：
// 它把剧情、设置、回想、暂停菜单各导出一张 PNG，方便和原版手册对照。
// 没有游戏目录时跳过。
func TestDbgVisual(t *testing.T) {
	dir := os.Getenv("TMRHIRO_GAME_DIR")
	if dir == "" {
		dir = "/root/code/Lune/游戏/巨乳女士官洗脳催眠"
	}
	out := os.Getenv("TMRHIRO_SHOT_DIR")
	if out == "" {
		t.Skip("需要 TMRHIRO_SHOT_DIR 指定输出目录")
	}
	s := New()
	if _, err := s.OpenExtras(dir, nil, t.TempDir()); err != nil {
		t.Skip(err)
	}
	s.Start(0)
	dumpFrame(t, s, out+"/title.png")
	s.SelectTitleItem("START")
	// 「任務開始」的第一步是姓名输入：确认默认名之后脚本才进正篇。
	if s.nameEntryActive() {
		s.Pointer(3, int32(nameConfirmBtn.X+10), int32(nameConfirmBtn.Y+10))
	}
	for i := 0; i < 30; i++ {
		s.Advance()
	}
	dumpFrame(t, s, out+"/game.png")
	fmt.Println("game text:", s.Text().Speaker, "|", s.Text().Text)
	s.Pointer(3, 1150, 700)
	dumpFrame(t, s, out+"/system.png")
	s.PressKey(27)
	s.Pointer(3, 880, 700)
	dumpFrame(t, s, out+"/backlog.png")
	s.PressKey(27)
	s.Pointer(4, 640, 300)
	dumpFrame(t, s, out+"/menu.png")
	s.PressKey(27)
	s.Pointer(3, 400, 20)
	dumpFrame(t, s, out+"/save.png")
}
