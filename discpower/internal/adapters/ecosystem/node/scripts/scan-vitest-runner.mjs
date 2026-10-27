// The test runner of a scan's Vitest runs, in the one fork the run uses. It
// extends Vitest's runner, or the project's own runner when
// MUSTHOLD_SCAN_VITEST_RUNNER names it, to:
//   - keep the selected tests alone, skipping every other one once collected;
//   - open the window of a test recorded alone before its first beforeEach
//     hook and close it when its test function settles;
//   - tell a hook failure from a failure of the test function, and spot
//     retries;
//   - record a test file that stopped before any test began.
import vm from 'node:vm';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const require = createRequire(import.meta.url);
const agent = require('./scan-agent.cjs');
const versions = require('./scan-runner-version.cjs');

const ROOT_VARIABLE = 'MUSTHOLD_SCAN_ROOT';
const RUNNER_VARIABLE = 'MUSTHOLD_SCAN_VITEST_RUNNER';
const RUNNER = 'vitest';
const ID_SEPARATOR = '::';
const TITLE_SEPARATOR = ' > ';
const RUNNERS_EXPORT = './runners';
const SUITE_EXPORT = './suite';
// UNRESOLVED_VERSION stands for the version of a Vitest the project's root
// does not resolve.
const UNRESOLVED_VERSION = '(unresolved)';
const NO_RUNNER_CLASS = 'it exports no test runner class from vitest/runners or vitest, or no test function getter';

const STATE_PASS = 'pass';
const STATE_FAIL = 'fail';
const MODE_SKIP = 'skip';
const TYPE_TEST = 'test';

const OUTCOME_PASSED = 'passed';
const OUTCOME_FAILED = 'failed';
const OUTCOME_ERRORED = 'errored';
const OUTCOME_SKIPPED = 'skipped';

async function importExport(root, subpath) {
  const url = versions.exportURL(root, versions.VITEST, subpath);
  return url ? import(url) : {};
}

// vitestRunnerAPI is the test runner class a scan's runner extends and the
// getter of a test's function, from where the installed major version of
// Vitest exports them: `VitestTestRunner` of `vitest/runners`, else
// `TestRunner` of the package itself; the runner's static `getTestFn`, else
// `getFn` of `vitest/suite`.
async function vitestRunnerAPI(root) {
  const version = versions.requireSupported(root, versions.VITEST);
  const TestRunner = (await importExport(root, RUNNERS_EXPORT)).VitestTestRunner || (await importExport(root, versions.MAIN_EXPORT)).TestRunner;
  const getTestFn = (TestRunner && TestRunner.getTestFn) || (await importExport(root, SUITE_EXPORT)).getFn;
  if (!TestRunner || !getTestFn) throw new Error(versions.refusal(versions.VITEST, version || UNRESOLVED_VERSION, NO_RUNNER_CLASS));
  return { TestRunner, getTestFn };
}

const root = process.env[ROOT_VARIABLE] || process.cwd();
const { TestRunner, getTestFn } = await vitestRunnerAPI(root);
const Base = process.env[RUNNER_VARIABLE] ? (await import(pathToFileURL(process.env[RUNNER_VARIABLE]).href)).default : TestRunner;

function titlesOf(task) {
  const titles = [];
  for (let current = task; current && current !== task.file; current = current.suite) titles.unshift(current.name);
  return titles;
}

function idOf(task) {
  return agent.relative(task.file.filepath) + ID_SEPARATOR + titlesOf(task).join(TITLE_SEPARATOR);
}

// select skips every test the selection leaves out, and every suite left with
// no test to run, and reports whether the task keeps one.
function select(task) {
  if (task.type === TYPE_TEST) {
    if (!agent.selected(idOf(task))) task.mode = MODE_SKIP;
    return task.mode !== MODE_SKIP;
  }
  let kept = false;
  for (const child of task.tasks || []) kept = select(child) || kept;
  if (!kept && task !== task.file) task.mode = MODE_SKIP;
  return kept;
}

function testsOf(task) {
  if (task.type === TYPE_TEST) return [task];
  return (task.tasks || []).flatMap(testsOf);
}

export default class ScanRunner extends Base {
  constructor(config) {
    super(config);
    agent.activate(RUNNER);
    agent.checkRealm((code, filename) => vm.runInThisContext(code, { filename }));
    this.scanRecords = new Map();
    this.scanRealTimeout = globalThis.setTimeout;
  }

