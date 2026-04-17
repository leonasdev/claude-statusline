package main

import (
	"os"
	"path/filepath"
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

func TestRenderBar(t *testing.T) {
	tests := []struct {
		pct  float64
		want string
	}{
		{0, "░░░░░░░░░░░░░░░░"},
		{6.25, "█░░░░░░░░░░░░░░░"},
		{50, "████████░░░░░░░░"},
		{100, "████████████████"},
		{150, "████████████████"}, // clamp
		{-5, "░░░░░░░░░░░░░░░░"}, // clamp
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := renderBar(tt.pct, 16); got != tt.want {
				t.Errorf("renderBar(%v, 16) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		secs int64
		want string
	}{
		{0, "0m"},
		{60, "1m"},
		{30, "0m"},
		{29, "0m"},
		{59, "0m"},
		{60 * 48, "48m"},
		{60 * 60, "1h"},
		{60*60 + 60*30, "1h30m"},
		{60 * 60 * 23, "23h"},
		{60 * 60 * 24, "1d"},
		{60*60*24 + 60*60*5, "1d 5h"},
		{60*60*24*3 + 60*60*2, "3d 2h"},
		{-100, "0m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatDuration(tt.secs); got != tt.want {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.secs, got, tt.want)
			}
		})
	}
}

func TestEffortDisplay(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"low", "○ low"},
		{"medium", "◐ medium"},
		{"high", "● high"},
		{"xhigh", "◉ xhigh"},
		{"max", "◈ max"},
		{"unknown", "● high"}, // default fallback
		{"", "● high"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := effortDisplay(tt.in); got != tt.want {
				t.Errorf("effortDisplay(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestModelDisplay(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Opus 4.7 (1M context) (default)", "Opus 4.7 (1M)"},
		{"Sonnet 4.6 (1M context)", "Sonnet 4.6 (1M)"},
		{"Sonnet 4.6 (1M context) (default)", "Sonnet 4.6 (1M)"},
		{"Sonnet 4.6", "Sonnet 4.6"},
		{"Haiku 4.5", "Haiku 4.5"},
		{"Opus 4.6", "Opus 4.6"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := modelDisplay(tt.in); got != tt.want {
				t.Errorf("modelDisplay(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestContextPercent(t *testing.T) {
	tests := []struct {
		usedPct float64
		want    string
	}{
		{0, "Ctx: 0.0%"},
		{4, "Ctx: 5.0%"},   // 4 / 0.8 = 5
		{8, "Ctx: 10.0%"},
		{40, "Ctx: 50.0%"},
		{80, "Ctx: 100.0%"},
		{4.48, "Ctx: 5.6%"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := contextDisplay(tt.usedPct); got != tt.want {
				t.Errorf("contextDisplay(%v) = %q, want %q", tt.usedPct, got, tt.want)
			}
		})
	}
}

func TestCacheHitDisplay(t *testing.T) {
	tests := []struct {
		input, read, creation int
		want                  string
	}{
		{1000, 0, 0, "Cache: 0%"},
		{1000, 3000, 0, "Cache: 75%"},
		{1000, 2000, 1000, "Cache: 50%"},
		{0, 0, 0, "Cache: -"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := cacheHitDisplay(tt.input, tt.read, tt.creation); got != tt.want {
				t.Errorf("cacheHitDisplay(%d, %d, %d) = %q, want %q",
					tt.input, tt.read, tt.creation, got, tt.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"raw ESC", "Set model to \x1b[1mOpus 4.7\x1b[22m effort", "Set model to Opus 4.7 effort"},
		// JSON-escaped form: real transcripts encode ESC as the 6-char literal `\u001b`
		{"JSON escaped", `Set model to \u001b[1mOpus 4.7\u001b[22m effort`, "Set model to Opus 4.7 effort"},
		{"mixed plain", "no escapes here", "no escapes here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in); got != tt.want {
				t.Errorf("stripANSI = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractEffort(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Set effort level to xhigh: deeper reasoning", "xhigh"},
		{"Set effort level to max (this session only): max blah", "max"},
		{"random other line", ""},
		{"Set effort level to high: some text", "high"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := extractLatestEffort(tt.in); got != tt.want {
				t.Errorf("extractLatestEffort(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractEffortLatestWins(t *testing.T) {
	in := "Set effort level to low: x\nSet effort level to xhigh: y"
	if got := extractLatestEffort(in); got != "xhigh" {
		t.Errorf("latest = %q, want xhigh", got)
	}
}

func TestExtractModelAndEffort(t *testing.T) {
	in := "Set model to Opus 4.7 (1M context) (default) with xhigh effort"
	model, effort := extractLatestModelSet(in)
	if model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("model = %q", model)
	}
	if effort != "xhigh" {
		t.Errorf("effort = %q", effort)
	}
}

func TestExtractKeptModel(t *testing.T) {
	in := "Kept model as Opus 4.7 (1M context) (default)</local-command-stdout>"
	if got := extractLatestKeptModel(in); got != "Opus 4.7 (1M context) (default)" {
		t.Errorf("kept = %q", got)
	}
}

func TestExtractKeptModelEOL(t *testing.T) {
	in := "Kept model as Sonnet 4.6"
	if got := extractLatestKeptModel(in); got != "Sonnet 4.6" {
		t.Errorf("kept = %q", got)
	}
}

func TestScanTranscriptEmpty(t *testing.T) {
	effort, model, err := scanTranscript("testdata/transcripts/empty.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if effort != "" || model != "" {
		t.Errorf("expected empty, got effort=%q model=%q", effort, model)
	}
}

func TestScanTranscriptSingleEffort(t *testing.T) {
	effort, model, err := scanTranscript("testdata/transcripts/single_effort.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if effort != "high" {
		t.Errorf("effort = %q, want high", effort)
	}
	if model != "" {
		t.Errorf("model = %q, want empty", model)
	}
}

func TestScanTranscriptMultiEffortLatestWins(t *testing.T) {
	effort, _, err := scanTranscript("testdata/transcripts/multi_effort.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if effort != "max" {
		t.Errorf("effort = %q, want max (latest)", effort)
	}
}

func TestScanTranscriptModelChange(t *testing.T) {
	effort, model, err := scanTranscript("testdata/transcripts/model_change.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	// Latest /effort line wins for effort
	if effort != "high" {
		t.Errorf("effort = %q, want high", effort)
	}
	// Only "Set model to" line provided model
	if model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("model = %q", model)
	}
}

func TestScanTranscriptMissingFile(t *testing.T) {
	_, _, err := scanTranscript("testdata/transcripts/does_not_exist.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCacheRoundtrip(t *testing.T) {
	dir := t.TempDir()
	c := &CacheEntry{
		Effort:            "xhigh",
		Model:             "Opus 4.7 (1M context) (default)",
		TranscriptMtimeNs: 1234567890,
		TranscriptSize:    9999,
	}
	path := filepath.Join(dir, "session.json")
	if err := saveCache(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := loadCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effort != "xhigh" || got.Model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if got.TranscriptMtimeNs != 1234567890 || got.TranscriptSize != 9999 {
		t.Errorf("mtime/size mismatch: %+v", got)
	}
}

func TestLoadCacheMissing(t *testing.T) {
	got, err := loadCache(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Errorf("expected nil error for missing, got %v", err)
	}
	if got == nil {
		t.Error("expected zero-value entry, got nil")
	}
}

func TestLoadCacheCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadCache(path)
	if err != nil {
		t.Errorf("expected nil error for corrupt, got %v", err)
	}
	if got == nil || got.Effort != "" {
		t.Errorf("expected empty entry, got %+v", got)
	}
}

func TestSaveCacheAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.json")
	c := &CacheEntry{Effort: "high"}
	if err := saveCache(path, c); err != nil {
		t.Fatal(err)
	}
	// .tmp file should not be left behind
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file should be cleaned up after rename, got: %v", err)
	}
}

func TestReadSettingsEffort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"effortLevel":"xhigh","other":"x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got := readSettingsEffort(path)
	if got != "xhigh" {
		t.Errorf("got %q, want xhigh", got)
	}
}

func TestReadSettingsEffortMissing(t *testing.T) {
	got := readSettingsEffort(filepath.Join(t.TempDir(), "nope.json"))
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestReadSettingsEffortNoField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"other":"x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if got := readSettingsEffort(path); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
