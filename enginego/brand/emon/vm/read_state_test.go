package vm

import "testing"

type memoryReads struct{ read map[string]bool }

func newMemoryReads() *memoryReads            { return &memoryReads{read: map[string]bool{}} }
func (r *memoryReads) IsRead(key string) bool { return r.read[key] }
func (r *memoryReads) MarkRead(key string)    { r.read[key] = true }
func (r *memoryReads) Flush()                 {}

// TestReadStateDrivesSkipModeFlag 覆盖既読/未読的引擎侧行为：
//   - 首次出现的正文对宿主是未读（MessageUnread=true），并写入已读库；
//   - 再次执行同一行时 SkipModeFlag 带 0x10（既読アイコン）；
//   - 宿主 SKIP/AUTO 开关进入 0x02/0x04 位（原脚本据此切换 SKIP/AUTO 图标）。
func TestReadStateDrivesSkipModeFlag(t *testing.T) {
	store := newMemoryReads()
	body := "本文テスト\n#Page()\n#End()\n"

	first := newTestMachine()
	first.Context().Reads = store
	if step := runBody(t, first, "read-state", body); step != EmonStepAwaitInput {
		t.Fatalf("first step = %d, want await input", int(step))
	}
	if !first.MessageUnread() {
		t.Fatal("first display must be unread")
	}
	if !store.read["read-state:1"] {
		t.Fatalf("read store not marked: %#v", store.read)
	}
	if got := first.Lookup("SkipModeFlag").AsInt(); got != 0 {
		t.Fatalf("first SkipModeFlag = %d, want 0", got)
	}

	second := newTestMachine()
	second.Context().Reads = store
	if step := runBody(t, second, "read-state", body); step != EmonStepAwaitInput {
		t.Fatalf("second step = %d, want await input", int(step))
	}
	if second.MessageUnread() {
		t.Fatal("second display must be read")
	}
	if got := second.Lookup("SkipModeFlag").AsInt(); got&0x10 == 0 {
		t.Fatalf("second SkipModeFlag = %#x, want bit 0x10", got)
	}

	second.SetHostSkip(true)
	if got := second.Lookup("SkipModeFlag").AsInt(); got&0x02 == 0 {
		t.Fatalf("skip flag = %#x, want bit 0x02", got)
	}
	second.SetHostAuto(true)
	if got := second.Lookup("SkipModeFlag").AsInt(); got&0x04 == 0 {
		t.Fatalf("auto flag = %#x, want bit 0x04", got)
	}
}
