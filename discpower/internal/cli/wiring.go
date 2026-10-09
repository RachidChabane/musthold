package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/config"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/golang"
	jvmecosystem "github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/localcache"
	gomutation "github.com/RachidChabane/spec-proof/discpower/internal/adapters/mutation/golang"
	javascriptmutation "github.com/RachidChabane/spec-proof/discpower/internal/adapters/mutation/javascript"
	jvmmutation "github.com/RachidChabane/spec-proof/discpower/internal/adapters/mutation/jvm"
	pythonmutation "github.com/RachidChabane/spec-proof/discpower/internal/adapters/mutation/python"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/proofcache"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/shallnot"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/prove"
	domain "github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/discpower/internal/version"
	"github.com/RachidChabane/spec-proof/format"
)

// Identity of the module in the Statements and the requests it emits.
const (
	moduleName = "spec-proof-dp"
	pluginName = hostharness.PluginName
)

// ErrNoEcosystem is returned when no registered ecosystem recognizes the project.
var ErrNoEcosystem = errors.New("no supported test ecosystem recognized in the project")

// rootOf resolves the project root: the directory given, or the working directory.
func (e Env) rootOf(given string) (string, error) {
	if given == "" {
		given = e.WorkDir
	}
	root, err := filepath.Abs(given)
	if err != nil {
		return "", fmt.Errorf("resolve the project root: %w", err)
	}
	// The host directory is addressed by the root's path, so every process that
	// serves one project must spell it the same way: symbolic links are resolved.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve the project root: %w", err)
	}
	return resolved, nil
}

func (e Env) hostStore(root string) (*hoststore.Store, error) {
	return hoststore.Open(xdg.HostDir(e.lookupEnv, e.HomeDir, root))
}

// settingsOf reads `.spec-proof/config.yaml`; a project without one has the
// default settings.
func settingsOf(root string) (projectconfig.Settings, error) {
	data, err := os.ReadFile(filepath.Join(root, projectconfig.Directory, projectconfig.FileName))
	if errors.Is(err, os.ErrNotExist) {
		return projectconfig.Settings{Proofs: map[format.Kind]bool{}}, nil
	}
	if err != nil {
		return projectconfig.Settings{}, fmt.Errorf("read the configuration: %w", err)
	}
	return config.Decode(data)
}

// project is everything `prove` and `doctor` wire together for one project root.
type project struct {
	root       string
	settings   projectconfig.Settings
	name       domain.Name
	strategy   ports.Ecosystem
	engines    *operatorgen.Registry
	store      *hoststore.Store
	workspaces *workspace.Manager
	deps       prove.Deps
	// classifyFirst is set when the first vote of a closed request is left to a
	// classification before the host votes are asked.
	classifyFirst bool
}

func (p *project) close() {
	if p.workspaces != nil {
		_ = p.workspaces.Close()
	}
}

// openProject wires the adapters of a project root.
func (e Env) openProject(rootFlag string) (*project, error) {
	root, err := e.rootOf(rootFlag)
	if err != nil {
		return nil, err
	}
	settings, err := settingsOf(root)
	if err != nil {
		return nil, err
	}
	runner, reader := process.New(), junitxml.NewReader()
	strategies, err := newStrategies(runner, reader)
	if err != nil {
		return nil, err
	}
	name, strategy, err := chooseEcosystem(strategies, settings, root)
	if err != nil {
		return nil, err
	}
	engines, err := newEngines()
	if err != nil {
		return nil, err
	}
	store, err := e.hostStore(root)
	if err != nil {
		return nil, err
	}
	workspaces, err := workspace.New(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("create the workspace manager: %w", err)
	}
	checker, err := schemacheck.New()
	if err != nil {
		return nil, err
	}
	cacheDir := xdg.CacheDir(e.lookupEnv, e.HomeDir, root)
	classifyFirst, err := e.classifiesFirst(root)
	if err != nil {
		return nil, err
	}
	return &project{classifyFirst: classifyFirst, root: root, settings: settings, name: name, strategy: strategy, engines: engines, store: store, workspaces: workspaces, deps: prove.Deps{
		Criteria:   &shallnot.Gateway{Process: runner, Binary: e.lookup(envShallnot), Root: root},
		Strategy:   strategy,
		Workspaces: workspaces,
		Engines:    engines,
		Project:    os.DirFS(root),
		Store:      store,
		Proofs:     proofcache.New(root, checker),
		Vault:      localcache.NewVault(cacheDir),
		Memo:       localcache.NewMemo(cacheDir),
		Statements: checker,
		Clock:      e.Now,
		Entropy:    e.Entropy,
	}}, nil
}

