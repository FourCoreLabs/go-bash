package gobash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/command"
	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/memfs"
	gbalias "github.com/mark3labs/go-bash/internal/alias"
	"github.com/mark3labs/go-bash/internal/ringbuf"
	"github.com/mark3labs/go-bash/internal/runtimestate"
	bashinterp "github.com/mark3labs/go-bash/interp"
	"github.com/mark3labs/go-bash/network"
	"github.com/mark3labs/go-bash/parser"
	"github.com/mark3labs/go-bash/transform"
)

// Bash is a reusable shell environment. Concurrent Exec calls on a single
// *Bash are safe; they serialize on an internal mutex.
//
// # Phase 5 scope
//
// Parsing now goes through gobash/parser.Parse (which already enforces
// the §4.2 parser-side hard limits), and the mvdan/sh runner is built
// by gobash/interp.BuildRunner — moving the VFS handlers, the
// commandExecHandler middleware, and the runner.Dir-not-interp.Dir
// quirk out of bash.go and into the bridge package. Bash.Exec still
// owns: per-call limit state (CallHandler closure + MaxOutputSize
// ringbuf), env-mutation propagation across Exec calls, the
// loop-sentinel AST rewrite, and the final result shape.
//
// Still missing (per build order): command registry (Phase 8),
// network (Phase 9), built-in commands (Phases 10–11). Until the
// registry lands, the commandExecHandler stub in the interp package
// falls through to mvdan/sh's DefaultExecHandler, which means unknown
// commands still reach the host via os/exec. That is the explicit
// gap Phase 8 closes.
type Bash struct {
	mu sync.Mutex

	env      map[string]string
	cwd      string
	limits   ResolvedLimits
	sleep    SleepFunc
	logger   Logger
	trace    TraceFunc
	procInfo ProcessInfo

	// shellOptions are applied to the interpreter at construction time in
	// `set` syntax (see BashOptions.ShellOptions).
	shellOptions []string

	// fs is the virtual filesystem backing every script-side file
	// operation. Phase 3 wires this up via BashOptions.{FS, Files};
	// the open/stat/readdir handlers passed to mvdan/sh during Exec
	// route through this field so the host disk is never touched.
	fs gbfs.FileSystem

	// registry is the command dispatch table consulted by the
	// interp.ExecHandlers middleware. Phase 8 plumbs it in;
	// CustomCommands land first (so they win over later built-in
	// registrations), Phase 10 will register filtered built-ins on
	// top. The registry is also the source of truth for the spec
	// /bin/X stub list.
	registry *command.Registry

	// fetch is the network Doer resolved at New() time from
	// BashOptions.Fetch (custom) or BashOptions.Network (config). It
	// is nil when neither was supplied — network-touching commands
	// MUST handle nil gracefully (see command.Context.Fetch doc).
	// Phase 9 wires this field; Phase 10's `curl` is the first
	// consumer.
	fetch network.Doer

	// aliases is the per-Bash alias table consulted by the Phase 10
	// Wave G `alias` / `unalias` built-ins (and, once Phase 11 lands,
	// by the alias-expansion path at parse time). Initialized to a
	// fresh empty table in New().
	aliases *runtimestate.AliasTable

	// history is the per-Bash command history ring for the Phase 10
	// Wave G `history` built-in. Bounded at runtimestate.DefaultHistorySize
	// (500 entries).
	history *runtimestate.HistoryRing

	// shopt is the per-Bash shopt table consulted by the `shopt`
	// builtin (Phase 11) and by the alias-expansion path at parse
	// time.
	shopt *runtimestate.ShoptTable

	// execDepth is the source-depth of the CURRENT in-flight Exec
	// (and of every recursive subExec running on this Bash). It is
	// surfaced to commands via command.Context.SourceDepth so
	// source / eval / . / bash / sh / timeout can enforce
	// Limits.MaxSourceDepth across nested invocations.
	execDepth int

	// plugins is the ordered transform-pipeline plugin slice consulted
	// by every Exec. Built from BashOptions.TransformPlugins at New
	// time and extendable via RegisterTransformPlugin. The slice is
	// guarded by b.mu; reads inside execLocked run with the lock
	// already held.
	plugins []transform.Plugin

	// funcs/exported (Phase 5), jsBoot/invoke (Phase 15) are added
	// when their owning phase lands. The full target field set is
	// frozen in the spec
}

// loopSentinelName is the literal command we inject at the start of every
// loop body's Do block during Exec. The CallHandler intercepts it to bump
// the loop-iteration counter without counting it as a real command, and
// rewrites the args to `:` (a no-op builtin) so it never touches the
// command registry or user-visible output.
//
// The name must not collide with any real or user-defined command. The
// double-underscore prefix and the project namespace make a collision in
// practice impossible.
const loopSentinelName = "__gobash_loop_iter__"

