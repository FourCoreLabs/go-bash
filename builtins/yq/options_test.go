package yq_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/yq"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestCommonFlags(t *testing.T) {
	tests := []struct {
		name, input string
		args        []string
		want        string
		code        int
	}{
		{"slurp yaml", "a: 1\n---\na: 2\n", []string{"-s", "-o", "json", "-c", "map(.a)"}, "[1,2]\n", 0},
		{"slurp json", "1\n2\n", []string{"--slurp", "--input-format=json", "--output-format=json", "add"}, "3\n", 0},
		{"slurp empty", "", []string{"-s", "-o", "json", "-c", "."}, "[]\n", 0},
		{"null ignores invalid input", "[broken", []string{"--null-input", "-o", "json", "-c", "{a: 1}"}, "{\"a\":1}\n", 0},
		{"null takes precedence over slurp", "a: 1", []string{"-ns", "-o", "json", "."}, "null\n", 0},
		{"null skips missing file", "", []string{"-n", "-o", "json", "true", "/missing"}, "true\n", 0},
		{"exit false", "false", []string{"-e", "."}, "false\n", 1},
		{"exit null", "null", []string{"--exit-status", "."}, "null\n", 1},
		{"exit empty", "a: 1", []string{"-e", "empty"}, "", 1},
		{"exit empty input", "", []string{"-e", "."}, "", 1},
		{"exit zero truthy", "0", []string{"-e", "."}, "0\n", 0},
		{"exit empty array truthy", "[]", []string{"-e", "."}, "[]\n", 0},
		{"exit any truthy upstream semantics", "", []string{"-ne", "-o", "json", "true, false, null"}, "true\nfalse\nnull\n", 0},
		{"join raw", "", []string{"-nrj", "-o", "json", "\"a\", \"b\""}, "ab", 0},
		{"join does not imply raw", "", []string{"-n", "--join-output", "-o", "json", "\"a\", \"b\""}, "\"a\"\"b\"", 0},
		{"join keeps string newlines", "", []string{"-nrj", "-o", "json", "\"a\\n\", \"b\""}, "a\nb", 0},
		{"join yaml", "", []string{"-nj", "1, 2"}, "12", 0},
		{"join pretty json", "", []string{"-nj", "-o", "json", "{a: 1}"}, "{\n  \"a\": 1\n}", 0},
		{"input only long", "{\"a\":1}", []string{"--input-format", "json", "."}, "a: 1\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err, code := run(t, tt.input, tt.args...)
			if out != tt.want || err != "" || code != tt.code {
				t.Fatalf("got (%q, %q, %d), want (%q, empty stderr, %d)", out, err, code, tt.want, tt.code)
			}
		})
	}
}

func TestInplace(t *testing.T) {
	for _, flag := range []string{"-i", "--inplace", "--in-place", "-ic"} {
		t.Run(flag, func(t *testing.T) {
			fs := memfs.New()
			if err := fs.MkdirAll("/work", 0755); err != nil {
				t.Fatal(err)
			}
			if err := fs.WriteFile("/work/data.yaml", []byte("a: 1\n"), 0640); err != nil {
				t.Fatal(err)
			}
			if err := fs.WriteFile("/work/other.yaml", []byte("a: 9\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			c := &command.Context{FS: fs, Cwd: "/work", Stdin: strings.NewReader("a: 99"), Stdout: &out, Stderr: &stderr}
			res := yq.New().Execute(context.Background(), []string{"yq", flag, ".a += 1", "data.yaml", "other.yaml"}, c)
			data, err := fs.ReadFile("/work/data.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if res.ExitCode != 0 || out.Len() != 0 || stderr.Len() != 0 || string(data) != "a: 2\n" {
				t.Fatalf("result=%+v stdout=%q stderr=%q file=%q", res, out.String(), stderr.String(), data)
			}
			info, _ := fs.Stat("/work/data.yaml")
			if info.Mode().Perm() != 0640 {
				t.Fatalf("mode=%o", info.Mode().Perm())
			}
			other, _ := fs.ReadFile("/work/other.yaml")
			if string(other) != "a: 9\n" {
				t.Fatalf("second file changed: %q", other)
			}

			// Evaluation and serialization errors must not truncate the source.
			for _, args := range [][]string{{"yq", "-i", ".a | error(\"bad\")", "data.yaml"}, {"yq", "-i", "-o", "toml", ".a", "data.yaml"}, {"yq", "-i", ".[", "data.yaml"}} {
				out.Reset()
				stderr.Reset()
				res = yq.New().Execute(context.Background(), args, c)
				data, _ = fs.ReadFile("/work/data.yaml")
				if res.ExitCode == 0 || out.Len() != 0 || string(data) != "a: 2\n" {
					t.Fatalf("failed command changed file: args=%v result=%+v data=%q", args, res, data)
				}
			}
		})
	}
}

func TestInplaceRequiresFile(t *testing.T) {
	for _, args := range [][]string{{"-i", "."}, {"--inplace", ".", "-"}, {"-ni", "true"}} {
		out, err, code := run(t, "a: 1", args...)
		if code != 1 || out != "" || !strings.Contains(err, "requires a file argument") {
			t.Fatalf("args=%v got (%q,%q,%d)", args, out, err, code)
		}
	}
}

func TestSlurpSeparateFiles(t *testing.T) {
	fs := memfs.New()
	_ = fs.WriteFile("/a.yaml", []byte("a: 1"), 0644)
	_ = fs.WriteFile("/b.yaml", []byte("a: 2"), 0644)
	var out, stderr bytes.Buffer
	res := yq.New().Execute(context.Background(), []string{"yq", "-sc", "-o", "json", "map(.a)", "/a.yaml", "/b.yaml"}, &command.Context{FS: fs, Cwd: "/", Stdout: &out, Stderr: &stderr})
	if res.ExitCode != 0 || out.String() != "[1]\n" || stderr.Len() != 0 {
		t.Fatalf("result=%+v stdout=%q stderr=%q", res, out.String(), stderr.String())
	}
}
