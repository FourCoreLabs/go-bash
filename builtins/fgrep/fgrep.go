// Package fgrep implements the `fgrep` built-in — grep with the
// fixed-string (-F) mode as the default. The spec Wave D.
package fgrep

import (
	"context"

	"github.com/FourCoreLabs/go-bash/builtins/grep"
	"github.com/FourCoreLabs/go-bash/command"
)

// New returns the fgrep command.
func New() command.Command { return command.Define("fgrep", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return grep.Run(c, args, grep.ModeFixed)
}

func init() { command.RegisterBuiltin(New()) }
