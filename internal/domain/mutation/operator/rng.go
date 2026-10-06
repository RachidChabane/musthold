package operator

import (
	"crypto/sha256"
	"encoding/binary"
)

const (
	splitMixIncrement = 0x9e3779b97f4a7c15
	splitMixFirst     = 0xbf58476d1ce4e5b9
	splitMixSecond    = 0x94d049bb133111eb
	splitMixShiftA    = 30
	splitMixShiftB    = 27
	splitMixShiftC    = 31
	seedBytes         = 8
)

// stream is a SplitMix64 generator. Its algorithm is fixed here so that the
// same seed picks the same mutants on every Go version.
type stream struct{ state uint64 }

// streamFor derives an independent stream from a seed and a label.
func streamFor(seed Seed, label string) *stream {
	raw := make([]byte, seedBytes)
	binary.BigEndian.PutUint64(raw, uint64(seed))
	sum := sha256.Sum256(append(raw, label...))
	return &stream{state: binary.BigEndian.Uint64(sum[:seedBytes])}
}

func (s *stream) next() uint64 {
	s.state += splitMixIncrement
	z := s.state
	z = (z ^ (z >> splitMixShiftA)) * splitMixFirst
	z = (z ^ (z >> splitMixShiftB)) * splitMixSecond
	return z ^ (z >> splitMixShiftC)
}

// intn returns a value in [0, n).
func (s *stream) intn(n int) int { return int(s.next() % uint64(n)) }

// shuffle permutes items in place (Fisher-Yates).
func shuffle[T any](s *stream, items []T) {
	for i := len(items) - 1; i > 0; i-- {
		j := s.intn(i + 1)
		items[i], items[j] = items[j], items[i]
	}
}
