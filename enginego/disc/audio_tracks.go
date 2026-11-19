package disc

// audio_tracks.go 把 CD RAW 镜像尾部的 CD-DA 音轨导出为 WAV，文件名前缀
// audio_track_lba 与 brand/bishop/audio.FindCdWave 的查找规则一致。
// 只在解包树里出现 SCNDAT.TBL 或 SCNFILE.PFT 时才导出（见 exportDiscAudio），
// 避免给其它光盘白写几百 MB。

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	cdFrameBytes     = 2352
	cdPCMRate        = 44100
	cdPCMChannels    = 2
	cdPCMBitsPerSamp = 16
	cdWaveHeaderSize = 44
)

// 数据扇区的同步头：00 FF*10 00。音轨扇区是随机 PCM，几乎不可能命中。
var cdSyncPattern = []byte{0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00}

// exportDiscAudio 在解包树里找到游戏数据目录后，为每张 RAW 镜像导出音轨。
// 失败只记日志，不影响解包结果。
func exportDiscAudio(mount string, paths []string) {
	dataDir := findSceneDataDir(mount)
	if dataDir == "" {
		return
	}
	for _, path := range paths {
		image, err := openSectorImage(path)
		if err != nil {
			log.Printf("disc: audio %s: %v", filepath.Base(path), err)
			continue
		}
		if image.layout.size == 2352 || image.layout.size == 2448 {
			if start, ok := findCDAudioStart(image); ok {
				target := filepath.Join(dataDir, "audio", fmt.Sprintf("audio_track_lba%d.wav", start))
				frames, err := writeCDAudioWave(image, start, target)
				if err != nil {
					log.Printf("disc: audio track lba=%d: %v", start, err)
				} else {
					log.Printf("disc: audio track lba=%d frames=%d -> %s", start, frames, target)
				}
			}
		}
		_ = image.Close()
	}
}

// findSceneDataDir 返回解包树里包含 SCNDAT.TBL 或 SCNFILE.PFT 的目录。
func findSceneDataDir(mount string) string {
	var dir string
	_ = filepath.WalkDir(mount, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || dir != "" {
			return nil
		}
		name := d.Name()
		if strings.EqualFold(name, "SCNDAT.TBL") || strings.EqualFold(name, "SCNFILE.PFT") || strings.EqualFold(name, "MAINCON.SCS") {
			dir = filepath.Dir(p)
		}
		return nil
	})
	return dir
}

// cdLogicalSectors 返回逻辑扇区总数。含重复扇区映射时以映射的最大 LBA 为准。
func cdLogicalSectors(image *sectorImage) int64 {
	if image.lbaMap != nil {
		return int64(image.maxLBA) + 1
	}
	return image.fileSector
}

func isCDDataSector(image *sectorImage, lba int64) bool {
	head := make([]byte, len(cdSyncPattern))
	offset := int64(image.physical(uint32(lba))) * int64(image.layout.size)
	n, _ := image.file.ReadAt(head, offset)
	return n == len(head) && bytes.Equal(head, cdSyncPattern)
}

// findCDAudioStart 二分查找最后一个数据扇区，返回其后第一个扇区的 LBA。
// 数据区连续排列、音轨在末尾，所以判定是单调的，参考 项目参考/1.py find_audio_start。
func findCDAudioStart(image *sectorImage) (uint32, bool) {
	total := cdLogicalSectors(image)
	if total <= 1 || !isCDDataSector(image, 0) {
		return 0, false
	}
	lo, hi := int64(0), total-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if isCDDataSector(image, mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo >= total-1 {
		return 0, false
	}
	return uint32(lo + 1), true
}

// writeCDAudioWave 把 [start, 末尾) 的音轨写成 44.1kHz 立体声 16bit WAV。
// 已存在且大小正确则跳过；先写临时文件再 rename，避免半成品被当作完整音轨。
func writeCDAudioWave(image *sectorImage, start uint32, target string) (int64, error) {
	total := cdLogicalSectors(image)
	if int64(start) >= total {
		return 0, errors.New("audio start beyond image")
	}
	frames := total - int64(start)
	pcmBytes := frames * cdFrameBytes
	if info, err := os.Stat(target); err == nil && info.Size() == cdWaveHeaderSize+pcmBytes {
		return frames, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	temp := target + ".tmp"
	out, err := os.Create(temp)
	if err != nil {
		return 0, err
	}
	if err := writeCDAudioBody(out, image, start, total, pcmBytes); err != nil {
		_ = out.Close()
		_ = os.Remove(temp)
		return 0, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(temp)
		return 0, err
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return 0, err
	}
	return frames, nil
}

func writeCDAudioBody(out io.Writer, image *sectorImage, start uint32, total, pcmBytes int64) error {
	if _, err := out.Write(wavePCMHeader(pcmBytes)); err != nil {
		return err
	}
	// RAW 扇区的前 2352 字节就是 PCM，2448 的子通道尾部忽略。
	buffer := make([]byte, cdFrameBytes)
	for lba := int64(start); lba < total; lba++ {
		offset := int64(image.physical(uint32(lba))) * int64(image.layout.size)
		n, err := image.file.ReadAt(buffer, offset)
		if n < len(buffer) {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
		if _, err := out.Write(buffer); err != nil {
			return err
		}
	}
	return nil
}

func wavePCMHeader(pcmBytes int64) []byte {
	header := make([]byte, cdWaveHeaderSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+pcmBytes))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], cdPCMChannels)
	binary.LittleEndian.PutUint32(header[24:28], cdPCMRate)
	binary.LittleEndian.PutUint32(header[28:32], cdPCMRate*cdPCMChannels*cdPCMBitsPerSamp/8)
	binary.LittleEndian.PutUint16(header[32:34], cdPCMChannels*cdPCMBitsPerSamp/8)
	binary.LittleEndian.PutUint16(header[34:36], cdPCMBitsPerSamp)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(pcmBytes))
	return header
}
