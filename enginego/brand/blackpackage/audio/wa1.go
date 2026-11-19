// Package audio 解码 FFA SYSTEM 的 WA1。
//
// WA1 有两种外壳：
//   - 文件/条目开头是 “packed+8==len” 时先走 LZSS，解出来多半直接是 RIFF；
//   - 否则是 type + RIFF 头（type 在 0，RIFF 从 4 开始），0x28 起是
//     "data"，0x2c 是 PCM 字节数。
//
// type 0/4/8/12 是四代 ADPCM，解出来统一是 16-bit PCM。这里只做“全解码成
// WAV 字节”，播放交给宿主（与其它品牌一致：引擎给路径，Flutter 解码）。
package audio

import (
	"encoding/binary"
	"fmt"

	"kizuna/enginego/brand/blackpackage/archive"
)

// DecodeWA1 把 WA1 原始字节解成完整 WAV（RIFF 头 + PCM）。
// 返回值可直接写进缓存目录，路径交给宿主播放。
func DecodeWA1(raw []byte) ([]byte, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("blackpackage: WA1 截断")
	}
	// LZSS 外壳：第一个 int 是压缩长度，且 packed+8 == 文件长度。
	packed := int(int32(binary.LittleEndian.Uint32(raw[0:4])))
	if packed > 12 && packed+8 == len(raw) {
		unpacked := int(int32(binary.LittleEndian.Uint32(raw[4:8])))
		if unpacked <= 0 || unpacked > 64<<20 {
			return nil, fmt.Errorf("blackpackage: WA1 解压长度非法")
		}
		inner, err := archive.Decompress(raw[8:8+packed], unpacked)
		if err != nil {
			return nil, err
		}
		if len(inner) >= 4 && string(inner[:4]) == "RIFF" {
			return inner, nil
		}
		return decodePCM(inner)
	}
	return decodePCM(raw)
}

func decodePCM(raw []byte) ([]byte, error) {
	if len(raw) < 0x30 {
		return nil, fmt.Errorf("blackpackage: WA1 头截断")
	}
	typ := int(int32(binary.LittleEndian.Uint32(raw[0:4])))
	if string(raw[4:8]) != "RIFF" || string(raw[0x28:0x2c]) != "data" {
		return nil, fmt.Errorf("blackpackage: WA1 头签名不符")
	}
	dataSize := int(int32(binary.LittleEndian.Uint32(raw[0x2c:0x30])))
	if dataSize <= 0 || dataSize > 64<<20 {
		return nil, fmt.Errorf("blackpackage: WA1 data 长度非法 %d", dataSize)
	}
	// 输出 = 0x2c 字节 RIFF 头（从原文件 4..0x2c 搬来）+ PCM。
	out := make([]byte, 0x2c+dataSize)
	copy(out, raw[4:0x30])
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	switch typ {
	case 0:
		unpackV0(raw, out, dataSize)
	case 4:
		unpackV4(raw, out, dataSize)
	case 8:
		unpackV8(raw, out, dataSize)
	case 12:
		unpackV12(raw, out, dataSize)
	default:
		return nil, fmt.Errorf("blackpackage: 未支持 WA1 type %d", typ)
	}
	return out, nil
}

var sampleTable = [16]uint16{
	0x39, 0x39, 0x39, 0x39, 0x4D, 0x66, 0x80, 0x99,
	0x39, 0x39, 0x39, 0x39, 0x4D, 0x66, 0x80, 0x99,
}

func unpackV0(raw, out []byte, dataSize int) {
	src, dst := 0x30, 0x2c
	var v12 uint32 = 127
	v13, v14, a5 := 0, 0, 0
	for v11 := dataSize >> 1; v11 != 0; v11-- {
		if (a5 >> 8) == 1 {
			a5 = int(raw[src])
			src++
		} else {
			a5 = 0x100 | (int(raw[src]) >> 4)
		}
		v15 := a5 & 0xF
		v17 := (v12 * uint32(byte(2*(v15&7)+1))) >> 3
		v18 := v17 & 0xffff
		if v15&8 != 0 {
			d := (v14 << 16) | (v13 & 0xffff)
			d -= int(v18)
			v13 = d & 0xffff
			v14 = d >> 16
			if v14 < 0 && v13 < 0x8000 {
				v13 = -32768
				v14 = -1
			}
		} else {
			d := (v14 << 16) | (v13 & 0xffff)
			d += int(v18)
			v13 = d & 0xffff
			v14 = d >> 16
			if v14 >= 0 && v13 >= 0x8000 {
				v13 = 32767
				v14 = 0
			}
		}
		v21 := uint32(sampleTable[v15]) * v12
		v12 = (v21 >> 6) & 0xffff
		if v12 < 0x7f {
			v12 = 127
		} else if v12 > 0x6000 {
			v12 = 0x6000
		}
		binary.LittleEndian.PutUint16(out[dst:], uint16(v13))
		dst += 2
	}
}

