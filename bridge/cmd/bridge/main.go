// Command bridge 是 Apple 平台（iOS / macOS）唯一导出的 FFI 入口，
// 把引擎（kizuna/engine）和内核（kizuna/kernel）的 C ABI 合并进同一个 Go 运行时。
//
// 为什么不直接链接 engine 与 kernel 两个 c-archive：
//   - iOS 只支持 -buildmode=c-archive，c-shared 在 ios/arm64 上被 Go 明确拒绝；
//   - 每个 c-archive 都自带一份 Go 运行时（_cgo_init、runtime.* 等符号），
//     两个 archive 同时链进 Runner.app 会报一堆 duplicate symbol。
//
// 因此这里做一个“组合根”：同一个 package main 同时 import 两个模块的库代码，
// 链接后只有一份运行时，符号表里同时存在 KzEngine* 与 Kz*。macOS 也用同一个
// 静态库（universal），这样 Dart 侧只需要 DynamicLibrary.process()，
// 不需要嵌入 dylib，也不需要为嵌套二进制单独签名。
package main

/* #include <stdlib.h> */
import "C"

import (
	"unsafe"

	"kizuna/engine/capi"
	_ "kizuna/kernel/drivers"
	"kizuna/kernel/shell"
)

// ---- engine ----

//export KzEngineCreate
func KzEngineCreate() unsafe.Pointer {
	return capi.Create()
}

//export KzEngineDestroy
func KzEngineDestroy(h unsafe.Pointer) {
	capi.Destroy(h)
}

//export KzEngineInvoke
func KzEngineInvoke(h unsafe.Pointer, req unsafe.Pointer, reqLen C.int, outLen *C.int) unsafe.Pointer {
	var request []byte
	if req != nil && reqLen > 0 {
		request = unsafe.Slice((*byte)(req), int(reqLen))
	}
	response := capi.Invoke(h, request)
	if outLen != nil {
		*outLen = C.int(len(response))
	}
	return C.CBytes(response)
}

//export KzEngineFree
func KzEngineFree(p unsafe.Pointer) {
	if p != nil {
		C.free(p)
	}
}

//export KzEngineVersion
func KzEngineVersion() *C.char {
	return C.CString(capi.Version())
}

// ---- kernel ----

//export KzInvoke
func KzInvoke(request *C.char) (out *C.char) {
	// 不能让 panic 穿过 cgo 边界：那会直接终止宿主应用。
	defer func() {
		if recover() != nil {
			out = C.CString(`{"ok":false,"error":"panic"}`)
		}
	}()
	if request == nil {
		return C.CString(`{"ok":false,"error":"nil request"}`)
	}
	return C.CString(shell.Invoke(C.GoString(request)))
}

//export KzVersion
func KzVersion() *C.char {
	return C.CString(shell.Version)
}

//export KzFree
func KzFree(ptr *C.char) {
	C.free(unsafe.Pointer(ptr))
}

func main() {}
