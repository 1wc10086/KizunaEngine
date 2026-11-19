package eme

import (
	"encoding/binary"
	"errors"
)

const passCount = 8

func Decrypt(span []byte, key []byte) error {
	if len(key) != KeySize {
		return errors.New("EmonEngine key must be exactly 40 bytes.")
	}

	decryptCore(span, key)
	return nil
}

func DecryptRange(buffer []byte, offset, length int, key []byte) error {
	if err := validateCipher(buffer, offset, length, key); err != nil {
		return err
	}

	decryptCore(buffer[offset:offset+length], key)
	return nil
}

func EncryptRange(buffer []byte, offset, length int, key []byte) error {
	if err := validateCipher(buffer, offset, length, key); err != nil {
		return err
	}

	encryptCore(buffer[offset:offset+length], key)
	return nil
}

func validateCipher(buffer []byte, offset, length int, key []byte) error {
	if len(key) != KeySize {
		return errors.New("EmonEngine key must be exactly 40 bytes.")
	}

	if offset < 0 {
		return errors.New("offset is out of range")
	}

	if length < 0 {
		return errors.New("length is out of range")
	}

	if len(buffer)-offset < length {
		return errors.New("Buffer offset and length are out of bounds.")
	}

	return nil
}

func decryptCore(span []byte, key []byte) {
	count := len(span) / 4
	for i := passCount - 1; i >= 0; i-- {
		sub := binary.LittleEndian.Uint32(key[8+i*4 : 8+i*4+4])
		switch key[i] {
		case 1:
			xorBlock(span, count, sub)
		case 2:
			decryptChain(span, count, sub)
		case 4:
			permute(span, count, sub)
		case 8:
			shuffle(span, sub)
		}
	}
}

func encryptCore(span []byte, key []byte) {
	count := len(span) / 4
	for i := 0; i < passCount; i++ {
		sub := binary.LittleEndian.Uint32(key[8+i*4 : 8+i*4+4])
		switch key[i] {
		case 1:
			xorBlock(span, count, sub)
		case 2:
			encryptChain(span, count, sub)
		case 4:
			unpermute(span, count, sub)
		case 8:
			unshuffle(span, sub)
		}
	}
}

func xorBlock(span []byte, count int, sub uint32) {
	for j := 0; j < count; j++ {
		at := j * 4
		value := binary.LittleEndian.Uint32(span[at : at+4])
		binary.LittleEndian.PutUint32(span[at:at+4], value^sub)
	}
}

func decryptChain(span []byte, count int, sub uint32) {
	key := sub
	for j := 0; j < count; j++ {
		at := j * 4
		value := binary.LittleEndian.Uint32(span[at : at+4])
		binary.LittleEndian.PutUint32(span[at:at+4], value^key)
		key = value
	}
}

func encryptChain(span []byte, count int, sub uint32) {
	key := sub
	for j := 0; j < count; j++ {
		at := j * 4
		value := binary.LittleEndian.Uint32(span[at:at+4]) ^ key
		binary.LittleEndian.PutUint32(span[at:at+4], value)
		key = value
	}
}

func permute(span []byte, count int, sub uint32) {
	for j := 0; j < count; j++ {
		at := j * 4
		binary.LittleEndian.PutUint32(span[at:at+4], shiftValue(binary.LittleEndian.Uint32(span[at:at+4]), sub))
	}
}

func unpermute(span []byte, count int, sub uint32) {
	for j := 0; j < count; j++ {
		at := j * 4
		binary.LittleEndian.PutUint32(span[at:at+4], unshiftValue(binary.LittleEndian.Uint32(span[at:at+4]), sub))
	}
}

func shiftValue(value, sub uint32) uint32 {
	var shift uint32
	var result uint32
	for bit := uint32(0); bit < 32; bit++ {
		shift += sub
		result |= ((value >> bit) & 1) << (shift & 31)
	}

	return result
}

func unshiftValue(value, sub uint32) uint32 {
	var shift uint32
	var result uint32
	for bit := uint32(0); bit < 32; bit++ {
		shift += sub
		if ((value >> (shift & 31)) & 1) != 0 {
			result |= 1 << bit
		}
	}

	return result
}

func shuffle(span []byte, sub uint32) {
	n := len(span)
	if n == 0 {
		return
	}

	table := make([]byte, n)
	var position uint64
	for i := 0; i < n; i++ {
		position = (position + uint64(sub)) % uint64(n)
		table[position] = span[i]
	}

	copy(span, table)
}

func unshuffle(span []byte, sub uint32) {
	n := len(span)
	if n == 0 {
		return
	}

	table := make([]byte, n)
	var position uint64
	for i := 0; i < n; i++ {
		position = (position + uint64(sub)) % uint64(n)
		table[i] = span[position]
	}

	copy(span, table)
}
