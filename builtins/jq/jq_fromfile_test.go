package jq_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/FourCoreLabs/go-bash/builtins/jq"
	"github.com/FourCoreLabs/go-bash/command"
	"github.com/FourCoreLabs/go-bash/fs/memfs"
)

func runFS(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := jq.New().Execute(context.Background(), append([]string{"jq"}, args...), &command.Context{
		FS: mfs, Cwd: "/work", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func fromFileFS(t *testing.T) *memfs.FS {
	t.Helper()
	mfs := memfs.New()
	_ = mfs.MkdirAll("/work", 0o755)
	_ = mfs.WriteFile("/work/build.jq", []byte("# pick the names\n[.items[] | .name]\n"), 0o644)
	_ = mfs.WriteFile("/work/in.json", []byte(`{"items":[{"name":"a"},{"name":"b"}]}`), 0o644)
	return mfs
}

func TestFromFileReadsFilterAndInputFile(t *testing.T) {
	for _, flag := range []string{"-f", "--from-file"} {
		out, e, code := runFS(t, fromFileFS(t), "", "-c", flag, "build.jq", "in.json")
		if code != 0 || out != "[\"a\",\"b\"]\n" {
			t.Errorf("%s: out=%q err=%q code=%d", flag, out, e, code)
		}
	}
}

func TestFromFileReadsStdinWithoutInputFile(t *testing.T) {
	out, e, code := runFS(t, fromFileFS(t), `{"items":[{"name":"c"}]}`, "-f", "/work/build.jq", "-c")
	if code != 0 || out != "[\"c\"]\n" {
		t.Errorf("out=%q err=%q code=%d", out, e, code)
	}
}

func TestFromFileBundledWithOtherFlags(t *testing.T) {
	mfs := fromFileFS(t)
	_ = mfs.WriteFile("/work/answer.jq", []byte("$x + 1"), 0o644)
	out, e, code := runFS(t, mfs, "", "-nrf", "answer.jq", "--argjson", "x", "41")
	if code != 0 || out != "42\n" {
		t.Errorf("out=%q err=%q code=%d", out, e, code)
	}
}

func TestFromFileMissingFile(t *testing.T) {
	_, e, code := runFS(t, fromFileFS(t), "", "-f", "absent.jq", "in.json")
	if code != 2 || !strings.Contains(e, "absent.jq") {
		t.Errorf("code=%d err=%q", code, e)
	}
}
