// Package script 解析并执行 TMR-Hiro 的 .sct 剧本（srp.pac 里的条目）。
//
// 容器是“逐记录”的，不是字节流：
//
//	u32 count
//	count × { u16 recordLen; u16 chunk[recordLen] }
//
// chunk 前 4 字节是 op + 子命令，原样（未旋转）；其余是参数，**整段做过 nibble
// 交换**（原引擎装载时对参数逐字节执行 `b = (b<<4)|(b>>4)`），所以正文与
// 资源名都要先还原。正文是 CP932。
//
// op 就是命令组：
//
//	0x0000 文本：arg 0x00 旁白 / 0x20 有人名 / 0x30 人名+语音 / 0x60 心声 /
//	             0x02 与 0x20 同义（不同世代写不同值）/ 0x05、0x41、0x50 标记
//	0x0010 控制：0x00 标签 / 0x02/0x01 跳转(脚本内/换脚本) / 0x04 条件跳转 /
//	             0x14 选项 / 0x20 调用 / 0x23 地图 / 0x29/0x2b/0x32/0x34 按钮 /
//	             0x30 结束 / 0x40 条件 / 0x41 选项 / 0x43 立绘一览 / 0x82 模态 /
//	             0x92 地图定义 / 0xb2 切场景
//	0x0020 图像：0x00 载图 / 0x10、0x80 背景位移 / 0x8c 清立绘 / 0xbc 单立绘 /
//	             0xcb 多立绘 / 0xe6 滤镜 / 0x1009、0x1019 淡入淡出 / 0x10ef 导航 /
//	             0x01 立绘槽 / 0x08 背景 / 0x6e 图层 / 0xc8 全屏 CG
//	0x0030 音频：0x00 BGM / 0x8c、0x1003、0x10c2、0x10d2、0x1019 SE/语音 /
//	             0x10、0xac、0x10e2 停止 / 0x46 角色音 / 0x64 全局
//	0x0040 系统：0x28、0x29、0x2a staffroll 与开关 / 0x64 鉴赏 / 0x82…0xc3 设置
//	0x0050 杂项：0x14 章节号 / 0x15 章节名 / 0x51 标题 / 0x82 等待毫秒 / 0x1e 结束 /
//	             0x41 存档位 / 0x01、0x00 模式切换
//
// 参数里的“通道”在高半字节：0x8c/0x18c/0x28c/0x38c 是 0..3 号通道的 SE，
// 0x10c2/0x11c2/0x12c2 是 0..2 号通道的循环 SE。
package script

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"kizuna/enginego/core/textcodec"
)

// op 值是记录头里的第一个 u16，按“命令组”分：
//
//	0 文本、1 控制流、2 图像、3 音频、4 系统、5 杂项
//
// 注意它与“参数里的子命令”是两层：0x10/0x20 这类看着像 op 的数字全都是 arg。
const (
	OpText   uint16 = 0
	OpFlow   uint16 = 1
	OpImage  uint16 = 2
	OpAudio  uint16 = 3
	OpSystem uint16 = 4
	OpMisc   uint16 = 5
)

// Record 是一条指令。
type Record struct {
	Index int    // 记录号（存档存它）
	Op    uint16 // 命令组
	Arg   uint16 // 子命令
	Raw   []byte // 还原后的参数（CP932 文本或 ASCII 名字）
}

// Text 把参数按 CP932 解成 UTF-8。
func (r *Record) Text() string { return DecodeText(r.Raw) }

// Script 是一个已解析的剧本。
type Script struct {
	Name    string
	Records []Record
	Labels  map[string]int
	// Title 来自 op 5 arg 0x51（章节标题），存档列表显示用。
	Title string
	// ID 来自 op 5 arg 0x14（章节编号）。
	ID int
}

// rot4 是 nibble 交换；对合，加解密同一个函数。
func rot4(b byte) byte { return (b << 4) | (b >> 4) }

func rot4Slice(in []byte) []byte {
	out := make([]byte, len(in))
	for i, b := range in {
		out[i] = rot4(b)
	}
	return out
}

