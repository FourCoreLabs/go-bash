package mktemp_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
)

// Behavioral cases ported from upstream mktemp.test.ts and mktemp-options.test.ts.
func TestUpstreamCreationParity(t *testing.T) {
	for _, tc := range []struct {
		script, pattern string
		dir, dry        bool
	}{
		{"mktemp", `^/tmp/tmp\.[0-9A-Za-z]{10}$`, false, false},
		{"mktemp -d", `^/tmp/tmp\.[0-9A-Za-z]{10}$`, true, false},
		{"mktemp bareXXXX", `^bare[0-9A-Za-z]{4}$`, false, false},
		{"mktemp --suffix=.txt fileXXXX", `^file[0-9A-Za-z]{4}\.txt$`, false, false},
		{"mktemp -t XXXX.log.XXXX", `^/tmp/XXXX\.log\.[0-9A-Za-z]{4}$`, false, false},
		{"mkdir /scratch; TMPDIR=/scratch mktemp -p ''", `^/scratch/tmp\.[0-9A-Za-z]{10}$`, false, false},
		{"mkdir /scratch; mktemp -dp/scratch", `^/scratch/tmp\.[0-9A-Za-z]{10}$`, true, false},
		{"TMPDIR=/ mktemp", `^/tmp\.[0-9A-Za-z]{10}$`, false, false},
		{"mktemp -u /missing/nameXXX", `^/missing/name[0-9A-Za-z]{3}$`, false, true},
		{"mkdir /scratch; TMPDIR=/scratch mktemp -t -p /tmp", `^/scratch/tmp\.[0-9A-Za-z]{10}$`, false, false},
		{"ln -s /tmp /link; mktemp -p /link", `^/link/tmp\.[0-9A-Za-z]{10}$`, false, false},
	} {
		t.Run(tc.script, func(t *testing.T) {
			b, err := gobash.New(gobash.BashOptions{})
			if err != nil {
				t.Fatal(err)
			}
			r, err := b.Exec(context.Background(), tc.script, gobash.ExecOptions{})
			name := strings.TrimSuffix(r.Stdout, "\n")
			if err != nil || r.ExitCode != 0 || r.Stderr != "" || !regexp.MustCompile(tc.pattern).MatchString(name) {
				t.Fatalf("%+v err=%v", r, err)
			}
			check, err := b.Exec(context.Background(), "stat -c '%a %s %F' '"+name+"'", gobash.ExecOptions{})
			if tc.dry {
				if check.ExitCode == 0 {
					t.Fatal("dry run created entry")
				}
				return
			}
			mode := "600"
			if tc.dir {
				mode = "700"
			}
			if err != nil || check.ExitCode != 0 || !strings.HasPrefix(check.Stdout, mode+" ") {
				t.Fatalf("stat=%+v err=%v", check, err)
			}
		})
	}
}

func TestUpstreamErrorsAndMetaFlags(t *testing.T) {
	for _, tc := range []struct{ script, stderr string }{
		{"mktemp -q bad.XX", "mktemp: too few X's in template 'bad.XX'\n"},
		{"mktemp -dp", "mktemp: option requires an argument -- 'p'\n"},
		{"mktemp -- --help", "mktemp: too few X's in template '--help'\n"},
		{"mktemp --bad --help", "mktemp: unrecognized option '--bad'\n"},
		{"mktemp -Zh", "mktemp: invalid option -- 'Z'\n"},
		{"mktemp aXXX bXXX", "mktemp: too many templates\n"},
		{"mktemp --suffix=.txt fooXXX.log", "mktemp: with --suffix, template 'fooXXX.log' must end in X\n"},
		{"mktemp --suffix=a/b fooXXX", "mktemp: invalid suffix 'a/b', contains directory separator\n"},
		{"mktemp -q /missing/fooXXX", ""},
	} {
		b, _ := gobash.New(gobash.BashOptions{})
		r, err := b.Exec(context.Background(), tc.script, gobash.ExecOptions{})
		if err != nil || r.ExitCode != 1 || r.Stdout != "" || r.Stderr != tc.stderr {
			t.Fatalf("%s: %+v err=%v", tc.script, r, err)
		}
	}
	b, _ := gobash.New(gobash.BashOptions{})
	for _, script := range []string{"mktemp --help --bad", "mktemp -dh", "mktemp chartXXXX --help"} {
		r, err := b.Exec(context.Background(), script, gobash.ExecOptions{})
		if err != nil || r.ExitCode != 0 || !strings.Contains(r.Stdout, "Usage: mktemp") {
			t.Fatalf("%+v %v", r, err)
		}
	}
	r, err := b.Exec(context.Background(), "f=$(mktemp); echo world > \"$f\"; cat \"$f\"", gobash.ExecOptions{})
	if err != nil || r.Stdout != "world\n" || r.ExitCode != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestLongTemplateAndUniquePrivateFiles(t *testing.T) {
	b, _ := gobash.New(gobash.BashOptions{})
	r, err := b.Exec(context.Background(), "mktemp -u /tmp/"+strings.Repeat("X", 70000), gobash.ExecOptions{})
	if err != nil || r.ExitCode != 0 || len(r.Stdout) != 70006 {
		t.Fatalf("long template: length=%d code=%d err=%v", len(r.Stdout), r.ExitCode, err)
	}
	seen := map[string]bool{}
	for range 50 {
		r, err := b.Exec(context.Background(), "mktemp /tmp/smallXXX", gobash.ExecOptions{})
		if err != nil || r.ExitCode != 0 || seen[r.Stdout] {
			t.Fatalf("duplicate/failure: %+v %v", r, err)
		}
		seen[r.Stdout] = true
		check, err := b.Exec(context.Background(), "stat -c '%a %s' "+strings.TrimSpace(r.Stdout), gobash.ExecOptions{})
		if err != nil || check.Stdout != "600 0\n" {
			t.Fatalf("not private empty: %+v %v", check, err)
		}
	}
}
