package logger

import (
	"testing"

	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name  string
		debug bool
	}{
		{
			name:  "production mode",
			debug: false,
		},
		{
			name:  "debug mode",
			debug: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := New(tt.debug)
			if log == nil {
				t.Fatal("New() returned nil")
			}
			if log.SugaredLogger == nil {
				t.Fatal("SugaredLogger is nil")
			}
			_ = log.Sync()
		})
	}
}

func TestNewNop(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "creates nop logger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := NewNop()
			if log == nil {
				t.Fatal("NewNop() returned nil")
			}
			if log.SugaredLogger == nil {
				t.Fatal("SugaredLogger is nil")
			}

			// Should not panic
			log.Info("test message")
			log.Debug("debug message")
			log.Warn("warn message")
			log.Error("error message")
			_ = log.Sync()
		})
	}
}

func TestNewFromZap(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "wraps zap sugared logger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sugar := zap.NewNop().Sugar()
			log := NewFromZap(sugar)
			if log == nil {
				t.Fatal("NewFromZap() returned nil")
			}
			if log.SugaredLogger != sugar {
				t.Error("SugaredLogger doesn't match input")
			}
		})
	}
}

func TestWith(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{
			name: "adds key-value pair",
			key:  "key",
			val:  "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := New(false)
			newLog := log.With(tt.key, tt.val)
			if newLog == nil {
				t.Fatal("With() returned nil")
			}
			if newLog == log {
				t.Error("With() should return new logger")
			}
			_ = newLog.Sync()
			_ = log.Sync()
		})
	}
}

func TestSync(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "sync does not panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := New(false)
			err := log.Sync()
			// Sync may return an error on some systems, but shouldn't panic
			_ = err
		})
	}
}

func TestLogMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "Info methods",
			method: "info",
		},
		{
			name:   "Debug methods",
			method: "debug",
		},
		{
			name:   "Warn methods",
			method: "warn",
		},
		{
			name:   "Error methods",
			method: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := NewNop()

			// Test various log methods don't panic
			switch tt.method {
			case "info":
				log.Info("info message")
				log.Infow("info with fields", "key", "value")
				log.Infof("info formatted %s", "message")
			case "debug":
				log.Debug("debug message")
				log.Debugw("debug with fields", "key", "value")
				log.Debugf("debug formatted %s", "message")
			case "warn":
				log.Warn("warn message")
				log.Warnw("warn with fields", "key", "value")
				log.Warnf("warn formatted %s", "message")
			case "error":
				log.Error("error message")
				log.Errorw("error with fields", "key", "value")
				log.Errorf("error formatted %s", "message")
			}
		})
	}
}