// New constructs a Bash environment from the supplied options. The zero
// value of BashOptions is valid and yields sane defaults; New returns a
// non-nil error only when option validation fails (no failure modes exist
// in Phases 1–2).
func New(opts BashOptions) (*Bash, error) {
	limits := ResolveLimits(opts.ExecutionLimits)
	// Deprecated non-zero knobs take precedence over ExecutionLimits, as upstream.
	// Zero cannot express an override in the legacy Go value fields; use a pointer.
	if opts.MaxCallDepth != 0 {
		limits.MaxCallDepth = opts.MaxCallDepth
	}
	if opts.MaxCommandCount != 0 {
		limits.MaxCommandCount = opts.MaxCommandCount
	}
	if opts.MaxLoopIterations != 0 {
		limits.MaxLoopIterations = opts.MaxLoopIterations
	}
	if err := validateAPILimits(limits); err != nil {
		return nil, err
	}
	b := &Bash{
		env:          cloneEnv(opts.Env),
		cwd:          opts.Cwd,
		limits:       limits,
		shellOptions: opts.ShellOptions,
		sleep:        opts.Sleep,
		logger:       opts.Logger,
		trace:        opts.Trace,
		aliases:      runtimestate.NewAliasTable(),
		history:      runtimestate.NewHistoryRing(0),
		shopt:        runtimestate.NewShoptTable(),
		plugins:      append([]transform.Plugin(nil), opts.TransformPlugins...),
	}
	if opts.ProcessInfo != nil {
		b.procInfo = *opts.ProcessInfo
	} else {
		b.procInfo = defaultProcessInfo()
	}
	// FS initialization: caller-supplied FS wins; otherwise spin up an
	// empty in-memory FS. Files (when present) are seeded into whatever
	// FS we end up with.
	if opts.FS != nil {
		b.fs = opts.FS
	} else {
		m := memfs.New()
		if err := m.SetMaxBytes(int64(limits.MaxFileSystemBytes)); err != nil {
			return nil, err
		}
		b.fs = m
	}
	// /dev/null must discard writes and read as empty for every
	// consumer — shell redirections and built-ins alike. This is
	// independent of the default layout below: the wrapper matches on
	// path, so /dev/null works even when the caller manages its own
	// layout and no /dev directory exists.
	b.fs = gbfs.WithNullDevice(b.fs)
	// Command registry. CustomCommands register first so name
	// collisions with later (Phase 10) built-in registrations resolve
	// in favor of the custom entry — the built-in bootstrap will skip
	// names already present via Registry.Has. The BashOptions.Commands
	// filter applies only to the built-in registration loop; custom
	// entries are never filtered.
	b.registry = command.NewRegistry()
	for _, c := range opts.CustomCommands {
		b.registry.Register(c)
	}
	// Phase 10 built-in registration. The default built-in slice is
	// populated by side-effect imports of every builtins/<name>/
	// package (the meta-package github.com/mark3labs/go-bash/builtins
	// imports them all; gobash root pulls that in via blank import).
	// We honor BashOptions.Commands as an allow-list (nil = all
	// built-ins; non-nil = only the named subset) and skip any name
	// already registered — customs registered above always win.
	var allow map[command.Name]bool
	if opts.Commands != nil {
		allow = make(map[command.Name]bool, len(opts.Commands))
		for _, n := range opts.Commands {
			allow[n] = true
		}
	}
	for _, c := range command.DefaultBuiltins() {
		if allow != nil && !allow[c.Name()] {
			continue
		}
		if b.registry.Has(string(c.Name())) {
			continue
		}
		b.registry.Register(c)
	}

	// Network Doer resolution. BashOptions.Fetch is the
	// strict override — when supplied, BashOptions.Network is
	// ignored even if non-nil. When Fetch is nil and Network is
	// non-nil we materialize a SecureFetch from the config. When
	// both are nil we leave b.fetch nil so the dispatch Context's
	// Fetch field stays nil, signaling "network disabled" to
	// downstream commands.
	if opts.Fetch != nil {
		b.fetch = opts.Fetch
	} else if opts.Network != nil {
		b.fetch = network.NewSecureFetch(opts.Network)
	}

	// The spec default filesystem layout. Only applied when the caller
	// supplied neither Cwd nor Files — either signal is read as
	// "I'm managing my own filesystem layout, don't preload anything."
	// FS supplied alone is fine: the layout is written onto whatever
	// FileSystem we ended up with above. Errors are deliberately
	// non-fatal: a read-only or restricted FileSystem may reject some
	// of the writes, but New should still succeed so the caller can
	// inspect / repair the FS post-construction.
	if len(opts.Files) > 0 {
		if err := seedFiles(b.fs, opts.Files); err != nil {
			return nil, err
		}
	}
	useDefaultLayout := opts.Cwd == "" && len(opts.Files) == 0
	if useDefaultLayout {
		_ = applyDefaultLayout(b.fs, b.procInfo, b.registry)
		// Seed $HOME and $PATH only when the caller didn't already
		// supply them. User-supplied values always win — the layout's
		// /home/user directory is harmless if HOME points elsewhere.
		if _, ok := b.env["HOME"]; !ok {
			b.env["HOME"] = "/home/user"
		}
		if _, ok := b.env["PATH"]; !ok {
			b.env["PATH"] = "/usr/bin:/bin"
		}
	}
	// Default Cwd: "/home/user" if neither Cwd nor Files was set; "/"
	// otherwise. Matches the spec
	if b.cwd == "" {
		if len(opts.Files) == 0 {
			b.cwd = "/home/user"
		} else {
			b.cwd = "/"
		}
	}
	// Ensure the cwd actually exists in the VFS — mvdan/sh stat-checks
	// the Dir at runner construction time, and the default "/home/user"
	// would otherwise fail. Errors here are non-fatal: a custom FS might
	// be read-only on purpose, in which case the user must pre-create
	// the cwd themselves.
	if b.cwd != "" {
		_ = b.fs.MkdirAll(b.cwd, 0o755)
	}
	return b, nil
}

