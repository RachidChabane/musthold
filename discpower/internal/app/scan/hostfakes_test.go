package scan_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// The promises of the fake project and the lines its witnesses require.
const (
	readmeText = "# Invoicing\n\nA discount of zero percent leaves the total as it is.\nShipping is free from fifty.\nTotals are rounded down.\n"
	// quoteZero, quoteShipping and quoteRounded are quotes of the README.
	quoteZero     = "A discount of zero percent leaves the total as it is."
	quoteShipping = "Shipping is free from fifty."
	quoteRounded  = "Totals are rounded down."
	// quoteAbsent is no part of any source.
	quoteAbsent = "Refunds are instant."

	// keepsTotal is the line of discount a promise's witnesses check.
	keepsTotal = "        return total"
	// breaksTotal replaces it in the model mutant the fake host proposes: no
	// test of the suite checks the line, so the mutant passes the whole suite.
	breaksTotal = "        return -total"

	witnessFilePrefix = "tests/test_witness_"
	witnessImports    = "from invoice.totals import discount, shipping"
	pluginName        = "spec-proof-dp"
)

// witnessBlock is how the fake witness host writes one witness: its test
// name, the name the writer gave it and the source text it requires.
var witnessBlock = regexp.MustCompile(`(?m)^def (\S+)\(\):\n    # witness: (\S+)\n    # requires: (.*)$`)

// requirement is the source text a witness's code says it requires.
var requirement = regexp.MustCompile(`(?m)# requires: (.*)$`)

// fakeMemo is an execution memo in memory that can be lost.
type fakeMemo struct {
	mutex   sync.Mutex
	entries map[string][]byte
}

func newFakeMemo() *fakeMemo { return &fakeMemo{entries: map[string][]byte{}} }

func (m *fakeMemo) Get(key string) ([]byte, bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	value, found := m.entries[key]
	return value, found, nil
}

func (m *fakeMemo) Put(key string, value []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entries[key] = slices.Clone(value)
	return nil
}

// lose forgets every fact.
func (m *fakeMemo) lose() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.entries = map[string][]byte{}
}

// fakeSources keeps the saved sources in memory.
type fakeSources struct {
	mutex sync.Mutex
	saved map[string]map[string][]byte
}

func (s *fakeSources) Save(commit string, sources []scan.Source) ([]scan.SavedSource, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.saved == nil {
		s.saved = map[string]map[string][]byte{}
	}
	s.saved[commit] = map[string][]byte{}
	var saved []scan.SavedSource
	for _, source := range sources {
		s.saved[commit][source.Name] = slices.Clone(source.Content)
		saved = append(saved, scan.SavedSource{Name: source.Name, Digest: format.SHA256Hex(source.Content)})
	}
	return saved, nil
}

func (s *fakeSources) Load(commit, name string) ([]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	content, found := s.saved[commit][name]
	if !found {
		return nil, fs.ErrNotExist
	}
	return content, nil
}

// fakeHead reads the README of the head commit, or has none.
type fakeHead struct{ readme []byte }

func (h fakeHead) ReadAtHead(_ context.Context, path string) ([]byte, error) {
	if path != format.ScanSourceReadme || h.readme == nil {
		return nil, scan.ErrNotAtHead
	}
	return h.readme, nil
}

// fakePullRequests gives one pull request whose base is the fake base commit.
type fakePullRequests struct{ pull scan.PullRequest }

func (p fakePullRequests) Read(_ context.Context, number int) (scan.PullRequest, error) {
	pull := p.pull
	pull.Number, pull.ScanBase = number, baseCommit
	return pull, nil
}

// fakeWitnessHost briefs from the source files and lays a set out as one
// test file, each witness renamed after its short id.
type fakeWitnessHost struct {
	mutex  sync.Mutex
	briefs []string
}

func (h *fakeWitnessHost) Brief(_ fs.FS, sources []mutation.SourceFile, specText string) (ports.WitnessBrief, error) {
	h.mutex.Lock()
	h.briefs = append(h.briefs, specText)
	h.mutex.Unlock()
	var signatures []string
	for _, source := range sources {
		for _, line := range strings.Split(string(source.Content), "\n") {
			if strings.HasPrefix(line, "def ") {
				signatures = append(signatures, source.Path+": "+strings.TrimSuffix(line, ":"))
			}
		}
	}
	return ports.WitnessBrief{Language: "Python 3, pytest", Conventions: "a test function per witness", Interface: signatures, Helpers: nil}, nil
}

