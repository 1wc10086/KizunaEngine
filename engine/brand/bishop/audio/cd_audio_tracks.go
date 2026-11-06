package audio

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	cdExpectedSampleRate = 44100
	cdExpectedBits       = 16
	cdWindowSeconds      = 0.01
	cdSilenceRms         = 300.0
)

var cdSubdirectories = []string{"audio", "AUDIO", "music", "MUSIC"}

type CdSegment struct {
	Start float64
	End   float64
}

func FindCdWave(directory string) string {
	return findFirstCdWave(directory)
}

func FindCdWaveRecursive(root string, maxDepth int) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return ""
	}
	type entry struct {
		path  string
		depth int
	}
	matches := []string{}
	queue := []entry{{path: root, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if direct := findFirstCdWave(current.path); direct != "" {
			matches = append(matches, direct)
		}
		if current.depth >= maxDepth {
			continue
		}
		children, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.IsDir() {
				queue = append(queue, entry{path: filepath.Join(current.path, child.Name()), depth: current.depth + 1})
			}
		}
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
	})
	return matches[0]
}

func findFirstCdWave(directory string) string {
	if strings.TrimSpace(directory) == "" {
		return ""
	}
	roots := make([]string, 0, len(cdSubdirectories)+1)
	roots = append(roots, directory)
	for _, sub := range cdSubdirectories {
		roots = append(roots, filepath.Join(directory, sub))
	}
	matches := []string{}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, item := range entries {
			if item.IsDir() {
				continue
			}
			if isCdWaveName(item.Name()) {
				matches = append(matches, filepath.Join(root, item.Name()))
			}
		}
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
	})
	return matches[0]
}

func isCdWaveName(path string) bool {
	name := filepath.Base(path)
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "audio_track_lba") && strings.HasSuffix(lower, ".wav")
}

func CdSegments(wavePath string, minimumGapSeconds float64, fallback []CdSegment) []CdSegment {
	if minimumGapSeconds <= 0 {
		return fallback
	}
	if strings.TrimSpace(wavePath) == "" {
		return fallback
	}
	sampleRate, channels, bits, dataOffset, dataLength, ok := readWaveHeader(wavePath)
	if !ok {
		return fallback
	}
	if sampleRate != cdExpectedSampleRate || bits != cdExpectedBits || (channels != 1 && channels != 2) {
		return fallback
	}
	totalFrames := dataLength / int64(channels*2)
	if totalFrames <= 0 {
		return fallback
	}
	totalSeconds := float64(totalFrames) / float64(sampleRate)
	silent, ok := collectSilenceWindows(wavePath, dataOffset, dataLength, channels, sampleRate/100)
	if !ok {
		return fallback
	}
	boundaries := []float64{}
	runStart := -1
	for index := 0; index <= len(silent); index++ {
		isSilent := index < len(silent) && silent[index]
		if isSilent {
			if runStart < 0 {
				runStart = index
			}
		} else if runStart >= 0 {
			end := math.Min(float64(index)*cdWindowSeconds, totalSeconds)
			if end-float64(runStart)*cdWindowSeconds >= minimumGapSeconds && end < totalSeconds-0.000001 {
				boundaries = append(boundaries, end)
			}
			runStart = -1
		}
	}
	result := make([]CdSegment, 0, len(boundaries)+1)
	previous := 0.0
	for _, boundary := range boundaries {
		if boundary > previous {
			result = append(result, CdSegment{Start: previous, End: boundary})
		}
		previous = boundary
	}
	result = append(result, CdSegment{Start: previous, End: totalSeconds})
	return result
}

