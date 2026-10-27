package oracle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The environment of every command the verb runs in the box. The container runs
// the command under `env -i` with this environment alone.
var boxEnv = []string{
	box.SystemPath,
	"HOME=/tmp",
	"PYTHONDONTWRITEBYTECODE=1",
}

// The names of the report files the pytest runs write.
const (
	oracleReportPattern  = "oracle-%d.xml"
	heldOutReportPattern = "heldout-%d.xml"
)

// oracleArgv is the command of PREREG §4.1 for the report file out.
func oracleArgv(out string) []string {
	return []string{
		"python", "-m", "pytest",
		"-c", "oracle/pytest.ini", "--rootdir", "oracle", "--noconftest",
		"-p", "no:cacheprovider", "-q", "--junitxml=" + out, "oracle",
	}
}

// heldOutArgv runs the tests directory of a tree with the tree's pytest.ini; the
// directory's own conftest.py files are kept.
func heldOutArgv(out string) []string {
	return []string{
		"python", "-m", "pytest",
		"-c", pytestIniFile, "--rootdir", ".",
		"-p", "no:cacheprovider", "-q", "--junitxml=" + out, testsDirName,
	}
}

// pytestRunner runs pytest commands through the box.
type pytestRunner struct {
	runner box.Runner
	// mountRoot is the directory the container may write; it holds every tree and
	// report file.
	mountRoot string
}

// runThrice runs the command three times in dir, the k-th run writing its report
// to the file named by reportPattern in outDir.
func (p pytestRunner) runThrice(ctx context.Context, dir, outDir, reportPattern string, argv func(out string) []string) ([study.OracleRuns]RunReport, error) {
	var reports [study.OracleRuns]RunReport
	if err := os.MkdirAll(outDir, dirMode); err != nil {
		return reports, err
	}
	for k := range reports {
		out := filepath.Join(outDir, fmt.Sprintf(reportPattern, k+1))
		report, err := p.runOnce(ctx, dir, out, argv(out))
		if err != nil {
			return reports, err
		}
		reports[k] = report
	}
	return reports, nil
}

func (p pytestRunner) runOnce(ctx context.Context, dir, out string, argv []string) (RunReport, error) {
	result, err := p.runner.Run(ctx, box.Command{
		Argv:   argv,
		Env:    boxEnv,
		Dir:    dir,
		Mounts: []box.Mount{{Path: p.mountRoot}},
		Limit:  study.OracleLimit,
	})
	if err != nil {
		return RunReport{}, fmt.Errorf("%w: %v", ErrBox, err)
	}
	junit, _ := os.ReadFile(out)
	return ClassifyRun(result, junit), nil
}
