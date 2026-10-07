package jvm

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const (
	gradleWrapper    = "gradlew"
	gradleExecutable = "gradle"
	gradleReportsDir = "build/test-results/test"
	gradleReportGlob = "*.xml"

	cleanTestTask   = "cleanTest"
	testTask        = "test"
	testClassesTask = "testClasses"
	testsFlag       = "--tests"
	classpathInit   = "-I"
	classpathOutput = "-PspecProofClasspathOutput="
	classpathTask   = "specProofClasspath"
	methodSeparator = "."
)

// gradleBuildFiles decide the dependencies of a Gradle project.
var gradleBuildFiles = []string{
	"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts",
	"gradle.properties", "gradle/libs.versions.toml",
}

// gradleDetectionFiles reveal a Gradle project.
var gradleDetectionFiles = []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}

// gradleBaseFlags keep a build offline, quiet, without a daemon that would outlive
// it, and with the Kotlin compiler inside the build process.
var gradleBaseFlags = []string{
	"--offline", "-q", "--no-daemon", "--console=plain", "-Pkotlin.compiler.execution.strategy=in-process",
}

// classpathScript registers a task that prints the test runtime classpath of every
// project that applies the Java plugin, one element per line.
const classpathScript = `allprojects {
    plugins.withId('java') {
        tasks.register('specProofClasspath') {
            def output = providers.gradleProperty('specProofClasspathOutput')
            doLast {
                def elements = project.extensions.getByName('sourceSets').getByName('test').runtimeClasspath.files
                new File(output.get()).text = elements.collect { it.absolutePath }.join('\n')
            }
        }
    }
}
`

// gradleTool drives JUnit 5 through Gradle's own test task.
type gradleTool struct{}

func (gradleTool) name() ecosystem.Name { return ecosystem.Gradle }

func (gradleTool) detect(root fs.FS) (ecosystem.Detection, bool) {
	var evidence []string
	for _, file := range gradleDetectionFiles {
		if _, err := fs.Stat(root, file); err == nil {
			evidence = append(evidence, file)
		}
	}
	if len(evidence) == 0 {
		return ecosystem.Detection{}, false
	}
	return ecosystem.Detection{Name: ecosystem.Gradle, Root: rootDirectory, Evidence: evidence}, true
}

// layout leaves out the project's cache directories, which hold state keyed by
// the absolute path of the project.
func (gradleTool) layout() ecosystem.Layout {
	return ecosystem.Layout{ExcludedDirs: []string{".gradle", ".kotlin"}, TestFileGlobs: testFileGlobs}
}

func (gradleTool) wrapperName() string { return gradleWrapper }

func (gradleTool) executableName() string { return gradleExecutable }

func (gradleTool) buildCheckArgs() []string {
	return append(append([]string{}, gradleBaseFlags...), testClassesTask)
}

// runArgs clean the previous results first, so that a task Gradle considers up to
// date is run again.
func (gradleTool) runArgs(filters []string) []string {
	args := append(append([]string{}, gradleBaseFlags...), cleanTestTask, testTask)
	return append(args, filters...)
}

// filters build one `--tests` filter per class or method.
func (gradleTool) filters(targets []testTarget) []string {
	var filters []string
	for _, class := range classesOf(targets) {
		if len(class.methods) == 0 {
			filters = append(filters, testsFlag, class.name)
			continue
		}
		for _, method := range class.methods {
			filters = append(filters, testsFlag, class.name+methodSeparator+method)
		}
	}
	return filters
}

func (gradleTool) reports() reportLocation {
	return reportLocation{directory: gradleReportsDir, pattern: gradleReportGlob, format: ecosystem.ReportGradle}
}

func (gradleTool) buildFiles() []string { return gradleBuildFiles }

func (gradleTool) classpath() classpathPlan {
	return classpathPlan{
		script: []byte(classpathScript),
		args: func(scriptFile, outputFile string) []string {
			return append(append([]string{}, gradleBaseFlags...), classpathInit, scriptFile, classpathOutput+outputFile, classpathTask)
		},
		entries: gradleClasspathEntries,
	}
}

// gradleClasspathEntries reads one element per line; the elements below the
// project directory are the project's build outputs and are kept relative.
func gradleClasspathEntries(workspaceDir, content string) []classpathEntry {
	var entries []classpathEntry
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if line == "" {
			continue
		}
		if relative, err := filepath.Rel(workspaceDir, line); err == nil && !strings.HasPrefix(relative, "..") {
			entries = append(entries, classpathEntry{path: relative, inWorkspace: true})
			continue
		}
		entries = append(entries, classpathEntry{path: line})
	}
	return entries
}
