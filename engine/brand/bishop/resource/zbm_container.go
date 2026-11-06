package resource

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"

	"kizuna/engine/core/textcodec"
)

const (
	zbmMetaSize          = 16
	zbmStampSize         = 7
	zbmMaxEntryCount     = 1 << 20
	zbmBitmapExtension   = ".bmp"
	zbmPackedSizeField   = 0
	zbmAttributesField   = 4
	zbmStampField        = 5
	zbmUnpackedSizeField = 12
)

type ZbmContainer struct {
	Header  ZbmHeader
	Entries []ZbmEntry
	lookup  map[string]ZbmEntry
}

func NewZbmContainer(header ZbmHeader, entries []ZbmEntry) *ZbmContainer {
	container := &ZbmContainer{
		Header:  header,
		Entries: entries,
		lookup:  make(map[string]ZbmEntry, len(entries)*2),
	}
	for _, entry := range entries {
		container.addLookup(entry.Name, entry)
		container.addLookup(stripZbmExtension(entry.Name), entry)
	}

	return container
}

func (c *ZbmContainer) TryFindEntry(name string) (ZbmEntry, bool) {
	if strings.TrimSpace(name) == "" {
		return ZbmEntry{}, false
	}

	request := strings.TrimSpace(name)
	if found, ok := c.lookup[strings.ToLower(request)]; ok {
		return found, true
	}
	if found, ok := c.lookup[strings.ToLower(stripZbmExtension(request))]; ok {
		return found, true
	}
	found, ok := c.lookup[strings.ToLower(request+zbmBitmapExtension)]
	return found, ok
}

func ZbmParseContainer(source io.Reader, length int64) (*ZbmContainer, error) {
	headerBytes := make([]byte, ZbmHeaderSize)
	if readExactly(source, headerBytes) != len(headerBytes) {
		return nil, errors.New("truncated ZBM header")
	}

	header, err := ZbmParseHeader(headerBytes)
	if err != nil {
		return nil, err
	}

	var entries []ZbmEntry
	dataOffset := int64(header.DataBase)
	flag := make([]byte, 1)
	meta := make([]byte, zbmMetaSize)

	for {
		if readExactly(source, flag) != 1 {
			return nil, errors.New("truncated ZBM directory")
		}
		if flag[0] != 1 {
			break
		}
		if len(entries) >= zbmMaxEntryCount {
			return nil, errors.New("too many ZBM entries")
		}
		if readExactly(source, meta) != zbmMetaSize {
			return nil, errors.New("truncated ZBM entry")
		}

		packedSize := binary.LittleEndian.Uint32(meta[zbmPackedSizeField:])
		attributes := meta[zbmAttributesField]
		unpackedSize := binary.LittleEndian.Uint32(meta[zbmUnpackedSizeField:])
		if packedSize > math.MaxInt32 || unpackedSize > math.MaxInt32 || dataOffset+int64(packedSize) > length {
			return nil, errors.New("ZBM entry is outside archive bounds")
		}

		name, err := readZbmName(source)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, errors.New("empty ZBM entry name")
		}

		stamp := make([]byte, zbmStampSize)
		copy(stamp, meta[zbmStampField:zbmStampField+zbmStampSize])
		entries = append(entries, ZbmEntry{
			Name:         name,
			DataOffset:   dataOffset,
			PackedSize:   int(packedSize),
			UnpackedSize: int(unpackedSize),
			Attributes:   attributes,
			Timestamp:    stamp,
		})
		dataOffset += int64(packedSize)
	}

	if len(entries) == 0 {
		return nil, errors.New("empty ZBM archive")
	}

	return NewZbmContainer(*header, entries), nil
}

func (c *ZbmContainer) addLookup(name string, entry ZbmEntry) {
	key := strings.ToLower(name)
	if _, taken := c.lookup[key]; !taken {
		c.lookup[key] = entry
	}
}

func readZbmName(source io.Reader) (string, error) {
	var raw []byte
	for {
		value, ok := readByte(source)
		if !ok {
			return "", errors.New("truncated ZBM entry name")
		}
		if value == 0 {
			break
		}

		raw = append(raw, value)
		if textcodec.IsCP932Lead(value) {
			trail, ok := readByte(source)
			if !ok {
				return "", errors.New("truncated ZBM entry name")
			}

			raw = append(raw, trail)
		}
	}

	if len(raw) == 0 {
		return "", nil
	}

	return textcodec.DecodeCP932(raw), nil
}

func stripZbmExtension(name string) string {
	dot := strings.LastIndexByte(name, '.')
	if dot > 0 {
		return name[:dot]
	}

	return name
}
