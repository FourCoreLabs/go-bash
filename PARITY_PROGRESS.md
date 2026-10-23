# Parity implementation progress

Baseline runtime: npm `just-bash@3.6.0`, locked with integrity metadata in
`internal/parity/upstream/package-lock.json`. The original source audit in
`PARITY_AUDIT.md` is historical; this file tracks subsequent implementation.

## Implemented and tested

- Literal per-call Args injection, RawScript indentation normalization, effective
  negative-limit validation, deprecated limit overrides, per-call env isolation.
- Dynamic native source/eval preprocessing: bounded input, parser budgets, loop
  instrumentation, virtual IDs, static expansion caps, VFS PATH search,
  current-shell state preservation, conservative depth checks. Supported EXIT/ERR
  trap callbacks are instrumented; function-shadowed calls stay unchanged.
- Context-shared nested command/loop/glob/output budgets, root cancellation, and
  avoiding double-charging already-limited output writers.
- Configurable input/heredoc parser budgets; default memory-FS total content
  budget with accounting for writes/truncation/lazy materialization/removal.
- New sandboxed mktemp and finite/bounded yes commands.
- sed in-place updates (suffix accepted but ignored, matching upstream), sort
  dictionary/month/output flags, uniq count padding.
- rg smartcase, ignore rules, common matching/pattern-file/replacement/traversal
  flags and upstream path/line output behavior.
- yq input/in-place flag corrections, slurp/null-input/exit-status/join-output.
- xan head/tail/sort/dedup/transpose/join, cat columns, zero-indexed headers.
- HTML-to-Markdown bullet/code/HR/heading customization.
- Cumulative network transforms, redirect transform recomputation, caller-header
  isolation, explicit empty method allow-list handling.
- Strict Go regression fixture loading and nonempty directory enforcement.
- Separate live differential CI gate with pinned npm dependency, exact stream/
  exit/filesystem comparisons and multi-exec cases. It does not normalize output
  or silently bless mismatches; exact waivers/XPASS handling are tested.

## Current verification

- `bash internal/parity/run.sh`: **24 scenarios PASS, zero waivers**.
- `go test -race ./...`: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.

Filesystem differential snapshots currently compare file bytes, directories,
symlink targets, creation and deletion under /work. Mode bits, hard-link identity,
network traces, optional runtimes, and full upstream test imports are not yet
covered by this gate. 24 passing scenarios do not establish full parity.

## Remaining work

- JavaScript and Python opt-in runtimes remain unimplemented; SQLite remains
  opt-in rather than the default real runtime.
- Complete xan expression/aggregation/groupby/pivot/map/transform semantics;
  additional rg/yq flags and format customization still require coverage.
- Default layout/environment parity and complex virtual process expansions.
- Dynamic alias/transform handling, diagnostic filenames/line numbers, complete
  wrapper behavior, aggregate substitution depth and complete dynamic AST paths.
  See `RUNTIME_DYNAMIC_NOTES.md` for architecture limitations.
- Full assignment/string, array-growth, descriptor, traversal/archive/database/
  worker/live-byte/deadline budgets and upstream profile selection. Existing
  defaults remain intentionally conservative; not all zero-budget conventions
  are uniform across builtins.
- Explicit network scalar omitted-vs-zero configuration and production private
  range defaults; preserve current Go API behavior pending an optional-field API.
- Tee PIPESTATUS preservation and complete synthesized AST serialization.
- Upstream-style result mapping for parse/limit/cancellation versus Go errors.
- Comprehensive version-provenanced upstream test conversion/import and broader
  differential cases including FS modes/hardlinks, mocked network, CLI/sandbox,
  transforms, limits and optional capabilities.

Security note: memfs storage enforcement prevents excess bytes being persisted,
but some native builtins ignore redirected writer errors. A zero exit status
alone must not be used as proof a redirected write succeeded. Storage and dynamic
instrumentation changes still merit independent adversarial review.
