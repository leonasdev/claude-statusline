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

// realEffortLine returns the byte sequence as it appears in a real CC
// transcript JSONL entry for a /effort output (verified against actual file).
func realEffortLine(level string) string {
	return `{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set effort level to ` + level + `: ...</local-command-stdout>"}}`
}

func realModelSetLine(model, level string) string {
	return `{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set model to ` + model + ` with ` + level + ` effort</local-command-stdout>"}}`
}

func realKeptModelLine(model string) string {
	return `{"type":"user","message":{"role":"user","content":"<local-command-stdout>Kept model as ` + model + `</local-command-stdout>"}}`
}

func TestExtractEffort(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"xhigh", realEffortLine("xhigh"), "xhigh"},
		{"max with parens", `{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set effort level to max (this session only): max blah</local-command-stdout>"}}`, "max"},
		{"random", "random other line", ""},
		{"high", realEffortLine("high"), "high"},
		// FALSE POSITIVE GUARDS — patterns appearing in prose (escaped quotes) must NOT match
		{"prose escaped", `the spec says \"content\":\"<local-command-stdout>Set effort level to xhigh: ...\"}`, ""},
		{"prose plain", "in the spec we say `Set effort level to xhigh:` is the pattern", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLatestEffort(tt.in); got != tt.want {
				t.Errorf("extractLatestEffort(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractEffortLatestWins(t *testing.T) {
	in := realEffortLine("low") + "\n" + realEffortLine("xhigh")
	if got := extractLatestEffort(in); got != "xhigh" {
		t.Errorf("latest = %q, want xhigh", got)
	}
}

func TestExtractEffortAutoFormat(t *testing.T) {
	// Real CC output for `/effort auto` — different word order, no description
	in := `{"content":"<local-command-stdout>Effort level set to auto</local-command-stdout>"}`
	if got := extractLatestEffort(in); got != "auto" {
		t.Errorf("got %q, want auto", got)
	}
}

func TestExtractEffortAutoLatestOverridesOldFormat(t *testing.T) {
	// User runs /effort low, then /effort auto. Auto should win.
	in := realEffortLine("low") + "\n" +
		`{"content":"<local-command-stdout>Effort level set to auto</local-command-stdout>"}`
	if got := extractLatestEffort(in); got != "auto" {
		t.Errorf("got %q, want auto (latest)", got)
	}
}

func TestExtractModelAndEffort(t *testing.T) {
	in := realModelSetLine("Opus 4.7 (1M context) (default)", "xhigh")
	model, effort := extractLatestModelSet(in)
	if model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("model = %q", model)
	}
	if effort != "xhigh" {
		t.Errorf("effort = %q", effort)
	}
}

func TestExtractModelVariants(t *testing.T) {
	tests := []struct {
		name, line, wantModel, wantEffort string
	}{
		{
			name:       "bare model",
			line:       `{"content":"<local-command-stdout>Set model to Opus 4.7 (1M context) (default)</local-command-stdout>"}`,
			wantModel:  "Opus 4.7 (1M context) (default)",
			wantEffort: "",
		},
		{
			name:       "model with effort",
			line:       `{"content":"<local-command-stdout>Set model to Opus 4.7 (1M context) with xhigh effort</local-command-stdout>"}`,
			wantModel:  "Opus 4.7 (1M context)",
			wantEffort: "xhigh",
		},
		{
			name:       "model with billing trailer (no effort)",
			line:       `{"content":"<local-command-stdout>Set model to Sonnet 4.6 (1M context) · Billed as extra usage</local-command-stdout>"}`,
			wantModel:  "Sonnet 4.6 (1M context)",
			wantEffort: "",
		},
		{
			name:       "model with effort and billing trailer",
			line:       `{"content":"<local-command-stdout>Set model to Sonnet 4.6 (1M context) with max effort · Billed as extra usage</local-command-stdout>"}`,
			wantModel:  "Sonnet 4.6 (1M context)",
			wantEffort: "max",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, effort := extractLatestModelSet(tt.line)
			if model != tt.wantModel || effort != tt.wantEffort {
				t.Errorf("got (model=%q, effort=%q), want (model=%q, effort=%q)",
					model, effort, tt.wantModel, tt.wantEffort)
			}
		})
	}
}

