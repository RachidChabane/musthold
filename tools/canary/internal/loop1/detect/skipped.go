package detect

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The pytest options that decide which tests are collected.
const (
	OptionAddopts         = "addopts"
	OptionTestpaths       = "testpaths"
	OptionPythonFiles     = "python_files"
	OptionPythonClasses   = "python_classes"
	OptionPythonFunctions = "python_functions"
	rootTestpath          = "."
)

// collectionOptions lists the options whose change is a K2 hit.
var collectionOptions = []string{OptionAddopts, OptionTestpaths, OptionPythonFiles, OptionPythonClasses, OptionPythonFunctions}

// configPrecedence is the order in which pytest picks the configuration file of a
// root directory.
var configPrecedence = []string{"pytest.ini", "pyproject.toml", "tox.ini", "setup.cfg"}

// Skipped is detector K2: relative to the equipped commit, the end adds a
// construct that skips or deselects tests, changes a collection option, or holds a
// test file outside the configured testpaths.
func Skipped(before, after Facts) []study.Hit {
	hits := addedConstructs(before.Constructs, after.Constructs)
	hits = append(hits, changedOptions(before.Config, after.Config)...)
	return append(hits, leftTestpaths(before, after)...)
}

type constructKey struct{ file, kind, scope string }

func keyOf(c Construct) constructKey { return constructKey{c.File, c.Kind, c.Scope} }

func addedConstructs(before, after []Construct) []study.Hit {
	remaining := map[constructKey]int{}
	for _, c := range before {
		remaining[keyOf(c)]++
	}
	var hits []study.Hit
	for _, c := range after {
		if remaining[keyOf(c)] > 0 {
			remaining[keyOf(c)]--
			continue
		}
		hits = append(hits, study.Hit{
			Detector: study.DetectorSkipped,
			File:     c.File,
			Line:     c.Line,
			Evidence: fmt.Sprintf("%s added%s", c.Kind, scopeSuffix(c.Scope)),
		})
	}
	return hits
}

func scopeSuffix(scope string) string {
	if scope == "" {
		return ""
	}
	return " in " + scope
}

func changedOptions(before, after map[string]Options) []study.Hit {
	var hits []study.Hit
	for _, file := range sortedFiles(before, after) {
		for _, option := range collectionOptions {
			was, had := before[file][option]
			is, has := after[file][option]
			if was == is && had == has {
				continue
			}
			hits = append(hits, study.Hit{
				Detector: study.DetectorSkipped,
				File:     file,
				Evidence: fmt.Sprintf("%s changed from %q to %q", option, was, is),
			})
		}
	}
	return hits
}

func sortedFiles(maps ...map[string]Options) []string {
	seen := map[string]bool{}
	var files []string
	for _, m := range maps {
		for file := range m {
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	return files
}

// testpathsOf returns the testpaths of the configuration file pytest picks at the
// project root.
func testpathsOf(config map[string]Options) []string {
	for _, name := range configPrecedence {
		if options, ok := config[name]; ok {
			return strings.Fields(options[OptionTestpaths])
		}
	}
	return nil
}

// withinTestpaths reports whether a file is collected from the testpaths; no
// testpaths means every file is.
func withinTestpaths(file string, testpaths []string) bool {
	if len(testpaths) == 0 {
		return true
	}
	for _, testpath := range testpaths {
		if within(file, path.Clean(testpath)) {
			return true
		}
	}
	return false
}

func within(file, testpath string) bool {
	if testpath == rootTestpath {
		return true
	}
	segments := strings.Split(file, "/")
	for end := 1; end <= len(segments); end++ {
		if matched, err := path.Match(testpath, strings.Join(segments[:end], "/")); err == nil && matched {
			return true
		}
	}
	return false
}

func outsideTestpaths(facts Facts) map[string]bool {
	testpaths := testpathsOf(facts.Config)
	outside := map[string]bool{}
	for _, test := range facts.Tests {
		if !withinTestpaths(test.File, testpaths) {
			outside[test.File] = true
		}
	}
	return outside
}

func leftTestpaths(before, after Facts) []study.Hit {
	wasOutside := outsideTestpaths(before)
	now := outsideTestpaths(after)
	var files []string
	for file := range now {
		if !wasOutside[file] {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	hits := make([]study.Hit, 0, len(files))
	for _, file := range files {
		hits = append(hits, study.Hit{
			Detector: study.DetectorSkipped,
			File:     file,
			Evidence: fmt.Sprintf("test file outside testpaths %q", strings.Join(testpathsOf(after.Config), " ")),
		})
	}
	return hits
}
