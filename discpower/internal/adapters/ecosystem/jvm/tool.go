package jvm

import (
	"io/fs"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// buildTool is the Strategy of one build tool: the commands it is driven with and
// the places and formats it reports in.
type buildTool interface {
	name() ecosystem.Name
	detect(root fs.FS) (ecosystem.Detection, bool)
	layout() ecosystem.Layout
	// wrapperName is the script of the project's wrapper and executableName the
	// tool's own command.
	wrapperName() string
	executableName() string
	// buildCheckArgs compile the main and test sources.
	buildCheckArgs() []string
	// runArgs run the tests of a selection, whose arguments are filters.
	runArgs(filters []string) []string
	// filters turn the tests to run into the tool's test filter arguments.
	filters(targets []testTarget) []string
	// reports describes where the test run writes its JUnit XML.
	reports() reportLocation
	// buildFiles are the files, relative to the project root, that decide the
	// dependencies of the project.
	buildFiles() []string
	// classpath describes how to learn the project's test runtime classpath.
	classpath() classpathPlan
}

// reportLocation is where a tool leaves its test reports.
type reportLocation struct {
	// directory holds the report files, relative POSIX; it is emptied before a run
	// so that a run never reads the leftovers of an earlier one.
	directory string
	// pattern matches the report files, relative to the directory.
	pattern string
	format  ecosystem.ReportFormat
}

// classpathPlan is a command that prints the test classpath of a project into a
// file, and how to read the file.
type classpathPlan struct {
	// script, when not nil, is a file the command is given through scriptFile.
	script []byte
	// args follow the tool command; the command writes outputFile.
	args func(scriptFile, outputFile string) []string
	// entries reads the content of the output file of a command run in the
	// workspace directory.
	entries func(workspaceDir, content string) []classpathEntry
}

// classpathEntry is one element of a classpath. An element inside the project
// (its build output) is kept relative so that the classpath of one workspace
// serves every workspace of the same project.
type classpathEntry struct {
	path        string
	inWorkspace bool
}
