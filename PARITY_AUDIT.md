# just-bash parity audit

## Verdict and baseline

**Feature parity is not achieved.** Passing the Go suite is evidence of internal
regression coverage, not of equivalence to just-bash. Do not advertise
byte-compatible behavior across the supported command set yet.

Baseline: `vercel-labs/just-bash` commit
`7537a260e38648e8998db7a95504102ad80b0194`, whose
`packages/just-bash/package.json` declares version **3.6.0**. This is a
commit-pinned source comparison, not a comparison against the published npm
artifact. Upstream paths below are relative to `packages/just-bash/src`.

Method: inspect Go registration, command implementations, interpreter bridge,
options, limits, filesystem/network initialization, fixtures, and upstream
counterparts; run `go test ./...`. All Go packages passed before changes.
No upstream runtime differential suite was executed. Findings below are
source-confirmed gaps or risks; exact output differences still need differential
reproductions. This is a prioritized audit, not an exhaustive flag-by-flag or
security certification. Real-bash differences alone do not establish a
just-bash mismatch.

## Highest-priority runtime findings

### Source/eval have split semantics and incomplete limit coverage

Go's `interp/runner.go:187–213,254–291` leaves bare mvdan builtins to mvdan.
Bare `source`, `.`, and `eval` therefore do not run the registered implementations.
The existing source-depth test uses `/bin/source`
(`phase11_sourcedepth_test.go:13–28`), so it does not establish depth enforcement
for ordinary `source file` scripts.

The registry implementations (`builtins/source/source.go:33–58`,
`builtins/eval/eval.go:32–48`) re-enter `execLocked` with a fresh runner. This
cannot provide upstream's current-shell mutation semantics. Source also resolves
only against cwd rather than upstream's PATH-first lookup for names without `/`.
Upstream: `interpreter/builtins/source.ts:29–147`,
`interpreter/builtins/eval.ts:59–100`.

Bare dynamically parsed source/eval bodies bypass the outer AST passes at
`bash.go:405–435`: loop instrumentation, virtual process-ID rewriting, and
expansion-cap checks. Investigate with bounded/cancellable regressions before
running adversarial cases. `DECISIONS.md`'s statement that these bare commands
re-enter the Go depth plumbing is not supported by this dispatch path.

### Declared execution options are not implemented

`ExecOptions.Args` and `RawScript` (`options.go:84–105`) are not consumed by
`execLocked`. Upstream injects literal Args into the first command and normalizes
multiline indentation unless rawScript is set
(`Bash.ts:775–785`, `interpreter/interpreter.ts:931–935`).
Deprecated top-level limit fields are also declared but not used in Go's limit
resolution; upstream applies their overrides (`Bash.ts:343–357`).

### Per-call environment isolation differs

Go merges returned exported variables into persistent `b.env` when no per-call
Env is supplied **or when ReplaceEnv is true** (`bash.go:601–617,770–778`).
Upstream replacement environments must not affect the next execution
(`Bash.exec-options.test.ts:117–143`, `Bash.ts:730–755`). Unsetting an exported
variable also does not remove old persistent Go entries via that merge.
The repo's README and AGENTS descriptions disagree about environment persistence.

### Limits are neither equivalent nor uniformly enforced

| Budget | Go default | Upstream normal default |
|---|---:|---:|
| Commands / shell loops / AWK / SED | 10,000 | 100,000 |
| jq iterations | 10,000 | 10,000,000 |
| Glob operations | 100,000 | 1,000,000 |
| String / heredoc bytes | 10 MiB | 64 MiB |
| Output bytes | 10 MiB | 256 MiB |
| Array elements | 100,000 | 1,000,000 |
| Brace results | 10,000 | 100,000 |
| File descriptors | 1,024 | 4,096 |
| SQLite timeout | 5 seconds | 30 seconds |

Sources: Go `limits.go:45–66`; upstream `limits.ts:163–260`. Go's defaults
resemble parts of upstream's hardened profile, but there is no profile selector.
Do not simply raise Go limits to claim parity without reviewing enforcement.

