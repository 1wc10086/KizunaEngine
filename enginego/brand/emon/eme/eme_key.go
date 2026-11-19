package eme

import (
	"encoding/binary"
	"errors"
)

const KeySize = 40

type EmeKey struct {
	bytes   []byte
	subKeys []uint32
}

func EmeKeyFromBytes(key []byte) (*EmeKey, error) {
	if key == nil {
		return nil, errors.New("key is null")
	}

	if len(key) != KeySize {
		return nil, errors.New("EmonEngine key must be exactly 40 bytes.")
	}

	bytes := make([]byte, len(key))
	copy(bytes, key)
	subKeys := make([]uint32, passCount)
	for i := 0; i < passCount; i++ {
		subKeys[i] = binary.LittleEndian.Uint32(bytes[8+i*4 : 8+i*4+4])
	}

	return &EmeKey{bytes: bytes, subKeys: subKeys}, nil
}

func (k *EmeKey) Bytes() []byte {
	out := make([]byte, len(k.bytes))
	copy(out, k.bytes)
	return out
}

func (k *EmeKey) Methods() []byte {
	return k.bytes[:passCount]
}

func (k *EmeKey) SubKeys() []uint32 {
	out := make([]uint32, len(k.subKeys))
	copy(out, k.subKeys)
	return out
}

func (k *EmeKey) GetMethod(index int) (byte, error) {
	if uint(index) >= passCount {
		return 0, errors.New("index is out of range")
	}

	return k.bytes[index], nil
}

func (k *EmeKey) GetSubKey(index int) (uint32, error) {
	if uint(index) >= passCount {
		return 0, errors.New("index is out of range")
	}

	return k.subKeys[index], nil
}