func MaterializeCdSlice(wavePath string, startSeconds float64, endSeconds float64, cacheDirectory string, key string) string {
	if strings.TrimSpace(wavePath) == "" || strings.TrimSpace(cacheDirectory) == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	if _, err := os.Stat(wavePath); err != nil {
		return ""
	}
	if !(endSeconds > startSeconds) || startSeconds < 0 {
		return ""
	}
	safeKey := fileBase(key)
	if safeKey == "" {
		return ""
	}
	sampleRate, channels, bits, dataOffset, dataLength, ok := readWaveHeader(wavePath)
	if !ok {
		return ""
	}
	if sampleRate <= 0 || channels < 1 || (bits != 8 && bits != 16) {
		return ""
	}
	frameSize := int64(channels * bits / 8)
	if frameSize <= 0 {
		return ""
	}
	totalFrames := dataLength / frameSize
	startFrame := int64(startSeconds * float64(sampleRate))
	endFrame := int64(endSeconds * float64(sampleRate))
	if startFrame < 0 {
		startFrame = 0
	}
	if endFrame > totalFrames {
		endFrame = totalFrames
	}
	if endFrame <= startFrame {
		return ""
	}
	pcmLength := (endFrame - startFrame) * frameSize
	target := cachePath(cacheDirectory, safeKey+".wav")
	if info, err := os.Stat(target); err == nil && info.Size() == 44+pcmLength {
		return target
	}
	if err := writeWaveSlice(wavePath, target, dataOffset+(startFrame*frameSize), pcmLength, sampleRate, channels, bits); err != nil {
		os.Remove(target)
		return ""
	}
	return target
}

func collectSilenceWindows(wavePath string, dataOffset int64, dataLength int64, channels int, windowFrames int) ([]bool, bool) {
	silent := []bool{}
	frameSize := channels * 2
	samplesPerWindow := windowFrames * channels
	buffer := make([]byte, frameSize*windowFrames*32)
	stream, err := os.Open(wavePath)
	if err != nil {
		return nil, false
	}
	defer stream.Close()
	if _, err := stream.Seek(dataOffset, 0); err != nil {
		return nil, false
	}
	windowSamples := 0
	sumSquares := 0.0
	remaining := dataLength
	for remaining > 0 {
		want := len(buffer)
		if int64(want) > remaining {
			want = int(remaining)
		}
		n, err := readAtLeast(stream, buffer[:want])
		if n <= 0 {
			break
		}
		remaining -= int64(n)
		for index := 0; index+1 < n; index += 2 {
			sample := int16(int(buffer[index]) | int(buffer[index+1])<<8)
			sumSquares += float64(sample) * float64(sample)
			windowSamples++
			if windowSamples >= samplesPerWindow {
				silent = append(silent, math.Sqrt(sumSquares/float64(windowSamples)) < cdSilenceRms)
				sumSquares = 0.0
				windowSamples = 0
			}
		}
		if err != nil {
			break
		}
	}
	if windowSamples > 0 {
		silent = append(silent, math.Sqrt(sumSquares/float64(windowSamples)) < cdSilenceRms)
	}
	return silent, true
}

func readWaveHeader(wavePath string) (sampleRate int, channels int, bits int, dataOffset int64, dataLength int64, ok bool) {
	stream, err := os.Open(wavePath)
	if err != nil {
		return 0, 0, 0, 0, 0, false
	}
	defer stream.Close()
	info, err := stream.Stat()
	if err != nil || info.Size() < 12 {
		return 0, 0, 0, 0, 0, false
	}
	head := make([]byte, 12)
	if !readFull(stream, head) {
		return 0, 0, 0, 0, 0, false
	}
	if !isFourCc(head, 0, 'R', 'I', 'F', 'F') || !isFourCc(head, 8, 'W', 'A', 'V', 'E') {
		return 0, 0, 0, 0, 0, false
	}
	chunkHead := make([]byte, 8)
	for guard := 0; guard < 64; guard++ {
		if !readFull(stream, chunkHead) {
			break
		}
		size := readUint32(chunkHead[4:8])
		isFmt := isFourCc(chunkHead, 0, 'f', 'm', 't', ' ')
		isData := isFourCc(chunkHead, 0, 'd', 'a', 't', 'a')
		if isFmt {
			if size < 16 {
				return 0, 0, 0, 0, 0, false
			}
			fmt := make([]byte, 16)
			if !readFull(stream, fmt) {
				return 0, 0, 0, 0, 0, false
			}
			if readUint16(fmt[0:2]) != 1 {
				return 0, 0, 0, 0, 0, false
			}
			channels = int(readUint16(fmt[2:4]))
			sampleRate = int(readUint32(fmt[4:8]))
			bits = int(readUint16(fmt[14:16]))
			skip := int64(size) - 16 + int64(size&1)
			if skip > 0 {
				if _, err := stream.Seek(skip, 1); err != nil {
					return 0, 0, 0, 0, 0, false
				}
			}
		} else if isData {
			if channels == 0 {
				return 0, 0, 0, 0, 0, false
			}
			position, err := stream.Seek(0, 1)
			if err != nil {
				return 0, 0, 0, 0, 0, false
			}
			if position+int64(size) > info.Size() {
				return 0, 0, 0, 0, 0, false
			}
			return sampleRate, channels, bits, position, int64(size), true
		} else {
			skip := int64(size) + int64(size&1)
			position, err := stream.Seek(0, 1)
			if err != nil {
				break
			}
			if skip <= 0 || position+skip > info.Size() {
				break
			}
			if _, err := stream.Seek(skip, 1); err != nil {
				break
			}
		}
	}
	return 0, 0, 0, 0, 0, false
}

