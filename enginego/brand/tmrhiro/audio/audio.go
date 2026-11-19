// Package audio 处理 TMR-Hiro 的音频资源。
//
// 三种容器在归档里混着放：
//
//   - Vorbis：直接以 "OggS" 开头（VIC 语音、部分 SE）；
//   - 原生 PCM：头 4 字节是 'D' + 3 字节保留，偏移 4 是一个 u8 标记（实测为 0），
//     偏移 5 是 i32 长度 = 文件长度-9，之后是 44.1kHz/双声道/16bit 裸 PCM。
//     （GARbro 的 ArcFormats/Tmr-Hiro/AudioTmr.cs 就是这个判定。）
//   - 裸 PCM 旧式包（MUS）：头部 0x2c 字节之后才是数据，采样率 44.1kHz。
//
// 宿主只吃文件，所以这里把三种都写成 .wav 或原样 .ogg 落到缓存目录，
// 由 op 14（ReadAsset）/ 文件路径两种方式交给播放器。
package audio

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind 是识别出的音频类型。
type Kind int

const (
	KindOgg Kind = iota
	KindTMRHiroPCM
	KindRawPCM
)

// Probe 按魔数判断类型。
func Probe(data []byte) Kind {
	if len(data) >= 4 && string(data[0:4]) == "OggS" {
		return KindOgg
	}
	if len(data) >= 9 && data[0] == 'D' && data[4] == 0 {
		if int32(binary.LittleEndian.Uint32(data[5:9])) == int32(len(data)-9) {
			return KindTMRHiroPCM
		}
	}
	return KindRawPCM
}

// WaveFormat 描述 PCM 的声道与采样率。
type WaveFormat struct {
	Channels   int
	SampleRate int
	BitsPerSam int
}

// Decode 把归档里的音频字节转成宿主能直接播放的文件内容。
//
// 返回 (扩展名, 内容)。Ogg 原样返回 ".ogg"；PCM 包成标准 RIFF/WAVE 返回 ".wav"。
// 裸 PCM 的采样率无法从文件里读出（旧版音乐包只有 0x2c 字节头），
// 按 Lune 系的惯例取 44100、双声道（GARbro 对 .bgm/.wda 用 44100/单声道，
// 对 MUS.PAC 系则不做转换；本作实测 BGM 是双声道，见 README 的 44.1kHz 说明）。
func Decode(data []byte) (string, []byte, error) {
	switch Probe(data) {
	case KindOgg:
		return ".ogg", data, nil
	case KindTMRHiroPCM:
		// 双声道 44.1kHz 16bit：与 AudioTmr.cs 一致。
		return ".wav", WrapPCM(data[9:], WaveFormat{Channels: 2, SampleRate: 44100, BitsPerSam: 16}), nil
	default:
		// 裸 PCM：跳过 0x2c 字节的头。头里没有可靠的长度字段，所以只做包装，
		// 不校验长度，避免把可播的 BGM 判成坏数据。
		body := data
		if len(body) > 0x2c {
			body = body[0x2c:]
		}
		return ".wav", WrapPCM(body, WaveFormat{Channels: 2, SampleRate: 44100, BitsPerSam: 16}), nil
	}
}

// WrapPCM 给裸 PCM 套上 RIFF/WAVE 头。
func WrapPCM(pcm []byte, format WaveFormat) []byte {
	blockAlign := format.Channels * format.BitsPerSam / 8
	avg := format.SampleRate * blockAlign
	out := make([]byte, 44+len(pcm))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+len(pcm)))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:24], uint16(format.Channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(format.SampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(avg))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], uint16(format.BitsPerSam))
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(pcm)))
	copy(out[44:], pcm)
	return out
}

// Materializer 把“资源名”写成宿主可播的绝对路径文件。
//
// 为什么不走 op 14（ReadAsset）直接交给 Dart 解码：宿主音频后端按路径流式
// 播放长曲（流式解码比一次读 10MB PCM 进内存更省），而且 Ogg/PCM 两种容器
// 在 Android 上的解码器路径不同，落盘后由同一条路径处理最省事。
type Materializer struct {
	dir  string
	seen map[string]string // 资源名 → 已写出的路径
}

// NewMaterializer 建物化器，dir 通常是 <cache>/kizuna/<hash>/audio。
func NewMaterializer(dir string) *Materializer {
	_ = os.MkdirAll(dir, 0o755)
	return &Materializer{dir: dir, seen: make(map[string]string)}
}

// SafeName 把资源名变成安全文件名（去掉路径分隔符与冒号）。
func SafeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, name)
}

// Write 落盘并返回绝对路径。取不到资源时返回空串。
func (m *Materializer) Write(name string, raw []byte) (string, error) {
	if m == nil || name == "" || len(raw) == 0 {
		return "", fmt.Errorf("tmrhiro/audio: 空资源")
	}
	if path, ok := m.seen[name]; ok {
		return path, nil
	}
	ext, body, err := Decode(raw)
	if err != nil {
		return "", err
	}
	path := filepath.Join(m.dir, SafeName(name)+ext)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	m.seen[name] = path
	return path, nil
}

// Reset 清空“已写出”记录（换游戏/重建缓存目录时用）。
func (m *Materializer) Reset() { m.seen = make(map[string]string) }
