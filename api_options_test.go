package gobash

import (
	"context"
	"testing"
)

func TestExecOptionsArgsAttachToFirstLexicalCommand(t *testing.T) {
	b, err := New(BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ script, want string }{
		{`echo "$(printf sub)"`, "sub\n"},
		{`echo first; echo second`, "first x\nsecond\n"},
	} {
		res, err := b.Exec(context.Background(), tc.script, ExecOptions{Args: []string{"x"}})
		if err != nil {
			t.Fatal(err)
		}
		if res.Stdout != tc.want {
			t.Errorf("Exec(%q) stdout=%q, want %q", tc.script, res.Stdout, tc.want)
		}
	}
}

func TestAPIOptionsEnvAndValidation(t *testing.T) {
	b, err := New(BashOptions{Env: map[string]string{"BASE": "base"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `printf '%s:%s' "$BASE" "$EXTRA"`, ExecOptions{Env: map[string]string{"EXTRA": "call"}})
	if err != nil || res.Stdout != "base:call" {
		t.Fatalf("merged env: %#v, %v", res, err)
	}
	res, err = b.Exec(context.Background(), `printf '%s:%s' "${BASE-unset}" "$EXTRA"`, ExecOptions{Env: map[string]string{"EXTRA": "call"}, ReplaceEnv: true})
	if err != nil || res.Stdout != "unset:call" {
		t.Fatalf("replaced env: %#v, %v", res, err)
	}
	if _, err := New(BashOptions{ExecutionLimits: &ExecutionLimits{MaxStringLength: intPtrTest(-1)}}); err == nil {
		t.Fatal("expected negative limit rejection")
	}
}

func intPtrTest(v int) *int { return &v }
