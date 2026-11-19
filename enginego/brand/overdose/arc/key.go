package arc

import (
	"encoding/binary"
)

// 流密码与密钥表（overdose_spec §2.1）：
// 调用方（存档读取）只依赖 GenKeyTable 的表值，不碰生成过程的内部状态，
// 所以这里把“种子 -> 256 个 dword”的完整计算都收敛在本文件。
//
// 为什么不用公式而要逐条移植原函数：
//  1. 简单状态（第 1 段，512 个 dword）是可复述 closed form 的，
//     我们也已经用 unicorn 对校验过（见下文 stateInit）。
//  2. 后续的 512 字扩展与 256 字洗牌在 yui.exe 里是同一套 64 位加/旋转/异或
//     链（sub_40BE4B / sub_40C3FF），被多个入口共享，且全部靠进位链
//     （adc/sbb/shld/shrd）工作，幂等、无分支投机；因此本文件把这两段直行
//     指令逐条降为 Go 语句。手抄 closed form 极易在某个进位上写错，
//     且错了只表现为“解出坏 ZIP / 魔数不符”，定位成本高。
//  3. fnSeedExpand 与 fnReshuffle 是 capstone 对 yui.exe 两段函数的
//     机械展开（偏移/寄存器名一字不落），可用 unicorn 镜像复算对照。
//
// 内存布局（平直 []byte，地址即字节偏移）：
//
//	ctxAddr = 0x1000：上下文基址。偏移 0x0004 起是 512 个 dword 的初始状态
//	  （初始状态长 0x800 字节，止于 0x0804）；0x0800 起是 512 个 dword 的
//	  生成区（止于 0x1000）；0x1010/0x1014 是计数对（与原栈偏移同值）。
//	memTop = 0x3000 / memTop2 = 0x4000：两段的独立栈，互不重叠。
//
// 之所以把上下文放在高地址独立于栈，是因为原代码里 [ecx+0x810] 的
// 256 对缓冲与槽表基址只差 0x10 就相邻，写错一位就直接闯进槽表，
// 平直内存下最容易眼见为实地校对。
const (
	ctxAddr  = 0x1000
	memTop   = 0x3000
	memTop2  = 0x4000
	memSize  = 0x5000
	keyMagic = 0x9e370001
)

var (
	keyMem  []byte
	callCtx uint32
)

// ld32 / st32 是平直内存的 dword 读写，与原 x86 的 push/pop/[ebp±x]/[reg+x]
// 语义完全对齐：地址一字不差，进位由各函数内部维护（cf 变量）。
func ld32(addr uint32) uint32 {
	return binary.LittleEndian.Uint32(keyMem[addr:])
}

func st32(addr uint32, v uint32) {
	binary.LittleEndian.PutUint32(keyMem[addr:], v)
}

// bto32 把布尔借位/进位转成 x86 CF 的 0/1。
func bto32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// stateInit 是调用方（sub_40CE19）的第一段：只负责 512 字简单状态，
// 不做扩展。closed form 在 spec 里有，且已被 unicorn 校验，
// 我们保留可读写法供对照；不能不校验就直接相信“state[i]”的记忆。
func stateInit(seed uint32) {
	stateBase := uint32(ctxAddr + 8)
	state := make([]uint32, 512)
	state[0] = seed ^ keyMagic
	for i := 1; i < 512; i++ {
		x := state[i-1]
		// 乘法全部按 uint32 回绕，i 只当两数之差的小加数，勿改成带符号运算。
		state[i] = uint32(i) - (x^(x>>30))*0x61C88647
	}
	for i := 0; i < 512; i++ {
		st32(stateBase+uint32(i*4), state[i])
	}
}

// GenKeyTable 给定种子产出 256 个 dword 的字节流密钥表。
// 种子域为 uint32，文件/条目大小直接传入即可（文件大小 < 2^32）。
// 表实为槽表 256 对的 XOR（低 ^ 高），槽次序是 255→0 倒序消耗；
// 若改成正序，header 与 index 都会错位但仍是 48 字节零门槛，
// 测试必须用真实存档头断言 ARC\0 才能拦住。
// 注意透镜仅校验前 256 项：像原调用方一样只消费一轮，不做二次洗牌。
func GenKeyTable(seed uint32) [256]uint32 {
	keyMem = make([]byte, memSize)
	// 初始上下文全零，只有状态与计数需要确定初值；
	// 剩余字节由扩展/洗牌逐个覆盖，留非零残字会污染槽表。
	stateInit(seed)
	st32(memTop, 0xdeadbeef)
	st32(memTop+4, ctxAddr)
	fnSeedExpand()
	var table [256]uint32
	// 0x40BE4B 把 512 个 dword 原地扩成 256 个 qword，缓冲从 ctx+8 开始；
	// 0x40CE19 从缓冲尾部（slot 255）倒序消费，table[i]=低^高。
	buffer := uint32(ctxAddr + 8)
	for i := 0; i < 256; i++ {
		slot := 255 - i
		low := ld32(buffer + uint32(slot*8))
		high := ld32(buffer + uint32(slot*8+4))
		table[i] = low ^ high
	}
	return table
}

// rol32 是密钥流的旋转：注释一下非显然点——旋转量取 (i>>8)&31，
// 位数 n 必须对 32 取模，x86 的 rol 不屏蔽位数也会在该位以后回绕。
func rol32(x uint32, n uint32) uint32 {
	n &= 31
	if n == 0 {
		return x
	}
	return (x << n) | (x >> (32 - n))
}

// Decrypt 按流下标逐字节异或解密，解密不改变入参（返回新切片）。
// start 是全局流下标：头与文件体都从 0 起，压缩索引从 0x30 起；
// 读写两侧必须对齐，否则每个字节都会差上错位密钥，表现为魔数对了
// 索引却成乱码。
func Decrypt(data []byte, table [256]uint32, start int) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	for i := range out {
		k := table[(start+i)&0xff] ^ 0x1000193
		k = rol32(k, uint32((start+i)>>8)&31)
		out[i] ^= byte(k & 0xff)
	}
	return out
}

