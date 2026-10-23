package gobash

// Dynamic scripts are preprocessed, not run recursively via Runner.Run.
// mvdan v3.13.1 Run resets exit/filename/expansion state and fires EXIT traps;
// reentering it from a handler is not a current-shell AST execution API.
// Instead eval receives serialized instrumented text, and source receives an
// ephemeral reader via the VFS OpenHandler. The native builtins retain scope,
// positional parameters, return, redirections, and exit semantics.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"

	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/parser"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type dynamicScripts struct {
	b       *Bash
	trip    func(*ExecutionLimitError) *ExecutionLimitError
	isFunc  func(string) bool
	mu      sync.Mutex
	next    uint64
	pending map[string]string
}

func (d *dynamicScripts) preprocess(src string) (string, error) {
	parsed, err := parser.ParseWithLimits(src, parser.Limits{
		MaxInputSize:   d.b.limits.MaxInputSize,
		MaxHeredocSize: d.b.limits.MaxHeredocSize,
	})
	if err != nil {
		return "", err
	}
	file := parsed.Origin
	instrumentLoops(file)
	rewriteProcInfo(file, d.b.procInfo.PID, d.b.procInfo.PPID)
	if err := enforceExpansionCaps(file, d.b.limits); err != nil {
		if limit, ok := err.(*ExecutionLimitError); ok {
			return "", d.trip(limit)
		}
		return "", err
	}
	var out bytes.Buffer
	if err := syntax.NewPrinter().Print(&out, file); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (d *dynamicScripts) call(ctx context.Context, args []string) ([]string, error) {
	if len(args) == 0 {
		return args, nil
	}
	// builtin bypasses CallHandler for its target; command does too. Unwrap
	// these ordinary forms to avoid leaving an instrumentation bypass.
	start := 0
	for start < len(args) && (args[start] == "builtin" || args[start] == "command") {
		start++
		if start < len(args) && args[start] == "--" {
			start++
		}
	}
	if start >= len(args) {
		return args, nil
	}
	name := args[start]
	// A declared function takes precedence over these command names. The
	// callback observes the live runner function table (including functions
	// declared earlier in this Exec), avoiding preprocessing shadow calls.
	if name == "trap" {
		// mvdan supports only EXIT and ERR callbacks (and trap - to clear).
		// Preprocess the callback text at registration, before its later
		// internal parse bypasses CallHandler. Leave listing/clear forms alone.
		if len(args) == start+3 && (args[start+2] == "EXIT" || args[start+2] == "ERR") && args[start+1] != "-" && args[start+1] != "" {
			text, err := d.preprocess(args[start+1])
			if err != nil {
				return nil, err
			}
			result := append([]string(nil), args...)
			result[start+1] = text
			return result, nil
		}
		return args, nil
	}
	if name != "eval" && name != "source" && name != "." {
		return args, nil
	}
	if d.isFunc != nil && d.isFunc(name) {
		return args, nil
	}
	if name != "eval" && len(args) <= start+1 {
		return args, nil
	}
	// Stack inspection is read-only and local to this goroutine. Counting
	// builtin ancestors is conservative (other wrapping builtins count too).
	if dynamicBuiltinDepth()+d.b.execDepth > d.b.limits.MaxSourceDepth {
		return nil, d.trip(&ExecutionLimitError{Limit: "MaxSourceDepth", Value: d.b.limits.MaxSourceDepth})
	}
	result := append([]string(nil), args...)
	if name == "eval" {
		// Bound the joined input BEFORE allocating it: per-argument string caps
		// alone do not bound eval's concatenation of many small arguments.
		n := 0
		for i, s := range args[start+1:] {
			if i > 0 {
				n++
			}
			if len(s) > d.b.limits.MaxInputSize-n {
				return nil, &ParseError{Msg: "eval input too large"}
			}
			n += len(s)
		}
		text, err := d.preprocess(strings.Join(args[start+1:], " "))
		if err != nil {
			return nil, err
		}
		return append(result[:start+1], text), nil
	}
	hc := interp.HandlerCtx(ctx)
	filename := args[start+1]
	resolved := gbfs.Resolve(hc.Dir, filename)
	// Search PATH in the VFS, never on the host. Native mvdan source's PATH
	// lookup uses host stat; giving it an absolute opaque name avoids that.
	if !strings.Contains(filename, "/") && hc.Env != nil {
		for dir := range strings.SplitSeq(hc.Env.Get("PATH").String(), ":") {
			candidate := gbfs.Resolve(hc.Dir, path.Join(dir, filename))
			if fi, err := d.b.fs.Stat(candidate); err == nil && !fi.IsDir() {
				resolved = candidate
				break
			}
		}
	}
	f, err := d.b.fs.OpenFile(resolved, os.O_RDONLY, 0)
	if err != nil {
		// Let the builtin produce its normal non-fatal file diagnostic. Absolute
		// VFS path prevents host PATH search selecting a different file.
		result[start+1] = resolved
		return result, nil
	}
	data, readErr := io.ReadAll(io.LimitReader(f, int64(d.b.limits.MaxInputSize)+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > d.b.limits.MaxInputSize {
		return nil, &ParseError{Msg: fmt.Sprintf("input too large: %d bytes (max %d)", len(data), d.b.limits.MaxInputSize)}
	}
	text, err := d.preprocess(string(data))
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	if d.pending == nil {
		d.pending = make(map[string]string)
	}
	d.next++
	token := fmt.Sprintf("/__gobash_dynamic_source__/%d", d.next)
	d.pending[token] = text
	d.mu.Unlock()
	result[start+1] = token
	return result, nil
}

func (d *dynamicScripts) open(_ context.Context, name string, flag int, _ os.FileMode) (io.ReadWriteCloser, error) {
	if flag != os.O_RDONLY {
		return nil, nil
	}
	d.mu.Lock()
	text, ok := d.pending[name]
	delete(d.pending, name)
	d.mu.Unlock()
	if !ok {
		return nil, nil
	}
	return &dynamicReader{Reader: strings.NewReader(text)}, nil
}

type dynamicReader struct{ *strings.Reader }

func (*dynamicReader) Close() error              { return nil }
func (*dynamicReader) Write([]byte) (int, error) { return 0, os.ErrPermission }

func dynamicBuiltinDepth() int {
	// Include the pending dynamic call itself, whose builtin frame has not
	// been entered yet. A growable buffer avoids silently saturating at 128.
	size := 64
	var pcs []uintptr
	var n int
	for {
		pcs = make([]uintptr, size)
		n = runtime.Callers(2, pcs)
		if n < size {
			break
		}
		size *= 2
	}
	depth := 1
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.Function, "interp.(*Runner).builtin") {
			depth++
		}
		if !more {
			break
		}
	}
	return depth
}
