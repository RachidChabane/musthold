package prove_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/operations"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/proofsource"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/shallnot"
	golangunits "github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/golang"
	pythonunits "github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/python"
	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/closedq"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

const (
	pluginName    = "spec-proof-completeness"
	moduleVersion = "0.1.0"
)

// criteriaSource serves a recorded report.
type criteriaSource struct{ report ports.Report }

func (c criteriaSource) Load(context.Context) (ports.Report, error) { return c.report, nil }

// changeSource serves a fixed change, or the absence of a base.
type changeSource struct {
	set ports.ChangeSet
	err error
	// bases, when set, receives the base every call was given.
	bases *[]string
}

func (c changeSource) Changes(_ context.Context, base string) (ports.ChangeSet, error) {
	if c.bases != nil {
		*c.bases = append(*c.bases, base)
	}
	return c.set, c.err
}

// memoryProofs keeps the Statements a run writes.
type memoryProofs struct {
	mutex      sync.Mutex
	statements map[string][]byte
}

func (m *memoryProofs) Has(spec, key string) (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	_, found := m.statements[spec+"\x00"+key]
	return found, nil
}

func (m *memoryProofs) Write(spec, key string, canonical []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.statements[spec+"\x00"+key] = canonical
	return nil
}

// bySpec returns the Statement of each spec, decoded, after checking that it is a
// valid one.
func (m *memoryProofs) bySpec(t testing.TB) map[string]format.Statement[format.Completeness] {
	t.Helper()
	found := map[string]format.Statement[format.Completeness]{}
	for key, content := range m.statements {
		statement, err := format.DecodeAs[format.Completeness](content)
		if err != nil {
			t.Fatal(err)
		}
		if err := statement.Validate(); err != nil {
			t.Fatalf("the Statement is invalid: %v", err)
		}
		found[strings.SplitN(key, "\x00", 2)[0]] = statement
	}
	return found
}

type memoryMemo struct{ entries map[string][]byte }

func (m *memoryMemo) Get(key string) ([]byte, bool, error) {
	value, found := m.entries[key]
	return value, found, nil
}

func (m *memoryMemo) Put(key string, value []byte) error {
	m.entries[key] = value
	return nil
}

type memoryPending struct{ current format.Pending }

func (m *memoryPending) Write(pending format.Pending) error {
	m.current = pending
	return nil
}

// honestReplayer answers the pending tickets from an answer source, through the
// same checks a script's submission goes through.
type honestReplayer struct {
	service requests.Service
	answers hostprototest.AnswerSource
	profile format.ExecutorProfile
	origin  requests.Origin
	served  int
}

func (r *honestReplayer) Serve(tickets []requests.PendingTicket) error {
	for _, pending := range tickets {
		ticket, err := hostproto.ParseTicketID(pending.Ticket)
		if err != nil {
			return err
		}
		view, err := r.service.Show(ticket, "")
		if err != nil {
			return err
		}
		output, err := r.answers.Answer(hostprototest.PersonaHonest, view)
		if err != nil {
			return err
		}
		profile := r.profile
		if _, err := r.service.Submit(requests.Submission{Ticket: ticket, Nonce: pending.Nonce, Output: []byte(output), Origin: r.origin, Executor: &profile}); err != nil {
			return err
		}
		r.served++
	}
	return nil
}

// world is a fixture project with everything a run needs around it.
type world struct {
	t       *testing.T
	root    string
	report  ports.Report
	changes changeSource
	policy  policy.Policy
	store   *hoststore.Store
	proofs  *memoryProofs
	memo    *memoryMemo
	pending *memoryPending
	// answers serves the requests; nil leaves them pending.
	answers hostprototest.AnswerSource
	replay  *honestReplayer
}

// newWorld builds the project of a fixture with its change applied, the recorded
// report and no evidence of other modules.
func newWorld(t *testing.T, fixture string) *world {
	t.Helper()
	root := testkit.MaterializeWithoutRepository(t, fixture)
	content, err := os.ReadFile(testkit.ReportPath(fixture))
	if err != nil {
		t.Fatal(err)
	}
	report, err := shallnot.ParseReport(content)
	if err != nil {
		t.Fatal(err)
	}
	store, err := hoststore.Open(filepath.Join(t.TempDir(), "host"))
	if err != nil {
		t.Fatal(err)
	}
	return &world{
		t: t, root: root, report: report, changes: changeSource{set: testkit.FixtureChanges(t, fixture)}, policy: policy.Default(),
		store: store, proofs: &memoryProofs{statements: map[string][]byte{}}, memo: &memoryMemo{entries: map[string][]byte{}},
		pending: &memoryPending{}, answers: testkit.Honest(),
	}
}

