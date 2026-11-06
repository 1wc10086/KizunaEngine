package eme

const (
	maxOffsetBits  = 12
	maxFrameSize   = 1 << maxOffsetBits
	minMatchLength = 3
	maxMatchLength = 18
	tokenGroupSize = 8
)

func lzssDecompress(input []byte, outputSize, frameSize, frameInitPos int) []byte {
	if outputSize <= 0 || len(input) == 0 {
		return nil
	}
	if frameSize == 0 {
		n := outputSize
		if n > len(input) {
			n = len(input)
		}
		out := make([]byte, n)
		copy(out, input[:n])
		return out
	}
	output := make([]byte, 0, outputSize)
	frame := make([]byte, frameSize)
	pos := ((frameInitPos % frameSize) + frameSize) % frameSize
	produced := 0
	cursor := 0
	for cursor < len(input) && produced < outputSize {
		control := input[cursor]
		cursor++
		for bit := 1; bit != 0x100 && produced < outputSize; bit <<= 1 {
			if control&byte(bit) != 0 {
				if cursor >= len(input) {
					return output
				}
				literal := input[cursor]
				cursor++
				frame[pos] = literal
				pos++
				if pos == frameSize {
					pos = 0
				}
				output = append(output, literal)
				produced++
				continue
			}
			if cursor+1 >= len(input) {
				return output
			}
			lo := uint16(input[cursor])
			hi := uint16(input[cursor+1])
			cursor += 2
			offset := int((lo | ((hi & 0xF0) << 4))) % frameSize
			count := minMatchLength + int(hi&0x0F)
			for k := 0; k < count && produced < outputSize; k++ {
				value := frame[offset]
				offset++
				if offset == frameSize {
					offset = 0
				}
				frame[pos] = value
				pos++
				if pos == frameSize {
					pos = 0
				}
				output = append(output, value)
				produced++
			}
		}
	}
	if produced < outputSize {
		return output
	}
	return output[:produced]
}

func lzssCompress(input []byte, frameSize, frameInitPos int) []byte {
	if len(input) == 0 {
		return nil
	}
	if frameSize == 0 {
		out := make([]byte, len(input))
		copy(out, input)
		return out
	}
	frame := make([]byte, frameSize)
	pos := ((frameInitPos % frameSize) + frameSize) % frameSize
	out := make([]byte, 0, len(input)+len(input)/tokenGroupSize+16)
	cursor := 0
	for cursor < len(input) {
		controlPos := len(out)
		out = append(out, 0)
		var control byte
		for bit := 0; bit < tokenGroupSize && cursor < len(input); bit++ {
			offset, length := lzssFindMatch(frame, frameSize, input, cursor)
			if length >= minMatchLength {
				out = append(out,
					byte(offset&0xFF),
					byte(((offset>>4)&0xF0)|(length-minMatchLength)),
				)
				for k := 0; k < length; k++ {
					v := frame[offset]
					offset++
					if offset == frameSize {
						offset = 0
					}
					frame[pos] = v
					pos++
					if pos == frameSize {
						pos = 0
					}
				}
				cursor += length
				continue
			}
			control |= 1 << bit
			literal := input[cursor]
			out = append(out, literal)
			frame[pos] = literal
			pos++
			if pos == frameSize {
				pos = 0
			}
			cursor++
		}
		out[controlPos] = control
	}
	return out
}

func lzssFindMatch(frame []byte, frameSize int, input []byte, cursor int) (int, int) {
	if cursor+minMatchLength > len(input) {
		return 0, 0
	}
	first := input[cursor]
	maxLength := len(input) - cursor
	if maxLength > maxMatchLength {
		maxLength = maxMatchLength
	}
	bestLength := 0
	bestOffset := 0
	for candidate := 0; candidate < frameSize; candidate++ {
		if frame[candidate] != first {
			continue
		}
		matchLength := 0
		for matchLength < maxLength &&
			frame[(candidate+matchLength)%frameSize] == input[cursor+matchLength] {
			matchLength++
		}
		if matchLength > bestLength {
			bestLength = matchLength
			bestOffset = candidate
			if bestLength == maxMatchLength {
				break
			}
		}
	}
	return bestOffset, bestLength
}
