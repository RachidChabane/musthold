package golang

import "testing"

func TestReportedNames(t *testing.T) {
	t.Run("a reported subtest name equals the canonical form of its title [verifies SP-323~1]", func(t *testing.T) {
		titles := []string{
			"prints its version " + cite("RT-101~1"),
			"prints its version",
			"a " + cite("RT-1~1") + " b",
			"tabs\tand  spaces",
		}
		reported := []string{
			"prints_its_version_" + citeReported("RT-101~1"),
			"prints_its_version",
			"a_" + citeReported("RT-1~1") + "_b",
			"tabs_and__spaces",
		}
		for index, title := range titles {
			if canonicalSubtestName(title) != parseTestName("TestX/"+reported[index]).subtestKey() {
				t.Errorf("%q and %q do not meet", title, reported[index])
			}
		}
	})
	t.Run("the test name splits into the function and the first subtest [verifies SP-323~1]", func(t *testing.T) {
		path := parseTestName("TestX#01/first/second")
		if path.function != "TestX" || path.subtest != "first" {
			t.Errorf("path = %+v", path)
		}
		if key := parseTestName("TestX").subtestKey(); key != "" {
			t.Errorf("a top-level test has the subtest key %q", key)
		}
	})
	t.Run("citation markers are removed with the space before them [verifies SP-323~1]", func(t *testing.T) {
		for text, want := range map[string]string{
			"x " + cite("RT-1~1"):                          "x",
			"x " + citeCased("RT-1~1") + " y":              "x y",
			cite("RT-1~1"):                                 "",
			"no marker [other RT-1~1]":                     "no marker [other RT-1~1]",
			"two " + cite("RT-2~1") + " " + cite("RT-3~1"): "two",
		} {
			if got := removeCitations(text); got != want {
				t.Errorf("removeCitations(%q) = %q, want %q", text, got, want)
			}
		}
	})
	t.Run("the module line is read with quotes and comments [verifies SP-324~1]", func(t *testing.T) {
		for goMod, want := range map[string]string{
			"module a.example/x\n\ngo 1.22\n":     "a.example/x",
			"// header\nmodule \"a.example/y\"\n": "a.example/y",
			"module a.example/z // trailing\n":    "a.example/z",
			"go 1.22\n":                           "",
		} {
			if got := parseModulePath(goMod); got != want {
				t.Errorf("parseModulePath(%q) = %q, want %q", goMod, got, want)
			}
		}
	})
}
