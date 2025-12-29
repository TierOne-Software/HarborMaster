package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	rootCmd.Version = version
	if err := Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
