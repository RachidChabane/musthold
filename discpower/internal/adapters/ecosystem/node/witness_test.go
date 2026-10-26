package node_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/nodeinstall"
)

const (
	witnessFixturesRoot = "../../../../fixtures/ecosystems"
	witnessRatesFile    = "src/rates.ts"
	witnessSourceRoot   = "src"
	shippingSpec        = "WHEN an order total reaches 50 THE SYSTEM SHALL waive the shippingFee."

	freeWitnessName = "check_free_from_fifty"
	freeWitnessCode = "function check_free_from_fifty(): void {\n  expect(shippingFee(FREE_SHIPPING_FROM)).toBe(0);\n}\n"
	feeWitnessName  = "check_fee_below_fifty"
	feeWitnessCode  = "async function check_fee_below_fifty(): Promise<void> {\n  expect(shippingFee(49.99)).toBe(SHIPPING_FEE);\n}\n"
	ratesImport     = "import { FREE_SHIPPING_FROM, SHIPPING_FEE, shippingFee } from '../src/rates';"
	vitestImport    = "import { expect } from 'vitest';"

	boundaryShippingEdit = "function shippingFee(total: number): number {\n  if (total > FREE_SHIPPING_FROM) {\n    return 0;\n  }\n  return SHIPPING_FEE;\n}"
)

var shippingCriterion = spec.Ref{ID: "PAR-3", Revision: 1}

func shippingSet(imports string) witness.Set {
	return witness.Set{Criterion: shippingCriterion, Imports: imports, Witnesses: []witness.Witness{
		witness.New(shippingCriterion, freeWitnessName, "waive the shipping fee", witness.KindExample, freeWitnessCode),
		witness.New(shippingCriterion, feeWitnessName, "reaches 50", witness.KindExample, feeWitnessCode),
	}}
}

func witnessStrategies() map[ecosystem.Name]*node.Strategy {
	return map[ecosystem.Name]*node.Strategy{
		ecosystem.Jest:   jestStrategy(nil, nil),
		ecosystem.Vitest: vitestStrategy(nil, nil),
	}
}

func readFixture(t *testing.T, name ecosystem.Name, file string) ([]byte, fstest.MapFS) {
	t.Helper()
	root := filepath.Join(witnessFixturesRoot, string(name))
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	project := fstest.MapFS{}
	for _, projectFile := range []string{"test/rates.test.ts", "tsconfig.json", "package.json"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(projectFile)))
		if err != nil {
			t.Fatal(err)
		}
		project[projectFile] = &fstest.MapFile{Data: data}
	}
	return content, project
}