// IsScript 判断条目是不是 .sct：u32 记录数 + 逐条长度恰好铺满文件。
// 比“看第一条 op”可靠——空的/半截剧本也能判对。
func IsScript(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	count := int(binary.LittleEndian.Uint32(data[0:4]))
	if count <= 0 || count > 1<<20 {
		return false
	}
	pos := 4
	for i := 0; i < count; i++ {
		if pos+2 > len(data) {
			return false
		}
		ln := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2
		if ln < 4 || pos+ln > len(data) {
			return false
		}
		pos += ln
	}
	return pos == len(data)
}

// Parse 解析一个 .sct。
func Parse(name string, data []byte) (*Script, error) {
	if !IsScript(data) {
		return nil, fmt.Errorf("tmrhiro/script: %s 不是 .sct", name)
	}
	count := int(binary.LittleEndian.Uint32(data[0:4]))
	s := &Script{Name: name, Records: make([]Record, 0, count), Labels: make(map[string]int, 64)}
	pos := 4
	for i := 0; i < count; i++ {
		ln := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2
		chunk := rot4Slice(data[pos : pos+ln])
		pos += ln
		op := binary.LittleEndian.Uint16(chunk[0:2])
		arg := binary.LittleEndian.Uint16(chunk[2:4])
		s.Records = append(s.Records, Record{Index: i, Op: op, Arg: arg, Raw: chunk[4:]})
	}
	for i := range s.Records {
		r := &s.Records[i]
		if r.Op == OpFlow && r.Arg == 0x0000 {
			if label := string(r.Raw); label != "" {
				s.Labels[label] = r.Index
			}
		}
	}
	for i := range s.Records {
		r := &s.Records[i]
		switch {
		case r.Op == OpMisc && r.Arg == 0x51:
			s.Title = r.Text()
		case r.Op == OpMisc && r.Arg == 0x14:
			if v, err := strconv.Atoi(strings.TrimSpace(string(r.Raw))); err == nil {
				s.ID = v
			}
		}
	}
	return s, nil
}

// Label 返回标签的记录号，不存在 -1。
func (s *Script) Label(name string) int {
	if idx, ok := s.Labels[name]; ok {
		return idx
	}
	return -1
}

// DecodeText 把 CP932 字节解成 UTF-8。
func DecodeText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return textcodec.Decode("cp932", raw)
}

// SplitText 按逗号拆文本字段，最多 want 个（余下的留在最后一个字段里）。
//
// 正文里的逗号在本作里写作全角“、”（CP932 0x81 0x41），与分隔符 0x2C 不混。
// 仍按 want 截断，是为了防止未来某个正文出现半角逗号时把字段拆歪。
func SplitText(raw []byte, want int) []string {
	if want <= 0 {
		return nil
	}
	parts := make([]string, 0, want)
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != ',' || len(parts) >= want-1 {
			continue
		}
		parts = append(parts, DecodeText(raw[start:i]))
		start = i + 1
	}
	parts = append(parts, DecodeText(raw[start:]))
	return parts
}

// SplitRaw 同上但不解码，资源名（ASCII）用它。
func SplitRaw(raw []byte, want int) []string {
	if want <= 0 {
		return nil
	}
	parts := make([]string, 0, want)
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != ',' || len(parts) >= want-1 {
			continue
		}
		parts = append(parts, string(raw[start:i]))
		start = i + 1
	}
	parts = append(parts, string(raw[start:]))
	return parts
}

// AudioChannel 取通道号（0..3）：op 3 的参数高半字节。
func AudioChannel(op, arg uint16) uint32 {
	if op != OpAudio {
		return 0
	}
	switch {
	case arg >= 0x1000:
		return uint32((arg >> 8) & 0xF)
	case arg >= 0x0100:
		return uint32((arg >> 8) & 0x3)
	}
	return 0
}

// AudioCommand 去掉通道后的低字节命令。
func AudioCommand(arg uint16) uint16 { return arg & 0x00FF }
