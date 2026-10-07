package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/config"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/localcache"
	golangmask "github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/golang"
	pythonmask "github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/python"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/pendingfile"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/proofcache"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/proofsource"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/shallnot"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/prove"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/alignment/internal/version"
	"github.com/RachidChabane/spec-proof/format"
	sharedinstall "github.com/RachidChabane/spec-proof/format/hostproto/hostinstall"
)

// Identity of the module in the Statements and the requests it emits.
const (
	moduleName = "spec-proof-alignment"
	pluginName = "spec-proof-alignment"
	// pendingModule names the module in its pending file.
	pendingModule = "alignment"
	// stateDirectory holds the project's spec-proof state, and the module's
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

// policyOf reads `.spec-proof/alignment.yaml`; a project without one has the
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

// maskers lists the strategies that build the masked input of a test file, one
// per ecosystem.
func maskers() []ports.Masker {
	return []ports.Masker{pythonmask.New(), golangmask.New()}
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
	cacheDir := xdg.CacheDir(e.lookupEnv, e.HomeDir, root)
	return &project{root: root, policy: decision, store: store,
		settings: prove.Settings{Policy: decision, Module: format.Module{Name: moduleName, Version: version.Version}, Plugin: pluginName, PendingModule: pendingModule},
		deps: prove.Deps{
			Criteria:   &shallnot.Gateway{Process: process.New(), Binary: e.lookup(envShallnot), Root: root},
			Maskers:    maskers(),
			Project:    os.DirFS(root),
			Store:      store,
			Proofs:     proofcache.New(root, checker),
			Power:      proofsource.Source{Root: root},
			Pending:    pendingfile.Directory{Root: root},
			Memo:       localcache.NewMemo(cacheDir),
			Statements: checker,
			Clock:      e.Now,
			Entropy:    e.Entropy,
		}}, nil
}