  recordOf(test) {
    if (!this.scanRecords.has(test)) {
      this.scanRecords.set(test, { id: idOf(test), fnStarted: false, fnFailed: false, errorsAfterFn: 0, retried: false, hook: false });
    }
    return this.scanRecords.get(test);
  }

  timers() {
    const clock = globalThis.setTimeout && globalThis.setTimeout.clock;
    if (clock) return agent.clockTimers(clock);
    return { faked: globalThis.setTimeout !== this.scanRealTimeout };
  }

  onCollectStart(file) {
    agent.phase(agent.FOREIGN);
    agent.file(file.filepath);
    return super.onCollectStart ? super.onCollectStart(file) : undefined;
  }

  onCollected(files) {
    for (const file of files) {
      if (file.result && file.result.state === STATE_FAIL && testsOf(file).length === 0) agent.fileFailed(file.filepath);
      if (agent.selecting()) select(file);
      for (const test of testsOf(file)) {
        if (test.mode === MODE_SKIP && !agent.selected(idOf(test))) continue;
        if (agent.hollow()) this.recordOf(test).hollow = true;
        this.report(test);
      }
    }
    return super.onCollected ? super.onCollected(files) : undefined;
  }

  onBeforeRunSuite(suite) {
    agent.phase(agent.FOREIGN);
    if (suite === suite.file) agent.file(suite.filepath);
    return super.onBeforeRunSuite ? super.onBeforeRunSuite(suite) : undefined;
  }

  onBeforeRunTask(test) {
    agent.phase(agent.NEUTRAL);
    return super.onBeforeRunTask ? super.onBeforeRunTask(test) : undefined;
  }

  onBeforeTryTask(test, options) {
    const record = this.recordOf(test);
    if (options && options.retry > 0) record.retried = true;
    agent.open(record.id, this.timers());
    if (test.concurrent) agent.concurrent(record.id);
    return super.onBeforeTryTask ? super.onBeforeTryTask(test, options) : undefined;
  }

  async runTask(test) {
    const record = this.recordOf(test);
    record.fnStarted = true;
    await agent.fnStart(record.id);
    let threw = false;
    try {
      if (record.hollow) return;
      if (super.runTask) await super.runTask(test);
      else await getTestFn(test)();
    } catch (error) {
      threw = true;
      record.fnFailed = true;
      throw error;
    } finally {
      record.errorsAfterFn = ((test.result && test.result.errors) || []).length + (threw ? 1 : 0);
      if (!threw && ((test.result && test.result.errors) || []).length > 0) record.fnFailed = true;
      await agent.fnEnd(record.id);
    }
  }

  onTaskFinished(test) {
    agent.close(this.recordOf(test).id);
    agent.phase(agent.FOREIGN);
    return super.onTaskFinished ? super.onTaskFinished(test) : undefined;
  }

  outcomeOf(test, record) {
    const state = test.result && test.result.state;
    if (record.hook) return OUTCOME_ERRORED;
    const retried = record.retried || ((test.result && test.result.retryCount) || 0) > 0;
    if (state === STATE_PASS) return retried ? OUTCOME_ERRORED : OUTCOME_PASSED;
    if (state !== STATE_FAIL) return OUTCOME_SKIPPED;
    const errors = ((test.result && test.result.errors) || []).length;
    if (!record.fnStarted || !record.fnFailed || retried || errors > record.errorsAfterFn) return OUTCOME_ERRORED;
    return OUTCOME_FAILED;
  }

  report(test) {
    const record = this.recordOf(test);
    agent.result(test, { id: record.id, file: agent.relative(test.file.filepath), outcome: this.outcomeOf(test, record) });
  }

  onAfterRunTask(test) {
    agent.close(this.recordOf(test).id);
    this.report(test);
    return super.onAfterRunTask ? super.onAfterRunTask(test) : undefined;
  }

  onAfterRunSuite(suite) {
    agent.phase(agent.FOREIGN);
    const errors = (suite.result && suite.result.errors) || [];
    if (errors.length > 0) {
      for (const test of testsOf(suite)) {
        if (!agent.selected(idOf(test))) continue;
        this.recordOf(test).hook = true;
        this.report(test);
      }
    }
    return super.onAfterRunSuite ? super.onAfterRunSuite(suite) : undefined;
  }

  async onAfterRunFiles(files) {
    await agent.flush();
    return super.onAfterRunFiles ? super.onAfterRunFiles(files) : undefined;
  }
}
