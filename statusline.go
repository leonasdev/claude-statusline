package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
