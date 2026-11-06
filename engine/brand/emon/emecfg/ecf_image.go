package emecfg

import (
	"bytes"
	"encoding/binary"
	"errors"

	"kizuna/engine/brand/emon/eme"
)

const (
	ecfHeaderSize       = 16
	ecfKeySize          = 40
	ecfUnchainedDwords  = 11
	ecfRecordHeaderSize = 68
	ecfRecordNameSize   = 0x40
	ecfUnchainSeed      = 0xDC38EB76
)

var ecfMagicPrefix = []byte("EMESYS_VER1.20")

type EcfRecord struct {
	Name  string
	Value string
}

type EcfImage struct {
	Key          []byte
	SettingsText string
	Macros       []EcfRecord
	Symbols      []EcfRecord
}

func DecodeEcfImage(fileBytes []byte) (*EcfImage, error) {
	if len(fileBytes) < ecfHeaderSize+ecfUnchainedDwords*4 {
		return nil, errors.New("emecfg payload is truncated.")
	}

	if !bytes.Equal(fileBytes[:len(ecfMagicPrefix)], ecfMagicPrefix) || fileBytes[len(ecfMagicPrefix)] != 0 {
		return nil, errors.New("emecfg magic mismatch.")
	}

	payload := make([]byte, len(fileBytes)-ecfHeaderSize)
	copy(payload, fileBytes[ecfHeaderSize:])

	prev := uint32(ecfUnchainSeed)
	for i := 0; i < ecfUnchainedDwords; i++ {
		raw := binary.LittleEndian.Uint32(payload[i*4:])
		binary.LittleEndian.PutUint32(payload[i*4:], raw^prev)
		prev = raw
	}

	key := make([]byte, ecfKeySize)
	copy(key, payload[:ecfKeySize])
	pathLength := binary.LittleEndian.Uint32(payload[ecfKeySize:])
	if int64(ecfHeaderSize)+int64(ecfUnchainedDwords)*4+int64(pathLength) > int64(len(fileBytes)) {
		return nil, errors.New("emecfg settings block is out of range.")
	}

	settingsText := decryptText(payload, ecfUnchainedDwords*4, int(pathLength), key)

	cursor := ecfUnchainedDwords*4 + int(pathLength)
	macros, err := readRecords(payload, &cursor, key)
	if err != nil {
		return nil, err
	}

	symbols, err := readRecords(payload, &cursor, key)
	if err != nil {
		return nil, err
	}

	return &EcfImage{Key: key, SettingsText: settingsText, Macros: macros, Symbols: symbols}, nil
}

func readRecords(payload []byte, cursor *int, key []byte) ([]EcfRecord, error) {
	if *cursor+4 > len(payload) {
		return nil, errors.New("emecfg record count is out of range.")
	}

	count := int(int32(binary.LittleEndian.Uint32(payload[*cursor:])))
	if count < 0 {
		return nil, errors.New("emecfg record count is negative.")
	}

	*cursor += 4
	if int64(count)*ecfRecordHeaderSize > int64(len(payload)-*cursor) {
		return nil, errors.New("emecfg record count is out of range.")
	}

	records := make([]EcfRecord, 0, count)
	for i := 0; i < count; i++ {
		record, err := readRecord(payload, cursor, key)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, nil
}

func readRecord(payload []byte, cursor *int, key []byte) (EcfRecord, error) {
	if *cursor+ecfRecordHeaderSize > len(payload) {
		return EcfRecord{}, errors.New("emecfg record header is out of range.")
	}

	header := make([]byte, ecfRecordHeaderSize)
	copy(header, payload[*cursor:])
	eme.Decrypt(header, key)

	nameLength := bytes.IndexByte(header[:ecfRecordNameSize], 0)
	if nameLength < 0 {
		nameLength = ecfRecordNameSize
	}

	name := DecodeCP932(header[:nameLength])
	valueLength := binary.LittleEndian.Uint32(header[ecfRecordNameSize:])
	if int64(*cursor)+ecfRecordHeaderSize+int64(valueLength) > int64(len(payload)) {
		return EcfRecord{}, errors.New("emecfg record value is out of range.")
	}

	*cursor += ecfRecordHeaderSize
	value := decryptText(payload, *cursor, int(valueLength), key)
	*cursor += int(valueLength)
	return EcfRecord{Name: name, Value: value}, nil
}

func decryptText(payload []byte, offset, length int, key []byte) string {
	if length == 0 {
		return ""
	}

	block := make([]byte, length)
	copy(block, payload[offset:offset+length])
	eme.Decrypt(block, key)
	return DecodeCP932(block)
}
