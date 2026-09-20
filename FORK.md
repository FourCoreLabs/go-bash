# FourCoreLabs fork of go-bash

Tracks `github.com/mark3labs/go-bash` with the fixes this product needs:

* `fix/cd-ls-rg` — `cd`, `ls` and `rg` resolve virtual paths, and the active
  runner is reachable so `cd` can move it (merged).
* `caps.go` — expansion caps treat a here-document body as data: braces are
  never counted there, and a quoted body is not searched for substitutions.
* `options.go`, `bash.go`, `interp/runner.go` — `BashOptions.ShellOptions`
  applies interpreter options at construction (`{"-o", "pipefail"}`).
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
