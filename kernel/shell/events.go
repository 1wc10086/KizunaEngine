package shell

import (
	"encoding/json"

	"kizuna/kernel/translator"
)

func (t *task) append(ev translator.Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.mu.Lock()
	t.events = append(t.events, json.RawMessage(data))
	t.mu.Unlock()
}

func logEvent(level, message string) translator.Event {
	return translator.Event{Type: "log", Level: level, Message: message}
}

func errorEvent(message string) translator.Event {
	return translator.Event{Type: "error", Message: message}
}
