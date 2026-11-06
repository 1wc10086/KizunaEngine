package audio

import (
	"errors"
	"io"
	"os"
)

func DecodeVorbisToWave(source io.ReadSeeker, target io.WriteSeeker) (int64, error) {
	if source == nil || target == nil {
		return 0, errors.New("vorbis: nil stream")
	}
	if _, err := source.Seek(0, 0); err != nil {
		return 0, errors.New("vorbis: source not seekable")
	}
	return 0, errors.New("vorbis: decode not supported; passthrough only")
}

func DecodeVorbisToWaveFile(source io.ReadSeeker, path string) (int64, error) {
	if path == "" {
		return 0, errors.New("vorbis: wave path is required")
	}
	if source == nil {
		return 0, errors.New("vorbis: nil stream")
	}
	stream, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer stream.Close()
	return DecodeVorbisToWave(source, stream)
}

func ClipVorbisToPcm16(samples []float32, target []byte) int {
	for index := range samples {
		value := samples[index]
		if value > 1 {
			value = 1
		} else if value < -1 {
			value = -1
		}
		sample := int(value * 32767)
		if sample > 32767 {
			sample = 32767
		} else if sample < -32768 {
			sample = -32768
		}
		target[2*index] = byte(sample & 0xFF)
		target[2*index+1] = byte((sample >> 8) & 0xFF)
	}
	return len(samples) * 2
}

func WriteWaveHeader(target io.Writer, sampleRate int, channels int) {
	head := make([]byte, 44)
	head[0] = 'R'
	head[1] = 'I'
	head[2] = 'F'
	head[3] = 'F'
	putUint32(head[4:8], 0)
	head[8] = 'W'
	head[9] = 'A'
	head[10] = 'V'
	head[11] = 'E'
	head[12] = 'f'
	head[13] = 'm'
	head[14] = 't'
	head[15] = ' '
	putUint32(head[16:20], 16)
	putUint16(head[20:22], 1)
	putUint16(head[22:24], uint16(channels))
	putUint32(head[24:28], uint32(sampleRate))
	putUint32(head[28:32], uint32(2*channels*sampleRate))
	putUint16(head[32:34], uint16(2*channels))
	putUint16(head[34:36], 16)
	head[36] = 'd'
	head[37] = 'a'
	head[38] = 't'
	head[39] = 'a'
	putUint32(head[40:44], 0)
	target.Write(head)
}
