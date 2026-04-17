package main

import (
	"fmt"
	"io"
	"os"
)

// ==== SECTION: MAIN ====

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "panic:", r)
		}
		os.Exit(0)
	}()

	_, _ = io.ReadAll(os.Stdin)
	fmt.Print("")
}
