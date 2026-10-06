package treesitter

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	megabyte = 1_000_000

	pythonOnlyProgram     = "./internal/adapters/sourceparse/treesitter/testdata/pyonly"
	allLanguagesProgram   = "./internal/adapters/sourceparse/treesitter/testdata/langs"
	missingGrammarProgram = "./internal/adapters/sourceparse/treesitter/testdata/missing"

	subsetTag       = "grammar_subset"
	pythonSubsetTag = "grammar_subset_python"

	strippedLinkerFlags = "-s -w"

	// fullRegistryMinimumRatio is how many times larger than the python-only
	// subset the full-registry binary must be.
	fullRegistryMinimumRatio = 2

	// Size budgets in bytes: the measured size of a CGO_ENABLED=0 build plus
	// about 15 percent headroom.
	pythonSubsetBudgetDarwinArm64         = 15_800_000
	pythonSubsetBudgetLinuxAmd64          = 16_600_000
	pythonSubsetStrippedBudgetDarwinArm64 = 10_700_000
	pythonSubsetStrippedBudgetLinuxAmd64  = 11_500_000
	sixGrammarStrippedBudgetDarwinArm64   = 11_900_000
	sixGrammarStrippedBudgetLinuxAmd64    = 12_600_000

	sampleDefinitionsMinimum = 1
)

var sixGrammarTags = []string{
	subsetTag,
	"grammar_subset_python", "grammar_subset_java", "grammar_subset_kotlin",
	"grammar_subset_typescript", "grammar_subset_tsx", "grammar_subset_javascript",
}

var languageNames = []string{"python", "java", "kotlin", "typescript", "tsx", "javascript"}

type target struct {
	goos, goarch string
}

func (t target) String() string { return t.goos + "/" + t.goarch }

var (
	linuxAmd64  = target{"linux", "amd64"}
	darwinArm64 = target{"darwin", "arm64"}
)

type buildSpec struct {
	program  string
	tags     []string
	stripped bool
	target   target
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// build compiles spec with CGO_ENABLED=0 and returns the path of the binary.
func build(t *testing.T, spec buildSpec) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "program")
	args := []string{"build", "-o", output}
	if len(spec.tags) > 0 {
		args = append(args, "-tags", strings.Join(spec.tags, " "))
	}
	if spec.stripped {
		args = append(args, "-ldflags", strippedLinkerFlags)
	}
	args = append(args, spec.program)

	command := exec.Command("go", args...)
	command.Dir = moduleRoot(t)
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+spec.target.goos, "GOARCH="+spec.target.goarch)
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build %v: %v\n%s", args, err, combined)
	}
	return output
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func run(t *testing.T, binary string) string {
	t.Helper()
	var stdout bytes.Buffer
	command := exec.Command(binary)
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		t.Fatalf("run %s: %v", binary, err)
	}
	return stdout.String()
}

func isHost(t target) bool { return runtime.GOOS == t.goos && runtime.GOARCH == t.goarch }

func skipUnlessLongAndRunnable(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("cross-compiling size checks are skipped in short mode")
	}
}

