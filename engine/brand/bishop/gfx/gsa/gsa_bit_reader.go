package gsa

type GsaBitReader struct {
	data     []byte
	position int
	bits     int
	cached   int
}

func NewGsaBitReader(data []byte, offset int) *GsaBitReader {
	return &GsaBitReader{data: data, position: offset}
}

func (r *GsaBitReader) GetBits(count int) int {
	var value int
	if r.cached >= count {
		mask := (1 << count) - 1
		value = r.bits & mask
		r.bits = int(uint(r.bits) >> count)
		r.cached -= count
	} else {
		value = r.bits & ((1 << r.cached) - 1)
		count -= r.cached
		shift := r.cached
		r.cached = 0
		for count >= 8 {
			if r.position < 0 || r.position >= len(r.data) {
				return -1
			}
			whole := int(r.data[r.position])
			r.position++
			value |= whole << shift
			shift += 8
			count -= 8
		}
		if count > 0 {
			if r.position < 0 || r.position >= len(r.data) {
				return -1
			}
			last := int(r.data[r.position])
			r.position++
			value |= (last & ((1 << count) - 1)) << shift
			r.bits = last >> count
			r.cached = 8 - count
		}
	}
	return value
}
