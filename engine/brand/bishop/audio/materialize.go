package audio

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/brand/bishop/resource"
)

type ResourceIndex interface {
	TryGet(name string) (*resource.ResourceLocation, bool)
}

const materializeOggSpill = ".spill"

func Materialize(name string, res ResourceIndex, cacheDir string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	if isAbsPath(name) {
		if _, err := os.Stat(name); err == nil {
			return prepareLooseFile(name, cacheDir)
		}
		if res == nil {
			return ""
		}
	}
	if res == nil {
		return ""
	}
	var location *resource.ResourceLocation
	var ok bool
	stem := fileStem(name)
	if stem != "" {
		location, ok = res.TryGet(stem)
	}
	if !ok {
		location, ok = res.TryGet(name)
	}
	if !ok {
		tail := name
		if index := strings.LastIndex(tail, "/"); index >= 0 {
			tail = tail[index+1:]
		}
		location, ok = res.TryGet(tail)
	}
	if !ok {
		return ""
	}
	if location.IsLoose() {
		loose := location.LooseFile()
		if loose == "" {
			return ""
		}
		if _, err := os.Stat(loose); err != nil {
			return ""
		}
		return prepareLooseFile(loose, cacheDir)
	}
	data := location.Read()
	if len(data) == 0 {
		return ""
	}
	return prepareBytes(data, stem, cacheDir)
}

func prepareLooseFile(path string, cacheDir string) string {
	head := readHead(path, 4096)
	if len(head) == 0 || IsWaveContainer(head) || !IsOggContainer(head) {
		return path
	}
	if cacheDir == "" || !IsVorbis(head) {
		return path
	}
	stem := fileStem(path)
	if stem == "" {
		return path
	}
	raw, err := os.ReadFile(path)
	if err != nil || !IsVorbis(raw) {
		return path
	}
	return writeOggFallback(raw, stem, cacheDir)
}

func prepareBytes(data []byte, stem string, cacheDir string) string {
	if cacheDir == "" {
		return ""
	}
	if stem == "" {
		stem = "audio"
	}
	if IsWaveContainer(data) {
		target := cacheTarget(cacheDir, stem, digestBytes(data), "wav")
		if info, err := os.Stat(target); err == nil && info.Size() == int64(len(data)) {
			return target
		}
		if err := writeFileAtomic(target, data); err != nil {
			return ""
		}
		return target
	}
	if IsVorbis(data) {
		return writeOggFallback(data, stem, cacheDir)
	}
	if IsOggContainer(data) {
		return writeOggFallback(data, stem, cacheDir)
	}
	wave := EnsureWave(data)
	if !IsWaveContainer(wave) {
		return ""
	}
	target := cacheTarget(cacheDir, stem, digestBytes(wave), "wav")
	if _, err := os.Stat(target); err == nil {
		return target
	}
	if err := writeFileAtomic(target, wave); err != nil {
		return ""
	}
	return target
}

func DecodeSpPlaResource(data []byte, trackID int) []byte {
	if !IsSpPla(data) || trackID < 1 {
		return nil
	}
	path := writeSpill(data)
	if path == "" {
		return nil
	}
	defer os.Remove(path)
	container, err := OpenSpPla(path)
	if err != nil {
		return nil
	}
	info, ok := container.TryGetTrack(trackID)
	if !ok {
		return nil
	}
	pcm := container.ReadTrack(trackID)
	if len(pcm) == 0 {
		return nil
	}
	raw := make([]byte, len(pcm)*2)
	for i, sample := range pcm {
		raw[2*i] = byte(uint16(sample) & 0xFF)
		raw[2*i+1] = byte((uint16(sample) >> 8) & 0xFF)
	}
	return PcmClip{SampleRate: info.SampleRate, Channels: info.Channels, Pcm: raw}.ToWaveContainer()
}

func writeOggFallback(data []byte, stem string, cacheDir string) string {
	target := cacheTarget(cacheDir, stem, digestBytes(data), "ogg")
	if _, err := os.Stat(target); err == nil {
		return target
	}
	if err := writeFileAtomic(target, data); err != nil {
		return ""
	}
	return target
}

func writeSpill(data []byte) string {
	file, err := os.CreateTemp("", "sppla-*"+materializeOggSpill)
	if err != nil {
		return ""
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return ""
	}
	file.Close()
	return path
}

func cacheTarget(cacheDir string, stem string, digest string, extension string) string {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return filepath.Join(cacheDir, stem+"-"+digest+"."+extension)
	}
	abs, err := filepath.Abs(cacheDir)
	if err != nil {
		return filepath.Join(cacheDir, stem+"-"+digest+"."+extension)
	}
	return filepath.Join(abs, stem+"-"+digest+"."+extension)
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

func digestText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

func readHead(path string, maximum int) []byte {
	stream, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer stream.Close()
	buffer := make([]byte, maximum)
	read := 0
	for read < len(buffer) {
		chunk, err := stream.Read(buffer[read:])
		read += chunk
		if err != nil {
			break
		}
	}
	return buffer[:read]
}

func readFull(stream io.Reader, buffer []byte) bool {
	done := 0
	for done < len(buffer) {
		got, err := stream.Read(buffer[done:])
		done += got
		if err != nil {
			break
		}
		if got == 0 {
			break
		}
	}
	return done == len(buffer)
}

func writeFileAtomic(target string, data []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp := target + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, target); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func isAbsPath(path string) bool {
	return filepath.IsAbs(path) || (len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\'))
}

func fileStem(path string) string {
	base := path
	if index := strings.LastIndex(base, "/"); index >= 0 {
		base = base[index+1:]
	}
	if index := strings.LastIndex(base, "\\"); index >= 0 {
		base = base[index+1:]
	}
	if index := strings.LastIndex(base, "."); index > 0 {
		base = base[:index]
	}
	return base
}

func itoa(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	digits := make([]byte, 0, 20)
	for {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
		if value == 0 {
			break
		}
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
