// The in-process recorder of a scan's runs of Jest and Vitest. The shipped Jest
// environment and Vitest runner load it before any module of the project and
// drive it: they set the phase the process is in, open and close the window of
// the test a recording selects alone, and record each test's outcome.
//
// It appends one JSON object per line to <SPEC_PROOF_SCAN_OUT>/<pid>.jsonl:
//   {"k":"process",...}       the runner and the capabilities the process showed;
//   {"k":"capabilities",...}  the capabilities, once the test realm is checked;
//   {"k":"script",...}        a source file of the project: its generated code,
//                             the source map that code carries and the length
//                             of the wrapper before it;
//   {"k":"take",...}          the counts V8 recorded, by script and function, and
//                             what the take marks;
//   {"k":"test",...}          the outcome of a test;
//   {"k":"file",...}          a test file that stopped before any test began.
//
// Coverage is V8 precise coverage (call counts, block detail) through an
// inspector session of the process itself. The process line says whether a
// take holds what ran since the take before it, or since the process began.
'use strict';

const asyncHooks = require('node:async_hooks');
const fs = require('node:fs');
const path = require('node:path');
const { fileURLToPath } = require('node:url');
const v8 = require('node:v8');
const vm = require('node:vm');

// scheduleImmediate is Node's own setImmediate, kept before a test can fake it.
const scheduleImmediate = setImmediate;

const AGENT_KEY = Symbol.for('spec-proof.scan.agent');
const TAG = Symbol.for('spec-proof.scan.origin');

const OUT_VARIABLE = 'SPEC_PROOF_SCAN_OUT';
const ROOT_VARIABLE = 'SPEC_PROOF_SCAN_ROOT';
const SELECTION_VARIABLE = 'SPEC_PROOF_SCAN_SELECTION';
const COVERAGE_VARIABLE = 'SPEC_PROOF_SCAN_COVERAGE';
const HOLLOW_VARIABLE = 'SPEC_PROOF_SCAN_HOLLOW';
const WITHOUT_VARIABLE = 'SPEC_PROOF_SCAN_WITHOUT';
// V8_COVERAGE_VARIABLE names the directory Node writes the coverage of a
// process into, which a process without an inspector session records by.
const V8_COVERAGE_VARIABLE = 'NODE_V8_COVERAGE';
const ENABLED = '1';
const LIST_SEPARATOR = ',';
const RECORD_SUFFIX = '.jsonl';
const DEPENDENCY_SEGMENT = path.sep + 'node_modules' + path.sep;
const FILE_URL_PREFIX = 'file://';
const DATA_URL_PREFIX = 'data:';
const BASE64_MARKER = ';base64,';
const PROMISE_TYPE = 'PROMISE';

// The capabilities a process shows: each names a precondition of the ownership
// rule, which the reader of the record replaces by its fallback when it is false.
const CAP_INSPECTOR = 'inspector';
const CAP_SYNCHRONOUS_TAKE = 'synchronousTake';
const CAP_ASYNC_HOOKS = 'asyncHooks';
const CAP_EVALUATION = 'evaluation';
const CAP_FAKE_TIMERS = 'fakeTimers';
const CAP_MAPS = 'maps';
const CAP_BOUNDS = 'bounds';
const CAP_RETENTION = 'retention';
const CAPABILITIES = [CAP_INSPECTOR, CAP_SYNCHRONOUS_TAKE, CAP_ASYNC_HOOKS, CAP_EVALUATION, CAP_FAKE_TIMERS, CAP_MAPS, CAP_BOUNDS, CAP_RETENTION];

// Phases a process is in outside a window, and the origins of its callbacks.
const FOREIGN = 'foreign';
const NEUTRAL = 'neutral';
const OWN = 'own';

// What a take marks.
const AT_START = 'start';
const AT_FILE = 'file';
const AT_OPEN = 'open';
const AT_CLOSE = 'close';
const AT_SLICE_START = 'sliceStart';
const AT_SLICE_END = 'sliceEnd';
const AT_END = 'end';

