package eme

import "errors"

const (
	maxOffsetBits  = 12
	maxFrameSize   = 1 << maxOffsetBits
	minMatchLength = 3
	maxMatchLength = 18
	tokenGroupSize = 8
)

func LzssDecompress(input []byte, outputSize, frameSize, frameInitPos int) ([]byte, error) {
	if outputSize <= 0 || len(input) == 0 {
		return nil, nil
	}

	if frameSize < 0 {
		return nil, errors.New("frameSize is out of range")
	}

	if frameSize == 0 {
		n := outputSize
		if n > len(input) {
			n = len(input)
		}

		out := make([]byte, n)
		copy(out, input[:n])
		return out, nil
	}

	output := make([]byte, outputSize)
	frame := make([]byte, frameSize)
	framePos := normalizePosition(frameInitPos, frameSize)
	produced := 0
	cursor := 0

	for cursor < len(input) && produced < outputSize {
		control := input[cursor]
		cursor++
		for bit := 1; bit != 0x100; bit <<= 1 {
			if produced >= outputSize {
				break
			}

			if control&byte(bit) != 0 {
				if cursor >= len(input) {
					return output[:produced], nil
				}

				literal := input[cursor]
				cursor++
				frame[framePos] = literal
				framePos = (framePos + 1) % frameSize
				output[produced] = literal
				produced++
			} else {
				if cursor+1 >= len(input) {
					return output[:produced], nil
				}

				lo := input[cursor]
				hi := input[cursor+1]
				cursor += 2
				offset := (int(lo) | ((int(hi) & 0xF0) << 4)) % frameSize
				count := minMatchLength + int(hi&0x0F)
				for k := 0; k < count; k++ {
					if produced >= outputSize {
						break
					}

					value := frame[offset]
					offset = (offset + 1) % frameSize
					frame[framePos] = value
					framePos = (framePos + 1) % frameSize
					output[produced] = value
					produced++
				}
			}
		}
	}

	if produced == outputSize {
		return output, nil
	}

	return output[:produced], nil
}

func LzssCompress(input []byte, frameSize, frameInitPos int) ([]byte, error) {
	if len(input) == 0 {
		return nil, nil
	}

	if frameSize < 0 || frameSize > maxFrameSize {
		return nil, errors.New("frameSize is out of range")
	}

	if frameSize == 0 {
		out := make([]byte, len(input))
		copy(out, input)
		return out, nil
	}

	frame := make([]byte, frameSize)
	framePos := normalizePosition(frameInitPos, frameSize)
	output := make([]byte, 0, len(input)+len(input)/tokenGroupSize+16)
	cursor := 0

	for cursor < len(input) {
		controlPos := len(output)
		output = append(output, 0)
		var control byte
		for bit := 0; bit < tokenGroupSize && cursor < len(input); bit++ {
			offset, length := findMatch(frame, frameSize, input, cursor)
			if length >= minMatchLength {
				output = append(output, byte(offset&0xFF), byte(((offset>>4)&0xF0)|(length-minMatchLength)))
				for k := 0; k < length; k++ {
					value := frame[offset]
					offset = (offset + 1) % frameSize
					frame[framePos] = value
					framePos = (framePos + 1) % frameSize
				}

				cursor += length
			} else {
				control |= 1 << bit
				literal := input[cursor]
				cursor++
				output = append(output, literal)
				frame[framePos] = literal
				framePos = (framePos + 1) % frameSize
			}
		}

		output[controlPos] = control
	}

	return output, nil
}

func findMatch(frame []byte, frameSize int, input []byte, cursor int) (int, int) {
	if cursor+minMatchLength > len(input) {
		return 0, 0
	}

	first := input[cursor]
	maxLength := maxMatchLength
	if remain := len(input) - cursor; remain < maxLength {
		maxLength = remain
	}

	bestLength := 0
	bestOffset := 0
	for candidate := 0; candidate < frameSize; candidate++ {
		if frame[candidate] != first {
			continue
		}

		matchLength := 0
		for matchLength < maxLength && frame[(candidate+matchLength)%frameSize] == input[cursor+matchLength] {
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

func normalizePosition(position, frameSize int) int {
	return ((position % frameSize) + frameSize) % frameSize
}
