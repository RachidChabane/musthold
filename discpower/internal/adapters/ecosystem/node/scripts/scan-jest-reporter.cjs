// The one reporter of a scan's Jest runs. It appends to
// <SPEC_PROOF_SCAN_OUT>/reporter.jsonl, per test file, whether the file failed
// to run and the status Jest gives each of its tests: the outcomes of a file
// whose tests ran outside the scan's environment, which a docblock can choose.
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const OUT_VARIABLE = 'SPEC_PROOF_SCAN_OUT';
const ROOT_VARIABLE = 'SPEC_PROOF_SCAN_ROOT';
const REPORT_FILE = 'reporter.jsonl';
const ID_SEPARATOR = '::';
const TITLE_SEPARATOR = ' > ';

class ScanReporter {
  onTestResult(test, result) {
    const out = process.env[OUT_VARIABLE];
    if (!out) return;
    const root = path.resolve(process.env[ROOT_VARIABLE] || process.cwd());
    const file = path.relative(root, result.testFilePath).split(path.sep).join('/');
    const tests = result.testResults.map((assertion) => ({
      id: file + ID_SEPARATOR + [...assertion.ancestorTitles, assertion.title].join(TITLE_SEPARATOR),
      status: assertion.status,
      invocations: assertion.invocations || 1,
    }));
    const entry = { k: 'result', file, failedToRun: Boolean(result.testExecError), tests };
    fs.appendFileSync(path.join(out, REPORT_FILE), JSON.stringify(entry) + '\n');
  }

  getLastError() {
    return undefined;
  }
}

module.exports = ScanReporter;
