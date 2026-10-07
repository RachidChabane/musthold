package hostinstall

import (
	"slices"
	"strings"
)

// Markers and roots of the project layouts the installer recognizes.
const (
	goModule        = "go.mod"
	goSources       = "**/*.go"
	nodeManifest    = "package.json"
	nodeSourceRoot  = "src"
	pythonSourceDir = "src"
	pythonPackage   = "__init__.py"
	jvmSourceRoot   = "src/main"
	pathSeparator   = "/"
	rootDirectory   = ""
)

var (
	// jvmBuildFiles mark a Maven or Gradle project.
	jvmBuildFiles = []string{"pom.xml", "build.gradle", "build.gradle.kts"}
	// pythonMarkers mark a project that runs pytest.
	pythonMarkers = []string{"pytest.ini", "pyproject.toml", "setup.cfg", "setup.py", "tox.ini", "conftest.py"}
	// notPackages are the top-level directories of a Python project that never
	// hold the code its proofs name.
	notPackages = []string{"test", "tests", "docs", "doc", "build", "dist", "venv", "node_modules"}
)

// SourceRoots lists the files, directories and globs a proof of the project may
// name as code, read from the layout the project shows: a Go module holds its code
// in `.go` files anywhere, a Node project in `src`, a Maven or Gradle project in
// `src/main`, and a Python project in `src` or, without it, in each top-level
// package. A project that shows none of these has no root to list, and the
// result is empty.
func SourceRoots(files Files) []string {
	var roots []string
	add := func(candidates ...string) {
		for _, candidate := range candidates {
			if !slices.Contains(roots, candidate) {
				roots = append(roots, candidate)
			}
		}
	}
	if files.Exists(goModule) {
		add(goSources)
	}
	if files.Exists(nodeManifest) && files.Exists(nodeSourceRoot) {
		add(nodeSourceRoot)
	}
	if anyExists(files, jvmBuildFiles) && files.Exists(jvmSourceRoot) {
		add(jvmSourceRoot)
	}
	if anyExists(files, pythonMarkers) {
		add(pythonRoots(files)...)
	}
	return roots
}

// pythonRoots is `src` when the project has one, and otherwise each top-level
// directory that is a package.
func pythonRoots(files Files) []string {
	if files.Exists(pythonSourceDir) {
		return []string{pythonSourceDir}
	}
	var packages []string
	for _, name := range files.Directories(rootDirectory) {
		if isPackageDirectory(files, name) {
			packages = append(packages, name)
		}
	}
	slices.Sort(packages)
	return packages
}

func isPackageDirectory(files Files, name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || slices.Contains(notPackages, name) {
		return false
	}
	return files.Exists(name + pathSeparator + pythonPackage)
}
