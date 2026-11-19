package bdt

import (
	"strings"
)

func ClassifyBdtLine(text string) BdtLineKind {
	if len(text) == 0 {
		return BdtLineKindEmpty
	}
	if text[0] == '#' {
		return BdtLineKindComment
	}
	if text[0] == '$' {
		return BdtLineKindLabel
	}
	if text[0] == '"' || text[0] == '@' {
		return BdtLineKindText
	}
	if _, ok := TryParseBdtDialogue(text); ok {
		return BdtLineKindDialogue
	}
	if _, ok := TryParseBdtFaceDialogue(text); ok {
		return BdtLineKindDialogue
	}
	if strings.Contains(text, "\t\"") {
		return BdtLineKindVoice
	}
	if len(text) > 0 {
		return BdtLineKindCommand
	}
	return BdtLineKindUnknown
}

func NormalizeBdtLabelId(id string) string {
	if strings.HasPrefix(id, "$") {
		return id[1:]
	}
	return id
}
