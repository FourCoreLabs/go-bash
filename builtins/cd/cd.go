// Package cd implements the `cd` shell built-in.
//
// mvdan/sh ships its own `cd`, but it is unusable here: after the stat
// succeeds it calls unix.Access(path, X_OK) against the HOST
// filesystem, which no virtual path satisfies, so every cd inside the
// sandbox fails with "permission denied". gobash therefore intercepts
// `cd` in the CallHandler (see bash.go) and routes it here, where the
// target is resolved against the VFS and applied through
// Context.SetCwd.
package cd

import (
	"context"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "cd [-L|-P] [DIR]"

// New returns the cd command.
func New() command.Command { return command.Define("cd", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	dir := ""
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "-L" || a == "-P":
			// accepted, no-op
		case a == "--":
			i++
			goto done
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	if i < len(args) {
		dir = args[i]
	}
	if dir == "" {
		dir = c.Env["HOME"]
		if dir == "" {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "HOME not set")
		}
	}
	if dir == "-" {
		if old := c.Env["OLDPWD"]; old != "" {
			dir = old
		} else {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "OLDPWD not set")
		}
	}
	if !strings.HasPrefix(dir, "/") {
		cwd := c.Cwd
		if cwd == "" {
			cwd = "/"
		}
		dir = path.Join(cwd, dir)
	}
	if c.FS != nil {
		info, err := c.FS.Stat(dir)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "%s: %v", args[len(args)-1], err)
		}
		if !info.IsDir() {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "%s: not a directory", args[len(args)-1])
		}
	}
	// Apply the move. Without the back-channel there is no interpreter
	// to move, so report that rather than silently succeeding.
	if c.SetCwd == nil {
		return builtinutil.Errorf(c.Stderr, "cd", 1, "cannot change directory in this context")
	}
	if err := c.SetCwd(dir); err != nil {
		return builtinutil.Errorf(c.Stderr, "cd", 1, "%v", err)
	}
	if c.Env != nil {
		c.Env["OLDPWD"] = c.Cwd
		c.Env["PWD"] = dir
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
