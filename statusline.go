package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"
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
	Version string `json:"version"`
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
	colorLightRed    = "\x1b[91m"
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
		return fmt.Sprintf("%dh%02dm%02ds", hours, mins, s)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm%02ds", mins, s)
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

func contextPct(usedPct float64) float64 {
	return usedPct / 0.8
}

func contextDisplay(usedPct float64) string {
	return fmt.Sprintf("Ctx: %.0f%%", contextPct(usedPct))
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
	pb, err := exec.Command("git", "-C", cwd, "--no-optional-locks", "status", "--porcelain=v1", "-uall").Output()
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

// pctColor returns the threshold color for a usage percentage:
// red at >= 95, yellow at >= 80, otherwise the dim default.
func pctColor(pct float64) string {
	switch {
	case pct >= 95:
		return colorLightRed
	case pct >= 80:
		return colorLightYellow
	default:
		return colorLightBlack
	}
}

func renderContextSegment(usedPct float64) string {
	pct := contextPct(usedPct)
	return colorize("Ctx: ", colorLightBlack) +
		colorize(fmt.Sprintf("%.0f%%", pct), pctColor(pct))
}

func renderSessionSegment(pct float64, resetsAt int64) string {
	if resetsAt == 0 {
		return colorize("Session: -%", colorLightBlack)
	}
	bar := renderBar(pct, 16)
	return colorize(fmt.Sprintf("Session: [%s] ", bar), colorLightBlack) +
		colorize(fmt.Sprintf("%.1f%%", pct), pctColor(pct))
}

func renderResetSegment(resetsAt int64, now int64) string {
	if resetsAt == 0 {
		return colorize("Reset: -", colorLightBlack)
	}
	remain := resetsAt - now
	if remain < 0 {
		remain = 0
	}
	return colorize("Reset: "+formatDuration(remain), colorLightBlack)
}

func renderVersionSegment(version string) string {
	if version == "" {
		return ""
	}
	return colorize("v"+version, colorLightBlack)
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

// visibleWidth returns the rune count of s ignoring ANSI escape sequences
// (`\x1b[...<letter>`). CJK / wide characters are counted as 1 — accept the
// minor misalignment to keep the implementation dependency-free.
func visibleWidth(s string) int {
	n := 0
	inEscape := false
	for _, r := range s {
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		if r == 0x1b {
			inEscape = true
			continue
		}
		n++
	}
	return n
}

// ccFrameMargin is the number of columns CC's TUI reserves around the
// statusline content (left + right gutter). Without this offset, right-
// aligning to the full terminal width pushes the trailing segment past CC's
// clip region and it shows up truncated as `…`. Empirically 4 (2 left + 2
// right) — bump it if the trailing segment still gets clipped.
const ccFrameMargin = 4

// widthFromPath ioctls TIOCGWINSZ on the given path and returns col, or 0.
func widthFromPath(p string) int {
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer f.Close()
	var ws struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0
	}
	return int(ws.Col)
}

// widthFromProcFD walks fd 0/1/2 of pid looking for a TTY symlink target,
// then queries its size.
func widthFromProcFD(pid int) int {
	for _, fd := range []string{"0", "1", "2"} {
		target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "/dev/pts/") || target == "/dev/tty" {
			if w := widthFromPath(target); w > 0 {
				return w
			}
		}
	}
	return 0
}

// readPPID parses /proc/<pid>/status for the PPid: field.
func readPPID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			var ppid int
			fmt.Sscanf(line, "PPid: %d", &ppid)
			return ppid
		}
	}
	return 0
}

// termWidth returns the terminal column count. CC ≥2.1.139 often spawns the
// statusline subprocess via setsid (no controlling TTY), so /dev/tty fails.
// Fallback: walk parent process tree until we find one whose stdin/out/err
// is a /dev/pts/* TTY (typically CC itself), then ioctl that.
func termWidth() int {
	if w := widthFromPath("/dev/tty"); w > 0 {
		return w
	}
	pid := os.Getppid()
	for i := 0; i < 6 && pid > 1; i++ {
		if w := widthFromProcFD(pid); w > 0 {
			return w
		}
		pid = readPPID(pid)
	}
	return 0
}

// alignRight glues right flush to column totalWidth, padding with spaces.
// If width is unknown (<=0) or there's no room, falls back to a single
// space gap so version still appears.
func alignRight(left, right string, totalWidth int) string {
	if right == "" {
		return left
	}
	used := visibleWidth(left) + visibleWidth(right)
	if totalWidth <= 0 || used > totalWidth {
		return left + " " + right
	}
	return left + strings.Repeat(" ", totalWidth-used) + right
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
	sessSeg := renderSessionSegment(in.RateLimits.FiveHour.UsedPercentage, in.RateLimits.FiveHour.ResetsAt)
	resetSeg := renderResetSegment(in.RateLimits.FiveHour.ResetsAt, time.Now().Unix())
	versionSeg := renderVersionSegment(in.Version)

	// Path + git share a "where am I" unit — glue with a space, no separator.
	pathGitSeg := pathSeg
	if gitSeg != "" {
		pathGitSeg += " " + gitSeg
	}

	left := joinSegments([]string{pathGitSeg, modelSeg, effortSeg, ctxSeg, sessSeg, resetSeg})
	width := termWidth() - ccFrameMargin
	out := alignRight(left, versionSeg, width)
	fmt.Print(out)
}
