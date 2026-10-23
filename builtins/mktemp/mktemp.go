// Package mktemp creates private temporary entries exclusively in the VFS.
package mktemp

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `mktemp - create a temporary file or directory and print its name
Usage: mktemp [OPTION]... [TEMPLATE]
TEMPLATE must contain at least three consecutive X's in its final run.
  -d, --directory       create a directory
  -u, --dry-run         only print an unused name
  -q, --quiet           suppress creation diagnostics
  -p DIR, --tmpdir[=DIR] use DIR, or TMPDIR (default /tmp)
  -t                    use a single-component template in TMPDIR
      --suffix=SUFF     append SUFF (no slash)
  -h, --help            display this help
      --version         display version`

func New() command.Command { return command.Define("mktemp", run) }
func init()                { command.RegisterBuiltin(New()) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	fail := func(format string, a ...any) command.Result {
		return builtinutil.Errorf(c.Stderr, "mktemp", 1, format, a...)
	}
	directory, dry, quiet, legacy, hasDir, hasSuffix := false, false, false, false, false, false
	dir, suffix := "", ""
	var operands []string
	ended := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if ended || !strings.HasPrefix(a, "-") || a == "-" {
			operands = append(operands, a)
			continue
		}
		if a == "--" {
			ended = true
			continue
		}
		if strings.HasPrefix(a, "--") {
			name, value, attached := strings.Cut(a[2:], "=")
			switch name {
			case "help", "version":
				if attached {
					return fail("option '--%s' doesn't allow an argument", name)
				}
				if name == "help" {
					builtinutil.PrintHelp(c.Stdout, helpText)
				} else {
					if _, err := io.WriteString(c.Stdout, "mktemp (just-bash) 9.4\n"); err != nil {
						return fail("write: %v", err)
					}
				}
				return command.Result{}
			case "directory", "dry-run", "quiet":
				if attached {
					return fail("option '--%s' doesn't allow an argument", name)
				}
				switch name {
				case "directory":
					directory = true
				case "dry-run":
					dry = true
				case "quiet":
					quiet = true
				}
			case "tmpdir":
				hasDir, dir = true, value
			case "suffix":
				if !attached {
					if i+1 == len(args) {
						return fail("option '--suffix' requires an argument")
					}
					i++
					value = args[i]
				}
				hasSuffix, suffix = true, value
			default:
				return fail("unrecognized option '%s'", a)
			}
			continue
		}
		for j := 1; j < len(a); j++ {
			switch a[j] {
			case 'd':
				directory = true
			case 'u':
				dry = true
			case 'q':
				quiet = true
			case 't':
				legacy = true
			case 'h':
				builtinutil.PrintHelp(c.Stdout, helpText)
				return command.Result{}
			case 'p':
				hasDir = true
				if j+1 < len(a) {
					dir = a[j+1:]
				} else {
					if i+1 == len(args) {
						return fail("option requires an argument -- 'p'")
					}
					i++
					dir = args[i]
				}
				j = len(a)
			default:
				return fail("invalid option -- '%s'", a[j:j+1])
			}
		}
	}
	if len(operands) > 1 {
		return fail("too many templates")
	}
	template := "tmp.XXXXXXXXXX"
	if len(operands) == 1 {
		template = operands[0]
	}
	stem := template
	if hasSuffix {
		if !strings.HasSuffix(stem, "X") {
			return fail("with --suffix, template '%s' must end in X", template)
		}
	} else if x := strings.LastIndexByte(stem, 'X'); x >= 0 {
		suffix = stem[x+1:]
		stem = stem[:x+1]
	}
	if strings.Contains(suffix, "/") {
		return fail("invalid suffix '%s', contains directory separator", suffix)
	}
	count := len(stem) - len(strings.TrimRight(stem, "X"))
	if count < 3 {
		return fail("too few X's in template '%s'", template)
	}
	envDir := c.Env["TMPDIR"]
	defaultDir := envDir
	if defaultDir == "" {
		defaultDir = "/tmp"
	}
	dest := ""
	if legacy {
		if strings.Contains(template, "/") {
			return fail("invalid template, '%s', contains directory separator", template)
		}
		dest = envDir
		if dest == "" {
			dest = dir
		}
		if dest == "" {
			dest = "/tmp"
		}
	} else if hasDir || len(operands) == 0 {
		if strings.HasPrefix(template, "/") {
			return fail("invalid template, '%s'; with --tmpdir, it may not be absolute", template)
		}
		dest = dir
		if dest == "" {
			dest = defaultDir
		}
	}
	prefix := stem[:len(stem)-count]
	if dest != "" {
		prefix = strings.TrimSuffix(dest, "/") + "/" + prefix
	}
	kind := "file"
	if directory {
		kind = "directory"
	}
	creationFailure := func(reason string) command.Result {
		if quiet {
			return command.Result{ExitCode: 1}
		}
		return fail("failed to create %s via template '%s': %s", kind, template, reason)
	}
	if c.FS == nil {
		return creationFailure("no filesystem")
	}
	if !dry {
		parent := builtinutil.ResolvePath(c.Cwd, path.Dir(prefix+strings.Repeat("X", count)))
		st, err := c.FS.Stat(parent)
		if err != nil {
			return creationFailure("No such file or directory")
		}
		if !st.IsDir() {
			return creationFailure("Not a directory")
		}
	}
	for range 100 {
		if err := ctx.Err(); err != nil {
			return creationFailure(err.Error())
		}
		random, err := randomChars(ctx, count)
		if err != nil {
			return creationFailure(err.Error())
		}
		name := prefix + random + suffix
		full := builtinutil.ResolvePath(c.Cwd, name)
		if dry {
			_, err = c.FS.Lstat(full)
			if err == nil {
				continue
			}
			if !errors.Is(err, iofs.ErrNotExist) {
				return creationFailure(err.Error())
			}
		} else if directory {
			err = c.FS.Mkdir(full, 0o700)
		} else {
			var f interface{ Close() error }
			f, err = c.FS.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err == nil {
				err = f.Close()
			}
		}
		if err != nil && !dry {
			if errors.Is(err, iofs.ErrExist) {
				continue
			}
			return creationFailure(err.Error())
		}
		if _, err := io.WriteString(c.Stdout, name+"\n"); err != nil {
			return fail("write: %v", err)
		}
		return command.Result{}
	}
	return creationFailure("File exists")
}

// Rejection sampling avoids modulo bias; fixed chunks keep entropy requests bounded.
func randomChars(ctx context.Context, count int) (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	out := make([]byte, count)
	var buf [4096]byte
	for n := 0; n < count; {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b >= 248 {
				continue
			}
			out[n] = alphabet[int(b)%len(alphabet)]
			n++
			if n == count {
				break
			}
		}
	}
	return string(out), nil
}