// Wrapper prefixes a runner compiles a module behind: Vitest's module runner
// wraps the transformed code in an async arrow whose body opens with `{{`.
const WRAPPER_PREFIXES = [/^'use strict';async \([^)]*\)=>\{\{/];

const RETENTION_PROBE_FILE = 'spec-proof-scan-retention.js';
const EVALUATION_PROBE_FILE = 'spec-proof-scan-evaluation.js';
const PROMISE_PROBE_FILE = 'spec-proof-scan-promise.js';
const RETAINED_FUNCTION = 'specProofRetained';
const RETENTION_PROBE = '(function specProofRetention(){ return function ' + RETAINED_FUNCTION + '(){ return 1; }; })';
const EVALUATION_PROBE = '(function specProofEvaluation(){ return 1; })()';
const PROMISE_PROBE = 'Promise.resolve(1)';
const GC_FLAG = '--expose-gc';
const GC_FUNCTION = 'gc';
const GC_COLLECTIONS = 2;
// PROBE_DIRECTORY holds the made-up files of the self-checks, outside every
// project.
const PROBE_DIRECTORY = path.join(path.parse(process.cwd()).root, 'spec-proof-scan-probe');

function readSelection(file) {
  if (!file) return null;
  try {
    return new Set(JSON.parse(fs.readFileSync(file, 'utf8')));
  } catch {
    return new Set();
  }
}

function decodeDataURL(url) {
  const marker = url.indexOf(BASE64_MARKER);
  if (marker >= 0) return Buffer.from(url.slice(marker + BASE64_MARKER.length), 'base64').toString('utf8');
  const comma = url.indexOf(',');
  return comma >= 0 ? decodeURIComponent(url.slice(comma + 1)) : '';
}

function fileOfURL(url) {
  if (url.startsWith(FILE_URL_PREFIX)) {
    try {
      return fileURLToPath(url);
    } catch {
      return '';
    }
  }
  return path.isAbsolute(url) ? url : '';
}

function prefixOf(source) {
  for (const pattern of WRAPPER_PREFIXES) {
    const match = pattern.exec(source);
    if (match) return match[0].length;
  }
  return 0;
}

function rangesOf(script) {
  return script.functions.map((fn) => fn.ranges.map((range) => [range.startOffset, range.endOffset, range.count]));
}

function createAgent() {
  const env = process.env;
  const out = env[OUT_VARIABLE] || '';
  const root = path.resolve(env[ROOT_VARIABLE] || process.cwd());
  const recording = env[COVERAGE_VARIABLE] === ENABLED;
  const hollow = env[HOLLOW_VARIABLE] === ENABLED;
  const without = new Set((env[WITHOUT_VARIABLE] || '').split(LIST_SEPARATOR).filter(Boolean));
  const selection = readSelection(env[SELECTION_VARIABLE]);
  const recordFile = out ? path.join(out, process.pid + RECORD_SUFFIX) : '';

  const capabilities = Object.fromEntries(CAPABILITIES.map((name) => [name, false]));
  const parsed = new Map();
  const sent = new Set();
  const results = new Map();
  let session = null;
  let active = false;
  let phase = FOREIGN;
  let window = null;
  let sliceDepth = 0;
  let sliceTimers = null;
  let inTake = false;
  // cumulative is whether a take reports the counts since the process began
  // rather than since the previous take.
  let cumulative = false;

  function append(entry) {
    if (!recordFile) return;
    try {
      fs.appendFileSync(recordFile, JSON.stringify(entry) + '\n');
    } catch {
      // A process whose record cannot be written leaves none, which its reader
      // counts as an error of the run.
    }
  }

  function relative(file) {
    return path.relative(root, file).split(path.sep).join('/');
  }

  // request posts a message to the inspector. A process whose takes are
  // synchronous gets the answer before request returns; one that is not gets
  // it once the event loop turns.
  function request(method, params, done) {
    if (capabilities[CAP_SYNCHRONOUS_TAKE]) {
      session.post(method, params, done);
      return;
    }
    session.post(method, params, (error, result) => {
      scheduleImmediate(() => {
        inTake = true;
        try {
          done(error, result);
        } finally {
          inTake = false;
        }
      });
    });
  }

  function syncPost(method, params) {
    let done = false;
    let failure = null;
    let response;
    session.post(method, params || {}, (error, result) => {
      done = true;
      failure = error;
      response = result;
    });
    if (!done) throw new Error('the inspector answered ' + method + ' once the event loop turned');
    if (failure) throw failure;
    return response;
  }

  function isProjectFile(file) {
    return file !== '' && file.startsWith(root + path.sep) && !file.includes(DEPENDENCY_SEGMENT);
  }

  function mapOf(scriptId, file) {
    const known = parsed.get(scriptId);
    if (!capabilities[CAP_MAPS] || !known || !known.sourceMapURL) return '';
    if (known.sourceMapURL.startsWith(DATA_URL_PREFIX)) return decodeDataURL(known.sourceMapURL);
    const mapFile = fileOfURL(known.sourceMapURL) || path.resolve(path.dirname(file), known.sourceMapURL);
    try {
      return fs.readFileSync(mapFile, 'utf8');
    } catch {
      return '';
    }
  }

  // take records what ran since the previous take, for the project's files,
  // and calls done once the record holds it.
  function take(at, extra, done) {
    const finish = done || (() => {});
    if (!capabilities[CAP_INSPECTOR]) {
      finish();
      return;
    }
    const outer = inTake;
    inTake = true;
    try {
      request('Profiler.takePreciseCoverage', {}, (error, taken) => recordTake(at, extra, error, taken, finish));
    } finally {
      inTake = outer;
    }
  }

  function recordTake(at, extra, error, taken, finish) {
    if (error) {
      finish();
      return;
    }
    const counts = {};
    const fresh = [];
    for (const script of taken.result) {
      const file = fileOfURL(script.url);
      if (!isProjectFile(file)) continue;
      if (!sent.has(script.scriptId)) {
        sent.add(script.scriptId);
        fresh.push({ id: script.scriptId, file });
      }
      counts[script.scriptId] = rangesOf(script);
    }
    let waiting = fresh.length;
    const written = () => {
      append(Object.assign({ k: 'take', at, c: counts }, extra || {}));
      finish();
    };
    if (waiting === 0) {
      written();
      return;
    }
    for (const script of fresh) {
      request('Debugger.getScriptSource', { scriptId: script.id }, (sourceError, answer) => {
        const source = sourceError ? '' : answer.scriptSource;
        append({ k: 'script', id: script.id, file: relative(script.file), source, map: mapOf(script.id, script.file), prefix: prefixOf(source) });
        waiting--;
        if (waiting === 0) written();
      });
    }
  }

  // takeAwaitable is a take a caller may await: a promise once the event loop
  // turns for a process whose takes are not synchronous, nothing otherwise.
  function takeAwaitable(at, extra) {
    if (capabilities[CAP_SYNCHRONOUS_TAKE]) {
      take(at, extra);
      return undefined;
    }
    return new Promise((resolve) => take(at, extra, resolve));
  }

  function startInspector() {
    if (without.has(CAP_INSPECTOR)) return false;
    try {
      const inspector = require('node:inspector');
      session = new inspector.Session();
      session.connect();
      session.on('Debugger.scriptParsed', (message) => {
        parsed.set(message.params.scriptId, { sourceMapURL: message.params.sourceMapURL || '' });
      });
      syncPost('Profiler.enable');
      syncPost('Profiler.startPreciseCoverage', { callCount: true, detailed: true });
      syncPost('Debugger.enable');
      syncPost('Debugger.setSkipAllPauses', { skip: true });
      capabilities[CAP_SYNCHRONOUS_TAKE] = !without.has(CAP_SYNCHRONOUS_TAKE);
      capabilities[CAP_MAPS] = !without.has(CAP_MAPS);
      return true;
    } catch {
      return false;
    }
  }

  // checkRetention runs a function of a script nothing keeps, collects the
  // garbage and checks that the next take still reports the function's count;
  // the take after it tells whether a take resets the counts.
  function checkRetention() {
    if (without.has(CAP_RETENTION)) return false;
    try {
      v8.setFlagsFromString(GC_FLAG);
      const collect = vm.runInNewContext(GC_FUNCTION);
      syncPost('Profiler.takePreciseCoverage');
      vm.runInThisContext(RETENTION_PROBE, { filename: path.join(PROBE_DIRECTORY, RETENTION_PROBE_FILE) })()();
      for (let index = 0; index < GC_COLLECTIONS; index++) collect();
      const retained = (taken) => taken.some((script) => script.url.endsWith(RETENTION_PROBE_FILE) && script.functions.some((fn) => fn.functionName === RETAINED_FUNCTION && fn.ranges[0].count === 1));
      const kept = retained(syncPost('Profiler.takePreciseCoverage').result);
      cumulative = retained(syncPost('Profiler.takePreciseCoverage').result);
      return kept;
    } catch {
      return false;
    }
  }

  // checkRealm checks, in the realm the tests run in, that the async hooks see
  // a promise it creates and that a take shows a module it evaluates.
  // evaluate(code, filename) runs code in that realm.
  function checkRealm(evaluate) {
    if (!capabilities[CAP_INSPECTOR]) {
      append({ k: 'capabilities', capabilities });
      return;
    }
    let seen = false;
    const probe = asyncHooks.createHook({
      init(_id, type) {
        if (type === PROMISE_TYPE) seen = true;
      },
    });
    probe.enable();
    try {
      evaluate(PROMISE_PROBE, path.join(PROBE_DIRECTORY, PROMISE_PROBE_FILE));
    } catch {
      seen = false;
    } finally {
      probe.disable();
    }
    capabilities[CAP_ASYNC_HOOKS] = seen && capabilities[CAP_SYNCHRONOUS_TAKE] && !without.has(CAP_ASYNC_HOOKS);
    let evaluated = false;
    inTake = true;
    try {
      syncPost('Profiler.takePreciseCoverage');
      const file = path.join(PROBE_DIRECTORY, EVALUATION_PROBE_FILE);
      evaluate(EVALUATION_PROBE, file);
      const taken = syncPost('Profiler.takePreciseCoverage').result;
      evaluated = taken.some((script) => fileOfURL(script.url) === file && script.functions.some((fn) => fn.ranges[0].startOffset === 0 && fn.ranges[0].count > 0));
    } catch {
      evaluated = false;
    } finally {
      inTake = false;
    }
    capabilities[CAP_EVALUATION] = evaluated && !without.has(CAP_EVALUATION);
    append({ k: 'capabilities', capabilities });
  }

  function originNow() {
    if (window !== null) return sliceDepth > 0 ? FOREIGN : OWN;
    return phase;
  }

  function startHooks() {
    asyncHooks
      .createHook({
        init(_id, _type, _trigger, resource) {
          if (inTake || resource === null || typeof resource !== 'object') return;
          try {
            resource[TAG] = originNow();
          } catch {
            // A frozen resource keeps no origin and counts as the test's own.
          }
        },
        before() {
          if (inTake || window === null) return;
          if (sliceDepth > 0) {
            sliceDepth++;
            return;
          }
          const resource = asyncHooks.executionAsyncResource();
          if (resource && resource[TAG] === FOREIGN) enterSlice();
        },
        after() {
          if (inTake || sliceDepth === 0) return;
          sliceDepth--;
          if (sliceDepth === 0) leaveSlice();
        },
      })
      .enable();
  }

  // enumerate lists the pending fake timers of a realm, or null when they
  // cannot be told apart: listing them throws, an entry holds no callback, or
  // the clock counts another number of them than the list holds.
  function enumerate(timers) {
    try {
      const entries = timers.pending();
      const complete = Array.isArray(entries) && entries.length === timers.count() &&
        entries.every((entry) => entry && entry.holder !== null && typeof entry.holder === 'object' && typeof entry.holder[entry.key] === 'function');
      return complete ? entries : null;
    } catch {
      return null;
    }
  }

  // pendingTimers lists the pending fake timers of the window's realm; when
  // they can no longer be told apart, the window owns nothing.
  function pendingTimers() {
    if (window === null || window.timers === null) return [];
    const entries = enumerate(window.timers);
    if (entries === null) {
      window.fakeUnseen = true;
      return [];
    }
    return entries;
  }

  // enterSlice starts a callback of foreign origin inside the window: a slice
  // whose counts are cut out of the window, or, where the async hooks are
  // blind, a callback the window records.
  function enterSlice() {
    if (!capabilities[CAP_ASYNC_HOOKS]) {
      window.callbacks = true;
      return;
    }
    sliceDepth = 1;
    sliceTimers = new Set(pendingTimers().map((entry) => entry.holder));
    take(AT_SLICE_START, { t: window.id });
  }

  // leaveSlice ends the slice, and treats the fake timers it scheduled as
  // foreign too.
  function leaveSlice() {
    take(AT_SLICE_END, { t: window.id });
    const before = sliceTimers || new Set();
    sliceTimers = null;
    for (const entry of pendingTimers()) {
      if (!before.has(entry.holder)) wrapForeign(entry);
    }
  }

  // wrapForeign makes the callback of a fake timer a slice of its own whenever
  // it runs inside the window.
  function wrapForeign(entry) {
    const callback = entry.holder[entry.key];
    if (typeof callback !== 'function' || callback[TAG] === FOREIGN) return;
    const wrapped = function () {
      if (window === null || sliceDepth > 0) return callback.apply(this, arguments);
      if (!capabilities[CAP_SYNCHRONOUS_TAKE]) {
        window.callbacks = true;
        return callback.apply(this, arguments);
      }
      sliceDepth = 1;
      take(AT_SLICE_START, { t: window.id });
      try {
        return callback.apply(this, arguments);
      } finally {
        sliceDepth = 0;
        take(AT_SLICE_END, { t: window.id });
      }
    };
    wrapped[TAG] = FOREIGN;
    entry.holder[entry.key] = wrapped;
  }

  const agent = {
    FOREIGN,
    NEUTRAL,
    // activate starts the recording of the process; runner names the runner in
    // the record.
    activate(runner) {
      if (active) return agent;
      active = true;
      if (recording) {
        capabilities[CAP_INSPECTOR] = startInspector();
        if (capabilities[CAP_INSPECTOR]) {
          capabilities[CAP_RETENTION] = checkRetention();
          capabilities[CAP_FAKE_TIMERS] = !without.has(CAP_FAKE_TIMERS);
          capabilities[CAP_BOUNDS] = !without.has(CAP_BOUNDS);
          startHooks();
        }
      }
      append({ k: 'process', pid: process.pid, runner, recording, cumulative, capabilities });
      take(AT_START);
      return agent;
    },
    checkRealm,
    relative,
    hollow: () => hollow,
    selecting: () => selection !== null,
    selected: (id) => selection === null || selection.has(id),
    // windows is whether the process opens the window of its test: a
    // recording of one test selected alone.
    windows: () => recording && capabilities[CAP_INSPECTOR] && selection !== null && selection.size === 1,
    // phase sets the origin of the work outside a window: foreign for the
    // project's setup, module evaluation, describe bodies and the hooks no
    // test owns, neutral for the runner's own work between them.
    phase(kind) {
      phase = kind === NEUTRAL ? NEUTRAL : FOREIGN;
    },
    file(file) {
      take(AT_FILE, { f: relative(file) });
    },
    // open opens the window of a test before its first beforeEach hook. timers
    // describes the fake timers of its realm: whether they are faked and, when
    // they can be enumerated, pending() lists each pending callback as the
    // holder object and the key it is kept under, and count() is how many the
    // clock counts. A realm that fakes timers its window cannot cut out, or
    // whose takes are not synchronous, owns nothing.
    open(id, timers) {
      if (!agent.windows() || window !== null) return;
      const faked = Boolean(timers && timers.faked);
      const listable = faked && typeof timers.pending === 'function' && typeof timers.count === 'function' && capabilities[CAP_FAKE_TIMERS] && capabilities[CAP_SYNCHRONOUS_TAKE];
      const enumerable = listable && enumerate(timers) !== null;
      window = { id, timers: enumerable ? timers : null, callbacks: false, fnStarted: false, fnSettled: false, fakeUnseen: faked && !enumerable, opened: false };
      if (!capabilities[CAP_SYNCHRONOUS_TAKE]) return;
      take(AT_OPEN, { t: id });
      window.opened = true;
      for (const entry of pendingTimers()) wrapForeign(entry);
    },
    // fnStart marks the start of the test function. Where takes are not
    // synchronous the window opens here, after the beforeEach hooks.
    fnStart(id) {
      if (window === null || window.id !== id) return undefined;
      window.fnStarted = true;
      if (window.opened) return undefined;
      window.opened = true;
      return takeAwaitable(AT_OPEN, { t: id });
    },
    // fnEnd marks the settling of the test function and closes the window.
    fnEnd(id) {
      if (window === null || window.id !== id) return undefined;
      if (window.fnStarted) window.fnSettled = true;
      return agent.close(id);
    },
    close(id) {
      if (window === null || window.id !== id) return undefined;
      const closing = window;
      if (sliceDepth > 0) {
        sliceDepth = 0;
        take(AT_SLICE_END, { t: id });
      }
      window = null;
      const bounded = closing.fnStarted && closing.fnSettled && !closing.concurrentUnbounded;
      return takeAwaitable(AT_CLOSE, { t: id, bounded, callbacks: closing.callbacks, fakeUnseen: closing.fakeUnseen });
    },
    // concurrent marks the open window as a concurrent test's, which a window
    // bounds only where the process shows bounded windows.
    concurrent(id) {
      if (window !== null && window.id === id && !capabilities[CAP_BOUNDS]) window.concurrentUnbounded = true;
    },
    // result records the outcome of the test the key stands for; a later one
    // replaces it.
    result(key, entry) {
      results.set(key, entry);
    },
    fileFailed(file) {
      append({ k: 'file', file: relative(file) });
    },
    // flush writes the outcomes recorded so far and takes what ran since the
    // last take.
    flush() {
      for (const entry of results.values()) append(Object.assign({ k: 'test' }, entry));
      results.clear();
      if (recording && !capabilities[CAP_INSPECTOR] && env[V8_COVERAGE_VARIABLE]) {
        try {
          v8.takeCoverage();
        } catch {
          // The run's coverage directory then holds what the process wrote at exit.
        }
      }
      return takeAwaitable(AT_END);
    },
  };
  return agent;
}

module.exports = globalThis[AGENT_KEY] || (globalThis[AGENT_KEY] = createAgent());