func TestExtractLatestState(t *testing.T) {
	// /effort sets max, then /model with low effort, then /model with high effort.
	// Effort must track /model's embedded effort (the most recent event),
	// not stay stuck on the older /effort line.
	in := realEffortLine("max") + "\n" +
		realModelSetLine("Opus 4.7 (1M context) (default)", "low") + "\n" +
		realModelSetLine("Opus 4.7 (1M context) (default)", "high")
	effort, model := extractLatestState(in)
	if effort != "high" {
		t.Errorf("effort = %q, want high (latest embedded in /model)", effort)
	}
	if model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("model = %q", model)
	}
}

func TestExtractLatestStateBareModelKeepsEffort(t *testing.T) {
	// /model with xhigh effort sets both. Later bare /model only updates model,
	// effort should persist.
	in := realModelSetLine("Opus 4.7 (1M context) (default)", "xhigh") + "\n" +
		`{"content":"<local-command-stdout>Set model to Sonnet 4.6 (1M context) · Billed as extra usage</local-command-stdout>"}`
	effort, model := extractLatestState(in)
	if effort != "xhigh" {
		t.Errorf("effort = %q, want xhigh (bare /model doesn't clear effort)", effort)
	}
	if model != "Sonnet 4.6 (1M context)" {
		t.Errorf("model = %q, want Sonnet", model)
	}
}

func TestExtractLatestStateKeptModelLatest(t *testing.T) {
	// Set model sets one, later Kept model as different one — Kept should win
	// if it's the latest by position (confirms current model).
	in := realModelSetLine("Sonnet 4.6 (1M context)", "max") + "\n" +
		realKeptModelLine("Opus 4.7 (1M context) (default)")
	_, model := extractLatestState(in)
	if model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("model = %q, want Opus (latest Kept)", model)
	}
}

func TestExtractModelLatestVariantWins(t *testing.T) {
	// Simulates real session: Sonnet+max → bare Opus. Latest (bare Opus) must win.
	in := `{"content":"<local-command-stdout>Set model to Sonnet 4.6 (1M context) with max effort · Billed as extra usage</local-command-stdout>"}` +
		"\n" +
		`{"content":"<local-command-stdout>Set model to Opus 4.7 (1M context) (default)</local-command-stdout>"}`
	model, effort := extractLatestModelSet(in)
	if model != "Opus 4.7 (1M context) (default)" {
		t.Errorf("latest model = %q, want Opus", model)
	}
	if effort != "" {
		t.Errorf("latest effort should be empty (bare line), got %q", effort)
	}
}

func TestExtractModelFalsePositive(t *testing.T) {
	// Prose containing the phrase wrapped in escaped quotes must NOT match
	in := `the regex \"content\":\"<local-command-stdout>Set model to FAKE with bad effort\" was here`
	model, effort := extractLatestModelSet(in)
	if model != "" || effort != "" {
		t.Errorf("false positive: model=%q effort=%q (should be empty)", model, effort)
	}
}

func TestExtractKeptModel(t *testing.T) {
	in := realKeptModelLine("Opus 4.7 (1M context) (default)")
	if got := extractLatestKeptModel(in); got != "Opus 4.7 (1M context) (default)" {
		t.Errorf("kept = %q", got)
	}
}

