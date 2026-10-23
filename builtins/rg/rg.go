// Package rg implements the `rg` (ripgrep) built-in subset (/ Wave D). Only the flags just-bash supports are implemented:
// Includes smartcase, ignore files, and common matching/traversal flags.
//
// The `--json` flag emits JSON Lines matching ripgrep's
// `begin`/`match`/`end`/`summary` shape.
package rg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	stdstrings "strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "rg [OPTIONS] PATTERN [PATH...]"
const helpText = `Usage: rg [OPTION]... PATTERNS [PATH]...
Recursively search the current directory for lines matching PATTERNS.

  -i, --ignore-case        ignore case distinctions
  -v, --invert-match       invert match
  -n, --line-number        print line numbers
  -N, --no-line-number     suppress line numbers
  -c, --count              count matching lines per file
  -l, --files-with-matches print only matching files
  -A NUM                   trailing context
  -B NUM                   leading context
  -C NUM                   surrounding context
  -e PATTERN               add pattern (may repeat)
  -t TYPE                  only search files of TYPE (e.g. py, go, md)
  -g GLOB                  include/exclude paths matching GLOB (! to negate)
      --hidden             include hidden files
      --no-ignore          do not honor .gitignore (we never do)
      --json               emit JSON Lines (ripgrep schema)`

// fileType maps a ripgrep TYPE alias to a set of filename globs.
// Subset matching what just-bash ships; not exhaustive ripgrep types.
var fileTypes = map[string][]string{
	"go":   {"*.go"},
	"py":   {"*.py"},
	"js":   {"*.js", "*.mjs", "*.cjs"},
	"ts":   {"*.ts", "*.tsx"},
	"json": {"*.json"},
	"md":   {"*.md", "*.markdown"},
	"yaml": {"*.yaml", "*.yml"},
	"toml": {"*.toml"},
	"sh":   {"*.sh", "*.bash"},
	"rs":   {"*.rs"},
	"c":    {"*.c", "*.h"},
	"cpp":  {"*.cpp", "*.cc", "*.cxx", "*.hpp", "*.hh"},
	"txt":  {"*.txt"},
	"html": {"*.html", "*.htm"},
	"css":  {"*.css"},
	"xml":  {"*.xml"},
}

type opts struct {
	smartCase, fixed, word, wholeLine, onlyMatching, quiet, noIgnore, follow bool
	patternFiles                                                             []string
	maxCount, maxDepth                                                       int
	replacement                                                              *string
	patterns                                                                 []string
	ignoreCase                                                               bool
	invert                                                                   bool
	lineNumber                                                               bool
	noLineNumber                                                             bool
	countOnly                                                                bool
	filesOnly                                                                bool
	after                                                                    int
	before                                                                   int
	types                                                                    []string
	globs                                                                    []string
	hidden                                                                   bool
	asJSON                                                                   bool
	// heading selects ripgrep's grouped layout (filename on its own
	// line, then `line:text`, then a blank separator). Real ripgrep
	// only uses it when stdout is a terminal; there is never a
	// terminal here, so the default is the pipe-friendly
	// `file:line:text` form and --heading opts back in.
	heading    bool
	headingSet bool
	// withFilename / noFilename mirror rg's -H / -I. When neither is
	// given the prefix is shown only if a directory or several paths
	// are searched.
	withFilename    bool
	withFilenameSet bool
	noFilename      bool
}

