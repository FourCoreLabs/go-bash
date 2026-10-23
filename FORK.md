# FourCoreLabs/go-bash

A fork of [mark3labs/go-bash](https://github.com/mark3labs/go-bash) (MIT, see
`LICENSE`) for sandboxed agent shells.

```
go get github.com/FourCoreLabs/go-bash@v1.0.0
```

The module path is `github.com/FourCoreLabs/go-bash`, and it builds on
[`github.com/FourCoreLabs/sh`](https://github.com/FourCoreLabs/sh), our fork of
mvdan/sh. Both are required by their own paths, so no consumer needs a `replace`
directive or a vendored copy of mvdan/sh. Versions start at v1.0.0.

## Base

Upstream mark3labs/go-bash **`a055aa7`** (v0.1.0), imported without history.
`git diff <import commit> HEAD` shows everything this fork changes.

| Commit | Change |
|---|---|
| Import mark3labs/go-bash a055aa7 | Upstream tree, verbatim |
| FourCore fixes for embedding go-bash in agent sandboxes | The fixes listed below |
| Declare module github.com/FourCoreLabs/go-bash | Module path and import rewrite |
| Build on github.com/FourCoreLabs/sh | `mvdan.cc/sh/v3` replaced by `github.com/FourCoreLabs/sh` v1.0.0 |

## The fixes

* `builtins/cd`, `ls`, `rg`: resolve virtual paths, and the active runner is
  reachable so `cd` can move it.
* `caps.go`: expansion caps treat a here-document body as data. Braces in it
  never count, and a quoted body isn't searched for substitutions.
* `options.go`, `bash.go`, `interp/runner.go`: `BashOptions.ShellOptions`
  applies interpreter options at construction (`{"-o", "pipefail"}`).
* `builtins/rg`: output matches real ripgrep when piped, which is the only
  mode a virtual shell has:
  * no line numbers unless `-n`
  * `file:text` lines when a directory or several paths are searched
  * `-H`/`-I`/`--heading`/`--no-heading` are supported
  * stdin is searched when piped and no path is given

  This differs from upstream, which always prints line numbers with a path
  prefix, so the rg test and `fixtures/rg/basic.json` keep the fork's
  expectations.
* `result.go`, `bash.go`: `BashExecResult.Cwd` reports the directory the script
  finished in, so a host can carry `cd` across executions.
* `interp/runner.go`: every runner uses the sh fork's `VFSPaths()`, because
  gobash always runs over a virtual filesystem.
* `command/define.go`, `builtins/sort`: builtin panics are contained by a
  recover in `Define`, plus a sort key-range guard. Panics in shell goroutines
  (pipeline stages, background jobs) are contained by the sh fork's
  `recoverStage`.

## Known limitations on Windows

These failures exist upstream too, and the move to sh v3.14 didn't cause them:

* **Globbing.** The mvdan/sh expander joins glob directories with `filepath`,
  so on Windows `*.txt` under `/work` resolves to `/work/\work` and matches
  nothing. `VFSPaths` covers the interpreter but not yet the expander. Linux and
  macOS are unaffected.
* **Process substitution** (`<(...)`) is unsupported on Windows upstream.
* Some `awk`, `rwfs` and `sandbox` tests assume POSIX line endings, symlink
  privileges or permissions.

## Updating from upstream

There is no shared history, so updates are applied as diffs:

1. In an upstream clone: `git diff a055aa7 <new commit> > up.patch`
2. Rewrite paths: `sed -i 's#github.com/mark3labs/go-bash#github.com/FourCoreLabs/go-bash#g; s#mvdan.cc/sh/v3#github.com/FourCoreLabs/sh#g' up.patch`
3. Here: `git apply -3 up.patch`, resolve, run `go test ./...`, and commit as
   "Update to mark3labs/go-bash <commit>". Update the Base section above.
