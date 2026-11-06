module kizuna/bridge

go 1.27.2

// Apple 静态库的组合根：只给 iOS / macOS 的 c-archive 使用。
// 两个 replace 让 bridge 直接复用工作区里的 engine / kernel 源码，
// 不需要发布模块版本，也不会复制业务代码。
require (
	kizuna/engine v0.0.0
	kizuna/kernel v0.0.0
)

require (
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace kizuna/engine => ../engine

replace kizuna/kernel => ../kernel
