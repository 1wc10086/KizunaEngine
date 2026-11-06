package audio

const (
	plzHistoryPrepad   = 16384
	plzMaxRunLength    = 512
	plzOverreadBits    = 64
	plzMaxTotalSamples = 134217728
)

type plzBitReader struct {
	data     []byte
	cap      int64
	position int
}

func newPlzBitReader(data []byte) plzBitReader {
	return plzBitReader{data: data, cap: int64(len(data))*8 + plzOverreadBits}
}

func (r *plzBitReader) hasBits() bool {
	return int64(r.position) < r.cap
}

func (r *plzBitReader) read(count int) int {
	value := 0
	for i := 0; i < count; i++ {
		value <<= 1
		p := r.position
		r.position++
		index := ((p >> 5) << 2) + 3 - ((p & 31) >> 3)
		if uint(index) < uint(len(r.data)) {
			value |= int((r.data[index] >> (7 - uint(p&7))) & 1)
		}
	}
	return value
}

func DecodePlzChannel(segments [][]byte, historyBits int, totalSamples int) []int16 {
	if len(segments) == 0 || historyBits < 1 || historyBits > 32 {
		return []int16{}
	}
	if totalSamples <= 0 || totalSamples > plzMaxTotalSamples {
		return []int16{}
	}
	history := make([]int16, plzHistoryPrepad+totalSamples+plzMaxRunLength)
	write := plzHistoryPrepad
	produced := 0
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		reader := newPlzBitReader(segment)
		for produced < totalSamples && reader.hasBits() {
			if reader.read(1) == 1 {
				run := plzRunLengths[reader.read(3)]
				distance := reader.read(historyBits) + 1
				coefficient := plzCoefficients[reader.read(8)]
				offset := plzOffsets[reader.read(9)]
				source := write - distance
				for i := 0; i < run; i++ {
					previous := 0
					if source >= 0 {
						previous = int(history[source])
					}
					source++
					value := (int32(previous)*int32(coefficient) + int32(offset)) >> 16
					history[write] = int16(value)
					write++
				}
				produced += run
			} else {
				for i := 0; i < 4; i++ {
					history[write] = plzDirectSamples[reader.read(8)]
					write++
				}
				produced += 4
			}
		}
		if produced >= totalSamples {
			break
		}
	}
	take := produced
	if take > totalSamples {
		take = totalSamples
	}
	result := make([]int16, totalSamples)
	copy(result, history[plzHistoryPrepad:plzHistoryPrepad+take])
	return result
}

func DecodePlzTrack(packet []byte, chunkLengths []int, historyBits int, channelCount int, samplesPerChannel int) []int16 {
	if packet == nil || len(chunkLengths) == 0 {
		return []int16{}
	}
	if (channelCount != 1 && channelCount != 2) || samplesPerChannel <= 0 || samplesPerChannel > plzMaxTotalSamples {
		return []int16{}
	}
	if historyBits < 1 || historyBits > 32 {
		return []int16{}
	}
	segments := make([][]byte, 0, len(chunkLengths))
	position := 0
	for _, length := range chunkLengths {
		if length < 0 || length > len(packet)-position {
			return []int16{}
		}
		segments = append(segments, packet[position:position+length])
		position += length
	}
	if channelCount == 1 {
		return DecodePlzChannel(segments, historyBits, samplesPerChannel)
	}
	left := make([][]byte, 0, (len(segments)+1)/2)
	right := make([][]byte, 0, len(segments)/2)
	for i, segment := range segments {
		if i&1 == 0 {
			left = append(left, segment)
		} else {
			right = append(right, segment)
		}
	}
	var l []int16
	if len(left) > 0 {
		l = DecodePlzChannel(left, historyBits, samplesPerChannel)
	} else {
		l = []int16{}
	}
	var r []int16
	if len(right) > 0 {
		r = DecodePlzChannel(right, historyBits, samplesPerChannel)
	} else {
		r = []int16{}
	}
	result := make([]int16, 2*samplesPerChannel)
	for i := 0; i < samplesPerChannel; i++ {
		if i < len(l) {
			result[2*i] = l[i]
		}
		if i < len(r) {
			result[2*i+1] = r[i]
		}
	}
	return result
}