Additional gaps:
- Parser input is fixed at 1 MiB; configured heredoc limits are not passed into
  parsing (`parser/parser.go:15–39`, `parser/translate.go:535–537`).
- Registry child executions allocate new counters rather than sharing one
  execution scope (`bash.go:483–486,555–565,820–836`).
- Negative limit overrides are not validated (`limits.go:71–130`).
- MaxFileDescriptors is not enforced by the runner's general open handler
  (`interp/runner.go:383–390`); tee checks operand count instead.
- MaxStringLength checks command arguments, not assignments (also recorded in
  `DECISIONS.md:75–87`).
- No memory-FS total storage budget (`fs/memfs/memfs.go:60–81`), unlike upstream
  `maxFileSystemBytes` (`Bash.ts:359–363`).
- Newer upstream aggregate work/input/live-byte, traversal/archive/database,
  worker, execution-depth, and deadline budgets have no equivalent Go surface
  (`limits.ts:12–150` versus Go `limits.go:12–31`).

### Error reporting contract differs

Go reports parse/limit/cancellation failures as Go errors, sometimes with a
zero-valued result exit status (`bash.go:405–408,623–648`). Upstream returns
execution results (cancellation 124, limit 126; parse errors in stderr)
(`Bash.ts:883–917`). A Go-style error API can be intentional, but callers need
an explicit compatibility mapping rather than assuming identical results.

## Command and optional-capability gaps

Go registration inventory: `builtins/builtins.go:19–159`.
Upstream: `commands/registry.ts`.

| Area | Gap | Go evidence | Upstream evidence |
|---|---|---|---|
| `mktemp`, `yes` | Missing implementations/registration | registration inventory | registry lines 26,75,148–151 |
| Python / JS | No opt-in `python3`/`python`, `js-exec`/`node` runtimes | README Optional Runtimes; no packages | registry lines 110–121 |
| SQLite | Default command is a stub; real runtime needs registration | `builtins/sqlite3/sqlite3.go:31–39`, `sqlite/sqlite.go:52` | registry line 103 |
| sed | Rejects `-i`; no in-place/backup equivalent | `builtins/sed/sed.go:80–81` | `commands/sed/sed.ts:372–375,481–519` |
| yq | `-i FMT` means input format, not in-place; `-p` changes both formats rather than input only | `builtins/yq/yq.go:45–49,69–100` | `commands/yq/yq.ts:115–129,184–245` |
| yq | Missing INI, slurp/null-input/exit-status/join-output/front-matter and multiple formatting options | same parser/help | same implementation |
| rg | Narrow flag subset; no .gitignore handling; no default smart-case | `builtins/rg/rg.go:95–168,185–190` | `commands/rg/rg.ts:26–85`, `rg-parser.ts` |
| xan | Only 10 subcommands; missing implemented upstream agg/groupby/join/map/sort/dedup/pivot/transform/transpose/head/tail; rejects cat columns | `builtins/xan/xan.go:62–84,451` | `commands/xan/xan.ts:14–55,74–116` |
| sort | Missing dictionary/month ordering and output file | `builtins/sort/sort.go:52–105` | `commands/sort/sort.ts:90–104,142–143` |
| html-to-markdown | Rejects formatting customization flags | `builtins/htmltomarkdown/htmltomarkdown.go:48–55,73` | `commands/html-to-markdown/html-to-markdown.ts:79–96` |

Count optional-runtime gaps separately from default-command gaps. Do not count
commands that upstream itself marks unimplemented as missing Go features.
CLI advertising of Python/JavaScript flags does not establish runtime support.
Different jq/AWK/regex/HTML/SQLite libraries also require substantial behavioral
coverage, not just command-name matching.

## Filesystem, network, and transform differences

- Default Go environment initialization seeds only HOME/PATH under its layout
  condition (`bash.go:217–237`); upstream also initializes shell metadata and
  retains default env values when files/cwd are supplied (`Bash.ts:366–382`).
