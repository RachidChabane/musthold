package scanscope_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
)

const (
	verifiesSentinels = " [verifies SP-2126~1]"

	maxSentinelFiles    = 20
	maxSentinelsPerFile = 3
	maxSentinelLimit    = 30
)

func drawSentinels(t *rapid.T) []operator.Mutant {
	var sentinels []operator.Mutant
	for file := range rapid.IntRange(0, maxSentinelFiles).Draw(t, "files") {
		for index := range rapid.IntRange(1, maxSentinelsPerFile).Draw(t, fmt.Sprintf("sentinels of %d", file)) {
			sentinels = append(sentinels, operator.Mutant{
				ID:   fmt.Sprintf("s-%02d-%d", file, index),
				Site: operator.Site{File: fmt.Sprintf("pkg/file_%02d.py", file)},
			})
		}
	}
	return sentinels
}

func perFile(sentinels []operator.Mutant) map[string]int {
	counts := map[string]int{}
	for _, sentinel := range sentinels {
		counts[sentinel.Site.File]++
	}
	return counts
}

func TestKeepSentinels(t *testing.T) {
	t.Run("a scan keeps twelve sentinels"+verifiesSentinels, func(t *testing.T) {
		if scanscope.SentinelCap != 12 {
			t.Fatalf("cap %d", scanscope.SentinelCap)
		}
	})

	t.Run("the kept sentinels are at most the limit, every sentinel when there are fewer, sorted by id"+verifiesSentinels, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			sentinels := drawSentinels(t)
			limit := rapid.IntRange(0, maxSentinelLimit).Draw(t, "limit")
			kept := scanscope.KeepSentinels(sentinels, limit, rapid.Uint64().Draw(t, "seed"))
			if len(kept) != min(limit, len(sentinels)) || !slices.IsSortedFunc(kept, func(a, b operator.Mutant) int { return strings.Compare(a.ID, b.ID) }) {
				t.Fatalf("kept %d of %d for a limit of %d", len(kept), len(sentinels), limit)
			}
		})
	})

	t.Run("no file gives a second sentinel before every file gave one"+verifiesSentinels, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			sentinels := drawSentinels(t)
			limit := rapid.IntRange(0, maxSentinelLimit).Draw(t, "limit")
			kept := perFile(scanscope.KeepSentinels(sentinels, limit, rapid.Uint64().Draw(t, "seed")))
			available := perFile(sentinels)
			for short, count := range available {
				for file := range available {
					if kept[short] < count && kept[file] > kept[short]+1 {
						t.Fatalf("%s kept %d of %d while %s kept %d", short, kept[short], count, file, kept[file])
					}
				}
			}
		})
	})

	t.Run("the kept sentinels are the same for the same seed whatever their order"+verifiesSentinels, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			sentinels := drawSentinels(t)
			limit := rapid.IntRange(0, maxSentinelLimit).Draw(t, "limit")
			seed := rapid.Uint64().Draw(t, "seed")
			reversed := slices.Clone(sentinels)
			slices.Reverse(reversed)
			first, second := scanscope.KeepSentinels(sentinels, limit, seed), scanscope.KeepSentinels(reversed, limit, seed)
			if !slices.EqualFunc(first, second, func(a, b operator.Mutant) bool { return a.ID == b.ID }) {
				t.Fatalf("%v != %v", first, second)
			}
		})
	})
}
