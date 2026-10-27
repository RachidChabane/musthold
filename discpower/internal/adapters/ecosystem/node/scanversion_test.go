package node_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/nodeinstall"
)

const (
	verifiesRunnerVersions = " [verifies SP-2659~1]"

	versionScript   = "scripts/scan-runner-version.cjs"
	runnerScript    = "scripts/scan-vitest-runner.mjs"
	rootVariable    = "MUSTHOLD_SCAN_ROOT"
	manifestName    = "package.json"
	jestRunnerKey   = "JEST"
	vitestRunnerKey = "VITEST"
	// supportedCheck prints, as JSON, the version requireSupported returns for
	// the runner the arguments name, or the refusal it throws.
	supportedCheck = `const versions = require(process.argv[1]);
try {
  process.stdout.write(JSON.stringify({ version: versions.requireSupported(process.argv[2], versions[process.argv[3]]) }));
} catch (error) {
  process.stdout.write(JSON.stringify({ refusal: error.message }));
}`
)

// installedPackage writes a project whose node_modules holds the package at
// the version; a package that keeps its manifest out of its exports is found
// from its main entry.
func installedPackage(t *testing.T, name, version string, exportsManifest bool) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, nodeModules, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exports := map[string]string{".": "./index.js"}
	if exportsManifest {
		exports["./"+manifestName] = "./" + manifestName
	}
	manifest, err := json.Marshal(map[string]any{"name": name, "version": version, "main": "index.js", "exports": exports})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.js"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type supportedResult struct {
	Version *string `json:"version"`
	Refusal string  `json:"refusal"`
}

func checkSupported(t *testing.T, root, runnerKey string) supportedResult {
	t.Helper()
	script, err := filepath.Abs(versionScript)
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("node", "-e", supportedCheck, script, root, runnerKey).Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var result supportedResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("output %q: %v", output, err)
	}
	return result
}

func TestScanRunnerVersions(t *testing.T) {
	nodeinstall.RequireNode(t)
	cases := []struct {
		name, packageName, runnerKey, version string
		exportsManifest                       bool
		refusal                               string
	}{
		{name: "the oldest supported Vitest passes", packageName: "vitest", runnerKey: vitestRunnerKey, version: "3.2.4", exportsManifest: true},
		{name: "a prerelease of a supported Vitest passes", packageName: "vitest", runnerKey: vitestRunnerKey, version: "5.1.0-beta.1", exportsManifest: true},
		{name: "a Vitest older than the supported majors is refused", packageName: "vitest", runnerKey: vitestRunnerKey, version: "2.1.9", exportsManifest: true, refusal: "musthold scan does not support vitest 2.1.9: it supports Vitest 3 to 5"},
		{name: "a Vitest newer than the supported majors is refused", packageName: "vitest", runnerKey: vitestRunnerKey, version: "6.0.0", exportsManifest: true, refusal: "musthold scan does not support vitest 6.0.0: it supports Vitest 3 to 5"},
		{name: "a Vitest that keeps its manifest out of its exports is read from its main entry", packageName: "vitest", runnerKey: vitestRunnerKey, version: "2.1.9", refusal: "musthold scan does not support vitest 2.1.9: it supports Vitest 3 to 5"},
		{name: "the oldest supported Jest passes", packageName: "jest", runnerKey: jestRunnerKey, version: "29.7.0", exportsManifest: true},
		{name: "a Jest older than the supported majors is refused", packageName: "jest", runnerKey: jestRunnerKey, version: "28.1.3", exportsManifest: true, refusal: "musthold scan does not support jest 28.1.3: it supports Jest 29 to 30"},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesRunnerVersions, func(t *testing.T) {
			result := checkSupported(t, installedPackage(t, c.packageName, c.version, c.exportsManifest), c.runnerKey)
			if result.Refusal != c.refusal {
				t.Fatalf("refusal %q, want %q", result.Refusal, c.refusal)
			}
			if c.refusal == "" && (result.Version == nil || *result.Version != c.version) {
				t.Fatalf("version %v, want %s", result.Version, c.version)
			}
		})
	}
	t.Run("a supported Vitest that exports no test runner class is refused, naming its version"+verifiesRunnerVersions, func(t *testing.T) {
		script, err := filepath.Abs(runnerScript)
		if err != nil {
			t.Fatal(err)
		}
		load := exec.Command("node", script)
		load.Env = append(os.Environ(), rootVariable+"="+installedPackage(t, "vitest", "4.0.0", true))
		output, err := load.CombinedOutput()
		want := "musthold scan does not support vitest 4.0.0: it exports no test runner class from vitest/runners or vitest, or no test function getter"
		if err == nil || !strings.Contains(string(output), want) {
			t.Fatalf("err %v, output %s", err, output)
		}
	})
	t.Run("a project whose root resolves no such runner passes"+verifiesRunnerVersions, func(t *testing.T) {
		if result := checkSupported(t, t.TempDir(), vitestRunnerKey); result.Refusal != "" || result.Version != nil {
			t.Fatalf("result %+v", result)
		}
	})
}

