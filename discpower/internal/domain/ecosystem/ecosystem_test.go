package ecosystem_test

import (
	"errors"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

func TestSpan(t *testing.T) {
	cases := []struct {
		name       string
		start, end int
		wantErr    bool
	}{
		{"an empty span", 3, 3, false},
		{"a span", 0, 5, false},
		{"a negative start", -1, 2, true},
		{"an inverted span", 4, 2, true},
	}
	for _, c := range cases {
		t.Run("NewSpan with "+c.name+" [verifies SP-31~1]", func(t *testing.T) {
			span, err := ecosystem.NewSpan(c.start, c.end)
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr && !errors.Is(err, ecosystem.ErrInvalidSpan) {
				t.Fatalf("error = %v, want ErrInvalidSpan", err)
			}
			if !c.wantErr && span.Len() != c.end-c.start {
				t.Fatalf("length = %d", span.Len())
			}
		})
	}

	t.Run("Slice returns the covered bytes and refuses a span outside the source [verifies SP-31~1]", func(t *testing.T) {
		source := []byte("def f(): pass")
		span, _ := ecosystem.NewSpan(4, 5)
		got, err := span.Slice(source)
		if err != nil || string(got) != "f" {
			t.Fatalf("slice = %q, %v", got, err)
		}
		outside := ecosystem.Span{Start: 4, End: 99}
		if _, err := outside.Slice(source); !errors.Is(err, ecosystem.ErrInvalidSpan) {
			t.Fatalf("error = %v, want ErrInvalidSpan", err)
		}
	})

	t.Run("a span built from ordered non-negative bounds always slices inside its source [verifies SP-31~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := rapid.SliceOf(rapid.Byte()).Draw(t, "source")
			start := rapid.IntRange(0, len(source)).Draw(t, "start")
			end := rapid.IntRange(start, len(source)).Draw(t, "end")
			span, err := ecosystem.NewSpan(start, end)
			if err != nil {
				t.Fatal(err)
			}
			got, err := span.Slice(source)
			if err != nil || len(got) != end-start {
				t.Fatalf("slice error %v, length %d", err, len(got))
			}
		})
	})
}

func TestNames(t *testing.T) {
	t.Run("the ecosystems are the six runners Shallnot binds [verifies SP-35~1]", func(t *testing.T) {
		want := map[ecosystem.Name]bool{
			ecosystem.Pytest: true, ecosystem.Jest: true, ecosystem.Vitest: true,
			ecosystem.Maven: true, ecosystem.Gradle: true, ecosystem.GoTesting: true,
		}
		names := ecosystem.Names()
		if len(names) != len(want) {
			t.Fatalf("names = %v", names)
		}
		for _, name := range names {
			if !want[name] || !name.Valid() {
				t.Errorf("unexpected ecosystem %s", name)
			}
		}
		if ecosystem.Name("cobol").Valid() {
			t.Error("cobol must not be valid")
		}
	})

	t.Run("mutating the returned list does not alter the set [verifies SP-35~1]", func(t *testing.T) {
		names := ecosystem.Names()
		names[0] = "cobol"
		if ecosystem.Names()[0] == "cobol" {
			t.Fatal("the internal list was exposed")
		}
	})
}