// FS returns the virtual filesystem this Bash is bound to. Useful for
// host-side inspection or post-Exec assertions in tests.
func (b *Bash) FS() gbfs.FileSystem { return gbfs.UnwrapNullDevice(b.fs) }

// Registry returns the command dispatch registry. The returned
// pointer is the live registry consulted by every Exec call; mutating
// it between calls is supported (additional Register calls will be
// honored by subsequent Execs). Concurrent Register calls are NOT
// safe — if a host needs that, wrap your own mutex around the
// Register sites.
func (b *Bash) Registry() *command.Registry { return b.registry }

// Aliases returns the live per-Bash alias table. Hosts can use this
// to seed aliases before running scripts; the `alias` / `unalias`
// built-ins read and mutate the same table.
func (b *Bash) Aliases() command.AliasTable { return b.aliases }

// History returns the live per-Bash command history ring. The
// runtime does NOT currently push parsed commands into the ring;
// hosts can populate it (or read from the `history` built-in's
// view) directly.
func (b *Bash) History() command.HistoryRing { return b.history }

// Shopt returns the live per-Bash shell-option table. Hosts can
// pre-seed options (e.g. `expand_aliases`) before running scripts;
// the `shopt` builtin reads and mutates the same table.
func (b *Bash) Shopt() command.ShoptTable { return b.shopt }

// RegisterTransformPlugin appends a transform-pipeline plugin to the
// per-Bash plugin slice. The plugin runs on every subsequent Exec call
// (parse → plugins → serialize → re-parse → run) and
// its metadata payload is surfaced in BashExecResult.Metadata under
// the plugin's Name().
//
// Calls to RegisterTransformPlugin acquire b.mu, so it is safe to call
// concurrently from outside Exec. Calling from inside a plugin's own
// Transform method or from a custom command's Run (which both hold
// b.mu) will deadlock — don't do that.
func (b *Bash) RegisterTransformPlugin(p transform.Plugin) {
	if p == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.plugins = append(b.plugins, p)
}