func (h *fakeWitnessHost) Layout(_ fs.FS, set witness.Set) (ports.WitnessFile, error) {
	file := witnessFilePrefix + set.Digest()[:8] + ".py"
	var content strings.Builder
	content.WriteString(set.Imports + "\n\n")
	tests := map[string]ecosystem.BoundTest{}
	for index, w := range set.Witnesses {
		name := "test_witness_" + w.ShortID()
		required := ""
		if found := requirement.FindStringSubmatch(w.Code); found != nil {
			required = found[1]
		}
		fmt.Fprintf(&content, "def %s():\n    # witness: %s\n    # requires: %s\n    assert True\n\n", name, w.Name, required)
		tests[w.ID] = ecosystem.NewBoundTest("tests.test_witness", name, file, index+1)
	}
	return ports.WitnessFile{Path: file, Content: []byte(content.String()), Tests: tests}, nil
}

// fakeSelector selects the bound tests by id.
type fakeSelector struct{}

func (fakeSelector) Select(bound []ecosystem.BoundTest, _ map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	selection := ecosystem.Selection{}
	for _, test := range bound {
		selection.Tests = append(selection.Tests, test.ID)
	}
	return selection, nil
}

// witnessRuns runs the witness files of a workspace: a witness passes when
// the source holds the text it requires. Its runs on the unmutated source are
// counted by witness name, and the fixture can make given runs fail.
type witnessRuns struct {
	workspaces *fakeWorkspaces
	mutex      sync.Mutex
	onSource   map[string]int
	// failRuns names, by witness name, the runs on the unmutated source that
	// fail, counted from 1 across invocations.
	failRuns map[string][]int
	// all counts every run of a witness file.
	all int
}

func (r *witnessRuns) Run(_ context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.all++
	source := r.workspaces.source(workspace)
	result := ecosystem.RunResult{Outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{}}
	for name, block := range r.blocks(workspace) {
		id := ecosystem.NewTestID("tests.test_witness", name)
		if !slices.Contains(selection.Tests, id) {
			continue
		}
		outcome := ecosystem.TestPassed
		if !strings.Contains(source, block.required) {
			outcome = ecosystem.TestFailed
		}
		if source == totalsSource {
			r.onSource[block.witness]++
			if slices.Contains(r.failRuns[block.witness], r.onSource[block.witness]) {
				outcome = ecosystem.TestFailed
			}
		}
		result.Outcomes[id] = outcome
	}
	return result, nil
}

// witnessTest is one witness test of a workspace.
type witnessTest struct {
	witness  string
	required string
}

// blocks reads the witness tests of the workspace's witness files, by test name.
func (r *witnessRuns) blocks(workspace ecosystem.Workspace) map[string]witnessTest {
	r.workspaces.mutex.Lock()
	defer r.workspaces.mutex.Unlock()
	found := map[string]witnessTest{}
	for path, content := range r.workspaces.files[workspace.Dir] {
		if !strings.HasPrefix(path, witnessFilePrefix) {
			continue
		}
		for _, match := range witnessBlock.FindAllStringSubmatch(string(content), -1) {
			found[match[1]] = witnessTest{witness: match[2], required: match[3]}
		}
	}
	return found
}

// hostEcosystem is the fake ecosystem with a runner and a selector of
// witness tests.
type hostEcosystem struct {
	fakeEcosystem
	runs *witnessRuns
}

func (e hostEcosystem) Runner() ports.SuiteRunner    { return e.runs }
func (e hostEcosystem) Selector() ports.TestSelector { return fakeSelector{} }
func (e hostEcosystem) BuildCheck() ports.BuildCheck { return fakeBuild(e.fakeEcosystem) }

// countingSuite counts every run and recording of the suite.
type countingSuite struct {
	*fakeSuite
	mutex sync.Mutex
	calls int
}

func (s *countingSuite) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanRunRequest) (ports.ScanRuns, error) {
	s.count()
	return s.fakeSuite.Run(ctx, workspace, toolchain, request)
}

func (s *countingSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error) {
	s.count()
	return s.fakeSuite.LineCoverage(ctx, workspace, toolchain, request)
}

func (s *countingSuite) count() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.calls++
}

// took reads and resets the count.
func (s *countingSuite) took() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	calls := s.calls
	s.calls = 0
	return calls
}

// hostFixture is a scan service over the fakes that asks a fake host.
type hostFixture struct {
	*fixture
	store hostproto.HostStore
	memo  *fakeMemo
	// diskMemo, when set, keeps the witness facts in place of memo.
	diskMemo  ports.ExecutionMemo
	sources   *fakeSources
	head      fakeHead
	pulls     *fakePullRequests
	host      *fakeHost
	witnesses *fakeWitnessHost
	runs      *witnessRuns
	suite     *countingSuite
}

func newHostFixture() *hostFixture {
	base := newFixture()
	f := &hostFixture{
		fixture: base, store: hostprototest.NewMemoryHostStore(), memo: newFakeMemo(), sources: &fakeSources{},
		head: fakeHead{readme: []byte(readmeText)}, witnesses: &fakeWitnessHost{},
		runs:  &witnessRuns{workspaces: base.workspaces, onSource: map[string]int{}, failRuns: map[string][]int{}},
		suite: &countingSuite{fakeSuite: base.suite},
	}
	f.host = newFakeHost(f.store, base.clock)
	return f
}

