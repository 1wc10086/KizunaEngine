// Command engine 是引擎的 FFI 入口：只负责 C 指针与 Go 切片的转换，
// 真正的句柄表与 RPC 分派在 kizuna/engine/capi 里。
//
// 为什么入口要拆成这么薄的一层：
//   - Android / Linux / Windows 各自加载独立的 libengine.so / engine.dll；
//   - Apple 平台（iOS 只能 c-archive）需要把引擎和内核编进同一个静态库，
//     见仓库根部 bridge/ 模块。cgo 的 //export 只能写在 package main 里，
//     所以两个入口各留一份薄壳，逻辑共用 capi，避免字段号与锁语义漂移。
package main

/* #include <stdlib.h> */
import "C"

import (
	"unsafe"

	"kizuna/engine/capi"
)

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
		// 这里不复制：capi.Invoke 在同一次同步调用内读完请求，
		// 返回前 Dart 侧不会改动这块内存。
		request = unsafe.Slice((*byte)(req), int(reqLen))
	}
	response := capi.Invoke(h, request)
	if outLen != nil {
		*outLen = C.int(len(response))
	}
	// C.CBytes 把响应复制到 C 堆：Dart 在 FFI 返回后才读取，必须独立于 Go 堆。
	// 调用方用 KzEngineFree 释放。
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

func main() {}
