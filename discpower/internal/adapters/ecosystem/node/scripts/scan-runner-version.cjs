// The versions of Jest and Vitest a scan runs: the major versions whose
// internals the shipped scripts extend. It reads the version the project
// installs, resolved from the project's root, and refuses any other, naming
// the supported majors and the installed version.
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const { pathToFileURL } = require('node:url');

const PACKAGE_MANIFEST = 'package.json';
const MAIN_EXPORT = '.';
const SUBPATH_SEPARATOR = '/';
const VERSION_SEPARATOR = '.';

// REFUSAL_PREFIX opens every refusal, so that the scan tells it from any
// other failure of the runner.
const REFUSAL_PREFIX = 'musthold scan does not support ';

const JEST = Object.freeze({ title: 'Jest', packageName: 'jest', firstMajor: 29, lastMajor: 30 });
const VITEST = Object.freeze({ title: 'Vitest', packageName: 'vitest', firstMajor: 3, lastMajor: 5 });

function requireFrom(root) {
  return createRequire(path.join(root, PACKAGE_MANIFEST));
}

function readManifest(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

// manifestFileOf is the package.json of the package the project's root
// resolves, or null when the root resolves no such package. A package whose
// exports leave out its package.json is found from its main entry.
function manifestFileOf(root, packageName) {
  const resolve = requireFrom(root).resolve;
  try {
    return resolve(packageName + SUBPATH_SEPARATOR + PACKAGE_MANIFEST);
  } catch {
    // The package does not export its manifest, or is not installed.
  }
  let entry;
  try {
    entry = resolve(packageName);
  } catch {
    return null;
  }
  for (let dir = path.dirname(entry); dir !== path.dirname(dir); dir = path.dirname(dir)) {
    const candidate = path.join(dir, PACKAGE_MANIFEST);
    if (fs.existsSync(candidate) && readManifest(candidate).name === packageName) return candidate;
  }
  return null;
}

// refusal refuses the installed version of the runner for the reason given.
function refusal(runner, version, reason) {
  return `${REFUSAL_PREFIX}${runner.packageName} ${version}: ${reason}`;
}

function outsideRange(runner) {
  return `it supports ${runner.title} ${runner.firstMajor} to ${runner.lastMajor}`;
}

// requireSupported returns the version of the runner the project installs,
// and throws the refusal when its major version is outside the supported
// ones. A project whose root resolves no such package passes: the runner it
// starts is not the scan's to judge.
function requireSupported(root, runner) {
  const manifestFile = manifestFileOf(root, runner.packageName);
  if (!manifestFile) return null;
  const version = String(readManifest(manifestFile).version || '');
  const major = Number.parseInt(version.split(VERSION_SEPARATOR)[0], 10);
  if (!(major >= runner.firstMajor && major <= runner.lastMajor)) throw new Error(refusal(runner, version, outsideRange(runner)));
  return version;
}

function pickImport(entry) {
  if (typeof entry === 'string') return entry;
  return entry ? pickImport(entry.import) || pickImport(entry.default) : null;
}

// exportURL is the URL of the file an ES import of the package's subpath
// loads, `.` for the package itself, or null when the installed package does
// not export that subpath.
function exportURL(root, runner, subpath) {
  const manifestFile = manifestFileOf(root, runner.packageName);
  if (!manifestFile) return null;
  const manifest = readManifest(manifestFile);
  let target = manifest.exports ? pickImport(manifest.exports[subpath]) : null;
  if (!target && subpath === MAIN_EXPORT) target = manifest.module || manifest.main;
  return target ? pathToFileURL(path.join(path.dirname(manifestFile), target)).href : null;
}

module.exports = { JEST, VITEST, REFUSAL_PREFIX, MAIN_EXPORT, requireSupported, refusal, exportURL };
