// Package htmltomarkdown implements the `html-to-markdown` built-in.
// It reads HTML from stdin or the first positional
// file argument and writes CommonMark Markdown to stdout. The
// conversion is delegated to github.com/JohannesKaufmann/html-to-markdown/v2
// (BSD-3-Clause, pure-Go).
//
// All file I/O routes through Context.FS; stdin / stdout via
// Context.Stdin / Context.Stdout. The command takes no network access
// and is always registered (unlike `curl`).
package htmltomarkdown

import (
	"context"
	"io"
	"slices"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"golang.org/x/net/html"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const cmdName = "html-to-markdown"
const usage = cmdName + " [OPTION]... [FILE]"

const helpText = `Usage: html-to-markdown [OPTION]... [FILE]
Convert HTML to CommonMark Markdown.

With no FILE (or FILE='-'), read HTML from stdin. Output is written
to stdout. File I/O is routed through the virtual filesystem.

Options:
  -b, --bullet=CHAR       unordered list marker (-, +, or *; default: -)
  -c, --code=FENCE        code block fence (three backticks or ~~~; default: backticks)
  -r, --hr=STRING         horizontal rule string (default: ---)
      --heading-style=STYLE
                         atx (default) or setext (h1/h2 only)
      --help             show this help and exit
`

// New returns the html-to-markdown command.
func New() command.Command { return command.Define(cmdName, run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var (
		file         string
		hasFile      bool
		bullet       = "-"
		codeFence    = "```"
		hr           = "---"
		headingStyle = commonmark.HeadingStyleATX
	)
	// Like upstream, options may follow the input file, and --help wins
	// over other arguments. Retain the existing -- end-of-options support.
	if slices.Contains(args[1:], "--help") {
		builtinutil.PrintHelp(c.Stdout, helpText)
		return command.Result{ExitCode: 0}
	}
	options := true
	for i := 1; i < len(args); i++ {
		a := args[i]
		value := func(fallback string) string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return fallback
		}
		switch {
		case options && a == "--":
			options = false
		case options && (a == "-b" || a == "--bullet"):
			bullet = value("-")
		case options && strings.HasPrefix(a, "--bullet="):
			bullet = strings.TrimPrefix(a, "--bullet=")
		case options && (a == "-c" || a == "--code"):
			codeFence = value("```")
		case options && strings.HasPrefix(a, "--code="):
			codeFence = strings.TrimPrefix(a, "--code=")
		case options && (a == "-r" || a == "--hr"):
			hr = value("---")
		case options && strings.HasPrefix(a, "--hr="):
			hr = strings.TrimPrefix(a, "--hr=")
		case options && strings.HasPrefix(a, "--heading-style="):
			// Upstream silently ignores unknown styles, retaining the last valid one.
			switch strings.TrimPrefix(a, "--heading-style=") {
			case "atx":
				headingStyle = commonmark.HeadingStyleATX
			case "setext":
				headingStyle = commonmark.HeadingStyleSetext
			}
		case options && strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			if !hasFile {
				file, hasFile = a, true
			}
		}
	}

	htmlBytes, err := readInput(c, file, hasFile)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "%v", err)
	}

	// Upstream returns early on empty input, before validating markers.
	if strings.TrimSpace(string(htmlBytes)) == "" {
		return command.Result{ExitCode: 0}
	}
	if bullet != "-" && bullet != "+" && bullet != "*" {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "invalid bullet marker")
	}
	if codeFence != "```" && codeFence != "~~~" {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "invalid code fence")
	}
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(
			commonmark.WithBulletListMarker(bullet),
			commonmark.WithCodeBlockFence(codeFence),
			commonmark.WithHeadingStyle(headingStyle),
		),
	))
	// Turndown accepts arbitrary (even empty) hr strings. Commonmark's
	// WithHorizontalRule validates Markdown syntax and replaces empty values
	// with its default, so override just the hr renderer instead.
	conv.Register.Renderer(func(_ converter.Context, w converter.Writer, n *html.Node) converter.RenderStatus {
		if n.Type != html.ElementNode || n.Data != "hr" {
			return converter.RenderTryNext
		}
		_, _ = w.WriteString("\n\n" + hr + "\n\n")
		return converter.RenderSuccess
	}, converter.PriorityEarly)
	md, err := conv.ConvertString(string(htmlBytes))
	if err != nil {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "%v", err)
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, md)
	}
	return command.Result{ExitCode: 0}
}

// readInput returns the HTML source for the conversion. When hasFile
// is true and file != "-", we load it through c.FS; otherwise we
// consume the entire c.Stdin stream.
func readInput(c *command.Context, file string, hasFile bool) ([]byte, error) {
	if hasFile && file != "-" {
		if c.FS == nil {
			return nil, ioErr("no filesystem available")
		}
		path := builtinutil.ResolvePath(c.Cwd, file)
		return c.FS.ReadFile(path)
	}
	if c.Stdin == nil {
		return nil, nil
	}
	return io.ReadAll(c.Stdin)
}

// ioErr is a tiny convenience that returns a single-line error
// without dragging in fmt for one allocation.
type ioErr string

func (e ioErr) Error() string { return string(e) }

func init() { command.RegisterBuiltin(New()) }
