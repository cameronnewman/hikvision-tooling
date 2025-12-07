package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.SugaredLogger
type Logger struct {
	*zap.SugaredLogger
}

// New creates a new Logger with the specified debug level
func New(debug bool) *Logger {
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return &Logger{logger.Sugar()}
}

// NewNop creates a no-op logger for testing
func NewNop() *Logger {
	return &Logger{zap.NewNop().Sugar()}
}

// NewFromZap creates a Logger from an existing zap.SugaredLogger
func NewFromZap(sugar *zap.SugaredLogger) *Logger {
	return &Logger{sugar}
}

// With returns a logger with the specified key-value pairs
func (l *Logger) With(args ...interface{}) *Logger {
	return &Logger{l.SugaredLogger.With(args...)}
}

// Sync flushes any buffered log entries
func (l *Logger) Sync() error {
	return l.SugaredLogger.Sync()
}