func writeWaveSlice(wavePath string, target string, sourceOffset int64, pcmLength int64, sampleRate int, channels int, bits int) error {
	head := make([]byte, 44)
	head[0] = 'R'
	head[1] = 'I'
	head[2] = 'F'
	head[3] = 'F'
	putUint32(head[4:8], uint32(36+pcmLength))
	head[8] = 'W'
	head[9] = 'A'
	head[10] = 'V'
	head[11] = 'E'
	head[12] = 'f'
	head[13] = 'm'
	head[14] = 't'
	head[15] = ' '
	putUint32(head[16:20], 16)
	putUint16(head[20:22], 1)
	putUint16(head[22:24], uint16(channels))
	putUint32(head[24:28], uint32(sampleRate))
	putUint32(head[28:32], uint32(sampleRate*channels*bits/8))
	putUint16(head[32:34], uint16(channels*bits/8))
	putUint16(head[34:36], uint16(bits))
	head[36] = 'd'
	head[37] = 'a'
	head[38] = 't'
	head[39] = 'a'
	putUint32(head[40:44], uint32(pcmLength))
	input, err := os.Open(wavePath)
	if err != nil {
		return err
	}
	defer input.Close()
	if _, err := input.Seek(sourceOffset, 0); err != nil {
		return err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp := target + ".tmp"
	output, err := os.Create(temp)
	if err != nil {
		return err
	}
	if _, err := output.Write(head); err != nil {
		output.Close()
		os.Remove(temp)
		return err
	}
	buffer := make([]byte, 65536)
	remaining := pcmLength
	for remaining > 0 {
		want := len(buffer)
		if int64(want) > remaining {
			want = int(remaining)
		}
		n, err := readAtLeast(input, buffer[:want])
		if n <= 0 {
			output.Close()
			os.Remove(temp)
			return errors.New("wave ends inside requested slice")
		}
		if _, err := output.Write(buffer[:n]); err != nil {
			output.Close()
			os.Remove(temp)
			return err
		}
		remaining -= int64(n)
		if err != nil {
			break
		}
	}
	if err := output.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, target); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func readAtLeast(stream *os.File, buffer []byte) (int, error) {
	read := 0
	for read < len(buffer) {
		chunk, err := stream.Read(buffer[read:])
		read += chunk
		if err != nil {
			return read, err
		}
		if chunk <= 0 {
			break
		}
	}
	return read, nil
}

func fileBase(path string) string {
	base := path
	if index := strings.LastIndex(base, "/"); index >= 0 {
		base = base[index+1:]
	}
	if index := strings.LastIndex(base, "\\"); index >= 0 {
		base = base[index+1:]
	}
	return base
}

func cachePath(directory string, name string) string {
	return filepath.Join(directory, name)
}
