package gothic

const ScsSignatureFirst byte = 0x50

const ScsSignatureSecond byte = 0x45

const scsKeyStep uint32 = 69069

func ScsHasSignature(data []byte) bool {
	return len(data) >= 2 && data[0] == ScsSignatureFirst && data[1] == ScsSignatureSecond
}

func ScsDecrypt(cipher []byte, seedOffset int, plain []byte) {
	seed := uint32(seedOffset | 1)
	length := len(cipher)
	if len(plain) < length {
		length = len(plain)
	}
	for i := 0; i < length; i++ {
		seed = seed * scsKeyStep
		plain[i] = cipher[i] ^ byte(seed&0xFF)
	}
}
