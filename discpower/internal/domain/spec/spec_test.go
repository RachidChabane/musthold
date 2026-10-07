package spec_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

const verifiesSP162 = " [verifies SP-162~1]"

func literal(kind spec.LiteralType, value string) spec.Literal {
	return spec.Literal{Type: kind, Value: value}
}

func TestLiteralEqual(t *testing.T) {
	cases := []struct {
		name  string
		left  spec.Literal
		right spec.Literal
		equal bool
	}{
		{"integer against the same integer", literal(spec.TypeInteger, "1"), literal(spec.TypeInteger, "1"), true},
		{"integer against a decimal of the same number", literal(spec.TypeInteger, "1"), literal(spec.TypeDecimal, "1.0"), true},
		{"decimal against an integer of the same number", literal(spec.TypeDecimal, "2.00"), literal(spec.TypeInteger, "2"), true},
		{"decimals with different spellings", literal(spec.TypeDecimal, "0.10"), literal(spec.TypeDecimal, "0.1"), true},
		{"integers with leading zeros", literal(spec.TypeInteger, "007"), literal(spec.TypeInteger, "7"), true},
		{"negative zero against zero", literal(spec.TypeInteger, "-0"), literal(spec.TypeInteger, "0"), true},
		{"integer in exponent form", literal(spec.TypeInteger, "1e3"), literal(spec.TypeInteger, "1000"), true},
		{"different integers", literal(spec.TypeInteger, "1"), literal(spec.TypeInteger, "2"), false},
		{"integer against a different decimal", literal(spec.TypeInteger, "2"), literal(spec.TypeDecimal, "2.5"), false},
		{"opposite signs", literal(spec.TypeInteger, "5"), literal(spec.TypeInteger, "-5"), false},
		{"numbers beyond float precision", literal(spec.TypeInteger, "9007199254740993"), literal(spec.TypeInteger, "9007199254740992"), false},
		{"unparsable numbers never equal", literal(spec.TypeInteger, "abc"), literal(spec.TypeInteger, "abc"), false},
		{"number against its text as a string", literal(spec.TypeInteger, "1"), literal(spec.TypeString, "1"), false},
		{"string against a number", literal(spec.TypeString, "1.0"), literal(spec.TypeDecimal, "1.0"), false},
		{"equal strings", literal(spec.TypeString, "abc"), literal(spec.TypeString, "abc"), true},
		{"strings differing by case", literal(spec.TypeString, "abc"), literal(spec.TypeString, "ABC"), false},
		{"numeric looking strings compare as text", literal(spec.TypeString, "1"), literal(spec.TypeString, "1.0"), false},
		{"equal booleans", literal(spec.TypeBoolean, "true"), literal(spec.TypeBoolean, "true"), true},
		{"different booleans", literal(spec.TypeBoolean, "true"), literal(spec.TypeBoolean, "false"), false},
		{"two nulls", literal(spec.TypeNull, ""), literal(spec.TypeNull, ""), true},
		{"null against an empty string", literal(spec.TypeNull, ""), literal(spec.TypeString, ""), false},
		{"equal datetimes", literal(spec.TypeDatetime, "2026-01-01T00:00:00Z"), literal(spec.TypeDatetime, "2026-01-01T00:00:00Z"), true},
		{"datetimes spelled in other zones compare as text", literal(spec.TypeDatetime, "2026-01-01T01:00:00+01:00"), literal(spec.TypeDatetime, "2026-01-01T00:00:00Z"), false},
		{"boolean against a string of the same text", literal(spec.TypeBoolean, "true"), literal(spec.TypeString, "true"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP162, func(t *testing.T) {
			if got := tc.left.Equal(tc.right); got != tc.equal {
				t.Errorf("%v.Equal(%v) = %v, want %v", tc.left, tc.right, got, tc.equal)
			}
			if got := tc.right.Equal(tc.left); got != tc.equal {
				t.Errorf("equality is not symmetric: %v.Equal(%v) = %v, want %v", tc.right, tc.left, got, tc.equal)
			}
		})
	}
}

func returns(kind spec.LiteralType, value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: kind, Value: value}
}

func raises(name string) spec.Observation {
	return spec.Observation{Kind: spec.Raises, Value: name}
}

