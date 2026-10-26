package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/platform/atomicfile"
)

// ErrScanConfig is returned when the runner's configuration cannot be read
// for a scan.
var ErrScanConfig = errors.New("the test runner's configuration cannot be read for a scan")

// The shipped scripts a scan's runs load, and the variables they read.
const (
	scanEnvironmentScript     = "scan-jest-environment.cjs"
	scanReporterScript        = "scan-jest-reporter.cjs"
	scanSequencerScript       = "scan-jest-sequencer.cjs"
	scanJestGlobalSetupScript = "scan-global-setup.cjs"
	scanVitestOverlay         = "musthold-scan.vitest.config.mjs"
	scanScriptsVariable       = "MUSTHOLD_SCAN_SCRIPTS"
	scanEnvironmentsVariable  = "MUSTHOLD_SCAN_JEST_ENVIRONMENTS"
	scanGlobalSetupVariable   = "MUSTHOLD_SCAN_GLOBAL_SETUP"
	jestShowConfigFlag        = "--showConfig"
	jestForceExitFlag         = "--forceExit"
	jestNoCoverageFlag        = "--coverage=false"
	jestNoBailFlag            = "--bail=0"
	jestEnvironmentFlag       = "--testEnvironment="
	jestReportersFlag         = "--reporters="
	jestSequencerFlag         = "--testSequencer="
	jestGlobalSetupFlag       = "--globalSetup="
	vitestConfigFlag          = "--config"
	environmentKeySeparator   = "\x00"
	overlayFileMode           = 0o644
	configDigestLength        = 16
	ciVariable                = "CI=true"
	environmentAssignment     = "="
	reproPackageRunner        = "npx"
	reproNoInstallFlag        = "--no-install"
	anchoredTitle             = "^%s$"
)

// vitestConfigFiles are the configuration files Vitest reads, in the order it
// looks for them.
var vitestConfigFiles = []string{
	"vitest.config.ts", "vitest.config.mts", "vitest.config.cts", "vitest.config.js", "vitest.config.mjs", "vitest.config.cjs",
	"vite.config.ts", "vite.config.mts", "vite.config.cts", "vite.config.js", "vite.config.mjs", "vite.config.cjs",
}

// scanFramework is what differs between Jest and Vitest when a scan runs them.
type scanFramework interface {
	// scanCommand is the command line, after the package runner, and the
	// environment of one run of the given test files, every file for none,
	// under the shipped scripts in the directory.
	scanCommand(ctx context.Context, suite scanSuite, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, scripts string, files []string) ([]string, []string, error)
	// caseInsensitiveTitles is whether the runner's title filter ignores case,
	// so that two ids equal ignoring case cannot be run apart.
	caseInsensitiveTitles() bool
	// reproArguments run one test of a file whose full title the anchored
	// pattern matches, after the package runner.
	reproArguments(file, pattern string) []string
}

// scanCache keeps what a scan reads once of a configuration, by its digest.
type scanCache struct {
	mutex sync.Mutex
	jest  map[string]jestProjects
}

func newScanCache() *scanCache { return &scanCache{jest: map[string]jestProjects{}} }

// jestProjects is what a scan's Jest runs need of the projects of a
// configuration: the environment each project configures, by its root
// relative to the workspace and its display name, and the global setup.
type jestProjects struct {
	environments map[string]string
	// globalSetup is the project's global setup, relative to the workspace
	// when it lies in it; empty for none.
	globalSetup string
}

type jestShownConfig struct {
	Configs []struct {
		RootDir     string `json:"rootDir"`
		DisplayName *struct {
			Name string `json:"name"`
		} `json:"displayName"`
		TestEnvironment string `json:"testEnvironment"`
	} `json:"configs"`
	GlobalConfig struct {
		GlobalSetup *string `json:"globalSetup"`
	} `json:"globalConfig"`
}

func (jest) caseInsensitiveTitles() bool { return true }

func (jest) reproArguments(file, pattern string) []string {
	return []string{jestTool, jestCIFlag, jestSingleProcessFlag, jestByPathFlag, file, jestTitleFlag, pattern}
}

// scanCommand runs Jest in band, under the shipped environment, reporter and
// test sequencer, with coverage off, no bail and a forced exit, each test file
// in path order; the environment each project configures is read once from
// `jest --showConfig`.
func (jest) scanCommand(ctx context.Context, suite scanSuite, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, scripts string, files []string) ([]string, []string, error) {
	projects, err := suite.jestProjects(ctx, workspace, toolchain)
	if err != nil {
		return nil, nil, err
	}
	environments, err := json.Marshal(projects.environments)
	if err != nil {
		return nil, nil, fmt.Errorf("encode the Jest environments: %w", err)
	}
	argv := []string{
		jestTool, jestCIFlag, jestSingleProcessFlag, jestNoWatchmanFlag, jestForceExitFlag, jestNoCoverageFlag, jestNoBailFlag,
		jestEnvironmentFlag + filepath.Join(scripts, scanEnvironmentScript),
		jestReportersFlag + filepath.Join(scripts, scanReporterScript),
		jestSequencerFlag + filepath.Join(scripts, scanSequencerScript),
	}
	env := []string{scanEnvironmentsVariable + environmentAssignment + string(environments)}
	if projects.globalSetup != "" {
		argv = append(argv, jestGlobalSetupFlag+filepath.Join(scripts, scanJestGlobalSetupScript))
		env = append(env, scanGlobalSetupVariable+environmentAssignment+inWorkspace(workspace, projects.globalSetup))
	}
	if len(files) > 0 {
		argv = append(append(argv, jestByPathFlag), files...)
	}
	return argv, env, nil
}