func fnSeedExpand() {
	var rEax, rEcx, rEdx, rEbx, rEsi, rEdi, rEbp uint32
	rEsp := uint32(memTop)
	var cf uint32
	var zf bool
	_ = cf
	_ = zf
	rEsp -= 4
	st32(rEsp, rEbp)
	rEbp = rEsp
	r1 := uint64(rEsp) - uint64(uint32(0x48))
	cf = bto32(uint64(rEsp) < uint64(uint32(0x48)))
	rEsp = uint32(r1)
	rEax = ld32(rEbp + 8)
	rEcx ^= rEcx
	cf = 0
	rEsp -= 4
	st32(rEsp, rEbx)
	rEsp -= 4
	st32(rEsp, rEsi)
	st32(rEax+4128, rEcx)
	st32(rEax+4132, rEcx)
	st32(rEax+4120, rEcx)
	st32(rEax+4124, rEcx)
	st32(rEax+4112, rEcx)
	st32(rEax+4116, rEcx)
	rEcx = uint32(0x7f4a7c13)
	rEax = uint32(0x9e3779b9)
	rEsp -= 4
	st32(rEsp, rEdi)
	rEdi = rEcx
	st32(rEbp-60, rEax)
	st32(rEbp-24, rEcx)
	st32(rEbp-20, rEax)
	st32(rEbp-16, rEcx)
	st32(rEbp-12, rEax)
	rEdx = rEcx
	rEsi = rEax
	st32(rEbp-32, rEcx)
	st32(rEbp-28, rEax)
	st32(rEbp-48, rEcx)
	st32(rEbp-44, rEax)
	st32(rEbp-56, rEcx)
	st32(rEbp-52, rEax)
	st32(rEbp-40, rEcx)
	st32(rEbp-36, rEax)
	st32(rEbp-4, uint32(4))
L_40bebb:
	r2 := uint64(ld32(rEbp-40)) - uint64(rEdx)
	cf = bto32(uint64(ld32(rEbp-40)) < uint64(rEdx))
	st32(rEbp-40, uint32(r2))
	rEcx = ld32(rEbp - 60)
	rEax = rEdi
	r3 := uint64(ld32(rEbp-36)) - uint64(rEsi) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-36)) < uint64(rEsi)+uint64(cf))
	st32(rEbp-36, uint32(r3))
	rEax = (rEax >> 9) | (rEcx << 23)
	st32(rEbp-16, ld32(rEbp-16)^rEax)
	cf = 0
	rEcx >>= 9
	st32(rEbp-12, ld32(rEbp-12)^rEcx)
	cf = 0
	r4 := uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r4 >> 32)
	rEdi = uint32(r4)
	rEax = ld32(rEbp - 36)
	r5 := uint64(ld32(rEbp-60)) + uint64(rEax) + uint64(cf)
	cf = uint32(r5 >> 32)
	st32(rEbp-60, uint32(r5))
	rEcx = ld32(rEbp - 16)
	r6 := uint64(ld32(rEbp-56)) - uint64(rEcx)
	cf = bto32(uint64(ld32(rEbp-56)) < uint64(rEcx))
	st32(rEbp-56, uint32(r6))
	rEcx = ld32(rEbp - 12)
	r7 := uint64(ld32(rEbp-52)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-52)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-52, uint32(r7))
	rEcx = ld32(rEbp - 40)
	rEax = (rEax << 9) | (rEcx >> 23)
	st32(rEbp-20, ld32(rEbp-20)^rEax)
	cf = 0
	rEax = ld32(rEbp - 52)
	rEcx <<= 9
	st32(rEbp-24, ld32(rEbp-24)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 56)
	r8 := uint64(ld32(rEbp-40)) + uint64(rEcx)
	cf = uint32(r8 >> 32)
	st32(rEbp-40, uint32(r8))
	rEbx = ld32(rEbp - 24)
	r9 := uint64(ld32(rEbp-36)) + uint64(rEax) + uint64(cf)
	cf = uint32(r9 >> 32)
	st32(rEbp-36, uint32(r9))
	r10 := uint64(ld32(rEbp-48)) - uint64(rEbx)
	cf = bto32(uint64(ld32(rEbp-48)) < uint64(rEbx))
	st32(rEbp-48, uint32(r10))
	rEbx = ld32(rEbp - 20)
	r11 := uint64(ld32(rEbp-44)) - uint64(rEbx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-44)) < uint64(rEbx)+uint64(cf))
	st32(rEbp-44, uint32(r11))
	rEbx = rEax
	rEcx = (rEcx >> 23) | (rEbx << 9)
	rEdi ^= rEcx
	cf = 0
	rEcx = ld32(rEbp - 48)
	rEbx >>= 23
	st32(rEbp-60, ld32(rEbp-60)^rEbx)
	cf = 0
	r12 := uint64(ld32(rEbp-56)) + uint64(rEcx)
	cf = uint32(r12 >> 32)
	st32(rEbp-56, uint32(r12))
	r13 := uint64(rEax) + uint64(ld32(rEbp-44)) + uint64(cf)
	cf = uint32(r13 >> 32)
	rEax = uint32(r13)
	r14 := uint64(ld32(rEbp-32)) - uint64(rEdi)
	cf = bto32(uint64(ld32(rEbp-32)) < uint64(rEdi))
	st32(rEbp-32, uint32(r14))
	st32(rEbp-52, rEax)
	rEax = ld32(rEbp - 60)
	r15 := uint64(ld32(rEbp-28)) - uint64(rEax) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-28)) < uint64(rEax)+uint64(cf))
	st32(rEbp-28, uint32(r15))
	rEax = rEcx
	rEcx = ld32(rEbp - 44)
	rEcx = (rEcx << 15) | (rEax >> 17)
	st32(rEbp-36, ld32(rEbp-36)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 32)
	rEax <<= 15
	st32(rEbp-40, ld32(rEbp-40)^rEax)
	cf = 0
	r16 := uint64(ld32(rEbp-48)) + uint64(rEcx)
	cf = uint32(r16 >> 32)
	st32(rEbp-48, uint32(r16))
	rEax = ld32(rEbp - 28)
	r17 := uint64(ld32(rEbp-44)) + uint64(rEax) + uint64(cf)
	cf = uint32(r17 >> 32)
	st32(rEbp-44, uint32(r17))
	r18 := uint64(rEdx) - uint64(ld32(rEbp-40))
	cf = bto32(uint64(rEdx) < uint64(ld32(rEbp-40)))
	rEdx = uint32(r18)
	r19 := uint64(rEsi) - uint64(ld32(rEbp-36)) - uint64(cf)
	cf = bto32(uint64(rEsi) < uint64(ld32(rEbp-36))+uint64(cf))
	rEsi = uint32(r19)
	rEcx = (rEcx >> 14) | (rEax << 18)
	st32(rEbp-56, ld32(rEbp-56)^rEcx)
	cf = 0
	rEax >>= 14
	st32(rEbp-52, ld32(rEbp-52)^rEax)
	cf = 0
	r20 := uint64(ld32(rEbp-32)) + uint64(rEdx)
	cf = uint32(r20 >> 32)
	st32(rEbp-32, uint32(r20))
	rEax = ld32(rEbp - 56)
	rEcx = rEsi
	r21 := uint64(ld32(rEbp-28)) + uint64(rEsi) + uint64(cf)
	cf = uint32(r21 >> 32)
	st32(rEbp-28, uint32(r21))
	r22 := uint64(ld32(rEbp-16)) - uint64(rEax)
	cf = bto32(uint64(ld32(rEbp-16)) < uint64(rEax))
	st32(rEbp-16, uint32(r22))
	rEax = ld32(rEbp - 52)
	r23 := uint64(ld32(rEbp-12)) - uint64(rEax) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-12)) < uint64(rEax)+uint64(cf))
	st32(rEbp-12, uint32(r23))
	rEax = rEdx
	rEcx = (rEcx << 20) | (rEax >> 12)
	st32(rEbp-44, ld32(rEbp-44)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 12)
	rEax <<= 20
	st32(rEbp-48, ld32(rEbp-48)^rEax)
	cf = 0
	r24 := uint64(rEdx) + uint64(ld32(rEbp-16))
	cf = uint32(r24 >> 32)
	rEdx = uint32(r24)
	rEax = ld32(rEbp - 48)
	r25 := uint64(rEsi) + uint64(ld32(rEbp-12)) + uint64(cf)
	cf = uint32(r25 >> 32)
	rEsi = uint32(r25)
	r26 := uint64(ld32(rEbp-24)) - uint64(rEax)
	cf = bto32(uint64(ld32(rEbp-24)) < uint64(rEax))
	st32(rEbp-24, uint32(r26))
	rEax = ld32(rEbp - 44)
	r27 := uint64(ld32(rEbp-20)) - uint64(rEax) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-20)) < uint64(rEax)+uint64(cf))
	st32(rEbp-20, uint32(r27))
	rEax = ld32(rEbp - 16)
	rEax = (rEax >> 17) | (rEcx << 15)
	st32(rEbp-32, ld32(rEbp-32)^rEax)
	cf = 0
	rEcx >>= 17
	st32(rEbp-28, ld32(rEbp-28)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 24)
	r28 := uint64(ld32(rEbp-16)) + uint64(rEcx)
	cf = uint32(r28 >> 32)
	st32(rEbp-16, uint32(r28))
	rEax = ld32(rEbp - 20)
	rEbx = ld32(rEbp - 28)
	r29 := uint64(ld32(rEbp-12)) + uint64(rEax) + uint64(cf)
	cf = uint32(r29 >> 32)
	st32(rEbp-12, uint32(r29))
	r30 := uint64(rEdi) - uint64(ld32(rEbp-32))
	cf = bto32(uint64(rEdi) < uint64(ld32(rEbp-32)))
	rEdi = uint32(r30)
	r31 := uint64(ld32(rEbp-60)) - uint64(rEbx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-60)) < uint64(rEbx)+uint64(cf))
	st32(rEbp-60, uint32(r31))
	rEbx = rEax
	rEbx = (rEbx << 14) | (rEcx >> 18)
	rEcx <<= 14
	rEdx ^= rEcx
	cf = 0
	rEsi ^= rEbx
	cf = 0
	r32 := uint64(ld32(rEbp-24)) + uint64(rEdi)
	cf = uint32(r32 >> 32)
	st32(rEbp-24, uint32(r32))
	r33 := uint64(rEax) + uint64(ld32(rEbp-60)) + uint64(cf)
	cf = uint32(r33 >> 32)
	rEax = uint32(r33)
	st32(rEbp-4, ld32(rEbp-4)-1)
	zf = ld32(rEbp-4) == 0
	st32(rEbp-20, rEax)
	if !zf {
		goto L_40bebb
	}
	rEax = ld32(rEbp + 8)
	r34 := uint64(rEax) + uint64(uint32(0x10))
	cf = uint32(r34 >> 32)
	rEax = uint32(r34)
	st32(rEbp-4, uint32(0x20))
