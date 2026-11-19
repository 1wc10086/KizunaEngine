package resource

import (
	"bytes"
	"strings"
)

func archiveReadName(source []byte) string {
	end := bytes.IndexByte(source, 0)
	if end < 0 {
		end = len(source)
	}

	if end == 0 {
		return ""
	}

	return strings.TrimRight(decodeUTF8(source[:end]), "\x00 \t")
}
