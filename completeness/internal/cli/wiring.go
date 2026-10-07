package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/config"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/git"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/localcache"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/operations"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/pendingfile"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/proofcache"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/proofsource"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/shallnot"
	golangunits "github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/golang"
	pythonunits "github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/python"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/completeness/internal/version"
	"github.com/RachidChabane/spec-proof/format"
)

// Identity of the module in the Statements and the requests it emits.
const (
	moduleName = "spec-proof-completeness"
	pluginName = "spec-proof-completeness"
	// pendingModule names the module in its pending file.
	pendingModule = "completeness"
	// stateDirectory holds the project's spec-proof state, and the module's
	// configuration file.
	stateDirectory = ".spec-proof"
)

// rootOf resolves the project root: the directory given, or the working directory.
func (e Env) rootOf(given string) (string, error) {
	if given == "" {
		given = e.WorkDir
	}
	root, err := filepath.Abs(given)
	if err != nil {
		return "", fmt.Errorf("resolve the project root: %w", err)
	}
	return root, nil
}

func (e Env) hostStore(root string) (*hoststore.Store, error) {
	return hoststore.Open(xdg.HostDir(e.lookupEnv, e.HomeDir, root))
}

// policyOf reads `.spec-proof/completeness.yaml`; a project without one has the
// default policy.
func policyOf(root string) (policy.Policy, error) {
	data, err := os.ReadFile(filepath.Join(root, stateDirectory, config.FileName))
	if os.IsNotExist(err) {
		return policy.Default(), nil
	}
	if err != nil {
		return policy.Policy{}, fmt.Errorf("read the configuration: %w", err)
	}
	return config.Decode(data)
}

// project is everything `prove` and `doctor` wire together for one project root.
type project struct {
	root     string
	policy   policy.Policy
	store    *hoststore.Store
	deps     prove.Deps
	settings prove.Settings
}

// indexers lists the strategies that read the units of a source file, one per
// ecosystem.
func indexers() []ports.Indexer {
	return []ports.Indexer{pythonunits.New(), golangunits.New()}
}

// openProject wires the adapters of a project root.
func (e Env) openProject(rootFlag string) (*project, error) {
	root, err := e.rootOf(rootFlag)
	if err != nil {
		return nil, err
	}
	decision, err := policyOf(root)
	if err != nil {
		return nil, err
	}
	store, err := e.hostStore(root)
	if err != nil {
		return nil, err
	}
	checker, err := schemacheck.New()
	if err != nil {
		return nil, err
	}
	bound, err := operations.Load(root)
	if err != nil {
		return nil, err
	}
	runner := process.New()
	cacheDir := xdg.CacheDir(e.lookupEnv, e.HomeDir, root)
	return &project{root: root, policy: decision, store: store,
		settings: prove.Settings{Policy: decision, Module: format.Module{Name: moduleName, Version: version.Version}, Plugin: pluginName, PendingModule: pendingModule},
		deps: prove.Deps{
			Criteria:   &shallnot.Gateway{Process: runner, Binary: e.lookup(envShallnot), Root: root},
			Indexers:   indexers(),
			Changes:    git.Source{Process: runner, Root: root},
			Operations: bound,
			Project:    os.DirFS(root),
			Store:      store,
			Proofs:     proofcache.New(root, checker),
			Evidence:   proofsource.Source{Root: root},
			Pending:    pendingfile.Directory{Root: root},
			Memo:       localcache.NewMemo(cacheDir),
			Statements: checker,
			Clock:      e.Now,
			Entropy:    e.Entropy,
		}}, nil
}
