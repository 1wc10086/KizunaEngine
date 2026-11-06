package bs

import (
	"kizuna/engine/core"
)

var ancientCandidates = []BsScriptDialect{
	BsScriptAncientWide,
	BsScriptAncientCompact,
}

func TryDetectBsDialect(code []byte, signature string) (BsScriptDialect, bool) {
	version := RevSignatureVersion(signature)
	hint, hasHint := RevForVersion(version)
	if !hasHint {
		if dialect, ok := tryDetectAncient(code); ok {
			return dialect, true
		}
		if dialect, ok := tryDetectRev(code, -1); ok {
			return dialect, true
		}
		return BsScriptAncientWide, false
	}
	if ConsumesExactly(code, hint) {
		return hint, true
	}
	if dialect, ok := tryDetectRev(code, int(hint)); ok {
		return dialect, true
	}
	if dialect, ok := tryDetectAncient(code); ok {
		return dialect, true
	}
	return hint, false
}

func tryDetectAncient(code []byte) (BsScriptDialect, bool) {
	wideExact := ConsumesExactly(code, BsScriptAncientWide)
	compactExact := ConsumesExactly(code, BsScriptAncientCompact)
	if wideExact && compactExact {
		core.Logf("script: BSScript dialect ambiguous: wide and compact both consume %d bytes exactly, preferring wide", len(code))
	}
	if wideExact {
		return BsScriptAncientWide, true
	}
	if compactExact {
		return BsScriptAncientCompact, true
	}
	return BsScriptAncientWide, false
}

func tryDetectRev(code []byte, hint int) (BsScriptDialect, bool) {
	for _, candidate := range RevVersions {
		if int(candidate) == hint {
			continue
		}
		if ConsumesExactly(code, candidate) {
			return candidate, true
		}
	}
	if hint >= 0 {
		return BsScriptDialect(hint), false
	}
	return BsScriptAncientWide, false
}

func ConsumesExactly(code []byte, dialect BsScriptDialect) bool {
	return len(code) > 0 && Consumed(code, dialect) == len(code)
}

func Consumed(code []byte, dialect BsScriptDialect) int {
	if RevIsRev(dialect) {
		position := 0
		for position < len(code) {
			size := RevSize(dialect, code, position)
			if size <= 0 {
				break
			}
			position += size
		}
		return position
	}
	cursor := 0
	for cursor < len(code) {
		opcode := BsSemantic(code[cursor], dialect)
		if !BsIsKnown(opcode) {
			break
		}
		cursor += BsSize(opcode)
	}
	return cursor
}

func AlignScore(code []byte, labels map[int]bool, dialect BsScriptDialect) int {
	limit := len(code)
	if limit > 0x800 {
		limit = 0x800
	}
	if RevIsRev(dialect) {
		position := 0
		score := 0
		for position < limit {
			if labels[position] {
				score++
			}
			size := RevSize(dialect, code, position)
			if size <= 0 {
				break
			}
			position += size
		}
		return score
	}
	cursor := 0
	hits := 0
	for cursor < limit {
		if labels[cursor] {
			hits++
		}
		opcode := BsSemantic(code[cursor], dialect)
		if !BsIsKnown(opcode) {
			break
		}
		cursor += BsSize(opcode)
	}
	return hits
}

func BestByAlignment(code []byte, labels map[int]bool, hasHint bool, hint BsScriptDialect) BsScriptDialect {
	if ConsumesExactly(code, BsScriptAncientWide) && ConsumesExactly(code, BsScriptAncientCompact) {
		core.Logf("script: BSScript alignment ambiguous: wide and compact both consume %d bytes exactly", len(code))
	}
	best := BsScriptAncientWide
	if hasHint {
		best = hint
	}
	bestScore := AlignScore(code, labels, best)
	revFirst := hasHint
	for pass := 0; pass < 2; pass++ {
		if (pass == 0) == revFirst {
			for _, candidate := range RevVersions {
				if candidate == best {
					continue
				}
				score := AlignScore(code, labels, candidate)
				if score > bestScore {
					bestScore = score
					best = candidate
				}
			}
			continue
		}
		for _, candidate := range ancientCandidates {
			if candidate == best {
				continue
			}
			score := AlignScore(code, labels, candidate)
			if score > bestScore {
				bestScore = score
				best = candidate
			}
		}
	}
	return best
}
