// The global setup of a scan's Jest runs: it runs the project's own global
// setup module, which MUSTHOLD_SCAN_GLOBAL_SETUP names, through Jest's own
// transform, and writes <MUSTHOLD_SCAN_OUT>/global-setup.failed when it
// throws: the positive evidence that no test of the run began.
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const { pathToFileURL } = require('node:url');

const SETUP_VARIABLE = 'MUSTHOLD_SCAN_GLOBAL_SETUP';
const OUT_VARIABLE = 'MUSTHOLD_SCAN_OUT';
const ROOT_VARIABLE = 'MUSTHOLD_SCAN_ROOT';
const FAILURE_FILE = 'global-setup.failed';
const TRANSFORM_PACKAGE = '@jest/transform';

async function runPlain(file, args) {
  let exported;
  try {
    exported = require(file);
  } catch (error) {
    if (!error || error.code !== 'ERR_REQUIRE_ESM') throw error;
    exported = await import(pathToFileURL(file).href);
  }
  const setup = exported.default || exported;
  if (typeof setup === 'function') await setup(...args);
}

async function runTransformed(file, args) {
  const projectConfig = args[1];
  let transform;
  try {
    const root = process.env[ROOT_VARIABLE] || process.cwd();
    transform = createRequire(path.join(root, 'package.json'))(TRANSFORM_PACKAGE);
  } catch {
    return runPlain(file, args);
  }
  const transformer = await transform.createScriptTransformer(projectConfig);
  await transformer.requireAndTranspileModule(file, async (exported) => {
    const setup = exported && exported.default ? exported.default : exported;
    if (typeof setup === 'function') await setup(...args);
  });
}

module.exports = async function scanGlobalSetup(...args) {
  const file = process.env[SETUP_VARIABLE];
  if (!file) return;
  try {
    await runTransformed(file, args);
  } catch (error) {
    const out = process.env[OUT_VARIABLE];
    if (out) fs.writeFileSync(path.join(out, FAILURE_FILE), String((error && error.stack) || error));
    throw error;
  }
};
