package audio

type PcmClip struct {
	SampleRate int
	Channels   int
	Pcm        []byte
}

func (c PcmClip) ToWaveContainer() []byte {
	container := make([]byte, 44+len(c.Pcm))
	head := container[:44]
	head[0] = 'R'
	head[1] = 'I'
	head[2] = 'F'
	head[3] = 'F'
	putUint32(head[4:8], uint32(36+len(c.Pcm)))
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
	putUint16(head[22:24], uint16(c.Channels))
	putUint32(head[24:28], uint32(c.SampleRate))
	putUint32(head[28:32], uint32(2*c.Channels*c.SampleRate))
	putUint16(head[32:34], uint16(2*c.Channels))
	putUint16(head[34:36], 16)
	head[36] = 'd'
	head[37] = 'a'
	head[38] = 't'
	head[39] = 'a'
	putUint32(head[40:44], uint32(len(c.Pcm)))
	copy(container[44:], c.Pcm)
	return container
}