- Go skips default layout with cwd or nonempty files; upstream still seeds
  bin/dev/proc on sync-initializable FS. Empty Files maps have different semantics.
  Go omits upstream's fd/stdin/stdout/stderr backing entries and richer proc data
  (`fs_init.go:34–46,99–127`; upstream `fs/init.ts:38–119`).
- Go picks the **first** matching network allow-list entry for transforms, while
  upstream merges **all** matching entries, later headers winning
  (`network/allowlist.go:117–126`, `network/securefetch.go:301–315`;
  upstream `network/fetch.ts:143–164`). This also conflicts with Go's own
  cumulative-transform comment in `network/config.go:90–93`.
- Network defaults differ: Go zero MaxRedirects becomes 20, explicit empty
  AllowedMethods becomes GET/HEAD, and private-range blocking defaults false;
  upstream accepts zero redirects/empty methods and defaults private blocking
  true in production (`network/config.go:47–67,124–140`;
  upstream `network/fetch.ts:167–182`). Pointer/optional fields may be needed to
  distinguish unset values from explicit zero values in Go.
- TeePlugin does not preserve PIPESTATUS; inverse AST serialization is partial;
  alias expansion drops some compound bodies/redirections. These known gaps
  are recorded in `DECISIONS.md`. They require upstream-backed tests, not
  assertions pinning Go's existing output.
- Process-ID rewriting covers simple expansions only (`procinfo.go`,
  `DECISIONS.md:322–346`); complex/dynamically parsed forms need tests for virtual
  identity and host-identity leakage. This is also a security concern.

## Test-evidence quality and changes from this audit

`internal/cmpfixture` runs Go against stored expected results, not against
just-bash. Expectations have no upstream commit/version provenance. Upstream
comparison fixtures are ID-keyed `{command, files, stdout, stderr, exitCode}`
records, not Go's single `{script, expected}` record
(upstream `comparison-tests/fixture-runner.ts:31–49`). Direct copying is invalid.

The sampled sed/rg/xan/yq directories each have only a basic fixture, not coverage
of their advanced options. SQLite stub expectations assert deliberate
non-parity. Fixture counts cannot be interpreted as matched feature counts.

This audit hardens the Go loader to reject unknown fields, missing/null required
fields, and trailing JSON; an explicitly empty script remains valid. Empty
fixture directories now fail. Tests cover malformed/upstream-shaped input.
Comments no longer claim direct upstream-format compatibility. These changes
prevent false-green fixture imports; they do **not** close runtime parity gaps.

## Recommended closure plan

1. **Fix instrumentation/security coverage first.** Add bounded tests of ordinary
   source/eval, recursive execution, dynamic IDs, storage and shared budgets.
   Route current-shell builtins through a consistent state/limit-aware mechanism.
2. **Implement declared API behavior.** Args, RawScript, legacy limits, environment
   isolation; explicitly document intentional Go API differences.
3. **Close command gaps.** mktemp/yes, yq flags, sed in-place, rg defaults/options,
   xan subcommands, sort and converter flags. Make optional runtimes a separately
   scoped milestone.
4. **Add a pinned differential gate.** Run fresh Go and upstream Bash instances
   with identical files/env/cwd/stdin/options. Compare stdout/stderr/exit status,
   resulting FS bytes/modes/links, and multi-exec state. Handle nondeterministic
   date/PID values explicitly; do not broadly normalize output. Test network with
   deterministic mocked requests and redirect/transform cases.
5. **Import upstream tests with conversion and provenance.** Record source commit,
   test ID, options and expected result origin. Separate deliberate differences
   from matches; never regenerate expected output from Go to bless a mismatch.
6. **Expand beyond command smoke tests.** Shell expansion/control flow, parser and
   AST round trips, FS contract, transforms, sandbox/CLI, limits and optional
   capabilities each need dedicated coverage. Review upstream test counts and
   unsupported cases before choosing a measurable parity target.

Certification requires a stated version/configuration scope, zero unexplained
mismatches in that scope, an explicit exclusion list, and repeatable CI evidence.
This audit does not certify full parity.
