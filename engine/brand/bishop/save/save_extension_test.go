package save

import (
	"testing"

	"kizuna/engine/brand/bishop/vm"
)

func TestSaveExtensionRoundTrip(t *testing.T) {
	store := NewSaveStoreWithSlots(t.TempDir(), 4)
	stored := &SaveData{
		GameName: "test",
		Summary:  "slot",
		Machine:  vm.MachineState{},
		ScriptDB: map[string]map[string]map[string]int32{
			"ID_BP": {"従順度": {"早紀": 7}},
		},
		PrivateData:          map[string]any{"play_status": map[string]any{"next_time": float64(3)}},
		CommonData:           map[string]any{"ter_grid": map[string]any{"0": float64(5)}},
		UnlockedAchievements: map[string]bool{"ID20": true},
	}
	if !store.Write(stored, 0) {
		t.Fatal("write failed")
	}
	loaded := store.Read(0)
	if loaded == nil {
		t.Fatal("read failed")
	}
	if got := loaded.ScriptDB["ID_BP"]["従順度"]["早紀"]; got != 7 {
		t.Fatalf("scriptDb=%d", got)
	}
	if !loaded.UnlockedAchievements["ID20"] {
		t.Fatal("achievement flag lost")
	}
	if len(loaded.PrivateData) != 1 || len(loaded.CommonData) != 1 {
		t.Fatalf("private=%v common=%v", loaded.PrivateData, loaded.CommonData)
	}
}
