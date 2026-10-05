# Opt-in live upstream differential gate

Run from the repository root (Go 1.25, Node >=20.19, npm):

```sh
bash internal/parity/run.sh
```

This installs **npm `just-bash@3.6.0`**, with transitive versions and integrity
hashes locked in `upstream/package-lock.json`, into a temporary directory outside
the repository. Lifecycle scripts are disabled; the selected cases do not need
optional native dependencies. The temporary install is deleted afterward. No npm
install, network request, or upstream execution is added to `go test ./...`.
The Go JSON adapter is compiled as an ordinary command by Go tooling.

For repeated local runs, install once outside the checkout:

```sh
mkdir -p /tmp/go-bash-parity-upstream
cp internal/parity/upstream/package{,-lock}.json /tmp/go-bash-parity-upstream/
npm ci --prefix /tmp/go-bash-parity-upstream --ignore-scripts --no-audit --no-fund
PARITY_UPSTREAM_DIR=/tmp/go-bash-parity-upstream bash internal/parity/run.sh
```

The harness verifies the installed version, invokes `go run
./internal/parity/cmd/runner`, and executes the same JSON scenarios against the
live upstream runtime. `PARITY_GO_RUNNER=/absolute/path/to/built/runner` can skip
`go run`. `PARITY_REPORT_DIR` changes the default `/tmp/go-bash-parity-report`.
CI runs `.github/workflows/parity.yml` independently of the default Go test job,
with a failing exit status for unexpected differences, improvements (XPASS),
runner errors, exceptions, or timeouts. Configure branch protection to require
**just-bash 3.6.0 differential gate** if desired. It uploads both runtimes' live
observations as `report.json` even on comparison failure.

## Baseline and comparison contract

The audit used git commit `7537a260e38648e8998db7a95504102ad80b0194`, which
identifies itself as 3.6.0. This suite deliberately uses the **published npm
artifact**, not that checkout. They are not assumed to be identical.

Each scenario creates a fresh Bash with cwd `/work`, UTF-8 seeded files and an
explicit constructor environment. Steps share the Bash/filesystem but get
individual exec options (stdin, cwd, env, replaceEnv, args, rawScript supported
by the adapter). Every step compares stdout, stderr and exitCode exactly: no
trimming, whitespace normalization, stderr suppression or real-bash oracle.
After each step, a recursive `/work` snapshot compares all path names, node
types, symlink targets and regular-file **bytes** (base64). Empty files and
empty directories count. Snapshots use filesystem APIs, not extra Exec calls,
so observing the filesystem cannot change shell state. Deletions, new files,
renames, redirected outputs and cross-call filesystem persistence are covered.

Only `/work` is snapshotted; default `/bin`, `/proc`, timestamps, inode IDs,
permissions and result metadata are deliberately outside this contract. Cases
must seed and mutate files under `/work`. Seeds are text; resulting files may
contain arbitrary bytes. This is bounded realistic coverage, not full parity
certification, limits/security fuzzing, optional runtimes, or network coverage.
Exec errors fail the suite, rather than silently mapping Go errors to upstream
exit codes. Each execution has a five-second deadline; the Go subprocess also
has a wall-clock timeout and bounded output capture.

## Cases and known mismatches

`cases.json` covers streams/nonzero status, text pipelines, filesystem lifecycle,
functions/loops/substitution, stdin and cwd isolation, multi-exec env mutations,
per-call env merge/replacement, literal Args, source/eval, sed backups, sort
output files, rg no-match, jq/yq transformations, rawScript options and binary
file round-tripping via base64.

Known mismatches have a reason and an **exact set of differing JSON-pointer
paths** in `expectedMismatch`, not stored approved outputs:

* `pipeline-text`: Go `uniq -c` pads counts to width 7, upstream to width 4;
  stdout and the redirected file bytes differ.
* `sed-inplace`: npm 3.6.0 does not create a backup for `-i.bak`, while Go does;
  the backup node, subsequent cat stdout/stderr and final exit status differ.

An extra differing field fails. A waived field becoming equal also fails, so
fixes require removing or narrowing the waiver. Existing waived values are not
frozen: review both live values in the artifact; field-level waivers cannot
guarantee that a still-differing value hasn't changed in another way. Never
populate golden Go outputs or automatically accept newly discovered mismatches.
To add a case, omit a waiver first, run both runtimes, inspect the report, and
only document a specific reproducible known gap when necessary.

Comparator regression checks run explicitly via:

```sh
node --test internal/parity/compare.test.mjs
```
