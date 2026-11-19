package arc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"kizuna/enginego/core/textcodec"
)

// ARC 容器布局（overdose_spec §2）：
//
//	0x00 48 字节头（流种子=文件大小）
//	0x30 压缩索引（流种子=文件大小，起始下标 0x30）
//	索引后是条目数据区；条目偏移相对数据区计算。
//
// 读头必须走流密码而不是直接比魔数——存档头在盘上全是密文，
// 否则 graphic.arc（1.1G）这种大文件连头都打不开。
const (
	headerSize = 0x30
	indexEntry = 48
	nameSize   = 0x20
	headMagic  = "ARC\x00"
)

// Entry 是索引表的一条 48 字节记录（解密后的视图）。
// 只存定位需要的字段；nameHash 在原格式里存在但读入时不需要，
// 留显式弃用是为了对“linear parse 不依赖 hash”的契约负责。
type entryMeta struct {
	offset uint32
	size   uint32
	name   string
}

// Archive 是一座 .arc 容器：只保留文件句柄与内存索引，
// 不把全图进内存。1.1G 的 graphic.arc 若顺手读完所有条目，
// 必然把解压与会话两层都堵死。
type Archive struct {
	Path      string
	Count     int
	ID        uint32
	dataStart int64
	entries   []entryMeta
	byName    map[string]int // 折叠名 → 下标
	f         *os.File
	tsize     int // 索引压缩大小
}

// IsArchive 只读 48 字节头并解出魔数，用于检测与拒绝普通文件。
// 这里踩过的一个坑：光比前 4 字节会把密文的巧合当真头，
// 必须全表解密头再比魔数，才不会把同目录的 .sum 误判为存档。
func IsArchive(path string) bool {
	_, err := Open(path)
	return err == nil
}

// Open 打开并解析 ARC，任何一步不一致都返回错而非半公开对象。
// headerLen 先只用于窗口解密，真正的索引长度靠头里的 isz 字段。
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := info.Size()
	if size < headerSize+4 {
		f.Close()
		return nil, fmt.Errorf("arc: %s 太短", path)
	}
	seed := uint32(size)
	hdrEnc := make([]byte, headerSize)
	if _, err := f.ReadAt(hdrEnc, 0); err != nil {
		f.Close()
		return nil, err
	}
	table := GenKeyTable(seed)
	hdr := Decrypt(hdrEnc, table, 0)
	if string(hdr[0:4]) != headMagic {
		f.Close()
		return nil, fmt.Errorf("arc: %s 魔数不符 %q，期望 ARC\\0", path, hdr[0:4])
	}
	id := binary.LittleEndian.Uint32(hdr[4:8])
	cnt := binary.LittleEndian.Uint32(hdr[8:12])
	isz := binary.LittleEndian.Uint32(hdr[12:16])
	if cnt > 1<<20 {
		f.Close()
		return nil, fmt.Errorf("arc: %s 条目数 %d 过大", path, cnt)
	}
	dataStart := headerSize + int64(isz)
	if dataStart > size {
		f.Close()
		return nil, fmt.Errorf("arc: %s 索引越界", path)
	}
	encIdx := make([]byte, int64(isz))
	if int64(isz) > 0 {
		if _, err := f.ReadAt(encIdx, headerSize); err != nil {
			f.Close()
			return nil, err
		}
	}
	plainIdx := Decrypt(encIdx, table, headerSize)
	var raw []byte
	if len(plainIdx) == 0 {
		raw = plainIdx
	} else {
		raw, err = inflate(plainIdx)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("arc: %s 索引解压失败 %w", path, err)
		}
	}
	if int64(len(raw)) != int64(cnt)*indexEntry {
		f.Close()
		return nil, fmt.Errorf("arc: %s 索引长度 %d 与 %d 条目不符", path, len(raw), cnt)
	}
	entries := make([]entryMeta, int(cnt))
	byName := make(map[string]int, int(cnt))
	for i := 0; i < int(cnt); i++ {
		rec := raw[i*indexEntry : (i+1)*indexEntry]
		off := binary.LittleEndian.Uint32(rec[0:4])
		sz := binary.LittleEndian.Uint32(rec[4:8])
		// name 是 CP932 零结尾；截零后取原文，避免 name[...] 过长
		nameBytes := rec[16 : 16+nameSize]
		if j := bytes.IndexByte(nameBytes, 0); j >= 0 {
			nameBytes = nameBytes[:j]
		}
		name := textcodec.DecodeCP932(nameBytes)
		entries[i] = entryMeta{offset: off, size: sz, name: name}
		key := foldName(name)
		if _, dup := byName[key]; !dup {
			byName[key] = i
		}
	}
	return &Archive{
		Path:      path,
		Count:     int(cnt),
		ID:        id,
		dataStart: dataStart,
		entries:   entries,
		byName:    byName,
		f:         f,
		tsize:     int(isz),
	}, nil
}

