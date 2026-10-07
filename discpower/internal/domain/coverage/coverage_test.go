package coverage_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

const verifiesSP172 = " [verifies SP-172~1]"

const (
	testA ecosystem.TestID = "tests/test_a.py::test_a"
	testB ecosystem.TestID = "tests/test_b.py::test_b"
	testC ecosystem.TestID = "tests/test_c.py::test_c"
)

func unit(file, qualname string) footprint.Ref { return footprint.Ref{File: file, Qualname: qualname} }

func sampleMap() coverage.Map {
	return coverage.Map{
		testA: {unit("src/b.py", "helper"), unit("src/a.py", "zeta"), unit("src/a.py", "alpha")},
		testB: {unit("src/a.py", "alpha"), unit("src/a.py", "Shape.area"), unit("src/a.py", "Shape.area.scale")},
		testC: {},
	}
}

func TestKnown(t *testing.T) {
	for _, tc := range []struct {
		name string
		test ecosystem.TestID
		want bool
	}{
		{"a test with units", testA, true},
		{"a test that executed nothing", testC, true},
		{"a test the map has not seen", "tests/test_x.py::test_x", false},
	} {
		t.Run(tc.name+verifiesSP172, func(t *testing.T) {
			if got := sampleMap().Known(tc.test); got != tc.want {
				t.Errorf("Known() = %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("a nil map knows no test"+verifiesSP172, func(t *testing.T) {
		if coverage.Map(nil).Known(testA) {
			t.Error("a nil map knows a test")
		}
	})
}

func TestEntriesOf(t *testing.T) {
	cases := []struct {
		name  string
		tests []ecosystem.TestID
		want  []footprint.Ref
	}{
		{"no test", nil, nil},
		{"a test that executed nothing", []ecosystem.TestID{testC}, nil},
		{"an unknown test", []ecosystem.TestID{"tests/test_x.py::test_x"}, nil},
		{"one test, sorted by file then qualname", []ecosystem.TestID{testA}, []footprint.Ref{unit("src/a.py", "alpha"), unit("src/a.py", "zeta"), unit("src/b.py", "helper")}},
		{"two tests, the units both executed listed once", []ecosystem.TestID{testA, testB}, []footprint.Ref{
			unit("src/a.py", "Shape.area"), unit("src/a.py", "Shape.area.scale"), unit("src/a.py", "alpha"), unit("src/a.py", "zeta"), unit("src/b.py", "helper")}},
		{"the same test twice", []ecosystem.TestID{testB, testB}, []footprint.Ref{unit("src/a.py", "Shape.area"), unit("src/a.py", "Shape.area.scale"), unit("src/a.py", "alpha")}},
		{"the order of the tests does not matter", []ecosystem.TestID{testB, testA}, []footprint.Ref{
			unit("src/a.py", "Shape.area"), unit("src/a.py", "Shape.area.scale"), unit("src/a.py", "alpha"), unit("src/a.py", "zeta"), unit("src/b.py", "helper")}},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP172, func(t *testing.T) {
			got := sampleMap().EntriesOf(tc.tests)
			if !slices.Equal(got, tc.want) {
				t.Errorf("EntriesOf() = %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("a unit listed twice by one test appears once"+verifiesSP172, func(t *testing.T) {
		repeated := coverage.Map{testA: {unit("f.py", "g"), unit("f.py", "g")}}
		if got := repeated.EntriesOf([]ecosystem.TestID{testA}); len(got) != 1 {
			t.Errorf("EntriesOf() = %v", got)
		}
	})
}

func TestCovering(t *testing.T) {
	cases := []struct {
		name       string
		candidates []ecosystem.TestID
		units      []footprint.Ref
		want       []ecosystem.TestID
	}{
		{"a unit executed exactly", []ecosystem.TestID{testA, testB}, []footprint.Ref{unit("src/b.py", "helper")}, []ecosystem.TestID{testA}},
		{"a unit executed by several tests, in the order of the candidates", []ecosystem.TestID{testB, testA}, []footprint.Ref{unit("src/a.py", "alpha")}, []ecosystem.TestID{testB, testA}},
		{"a definition nested in the unit", []ecosystem.TestID{testA, testB}, []footprint.Ref{unit("src/a.py", "Shape")}, []ecosystem.TestID{testB}},
		{"a definition nested two levels below the unit", []ecosystem.TestID{testB}, []footprint.Ref{unit("src/a.py", "Shape.area")}, []ecosystem.TestID{testB}},
		{"a nested definition does not cover its enclosing unit", []ecosystem.TestID{testB}, []footprint.Ref{unit("src/a.py", "Shape.area.scale.inner")}, nil},
		{"a unit whose name only starts like another", []ecosystem.TestID{testB}, []footprint.Ref{unit("src/a.py", "Sha")}, nil},
		{"the same qualname in another file", []ecosystem.TestID{testA, testB}, []footprint.Ref{unit("src/c.py", "alpha")}, nil},
		{"several units, a test covering any of them", []ecosystem.TestID{testA, testB}, []footprint.Ref{unit("src/b.py", "helper"), unit("src/a.py", "Shape")}, []ecosystem.TestID{testA, testB}},
		{"no unit", []ecosystem.TestID{testA, testB}, nil, nil},
		{"no candidate", nil, []footprint.Ref{unit("src/a.py", "alpha")}, nil},
		{"a test that executed nothing", []ecosystem.TestID{testC}, []footprint.Ref{unit("src/a.py", "alpha")}, nil},
		{"a candidate the map has not seen", []ecosystem.TestID{"tests/test_x.py::test_x"}, []footprint.Ref{unit("src/a.py", "alpha")}, nil},
		{"a candidate listed twice is listed twice", []ecosystem.TestID{testA, testA}, []footprint.Ref{unit("src/b.py", "helper")}, []ecosystem.TestID{testA, testA}},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP172, func(t *testing.T) {
			got := sampleMap().Covering(tc.candidates, tc.units)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Covering() = %v, want %v", got, tc.want)
			}
		})
	}
}
