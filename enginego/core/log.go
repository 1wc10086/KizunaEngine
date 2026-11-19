package core

type Logger interface {
	Printf(format string, args ...any)
}

type discardLogger struct{}

func (discardLogger) Printf(format string, args ...any) {}

var activeLogger Logger = discardLogger{}

func SetLogger(l Logger) {
	if l == nil {
		activeLogger = discardLogger{}
		return
	}
	activeLogger = l
}

func Logf(format string, args ...any) {
	activeLogger.Printf(format, args...)
}