func TestSubsetBinariesStayBelowSizeBudget(t *testing.T) {
	skipUnlessLongAndRunnable(t)
	tests := []struct {
		name   string
		spec   buildSpec
		budget int64
	}{
		{"python subset linux/amd64 [verifies SP-43~1]",
			buildSpec{pythonOnlyProgram, []string{subsetTag, pythonSubsetTag}, false, linuxAmd64}, pythonSubsetBudgetLinuxAmd64},
		{"python subset darwin/arm64 [verifies SP-43~1]",
			buildSpec{pythonOnlyProgram, []string{subsetTag, pythonSubsetTag}, false, darwinArm64}, pythonSubsetBudgetDarwinArm64},
		{"python subset stripped linux/amd64 [verifies SP-43~1]",
			buildSpec{pythonOnlyProgram, []string{subsetTag, pythonSubsetTag}, true, linuxAmd64}, pythonSubsetStrippedBudgetLinuxAmd64},
		{"python subset stripped darwin/arm64 [verifies SP-43~1]",
			buildSpec{pythonOnlyProgram, []string{subsetTag, pythonSubsetTag}, true, darwinArm64}, pythonSubsetStrippedBudgetDarwinArm64},
		{"six grammars stripped linux/amd64 [verifies SP-43~1]",
			buildSpec{allLanguagesProgram, sixGrammarTags, true, linuxAmd64}, sixGrammarStrippedBudgetLinuxAmd64},
		{"six grammars stripped darwin/arm64 [verifies SP-43~1]",
			buildSpec{allLanguagesProgram, sixGrammarTags, true, darwinArm64}, sixGrammarStrippedBudgetDarwinArm64},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binary := build(t, test.spec)
			size := sizeOf(t, binary)
			t.Logf("size %d bytes (budget %d)", size, test.budget)
			if size >= test.budget {
				t.Fatalf("size %d bytes reaches the budget of %d bytes", size, test.budget)
			}
			if isHost(test.spec.target) {
				assertProgramOutput(t, test.spec.program, run(t, binary))
			}
		})
	}
}

func assertProgramOutput(t *testing.T, program, output string) {
	t.Helper()
	switch program {
	case pythonOnlyProgram:
		if !strings.HasPrefix(output, "function_definition m ") {
			t.Fatalf("output %q does not report the method m", output)
		}
	case allLanguagesProgram:
		assertEveryLanguageFindsDefinitions(t, output)
	}
}

func assertEveryLanguageFindsDefinitions(t *testing.T, output string) {
	t.Helper()
	counts := map[string]int{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var language string
		var count int
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("unexpected line %q", scanner.Text())
		}
		language = fields[0]
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("line %q: %v", scanner.Text(), err)
		}
		counts[language] = count
	}
	for _, language := range languageNames {
		if counts[language] < sampleDefinitionsMinimum {
			t.Errorf("%s reports %d definitions in %q", language, counts[language], output)
		}
	}
}

func TestFullRegistryIsLargerThanPythonSubset(t *testing.T) {
	skipUnlessLongAndRunnable(t)
	for _, tgt := range []target{linuxAmd64, darwinArm64} {
		t.Run(tgt.String()+" [verifies SP-43~1]", func(t *testing.T) {
			subset := sizeOf(t, build(t, buildSpec{pythonOnlyProgram, []string{subsetTag, pythonSubsetTag}, false, tgt}))
			full := sizeOf(t, build(t, buildSpec{pythonOnlyProgram, nil, false, tgt}))
			t.Logf("subset %d bytes, full registry %d bytes", subset, full)
			if full <= fullRegistryMinimumRatio*subset {
				t.Fatalf("full registry %d bytes is not more than %dx the subset %d bytes", full, fullRegistryMinimumRatio, subset)
			}
			if isHost(tgt) {
				fullOutput := run(t, build(t, buildSpec{pythonOnlyProgram, nil, false, tgt}))
				assertProgramOutput(t, pythonOnlyProgram, fullOutput)
			}
		})
	}
}

func TestGrammarOutsideSubsetPanicsWhenLoaded(t *testing.T) {
	skipUnlessLongAndRunnable(t)
	t.Run("java inside the python-only subset [verifies SP-43~1]", func(t *testing.T) {
		binary := build(t, buildSpec{missingGrammarProgram, []string{subsetTag, pythonSubsetTag}, false, target{runtime.GOOS, runtime.GOARCH}})
		output := strings.TrimSpace(run(t, binary))
		want := fmt.Sprintf("panic: gotreesitter: failed to load grammar %q", "java.bin")
		if !strings.HasPrefix(output, want) || !strings.Contains(output, "is not embedded in this grammar_subset build") {
			t.Fatalf("output %q, want a panic naming the missing java.bin grammar", output)
		}
	})
}
