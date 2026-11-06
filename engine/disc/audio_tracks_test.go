package disc

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestCDAudioDetectsTrackAndWritesWave(t *testing.T) {
	const dataSectors = 5
	const audioSectors = 3
	const sectorSize = 2352
	raw := make([]byte, (dataSectors+audioSectors)*sectorSize)
	for lba := 0; lba < dataSectors; lba++ {
		copy(raw[lba*sectorSize:], cdSyncPattern)
	}
	for i := dataSectors * sectorSize; i < len(raw); i++ {
		raw[i] = 0x12
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "disc.bin")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image := &sectorImage{
		path:       path,
		file:       file,
		size:       int64(len(raw)),
		layout:     sectorLayout{size: sectorSize, userOffset: 16, name: "cd-raw-2352"},
		fileSector: dataSectors + audioSectors,
	}

	start, ok := findCDAudioStart(image)
	if !ok || start != dataSectors {
		t.Fatalf("findCDAudioStart = %d, %v; want %d, true", start, ok, dataSectors)
	}

	target := filepath.Join(dir, "audio", "audio_track_lba5.wav")
	frames, err := writeCDAudioWave(image, start, target)
	if err != nil {
		t.Fatal(err)
	}
	if frames != audioSectors {
		t.Errorf("frames = %d, want %d", frames, audioSectors)
	}
	wave, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(wave) != cdWaveHeaderSize+audioSectors*sectorSize {
		t.Fatalf("wave size = %d", len(wave))
	}
	if string(wave[0:4]) != "RIFF" || string(wave[8:12]) != "WAVE" {
		t.Error("bad RIFF/WAVE tags")
	}
	if binary.LittleEndian.Uint16(wave[22:]) != 2 ||
		binary.LittleEndian.Uint32(wave[24:]) != 44100 ||
		binary.LittleEndian.Uint16(wave[34:]) != 16 {
		t.Error("fmt chunk should be 2ch 44100Hz 16bit")
	}
	if wave[cdWaveHeaderSize] != 0x12 {
		t.Error("PCM payload not copied from audio sectors")
	}
}

func TestCDAudioNoTrackWhenAllData(t *testing.T) {
	const sectors = 4
	raw := make([]byte, sectors*2352)
	for lba := 0; lba < sectors; lba++ {
		copy(raw[lba*2352:], cdSyncPattern)
	}
	path := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image := &sectorImage{
		path:       path,
		file:       file,
		size:       int64(len(raw)),
		layout:     sectorLayout{size: 2352, userOffset: 16},
		fileSector: sectors,
	}
	if _, ok := findCDAudioStart(image); ok {
		t.Error("all-data image should have no audio track")
	}
}
