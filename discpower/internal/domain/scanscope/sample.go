package scanscope

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// Case is a test a scan may sample, with the file that holds it.
type Case struct {
	ID   ecosystem.TestID
	File string
}

// Labels of the draws a sample makes.
const (
	labelFiles = "sample files"
	labelTests = "sample tests\x00"
)

// Sample draws up to size tests, stratified by file: the files are shuffled by
// the seed, so are the tests of each file, and one test of each file is taken
// in turn until the sample is full. The result is sorted by test id and does
// not depend on the order of the cases.
func Sample(cases []Case, size int, seed uint64) []ecosystem.TestID {
	if size <= 0 {
		return nil
	}
	byFile := map[string][]ecosystem.TestID{}
	for _, scanCase := range cases {
		if !slices.Contains(byFile[scanCase.File], scanCase.ID) {
			byFile[scanCase.File] = append(byFile[scanCase.File], scanCase.ID)
		}
	}
	files := slices.Sorted(maps.Keys(byFile))
	drawSeed := DrawSeed(seed)
	operator.Shuffle(drawSeed, labelFiles, files)
	for _, file := range files {
		slices.SortFunc(byFile[file], compareIDs)
		operator.Shuffle(drawSeed, labelTests+file, byFile[file])
	}
	var picked []ecosystem.TestID
	for round := 0; len(picked) < size; round++ {
		progressed := false
		for _, file := range files {
			if round < len(byFile[file]) && len(picked) < size {
				picked = append(picked, byFile[file][round])
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	slices.SortFunc(picked, compareIDs)
	return picked
}

func compareIDs(a, b ecosystem.TestID) int { return cmp.Compare(a, b) }

// IDs lists the ids of cases, sorted and without repeats.
func IDs(cases []Case) []ecosystem.TestID {
	ids := make([]ecosystem.TestID, 0, len(cases))
	for _, scanCase := range cases {
		ids = append(ids, scanCase.ID)
	}
	slices.SortFunc(ids, func(a, b ecosystem.TestID) int { return strings.Compare(string(a), string(b)) })
	return slices.Compact(ids)
}
