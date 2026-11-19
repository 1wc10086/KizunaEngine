package audio

import (
	"errors"
	"os"
	"sync"
)

const (
	spPlaHeaderSize      = 20
	spPlaTrackRecordSize = 16
	spPlaMaxTrackCount   = 1 << 20
	spPlaMaxSamples      = 1 << 28
)

type SpPlaTrack struct {
	Offset            int64
	Length            int
	SampleRate        int
	Channels          int
	SamplesPerChannel int
}

type SpPlaContainer struct {
	path        string
	offsets     []int64
	lengths     []int
	sampleRates []int
	channels    []int
	samples     []int
	historyBits []int
	chunkLens   [][]int
	mu          sync.Mutex
	cachedID    int
	cachedPcm   []int16
}

func OpenSpPla(path string) (*SpPlaContainer, error) {
	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	info, err := stream.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := info.Size()
	header := make([]byte, spPlaHeaderSize)
	if !readFull(stream, header) {
		return nil, errors.New("sppla: short header")
	}
	if header[0] != 'P' || header[1] != 'l' || header[2] != 'a' || header[3] != '.' {
		return nil, errors.New("sppla: bad magic")
	}
	declaredSize := readUint32(header[4:8])
	if int64(declaredSize) != fileSize {
		return nil, errors.New("sppla: size mismatch")
	}
	count := int(readUint16(header[14:16]))
	if count < 1 || count > spPlaMaxTrackCount {
		return nil, errors.New("sppla: bad track count")
	}
	table2Offset := int64(spPlaHeaderSize + 4*count)
	table3Offset := table2Offset + int64(spPlaTrackRecordSize)*int64(count)
	table4Offset := table3Offset + 4*int64(count)
	if table4Offset > fileSize {
		return nil, errors.New("sppla: table overflow")
	}
	table2 := make([]byte, spPlaTrackRecordSize*count)
	table3Raw := make([]byte, 4*count)
	if _, err := stream.Seek(table2Offset, 0); err != nil {
		return nil, err
	}
	if !readFull(stream, table2) || !readFull(stream, table3Raw) {
		return nil, errors.New("sppla: short tables")
	}
	offsets := make([]int64, count)
	for i := 0; i < count; i++ {
		value := readUint32(table3Raw[i*4 : i*4+4])
		offsets[i] = int64(value)
		if int64(value) >= fileSize || (i > 0 && int64(value) <= offsets[i-1]) {
			return nil, errors.New("sppla: bad packet offset")
		}
	}
	if offsets[0] < table4Offset || (offsets[0]-table4Offset)%4 != 0 {
		return nil, errors.New("sppla: bad table4 range")
	}
	table4Words := int((offsets[0] - table4Offset) / 4)
	table4Raw := make([]byte, 4*table4Words)
	if _, err := stream.Seek(table4Offset, 0); err != nil {
		return nil, err
	}
	if !readFull(stream, table4Raw) {
		return nil, errors.New("sppla: short chunk table")
	}
	table4 := make([]uint32, table4Words)
	for i := 0; i < table4Words; i++ {
		table4[i] = readUint32(table4Raw[i*4 : i*4+4])
	}
	lengths := make([]int, count)
	sampleRates := make([]int, count)
	channels := make([]int, count)
	samples := make([]int, count)
	historyBits := make([]int, count)
	chunkLengths := make([][]int, count)
	position := 0
	for i := 0; i < count; i++ {
		var packetLength int64
		if i+1 < count {
			packetLength = offsets[i+1] - offsets[i]
		} else {
			packetLength = fileSize - offsets[i]
		}
		if packetLength < 1 || packetLength > 0x7FFFFFFF {
			return nil, errors.New("sppla: bad packet length")
		}
		lengths[i] = int(packetLength)
		totalSamples := int64(readUint32(table2[i*spPlaTrackRecordSize : i*spPlaTrackRecordSize+4]))
		sampleRate := int64(readUint32(table2[i*spPlaTrackRecordSize+4 : i*spPlaTrackRecordSize+8]))
		partCount := int64(readUint32(table2[i*spPlaTrackRecordSize+8 : i*spPlaTrackRecordSize+12]))
		packed := readUint32(table2[i*spPlaTrackRecordSize+12 : i*spPlaTrackRecordSize+16])
		channelCount := int(packed & 0xFF)
		history := int64(packed >> 8)
		if totalSamples < 1 || totalSamples > spPlaMaxSamples || sampleRate < 1 {
			return nil, errors.New("sppla: bad track header")
		}
		if (channelCount != 1 && channelCount != 2) || history < 1 || history > 32 {
			return nil, errors.New("sppla: bad track format")
		}
		if partCount < 1 || partCount > spPlaMaxTrackCount {
			return nil, errors.New("sppla: bad part count")
		}
		samples[i] = int(totalSamples)
		sampleRates[i] = int(sampleRate)
		channels[i] = channelCount
		historyBits[i] = int(history)
		parts := int(partCount)
		remaining := table4Words - position
		if i+1 < count {
			if 2*int64(parts) > int64(remaining) {
				return nil, errors.New("sppla: chunk table overflow")
			}
			chunkLengths[i] = readSpPlaChunkLengths(table4, position, parts, lengths[i])
			position += 2 * parts
		} else if remaining == 2*parts {
			chunkLengths[i] = readSpPlaChunkLengths(table4, position, parts, lengths[i])
			position += remaining
		} else if remaining == 2*parts-1 && parts >= 1 {
			chunkLengths[i] = readSpPlaLastChunkLengths(table4, position, parts, lengths[i])
			position += remaining
		} else {
			return nil, errors.New("sppla: bad last chunk table")
		}
	}
	if position != table4Words {
		return nil, errors.New("sppla: chunk table size mismatch")
	}
	return &SpPlaContainer{
		path:        path,
		offsets:     offsets,
		lengths:     lengths,
		sampleRates: sampleRates,
		channels:    channels,
		samples:     samples,
		historyBits: historyBits,
		chunkLens:   chunkLengths,
		cachedID:    -1,
		cachedPcm:   []int16{},
	}, nil
}

