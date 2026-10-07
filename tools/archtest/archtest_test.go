// Package archtest_test checks the rules of ADR 0005 that span the modules of the
// repository, using `go list` on each module.
package archtest_test

import (
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
)

const (
	rootPath      = "github.com/RachidChabane/spec-proof"
	repositoryDir = "../.."
	toolsDir      = "tools"
	goModFile     = "go.mod"

	formatModule       = "format"
	discpowerModule    = "discpower"
	alignmentModule    = "alignment"
	completenessModule = "completeness"
	ledgerModule       = "ledger"
	verifyModule       = "verify"
	s1relayModule      = "s1relay"

	formatTestSupportDir = "formattest"
	hostprotoDir         = "hostproto"
	mcpstdioDir          = "mcpstdio"
	hostprototestDir     = "hostprototest"

	netPackage      = "net"
	netHTTPPackage  = "net/http"
	cryptoTLS       = "crypto/tls"
	osPackage       = "os"
	ioFSPackage     = "io/fs"
	osExecPackage   = "os/exec"
	pathSeparator   = "/"
	signingAdapters = "internal/adapters/signing"

	listImportsTemplate      = "{{.ImportPath}}|{{join .Imports \" \"}}"
	listWithTestsTemplate    = "{{.ImportPath}}|{{join .Imports \" \"}} {{join .TestImports \" \"}} {{join .XTestImports \" \"}}"
	listFieldSeparator       = "|"
	listFieldCount           = 2
	minimumListedPackages    = 1
	minimumDependencyListing = 1
)

// modules lists every module of the repository except the tools module.
var modules = []string{
	formatModule, discpowerModule, alignmentModule, completenessModule,
	ledgerModule, verifyModule, s1relayModule,
}

// proofModules are the modules that produce proofs; none of them reaches the
// network.
var proofModules = []string{discpowerModule, alignmentModule, completenessModule}

// httpAllowedPrefixes are the first-party packages allowed to import net/http.
var httpAllowedPrefixes = []string{
	modulePath(s1relayModule) + pathSeparator,
	modulePath(ledgerModule) + pathSeparator + signingAdapters + pathSeparator,
	modulePath(verifyModule) + pathSeparator + signingAdapters + pathSeparator,
}

func modulePath(module string) string {
	return rootPath + pathSeparator + module
}

func moduleDir(module string) string {
	return path.Join(repositoryDir, module)
}

func formatPackage(subpath ...string) string {
	return path.Join(append([]string{modulePath(formatModule)}, subpath...)...)
}

// isUnder reports whether pkg is the package root or one of its descendants.
func isUnder(pkg, root string) bool {
	return pkg == root || strings.HasPrefix(pkg, root+pathSeparator)
}

// isFormatTestSupport reports whether a package of the format module is test
// support, which may use the operating system: the vectors' generators and the
// host protocol's test support with its fake host.
func isFormatTestSupport(pkg string) bool {
	return isUnder(pkg, formatPackage(formatTestSupportDir)) || isUnder(pkg, formatPackage(hostprotoDir, hostprototestDir))
}

// reachesAny lists the packages of a dependency list that are under any root.
func reachesAny(dependencies []string, roots ...string) []string {
	var found []string
	for _, dependency := range dependencies {
		if slices.ContainsFunc(roots, func(root string) bool { return isUnder(dependency, root) }) {
			found = append(found, dependency)
		}
	}
	return found
}

// moduleOf returns the module a first-party import path belongs to, and false
// for a path outside the repository.
func moduleOf(importPath string) (string, bool) {
	rest, found := strings.CutPrefix(importPath, rootPath+pathSeparator)
	if !found {
		return "", false
	}
	module, _, _ := strings.Cut(rest, pathSeparator)
	return module, true
}

func isNet(importPath string) bool {
	return importPath == netPackage || strings.HasPrefix(importPath, netPackage+pathSeparator)
}

// crossModuleViolations lists the imports of another module than self, other
// than format.
func crossModuleViolations(self string, imports []string) []string {
	var found []string
	for _, importPath := range imports {
		module, first := moduleOf(importPath)
		if first && module != self && module != formatModule {
			found = append(found, importPath)
		}
	}
	return found
}

// networkViolations lists the packages of the networking family a dependency
// list contains.
func networkViolations(dependencies []string) []string {
	var found []string
	for _, dependency := range dependencies {
		if dependency == netPackage || dependency == netHTTPPackage || dependency == cryptoTLS {
			found = append(found, dependency)
		}
	}
	return found
}

