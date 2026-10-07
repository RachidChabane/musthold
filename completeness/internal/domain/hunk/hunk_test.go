package hunk_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/hunk"
)

func unit(qualname, normalized string, line int) code.Unit {
	return code.Unit{File: "totals.py", Qualname: qualname, Name: qualname, Kind: code.KindFunction, Normalized: normalized, Text: normalized + " // text", StartLine: line}
}

func TestBetween(t *testing.T) {
	before := []code.Unit{unit("total", "a", 1), unit("render", "b", 5), unit("gone", "c", 9)}

	t.Run("a unit that is new or whose normal form changed is a hunk, in line order [verifies SP-504~1]", func(t *testing.T) {
		after := []code.Unit{unit("discount", "n", 12), unit("total", "a2", 1), unit("render", "b", 5)}
		got := hunk.Between(before, after)
		if len(got) != 2 || got[0].Unit.Qualname != "total" || got[1].Unit.Qualname != "discount" {
			t.Fatalf("hunks %+v", got)
		}
		if got[0].New || got[0].Before != "a // text" {
			t.Fatalf("a changed unit keeps its text before: %+v", got[0])
		}
		if !got[1].New || got[1].Before != "" {
			t.Fatalf("a new unit has nothing before: %+v", got[1])
		}
	})

	t.Run("a unit whose normal form is unchanged is no hunk, whatever its text says [verifies SP-504~1]", func(t *testing.T) {
		reworded := unit("render", "b", 5)
		reworded.Text = "a reformatted text with a new comment"
		if got := hunk.Between(before, []code.Unit{reworded}); len(got) != 0 {
			t.Fatalf("hunks %+v", got)
		}
	})

	t.Run("a deleted unit is no hunk, and a file that did not exist makes every unit new [verifies SP-504~1]", func(t *testing.T) {
		if got := hunk.Between(before, nil); len(got) != 0 {
			t.Fatalf("hunks %+v", got)
		}
		got := hunk.Between(nil, before)
		if len(got) != 3 || !got[0].New || !got[1].New || !got[2].New {
			t.Fatalf("hunks %+v", got)
		}
	})

	t.Run("a file compared with itself has no hunk, and the hunks of a file are a subset of its units [verifies SP-504~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			names := []string{"a", "b", "c", "d", "e"}
			build := func(label string) []code.Unit {
				var units []code.Unit
				for index, name := range names {
					if rapid.Bool().Draw(t, label+name+"present") {
						units = append(units, unit(name, rapid.SampledFrom([]string{"x", "y"}).Draw(t, label+name+"form"), index))
					}
				}
				return units
			}
			old, current := build("old"), build("new")
			if got := hunk.Between(current, current); len(got) != 0 {
				t.Fatalf("a file against itself has hunks %+v", got)
			}
			for _, found := range hunk.Between(old, current) {
				present := false
				for _, candidate := range current {
					present = present || candidate.Qualname == found.Unit.Qualname
				}
				if !present {
					t.Fatalf("hunk %s is not a unit of the changed file", found.Unit.Qualname)
				}
			}
		})
	})
}

func TestID(t *testing.T) {
	t.Run("a hunk is identified by its file, unit and code, so the same change always has the same identity [verifies SP-504~1]", func(t *testing.T) {
		first := hunk.Hunk{Unit: unit("total", "a", 1)}
		same := hunk.Hunk{Unit: unit("total", "a", 40), Before: "other", New: true}
		changed := hunk.Hunk{Unit: unit("total", "b", 1)}
		moved := hunk.Hunk{Unit: unit("other", "a", 1)}
		if first.ID() != same.ID() || first.ID() == changed.ID() || first.ID() == moved.ID() {
			t.Fatalf("ids %s %s %s %s", first.ID(), same.ID(), changed.ID(), moved.ID())
		}
	})
}
