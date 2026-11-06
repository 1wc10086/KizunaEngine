package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	_ "kizuna/kernel/drivers"
	"kizuna/kernel/shell"
)

//export KzInvoke
func KzInvoke(request *C.char) (out *C.char) {
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