L_40bfed:
	rEcx = ld32(rEax - 8)
	r35 := uint64(ld32(rEbp-40)) + uint64(rEcx)
	cf = uint32(r35 >> 32)
	st32(rEbp-40, uint32(r35))
	rEcx = ld32(rEax - 4)
	r36 := uint64(ld32(rEbp-36)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r36 >> 32)
	st32(rEbp-36, uint32(r36))
	rEcx = ld32(rEax)
	r37 := uint64(ld32(rEbp-56)) + uint64(rEcx)
	cf = uint32(r37 >> 32)
	st32(rEbp-56, uint32(r37))
	rEcx = ld32(rEax + 4)
	r38 := uint64(ld32(rEbp-52)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r38 >> 32)
	st32(rEbp-52, uint32(r38))
	rEcx = ld32(rEax + 8)
	r39 := uint64(ld32(rEbp-48)) + uint64(rEcx)
	cf = uint32(r39 >> 32)
	st32(rEbp-48, uint32(r39))
	rEcx = ld32(rEax + 12)
	r40 := uint64(ld32(rEbp-44)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r40 >> 32)
	st32(rEbp-44, uint32(r40))
	rEcx = ld32(rEax + 16)
	r41 := uint64(ld32(rEbp-32)) + uint64(rEcx)
	cf = uint32(r41 >> 32)
	st32(rEbp-32, uint32(r41))
	rEcx = ld32(rEax + 20)
	r42 := uint64(ld32(rEbp-28)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r42 >> 32)
	st32(rEbp-28, uint32(r42))
	r43 := uint64(rEdx) + uint64(ld32(rEax+24))
	cf = uint32(r43 >> 32)
	rEdx = uint32(r43)
	rEcx = ld32(rEax + 32)
	r44 := uint64(rEsi) + uint64(ld32(rEax+28)) + uint64(cf)
	cf = uint32(r44 >> 32)
	rEsi = uint32(r44)
	r45 := uint64(ld32(rEbp-16)) + uint64(rEcx)
	cf = uint32(r45 >> 32)
	st32(rEbp-16, uint32(r45))
	rEcx = ld32(rEax + 36)
	r46 := uint64(ld32(rEbp-12)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r46 >> 32)
	st32(rEbp-12, uint32(r46))
	rEcx = ld32(rEax + 40)
	r47 := uint64(ld32(rEbp-24)) + uint64(rEcx)
	cf = uint32(r47 >> 32)
	st32(rEbp-24, uint32(r47))
	rEcx = ld32(rEax + 44)
	r48 := uint64(ld32(rEbp-20)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r48 >> 32)
	st32(rEbp-20, uint32(r48))
	r49 := uint64(rEdi) + uint64(ld32(rEax+48))
	cf = uint32(r49 >> 32)
	rEdi = uint32(r49)
	rEcx = ld32(rEax + 52)
	r50 := uint64(ld32(rEbp-60)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r50 >> 32)
	st32(rEbp-60, uint32(r50))
	r51 := uint64(ld32(rEbp-40)) - uint64(rEdx)
	cf = bto32(uint64(ld32(rEbp-40)) < uint64(rEdx))
	st32(rEbp-40, uint32(r51))
	rEcx = ld32(rEbp - 60)
	st32(rEbp-64, rEdi)
	r52 := uint64(ld32(rEbp-36)) - uint64(rEsi) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-36)) < uint64(rEsi)+uint64(cf))
	st32(rEbp-36, uint32(r52))
	rEdi = (rEdi >> 9) | (rEcx << 23)
	st32(rEbp-16, ld32(rEbp-16)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 40)
	rEbx = ld32(rEbp - 16)
	rEcx >>= 9
	st32(rEbp-12, ld32(rEbp-12)^rEcx)
	cf = 0
	r53 := uint64(ld32(rEbp-64)) + uint64(rEdi)
	cf = uint32(r53 >> 32)
	st32(rEbp-64, uint32(r53))
	rEcx = ld32(rEbp - 36)
	r54 := uint64(ld32(rEbp-60)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r54 >> 32)
	st32(rEbp-60, uint32(r54))
	r55 := uint64(ld32(rEbp-56)) - uint64(rEbx)
	cf = bto32(uint64(ld32(rEbp-56)) < uint64(rEbx))
	st32(rEbp-56, uint32(r55))
	rEbx = ld32(rEbp - 12)
	r56 := uint64(ld32(rEbp-52)) - uint64(rEbx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-52)) < uint64(rEbx)+uint64(cf))
	st32(rEbp-52, uint32(r56))
	rEcx = (rEcx << 9) | (rEdi >> 23)
	st32(rEbp-20, ld32(rEbp-20)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 52)
	rEdi <<= 9
	st32(rEbp-24, ld32(rEbp-24)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 56)
	r57 := uint64(ld32(rEbp-40)) + uint64(rEdi)
	cf = uint32(r57 >> 32)
	st32(rEbp-40, uint32(r57))
	rEbx = ld32(rEbp - 24)
	r58 := uint64(ld32(rEbp-36)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r58 >> 32)
	st32(rEbp-36, uint32(r58))
	r59 := uint64(ld32(rEbp-48)) - uint64(rEbx)
	cf = bto32(uint64(ld32(rEbp-48)) < uint64(rEbx))
	st32(rEbp-48, uint32(r59))
	rEbx = ld32(rEbp - 20)
	r60 := uint64(ld32(rEbp-44)) - uint64(rEbx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-44)) < uint64(rEbx)+uint64(cf))
	st32(rEbp-44, uint32(r60))
	rEdi = (rEdi >> 23) | (rEcx << 9)
	st32(rEbp-64, ld32(rEbp-64)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 48)
	rEbx = ld32(rEbp - 64)
	rEcx >>= 23
	st32(rEbp-60, ld32(rEbp-60)^rEcx)
	cf = 0
	r61 := uint64(ld32(rEbp-56)) + uint64(rEdi)
	cf = uint32(r61 >> 32)
	st32(rEbp-56, uint32(r61))
	rEcx = ld32(rEbp - 44)
	r62 := uint64(ld32(rEbp-52)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r62 >> 32)
	st32(rEbp-52, uint32(r62))
	r63 := uint64(ld32(rEbp-32)) - uint64(rEbx)
	cf = bto32(uint64(ld32(rEbp-32)) < uint64(rEbx))
	st32(rEbp-32, uint32(r63))
	rEbx = ld32(rEbp - 60)
	r64 := uint64(ld32(rEbp-28)) - uint64(rEbx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-28)) < uint64(rEbx)+uint64(cf))
	st32(rEbp-28, uint32(r64))
	rEcx = (rEcx << 15) | (rEdi >> 17)
	st32(rEbp-36, ld32(rEbp-36)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 28)
	rEdi <<= 15
	st32(rEbp-40, ld32(rEbp-40)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 32)
	r65 := uint64(ld32(rEbp-48)) + uint64(rEdi)
	cf = uint32(r65 >> 32)
	st32(rEbp-48, uint32(r65))
	r66 := uint64(ld32(rEbp-44)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r66 >> 32)
	st32(rEbp-44, uint32(r66))
	r67 := uint64(rEdx) - uint64(ld32(rEbp-40))
	cf = bto32(uint64(rEdx) < uint64(ld32(rEbp-40)))
	rEdx = uint32(r67)
	r68 := uint64(rEsi) - uint64(ld32(rEbp-36)) - uint64(cf)
	cf = bto32(uint64(rEsi) < uint64(ld32(rEbp-36))+uint64(cf))
	rEsi = uint32(r68)
	rEdi = (rEdi >> 14) | (rEcx << 18)
	rEcx >>= 14
	st32(rEbp-52, ld32(rEbp-52)^rEcx)
	cf = 0
	st32(rEbp-56, ld32(rEbp-56)^rEdi)
	cf = 0
	r69 := uint64(ld32(rEbp-32)) + uint64(rEdx)
	cf = uint32(r69 >> 32)
	st32(rEbp-32, uint32(r69))
	rEcx = ld32(rEbp - 56)
	rEdi = rEsi
	r70 := uint64(ld32(rEbp-28)) + uint64(rEsi) + uint64(cf)
	cf = uint32(r70 >> 32)
	st32(rEbp-28, uint32(r70))
	r71 := uint64(ld32(rEbp-16)) - uint64(rEcx)
	cf = bto32(uint64(ld32(rEbp-16)) < uint64(rEcx))
	st32(rEbp-16, uint32(r71))
	rEcx = ld32(rEbp - 52)
	r72 := uint64(ld32(rEbp-12)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-12)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-12, uint32(r72))
	rEcx = rEdx
	rEdi = (rEdi << 20) | (rEcx >> 12)
	st32(rEbp-44, ld32(rEbp-44)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 12)
	rEcx <<= 20
	st32(rEbp-48, ld32(rEbp-48)^rEcx)
	cf = 0
	r73 := uint64(rEdx) + uint64(ld32(rEbp-16))
	cf = uint32(r73 >> 32)
	rEdx = uint32(r73)
	rEcx = ld32(rEbp - 48)
	r74 := uint64(rEsi) + uint64(ld32(rEbp-12)) + uint64(cf)
	cf = uint32(r74 >> 32)
	rEsi = uint32(r74)
	r75 := uint64(ld32(rEbp-24)) - uint64(rEcx)
	cf = bto32(uint64(ld32(rEbp-24)) < uint64(rEcx))
	st32(rEbp-24, uint32(r75))
	rEcx = ld32(rEbp - 44)
	r76 := uint64(ld32(rEbp-20)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-20)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-20, uint32(r76))
	rEcx = ld32(rEbp - 16)
	rEbx = ld32(rEbp - 20)
	rEcx = (rEcx >> 17) | (rEdi << 15)
	st32(rEbp-32, ld32(rEbp-32)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 24)
	rEdi >>= 17
	st32(rEbp-28, ld32(rEbp-28)^rEdi)
	cf = 0
	r77 := uint64(ld32(rEbp-16)) + uint64(rEcx)
	cf = uint32(r77 >> 32)
	st32(rEbp-16, uint32(r77))
	rEcx = ld32(rEbp - 20)
	rEdi = ld32(rEbp - 64)
	r78 := uint64(ld32(rEbp-12)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r78 >> 32)
	st32(rEbp-12, uint32(r78))
	r79 := uint64(rEdi) - uint64(ld32(rEbp-32))
	cf = bto32(uint64(rEdi) < uint64(ld32(rEbp-32)))
	rEdi = uint32(r79)
	rEcx = ld32(rEbp - 28)
	r80 := uint64(ld32(rEbp-60)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-60)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-60, uint32(r80))
	rEcx = ld32(rEbp - 24)
	rEbx = (rEbx << 14) | (rEcx >> 18)
	rEsi ^= rEbx
	cf = 0
	rEbx = ld32(rEbp - 40)
	st32(rEax+2048, rEbx)
	rEbx = ld32(rEbp - 36)
	st32(rEax+2052, rEbx)
	rEbx = ld32(rEbp - 56)
	st32(rEax+2056, rEbx)
	rEbx = ld32(rEbp - 52)
	st32(rEax+2060, rEbx)
	rEbx = ld32(rEbp - 48)
	st32(rEax+2064, rEbx)
	rEbx = ld32(rEbp - 44)
	st32(rEax+2068, rEbx)
	rEbx = ld32(rEbp - 32)
	st32(rEax+2072, rEbx)
	rEbx = ld32(rEbp - 28)
	st32(rEax+2076, rEbx)
	rEbx = ld32(rEbp - 16)
	rEcx <<= 14
	rEdx ^= rEcx
	cf = 0
	r81 := uint64(ld32(rEbp-24)) + uint64(rEdi)
	cf = uint32(r81 >> 32)
	st32(rEbp-24, uint32(r81))
	rEcx = ld32(rEbp - 60)
	st32(rEax+2088, rEbx)
	rEbx = ld32(rEbp - 12)
	r82 := uint64(ld32(rEbp-20)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r82 >> 32)
	st32(rEbp-20, uint32(r82))
	st32(rEax+2092, rEbx)
	rEbx = ld32(rEbp - 24)
	st32(rEax+2096, rEbx)
	rEbx = ld32(rEbp - 20)
	st32(rEax+2080, rEdx)
	st32(rEax+2084, rEsi)
	st32(rEax+2100, rEbx)
	st32(rEax+2104, rEdi)
	st32(rEax+2108, rEcx)
	r83 := uint64(rEax) + uint64(uint32(0x40))
	cf = uint32(r83 >> 32)
	rEax = uint32(r83)
	st32(rEbp-4, ld32(rEbp-4)-1)
	zf = ld32(rEbp-4) == 0
	if !zf {
		goto L_40bfed
	}
	rEax = ld32(rEbp + 8)
	r84 := uint64(rEax) + uint64(uint32(0x818))
	cf = uint32(r84 >> 32)
	rEax = uint32(r84)
	st32(rEbp-4, uint32(0x20))
