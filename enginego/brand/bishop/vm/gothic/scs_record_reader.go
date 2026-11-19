package gothic

import "errors"

var ErrScsTruncatedLength = errors.New("scs: truncated length field")
var ErrScsTruncatedRecord = errors.New("scs: truncated record")

type ScsRecordReader struct {
	data      []byte
	encrypted bool
	position  int
}

func NewScsRecordReader(data []byte) *ScsRecordReader {
	encrypted := ScsHasSignature(data)
	position := 0
	if encrypted {
		position = 2
	}
	return &ScsRecordReader{data: data, encrypted: encrypted, position: position}
}

func (r *ScsRecordReader) Encrypted() bool {
	return r.encrypted
}

func (r *ScsRecordReader) Rewind() {
	if r.encrypted {
		r.position = 2
		return
	}
	r.position = 0
}

func (r *ScsRecordReader) TryReadLine() (line string, offset int64, ok bool, err error) {
	if r.encrypted {
		return r.tryReadRecord()
	}
	return r.tryReadPlain()
}

func (r *ScsRecordReader) tryReadRecord() (string, int64, bool, error) {
	if r.position >= len(r.data) {
		return "", 0, false, nil
	}
	if r.position+2 > len(r.data) {
		return "", 0, false, ErrScsTruncatedLength
	}
	offset := int64(r.position)
	length := int(r.data[r.position]) | int(r.data[r.position+1])<<8
	r.position += 2
	if r.position+length > len(r.data) {
		return "", 0, false, ErrScsTruncatedRecord
	}
	if length == 0 {
		return "", offset, true, nil
	}
	plain := make([]byte, length)
	ScsDecrypt(r.data[r.position:r.position+length], int(offset), plain)
	r.position += length
	return DecodeGothicText(plain, DetectGothicTextEncoding(plain)), offset, true, nil
}

func (r *ScsRecordReader) tryReadPlain() (string, int64, bool, error) {
	if r.position >= len(r.data) {
		return "", 0, false, nil
	}
	offset := int64(r.position)
	end := r.position
	for end < len(r.data) && r.data[end] != 0x0A {
		end++
	}
	length := end - r.position
	if length > 0 && r.data[end-1] == 0x0D {
		length--
	}
	var line string
	if length != 0 {
		plain := r.data[r.position : r.position+length]
		line = DecodeGothicText(plain, DetectGothicTextEncoding(plain))
	}
	if end < len(r.data) {
		r.position = end + 1
	} else {
		r.position = end
	}
	return line, offset, true, nil
}
