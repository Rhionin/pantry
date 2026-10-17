// @stryker-mutator/vitest-runner@10.0.0 joins suite and test names with a space
// when it builds test ids and the per-test name filter. Vitest 5 matches
// testNamePattern against the full name joined with " > " (see the Vitest 5
// migration guide). The filter matches nothing, so every covered mutant is
// reported Survived after running zero tests.
//
// The dry run still records coverage, and the runner returns that coverage
// even when coverageAnalysis is "off", so the planner keeps using the broken
// filter. https://github.com/stryker-mutator/stryker-js/issues/6210
//
// Two copies of collectTestName have to change together:
//   dist/src/test-helpers.js   — test ids and the mutant-run name filter
//   dist/src/stryker-setup.js  — coverage keys written during the dry run
// Patching only one of them makes every mutant NoCoverage.
//
// Drop this script when a published runner contains the fix (stryker-js
// #6214 / #6220). A runner whose collectTestName no longer matches either
// shape fails here instead of reporting a silent 0% score.

import { readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';

const broken = "return nameParts.join(' ').trim();";
const fixed = "return nameParts.filter((part) => part !== '').join(' > ').trim();";
const files = ['test-helpers.js', 'stryker-setup.js'];

const require = createRequire(import.meta.url);
let packageJson;
try {
  packageJson = require.resolve('@stryker-mutator/vitest-runner/package.json');
} catch {
  console.error(
    'Cannot find @stryker-mutator/vitest-runner. Install frontend dependencies before running mutation tests.',
  );
  process.exit(1);
}

const dist = path.join(path.dirname(packageJson), 'dist', 'src');

for (const file of files) {
  const fullPath = path.join(dist, file);
  const source = readFileSync(fullPath, 'utf8');
  const brokenCount = countOccurrences(source, broken);
  const fixedCount = countOccurrences(source, fixed);

  if (brokenCount === 1 && fixedCount === 0) {
    writeFileSync(fullPath, source.replace(broken, fixed));
    console.log(`Patched ${file} so Vitest 5 test names use " > ".`);
    continue;
  }

  if (brokenCount === 0 && fixedCount === 1) {
    console.log(`${file} already joins Vitest test names with " > ".`);
    continue;
  }

  console.error(
    `${fullPath} does not match the known vitest-runner@10.0.0 collectTestName shape ` +
      `(space-joined copies: ${brokenCount}, Vitest 5 copies: ${fixedCount}). ` +
      'Refusing to run Stryker, because an unpatched Vitest 5 filter reports false survivors. ' +
      'Update scripts/ensure-stryker-vitest5.mjs for this runner version.',
  );
  process.exit(1);
}

function countOccurrences(source, needle) {
  return source.split(needle).length - 1;
}
