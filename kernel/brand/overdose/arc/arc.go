// Package arc 是 kernel 侧的 Over-Dose script.arc codec：直接把整座归档
// 解成一个文档（内含所有 .asb 子文档），翻译后整体重打包。
//
// 为什么不要求用户先手工抽出 .asb：
//   - script.arc 是加密归档，普通翻译工具解不开；
//   - 逐文件翻译后再让用户重打包，入口/索引/加密任何一步错都会让游戏黑屏。
//
// 重新打包必须重算三层：每条 entry 的加密种子（= 存储长度）、索引的
// 压缩/加密（种子 = 最终文件大小）、头部（种子同样是最终文件大小）。
// 因此文件大小只能等数据区和索引都算完后再回填头部。
package arc

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"kizuna/kernel/brand/overdose/asb"
	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

const (
	FeatureID   = "overdose.arc"
	FeatureName = "Over-Dose Script ARC"
	headerSize  = 0x30
	indexEntry  = 48
	nameSize    = 0x20
	headMagic   = "ARC\x00"

	sectionIndex = "arc-index"
	sectionEntry = "arc-entry"
	encodingASB  = "asb"
	encodingRaw  = "raw"
)

type codec struct{}

func init() {
	core.Register(codec{})
}

func (codec) ID() string          { return FeatureID }
func (codec) Name() string        { return FeatureName }
func (codec) Target() core.Target { return core.TargetFile }

func (codec) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".arc"}, OutputSuffix: ".arc.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".arc.json"}, OutputSuffix: ""},
	}
}

func (codec) DetectFormat(path string, head []byte) (string, bool) {
	if strings.HasSuffix(strings.ToLower(path), ".arc.json") {
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
		return "ARC", true
	}
	if !strings.HasSuffix(strings.ToLower(path), ".arc") {
		return "", false
	}
	a, err := open(path)
	if err != nil {
		return "", false
	}
	// graphic.arc/music.arc 也是 ARC；只有含 .asb 条目的归档才当剧本源。
	// 索引已在 open 里解出，检查名字不额外读大文件。
	for _, e := range a.entries {
		if strings.HasSuffix(strings.ToLower(e.name), ".asb") {
			return "ARC", true
		}
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
		return core.Result{}, fmt.Errorf("overdose.arc: unknown action %q", action)
	}
}

func (codec) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (codec) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

// -------- 归档读取 --------

type entry struct {
	offset uint32
	size   uint32
	h1     uint32
	h2     uint32
	name   string
}

type archive struct {
	header  []byte // 解密后的 48 字节头
	index   []byte // 解密、解压后的索引原始记录
	entries []entry
	data    []byte
}

func open(path string) (*archive, error) {
	// 探测只读头 + 索引：graphic.arc 有 1.1GB，绝不能为了判断“是不是
	// 剧本归档”把整文件读进内存。条目数据区在 Decode 时才按需读取。
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size < headerSize+4 {
		return nil, fmt.Errorf("arc: 文件过短")
	}
	hdrEnc := make([]byte, headerSize)
	if _, err := f.ReadAt(hdrEnc, 0); err != nil {
		return nil, err
	}
	table := GenKeyTable(uint32(size))
	hdr := Decrypt(hdrEnc, table, 0)
	if string(hdr[0:4]) != headMagic {
		return nil, fmt.Errorf("arc: 魔数不符 %q", hdr[0:4])
	}
	cnt := binary.LittleEndian.Uint32(hdr[8:12])
	isz := binary.LittleEndian.Uint32(hdr[12:16])
	if cnt > 1<<20 {
		return nil, fmt.Errorf("arc: 条目数 %d 过大", cnt)
	}
	if headerSize+int64(isz) > size {
		return nil, fmt.Errorf("arc: 索引越界")
	}
	encIdx := make([]byte, int(isz))
	if _, err := f.ReadAt(encIdx, headerSize); err != nil {
		return nil, err
	}
	plainIdx := Decrypt(encIdx, table, headerSize)
	index, err := inflate(plainIdx)
	if err != nil {
		return nil, fmt.Errorf("arc: 索引解压失败 %w", err)
	}
	return indexArchive(hdr, index, cnt, nil)
}

