package testkit

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Constants of the fixture repository.
const (
	FixtureModule    = "discpower"
	FixtureBinary    = "spec-proof-dp"
	FixturePluginDir = FixtureModule + "/plugin/claude/.claude-plugin"
	workspaceFile    = "go.work"
	scriptMode       = 0o755
	fileMode         = 0o644
)

// RepositoryRoot finds the root of the real repository by walking up from the
// directory of the running test to the one that holds go.work.
func RepositoryRoot(tb testing.TB) string {
	tb.Helper()
	directory, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, workspaceFile)); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			tb.Fatal("no go.work above the test directory")
		}
		directory = parent
	}
}

const fixtureMain = `package main

import (
	"fmt"
	"os"

	"github.com/RachidChabane/spec-proof/format"
)

var version, commit string

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	fmt.Println(format.Marker, commit)
}
`

const fixtureMarketplace = `{
  "name": "spec-proof",
  "plugins": [
    {
      "name": "spec-proof-dp",
      "source": {"source": "git-subdir", "url": "RachidChabane/spec-proof", "path": "discpower/plugin/claude"}
    },
    {
      "name": "spec-proof",
      "source": "./umbrella/plugins/spec-proof"
    }
  ]
}
`

// NewReleaseRepository builds a repository the release scripts run in: the real
// scripts and the real release tool, and a small module standing in for
// discpower that requires a small format module through a replace rule, with a
// plugin manifest and a marketplace entry that do not carry a release yet. Its
// single commit is checked out clean.
func NewReleaseRepository(tb testing.TB) *Repository {
	tb.Helper()
	real := RepositoryRoot(tb)
	repository := NewRepository(tb)
	repository.Write(".gitignore", "/.tools/\n/dist/\n", fileMode)
	repository.Write("LICENSE", "Apache License fixture\n", fileMode)
	repository.Write("NOTICE", "spec-proof fixture\n", fileMode)
	repository.Write("THIRD_PARTY_NOTICES.md", "# Third-party notices\n", fileMode)
	repository.Write("format/go.mod", "module github.com/RachidChabane/spec-proof/format\n\ngo 1.26.4\n", fileMode)
	repository.Write("format/format.go", "package format\n\n// Marker is exported so that a binary links the module.\nconst Marker = \"fixture\"\n", fileMode)
	repository.Write("format/README.md", "# format\n", fileMode)
	repository.Write(FixtureModule+"/go.mod", "module github.com/RachidChabane/spec-proof/"+FixtureModule+"\n\ngo 1.26.4\n\n"+
		"require github.com/RachidChabane/spec-proof/format v0.0.0\n\nreplace github.com/RachidChabane/spec-proof/format => ../format\n", fileMode)
	repository.Write(FixtureModule+"/cmd/"+FixtureBinary+"/main.go", fixtureMain, fileMode)
	repository.Write(FixtureModule+"/README.md", "# "+FixtureModule+"\n", fileMode)
	repository.Write(FixturePluginDir+"/plugin.json", `{"name":"spec-proof-dp","version":"0.1.0"}`+"\n", fileMode)
	repository.Write(".claude-plugin/marketplace.json", fixtureMarketplace, fileMode)
	repository.Write("script/plugin-gen", "#!/usr/bin/env bash\nexit 0\n", scriptMode)
	for _, script := range []string{"release", "install", "mem-guard"} {
		data, err := os.ReadFile(filepath.Join(real, "script", script))
		if err != nil {
			tb.Fatal(err)
		}
		repository.Write("script/"+script, string(data), scriptMode)
	}
	CopyTree(tb, filepath.Join(real, "tools", "release"), filepath.Join(repository.Root, "tools", "release"), releaseToolSource)
	repository.CommitAll("fixture: release repository")
	return repository
}

// releaseToolSource keeps what building the release tool needs: its module files,
// its command and its non-test packages.
func releaseToolSource(relative string, entry fs.DirEntry) bool {
	if entry.IsDir() {
		switch filepath.Base(relative) {
		case "testdata", "testkit", "e2e", "specs", "build":
			return true
		}
		return false
	}
	return strings.HasSuffix(relative, "_test.go") || strings.HasSuffix(relative, ".yaml") || strings.HasSuffix(relative, ".md")
}
