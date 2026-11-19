package audio

import (
	"encoding/binary"
	"testing"
)

func TestProbeKinds(t *testing.T) {
	ogg := []byte("OggS\x00\x02")
	if got := Probe(ogg); got != KindOgg {
		t.Fatalf("OggS 判成 %v", got)
	}
	// 44 字节头：'D' + 3 保留 + 标志 0 + i32 长度(=总长-9)
	pcm := make([]byte, 9+16)
	pcm[0] = 'D'
	binary.LittleEndian.PutUint32(pcm[5:9], 16)
	if got := Probe(pcm); got != KindTMRHiroPCM {
		t.Fatalf("TMR-Hiro PCM 判成 %v", got)
	}
	if got := Probe([]byte{1, 2, 3}); got != KindRawPCM {
		t.Fatalf("其它判成 %v", got)
	}
}

func TestWrapPCMHeader(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	out := WrapPCM(pcm, WaveFormat{Channels: 2, SampleRate: 44100, BitsPerSam: 16})
	if string(out[0:4]) != "RIFF" || string(out[8:12]) != "WAVE" {
		t.Fatalf("RIFF/WAVE 头不对: %q", out[:12])
	}
	if got := int(binary.LittleEndian.Uint32(out[4:8])); got != 36+len(pcm) {
		t.Fatalf("RIFF 长度 = %d", got)
	}
	if got := int(binary.LittleEndian.Uint32(out[24:28])); got != 44100 {
		t.Fatalf("采样率 = %d", got)
	}
	if got := int(binary.LittleEndian.Uint16(out[32:34])); got != 4 {
		t.Fatalf("blockAlign = %d", got)
	}
	if got := int(binary.LittleEndian.Uint32(out[40:44])); got != len(pcm) {
		t.Fatalf("data 长度 = %d", got)
	}
}

func TestDecodeOggPassthrough(t *testing.T) {
	raw := []byte("OggS\x00\x02rest")
	ext, body, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ext != ".ogg" || len(body) != len(raw) {
		t.Fatalf("ext=%s len=%d", ext, len(body))
	}
}

func TestSafeNameStripsSeparators(t *testing.T) {
	if got := SafeName("a/b:c"); got != "a_b_c" {
		t.Fatalf("SafeName = %q", got)
	}
}