// indexArchive 把已解出的头/索引装成 archive；data 可空（探测路径不读数据区）。
func indexArchive(hdr, index []byte, cnt uint32, data []byte) (*archive, error) {
	if len(index) != int(cnt)*indexEntry {
		return nil, fmt.Errorf("arc: 索引长度 %d 与 %d 条目不符", len(index), cnt)
	}
	entries := make([]entry, cnt)
	for i := 0; i < int(cnt); i++ {
		rec := index[i*indexEntry:]
		nameBytes := rec[16 : 16+nameSize]
		if j := bytes.IndexByte(nameBytes, 0); j >= 0 {
			nameBytes = nameBytes[:j]
		}
		entries[i] = entry{
			offset: binary.LittleEndian.Uint32(rec[0:4]),
			size:   binary.LittleEndian.Uint32(rec[4:8]),
			h1:     binary.LittleEndian.Uint32(rec[8:12]),
			h2:     binary.LittleEndian.Uint32(rec[12:16]),
			name:   decodeCP932(nameBytes),
		}
	}
	return &archive{
		header:  hdr,
		index:   index,
		entries: entries,
		data:    data,
	}, nil
}

// payload 取一条 entry 的逻辑内容：ARC 层解密 + 可选 zlib。
func (a *archive) payload(i int) ([]byte, bool, error) {
	e := a.entries[i]
	start := int(e.offset)
	end := start + int(e.size)
	if start < 0 || end > len(a.data) || end < start {
		return nil, false, fmt.Errorf("arc: 条目 %q 偏移越界", e.name)
	}
	plain := Decrypt(a.data[start:end], GenKeyTable(e.size), 0)
	if len(plain) > 0 && plain[0] == 0x78 {
		if z, err := inflate(plain); err == nil {
			return z, true, nil
		}
	}
	return plain, false, nil
}

// -------- Decode --------

func (codec) Decode(data []byte, name string) (*core.Document, error) {
	// Decode 拿到的是整文件字节；先落到临时目录会让大归档多一次 IO，
	// 这里复用内存解析路径。
	a, err := parseBytes(data)
	if err != nil {
		return nil, err
	}
	doc := &core.Document{
		Feature:  FeatureID,
		Format:   "ARC",
		Version:  "1",
		Encoding: "utf-8",
		Meta: core.Meta{Values: map[string]string{
			"header": base64.StdEncoding.EncodeToString(a.header),
			"count":  fmt.Sprintf("%d", len(a.entries)),
		}},
		Sections: []core.Section{{
			Kind:   sectionIndex,
			Name:   name,
			Base64: base64.StdEncoding.EncodeToString(a.index),
			Size:   len(a.index),
		}},
	}
	for i := range a.entries {
		e := a.entries[i]
		payload, _, err := a.payload(i)
		if err != nil {
			return nil, err
		}
		sec := core.Section{
			Kind:   sectionEntry,
			Name:   e.name,
			Size:   int(e.size),
			Offset: int(e.offset),
		}
		if strings.HasSuffix(strings.ToLower(e.name), ".asb") && len(payload) >= 4 && string(payload[:4]) == "ASB\x00" {
			sub, err := asb.DecodeBytes(payload, e.name)
			if err != nil {
				return nil, fmt.Errorf("arc: %s: %w", e.name, err)
			}
			sec.Encoding = encodingASB
			// ASB 文档只有一个 asb-plain section；把原始指令流搬到 ARC 条目里，
			// 这样 ARC 文档本身就足够重建，不依赖临时文件。
			if len(sub.Sections) == 0 {
				return nil, fmt.Errorf("arc: %s: ASB 文档缺少原始流", e.name)
			}
			sec.Base64 = sub.Sections[0].Base64
			prefix := e.name + "#"
			for _, u := range sub.Units {
				doc.Units = append(doc.Units, core.Unit{
					ID:      prefix + u.ID,
					Kind:    u.Kind,
					Speaker: u.Speaker,
					Text:    u.Text,
				})
			}
		} else {
			// 非 ASB 条目原样保留**存储字节**（已经是 ARC 加密后的形态），
			// 编码时直接复制，避免还要记住它原来是否 zlib。
			start, end := int(e.offset), int(e.offset)+int(e.size)
			if start < 0 || end > len(a.data) || end < start {
				return nil, fmt.Errorf("arc: 条目 %q 偏移越界", e.name)
			}
			sec.Encoding = encodingRaw
			sec.Base64 = base64.StdEncoding.EncodeToString(a.data[start:end])
		}
		doc.Sections = append(doc.Sections, sec)
	}
	return doc, nil
}

// -------- Encode --------

