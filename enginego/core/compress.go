package core

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

// maxDeflateRatio 是 DEFLATE 的理论极限压缩比（1032:1）。解压输出的大小只由数据自己
// 带的那个字段决定（归档目录项的 Unpacked、图像头里的两块解压后尺寸、存档快照头），
// 字段一旦损坏成几 GB，按它一次分配会当场把进程打死；所以先按上界判一次，坏数据走报错
// 路径而不是 OOM 路径。合法流不可能超过这个比值，被这条挡下的全是本来也会失败的数据。
const maxDeflateRatio = 1032

// sizeMismatch 表示流本身解完了、但输出长度不是要求的那个数；got 是实际解出的字节数
// （比目标多出字节时记 len(dst)+1）。单独成类型是为了让报错文案保持"解出 N 字节、期望
// M 字节"这个原有说法——上层日志一直靠它区分"流坏了"和"长度字段写错了"。
type sizeMismatch struct{ got int }

func (e *sizeMismatch) Error() string { return fmt.Sprintf("unpacked size %d != expected", e.got) }

// InflateExact 解压 data，要求输出恰好 expected 字节，返回精确长度的切片。
//
// 旧实现走 io.ReadAll：ReadAll 只能靠 append 倍增扩容，它并不知道结果有多长。解一张
// 2760x1640 的图（18MB 像素）时，那意味着额外分配并拷贝约两倍于结果的中间缓冲，全是
// 瞬时垃圾——低端 ARM 手机上正好在大图落地那一刻凑够触发 GC 的条件，表现为点一下卡
// 0.5~1.1 秒。expected 本来就是调用方已经知道的精确尺寸，直接按它一次分配、一遍写入
// 即可：输出字节、长度校验、报错文案都与旧实现逐位一致。
func InflateExact(data []byte, expected int, label string) ([]byte, error) {
	if expected < 0 {
		expected = 0
	}
	if int64(expected) > int64(len(data))*maxDeflateRatio+64 {
		return nil, fmt.Errorf("%s: unpacked size %d implausible for %d compressed bytes", label, expected, len(data))
	}
	out := make([]byte, expected)
	if err := InflateExactInto(data, out, label); err != nil {
		return nil, err
	}
	return out, nil
}

// InflateExactInto 把 data 解压进 dst，要求输出恰好 len(dst) 字节。
// 供调用方自己备好缓冲的场合使用：像素由两段流拼成，两段可以直接解进同一张图
// 缓冲的两个区间，省掉每段一次"解到临时切片、再 copy 进像素缓冲"的分配加拷贝。
func InflateExactInto(data, dst []byte, label string) error {
	// 先按 raw deflate 解、失败再按 zlib 解：两种流在数据里都出现过（像素块是 zlib，
	// 归档条目两种都有）。两条路径都失败时返回 raw deflate 那条错误，
	// 顺序与旧实现一致。
	err := readExact(flate.NewReader(bytes.NewReader(data)), dst)
	if err == nil {
		return nil
	}
	if zr, zerr := zlib.NewReader(bytes.NewReader(data)); zerr == nil && readExact(zr, dst) == nil {
		return nil
	}
	var mm *sizeMismatch
	if errors.As(err, &mm) {
		return fmt.Errorf("%s: unpacked size %d != expected %d", label, mm.got, len(dst))
	}
	return err
}

// readExact 要求 r 恰好产出 len(dst) 字节：少一个字节是长度字段写得比实际大，多一个
// 字节是写得比实际小，两种都算失败。与 io.ReadAll 的差别只在"往哪写"：这里一遍写进
// 调用方给的缓冲，不产生中间垃圾。
func readExact(r io.ReadCloser, dst []byte) error {
	defer r.Close()
	n := 0
	for n < len(dst) {
		m, err := r.Read(dst[n:])
		n += m
		if err != nil {
			if err == io.EOF {
				if n < len(dst) {
					// 流干净地结束却没填满 dst，长度字段比实际输出大。
					return &sizeMismatch{got: n}
				}
				// Reader 允许在填满缓冲的同一次调用里返回 io.EOF，这不是失败。
				return nil
			}
			return err
		}
	}
	// dst 已填满：再多解出一个字节就说明长度字段写小了。旧实现用 LimitReader(expected+1)
	// 多读那一个字节，同样算失败，且报错里的字节数按期望长度 +1 记——保持这个文案。
	var tail [1]byte
	m, err := r.Read(tail[:])
	switch {
	case m > 0:
		return &sizeMismatch{got: n + 1}
	case err == io.EOF, err == nil:
		return nil
	default:
		return err
	}
}
