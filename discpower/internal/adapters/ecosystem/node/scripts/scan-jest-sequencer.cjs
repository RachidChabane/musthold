// The test sequencer of a scan's Jest runs: test files run in path order, so
// that a recording of several files segments them the same way every time.
'use strict';

class PathSequencer {
  sort(tests) {
    return [...tests].sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
  }

  shard(tests) {
    return tests;
  }

  allFailedTests(tests) {
    return tests;
  }

  cacheResults() {}
}

module.exports = PathSequencer;