func unpackV4(raw, out []byte, dataSize int) {
	src, dst := 0x30, 0x2c
	var v72 uint32 = 127
	v73, v74, a5 := 0, 0, 0
	for v71 := dataSize >> 1; v71 != 0; v71-- {
		v75 := a5
		v76 := a5 & 0xFFFF
		v77 := v75 >> 16
		var v80, v81 int
		if byte(v77) < 8 {
			b := int(raw[src])
			src++
			v80 = b << uint(v77)
			v77 = (v77 + 8) & 0xff
			v76 |= v80
		}
		if v76&3 == 2 {
			v80 = v76 >> 2
			v77 = (v77 - 2) & 0xff
			v81 = 0
		} else if v76&3 != 0 {
			switch {
			case v76&7 == 5:
				v80, v81 = v76>>3, 1
				v77 = (v77 - 3) & 0xff
			case v76&7 == 1:
				v80, v81 = v76>>3, 9
				v77 = (v77 - 3) & 0xff
			case v76&0xF == 11:
				v80, v81 = v76>>4, 2
				v77 = (v77 - 4) & 0xff
			case v76&0xF == 3:
				v80, v81 = v76>>4, 10
				v77 = (v77 - 4) & 0xff
			case v76&0x1F == 23:
				v80, v81 = v76>>5, 3
				v77 = (v77 - 5) & 0xff
			case v76&0x1F == 7:
				v80, v81 = v76>>5, 11
				v77 = (v77 - 5) & 0xff
			case v76&0x3F == 47:
				v80, v81 = v76>>6, 4
				v77 = (v77 - 6) & 0xff
			case v76&0x3F == 15:
				v80, v81 = v76>>6, 12
				v77 = (v77 - 6) & 0xff
			case v76&0x7F == 95:
				v80, v81 = v76>>7, 5
				v77 = (v77 - 7) & 0xff
			case v76&0x7F == 31:
				v80, v81 = v76>>7, 13
				v77 = (v77 - 7) & 0xff
			default:
				switch v76 & 0xff {
				case 0x7F:
					v81 = 6
				case 0xFF:
					v81 = 14
				case 0xBF:
					v81 = 7
				default:
					v81 = 15
				}
				v80 = v76 >> 8
				v77 = (v77 - 8) & 0xff
			}
		} else {
			v80 = v76 >> 2
			v77 = (v77 - 2) & 0xff
			v81 = 8
		}
		a5 = (v77 << 16) | v80
		v82 := ((v81 & 7) << 1) + 1
		v84 := (v72 * uint32(v82)) >> 3
		v85 := v84 & 0xffff
		if v81&8 != 0 {
			d := (v74 << 16) | (v73 & 0xffff)
			d -= int(v85)
			v73 = d & 0xffff
			v74 = d >> 16
			if v74 < 0 && v73 < 0x8000 {
				v73, v74 = -32768, -1
			}
		} else {
			d := (v74 << 16) | (v73 & 0xffff)
			d += int(v85)
			v73 = d & 0xffff
			v74 = d >> 16
			if v74 >= 0 && v73 >= 0x8000 {
				v73, v74 = 32767, 0
			}
		}
		v88 := uint32(sampleTable[v81]) * v72
		v72 = (v88 >> 6) & 0xffff
		if v72 < 0x7f {
			v72 = 0x7f
		} else if v72 > 0x6000 {
			v72 = 0x6000
		}
		binary.LittleEndian.PutUint16(out[dst:], uint16(v73))
		dst += 2
	}
}

