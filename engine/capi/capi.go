// Package capi 是引擎的 C ABI 实现层：句柄表、RPC 分派、合成与状态编码。
//
// 这里刻意不导出任何 `//export` 符号——那五个 FFI 入口留在各自的 cmd 包里
// （`cmd/engine` 供 Android/桌面使用，仓库根部的 `bridge` 模块把引擎与内核
// 合成一个 Apple 静态库）。原因见 bridge/cmd/bridge/main.go 的说明：
// iOS 只能静态链接，而两个独立的 Go c-archive 会把各自的运行时符号带进来，
// 链接时报重复定义。capi 不依赖 cgo 之外的东西，因此两种入口都能复用同一份逻辑，
// 分派语义、字段号与锁顺序只在这里维护一次。
package capi

/* #include <stdlib.h> */
import "C"

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"unsafe"

	"kizuna/engine/core"
	"kizuna/engine/disc"
	_ "kizuna/engine/drivers"
	"kizuna/engine/library"
	"kizuna/engine/wire"
)

const versionString = "kizuna-engine/1.0.0"

type handle struct {
	// mu 是逻辑锁：Run/Advance/Pointer/Choose/Key/Wheel/Load/Save/Text/State… 以及取用
	// h.engine 指针都在它下面，OpCompose 也从 1.0.4 起回到这把锁下（回退理由见
	// dispatchCompose）：Session.Compose 会经 syncScene/absorb/syncTable 读写 Machine
	// 与 Stage，而 Machine 的状态机没有为"被外部 op 并发调用"设计。
	mu sync.Mutex
	// composeMu 是合成的第二道锁，锁顺序恒为 composeMu → mu（dispatchCompose 与
	// KzEngineDestroy 都按这个顺序取）。mu 已经足以挡住"合成与点击 op 并行"和"销毁时
	// 合成还在跑"；留着它是因为合成的互斥不再按品牌分支，入口自己取锁比依赖调用点可靠。
	composeMu sync.Mutex
	engine    core.Engine
}

var (
	handlesMu sync.Mutex
	handles   = map[uintptr]*handle{}
)

func init() {
	installLogger(filepath.Join(os.TempDir(), "kizuna-engine.log"))
	// GC 参数只在这里设一次，作用域是整个进程，理由见 core/gc.go。
	core.ConfigureGC()
}

// Create 分配一个不透明句柄并登记到 handles 表。
//
// 句柄用 C 内存而不是 Go 指针：Dart 会长期保存这个地址，Go GC 看不到它，
// 地址必须保持稳定；同时把 C 指针交还给调用方也符合 cgo 的指针传递规则。
func Create() unsafe.Pointer {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	p := C.malloc(1)
	if p == nil {
		return nil
	}
	handlesMu.Lock()
	handles[uintptr(p)] = &handle{}
	handlesMu.Unlock()
	return p
}

// Destroy 关闭引擎并释放句柄。
func Destroy(h unsafe.Pointer) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	if h == nil {
		return
	}
	handlesMu.Lock()
	entry, ok := handles[uintptr(h)]
	delete(handles, uintptr(h))
	handlesMu.Unlock()
	if !ok {
		return
	}
	if entry != nil {
		// 锁顺序 composeMu → mu，与 dispatchCompose 一致。销毁必须把"正在跑的合成"挡在
		// Close 之外：Close() 会关掉 resource.Set 与 Stage 的解码/音频 worker，而合成还在
		// 读它们。
		entry.composeMu.Lock()
		entry.mu.Lock()
		if closer, ok := entry.engine.(core.Closer); ok && closer != nil {
			_ = closer.Close()
		}
		entry.engine = nil
		entry.mu.Unlock()
		entry.composeMu.Unlock()
	}
	C.free(h)
}

