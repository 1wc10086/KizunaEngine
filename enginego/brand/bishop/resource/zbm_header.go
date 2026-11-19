package resource

import (
	"encoding/binary"
	"errors"
)

const (
	ZbmHeaderSize       = 32
	ZbmSupportedVersion = 2
	ZbmLabelOffset      = 14
)

var ZbmMagic = []byte("amp_")

type ZbmHeader struct {
	Version  int
	Reserved uint32
	DataBase uint32
	Label    string
}

func ZbmParseHeader(data []byte) (*ZbmHeader, error) {
	if len(data) < ZbmHeaderSize {
		return nil, errors.New("ZBM header is truncated")
	}
	if data[0] != 'a' || data[1] != 'm' || data[2] != 'p' || data[3] != '_' {
		return nil, errors.New("not a ZBM archive")
	}

	version := binary.LittleEndian.Uint16(data[4:])
	if version != ZbmSupportedVersion {
		return nil, errors.New("unsupported ZBM version")
	}

	reserved := binary.LittleEndian.Uint32(data[6:])
	dataBase := binary.LittleEndian.Uint32(data[10:])
	if dataBase < ZbmHeaderSize {
		return nil, errors.New("ZBM data offset is out of range")
	}

	return &ZbmHeader{
		Version:  int(version),
		Reserved: reserved,
		DataBase: dataBase,
		Label:    readZbmLabel(data[ZbmLabelOffset:ZbmHeaderSize]),
	}, nil
}

func readZbmLabel(source []byte) string {
	end := len(source)
	for i, value := range source {
		if value == 0 {
			end = i
			break
		}
	}

	if end == 0 {
		return ""
	}

	raw := source[:end]
	out := make([]byte, end)
	for i, value := range raw {
		if value > 0x7F {
			out[i] = '?'
		} else {
			out[i] = value
		}
	}
	return string(out)
}
