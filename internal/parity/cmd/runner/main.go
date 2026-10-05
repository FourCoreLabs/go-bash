// Command runner is a JSON adapter for the opt-in upstream differential suite.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
)

type step struct {
	Script     string            `json:"script"`
	Env        map[string]string `json:"env"`
	ReplaceEnv bool              `json:"replaceEnv"`
	Cwd        string            `json:"cwd"`
	Stdin      string            `json:"stdin"`
	Args       []string          `json:"args"`
	RawScript  bool              `json:"rawScript"`
}
type scenario struct {
	ID    string            `json:"id"`
	Files map[string]string `json:"files"`
	Env   map[string]string `json:"env"`
	Steps []step            `json:"steps"`
	// Consumed by the comparator, not the execution adapter.
	ExpectedMismatch json.RawMessage `json:"expectedMismatch"`
}
type entry struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Target  string `json:"target,omitempty"`
}
type result struct {
	Stdout   string           `json:"stdout"`
	Stderr   string           `json:"stderr"`
	ExitCode int              `json:"exitCode"`
	Files    map[string]entry `json:"files"`
}

func run(s scenario) ([]result, error) {
	files := map[string]gbfs.FileInit{}
	for p, text := range s.Files {
		files[p] = gbfs.FileInit{Content: []byte(text)}
	}
	b, err := gobash.New(gobash.BashOptions{Cwd: "/work", Files: files, Env: s.Env})
	if err != nil {
		return nil, err
	}
	out := []result{}
	for _, st := range s.Steps {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		r, err := b.Exec(ctx, st.Script, gobash.ExecOptions{Env: st.Env, ReplaceEnv: st.ReplaceEnv, Cwd: st.Cwd, Stdin: strings.NewReader(st.Stdin), Args: st.Args, RawScript: st.RawScript})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("%s: Exec: %w", s.ID, err)
		}
		snapshot := map[string]entry{}
		var walk func(string) error
		walk = func(p string) error {
			info, err := b.FS().Lstat(p)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := b.FS().Readlink(p)
				if err != nil {
					return err
				}
				snapshot[p] = entry{Type: "symlink", Target: target}
			} else if info.IsDir() {
				snapshot[p] = entry{Type: "directory"}
				children, err := b.FS().ReadDir(p)
				if err != nil {
					return err
				}
				for _, child := range children {
					if err := walk(path.Join(p, child.Name())); err != nil {
						return err
					}
				}
			} else {
				data, err := b.FS().ReadFile(p)
				if err != nil {
					return err
				}
				snapshot[p] = entry{Type: "file", Content: base64.StdEncoding.EncodeToString(data)}
			}
			return nil
		}
		if err := walk("/work"); err != nil {
			return nil, err
		}
		out = append(out, result{r.Stdout, r.Stderr, r.ExitCode, snapshot})
	}
	return out, nil
}
func main() {
	dec := json.NewDecoder(os.Stdin)
	dec.DisallowUnknownFields()
	var cases []scenario
	if err := dec.Decode(&cases); err != nil {
		fatal(err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		fatal(fmt.Errorf("trailing JSON: %v", err))
	}
	out := map[string][]result{}
	for _, s := range cases {
		r, err := run(s)
		if err != nil {
			fatal(err)
		}
		out[s.ID] = r
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
