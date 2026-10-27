package node_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// tsWorkspace is a monorepo whose root tsconfig only references its packages,
// one of which only references its application and tooling projects.
const tsWorkspace = "testdata/ts-workspace"

// The projects of the workspace, as tsc is pointed at them.
const (
	ratesProject     = "packages/rates/tsconfig.json"
	dashboardProject = "packages/dashboard/tsconfig.app.json"
	toolingProject   = "packages/dashboard/tsconfig.node.json"
	rootProject      = "tsconfig.json"
)

// typecheckByProject answers each tsc run with the output given for its
// project, and records the project of every run.
type typecheckByProject struct {
	output   map[string]string
	projects []string
}

func (p *typecheckByProject) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	project := command.Argv[len(command.Argv)-1]
	p.projects = append(p.projects, project)
	if out := p.output[project]; out != "" {
		return ports.CommandResult{ExitCode: failedTypecheck, Stdout: []byte(out)}, nil
	}
	return ports.CommandResult{}, nil
}

// copyWorkspace copies the fixture into a fresh workspace, with an installed
// package that ships a tsconfig of its own.
func copyWorkspace(t *testing.T, fixture string, extra map[string]string) ecosystem.Workspace {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	for name, content := range extra {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ecosystem.Workspace{Dir: dir}
}

func TestTypeCheckOfAWorkspace(t *testing.T) {
	ctx := context.Background()
	toolchain := ecosystem.Toolchain{Command: []string{"pnpm", "exec"}}
	installed := map[string]string{"node_modules/zod/tsconfig.json": `{"compilerOptions": {"strict": true}}`}
	buildCheck := func(process ports.ProcessRunner) ports.BuildCheck {
		return node.NewVitest(dependencies(process, nil, found)).BuildCheck()
	}

	t.Run("a changed file is type-checked with the projects its package's solution tsconfig references, not with the root's [verifies SP-1012~2]", func(t *testing.T) {
		process := &typecheckByProject{}
		result, err := buildCheck(process).Check(ctx, copyWorkspace(t, tsWorkspace, installed), toolchain, []string{"packages/dashboard/src/Banner.tsx"}, ecosystem.BuildBaseline{})
		if err != nil || !result.OK {
			t.Fatalf("%+v %v", result, err)
		}
		if want := []string{dashboardProject, toolingProject}; !slices.Equal(process.projects, want) {
			t.Fatalf("projects %v, want %v", process.projects, want)
		}
	})

	t.Run("a changed file is type-checked with the nearest tsconfig of its package, read with its comments and trailing commas [verifies SP-1012~2]", func(t *testing.T) {
		process := &typecheckByProject{}
		if _, err := buildCheck(process).Check(ctx, copyWorkspace(t, tsWorkspace, nil), toolchain, []string{"packages/rates/src/rates.ts", "packages/rates/src/rates.ts"}, ecosystem.BuildBaseline{}); err != nil {
			t.Fatal(err)
		}
		if want := []string{ratesProject}; !slices.Equal(process.projects, want) {
			t.Fatalf("projects %v, want %v", process.projects, want)
		}
	})

	t.Run("the baseline checks every project of the workspace once, the solution tsconfigs and the installed packages aside [verifies SP-1012~2]", func(t *testing.T) {
		process := &typecheckByProject{}
		if _, err := buildCheck(process).Baseline(ctx, copyWorkspace(t, tsWorkspace, installed), toolchain); err != nil {
			t.Fatal(err)
		}
		if want := []string{dashboardProject, toolingProject, ratesProject}; !slices.Equal(process.projects, want) {
			t.Fatalf("projects %v, want %v", process.projects, want)
		}
	})

	t.Run("a root tsconfig that fails alone on the reference fails no mutant of a package, and a package's own new diagnostic does [verifies SP-1012~2]", func(t *testing.T) {
		rootAlone := map[string]string{rootProject: `{"compilerOptions": {"strict": true}}`}
		jsxUnset := "packages/dashboard/src/Banner.tsx(4,10): error TS17004: Cannot use JSX unless the '--jsx' flag is provided."
		reference := &typecheckByProject{output: map[string]string{rootProject: jsxUnset}}
		baseline, err := buildCheck(reference).Baseline(ctx, copyWorkspace(t, tsWorkspace, rootAlone), toolchain)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(reference.projects, rootProject) {
			t.Fatalf("baseline projects %v", reference.projects)
		}
		changed := []string{"packages/dashboard/src/Banner.tsx"}
		quiet := &typecheckByProject{output: map[string]string{rootProject: jsxUnset}}
		if result, err := buildCheck(quiet).Check(ctx, copyWorkspace(t, tsWorkspace, rootAlone), toolchain, changed, baseline); err != nil || !result.OK || slices.Contains(quiet.projects, rootProject) {
			t.Fatalf("%+v %v %v", result, err, quiet.projects)
		}
		typeError := "packages/dashboard/src/Banner.tsx(4,40): error TS2345: Argument of type 'string' is not assignable to parameter of type 'number'."
		broken := &typecheckByProject{output: map[string]string{dashboardProject: typeError}}
		result, err := buildCheck(broken).Check(ctx, copyWorkspace(t, tsWorkspace, rootAlone), toolchain, changed, baseline)
		if err != nil || result.OK || !strings.Contains(result.Diagnostics, "TS2345") || !strings.Contains(result.Diagnostics, "-p "+dashboardProject) {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("a diagnostic one project reports on the reference does not excuse it in another project [verifies SP-1012~2]", func(t *testing.T) {
		shared := "packages/rates/src/rates.ts(1,14): error TS2451: Cannot redeclare block-scoped variable 'FULL_PERCENT'."
		reference := &typecheckByProject{output: map[string]string{toolingProject: shared}}
		baseline, err := buildCheck(reference).Baseline(ctx, copyWorkspace(t, tsWorkspace, nil), toolchain)
		if err != nil {
			t.Fatal(err)
		}
		mutant := &typecheckByProject{output: map[string]string{ratesProject: shared}}
		if result, err := buildCheck(mutant).Check(ctx, copyWorkspace(t, tsWorkspace, nil), toolchain, []string{"packages/rates/src/rates.ts"}, baseline); err != nil || result.OK {
			t.Fatalf("%+v %v", result, err)
		}
	})
}
