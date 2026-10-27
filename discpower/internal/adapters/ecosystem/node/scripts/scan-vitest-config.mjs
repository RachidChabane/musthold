// The configuration of a scan's Vitest runs: the project's own configuration
// with the scan's settings. A Vitest outside the supported major versions is
// refused before any test runs. The shipped runner records the tests in one
// fork with no file parallelism; the shipped global setup wraps the project's;
// one reporter writes to the console; retries, bail and coverage are off; a
// suite with no test exits with 0; test files run in path order. Browser mode
// is refused: a scan records V8 coverage in Node.
import path from 'node:path';
import { createRequire } from 'node:module';
import { mergeConfig } from 'vitest/config';
// musthold:base

const SCRIPTS_VARIABLE = 'MUSTHOLD_SCAN_SCRIPTS';
const ROOT_VARIABLE = 'MUSTHOLD_SCAN_ROOT';
const RUNNER_VARIABLE = 'MUSTHOLD_SCAN_VITEST_RUNNER';
const SETUP_VARIABLE = 'MUSTHOLD_SCAN_GLOBAL_SETUP';
const RUNNER_SCRIPT = 'scan-vitest-runner.mjs';
const SETUP_SCRIPT = 'scan-vitest-global-setup.mjs';
const VERSION_SCRIPT = 'scan-runner-version.cjs';
const REPORTER = 'dot';
const POOL = 'forks';
const BROWSER_REFUSED = 'musthold scan runs Vitest in Node: browser mode is not supported';

class PathSequencer {
  constructor(context) {
    this.context = context;
  }

  async sort(files) {
    return [...files].sort((a, b) => (a.moduleId < b.moduleId ? -1 : a.moduleId > b.moduleId ? 1 : 0));
  }

  async shard(files) {
    return files;
  }
}

const listOf = (value) => (value === undefined || value === null ? [] : Array.isArray(value) ? value : [value]);

export default async (environment) => {
  const scripts = process.env[SCRIPTS_VARIABLE];
  const versions = createRequire(import.meta.url)(path.join(scripts, VERSION_SCRIPT));
  versions.requireSupported(process.env[ROOT_VARIABLE] || process.cwd(), versions.VITEST);
  const resolved = typeof base === 'function' ? await base(environment) : await base;
  const merged = mergeConfig(resolved || {}, {});
  const test = (merged.test = merged.test || {});
  if (test.browser && test.browser.enabled) throw new Error(BROWSER_REFUSED);
  const root = path.resolve(merged.root || test.root || process.cwd());
  if (test.runner) process.env[RUNNER_VARIABLE] = path.resolve(root, test.runner);
  process.env[SETUP_VARIABLE] = JSON.stringify(listOf(test.globalSetup).map((file) => path.resolve(root, file)));
  test.runner = path.join(scripts, RUNNER_SCRIPT);
  test.globalSetup = [path.join(scripts, SETUP_SCRIPT)];
  test.reporters = [REPORTER];
  test.pool = POOL;
  test.fileParallelism = false;
  test.maxWorkers = 1;
  test.retry = 0;
  test.bail = 0;
  test.passWithNoTests = true;
  test.watch = false;
  test.coverage = Object.assign({}, test.coverage, { enabled: false });
  test.sequence = Object.assign({}, test.sequence, { sequencer: PathSequencer, shuffle: false });
  return merged;
};
