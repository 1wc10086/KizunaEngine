package lua

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestTableToGoKeepsArrays(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	array := L.NewTable()
	array.RawSetInt(1, lua.LNumber(3))
	array.RawSetInt(2, lua.LString("x"))
	object := L.NewTable()
	object.RawSetString("array", array)
	object.RawSetString("flag", lua.LTrue)

	converted := tableToGo(object)
	root, ok := converted.(map[string]any)
	if !ok {
		t.Fatalf("converted=%T", converted)
	}
	list, ok := root["array"].([]any)
	if !ok || len(list) != 2 || list[0] != float64(3) || list[1] != "x" {
		t.Fatalf("array=%#v", root["array"])
	}
	if root["flag"] != true {
		t.Fatalf("flag=%#v", root["flag"])
	}

	restored := goToTable(converted)
	back := restored.RawGetString("array")
	backTable, ok := back.(*lua.LTable)
	if !ok || backTable.Len() != 2 || backTable.RawGetInt(1) != lua.LNumber(3) {
		t.Fatalf("restored=%v", back)
	}
}

func TestCallName(t *testing.T) {
	cases := map[string]string{
		"ResultTraining()":                      "ResultTraining",
		"NextTraining('#result')":               "NextTraining",
		"  CheckAndUnlockAchievement( 'ID50' )": "CheckAndUnlockAchievement",
		"$#ID_BP[0,'BT値'] = 0":                  "",
	}
	for input, want := range cases {
		if got := callName(input); got != want {
			t.Errorf("%q -> %q want %q", input, got, want)
		}
	}
}
