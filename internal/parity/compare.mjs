import { isDeepStrictEqual } from 'node:util';

// Compare byte-preserving observations, reporting leaf JSON pointers.
export function differences(a, b, p = '') {
  if (isDeepStrictEqual(a, b)) return [];
  if (a && b && typeof a === 'object' && typeof b === 'object' && Array.isArray(a) === Array.isArray(b)) {
    return [...new Set([...Object.keys(a), ...Object.keys(b)])].sort().flatMap(k =>
      differences(a[k], b[k], `${p}/${k.replaceAll('~', '~0').replaceAll('/', '~1')}`));
  }
  return [p];
}
export function classify(paths, expected = []) {
  return isDeepStrictEqual([...paths].sort(), [...expected].sort())
    ? (paths.length ? 'XFAIL' : 'PASS') : (!paths.length ? 'XPASS' : 'FAIL');
}
