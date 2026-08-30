package main

import (
	"os"

	"github.com/minoa/guddit/internal/cmd"
)

// Build-time variables, injected by the Makefile via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetBuildInfo(version, commit, date)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}