func TestExtractKeptModelFalsePositive(t *testing.T) {
	// Escaped-quote prose must NOT match
	in := `the \"content\":\"<local-command-stdout>Kept model as FAKE</local-command-stdout>\" pattern`
	if got := extractLatestKeptModel(in); got != "" {
		t.Errorf("false positive: %q (should be empty)", got)
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

func TestParsePorcelain(t *testing.T) {
	tests := []struct {
		name                          string
		raw                           string
		untracked, modified, deleted  int
	}{
		{"empty", "", 0, 0, 0},
		{"untracked only", "?? a.txt\n?? b.txt\n", 2, 0, 0},
		{"modified", " M a.txt\nM  b.txt\n", 0, 2, 0},
		{"deleted", " D a.txt\nD  b.txt\n", 0, 0, 2},
		{"mixed", "?? a\n M b\nD  c\n", 1, 1, 1},
		{"MM same file", "MM a\n", 0, 1, 0},
		{"renamed (ignored)", "R  a -> b\n", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, m, d := parsePorcelain(tt.raw)
			if u != tt.untracked || m != tt.modified || d != tt.deleted {
				t.Errorf("parsePorcelain(%q) = (u=%d m=%d d=%d), want (u=%d m=%d d=%d)",
					tt.raw, u, m, d, tt.untracked, tt.modified, tt.deleted)
			}
		})
	}
}

func TestFormatGit(t *testing.T) {
	tests := []struct {
		branch                       string
		untracked, modified, deleted int
		want                         string
	}{
		{"main", 0, 0, 0, "\ue725 main"},
		{"master", 3, 0, 0, "\ue725 master ?3"},
		{"feat/x", 0, 8, 0, "\ue725 feat/x ~8"},
		{"main", 0, 0, 2, "\ue725 main -2"},
		{"main", 3, 8, 2, "\ue725 main ?3 ~8 -2"},
		{"", 0, 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatGit(tt.branch, tt.untracked, tt.modified, tt.deleted)
			if got != tt.want {
				t.Errorf("formatGit = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFitPath(t *testing.T) {
	tests := []struct {
		name, path, want string
	}{
		// pathThreshold = 40
		{"short stays full", "~/short", "~/short"},
		{"exactly threshold (40 chars) stays full", "~/aaaaaaaaaa/bbbbbbbbbb/cccccccc/dddd", "~/aaaaaaaaaa/bbbbbbbbbb/cccccccc/dddd"},
		{"over threshold abbreviates first parent", "~/projects/test-projects/cool-project-name", "~/p/test-projects/cool-project-name"},
		{"long enough to need fish-style", "~/very/long/path/to/some/deeply/nested/source/file", "~/v/l/p/t/s/d/n/s/file"},
		{"empty stays sentinel", "", "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fitPath(tt.path)
			if got != tt.want {
				t.Errorf("fitPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRenderModelSegment(t *testing.T) {
	got := renderModelSegment("Opus 4.7 (1M context) (default)")
	want := colorClaudeBold + "Opus 4.7 (1M)" + colorReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEffortSegment(t *testing.T) {
	got := renderEffortSegment("xhigh", "Opus 4.7 (1M)")
	want := colorLightBlack + "◉ xhigh" + colorReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEffortSegmentHaikuHidden(t *testing.T) {
	if got := renderEffortSegment("xhigh", "Haiku 4.5"); got != "" {
		t.Errorf("Haiku should hide effort, got %q", got)
	}
}

func TestRenderEffortSegmentAutoOpus(t *testing.T) {
	got := renderEffortSegment("auto", "Opus 4.7 (1M)")
	want := colorLightBlack + "◉ xhigh" + colorReset
	if got != want {
		t.Errorf("auto on Opus = %q, want %q", got, want)
	}
}

func TestRenderEffortSegmentAutoSonnet(t *testing.T) {
	got := renderEffortSegment("auto", "Sonnet 4.6 (1M)")
	want := colorLightBlack + "◐ medium" + colorReset
	if got != want {
		t.Errorf("auto on Sonnet = %q, want %q", got, want)
	}
}

func TestRenderEffortSegmentAutoUnknown(t *testing.T) {
	// Unknown model defaults to "high" (safe fallback)
	got := renderEffortSegment("auto", "FutureModel 5.0")
	want := colorLightBlack + "● high" + colorReset
	if got != want {
		t.Errorf("auto unknown = %q, want %q", got, want)
	}
}

func TestRenderGitSegmentEmpty(t *testing.T) {
	if got := renderGitSegment("", 0, 0, 0); got != "" {
		t.Errorf("git with no branch should be empty, got %q", got)
	}
}

func TestRenderGitSegment(t *testing.T) {
	got := renderGitSegment("main", 3, 8, 2)
	want := colorLightYellow + "\ue725 main ?3 ~8 -2" + colorReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJoinSegments(t *testing.T) {
	sep := colorize(" │ ", colorLightBlack)
	tests := []struct {
		name     string
		segments []string
		want     string
	}{
		{"all empty", []string{}, ""},
		{"one", []string{"a"}, "a"},
		{"two", []string{"a", "b"}, "a" + sep + "b"},
		{"with empty middle", []string{"a", "", "c"}, "a" + sep + "c"},
		{"with empty start", []string{"", "b"}, "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinSegments(tt.segments); got != tt.want {
				t.Errorf("joinSegments = %q, want %q", got, tt.want)
			}
		})
	}
}
