# FourCoreLabs fork of go-bash

Tracks `github.com/mark3labs/go-bash` (merged through upstream `a055aa7`, v0.1.0)
with the fixes this product needs:

* `fix/cd-ls-rg` — `cd`, `ls` and `rg` resolve virtual paths, and the active
  runner is reachable so `cd` can move it (merged).
* `caps.go` — expansion caps treat a here-document body as data: braces are
  never counted there, and a quoted body is not searched for substitutions.
* `options.go`, `bash.go`, `interp/runner.go` — `BashOptions.ShellOptions`
  applies interpreter options at construction (`{"-o", "pipefail"}`).
* `builtins/rg` — rg output matches real ripgrep when piped, which is the only
  mode a virtual shell has: no line numbers unless `-n`, `file:text` lines when
  a directory or several paths are searched, `-H`/`-I`/`--heading`/`--no-heading`,
  and stdin is searched when piped and no path is given. This deliberately
  differs from upstream, which always prints line numbers with a path prefix,
  so the rg test and `fixtures/rg/basic.json` keep the fork's expectations.
  Upstream's newer rg flags (`-F -w -x -o -q -L -f -m -r --max-depth
  --no-ignore`, ignore files) are kept.
* `result.go`, `bash.go` — `BashExecResult.Cwd` reports the working directory
  the script finished in, so a host can carry `cd` across executions.
* `interp/runner.go` — enables `mvdan/sh`'s `VFSPaths()` for every runner,
  because gobash always runs over a virtual filesystem.

## Pairing

`VFSPaths()` only exists in the sibling fork
[`github.com/FourCoreLabs/sh`](https://github.com/FourCoreLabs/sh) (tag
`v3.13.2-attack1`), which also keeps quoted heredoc bodies literal. A consumer
must supply that patch through its own module graph, for example:

    replace mvdan.cc/sh/v3 => ./third_party/mvdan-sh

Go cannot consume a fork of a `/vN` module from a different module path: its
internal packages require the original path, and a replacement path without the
`/v3` suffix cannot carry a v3 version. This module therefore builds against
`mvdan.cc/sh/v3` as its consumer resolves it, not on its own.
