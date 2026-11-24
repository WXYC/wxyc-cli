//go:build unix

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// interruptHelperEnv makes the test binary act as a CLI blocked on something
// that ignores the context. It is not WXYC_*, so TestMain's scrub leaves it
// alone.
const interruptHelperEnv = "INTERRUPT_CONTEXT_HELPER"

func TestMain_interruptHelper(t *testing.T) {
	if os.Getenv(interruptHelperEnv) != "1" {
		t.Skip("helper process only")
	}
	ctx, stop := interruptContext(context.Background())
	defer stop()
	fmt.Println("ready")
	<-ctx.Done()
	fmt.Println("cancelled")
	select {} // a prompt that never looks at ctx
}

// The first Ctrl-C cancels the context; a second must kill the process even
// when the command is blocked on something that ignores the context.
func TestInterruptContext_SecondInterruptKills(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_interruptHelper$")
	cmd.Env = append(os.Environ(), interruptHelperEnv+"=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })

	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-lines:
			if got != want {
				t.Fatalf("helper printed %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("helper never printed %q", want)
		}
	}

	expect("ready")
	cmd.Process.Signal(os.Interrupt)
	expect("cancelled") // first Ctrl-C cancels rather than kills

	cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("Wait() = %v, want the process killed by SIGINT", err)
		}
		ws := ee.Sys().(syscall.WaitStatus)
		if !ws.Signaled() || ws.Signal() != syscall.SIGINT {
			t.Fatalf("process ended with %v, want killed by SIGINT", ws)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second Ctrl-C did not kill a process blocked on a prompt")
	}
}
