// The test environment of a scan's Jest runs. It wraps the environment each
// project configures, which SPEC_PROOF_SCAN_JEST_ENVIRONMENTS names, and reads
// jest-circus events to:
//   - keep the selected tests alone, skipping every other one at run_start;
//   - open the window of a test recorded alone before its first beforeEach
//     hook and close it when its test function settles;
//   - tell a hook failure from a failure of the test function, and spot
//     retries;
//   - record a test file that stopped before any test began.
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const agent = require('./scan-agent.cjs');

const ENVIRONMENTS_VARIABLE = 'SPEC_PROOF_SCAN_JEST_ENVIRONMENTS';
const ROOT_VARIABLE = 'SPEC_PROOF_SCAN_ROOT';
const DEFAULT_ENVIRONMENT = 'jest-environment-node';
const KEY_SEPARATOR = '\u0000';
const RUNNER = 'jest';
const ID_SEPARATOR = '::';
const TITLE_SEPARATOR = ' > ';
const STATE = Symbol('spec-proof.scan.jest');
const SINON_TIMER_KEY = 'func';
const LEGACY_TIMER_KEY = 'callback';

const OUTCOME_PASSED = 'passed';
const OUTCOME_FAILED = 'failed';
const OUTCOME_ERRORED = 'errored';
const OUTCOME_SKIPPED = 'skipped';

const HOOK_BEFORE_EACH = 'beforeEach';
const HOOK_AFTER_EACH = 'afterEach';
const EACH_HOOKS = new Set([HOOK_BEFORE_EACH, HOOK_AFTER_EACH]);

function innerEnvironments() {
  try {
    return JSON.parse(process.env[ENVIRONMENTS_VARIABLE] || '{}');
  } catch {
    return {};
  }
}

function load(file) {
  const exported = require(file);
  return exported.TestEnvironment || exported.default || exported;
}

function realpath(file) {
  try {
    return fs.realpathSync(file);
  } catch {
    return file;
  }
}

// innerOf loads the environment a project configures, which the variable
// names by the project's root relative to the scan's root and its display
// name, relative to the scan's root when it lies in it.
function innerOf(projectConfig) {
  const root = realpath(process.env[ROOT_VARIABLE] || projectConfig.rootDir || process.cwd());
  const key = path.relative(root, realpath(projectConfig.rootDir)).split(path.sep).join('/') || '.';
  const displayName = projectConfig.displayName ? projectConfig.displayName.name || '' : '';
  const named = innerEnvironments()[key + KEY_SEPARATOR + displayName];
  if (named) return load(path.isAbsolute(named) ? named : path.join(root, named));
  return load(createRequire(path.join(root, 'package.json')).resolve(DEFAULT_ENVIRONMENT));
}

function titlesOf(test) {
  const titles = [];
  for (let block = test; block && block.parent; block = block.parent) titles.unshift(block.name);
  return titles;
}

function isUnder(test, block) {
  for (let parent = test.parent; parent; parent = parent.parent) {
    if (parent === block) return true;
  }
  return false;
}

function testsOf(block) {
  const tests = [];
  for (const child of block.children) {
    if (child.type === 'describeBlock') tests.push(...testsOf(child));
    else tests.push(child);
  }
  return tests;
}

// hollowed is a function that does nothing in place of a test function, with
// its arity, so that a test taking a done callback still ends.
function hollowed(fn) {
  return fn.length > 0 ? (done) => done() : () => {};
}

// timersOf describes the fake timers of the environment: the modern ones keep
// their pending callbacks in the clock, the legacy ones in maps of their own;
// timer functions replaced by anything else are faked and not enumerable.
function timersOf(environment, realTimeout) {
  const modern = environment.fakeTimersModern;
  const clock = modern && modern._fakingTime ? modern._clock : null;
  if (clock) {
    return {
      faked: true,
      pending: () => [...(clock.timers ? clock.timers.values() : []), ...(clock.jobs || [])].map((holder) => ({ holder, key: SINON_TIMER_KEY })),
    };
  }
  const legacy = environment.fakeTimers;
  if (legacy && legacy._fakingTime) {
    return {
      faked: true,
      pending: () => [...legacy._timers.values(), ...legacy._immediates, ...legacy._ticks].map((holder) => ({ holder, key: LEGACY_TIMER_KEY })),
    };
  }
  return { faked: environment.global.setTimeout !== realTimeout };
}

const scannedClasses = new Map();

