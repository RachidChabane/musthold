package mutation

import (
	"encoding/json"
	"errors"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
)

func definitionEdit(file, qualname, source string) Edit {
	return Edit{File: file, Qualname: qualname, Target: TargetDefinition, NewSource: source}
}

type failer interface {
	Helper()
	Fatalf(format string, args ...any)
}

func mustID(t failer, set EditSet) string {
	t.Helper()
	id, err := set.ID()
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	return id
}

func TestEditSetID(t *testing.T) {
	first := definitionEdit("a.py", "f", "def f():\n    return 1")
	second := definitionEdit("b.py", "g", "def g():\n    return 2")

	t.Run("the id does not depend on the order of the edits [verifies SP-83~1]", func(t *testing.T) {
		forward := mustID(t, EditSet{Edits: []Edit{first, second}})
		backward := mustID(t, EditSet{Edits: []Edit{second, first}})
		if forward != backward {
			t.Fatalf("ids differ: %s %s", forward, backward)
		}
	})

	t.Run("the id changes with any field of any edit [verifies SP-83~1]", func(t *testing.T) {
		base := mustID(t, EditSet{Edits: []Edit{first}})
		variants := []Edit{
			definitionEdit("c.py", "f", first.NewSource),
			definitionEdit("a.py", "h", first.NewSource),
			definitionEdit("a.py", "f", first.NewSource+" "),
			{File: "a.py", Qualname: "f", Target: TargetModuleAssignment, NewSource: first.NewSource},
		}
		for _, variant := range variants {
			if mustID(t, EditSet{Edits: []Edit{variant}}) == base {
				t.Errorf("id unchanged for %+v", variant)
			}
		}
	})

	t.Run("the id is the SHA-256 of the canonical form with the target omitted for a definition [verifies SP-83~1]", func(t *testing.T) {
		raw, err := json.Marshal(EditSet{Edits: []Edit{first}})
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"edits":[{"file":"a.py","qualname":"f","new_source":"def f():\n    return 1"}]}`
		if string(raw) != want {
			t.Fatalf("wire form = %s", raw)
		}
		const canonical = `{"edits":[{"file":"a.py","new_source":"def f():\n    return 1","qualname":"f"}]}`
		if got, want := mustID(t, EditSet{Edits: []Edit{first}}), format.SHA256Hex([]byte(canonical)); got != want {
			t.Fatalf("id = %s, want %s", got, want)
		}
	})

	t.Run("the wire form reads back to the same edit set [verifies SP-83~1]", func(t *testing.T) {
		set := EditSet{Edits: []Edit{first, {File: "a.py", Qualname: "X", Target: TargetModuleAssignment, NewSource: "X = 2"}}}
		raw, err := json.Marshal(set)
		if err != nil {
			t.Fatal(err)
		}
		var back EditSet
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatal(err)
		}
		if mustID(t, back) != mustID(t, set) {
			t.Fatalf("round trip changed the id: %s", raw)
		}
	})

	t.Run("the id is invariant to the order of the edits for any edits [verifies SP-83~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			edits := rapid.SliceOfNDistinct(rapid.Custom(func(t *rapid.T) Edit {
				return definitionEdit(rapid.StringMatching(`[a-z]{1,4}\.py`).Draw(t, "file"), rapid.StringMatching(`[A-Za-z_.]{1,6}`).Draw(t, "qualname"), rapid.String().Draw(t, "source"))
			}), 1, 5, func(e Edit) string { return e.File + "\x00" + e.Qualname }).Draw(t, "edits")
			reversed := make([]Edit, len(edits))
			for i, e := range edits {
				reversed[len(edits)-1-i] = e
			}
			if mustID(t, EditSet{Edits: edits}) != mustID(t, EditSet{Edits: reversed}) {
				t.Fatal("order changed the id")
			}
		})
	})
}

func TestEditSetValidation(t *testing.T) {
	valid := definitionEdit("a.py", "f", "def f(): pass")
	cases := map[string]EditSet{
		"empty set":       {},
		"unknown target":  {Edits: []Edit{{File: "a.py", Qualname: "f", Target: "weird", NewSource: "x"}}},
		"empty file":      {Edits: []Edit{{Qualname: "f", Target: TargetDefinition}}},
		"empty qualname":  {Edits: []Edit{{File: "a.py", Target: TargetDefinition}}},
		"same unit twice": {Edits: []Edit{valid, valid}},
	}
	for name, set := range cases {
		t.Run(name+" is refused [verifies SP-84~1]", func(t *testing.T) {
			if err := set.Validate(); !errors.Is(err, ErrInvalidEditSet) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	t.Run("an absent target reads as a definition [verifies SP-84~1]", func(t *testing.T) {
		var edit Edit
		if err := json.Unmarshal([]byte(`{"file":"a.py","qualname":"f","new_source":"x"}`), &edit); err != nil {
			t.Fatal(err)
		}
		if edit.Target != TargetDefinition {
			t.Fatalf("target = %q", edit.Target)
		}
	})

	t.Run("two units of different kinds with one qualname are distinct [verifies SP-84~1]", func(t *testing.T) {
		set := EditSet{Edits: []Edit{valid, {File: "a.py", Qualname: "f", Target: TargetModuleAssignment, NewSource: "f = 1"}}}
		if err := set.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestEmptyTargetIsADefinition(t *testing.T) {
	t.Run("an edit without a target has the id of the same definition edit [verifies SP-83~1]", func(t *testing.T) {
		bare := Edit{File: "a.py", Qualname: "f", NewSource: "def f(): pass"}
		explicit := definitionEdit("a.py", "f", "def f(): pass")
		if bare.Kind() != TargetDefinition {
			t.Fatalf("kind = %q", bare.Kind())
		}
		if mustID(t, EditSet{Edits: []Edit{bare}}) != mustID(t, EditSet{Edits: []Edit{explicit}}) {
			t.Fatal("the ids differ")
		}
	})
}
