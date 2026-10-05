// Package yes implements the sandbox's finite, bounded yes stream.
package yes

import (
	"context"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `yes - output a string repeatedly
Usage: yes [STRING]...
Repeatedly output a line with all specified STRING(s), or 'y'.
    --help  display this help and exit
The sandbox stops after MaxLoopIterations lines, or when the output budget is full.`

func New() command.Command { return command.Define("yes", run) }
func init()                { command.RegisterBuiltin(New()) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	var operands []string
	ended := false
	for _, a := range args[1:] {
		switch {
		case ended:
			operands = append(operands, a)
		case a == "--":
			ended = true
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{}
		case strings.HasPrefix(a, "--"):
			return builtinutil.Errorf(c.Stderr, "yes", 1, "unrecognized option '%s'", a)
		case strings.HasPrefix(a, "-") && a != "-":
			return builtinutil.Errorf(c.Stderr, "yes", 1, "invalid option -- '%s'", a[1:2])
		default:
			operands = append(operands, a)
		}
	}
	budget := min(c.Limits.MaxOutputSize, c.Limits.MaxStringLength)
	limitError := func() command.Result {
		return builtinutil.Errorf(c.Stderr, "bash", 126, "yes: output size limit exceeded (%d bytes)", budget)
	}
	// Charge the join before allocating; lengths are UTF-8 byte lengths.
	size := 1
	if len(operands) == 0 {
		size = 2
	} else {
		for i, a := range operands {
			if i > 0 {
				size++
			}
			if size > budget || len(a) > budget-size {
				return limitError()
			}
			size += len(a)
		}
	}
	if size > budget && c.Limits.MaxLoopIterations > 0 {
		return limitError()
	}
	if c.Limits.MaxLoopIterations <= 0 {
		return command.Result{}
	}
	line := "y\n"
	if len(operands) > 0 {
		line = strings.Join(operands, " ") + "\n"
	}
	if c.Stdout == nil {
		return builtinutil.Errorf(c.Stderr, "yes", 1, "no output stream")
	}
	lines := min(c.Limits.MaxLoopIterations, budget/size)
	for range lines {
		if err := ctx.Err(); err != nil {
			return builtinutil.Errorf(c.Stderr, "yes", 1, "%v", err)
		}
		n, err := io.WriteString(c.Stdout, line)
		if err == nil && n != len(line) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "yes", 1, "write: %v", err)
		}
	}
	return command.Result{}
}
