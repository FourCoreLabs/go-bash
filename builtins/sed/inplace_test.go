package sed_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sed"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestInPlace(t *testing.T) {
	for _, flag := range []string{"-i", "--in-place", "-i.bak", "--in-place=.bak", "-Ei.bak"} {
		t.Run(flag, func(t *testing.T) {
			fs := memfs.New()
			_ = fs.MkdirAll("/work", 0755)
			for _, name := range []string{"a", "b"} {
				_ = fs.WriteFile("/work/"+name, []byte("foo\nfoo\n"), 0600)
			}
			var out, stderr bytes.Buffer
			res := sed.New().Execute(context.Background(), []string{"sed", flag, "1s/foo/bar/", "a", "-", "b"}, &command.Context{FS: fs, Cwd: "/work", Stdin: strings.NewReader("foo\n"), Stdout: &out, Stderr: &stderr})
			if res.ExitCode != 0 || out.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("code=%d out=%q err=%q", res.ExitCode, out.String(), stderr.String())
			}
			for _, name := range []string{"a", "b"} {
				data, err := fs.ReadFile("/work/" + name)
				if err != nil || string(data) != "bar\nfoo\n" {
					t.Fatalf("%s: %q %v", name, data, err)
				}
				info, _ := fs.Stat("/work/" + name)
				if info.Mode().Perm() != 0600 {
					t.Errorf("mode=%o", info.Mode().Perm())
				}
				_, err = fs.ReadFile("/work/" + name + ".bak")
				if err == nil {
					t.Error("unexpected backup")
				}
			}
		})
	}
}

func TestInPlaceSilentAndEmpty(t *testing.T) {
	for _, input := range []string{"foo\nbar\n", ""} {
		fs := memfs.New()
		_ = fs.WriteFile("/a", []byte(input), 0644)
		var out, stderr bytes.Buffer
		res := sed.New().Execute(context.Background(), []string{"sed", "-ni", "s/foo/baz/p", "/a"}, &command.Context{FS: fs, Cwd: "/", Stdout: &out, Stderr: &stderr})
		want := ""
		if input != "" {
			want = "baz\n"
		}
		data, _ := fs.ReadFile("/a")
		if res.ExitCode != 0 || out.Len() != 0 || string(data) != want {
			t.Fatalf("code=%d data=%q stderr=%q", res.ExitCode, data, stderr.String())
		}
	}
}

func TestInPlaceErrorsDoNotModifyInput(t *testing.T) {
	for _, args := range [][]string{{"-i", "s/[//", "/a"}, {"-i", "s/a/b/", "missing"}} {
		fs := memfs.New()
		_ = fs.WriteFile("/a", []byte("a\n"), 0644)
		var out, stderr bytes.Buffer
		res := sed.New().Execute(context.Background(), append([]string{"sed"}, args...), &command.Context{FS: fs, Cwd: "/", Stdout: &out, Stderr: &stderr})
		data, _ := fs.ReadFile("/a")
		if res.ExitCode == 0 || stderr.Len() == 0 || string(data) != "a\n" || out.Len() != 0 {
			t.Fatalf("args=%v code=%d data=%q stderr=%q", args, res.ExitCode, data, stderr.String())
		}
	}
}
