package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	currentLevelVar slog.LevelVar
	initOnce        sync.Once
)

// ParseLevel converts a level string into a slog.Level.
// Supported values (case-insensitive): "debug", "info", "warn", "warning", "error".
// If the string is unrecognized, it defaults to slog.LevelInfo and returns false.
func ParseLevel(lvl string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// Init initializes the global default slog logger with the specified log level.
// Standard library log package calls will automatically flow through this logger at INFO level.
func Init(levelStr string) *slog.Logger {
	level, valid := ParseLevel(levelStr)
	currentLevelVar.Set(level)

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: &currentLevelVar,
	})
	l := slog.New(handler)
	slog.SetDefault(l)

	if !valid && strings.TrimSpace(levelStr) != "" {
		l.Warn("Unknown log level specified, falling back to default 'info'", "provided", levelStr)
	}

	return l
}

// SetLevel dynamically changes the logging level at runtime.
func SetLevel(levelStr string) {
	level, _ := ParseLevel(levelStr)
	currentLevelVar.Set(level)
}

// GetLevel returns the currently configured logging level.
func GetLevel() slog.Level {
	return currentLevelVar.Level()
}

// ForModule returns a logger with the module attribute attached.
func ForModule(name string) *slog.Logger {
	return slog.Default().With("module", name)
}
