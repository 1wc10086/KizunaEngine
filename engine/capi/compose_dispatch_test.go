package capi

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"kizuna/engine/core"
	"kizuna/engine/wire"
)

// 这组测试钉住 1.0.4 的分派语义：OpCompose 与其余 op 一样整段持 handle.mu。
// 1.0.2/1.0.3 里声明了 core.ConcurrentCompose 的品牌可以绕开这把锁，让合成与
// Pointer/Advance/Run 真并发；真机后果是 ES.FIRST 版权页黑屏 46~106s（Compose 会经
// syncScene/absorb/syncTable 写 Machine 与 Stage，而 Machine 的状态机不是为外部并发
// 调用设计的）。core.ConcurrentCompose 已随之删除，这里只验证互斥本身。

// lockProbeEngine 用 TryLock 探测"Compose 运行时 handle.mu 是否被持有"：拿得到锁说明
// 逻辑锁被放开了，即并发分派的老语义。
type lockProbeEngine struct {
	core.Engine
	mu          *sync.Mutex
	composeSeen bool
	muHeld      bool
}

func (e *lockProbeEngine) Info() core.Info { return core.Info{Width: 4, Height: 4} }

func (e *lockProbeEngine) Compose(dst unsafe.Pointer, n int) (bool, error) {
	e.composeSeen = true
	e.muHeld = !e.mu.TryLock()
	if !e.muHeld {
		e.mu.Unlock()
	}
	return false, nil
}

// blockingEngine 的 Compose 停在 entered 上等 release，用来观察"合成还没结束时别的 op
// 能不能进引擎"。active 记录引擎调用的重入情况：>1 就是真的并发执行了。
type blockingEngine struct {
	core.Engine
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	active      atomic.Int32
	overlap     atomic.Bool
	advanced    atomic.Bool
}

func (e *blockingEngine) enter() {
	if e.active.Add(1) != 1 {
		e.overlap.Store(true)
	}
}

func (e *blockingEngine) leave() { e.active.Add(-1) }

func (e *blockingEngine) Compose(dst unsafe.Pointer, n int) (bool, error) {
	e.enter()
	defer e.leave()
	close(e.entered)
	<-e.release
	return false, nil
}

func (e *blockingEngine) Advance() (core.Step, error) {
	e.enter()
	defer e.leave()
	e.advanced.Store(true)
	return core.StepContinue, nil
}

func (e *blockingEngine) unblock() {
	e.releaseOnce.Do(func() { close(e.release) })
}

func composePayload() []byte {
	w := wire.NewWriter()
	w.U64(15, 0)
	w.U32(16, 0)
	return w.Bytes()
}

func composeReader(t *testing.T) *wire.Reader {
	t.Helper()
	return wire.NewReader(composePayload())
}

func composeStatus(t *testing.T, resp []byte) uint16 {
	t.Helper()
	if len(resp) < 16 || string(resp[:4]) != "KZES" {
		t.Fatalf("bad response: %q", resp)
	}
	return binary.LittleEndian.Uint16(resp[4:6])
}

func TestDispatchComposeHoldsLogicalLock(t *testing.T) {
	h := &handle{}
	e := &lockProbeEngine{mu: &h.mu}
	h.engine = e
	if status := composeStatus(t, dispatchCompose(h, 1, composeReader(t))); status != wire.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if !e.composeSeen {
		t.Fatal("Compose was not called")
	}
	if !e.muHeld {
		t.Fatal("handle.mu was not held while Compose ran")
	}
}

// TestComposeExcludesOtherOps 从真正的外部入口 invokeHandleBytes 进：合成把 handle.mu
// 握在手里不放时，Advance 必须进不来；合成一结束，它才被放行。
func TestComposeExcludesOtherOps(t *testing.T) {
	e := &blockingEngine{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(e.unblock)
	var key byte
	ptr := unsafe.Pointer(&key)
	handlesMu.Lock()
	handles[uintptr(ptr)] = &handle{engine: e}
	handlesMu.Unlock()
	t.Cleanup(func() {
		handlesMu.Lock()
		delete(handles, uintptr(ptr))
		handlesMu.Unlock()
	})

	composeDone := make(chan []byte, 1)
	go func() { composeDone <- invokeHandleBytes(ptr, buildSmokeRequest(wire.OpCompose, 1, composePayload())) }()

	select {
	case <-e.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Compose never started")
	}

	advanceDone := make(chan []byte, 1)
	go func() { advanceDone <- invokeHandleBytes(ptr, buildSmokeRequest(wire.OpAdvance, 2, nil)) }()

	select {
	case resp := <-advanceDone:
		t.Fatalf("Advance returned while Compose was running (status=%d)", composeStatus(t, resp))
	case <-time.After(150 * time.Millisecond):
	}
	if e.advanced.Load() {
		t.Fatal("Advance reached the engine while Compose held the logical lock")
	}

	e.unblock()
	if resp := <-composeDone; composeStatus(t, resp) != wire.StatusOK {
		t.Fatalf("compose status=%d", composeStatus(t, resp))
	}
	select {
	case resp := <-advanceDone:
		if status := composeStatus(t, resp); status != wire.StatusOK {
			t.Fatalf("advance status=%d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Advance did not run after Compose finished")
	}
	if !e.advanced.Load() {
		t.Fatal("Advance never reached the engine")
	}
	if e.overlap.Load() {
		t.Fatal("engine methods overlapped: ops must be serialized")
	}
}

func TestDispatchComposeWithoutEngine(t *testing.T) {
	resp := dispatchCompose(&handle{}, 3, composeReader(t))
	if status := composeStatus(t, resp); status != wire.StatusError {
		t.Fatalf("status=%d", status)
	}
	r := wire.NewReader(resp[16 : 16+binary.LittleEndian.Uint32(resp[12:16])])
	if r.Str(42) == "" {
		t.Fatal("empty error message")
	}
}
