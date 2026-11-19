package audio

const (
	adpHeaderSize = 0xC4
	adpSamplesOff = 0xBC
	adpDataStart  = 0xC0
	adpQuantMax   = 0x58
	adpMaxSample  = 0x7FFF
	adpMinSample  = -32768
	adpMaxTotal   = 0x7FFFFFFF / 2
)

var adpQuantizeTable = []uint16{
	0x0007, 0x0008, 0x0009, 0x000A, 0x000B, 0x000C, 0x000D, 0x000E,
	0x0010, 0x0011, 0x0013, 0x0015, 0x0017, 0x0019, 0x001C, 0x001F,
	0x0022, 0x0025, 0x0029, 0x002D, 0x0032, 0x0037, 0x003C, 0x0042,
	0x0049, 0x0050, 0x0058, 0x0061, 0x006B, 0x0076, 0x0082, 0x008F,
	0x009D, 0x00AD, 0x00BE, 0x00D1, 0x00E6, 0x00FD, 0x0117, 0x0133,
	0x0151, 0x0173, 0x0198, 0x01C1, 0x01EE, 0x0220, 0x0256, 0x0292,
	0x02D4, 0x031C, 0x036C, 0x03C3, 0x0424, 0x048E, 0x0502, 0x0583,
	0x0610, 0x06AB, 0x0756, 0x0812, 0x08E0, 0x09C3, 0x0ABD, 0x0BD0,
	0x0CFF, 0x0E4C, 0x0FBA, 0x114C, 0x1307, 0x14EE, 0x1706, 0x1954,
	0x1BDC, 0x1EA5, 0x21B6, 0x2515, 0x28CA, 0x2CDF, 0x315B, 0x364B,
	0x3BB9, 0x41B2, 0x4844, 0x4F7E, 0x5771, 0x602F, 0x69CE, 0x7462, 0x7FFF,
}

var adpIncrementTable = []int{
	-1, -1, -1, -1, 2, 4, 6, 8, -1, -1, -1, -1, 2, 4, 6, 8,
}

type adpNibbleDecoder struct {
	prevSample int
	quantIndex int
}

func (d *adpNibbleDecoder) decodeSample(sample int) int16 {
	sample &= 0xF
	quant := int(adpQuantizeTable[d.quantIndex])
	d.quantIndex += adpIncrementTable[sample]
	if d.quantIndex < 0 {
		d.quantIndex = 0
	} else if d.quantIndex > adpQuantMax {
		d.quantIndex = adpQuantMax
	}
	step := (2*(sample&7) + 1) * quant >> 3
	if sample < 8 {
		sample = d.prevSample + step
		if sample > adpMaxSample {
			sample = adpMaxSample
		}
	} else {
		sample = d.prevSample - step
		if sample < adpMinSample {
			sample = adpMinSample
		}
	}
	d.prevSample = sample
	return int16(sample)
}

func IsAdp(data []byte) bool {
	if len(data) < adpHeaderSize {
		return false
	}
	if data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' {
		return false
	}
	sampleRate := readUint32(data[:4])
	if sampleRate < 8000 || sampleRate > 96000 {
		return false
	}
	channels := readUint16(data[4:6])
	if channels != 1 && channels != 2 {
		return false
	}
	samples := int64(readInt32(data[adpSamplesOff:adpSamplesOff+4])) * int64(channels)
	if samples <= 0 {
		return false
	}
	dataStart := readInt32(data[adpDataStart : adpDataStart+4])
	if dataStart < 0 || int64(dataStart) >= int64(len(data)) {
		return false
	}
	return true
}

func DecodeAdp(data []byte) *PcmClip {
	if data == nil || !IsAdp(data) {
		return nil
	}
	sampleRate := int(readUint32(data[0:4]))
	channels := int(readUint16(data[4:6]))
	totalSamples := int64(readInt32(data[adpSamplesOff:adpSamplesOff+4])) * int64(channels)
	dataStart := int(readInt32(data[adpDataStart : adpDataStart+4]))
	if totalSamples <= 0 || totalSamples > adpMaxTotal {
		return nil
	}
	needBytes := (totalSamples + 1) / 2
	if int64(dataStart)+needBytes > int64(len(data)) {
		return nil
	}
	samples := int(totalSamples)
	output := make([]byte, 2*samples)
	first := &adpNibbleDecoder{}
	var second *adpNibbleDecoder
	if channels > 1 {
		second = &adpNibbleDecoder{}
	} else {
		second = first
	}
	src := dataStart
	dst := 0
	for samples > 0 {
		v := data[src]
		src++
		left := uint16(first.decodeSample(int(v >> 4)))
		output[dst] = byte(left & 0xFF)
		output[dst+1] = byte((left >> 8) & 0xFF)
		samples--
		if samples == 0 {
			break
		}
		dst += 2
		right := uint16(second.decodeSample(int(v & 0xF)))
		output[dst] = byte(right & 0xFF)
		output[dst+1] = byte((right >> 8) & 0xFF)
		dst += 2
		samples--
	}
	return &PcmClip{SampleRate: sampleRate, Channels: channels, Pcm: output}
}
