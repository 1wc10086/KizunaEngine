package capi

import (
	"encoding/binary"
	"testing"

	"kizuna/engine/wire"
)

func buildSmokeRequest(op uint16, seq uint32, payload []byte) []byte {
	req := make([]byte, 16+len(payload))
	req[0] = 'K'
	req[1] = 'Z'
	req[2] = 'E'
	req[3] = 'Q'
	binary.LittleEndian.PutUint16(req[4:6], op)
	binary.LittleEndian.PutUint32(req[8:12], seq)
	binary.LittleEndian.PutUint32(req[12:16], uint32(len(payload)))
	copy(req[16:], payload)
	return req
}

func invokeSmoke(t *testing.T, h *handle, op uint16, seq uint32, payload []byte) (uint16, *wire.Reader) {
	t.Helper()
	gotOp, gotSeq, r, err := wire.ParseRequest(buildSmokeRequest(op, seq, payload))
	if err != nil {
		t.Fatal(err)
	}
	if gotOp != op || gotSeq != seq {
		t.Fatalf("op=%d seq=%d", gotOp, gotSeq)
	}
	resp := dispatch(h, gotOp, gotSeq, r)
	if len(resp) < 16 {
		t.Fatal("short response")
	}
	if resp[0] != 'K' || resp[1] != 'Z' || resp[2] != 'E' || resp[3] != 'S' {
		t.Fatal("bad response magic")
	}
	status := binary.LittleEndian.Uint16(resp[4:6])
	n := binary.LittleEndian.Uint32(resp[12:16])
	if uint64(n) > uint64(len(resp)-16) {
		t.Fatal("truncated response")
	}
	return status, wire.NewReader(resp[16 : 16+int(n)])
}

func TestSmokeBrands(t *testing.T) {
	status, r := invokeSmoke(t, &handle{}, wire.OpBrands, 1, nil)
	if status != wire.StatusOK {
		t.Fatalf("status=%d", status)
	}
	want := map[string]bool{
		"bishop":        false,
		"bishop-legacy": false,
		"bishop-gothic": false,
		"bishop-tj2":    false,
		"bishop-bdt":    false,
		"emon":          false,
	}
	count := 0
	r.Each(18, func(item *wire.Reader) {
		count++
		id := item.Str(19)
		if _, ok := want[id]; ok {
			want[id] = true
		}
		if item.Str(20) == "" {
			t.Errorf("brand %q has empty name", id)
		}
	})
	if count != len(want) {
		t.Fatalf("brands=%d want=%d", count, len(want))
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("brand %q missing", id)
		}
	}
}

func TestSmokeDetectEmpty(t *testing.T) {
	w := wire.NewWriter()
	w.Str(1, t.TempDir())
	status, r := invokeSmoke(t, &handle{}, wire.OpDetect, 2, w.Bytes())
	if status != wire.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if r.Str(19) != "" || r.Str(20) != "" {
		t.Fatalf("id=%q name=%q", r.Str(19), r.Str(20))
	}
}

func TestSmokeScanEmpty(t *testing.T) {
	w := wire.NewWriter()
	w.Str(1, t.TempDir())
	status, r := invokeSmoke(t, &handle{}, wire.OpScan, 3, w.Bytes())
	if status != wire.StatusOK {
		t.Fatalf("status=%d", status)
	}
	count := 0
	r.Each(71, func(_ *wire.Reader) {
		count++
	})
	if count != 0 {
		t.Fatalf("entries=%d", count)
	}
}

func TestSmokeOpenUnknownBrand(t *testing.T) {
	w := wire.NewWriter()
	w.Str(1, t.TempDir())
	w.Str(44, "no-such-brand")
	status, r := invokeSmoke(t, &handle{}, wire.OpOpen, 4, w.Bytes())
	if status != wire.StatusError {
		t.Fatalf("status=%d", status)
	}
	if r.Str(42) == "" {
		t.Fatal("empty error message")
	}
}

func TestSmokeCapabilityOpsWithoutOpen(t *testing.T) {
	namePayload := func() []byte {
		w := wire.NewWriter()
		w.Str(1, "smoke")
		return w.Bytes()
	}
	audioDonePayload := func() []byte {
		w := wire.NewWriter()
		w.Str(1, "smoke.wav")
		w.Bool(2, true)
		w.U32(3, 1)
		return w.Bytes()
	}
	framePayload := func() []byte {
		w := wire.NewWriter()
		w.Str(1, "frame")
		return w.Bytes()
	}
	imagesPayload := func() []byte {
		w := wire.NewWriter()
		w.StrList(53, []string{"a"})
		return w.Bytes()
	}
	previewPayload := func() []byte {
		w := wire.NewWriter()
		w.U32(2, 2)
		w.U32(3, 2)
		return w.Bytes()
	}
	slotPayload := func() []byte {
		w := wire.NewWriter()
		w.U32(11, 0)
		return w.Bytes()
	}
	optionPayload := func() []byte {
		w := wire.NewWriter()
		w.U32(57, 0)
		w.Bool(7, true)
		return w.Bytes()
	}
	keyPayload := func() []byte {
		w := wire.NewWriter()
		w.U32(58, 65)
		return w.Bytes()
	}
	wheelPayload := func() []byte {
		w := wire.NewWriter()
		w.I32(59, 1)
		return w.Bytes()
	}
	cases := []struct {
		op      uint16
		payload []byte
	}{
		{wire.OpSubmitName, namePayload()},
		{wire.OpAudioDone, audioDonePayload()},
		{wire.OpMapButtons, nil},
		{wire.OpMapMask, nil},
		{wire.OpSelectItems, nil},
		{wire.OpSelectFrame, framePayload()},
		{wire.OpComposeImages, imagesPayload()},
		{wire.OpSetPreview, previewPayload()},
		{wire.OpSlotPreview, slotPayload()},
		{wire.OpCgCatalog, nil},
		{wire.OpSetOption, optionPayload()},
		{wire.OpKey, keyPayload()},
		{wire.OpWheel, wheelPayload()},
	}
	for i, c := range cases {
		status, r := invokeSmoke(t, &handle{}, c.op, uint32(100+i), c.payload)
		if status != wire.StatusOK {
			t.Fatalf("op=%d status=%d", c.op, status)
		}
		switch c.op {
		case wire.OpSelectFrame, wire.OpComposeImages, wire.OpSlotPreview:
			if r.U32(2) != 0 || r.U32(3) != 0 {
				t.Fatalf("op=%d size=%d,%d", c.op, r.U32(2), r.U32(3))
			}
		}
	}
}
