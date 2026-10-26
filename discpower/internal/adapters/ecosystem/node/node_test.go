package node_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// recorder is a ProcessRunner that records commands and answers by exit code.
type recorder struct {
	commands []ports.Command
	exit     func(ports.Command) int
	timedOut bool
}

func (r *recorder) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	r.commands = append(r.commands, command)
	code := 0
	if r.exit != nil {
		code = r.exit(command)
	}
	return ports.CommandResult{ExitCode: code, Stdout: []byte("src/rates.ts(3,1): error TS2322"), TimedOut: r.timedOut}, nil
}

type fixedReader struct {
	outcomes map[ecosystem.TestID]ecosystem.TestOutcome
	patterns []string
	format   ecosystem.ReportFormat
}

func (f *fixedReader) Read(patterns []string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	f.patterns, f.format = patterns, format
	return f.outcomes, nil
}

func found(name string) (string, error) { return "/usr/bin/" + name, nil }

func notFound(string) (string, error) { return "", errors.New("not found") }

func dependencies(process ports.ProcessRunner, reader ports.ResultsReader, lookPath func(string) (string, error)) node.Dependencies {
	return node.Dependencies{Process: process, Reader: reader, Locator: treesitter.NewLocator(), LookPath: lookPath}
}

func jestStrategy(process ports.ProcessRunner, reader ports.ResultsReader) *node.Strategy {
	return node.NewJest(dependencies(process, reader, found))
}

func vitestStrategy(process ports.ProcessRunner, reader ports.ResultsReader) *node.Strategy {
	return node.NewVitest(dependencies(process, reader, found))
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name      string
		strategy  *node.Strategy
		files     fstest.MapFS
		want      bool
		evidences []string
	}{
		{"jest config", jestStrategy(nil, nil), fstest.MapFS{"jest.config.js": {}}, true, []string{"jest.config.js"}},
		{"jest typescript config", jestStrategy(nil, nil), fstest.MapFS{"jest.config.ts": {}}, true, []string{"jest.config.ts"}},
		{"jest dev dependency", jestStrategy(nil, nil), fstest.MapFS{"package.json": {Data: []byte(`{"devDependencies":{"jest":"^30.0.0"}}`)}}, true, []string{"package.json"}},
		{"jest key of package.json", jestStrategy(nil, nil), fstest.MapFS{"package.json": {Data: []byte(`{"jest":{"testEnvironment":"node"}}`)}}, true, []string{"package.json"}},
		{"jest config and dependency", jestStrategy(nil, nil), fstest.MapFS{"jest.config.js": {}, "package.json": {Data: []byte(`{"dependencies":{"jest":"1"}}`)}}, true, []string{"jest.config.js", "package.json"}},
		{"vitest config", vitestStrategy(nil, nil), fstest.MapFS{"vitest.config.ts": {}}, true, []string{"vitest.config.ts"}},
		{"vitest workspace", vitestStrategy(nil, nil), fstest.MapFS{"vitest.workspace.ts": {}}, true, []string{"vitest.workspace.ts"}},
		{"vitest dependency", vitestStrategy(nil, nil), fstest.MapFS{"package.json": {Data: []byte(`{"devDependencies":{"vitest":"^5.0.0"}}`)}}, true, []string{"package.json"}},
		{"vitest is not jest", jestStrategy(nil, nil), fstest.MapFS{"package.json": {Data: []byte(`{"devDependencies":{"vitest":"^5.0.0"}}`)}}, false, nil},
		{"jest is not vitest", vitestStrategy(nil, nil), fstest.MapFS{"jest.config.js": {}}, false, nil},
		{"a package without a runner", jestStrategy(nil, nil), fstest.MapFS{"package.json": {Data: []byte(`{"name":"app"}`)}}, false, nil},
		{"an unreadable package.json", vitestStrategy(nil, nil), fstest.MapFS{"package.json": {Data: []byte(`{`)}}, false, nil},
		{"an empty tree", jestStrategy(nil, nil), fstest.MapFS{}, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-1010~1]", func(t *testing.T) {
			detection, ok := c.strategy.Detect(c.files)
			if ok != c.want {
				t.Fatalf("detection = %+v, %v", detection, ok)
			}
			if ok && (detection.Name != c.strategy.Name() || detection.Root != "." || !reflect.DeepEqual(detection.Evidence, c.evidences)) {
				t.Fatalf("detection = %+v", detection)
			}
		})
	}

	t.Run("the strategies are named after the runners they bind [verifies SP-1010~1]", func(t *testing.T) {
		if jestStrategy(nil, nil).Name() != ecosystem.Jest || vitestStrategy(nil, nil).Name() != ecosystem.Vitest {
			t.Fatal("the names differ from the ecosystems of the binding")
		}
	})
}

