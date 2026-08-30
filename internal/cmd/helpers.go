package cmd

import (
	"fmt"
	"strings"
)

// errInvalid returns a formatted "invalid value" error for a flag/arg.
func errInvalid(name, value, allowed string) error {
	if allowed == "" {
		return fmt.Errorf("invalid %s: %q", name, value)
	}
	return fmt.Errorf("invalid %s %q (want %s)", name, value, allowed)
}

// joinValid builds a human-readable allowed-values hint.
func joinValid(vs ...string) string {
	return strings.Join(vs, "|")
}

// joinSpace joins strings with a single space.
func joinSpace(vs []string) string {
	return strings.Join(vs, " ")
}

var _ = joinValid // reserved for future subcommands
