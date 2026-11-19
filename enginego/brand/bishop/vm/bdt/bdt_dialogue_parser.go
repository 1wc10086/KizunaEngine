package bdt

import (
	"strings"
)

func TryParseBdtDialogue(line string) (BdtDialogue, bool) {
	var dialogue BdtDialogue
	face, speaker, voiceCode, position, ok := scanBdtPrefix(line, '"', 0)
	if !ok {
		return dialogue, false
	}
	body := line[position+1:]
	if len(body) > 0 && body[len(body)-1] == '"' && strings.IndexByte(body, '"') == len(body)-1 {
		body = body[:len(body)-1]
	}
	if len(body) == 0 {
		return dialogue, false
	}
	dialogue = BdtDialogue{Face: face, Speaker: speaker, Text: body, VoiceCode: voiceCode}
	return dialogue, true
}

func TryParseBdtFaceDialogue(line string) (BdtDialogue, bool) {
	var dialogue BdtDialogue
	face, speaker, voiceCode, position, ok := scanBdtPrefix(line, '@', 0)
	if !ok {
		return dialogue, false
	}
	if len(face) == 0 && len(speaker) == 0 && len(voiceCode) == 0 {
		return dialogue, false
	}
	body := line[position+1:]
	if len(body) == 0 {
		return dialogue, false
	}
	dialogue = BdtDialogue{Face: face, Speaker: speaker, Text: body, VoiceCode: voiceCode}
	return dialogue, true
}

func scanBdtPrefix(line string, bodyLead byte, position int) (string, string, string, int, bool) {
	face := ""
	speaker := ""
	voiceCode := ""
	for {
		for position < len(line) && (line[position] == ' ' || line[position] == '\t') {
			position++
		}
		if position >= len(line) {
			return "", "", "", position, false
		}
		if line[position] == bodyLead {
			return face, speaker, voiceCode, position, true
		}
		end := position
		for end < len(line) && line[end] != ' ' && line[end] != '\t' {
			end++
		}
		token := line[position:end]
		position = end
		if isBdtVoiceToken(token) {
			voiceCode = token[1:]
		} else if len(token) > 2 && token[0] == 'F' && token[1] == 'D' {
			face = token[2:]
		} else if len(token) > 0 && token[0] == '!' {
			speaker = token[1:]
		} else {
			return "", "", "", position, false
		}
	}
}

func isBdtVoiceToken(token string) bool {
	if len(token) < 2 || token[0] != 'V' {
		return false
	}
	for i := 1; i < len(token); i++ {
		if token[i] < '0' || token[i] > '9' {
			return false
		}
	}
	return true
}
