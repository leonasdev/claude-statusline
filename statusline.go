package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ==== SECTION: INPUT TYPES ====

type Input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Model          struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
		ProjectDir string `json:"project_dir"`
	} `json:"workspace"`
	ContextWindow struct {
		UsedPercentage    float64 `json:"used_percentage"`
		ContextWindowSize int     `json:"context_window_size"`
		CurrentUsage      struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	RateLimits struct {
		FiveHour struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"five_hour"`
	} `json:"rate_limits"`
}

func parseInput(r io.Reader) (*Input, error) {
	var in Input
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return nil, err
	}
	return &in, nil
}

// ==== SECTION: COLORS ====

const (
	colorReset       = "\x1b[0m"
	colorLightYellow = "\x1b[93m"
	colorLightBlack  = "\x1b[90m"
	colorClaudeBold  = "\x1b[1;38;2;217;119;87m"
	colorNone        = ""
)

func colorize(s, color string) string {
	if s == "" {
		return ""
	}
	if color == "" {
		return s
	}
	return color + s + colorReset
}

// ==== SECTION: PATH ====

func substituteHome(path, home string) string {
	if path == "" || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

func abbreviatePath(path string, level int) string {
	if path == "" {
		return "?"
	}
	parts := strings.Split(path, "/")
	// "/foo" → ["", "foo"], "~/foo" → ["~", "foo"], "~" → ["~"]
	if len(parts) == 0 {
		return path
	}
	leaf := parts[len(parts)-1]
	if len(parts) == 1 {
		// no separators, e.g. "~" or "etc"
		switch level {
		case 4:
			if len(leaf) > 1 {
				return "…" + leaf[len(leaf)-1:]
			}
			return leaf
		default:
			return leaf
		}
	}

	parents := parts[:len(parts)-1] // may include empty first element for absolute paths

	abbrevSegment := func(s string) string {
		if s == "" {
			return ""
		}
		return s[:1]
	}

	switch level {
	case 0:
		return path
	case 1:
		if len(parents) >= 2 {
			// abbreviate the FIRST non-empty / non-anchor parent
			// anchor = "" (absolute) or "~" (home)
			// example: ~/projects/test-projects/project-1
			//   parents=["~", "projects", "test-projects"]
			//   abbreviate parents[1] only → ["~","p","test-projects"]
			out := make([]string, len(parents))
			copy(out, parents)
			abbrevIndex := -1
			for i, p := range parents {
				if p == "" || p == "~" {
					continue
				}
				abbrevIndex = i
				break
			}
			if abbrevIndex >= 0 {
				out[abbrevIndex] = abbrevSegment(parents[abbrevIndex])
			}
			return strings.Join(out, "/") + "/" + leaf
		}
		return path
	case 2:
		// fish-style: every parent (except anchor) → first char
		out := make([]string, len(parents))
		for i, p := range parents {
			if p == "" || p == "~" {
				out[i] = p
			} else {
				out[i] = abbrevSegment(p)
			}
		}
		return strings.Join(out, "/") + "/" + leaf
	case 3:
		return "…/" + leaf
	case 4:
		if len(leaf) > 1 {
			return "…" + leaf[len(leaf)-1:]
		}
		return "…" + leaf
	default:
		return path
	}
}

// ==== SECTION: BAR ====

func renderBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// ==== SECTION: DURATION ====

func formatDuration(secs int64) string {
	if secs < 0 {
		secs = 0
	}
	mins := secs / 60
	if mins >= 60*24 {
		days := mins / (60 * 24)
		hours := (mins % (60 * 24)) / 60
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if mins >= 60 {
		hours := mins / 60
		m := mins % 60
		if m == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, m)
	}
	return fmt.Sprintf("%dm", mins)
}

// ==== SECTION: EFFORT/MODEL DISPLAY ====

var effortIcons = map[string]string{
	"low":    "○ low",
	"medium": "◐ medium",
	"high":   "● high",
	"xhigh":  "◉ xhigh",
	"max":    "◈ max",
}

func effortDisplay(level string) string {
	if v, ok := effortIcons[strings.ToLower(level)]; ok {
		return v
	}
	return effortIcons["high"]
}

func modelDisplay(name string) string {
	if name == "" {
		return ""
	}
	name = strings.TrimSuffix(name, " (default)")
	name = strings.ReplaceAll(name, " (1M context)", " (1M)")
	return name
}

// ==== SECTION: CTX/CACHE ====

func contextDisplay(usedPct float64) string {
	pct := usedPct / 0.8
	return fmt.Sprintf("Ctx: %.1f%%", pct)
}

func cacheHitDisplay(input, cacheRead, cacheCreation int) string {
	denom := input + cacheRead + cacheCreation
	if denom == 0 {
		return "Cache: -"
	}
	pct := float64(cacheRead) / float64(denom) * 100.0
	return fmt.Sprintf("Cache: %d%%", int(pct))
}

// ==== SECTION: TRANSCRIPT REGEX ====

var (
	// Match raw ESC byte OR JSON-escaped \u001b form (transcripts use the latter)
	ansiRE       = regexp.MustCompile(`(\x1b|\\u001b)\[[0-9;]*m`)
	effortRE     = regexp.MustCompile(`Set effort level to (\S+?)[: (]`)
	modelSetRE   = regexp.MustCompile(`Set model to (.+?) with (\S+) effort`)
	keptModelRE  = regexp.MustCompile(`Kept model as (.+?)(?:</|\n|$)`)
)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// Find the LAST (rightmost) match of effortRE in s. Returns "" if none.
func extractLatestEffort(s string) string {
	matches := effortRE.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

// Find the last "Set model to ... with X effort" match. Returns ("", "") if none.
func extractLatestModelSet(s string) (model, effort string) {
	matches := modelSetRE.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return "", ""
	}
	last := matches[len(matches)-1]
	return last[1], last[2]
}

// Find the last "Kept model as X" match. Returns "" if none.
func extractLatestKeptModel(s string) string {
	matches := keptModelRE.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

// ==== SECTION: TRANSCRIPT SCAN ====

const (
	transcriptChunkSize = 64 * 1024
	transcriptMaxBudget = 1024 * 1024
)

func scanTranscript(path string) (effort, model string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", "", err
	}
	size := stat.Size()
	if size == 0 {
		return "", "", nil
	}

	pos := size
	bytesRead := int64(0)
	var buffer []byte

	for pos > 0 && bytesRead < transcriptMaxBudget {
		readSize := int64(transcriptChunkSize)
		if pos < readSize {
			readSize = pos
		}
		newPos := pos - readSize
		if _, err := f.Seek(newPos, io.SeekStart); err != nil {
			return "", "", err
		}
		chunk := make([]byte, readSize)
		if _, err := io.ReadFull(f, chunk); err != nil {
			return "", "", err
		}
		pos = newPos
		bytesRead += readSize
		buffer = append(chunk, buffer...)

		stripped := stripANSI(string(buffer))

		if effort == "" {
			if v := extractLatestEffort(stripped); v != "" {
				effort = v
			}
			if m, e := extractLatestModelSet(stripped); e != "" {
				if effort == "" {
					effort = e
				}
				if model == "" {
					model = m
				}
			}
		}
		if model == "" {
			if m, _ := extractLatestModelSet(stripped); m != "" {
				model = m
			}
			if v := extractLatestKeptModel(stripped); v != "" {
				model = v
			}
		}

		if effort != "" && model != "" {
			break
		}
	}

	return effort, model, nil
}

// ==== SECTION: CACHE ====

type CacheEntry struct {
	Effort            string `json:"effort,omitempty"`
	Model             string `json:"model,omitempty"`
	TranscriptMtimeNs int64  `json:"transcript_mtime_ns,omitempty"`
	TranscriptSize    int64  `json:"transcript_size,omitempty"`
}

func cachePath(sessionID string) string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(dir, "claude-statusline", sessionID+".json")
}

func loadCache(path string) (*CacheEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CacheEntry{}, nil
		}
		return &CacheEntry{}, nil // treat any read error as empty
	}
	var c CacheEntry
	if err := json.Unmarshal(data, &c); err != nil {
		return &CacheEntry{}, nil // corrupt → empty
	}
	return &c, nil
}

func saveCache(path string, c *CacheEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ==== SECTION: SETTINGS FALLBACK ====

func readSettingsEffort(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s struct {
		EffortLevel string `json:"effortLevel"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s.EffortLevel
}

func defaultSettingsPath() string {
	return filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
}

// ==== SECTION: GIT ====

const gitBranchIcon = "\ue0a0" //

func parsePorcelain(raw string) (untracked, modified, deleted int) {
	for _, line := range strings.Split(raw, "\n") {
		if len(line) < 2 {
			continue
		}
		xy := line[:2]
		if xy == "??" {
			untracked++
			continue
		}
		// rename / copy entries (R/C) ignored entirely
		if xy[0] == 'R' || xy[0] == 'C' {
			continue
		}
		hasM := xy[0] == 'M' || xy[1] == 'M'
		hasD := xy[0] == 'D' || xy[1] == 'D'
		if hasM {
			modified++
		}
		if hasD {
			deleted++
		}
	}
	return
}

func formatGit(branch string, untracked, modified, deleted int) string {
	if branch == "" {
		return ""
	}
	out := gitBranchIcon + " " + branch
	if untracked > 0 {
		out += fmt.Sprintf(" ?%d", untracked)
	}
	if modified > 0 {
		out += fmt.Sprintf(" ~%d", modified)
	}
	if deleted > 0 {
		out += fmt.Sprintf(" -%d", deleted)
	}
	return out
}

// runGitInfo fetches branch + porcelain. Returns ("", 0,0,0) if not in a repo.
func runGitInfo(cwd string) (branch string, untracked, modified, deleted int) {
	bb, err := exec.Command("git", "-C", cwd, "branch", "--show-current").Output()
	if err != nil {
		return "", 0, 0, 0
	}
	branch = strings.TrimSpace(string(bb))
	if branch == "" {
		// detached HEAD or empty repo — try short SHA
		ss, err := exec.Command("git", "-C", cwd, "rev-parse", "--short", "HEAD").Output()
		if err == nil {
			branch = strings.TrimSpace(string(ss))
		}
	}
	pb, err := exec.Command("git", "-C", cwd, "status", "--porcelain=v1", "-uall").Output()
	if err != nil {
		return branch, 0, 0, 0
	}
	u, m, d := parsePorcelain(string(pb))
	return branch, u, m, d
}

// ==== SECTION: MAIN ====

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "panic:", r)
		}
		os.Exit(0)
	}()

	in, err := parseInput(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse stdin:", err)
		return
	}
	_ = in
	fmt.Print("")
}