// newStrategies registers the strategy of every supported ecosystem.
func newStrategies(runner ports.ProcessRunner, reader ports.ResultsReader) (*ecosystem.Registry, error) {
	return ecosystem.New(
		pytest.New(pytest.Dependencies{Process: runner, Reader: reader, Locator: treesitter.NewLocator(), Assignments: pythonmutation.New()}),
		golang.New(golang.Dependencies{Process: runner, Reader: reader, Locator: goparser.NewLocator(), Assignments: gomutation.New()}),
		node.NewJest(nodeDependencies(runner, reader)),
		node.NewVitest(nodeDependencies(runner, reader)),
		jvmecosystem.NewMaven(jvmDependencies(runner, reader)),
		jvmecosystem.NewGradle(jvmDependencies(runner, reader)),
	)
}

func nodeDependencies(runner ports.ProcessRunner, reader ports.ResultsReader) node.Dependencies {
	return node.Dependencies{Process: runner, Reader: reader, Locator: treesitter.NewLocator(), Assignments: javascriptmutation.New()}
}

// jvmDependencies are the ports the Maven and Gradle strategies share.
func jvmDependencies(runner ports.ProcessRunner, reader ports.ResultsReader) jvmecosystem.Dependencies {
	return jvmecosystem.Dependencies{Process: runner, Reader: reader, Locator: treesitter.NewLocator(), Assignments: jvmmutation.New()}
}

func chooseEcosystem(strategies *ecosystem.Registry, settings projectconfig.Settings, root string) (domain.Name, ports.Ecosystem, error) {
	detected := strategies.Detect(os.DirFS(root))
	for _, candidate := range detected {
		if len(settings.Ecosystems) == 0 || containsEcosystem(settings.Ecosystems, candidate.Detection.Name) {
			return candidate.Detection.Name, candidate.Strategy, nil
		}
	}
	return "", nil, ErrNoEcosystem
}

func containsEcosystem(names []domain.Name, name domain.Name) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

// newEngines registers the mutation engine of every supported language.
func newEngines() (*operatorgen.Registry, error) {
	registry := operatorgen.NewRegistry()
	if err := registry.Register(pythonmutation.New(), treesitter.NewLocator()); err != nil {
		return nil, err
	}
	if err := registry.Register(gomutation.New(), goparser.NewLocator()); err != nil {
		return nil, err
	}
	if err := registry.Register(javascriptmutation.New(), treesitter.NewLocator()); err != nil {
		return nil, err
	}
	if err := registry.Register(jvmmutation.New(), treesitter.NewLocator()); err != nil {
		return nil, err
	}
	return registry, nil
}

// proveSettings assembles the settings of a run from the project's configuration.
func (p *project) proveSettings() prove.Settings {
	return prove.Settings{
		Root: p.root, Ecosystem: p.name, Policy: p.settings.Policy(),
		Operations: execution.Operations(p.settings.OperationsFor(p.name)),
		Seed:       uint64(operator.LocalSeed),
		Module:     format.Module{Name: moduleName, Version: version.Version},
		Plugin:     pluginName,

		ClassifyFirst: p.classifyFirst,
	}
}
