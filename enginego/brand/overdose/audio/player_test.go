package audio

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/brand/overdose/arc"
)

// 测试都只查真实游戏文件，文件不在时整轮跳过，
// 与 arc 包的 arc_test.go 保持同一套目录发现逻辑。
var gameDirCandidates = []string{
	os.Getenv("GAME_DIR"),
	"/root/code/engine/游戏/YUI/優衣",
	"../../../engine/游戏/YUI/優衣",
	"./testdata",
}

func gameDir(t *testing.T) string {
	t.Helper()
	for _, c := range gameDirCandidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "voice.arc")); err == nil {
			return c
		}
	}
	t.Skip("未找到游戏数据目录（设置 GAME_DIR）")
	return ""
}

// 物化已知小语音条目：头四字节必须是 OggS，且第二次物化返回同一路径
// （复用逻辑）。选 kyo_0442.ogg 是因为它是 voice.arc 里最小的一条
// （约 9KB），既验证完整链路又不让测试变慢。
func TestMaterializeSmallVoice(t *testing.T) {
	dir := gameDir(t)
	set, err := arc.OpenSet(dir)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}
	defer set.Close()

	cache := t.TempDir()
	p := New(set, filepath.Join(cache, "audio"))

	// 任务要求的是 <cacheDir>/<gamehash>/audio/<archive>/<name>.ogg；
	// 这里把 cacheDir/<gamehash> 由调用方传入 test 的 TempDir 模拟，
	// Player 只负责 audio/ 之后的路径拼接。
	first, err := p.Materialize("voice", "kyo_0442.ogg")
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if first != filepath.Join(cache, "audio", "voice", "kyo_0442.ogg") {
		t.Fatalf("materialized path = %q, want 含 voice/kyo_0442.ogg 的稳定路径", first)
	}
	// 稳定路径：再次物化（这次名字故意省略 .ogg）必须返回同一路径，
	// 证明 look-up 兜底与“已存在复用”都落在同一目标上。
	second, err := p.Materialize("voice", "kyo_0442")
	if err != nil {
		t.Fatalf("Materialize（无后缀）: %v", err)
	}
	if second != first {
		t.Fatalf("重复物化路径不一致: %q vs %q", first, second)
	}
	// 文件内容：OggS 头必备（宿主播不了非 OggS 的裸字节）。
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read %s: %v", first, err)
	}
	if len(raw) < 4 || string(raw[:4]) != "OggS" {
		t.Fatalf("materialized %d bytes head %q, want OggS", len(raw), raw[:min(4, len(raw))])
	}
	if len(raw) == 0 {
		t.Fatalf("materialized empty")
	}

	// 绝对路径断言：宿主播放只认绝对路径，相对路径会让不同工作目录
	// 解读出两个位置，静音的问题难以定位。
	abs, err := filepath.Abs(first)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if first != abs {
		// 临时目录本身多半已是绝对路径；不一致就说明 Player 给的是
		// 相对拼接，需要在调用方统一转绝对，测试里把这个契约钉住。
		t.Fatalf("path %q not absolute (want %q)", first, abs)
	}
}

