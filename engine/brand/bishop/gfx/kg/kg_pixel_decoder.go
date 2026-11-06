package kg

import (
	"errors"

	"kizuna/engine/core/binutil"
)

type KgPixelDecoder struct {
	Output    []byte
	PixelSize int
	Stride    int
	dict      [0x800]byte
}

func NewKgPixelDecoder(output []byte, pixelSize, stride int) *KgPixelDecoder {
	decoder := &KgPixelDecoder{Output: output, PixelSize: pixelSize, Stride: stride}
	decoder.ResetDict()
	return decoder
}

func (d *KgPixelDecoder) ResetDict() {
	for i := range d.dict {
		d.dict[i] = byte(i & 7)
	}
}

func (d *KgPixelDecoder) UnpackChannel(bits *binutil.BitReader, channel int) error {
	first, ok := bits.ReadBits(8)
	if !ok {
		return errors.New("KG channel data is truncated")
	}
	d.Output[channel] = byte(first)
	dst := channel + d.PixelSize
	second, ok := bits.ReadBits(8)
	if !ok {
		return errors.New("KG channel data is truncated")
	}
	d.Output[dst] = byte(second)
	dst += d.PixelSize
	for dst < len(d.Output) {
		ctl, ok := bits.ReadBits(1)
		if !ok {
			return errors.New("KG channel data is truncated")
		}
		if ctl == 0 {
			value, err := d.getPixel(bits, dst)
			if err != nil {
				return err
			}
			d.Output[dst] = value
			d.updateDict(value, d.Output[dst-d.PixelSize])
			dst += d.PixelSize
			continue
		}
		flag, ok := bits.ReadBits(1)
		if !ok {
			return errors.New("KG channel data is truncated")
		}
		if flag != 0 {
			ctl, ok = bits.ReadBits(2)
			if !ok {
				return errors.New("KG channel data is truncated")
			}
		} else {
			ctl = 4
		}
		var offset int
		switch ctl {
		case 0:
			offset = d.Stride
		case 1:
			offset = d.Stride - d.PixelSize
		case 2:
			offset = d.Stride + d.PixelSize
		case 3:
			offset = 2 * d.PixelSize
		default:
			offset = d.PixelSize
		}
		count, err := getCount(bits)
		if err != nil {
			return err
		}
		if count <= 0 {
			return errors.New("KG match length is invalid")
		}
		src := dst - offset
		if src < 0 {
			return errors.New("KG match offset is out of range")
		}
		for i := 0; i < count; i++ {
			if src >= len(d.Output) || dst >= len(d.Output) {
				return errors.New("KG match runs past the image end")
			}
			d.Output[dst] = d.Output[src]
			dst += d.PixelSize
			src += d.PixelSize
		}
	}
	return nil
}

func (d *KgPixelDecoder) getPixel(bits *binutil.BitReader, dst int) (byte, error) {
	flag, ok := bits.ReadBits(1)
	if !ok {
		return 0, errors.New("KG channel data is truncated")
	}
	if flag == 1 {
		direct, ok := bits.ReadBits(8)
		if !ok {
			return 0, errors.New("KG channel data is truncated")
		}
		return byte(direct), nil
	}
	index, ok := bits.ReadBits(3)
	if !ok {
		return 0, errors.New("KG channel data is truncated")
	}
	return d.dict[8*int(d.Output[dst-d.PixelSize])+index], nil
}

func (d *KgPixelDecoder) updateDict(value, previous byte) {
	start := 8 * int(previous)
	slot := 0
	for slot < 8 && d.dict[start+slot] != value {
		slot++
	}
	if slot != 0 {
		if slot == 8 {
			slot = 7
		}
		copy(d.dict[start+1:start+1+slot], d.dict[start:start+slot])
		d.dict[start] = value
	}
}

func getCount(bits *binutil.BitReader) (int, error) {
	count, ok := bits.ReadBits(2)
	if !ok {
		return 0, errors.New("KG channel data is truncated")
	}
	if count != 0 {
		return count, nil
	}
	count, ok = bits.ReadBits(4)
	if !ok {
		return 0, errors.New("KG channel data is truncated")
	}
	if count != 0 {
		return count + 3, nil
	}
	count, ok = bits.ReadBits(8)
	if !ok {
		return 0, errors.New("KG channel data is truncated")
	}
	if count != 0 {
		return count, nil
	}
	count, ok = bits.ReadBits(16)
	if !ok {
		return 0, errors.New("KG channel data is truncated")
	}
	if count != 0 {
		return count, nil
	}
	high, highOK := bits.ReadBits(16)
	low, lowOK := bits.ReadBits(16)
	if !highOK || !lowOK {
		return 0, errors.New("KG channel data is truncated")
	}
	return (high << 16) | low, nil
}
