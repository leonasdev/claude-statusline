package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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
	colorDefaultFg   = "\x1b[39m" // explicit "default foreground" — overrides any ambient color
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
	ansiRE = regexp.MustCompile(`(\x1b|\\u001b)\[[0-9;]*m`)
	// Anchor to `"content":"<local-command-stdout>` — this is the exact byte
	// sequence at the start of a CC slash-command output entry's content
	// field. Prose mentioning the same strings is wrapped in JSON-escaped
	// quotes (`\"content\":\"`) so won't match.
	effortRE = regexp.MustCompile(`"content":"<local-command-stdout>Set effort level to (\S+?)[: (]`)
	// "Set model to <model>" handles four variants observed in real CC output:
	//   Set model to X
	//   Set model to X with Y effort
	//   Set model to X · Billed as extra usage
	//   Set model to X with Y effort · Billed as extra usage
	// Captures: 1=model name, 2=effort (may be empty if "with ... effort" is absent)
	modelSetRE  = regexp.MustCompile(`"content":"<local-command-stdout>Set model to (.+?)(?: with (\S+) effort)?(?: ·[^<]*?)?</local-command-stdout>`)
	keptModelRE = regexp.MustCompile(`"content":"<local-command-stdout>Kept model as (.+?)</local-command-stdout>`)
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

// extractLatestState replays every effort/model change event in s ordered by
// position, returning the final (effort, model) state. This handles the case
// where the most recent change is embedded in a "Set model to X with Y effort"
// line rather than a standalone "Set effort level to" line.
func extractLatestState(s string) (effort, model string) {
	type event struct {
		pos            int
		effort, model  string
	}
	var events []event

	for _, m := range effortRE.FindAllStringSubmatchIndex(s, -1) {
		events = append(events, event{pos: m[1], effort: s[m[2]:m[3]]})
	}
	for _, m := range modelSetRE.FindAllStringSubmatchIndex(s, -1) {
		ev := event{pos: m[1], model: s[m[2]:m[3]]}
		// modelSetRE's second capture (effort) is optional; when absent the
		// submatch index is -1.
		if len(m) > 5 && m[4] >= 0 {
			ev.effort = s[m[4]:m[5]]
		}
		events = append(events, ev)
	}
	for _, m := range keptModelRE.FindAllStringSubmatchIndex(s, -1) {
		events = append(events, event{pos: m[1], model: s[m[2]:m[3]]})
	}

	sort.Slice(events, func(i, j int) bool { return events[i].pos < events[j].pos })

	for _, ev := range events {
		if ev.effort != "" {
			effort = ev.effort
		}
		if ev.model != "" {
			model = ev.model
		}
	}
	return
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

		e, m := extractLatestState(stripped)
		if e != "" {
			effort = e
		}
		if m != "" {
			model = m
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
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

const gitBranchIcon = "" //

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


// ==== SECTION: SEGMENT RENDERERS ====

func renderPathSegment(cwd string) string {
	home := os.Getenv("HOME")
	displayed := substituteHome(cwd, home)
	// Wrap with colorDefaultFg so ambient colors (e.g. CC's TUI chrome ahead
	// of the statusline) don't bleed through and tint the path light_black.
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

// modelDefaultEffort maps a model family to its effort level when /effort=auto.
// Updated based on observed CC behavior; tweak when official defaults change.
var modelDefaultEffort = map[string]string{
	"Opus":   "xhigh",
	"Sonnet": "medium",
}

// defaultEffortFor returns the model's default effort, or "high" if unknown.
func defaultEffortFor(model string) string {
	for prefix, eff := range modelDefaultEffort {
		if strings.Contains(model, prefix) {
			return eff
		}
	}
	return "high"
}

// modelHasNoEffort returns true for models that don't support effort levels.
func modelHasNoEffort(model string) bool {
	return strings.Contains(model, "Haiku")
}

func renderEffortSegment(level, model string) string {
	if modelHasNoEffort(model) {
		return ""
	}
	if level == "auto" {
		level = defaultEffortFor(model)
	}
	return colorize(effortDisplay(level), colorLightBlack)
}

func renderContextSegment(usedPct float64) string {
	return colorize(contextDisplay(usedPct), colorLightBlack)
}

func renderCacheSegment(input, cacheRead, cacheCreation int) string {
	return colorize(cacheHitDisplay(input, cacheRead, cacheCreation), colorLightBlack)
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

	// Resolve effort + model with sidecar cache
	cachePathStr := cachePath(in.SessionID)
	cache, _ := loadCache(cachePathStr)
	effort, model := resolveEffortAndModel(in, cache)
	cacheChanged := updateCache(cache, in, effort, model)

	gitBranch, gitU, gitM, gitD := runGitInfo(in.Workspace.CurrentDir)

	pathSeg := renderPathSegment(in.Workspace.CurrentDir)
	gitSeg := renderGitSegment(gitBranch, gitU, gitM, gitD)
	modelSeg := renderModelSegment(model)
	effortSeg := renderEffortSegment(effort, model)
	ctxSeg := renderContextSegment(in.ContextWindow.UsedPercentage)
	cacheSeg := renderCacheSegment(
		in.ContextWindow.CurrentUsage.InputTokens,
		in.ContextWindow.CurrentUsage.CacheReadInputTokens,
		in.ContextWindow.CurrentUsage.CacheCreationInputTokens,
	)
	sessSeg := renderSessionSegment(in.RateLimits.FiveHour.UsedPercentage)
	resetSeg := renderResetSegment(in.RateLimits.FiveHour.ResetsAt, time.Now().Unix())

	out := joinSegments([]string{pathSeg, gitSeg, modelSeg, effortSeg, ctxSeg, cacheSeg, sessSeg, resetSeg})
	fmt.Print(out)

	if cacheChanged {
		_ = saveCache(cachePathStr, cache)
	}
}

// resolveEffortAndModel walks: cache → transcript scan → settings/Input fallbacks.
func resolveEffortAndModel(in *Input, cache *CacheEntry) (effort, model string) {
	stat, err := os.Stat(in.TranscriptPath)
	if err == nil &&
		stat.ModTime().UnixNano() == cache.TranscriptMtimeNs &&
		stat.Size() == cache.TranscriptSize &&
		(cache.Effort != "" || cache.Model != "") {
		return cache.Effort, cache.Model
	}

	if in.TranscriptPath != "" {
		eff, mod, _ := scanTranscript(in.TranscriptPath)
		effort = eff
		model = mod
	}

	if effort == "" {
		effort = readSettingsEffort(defaultSettingsPath())
	}
	if effort == "" {
		effort = "high"
	}
	if model == "" {
		model = in.Model.DisplayName
	}
	return effort, model
}

func updateCache(cache *CacheEntry, in *Input, effort, model string) bool {
	changed := false
	if cache.Effort != effort {
		cache.Effort = effort
		changed = true
	}
	if cache.Model != model {
		cache.Model = model
		changed = true
	}
	stat, err := os.Stat(in.TranscriptPath)
	if err == nil {
		ns := stat.ModTime().UnixNano()
		sz := stat.Size()
		if cache.TranscriptMtimeNs != ns || cache.TranscriptSize != sz {
			cache.TranscriptMtimeNs = ns
			cache.TranscriptSize = sz
			changed = true
		}
	}
	return changed
}