func TestSanitizeName(t *testing.T) {
	// 路径穿越必须在参数进文件系统前拦下：首位圆点、../、反斜杠与
	// 空串都不能产生可用的写盘路径。
	cases := []struct{ in, want string }{
		{"kyo_0442.ogg", "kyo_0442.ogg"},
		{"bgm01.ogg", "bgm01.ogg"},
		{"../evil.ogg", "evil.ogg"},
		{`a\b.ogg`, "b.ogg"},
		{".hidden.ogg", "hidden.ogg"},
		{"", ""},
	}
	for _, c := range cases {
		if got := SanitizeName(c.in); got != c.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 危险字符应被换成下划线而不是报错-吞掉：否则调用方无法区分失败。
	if got := SanitizeName("a/b:c*.ogg"); got != "b_c_.ogg" {
		// 此例中 LastIndexAny('/') 短路到 b:c*.ogg，后面 ':' 与 '*'
		// 换成 '_'，得到 b_c_.ogg
		t.Errorf("SanitizeName 换行不符: %q", got)
	}
}

func TestPlayerPlayStopVolume(t *testing.T) {
	// 不依赖游戏数据：物化失败时 Player 会透传原始名字，保证 Play
	// 始终发出事件；这里只断言事件形状与状态机行为。
	p := New(nil, t.TempDir())

	bgmCh := Channel{Kind: CategoryMusic, Idx: 0}
	p.Play(PlayRequest{Channel: bgmCh, Name: "bgm01", Loop: true, Volume: 100, Fade: 500})
	if got := p.Events(); len(got) != 1 || got[0].Kind != KindBGM || !got[0].Loop || got[0].Volume != 1.0 {
		t.Fatalf("play events = %+v, want 一条 KindBGM", got)
	}
	// 同通道新文件覆盖旧文件（换歌不是叠加）。
	p.Play(PlayRequest{Channel: bgmCh, Name: "bgm02", Loop: false, Volume: 80})
	ev := p.Events()
	if len(ev) != 1 || ev[0].Kind != KindBGM || ev[0].Volume != 0.8 {
		t.Fatalf("replace events = %+v", ev)
	}

	// 音量 0..100 钳制：超出静音、超过满格都是调用方传错，最坏情况是
	// 宿主静音。这里验证 0 与 200 的两端。
	p.SetVolume(bgmCh, 200)
	if got := p.Events(); len(got) != 1 || got[0].Kind != KindVolume || got[0].Volume != 1.0 {
		t.Fatalf("volume clamp up = %+v", got)
	}
	p.SetVolume(bgmCh, -5)
	if got := p.Events(); len(got) != 1 || got[0].Kind != KindVolume || got[0].Volume != 0 {
		t.Fatalf("volume clamp down = %+v", got)
	}

	// Stop 发停止事件且清槽；再次 Tick 不补发。
	p.Stop(bgmCh)
	if got := p.Events(); len(got) != 1 || got[0].Kind != KindStopBGM {
		t.Fatalf("stop = %+v", got)
	}
	if got := p.Events(); len(got) != 0 {
		t.Fatalf("events after drain = %+v", got)
	}
}

func TestPlayerVoiceAndFadeOut(t *testing.T) {
	p := New(nil, t.TempDir())
	voiceCh := Channel{Kind: CategoryVoice, Idx: 2}
	p.Play(PlayRequest{Channel: voiceCh, Name: "miy_0792", Loop: false, Volume: 100})
	ev := p.Events()
	if len(ev) != 1 || ev[0].Kind != KindVoice {
		t.Fatalf("voice play = %+v", ev)
	}

	// FadeOut 先发 KindFadeOut，Tick 跨过 fadeMs 后再补 KindStopVoice。
	p.FadeOut(voiceCh, 300)
	got := p.Events()
	if len(got) != 1 || got[0].Kind != KindFadeOut || got[0].Frames != 300 {
		t.Fatalf("fadeOut = %+v", got)
	}
	p.Tick(100)
	if got := p.Events(); len(got) != 0 {
		t.Fatalf("fadeOut 提前补 stop: %+v", got)
	}
	p.Tick(200)
	if got := p.Events(); len(got) != 1 || got[0].Kind != KindStopVoice {
		t.Fatalf("fadeOut 到期补 stop = %+v", got)
	}

	// 补 stop 后再次 Tick 不重复发送（槽位已清）。
	p.Tick(1000)
	if got := p.Events(); len(got) != 0 {
		t.Fatalf("重复补 stop: %+v", got)
	}

	// SE 与 StopAll 的形状
	seCh := Channel{Kind: CategorySound, Idx: 0}
	p.Play(PlayRequest{Channel: seCh, Name: "se_001", Volume: 50})
	_ = p.Events()
	p.StopAll()
	if got := p.Events(); len(got) != 1 || got[0].Kind != KindStopAll {
		t.Fatalf("stopAll = %+v", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
