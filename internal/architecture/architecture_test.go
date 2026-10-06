// Package architecture_test checks the dependency rules of ADR 0002 with
// `go list`.
package architecture_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePath       = "github.com/RachidChabane/spec-proof"
	adaptersPath     = modulePath + "/internal/adapters"
	domainPattern    = modulePath + "/internal/domain/..."
	formatPattern    = modulePath + "/format/..."
	appPattern       = modulePath + "/internal/app/..."
	adaptersPattern  = modulePath + "/internal/adapters/..."
	verifyBinary     = modulePath + "/cmd/spec-proof-verify"
	listFieldSep     = "|"
	listImportSep    = ","
	listTemplate     = "{{.ImportPath}}" + listFieldSep + "{{join .Imports \"" + listImportSep + "\"}}"
	pathSeparator    = "/"
	osExecPackage    = "os/exec"
	netPackage       = "net"
	osPackage        = "os"
	ioFSPackage      = "io/fs"
	minimumPackages  = 1
	listFieldCount   = 2
	verifyForbidden  = "model ecosystem process git workspace"
	verifyForbidSep  = " "
	testkitDirectory = modulePath + "/internal/testkit"
)

// packageImports maps a package import path to its direct imports.
type packageImports map[string][]string

func goList(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.Command("go", append([]string{"list"}, args...)...).Output()
	if err != nil {
		t.Fatalf("go list %v: %v", args, err)
	}
	return string(output)
}

func directImports(t *testing.T, patterns ...string) packageImports {
	t.Helper()
	listed := goList(t, append([]string{"-f", listTemplate}, patterns...)...)
	packages := packageImports{}
	for _, line := range strings.Split(strings.TrimSpace(listed), "\n") {
		fields := strings.SplitN(line, listFieldSep, listFieldCount)
		if len(fields) != listFieldCount {
			t.Fatalf("unexpected go list line %q", line)
		}
		packages[fields[0]] = strings.FieldsFunc(fields[1], func(r rune) bool { return string(r) == listImportSep })
	}
	if len(packages) < minimumPackages {
		t.Fatalf("no package matches %v", patterns)
	}
	return packages
}

func dependencies(t *testing.T, target string) []string {
	t.Helper()
	return strings.Fields(goList(t, "-deps", target))
}

func isNet(path string) bool {
	return path == netPackage || strings.HasPrefix(path, netPackage+pathSeparator)
}

func isAdapter(path string) bool {
	return path == adaptersPath || strings.HasPrefix(path, adaptersPath+pathSeparator)
}

// pureCoreViolations lists what a package of the pure core may not import.
func pureCoreViolations(imports []string) []string {
	var found []string
	for _, path := range imports {
		if isAdapter(path) || path == osPackage || path == ioFSPackage || path == osExecPackage || isNet(path) {
			found = append(found, path)
		}
	}
	return found
}

// adapterToAdapterViolations lists the adapters an adapter imports besides its
// own sub-packages.
func adapterToAdapterViolations(self string, imports []string) []string {
	var found []string
	for _, path := range imports {
		if isAdapter(path) && path != self && !strings.HasPrefix(path, self+pathSeparator) {
			found = append(found, path)
		}
	}
	return found
}

func adapterImports(imports []string) []string {
	var found []string
	for _, path := range imports {
		if isAdapter(path) {
			found = append(found, path)
		}
	}
	return found
}

func verifyViolations(dependencyList []string) []string {
	var found []string
	forbidden := strings.Split(verifyForbidden, verifyForbidSep)
	for _, path := range dependencyList {
		if path == osExecPackage {
			found = append(found, path)
		}
		for _, family := range forbidden {
			prefix := adaptersPath + pathSeparator + family
			if path == prefix || strings.HasPrefix(path, prefix+pathSeparator) {
				found = append(found, path)
			}
		}
	}
	return found
}

func TestCoreIsPure(t *testing.T) {
	t.Run("format and the domain import no adapter, os, io/fs, os/exec or net [verifies SP-36~1]", func(t *testing.T) {
		for pkg, imports := range directImports(t, formatPattern, domainPattern) {
			if strings.HasPrefix(pkg, testkitDirectory) {
				continue
			}
			if found := pureCoreViolations(imports); len(found) > 0 {
				t.Errorf("%s imports %v", pkg, found)
			}
		}
	})

	t.Run("format and the domain link no networking and no process package [verifies SP-36~1]", func(t *testing.T) {
		for pkg := range directImports(t, formatPattern, domainPattern) {
			for _, dependency := range dependencies(t, pkg) {
				if isNet(dependency) || dependency == osExecPackage {
					t.Errorf("%s links %s", pkg, dependency)
				}
			}
		}
	})

	t.Run("the rule detects a forbidden import [verifies SP-36~1]", func(t *testing.T) {
		got := pureCoreViolations([]string{"fmt", "os", "net/url", adaptersPath + "/config", "os/exec", "io/fs", "strings"})
		if len(got) != 5 {
			t.Fatalf("violations = %v", got)
		}
	})
}

func TestApplicationLayer(t *testing.T) {
	t.Run("the application layer imports no adapter [verifies SP-37~1]", func(t *testing.T) {
		for pkg, imports := range directImports(t, appPattern) {
			if found := adapterImports(imports); len(found) > 0 {
				t.Errorf("%s imports %v", pkg, found)
			}
		}
	})

	t.Run("the rule detects an adapter import [verifies SP-37~1]", func(t *testing.T) {
		if got := adapterImports([]string{"fmt", adaptersPath + "/schemacheck"}); len(got) != 1 {
			t.Fatalf("violations = %v", got)
		}
	})
}

func TestAdaptersAreIndependent(t *testing.T) {
	t.Run("an adapter imports no other adapter [verifies SP-38~1]", func(t *testing.T) {
		for pkg, imports := range directImports(t, adaptersPattern) {
			if found := adapterToAdapterViolations(pkg, imports); len(found) > 0 {
				t.Errorf("%s imports %v", pkg, found)
			}
		}
	})

	t.Run("an adapter may import its own sub-packages only [verifies SP-38~1]", func(t *testing.T) {
		self := adaptersPath + "/ecosystem"
		got := adapterToAdapterViolations(self, []string{self + "/pytest", adaptersPath + "/git", self + "x"})
		if len(got) != 2 {
			t.Fatalf("violations = %v", got)
		}
	})
}

func TestVerifierBinary(t *testing.T) {
	t.Run("spec-proof-verify links no process package and no model, ecosystem, process, git or workspace adapter [verifies SP-39~1]", func(t *testing.T) {
		if found := verifyViolations(dependencies(t, verifyBinary)); len(found) > 0 {
			t.Errorf("the verifier links %v", found)
		}
	})

	t.Run("the rule detects each forbidden family [verifies SP-39~1]", func(t *testing.T) {
		got := verifyViolations([]string{
			"os/exec",
			adaptersPath + "/model/claudecli",
			adaptersPath + "/ecosystem",
			adaptersPath + "/process",
			adaptersPath + "/git",
			adaptersPath + "/workspace/clone",
			adaptersPath + "/schemacheck",
			adaptersPath + "/gitlike",
		})
		if len(got) != 6 {
			t.Fatalf("violations = %v", got)
		}
	})
}
