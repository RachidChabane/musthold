// Package audit selects the verdicts that the shadow audit also sends to the
// judge. The selection is a function of a digest of the work, so it never
// depends on a clock, a random source or the order the work was done in.
package audit

import (
	"crypto/sha256"
	"encoding/binary"
)

// resolution is the number of equally likely buckets a digest falls into.
const resolution = 1 << 32

// Selected reports whether the work identified by key is in the sample of the
// given rate. A rate of zero selects nothing and a rate of one selects
// everything; the sample of a smaller rate is contained in that of a larger one.
func Selected(key string, rate float64) bool {
	switch {
	case rate <= 0:
		return false
	case rate >= 1:
		return true
	}
	digest := sha256.Sum256([]byte(key))
	bucket := binary.BigEndian.Uint32(digest[:4])
	return float64(bucket)/resolution < rate
}
