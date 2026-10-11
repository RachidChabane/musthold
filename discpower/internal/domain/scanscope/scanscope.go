// Package scanscope holds the pure rules of what `spec-proof scan` samples and
// draws (ADR 0014): the seed a scan draws with, the sample of tests stratified
// by file, the lines a mutant mutates, the lines a draw may mutate, and what a
// scan never mutates. It reads no file and runs nothing.
package scanscope

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// Defaults of a scan without configuration.
const (
	// DefaultSampleSize is how many green tests a repository scan samples.
	DefaultSampleSize = 40
	// DefaultMutants is how many operator mutants a scan draws, sentinels aside.
	DefaultMutants = 150
	// DefaultSeed is the seed a scan draws from when none is given.
	DefaultSeed uint64 = 7
)

// seedDomain separates every draw of a scan from the draws of a proof: the
// operator seed of a scan is derived from it and the scan seed, so a scan seed
// never yields the seed of a local proof or of a CI run.
const seedDomain = "spec-proof scan\x00"

// seedBytes is the size of a seed in bytes.
const seedBytes = 8

// DrawSeed is the operator seed every draw of a scan with the given seed uses.
func DrawSeed(seed uint64) operator.Seed {
	sum := sha256.Sum256([]byte(seedDomain + strconv.FormatUint(seed, decimalBase)))
	return operator.Seed(binary.BigEndian.Uint64(sum[:seedBytes]))
}

// decimalBase writes a seed in decimal.
const decimalBase = 10
