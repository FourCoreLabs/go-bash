import { createRequire } from 'node:module';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { differences, classify } from './compare.mjs';
import { fileURLToPath } from 'node:url';

// No ambient node_modules lookup: the upstream install must be supplied explicitly.
const upstreamDir = process.env.PARITY_UPSTREAM_DIR;
if (!upstreamDir) throw new Error('Set PARITY_UPSTREAM_DIR to the external npm install directory');
const require = createRequire(join(resolve(upstreamDir), 'package.json'));
const version = JSON.parse(readFileSync(join(resolve(upstreamDir), 'node_modules/just-bash/package.json'), 'utf8')).version;
if (version !== '3.6.0') throw new Error(`Wrong upstream version: ${version}`);
const { Bash } = require('just-bash');
const here = fileURLToPath(new URL('.', import.meta.url));
const cases = JSON.parse(readFileSync(join(here, 'cases.json'), 'utf8'));
const ids = new Set();
for (const c of cases) {
  if (!c.id || ids.has(c.id) || !c.steps?.length) throw new Error('Missing/duplicate ID or empty steps');
  ids.add(c.id);
  for (const p of Object.keys(c.files ?? {})) {
    if (!p.startsWith('/work/') || p.split('/').includes('..')) throw new Error(`Seed outside snapshot: ${p}`);
  }
  if (c.expectedMismatch && (!c.expectedMismatch.reason || !c.expectedMismatch.paths?.length || new Set(c.expectedMismatch.paths).size !== c.expectedMismatch.paths.length)) throw new Error(`Invalid expectation: ${c.id}`);
}
const go = spawnSync(process.env.PARITY_GO_RUNNER ?? 'go', process.env.PARITY_GO_RUNNER ? [] : ['run', './internal/parity/cmd/runner'], {
  input: JSON.stringify(cases), encoding: 'utf8', timeout: 120_000, maxBuffer: 16 * 1024 * 1024,
});
if (go.error || go.status !== 0) throw new Error(`Go adapter failed: ${go.error ?? go.stderr}`);
const actual = JSON.parse(go.stdout);

async function snapshot(fs) {
  const entries = {};
  async function walk(p) {
    const st = await fs.lstat(p);
    if (st.isSymbolicLink) entries[p] = { type: 'symlink', target: await fs.readlink(p) };
    else if (st.isDirectory) {
      entries[p] = { type: 'directory' };
      for (const name of (await fs.readdir(p)).sort()) await walk(`${p}/${name}`);
    } else entries[p] = { type: 'file', ...((await fs.readFileBuffer(p)).length ? {content: Buffer.from(await fs.readFileBuffer(p)).toString('base64')} : {}) };
  }
  await walk('/work');
  return entries;
}
// JSON pointer paths identify differences, not approved values. Both live outputs
// remain in the report. A newly matching path (XPASS) requires removing the waiver.
const report = { baseline: { package: 'just-bash', version, sourceAuditCommit: '7537a260e38648e8998db7a95504102ad80b0194', artifact: 'npm (not the audited git checkout)' }, cases: [] };
let failures = 0;
for (const c of cases) {
  const bash = new Bash({ cwd: '/work', files: c.files ?? {}, env: c.env ?? {} });
  const upstream = [];
  for (const { script, ...options } of c.steps) {
    // Exceptions are infrastructure/runtime failures, never expected mismatches.
    const r = await bash.exec(script, { ...options, signal: AbortSignal.timeout(5000) });
    upstream.push({ stdout: r.stdout, stderr: r.stderr, exitCode: r.exitCode, files: await snapshot(bash.fs) });
  }
  const paths = differences(actual[c.id], upstream).sort();
  const expected = (c.expectedMismatch?.paths ?? []).slice().sort();
  const status = classify(paths, expected);
  if (status === 'FAIL' || status === 'XPASS') failures++;
  console.log(`${status} ${c.id}${paths.length ? `: ${paths.join(', ')}` : ''}`);
  report.cases.push({ id: c.id, status, paths, expectedMismatch: c.expectedMismatch ?? null, go: actual[c.id], upstream });
}
const reportDir = resolve(process.env.PARITY_REPORT_DIR ?? '/tmp/go-bash-parity-report');
mkdirSync(reportDir, { recursive: true });
writeFileSync(join(reportDir, 'report.json'), JSON.stringify(report, null, 2) + '\n');
console.log(`Compared ${cases.length} scenarios against just-bash@${version}; report: ${reportDir}/report.json`);
if (failures) process.exitCode = 1;
