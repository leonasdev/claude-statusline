package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ==== SECTION: INPUT TYPES ====

type Input struct {
	Model struct {
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
	Effort struct {
		Level string `json:"level"`
	} `json:"effort"`
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
	// Bright white. Used for the path segment. Neither `\x1b[0m` nor
	// `\x1b[22;39m` (default fg) works inside CC's TUI — both collapse to
	// the ambient frame-gray of the statusline container. Only an EXPLICIT
	// color code forces CC to render our chosen shade.
	colorDefaultFg = "\x1b[97m"
	colorNone      = ""
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

// pathThreshold caps path display length. Paths above this get progressively
// abbreviated. Independent of terminal width — set the value you find readable.
const pathThreshold = 40

func fitPath(path string) string {
	for level := 0; level <= 4; level++ {
		v := abbreviatePath(path, level)
		if len([]rune(v)) <= pathThreshold {
			return v
		}
	}
	return abbreviatePath(path, 4)
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
	days := secs / (60 * 60 * 24)
	hours := (secs / 3600) % 24
	mins := (secs / 60) % 60
	s := secs % 60

	if days > 0 {
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		out := fmt.Sprintf("%dh", hours)
		if mins > 0 {
			out += fmt.Sprintf("%dm", mins)
		}
		if s > 0 {
			out += fmt.Sprintf("%ds", s)
		}
		return out
	}
	if mins > 0 {
		if s == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm%ds", mins, s)
	}
	return fmt.Sprintf("%ds", s)
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

// ==== SECTION: CTX ====

func contextDisplay(usedPct float64) string {
	pct := usedPct / 0.8
	return fmt.Sprintf("Ctx: %.1f%%", pct)
}

// ==== SECTION: GIT ====

const gitBranchIcon = "" // Nerd Font branch glyph

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
	pb, err := exec.Command("git", "-C", cwd, "status", "--no-optional-locks", "--porcelain=v1", "-uall").Output()
	if err != nil {
		return branch, 0, 0, 0
	}
	u, m, d := parsePorcelain(string(pb))
	return branch, u, m, d
}

// ==== SECTION: SEGMENT RENDERERS ====

func renderPathSegment(cwd string) string {
	home := os.Getenv("HOME")
	displayed := substituteHome(cwd, home)
	// CC's TUI renders the statusline inside a gray-styled container and
	// treats `\x1b[0m` / `\x1b[39m` (default fg) as "inherit container" —
	// so we have to pick an explicit color. Bright white tracks most
	// terminal default fg themes closely.
	return colorize(fitPath(displayed), colorDefaultFg)
}

func renderGitSegment(branch string, untracked, modified, deleted int) string {
	raw := formatGit(branch, untracked, modified, deleted)
	if raw == "" {
		return ""
	}
	return colorize(raw, colorLightYellow)
}

func renderModelSegment(name string) string {
	return colorize(modelDisplay(name), colorClaudeBold)
}

// modelHasNoEffort returns true for models that don't support effort levels.
func modelHasNoEffort(model string) bool {
	return strings.Contains(model, "Haiku")
}

func renderEffortSegment(level, model string) string {
	if modelHasNoEffort(model) {
		return ""
	}
	return colorize(effortDisplay(level), colorLightBlack)
}

func renderContextSegment(usedPct float64) string {
	return colorize(contextDisplay(usedPct), colorLightBlack)
}

func renderSessionSegment(pct float64) string {
	bar := renderBar(pct, 16)
	return colorize(fmt.Sprintf("Session: [%s] %.1f%%", bar, pct), colorLightBlack)
}

func renderResetSegment(resetsAt int64, now int64) string {
	remain := resetsAt - now
	if remain < 0 {
		remain = 0
	}
	return colorize("Reset: "+formatDuration(remain), colorLightBlack)
}

func joinSegments(parts []string) string {
	sep := colorize(" │ ", colorLightBlack)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
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

	gitBranch, gitU, gitM, gitD := runGitInfo(in.Workspace.CurrentDir)

	pathSeg := renderPathSegment(in.Workspace.CurrentDir)
	gitSeg := renderGitSegment(gitBranch, gitU, gitM, gitD)
	modelSeg := renderModelSegment(in.Model.DisplayName)
	effortSeg := renderEffortSegment(in.Effort.Level, in.Model.DisplayName)
	ctxSeg := renderContextSegment(in.ContextWindow.UsedPercentage)
	sessSeg := renderSessionSegment(in.RateLimits.FiveHour.UsedPercentage)
	resetSeg := renderResetSegment(in.RateLimits.FiveHour.ResetsAt, time.Now().Unix())

	out := joinSegments([]string{pathSeg, gitSeg, modelSeg, effortSeg, ctxSeg, sessSeg, resetSeg})
	fmt.Print(out)
}