// relabelled is a node_modules directory that links to each package of the
// installed one, the named package's manifest aside, which declares the
// version instead.
func relabelled(t *testing.T, modules, name, version string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), nodeModules)
	entries, err := os.ReadDir(modules)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != name {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(modules, entry.Name()), filepath.Join(dir, entry.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	installed := filepath.Join(modules, name)
	files, err := os.ReadDir(installed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() != manifestName {
			if err := os.Symlink(filepath.Join(installed, file.Name()), filepath.Join(dir, name, file.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	content, err := os.ReadFile(filepath.Join(installed, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["version"] = version
	if content, err = json.Marshal(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, manifestName), content, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestScanRefusesUnsupportedRunnerVersions(t *testing.T) {
	refusals := []struct {
		runner            scanRunner
		version, supports string
	}{
		{runner: scanRunnersOf(ecosystem.Jest)[0], version: "28.1.3", supports: "Jest 29 to 30"},
		{runner: scanRunnersOf(ecosystem.Vitest)[0], version: "2.1.9", supports: "Vitest 3 to 5"},
	}
	for _, refused := range refusals {
		t.Run(refused.runner.label+": a run of a project whose runner is outside the supported majors is refused, naming its version"+verifiesRunnerVersions, func(t *testing.T) {
			name := string(refused.runner.name)
			modules := relabelled(t, filepath.Join(nodeinstall.Installed(t, refused.runner.installed), nodeModules), name, refused.version)
			s := scanProjectLinked(t, refused.runner, modules)
			_, err := s.suite.Run(context.Background(), s.workspace, s.toolchain, ports.ScanRunRequest{Runs: 1, Timeout: scanRunTimeout})
			want := "musthold scan does not support " + name + " " + refused.version + ": it supports " + refused.supports
			if !errors.Is(err, node.ErrUnsupportedVersion) || !strings.HasSuffix(err.Error(), want) {
				t.Fatalf("err %v, want %q", err, want)
			}
		})
	}
	t.Run("a refusal the runner prints is returned as an unsupported version, from its opening to the end of its line"+verifiesRunnerVersions, func(t *testing.T) {
		suite := node.NewVitest(node.Dependencies{Process: process.New(), Locator: treesitter.NewLocator()}).Scan()
		workspace := ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}
		refusing := ecosystem.Toolchain{Command: []string{"sh", "-c", `printf 'Error: musthold scan does not support vitest 2.1.9: it supports Vitest 3 to 5\n    at file.mjs:1:1\n' >&2; exit 1`, "npx"}}
		_, err := suite.Run(context.Background(), workspace, refusing, ports.ScanRunRequest{Runs: 1, Timeout: scanRunTimeout})
		if !errors.Is(err, node.ErrUnsupportedVersion) || err.Error() != "unsupported test runner version: musthold scan does not support vitest 2.1.9: it supports Vitest 3 to 5" {
			t.Fatalf("err %v", err)
		}
	})
}
