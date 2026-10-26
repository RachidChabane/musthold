package pytest

import (
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

func TestTestIDOfNode(t *testing.T) {
	cases := []struct {
		name string
		node string
		want ecosystem.TestID
	}{
		{"a module function", "tests/test_rates.py::test_line_total", ecosystem.NewTestID("tests.test_rates", "test_line_total")},
		{"a method of a class", "tests/test_rates.py::TestLimits::test_clamped", ecosystem.NewTestID("tests.test_rates.TestLimits", "test_clamped")},
		{"a nested class", "tests/test_rates.py::Outer::Inner::test_x", ecosystem.NewTestID("tests.test_rates.Outer.Inner", "test_x")},
		{"a parametrized test", "tests/test_rates.py::test_discount[10-200-180.0]", ecosystem.NewTestID("tests.test_rates", "test_discount[10-200-180.0]")},
		{"a parameter id holding the node separator", "test_a.py::test_x[a::b]", ecosystem.NewTestID("test_a", "test_x[a::b]")},
		{"a file at the root", "test_a.py::test_x", ecosystem.NewTestID("test_a", "test_x")},
	}
	for _, c := range cases {
		t.Run(c.name+" maps to its JUnit identity [verifies SP-161~1]", func(t *testing.T) {
			got, ok := testIDOfNode(c.node)
			if !ok || got != c.want {
				t.Fatalf("testIDOfNode(%q) = %q, %t; want %q", c.node, got, ok, c.want)
			}
		})
	}
	t.Run("a node id without a test segment is refused [verifies SP-161~1]", func(t *testing.T) {
		if id, ok := testIDOfNode("tests/test_rates.py"); ok {
			t.Fatalf("mapped to %q", id)
		}
	})
}

func TestHintOf(t *testing.T) {
	known := []differential.Hint{
		differential.HintInteger, differential.HintDecimal, differential.HintString,
		differential.HintBoolean, differential.HintDatetime, differential.HintUnknown,
	}
	for _, hint := range known {
		t.Run(string(hint)+" is kept [verifies SP-159~1]", func(t *testing.T) {
			if got := hintOf(string(hint)); got != hint {
				t.Fatalf("hintOf(%q) = %q", hint, got)
			}
		})
	}
	t.Run("a name outside the vocabulary is unknown [verifies SP-159~1]", func(t *testing.T) {
		if got := hintOf("complex"); got != differential.HintUnknown {
			t.Fatalf("hintOf = %q", got)
		}
	})
}