func IsSpPla(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'l' && data[2] == 'a' && data[3] == '.'
}

func (c *SpPlaContainer) TrackCount() int {
	return len(c.offsets)
}

func (c *SpPlaContainer) TryGetTrack(id int) (SpPlaTrack, bool) {
	if id >= 1 && id <= len(c.offsets) {
		index := id - 1
		return SpPlaTrack{
			Offset:            c.offsets[index],
			Length:            c.lengths[index],
			SampleRate:        c.sampleRates[index],
			Channels:          c.channels[index],
			SamplesPerChannel: c.samples[index],
		}, true
	}
	return SpPlaTrack{}, false
}

func (c *SpPlaContainer) ReadTrack(id int) []int16 {
	c.mu.Lock()
	if id == c.cachedID && len(c.cachedPcm) != 0 {
		out := make([]int16, len(c.cachedPcm))
		copy(out, c.cachedPcm)
		c.mu.Unlock()
		return out
	}
	c.mu.Unlock()
	samples := c.loadTrack(id)
	if len(samples) == 0 {
		return []int16{}
	}
	c.mu.Lock()
	c.cachedID = id
	c.cachedPcm = make([]int16, len(samples))
	copy(c.cachedPcm, samples)
	c.mu.Unlock()
	return samples
}

func (c *SpPlaContainer) loadTrack(id int) (samples []int16) {
	defer func() {
		if recover() != nil {
			samples = []int16{}
		}
	}()
	if id < 1 || id > len(c.offsets) {
		return []int16{}
	}
	index := id - 1
	packet := make([]byte, c.lengths[index])
	stream, err := os.Open(c.path)
	if err != nil {
		return []int16{}
	}
	defer stream.Close()
	if _, err := stream.Seek(c.offsets[index], 0); err != nil {
		return []int16{}
	}
	if !readFull(stream, packet) {
		return []int16{}
	}
	return DecodePlzTrack(packet, c.chunkLens[index], c.historyBits[index], c.channels[index], c.samples[index])
}

func readSpPlaChunkLengths(table []uint32, position int, parts int, packetLength int) []int {
	lengths := make([]int, parts)
	var sum int64
	for j := 0; j < parts; j++ {
		value := int64(table[position+1+2*j])
		if value > int64(packetLength) {
			panic("sppla: chunk too large")
		}
		lengths[j] = int(value)
		sum += value
	}
	if sum != int64(packetLength) {
		panic("sppla: chunk sum mismatch")
	}
	return lengths
}

func readSpPlaLastChunkLengths(table []uint32, position int, parts int, packetLength int) []int {
	lengths := make([]int, parts)
	var sum int64
	for j := 0; j+1 < parts; j++ {
		value := int64(table[position+1+2*j])
		if value > int64(packetLength) {
			panic("sppla: chunk too large")
		}
		lengths[j] = int(value)
		sum += value
	}
	last := int64(packetLength) - sum
	if last < 0 || last > int64(packetLength) {
		panic("sppla: bad last chunk")
	}
	lengths[parts-1] = int(last)
	return lengths
}