func TestWitnessBrief(t *testing.T) {
	for name, strategy := range witnessStrategies() {
		rates, project := readFixture(t, name, witnessRatesFile)
		sources := []mutation.SourceFile{{Path: witnessRatesFile, Content: rates}}
		host := strategy.Witnesses()

		t.Run(string(name)+": the brief lists the headers of the exported definitions the spec mentions, never a body [verifies SP-1985~1]", func(t *testing.T) {
			brief, err := host.Brief(project, sources, shippingSpec)
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{"../src/rates: export function shippingFee(total: number): number"}; !slices.Equal(brief.Interface, want) {
				t.Errorf("interface %q", brief.Interface)
			}
			if !strings.HasPrefix(brief.Language, "TypeScript, ") || brief.Conventions == "" {
				t.Errorf("language %q, conventions %q", brief.Language, brief.Conventions)
			}
			if !strings.Contains(brief.Conventions, "directory test") {
				t.Errorf("conventions %q", brief.Conventions)
			}
		})

		t.Run(string(name)+": a spec that mentions no definition gets every exported one, constants by name and type [verifies SP-1985~1]", func(t *testing.T) {
			brief, err := host.Brief(project, sources, "nothing named here")
			if err != nil {
				t.Fatal(err)
			}
			if len(brief.Interface) != 7 {
				t.Fatalf("interface %q", brief.Interface)
			}
			if brief.Interface[0] != "../src/rates: export const FREE_SHIPPING_FROM" {
				t.Errorf("first entry %q", brief.Interface[0])
			}
			for _, entry := range brief.Interface {
				if strings.Contains(entry, "return") || strings.Contains(entry, "= 5") {
					t.Errorf("a body or a value leaked: %q", entry)
				}
			}
		})
	}

	t.Run("declarations other than functions are listed without their values, a class by its public members [verifies SP-1985~1]", func(t *testing.T) {
		source := "export const double = (value: number): number => value * 2;\n" +
			"export type Money = { amount: number; currency: string };\n" +
			"export interface Cart {\n  items: number[];\n}\n" +
			"export class Till {\n  total(cart: Cart): number {\n    return 0;\n  }\n  private secret(): number {\n    return 1;\n  }\n}\n" +
			"function hidden(): number {\n  return 2;\n}\n"
		brief, err := jestStrategy(nil, nil).Witnesses().Brief(fstest.MapFS{}, []mutation.SourceFile{{Path: "src/till.ts", Content: []byte(source)}}, "nothing")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"../src/till: export const double = (value: number): number =>",
			"../src/till: export type Money = { amount: number; currency: string }",
			"../src/till: export interface Cart {\n  items: number[];\n}",
			"../src/till: export class Till",
			"../src/till: total(cart: Cart): number (member of class Till)",
		}
		if !slices.Equal(brief.Interface, want) {
			t.Errorf("interface %q", brief.Interface)
		}
	})

	t.Run("the exported declarations of every test file are the helpers [verifies SP-1985~1]", func(t *testing.T) {
		project := fstest.MapFS{
			"test/a.test.ts":         {Data: []byte("export function makeCart(items: number[]): Cart {\n  return { items };\n}\n\nfunction local(): number {\n  return 1;\n}\n")},
			"test/support/b.test.ts": {Data: []byte("export const fixtureTotal = 12;\n")},
			"tsconfig.json":          {Data: []byte("{}")},
		}
		brief, err := vitestStrategy(nil, nil).Witnesses().Brief(project, nil, "nothing")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"./a.test: export function makeCart(items: number[]): Cart", "./support/b.test: export const fixtureTotal"}
		if !slices.Equal(brief.Helpers, want) {
			t.Errorf("helpers %q", brief.Helpers)
		}
	})

	t.Run("a JavaScript project is briefed in JavaScript, its modules required unless it is ESM [verifies SP-1985~1]", func(t *testing.T) {
		project := fstest.MapFS{"test/a.test.js": {Data: []byte("describe('a', () => {});\n")}}
		sources := []mutation.SourceFile{{Path: "lib/clamp.js", Content: []byte("function clamp(value) {\n  return value;\n}\nmodule.exports = { clamp };\n")}}
		brief, err := jestStrategy(nil, nil).Witnesses().Brief(project, sources, "clamp")
		if err != nil {
			t.Fatal(err)
		}
		if brief.Language != "JavaScript, Jest" || !strings.Contains(brief.Conventions, "require('../lib/clamp')") {
			t.Errorf("language %q, conventions %q", brief.Language, brief.Conventions)
		}
		if !slices.Equal(brief.Interface, []string{"../lib/clamp: function clamp(value)"}) {
			t.Errorf("interface %q", brief.Interface)
		}
	})
}