function scannedOf(Inner) {
  if (scannedClasses.has(Inner)) return scannedClasses.get(Inner);
  class ScannedEnvironment extends Inner {
    constructor(config, context) {
      super(config, context);
      agent.activate(RUNNER);
      agent.phase(agent.FOREIGN);
      this[STATE] = { file: context.testPath, started: false, records: new Map(), failedBlocks: [], realTimeout: this.global.setTimeout };
      agent.file(context.testPath);
    }

    async setup() {
      await super.setup();
      const context = this.getVmContext();
      if (context) agent.checkRealm((code, filename) => vm.runInContext(code, context, { filename }));
    }

    idOf(test) {
      return agent.relative(this[STATE].file) + ID_SEPARATOR + titlesOf(test).join(TITLE_SEPARATOR);
    }

    recordOf(test) {
      const records = this[STATE].records;
      if (!records.has(test)) records.set(test, { id: this.idOf(test), fnStarted: false, fnFailed: false, hook: false, retried: false, skipped: false });
      return records.get(test);
    }

    select(state) {
      for (const test of testsOf(state.rootDescribeBlock)) {
        const id = this.idOf(test);
        if (!agent.selected(id)) {
          test.mode = 'skip';
          continue;
        }
        if (agent.hollow()) test.fn = hollowed(test.fn);
      }
    }

    outcomeOf(test, record) {
      if (record.skipped) return OUTCOME_SKIPPED;
      const blockFailed = this[STATE].failedBlocks.some((block) => isUnder(test, block));
      const retried = record.retried || test.invocations > 1;
      if (test.errors.length === 0 && !blockFailed && !record.hook) {
        if (!record.fnStarted) return OUTCOME_SKIPPED;
        return retried ? OUTCOME_ERRORED : OUTCOME_PASSED;
      }
      if (record.hook || blockFailed || retried || !record.fnStarted || !record.fnFailed) return OUTCOME_ERRORED;
      return OUTCOME_FAILED;
    }

    report(test) {
      const record = this.recordOf(test);
      agent.result(test, { id: record.id, file: agent.relative(this[STATE].file), outcome: this.outcomeOf(test, record) });
    }

    async handleTestEvent(event, state) {
      if (super.handleTestEvent) await super.handleTestEvent(event, state);
      const scan = this[STATE];
      switch (event.name) {
        case 'run_start':
          scan.started = true;
          agent.phase(agent.NEUTRAL);
          this.select(state);
          break;
        case 'hook_start':
          if (event.hook.type !== HOOK_BEFORE_EACH) agent.phase(agent.FOREIGN);
          break;
        case 'hook_success':
          if (event.hook.type !== HOOK_BEFORE_EACH) agent.phase(agent.NEUTRAL);
          break;
        case 'hook_failure':
          if (event.hook.type !== HOOK_BEFORE_EACH) agent.phase(agent.NEUTRAL);
          if (EACH_HOOKS.has(event.hook.type) && event.test) {
            this.recordOf(event.test).hook = true;
          } else if (event.describeBlock) {
            scan.failedBlocks.push(event.describeBlock);
            for (const [test] of scan.records) {
              if (isUnder(test, event.describeBlock)) this.report(test);
            }
          }
          break;
        case 'test_start':
          agent.phase(agent.NEUTRAL);
          this.recordOf(event.test);
          break;
        case 'test_started': {
          const id = this.recordOf(event.test).id;
          agent.open(id, timersOf(this, scan.realTimeout));
          if (event.test.concurrent) agent.concurrent(id);
          break;
        }
        case 'test_fn_start':
          this.recordOf(event.test).fnStarted = true;
          await agent.fnStart(this.recordOf(event.test).id);
          break;
        case 'test_fn_failure':
          this.recordOf(event.test).fnFailed = true;
          await agent.fnEnd(this.recordOf(event.test).id);
          break;
        case 'test_fn_success':
          await agent.fnEnd(this.recordOf(event.test).id);
          break;
        case 'test_retry':
          this.recordOf(event.test).retried = true;
          break;
        case 'test_skip':
        case 'test_todo':
          this.recordOf(event.test).skipped = true;
          this.report(event.test);
          break;
        case 'test_done':
          await agent.close(this.recordOf(event.test).id);
          this.report(event.test);
          break;
        case 'run_finish':
          agent.phase(agent.FOREIGN);
          break;
        default:
          break;
      }
    }

    async teardown() {
      const scan = this[STATE];
      if (!scan.started) agent.fileFailed(scan.file);
      await agent.flush();
      await super.teardown();
    }
  }
  scannedClasses.set(Inner, ScannedEnvironment);
  return ScannedEnvironment;
}

// ScanEnvironment builds the scanned environment of the project's own one.
class ScanEnvironment {
  constructor(config, context) {
    const Scanned = scannedOf(innerOf(config.projectConfig));
    return new Scanned(config, context);
  }
}

module.exports = ScanEnvironment;
module.exports.TestEnvironment = ScanEnvironment;
