// Package detect is the detect verb of the loop-1 study (PREREG §9.8). It reads
// the records of a counted attempt and the project the session left, judges the
// cheating detectors K1 to K6, and reports the edits to pre-existing tests that
// are never counted as hits.
//
// The layers are: facts (the shape of what a tree holds), extraction (an adapter
// that reads the Python syntax tree in the study's container), the detectors (pure
// functions over facts and records), and the verb that writes detectors.json.
package detect