// New returns the rg command.
func New() command.Command { return command.Define("rg", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	// Line numbers are on only with -n. Real ripgrep enables them by
	// default when stdout is a terminal and suppresses them otherwise;
	// there is no terminal here, so the piped behavior is the default
	// and -n opts in. (Verified against ripgrep: `rg pat f`, `rg pat d/`
	// and `... | rg pat` all print no line numbers when piped.)
	o := opts{smartCase: true, maxDepth: 256}
	args = normalizeArgs(args)
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-i", a == "--ignore-case":
			o.ignoreCase = true
			o.smartCase = false
		case a == "-s", a == "--case-sensitive":
			o.ignoreCase, o.smartCase = false, false
		case a == "-S", a == "--smart-case":
			o.ignoreCase, o.smartCase = false, true
		case a == "-F", a == "--fixed-strings":
			o.fixed = true
		case a == "-w", a == "--word-regexp":
			o.word = true
		case a == "-x", a == "--line-regexp":
			o.wholeLine = true
		case a == "-o", a == "--only-matching":
			o.onlyMatching = true
		case a == "-q", a == "--quiet":
			o.quiet = true
		case a == "-L", a == "--follow":
			o.follow = true
		case a == "-f", a == "--file", a == "-m", a == "--max-count", a == "--max-depth", a == "-r", a == "--replace":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			switch a {
			case "-f", "--file":
				o.patternFiles = append(o.patternFiles, args[i])
			case "-r", "--replace":
				value := args[i]
				o.replacement = &value
			default:
				n, err := parseInt(args[i])
				if err != nil {
					return builtinutil.UsageError(c.Stderr, usage)
				}
				if a == "--max-depth" {
					o.maxDepth = n
				} else {
					o.maxCount = n
				}
			}
		case a == "-v", a == "--invert-match":
			o.invert = true
		case a == "-n", a == "--line-number":
			o.lineNumber = true
			o.noLineNumber = false
		case a == "-N", a == "--no-line-number":
			o.noLineNumber = true
			o.lineNumber = false
		case a == "-c", a == "--count":
			o.countOnly = true
		case a == "-l", a == "--files-with-matches":
			o.filesOnly = true
		case a == "-A", a == "--after-context":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseInt(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.after = n
		case a == "-B", a == "--before-context":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseInt(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.before = n
		case a == "-C", a == "--context":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseInt(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.before, o.after = n, n
		case a == "-e", a == "--regexp":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.patterns = append(o.patterns, args[i])
		case a == "-t", a == "--type":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.types = append(o.types, args[i])
		case a == "-g", a == "--glob":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.globs = append(o.globs, args[i])
		case a == "--heading":
			o.heading, o.headingSet = true, true
		case a == "--no-heading":
			o.heading, o.headingSet = false, true
		case a == "-H", a == "--with-filename":
			o.withFilename, o.withFilenameSet = true, true
		case a == "-I", a == "--no-filename":
			o.noFilename, o.withFilenameSet = true, true
		case a == "--hidden":
			o.hidden = true
		case a == "--no-ignore":
			o.noIgnore = true
		case a == "--json":
			o.asJSON = true
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			paths = append(paths, a)
		}
	}
run:
	if len(o.patterns) == 0 && len(o.patternFiles) == 0 {
		if len(paths) == 0 {
			return builtinutil.UsageError(c.Stderr, usage)
		}
		o.patterns = append(o.patterns, paths[0])
		paths = paths[1:]
	}
	// No explicit path. Real ripgrep reads stdin when stdin is not a
	// terminal and recurses from the cwd when it is. There is no
	// terminal here, so peek instead: if something was piped in, search
	// that; otherwise fall back to recursing from the cwd, which is
	// what `rg pattern` on its own is expected to do. A `-f -` pattern
	// file owns stdin, so it is never also searched.
	stdinOnly := false
	var stdinData []byte
	if len(paths) == 0 {
		patternsFromStdin := false
		for _, name := range o.patternFiles {
			if name == "-" {
				patternsFromStdin = true
			}
		}
		if c.Stdin != nil && !patternsFromStdin {
			br := bufio.NewReader(c.Stdin)
			if _, err := br.Peek(1); err == nil {
				stdinData, _ = io.ReadAll(br)
				stdinOnly = true
			}
		}
		if !stdinOnly {
			paths = []string{"."}
		}
	}

	for _, name := range o.patternFiles {
		var data []byte
		var err error
		if name == "-" {
			if c.Stdin != nil {
				data, err = io.ReadAll(c.Stdin)
			}
		} else {
			data, err = c.FS.ReadFile(builtinutil.ResolvePath(c.Cwd, name))
		}
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "rg", 2, "%s: No such file or directory", name)
		}
		for line := range stdstrings.SplitSeq(string(data), "\n") {
			if line != "" {
				o.patterns = append(o.patterns, line)
			}
		}
	}
	if len(o.patterns) == 0 {
		return command.Result{ExitCode: 1}
	}
	if o.smartCase {
		o.ignoreCase = true
		for _, p := range o.patterns {
			if stdstrings.ContainsAny(p, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
				o.ignoreCase = false
				break
			}
		}
	}
	flags := ""
	if o.ignoreCase {
		flags = "(?i)"
	}
	patterns := append([]string(nil), o.patterns...)
	if o.fixed {
		for i := range patterns {
			patterns[i] = regexp.QuoteMeta(patterns[i])
		}
	}
	pat := "(?:" + stdstrings.Join(patterns, "|") + ")"
	if o.wholeLine {
		pat = "^(?:" + pat + ")$"
	}
	if o.word {
		pat = `\b(?:` + pat + `)\b`
	}
	re, err := regexp.Compile(flags + pat)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "rg", 2, "regex: %v", err)
	}

	// Type and glob filters.
	var typeGlobs []string
	for _, t := range o.types {
		if pats, ok := fileTypes[t]; ok {
			typeGlobs = append(typeGlobs, pats...)
		}
	}

	var includes, excludes []string
	for _, g := range o.globs {
		if stdstrings.HasPrefix(g, "!") {
			excludes = append(excludes, g[1:])
		} else {
			includes = append(includes, g)
		}
	}

	var files []string
	// searchedDir records whether any argument named a directory. It,
	// not the resulting file count, drives the filename prefix: ripgrep
	// labels every line when it was asked to search a tree, even if the
	// tree happens to hold a single file.
	searchedDir := false
	if stdinOnly {
		files = []string{"-"}
	} else {
		for _, p := range paths {
			if fi, err := c.FS.Stat(builtinutil.ResolvePath(c.Cwd, p)); err == nil && fi.IsDir() {
				searchedDir = true
			}
			collect(c, p, p, &o, &files)
		}
		sort.Strings(files)
	}

	// Filename prefix default: on when a directory was searched or more
	// than one path was given, off for a single explicit file and for
	// stdin. -H / -I override.
	if !o.withFilenameSet {
		o.withFilename = searchedDir || len(paths) > 1
	}
	if o.noFilename {
		o.withFilename = false
	}
	// Grouped headings only make sense when filenames are shown at all.
	if !o.headingSet {
		o.heading = false
	}

	anyMatch := false
	for _, f := range files {
		base := path.Base(f)
		if len(typeGlobs) > 0 {
			ok := false
			for _, g := range typeGlobs {
				if m, _ := path.Match(g, base); m {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if len(includes) > 0 {
			ok := false
			for _, g := range includes {
				if m, _ := path.Match(g, base); m {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		skip := false
		for _, g := range excludes {
			if m, _ := path.Match(g, base); m {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if searchFile(c, &o, re, f, stdinData) {
			anyMatch = true
			if o.quiet {
				break
			}
		}
	}
	if anyMatch {
		return command.Result{ExitCode: 0}
	}
	return command.Result{ExitCode: 1}
}

func collect(c *command.Context, abs, display string, o *opts, out *[]string) {
	abs = builtinutil.ResolvePath(c.Cwd, abs)
	fi, err := c.FS.Stat(abs)
	if err != nil {
		return
	}
	if !fi.IsDir() {
		*out = append(*out, display)
		return
	}
	walk(c, abs, display, o, 0, map[string]bool{}, out)
}

func walk(c *command.Context, abs, display string, o *opts, depth int, active map[string]bool, out *[]string) {
	if depth >= o.maxDepth {
		return
	}
	identity, err := c.FS.Realpath(abs)
	if err != nil || active[identity] {
		return
	}
	active[identity] = true
	defer delete(active, identity)
	entries, err := c.FS.ReadDir(abs)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		childAbs, childDisp := path.Join(abs, name), path.Join(display, name)
		fi, err := c.FS.Lstat(childAbs)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if !o.follow {
				continue
			}
			fi, err = c.FS.Stat(childAbs)
			if err != nil {
				continue
			}
		}
		ignored, whitelisted := ignoreStatus(c, childAbs, fi.IsDir(), o.noIgnore)
		if ignored || (!o.hidden && stdstrings.HasPrefix(name, ".") && !whitelisted) {
			continue
		}
		if fi.IsDir() {
			walk(c, childAbs, childDisp, o, depth+1, active, out)
		} else if fi.Mode().IsRegular() {
			*out = append(*out, childDisp)
		}
	}
}

func searchFile(c *command.Context, o *opts, re *regexp.Regexp, name string, stdin []byte) bool {
	var data []byte
	if name == "-" {
		data = stdin
	} else {
		abs := builtinutil.ResolvePath(c.Cwd, name)
		var err error
		data, err = c.FS.ReadFile(abs)
		if err != nil {
			return false
		}
	}
	lines := splitLines(data)

	display := name
	if name == "-" {
		display = "<stdin>"
	}

	matchLines := make([]int, 0)
	matchSpans := make(map[int][][2]int)
	for idx, line := range lines {
		spans := re.FindAllStringIndex(line, -1)
		has := spans != nil
		if o.invert {
			has = !has
		}
		if has {
			matchLines = append(matchLines, idx)
			conv := make([][2]int, 0, len(spans))
			for _, s := range spans {
				conv = append(conv, [2]int{s[0], s[1]})
			}
			matchSpans[idx] = conv
			if o.quiet {
				return true
			}
			if o.maxCount > 0 && len(matchLines) >= o.maxCount {
				break
			}
		}
	}

	if o.asJSON {
		emitJSON(c.Stdout, name, lines, matchLines, matchSpans, o)
		return len(matchLines) > 0
	}

	if o.filesOnly {
		if len(matchLines) > 0 {
			_, _ = fmt.Fprintf(c.Stdout, "%s\n", display)
		}
		return len(matchLines) > 0
	}
	if o.countOnly {
		if o.withFilename {
			_, _ = fmt.Fprintf(c.Stdout, "%s:%d\n", display, len(matchLines))
		} else {
			_, _ = fmt.Fprintf(c.Stdout, "%d\n", len(matchLines))
		}
		return len(matchLines) > 0
	}
	if len(matchLines) == 0 {
		return false
	}

	// linePrefix is what precedes each emitted line. In the grouped
	// (--heading) layout the filename is printed once above the block;
	// otherwise it rides on every line, which is what ripgrep does
	// whenever stdout is not a terminal.
	linePrefix := ""
	if o.heading {
		_, _ = fmt.Fprintf(c.Stdout, "%s\n", display)
	} else if o.withFilename {
		linePrefix = display + ":"
	}

	// Determine printed-line set with before/after context.
	printed := make(map[int]bool)
	for _, l := range matchLines {
		for k := l - o.before; k <= l+o.after; k++ {
			if k >= 0 && k < len(lines) {
				printed[k] = true
			}
		}
	}
	keys := make([]int, 0, len(printed))
	for k := range printed {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	prev := -2
	for _, k := range keys {
		if prev >= 0 && k > prev+1 && (o.before > 0 || o.after > 0) {
			_, _ = io.WriteString(c.Stdout, "--\n")
		}
		isMatch := matchSpans[k] != nil
		text := lines[k]
		if o.onlyMatching && isMatch {
			for _, span := range matchSpans[k] {
				if span[0] == span[1] {
					continue
				}
				part := text[span[0]:span[1]]
				if o.replacement != nil {
					part = re.ReplaceAllString(part, *o.replacement)
				}
				if o.lineNumber && !o.noLineNumber {
					_, _ = fmt.Fprintf(c.Stdout, "%s%d:%s\n", linePrefix, k+1, part)
				} else {
					_, _ = fmt.Fprintf(c.Stdout, "%s%s\n", linePrefix, part)
				}
			}
			continue
		}
		if o.replacement != nil && isMatch {
			text = re.ReplaceAllString(text, *o.replacement)
		}
		sep := "-"
		if isMatch {
			sep = ":"
		}
		if o.lineNumber && !o.noLineNumber {
			_, _ = fmt.Fprintf(c.Stdout, "%s%d%s%s\n", linePrefix, k+1, sep, text)
		} else {
			_, _ = fmt.Fprintf(c.Stdout, "%s%s\n", linePrefix, text)
		}
		prev = k
	}
	// The blank separator between files belongs to the grouped layout
	// only; in the flat layout it would be spurious output that breaks
	// `rg ... | wc -l`.
	if o.heading {
		_, _ = io.WriteString(c.Stdout, "\n")
	}
	return true
}

// emitJSON emits ripgrep-shape JSON Lines: begin, match*, end, summary.
func emitJSON(w io.Writer, name string, lines []string, matchLines []int, spans map[int][][2]int, o *opts) {
	start := time.Now()
	// begin
	encode(w, map[string]any{
		"type": "begin",
		"data": map[string]any{
			"path": map[string]any{"text": name},
		},
	})
	matched := 0
	for _, idx := range matchLines {
		matched++
		ms := spans[idx]
		subs := make([]map[string]any, 0, len(ms))
		for _, s := range ms {
			subs = append(subs, map[string]any{
				"match": map[string]any{"text": lines[idx][s[0]:s[1]]},
				"start": s[0],
				"end":   s[1],
			})
		}
		// Encode lines as text when valid UTF-8; otherwise base64 (rg's shape).
		text := lines[idx] + "\n"
		var linesField map[string]any
		if valid := stdstrings.ToValidUTF8(text, "") == text; valid {
			linesField = map[string]any{"text": text}
		} else {
			linesField = map[string]any{"bytes": base64.StdEncoding.EncodeToString([]byte(text))}
		}
		encode(w, map[string]any{
			"type": "match",
			"data": map[string]any{
				"path":            map[string]any{"text": name},
				"lines":           linesField,
				"line_number":     idx + 1,
				"absolute_offset": 0,
				"submatches":      subs,
			},
		})
	}
	// end
	dur := time.Since(start)
	encode(w, map[string]any{
		"type": "end",
		"data": map[string]any{
			"path": map[string]any{"text": name},
			"stats": map[string]any{
				"elapsed":             map[string]any{"secs": int(dur.Seconds()), "nanos": int(dur.Nanoseconds() % 1e9), "human": dur.String()},
				"searches":            1,
				"searches_with_match": boolToInt(matched > 0),
				"bytes_searched":      sumBytes(lines),
				"bytes_printed":       0,
				"matched_lines":       matched,
				"matches":             matched,
			},
		},
	})
	// summary (one per file in our simplified shape)
	encode(w, map[string]any{
		"type": "summary",
		"data": map[string]any{
			"elapsed_total": map[string]any{"secs": int(dur.Seconds()), "nanos": int(dur.Nanoseconds() % 1e9), "human": dur.String()},
			"stats": map[string]any{
				"matched_lines":       matched,
				"matches":             matched,
				"searches":            1,
				"searches_with_match": boolToInt(matched > 0),
			},
		},
	})
	_ = o
}

func encode(w io.Writer, v any) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	_, _ = w.Write(buf.Bytes())
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sumBytes(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := stdstrings.TrimSuffix(string(data), "\n")
	if s == "" {
		return []string{""}
	}
	return stdstrings.Split(s, "\n")
}

func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid number: %q", s)
	}
	return n, nil
}

// Normalize attached values and short flag bundles before parsing.
func normalizeArgs(args []string) []string {
	out := []string{args[0]}
	values := "ABCetgfm r"
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if stdstrings.HasPrefix(a, "--") {
			if key, value, ok := stdstrings.Cut(a, "="); ok {
				out = append(out, key, value)
			} else {
				out = append(out, a)
			}
		} else if stdstrings.HasPrefix(a, "-") && len(a) > 2 {
			for j := 1; j < len(a); j++ {
				out = append(out, "-"+a[j:j+1])
				if stdstrings.ContainsRune(values, rune(a[j])) {
					if j+1 < len(a) {
						out = append(out, a[j+1:])
					}
					break
				}
			}
		} else {
			out = append(out, a)
		}
		// A separate option value must not itself be normalized.
		if len(out) > 0 && (stdstrings.Contains(values, stdstrings.TrimPrefix(out[len(out)-1], "-")) && len(out[len(out)-1]) == 2 || out[len(out)-1] == "--file" || out[len(out)-1] == "--replace" || out[len(out)-1] == "--max-count" || out[len(out)-1] == "--max-depth" || out[len(out)-1] == "--regexp" || out[len(out)-1] == "--glob" || out[len(out)-1] == "--type") && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
