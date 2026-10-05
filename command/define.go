package command

import (
	"context"
	"fmt"
)

// Define wraps a plain function in a Command, mirroring the TS
// `defineCommand` helper from the spec The resulting Command's
// Name is the supplied string; Trusted is true (the sandbox-untrust
// case is opt-in via DefineUntrusted once Phase 17 needs it).
//
// Define is the recommended way to construct simple commands in
// tests and in the Phase 10 built-in packages; it keeps the
// boilerplate of the Command interface out of every package init.
func Define(name string, fn func(ctx context.Context, args []string, c *Context) Result) Command {
	return &funcCommand{name: Name(name), fn: fn, trusted: true}
}

type funcCommand struct {
	name    Name
	fn      func(ctx context.Context, args []string, c *Context) Result
	trusted bool
}

func (f *funcCommand) Name() Name { return f.name }

// Execute runs the command, converting a panic inside it into a failed result.
//
// Every command in this runtime - a builtin or an application registration -
// is third-party code executing inside the host process, and the interpreter
// dispatches it from many goroutines at once: pipeline stages, background
// jobs, and the worker goroutines of xargs, find -exec and env. A bug in any of
// them must surface as a command that failed, never as a process that died, so
// this is the one point every command passes through and the one place a panic
// is turned into shell state.
//
// Exit status 2 is the shell's own "internal error" status, distinct from the
// statuses a command reports for its own failures.
func (f *funcCommand) Execute(ctx context.Context, args []string, c *Context) (result Result) {
	defer func() {
		if rec := recover(); rec != nil {
			if c != nil && c.Stderr != nil {
				_, _ = fmt.Fprintf(c.Stderr, "%s: internal error: %v\n", f.name, rec)
			}

			result = Result{ExitCode: 2}
		}
	}()

	return f.fn(ctx, args, c)
}
func (f *funcCommand) Trusted() bool { return f.trusted }
