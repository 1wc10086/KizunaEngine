package save

import (
	"encoding/json"

	"kizuna/engine/brand/emon/script"
)

func MarshalSavePayload(payload *EmonSavePayload) ([]byte, error) {
	return json.Marshal(payload)
}

func UnmarshalSavePayload(body []byte) (*EmonSavePayload, error) {
	var payload *EmonSavePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func MarshalSystemStatePayload(payload *EmonSystemStatePayload) ([]byte, error) {
	return json.Marshal(payload)
}

func UnmarshalSystemStatePayload(body []byte) (*EmonSystemStatePayload, error) {
	var payload *EmonSystemStatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func EncodeCP932(text string) []byte {
	return script.EncodeCP932(text)
}

func DecodeCP932(data []byte) string {
	return script.Decode(data)
}

func DecodeCP932Range(data []byte, offset int, length int) string {
	return script.Decode(data[offset : offset+length])
}
