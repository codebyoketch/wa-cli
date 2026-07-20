package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"  debug  ", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"nonsense", slog.LevelInfo},
	}
	for _, c := range cases {
		if got := parseLevel(c.in); got != c.want {
			t.Errorf("parseLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNew_RespectsConfiguredLevel(t *testing.T) {
	log := New(Options{Level: "warn"})
	ctx := context.Background()

	if log.Enabled(ctx, slog.LevelInfo) {
		t.Error("logger built with Level: warn should not have Info enabled")
	}
	if !log.Enabled(ctx, slog.LevelWarn) {
		t.Error("logger built with Level: warn should have Warn enabled")
	}
}

func TestNew_DefaultsToInfo(t *testing.T) {
	log := New(Options{})
	ctx := context.Background()

	if !log.Enabled(ctx, slog.LevelInfo) {
		t.Error("logger built with no Level set should default to Info enabled")
	}
	if log.Enabled(ctx, slog.LevelDebug) {
		t.Error("logger built with no Level set should not have Debug enabled")
	}
}

func TestNew_NeverNil(t *testing.T) {
	if New(Options{Level: "debug", JSON: true}) == nil {
		t.Fatal("New returned nil")
	}
	if New(Options{Level: "garbage", JSON: false}) == nil {
		t.Fatal("New returned nil for an unrecognized level")
	}
}