func (codec) Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("overdose.arc: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	headerB64 := doc.Meta.Values["header"]
	header, err := base64.StdEncoding.DecodeString(headerB64)
	if err != nil || len(header) != headerSize {
		return nil, fmt.Errorf("overdose.arc: 头部缺失或长度不符")
	}
	var index []byte
	var sections []core.Section
	for _, sec := range doc.Sections {
		switch sec.Kind {
		case sectionIndex:
			index, err = base64.StdEncoding.DecodeString(sec.Base64)
			if err != nil {
				return nil, fmt.Errorf("overdose.arc: 索引 base64: %w", err)
			}
		case sectionEntry:
			sections = append(sections, sec)
		}
	}
	if index == nil || len(index)%indexEntry != 0 {
		return nil, fmt.Errorf("overdose.arc: 索引缺失或长度不符")
	}
	if len(sections) != len(index)/indexEntry {
		return nil, fmt.Errorf("overdose.arc: 条目数 %d 与索引 %d 不符", len(sections), len(index)/indexEntry)
	}

	// ID 前缀 → 译文，只取本条目自己的 unit，避免把同名脚本串味。
	byEntry := map[string][]core.Unit{}
	for _, u := range doc.Units {
		i := strings.Index(u.ID, "#")
		if i <= 0 {
			continue
		}
		byEntry[u.ID[:i]] = append(byEntry[u.ID[:i]], core.Unit{
			ID: u.ID[i+1:], Kind: u.Kind, Speaker: u.Speaker, Text: u.Text,
		})
	}

	var data bytes.Buffer
	for i := range sections {
		sec := &sections[i]
		rec := index[i*indexEntry:]
		var stored []byte
		if sec.Encoding == encodingASB {
			sub := &core.Document{
				Feature:  asb.FeatureID,
				Format:   "ASB",
				Version:  "1",
				Encoding: doc.Encoding,
				Sections: []core.Section{{
					Kind:     asb.ASBSectionKind,
					Name:     sec.Name,
					Base64:   sec.Base64,
					Encoding: "utf-8",
				}},
				Units: byEntry[sec.Name],
			}
			container, err := asb.EncodeDocument(sub)
			if err != nil {
				return nil, fmt.Errorf("overdose.arc: %s: %w", sec.Name, err)
			}
			compressed, err := deflate(container)
			if err != nil {
				return nil, err
			}
			stored = Decrypt(compressed, GenKeyTable(uint32(len(compressed))), 0)
		} else {
			stored, err = base64.StdEncoding.DecodeString(sec.Base64)
			if err != nil {
				return nil, fmt.Errorf("overdose.arc: %s 原始条目 base64: %w", sec.Name, err)
			}
		}
		binary.LittleEndian.PutUint32(rec[0:4], uint32(data.Len()))
		binary.LittleEndian.PutUint32(rec[4:8], uint32(len(stored)))
		data.Write(stored)
	}

	idxCompressed, err := deflate(index)
	if err != nil {
		return nil, err
	}
	isz := len(idxCompressed)
	finalSize := headerSize + isz + data.Len()
	table := GenKeyTable(uint32(finalSize))
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(sections)))
	binary.LittleEndian.PutUint32(header[12:16], uint32(isz))
	out := make([]byte, 0, finalSize)
	out = append(out, Decrypt(header, table, 0)...)
	out = append(out, Decrypt(idxCompressed, table, headerSize)...)
	out = append(out, data.Bytes()...)
	return out, nil
}

// -------- 容器工具 --------

func parseBytes(raw []byte) (*archive, error) {
	// 与 open 同逻辑，区别只是字节已在内存；抽出来避免 Decode 先写临时文件。
	if len(raw) < headerSize+4 {
		return nil, fmt.Errorf("arc: 文件过短")
	}
	size := uint32(len(raw))
	table := GenKeyTable(size)
	hdr := Decrypt(raw[:headerSize], table, 0)
	if string(hdr[0:4]) != headMagic {
		return nil, fmt.Errorf("arc: 魔数不符 %q", hdr[0:4])
	}
	cnt := binary.LittleEndian.Uint32(hdr[8:12])
	isz := binary.LittleEndian.Uint32(hdr[12:16])
	if headerSize+int64(isz) > int64(len(raw)) {
		return nil, fmt.Errorf("arc: 索引越界")
	}
	plainIdx := Decrypt(raw[headerSize:headerSize+int(isz)], table, headerSize)
	index, err := inflate(plainIdx)
	if err != nil {
		return nil, fmt.Errorf("arc: 索引解压失败 %w", err)
	}
	return indexArchive(hdr, index, cnt, raw[headerSize+int(isz):])
}

func inflate(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 512<<20))
}

func deflate(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeCP932(b []byte) string {
	return textcodec.DecodeCP932(b)
}
