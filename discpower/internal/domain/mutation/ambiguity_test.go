package mutation_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

const repeatedSource = "RATE = 1\nRATE = 2\n\ndef add(a, b):\n    return a + b\n\ndef neg(a):\n    return -a\n\ndef neg(a):\n    return a\n"

func TestAmbiguousUnits(t *testing.T) {
	splicer := mutation.Splicer{Locator: lineLocator{failOn: "BROKEN"}, Assignments: lineAssignments{}}
	ambiguous, err := splicer.Ambiguous(sampleFile, []byte(repeatedSource))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a definition and an assignment each named by several units are ambiguous [verifies SP-2146~1]", func(t *testing.T) {
		if len(ambiguous) != 2 ||
			!ambiguous[mutation.UnitKey{Qualname: "neg"}] ||
			!ambiguous[mutation.UnitKey{Assignment: true, Qualname: "RATE"}] {
			t.Fatalf("ambiguous = %v", ambiguous)
		}
	})

	t.Run("an edit is covered exactly when its splice is refused as ambiguous [verifies SP-2146~1]", func(t *testing.T) {
		edits := []mutation.Edit{
			{Qualname: "add", NewSource: "def add(a, b):\n    return a - b"},
			{Qualname: "neg", NewSource: "def neg(a):\n    return a"},
			{Qualname: "RATE", Target: mutation.TargetModuleAssignment, NewSource: "RATE = 3"},
			{Qualname: "add", Target: mutation.TargetModuleAssignment, NewSource: "add = 3"},
			{Qualname: "RATE", NewSource: "def RATE(): pass"},
			{Qualname: "ghost", NewSource: "def ghost(): pass"},
		}
		for _, edit := range edits {
			_, err := splicer.Splice(sampleFile, []byte(repeatedSource), edit)
			if refused := errors.Is(err, mutation.ErrAmbiguousDefinition); ambiguous.Covers(edit) != refused {
				t.Fatalf("%s %s: covered = %v, splice error = %v", edit.Kind(), edit.Qualname, ambiguous.Covers(edit), err)
			}
		}
	})

	t.Run("a module addition targets no unit and is never covered [verifies SP-2146~1]", func(t *testing.T) {
		edit := mutation.Edit{Qualname: "neg", Target: mutation.TargetModuleAddition, NewSource: "UNUSED = 1"}
		if _, targets := edit.Targets(); targets || ambiguous.Covers(edit) {
			t.Fatal("a module addition was covered")
		}
	})

	t.Run("a source whose names are all unique has no ambiguous unit [verifies SP-2146~1]", func(t *testing.T) {
		none, err := splicer.Ambiguous(sampleFile, []byte(sampleSource))
		if err != nil || len(none) != 0 {
			t.Fatalf("ambiguous = %v, %v", none, err)
		}
	})

	t.Run("a source that cannot be located is an error [verifies SP-2146~1]", func(t *testing.T) {
		if _, err := splicer.Ambiguous(sampleFile, []byte("def BROKEN(")); err == nil {
			t.Fatal("no error")
		}
	})
}
