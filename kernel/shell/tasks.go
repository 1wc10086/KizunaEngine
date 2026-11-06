package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"kizuna/kernel/translator"
)

type task struct {
	mu         sync.Mutex
	events     []json.RawMessage
	done       bool
	finishedAt time.Time
	cancel     context.CancelFunc
}

type taskCenter struct {
	mu    sync.Mutex
	next  int64
	tasks map[int64]*task
}

var center = &taskCenter{tasks: make(map[int64]*task)}

func startTask(req translator.Request) int64 {
	center.mu.Lock()
	center.next++
	id := center.next
	ctx, cancel := context.WithCancel(context.Background())
	t := &task{cancel: cancel}
	center.tasks[id] = t
	center.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.append(errorEvent(fmt.Sprint(r)))
			}
			t.mu.Lock()
			t.done = true
			t.finishedAt = time.Now()
			t.mu.Unlock()
		}()
		emit := func(ev translator.Event) {
			t.append(ev)
		}
		if err := TranslationRunner(ctx, req, emit); err != nil {
			t.append(errorEvent(err.Error()))
		}
	}()
	return id
}

func pollTask(id int64) (bool, []json.RawMessage, bool) {
	center.mu.Lock()
	t, ok := center.tasks[id]
	if ok {
		sweepLocked()
	}
	center.mu.Unlock()
	if !ok {
		return false, nil, false
	}
	t.mu.Lock()
	evs := make([]json.RawMessage, 0, len(t.events))
	evs = append(evs, t.events...)
	t.events = nil
	done := t.done
	t.mu.Unlock()
	return done, evs, true
}

func cancelTask(id int64) bool {
	center.mu.Lock()
	t, ok := center.tasks[id]
	center.mu.Unlock()
	if !ok {
		return false
	}
	t.mu.Lock()
	done := t.done
	t.mu.Unlock()
	if done {
		return false
	}
	t.cancel()
	t.append(logEvent("warn", "已取消"))
	return true
}

func sweepLocked() {
	now := time.Now()
	for id, t := range center.tasks {
		t.mu.Lock()
		expired := t.done && now.Sub(t.finishedAt) > 5*time.Minute
		t.mu.Unlock()
		if expired {
			delete(center.tasks, id)
		}
	}
}
