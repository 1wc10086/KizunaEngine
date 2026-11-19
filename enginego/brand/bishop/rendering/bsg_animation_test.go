package rendering

import (
	"encoding/binary"
	"testing"
)

type fakeAnimationLocation struct {
	data []byte
}

func (f fakeAnimationLocation) Name() string { return "fake-anim" }

func (f fakeAnimationLocation) Size() int64 { return int64(len(f.data)) }

func (f fakeAnimationLocation) ReadRange(offset, length int64) []byte {
	if offset < 0 || offset >= int64(len(f.data)) || length <= 0 {
		return nil
	}
	if offset+length > int64(len(f.data)) {
		length = int64(len(f.data)) - offset
	}
	return f.data[offset : offset+length]
}

// tinyBgraFrame 造一个 1x1 的 BSG 帧，颜色由参数决定。
func tinyBgraFrame(b, g, r, a byte) []byte {
	frame := make([]byte, 0x40+4)
	copy(frame, "BSS-Graphics")
	binary.LittleEndian.PutUint16(frame[0x16:], 1)
	binary.LittleEndian.PutUint16(frame[0x18:], 1)
	frame[0x30] = 0 // BGRA32
	frame[0x31] = 0 // no compression
	binary.LittleEndian.PutUint32(frame[0x32:], 0x40)
	binary.LittleEndian.PutUint32(frame[0x36:], 4)
	frame[0x40] = b
	frame[0x41] = g
	frame[0x42] = r
	frame[0x43] = a
	return frame
}

func animationContainer(frames ...[]byte) []byte {
	data := make([]byte, 0x20)
	copy(data, "BSS-Composition")
	data[0x11] = byte(len(frames))
	for _, frame := range frames {
		data = append(data, frame...)
	}
	return data
}

func TestParseAnimationScript(t *testing.T) {
	script := []byte(`<layer fps="35" begin="0" end="2" continue="true" count="0">
	<frame no="0" layer="2"/>
	<frame no="1" layer="1"/>
	<frame no="2" layer="0"/>
</layer>`)
	spec := parseAnimationScript(script, 3)
	if spec.fps != 35 || spec.begin != 0 || spec.end != 2 {
		t.Fatalf("spec = %+v", spec)
	}
	if spec.mapping[0] != 2 || spec.mapping[1] != 1 || spec.mapping[2] != 0 {
		t.Fatalf("mapping = %v", spec.mapping)
	}
}

func TestAnimationPlayerReadsMappedFrame(t *testing.T) {
	first := tinyBgraFrame(0, 0, 255, 255)  // 红色
	second := tinyBgraFrame(0, 255, 0, 255) // 绿色
	container := animationContainer(first, second)
	script := []byte(`<layer fps="1" begin="0" end="1"><frame no="0" layer="1"/><frame no="1" layer="0"/></layer>`)

	player, err := NewBsgAnimationPlayer(fakeAnimationLocation{data: container}, script)
	if err != nil {
		t.Fatal(err)
	}
	image := player.Frame(player.start)
	if image == nil || image.Width != 1 || image.Height != 1 {
		t.Fatalf("frame = %+v", image)
	}
	// 第 0 帧映射到 layer=1（绿色）。
	if image.Pixels[1] < 200 || image.Pixels[2] > 50 {
		t.Fatalf("expected the green frame, got BGRA=%v", image.Pixels[:4])
	}
}
