package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/config"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/git"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/localcache"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/operations"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/pendingfile"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/process"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/proofcache"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/proofsource"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/schemacheck"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/shallnot"
	golangunits "github.com/RachidChabane/musthold/completeness/internal/adapters/units/golang"
	javaunits "github.com/RachidChabane/musthold/completeness/internal/adapters/units/java"
	javascriptunits "github.com/RachidChabane/musthold/completeness/internal/adapters/units/javascript"
	kotlinunits "github.com/RachidChabane/musthold/completeness/internal/adapters/units/kotlin"
	pythonunits "github.com/RachidChabane/musthold/completeness/internal/adapters/units/python"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/xdg"
	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/app/prove"
	"github.com/RachidChabane/musthold/completeness/internal/domain/policy"
	"github.com/RachidChabane/musthold/completeness/internal/version"
	"github.com/RachidChabane/musthold/format"
	sharedinstall "github.com/RachidChabane/musthold/format/hostproto/hostinstall"
)

// Identity of the module in the Statements and the requests it emits.
const (
	moduleName = "musthold-completeness"
	pluginName = "musthold-completeness"
	// pendingModule names the module in its pending file.
	pendingModule = "completeness"
	// stateDirectory holds the project's musthold state, and the module's
	// configuration file.
	stateDirectory = sharedinstall.StateDirectory
	// sharedConfigFile is the configuration file the modules share, in stateDirectory.
	sharedConfigFile = sharedinstall.SharedConfigFile
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

// policyOf reads `.musthold/completeness.yaml`; a project without one has the
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
	return []ports.Indexer{pythonunits.New(), golangunits.New(), javascriptunits.New(), javaunits.New(), kotlinunits.New()}
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
