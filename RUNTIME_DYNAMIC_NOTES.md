# Dynamic source/eval instrumentation

Implemented in `runtime_dynamic.go`, with narrow wiring in `bash.go` and
`interp.Config.OpenHandlerHook` in `interp/runner.go`.

Research: mvdan.cc/sh/v3 v3.13.1 `Runner.Run` resets exit state, filename and
expansion configuration, invokes EXIT traps and publishes variables. It is
not a reentrant current-shell execution entry point. `HandlerContext.Builtin`
only works from ExecHandler, and there is no dynamic AST hook. No fork or
nested Runner.Run, unsafe access, or new goroutines are used here.

The CallHandler preprocesses eval input and bounded source reads using the
existing parser, loop instrumentation, virtual process-ID rewriting and
expansion caps. Eval still executes as the native builtin. Source gets a
one-shot serialized reader through OpenHandler and still executes as the
native builtin (including positional-argument restoration and return).
Readers are stored per Exec with a mutex; each source invocation has a unique
key, so pipelines/substitutions do not share a mutable pending-source slot.
Source PATH lookup uses VFS stat, not host stat. Depth uses read-only local
stack inspection, conservatively counting builtin ancestors.

Limitations / follow-ups:
- Parse errors in dynamic code become fatal API parse errors, rather than
  native builtin diagnostics with a recoverable exit status. This is the safe
  fail-closed behavior for bounded preprocessing.
- Source serialization changes line numbering and source filename diagnostics
  to an opaque internal name. Native eval printing also normalizes formatting.
- Dynamic alias expansion and transform plugins are not applied.
- Function-shadowed eval/source/./command/builtin calls are left untouched using
  the live runner function table. Special command-wrapper flags other than
  ordinary command/builtin and -- forms are not rewritten.
- Supported EXIT/ERR trap callback strings are instrumented at registration;
  trap listing/clear forms remain unchanged. This does NOT claim all dynamic
  AST paths or unsupported signal traps are covered.
- Depth counts wrapping builtins conservatively; stack inspection is tied to
  mvdan's private builtin method name and must be revalidated on upgrades.
- Static substitution depth is per dynamic script, not aggregate ancestry.
- Dynamic input uses the configured MaxInputSize (default 1 MiB), including a
  bounded source reader and pre-allocation eval concatenation check. Configured
  heredoc budgets also apply.

Validation:
`go test . ./interp -run 'TestDynamic|TestMaxLoop' -count=1`
`go test -race . -run TestDynamic -count=1`
Both pass. Integrated `go test -race ./...` and `go vet ./...` also pass after
updating environment-isolation regressions to the upstream contract.
