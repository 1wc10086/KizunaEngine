package runtime

import (
	"encoding/binary"
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
)

type motionTestLocation struct {
	name string
	data []byte
}

func (l motionTestLocation) Read() []byte            { return l.data }
func (l motionTestLocation) ReadPrefix(n int) []byte { return l.data[:min(n, len(l.data))] }
func (l motionTestLocation) ReadRange(offset, length int64) []byte {
	if offset < 0 || offset >= int64(len(l.data)) {
		return nil
	}
	end := offset + length
	if end > int64(len(l.data)) {
		end = int64(len(l.data))
	}
	return l.data[offset:end]
}
func (l motionTestLocation) Size() int64  { return int64(len(l.data)) }
func (l motionTestLocation) Name() string { return l.name }

type motionTestIndex struct {
	entries map[string]motionTestLocation
}

func (i motionTestIndex) ContainsExact(name string) bool { _, ok := i.entries[name]; return ok }
func (i motionTestIndex) TryGet(name string) (state.ResourceLocation, bool) {
	location, ok := i.entries[name]
	if !ok {
		return nil, false
	}
	return location, true
}
func (i motionTestIndex) ArchivesInCategory(string) []resource.Archive { return nil }

func fakeBsg(width, height int) []byte {
	data := make([]byte, 0x40)
	copy(data, []byte("BSS-Graphics"))
	binary.LittleEndian.PutUint16(data[0x16:], uint16(width))
	binary.LittleEndian.PutUint16(data[0x18:], uint16(height))
	return data
}

func TestApplyLayerMotionParsesAhe(t *testing.T) {
	gameState := state.NewGameState()
	gameState.SetLayerAt(4, "ev01_022_2p04a", state.StageLayerScreen,
		[]state.LayerArtwork{{Image: "ev01_022_2p04a", Frames: 1, AnimatedPart: -1}})
	director := &StageDirector{
		state:         gameState,
		configuration: &config.BsxxConfiguration{Width: 1024, Height: 768},
		resources: motionTestIndex{entries: map[string]motionTestLocation{
			"A-H-E.txt": {name: "A-H-E.txt", data: []byte(`<layer fps="29.97" begin="1" end="82">
	<frame no="1" opacity="20" pos="512 100"/>
	<frame no="4.5" opacity="100" pos="512 480"/>
	<frame no="70" opacity="100"/>
	<frame no="82" opacity="0" pos="512 500"/>
</layer>`)},
			"ev01_022_2p04a": {name: "ev01_022_2p04a", data: fakeBsg(400, 1000)},
		}},
	}
	if !director.applyLayerMotion(4, "A-H-E") {
		t.Fatal("A-H-E should apply")
	}
	motion := gameState.LayerMotion(4)
	if motion == nil || len(motion.Keyframes) != 4 {
		t.Fatalf("motion = %+v", motion)
	}
	dx, dy, opacity, ok := motion.Transform(motion.Start)
	if !ok || dx != 312 || dy != -400 || opacity < 0.19 || opacity > 0.21 {
		t.Fatalf("start transform = (%d,%d,%v,%v)", dx, dy, opacity, ok)
	}

	// AHE OFF：同一层应被标记为不绘制，而不是继续播放动画。
	director.configuration.AheOff = true
	if !director.applyLayerMotion(4, "A-H-E") {
		t.Fatal("A-H-E should still be consumed when off")
	}
	if !gameState.LayerMotion(4).Hidden {
		t.Fatal("motion should be hidden when AheOff is on")
	}
}