// seedFiles applies the BashOptions.Files map to the FS. memfs has a
// dedicated Seed method that handles lazy providers; for any other FS
// we apply the entries via the public FileSystem API and silently skip
// lazy entries with an error.
func seedFiles(target gbfs.FileSystem, files map[string]gbfs.FileInit) error {
	if m, ok := target.(*memfs.FS); ok {
		return m.Seed(files)
	}
	for p, init := range files {
		if err := gbfs.Validate(p); err != nil {
			return err
		}
		clean := gbfs.Clean(p)
		if init.Dir {
			mode := init.Mode
			if mode == 0 {
				mode = 0o755
			}
			if err := target.MkdirAll(clean, mode); err != nil {
				return err
			}
			continue
		}
		if init.Symlink != "" {
			if err := target.Symlink(init.Symlink, clean); err != nil {
				return err
			}
			continue
		}
		if init.Lazy != nil {
			return errors.New("gobash: lazy file providers require *memfs.FS")
		}
		if dir := gbfs.Dirname(clean); dir != "." && dir != "/" {
			if err := target.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		mode := init.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := target.WriteFile(clean, init.Content, mode); err != nil {
			return err
		}
	}
	return nil
}

// Exec parses and runs a bash script against this environment.
//
// A non-zero exit code is reported via BashExecResult.ExitCode and does
// NOT produce a non-nil error. The returned error is reserved for harness
// failures: parse errors (*ParseError), execution-limit overruns
// (*ExecutionLimitError), context cancellation (context.Canceled /
// context.DeadlineExceeded), and host-side I/O failures.
//
// Concurrent Exec calls on the same *Bash serialize via an internal mutex.
//
// Per the resolved decisions in the spec, background jobs (`&`) and `wait`
// run synchronously with virtual PIDs; `wait` is a no-op. Function
// definitions made inside a script live only for that Exec.
//
// The following limits from the spec are enforced in this phase:
// MaxCommandCount, MaxLoopIterations, MaxCallDepth, MaxOutputSize. The
// remaining limits land in their owning phases (Phase 4 for expansion
// caps, Phase 11 for source-depth via the source/. builtin, etc.).
func (b *Bash) Exec(ctx context.Context, script string, opts ExecOptions) (BashExecResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.execLocked(ctx, script, opts)
}

// execLocked is the body of Exec without the per-Bash mutex. It is
// called by Exec (which takes the lock) and by the sub-shell Exec
// closure plumbed through to commands via Context.Exec. The closure
// runs from inside an in-progress Exec call on the SAME goroutine,
// so the lock is already held — a recursive sync.Mutex.Lock here
// would deadlock. Phase 11 will revisit this once `source` / `eval`
// land and want sub-shell semantics richer than the Phase 10 Wave G
// `bash -c` form.
func (b *Bash) execLocked(ctx context.Context, script string, opts ExecOptions) (BashExecResult, error) {
	var result BashExecResult
	scope, nested := executionScopeFromContext(ctx)
	if !nested {
		scope = newExecutionScope()
		ctx = context.WithValue(ctx, executionScopeKey{}, scope)
	}

	if !opts.RawScript {
		script = normalizeAPIScript(script)
	}

	// The spec: when transform plugins are registered, run the
	// pipeline first. The pipeline parses, dispatches each plugin in
	// order, and re-serializes the post-transform AST. We then drop
	// the original script in favor of the transformed source and
	// re-parse below — the cheapest path that lets the limits and
	// instrumentation see the post-transform tree.
	var pluginMetadata map[string]any
	if len(b.plugins) > 0 {
		pipeline := transform.New()
		for _, p := range b.plugins {
			pipeline.Use(p)
		}
		tr, err := pipeline.Transform(script)
		if err != nil {
			return result, err
		}
		script = tr.Script
		pluginMetadata = tr.Metadata
	}

	// Parse via gobash/parser so the §4.2 hard limits (MaxInputSize,
	// MaxTokens, MaxParserDepth, MaxHeredocSize) are enforced before we
	// hand anything to mvdan/sh's interpreter. parser.Parse returns
	// *parser.ParseError on failure, which is aliased to gobash.ParseError.
	parsed, err := parser.ParseWithLimits(script, parser.Limits{
		MaxInputSize:   b.limits.MaxInputSize,
		MaxHeredocSize: b.limits.MaxHeredocSize,
	})
	if err != nil {
		return result, err
	}
	file := parsed.Origin

	// Phase 11 alias expansion: when `shopt expand_aliases` is on,
	// rewrite each simple command's first word against the alias
	// table. The pass runs after parse but before runtime so the
	// limits and instrumentation see the expanded form.
	if b.shopt.IsSet("expand_aliases") {
		gbalias.Expand(file, b.aliases.All())
	}

	// Instrument the AST so every loop body's Do block opens with our
	// sentinel call. The CallHandler intercepts the sentinel to bump
	// the loop-iteration counter and rewrite the args to `:` (no-op).
	instrumentLoops(file)

	// The spec: rewrite $$, $PPID, $BASHPID to the virtualized values
	// from procInfo. mvdan/sh hardcodes $$/$PPID to the host process's
	// real os.Getpid()/os.Getppid() — see procinfo.go for the full
	// rationale and the per-subshell BASHPID counter rules.
	rewriteProcInfo(file, b.procInfo.PID, b.procInfo.PPID)

	// The spec expansion-side runtime caps: bound brace expansion,
	// substitution depth, and literal array element counts before any
	// runtime allocation can balloon. enforceExpansionCaps splits
	// braces in-place, so subsequent runtime expansion is unaffected.
	if err := enforceExpansionCaps(file, b.limits); err != nil {
		return result, err
	}

	stdout, stderr, captureOut, captureErr, outBuf, errBuf := wireStdio(opts)
	stdin := opts.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}

	env := mergeEnv(b.env, opts.Env, opts.ReplaceEnv)
	cwd := opts.Cwd
	if cwd == "" {
		cwd = b.cwd
	}

	// Per-Exec limit state. execCtx is cancelled when a limit that
	// can't return its error through the runner directly (today: only
	// MaxOutputSize, because r.out swallows io.Writer errors) is
	// tripped. The stashed limitErr is then surfaced from Exec in place
	// of context.Canceled.
	execCtx, cancelExec := context.WithCancel(ctx)
	defer cancelExec()
	if !nested {
		scope.setCancel(cancelExec)
	}

	trip := func(e *ExecutionLimitError) *ExecutionLimitError {
		scope.trip(e, cancelExec)
		return e
	}

	// MaxOutputSize: a single Tracker shared by both writers so the cap
	// is the combined stdout+stderr budget.
	outTracker := scope.outputTracker(int64(b.limits.MaxOutputSize), func(limit int64) error {
		return trip(&ExecutionLimitError{Limit: "MaxOutputSize", Value: int(limit)})
	})
	stdoutW := ringbuf.WrapLimitedWriter(stdout, outTracker)
	stderrW := ringbuf.WrapLimitedWriter(stderr, outTracker)

	// MaxCommandCount, MaxLoopIterations, MaxCallDepth.
	// These counters can be touched concurrently when a script uses
	// process substitution (mvdan/sh runs the substituted command in
	// a goroutine that shares the same CallHandler closure), so we
	// guard them with atomic ops.
	cmdCount := &scope.cmdCount
	loopIters := &scope.loopIters
	// runnerRef is closed over by the CallHandler so it can inspect
	// r.Funcs to decide whether args[0] is a function call (and hence
	// participates in MaxCallDepth accounting). It is set after
	// interp.New succeeds, which is before any CallHandler invocation
	// possible (CallHandler only fires from runner.Run).
	var runnerRef *interp.Runner

	limits := b.limits
	extraArgs := append([]string(nil), opts.Args...)
	var argsConsumed atomic.Bool
	dynamic := &dynamicScripts{b: b, trip: trip, isFunc: func(name string) bool {
		if runnerRef == nil {
			return false
		}
		_, ok := runnerRef.Funcs[name]
		return ok
	}}
	callHandler := func(callCtx context.Context, args []string) ([]string, error) {
		if len(args) == 0 {
			return args, nil
		}
		// Loop sentinel: bump iteration counter, rewrite to no-op, do
		// NOT count toward MaxCommandCount.
		if args[0] == loopSentinelName {
			iter := loopIters.Add(1)
			if iter > int64(limits.MaxLoopIterations) {
				return nil, trip(&ExecutionLimitError{
					Limit: "MaxLoopIterations",
					Value: limits.MaxLoopIterations,
				})
			}
			return []string{":"}, nil
		}
		// Inject literal argv only at the first real dispatch, after expansion.
		// Substitutions and pipelines share this closure; atomics ensure exactly
		// one dispatch consumes the arguments, never an instrumentation sentinel.
		if argsConsumed.CompareAndSwap(false, true) && len(extraArgs) > 0 {
			injected := make([]string, 0, len(args)+len(extraArgs))
			injected = append(injected, args...)
			args = append(injected, extraArgs...)
		}
		// The spec MaxStringLength: cap the size of any single argument
		// reaching a command. mvdan/sh produces these via the full
		// expansion pipeline (parameter, command sub, brace, glob), so a
		// CallHandler check is the latest possible hook before a builtin
		// sees the value.
		for _, a := range args {
			if len(a) > limits.MaxStringLength {
				return nil, trip(&ExecutionLimitError{
					Limit: "MaxStringLength",
					Value: limits.MaxStringLength,
				})
			}
		}
		// mvdan/sh's own `cd` and `pwd` are unusable inside the sandbox:
		// cd gates on unix.Access() against the HOST filesystem (see
		// builtins/cd), and pwd reads the $PWD variable that only that
		// broken cd maintains. Both are shadowed by gobash built-ins
		// that work off the VFS, but mvdan/sh dispatches its builtins
		// before the exec handler ever sees the name — so redirect them
		// here, by path, which routes through lookupCommand's /bin/
		// basename fallback into the registry.
		//
		// A user-defined shell function of the same name still wins:
		// r.Funcs is consulted after the CallHandler, and rewriting the
		// word would hide the function.
		if runnerRef != nil {
			if _, isFunc := runnerRef.Funcs[args[0]]; !isFunc {
				switch args[0] {
				case "cd", "pwd":
					args = append([]string{"/bin/" + args[0]}, args[1:]...)
				}
			}
		}
		cmd := cmdCount.Add(1)
		if cmd > int64(limits.MaxCommandCount) {
			return nil, trip(&ExecutionLimitError{
				Limit: "MaxCommandCount",
				Value: limits.MaxCommandCount,
			})
		}
		// MaxCallDepth: when args[0] resolves to a declared function
		// we count the number of mvdan/sh interp.(*Runner).call frames
		// currently on the goroutine stack. mvdan/sh exposes no
		// per-function entry/exit callback, so a stack walk is the
		// cleanest workaround.
		if runnerRef != nil {
			if _, isFunc := runnerRef.Funcs[args[0]]; isFunc {
				if depth := countMvdanCallFrames(); depth > limits.MaxCallDepth {
					return nil, trip(&ExecutionLimitError{
						Limit: "MaxCallDepth",
						Value: limits.MaxCallDepth,
					})
				}
			}
		}
		return dynamic.call(callCtx, args)
	}

	// MaxGlobOperations: every ReadDir during pathname expansion (or
	// any other VFS readdir, e.g. shopt-driven completion) bumps this
	// counter. mvdan/sh routes glob walks through the ReadDirHandler,
	// so the hook fires once per directory probed. There's no way to
	// distinguish glob-driven ReadDirs from builtin-driven ones at
	// this layer; we document the over-count caveat in DECISIONS.md.
	globOps := &scope.globOps
	readDirHook := func(_ context.Context, _ string) error {
		n := globOps.Add(1)
		if n > int64(limits.MaxGlobOperations) {
			return trip(&ExecutionLimitError{
				Limit: "MaxGlobOperations",
				Value: limits.MaxGlobOperations,
			})
		}
		return nil
	}

	runner, err := bashinterp.BuildRunner(execCtx, bashinterp.Config{
		Env:             envSlice(env),
		ShellOptions:    b.shellOptions,
		Cwd:             cwd,
		Stdin:           stdin,
		Stdout:          stdoutW,
		Stderr:          stderrW,
		FS:              b.fs,
		CallHandler:     callHandler,
		OpenHandlerHook: dynamic.open,
		ReadDirHook:     readDirHook,
		Registry:        b.registry,
		Fetch:           b.fetch,
		Sleep:           b.sleep,
		Trace:           b.trace,
		Limits:          b.limits,
		ExportedEnv:     env,
		Aliases:         b.aliases,
		History:         b.history,
		Exec:            b.subExec,
		SourceDepth:     b.execDepth,
		Shopt:           b.shopt,
	})
	if err != nil {
		return result, err
	}
	runnerRef = runner

	runErr := runner.Run(execCtx, file)

	if captureOut {
		result.Stdout = outBuf.String()
	}
	if captureErr {
		result.Stderr = errBuf.String()
	}
	result.Env = exportedEnv(runner)
	// The working directory is observable state: record it before any of the
	// error paths below can return, so a host can carry `cd` across
	// executions even when the script failed.
	result.Cwd = runner.Dir
	if pluginMetadata != nil {
		result.Metadata = pluginMetadata
	}

	// Upstream executes against a cloned environment on every call. Neither
	// exports nor ReplaceEnv may mutate the constructor's base environment.

	// If a limit was tripped from a non-handler path (today: only
	// MaxOutputSize, because r.out swallows the LimitedWriter's error),
	// surface our typed error regardless of whether runErr is nil or
	// already a context sentinel.
	if limitErr := scope.err(); limitErr != nil {
		return result, limitErr
	}

	if runErr != nil {
		// A handler-returned limit error may be wrapped in the runner's
		// fatal-err path. Extract it first.
		var ele *ExecutionLimitError
		if errors.As(runErr, &ele) {
			return result, ele
		}
		var status interp.ExitStatus
		if errors.As(runErr, &status) {
			result.ExitCode = int(status)
			return result, nil
		}
		// Preserve the raw context sentinel so callers can use errors.Is.
		// Note: if the caller's ctx was cancelled while we were also
		// holding execCtx open, surfacing context.Canceled is correct.
		if errors.Is(runErr, context.Canceled) {
			return result, context.Canceled
		}
		if errors.Is(runErr, context.DeadlineExceeded) {
			return result, context.DeadlineExceeded
		}
		return result, runErr
	}
	return result, nil
}