// Close 关闭文件句柄；多次关闭无害（文件句柄已空则跳过）。
func (a *Archive) Close() error {
	if a == nil || a.f == nil {
		return nil
	}
	err := a.f.Close()
	a.f = nil
	return err
}

// find 是查名逻辑：原名、折叠名、无扩展名三级兜底。
// 无扩展名兜底是给脚本层准备的（表里记的图片名不带 .cpb）。
func (a *Archive) find(name string) (int, bool) {
	if a.byName == nil || name == "" {
		return 0, false
	}
	if i, ok := a.byName[foldName(name)]; ok {
		return i, true
	}
	// 允许调用方传无扩展名（表里的 CG 名就是这种形态）
	base := foldName(stripExt(name))
	if i, ok := a.byName[base]; ok {
		return i, true
	}
	for i, e := range a.entries {
		if foldName(stripExt(e.name)) == base {
			return i, true
		}
	}
	return 0, false
}

// Entry 取回并解密一个条目，遇 zlib 特征头则再解一层。
// 这里只把 0x78 开头的才试 zlib：sysenv.tbl 的原始 7364 字节
// 也属于此类但不是 zlib，硬试会啪一下炸；
// 试解压时只在本调用内吃光流，失败就退回解密明文。
func (a *Archive) Entry(name string) ([]byte, error) {
	idx, ok := a.find(name)
	if !ok {
		return nil, fmt.Errorf("arc: %s 无条目 %q", a.Path, name)
	}
	e := a.entries[idx]
	if e.size == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, e.size)
	if _, err := a.f.ReadAt(buf, a.dataStart+int64(e.offset)); err != nil {
		return nil, err
	}
	seed := e.size
	table := GenKeyTable(seed)
	plain := Decrypt(buf, table, 0)
	if len(plain) > 0 && plain[0] == 0x78 {
		if z, zerr := inflate(plain); zerr == nil {
			plain = z
		} else {
			// 大多数是 zlib，少数是“首字节恰好 0x78”的裸二进制（如 sysenv.tbl 是密文 0x78 也会蒙中）；
			// zlib 解开失败就认为该条目本就不是 zlib，保持 XOR 后明文。
		}
	}
	return plain, nil
}

// EntrySize 返回条目在索引里的存储长度（解密前口径）。
// 会话层用它确定文件类别与存在性，避免为判断绑带而整段解密。
func (a *Archive) EntrySize(name string) (int64, bool) {
	idx, ok := a.find(name)
	if !ok {
		return 0, false
	}
	return int64(a.entries[idx].size), true
}

// Names 返回条目名，ext 传后缀（.cpb / .tbl / .ogg），用不用点都行，
// 传空字符串摘回全部。保持索引顺序，方便失踪名按名字回问。
func (a *Archive) Names(ext string) []string {
	want := ""
	if ext != "" {
		want = strings.TrimPrefix(strings.ToLower(ext), ".")
	}
	out := make([]string, 0)
	for _, e := range a.entries {
		if want == "" || extOf(e.name) == want {
			out = append(out, e.name)
		}
	}
	return out
}

// Image 将图形条目解回 (宽, 高, BGRA)。
// 只认 TYP1；未知魔数早报错，不去猜 BGR/RGBA 方向，
// 因为猜错的叠图会全屏错色且没地方纠错。
func (a *Archive) Image(name string) (int, int, []byte, error) {
	raw, err := a.Entry(name)
	if err != nil {
		return 0, 0, nil, err
	}
	return DecodeCPB(raw)
}

// Table 将 .tbl 条目解回记录表（在 system.arc 里）。
// sysenv.tbl 不是 zlib、不是 TBL 格式，这里会明确报错留给调用方。
func (a *Archive) Table(name string) ([]Record, error) {
	raw, err := a.Entry(name)
	if err != nil {
		return nil, err
	}
	return ParseTable(raw)
}

// inflate 尝试 zlib；首字节 0x78 的也可能是裸二进制，失败由调用方兜底。
func inflate(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, 512<<20))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func stripExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return name
}

func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

func foldName(name string) string {
	return strings.ToLower(name)
}
