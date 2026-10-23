// Package sha256sum implements the `sha256sum` built-in.
package sha256sum

import (
	"context"
	"crypto/sha256"
	"hash"

	"github.com/FourCoreLabs/go-bash/builtins/md5sum"
	"github.com/FourCoreLabs/go-bash/command"
)

// New returns the sha256sum command.
func New() command.Command { return command.Define("sha256sum", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return md5sum.Sum(c, args, "sha256sum", func() hash.Hash { return sha256.New() })
}

func init() { command.RegisterBuiltin(New()) }