func TestObservationSame(t *testing.T) {
	cases := []struct {
		name  string
		left  spec.Observation
		right spec.Observation
		same  bool
	}{
		{"same returned integer", returns(spec.TypeInteger, "3"), returns(spec.TypeInteger, "3"), true},
		{"integer and decimal of the same number", returns(spec.TypeInteger, "3"), returns(spec.TypeDecimal, "3.0"), true},
		{"different returned numbers", returns(spec.TypeInteger, "3"), returns(spec.TypeInteger, "4"), false},
		{"returned strings by text", returns(spec.TypeString, "x"), returns(spec.TypeString, "x"), true},
		{"returned string against returned number", returns(spec.TypeString, "3"), returns(spec.TypeInteger, "3"), false},
		{"same exception name", raises("ValueError"), raises("ValueError"), true},
		{"different exception names", raises("ValueError"), raises("KeyError"), false},
		{"exception names are case sensitive", raises("ValueError"), raises("valueerror"), false},
		{"raised exception ignores a type", spec.Observation{Kind: spec.Raises, Type: spec.TypeString, Value: "E"}, raises("E"), true},
		{"a return is never a raise of the same text", returns(spec.TypeString, "ValueError"), raises("ValueError"), false},
		{"a raise is never a return", raises("ValueError"), returns(spec.TypeString, "ValueError"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP162, func(t *testing.T) {
			if got := tc.left.Same(tc.right); got != tc.same {
				t.Errorf("%v.Same(%v) = %v, want %v", tc.left, tc.right, got, tc.same)
			}
			if got := tc.right.Same(tc.left); got != tc.same {
				t.Errorf("sameness is not symmetric: %v.Same(%v) = %v, want %v", tc.right, tc.left, got, tc.same)
			}
		})
	}
}

func TestExampleMet(t *testing.T) {
	example := spec.Example{Index: 1, Expect: returns(spec.TypeDecimal, "2.50")}
	raising := spec.Example{Index: 2, Expect: raises("ZeroDivisionError")}
	cases := []struct {
		name     string
		example  spec.Example
		observed spec.Observation
		met      bool
	}{
		{"a returned value of another spelling", example, returns(spec.TypeInteger, "2.5"), true},
		{"a returned value that differs", example, returns(spec.TypeDecimal, "2.51"), false},
		{"a raise instead of a return", example, raises("ZeroDivisionError"), false},
		{"the expected exception", raising, raises("ZeroDivisionError"), true},
		{"another exception", raising, raises("ValueError"), false},
		{"a return instead of the expected exception", raising, returns(spec.TypeInteger, "0"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP162, func(t *testing.T) {
			if got := tc.example.Met(tc.observed); got != tc.met {
				t.Errorf("Met(%v) = %v, want %v", tc.observed, got, tc.met)
			}
		})
	}
}

func TestRef(t *testing.T) {
	t.Run("a reference renders as ID~REV"+verifiesSP162, func(t *testing.T) {
		if got := (spec.Ref{ID: "SP-7", Revision: 2}).String(); got != "SP-7~2" {
			t.Errorf("String() = %q, want SP-7~2", got)
		}
	})
	cases := []struct {
		name  string
		left  spec.Ref
		right spec.Ref
		want  int
	}{
		{"identifier first", spec.Ref{ID: "SP-1", Revision: 9}, spec.Ref{ID: "SP-2", Revision: 1}, -1},
		{"larger identifier", spec.Ref{ID: "SP-3", Revision: 1}, spec.Ref{ID: "SP-2", Revision: 9}, 1},
		{"revision breaks a tie", spec.Ref{ID: "SP-1", Revision: 1}, spec.Ref{ID: "SP-1", Revision: 3}, -2},
		{"equal references", spec.Ref{ID: "SP-1", Revision: 1}, spec.Ref{ID: "SP-1", Revision: 1}, 0},
	}
	for _, tc := range cases {
		t.Run("compare orders references: "+tc.name+verifiesSP162, func(t *testing.T) {
			got := tc.left.Compare(tc.right)
			if (got < 0) != (tc.want < 0) || (got > 0) != (tc.want > 0) {
				t.Errorf("Compare = %d, want sign of %d", got, tc.want)
			}
		})
	}
}

func TestCriterionTestIDs(t *testing.T) {
	bound := func(ids ...string) []ecosystem.BoundTest {
		tests := make([]ecosystem.BoundTest, len(ids))
		for index, id := range ids {
			tests[index] = ecosystem.BoundTest{ID: ecosystem.TestID(id)}
		}
		return tests
	}
	cases := []struct {
		name  string
		tests []ecosystem.BoundTest
		want  []ecosystem.TestID
	}{
		{"no tests", nil, nil},
		{"order is kept", bound("b", "a", "c"), []ecosystem.TestID{"b", "a", "c"}},
		{"duplicates are dropped at their first place", bound("a", "b", "a", "b", "c"), []ecosystem.TestID{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP162, func(t *testing.T) {
			got := spec.Criterion{Tests: tc.tests}.TestIDs()
			if len(got) != len(tc.want) {
				t.Fatalf("TestIDs() = %v, want %v", got, tc.want)
			}
			for index := range got {
				if got[index] != tc.want[index] {
					t.Fatalf("TestIDs() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestParseLiteralType(t *testing.T) {
	for _, kind := range []spec.LiteralType{spec.TypeInteger, spec.TypeDecimal, spec.TypeString, spec.TypeBoolean, spec.TypeNull, spec.TypeDatetime} {
		t.Run("the grammar type "+string(kind)+" is read"+verifiesSP162, func(t *testing.T) {
			got, err := spec.ParseLiteralType(string(kind))
			if err != nil || got != kind {
				t.Errorf("ParseLiteralType(%q) = %q, %v", kind, got, err)
			}
		})
	}
	for _, name := range []string{"", "float", "Integer", "int "} {
		t.Run("the name "+name+" is refused"+verifiesSP162, func(t *testing.T) {
			if _, err := spec.ParseLiteralType(name); !errors.Is(err, spec.ErrUnknownLiteralType) {
				t.Errorf("ParseLiteralType(%q) error = %v, want ErrUnknownLiteralType", name, err)
			}
		})
	}
}
