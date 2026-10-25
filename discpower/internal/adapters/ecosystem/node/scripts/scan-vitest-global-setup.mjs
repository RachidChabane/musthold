// The global setup of a scan's Vitest runs: it runs the project's own global
// setup modules, which SPEC_PROOF_SCAN_GLOBAL_SETUP lists as a JSON array, and
// writes <SPEC_PROOF_SCAN_OUT>/global-setup.failed when one of them throws:
// the positive evidence that no test of the run began.
import fs from 'node:fs';
import path from 'node:path';

const SETUP_VARIABLE = 'SPEC_PROOF_SCAN_GLOBAL_SETUP';
const OUT_VARIABLE = 'SPEC_PROOF_SCAN_OUT';
const FAILURE_FILE = 'global-setup.failed';

export default async function scanGlobalSetup(project) {
  const files = JSON.parse(process.env[SETUP_VARIABLE] || '[]');
  const teardowns = [];
  try {
    for (const file of files) {
      const exported = await import(file);
      const setup = exported.default || exported.setup;
      if (typeof setup === 'function') {
        const teardown = await setup(project);
        if (typeof teardown === 'function') teardowns.push(teardown);
      }
      if (typeof exported.teardown === 'function') teardowns.push(exported.teardown);
    }
  } catch (error) {
    const out = process.env[OUT_VARIABLE];
    if (out) fs.writeFileSync(path.join(out, FAILURE_FILE), String((error && error.stack) || error));
    throw error;
  }
  return async () => {
    for (const teardown of teardowns.reverse()) await teardown();
  };
}
