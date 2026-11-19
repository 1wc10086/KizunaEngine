package disc

// sector.go 负责“镜像文件 -> 逻辑扇区”的映射。
//
// 光盘镜像不是简单的连续数据：CD RAW 每扇区 2352 字节（16 字节同步头 +
// 2048 字节用户数据 + 288 字节 ECC/EDC），DVD RAW 则是 2064 字节。更麻烦的是
// Alcohol/CloneCD 在拼接多段镜像时可能插入重复扇区，文件偏移不再等于逻辑 LBA。
// 参考实现（项目参考/1.py 的 SectorImage）以扇区头里的 MSF 为唯一权威，
// 这里采用同样的策略：只有检测到“文件扇区数 > 描述符声明扇区数”时才扫描建表，
// 正常镜像仍然走线性映射，避免每次打开都多花时间。

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type sectorLayout struct {
	size       int
	userOffset int
	name       string
}

// 顺序即探测优先级。2048 的纯 ISO 最常见，其次是 CD RAW。
var sectorLayouts = []sectorLayout{
	{2048, 0, "iso-2048"},
	{2352, 16, "cd-raw-2352"},
	{2352, 24, "cd-raw-2352-mode2"},
	{2448, 16, "cd-raw+sub-2448"},
	{2336, 8, "cd-mode2-2336"},
	{2324, 0, "cd-mode2-form1-2324"},
	{2064, 12, "dvd-raw-2064"},
	{2418, 0, "dvd-raw-ecc-2418"},
}

const isoMagic = "CD001"

type sectorImage struct {
	path       string
	file       *os.File
	size       int64
	layout     sectorLayout
	dataTrack  int64
	fileSector int64
	// lbaMap 只在镜像含重复/缺失扇区时非 nil：逻辑 LBA -> 物理扇区号。
	lbaMap map[uint32]uint32
	// maxLBA/tailShift 让“映射表最后一项之后”的逻辑 LBA 仍然能线性外推。
	maxLBA    uint32
	tailShift int64
}

func openSectorImage(path string) (*sectorImage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	image := &sectorImage{path: path, file: file, size: info.Size()}

	hint, dataTrack, declared, err := readDescriptorHint(path)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	image.dataTrack = dataTrack

	layout, ok := probeSectorLayout(file, image.size, dataTrack, hint)
	if !ok {
		_ = file.Close()
		return nil, fmt.Errorf("not an ISO9660 image: %s", path)
	}
	image.layout = layout
	image.fileSector = image.size / int64(layout.size)
	if layout.size == 2352 || layout.size == 2448 {
		// 只有声明值与实际扇区数不一致时才建表：正常镜像线性读取即可。
		if declared > 0 && image.fileSector > int64(declared)+1 {
			image.buildLBAMap(declared)
		}
	}
	return image, nil
}

func (i *sectorImage) Close() error {
	if i == nil || i.file == nil {
		return nil
	}
	return i.file.Close()
}

func probeSectorLayout(file *os.File, size int64, dataTrack int64, hint *sectorLayout) (sectorLayout, bool) {
	// cue 的 MODE1/2048 是数据轨布局的直接证据，先试它。
	if hint != nil {
		if layout, ok := probeOneLayout(file, size, dataTrack, *hint); ok {
			return layout, true
		}
	}
	best := sectorLayout{}
	bestScore := -1
	for _, layout := range sectorLayouts {
		score, ok := scoreLayout(file, size, dataTrack, layout)
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = layout
		}
	}
	// 参考实现的阈值是 10：只认真正带 CD001 与合法根目录记录的布局。
	return best, bestScore >= 10
}

func probeOneLayout(file *os.File, size int64, dataTrack int64, layout sectorLayout) (sectorLayout, bool) {
	score, ok := scoreLayout(file, size, dataTrack, layout)
	if !ok || score < 10 {
		return sectorLayout{}, false
	}
	return layout, true
}

