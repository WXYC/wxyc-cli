package cmd

import (
	"strings"
	"testing"
)

// -n below 1 would otherwise be passed along and silently replaced by the
// server's default; refuse it instead. The trap server fails the test if
// anything is sent.
func TestLabelsSearch_RejectsLimitBelowOne(t *testing.T) {
	t.Setenv("WXYC_JWT", "unused")
	for _, n := range []string{"0", "-5"} {
		_, err := runCLI(t, "labels", "search", "Merge", "-n", n)
		if want := "--limit must be at least 1, got " + n; err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("-n %s: err = %v, want %q", n, err, want)
		}
	}
}