// instrumentLoops walks the parsed AST and prepends a sentinel call to
// the Do block of every for/while/until loop. The sentinel becomes a
// CallExpr that the CallHandler intercepts to bump the loop-iteration
// counter. Loops with empty bodies (which mvdan/sh's parser would reject
// at parse time anyway) are not special-cased.
func instrumentLoops(file *syntax.File) {
	syntax.Walk(file, func(n syntax.Node) bool {
		switch t := n.(type) {
		case *syntax.WhileClause:
			t.Do = prependSentinel(t.Do)
		case *syntax.ForClause:
			t.Do = prependSentinel(t.Do)
		}
		return true
	})
}

func prependSentinel(body []*syntax.Stmt) []*syntax.Stmt {
	out := make([]*syntax.Stmt, 0, len(body)+1)
	out = append(out, newSentinelStmt())
	out = append(out, body...)
	return out
}

func newSentinelStmt() *syntax.Stmt {
	return &syntax.Stmt{
		Cmd: &syntax.CallExpr{
			Args: []*syntax.Word{{
				Parts: []syntax.WordPart{
					&syntax.Lit{Value: loopSentinelName},
				},
			}},
		},
	}
}

// countMvdanCallFrames returns the number of mvdan/sh interp.(*Runner).call
// frames currently on the goroutine stack.
//
// mvdan/sh does not provide a function-entry/exit hook; CallHandler only
// fires on entry. To track call depth without a decrement signal we walk
// the goroutine stack on each function-entry event and count nested
// interp.(*Runner).call frames. The runner recursively invokes its own
// call method for nested function bodies and builtins like source/eval,
// so this count is an exact lower bound on the shell call depth.
//
// This is documented as a workaround and may be
// revisited if mvdan/sh ever exposes a richer call lifecycle API.
func countMvdanCallFrames() int {
	var pcs [128]uintptr
	n := runtime.Callers(0, pcs[:])
	if n == 0 {
		return 0
	}
	frames := runtime.CallersFrames(pcs[:n])
	depth := 0
	for {
		f, more := frames.Next()
		if isMvdanCallFrame(f.Function) {
			depth++
		}
		if !more {
			break
		}
	}
	return depth
}

