package audio

var vorbisIdentifier = []byte{0x01, 'v', 'o', 'r', 'b', 'i', 's'}

var opusHead = []byte{'O', 'p', 'u', 's', 'H', 'e', 'a', 'd'}

func IsVorbis(data []byte) bool {
	if len(data) < 28 ||
		data[0] != 0x4F ||
		data[1] != 0x67 ||
		data[2] != 0x67 ||
		data[3] != 0x53 {
		return false
	}
	segments := int(data[26])
	head := 27 + segments
	if head > len(data) {
		return false
	}
	size := 0
	for i := 0; i < segments; i++ {
		size += int(data[27+i])
	}
	room := len(data) - head
	if size > room {
		size = room
	}
	body := data[head : head+size]
	if indexBytes(body, opusHead) >= 0 {
		return false
	}
	return indexBytes(body, vorbisIdentifier) >= 0
}

func indexBytes(data []byte, pattern []byte) int {
	if len(pattern) == 0 || len(data) < len(pattern) {
		return -1
	}
	for i := 0; i <= len(data)-len(pattern); i++ {
		j := 0
		for j < len(pattern) && data[i+j] == pattern[j] {
			j++
		}
		if j == len(pattern) {
			return i
		}
	}
	return -1
}
