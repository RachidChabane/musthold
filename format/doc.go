// Package format is the public contract of the proof format: in-toto Statement v1
// envelopes with three project-owned predicates (alignment, discriminating power,
// completeness), the claim model that separates executed from judged evidence,
// RFC 8785 (JCS) serialization, the file-set digest, the cache key, the subject
// rules, the sort orders and the embedded JSON Schemas.
//
// The package is pure: it reads no file, runs no process and opens no network
// connection, so third-party Go modules can import it freely.
//
// # Sort orders
//
// Every array of a predicate is strictly ascending under byte-wise (not
// locale-aware) string comparison, so "PARK-10" sorts before "PARK-2":
//
//   - inputs: by role, then name;
//   - tests, annotations.globs, requirementIds, deciders: by value;
//   - mutants: by digest;
//   - mutant results: by test name;
//   - examples: by index, ascending;
//   - questions: by dimension, then question text;
//   - outOfScopeDeclared: by dimension, then reason.
//
// A statement carries one criterion, so the requirement ID order of ADR 0001 is
// the order of statements, never of an array inside one.
package format
