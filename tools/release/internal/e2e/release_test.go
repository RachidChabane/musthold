// Package e2e_test runs the release scripts and the release tool end to end in a
// fixture repository, building one target at a time, and the installer against
// release directories written for the purpose.
package e2e_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/spec-proof/tools/release/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
	"github.com/RachidChabane/spec-proof/tools/release/internal/testkit"
)

const (
	verifiesDryRun         = " [verifies SP-1820~1]"
	verifiesReproducible   = " [verifies SP-1810~1]"
	verifiesArchiveContent = " [verifies SP-1805~2]"
	verifiesSBOMSchema     = " [verifies SP-1807~1]"
	verifiesChecksumTool   = " [verifies SP-1806~1]"
	throwawayTag           = "discpower/v0.0.0-e2e"
	throwawaySemver        = "0.0.0-e2e"
	exitRefused            = 3
	exitDistInvalid        = 4
	toolPath               = ".tools/bin/spec-proof-release"
	spdxSchemaRelative     = "tools/release/testdata/spdx-2.3/spdx-schema.json"
	spdxSchemaID           = "http://spdx.org/rdf/terms/2.3"
)

// sequentialBuilds keeps the fixture builds to one target at a time.
var sequentialBuilds = []string{"RELEASE_JOBS=1"}

// requireMachine skips the test when the memory guard refuses a heavy run or the
// short mode is on.
func requireMachine(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("cross-builds a fixture module for every target")
	}
	guard := filepath.Join(testkit.RepositoryRoot(t), "script", "mem-guard")
	if output, err := exec.Command("bash", guard, "check").CombinedOutput(); err != nil {
		t.Skipf("the memory guard refused the run: %s", strings.TrimSpace(string(output)))
	}
}

func release(t *testing.T, repository *testkit.Repository, arguments ...string) (string, int) {
	t.Helper()
	return repository.Run("bash", sequentialBuilds, append([]string{"script/release"}, arguments...)...)
}

func readDist(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = data
	}
	return files
}

func fileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func expectedNames(t *testing.T) []string {
	t.Helper()
	version := domain.Version{Major: 0, Minor: 0, Patch: 0, Prerelease: "e2e"}
	names := append(domain.ReleaseFiles(testkit.FixtureBinary, version, domain.Targets()), domain.InstallerFileName, domain.ChecksumsFileName)
	slices.Sort(names)
	return names
}

func compileSPDXSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testkit.RepositoryRoot(t), spdxSchemaRelative))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(spdxSchemaID, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(spdxSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestDryRunEndToEnd(t *testing.T) {
	requireMachine(t)
	repository := testkit.NewReleaseRepository(t)
	repository.Tag(throwawayTag)

	t.Run("the plan names the module, the binary and the version"+verifiesDryRun, func(t *testing.T) {
		output, code := release(t, repository, "plan", throwawayTag)
		if code != 0 || !strings.Contains(output, "binary=spec-proof-dp\n") || !strings.Contains(output, "semver="+throwawaySemver+"\n") {
			t.Fatalf("exit %d:\n%s", code, output)
		}
	})

	t.Run("a real release of the fixture is refused for the replace rule, the untagged format, the manifest and the marketplace"+verifiesDryRun, func(t *testing.T) {
		output, code := release(t, repository, throwawayTag, "--skip-tests")
		if code != exitRefused {
			t.Fatalf("exit %d:\n%s", code, output)
		}
		for _, rule := range []string{"go-mod-releasable", "format-tagged", "plugin-versions", "marketplace-ref"} {
			if !strings.Contains(output, "refused: "+rule+":") {
				t.Errorf("the refusal does not name %s:\n%s", rule, output)
			}
		}
		if _, err := os.Stat(filepath.Join(repository.Root, "dist")); err == nil {
			t.Error("a refused release built into dist")
		}
	})

	var firstDist map[string][]byte
	t.Run("the dry run reports those findings, builds every target, verifies the directory and installs from it"+verifiesDryRun, func(t *testing.T) {
		output, code := release(t, repository, throwawayTag, "--dry-run")
		if code != 0 {
			t.Fatalf("exit %d:\n%s", code, output)
		}
		for _, rule := range []string{"go-mod-releasable", "format-tagged", "plugin-versions", "marketplace-ref"} {
			if !strings.Contains(output, "a real release would be refused: "+rule+":") {
				t.Errorf("the dry run does not report %s:\n%s", rule, output)
			}
		}
		for _, want := range []string{"dist " + throwawayTag + ": ok", "installed " + testkit.FixtureBinary + " " + throwawaySemver, throwawayTag + " done"} {
			if !strings.Contains(output, want) {
				t.Errorf("the output lacks %q:\n%s", want, output)
			}
		}
		firstDist = readDist(t, filepath.Join(repository.Root, "dist"))
		if got, want := fileNames(firstDist), expectedNames(t); !slices.Equal(got, want) {
			t.Fatalf("dist holds %v, want %v", got, want)
		}
	})

	t.Run("every archive holds the binary, the licence, the README, the NOTICE and the third-party notices"+verifiesArchiveContent, func(t *testing.T) {
		tag, err := domain.ParseTag(throwawayTag)
		if err != nil {
			t.Fatal(err)
		}
		facts, err := adapters.DistDirectory{}.ReadDist(filepath.Join(repository.Root, "dist"), tag, domain.Targets())
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range domain.Targets() {
			name := target.ArchiveName(testkit.FixtureBinary, tag.Version)
			var got []string
			for _, entry := range facts.Files[name].Entries {
				got = append(got, entry.Name)
			}
			if want := domain.ExpectedEntryNames(target, testkit.FixtureBinary, true); !slices.Equal(got, want) {
				t.Errorf("%s holds %v, want %v", name, got, want)
			}
		}
	})

	t.Run("SHA256SUMS is accepted by the checksum tool of the machine"+verifiesChecksumTool, func(t *testing.T) {
		name, arguments := "sha256sum", []string{"-c", domain.ChecksumsFileName}
		if _, err := exec.LookPath(name); err != nil {
			name, arguments = "shasum", []string{"-a", "256", "-c", domain.ChecksumsFileName}
		}
		command := exec.Command(name, arguments...)
		command.Dir = filepath.Join(repository.Root, "dist")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, output)
		}
	})

	t.Run("every SBOM validates against the SPDX 2.3 schema"+verifiesSBOMSchema, func(t *testing.T) {
		schema := compileSPDXSchema(t)
		checked := 0
		for name, data := range firstDist {
			if !strings.HasSuffix(name, ".spdx.json") {
				continue
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(instance); err != nil {
				t.Errorf("%s: %v", name, err)
			}
			if !bytes.Contains(data, []byte("github.com/RachidChabane/spec-proof/format")) {
				t.Errorf("%s does not list the linked format module", name)
			}
			checked++
		}
		if checked != len(domain.Targets()) {
			t.Fatalf("%d SBOMs checked", checked)
		}
	})

	t.Run("a second build gives the same SHA256SUMS and the same bytes"+verifiesReproducible, func(t *testing.T) {
		output, code := release(t, repository, throwawayTag, "--dry-run", "--skip-tests", "--dist", "again")
		if code != 0 {
			t.Fatalf("exit %d:\n%s", code, output)
		}
		second := readDist(t, filepath.Join(repository.Root, "again"))
		if !bytes.Equal(firstDist[domain.ChecksumsFileName], second[domain.ChecksumsFileName]) {
			t.Fatalf("SHA256SUMS differ:\n%s\nagainst\n%s", firstDist[domain.ChecksumsFileName], second[domain.ChecksumsFileName])
		}
		for name, data := range firstDist {
			if !bytes.Equal(data, second[name]) {
				t.Errorf("%s differs between the two builds", name)
			}
		}
	})

	t.Run("installing from the dist directory puts a binary on the prefix that prints exactly the version"+verifiesDryRun, func(t *testing.T) {
		prefix := t.TempDir()
		output, code := repository.Run("bash", nil, "script/install", "discpower", throwawaySemver, "--from", "dist", "--prefix", prefix)
		if code != 0 {
			t.Fatalf("exit %d:\n%s", code, output)
		}
		reported, err := exec.Command(filepath.Join(prefix, "bin", testkit.FixtureBinary), "--version").Output()
		if err != nil || string(reported) != throwawaySemver+"\n" {
			t.Fatalf("the installed binary reports %q, error %v", reported, err)
		}
	})

	t.Run("the linked commit is the commit of the tag"+verifiesDryRun, func(t *testing.T) {
		head := repository.Git("rev-parse", "HEAD")
		prefix := t.TempDir()
		if output, code := repository.Run("bash", nil, "script/install", "discpower", throwawaySemver, "--from", "dist", "--prefix", prefix); code != 0 {
			t.Fatalf("exit %d:\n%s", code, output)
		}
		output, err := exec.Command(filepath.Join(prefix, "bin", testkit.FixtureBinary)).Output()
		if err != nil || !strings.Contains(string(output), head) {
			t.Fatalf("the binary prints %q, want the commit %s, error %v", output, head, err)
		}
	})

	t.Run("the formula and the notes are rendered from the dist directory"+verifiesDryRun, func(t *testing.T) {
		if output, code := repository.Run(toolPath, nil, "formula", throwawayTag); code != 0 {
			t.Fatalf("formula: exit %d:\n%s", code, output)
		}
		formula, err := os.ReadFile(filepath.Join(repository.Root, "dist", "spec-proof-dp.rb"))
		if err != nil || !strings.Contains(string(formula), "releases/download/discpower/v"+throwawaySemver+"/spec-proof-dp_"+throwawaySemver+"_darwin_arm64.tar.gz") {
			t.Fatalf("formula %q, error %v", formula, err)
		}
		if output, code := repository.Run(toolPath, nil, "verify-dist", throwawayTag); code != 0 {
			t.Fatalf("a dist directory with its formula is invalid: exit %d:\n%s", code, output)
		}
		if output, code := repository.Run(toolPath, nil, "notes", "--out", "notes.md", throwawayTag); code != 0 {
			t.Fatalf("notes: exit %d:\n%s", code, output)
		}
		notes, err := os.ReadFile(filepath.Join(repository.Root, "notes.md"))
		if err != nil || !strings.Contains(string(notes), "## discpower "+throwawaySemver) {
			t.Fatalf("notes %q, error %v", notes, err)
		}
	})

	t.Run("verifying a dist directory with a tampered archive finds it, and so does the installer"+verifiesDryRun, func(t *testing.T) {
		tampered := filepath.Join(t.TempDir(), "tampered")
		testkit.CopyTree(t, filepath.Join(repository.Root, "dist"), tampered, nil)
		archive := domain.Targets()[0].ArchiveName(testkit.FixtureBinary, domain.Version{Prerelease: "e2e"})
		data, err := os.ReadFile(filepath.Join(tampered, archive))
		if err != nil {
			t.Fatal(err)
		}
		data[len(data)/2] ^= 0xff
		if err := os.WriteFile(filepath.Join(tampered, archive), data, 0o644); err != nil {
			t.Fatal(err)
		}
		output, code := repository.Run(toolPath, nil, "verify-dist", "--dist", tampered, throwawayTag)
		if code != exitDistInvalid || !strings.Contains(output, archive) {
			t.Fatalf("verify-dist: exit %d:\n%s", code, output)
		}
	})
}

func TestBuildsAreReproducible(t *testing.T) {
	requireMachine(t)
	repository := testkit.NewReleaseRepository(t)
	repository.Tag(throwawayTag)
	if output, code := release(t, repository, "plan", throwawayTag); code != 0 {
		t.Fatalf("plan: exit %d:\n%s", code, output)
	}
	t.Run("two builds of the fixture, one target at a time, give byte-identical archives and SBOMs"+verifiesReproducible, func(t *testing.T) {
		var builds [2]map[string][]byte
		for index := range builds {
			dist := filepath.Join(t.TempDir(), "dist")
			output, code := repository.Run(toolPath, nil, "build", "--jobs", "1", "--dist", dist, throwawayTag)
			if code != 0 {
				t.Fatalf("build %d: exit %d:\n%s", index, code, output)
			}
			builds[index] = readDist(t, dist)
		}
		for name, data := range builds[0] {
			if !bytes.Equal(data, builds[1][name]) {
				t.Errorf("%s differs between the builds", name)
			}
		}
		if len(builds[0]) != len(expectedNames(t)) || len(builds[1]) != len(builds[0]) {
			t.Fatalf("builds hold %d and %d files", len(builds[0]), len(builds[1]))
		}
	})
}
