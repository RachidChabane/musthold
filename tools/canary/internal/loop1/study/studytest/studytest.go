// Package studytest holds the study configuration that tests use as their
// fixture, so that no test depends on the committed state of the study.
package studytest

import (
	_ "embed"
)

//go:embed boxoffice-loop-1.json
var boxofficeConfig []byte

// BoxofficeConfig returns a valid loop-1.json whose only task is boxoffice.
// Every call returns its own copy.
func BoxofficeConfig() []byte {
	return append([]byte(nil), boxofficeConfig...)
}