func unpackV8(raw, out []byte, dataSize int) {
	src, dst := 0x30, 0x2c
	v95 := dataSize >> 2
	for i := 0; i < 2; i++ {
		a5 := i
		var v96 uint32 = 127
		v97, v98 := 0, 0
		v172 := dst
		for j := 0; j < v95; j++ {
			if a5>>8 == 1 {
				a5 = int(raw[src])
				src++
			} else {
				a5 = 0x100 | (int(raw[src]) >> 4)
			}
			v99 := a5 & 0xF
			v101 := (v96 * uint32(byte(2*(v99&7)+1))) >> 3
			v102 := v101 & 0xffff
			if v99&8 != 0 {
				d := (v98 << 16) | (v97 & 0xffff)
				d -= int(v102)
				v97, v98 = d&0xffff, d>>16
				if v98 < 0 && v97 < 0x8000 {
					v97, v98 = -32768, -1
				}
			} else {
				d := (v98 << 16) | (v97 & 0xffff)
				d += int(v102)
				v97, v98 = d&0xffff, d>>16
				if v98 >= 0 && v97 >= 0x8000 {
					v97, v98 = 32767, 0
				}
			}
			v105 := uint32(sampleTable[v99]) * v96
			v96 = (v105 >> 6) & 0xffff
			if v96 < 0x7f {
				v96 = 127
			} else if v96 > 0x6000 {
				v96 = 0x6000
			}
			binary.LittleEndian.PutUint16(out[dst:], uint16(v97))
			dst += 4
		}
		dst = v172 + 2
	}
}

func unpackV12(raw, out []byte, dataSize int) {
	src, dst := 0x30, 0x2c
	v124 := dataSize >> 2
	v125 := 0
	for i := 0; i < 2; i++ {
		var v126 uint32 = 127
		v127, v128 := 0, 0
		outputBegin := dst
		for j := 0; j < v124; j++ {
			v130 := v125 & 0xFFFF
			v131 := v125 >> 16
			var v134, v135 int
			if byte(v131) < 8 {
				b := int(raw[src])
				src++
				v133 := b << uint(v131)
				v131 = (v131 + 8) & 0xff
				v130 |= v133
			}
			switch {
			case v130&3 == 2:
				v134, v135 = v130>>2, 0
				v131 = (v131 - 2) & 0xff
			case v130&3 == 0:
				v134, v135 = v130>>2, 8
				v131 = (v131 - 2) & 0xff
			case v130&7 == 5:
				v134, v135 = v130>>3, 1
				v131 = (v131 - 3) & 0xff
			case v130&7 == 1:
				v134, v135 = v130>>3, 9
				v131 = (v131 - 3) & 0xff
			case v130&0xF == 11:
				v134, v135 = v130>>4, 2
				v131 = (v131 - 4) & 0xff
			case v130&0xF == 3:
				v134, v135 = v130>>4, 10
				v131 = (v131 - 4) & 0xff
			case v130&0x1F == 23:
				v134, v135 = v130>>5, 3
				v131 = (v131 - 5) & 0xff
			case v130&0x1F == 7:
				v134, v135 = v130>>5, 11
				v131 = (v131 - 5) & 0xff
			case v130&0x3F == 47:
				v134, v135 = v130>>6, 4
				v131 = (v131 - 6) & 0xff
			case v130&0x3F == 15:
				v134, v135 = v130>>6, 12
				v131 = (v131 - 6) & 0xff
			case v130&0x7F == 95:
				v134, v135 = v130>>7, 5
				v131 = (v131 - 7) & 0xff
			case v130&0x7F == 31:
				v134, v135 = v130>>7, 13
				v131 = (v131 - 7) & 0xff
			default:
				switch v130 & 0xff {
				case 0x7F:
					v135 = 6
				case 0xFF:
					v135 = 14
				case 0xBF:
					v135 = 7
				default:
					v135 = 15
				}
				v134 = v130 >> 8
				v131 = (v131 - 8) & 0xff
			}
			v125 = (v131 << 16) | v134
			v136 := ((v135 & 7) << 1) + 1
			v138 := int((v126 * uint32(v136)) >> 3)
			v139 := v138 & 0xFFFF
			if v135&8 != 0 {
				d := (v128 << 16) | (v127 & 0xffff)
				d -= v139
				v127, v128 = d&0xffff, d>>16
				if v128 < 0 && v127 < 0x8000 {
					v127, v128 = -32768, -1
				}
			} else {
				d := (v128 << 16) | (v127 & 0xffff)
				d += v139
				v127, v128 = d&0xffff, d>>16
				if v128 >= 0 && v127 >= 0x8000 {
					v127, v128 = 32767, 0
				}
			}
			v142 := uint32(sampleTable[v135]) * v126
			v126 = (v142 >> 6) & 0xffff
			if v126 < 0x7F {
				v126 = 0x7F
			} else if v126 > 0x6000 {
				v126 = 0x6000
			}
			binary.LittleEndian.PutUint16(out[dst:], uint16(v127))
			dst += 4
		}
		dst = outputBegin + 2
	}
}
