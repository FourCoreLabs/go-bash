package rg_test

import (
	"strings"
	"testing"

	"bytes"
	"context"
	"github.com/mark3labs/go-bash/builtins/rg"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func executeRG(t *testing.T, files map[string]string, links map[string]string, args ...string) (string, string, int) {
	t.Helper()
	fs := memfs.New()
	for p, v := range files {
		if err := fs.MkdirAll(parentDir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(p, []byte(v), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for p, target := range links {
		if err := fs.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	res := rg.New().Execute(context.Background(), append([]string{"rg"}, args...), &command.Context{FS: fs, Cwd: "/", Stdout: &out, Stderr: &stderr})
	return out.String(), stderr.String(), res.ExitCode
}

func TestNewMatchingFlags(t *testing.T) {
	files := map[string]string{"/d/a.txt": "FOO foo\nfoo.bar\nfoobar\n", "/d/pat": "foo\nbar\n"}
	cases := []struct {
		name string
		args []string
		want string
		code int
	}{
		{"smartcase-sensitive", []string{"Foo", "/d/a.txt"}, "", 1},
		{"smartcase-lowercase", []string{"foo", "/d/a.txt"}, "FOO foo", 0},
		{"smartcase-opt-in", []string{"-S", "foo", "/d/a.txt"}, "FOO foo", 0},
		{"fixed", []string{"-F", ".", "/d/a.txt"}, "foo.bar", 0},
		{"word", []string{"-w", "foo", "/d/a.txt"}, "FOO foo", 0},
		{"whole-line", []string{"-x", "foo", "/d/a.txt"}, "", 1},
		{"only-match", []string{"-o", "foo", "/d/a.txt"}, "foo", 0},
		{"replace", []string{"-r", "X", "foo", "/d/a.txt"}, "X", 0},
		{"pattern-file", []string{"-f", "/d/pat", "/d/a.txt"}, "foo", 0},
		{"max-count", []string{"-m", "1", "foo", "/d/a.txt"}, "FOO foo", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, code := executeRG(t, files, nil, tc.args...)
			if code != tc.code || !strings.Contains(out, tc.want) {
				t.Errorf("out=%q code=%d want substring %q code %d", out, code, tc.want, tc.code)
			}
		})
	}
}

func TestQuietSuppressesOutputAndReportsStatus(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		code    int
	}{{"yes", 0}, {"no", 1}} {
		out, _, code := executeRG(t, map[string]string{"/d/a": "yes\n"}, nil, "-q", tc.pattern, "/d/a")
		if out != "" || code != tc.code {
			t.Errorf("pattern %q: out=%q code=%d", tc.pattern, out, code)
		}
	}
}

func TestGitignoreAndNoIgnore(t *testing.T) {
	files := map[string]string{"/d/.gitignore": "*.log\n!important.log\nignored/\n", "/d/a.log": "match\n", "/d/important.log": "match\n", "/d/ignored/x": "match\n", "/d/keep.txt": "match\n"}
	out, _, code := executeRG(t, files, nil, "--hidden", "match", "/d")
	if code != 0 || strings.Contains(out, "a.log") || strings.Contains(out, "ignored/x") || !strings.Contains(out, "important.log") || !strings.Contains(out, "keep.txt") {
		t.Fatalf("ignore result code=%d output=%q", code, out)
	}
	out, _, code = executeRG(t, files, nil, "--hidden", "--no-ignore", "match", "/d")
	if code != 0 || !strings.Contains(out, "a.log") || !strings.Contains(out, "ignored/x") {
		t.Fatalf("no-ignore result code=%d output=%q", code, out)
	}
}

func TestMaxDepthAndFollowCycle(t *testing.T) {
	files := map[string]string{"/d/top": "hit\n", "/d/sub/child": "hit\n"}
	out, _, code := executeRG(t, files, nil, "--max-depth", "1", "hit", "/d")
	if code != 0 || !strings.Contains(out, "/d/top") || strings.Contains(out, "child") {
		t.Fatalf("max depth: code=%d output=%q", code, out)
	}
	out, _, code = executeRG(t, files, map[string]string{"/d/sub/back": "/d"}, "-L", "hit", "/d")
	if code != 0 || !strings.Contains(out, "child") {
		t.Fatalf("follow cycle: code=%d output=%q", code, out)
	}
}
