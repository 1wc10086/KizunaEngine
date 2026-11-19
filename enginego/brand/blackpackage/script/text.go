package script

import (
	"strconv"
	"strings"
)

// 本文件复刻原版 0x415af0 的文本/markup 展开器。它不只服务对白，所有资源名
// （BGM、语音、图片、CG）也走这里，所以“名字解析不出来”的根因通常在这里。
//
// 原版语法（0x41ab00 的 token 解析器 + 0x415f58 的指令分派表，已用反汇编+探针核对）：
//
//	$A<n>   / $a<n>   ASCII 十进制（负数带 '-'；flags=1 时先输出 '#'）
//	$N<n>   / $n<n>   CP932 全角十进制
//	$D<n>   / $d<n>   原始字节（CP932 前导字节时输出 2 字节）
//	$S<n>   / $s<n>   内联 buffer n，**内容会被递归展开**
//	$P<n>.<m>         从 buffer n 的字节偏移 m 处内联，同样递归展开
//	`<n>              反引号：内联 buffer n（等价 $S）
//	$.  $/            字面量转义（原版把这两个字符按普通字符输出）
//	$<其他>           原版输出 '$' 后继续按普通字符复制
//
// <n>/<m> 是 token：可选 'V'/'v' 前缀、可选 'R'/'r' 前缀，再接十进制。
//   - 无前缀：立即数 n
//   - R<n>：vars[n]
//   - VR<n>：vars[vars[n]]
//
// 语音名就是靠这个拼出来的，例如 `TR$ar13900.wa1` = "TR" + vars[13900] + ".wa1"，
// `$p127.r44274.wa1` = buffer127 从 vars[44274] 起的文本 + ".wa1"。漏掉任何一条，
// 对应语音/曲目就永远物化不出来（表现为“有事件但没声音”）。
func (v *VM) expand(s string) string {
	if s == "" {
		return ""
	}
	return v.expandDepth(s, 0)
}

// expandDepth 限制递归深度。原版没有显式上限，但 buffer 互相内联的坏数据会
// 无限膨胀；这里给 16 层，正常脚本最多 3~4 层。
func (v *VM) expandDepth(s string, depth int) string {
	if s == "" {
		return ""
	}
	if depth > 16 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); {
		c := s[i]
		if c != '$' && c != '`' {
			b.WriteByte(c)
			i++
			continue
		}
		if c == '`' {
			// 反引号必须紧跟 token；解析失败按字面量输出。
			j := i + 1
			n, ok := v.parseToken(s, &j)
			if !ok {
				b.WriteByte(c)
				i++
				continue
			}
			b.WriteString(v.expandDepth(v.bufferString(n), depth+1))
			i = j
			continue
		}
		// '$' 指令。原版把指令字符转大写后查表，所以 $s 与 $S 等价。
		if i+1 >= len(s) {
			b.WriteByte('$')
			break
		}
		op := s[i+1]
		if op >= 'a' && op <= 'z' {
			op -= 'a' - 'A'
		}
		switch op {
		case '.', '/':
			// 转义：输出下一个字符（原版 0x415ee4/0x415efa 两支都一样）。
			j := i + 2
			if j < len(s) {
				b.WriteByte(s[j])
				// 原版这两支会把后续普通字符继续复制，循环体统一处理。
				i = j + 1
			} else {
				i = j
			}
		case 'S':
			j := i + 2
			n, ok := v.parseToken(s, &j)
			if !ok {
				b.WriteByte('$')
				i++
				continue
			}
			b.WriteString(v.expandDepth(v.bufferString(n), depth+1))
			i = j
		case 'P':
			j := i + 2
			n, ok := v.parseToken(s, &j)
			if !ok || j >= len(s) || s[j] != '.' {
				b.WriteByte('$')
				i++
				continue
			}
			j++
			off, ok2 := v.parseToken(s, &j)
			if !ok2 {
				b.WriteByte('$')
				i++
				continue
			}
			b.WriteString(v.expandDepth(v.bufferSubstring(n, off), depth+1))
			i = j
		case 'D':
			j := i + 2
			val, ok := v.parseToken(s, &j)
			if !ok {
				b.WriteByte('$')
				i++
				continue
			}
			// 原版 0x415e85：低字节是 CP932 前导字节时写 2 字节（保留原样）。
			if isLeadByte(byte(val)) {
				b.WriteByte(byte(val))
				b.WriteByte(byte(val >> 8))
			} else {
				b.WriteByte(byte(val))
			}
			i = j
		case 'N':
			j := i + 2
			val, ok := v.parseToken(s, &j)
			if !ok {
				b.WriteByte('$')
				i++
				continue
			}
			b.WriteString(toWideDigits(val))
			i = j
		case 'A':
			j := i + 2
			val, ok := v.parseToken(s, &j)
			if !ok {
				b.WriteByte('$')
				i++
				continue
			}
			// flags=0 时负数只输出 '-'（与探针一致）。
			b.WriteString(strconv.Itoa(val))
			i = j
		default:
			b.WriteByte('$')
			i++
		}
	}
	return b.String()
}

// parseToken 复刻原版 0x41ab00：可选 V/v、R/r 前缀 + 十进制。
// 返回最终值；ok=false 表示这里不是 token（调用方应把 '$' 当字面量）。
func (v *VM) parseToken(s string, i *int) (int, bool) {
	pos := *i
	mode := 0 // 0=立即数, 1=vars[n], 2=vars[vars[n]]
	if pos < len(s) && (s[pos] == 'V' || s[pos] == 'v') {
		pos++
		mode = 2
		if pos < len(s) && (s[pos] == 'R' || s[pos] == 'r') {
			pos++
		} else {
			// 'V' 必须紧跟 'R'；否则不是 token。
			return 0, false
		}
	} else if pos < len(s) && (s[pos] == 'R' || s[pos] == 'r') {
		pos++
		mode = 1
	}
	start := pos
	n := 0
	for pos < len(s) && s[pos] >= '0' && s[pos] <= '9' {
		n = n*10 + int(s[pos]-'0')
		pos++
		if n > 1<<24 {
			break
		}
	}
	if pos == start {
		return 0, false
	}
	*i = pos
	switch mode {
	case 1:
		return int(v.varAt(int32(n))), true
	case 2:
		return int(v.varAt(v.varAt(int32(n)))), true
	default:
		return n, true
	}
}

// toWideDigits 把数字转成 CP932 全角数字（"１２３"）。
func toWideDigits(v int) string {
	s := strconv.Itoa(v)
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(rune('０' + (r - '0')))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isLeadByte 判断 CP932 前导字节。原版查 0x557621 的分类表（bit 0x04）；
// 这里用区间近似：0x81-0x9F、0xE0-0xFC 是双字节前导。
func isLeadByte(b byte) bool {
	return (b >= 0x81 && b <= 0x9f) || (b >= 0xe0 && b <= 0xfc)
}

func (v *VM) bufferString(buf int) string {
	if buf < 0 || buf >= numBuffers {
		return ""
	}
	return v.readCString(int32(buf), 0)
}

func (v *VM) bufferSubstring(buf, off int) string {
	if buf < 0 || buf >= numBuffers || off < 0 {
		return ""
	}
	return decodeCP932(v.readCStringAt(int32(buf), int32(off), 0x4000))
}