func scoreLayout(file *os.File, size int64, dataTrack int64, layout sectorLayout) (int, bool) {
	offset := (dataTrack+16)*int64(layout.size) + int64(layout.userOffset)
	if offset < 0 || offset+2048 > size {
		return 0, false
	}
	vd := make([]byte, 2048)
	if _, err := file.ReadAt(vd, offset); err != nil {
		return 0, false
	}
	score := 0
	if vd[0] == 1 || vd[0] == 2 {
		score += 2
	}
	if string(vd[1:6]) == isoMagic {
		score += 10
	}
	if vd[6] == 1 {
		score++
	}
	if binary.LittleEndian.Uint16(vd[128:]) == 2048 {
		score += 2
	}
	root := vd[156:190]
	if root[0] == 34 {
		score += 2
		rootLBA := int64(binary.LittleEndian.Uint32(root[2:]))
		if rootLBA > 0 && rootLBA < size {
			score += 2
		}
		if root[25]&0x02 != 0 {
			score++
		}
	}
	if size%int64(layout.size) == 0 {
		score++
	}
	return score, true
}

func (i *sectorImage) buildLBAMap(declared uint32) {
	const chunkSectors = 1024
	chunk := make([]byte, i.layout.size*chunkSectors)
	mapping := make(map[uint32]uint32, declared)
	valid := 0
	var processed int64
	for processed < i.fileSector {
		want := chunkSectors
		if remain := i.fileSector - processed; remain < int64(want) {
			want = int(remain)
		}
		n, err := i.file.ReadAt(chunk[:i.layout.size*want], processed*int64(i.layout.size))
		if err != nil && err != io.EOF {
			return
		}
		sectors := n / i.layout.size
		for s := 0; s < sectors; s++ {
			header := chunk[s*i.layout.size:]
			lba, ok := sectorHeaderLBA(header)
			if !ok || uint32(lba) >= declared {
				continue
			}
			valid++
			// 重复扇区保留靠后的那一份：它是主数据流里的真实副本。
			mapping[uint32(lba)] = uint32(processed) + uint32(s)
		}
		processed += int64(sectors)
		if sectors == 0 {
			break
		}
	}
	if len(mapping) == 0 {
		return
	}
	if float64(valid) < float64(i.fileSector)*0.98 {
		// 头标不可信（音轨、伪头标）：宁可线性读，也不要用一张残缺表。
		return
	}
	i.lbaMap = mapping
	for lba := range mapping {
		if lba > i.maxLBA {
			i.maxLBA = lba
		}
	}
	if physical, ok := mapping[i.maxLBA]; ok {
		i.tailShift = int64(physical) - int64(i.maxLBA)
	}
}

// sectorHeaderLBA 解析 CD RAW 扇区头的 BCD MSF。
// 格式：00 FF*10 00 + MM SS FF + mode（BCD）。
func sectorHeaderLBA(head []byte) (int, bool) {
	if len(head) < 16 || head[0] != 0 || head[11] != 0 {
		return 0, false
	}
	for index := 1; index < 11; index++ {
		if head[index] != 0xff {
			return 0, false
		}
	}
	mode := head[15]
	if mode > 2 {
		return 0, false
	}
	msf := [3]int{}
	for index, raw := range head[12:15] {
		high, low := int(raw>>4), int(raw&0x0f)
		if high > 9 || low > 9 {
			return 0, false
		}
		msf[index] = high*10 + low
	}
	lba := (msf[0]*60+msf[1])*75 + msf[2] - 150
	if lba < 0 {
		return 0, false
	}
	return lba, true
}

func (i *sectorImage) physical(lba uint32) uint32 {
	if i.lbaMap != nil {
		if physical, ok := i.lbaMap[lba]; ok {
			return physical
		}
		if lba > i.maxLBA {
			return uint32(int64(lba) + i.tailShift)
		}
	}
	return lba
}

func (i *sectorImage) sector(lba uint32, dst []byte) (int, error) {
	physical := i.physical(lba)
	offset := int64(physical)*int64(i.layout.size) + int64(i.layout.userOffset)
	if offset < 0 || offset+int64(len(dst)) > i.size {
		return 0, io.EOF
	}
	return i.file.ReadAt(dst, offset)
}

// extent 读取一段连续的用户数据，自动逐扇区跨过 RAW 头。
func (i *sectorImage) extent(lba uint32, size int64) ([]byte, error) {
	if size <= 0 {
		return []byte{}, nil
	}
	// ISO 文件大小是 2048 的整数倍（目录可能更小），按扇区向上取整。
	sectors := (size + 2047) / 2048
	out := make([]byte, sectors*2048)
	buffer := make([]byte, 2048)
	for index := int64(0); index < sectors; index++ {
		n, err := i.sector(lba+uint32(index), buffer)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n <= 0 {
			break
		}
		copy(out[index*2048:], buffer[:n])
	}
	return out[:size], nil
}
