package eme

import "encoding/binary"

const keySize = 40

const passCount = 8

func decrypt(buf []byte, key []byte) {
	if len(key) != keySize {
		return
	}
	count := len(buf) / 4
	for i := passCount - 1; i >= 0; i-- {
		sub := binary.LittleEndian.Uint32(key[8+i*4 : 8+i*4+4])
		switch key[i] {
		case 1:
			xorPass(buf, count, sub)
		case 2:
			chainDecrypt(buf, count, sub)
		case 4:
			permute(buf, count, sub)
		case 8:
			shuffleDecrypt(buf, sub)
		}
	}
}

func encrypt(buf []byte, key []byte) {
	if len(key) != keySize {
		return
	}
	count := len(buf) / 4
	for i := 0; i < passCount; i++ {
		sub := binary.LittleEndian.Uint32(key[8+i*4 : 8+i*4+4])
		switch key[i] {
		case 1:
			xorPass(buf, count, sub)
		case 2:
			chainEncrypt(buf, count, sub)
		case 4:
			unpermute(buf, count, sub)
		case 8:
			shuffleEncrypt(buf, sub)
		}
	}
}

func xorPass(buf []byte, count int, sub uint32) {
	for j := 0; j < count; j++ {
		at := j * 4
		v := binary.LittleEndian.Uint32(buf[at : at+4])
		binary.LittleEndian.PutUint32(buf[at:at+4], v^sub)
	}
}

func chainDecrypt(buf []byte, count int, sub uint32) {
	key := sub
	for j := 0; j < count; j++ {
		at := j * 4
		v := binary.LittleEndian.Uint32(buf[at : at+4])
		binary.LittleEndian.PutUint32(buf[at:at+4], v^key)
		key = v
	}
}

func chainEncrypt(buf []byte, count int, sub uint32) {
	key := sub
	for j := 0; j < count; j++ {
		at := j * 4
		cur := binary.LittleEndian.Uint32(buf[at : at+4])
		v := cur ^ key
		binary.LittleEndian.PutUint32(buf[at:at+4], v)
		key = v
	}
}

func permute(buf []byte, count int, sub uint32) {
	for j := 0; j < count; j++ {
		at := j * 4
		v := binary.LittleEndian.Uint32(buf[at : at+4])
		binary.LittleEndian.PutUint32(buf[at:at+4], shiftBits(v, sub))
	}
}

func unpermute(buf []byte, count int, sub uint32) {
	for j := 0; j < count; j++ {
		at := j * 4
		v := binary.LittleEndian.Uint32(buf[at : at+4])
		binary.LittleEndian.PutUint32(buf[at:at+4], unshiftBits(v, sub))
	}
}

func shiftBits(value, sub uint32) uint32 {
	var shift uint32
	var result uint32
	for bit := uint32(0); bit < 32; bit++ {
		shift += sub
		if (value>>bit)&1 != 0 {
			result |= 1 << (shift & 31)
		}
	}
	return result
}

func unshiftBits(value, sub uint32) uint32 {
	var shift uint32
	var result uint32
	for bit := uint32(0); bit < 32; bit++ {
		shift += sub
		if (value>>(shift&31))&1 != 0 {
			result |= 1 << bit
		}
	}
	return result
}

func shuffleDecrypt(buf []byte, sub uint32) {
	n := len(buf)
	if n == 0 {
		return
	}
	table := make([]byte, n)
	pos := uint64(0)
	for i := 0; i < n; i++ {
		pos = (pos + uint64(sub)) % uint64(n)
		table[pos] = buf[i]
	}
	copy(buf, table)
}

func shuffleEncrypt(buf []byte, sub uint32) {
	n := len(buf)
	if n == 0 {
		return
	}
	table := make([]byte, n)
	pos := uint64(0)
	for i := 0; i < n; i++ {
		pos = (pos + uint64(sub)) % uint64(n)
		table[i] = buf[pos]
	}
	copy(buf, table)
}