L_40c203:
	r85 := uint64(rEdx) + uint64(ld32(rEax+24))
	cf = uint32(r85 >> 32)
	rEdx = uint32(r85)
	rEcx = ld32(rEax + 52)
	r86 := uint64(rEsi) + uint64(ld32(rEax+28)) + uint64(cf)
	cf = uint32(r86 >> 32)
	rEsi = uint32(r86)
	r87 := uint64(rEdi) + uint64(ld32(rEax+48))
	cf = uint32(r87 >> 32)
	rEdi = uint32(r87)
	rEbx = ld32(rEax - 4)
	r88 := uint64(ld32(rEbp-60)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r88 >> 32)
	st32(rEbp-60, uint32(r88))
	rEcx = ld32(rEax - 8)
	r89 := uint64(rEcx) - uint64(rEdx)
	cf = bto32(uint64(rEcx) < uint64(rEdx))
	rEcx = uint32(r89)
	r90 := uint64(rEbx) - uint64(rEsi) - uint64(cf)
	cf = bto32(uint64(rEbx) < uint64(rEsi)+uint64(cf))
	rEbx = uint32(r90)
	r91 := uint64(ld32(rEbp-40)) + uint64(rEcx)
	cf = uint32(r91 >> 32)
	st32(rEbp-40, uint32(r91))
	rEcx = ld32(rEax + 32)
	st32(rEbp-68, rEsi)
	r92 := uint64(ld32(rEbp-36)) + uint64(rEbx) + uint64(cf)
	cf = uint32(r92 >> 32)
	st32(rEbp-36, uint32(r92))
	r93 := uint64(rEcx) + uint64(ld32(rEbp-16))
	cf = uint32(r93 >> 32)
	rEcx = uint32(r93)
	rEbx = ld32(rEbp - 60)
	rEsi = ld32(rEax + 36)
	r94 := uint64(rEsi) + uint64(ld32(rEbp-12)) + uint64(cf)
	cf = uint32(r94 >> 32)
	rEsi = uint32(r94)
	st32(rEbp-64, rEdi)
	rEdi = (rEdi >> 9) | (rEbx << 23)
	rEbx >>= 9
	rEsi ^= rEbx
	cf = 0
	rEbx = ld32(rEax + 4)
	rEcx ^= rEdi
	cf = 0
	rEdi = ld32(rEax)
	st32(rEbp-12, rEsi)
	rEsi = ld32(rEbp - 40)
	r95 := uint64(ld32(rEbp-64)) + uint64(rEsi)
	cf = uint32(r95 >> 32)
	st32(rEbp-64, uint32(r95))
	st32(rEbp-16, rEcx)
	rEcx = ld32(rEbp - 36)
	r96 := uint64(ld32(rEbp-60)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r96 >> 32)
	st32(rEbp-60, uint32(r96))
	r97 := uint64(rEdi) - uint64(ld32(rEbp-16))
	cf = bto32(uint64(rEdi) < uint64(ld32(rEbp-16)))
	rEdi = uint32(r97)
	r98 := uint64(rEbx) - uint64(ld32(rEbp-12)) - uint64(cf)
	cf = bto32(uint64(rEbx) < uint64(ld32(rEbp-12))+uint64(cf))
	rEbx = uint32(r98)
	r99 := uint64(ld32(rEbp-56)) + uint64(rEdi)
	cf = uint32(r99 >> 32)
	st32(rEbp-56, uint32(r99))
	rEdi = ld32(rEbp - 24)
	r100 := uint64(ld32(rEbp-52)) + uint64(rEbx) + uint64(cf)
	cf = uint32(r100 >> 32)
	st32(rEbp-52, uint32(r100))
	r101 := uint64(rEdi) + uint64(ld32(rEax+40))
	cf = uint32(r101 >> 32)
	rEdi = uint32(r101)
	rEbx = ld32(rEbp - 20)
	r102 := uint64(rEbx) + uint64(ld32(rEax+44)) + uint64(cf)
	cf = uint32(r102 >> 32)
	rEbx = uint32(r102)
	rEcx = (rEcx << 9) | (rEsi >> 23)
	rEsi <<= 9
	rEdi ^= rEsi
	cf = 0
	rEsi = ld32(rEax + 12)
	rEbx ^= rEcx
	cf = 0
	rEcx = ld32(rEbp - 52)
	st32(rEbp-24, rEdi)
	rEdi = ld32(rEbp - 56)
	r103 := uint64(ld32(rEbp-40)) + uint64(rEdi)
	cf = uint32(r103 >> 32)
	st32(rEbp-40, uint32(r103))
	st32(rEbp-20, rEbx)
	r104 := uint64(ld32(rEbp-36)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r104 >> 32)
	st32(rEbp-36, uint32(r104))
	rEcx = ld32(rEax + 8)
	r105 := uint64(rEcx) - uint64(ld32(rEbp-24))
	cf = bto32(uint64(rEcx) < uint64(ld32(rEbp-24)))
	rEcx = uint32(r105)
	r106 := uint64(rEsi) - uint64(rEbx) - uint64(cf)
	cf = bto32(uint64(rEsi) < uint64(rEbx)+uint64(cf))
	rEsi = uint32(r106)
	r107 := uint64(ld32(rEbp-48)) + uint64(rEcx)
	cf = uint32(r107 >> 32)
	st32(rEbp-48, uint32(r107))
	rEcx = ld32(rEbp - 44)
	rEbx = ld32(rEbp - 52)
	r108 := uint64(rEcx) + uint64(rEsi) + uint64(cf)
	cf = uint32(r108 >> 32)
	rEcx = uint32(r108)
	rEsi = rEdi
	rEsi = (rEsi >> 23) | (rEbx << 9)
	st32(rEbp-64, ld32(rEbp-64)^rEsi)
	cf = 0
	rEsi = ld32(rEax + 16)
	rEbx >>= 23
	st32(rEbp-60, ld32(rEbp-60)^rEbx)
	cf = 0
	r109 := uint64(rEdi) + uint64(ld32(rEbp-48))
	cf = uint32(r109 >> 32)
	rEdi = uint32(r109)
	rEbx = ld32(rEax + 20)
	r110 := uint64(ld32(rEbp-52)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r110 >> 32)
	st32(rEbp-52, uint32(r110))
	r111 := uint64(rEsi) - uint64(ld32(rEbp-64))
	cf = bto32(uint64(rEsi) < uint64(ld32(rEbp-64)))
	rEsi = uint32(r111)
	r112 := uint64(rEbx) - uint64(ld32(rEbp-60)) - uint64(cf)
	cf = bto32(uint64(rEbx) < uint64(ld32(rEbp-60))+uint64(cf))
	rEbx = uint32(r112)
	r113 := uint64(ld32(rEbp-32)) + uint64(rEsi)
	cf = uint32(r113 >> 32)
	st32(rEbp-32, uint32(r113))
	rEsi = ld32(rEbp - 48)
	r114 := uint64(ld32(rEbp-28)) + uint64(rEbx) + uint64(cf)
	cf = uint32(r114 >> 32)
	st32(rEbp-28, uint32(r114))
	rEbx = rEcx
	rEbx = (rEbx << 15) | (rEsi >> 17)
	st32(rEbp-36, ld32(rEbp-36)^rEbx)
	cf = 0
	rEsi <<= 15
	st32(rEbp-40, ld32(rEbp-40)^rEsi)
	cf = 0
	rEsi = ld32(rEbp - 32)
	r115 := uint64(ld32(rEbp-48)) + uint64(rEsi)
	cf = uint32(r115 >> 32)
	st32(rEbp-48, uint32(r115))
	rEsi = ld32(rEbp - 68)
	r116 := uint64(rEcx) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r116 >> 32)
	rEcx = uint32(r116)
	r117 := uint64(rEdx) - uint64(ld32(rEbp-40))
	cf = bto32(uint64(rEdx) < uint64(ld32(rEbp-40)))
	rEdx = uint32(r117)
	st32(rEbp-44, rEcx)
	r118 := uint64(rEsi) - uint64(ld32(rEbp-36)) - uint64(cf)
	cf = bto32(uint64(rEsi) < uint64(ld32(rEbp-36))+uint64(cf))
	rEsi = uint32(r118)
	rEcx = ld32(rEbp - 32)
	rEbx = ld32(rEbp - 28)
	rEcx = (rEcx >> 14) | (rEbx << 18)
	rEdi ^= rEcx
	cf = 0
	rEbx >>= 14
	st32(rEbp-52, ld32(rEbp-52)^rEbx)
	cf = 0
	r119 := uint64(ld32(rEbp-32)) + uint64(rEdx)
	cf = uint32(r119 >> 32)
	st32(rEbp-32, uint32(r119))
	rEcx = ld32(rEbp - 52)
	st32(rEbp-56, rEdi)
	r120 := uint64(ld32(rEbp-28)) + uint64(rEsi) + uint64(cf)
	cf = uint32(r120 >> 32)
	st32(rEbp-28, uint32(r120))
	r121 := uint64(ld32(rEbp-16)) - uint64(rEdi)
	cf = bto32(uint64(ld32(rEbp-16)) < uint64(rEdi))
	st32(rEbp-16, uint32(r121))
	rEdi = rEsi
	r122 := uint64(ld32(rEbp-12)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-12)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-12, uint32(r122))
	rEcx = rEdx
	rEdi = (rEdi << 20) | (rEcx >> 12)
	st32(rEbp-44, ld32(rEbp-44)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 12)
	rEcx <<= 20
	st32(rEbp-48, ld32(rEbp-48)^rEcx)
	cf = 0
	r123 := uint64(rEdx) + uint64(ld32(rEbp-16))
	cf = uint32(r123 >> 32)
	rEdx = uint32(r123)
	rEcx = ld32(rEbp - 48)
	r124 := uint64(rEsi) + uint64(ld32(rEbp-12)) + uint64(cf)
	cf = uint32(r124 >> 32)
	rEsi = uint32(r124)
	r125 := uint64(ld32(rEbp-24)) - uint64(rEcx)
	cf = bto32(uint64(ld32(rEbp-24)) < uint64(rEcx))
	st32(rEbp-24, uint32(r125))
	rEcx = ld32(rEbp - 44)
	r126 := uint64(ld32(rEbp-20)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-20)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-20, uint32(r126))
	rEcx = ld32(rEbp - 16)
	rEbx = ld32(rEbp - 20)
	rEcx = (rEcx >> 17) | (rEdi << 15)
	st32(rEbp-32, ld32(rEbp-32)^rEcx)
	cf = 0
	rEcx = ld32(rEbp - 24)
	rEdi >>= 17
	st32(rEbp-28, ld32(rEbp-28)^rEdi)
	cf = 0
	r127 := uint64(ld32(rEbp-16)) + uint64(rEcx)
	cf = uint32(r127 >> 32)
	st32(rEbp-16, uint32(r127))
	rEcx = ld32(rEbp - 20)
	rEdi = ld32(rEbp - 64)
	r128 := uint64(ld32(rEbp-12)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r128 >> 32)
	st32(rEbp-12, uint32(r128))
	r129 := uint64(rEdi) - uint64(ld32(rEbp-32))
	cf = bto32(uint64(rEdi) < uint64(ld32(rEbp-32)))
	rEdi = uint32(r129)
	rEcx = ld32(rEbp - 28)
	r130 := uint64(ld32(rEbp-60)) - uint64(rEcx) - uint64(cf)
	cf = bto32(uint64(ld32(rEbp-60)) < uint64(rEcx)+uint64(cf))
	st32(rEbp-60, uint32(r130))
	rEcx = ld32(rEbp - 24)
	rEbx = (rEbx << 14) | (rEcx >> 18)
	rEsi ^= rEbx
	cf = 0
	rEbx = ld32(rEbp - 40)
	st32(rEax-8, rEbx)
	rEbx = ld32(rEbp - 36)
	st32(rEax-4, rEbx)
	rEbx = ld32(rEbp - 56)
	st32(rEax, rEbx)
	rEbx = ld32(rEbp - 52)
	st32(rEax+4, rEbx)
	rEbx = ld32(rEbp - 48)
	st32(rEax+8, rEbx)
	rEbx = ld32(rEbp - 44)
	st32(rEax+12, rEbx)
	rEbx = ld32(rEbp - 32)
	st32(rEax+16, rEbx)
	rEbx = ld32(rEbp - 28)
	st32(rEax+20, rEbx)
	rEbx = ld32(rEbp - 16)
	rEcx <<= 14
	rEdx ^= rEcx
	cf = 0
	r131 := uint64(ld32(rEbp-24)) + uint64(rEdi)
	cf = uint32(r131 >> 32)
	st32(rEbp-24, uint32(r131))
	rEcx = ld32(rEbp - 60)
	st32(rEax+32, rEbx)
	rEbx = ld32(rEbp - 12)
	r132 := uint64(ld32(rEbp-20)) + uint64(rEcx) + uint64(cf)
	cf = uint32(r132 >> 32)
	st32(rEbp-20, uint32(r132))
	st32(rEax+36, rEbx)
	rEbx = ld32(rEbp - 24)
	st32(rEax+40, rEbx)
	rEbx = ld32(rEbp - 20)
	st32(rEax+24, rEdx)
	st32(rEax+28, rEsi)
	st32(rEax+44, rEbx)
	st32(rEax+48, rEdi)
	st32(rEax+52, rEcx)
	r133 := uint64(rEax) + uint64(uint32(0x40))
	cf = uint32(r133 >> 32)
	rEax = uint32(r133)
	st32(rEbp-4, ld32(rEbp-4)-1)
	zf = ld32(rEbp-4) == 0
	if !zf {
		goto L_40c203
	}
	rEsi = ld32(rEbp + 8)
	rEcx = rEsi
	callCtx = rEcx
	fnReshuffle()
	st32(rEsi+2060, ld32(rEsi+2060)&uint32(0))
	cf = 0
	rEdi = ld32(rEsp)
	rEsp += 4
	st32(rEsi+2056, uint32(0x100))
	rEsi = ld32(rEsp)
	rEsp += 4
	rEbx = ld32(rEsp)
	rEsp += 4
	rEsp = rEbp
	rEbp = ld32(rEsp)
	rEsp += 4
}
func fnReshuffle() {
	var (
		r1, r2, r3, r4, r5, r6, r7, r8, r9, r10, r11, r12, r13, r14, r15, r16, r17, r18, r19, r20, r21, r22, r23, r24, r25, r26, r27, r28, r29, r30, r31, r32, r33, r34, r35, r36, r37, r38, r39, r40, r41, r42, r43, r44, r45, r46, r47, r48, r49, r50, r51, r52, r53, r54, r55, r56, r57, r58, r59, r60, r61, r62, r63, r64, r65, r66, r67, r68, r69, r70, r71, r72, r73, r74, r75, r76, r77, r78, r79, r80, r81, r82, r83, r84, r85, r86, r87, r88, r89, r90, r91 uint64
	)

	var r uint64
	var rEax, rEcx, rEdx, rEbx, rEsi, rEdi, rEbp uint32
	rEsp := uint32(memTop2)
	var cf uint32
	var zf bool
	_ = cf
	_ = zf
	rEcx = callCtx
	rEsp -= 4
	st32(rEsp, rEbp)
	rEbp = rEsp
	r1 = uint64(rEsp) - uint64(uint32(0x60))
	cf = uint32(r1 >> 63)
	rEsp = uint32(r1)
	r2 = uint64(ld32(rEcx+4128)) + uint64(uint32(1))
	cf = uint32(r2 >> 32)
	st32(rEcx+4128, uint32(r2))
	rEax = ld32(rEcx + 4112)
	rEdx = ld32(rEcx + 4128)
	r3 = uint64(ld32(rEcx+4132)) + uint64(uint32(0)) + uint64(cf)
	cf = uint32(r3 >> 32)
	st32(rEcx+4132, uint32(r3))
	r4 = uint64(rEdx) + uint64(ld32(rEcx+4120))
	cf = uint32(r4 >> 32)
	rEdx = uint32(r4)
	st32(rEbp-24, rEax)
	rEax = ld32(rEcx + 4116)
	st32(rEbp-20, rEax)
	rEax = ld32(rEcx + 4132)
	r5 = uint64(rEax) + uint64(ld32(rEcx+4124)) + uint64(cf)
	cf = uint32(r5 >> 32)
	rEax = uint32(r5)
	rEsp -= 4
	st32(rEsp, rEbx)
	rEsp -= 4
	st32(rEsp, rEsi)
	st32(rEbp-28, rEax)
	rEax = uint32(rEcx + 2064)
	rEsp -= 4
	st32(rEsp, rEdi)
	rEdi = uint32(rEax + 1024)
	st32(rEbp-32, rEdx)
	rEsi = uint32(rEcx + 8)
	st32(rEbp-4, rEdi)
	st32(rEbp-12, rEdi)
	rEdx = uint32(0x7f8)
	r6 = uint64(rEax)
	r7 = uint64(rEdi)
	r = r6 - r7
	cf = uint32(r >> 63)
	zf = r6 == r7
	if cf == 0 {
		goto L_40c71c
	}
L_40c468:
	rEdi = ld32(rEax)
	st32(rEbp-40, rEdi)
	rEdi = ld32(rEax + 4)
	st32(rEbp-36, rEdi)
	rEdi = ld32(rEbp - 24)
	rEbx = ld32(rEbp - 20)
	rEbx = (rEbx << 21) | (rEdi >> 11)
	rEbx ^= ld32(rEbp - 20)
	cf = 0
	rEdi <<= 21
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	rEbx = ^rEbx
	rEdi = ^rEdi
	st32(rEbp-92, rEbx)
	rEbx = ld32(rEbp - 4)
	r8 = uint64(rEdi) + uint64(ld32(rEbx))
	cf = uint32(r8 >> 32)
	rEdi = uint32(r8)
	st32(rEbp-24, rEdi)
	rEdi = ld32(rEbp - 92)
	r9 = uint64(rEdi) + uint64(ld32(rEbx+4)) + uint64(cf)
	cf = uint32(r9 >> 32)
	rEdi = uint32(r9)
	r10 = uint64(rEbx) + uint64(uint32(8))
	cf = uint32(r10 >> 32)
	rEbx = uint32(r10)
	st32(rEbp-20, rEdi)
	rEdi = ld32(rEbp - 40)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r11 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r11 >> 32)
	rEdi = uint32(r11)
	st32(rEbp-4, rEbx)
	st32(rEbp-72, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r12 = uint64(rEdi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r12 >> 32)
	rEdi = uint32(r12)
	st32(rEbp-68, rEdi)
	rEdi = ld32(rEbp - 72)
	r13 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r13 >> 32)
	rEdi = uint32(r13)
	st32(rEbp-48, rEdi)
	rEdi = ld32(rEbp - 68)
	r14 = uint64(rEdi) + uint64(ld32(rEbp-20)) + uint64(cf)
	cf = uint32(r14 >> 32)
	rEdi = uint32(r14)
	rEbx = ld32(rEbp - 48)
	st32(rEax+4, rEdi)
	st32(rEax, rEbx)
	rEbx = (rEbx >> 8) | (rEdi << 24)
	rEdi >>= 8
	st32(rEbp-8, rEdi)
	rEdi = rEbx
	rEbx = ld32(rEbp - 8)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r15 = uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r15 >> 32)
	rEdi = uint32(r15)
	st32(rEbp-32, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r16 = uint64(rEdi) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r16 >> 32)
	rEdi = uint32(r16)
	rEbx = ld32(rEbp - 32)
	st32(rEsi+4, rEdi)
	st32(rEbp-28, rEdi)
	st32(rEsi, rEbx)
	rEdi = ld32(rEax + 8)
	rEbx = ld32(rEbp - 20)
	st32(rEbp-40, rEdi)
	rEdi = ld32(rEax + 12)
	st32(rEbp-36, rEdi)
	rEdi = ld32(rEbp - 24)
	rEdi = (rEdi >> 5) | (rEbx << 27)
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	rEbx >>= 5
	st32(rEbp-56, rEdi)
	rEdi = ld32(rEbp - 20)
	st32(rEbp-52, rEbx)
	st32(rEbp-52, ld32(rEbp-52)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 4)
	rEbx = ld32(rEbp - 56)
	r17 = uint64(rEbx) + uint64(ld32(rEdi))
	cf = uint32(r17 >> 32)
	rEbx = uint32(r17)
	st32(rEbp-24, rEbx)
	rEbx = ld32(rEbp - 52)
	r18 = uint64(rEbx) + uint64(ld32(rEdi+4)) + uint64(cf)
	cf = uint32(r18 >> 32)
	rEbx = uint32(r18)
	r19 = uint64(rEdi) + uint64(uint32(8))
	cf = uint32(r19 >> 32)
	rEdi = uint32(r19)
	st32(rEbp-4, rEdi)
	rEdi = ld32(rEbp - 40)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-20, rEbx)
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r20 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r20 >> 32)
	rEdi = uint32(r20)
	st32(rEbp-80, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r21 = uint64(rEdi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r21 >> 32)
	rEdi = uint32(r21)
	st32(rEbp-76, rEdi)
	rEdi = ld32(rEbp - 80)
	r22 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r22 >> 32)
	rEdi = uint32(r22)
	st32(rEbp-48, rEdi)
	rEdi = ld32(rEbp - 76)
	r23 = uint64(rEdi) + uint64(rEbx) + uint64(cf)
	cf = uint32(r23 >> 32)
	rEdi = uint32(r23)
	rEbx = ld32(rEbp - 48)
	st32(rEax+12, rEdi)
	st32(rEax+8, rEbx)
	rEbx = (rEbx >> 8) | (rEdi << 24)
	rEdi >>= 8
	st32(rEbp-8, rEdi)
	rEdi = rEbx
	rEbx = ld32(rEbp - 8)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r24 = uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r24 >> 32)
	rEdi = uint32(r24)
	st32(rEbp-32, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r25 = uint64(rEdi) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r25 >> 32)
	rEdi = uint32(r25)
	rEbx = ld32(rEbp - 32)
	st32(rEsi+12, rEdi)
	st32(rEbp-28, rEdi)
	st32(rEsi+8, rEbx)
	rEdi = ld32(rEax + 16)
	rEbx = ld32(rEbp - 20)
	st32(rEbp-40, rEdi)
	rEdi = ld32(rEax + 20)
	st32(rEbp-36, rEdi)
	rEdi = ld32(rEbp - 24)
	rEbx = (rEbx << 12) | (rEdi >> 20)
	rEdi <<= 12
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	st32(rEbp-60, rEbx)
	st32(rEbp-64, rEdi)
	rEdi = ld32(rEbp - 20)
	st32(rEbp-60, ld32(rEbp-60)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 4)
	rEbx = ld32(rEbp - 64)
	r26 = uint64(rEbx) + uint64(ld32(rEdi))
	cf = uint32(r26 >> 32)
	rEbx = uint32(r26)
	st32(rEbp-24, rEbx)
	rEbx = ld32(rEbp - 60)
	r27 = uint64(rEbx) + uint64(ld32(rEdi+4)) + uint64(cf)
	cf = uint32(r27 >> 32)
	rEbx = uint32(r27)
	r28 = uint64(rEdi) + uint64(uint32(8))
	cf = uint32(r28 >> 32)
	rEdi = uint32(r28)
	st32(rEbp-4, rEdi)
	rEdi = ld32(rEbp - 40)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r29 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r29 >> 32)
	rEdi = uint32(r29)
	st32(rEbp-20, rEbx)
	st32(rEbp-88, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r30 = uint64(rEdi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r30 >> 32)
	rEdi = uint32(r30)
	st32(rEbp-84, rEdi)
	rEdi = ld32(rEbp - 88)
	r31 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r31 >> 32)
	rEdi = uint32(r31)
	st32(rEbp-48, rEdi)
	rEdi = ld32(rEbp - 84)
	r32 = uint64(rEdi) + uint64(rEbx) + uint64(cf)
	cf = uint32(r32 >> 32)
	rEdi = uint32(r32)
	rEbx = ld32(rEbp - 48)
	st32(rEax+16, rEbx)
	rEbx = (rEbx >> 8) | (rEdi << 24)
	st32(rEax+20, rEdi)
	rEdi >>= 8
	st32(rEbp-8, rEdi)
	rEdi = rEbx
	rEbx = ld32(rEbp - 8)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r33 = uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r33 >> 32)
	rEdi = uint32(r33)
	st32(rEbp-32, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r34 = uint64(rEdi) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r34 >> 32)
	rEdi = uint32(r34)
	rEbx = ld32(rEbp - 32)
	st32(rEsi+20, rEdi)
	st32(rEsi+16, rEbx)
	r35 = uint64(rEsi) + uint64(uint32(0x18))
	cf = uint32(r35 >> 32)
	rEsi = uint32(r35)
	st32(rEbp-8, rEsi)
	rEsi = ld32(rEax + 24)
	st32(rEbp-28, rEdi)
	rEdi = ld32(rEbp - 20)
	st32(rEbp-40, rEsi)
	rEsi = ld32(rEax + 28)
	st32(rEbp-36, rEsi)
	rEsi = ld32(rEbp - 4)
	rEdi >>= 1
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	rEbx ^= rEbx
	cf = 0
	rEbx ^= ld32(rEbp - 20)
	cf = 0
	r36 = uint64(rEdi) + uint64(ld32(rEsi))
	cf = uint32(r36 >> 32)
	rEdi = uint32(r36)
	r37 = uint64(rEbx) + uint64(ld32(rEsi+4)) + uint64(cf)
	cf = uint32(r37 >> 32)
	rEbx = uint32(r37)
	r38 = uint64(rEsi) + uint64(uint32(8))
	cf = uint32(r38 >> 32)
	rEsi = uint32(r38)
	st32(rEbp-4, rEsi)
	rEsi = ld32(rEbp - 40)
	rEsi &= rEdx
	cf = 0
	st32(rEbp-24, rEdi)
	rEdi = ld32(rEsi + rEcx + 2064)
	rEsi = ld32(rEsi + rEcx + 2068)
	r39 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r39 >> 32)
	rEdi = uint32(r39)
	st32(rEbp-20, rEbx)
	r40 = uint64(rEsi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r40 >> 32)
	rEsi = uint32(r40)
	r41 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r41 >> 32)
	rEdi = uint32(r41)
	r42 = uint64(rEsi) + uint64(rEbx) + uint64(cf)
	cf = uint32(r42 >> 32)
	rEsi = uint32(r42)
	st32(rEax+24, rEdi)
	rEdi = (rEdi >> 8) | (rEsi << 24)
	st32(rEax+28, rEsi)
	rEdi &= rEdx
	cf = 0
	rEbx = ld32(rEdi + rEcx + 2068)
	rEsi >>= 8
	rEsi = ld32(rEdi + rEcx + 2064)
	r43 = uint64(rEax) + uint64(uint32(0x20))
	cf = uint32(r43 >> 32)
	rEax = uint32(r43)
	r44 = uint64(rEsi) + uint64(ld32(rEbp-40))
	cf = uint32(r44 >> 32)
	rEsi = uint32(r44)
	r45 = uint64(rEbx) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r45 >> 32)
	rEbx = uint32(r45)
	rEdi = rEsi
	rEsi = ld32(rEbp - 8)
	st32(rEsi, rEdi)
	st32(rEsi+4, rEbx)
	r46 = uint64(rEsi) + uint64(uint32(8))
	cf = uint32(r46 >> 32)
	rEsi = uint32(r46)
	st32(rEbp-32, rEdi)
	st32(rEbp-28, rEbx)
	r47 = uint64(rEax)
	r48 = uint64(ld32(rEbp - 12))
	r = r47 - r48
	cf = uint32(r >> 63)
	zf = r47 == r48
	if cf != 0 {
		goto L_40c468
	}
