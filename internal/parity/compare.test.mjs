import test from 'node:test';
import assert from 'node:assert/strict';
import { differences, classify } from './compare.mjs';

test('compares streams, exit status and file bytes without normalization', () => {
  assert.deepEqual(differences({stdout:'a\n'}, {stdout:'a'}), ['/stdout']);
  assert.deepEqual(differences({stderr:'err',exitCode:1}, {stderr:'',exitCode:0}), ['/exitCode','/stderr']);
  assert.deepEqual(differences({files:{'/work/a':{content:'YQ=='}}}, {files:{}}), ['/files/~1work~1a']);
});
test('waivers are exact, improvements and additional differences fail the gate', () => {
  assert.equal(classify([], []), 'PASS');
  assert.equal(classify(['/stdout'], ['/stdout']), 'XFAIL');
  assert.equal(classify([], ['/stdout']), 'XPASS');
  assert.equal(classify(['/stdout','/stderr'], ['/stdout']), 'FAIL');
  assert.equal(classify(['/stderr'], ['/stdout']), 'FAIL');
});
