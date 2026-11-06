package bsmap

import (
	"os"
	"strings"

	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const BsMapParametersFileName = "MapPrms.dat"

const (
	bsMapParametersHeaderSize         = 60
	bsMapParametersMinimumBlockOffset = 60
	bsMapParametersJumpOpcode         = 0x85
	bsMapParametersJumpSize           = 5
	bsMapParametersPairSize           = 8
	bsMapParametersPlaceCountWord     = 0
	bsMapParametersBlockAOffsetWord   = 7
	bsMapParametersBlockASizeWord     = 8
	bsMapParametersBlockBOffsetWord   = 9
	bsMapParametersBlockBSizeWord     = 10
	bsMapParametersBlockCOffsetWord   = 11
	bsMapParametersBlockCSizeWord     = 12
	bsMapParametersStringOffsetWord   = 13
	bsMapParametersStringSizeWord     = 14
)

type BsMapPair struct {
	Key   int
	Value int
}

type BsMapParameters struct {
	PlaceCount int
	BlockA     []byte
	BlockB     []byte
	BlockC     []BsMapPair
	Names      []string
	Jumps      []int
}

func (p *BsMapParameters) IsEmpty() bool {
	return len(p.Names) == 0
}

func (p *BsMapParameters) Name(index int) string {
	if uint(index) < uint(len(p.Names)) {
		return p.Names[index]
	}
	return ""
}

func (p *BsMapParameters) PlaceLabelIndex(row int) int {
	if uint(row) < uint(len(p.Jumps)) {
		return p.Jumps[row]
	}
	return -1
}

func (p *BsMapParameters) PlaceLabelName(row int) string {
	index := p.PlaceLabelIndex(row)
	if index < 0 {
		return ""
	}
	return p.Name(index)
}

func LoadBsMapParameters(gameRoot string) *BsMapParameters {
	if len(gameRoot) == 0 {
		return nil
	}
	path := gameRoot + "/" + BsMapParametersFileName
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Android 文件系统区分大小写，原版 MAPPRMS.DAT 与引擎的小写名对不上；精确路径失败后
		// 再扫描顶层目录，用大小写无关的比较找到真实文件名。
		if entries, dirErr := os.ReadDir(gameRoot); dirErr == nil {
			for _, entry := range entries {
				if strings.EqualFold(entry.Name(), BsMapParametersFileName) {
					raw, err = os.ReadFile(gameRoot + "/" + entry.Name())
					break
				}
			}
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			core.Logf("script: '%s' is missing, the map falls back to the label scan", BsMapParametersFileName)
		} else {
			core.Logf("script: '%s' unreadable: %s", BsMapParametersFileName, err.Error())
		}
		return nil
	}
	if parameters := ParseBsMapParameters(raw); parameters != nil {
		core.Logf("script: '%s' places=%d names=%d jumps=%d", BsMapParametersFileName, parameters.PlaceCount, len(parameters.Names), len(parameters.Jumps))
		return parameters
	}
	core.Logf("script: '%s' could not be parsed, the map falls back to the label scan", BsMapParametersFileName)
	return nil
}

func ParseBsMapParameters(data []byte) *BsMapParameters {
	if len(data) < bsMapParametersHeaderSize {
		return nil
	}
	blockAOffset := bsMapParametersWord(data, bsMapParametersBlockAOffsetWord)
	blockASize := bsMapParametersWord(data, bsMapParametersBlockASizeWord)
	blockBOffset := bsMapParametersWord(data, bsMapParametersBlockBOffsetWord)
	blockBSize := bsMapParametersWord(data, bsMapParametersBlockBSizeWord)
	blockCOffset := bsMapParametersWord(data, bsMapParametersBlockCOffsetWord)
	blockCSize := bsMapParametersWord(data, bsMapParametersBlockCSizeWord)
	stringOffset := bsMapParametersWord(data, bsMapParametersStringOffsetWord)
	stringSize := bsMapParametersWord(data, bsMapParametersStringSizeWord)
	if !bsMapParametersIsRange(len(data), blockAOffset, blockASize) || blockASize <= 0 {
		return nil
	}
	if !bsMapParametersIsRange(len(data), blockBOffset, blockBSize) {
		return nil
	}
	if !bsMapParametersIsRange(len(data), blockCOffset, blockCSize) {
		return nil
	}
	if !bsMapParametersIsRange(len(data), stringOffset, stringSize) || stringSize <= 0 {
		return nil
	}
	if blockAOffset < bsMapParametersMinimumBlockOffset || blockBOffset < bsMapParametersMinimumBlockOffset || stringOffset < bsMapParametersMinimumBlockOffset {
		return nil
	}
	names := readBsMapParameterNames(data[stringOffset : stringOffset+stringSize])
	if len(names) == 0 {
		return nil
	}
	blockB := append([]byte(nil), data[blockBOffset:blockBOffset+blockBSize]...)
	return &BsMapParameters{
		PlaceCount: bsMapParametersWord(data, bsMapParametersPlaceCountWord),
		BlockA:     append([]byte(nil), data[blockAOffset:blockAOffset+blockASize]...),
		BlockB:     blockB,
		BlockC:     readBsMapParameterPairs(data[blockCOffset : blockCOffset+blockCSize]),
		Names:      names,
		Jumps:      scanBsMapParameterJumps(blockB),
	}
}

func bsMapParametersWord(data []byte, index int) int {
	return int(binutil.U32(data[index*4:]))
}

func bsMapParametersIsRange(length, offset, size int) bool {
	return offset >= 0 && size >= 0 && int64(offset)+int64(size) <= int64(length)
}

func readBsMapParameterNames(block []byte) []string {
	encoding := vm.DetectTextEncoding(block)
	names := []string{}
	position := 0
	for position < len(block) {
		end := vm.FindTerminator(block, position, encoding)
		if end > position {
			names = append(names, vm.ReadString(block, position, encoding))
		} else {
			names = append(names, "")
		}
		position = end + 1
	}
	return names
}

func readBsMapParameterPairs(block []byte) []BsMapPair {
	pairs := make([]BsMapPair, 0, len(block)/bsMapParametersPairSize)
	for offset := 0; offset+bsMapParametersPairSize <= len(block); offset += bsMapParametersPairSize {
		pairs = append(pairs, BsMapPair{
			Key:   int(binutil.U32(block[offset:])),
			Value: int(binutil.U32(block[offset+4:])),
		})
	}
	return pairs
}

func scanBsMapParameterJumps(block []byte) []int {
	jumps := []int{}
	index := 0
	for index+bsMapParametersJumpSize <= len(block) {
		if block[index] == bsMapParametersJumpOpcode {
			jumps = append(jumps, int(binutil.U32(block[index+1:])))
			index += bsMapParametersJumpSize
			continue
		}
		index++
	}
	return jumps
}
