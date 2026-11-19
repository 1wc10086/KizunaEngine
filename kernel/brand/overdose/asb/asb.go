// Package asb 是 kernel 侧的 Over-Dose Moon 剧本 codec。
//
// 原版剧本是 script.arc 里的 ASB：16 字节容器头 + dword 加法掩码 + zlib，
// 解压后是“指令流”，每条指令 12 字节头 + 若干参数节点，字符串节点是
// 零结尾 CP932。汉化流程需要把单条剧本解成 JSON、翻译、再编回 UTF-8，
// 所以这里必须同时做容器、指令 size、跳转 field 三层的重算——只改字符串
// 字节而不修 size/field，VM 会跳到半条指令上。
package asb

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

const (
	FeatureID   = "overdose.asb"
	FeatureName = "Over-Dose ASB"
	magic       = "ASB\x00"
	headerSize  = 16
	// instrMin 是 12 字节指令头（size/op/field/argc/line）。
	instrMin  = 12
	container = "asb-plain"
	// ASBSectionKind 是 ASB 文档里保存“原始解压指令流”的 section 名。
	// ARC codec 复用它来按条目重建子文档。
	ASBSectionKind = container
)

// DecodeBytes/EncodeDocument 是给同一品牌的 ARC codec 用的包级入口：
// codec 类型不导出，但 ARC 需要把每个 .asb 条目当子文档解码/重编。
func DecodeBytes(raw []byte, name string) (*core.Document, error) {
	return codec{}.Decode(raw, name)
}

func EncodeDocument(doc *core.Document) ([]byte, error) {
	return codec{}.Encode(doc)
}

type codec struct{}

func init() {
	core.Register(codec{})
}

func (codec) ID() string          { return FeatureID }
func (codec) Name() string        { return FeatureName }
func (codec) Target() core.Target { return core.TargetFile }

func (codec) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".asb"}, OutputSuffix: ".asb.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".asb.json"}, OutputSuffix: ""},
	}
}

func (codec) DetectFormat(path string, head []byte) (string, bool) {
	if strings.HasSuffix(strings.ToLower(path), ".asb.json") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		var probe struct {
			Feature string `json:"feature"`
			Format  string `json:"format"`
		}
		if err := json.Unmarshal(data, &probe); err != nil || probe.Feature != FeatureID {
			return "", false
		}
		if probe.Format != "" {
			return probe.Format, true
		}
		return "ASB", true
	}
	if len(head) >= 4 && string(head[:4]) == magic {
		return "ASB", true
	}
	return "", false
}

func (c codec) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	switch action {
	case "decode":
		return core.RunCodecDecode(c, in, out)
	case "encode":
		return core.RunCodecEncode(c, in, out)
	default:
		return core.Result{}, fmt.Errorf("overdose.asb: unknown action %q", action)
	}
}

func (codec) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (codec) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

// -------- 指令流结构 --------

type argNode struct {
	typ      byte
	start    int // 节点头在 plain 中的起点
	size     int
	payloadS int
	payloadE int
}

type instruction struct {
	offset int
	size   int
	op     uint16
	field  uint32
	argc   uint16
	line   uint16
	args   []argNode
}

// parseStream 按每条指令的 size 推进；任何越界都报错，绝不猜长度。
// 参数节点从指令头之后一直解析到 size 末尾，必须恰好填满。
func parseStream(plain []byte, name string) ([]instruction, error) {
	var out []instruction
	for pc := 0; pc < len(plain); {
		if pc+instrMin > len(plain) {
			return nil, fmt.Errorf("%s: 偏移 %#x 剩余 %d 字节放不下指令头", name, pc, len(plain)-pc)
		}
		size := int(binary.LittleEndian.Uint16(plain[pc:]))
		if size < instrMin || pc+size > len(plain) {
			return nil, fmt.Errorf("%s: 偏移 %#x 指令 size %d 越界", name, pc, size)
		}
		ins := instruction{
			offset: pc,
			size:   size,
			op:     binary.LittleEndian.Uint16(plain[pc+2:]),
			field:  binary.LittleEndian.Uint32(plain[pc+4:]),
			argc:   binary.LittleEndian.Uint16(plain[pc+8:]),
			line:   binary.LittleEndian.Uint16(plain[pc+10:]),
		}
		pos := pc + instrMin
		end := pc + size
		for pos < end {
			if pos+2 > end {
				return nil, fmt.Errorf("%s: 偏移 %#x 参数节点头越界", name, pos)
			}
			nsz := int(plain[pos])
			if nsz < 2 || pos+nsz > end {
				return nil, fmt.Errorf("%s: 偏移 %#x 参数节点 size %d 越界", name, pos, nsz)
			}
			ins.args = append(ins.args, argNode{
				typ:      plain[pos+1],
				start:    pos,
				size:     nsz,
				payloadS: pos + 2,
				payloadE: pos + nsz,
			})
			pos += nsz
		}
		if len(ins.args) != int(ins.argc) {
			return nil, fmt.Errorf("%s: 偏移 %#x argc %d 与实际节点 %d 不符", name, pc, ins.argc, len(ins.args))
		}
		out = append(out, ins)
		pc += size
	}
	return out, nil
}

