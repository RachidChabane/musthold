package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

// The separators of a probe target and of a qualified name.
const (
	probeTargetSeparator = ":"
	qualnameDot          = "."
	v8CoverageExtension  = ".json"
)

// v8CoverageFile is a file Node writes into NODE_V8_COVERAGE: the scripts of a
// process and the counts of their functions.
type v8CoverageFile struct {
	Result []struct {
		URL       string `json:"url"`
		Functions []struct {
			Ranges []struct {
				Count int `json:"count"`
			} `json:"ranges"`
		} `json:"functions"`
	} `json:"result"`
}

// linesThroughNode records a test alone once through NODE_V8_COVERAGE: its
// lines are every line of each project file a function of which counted.
// Without positions mapped to the sources, the test owns nothing.
func (s scanSuite) linesThroughNode(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest, test ecosystem.TestID, recorder coverageRecorder) ([]vacuity.Line, error) {
	ran, err := s.runProcess(ctx, workspace, toolchain, scanProcess{tests: []ecosystem.TestID{test}, coverage: true, v8Coverage: true, timeout: request.Timeout})
	if err != nil {
		return nil, err
	}
	if ran.timedOut {
		return nil, fmt.Errorf("%w: %s ran past %s", ports.ErrCoverageTimedOut, test, request.Timeout)
	}
	set, err := wholeFilesRun(ran.v8Dir, recorder)
	if err != nil {
		return nil, err
	}
	return sortedLines(set, recorder.source), nil
}

// togetherThroughNode records the tests in one process through
// NODE_V8_COVERAGE: each gets every line of each project file a function of
// which counted in the process.
func (s scanSuite) togetherThroughNode(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest, tests []ports.ScanCase, recorder coverageRecorder) (map[ecosystem.TestID]vacuity.Coverage, error) {
	ran, err := s.runProcess(ctx, workspace, toolchain, scanProcess{tests: idsOfCases(tests), coverage: true, v8Coverage: true, timeout: request.Timeout})
	if err != nil {
		return nil, err
	}
	if ran.timedOut {
		return nil, fmt.Errorf("%w: the recording of %d tests ran past %s", ports.ErrCoverageTimedOut, len(tests), request.Timeout)
	}
	set, err := wholeFilesRun(ran.v8Dir, recorder)
	if err != nil {
		return nil, err
	}
	lines := sortedLines(set, recorder.source)
	covered := make(map[ecosystem.TestID]vacuity.Coverage, len(tests))
	for _, test := range tests {
		covered[test.ID] = vacuity.Coverage{Lines: lines}
	}
	return covered, nil
}

// wholeFilesRun reads the files Node wrote into a NODE_V8_COVERAGE directory:
// every line of each project file whose script had a function that counted.
func wholeFilesRun(dir string, recorder coverageRecorder) (map[vacuity.Line]bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[vacuity.Line]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the Node coverage of %s: %w", dir, err)
	}
	set := map[vacuity.Line]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), v8CoverageExtension) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read the Node coverage %s: %w", entry.Name(), err)
		}
		var written v8CoverageFile
		if json.Unmarshal(content, &written) != nil {
			continue
		}
		for _, script := range written.Result {
			if !anyCounted(script.Functions) {
				continue
			}
			file, inside := recorder.files.absolute(script.URL)
			if !inside {
				continue
			}
			source, read := recorder.files.read(file)
			if !read {
				continue
			}
			for line := range bytes.Count(source, []byte{lineFeed}) + 1 {
				set[vacuity.Line{File: file, Line: line + firstLineNumber}] = true
			}
		}
	}
	return set, nil
}

func anyCounted(functions []struct {
	Ranges []struct {
		Count int `json:"count"`
	} `json:"ranges"`
}) bool {
	for _, function := range functions {
		if len(function.Ranges) > 0 && function.Ranges[0].Count > 0 {
			return true
		}
	}
	return false
}