// Invoke 执行一次 RPC 并返回响应字节。调用方负责释放响应（见 cmd 包的 C.CBytes
// 包装）。panic 会被兜住并转成错误响应：让 panic 穿过 cgo 边界会直接终止宿主进程。
func Invoke(h unsafe.Pointer, request []byte) (payload []byte) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("panic: %v\n%s", r, debug.Stack())
			payload = wire.Error(0, "engine panic")
		}
	}()
	return invokeHandleBytes(h, request)
}

// invokeHandleBytes 是请求分派的实体：各入口的 cgo 薄壳只负责把 C 缓冲变成 []byte，
// 好让分派语义（含"OpCompose 也持逻辑锁"）能在不碰 cgo 的测试里被真实调用。
func invokeHandleBytes(h unsafe.Pointer, buf []byte) []byte {
	if h == nil {
		return wire.Error(0, "nil handle")
	}
	handlesMu.Lock()
	entry, ok := handles[uintptr(h)]
	handlesMu.Unlock()
	if !ok {
		return wire.Error(0, "unknown handle")
	}
	if len(buf) == 0 {
		return wire.BuildResponse(wire.StatusBadRequest, 0, nil)
	}
	op, seq, r, err := wire.ParseRequest(buf)
	if err != nil {
		return wire.BuildResponse(wire.StatusBadRequest, seq, nil)
	}
	// 合成与其余 op 一样整段持逻辑锁，见 dispatchCompose。
	if op == wire.OpCompose {
		return dispatchCompose(entry, seq, r)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return dispatch(entry, op, seq, r)
}

// dispatchCompose 是 OpCompose 的专用入口：连 composeMu 一起取，然后与其余 op 一样持
// handle.mu 整段执行合成。
//
// 1.0.2~1.0.3 这里曾按 core.ConcurrentCompose 放行并发品牌（不取 mu，合成与 Pointer/
// Advance/Run 真并发），1.0.4 回退，理由是数据竞争而不是性能偏好：
//   - Session.Compose 不是只读，它经 s.syncScene/s.absorb/stage.syncTable 写 Machine 与
//     Stage 的状态；Machine 的 Run/execGroup/状态机没有为"被外部 op 并发调用"设计，各自
//     加锁的读序列与 pointer 侧的写序列之间没有事务性；
//   - 真机证据：声明并发的品牌在 1.0.2/1.0.3 上 ES.FIRST 版权页黑屏 46~106s（1.0.1
//     同一台机器 1.7s），并伴随 "compose slow: layers=0 scene=266ms" 这种 0 层合成却要
//     几百毫秒的记录；不声明并发的 bishop/emon 完全正常。开发环境用不了 ThreadSanitizer，
//     但三品牌对照 + 回归时间线已经足够定罪；
//   - 双 isolate 的收益（UI isolate 不再被合成排队堵住）留在 Dart 侧，不需要 Go 侧用数据
//     竞争去换。
//
// 代价是点击延迟上限变成一次合成的耗时，补偿在合成本身：Stage.scene 的场景快照缓存把
// 稳态 compose 从"每帧全表重建 + 排序"降到一次键比较。
func dispatchCompose(h *handle, seq uint32, r *wire.Reader) []byte {
	// 锁顺序 composeMu → mu，必须与 KzEngineDestroy 保持一致。
	h.composeMu.Lock()
	defer h.composeMu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.engine == nil {
		return respondError(seq, wire.StatusError, "engine: not open")
	}
	return opCompose(h.engine, seq, r)
}

func dispatch(h *handle, op uint16, seq uint32, r *wire.Reader) []byte {
	switch op {
	case wire.OpOpen:
		return opOpen(h, seq, r)
	case wire.OpBrands:
		return opBrands(seq)
	case wire.OpDetect:
		return opDetect(seq, r)
	case wire.OpScan:
		return opScan(seq, r)
	}
	if h.engine == nil {
		switch op {
		case wire.OpSubmitName,
			wire.OpAudioDone,
			wire.OpMapButtons,
			wire.OpMapMask,
			wire.OpSelectItems,
			wire.OpSelectFrame,
			wire.OpComposeImages,
			wire.OpSetPreview,
			wire.OpSlotPreview,
			wire.OpCgCatalog,
			wire.OpSetOption,
			wire.OpKey,
			wire.OpWheel,
			// 主界面/影片这四条和上面几条一样是"引擎还没打开时也可能被问到"的查询：
			// 宿主在 open 完成前就会开始轮询 frame，顺手问标题菜单/影片是否存在。
			// 不在这里返回空响应的话，这些查询会得到 error 状态，
			// Dart 侧会把它当成真正的失败抛出来（启动瞬间偶发报错）。
			wire.OpTitleMenu,
			wire.OpTitleSelect,
			wire.OpMovie,
			wire.OpMovieDone:
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		return respondError(seq, wire.StatusError, "engine: not open")
	}
	e := h.engine
	switch op {
	case wire.OpInfo:
		info := e.Info()
		w := wire.NewWriter()
		w.U32(2, uint32(info.Width))
		w.U32(3, uint32(info.Height))
		w.Str(4, info.Title)
		w.U32(6, uint32(info.SceneCount))
		w.U32(7, uint32(info.EntryCount))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpStart:
		entry := r.U32(8)
		if _, err := e.Start(entry); err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.Bool(1, true)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpRun:
		budget := r.U32(9)
		step, err := e.Run(budget)
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpAdvance:
		step, err := e.Advance()
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpChoose:
		idx := r.U32(11)
		step, err := e.Choose(idx)
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpPointer:
		opCode := r.U32(12)
		x := r.I32(13)
		y := r.I32(14)
		step, err := e.Pointer(opCode, x, y)
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpScenes:
		scenes := e.Scenes()
		w := wire.NewWriter()
		w.List(18, func(sub *wire.Writer) {
			for i := range scenes {
				sub.List(0, func(item *wire.Writer) {
					item.U32(19, scenes[i].ID)
					item.Str(20, scenes[i].Name)
					item.U32(65, scenes[i].Group)
					item.U32(66, scenes[i].Index)
					item.Str(80, scenes[i].Title)
					item.StrList(81, scenes[i].Images)
					// f87 与 CG 解锁位共用同一个字段号：两者都是"鉴赏是否可见"，
					// 宿主用同一个读法处理，减少协议面。
					item.Bool(87, scenes[i].Unlocked)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSlots:
		slots := e.Slots()
		w := wire.NewWriter()
		w.List(21, func(sub *wire.Writer) {
			for i := range slots {
				sub.List(0, func(item *wire.Writer) {
					item.U32(11, slots[i].Index)
					item.Str(22, slots[i].Title)
					item.Str(24, slots[i].SavedAt)
					item.Str(63, slots[i].Summary)
					item.Bool(64, slots[i].Exists)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSave:
		slot := r.U32(11)
		title := r.Str(24)
		if err := e.Save(slot, title); err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.Bool(1, true)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpLoad:
		slot := r.U32(11)
		step, err := e.Load(slot)
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpAssetSize:
		path := r.Str(1)
		size := e.AssetSize(path)
		w := wire.NewWriter()
		w.U32(25, uint32(size))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpReadAsset:
		path := r.Str(1)
		p := r.U64(15)
		n := r.U32(16)
		written := e.ReadAsset(path, unsafe.Add(unsafe.Pointer(nil), p), int(n))
		w := wire.NewWriter()
		w.U32(25, uint32(written))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpFrame:
		return opFrame(e, seq)
	case wire.OpAudio:
		audio := e.Audio()
		w := wire.NewWriter()
		writeAudioList(w, audio)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpText:
		w := wire.NewWriter()
		writeTextField(w, e.Text())
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpChoices:
		choices := e.Choices()
		w := wire.NewWriter()
		w.List(33, func(sub *wire.Writer) {
			for i := range choices {
				sub.List(0, func(item *wire.Writer) {
					item.Str(20, choices[i].Name)
					item.I32(34, choices[i].Value)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpState:
		w := wire.NewWriter()
		writeStateFields(w, e.State())
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpTextList:
		units := e.TextList()
		w := wire.NewWriter()
		w.List(39, func(sub *wire.Writer) {
			for i := range units {
				sub.List(0, func(item *wire.Writer) {
					item.Str(19, units[i].ID)
					item.Str(40, units[i].Text)
					item.Str(41, units[i].File)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpLastError:
		msg := e.LastError()
		w := wire.NewWriter()
		w.Str(42, msg)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSubmitName:
		prompter, ok := e.(core.NamePrompter)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		value := r.Str(1)
		w := wire.NewWriter()
		w.Bool(1, prompter.SubmitName(value))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpAudioDone:
		notifier, ok := e.(core.AudioNotifier)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		notifier.NotifyAudioCompleted(r.Str(1), r.Bool(2), r.U32(3))
		w := wire.NewWriter()
		w.Bool(1, true)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpMapButtons:
		exposer, ok := e.(core.MapExposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		buttons := exposer.MapButtons()
		w := wire.NewWriter()
		w.List(51, func(sub *wire.Writer) {
			for i := range buttons {
				sub.List(0, func(item *wire.Writer) {
					item.U32(11, buttons[i].Index)
					item.Str(1, buttons[i].Image)
					item.I32(13, buttons[i].X)
					item.I32(14, buttons[i].Y)
					item.U32(2, buttons[i].Width)
					item.U32(3, buttons[i].Height)
					item.I32(34, buttons[i].Value)
					item.Str(20, buttons[i].Label)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpMapMask:
		exposer, ok := e.(core.MapExposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		has, width, height := exposer.MapMask()
		if width < 0 {
			width = 0
		}
		if height < 0 {
			height = 0
		}
		w := wire.NewWriter()
		w.Bool(1, has)
		w.U32(2, uint32(width))
		w.U32(3, uint32(height))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSelectItems:
		exposer, ok := e.(core.SelectItemExposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		items := exposer.SelectItems()
		w := wire.NewWriter()
		w.List(52, func(sub *wire.Writer) {
			for i := range items {
				sub.List(0, func(item *wire.Writer) {
					item.U32(11, items[i].Index)
					item.Str(31, items[i].Text)
					item.Str(1, items[i].Frame)
					item.I32(13, items[i].X)
					item.I32(14, items[i].Y)
					item.U32(2, items[i].Width)
					item.U32(3, items[i].Height)
					item.I32(15, items[i].TextX)
					item.I32(16, items[i].TextY)
					item.U32(83, items[i].Alignment)
					item.U32(84, items[i].DefaultColor)
					item.U32(85, items[i].SelectColor)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSelectFrame:
		composer, ok := e.(core.GalleryComposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		frame, ok := composer.SelectFrame(r.Str(1))
		return frameResponse(seq, frame, ok)
	case wire.OpComposeImages:
		composer, ok := e.(core.GalleryComposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		frame, ok := composer.ComposeImages(r.StrList(53))
		return frameResponse(seq, frame, ok)
	case wire.OpSetPreview:
		composer, ok := e.(core.GalleryComposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		width := r.U32(2)
		height := r.U32(3)
		data := r.Raw(5)
		if len(data) > 0 {
			pixels := append([]byte(nil), data...)
			swapRB(pixels)
			composer.SetPreview(core.Frame{Width: int(width), Height: int(height), Pixels: pixels})
		} else {
			composer.SetPreview(core.Frame{})
		}
		w := wire.NewWriter()
		w.Bool(1, true)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSlotPreview:
		previewer, ok := e.(core.SlotPreviewer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		preview, ok := previewer.PreviewSlot(r.U32(11))
		w := wire.NewWriter()
		w.Str(24, preview.SavedAt)
		w.Str(63, preview.Summary)
		w.Str(30, preview.Speaker)
		w.Str(31, preview.Text)
		if ok && preview.Thumb.Width > 0 && preview.Thumb.Height > 0 && len(preview.Thumb.Pixels) > 0 {
			pixels := append([]byte(nil), preview.Thumb.Pixels...)
			swapRB(pixels)
			w.U32(2, uint32(preview.Thumb.Width))
			w.U32(3, uint32(preview.Thumb.Height))
			w.Raw(5, pixels)
		} else {
			w.U32(2, 0)
			w.U32(3, 0)
		}
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpCgCatalog:
		composer, ok := e.(core.GalleryComposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		groups := composer.CgGroups()
		// 解锁状态单独取：它来自原引擎的全局存档，和"目录里有哪些 CG"是两份数据。
		// 引擎没实现 GalleryUnlocker（旧品牌）时按"全部解锁"处理，
		// 保持鉴赏界面可用而不是整片变灰。
		unlocker, hasUnlock := e.(core.GalleryUnlocker)
		w := wire.NewWriter()
		w.List(86, func(sub *wire.Writer) {
			for i := range groups {
				sub.List(0, func(group *wire.Writer) {
					group.Str(20, groups[i].Name)
					group.List(54, func(variants *wire.Writer) {
						for _, variant := range groups[i].Variants {
							variants.List(0, func(entry *wire.Writer) {
								entry.StrList(56, variant)
								// 解锁判定按"变体里任意一张图解锁即可见"：
								// 差分图常常是同一张 CG 的不同状态，玩家看过其中一版
								// 就说明这一幕已经解锁，全锁着会让鉴赏目录大半是空的。
								visible := true
								if hasUnlock {
									visible = false
									for _, name := range variant {
										if unlocker.CgUnlockedByName(name) {
											visible = true
											break
										}
									}
								}
								entry.Bool(87, visible)
							})
						}
					})
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpSetOption:
		setter, ok := e.(core.OptionSetter)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		setter.SetOption(r.U32(57), r.Bool(7))
		w := wire.NewWriter()
		w.Bool(1, true)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpKey:
		receiver, ok := e.(core.KeyReceiver)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		step, err := receiver.PressKey(r.U32(58))
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpWheel:
		receiver, ok := e.(core.WheelReceiver)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		step, err := receiver.Wheel(r.I32(59))
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpTitleMenu:
		exposer, ok := e.(core.TitleMenuExposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		// 编码成"一个 f60 列表，内含若干 f0 子列表"，与 CgCatalog（op 33）一致。
		//
		// 不能把每个菜单项各写一个 f60：Reader 用 map 按 id 存字段，
		// 同 id 的后写会覆盖先写，最终只剩最后一项（实测解码结果是 0 项——
		// 因为保留下来的那个 f60 里装的是 Str/I32 而不是子列表）。
		// 后果是宿主拿不到菜单，标题层的点击分派整层失效。
		w := wire.NewWriter()
		w.List(60, func(outer *wire.Writer) {
			for _, item := range exposer.TitleMenu() {
				outer.List(0, func(entry *wire.Writer) {
					entry.Str(61, item.Key)
					entry.Str(62, item.Label)
					entry.I32(63, item.X)
					entry.I32(64, item.Y)
					entry.U32(65, item.Width)
					entry.U32(66, item.Height)
					entry.Bool(67, item.Enabled)
				})
			}
		})
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpTitleSelect:
		exposer, ok := e.(core.TitleMenuExposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		step, err := exposer.SelectTitleItem(r.Str(61))
		if err != nil {
			return respondError(seq, wire.StatusError, err.Error())
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpMovie:
		exposer, ok := e.(core.MovieExposer)
		if !ok {
			return wire.BuildResponse(wire.StatusOK, seq, nil)
		}
		w := wire.NewWriter()
		w.Str(68, exposer.CurrentMovie())
		w.U32(69, uint32(exposer.MovieSerial()))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	case wire.OpMovieDone:
		// 宿主播完影片后回调。清状态与"接下来往哪走"都由引擎一次做完
		// （FinishMovie 内部幂等地清掉影片状态），宿主不需要知道有两种结局。
		var step core.Step
		if finisher, ok := e.(core.MovieFinisher); ok {
			var err error
			step, err = finisher.FinishMovie()
			if err != nil {
				return respondError(seq, wire.StatusError, err.Error())
			}
		} else {
			var err error
			step, err = e.Advance()
			if err != nil {
				return respondError(seq, wire.StatusError, err.Error())
			}
		}
		w := wire.NewWriter()
		w.U32(10, uint32(step))
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	default:
		return wire.BuildResponse(wire.StatusBadRequest, seq, nil)
	}
}

// opFrame 是"一次 RPC 取齐"：step + text + state + audio 事件列表合成一个响应。
// 宿主每次 _sync 原本要 text/state/audio 等 4~6 个 RPC，每个都要一次 isolate 往返 +
// handle.mu；合并后只剩一次。字段号全部复用旧 op 的字段号（step=f10，text=f30/f31/f32，
// state=f35..f79，audio 列表=f26），所以 Dart 侧用同一套读法，旧 op 也照旧保留。
// 这里刻意不返回 Choices/SelectItems/Map*：只有特定 step 才需要，宿主仍按 step 条件调旧 op。
// 注意 Audio() 是 drain 语义（取走即清空），一次响应里的 audio 列表从某个时刻起是完整的；
// 与 OpAudio 的另一个 drain 点谁先取到都不会丢事件，两边都把事件交给宿主同一个播放队列。
func opFrame(e core.Engine, seq uint32) []byte {
	// 没有实现 StepReporter 的品牌（bishop/emon）写 StepContinue：它们没有"当前步"这个
	// 无副作用读法，宿主那侧仍以 advance/pointer 的返回值为准。
	step := core.StepContinue
	if reporter, ok := e.(core.StepReporter); ok {
		step = reporter.CurrentStep()
	}
	w := wire.NewWriter()
	w.U32(10, uint32(step))
	writeTextField(w, e.Text())
	writeStateFields(w, e.State())
	writeAudioList(w, e.Audio())
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

// bgraComposer 由"Compose 直接往 dst 写 BGRA"的品牌实现（见
// Session.ComposeWritesBGRA）。opCompose 的 dst 是宿主（Dart）交给本 op 独占的缓冲：
// 只有 Compose 里的 scene.Clear 与 drawLayer 往里写，缓冲的读者是宿主，读到的字节序
// 必须始终是 BGRA。所以声明了 BGRA 的品牌要跳过下面那趟换位遍历，否则颜色会被换回去。
type bgraComposer interface {
	ComposeWritesBGRA() bool
}

func composesBGRA(e core.Engine) bool {
	b, ok := e.(bgraComposer)
	return ok && b != nil && b.ComposeWritesBGRA()
}

// opCompose 是 OpCompose 的实际执行体，被 dispatchCompose 的两条路径（并发 / 持 mu）
// 共用，字段与错误路径与旧实现完全一致。
func opCompose(e core.Engine, seq uint32, r *wire.Reader) []byte {
	p := r.U64(15)
	n := r.U32(16)
	dst := unsafe.Add(unsafe.Pointer(nil), p)
	dirty, err := e.Compose(dst, int(n))
	if err != nil {
		return respondError(seq, wire.StatusError, err.Error())
	}
	// dirty=false 时 session 的 Compose 早退且完全不写 dst，此时再全量扫一遍
	// 做 RGBA→BGRA 交换纯属浪费：这正是每帧 3.7MB 无用内存流量的来源之一。
	// 写 BGRA 的品牌同样跳过：它写出来的就是宿主读的字节序。
	if dirty && !composesBGRA(e) {
		info := e.Info()
		if dst != nil && info.Width > 0 && info.Height > 0 {
			limit := info.Width * info.Height * 4
			if limit <= int(n) {
				swapRB(unsafe.Slice((*byte)(dst), limit))
			}
		}
	}
	w := wire.NewWriter()
	w.Bool(17, dirty)
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

// writeTextField / writeStateFields / writeAudioList 是 OpText/OpState/OpAudio 与
// OpFrame 共用的字段编码，保证两条路径的字段号不会各自漂移。
func writeTextField(w *wire.Writer, t core.TextState) {
	w.Str(30, t.Speaker)
	w.Str(31, t.Text)
	w.U32(32, t.Awaiting)
}

func writeStateFields(w *wire.Writer, st core.State) {
	w.Str(35, st.Bgm)
	w.U32(36, st.PC)
	w.U32(37, st.Scene)
	w.Bool(38, st.Animating)
	w.U32(45, st.WaitMs)
	w.Bool(46, st.MessageWindowVisible)
	w.Bool(82, st.EngineMessageWindow)
	w.Bool(47, st.AwaitingName)
	w.Bool(48, st.BackLogActive)
	w.Bool(49, st.ReplayActive)
	w.U32(60, st.InstructionStart)
	w.Str(79, st.NamePrompt)
	// 宿主动作：引擎打开不了 Flutter 的设置页，也关不掉播放页，只能把请求放进
	// 状态快照里。Serial 让宿主能区分"新请求"和"每帧都在读到的同一个请求"。
	w.Str(80, st.HostRequest)
	w.U32(81, st.HostRequestSerial)
	// 88..91 是给宿主的自动推进/跳过策略：未读标记与设置必须一起到达，
	// 否则「跳过遇到未读停下」会在设置读取前后出现一帧误判。
	// 83..87 已被选择肢样式/鉴赏解锁占用，不能复用。
	w.Bool(88, st.MessageUnread)
	w.Bool(89, st.CeaseSkip)
	w.Bool(90, st.SkipUnread)
	w.U32(91, uint32(st.AutoWait))
}

func writeAudioList(w *wire.Writer, audio []core.AudioEvent) {
	w.List(26, func(sub *wire.Writer) {
		for i := range audio {
			sub.List(0, func(item *wire.Writer) {
				item.U32(27, audio[i].Kind)
				item.Str(1, audio[i].Path)
				item.F64(28, audio[i].Volume)
				item.Bool(29, audio[i].Loop)
				item.U32(3, audio[i].Channel)
				item.U32(69, audio[i].Group)
				item.StrList(70, audio[i].Names)
				item.U32(71, audio[i].Frames)
			})
		}
	})
}

func frameResponse(seq uint32, frame core.Frame, ok bool) []byte {
	w := wire.NewWriter()
	if !ok || frame.Width <= 0 || frame.Height <= 0 || len(frame.Pixels) == 0 {
		w.U32(2, 0)
		w.U32(3, 0)
		return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
	}
	pixels := append([]byte(nil), frame.Pixels...)
	swapRB(pixels)
	w.U32(2, uint32(frame.Width))
	w.U32(3, uint32(frame.Height))
	w.Raw(5, pixels)
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

func swapRB(b []byte) {
	for i := 0; i+3 < len(b); i += 4 {
		b[i], b[i+2] = b[i+2], b[i]
	}
}

func opOpen(h *handle, seq uint32, r *wire.Reader) []byte {
	dir := r.Str(1)
	id := r.Str(44)
	extraDirs := r.StrList(7)
	cacheDir := r.Str(10)
	if cacheDir != "" {
		installLogger(filepath.Join(cacheDir, "engine.log"))
	}
	// 光盘镜像在宿主的游戏库里只是一个路径：真正进入游戏时才流式解包。
	// 解包后所有盘合并到同一个目录，Bishop 的 Graphics2.Inx/Graphics2.Snn
	// 分离在不同光盘时也能重新配对。
	if mounted, ok, err := maybeMountDisc(dir, cacheDir); err != nil {
		return respondError(seq, wire.StatusError, err.Error())
	} else if ok {
		core.Logf("engine: disc image '%s' mounted at '%s'", dir, mounted)
		dir = mounted
		// 多光盘安装的资源可能落在不同子目录，统一交给引擎作为额外资源根。
		if data, ok := library.FindDataDirectory(dir); ok {
			for _, extra := range library.CompanionDataDirs(dir, data) {
				if !containsFold(extraDirs, extra) && !strings.EqualFold(extra, dir) {
					extraDirs = append(extraDirs, extra)
				}
			}
		}
	}
	var e core.Engine
	if id != "" {
		b, ok := core.Find(id)
		if !ok {
			return respondError(seq, wire.StatusError, "engine: unknown brand: "+id)
		}
		e = b.New()
	} else {
		b, ok := core.Detect(dir)
		if !ok {
			return respondError(seq, wire.StatusNotFound, "engine: no brand matches directory: "+dir)
		}
		e = b.New()
	}
	h.engine = e
	var (
		info core.Info
		err  error
	)
	if extras, ok := e.(core.OpenerExtras); ok {
		info, err = extras.OpenExtras(dir, extraDirs, cacheDir)
	} else {
		info, err = e.Open(dir)
	}
	if err != nil {
		return respondError(seq, wire.StatusError, err.Error())
	}
	// 引擎打开完成（脚本、配置、目录索引都已装载）：这是进程里最集中的一次性分配
	// 段，此时把浮动垃圾交还给 OS，进游戏后就不再需要完整的 STW 回收，见 core/gc.go。
	core.ReleaseMemory()
	w := wire.NewWriter()
	w.Bool(1, true)
	w.U32(2, uint32(info.Width))
	w.U32(3, uint32(info.Height))
	w.Str(4, info.Title)
	w.Str(5, info.Version)
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

func containsFold(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// maybeMountDisc 只在“目录里没有可直接游玩的脚本/归档”时才解包镜像。
// 用户常常同时保留镜像和已解包目录；优先用已解包内容，既能省掉几分钟的解包，
// 也允许用户随时删掉镜像。
func maybeMountDisc(dir, cacheDir string) (string, bool, error) {
	if !disc.IsContainer(dir) {
		return dir, false, nil
	}
	if library.HasPlayableData(dir) {
		return dir, false, nil
	}
	return disc.Prepare(dir, cacheDir)
}

func opScan(seq uint32, r *wire.Reader) []byte {
	entries := library.Scan(r.Str(1))
	w := wire.NewWriter()
	w.List(71, func(sub *wire.Writer) {
		for i := range entries {
			sub.List(0, func(item *wire.Writer) {
				item.Str(19, entries[i].BrandID)
				item.Str(20, entries[i].BrandName)
				item.Str(72, entries[i].Name)
				item.Str(73, entries[i].Directory)
				item.Str(74, entries[i].DataDirectory)
				item.Str(75, entries[i].CoverPath)
				item.StrList(76, entries[i].ExtraDirs)
				item.Str(77, entries[i].ScriptPath)
				item.Bool(78, entries[i].HasScript)
			})
		}
	})
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

func opBrands(seq uint32) []byte {
	brands := core.Brands()
	w := wire.NewWriter()
	w.List(18, func(sub *wire.Writer) {
		for i := range brands {
			sub.List(0, func(item *wire.Writer) {
				item.Str(19, brands[i].ID())
				item.Str(20, brands[i].Name())
			})
		}
	})
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

func opDetect(seq uint32, r *wire.Reader) []byte {
	dir := r.Str(1)
	w := wire.NewWriter()
	b, ok := core.Detect(dir)
	if ok {
		w.Str(19, b.ID())
		w.Str(20, b.Name())
	} else {
		w.Str(19, "")
		w.Str(20, "")
	}
	return wire.BuildResponse(wire.StatusOK, seq, w.Bytes())
}

func respondError(seq uint32, status uint16, msg string) []byte {
	w := wire.NewWriter()
	w.Str(42, msg)
	return wire.BuildResponse(status, seq, w.Bytes())
}

// Version 返回引擎版本串，形如 kizuna-engine/1.0.0。
func Version() string { return versionString }