// isMvdanCallFrame matches the qualified name of mvdan.cc/sh/v3/interp's
// Runner.call method. We compare via HasSuffix because the package path
// prefix is stable but the leading import comments may vary across Go
// toolchain versions. Kept private so the matcher can be tightened
// without churning callers.
func isMvdanCallFrame(name string) bool {
	return strings.HasSuffix(name, "interp.(*Runner).call")
}

// wireStdio returns the stdout/stderr writers Exec should hand to the
// interpreter, along with flags and buffers used to populate the
// BashExecResult string fields when the caller did not provide their own
// writer.
func wireStdio(opts ExecOptions) (
	stdout io.Writer,
	stderr io.Writer,
	captureOut bool,
	captureErr bool,
	outBuf *bytes.Buffer,
	errBuf *bytes.Buffer,
) {
	if opts.Stdout != nil {
		stdout = opts.Stdout
	} else {
		outBuf = &bytes.Buffer{}
		stdout = outBuf
		captureOut = true
	}
	if opts.Stderr != nil {
		stderr = opts.Stderr
	} else {
		errBuf = &bytes.Buffer{}
		stderr = errBuf
		captureErr = true
	}
	return
}

func cloneEnv(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}

// mergeEnv returns the effective environment for an Exec call. When
// replace is true, only overlay is honored (matching ExecOptions.ReplaceEnv);
// otherwise overlay's keys win over base's.
func mergeEnv(base, overlay map[string]string, replace bool) map[string]string {
	if replace {
		return cloneEnv(overlay)
	}
	out := cloneEnv(base)
	maps.Copy(out, overlay)
	return out
}

