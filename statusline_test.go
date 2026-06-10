package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseInput(t *testing.T) {
	raw := `{
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
		},
		"effort": {"level": "max"},
		"version": "2.1.119"
	}`

	in, err := parseInput(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseInput failed: %v", err)
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
	if in.Effort.Level != "max" {
		t.Errorf("Effort.Level = %q, want max", in.Effort.Level)
	}
	if in.Version != "2.1.119" {
		t.Errorf("Version = %q, want 2.1.119", in.Version)
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
		{-5, "░░░░░░░░░░░░░░░░"},  // clamp
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
		{0, "0s"},
		{1, "1s"},
		{29, "29s"},
		{59, "59s"},
		{60, "1m00s"},
		{61, "1m01s"},
		{90, "1m30s"},
		{60 * 48, "48m00s"},
		{60*48 + 15, "48m15s"},
		{60 * 60, "1h00m00s"},
		{60*60 + 5, "1h00m05s"},
		{60*60 + 60*30, "1h30m00s"},
		{60*60 + 60*30 + 15, "1h30m15s"},
		{60 * 60 * 23, "23h00m00s"},
		{60 * 60 * 24, "1d"},
		{60*60*24 + 60*60*5, "1d 5h"},
		{60*60*24*3 + 60*60*2, "3d 2h"},
		{-100, "0s"},
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
		{"ultracode", "❖ ultracode"},
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

func TestLastEffortChange(t *testing.T) {
	marker := func(level, scope string) string {
		return `{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set effort level to ` +
			level + ` (` + scope + `): description</local-command-stdout>"}}`
	}
	tests := []struct {
		name string
		data string
		want string
	}{
		{"ultracode entry", marker("ultracode", "this session only"), "ultracode"},
		{"last entry wins", marker("ultracode", "this session only") + "\n" + marker("xhigh", "saved as your default for new sessions"), "xhigh"},
		{"ultracode after xhigh", marker("xhigh", "saved") + "\n" + marker("ultracode", "this session only"), "ultracode"},
		{"no marker", `{"type":"user","message":{"role":"user","content":"hello"}}`, ""},
		{"empty input", "", ""},
		{"quoted in tool result is array content", `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"<local-command-stdout>Set effort level to ultracode (this session only)</local-command-stdout>"}]}}`, ""},
		{"marker mid-string is not a match", `{"type":"user","message":{"role":"user","content":"see <local-command-stdout>Set effort level to ultracode (x)</local-command-stdout>"}}`, ""},
		{"non-user type skipped", `{"type":"assistant","message":{"role":"assistant","content":"<local-command-stdout>Set effort level to ultracode (x)</local-command-stdout>"}}`, ""},
		{"malformed json skipped", `not json <local-command-stdout>Set effort level to ultracode`, ""},
		{"truncated first line skipped, later entry found", `ent level to max (x)</local-command-stdout>"}}` + "\n" + marker("ultracode", "this session only"), "ultracode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := lastEffortChange([]byte(tt.data)); got != tt.want {
				t.Errorf("lastEffortChange = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLastEffortChangeTimestamp(t *testing.T) {
	line := `{"type":"user","timestamp":"2026-06-10T04:15:49.297Z","message":{"role":"user","content":"<local-command-stdout>Set effort level to ultracode (this session only): desc</local-command-stdout>"}}`
	level, ts := lastEffortChange([]byte(line))
	if level != "ultracode" {
		t.Fatalf("level = %q, want ultracode", level)
	}
	want := time.Date(2026, 6, 10, 4, 15, 49, 297000000, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("ts = %v, want %v", ts, want)
	}

	noTS := `{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set effort level to ultracode (x): desc</local-command-stdout>"}}`
	if _, ts := lastEffortChange([]byte(noTS)); !ts.IsZero() {
		t.Errorf("missing timestamp should yield zero time, got %v", ts)
	}
}

func TestUltracodeMarkerStale(t *testing.T) {
	ts := time.Date(2026, 6, 10, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		ts      time.Time
		ccStart int64
		want    bool
	}{
		{"marker before process start", ts, ts.Unix() + 100, true},
		{"marker after process start", ts, ts.Unix() - 100, false},
		{"marker at process start", ts, ts.Unix(), false},
		{"unknown process start", ts, 0, false},
		{"zero timestamp", time.Time{}, ts.Unix(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ultracodeMarkerStale(tt.ts, tt.ccStart); got != tt.want {
				t.Errorf("ultracodeMarkerStale = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseStarttimeTicks(t *testing.T) {
	tests := []struct {
		name string
		stat string
		want int64
	}{
		{"simple comm", "123 (claude) S 1 123 123 0 -1 4194304 1 0 0 0 5 3 0 0 20 0 8 0 4567890 1000 1 18446744073709551615", 4567890},
		{"comm with space and parens", "6657 (tmux: (server)) S 1 6657 6657 0 -1 4194304 1 0 0 0 5 3 0 0 20 0 1 0 998877 1000 1 18446744073709551615", 998877},
		{"too few fields", "123 (x) S 1 2", 0},
		{"no parens", "garbage", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseStarttimeTicks(tt.stat); got != tt.want {
				t.Errorf("parseStarttimeTicks = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseBtime(t *testing.T) {
	procStat := "cpu  1 2 3 4\ncpu0 1 2 3 4\nbtime 1749500000\nprocesses 999\n"
	if got := parseBtime(procStat); got != 1749500000 {
		t.Errorf("parseBtime = %d, want 1749500000", got)
	}
	if got := parseBtime("cpu 1 2 3\n"); got != 0 {
		t.Errorf("parseBtime without btime = %d, want 0", got)
	}
}

func TestResolveEffortLevel(t *testing.T) {
	if got := resolveEffortLevel("max", "/nonexistent"); got != "max" {
		t.Errorf("non-xhigh level should pass through, got %q", got)
	}
	// Future CC versions may emit ultracode directly; it must pass through
	// without touching the transcript.
	if got := resolveEffortLevel("ultracode", "/nonexistent"); got != "ultracode" {
		t.Errorf("direct ultracode should pass through, got %q", got)
	}
	if got := resolveEffortLevel("xhigh", ""); got != "xhigh" {
		t.Errorf("empty transcript path should pass through, got %q", got)
	}
	if got := resolveEffortLevel("xhigh", "/nonexistent"); got != "xhigh" {
		t.Errorf("unreadable transcript should pass through, got %q", got)
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
		{0, "Ctx: 0%"},
		{4, "Ctx: 5%"}, // 4 / 0.8 = 5
		{8, "Ctx: 10%"},
		{40, "Ctx: 50%"},
		{80, "Ctx: 100%"},
		{4.48, "Ctx: 6%"}, // 4.48 / 0.8 = 5.6 → rounds to 6
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := contextDisplay(tt.usedPct); got != tt.want {
				t.Errorf("contextDisplay(%v) = %q, want %q", tt.usedPct, got, tt.want)
			}
		})
	}
}

func TestParsePorcelain(t *testing.T) {
	tests := []struct {
		name                         string
		raw                          string
		untracked, modified, deleted int
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
		{"main", 0, 0, 0, " main"},
		{"master", 3, 0, 0, " master ?3"},
		{"feat/x", 0, 8, 0, " feat/x ~8"},
		{"main", 0, 0, 2, " main -2"},
		{"main", 3, 8, 2, " main ?3 ~8 -2"},
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
	got := renderEffortSegment("max", "Opus 4.7 (1M)")
	want := colorLightBlack + "◈ max" + colorReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEffortSegmentHaikuHidden(t *testing.T) {
	if got := renderEffortSegment("max", "Haiku 4.5"); got != "" {
		t.Errorf("Haiku should hide effort, got %q", got)
	}
}

func TestRenderGitSegmentEmpty(t *testing.T) {
	if got := renderGitSegment("", 0, 0, 0); got != "" {
		t.Errorf("git with no branch should be empty, got %q", got)
	}
}

func TestRenderGitSegment(t *testing.T) {
	got := renderGitSegment("main", 3, 8, 2)
	want := colorLightYellow + " main ?3 ~8 -2" + colorReset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPctColor(t *testing.T) {
	tests := []struct {
		pct  float64
		want string
	}{
		{0, colorLightBlack},
		{50, colorLightBlack},
		{79.9, colorLightBlack},
		{80, colorLightYellow},
		{94.9, colorLightYellow},
		{95, colorLightRed},
		{100, colorLightRed},
		{120, colorLightRed},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.1f", tt.pct), func(t *testing.T) {
			if got := pctColor(tt.pct); got != tt.want {
				t.Errorf("pctColor(%v) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestRenderContextSegmentColors(t *testing.T) {
	// Label stays dim; only the value flips. Ctx is rescaled (used / 0.8), so
	// stdin 64 -> displayed 80% (yellow); stdin 76 -> 95% (red).
	tests := []struct {
		usedPct  float64
		valColor string
	}{
		{0, colorLightBlack},
		{40, colorLightBlack},
		{63.9, colorLightBlack},
		{64, colorLightYellow},
		{75.9, colorLightYellow},
		{76, colorLightRed},
		{80, colorLightRed},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.1f", tt.usedPct), func(t *testing.T) {
			got := renderContextSegment(tt.usedPct)
			labelWrapped := colorize("Ctx: ", colorLightBlack)
			if !strings.HasPrefix(got, labelWrapped) {
				t.Errorf("label not dim: %q", got)
			}
			valueWrapped := colorize(fmt.Sprintf("%.0f%%", contextPct(tt.usedPct)), tt.valColor)
			if !strings.Contains(got, valueWrapped) {
				t.Errorf("renderContextSegment(%v) = %q, missing value %q", tt.usedPct, got, valueWrapped)
			}
		})
	}
}

func TestRenderSessionSegmentColors(t *testing.T) {
	const resetsAt = int64(1_700_000_000)
	tests := []struct {
		pct      float64
		valColor string
	}{
		{0, colorLightBlack},
		{79.9, colorLightBlack},
		{80, colorLightYellow},
		{94.9, colorLightYellow},
		{95, colorLightRed},
		{100, colorLightRed},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.1f", tt.pct), func(t *testing.T) {
			got := renderSessionSegment(tt.pct, resetsAt)
			if !strings.HasPrefix(got, colorLightBlack) {
				t.Errorf("label not dim: %q", got)
			}
			valueWrapped := colorize(fmt.Sprintf("%.1f%%", tt.pct), tt.valColor)
			if !strings.Contains(got, valueWrapped) {
				t.Errorf("renderSessionSegment(%v) = %q, missing value %q", tt.pct, got, valueWrapped)
			}
		})
	}
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{colorize("abc", colorLightBlack), 3},
		{colorize("abc", colorLightYellow) + colorize("de", colorLightRed), 5},
		{"a" + colorReset + "b", 2},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := visibleWidth(tt.in); got != tt.want {
				t.Errorf("visibleWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestAlignRight(t *testing.T) {
	left := "abc"
	right := "ver"
	tests := []struct {
		name       string
		totalWidth int
		want       string
	}{
		{"unknown width", 0, "abc ver"},
		{"exact fit", 6, "abcver"},
		{"too narrow", 5, "abc ver"},
		{"with padding", 10, "abc    ver"},
		{"no right", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := right
			if tt.name == "no right" {
				r = ""
				tt.want = left
			}
			if got := alignRight(left, r, tt.totalWidth); got != tt.want {
				t.Errorf("alignRight(%q, %q, %d) = %q, want %q", left, r, tt.totalWidth, got, tt.want)
			}
		})
	}
}

func TestAlignRightStripsAnsiForWidth(t *testing.T) {
	// Visible width of "abc" wrapped in dim is 3, not 8+.
	left := colorize("abc", colorLightBlack)
	right := colorize("ver", colorLightBlack)
	got := alignRight(left, right, 10)
	// 10 - 3 - 3 = 4 padding spaces between them.
	want := left + "    " + right
	if got != want {
		t.Errorf("alignRight stripped width wrong: got %q, want %q", got, want)
	}
}

func TestRenderVersionSegment(t *testing.T) {
	if got := renderVersionSegment(""); got != "" {
		t.Errorf("empty version should hide segment, got %q", got)
	}
	got := renderVersionSegment("2.1.119")
	want := colorize("v2.1.119", colorLightBlack)
	if got != want {
		t.Errorf("renderVersionSegment = %q, want %q", got, want)
	}
}

func TestRenderSessionSegmentPlaceholderStaysDim(t *testing.T) {
	// resetsAt == 0 means data not populated yet; stay dim regardless of pct.
	got := renderSessionSegment(99, 0)
	if !strings.HasPrefix(got, colorLightBlack) {
		t.Errorf("placeholder prefix = %q, want %q", got, colorLightBlack)
	}
	if strings.Contains(got, colorLightRed) || strings.Contains(got, colorLightYellow) {
		t.Errorf("placeholder leaked threshold color: %q", got)
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