L_40c71c:
	rEdi = uint32(rEcx + 2064)
	st32(rEbp-4, rEdi)
	r49 = uint64(rEdi)
	r50 = uint64(ld32(rEbp - 12))
	r = r49 - r50
	cf = uint32(r >> 63)
	zf = r49 == r50
	if cf == 0 {
		goto L_40c9e7
	}
L_40c72e:
	rEdi = ld32(rEax)
	st32(rEbp-40, rEdi)
	rEdi = ld32(rEax + 4)
	st32(rEbp-36, rEdi)
	rEdi = ld32(rEbp - 24)
	rEbx = ld32(rEbp - 20)
	rEbx = (rEbx << 21) | (rEdi >> 11)
	rEbx ^= ld32(rEbp - 20)
	cf = 0
	rEdi <<= 21
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	rEbx = ^rEbx
	rEdi = ^rEdi
	st32(rEbp-92, rEbx)
	rEbx = ld32(rEbp - 4)
	r51 = uint64(rEdi) + uint64(ld32(rEbx))
	cf = uint32(r51 >> 32)
	rEdi = uint32(r51)
	st32(rEbp-24, rEdi)
	rEdi = ld32(rEbp - 92)
	r52 = uint64(rEdi) + uint64(ld32(rEbx+4)) + uint64(cf)
	cf = uint32(r52 >> 32)
	rEdi = uint32(r52)
	r53 = uint64(rEbx) + uint64(uint32(8))
	cf = uint32(r53 >> 32)
	rEbx = uint32(r53)
	st32(rEbp-20, rEdi)
	rEdi = ld32(rEbp - 40)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r54 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r54 >> 32)
	rEdi = uint32(r54)
	st32(rEbp-4, rEbx)
	st32(rEbp-88, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r55 = uint64(rEdi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r55 >> 32)
	rEdi = uint32(r55)
	st32(rEbp-84, rEdi)
	rEdi = ld32(rEbp - 88)
	r56 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r56 >> 32)
	rEdi = uint32(r56)
	st32(rEbp-48, rEdi)
	rEdi = ld32(rEbp - 84)
	r57 = uint64(rEdi) + uint64(ld32(rEbp-20)) + uint64(cf)
	cf = uint32(r57 >> 32)
	rEdi = uint32(r57)
	rEbx = ld32(rEbp - 48)
	st32(rEax+4, rEdi)
	st32(rEax, rEbx)
	rEbx = (rEbx >> 8) | (rEdi << 24)
	rEdi >>= 8
	st32(rEbp-8, rEdi)
	rEdi = rEbx
	rEbx = ld32(rEbp - 8)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r58 = uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r58 >> 32)
	rEdi = uint32(r58)
	st32(rEbp-32, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r59 = uint64(rEdi) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r59 >> 32)
	rEdi = uint32(r59)
	rEbx = ld32(rEbp - 32)
	st32(rEsi+4, rEdi)
	st32(rEbp-28, rEdi)
	st32(rEsi, rEbx)
	rEdi = ld32(rEax + 8)
	rEbx = ld32(rEbp - 20)
	st32(rEbp-40, rEdi)
	rEdi = ld32(rEax + 12)
	st32(rEbp-36, rEdi)
	rEdi = ld32(rEbp - 24)
	rEdi = (rEdi >> 5) | (rEbx << 27)
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	rEbx >>= 5
	st32(rEbp-64, rEdi)
	rEdi = ld32(rEbp - 20)
	st32(rEbp-60, rEbx)
	st32(rEbp-60, ld32(rEbp-60)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 4)
	rEbx = ld32(rEbp - 64)
	r60 = uint64(rEbx) + uint64(ld32(rEdi))
	cf = uint32(r60 >> 32)
	rEbx = uint32(r60)
	st32(rEbp-24, rEbx)
	rEbx = ld32(rEbp - 60)
	r61 = uint64(rEbx) + uint64(ld32(rEdi+4)) + uint64(cf)
	cf = uint32(r61 >> 32)
	rEbx = uint32(r61)
	r62 = uint64(rEdi) + uint64(uint32(8))
	cf = uint32(r62 >> 32)
	rEdi = uint32(r62)
	st32(rEbp-4, rEdi)
	rEdi = ld32(rEbp - 40)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-20, rEbx)
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r63 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r63 >> 32)
	rEdi = uint32(r63)
	st32(rEbp-80, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r64 = uint64(rEdi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r64 >> 32)
	rEdi = uint32(r64)
	st32(rEbp-76, rEdi)
	rEdi = ld32(rEbp - 80)
	r65 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r65 >> 32)
	rEdi = uint32(r65)
	st32(rEbp-48, rEdi)
	rEdi = ld32(rEbp - 76)
	r66 = uint64(rEdi) + uint64(rEbx) + uint64(cf)
	cf = uint32(r66 >> 32)
	rEdi = uint32(r66)
	rEbx = ld32(rEbp - 48)
	st32(rEax+12, rEdi)
	st32(rEax+8, rEbx)
	rEbx = (rEbx >> 8) | (rEdi << 24)
	rEdi >>= 8
	st32(rEbp-8, rEdi)
	rEdi = rEbx
	rEbx = ld32(rEbp - 8)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r67 = uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r67 >> 32)
	rEdi = uint32(r67)
	st32(rEbp-32, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r68 = uint64(rEdi) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r68 >> 32)
	rEdi = uint32(r68)
	rEbx = ld32(rEbp - 32)
	st32(rEsi+12, rEdi)
	st32(rEbp-28, rEdi)
	st32(rEsi+8, rEbx)
	rEdi = ld32(rEax + 16)
	rEbx = ld32(rEbp - 20)
	st32(rEbp-40, rEdi)
	rEdi = ld32(rEax + 20)
	st32(rEbp-36, rEdi)
	rEdi = ld32(rEbp - 24)
	rEbx = (rEbx << 12) | (rEdi >> 20)
	rEdi <<= 12
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	st32(rEbp-52, rEbx)
	st32(rEbp-56, rEdi)
	rEdi = ld32(rEbp - 20)
	st32(rEbp-52, ld32(rEbp-52)^rEdi)
	cf = 0
	rEdi = ld32(rEbp - 4)
	rEbx = ld32(rEbp - 56)
	r69 = uint64(rEbx) + uint64(ld32(rEdi))
	cf = uint32(r69 >> 32)
	rEbx = uint32(r69)
	st32(rEbp-24, rEbx)
	rEbx = ld32(rEbp - 52)
	r70 = uint64(rEbx) + uint64(ld32(rEdi+4)) + uint64(cf)
	cf = uint32(r70 >> 32)
	rEbx = uint32(r70)
	r71 = uint64(rEdi) + uint64(uint32(8))
	cf = uint32(r71 >> 32)
	rEdi = uint32(r71)
	st32(rEbp-4, rEdi)
	rEdi = ld32(rEbp - 40)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r72 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r72 >> 32)
	rEdi = uint32(r72)
	st32(rEbp-20, rEbx)
	st32(rEbp-72, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r73 = uint64(rEdi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r73 >> 32)
	rEdi = uint32(r73)
	st32(rEbp-68, rEdi)
	rEdi = ld32(rEbp - 72)
	r74 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r74 >> 32)
	rEdi = uint32(r74)
	st32(rEbp-48, rEdi)
	rEdi = ld32(rEbp - 68)
	r75 = uint64(rEdi) + uint64(rEbx) + uint64(cf)
	cf = uint32(r75 >> 32)
	rEdi = uint32(r75)
	rEbx = ld32(rEbp - 48)
	st32(rEax+16, rEbx)
	rEbx = (rEbx >> 8) | (rEdi << 24)
	st32(rEax+20, rEdi)
	rEdi >>= 8
	st32(rEbp-8, rEdi)
	rEdi = rEbx
	rEbx = ld32(rEbp - 8)
	rEdi &= rEdx
	cf = 0
	st32(rEbp-8, rEdi)
	rEdi = ld32(rEdi + rEcx + 2064)
	r76 = uint64(rEdi) + uint64(ld32(rEbp-40))
	cf = uint32(r76 >> 32)
	rEdi = uint32(r76)
	st32(rEbp-32, rEdi)
	rEdi = ld32(rEbp - 8)
	rEdi = ld32(rEdi + rEcx + 2068)
	r77 = uint64(rEdi) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r77 >> 32)
	rEdi = uint32(r77)
	rEbx = ld32(rEbp - 32)
	st32(rEsi+20, rEdi)
	st32(rEsi+16, rEbx)
	r78 = uint64(rEsi) + uint64(uint32(0x18))
	cf = uint32(r78 >> 32)
	rEsi = uint32(r78)
	st32(rEbp-8, rEsi)
	rEsi = ld32(rEax + 24)
	st32(rEbp-28, rEdi)
	rEdi = ld32(rEbp - 20)
	st32(rEbp-40, rEsi)
	rEsi = ld32(rEax + 28)
	st32(rEbp-36, rEsi)
	rEsi = ld32(rEbp - 4)
	rEdi >>= 1
	rEdi ^= ld32(rEbp - 24)
	cf = 0
	rEbx ^= rEbx
	cf = 0
	rEbx ^= ld32(rEbp - 20)
	cf = 0
	r79 = uint64(rEdi) + uint64(ld32(rEsi))
	cf = uint32(r79 >> 32)
	rEdi = uint32(r79)
	r80 = uint64(rEbx) + uint64(ld32(rEsi+4)) + uint64(cf)
	cf = uint32(r80 >> 32)
	rEbx = uint32(r80)
	r81 = uint64(rEsi) + uint64(uint32(8))
	cf = uint32(r81 >> 32)
	rEsi = uint32(r81)
	st32(rEbp-4, rEsi)
	rEsi = ld32(rEbp - 40)
	rEsi &= rEdx
	cf = 0
	st32(rEbp-24, rEdi)
	rEdi = ld32(rEsi + rEcx + 2064)
	rEsi = ld32(rEsi + rEcx + 2068)
	r82 = uint64(rEdi) + uint64(ld32(rEbp-32))
	cf = uint32(r82 >> 32)
	rEdi = uint32(r82)
	st32(rEbp-20, rEbx)
	r83 = uint64(rEsi) + uint64(ld32(rEbp-28)) + uint64(cf)
	cf = uint32(r83 >> 32)
	rEsi = uint32(r83)
	r84 = uint64(rEdi) + uint64(ld32(rEbp-24))
	cf = uint32(r84 >> 32)
	rEdi = uint32(r84)
	r85 = uint64(rEsi) + uint64(rEbx) + uint64(cf)
	cf = uint32(r85 >> 32)
	rEsi = uint32(r85)
	st32(rEax+24, rEdi)
	rEdi = (rEdi >> 8) | (rEsi << 24)
	st32(rEax+28, rEsi)
	rEdi &= rEdx
	cf = 0
	rEbx = ld32(rEdi + rEcx + 2068)
	rEsi >>= 8
	rEsi = ld32(rEdi + rEcx + 2064)
	r86 = uint64(rEax) + uint64(uint32(0x20))
	cf = uint32(r86 >> 32)
	rEax = uint32(r86)
	r87 = uint64(rEsi) + uint64(ld32(rEbp-40))
	cf = uint32(r87 >> 32)
	rEsi = uint32(r87)
	r88 = uint64(rEbx) + uint64(ld32(rEbp-36)) + uint64(cf)
	cf = uint32(r88 >> 32)
	rEbx = uint32(r88)
	rEdi = rEsi
	rEsi = ld32(rEbp - 8)
	st32(rEsi+4, rEbx)
	st32(rEbp-28, rEbx)
	rEbx = ld32(rEbp - 4)
	st32(rEsi, rEdi)
	r89 = uint64(rEsi) + uint64(uint32(8))
	cf = uint32(r89 >> 32)
	rEsi = uint32(r89)
	st32(rEbp-32, rEdi)
	r90 = uint64(rEbx)
	r91 = uint64(ld32(rEbp - 12))
	r = r90 - r91
	cf = uint32(r >> 63)
	zf = r90 == r91
	if cf != 0 {
		goto L_40c72e
	}
	goto L_40c9ea
L_40c9e7:
	rEdi = ld32(rEbp - 32)
L_40c9ea:
	rEax = ld32(rEbp - 28)
	st32(rEcx+4124, rEax)
	rEax = ld32(rEbp - 24)
	st32(rEcx+4120, rEdi)
	rEdi = ld32(rEsp)
	rEsp += 4
	st32(rEcx+4112, rEax)
	rEax = ld32(rEbp - 20)
	rEsi = ld32(rEsp)
	rEsp += 4
	st32(rEcx+4116, rEax)
	rEbx = ld32(rEsp)
	rEsp += 4
	rEsp = rEbp
	rEbp = ld32(rEsp)
	rEsp += 4
	return
}
