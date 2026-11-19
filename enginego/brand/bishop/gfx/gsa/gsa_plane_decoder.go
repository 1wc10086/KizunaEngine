package gsa

import "errors"

type GsaPlaneDecoder struct {
	output        []byte
	stride        int
	width         int
	height        int
	bytesPerPixel int
}

func NewGsaPlaneDecoder(output []byte, stride, width, height, bytesPerPixel int) *GsaPlaneDecoder {
	return &GsaPlaneDecoder{
		output:        output,
		stride:        stride,
		width:         width,
		height:        height,
		bytesPerPixel: bytesPerPixel,
	}
}

func (d *GsaPlaneDecoder) ReadPlane(bits *GsaBitReader, plane int) error {
	row := plane
	for y := 0; y < d.height; y += 2 {
		dst := row
		row += 2 * d.stride
		for x := 0; x < d.width; x += 2 {
			switch bits.GetBits(3) {
			case 0:
				d.copyBits0(dst)
			case 1:
				d.readBits1(bits, dst, 1, 0)
			case 2:
				d.readBits1(bits, dst, 2, 0xFF)
			case 3:
				d.readBits1(bits, dst, 3, 0xFD)
			case 4:
				d.readBits4(bits, dst, 4, 0xF9)
			case 5:
				d.readBits5(bits, dst, 6)
			case 6:
				d.readBits5(bits, dst, 7)
			case 7:
				d.readBits5(bits, dst, 8)
			default:
				return errors.New("unexpected end of GSA bitstream")
			}
			dst += d.bytesPerPixel * 2
		}
	}
	return nil
}

func (d *GsaPlaneDecoder) copyBits0(dst int) {
	d.output[dst] = d.output[dst-d.bytesPerPixel*2]
	d.output[dst+d.bytesPerPixel] = d.output[dst-d.bytesPerPixel]
	d.output[dst+d.stride] = d.output[dst+d.stride-d.bytesPerPixel*2]
	d.output[dst+d.stride+d.bytesPerPixel] = d.output[dst+d.stride-d.bytesPerPixel]
}

func (d *GsaPlaneDecoder) readBits1(bits *GsaBitReader, dst, count int, n byte) {
	d.output[dst] = byte(int(n) + bits.GetBits(count) + int(d.output[dst-d.bytesPerPixel*2]))
	d.output[dst+d.bytesPerPixel] = byte(int(n) + bits.GetBits(count) + int(d.output[dst-d.bytesPerPixel]))
	d.output[dst+d.stride] = byte(int(n) + bits.GetBits(count) + int(d.output[dst+d.stride-d.bytesPerPixel*2]))
	d.output[dst+d.stride+d.bytesPerPixel] = byte(int(n) + bits.GetBits(count) + int(d.output[dst+d.stride-d.bytesPerPixel]))
}

func (d *GsaPlaneDecoder) readBits4(bits *GsaBitReader, dst, count int, n byte) {
	prev := dst - 2*d.stride
	d.output[dst] = byte(int(n) + bits.GetBits(count) + int(d.output[prev]))
	d.output[dst+d.bytesPerPixel] = byte(int(n) + bits.GetBits(count) + int(d.output[prev+d.bytesPerPixel]))
	d.output[dst+d.stride] = byte(int(n) + bits.GetBits(count) + int(d.output[dst-d.stride]))
	d.output[dst+d.stride+d.bytesPerPixel] = byte(int(n) + bits.GetBits(count) + int(d.output[dst-d.stride+d.bytesPerPixel]))
}

func (d *GsaPlaneDecoder) readBits5(bits *GsaBitReader, dst, count int) {
	d.output[dst] = byte(bits.GetBits(count))
	d.output[dst+d.bytesPerPixel] = byte(bits.GetBits(count))
	d.output[dst+d.stride] = byte(bits.GetBits(count))
	d.output[dst+d.stride+d.bytesPerPixel] = byte(bits.GetBits(count))
}