func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// exportedEnv extracts the post-run exported variables from the mvdan/sh
// runner. Only string-typed variables are surfaced for now; arrays and
// associative arrays are added in Phase 5 alongside the interp bridge.
func exportedEnv(r *interp.Runner) map[string]string {
	out := make(map[string]string, len(r.Vars))
	for k, v := range r.Vars {
		if !v.Exported {
			continue
		}
		out[k] = v.String()
	}
	return out
}

// defaultProcessInfo matches the table fixed in the spec
func defaultProcessInfo() ProcessInfo {
	return ProcessInfo{PID: 1, PPID: 0, UID: 1000, GID: 1000}
}

// subExec is the SubExecFunc plumbed through to commands via
// command.Context.Exec. It is invoked by the Phase 10 Wave G
// `bash` / `sh` / `timeout` built-ins to run a child script with a
// derived Env / Cwd / Stdio. It calls execLocked directly because the
// parent Exec call already holds b.mu and runs on the same goroutine.
//
// The translation is straightforward: command.SubExecOptions →
// gobash.ExecOptions. Args retains the public literal first-dispatch semantics;
// it is not a shell positional-parameter API.
func (b *Bash) subExec(ctx context.Context, script string, opts command.SubExecOptions) (command.Result, error) {
	prevDepth := b.execDepth
	if opts.SourceDepth > 0 {
		b.execDepth = opts.SourceDepth
	} else {
		b.execDepth = prevDepth + 1
	}
	defer func() { b.execDepth = prevDepth }()
	if scope, ok := executionScopeFromContext(ctx); ok {
		ctx = context.WithValue(ctx, executionScopeKey{}, scope)
	}
	res, err := b.execLocked(ctx, script, ExecOptions{
		Env:        opts.Env,
		ReplaceEnv: opts.ReplaceEnv,
		Cwd:        opts.Cwd,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
		Args:       opts.Args,
	})
	return command.Result{
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
	}, err
}