// httpViolation reports whether a first-party package imports net/http outside
// the packages allowed to.
func httpViolation(pkg string, imports []string) bool {
	if !slices.Contains(imports, netHTTPPackage) {
		return false
	}
	return !slices.ContainsFunc(httpAllowedPrefixes, func(prefix string) bool {
		return strings.HasPrefix(pkg+pathSeparator, prefix)
	})
}

// processImporters lists the packages of a dependency listing that import os/exec.
// A listing line is "<import path>|<imports separated by spaces>".
func processImporters(listing string) []string {
	var found []string
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		fields := strings.SplitN(line, listFieldSeparator, listFieldCount)
		if len(fields) == listFieldCount && slices.Contains(strings.Fields(fields[1]), osExecPackage) {
			found = append(found, fields[0])
		}
	}
	slices.Sort(found)
	return found
}

// execAllowlist names the upstream packages that import os/exec in code the
// verifier never runs, each with the reason. The set of importers reachable from
// the verifier must equal it, so a dependency change that adds or removes one is
// reviewed.
var execAllowlist = []struct{ importPath, reason string }{
	{"github.com/digitorus/pkcs7", "a test helper of the library, compiled into it, runs openssl"},
	{"github.com/google/certificate-transparency-go/x509", "root_darwin.go reads the macOS system roots with the security tool"},
	{"github.com/in-toto/in-toto-golang/in_toto", "runlib.go runs the commands of an in-toto layout"},
}

func allowedExecImporters() []string {
	allowed := make([]string, 0, len(execAllowlist))
	for _, entry := range execAllowlist {
		allowed = append(allowed, entry.importPath)
	}
	slices.Sort(allowed)
	return allowed
}

// verifierExecImporters lists the packages the verifier links on the given
// platform that import os/exec.
func verifierExecImporters(t *testing.T, goos string) []string {
	t.Helper()
	command := exec.Command("go", "list", "-C", moduleDir(verifyModule), "-deps", "-f", listImportsTemplate, "./cmd/...")
	command.Env = append(os.Environ(), "GOOS="+goos)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list for %s: %v", goos, err)
	}
	return processImporters(string(output))
}

// linkedProcessSymbols builds the verifier and lists the symbols of os/exec its
// binary holds: a binary whose linker pruned every use of the package holds none.
func linkedProcessSymbols(t *testing.T, cgo string) []string {
	t.Helper()
	binary := path.Join(t.TempDir(), "spec-proof-verify")
	build := exec.Command("go", "build", "-C", moduleDir(verifyModule), "-p", "1", "-o", binary, "./cmd/spec-proof-verify")
	build.Env = append(os.Environ(), "CGO_ENABLED="+cgo)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the verifier: %v\n%s", err, output)
	}
	symbols, err := exec.Command("go", "tool", "nm", binary).Output()
	if err != nil {
		t.Fatalf("go tool nm: %v", err)
	}
	var found []string
	for _, line := range strings.Split(string(symbols), "\n") {
		if strings.Contains(line, osExecPackage+".") {
			found = append(found, strings.TrimSpace(line))
		}
	}
	return found
}

// formatCoreViolations lists what a package of the format library may not
// import.
func formatCoreViolations(imports []string) []string {
	var found []string
	for _, importPath := range imports {
		if importPath == osPackage || importPath == ioFSPackage || importPath == osExecPackage || isNet(importPath) {
			found = append(found, importPath)
		}
	}
	return found
}

func goList(t *testing.T, module string, args ...string) string {
	t.Helper()
	command := exec.Command("go", append([]string{"list", "-C", moduleDir(module)}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list in %s %v: %v", module, args, err)
	}
	return string(output)
}

// directImports maps each package matching pattern in module to its imports.
func directImports(t *testing.T, module, pattern string, withTests bool) map[string][]string {
	t.Helper()
	template := listImportsTemplate
	if withTests {
		template = listWithTestsTemplate
	}
	packages := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(goList(t, module, "-f", template, pattern)), "\n") {
		fields := strings.SplitN(line, listFieldSeparator, listFieldCount)
		if len(fields) != listFieldCount {
			t.Fatalf("unexpected go list line %q", line)
		}
		packages[fields[0]] = strings.Fields(fields[1])
	}
	if len(packages) < minimumListedPackages {
		t.Fatalf("no package of %s matches %s", module, pattern)
	}
	return packages
}

// dependencies lists every package pattern in module links, transitively.
func dependencies(t *testing.T, module, pattern string) []string {
	t.Helper()
	listed := strings.Fields(goList(t, module, "-deps", pattern))
	if len(listed) < minimumDependencyListing {
		t.Fatalf("no dependency listed for %s %s", module, pattern)
	}
	return listed
}

