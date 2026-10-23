package cd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

// captureCwd stands in for the interpreter back-channel the dispatcher
// normally supplies, and records the directory cd asked to move to.
func captureCwd(got *string) func(string) error {
	return func(dir string) error {
		*got = dir
		return nil
	}
}

func TestCdValidDir(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/tmp/x", 0o755)
	var moved string
	r := New().Execute(context.Background(), []string{"cd", "/tmp/x"},
		&command.Context{FS: fs, Env: map[string]string{}, SetCwd: captureCwd(&moved)})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
	if moved != "/tmp/x" {
		t.Errorf("cwd moved to %q, want /tmp/x", moved)
	}
}

// TestCdWithoutBackChannel pins the fail-closed path: with no SetCwd
// there is no interpreter to move, and cd must say so rather than
// report a success it did not perform.
func TestCdWithoutBackChannel(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/tmp/x", 0o755)
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"cd", "/tmp/x"},
		&command.Context{FS: fs, Env: map[string]string{}, Stderr: &e})
	if r.ExitCode == 0 {
		t.Errorf("exit=%d, want non-zero", r.ExitCode)
	}
	if !strings.Contains(e.String(), "cd:") {
		t.Errorf("stderr=%q", e.String())
	}
}

func TestCdMissing(t *testing.T) {
	fs := memfs.New()
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"cd", "/nope"}, &command.Context{FS: fs, Stderr: &e, Env: map[string]string{}})
	if r.ExitCode == 0 || !strings.Contains(e.String(), "cd:") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}

func TestCdHome(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/home/user", 0o755)
	var moved string
	r := New().Execute(context.Background(), []string{"cd"},
		&command.Context{FS: fs, Env: map[string]string{"HOME": "/home/user"}, SetCwd: captureCwd(&moved)})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
	if moved != "/home/user" {
		t.Errorf("cwd moved to %q, want /home/user", moved)
	}
}

func TestCdHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"cd", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "cd") {
		t.Errorf("help=%q", o.String())
	}
}