func (w *world) deps() prove.Deps {
	w.t.Helper()
	checker, err := schemacheck.New()
	if err != nil {
		w.t.Fatal(err)
	}
	bound, err := operations.Load(w.root)
	if err != nil {
		w.t.Fatal(err)
	}
	deps := prove.Deps{
		Criteria:   criteriaSource{report: w.report},
		Indexers:   []ports.Indexer{pythonunits.New(), golangunits.New()},
		Changes:    w.changes,
		Operations: bound,
		Project:    os.DirFS(w.root),
		Store:      w.store,
		Proofs:     w.proofs,
		Evidence:   proofsource.Source{Root: w.root},
		Pending:    w.pending,
		Memo:       w.memo,
		Statements: checker,
		Clock:      func() time.Time { return time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC) },
		Entropy:    rand.Reader,
	}
	if w.answers != nil {
		w.replay = &honestReplayer{
			service: requests.Service{Plugin: "spec-proof-completeness", Store: w.store, Validator: responsecheck.New(), Clock: deps.Clock},
			answers: w.answers,
			origin:  requests.OriginReplay,
			profile: format.ExecutorProfile{
				Channel: format.ChannelHostSubagent, Harness: format.Harness{Name: "fixture-host", Version: "1"},
				Isolation: format.IsolationRestricted, Model: format.ExecutorModel{Name: "fixture-model", Source: format.ModelSourceDeclared},
			},
		}
		if _, relayed := w.answers.(relay); relayed {
			w.replay.profile, w.replay.origin = requests.RelayProfile(relaySource), requests.OriginRelay
		}
		deps.Replay = w.replay
	}
	return deps
}

func (w *world) run(options prove.Options) (prove.Result, error) {
	w.t.Helper()
	if options.Selection == "" {
		options.Selection = prove.SelectAll
	}
	useCase := prove.UseCase{
		Deps: w.deps(),
		Settings: prove.Settings{
			Policy: w.policy, Module: format.Module{Name: pluginName, Version: moduleVersion}, Plugin: pluginName, PendingModule: "completeness",
		},
	}
	return useCase.Run(context.Background(), options)
}

// mustRun runs and fails the test on an error.
func (w *world) mustRun(options prove.Options) prove.Result {
	w.t.Helper()
	result, err := w.run(options)
	if err != nil {
		w.t.Fatal(err)
	}
	return result
}

// statement returns the Statement of the fixture spec.
func (w *world) statement() format.Statement[format.Completeness] {
	w.t.Helper()
	statement, found := w.proofs.bySpec(w.t)[testkit.SpecFile]
	if !found {
		w.t.Fatalf("no Statement was written for %s", testkit.SpecFile)
	}
	return statement
}

// requestKinds counts the closed requests the host store holds by what they ask:
// `dimensions`, `hunk` or `testability`.
func (w *world) requestKinds() map[string]int {
	w.t.Helper()
	entries, err := w.store.ListEntries()
	if err != nil {
		w.t.Fatal(err)
	}
	seen := map[hostproto.RequestID]bool{}
	kinds := map[string]int{}
	for _, entry := range entries {
		id := entry.Ticket.RequestID
		if seen[id] {
			continue
		}
		seen[id] = true
		request, err := w.store.GetRequest(id)
		if err != nil {
			w.t.Fatal(err)
		}
		var set closedq.Set
		if err := json.Unmarshal(request.Closed, &set); err != nil {
			w.t.Fatal(err)
		}
		switch {
		case set.Questions[0].ID == "criterion":
			kinds["hunk"]++
		case strings.HasPrefix(set.Questions[0].ID, "K-"):
			kinds["testability"]++
		default:
			kinds["dimensions"]++
		}
	}
	return kinds
}

// withEvidence writes the Statements the other modules would have written.
func (w *world) withEvidence(power testkit.Power, aligned ...testkit.Aligned) {
	w.t.Helper()
	power.Write(w.t, w.root)
	for _, item := range aligned {
		item.Write(w.t, w.root)
	}
}

// questions returns the questions of the Statement as `dimension: text` lines.
func questionLines(statement format.Statement[format.Completeness]) []string {
	lines := make([]string, len(statement.Predicate.Questions))
	for index, q := range statement.Predicate.Questions {
		lines[index] = q.Dimension + ": " + q.Question
	}
	sort.Strings(lines)
	return lines
}

// questionsOf lists the questions of a dimension.
func questionsOf(statement format.Statement[format.Completeness], dimension string) []format.Question {
	var found []format.Question
	for _, q := range statement.Predicate.Questions {
		if q.Dimension == dimension {
			found = append(found, q)
		}
	}
	return found
}

func anyContains(lines []string, text string) bool {
	return slices.ContainsFunc(lines, func(line string) bool { return strings.Contains(line, text) })
}

// relaySource names the System One endpoint a relay answers as.
const relaySource = "nimble:9b@endpoint"

// relay answers like the honest host and states one confidence for every answer,
// the way a System One endpoint does; a negative confidence states none.
type relay struct {
	honest     hostprototest.AnswerSource
	confidence float64
}

func (r relay) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	output, err := r.honest.Answer(persona, view)
	if err != nil {
		return "", err
	}
	var document struct {
		Answers []map[string]any `json:"answers"`
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		return "", err
	}
	for _, answer := range document.Answers {
		if r.confidence < 0 {
			delete(answer, "confidence")
		} else {
			answer["confidence"] = r.confidence
		}
	}
	rewritten, err := json.Marshal(document)
	return string(rewritten), err
}

// appendTo adds text to a file of the project.
func appendTo(t testing.TB, root, file, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(file))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(content, []byte(text)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSpec writes a spec file of the project.
func (w *world) writeSpec(file, content string) {
	w.t.Helper()
	path := filepath.Join(w.root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// freshStore opens an empty host store.
func freshStore(t testing.TB) *hoststore.Store {
	t.Helper()
	store, err := hoststore.Open(filepath.Join(t.TempDir(), "host"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}
