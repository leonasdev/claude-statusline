package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
