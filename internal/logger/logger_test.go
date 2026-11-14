package logger

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input     string
		wantLevel slog.Level
		wantValid bool
	}{
		{"debug", slog.LevelDebug, true},
		{"DEBUG", slog.LevelDebug, true},
		{" debug ", slog.LevelDebug, true},
		{"info", slog.LevelInfo, true},
		{"INFO", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"WARN", slog.LevelWarn, true},
		{"warning", slog.LevelWarn, true},
		{"WARNING", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"ERROR", slog.LevelError, true},
		{"unknown", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
	}

	for _, tc := range tests {
		gotLevel, gotValid := ParseLevel(tc.input)
		if gotLevel != tc.wantLevel {
			t.Errorf("ParseLevel(%q) got level %v, want %v", tc.input, gotLevel, tc.wantLevel)
		}
		if gotValid != tc.wantValid {
			t.Errorf("ParseLevel(%q) got valid %v, want %v", tc.input, gotValid, tc.wantValid)
		}
	}
}

func TestInitAndSetLevel(t *testing.T) {
	l := Init("debug")
	if l == nil {
		t.Fatal("expected non-nil logger from Init")
	}
	if GetLevel() != slog.LevelDebug {
		t.Errorf("expected level DEBUG, got %v", GetLevel())
	}

	SetLevel("warn")
	if GetLevel() != slog.LevelWarn {
		t.Errorf("expected level WARN, got %v", GetLevel())
	}

	modLogger := ForModule("test")
	if modLogger == nil {
		t.Fatal("expected non-nil module logger")
	}
}
