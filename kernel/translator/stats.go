package translator

import (
	"sync"
	"time"
)

type runStats struct {
	mu               sync.Mutex
	requests         int
	failed           int
	promptTokens     int
	completionTokens int
	doneUnits        int
	start            time.Time
}

func newRunStats() *runStats {
	return &runStats{start: time.Now()}
}

func (s *runStats) addRequest(promptTokens int, completionTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	s.promptTokens += promptTokens
	s.completionTokens += completionTokens
}

func (s *runStats) addFailed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	s.failed++
}

func (s *runStats) addDoneUnits(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doneUnits += n
}

func (s *runStats) snapshot() (int, int, int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, s.failed, s.promptTokens, s.completionTokens, s.doneUnits
}

func (s *runStats) event() Event {
	requests, failed, promptTokens, completionTokens, doneUnits := s.snapshot()
	elapsed := time.Since(s.start)
	elapsedMs := elapsed.Milliseconds()
	var ups float64
	if secs := elapsed.Seconds(); secs > 0 {
		ups = float64(doneUnits) / secs
	}
	return Event{
		Type:             "stats",
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Requests:         requests,
		Failed:           failed,
		ElapsedMs:        elapsedMs,
		UnitsPerSecond:   ups,
	}
}
