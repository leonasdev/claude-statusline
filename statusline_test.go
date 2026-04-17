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