func repositoryModules(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(repositoryDir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == toolsDir {
			continue
		}
		if _, err := os.Stat(path.Join(repositoryDir, entry.Name(), goModFile)); err == nil {
			found = append(found, entry.Name())
		}
	}
	return found
}

func TestModulesAreIndependent(t *testing.T) {
	t.Run("every module of the repository is checked [verifies SP-107~1]", func(t *testing.T) {
		found := repositoryModules(t)
		if !slices.Equal(slices.Sorted(slices.Values(found)), slices.Sorted(slices.Values(modules))) {
			t.Fatalf("modules on disk %v, checked %v", found, modules)
		}
	})

	t.Run("no module imports another module except format [verifies SP-107~1]", func(t *testing.T) {
		for _, module := range modules {
			for pkg, imports := range directImports(t, module, "./...", true) {
				if found := crossModuleViolations(module, imports); len(found) > 0 {
					t.Errorf("%s imports %v", pkg, found)
				}
			}
		}
	})

	t.Run("the rule detects an import of another module [verifies SP-107~1]", func(t *testing.T) {
		got := crossModuleViolations(discpowerModule, []string{
			"fmt",
			modulePath(formatModule) + "/hostproto",
			modulePath(discpowerModule) + "/internal/cli",
			modulePath(ledgerModule) + "/internal/cli",
			modulePath(discpowerModule+"x") + "/internal/cli",
			"github.com/example/other",
		})
		if len(got) != 2 {
			t.Fatalf("violations = %v", got)
		}
	})
}

func TestFormatIsPure(t *testing.T) {
	t.Run("format imports no os, io/fs, os/exec or net and links no networking [verifies SP-106~2]", func(t *testing.T) {
		for pkg, imports := range directImports(t, formatModule, "./...", false) {
			if isFormatTestSupport(pkg) {
				continue
			}
			if found := formatCoreViolations(imports); len(found) > 0 {
				t.Errorf("%s imports %v", pkg, found)
			}
			for _, dependency := range dependencies(t, formatModule, pkg) {
				if isNet(dependency) {
					t.Errorf("%s links %s", pkg, dependency)
				}
			}
		}
	})

	t.Run("the rule detects each forbidden import [verifies SP-106~2]", func(t *testing.T) {
		got := formatCoreViolations([]string{"fmt", "os", "net/url", "os/exec", "io/fs", "strings", "net"})
		if len(got) != 5 {
			t.Fatalf("violations = %v", got)
		}
	})
}

func TestNetworkIsConfined(t *testing.T) {
	t.Run("nothing a proof module's binaries link imports net, net/http or crypto/tls [verifies SP-108~1]", func(t *testing.T) {
		for _, module := range proofModules {
			if found := networkViolations(dependencies(t, module, "./cmd/...")); len(found) > 0 {
				t.Errorf("%s links %v", module, found)
			}
		}
	})

	t.Run("the rule detects each networking package [verifies SP-108~1]", func(t *testing.T) {
		got := networkViolations([]string{"fmt", "net", "net/http", "net/url", "crypto/tls", "crypto/sha256"})
		if len(got) != 3 {
			t.Fatalf("violations = %v", got)
		}
	})

	t.Run("net/http is imported only by s1relay and the signing adapters of ledger and verify [verifies SP-109~1]", func(t *testing.T) {
		for _, module := range modules {
			for pkg, imports := range directImports(t, module, "./...", false) {
				if httpViolation(pkg, imports) {
					t.Errorf("%s imports %s", pkg, netHTTPPackage)
				}
			}
		}
	})

	t.Run("the rule allows net/http only where ADR 0005 does [verifies SP-109~1]", func(t *testing.T) {
		withHTTP := []string{"fmt", netHTTPPackage}
		cases := []struct {
			pkg  string
			want bool
		}{
			{modulePath(s1relayModule) + "/internal/relay", false},
			{modulePath(ledgerModule) + "/internal/adapters/signing/sigstore", false},
			{modulePath(verifyModule) + "/internal/adapters/signing/sigstore", false},
			{modulePath(ledgerModule) + "/internal/app/sign", true},
			{modulePath(discpowerModule) + "/internal/adapters/process", true},
			{modulePath(s1relayModule) + "x/internal/relay", true},
		}
		for _, c := range cases {
			if got := httpViolation(c.pkg, withHTTP); got != c.want {
				t.Errorf("%s: violation = %v, want %v", c.pkg, got, c.want)
			}
		}
		if httpViolation(modulePath(discpowerModule)+"/internal/cli", []string{"fmt"}) {
			t.Error("a package that does not import net/http is not a violation")
		}
	})
}

