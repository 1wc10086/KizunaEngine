package wire

import (
	"errors"

	"kizuna/engine/core/binutil"
)

var (
	errShort     = errors.New("wire: request too short")
	errMagic     = errors.New("wire: bad request magic")
	errTruncated = errors.New("wire: truncated payload")
)

func ParseRequest(b []byte) (op uint16, seq uint32, r *Reader, err error) {
	if len(b) < 16 {
		return 0, 0, nil, errShort
	}
	if b[0] != 'K' || b[1] != 'Z' || b[2] != 'E' || b[3] != 'Q' {
		return 0, 0, nil, errMagic
	}
	op = binutil.U16(b[4:6])
	seq = binutil.U32(b[8:12])
	n := binutil.U32(b[12:16])
	if uint64(n) > uint64(len(b)-16) {
		return 0, 0, nil, errTruncated
	}
	return op, seq, NewReader(b[16 : 16+int(n)]), nil
}

func BuildResponse(status uint16, seq uint32, payload []byte) []byte {
	out := make([]byte, 16+len(payload))
	out[0] = 'K'
	out[1] = 'Z'
	out[2] = 'E'
	out[3] = 'S'
	binutil.PutU16(out[4:6], status)
	binutil.PutU16(out[6:8], 0)
	binutil.PutU32(out[8:12], seq)
	binutil.PutU32(out[12:16], uint32(len(payload)))
	copy(out[16:], payload)
	return out
}

func Error(seq uint32, msg string) []byte {
	w := NewWriter()
	w.Str(42, msg)
	return BuildResponse(StatusError, seq, w.Bytes())
}
