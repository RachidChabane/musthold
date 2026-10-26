package scanscope

import (
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// SentinelCap is how many sentinels a scan runs. A test that fails a change
// that keeps behaviour depends on the text of the code or on what changes from
// run to run; when such a dependency makes one sentinel in four fail, a scan of
// twelve sentinels misses it with a chance of (3/4)^12, under 4 %.
const SentinelCap = 12

// Labels of the draws of the sentinels.
const (
	labelSentinelFiles = "sentinel files"
	labelSentinelsOf   = "sentinels of\x00"
)

// KeepSentinels keeps at most limit sentinels, stratified by file: the files
// and the sentinels of each file shuffled by the seed, one sentinel of each
// file in turn, so that no file gives a second sentinel before every file gave
// one. The result is sorted by id and does not depend on the order of the
// sentinels.
func KeepSentinels(sentinels []operator.Mutant, limit int, seed uint64) []operator.Mutant {
	byFile := map[string][]operator.Mutant{}
	for _, sentinel := range sentinels {
		byFile[sentinel.Site.File] = append(byFile[sentinel.Site.File], sentinel)
	}
	files := slices.Sorted(maps.Keys(byFile))
	drawSeed := DrawSeed(seed)
	operator.Shuffle(drawSeed, labelSentinelFiles, files)
	for _, file := range files {
		slices.SortFunc(byFile[file], compareMutantIDs)
		operator.Shuffle(drawSeed, labelSentinelsOf+file, byFile[file])
	}
	var kept []operator.Mutant
	for round := 0; len(kept) < limit; round++ {
		progressed := false
		for _, file := range files {
			if round < len(byFile[file]) && len(kept) < limit {
				kept = append(kept, byFile[file][round])
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	slices.SortFunc(kept, compareMutantIDs)
	return kept
}

func compareMutantIDs(a, b operator.Mutant) int { return strings.Compare(a.ID, b.ID) }
