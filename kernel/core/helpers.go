package core

func FindSection(sections []Section, kind, name string) *Section {
	for i := range sections {
		if sections[i].Kind == kind && sections[i].Name == name {
			return &sections[i]
		}
	}
	return nil
}

func NegateBytes(dst []byte) {
	for i := range dst {
		dst[i] = ^dst[i]
	}
}

func SummarizeEOL(eols []string) string {
	crlf := false
	lf := false
	for _, e := range eols {
		switch e {
		case "crlf":
			crlf = true
		case "lf":
			lf = true
		}
	}
	switch {
	case crlf && lf:
		return "mixed"
	case lf:
		return "lf"
	default:
		return "crlf"
	}
}