// -------- 容器加解密 --------

func decompress(raw []byte) ([]byte, error) {
	if len(raw) < headerSize || string(raw[:4]) != magic {
		return nil, fmt.Errorf("asb: 魔数不符")
	}
	encSize := binary.LittleEndian.Uint32(raw[4:8])
	uncompSize := binary.LittleEndian.Uint32(raw[8:12])
	if encSize != uint32(len(raw)-headerSize) {
		return nil, fmt.Errorf("asb: 加密长度 %d 与实际 %d 不符", encSize, len(raw)-headerSize)
	}
	key := uncompSize ^ 0x9e370001
	body := raw[headerSize:]
	n := len(body) / 4
	plainBody := make([]byte, len(body))
	for i := 0; i < n; i++ {
		w := binary.LittleEndian.Uint32(body[i*4:])
		binary.LittleEndian.PutUint32(plainBody[i*4:], w-key)
	}
	copy(plainBody[n*4:], body[n*4:])
	zr, err := zlib.NewReader(bytes.NewReader(plainBody))
	if err != nil {
		return nil, fmt.Errorf("asb: zlib 起始失败 %w", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("asb: zlib 解压失败 %w", err)
	}
	if uncompSize != 0 && uint32(len(out)) != uncompSize {
		return nil, fmt.Errorf("asb: 解压长度 %d 与头 %d 不符", len(out), uncompSize)
	}
	return out, nil
}

func compress(plain []byte, instrCount int) ([]byte, error) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(plain); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	compressed := buf.Bytes()

	key := uint32(len(plain)) ^ 0x9e370001
	n := len(compressed) / 4
	body := make([]byte, len(compressed))
	for i := 0; i < n; i++ {
		w := binary.LittleEndian.Uint32(compressed[i*4:])
		binary.LittleEndian.PutUint32(body[i*4:], w+key)
	}
	copy(body[n*4:], compressed[n*4:])

	out := make([]byte, headerSize+len(body))
	copy(out[:4], magic)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(body)))
	binary.LittleEndian.PutUint32(out[8:], uint32(len(plain)))
	binary.LittleEndian.PutUint32(out[12:], uint32(instrCount))
	copy(out[headerSize:], body)
	return out, nil
}

// -------- 文本 --------

// decodeText 原版是 CP932，汉化后是 UTF-8。纯 ASCII 两种解码一致；
// 含高位字节时用严格 UTF-8 校验区分，CP932 双字节序列几乎不可能合法 UTF-8。
func decodeText(payload []byte) string {
	if len(payload) >= 3 && payload[0] == 0xEF && payload[1] == 0xBB && payload[2] == 0xBF {
		payload = payload[3:]
	}
	if utf8.Valid(payload) && hasNonASCII(payload) {
		return string(payload)
	}
	return textcodec.DecodeCP932(payload)
}

func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