func TestWitnessLayout(t *testing.T) {
	for name, strategy := range witnessStrategies() {
		_, project := readFixture(t, name, witnessRatesFile)
		host := strategy.Witnesses()

		t.Run(string(name)+": a set is laid out beside the first test file, each witness renamed and registered as the reporter names it [verifies SP-1985~1]", func(t *testing.T) {
			set := shippingSet(ratesImport)
			file, err := host.Layout(project, set)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(file.Path, "test/musthold_") || !strings.HasSuffix(file.Path, ".test.ts") {
				t.Fatalf("path %q", file.Path)
			}
			content := string(file.Content)
			if !strings.Contains(content, ratesImport) || strings.Contains(content, "function "+freeWitnessName) {
				t.Errorf("content %q", content)
			}
			for _, w := range set.Witnesses {
				test, known := file.Tests[w.ID]
				renamed := "musthold_witness_" + w.ShortID()
				if !known || !strings.HasSuffix(test.Name, renamed) || !strings.Contains(content, "function "+renamed+"(") {
					t.Fatalf("test %+v of %s", test, w.Name)
				}
				if test.File != file.Path {
					t.Errorf("file %q", test.File)
				}
				if line := strings.Split(content, "\n")[test.Line-1]; !strings.Contains(line, `"`+renamed+`", `+renamed) {
					t.Errorf("line %d is %q", test.Line, line)
				}
				assertReporterNames(t, name, file.Path, renamed, test)
			}
		})

		t.Run(string(name)+": a witness that does not declare its function is left out [verifies SP-1985~1]", func(t *testing.T) {
			set := shippingSet(ratesImport)
			set.Witnesses[0].Code = "expect(true).toBe(true);\n"
			file, err := host.Layout(project, set)
			if err != nil {
				t.Fatal(err)
			}
			if _, known := file.Tests[set.Witnesses[0].ID]; known || len(file.Tests) != 1 {
				t.Errorf("tests %+v", file.Tests)
			}
		})
	}

	t.Run("Vitest imports its test functions under names a witness cannot shadow [verifies SP-1985~1]", func(t *testing.T) {
		_, project := readFixture(t, ecosystem.Vitest, witnessRatesFile)
		file, err := vitestStrategy(nil, nil).Witnesses().Layout(project, shippingSet(vitestImport))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(file.Content), "import { describe as mustholdDescribe, test as mustholdTest } from 'vitest';\n") {
			t.Errorf("content %q", file.Content)
		}
	})

	t.Run("a JavaScript project gets a JavaScript witness file [verifies SP-1985~1]", func(t *testing.T) {
		project := fstest.MapFS{"spec/a.spec.js": {Data: []byte("")}}
		file, err := jestStrategy(nil, nil).Witnesses().Layout(project, shippingSet("const { shippingFee } = require('../src/rates');"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(file.Path, "spec/musthold_") || !strings.HasSuffix(file.Path, ".test.js") {
			t.Errorf("path %q", file.Path)
		}
	})

	t.Run("a laid-out witness file is a test file of the ecosystem [verifies SP-1985~1]", func(t *testing.T) {
		_, project := readFixture(t, ecosystem.Jest, witnessRatesFile)
		strategy := jestStrategy(nil, nil)
		file, err := strategy.Witnesses().Layout(project, shippingSet(ratesImport))
		if err != nil {
			t.Fatal(err)
		}
		if !strategy.Layout().IsTestFile(file.Path) {
			t.Errorf("%s is not a test file", file.Path)
		}
	})
}

// assertReporterNames checks the classname and name of a laid-out test against
// the form each reporter writes for a test of a describe block.
func assertReporterNames(t *testing.T, name ecosystem.Name, file, renamed string, test ecosystem.BoundTest) {
	t.Helper()
	switch name {
	case ecosystem.Jest:
		title := "musthold_witnesses " + renamed
		if test.Classname != title || test.Name != title {
			t.Errorf("jest names %q %q", test.Classname, test.Name)
		}
	case ecosystem.Vitest:
		if test.Classname != file || test.Name != "musthold_witnesses > "+renamed {
			t.Errorf("vitest names %q %q", test.Classname, test.Name)
		}
	}
}

// liveRunner prepares an installed copy of the fixture as a reference and
// returns a runner of its witnesses.
func liveRunner(t *testing.T, name ecosystem.Name, build func(node.Dependencies) *node.Strategy) execution.Runner {
	t.Helper()
	project := nodeinstall.Project(t, filepath.Join(witnessFixturesRoot, string(name)))
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := build(node.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
	executor := mutantrun.Executor{Ecosystem: strategy, Workspaces: manager, Policy: mutation.Policy{SourceRoots: []string{witnessSourceRoot}, Layout: strategy.Layout()}}
	reference, err := executor.PrepareReference(context.Background(), project)
	if err != nil {
		t.Fatalf("prepare the reference: %v", err)
	}
	built, err := executor.BuildReference(context.Background(), reference, []string{witnessRatesFile})
	if err != nil || !built.OK {
		t.Fatalf("the reference does not build: %v %s", err, built.Diagnostics)
	}
	return execution.Runner{
		Executor: executor, Reference: reference,
		Witnesses: execution.Witnesses{Host: strategy.Witnesses(), Project: os.DirFS(project)},
	}
}

func TestWitnessRuns(t *testing.T) {
	frameworks := []struct {
		name    ecosystem.Name
		build   func(node.Dependencies) *node.Strategy
		imports string
	}{
		{ecosystem.Jest, node.NewJest, ratesImport},
		{ecosystem.Vitest, node.NewVitest, vitestImport + "\n" + ratesImport},
	}
	for _, framework := range frameworks {
		t.Run(string(framework.name), func(t *testing.T) {
			runner := liveRunner(t, framework.name, framework.build)
			ctx := context.Background()
			set := shippingSet(framework.imports)

			t.Run("a laid-out witness passes the reference three times out of three [verifies SP-1985~1]", func(t *testing.T) {
				runs, err := runner.ReferenceWitnesses(ctx, set)
				if err != nil {
					t.Fatal(err)
				}
				for _, w := range set.Witnesses {
					if witness.ReadingOf(runs[w.ID]) != witness.ReadingPasses {
						t.Errorf("%s ran %v", w.Name, runs[w.ID])
					}
				}
			})

			t.Run("a mutant that breaks what the witness checks fails it, reproduced [verifies SP-1985~1]", func(t *testing.T) {
				edits := mutation.EditSet{Edits: []mutation.Edit{{File: witnessRatesFile, Qualname: "shippingFee", Target: mutation.TargetDefinition, NewSource: boundaryShippingEdit}}}
				outcomes, err := runner.EditWitnesses(ctx, "boundary", edits, []witness.Set{set})
				if err != nil {
					t.Fatal(err)
				}
				if outcomes[set.Witnesses[0].ID] != witness.OutcomeFailed || outcomes[set.Witnesses[1].ID] != witness.OutcomePassed {
					t.Errorf("outcomes %v", outcomes)
				}
			})
		})
	}
}
