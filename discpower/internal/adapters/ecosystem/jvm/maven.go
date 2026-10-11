package jvm

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const (
	pomFile           = "pom.xml"
	mavenWrapper      = "mvnw"
	mavenExecutable   = "mvn"
	mavenReportsDir   = "target/surefire-reports"
	mavenReportsGlob  = "TEST-*.xml"
	mavenClassesDir   = "target/classes"
	mavenTestClasses  = "target/test-classes"
	rootDirectory     = "."
	testClassSelector = "#"
	methodJoiner      = "+"
	classJoiner       = ","

	testCompileGoal = "test-compile"
	surefireGoal    = "surefire:test"
	testProperty    = "-Dtest="
	// DependencyPlugin is the goal that prints a project's classpath, pinned so
	// that an offline run finds the version it fetched.
	DependencyPlugin    = "org.apache.maven.plugins:maven-dependency-plugin:3.8.1:build-classpath"
	classpathOutputFlag = "-Dmdep.outputFile="
	classpathScopeFlag  = "-Dmdep.includeScope=test"
)

// mavenBaseFlags keep a build offline, quiet, non-interactive and single threaded.
var mavenBaseFlags = []string{"-o", "-q", "-B", "-T", "1"}

// mavenSurefireFlags run an unselected suite and a selection that matches nothing
// without failing the build.
var mavenSurefireFlags = []string{"-Dsurefire.failIfNoSpecifiedTests=false", "-DfailIfNoTests=false"}

// mavenTool drives JUnit 5 through Maven and Surefire.
type mavenTool struct{}

func (mavenTool) name() ecosystem.Name { return ecosystem.Maven }

func (mavenTool) detect(root fs.FS) (ecosystem.Detection, bool) {
	if _, err := fs.Stat(root, pomFile); err != nil {
		return ecosystem.Detection{}, false
	}
	return ecosystem.Detection{Name: ecosystem.Maven, Root: rootDirectory, Evidence: []string{pomFile}}, true
}

func (mavenTool) layout() ecosystem.Layout {
	return ecosystem.Layout{TestFileGlobs: testFileGlobs}
}

func (mavenTool) wrapperName() string { return mavenWrapper }

func (mavenTool) executableName() string { return mavenExecutable }

func (mavenTool) buildCheckArgs() []string {
	return append(append([]string{}, mavenBaseFlags...), testCompileGoal)
}

// runArgs compile the test classes first, so that a test file written after the
// build check is compiled too; with nothing changed the compilation is a no-op.
func (mavenTool) runArgs(filters []string) []string {
	args := append(append([]string{}, mavenBaseFlags...), testCompileGoal, surefireGoal)
	return append(append(args, mavenSurefireFlags...), filters...)
}

// filters build one `-Dtest=` pattern: `Class#method1+method2,Other`.
func (mavenTool) filters(targets []testTarget) []string {
	if len(targets) == 0 {
		return nil
	}
	var patterns []string
	for _, class := range classesOf(targets) {
		pattern := class.name
		if len(class.methods) > 0 {
			pattern += testClassSelector + strings.Join(class.methods, methodJoiner)
		}
		patterns = append(patterns, pattern)
	}
	return []string{testProperty + strings.Join(patterns, classJoiner)}
}

func (mavenTool) reports() reportLocation {
	return reportLocation{directory: mavenReportsDir, pattern: mavenReportsGlob, format: ecosystem.ReportSurefire}
}

func (mavenTool) buildFiles() []string { return []string{pomFile} }

func (mavenTool) classpath() classpathPlan {
	return classpathPlan{
		args: func(_, outputFile string) []string {
			return append(append([]string{}, mavenBaseFlags...), DependencyPlugin, classpathOutputFlag+outputFile, classpathScopeFlag)
		},
		entries: mavenClasspathEntries,
	}
}

// mavenClasspathEntries lists the build outputs of the project, then the
// dependencies the plugin printed.
func mavenClasspathEntries(_, content string) []classpathEntry {
	entries := []classpathEntry{{path: mavenTestClasses, inWorkspace: true}, {path: mavenClassesDir, inWorkspace: true}}
	for _, element := range filepath.SplitList(strings.TrimSpace(content)) {
		if element != "" {
			entries = append(entries, classpathEntry{path: element})
		}
	}
	return entries
}
