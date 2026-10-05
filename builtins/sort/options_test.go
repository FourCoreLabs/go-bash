package sort_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sort"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestDictionaryAndMonth(t *testing.T) {
	for _, tt := range []struct {
		args        []string
		input, want string
	}{
		{[]string{"-d"}, "b\n!a\n", "!a\nb\n"},
		{[]string{"--dictionary-order", "-f"}, "B\n!a\n", "!a\nB\n"},
		{[]string{"-ds"}, "a!\na#\n", "a!\na#\n"},
		{[]string{"-du"}, "a!\na!\na#\n", "a!\na#\n"},
		{[]string{"-M"}, "Dec\nunknown\nfebruary\n JAN\n", "unknown\n JAN\nfebruary\nDec\n"},
		{[]string{"--month-sort", "-r"}, "Dec\nFeb\nJan\n", "Dec\nFeb\nJan\n"},
		{[]string{"-Ms"}, "JAN-z\nJAN-a\n", "JAN-z\nJAN-a\n"},
		{[]string{"-M", "-t", ":", "-k", "2,2"}, "x:Dec\ny:Jan\n", "y:Jan\nx:Dec\n"},
	} {
		out, err, code := run(t, tt.input, tt.args...)
		if code != 0 || err != "" || out != tt.want {
			t.Errorf("args=%v got=%q err=%q code=%d want=%q", tt.args, out, err, code, tt.want)
		}
	}
	_, _, code := run(t, "Feb\nJan\n", "-Mc")
	if code != 1 {
		t.Errorf("check code=%d", code)
	}
}

func TestOutputFile(t *testing.T) {
	for _, flags := range [][]string{{"-o", "data"}, {"-odata"}, {"--output=data"}, {"--output", "data"}} {
		fs := memfs.New()
		_ = fs.MkdirAll("/work", 0755)
		_ = fs.WriteFile("/work/data", []byte("b\na\n"), 0644)
		var out, stderr bytes.Buffer
		args := append([]string{"sort", "data"}, flags...)
		res := sort.New().Execute(context.Background(), args, &command.Context{FS: fs, Cwd: "/work", Stdout: &out, Stderr: &stderr})
		data, _ := fs.ReadFile("/work/data")
		if res.ExitCode != 0 || out.Len() != 0 || stderr.Len() != 0 || string(data) != "a\nb\n" {
			t.Fatalf("args=%v code=%d data=%q err=%q", args, res.ExitCode, data, stderr.String())
		}
	}
}

func TestOutputErrorsAndCheck(t *testing.T) {
	for _, tt := range []struct {
		args []string
		code int
	}{
		{[]string{"-o"}, 2}, {[]string{"-o", "/missing/out"}, 2}, {[]string{"-c", "-o", "/result"}, 0},
	} {
		fs := memfs.New()
		var out, stderr bytes.Buffer
		res := sort.New().Execute(context.Background(), append([]string{"sort"}, tt.args...), &command.Context{FS: fs, Cwd: "/", Stdin: strings.NewReader("a\nb\n"), Stdout: &out, Stderr: &stderr})
		if res.ExitCode != tt.code || out.Len() != 0 {
			t.Errorf("args=%v code=%d out=%q err=%q", tt.args, res.ExitCode, out.String(), stderr.String())
		}
		if _, err := fs.Stat("/result"); err == nil {
			t.Error("check created output")
		}
	}
}
