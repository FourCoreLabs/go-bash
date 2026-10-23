// Package builtins is the meta-package that side-effect-imports every
// Phase 10 built-in. Each command's init() calls
// command.RegisterBuiltin, populating the package-level slice that
// gobash.New consumes when constructing a *Bash.
//
// Hosts that want the full default registry should not import this
// package directly — the gobash root package blank-imports it via
// builtins_init.go. Hosts that want a CUSTOM subset (e.g. only `echo`
// and `printf`) should set BashOptions.Commands to filter.
//
// Hosts that want NO built-ins at all must construct a *Bash and
// then drop entries from Bash.Registry() manually; the blank import
// here is not conditional.
//
// Cited surface: the spec ("Lazy loading" — Go has no dynamic
// import, so we replace it with side-effect registration).
package builtins

import (
	// Phase 10 Wave A: trivial commands.
	_ "github.com/FourCoreLabs/go-bash/builtins/basename"
	_ "github.com/FourCoreLabs/go-bash/builtins/clear"
	_ "github.com/FourCoreLabs/go-bash/builtins/dirname"
	_ "github.com/FourCoreLabs/go-bash/builtins/echo"
	_ "github.com/FourCoreLabs/go-bash/builtins/expr"
	_ "github.com/FourCoreLabs/go-bash/builtins/falsecmd"
	_ "github.com/FourCoreLabs/go-bash/builtins/hostname"
	_ "github.com/FourCoreLabs/go-bash/builtins/printf"
	_ "github.com/FourCoreLabs/go-bash/builtins/pwd"
	_ "github.com/FourCoreLabs/go-bash/builtins/seq"
	_ "github.com/FourCoreLabs/go-bash/builtins/sleep"
	_ "github.com/FourCoreLabs/go-bash/builtins/truecmd"
	_ "github.com/FourCoreLabs/go-bash/builtins/which"
	_ "github.com/FourCoreLabs/go-bash/builtins/whoami"
	_ "github.com/FourCoreLabs/go-bash/builtins/yes"

	// Phase 10 Wave B: file operations.
	_ "github.com/FourCoreLabs/go-bash/builtins/chmod"
	_ "github.com/FourCoreLabs/go-bash/builtins/cp"
	_ "github.com/FourCoreLabs/go-bash/builtins/du"
	_ "github.com/FourCoreLabs/go-bash/builtins/file"
	_ "github.com/FourCoreLabs/go-bash/builtins/ln"
	_ "github.com/FourCoreLabs/go-bash/builtins/ls"
	_ "github.com/FourCoreLabs/go-bash/builtins/mkdir"
	_ "github.com/FourCoreLabs/go-bash/builtins/mktemp"
	_ "github.com/FourCoreLabs/go-bash/builtins/mv"
	_ "github.com/FourCoreLabs/go-bash/builtins/readlink"
	_ "github.com/FourCoreLabs/go-bash/builtins/rm"
	_ "github.com/FourCoreLabs/go-bash/builtins/rmdir"
	_ "github.com/FourCoreLabs/go-bash/builtins/split"
	_ "github.com/FourCoreLabs/go-bash/builtins/stat"
	_ "github.com/FourCoreLabs/go-bash/builtins/touch"
	_ "github.com/FourCoreLabs/go-bash/builtins/tree"

	// Phase 10 Wave C: text processing.
	_ "github.com/FourCoreLabs/go-bash/builtins/base64"
	_ "github.com/FourCoreLabs/go-bash/builtins/cat"
	_ "github.com/FourCoreLabs/go-bash/builtins/column"
	_ "github.com/FourCoreLabs/go-bash/builtins/comm"
	_ "github.com/FourCoreLabs/go-bash/builtins/cut"
	_ "github.com/FourCoreLabs/go-bash/builtins/diff"
	_ "github.com/FourCoreLabs/go-bash/builtins/expand"
	_ "github.com/FourCoreLabs/go-bash/builtins/find"
	_ "github.com/FourCoreLabs/go-bash/builtins/fold"
	_ "github.com/FourCoreLabs/go-bash/builtins/head"
	_ "github.com/FourCoreLabs/go-bash/builtins/join"
	_ "github.com/FourCoreLabs/go-bash/builtins/md5sum"
	_ "github.com/FourCoreLabs/go-bash/builtins/nl"
	_ "github.com/FourCoreLabs/go-bash/builtins/od"
	_ "github.com/FourCoreLabs/go-bash/builtins/paste"
	_ "github.com/FourCoreLabs/go-bash/builtins/rev"
	_ "github.com/FourCoreLabs/go-bash/builtins/sha1sum"
	_ "github.com/FourCoreLabs/go-bash/builtins/sha256sum"
	_ "github.com/FourCoreLabs/go-bash/builtins/sort"
	_ "github.com/FourCoreLabs/go-bash/builtins/strings"
	_ "github.com/FourCoreLabs/go-bash/builtins/tac"
	_ "github.com/FourCoreLabs/go-bash/builtins/tail"
	_ "github.com/FourCoreLabs/go-bash/builtins/tee"
	_ "github.com/FourCoreLabs/go-bash/builtins/tr"
	_ "github.com/FourCoreLabs/go-bash/builtins/unexpand"
	_ "github.com/FourCoreLabs/go-bash/builtins/uniq"
	_ "github.com/FourCoreLabs/go-bash/builtins/wc"
	_ "github.com/FourCoreLabs/go-bash/builtins/xargs"

	// Phase 10 Wave D: pattern engines.
	_ "github.com/FourCoreLabs/go-bash/builtins/awk"
	_ "github.com/FourCoreLabs/go-bash/builtins/egrep"
	_ "github.com/FourCoreLabs/go-bash/builtins/fgrep"
	_ "github.com/FourCoreLabs/go-bash/builtins/grep"
	_ "github.com/FourCoreLabs/go-bash/builtins/jq"
	_ "github.com/FourCoreLabs/go-bash/builtins/rg"
	_ "github.com/FourCoreLabs/go-bash/builtins/sed"

	// Phase 10 Wave E: data formats.
	_ "github.com/FourCoreLabs/go-bash/builtins/sqlite3"
	_ "github.com/FourCoreLabs/go-bash/builtins/xan"
	_ "github.com/FourCoreLabs/go-bash/builtins/yq"

	// Phase 10 Wave F: archive / compression.
	_ "github.com/FourCoreLabs/go-bash/builtins/gunzip"
	_ "github.com/FourCoreLabs/go-bash/builtins/gzip"
	_ "github.com/FourCoreLabs/go-bash/builtins/tar"
	_ "github.com/FourCoreLabs/go-bash/builtins/zcat"

	// Phase 10 Wave G: environment / shell.
	_ "github.com/FourCoreLabs/go-bash/builtins/alias"
	_ "github.com/FourCoreLabs/go-bash/builtins/bash"
	_ "github.com/FourCoreLabs/go-bash/builtins/date"
	_ "github.com/FourCoreLabs/go-bash/builtins/env"
	_ "github.com/FourCoreLabs/go-bash/builtins/help"
	_ "github.com/FourCoreLabs/go-bash/builtins/history"
	_ "github.com/FourCoreLabs/go-bash/builtins/printenv"
	_ "github.com/FourCoreLabs/go-bash/builtins/sh"
	_ "github.com/FourCoreLabs/go-bash/builtins/time"
	_ "github.com/FourCoreLabs/go-bash/builtins/timeout"
	_ "github.com/FourCoreLabs/go-bash/builtins/unalias"

	// Phase 10 Wave H: network.
	_ "github.com/FourCoreLabs/go-bash/builtins/curl"
	_ "github.com/FourCoreLabs/go-bash/builtins/htmltomarkdown"

	// Phase 11: interpreter built-ins. Many command names below are
	// shadowed by mvdan/sh's native built-ins (cd, export, unset,
	// declare, read, :, true, false, pwd, set, shopt, exit, return,
	// break, continue, source, ., eval, hash, trap, readarray,
	// mapfile, dirs, pushd, popd, [, test, wait, getopts). For
	// shadowed entries the registered version is reachable only via
	// /bin/<name>; see DECISIONS.md (Phase 11) for the per-command
	// decision (override vs. accept shadow).
	_ "github.com/FourCoreLabs/go-bash/builtins/breakcmd"
	_ "github.com/FourCoreLabs/go-bash/builtins/cd"
	_ "github.com/FourCoreLabs/go-bash/builtins/colon"
	_ "github.com/FourCoreLabs/go-bash/builtins/compgen"
	_ "github.com/FourCoreLabs/go-bash/builtins/complete"
	_ "github.com/FourCoreLabs/go-bash/builtins/compopt"
	_ "github.com/FourCoreLabs/go-bash/builtins/continuecmd"
	_ "github.com/FourCoreLabs/go-bash/builtins/declare"
	_ "github.com/FourCoreLabs/go-bash/builtins/dirs"
	_ "github.com/FourCoreLabs/go-bash/builtins/eval"
	_ "github.com/FourCoreLabs/go-bash/builtins/exit"
	_ "github.com/FourCoreLabs/go-bash/builtins/export"
	_ "github.com/FourCoreLabs/go-bash/builtins/getopts"
	_ "github.com/FourCoreLabs/go-bash/builtins/hash"
	_ "github.com/FourCoreLabs/go-bash/builtins/jobs"
	_ "github.com/FourCoreLabs/go-bash/builtins/let"
	_ "github.com/FourCoreLabs/go-bash/builtins/local"
	_ "github.com/FourCoreLabs/go-bash/builtins/mapfile"
	_ "github.com/FourCoreLabs/go-bash/builtins/popd"
	_ "github.com/FourCoreLabs/go-bash/builtins/pushd"
	_ "github.com/FourCoreLabs/go-bash/builtins/read"
	_ "github.com/FourCoreLabs/go-bash/builtins/readonly"
	_ "github.com/FourCoreLabs/go-bash/builtins/returncmd"
	_ "github.com/FourCoreLabs/go-bash/builtins/set"
	_ "github.com/FourCoreLabs/go-bash/builtins/shopt"
	_ "github.com/FourCoreLabs/go-bash/builtins/source"
	_ "github.com/FourCoreLabs/go-bash/builtins/test"
	_ "github.com/FourCoreLabs/go-bash/builtins/trap"
	_ "github.com/FourCoreLabs/go-bash/builtins/umask"
	_ "github.com/FourCoreLabs/go-bash/builtins/unset"
	_ "github.com/FourCoreLabs/go-bash/builtins/wait"
)
