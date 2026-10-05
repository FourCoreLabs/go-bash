package command_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

// A command registered through Define is third-party code run inside the host
// process, from goroutines the caller does not own. A panic in it must come
// back as a failed command with the shell's internal-error status.
func TestDefinePanicBecomesFailedResult(t *testing.T) {
	cmd := command.Define("boom", func(context.Context, []string, *command.Context) command.Result {
		panic("kaboom")
	})

	var stderr bytes.Buffer
	result := cmd.Execute(context.Background(), []string{"boom"}, &command.Context{Stderr: &stderr})

	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if got := stderr.String(); !strings.Contains(got, "boom: internal error: kaboom") {
		t.Fatalf("stderr = %q, want it to name the command and the panic", got)
	}
}

func TestDefinePanicWithoutStderr(t *testing.T) {
	cmd := command.Define("boom", func(context.Context, []string, *command.Context) command.Result {
		panic("kaboom")
	})
	if got := cmd.Execute(context.Background(), []string{"boom"}, nil).ExitCode; got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
}