func lockfileWorkspace(t *testing.T, files ...string) ecosystem.Workspace {
	t.Helper()
	dir := t.TempDir()
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ecosystem.Workspace{Dir: dir}
}

func TestToolchainResolution(t *testing.T) {
	resolve := func(w ecosystem.Workspace, deps node.Dependencies) (ecosystem.Toolchain, error) {
		return node.NewJest(deps).Toolchain().Resolve(context.Background(), w)
	}
	cases := []struct {
		name     string
		files    []string
		expected []string
	}{
		{"an npm lock file gives npx without installation", []string{"package-lock.json"}, []string{"/usr/bin/npx", "--no-install"}},
		{"an npm shrinkwrap gives npx", []string{"npm-shrinkwrap.json"}, []string{"/usr/bin/npx", "--no-install"}},
		{"a pnpm lock file gives pnpm exec", []string{"pnpm-lock.yaml"}, []string{"/usr/bin/pnpm", "exec"}},
		{"a yarn lock file gives yarn", []string{"yarn.lock"}, []string{"/usr/bin/yarn"}},
		{"a bun lock file gives bunx", []string{"bun.lock"}, []string{"/usr/bin/bunx", "--no-install"}},
		{"a binary bun lock file gives bunx", []string{"bun.lockb"}, []string{"/usr/bin/bunx", "--no-install"}},
		{"a project without a lock file gives npx", nil, []string{"/usr/bin/npx", "--no-install"}},
		{"the lock file of the package manager in use wins over an older one", []string{"package-lock.json", "pnpm-lock.yaml"}, []string{"/usr/bin/pnpm", "exec"}},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-1011~1]", func(t *testing.T) {
			got, err := resolve(lockfileWorkspace(t, c.files...), dependencies(nil, nil, found))
			if err != nil || !reflect.DeepEqual(got.Command, c.expected) {
				t.Fatalf("%v %v", got, err)
			}
		})
	}

	t.Run("a configured runner wins over the lock file [verifies SP-1011~1]", func(t *testing.T) {
		deps := dependencies(nil, nil, found)
		deps.PackageRunner = []string{"corepack", "pnpm", "exec"}
		got, err := resolve(lockfileWorkspace(t, "yarn.lock"), deps)
		if err != nil || !reflect.DeepEqual(got.Command, []string{"corepack", "pnpm", "exec"}) {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("a missing package runner is an error [verifies SP-1011~1]", func(t *testing.T) {
		if _, err := resolve(lockfileWorkspace(t, "yarn.lock"), dependencies(nil, nil, notFound)); !errors.Is(err, node.ErrNoToolchain) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestBuildCheck(t *testing.T) {
	ctx := context.Background()
	toolchain := ecosystem.Toolchain{Command: []string{"npx", "--no-install"}}
	check := func(process ports.ProcessRunner, workspace ecosystem.Workspace, lookPath func(string) (string, error), changed ...string) (ecosystem.BuildResult, error) {
		return node.NewVitest(dependencies(process, nil, lookPath)).BuildCheck().Check(ctx, workspace, toolchain, changed, ecosystem.BuildBaseline{})
	}

	t.Run("a changed TypeScript file is type-checked with the project's tsconfig [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{}
		workspace := lockfileWorkspace(t, "tsconfig.json")
		result, err := check(process, workspace, found, "src/rates.ts")
		if err != nil || !result.OK || len(process.commands) != 1 {
			t.Fatalf("%+v %v %v", result, err, process.commands)
		}
		if want := []string{"npx", "--no-install", "tsc", "--noEmit", "-p", "tsconfig.json"}; !reflect.DeepEqual(process.commands[0].Argv, want) || process.commands[0].Dir != workspace.Dir {
			t.Fatalf("command = %+v", process.commands[0])
		}
	})

	t.Run("a type error is a failed build with the compiler's message [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{exit: func(ports.Command) int { return 2 }}
		result, err := check(process, lockfileWorkspace(t, "tsconfig.json"), found, "src/rates.ts")
		if err != nil || result.OK || !strings.Contains(result.Diagnostics, "TS2322") {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("each changed JavaScript file is syntax-checked with node [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{}
		result, err := check(process, lockfileWorkspace(t), found, "src/a.js", "src/b.mjs", "lib/c.cjs", "README.md")
		if err != nil || !result.OK || len(process.commands) != 3 {
			t.Fatalf("%+v %v %v", result, err, process.commands)
		}
		if want := []string{"/usr/bin/node", "--check", "src/b.mjs"}; !reflect.DeepEqual(process.commands[1].Argv, want) {
			t.Fatalf("argv = %v", process.commands[1].Argv)
		}
	})

	t.Run("a syntax error stops the check at the first failing file [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{exit: func(ports.Command) int { return 1 }}
		result, err := check(process, lockfileWorkspace(t), found, "src/a.js", "src/b.js")
		if err != nil || result.OK || len(process.commands) != 1 {
			t.Fatalf("%+v %v %v", result, err, process.commands)
		}
	})

	t.Run("a TypeScript project without a tsconfig has nothing to check but its JavaScript [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{}
		result, err := check(process, lockfileWorkspace(t), found, "src/rates.ts")
		if err != nil || !result.OK || len(process.commands) != 0 {
			t.Fatalf("%+v %v %v", result, err, process.commands)
		}
	})

	t.Run("a file nothing checks means nothing to check [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{}
		result, err := check(process, lockfileWorkspace(t, "tsconfig.json"), found, "notes.md")
		if err != nil || !result.OK || len(process.commands) != 0 {
			t.Fatalf("%+v %v %v", result, err, process.commands)
		}
	})

	t.Run("a timed out check is a failed build [verifies SP-1012~1]", func(t *testing.T) {
		process := &recorder{timedOut: true}
		result, err := check(process, lockfileWorkspace(t, "tsconfig.json"), found, "src/rates.ts")
		if err != nil || result.OK {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("a missing node is an error, not a failed build [verifies SP-1012~1]", func(t *testing.T) {
		if _, err := check(&recorder{}, lockfileWorkspace(t), notFound, "src/a.js"); err == nil {
			t.Fatal("no error")
		}
	})
}

func TestLayout(t *testing.T) {
	layout := jestStrategy(nil, nil).Layout()
	t.Run("coverage reports are excluded and node_modules is a dependency directory [verifies SP-1016~1]", func(t *testing.T) {
		for _, excluded := range []string{"coverage", ".nyc_output"} {
			if !contains(layout.ExcludedDirs, excluded) {
				t.Errorf("%s is not excluded: %v", excluded, layout.ExcludedDirs)
			}
		}
		if !reflect.DeepEqual(layout.DependencyDirs, []string{"node_modules"}) {
			t.Fatalf("dependencies = %v", layout.DependencyDirs)
		}
	})

	t.Run("tests, their setup and the tooling configuration are not source [verifies SP-1016~1]", func(t *testing.T) {
		for _, file := range []string{"test/rates.test.ts", "src/rates.spec.tsx", "jest.config.js", "vitest.config.ts", "vite.config.mts", "jest.setup.ts", "setupTests.ts", "types/globals.d.ts"} {
			if !layout.IsTestFile(file) {
				t.Errorf("%s is not a test file", file)
			}
		}
		for _, file := range []string{"src/rates.ts", "src/index.js", "lib/clamp.mjs"} {
			if layout.IsTestFile(file) {
				t.Errorf("%s is a test file", file)
			}
		}
	})
}

func contains(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}
