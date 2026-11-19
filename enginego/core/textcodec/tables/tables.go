package tables

func Cp932Lookup(lead, trail byte) (rune, bool) {
	var row int
	if lead >= 0x81 && lead <= 0x9F {
		row = int(lead - 0x81)
	} else if lead >= 0xE0 && lead <= 0xFC {
		row = int(lead-0xE0) + 31
	} else {
		return 0, false
	}
	var col int
	if trail >= 0x40 && trail <= 0x7E {
		col = int(trail - 0x40)
	} else if trail >= 0x80 && trail <= 0xFC {
		col = int(trail-0x80) + 63
	} else {
		return 0, false
	}
	r := Cp932Table[row*188+col]
	if r < 0 {
		return 0, false
	}
	return r, true
}

func GbkLookup(lead, trail byte) (rune, bool) {
	var row int
	if lead >= 0x81 && lead <= 0xFE {
		row = int(lead - 0x81)
	} else {
		return 0, false
	}
	var col int
	if trail >= 0x40 && trail <= 0x7E {
		col = int(trail - 0x40)
	} else if trail >= 0x80 && trail <= 0xFE {
		col = int(trail-0x80) + 63
	} else {
		return 0, false
	}
	r := GbkTable[row*190+col]
	if r < 0 {
		return 0, false
	}
	return r, true
}

func Cp932Single(b byte) (rune, bool) {
	if b < 0x80 {
		return rune(b), true
	}
	r, ok := Cp932Singles[b]
	return r, ok
}

func GbkSingle(b byte) (rune, bool) {
	if b < 0x80 {
		return rune(b), true
	}
	r, ok := GbkSingles[b]
	return r, ok
}
