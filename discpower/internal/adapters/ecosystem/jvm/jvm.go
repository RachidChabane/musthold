// Package jvm is the Strategy of the JUnit 5 ecosystems on the Java virtual
// machine: Maven with Surefire and Gradle, for Java and Kotlin sources. The two
// build tools differ in commands, report locations and test filters, which the
// buildTool Strategy captures; everything else (toolchain resolution, build
// check, selection mapping, runner, prober) is written once.
package jvm

import (
	"io/fs"
	"os"
	"os/exec"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm/probe"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// DefaultTimeout bounds one build check or suite run.
const DefaultTimeout = 5 * time.Minute

// ProbeTimeout bounds one run of the probe program or one resolution of the
// project's classpath.
const ProbeTimeout = 2 * time.Minute

// Dependencies are the ports a JVM Strategy works through.
type Dependencies struct {
	Process ports.ProcessRunner
	Reader  ports.ResultsReader
	// Locator finds the definitions of Java and Kotlin sources.
	Locator ports.DefinitionLocator
	// Assignments is optional; without it module assignment edits are refused.
	Assignments ports.AssignmentLocator
	// LookPath finds an executable on PATH; it defaults to exec.LookPath.
	LookPath func(string) (string, error)
	// Getenv reads an environment variable; it defaults to os.Getenv.
	Getenv func(string) string
	// Java is the launcher of the probe program; it defaults to the java of
	// JAVA_HOME, then the java on PATH.
	Java []string
	// Timeout bounds one build or suite process; it defaults to DefaultTimeout.
	Timeout time.Duration
}

var (
	_ ports.Ecosystem = (*Strategy)(nil)
	_ ports.Probing   = (*Strategy)(nil)
)

// Strategy implements ports.Ecosystem for one build tool.
type Strategy struct {
	deps       Dependencies
	tool       buildTool
	classpaths *classpathCache
	// witnessTarget remembers the package a witness brief placed the witnesses in.
	witnessTarget *targetMemo
}

// NewMaven builds the Strategy of JUnit 5 on Maven.
func NewMaven(deps Dependencies) *Strategy { return newStrategy(deps, mavenTool{}) }

// NewGradle builds the Strategy of JUnit 5 on Gradle.
func NewGradle(deps Dependencies) *Strategy { return newStrategy(deps, gradleTool{}) }

func newStrategy(deps Dependencies, tool buildTool) *Strategy {
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.Timeout <= 0 {
		deps.Timeout = DefaultTimeout
	}
	return &Strategy{deps: deps, tool: tool, classpaths: newClasspathCache(), witnessTarget: &targetMemo{}}
}

// Name is ecosystem.Maven or ecosystem.Gradle.
func (s *Strategy) Name() ecosystem.Name { return s.tool.name() }

// Detect recognizes the build tool's project by its build files.
func (s *Strategy) Detect(root fs.FS) (ecosystem.Detection, bool) { return s.tool.detect(root) }

// Layout excludes the build tool's private state and recognizes test classes by
// their names.
func (s *Strategy) Layout() ecosystem.Layout { return s.tool.layout() }

// Toolchain resolves the project's wrapper, else the build tool on PATH.
func (s *Strategy) Toolchain() ports.ToolchainResolver { return toolchainResolver{s: s} }

// Locator is the injected tree-sitter locator.
func (s *Strategy) Locator() ports.DefinitionLocator { return s.deps.Locator }

// Assignments is the injected assignment locator, nil when none is configured.
func (s *Strategy) Assignments() ports.AssignmentLocator { return s.deps.Assignments }

// BuildCheck compiles the project's sources and tests.
func (s *Strategy) BuildCheck() ports.BuildCheck { return buildCheck{s: s} }

// Selector maps bound tests to the build tool's test filter.
func (s *Strategy) Selector() ports.TestSelector { return selector{tool: s.tool} }

// Runner runs the selection through the build tool and reads its JUnit XML.
func (s *Strategy) Runner() ports.SuiteRunner { return runner{s: s} }

// Prober calls the bound operations through the probe program.
func (s *Strategy) Prober() ports.Prober { return prober{s: s} }

// testFileGlobs name the classes that hold tests, in Java and in Kotlin.
var testFileGlobs = []string{
	"*Test.java", "*Tests.java", "Test*.java", "*IT.java",
	"*Test.kt", "*Tests.kt", "Test*.kt", "*IT.kt",
}

// sourceSuffixes are the extensions of the sources the build compiles.
var sourceSuffixes = []string{probe.JavaExtension, probe.KotlinExtension}
