package pluginfiles_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecli"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

const (
	verifiesModShape     = " [verifies SP-1920~1]"
	verifiesModBehaviour = " [verifies SP-1921~1]"

	modGoldenPath    = "testdata/mod/register.golden.ts"
	modTestPath      = "testdata/mod/register.test.ts"
	modTestTarget    = "hooks/register.test.ts"
	modSampleName    = "full"
	modPassedPattern = `(\d+) pass`
	modFailedLine    = " 0 fail"
	filePermission   = 0o644
	directoryMode    = 0o755
)

var updateGolden = flag.Bool("update", false, "rewrite the golden file of the mod from the generator")

func sampleModule(t *testing.T) pluginfiles.Module {
	t.Helper()
	for _, s := range samples() {
		if s.name == modSampleName {
			return s.module
		}
	}
	t.Fatalf("no sample named %s", modSampleName)
	return pluginfiles.Module{}
}

func modSource(t *testing.T, module pluginfiles.Module) string {
	t.Helper()
	return generated(t, module, harness.Claude(module.Identity))[pluginfiles.ClaudeModPath]
}

// writeTree writes generated files under a directory.
func writeTree(t *testing.T, directory string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		target := filepath.Join(directory, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), filePermission); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClaudeMod(t *testing.T) {
	t.Run("the hooks file keeps the command hooks and loads the mod beside it"+verifiesModShape, func(t *testing.T) {
		for _, s := range samples() {
			files := generated(t, s.module, harness.Claude(s.module.Identity))
			var parsed snakeHooksFile
			decodeJSON(t, files["hooks/hooks.json"], &parsed)
			if len(parsed.Modules) != 1 || filepath.Join("hooks", parsed.Modules[0]) != pluginfiles.ClaudeModPath {
				t.Errorf("%s: modules = %v", s.name, parsed.Modules)
			}
			if len(parsed.Hooks) == 0 {
				t.Errorf("%s: the command hooks are gone", s.name)
			}
		}
	})

	t.Run("the mod of a module is the shared source with its binary and nothing else different"+verifiesModShape, func(t *testing.T) {
		samplesList := samples()
		reference := modSource(t, samplesList[0].module)
		for _, s := range samplesList[1:] {
			own := strings.ReplaceAll(modSource(t, s.module), s.module.Binary, samplesList[0].module.Binary)
			if own != reference {
				t.Errorf("%s: the mod differs from the shared source beyond the binary name", s.name)
			}
		}
		for _, s := range samplesList {
			if !strings.Contains(modSource(t, s.module), `"`+s.module.Binary+`"`) {
				t.Errorf("%s: the mod does not name its binary", s.name)
			}
		}
	})

	t.Run("the generated mod is the golden file byte for byte"+verifiesModShape, func(t *testing.T) {
		got := modSource(t, sampleModule(t))
		if *updateGolden {
			if err := os.WriteFile(modGoldenPath, []byte(got), filePermission); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(modGoldenPath)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("the generated mod differs from %s: run the tests with -update", modGoldenPath)
		}
	})

	t.Run("claude plugin validate accepts the generated plugin of every sample"+verifiesModShape, func(t *testing.T) {
		for _, s := range samples() {
			directory := t.TempDir()
			writeTree(t, directory, generated(t, s.module, harness.Claude(s.module.Identity)))
			claudecli.Validate(t, directory)
		}
	})
}

// Rules of the mods documentation a hooks module follows so that the CLI reads it
// statically.
var (
	moduleImport        = regexp.MustCompile(`(?m)^import\b[^\n]*?from\s+"([^"]+)"`)
	anyImportKeyword    = regexp.MustCompile(`(?m)^\s*import\b`)
	dynamicImport       = regexp.MustCompile(`\bimport\s*\(`)
	registrationCall    = regexp.MustCompile(`\bon\(`)
	literalRegistration = regexp.MustCompile(`\bon\(\s*"[a-z]+\.[a-z]+"`)
	destructuredEngine  = regexp.MustCompile(`(?:const|let|var)\s*\{[^}]*\}\s*=\s*\$(?:[^\w$]|$)|\(\s*\{[^}]*\}\s*,[^=]*=>`)
	nodeAPI             = regexp.MustCompile(`(?:^|[^.\w$])(?:process|Buffer|require|__dirname|__filename)\b|\bfrom\s+"node:`)
)

// engineModule is the only module a hooks module imports.
const engineModule = "claude-code"

// staticViolations lists the rules of the mods documentation the source breaks.
func staticViolations(source string) []string {
	var violations []string
	if dynamicImport.MatchString(source) {
		violations = append(violations, "a dynamic import")
	}
	imports := moduleImport.FindAllStringSubmatch(source, -1)
	if len(anyImportKeyword.FindAllString(source, -1)) != len(imports) {
		violations = append(violations, "an import that is not `import ... from \"module\"` on one line")
	}
	for _, match := range imports {
		if match[1] != engineModule {
			violations = append(violations, "an import of "+match[1])
		}
	}
	registrations, literals := len(registrationCall.FindAllString(source, -1)), len(literalRegistration.FindAllString(source, -1))
	if registrations != literals {
		violations = append(violations, "a hook registration without a literal event name")
	}
	if destructuredEngine.MatchString(source) {
		violations = append(violations, "a destructured engine interface")
	}
	if found := nodeAPI.FindString(source); found != "" {
		violations = append(violations, "a Node API: "+strings.TrimSpace(found))
	}
	return violations
}

func TestClaudeModStaticRules(t *testing.T) {
	t.Run("the generated mod registers literal events, imports only claude-code and keeps the engine whole"+verifiesModShape, func(t *testing.T) {
		for _, s := range samples() {
			source := modSource(t, s.module)
			if violations := staticViolations(source); len(violations) != 0 {
				t.Errorf("%s: %v", s.name, violations)
			}
			if len(registrationCall.FindAllString(source, -1)) == 0 {
				t.Errorf("%s: the mod registers no hook", s.name)
			}
		}
	})

	t.Run("the checker rejects each way of breaking a rule"+verifiesModShape, func(t *testing.T) {
		violations := map[string]string{
			"a destructured engine parameter": "on(\"tool.call\", async ({ model }, e, next) => next(e))\n",
			"a destructured constant":         "const { model } = $\n",
			"a computed event name":           "on(EVENT, async ($, e, next) => next(e))\n",
			"a foreign import":                "import { readFile } from \"node:fs\"\n",
			"a dynamic import":                "const loaded = import(\"./other.ts\")\n",
			"a Node API":                      "const home = process.env.HOME\n",
		}
		for name, source := range violations {
			if len(staticViolations(source)) == 0 {
				t.Errorf("%s is not rejected", name)
			}
		}
	})
}

func TestClaudeModBehaviour(t *testing.T) {
	t.Run("claude plugin test runs the behaviour tests of the mod over the generated plugin"+verifiesModBehaviour, func(t *testing.T) {
		module := sampleModule(t)
		directory := t.TempDir()
		writeTree(t, directory, generated(t, module, harness.Claude(module.Identity)))
		test, err := os.ReadFile(modTestPath)
		if err != nil {
			t.Fatal(err)
		}
		writeTree(t, directory, map[string]string{modTestTarget: string(test)})
		output := claudecli.Test(t, directory)
		passed := regexp.MustCompile(modPassedPattern).FindStringSubmatch(output)
		if passed == nil || passed[1] == "0" || !strings.Contains(output, modFailedLine) {
			t.Errorf("the mod tests did not all pass:\n%s", output)
		}
	})
}
