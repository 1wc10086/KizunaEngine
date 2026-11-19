package config

import "testing"

const statusLayoutFixture = `{
	base: "status_base"

	"mode button": [
		{ name:"inventory_back", max_state:2, pos:[901,    0], no:125 }
	]

	layout: {
		inventory: {
			base: "inventory_base"
			slot: [
				{ name:"status_ch01_01", pos:[   0,   63 + 133 * 0], condition:"#status_ch01 == 1" }
				{ name:"status_ch01_ev01", pos:[ 145 + 3 * 220,   63 + 0 * 133], condition:"#status_ch01_ev01 != 0" }
			]
		}
	}
}`

func TestParseStatusScreenLayout(t *testing.T) {
	layout, err := ParseStatusScreen([]byte(statusLayoutFixture))
	if err != nil {
		t.Fatal(err)
	}
	if layout.Base != "status_base" {
		t.Fatalf("base = %q", layout.Base)
	}
	if len(layout.ModeButtons) != 1 {
		t.Fatalf("mode buttons = %d", len(layout.ModeButtons))
	}
	button := layout.ModeButtons[0]
	if button.Name != "inventory_back" || button.MaxState != 2 || button.X != 901 || button.Y != 0 || button.No != 125 {
		t.Fatalf("button = %+v", button)
	}
	if len(layout.Pages) != 1 || layout.Pages[0].Key != "inventory" || layout.Pages[0].Base != "inventory_base" {
		t.Fatalf("pages = %+v", layout.Pages)
	}
	slots := layout.Pages[0].Slots
	if len(slots) != 2 {
		t.Fatalf("slots = %d", len(slots))
	}
	if slots[1].X != 805 || slots[1].Y != 63 || slots[1].Condition != "#status_ch01_ev01 != 0" {
		t.Fatalf("slot 1 = %+v", slots[1])
	}
}

func TestEvalStatusExpression(t *testing.T) {
	cases := map[string]int{
		"901":           901,
		"63 + 133 * 2":  329,
		"145 + 3 * 220": 805,
		"(2 + 3) * 4":   20,
		"10 - 4 / 2":    8,
	}
	for expression, want := range cases {
		got, ok := evalStatusExpression(expression)
		if !ok || got != want {
			t.Fatalf("%q = %d,%v; want %d", expression, got, ok, want)
		}
	}
}
