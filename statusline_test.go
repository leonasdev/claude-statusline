package main

import (
	"strings"
	"testing"
)

func TestParseInput(t *testing.T) {
	raw := `{
		"session_id": "abc123",
		"transcript_path": "/tmp/x.jsonl",
		"model": {"id": "claude-opus-4-7", "display_name": "Opus 4.7 (1M context)"},
		"workspace": {"current_dir": "/home/u/projects/x"},
		"context_window": {
			"used_percentage": 8.0,
			"current_usage": {
				"input_tokens": 1000,
				"output_tokens": 200,
				"cache_creation_input_tokens": 500,
				"cache_read_input_tokens": 2000
			}
		},
		"rate_limits": {
			"five_hour": {"used_percentage": 23.5, "resets_at": 1745683200}
		}
	}`

	in, err := parseInput(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseInput failed: %v", err)
	}
	if in.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want abc123", in.SessionID)
	}
	if in.Model.DisplayName != "Opus 4.7 (1M context)" {
		t.Errorf("Model.DisplayName = %q", in.Model.DisplayName)
	}
	if in.Workspace.CurrentDir != "/home/u/projects/x" {
		t.Errorf("Workspace.CurrentDir = %q", in.Workspace.CurrentDir)
	}
	if in.ContextWindow.UsedPercentage != 8.0 {
		t.Errorf("UsedPercentage = %v", in.ContextWindow.UsedPercentage)
	}
	if in.ContextWindow.CurrentUsage.CacheReadInputTokens != 2000 {
		t.Errorf("cache_read = %v", in.ContextWindow.CurrentUsage.CacheReadInputTokens)
	}
	if in.RateLimits.FiveHour.ResetsAt != 1745683200 {
		t.Errorf("ResetsAt = %v", in.RateLimits.FiveHour.ResetsAt)
	}
}

func TestColorize(t *testing.T) {
	got := colorize("hello", colorLightYellow)
	want := "\x1b[93mhello\x1b[0m"
	if got != want {
		t.Errorf("colorize = %q, want %q", got, want)
	}
}

func TestColorizeEmpty(t *testing.T) {
	if colorize("", colorLightYellow) != "" {
		t.Error("colorize of empty should be empty")
	}
}

func TestColorizeNoColor(t *testing.T) {
	if got := colorize("hi", ""); got != "hi" {
		t.Errorf("colorize with empty color = %q, want hi", got)
	}
}

func TestAbbreviatePath(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		level int
		want  string
	}{
		{"l0 full", "~/projects/test-projects/project-1", 0, "~/projects/test-projects/project-1"},
		{"l1 first parent abbrev", "~/projects/test-projects/project-1", 1, "~/p/test-projects/project-1"},
		{"l2 fish style", "~/projects/test-projects/project-1", 2, "~/p/t/project-1"},
		{"l3 ellipsis parents", "~/projects/test-projects/project-1", 3, "…/project-1"},
		{"l4 truncate leaf", "~/projects/test-projects/project-1", 4, "…1"},
		{"home only l0", "~", 0, "~"},
		{"home only l2", "~", 2, "~"},
		{"single segment l2", "/etc", 2, "/etc"},
		{"two segments l2", "/etc/nginx", 2, "/e/nginx"},
		{"empty", "", 0, "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := abbreviatePath(tt.path, tt.level)
			if got != tt.want {
				t.Errorf("abbreviatePath(%q, %d) = %q, want %q", tt.path, tt.level, got, tt.want)
			}
		})
	}
}

func TestSubstituteHome(t *testing.T) {
	tests := []struct {
		path, home, want string
	}{
		{"/home/u/projects/x", "/home/u", "~/projects/x"},
		{"/home/u", "/home/u", "~"},
		{"/etc/nginx", "/home/u", "/etc/nginx"},
		{"/home/user2/x", "/home/u", "/home/user2/x"},
		{"", "/home/u", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := substituteHome(tt.path, tt.home); got != tt.want {
				t.Errorf("substituteHome(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
			}
		})
	}
}