// jestProjects reads the projects of the workspace's Jest configuration once
// per configuration digest.
func (s scanSuite) jestProjects(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain) (jestProjects, error) {
	digest := configDigest(workspace.Dir)
	s.cache.mutex.Lock()
	known, found := s.cache.jest[digest]
	s.cache.mutex.Unlock()
	if found {
		return known, nil
	}
	argv := append(append([]string{}, toolchain.Command...), jestTool, jestShowConfigFlag, jestCIFlag, jestNoWatchmanFlag)
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Env: append(quietEnvironment(), ciVariable), Timeout: s.deps.Timeout})
	if err != nil {
		return jestProjects{}, fmt.Errorf("read the Jest configuration: %w", err)
	}
	var shown jestShownConfig
	if executed.ExitCode != 0 || json.Unmarshal(executed.Stdout, &shown) != nil || len(shown.Configs) == 0 {
		return jestProjects{}, fmt.Errorf("%w: jest --showConfig exited %d: %s", ErrScanConfig, executed.ExitCode, strings.TrimSpace(string(executed.Stderr)))
	}
	projects := jestProjects{environments: map[string]string{}}
	for _, config := range shown.Configs {
		name := ""
		if config.DisplayName != nil {
			name = config.DisplayName.Name
		}
		projects.environments[relativeTo(workspace.Dir, config.RootDir)+environmentKeySeparator+name] = relativeTo(workspace.Dir, config.TestEnvironment)
	}
	if setup := shown.GlobalConfig.GlobalSetup; setup != nil && *setup != "" {
		projects.globalSetup = relativeTo(workspace.Dir, *setup)
	}
	s.cache.mutex.Lock()
	s.cache.jest[digest] = projects
	s.cache.mutex.Unlock()
	return projects, nil
}

// configDigest identifies the configuration of a workspace: its manifest and
// the files at its root that configure a runner.
func configDigest(dir string) string {
	hash := sha256.New()
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (name != packageFile && !strings.HasPrefix(name, jestConfigPrefix) && !strings.HasPrefix(name, vitestConfigPrefix)) {
			continue
		}
		content, _ := os.ReadFile(filepath.Join(dir, name))
		hash.Write([]byte(name + environmentKeySeparator))
		hash.Write(content)
	}
	return hex.EncodeToString(hash.Sum(nil))[:configDigestLength]
}

// relativeTo makes a path inside the workspace relative to it, resolving
// symbolic links on both sides; a path outside it stays as it is.
func relativeTo(dir, file string) string {
	for _, root := range rootsOf(dir) {
		resolved := file
		if real, err := filepath.EvalSymlinks(file); err == nil {
			resolved = real
		}
		for _, candidate := range []string{file, resolved} {
			if relative, err := filepath.Rel(root, candidate); err == nil && relative != parentDirectory && !strings.HasPrefix(relative, parentDirectory+string(filepath.Separator)) {
				return filepath.ToSlash(relative)
			}
		}
	}
	return file
}

// inWorkspace is the path in this workspace of a path relativeTo kept.
func inWorkspace(workspace ecosystem.Workspace, file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(workspace.Dir, filepath.FromSlash(file))
}

// rootsOf is the directory as given and with its symbolic links resolved.
func rootsOf(dir string) []string {
	roots := []string{dir}
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		roots = append(roots, real)
	}
	return roots
}

func (vitest) caseInsensitiveTitles() bool { return false }

func (vitest) reproArguments(file, pattern string) []string {
	return []string{vitestTool, vitestRunCommand, file, vitestTitleFlag, pattern}
}

// scanCommand runs Vitest under the shipped overlay of the project's
// configuration, written once into the workspace: the shipped runner in one
// fork with no file parallelism, the shipped global setup, no retry, no bail,
// coverage off and the test files in path order.
func (vitest) scanCommand(_ context.Context, _ scanSuite, workspace ecosystem.Workspace, _ ecosystem.Toolchain, scripts string, files []string) ([]string, []string, error) {
	overlay, err := vitestOverlay(workspace.Dir)
	if err != nil {
		return nil, nil, err
	}
	if err := atomicfile.Write(filepath.Join(workspace.Dir, scanVitestOverlay), overlay, overlayFileMode); err != nil {
		return nil, nil, fmt.Errorf("write the scan's Vitest configuration: %w", err)
	}
	argv := append([]string{vitestTool, vitestRunCommand, vitestConfigFlag, scanVitestOverlay}, files...)
	return argv, []string{scanScriptsVariable + environmentAssignment + scripts}, nil
}

// vitestOverlay is the configuration a scan runs Vitest with: the project's
// own, when it has one, merged with the scan's settings.
func vitestOverlay(dir string) ([]byte, error) {
	base := ""
	for _, name := range vitestConfigFiles {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			base = name
			break
		}
	}
	importLine := "const base = {};"
	if base != "" {
		quoted, err := json.Marshal("./" + base)
		if err != nil {
			return nil, err
		}
		importLine = "import base from " + string(quoted) + ";"
	}
	return []byte(strings.Replace(vitestOverlaySource, overlayBasePlaceholder, importLine, 1)), nil
}

// reproPattern is the anchored title pattern of a test id, matching titles
// joined by the separator or a space, which Jest's and Vitest's filters
// both accept.
func reproPattern(test ecosystem.TestID) string {
	return fmt.Sprintf(anchoredTitle, quoteTitle(strings.Join(titlesOfTest(test), titleSeparator), titleSeparator))
}
