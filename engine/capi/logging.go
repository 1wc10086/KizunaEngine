package capi

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"kizuna/engine/core"
)

const (
	logClock       = "15:04:05.000"
	maxPendingLogs = 512

	// logBufferSize 把成百上千行日志压成一次 write(2)：Android 上 fmt.Fprintln 落文件
	// 每次都是一次系统调用，日志高峰会让引擎 op 的关键路径排在 I/O 后面。
	logBufferSize = 64 << 10
	// logFlushInterval 是缓冲刷盘周期：崩溃时最多丢 1 秒尾部日志，排查卡顿足够；
	// 再短就退化成"每条日志一次系统调用"，再长丢的尾部日志会盖过现场。
	logFlushInterval = time.Second
)

// stderrEcho 默认关闭：Android 上 stderr 同样是一次 write(2)，且生产环境没人看；
// 只有本地调试才用 KIZUNA_ENGINE_STDERR 打开回显。
var stderrEcho = os.Getenv("KIZUNA_ENGINE_STDERR") != ""

type engineFileLogger struct {
	mu      sync.Mutex
	file    *os.File
	bw      *bufio.Writer
	pending []string
	stop    chan struct{}
}

var (
	loggerMu      sync.Mutex
	activeLogFile *engineFileLogger
)

func newEngineFileLogger(path string) *engineFileLogger {
	l := &engineFileLogger{}
	if path == "" {
		return l
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return l
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return l
	}
	l.file = f
	l.bw = bufio.NewWriterSize(f, logBufferSize)
	l.stop = make(chan struct{})
	go l.flushLoop(l.stop)
	return l
}

// flushLoop 周期把缓冲刷进文件。没有它，缓冲里的日志会在进程被系统杀掉时整段丢失；
// 有了它，最坏情况也只是丢掉最后一秒。
func (l *engineFileLogger) flushLoop(stop <-chan struct{}) {
	t := time.NewTicker(logFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.mu.Lock()
			if l.bw != nil {
				_ = l.bw.Flush()
			}
			l.mu.Unlock()
		case <-stop:
			return
		}
	}
}

func installLogger(path string) {
	next := newEngineFileLogger(path)
	loggerMu.Lock()
	prev := activeLogFile
	activeLogFile = next
	loggerMu.Unlock()
	core.SetLogger(next)
	if prev == nil {
		return
	}
	prev.replay(next)
	_ = prev.Close()
}

func (l *engineFileLogger) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	defer func() { _ = recover() }()
	line := time.Now().Format(logClock) + " " + fmt.Sprintf(format, args...)
	if stderrEcho {
		fmt.Fprintln(os.Stderr, line)
	}
	l.write(line)
}

func (l *engineFileLogger) write(line string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.bw != nil {
		_, _ = fmt.Fprintln(l.bw, line)
		return
	}
	if len(l.pending) < maxPendingLogs {
		l.pending = append(l.pending, line)
	}
}

func (l *engineFileLogger) replay(next *engineFileLogger) {
	if l == nil || next == nil {
		return
	}
	l.mu.Lock()
	lines := l.pending
	l.pending = nil
	l.mu.Unlock()
	for _, line := range lines {
		next.write(line)
	}
}

func (l *engineFileLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// 先停 flush goroutine，避免它在 file 关闭后还去 Flush 已失效的缓冲。
	if l.stop != nil {
		close(l.stop)
		l.stop = nil
	}
	if l.file == nil {
		return nil
	}
	var err error
	if l.bw != nil {
		err = l.bw.Flush()
		l.bw = nil
	}
	if cerr := l.file.Close(); err == nil {
		err = cerr
	}
	l.file = nil
	return err
}