// validateAPILimits validates the effective (post-legacy-override) policy.
// Zero is a valid budget, consistent with upstream's resolveLimits.
func validateAPILimits(l ResolvedLimits) error {
	values := []struct {
		name  string
		value int64
	}{
		{"MaxCallDepth", int64(l.MaxCallDepth)},
		{"MaxCommandCount", int64(l.MaxCommandCount)},
		{"MaxLoopIterations", int64(l.MaxLoopIterations)},
		{"MaxAwkIterations", int64(l.MaxAwkIterations)},
		{"MaxSedIterations", int64(l.MaxSedIterations)},
		{"MaxJqIterations", int64(l.MaxJqIterations)},
		{"MaxSqliteTimeout", int64(l.MaxSqliteTimeout)},
		{"MaxPythonTimeout", int64(l.MaxPythonTimeout)},
		{"MaxJsTimeout", int64(l.MaxJsTimeout)},
		{"MaxGlobOperations", int64(l.MaxGlobOperations)},
		{"MaxStringLength", int64(l.MaxStringLength)},
		{"MaxArrayElements", int64(l.MaxArrayElements)},
		{"MaxHeredocSize", int64(l.MaxHeredocSize)},
		{"MaxSubstitutionDepth", int64(l.MaxSubstitutionDepth)},
		{"MaxBraceExpansionResults", int64(l.MaxBraceExpansionResults)},
		{"MaxOutputSize", int64(l.MaxOutputSize)},
		{"MaxFileDescriptors", int64(l.MaxFileDescriptors)},
		{"MaxSourceDepth", int64(l.MaxSourceDepth)},
		{"MaxInputSize", int64(l.MaxInputSize)},
		{"MaxFileSystemBytes", int64(l.MaxFileSystemBytes)},
	}
	for _, v := range values {
		if v.value < 0 {
			return fmt.Errorf("gobash: %s must be non-negative", v.name)
		}
	}
	return nil
}

// Upstream normalizeScript deliberately trims each unquoted physical line,
// rather than removing a common indentation prefix. Its heredoc recognition
// is a lightweight lexical heuristic, not a second shell parser.
var apiHeredocPattern = regexp.MustCompile(`<<(-?)\s*(['"]?)([\w-]+)`)

func normalizeAPIScript(script string) string {
	type delimiter struct {
		word      string
		stripTabs bool
	}
	lines := strings.Split(script, "\n")
	var pending []delimiter
	var quote byte
	for i, line := range lines {
		if len(pending) > 0 {
			d := pending[len(pending)-1]
			check := line
			if d.stripTabs {
				check = strings.TrimLeft(check, "\t")
			}
			if check == d.word {
				lines[i] = strings.TrimLeftFunc(line, apiScriptSpace)
				pending = pending[:len(pending)-1]
			}
			continue
		}
		start := quote
		if start == 0 {
			lines[i] = strings.TrimLeftFunc(line, apiScriptSpace)
		}
		quote = scanAPIQuoteState(line, start)
		if start != 0 {
			continue
		}
		for _, match := range apiHeredocPattern.FindAllStringSubmatchIndex(lines[i], -1) {
			// Go regexps lack backreferences: check upstream's closing quote
			// constraint explicitly for quoted delimiters.
			q := lines[i][match[4]:match[5]]
			if q != "" && (match[1] >= len(lines[i]) || lines[i][match[1]:match[1]+1] != q) {
				continue
			}
			pending = append(pending, delimiter{
				word:      lines[i][match[6]:match[7]],
				stripTabs: lines[i][match[2]:match[3]] == "-",
			})
		}
	}
	return strings.Join(lines, "\n")
}

// ECMAScript trimStart includes BOM, but excludes Unicode NEL.
func apiScriptSpace(r rune) bool {
	return r == '\uFEFF' || (r != '\u0085' && unicode.IsSpace(r))
}

func scanAPIQuoteState(line string, quote byte) byte {
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch quote {
		case '\'':
			if ch == '\'' {
				quote = 0
			}
		case '"':
			switch ch {
			case '\\':
				i++
			case '"':
				quote = 0
			}
		default:
			switch ch {
			case '\'', '"':
				quote = ch
			case '\\':
				i++
			case '#':
				if i == 0 || apiScriptSpace(rune(line[i-1])) {
					return quote
				}
			}
		}
	}
	return quote
}
