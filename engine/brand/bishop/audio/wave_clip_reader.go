package audio

func ReadWave(data []byte) *PcmClip {
	if data == nil || len(data) < 12 {
		return nil
	}
	if !isFourCc(data, 0, 'R', 'I', 'F', 'F') || !isFourCc(data, 8, 'W', 'A', 'V', 'E') {
		return nil
	}
	channels := 0
	sampleRate := 0
	bitsPerSample := 0
	var pcm []byte
	pos := 12
	for pos+8 <= len(data) {
		isFmt := isFourCc(data, pos, 'f', 'm', 't', ' ')
		isData := isFourCc(data, pos, 'd', 'a', 't', 'a')
		size := int64(readUint32(data[pos+4 : pos+8]))
		body := int64(pos) + 8
		if isFmt {
			if size < 16 || body+16 > int64(len(data)) {
				return nil
			}
			formatTag := readUint16(data[pos+8 : pos+10])
			if formatTag != 1 {
				return nil
			}
			channels = int(readUint16(data[pos+10 : pos+12]))
			sampleRate = int(readUint32(data[pos+12 : pos+16]))
			bitsPerSample = int(readUint16(data[pos+22 : pos+24]))
			if channels < 1 || sampleRate <= 0 || (bitsPerSample != 8 && bitsPerSample != 16) {
				return nil
			}
		} else if isData {
			if body+size > int64(len(data)) {
				return nil
			}
			pcm = make([]byte, size)
			copy(pcm, data[body:body+size])
		}
		if channels > 0 && pcm != nil {
			break
		}
		next := body + size + (size & 1)
		if next <= int64(pos) || next > int64(len(data)) {
			break
		}
		pos = int(next)
	}
	if channels == 0 || pcm == nil {
		return nil
	}
	if bitsPerSample == 8 {
		widened := make([]byte, 2*len(pcm))
		for i := 0; i < len(pcm); i++ {
			widened[2*i] = 0
			widened[2*i+1] = pcm[i] - 128
		}
		pcm = widened
	}
	return &PcmClip{SampleRate: sampleRate, Channels: channels, Pcm: pcm}
}

func EnsureWave(data []byte) []byte {
	if data == nil || IsWaveContainer(data) {
		return data
	}
	if IsAdp(data) {
		if clip := DecodeAdp(data); clip != nil {
			return clip.ToWaveContainer()
		}
	}
	return data
}

func IsWaveContainer(data []byte) bool {
	return len(data) >= 12 &&
		data[0] == 0x52 &&
		data[1] == 0x49 &&
		data[2] == 0x46 &&
		data[3] == 0x46 &&
		data[8] == 0x57 &&
		data[9] == 0x41 &&
		data[10] == 0x56 &&
		data[11] == 0x45
}

func IsOggContainer(data []byte) bool {
	return len(data) >= 4 &&
		data[0] == 0x4F &&
		data[1] == 0x67 &&
		data[2] == 0x67 &&
		data[3] == 0x53
}
