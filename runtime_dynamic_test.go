package gobash_test

import (
	"context"
	"errors"
	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
	"strings"
	"testing"
	"time"
)

func TestDynamicScriptLimits(t *testing.T) {
	for _, script := range []string{
		`eval 'while true; do :; done'`,
		`source /work/loop`,
		`. /work/loop`,
		`builtin eval 'while true; do :; done'`,
		`command eval 'while true; do :; done'`,
		`eval "$(printf '%s' 'while true; do :; done')"`,
	} {
		t.Run(script, func(t *testing.T) {
			b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{"/work/loop": {Content: []byte(`while true; do :; done`)}}, ExecutionLimits: &gobash.ExecutionLimits{MaxLoopIterations: intPtr(3)}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = b.Exec(ctx, script, gobash.ExecOptions{})
			var limit *gobash.ExecutionLimitError
			if !errors.As(err, &limit) || limit.Limit != "MaxLoopIterations" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDynamicShadowedFunctionNames(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{"/work/loop": {Content: []byte(`while true; do :; done`)}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Exec(context.Background(), `eval() { echo function-eval; }; eval; source() { echo function-source; }; source; .() { echo function-dot; }; .`, gobash.ExecOptions{})
	if err != nil || r.Stdout != "function-eval\nfunction-source\nfunction-dot\n" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestDynamicTrapLoopInstrumentation(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{ExecutionLimits: &gobash.ExecutionLimits{MaxLoopIterations: intPtr(3)}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = b.Exec(ctx, `trap 'while true; do :; done' EXIT`, gobash.ExecOptions{})
	var limit *gobash.ExecutionLimitError
	if !errors.As(err, &limit) || limit.Limit != "MaxLoopIterations" {
		t.Fatalf("error = %v", err)
	}
}

func TestDynamicScriptDepth(t *testing.T) {
	for _, script := range []string{`source /work/recurse`, `eval "$code"`} {
		b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{"/work/recurse": {Content: []byte(`source /work/recurse`)}}, Env: map[string]string{"code": `eval "$code"`}, ExecutionLimits: &gobash.ExecutionLimits{MaxSourceDepth: intPtr(3)}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = b.Exec(context.Background(), script, gobash.ExecOptions{})
		var limit *gobash.ExecutionLimitError
		if !errors.As(err, &limit) || limit.Limit != "MaxSourceDepth" {
			t.Fatalf("%s: error = %v", script, err)
		}
	}
}

func TestDynamicScriptCurrentShell(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{
		"/work/set": {Content: []byte(`x=source; export SAVED=yes; f() { echo function; }; echo "$1"; return 7; echo unreachable`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Exec(context.Background(), `mkdir -p /tmp; set -- outer; source /work/set inner; echo "$? $x $1"; f; eval 'x=eval; cd /tmp'; echo "$x $PWD $SAVED"`, gobash.ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Stdout != "inner\n7 source outer\nfunction\neval /tmp yes\n" {
		t.Fatalf("stdout = %q stderr=%q", r.Stdout, r.Stderr)
	}
}

func TestDynamicScriptStaticCaps(t *testing.T) {
	for _, script := range []string{`eval 'echo {1..100}'`, `source /work/cap`} {
		b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{"/work/cap": {Content: []byte(`echo {1..100}`)}}, ExecutionLimits: &gobash.ExecutionLimits{MaxBraceExpansionResults: intPtr(3)}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = b.Exec(context.Background(), script, gobash.ExecOptions{})
		var limit *gobash.ExecutionLimitError
		if !errors.As(err, &limit) || limit.Limit != "MaxBraceExpansionResults" {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestDynamicScriptVirtualIDs(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{ProcessInfo: &gobash.ProcessInfo{PID: 12345, PPID: 6789}, Files: map[string]gbfs.FileInit{"/work/ids": {Content: []byte(`echo $$ $PPID`)}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Exec(context.Background(), `eval 'echo $$ $PPID'; source /work/ids`, gobash.ExecOptions{})
	if err != nil || r.Stdout != "12345 6789\n12345 6789\n" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestDynamicSourceReadBound(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{"/work/large": {Content: []byte(strings.Repeat(" ", (1<<20)+1))}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(), `source /work/large`, gobash.ExecOptions{})
	var pe *gobash.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error=%v", err)
	}
}

func TestDynamicSubstitutionParallel(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{Files: map[string]gbfs.FileInit{"/work/text": {Content: []byte(`eval 'echo ok'`)}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Exec(context.Background(), `(source /work/text) | (source /work/text; cat)`, gobash.ExecOptions{})
	if err != nil || r.Stdout != "ok\nok\n" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}
