package yes_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/builtins/yes"
	"github.com/mark3labs/go-bash/command"
)

// Cases ported from upstream commands/yes/yes.test.ts.
func TestUpstreamParity(t *testing.T) {
	for _, tc := range []struct{ script, out string }{
		{"yes | head -3", "y\ny\ny\n"},
		{"yes a b | head -2", "a b\na b\n"},
		{"yes '' | head -2", "\n\n"},
		{"yes -- -x --help | head -1", "-x --help\n"},
		{"yes - | head -1", "-\n"},
		{"yes ok > out.txt; cat out.txt", strings.Repeat("ok\n", 7)},
		{"yes | wc -l", "      7\n"},
	} {
		t.Run(tc.script, func(t *testing.T) {
			n := 7
			b, err := gobash.New(gobash.BashOptions{ExecutionLimits: &gobash.ExecutionLimits{MaxLoopIterations: &n}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := b.Exec(context.Background(), tc.script, gobash.ExecOptions{})
			if err != nil || r.ExitCode != 0 || r.Stdout != tc.out || r.Stderr != "" {
				t.Fatalf("result=%+v err=%v", r, err)
			}
		})
	}
}

func TestBoundsAndOptions(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		loops, budget int
		out, stderr   string
		code          int
	}{
		{[]string{"ab"}, 1000, 9, "ab\nab\nab\n", "", 0},
		{[]string{"é"}, 1000, 12, "é\né\né\né\n", "", 0},
		{nil, 0, 12, "", "", 0},
		{[]string{"abcd"}, 1, 4, "", "bash: yes: output size limit exceeded (4 bytes)\n", 126},
		{[]string{"aaaa", "bbbb", "cccc"}, 1, 8, "", "bash: yes: output size limit exceeded (8 bytes)\n", 126},
		{[]string{"a", "-xy"}, 7, 100, "", "yes: invalid option -- 'x'\n", 1},
		{[]string{"--forever"}, 7, 100, "", "yes: unrecognized option '--forever'\n", 1},
	} {
		var out, stderr bytes.Buffer
		c := &command.Context{Stdout: &out, Stderr: &stderr, Limits: command.Limits{MaxLoopIterations: tc.loops, MaxOutputSize: tc.budget, MaxStringLength: tc.budget}}
		r := yes.New().Execute(context.Background(), append([]string{"yes"}, tc.args...), c)
		if r.ExitCode != tc.code || out.String() != tc.out || stderr.String() != tc.stderr {
			t.Fatalf("%v: %+v %q %q", tc.args, r, out.String(), stderr.String())
		}
	}
}

type failingWriter struct {
	calls int
	short bool
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.short {
		return 0, nil
	}
	return 0, errors.New("broken output")
}
func TestCancellationAndWriterErrors(t *testing.T) {
	for _, short := range []bool{false, true} {
		w := &failingWriter{short: short}
		c := &command.Context{Stdout: w, Stderr: io.Discard, Limits: command.Limits{MaxLoopIterations: 100, MaxOutputSize: 1000, MaxStringLength: 1000}}
		r := yes.New().Execute(context.Background(), []string{"yes"}, c)
		if r.ExitCode != 1 || w.calls != 1 {
			t.Fatalf("%+v calls=%d", r, w.calls)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w.calls = 0
		r = yes.New().Execute(ctx, []string{"yes"}, c)
		if r.ExitCode != 1 || w.calls != 0 {
			t.Fatalf("cancel: %+v calls=%d", r, w.calls)
		}
	}
}