func TestVerifierStartsNoProcess(t *testing.T) {
	platforms := []string{"linux", "darwin", "windows"}

	t.Run("the packages that import os/exec below the verifier are exactly the allowlist [verifies SP-39~2]", func(t *testing.T) {
		union := map[string]bool{}
		for _, goos := range platforms {
			for _, importer := range verifierExecImporters(t, goos) {
				union[importer] = true
			}
		}
		var found []string
		for importer := range union {
			found = append(found, importer)
		}
		slices.Sort(found)
		if want := allowedExecImporters(); !slices.Equal(found, want) {
			t.Errorf("importers of os/exec reachable from the verifier: %v, allowlist: %v", found, want)
		}
	})

	t.Run("no package of the verifier module imports os/exec [verifies SP-39~2]", func(t *testing.T) {
		for _, goos := range platforms {
			for _, importer := range verifierExecImporters(t, goos) {
				if isUnder(importer, modulePath(verifyModule)) {
					t.Errorf("%s imports os/exec on %s", importer, goos)
				}
			}
		}
	})

	t.Run("the verifier binary holds no symbol of os/exec, with and without cgo [verifies SP-39~2]", func(t *testing.T) {
		for _, cgo := range []string{"1", "0"} {
			if found := linkedProcessSymbols(t, cgo); len(found) > 0 {
				t.Errorf("CGO_ENABLED=%s: the binary links %v", cgo, found[:min(len(found), 5)])
			}
		}
	})

	t.Run("the rule finds the importers of os/exec [verifies SP-39~2]", func(t *testing.T) {
		listing := "a|fmt os\nb|os/exec io\nc|os/signal\nd|os/exec"
		if got := processImporters(listing); !slices.Equal(got, []string{"b", "d"}) {
			t.Fatalf("importers = %v", got)
		}
	})
}

func TestHostProtocolLayering(t *testing.T) {
	supportRoots := []string{formatPackage(formatTestSupportDir), formatPackage(hostprotoDir, hostprototestDir)}

	t.Run("the format root and the host protocol link neither the MCP codec nor test support [verifies SP-143~1]", func(t *testing.T) {
		forbidden := append([]string{formatPackage(hostprotoDir, mcpstdioDir)}, supportRoots...)
		for _, pattern := range []string{".", "./" + hostprotoDir} {
			if found := reachesAny(dependencies(t, formatModule, pattern), forbidden...); len(found) > 0 {
				t.Errorf("%s links %v", pattern, found)
			}
		}
	})

	t.Run("the MCP codec links no test support [verifies SP-143~1]", func(t *testing.T) {
		pattern := "./" + hostprotoDir + pathSeparator + mcpstdioDir
		if found := reachesAny(dependencies(t, formatModule, pattern), supportRoots...); len(found) > 0 {
			t.Errorf("the codec links %v", found)
		}
	})

	t.Run("no binary of a shipped module links the test support of the format module [verifies SP-144~1]", func(t *testing.T) {
		for _, module := range modules {
			if module == formatModule {
				continue
			}
			if found := reachesAny(dependencies(t, module, "./cmd/..."), supportRoots...); len(found) > 0 {
				t.Errorf("%s links %v", module, found)
			}
		}
	})

	t.Run("the rule detects a package under a test support root [verifies SP-144~1]", func(t *testing.T) {
		got := reachesAny([]string{
			"fmt",
			formatPackage(hostprotoDir, hostprototestDir),
			formatPackage(hostprotoDir, hostprototestDir, "cmd", "fakehost"),
			formatPackage(formatTestSupportDir, "proofs"),
			formatPackage(hostprotoDir, hostprototestDir+"x"),
			formatPackage(hostprotoDir),
		}, supportRoots...)
		if len(got) != 3 {
			t.Fatalf("violations = %v", got)
		}
	})

	t.Run("test support packages of the format module are recognised as such [verifies SP-106~2]", func(t *testing.T) {
		cases := map[string]bool{
			formatPackage(formatTestSupportDir, "golden"):                    true,
			formatPackage(hostprotoDir, hostprototestDir):                    true,
			formatPackage(hostprotoDir, hostprototestDir, "cmd", "fakehost"): true,
			formatPackage(hostprotoDir):                                      false,
			formatPackage(hostprotoDir, mcpstdioDir):                         false,
			formatPackage(hostprotoDir, hostprototestDir+"x"):                false,
			modulePath(formatModule):                                         false,
		}
		for pkg, want := range cases {
			if got := isFormatTestSupport(pkg); got != want {
				t.Errorf("%s: %v, want %v", pkg, got, want)
			}
		}
	})
}
