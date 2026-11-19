package core

import "runtime/debug"

// 引擎进程的 GC 参数。取值针对的是本引擎最重的负载形态：一次解码就要一块
// 十几 MB 的连续像素缓冲（1920x1080 RGBA = 8.3MB、2560x1440 = 14.7MB、
// 2760x1640 的全屏背景 = 18.1MB），而且这种分配是"点一下就来一次"的突发。
//
// gcPercent = 300：默认 GOGC=100 是"堆一翻倍就回收"。解一张大图的过程中堆史无
// 前例地翻倍，于是每次点到大图都对上一次 GC；标记要扫整堆，在低端 ARM 上就是
// 几百毫秒级的 STW / mutator assist 停顿，正好落在真机日志里"点击后 0.5~1.1 秒
// 超窗"的时间点上。本引擎的常驻大头（128MB 解码缓存、合成帧缓冲、脚本/目录索引）
// 都是长命对象，堆里真垃圾的比例很低，把阈值抬到 300 等于用多留一倍的垃圾空间
// 换取回收次数降到三分之一——对"单次 18MB 分配"这种负载，内存换停顿是划算的。
//
// gcMemLimit = 512MiB：手机进程预算内的软上限。光靠比例阈值有失控风险——一次换景
// 连解四五张大图，GOGC=300 会让堆推到几百 MB 才回收，低端机上会被系统 LMK 直接
// 杀掉。SetMemoryLimit 是软限制：堆逼近 512MiB 时 GC 主动收紧（必要时退化成连续
// 回收），既不会像硬限制那样直接致命，也把峰值锁在预算里。
const (
	gcPercent  = 300
	gcMemLimit = 512 << 20
)

// ConfigureGC 在进程启动时设置全局 GC 参数，取值理由见上面的常量注释。
// 只由 cmd/engine 的 init 调用：测试进程不该被套上 512MiB 的软上限。
func ConfigureGC() {
	debug.SetGCPercent(gcPercent)
	debug.SetMemoryLimit(gcMemLimit)
}

// ReleaseMemory 把仍在 GC 阈值窗口里的浮动垃圾还给操作系统，只在引擎打开完成
// （OpenExtras 返回、脚本与目录索引都已装载）之后调一次。那一段是进程里最集中的
// 一次性分配；之后不再定时调用，因为 FreeOSMemory 会强制一次完整 GC（STW），
// 放进游戏过程中任何一帧都会反过来变成卡顿。
func ReleaseMemory() {
	debug.FreeOSMemory()
}