// hostService is the scan service of the fixture, with the host's ports.
func (f *hostFixture) hostService(t *testing.T) *scan.Service {
	t.Helper()
	return scan.New(f.deps(t, true))
}

// replayService is the scan service of the fixture with no host store and no
// memo, as a replay has it.
func (f *hostFixture) replayService(t *testing.T) *scan.Service {
	t.Helper()
	return scan.New(f.deps(t, false))
}

func (f *hostFixture) deps(t *testing.T, withHost bool) scan.Deps {
	t.Helper()
	plain := f.fixture
	engines := enginesFor(t)
	ecosystem := hostEcosystem{fakeEcosystem: fakeEcosystem{workspaces: plain.workspaces, broken: plain.broken}, runs: f.runs}
	result := scan.Deps{
		Ecosystem: ecosystem, Suite: f.suite, Workspaces: plain.workspaces, Engines: engines, Prober: plain.prober,
		Source: plain.source, Project: plain.workspaces.project, Clock: plain.clock.read,
		WitnessHost: f.witnesses, Witnesses: scan.ExecutionWitnesses{Host: f.witnesses, Project: plain.workspaces.project},
	}
	if f.pulls != nil {
		result.PullRequests = f.pulls
	}
	if withHost {
		var memo ports.ExecutionMemo = f.memo
		if f.diskMemo != nil {
			memo = f.diskMemo
		}
		result.Host = &scan.HostDeps{
			Store: f.store, Memo: memo, Sources: f.sources, Head: f.head, Entropy: bytes.NewReader(bytes.Repeat([]byte{7}, 1<<16)), Plugin: pluginName,
		}
	}
	return result
}

// invoke runs one invocation of the host scan.
func (f *hostFixture) invoke(t *testing.T, request scan.Request) scan.HostResult {
	t.Helper()
	result, err := f.hostService(t).RunHost(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// loop invokes the scan, answering what it waits for, until it finishes or
// the invocations run out.
func (f *hostFixture) loop(t *testing.T, request scan.Request, invocations int) []scan.HostResult {
	t.Helper()
	var results []scan.HostResult
	for range invocations {
		result := f.invoke(t, request)
		results = append(results, result)
		if result.Finished {
			return results
		}
		f.host.answer(t, result.Session)
		request.Resume = true
	}
	t.Fatalf("the scan did not finish in %d invocations", invocations)
	return nil
}

func hostRequest() scan.Request {
	req := request()
	req.Module = format.Module{Name: pluginName, Version: "0.1.0"}
	return req
}

// sessionRequests lists the request ids the session holds, sorted.
func sessionRequests(t *testing.T, store hostproto.HostStore, session hostproto.Session) []hostproto.RequestID {
	t.Helper()
	items, err := store.Items()
	if err != nil {
		t.Fatal(err)
	}
	var ids []hostproto.RequestID
	for _, item := range items {
		if slices.Contains(item.Sessions, session) {
			ids = append(ids, item.RequestID)
		}
	}
	return ids
}

// emissions counts the emissions of the session.
func emissions(t *testing.T, store hostproto.HostStore, session hostproto.Session) int {
	t.Helper()
	records, err := store.Records()
	if err != nil {
		t.Fatal(err)
	}
	return hostproto.Emissions(records, session)
}

// pendingKinds lists the kinds of the tickets a result waits for, sorted.
func pendingKinds(t *testing.T, store hostproto.HostStore, result scan.HostResult) []hostproto.Kind {
	t.Helper()
	var kinds []hostproto.Kind
	for _, ticket := range result.Pending {
		request, err := store.GetRequest(ticket.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, request.Kind)
	}
	slices.Sort(kinds)
	return kinds
}

// must decodes a JSON document for a fake answer.
func must(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// hostprototestRequest is a request of the prove session, unrelated to the
// scan.
func hostprototestRequest(t *testing.T) hostbridge.Ask {
	t.Helper()
	return hostbridge.Ask{Request: hostprototest.Request(), Votes: 1, Criteria: []string{"SP-1~1"}, Purpose: string(hostproto.KindJudgeValidity)}
}

// proveBridge is the bridge of a prove wave on the fixture's store.
func proveBridge(t *testing.T, f *hostFixture) *hostbridge.Bridge {
	t.Helper()
	wave, err := f.store.AdvanceWave(hostproto.SessionProve)
	if err != nil {
		t.Fatal(err)
	}
	return hostbridge.New(f.store, f.clock.read, bytes.NewReader(bytes.Repeat([]byte{9}, 1<<12)), hostproto.SessionProve, wave)
}

func mustID(t *testing.T, request hostproto.Request) hostproto.RequestID {
	t.Helper()
	id, err := request.ID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