func stripZero(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

func encodeText(text string, encoding string) ([]byte, error) {
	text = strings.ReplaceAll(text, "\x00", "")
	if textcodec.CanonicalName(encoding) == "cp932" {
		out, err := textcodec.EncodeCP932(text)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	return []byte(text), nil
}

// -------- Decode --------

func (codec) Decode(data []byte, name string) (*core.Document, error) {
	plain, err := decompress(data)
	if err != nil {
		return nil, err
	}
	ins, err := parseStream(plain, name)
	if err != nil {
		return nil, err
	}
	doc := &core.Document{
		Feature:  FeatureID,
		Format:   "ASB",
		Version:  "1",
		Encoding: "utf-8",
		// 原始指令流整段带走：Encode 只替换文本节点并重算 size/field，
		// 其余字节（未知节点类型、尾部数据）原样保留，避免“解析不了的
		// 节点被静默丢弃”这种翻译后跑不动的问题。
		Sections: []core.Section{{
			Kind:     container,
			Name:     name,
			Offset:   0,
			Size:     len(plain),
			Base64:   base64.StdEncoding.EncodeToString(plain),
			Encoding: "utf-8",
		}},
		Meta: core.Meta{Values: map[string]string{
			"instructions": strconv.Itoa(len(ins)),
		}},
	}
	for ii := range ins {
		unit := func(ai int, kind string, speaker string) {
			node := ins[ii].args[ai]
			if node.typ != 7 {
				return
			}
			text := decodeText(stripZero(plain[node.payloadS:node.payloadE]))
			if text == "" {
				return
			}
			doc.Units = append(doc.Units, core.Unit{
				ID:      fmt.Sprintf("asb:%d:%d", ii, ai),
				Kind:    kind,
				Speaker: speaker,
				Text:    text,
			})
		}
		switch ins[ii].op {
		case 0x1f:
			// 显示一行：STR(0)=说话人，STR(1)=正文，后续偶有多余字符串。
			// 说话人单独作为 name 单元；正文单元的 Speaker 也填上，翻译模型
			// 才有上下文，不会把“谢谢你”翻成对空气说话。
			speaker := ""
			if len(ins[ii].args) > 0 && ins[ii].args[0].typ == 7 {
				speaker = decodeText(stripZero(plain[ins[ii].args[0].payloadS:ins[ii].args[0].payloadE]))
			}
			for ai := range ins[ii].args {
				kind := "dialogue"
				sp := speaker
				if ai == 0 {
					kind = "name"
					sp = ""
				}
				unit(ai, kind, sp)
			}
		case 0x1c:
			// 简单选项：INT(0)=idx，STR(1)=文本，BOOL(2)=enabled。
			if len(ins[ii].args) > 1 {
				unit(1, "choice", "")
			}
		case 0x1d:
			// 高级选项：INT(0)=idx，STR(1)=内部名，INT(2/3)=坐标，STR(4)=文本。
			if len(ins[ii].args) > 4 {
				unit(4, "choice", "")
			}
		}
	}
	return doc, nil
}

// -------- Encode --------

func (codec) Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("overdose.asb: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	var section *core.Section
	for i := range doc.Sections {
		if doc.Sections[i].Kind == container {
			section = &doc.Sections[i]
			break
		}
	}
	if section == nil || section.Base64 == "" {
		return nil, fmt.Errorf("overdose.asb: missing %s section", container)
	}
	plain, err := base64.StdEncoding.DecodeString(section.Base64)
	if err != nil {
		return nil, fmt.Errorf("overdose.asb: bad section base64: %w", err)
	}
	ins, err := parseStream(plain, section.Name)
	if err != nil {
		return nil, err
	}
	// ID -> 新文本；ID 里的指令下标/参数下标与 parseStream 的顺序一一对应。
	repl := make(map[string]string, len(doc.Units))
	for _, u := range doc.Units {
		repl[u.ID] = u.Text
	}

	var out bytes.Buffer
	oldToNew := make(map[int]int, len(ins)+1)
	type jumpFix struct {
		newOffset int
		field     uint32
		op        uint16
	}
	var fixes []jumpFix

	for ii := range ins {
		cur := ins[ii]
		start := out.Len()
		oldToNew[cur.offset] = start
		out.Write(make([]byte, instrMin)) // 先占位，参数写在其后
		for ai, node := range cur.args {
			if node.typ == 7 {
				if text, ok := repl[fmt.Sprintf("asb:%d:%d", ii, ai)]; ok {
					payload, err := encodeText(text, doc.Encoding)
					if err != nil {
						return nil, fmt.Errorf("overdose.asb: unit %d:%d: %w", ii, ai, err)
					}
					// ASB 节点的 size 只有 1 字节：2 字节节点头 + 文本 + 1 字节 0
					// 必须 <= 255。超长译文无法用“再拆一个节点”绕开，因为那会改变
					// argc 和参数位置；这里明确报错让翻译端缩短。
					if len(payload)+3 > 255 {
						return nil, fmt.Errorf("overdose.asb: unit %d:%d 译文字节 %d 超过单节点上限 253", ii, ai, len(payload))
					}
					out.WriteByte(byte(len(payload) + 3)) // size = type + payload + 结尾 0
					out.WriteByte(7)
					out.Write(payload)
					out.WriteByte(0)
					continue
				}
			}
			out.Write(plain[node.start : node.start+node.size])
		}
		size := out.Len() - start
		if size > 0xFFFF {
			return nil, fmt.Errorf("overdose.asb: 偏移 %#x 指令重写后过大 %d", cur.offset, size)
		}
		header := make([]byte, instrMin)
		binary.LittleEndian.PutUint16(header[0:], uint16(size))
		binary.LittleEndian.PutUint16(header[2:], cur.op)
		binary.LittleEndian.PutUint32(header[4:], cur.field)
		binary.LittleEndian.PutUint16(header[8:], cur.argc)
		binary.LittleEndian.PutUint16(header[10:], cur.line)
		copy(out.Bytes()[start:], header)
		fixes = append(fixes, jumpFix{newOffset: start, field: cur.field, op: cur.op})
	}
	oldToNew[len(plain)] = out.Len()

	rebuilt := out.Bytes()
	for _, fix := range fixes {
		switch fix.op {
		case 0x04, 0x05, 0x06, 0x07:
			// 跳转 field 是“目标指令在解压流里的字节偏移”。字符串长度变了，
			// 所有后续指令都会平移；不 remap 会跳到旧偏移的半个节点上。
			// 0x07（else/块尾）在部分版本里也带 field 跳转，一并重算。
			if mapped, ok := oldToNew[int(fix.field)]; ok {
				binary.LittleEndian.PutUint32(rebuilt[fix.newOffset+4:], uint32(mapped))
			}
		}
	}
	return compress(rebuilt, len(ins))
}
