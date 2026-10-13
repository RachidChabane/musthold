package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/gitrepo"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// The exit-3 loop of a scan that asks its host.
const (
	exitAwaitingHost = 3
	exitUsage        = 2
	// maxScanWaves bounds the invocations one drive makes, so that a scan that
	// never stops waiting fails instead of running until the command timeout.
	maxScanWaves = 12
	sessionLine  = "session "
	scanCommand  = "scan"
	hostFlag     = "--host"
	resumeFlag   = "--resume"
	// sessionCap is the most emissions a scan session makes.
	sessionCap = 18
)

// The kinds of answer a scan's host is asked for, told apart by the member the
// response schema requires, as a host reading the request does.
const (
	answerPromises   = "promises"
	answerWitnesses  = "witnesses"
	answerScope      = "verdicts"
	answerGeneration = "mutants"
	answerLabels     = "labels"
)

// hostRepository is a copy of a fixture committed in a repository of its own,
// with the state and cache a user's machine gives the binary and, for a pull
// request, the directory of a stub gh.
type hostRepository struct {
	t      *testing.T
	root   string
	binary string
	state  string
	cache  string
	tmp    string
	// path is prepended to PATH, empty for none.
	path string
	// extra are environment variables the commands run with besides.
	extra []string
}

func newHostRepository(t *testing.T, fixture string) *hostRepository {
	t.Helper()
	gitrepo.Isolate(t)
	guardMemory(t)
	binary := productBinary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &hostRepository{
		t: t, root: filepath.Join(base, "project"), binary: binary,
		state: filepath.Join(base, "state"), cache: filepath.Join(base, "cache"), tmp: filepath.Join(base, "tmp"),
	}
	for _, dir := range []string{r.state, r.cache, r.tmp} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			t.Fatal(err)
		}
	}
	copyTree(t, filepath.Join(abs(t, discpowerDir), fixture), r.root)
	r.git("init", "-q", "-b", "main")
	r.commit("first")
	return r
}

// git runs git in the repository with an identity of its own and fails the
// test when git does.
func (r *hostRepository) git(args ...string) string {
	r.t.Helper()
	config := []string{"-c", "user.name=scan", "-c", "user.email=scan@example.com", "-c", "commit.gpgsign=false"}
	ran := r.run("", "git", append(config, args...)...)
	if ran.code != 0 {
		r.t.Fatalf("git %v: %s", args, ran.output())
	}
	return strings.TrimSpace(ran.stdout)
}

// commit stages everything and commits it, returning the new commit id.
func (r *hostRepository) commit(message string) string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-qm", message)
	return r.git("rev-parse", "HEAD")
}

func (r *hostRepository) environment() []string {
	path := os.Getenv("PATH")
	if r.path != "" {
		path = r.path + string(os.PathListSeparator) + path
	}
	env := append(os.Environ(), "PATH="+path, "XDG_STATE_HOME="+r.state, "XDG_CACHE_HOME="+r.cache, "TMPDIR="+r.tmp, "GOMAXPROCS=2")
	return append(env, r.extra...)
}

// run runs a program in the repository with the standard input.
func (r *hostRepository) run(stdin, name string, args ...string) result {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "nice", append([]string{"-n", "10", name}, args...)...)
	command.Dir = r.root
	command.Env = r.environment()
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		r.t.Fatalf("%s %v: %v", name, args, err)
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// dp runs the product binary in the repository.
func (r *hostRepository) dp(stdin string, args ...string) result {
	r.t.Helper()
	return r.run(stdin, r.binary, args...)
}

// short is the short commit the scan's reports are written under.
func (r *hostRepository) short() string {
	r.t.Helper()
	return r.git("rev-parse", "HEAD")[:7]
}

// reportPath is a path under the folder of the scan's reports.
func (r *hostRepository) reportPath(parts ...string) string {
	return filepath.Join(append([]string{r.root, ".spec-proof", "scan", r.short()}, parts...)...)
}

// statement reads the scan statement and admits it as the format reads it.
func (r *hostRepository) statement() format.ScanStatement {
	r.t.Helper()
	raw, err := os.ReadFile(r.reportPath("scan.json"))
	if err != nil {
		r.t.Fatal(err)
	}
	admitted, err := format.AdmitScan(raw)
	if err != nil {
		r.t.Fatalf("the statement is not admitted: %v\n%s", err, raw)
	}
	return admitted
}

// emissions counts the emissions the host store journals for the session.
func (r *hostRepository) emissions(session hostproto.Session) int {
	r.t.Helper()
	lookup := func(name string) (string, bool) {
		if name == "XDG_STATE_HOME" {
			return r.state, true
		}
		return "", false
	}
	store, err := hoststore.Open(xdg.HostDir(lookup, r.state, r.root))
	if err != nil {
		r.t.Fatal(err)
	}
	records, err := store.Records()
	if err != nil {
		r.t.Fatal(err)
	}
	return hostproto.Emissions(records, session)
}

// sessionOf reads the session a waiting scan printed.
func sessionOf(t *testing.T, output string) hostproto.Session {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		if text, found := strings.CutPrefix(scanner.Text(), sessionLine); found {
			session, err := hostproto.ParseSession(strings.TrimSpace(text))
			if err != nil {
				t.Fatalf("an unreadable session %q: %v", text, err)
			}
			return session
		}
	}
	t.Fatalf("no session printed:\n%s", output)
	return ""
}

// drive is a scan that asks its host, driven through its exit-3 loop: what
// each invocation printed and the session it waited in.
type drive struct {
	waves   []result
	session hostproto.Session
}

// last is the invocation the drive ended with.
func (d drive) last() result { return d.waves[len(d.waves)-1] }

// driveScan runs `scan --host` and lets the host answer the tickets of the
// session between invocations, resuming until the scan stops waiting or the
// waves run out.
func (r *hostRepository) driveScan(host *scanHost, args ...string) drive {
	r.t.Helper()
	ran := r.dp("", append([]string{scanCommand, hostFlag}, args...)...)
	d := drive{waves: []result{ran}}
	for ran.code == exitAwaitingHost && len(d.waves) < maxScanWaves {
		d.session = sessionOf(r.t, ran.stdout)
		host.answerSession(r, d.session)
		ran = r.dp("", append([]string{scanCommand, resumeFlag}, args...)...)
		d.waves = append(d.waves, ran)
	}
	if ran.code != 0 {
		r.t.Fatalf("the scan ended with exit %d after %d invocations:\n%s", ran.code, len(d.waves), ran.output())
	}
	return d
}

// scanHost answers the tickets of a scan session through the command line, as
// a host relaying them to its models does: it lists the session's tickets,
// shows each and submits the persona's answer.
type scanHost struct {
	t       *testing.T
	persona persona
	// times counts how often each request and vote was answered.
	times map[string]int
	// asked are the prompts shown, by kind of answer.
	asked map[string][]string
}

// ticketAsk is what a host reads of one ticket: the kind of answer, the user
// prompt, the vote and how often the same request and vote was answered
// before.
type ticketAsk struct {
	kind, user  string
	vote, times int
}

// persona answers one ticket.
type persona func(ask ticketAsk) string

func newScanHost(t *testing.T, answer persona) *scanHost {
	return &scanHost{t: t, persona: answer, times: map[string]int{}, asked: map[string][]string{}}
}

// answerSession answers every waiting ticket of the session.
func (h *scanHost) answerSession(r *hostRepository, session hostproto.Session) {
	h.t.Helper()
	listed := r.dp("", hostproto.ListArgs(session)...)
	if listed.code != 0 {
		h.t.Fatalf("request list: %s", listed.output())
	}
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(listed.stdout), &listing); err != nil {
		h.t.Fatalf("request list printed %q: %v", listed.stdout, err)
	}
	for _, item := range listing.Items {
		if !slicesContain(item.Sessions, session) {
			h.t.Fatalf("ticket %s of sessions %v listed for %s", item.Ticket.TicketID, item.Sessions, session)
		}
		kind := answerKind(h.t, item.View.ResponseSchema)
		key := string(item.Ticket.RequestID) + "." + strconv.Itoa(item.Ticket.Vote)
		output := h.persona(ticketAsk{kind: kind, user: item.View.User, vote: item.Ticket.Vote, times: h.times[key]})
		h.times[key]++
		h.asked[kind] = append(h.asked[kind], item.View.User)
		if shown := r.dp("", hostproto.ShowArgs(item.Ticket.TicketID)...); shown.code != 0 {
			h.t.Fatalf("request show %s: %s", item.Ticket.TicketID, shown.output())
		}
		if submitted := r.dp(output, hostproto.SubmitArgs(item.Ticket.TicketID, item.Ticket.Nonce)...); submitted.code != 0 {
			h.t.Fatalf("request submit %s of a %s answer: %s\n%s", item.Ticket.TicketID, kind, submitted.output(), output)
		}
	}
}

func slicesContain(sessions []hostproto.Session, session hostproto.Session) bool {
	for _, one := range sessions {
		if one == session {
			return true
		}
	}
	return false
}

// answerKind reads the kind of answer from the member the response schema
// requires last: promises, witnesses, verdicts, mutants or labels.
func answerKind(t *testing.T, schema json.RawMessage) string {
	t.Helper()
	var document struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &document); err != nil || len(document.Required) == 0 {
		t.Fatalf("an unreadable response schema %s: %v", schema, err)
	}
	return document.Required[len(document.Required)-1]
}

// encode writes an answer as JSON.
func encode(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// Patterns of the prompts a scan shows its host.
var (
	// targetQuote is the requirement a writer or a scope judge is asked about.
	targetQuote = regexp.MustCompile(`(?m)^Requirement (?:to check|the witnesses check): R\d+\n(.+)$`)
	// generationQuote is the requirement a generation ticket breaks.
	generationQuote = regexp.MustCompile(`(?m)^Requirement:\n(.+)$`)
	// scopeWitness is a witness a scope request shows.
	scopeWitness = regexp.MustCompile(`Witness (w\d+), for the clause`)
	// labelPromise is a promise a labels request shows, with its id.
	labelPromise = regexp.MustCompile(`(?m)^(p\d+): "(.*)"$`)
	// labelFunction is a function a labels request shows, with its id.
	labelFunction = regexp.MustCompile(`(?m)^Function (f\d+), [^,]+, (.*):$`)
	// unitHeader opens a unit of the code a generation ticket shows.
	unitHeader = regexp.MustCompile(`(?m)^--- (\S+) :: (\S+)$`)
)

// quoteOf reads the quote a prompt is about, empty when it shows none.
func quoteOf(pattern *regexp.Regexp, user string) string {
	if match := pattern.FindStringSubmatch(user); match != nil {
		return match[1]
	}
	return ""
}

// shownUnit is one unit of the code a generation ticket shows.
type shownUnit struct {
	file, qualname, source string
}

// shownUnits reads the units a generation ticket shows, each up to the next
// unit or the first empty line, as the exact text of the unit: a unit's
// source ends with its last character, never with a line break.
func shownUnits(user string) []shownUnit {
	var units []shownUnit
	lines := strings.Split(user, "\n")
	for index := 0; index < len(lines); index++ {
		match := unitHeader.FindStringSubmatch(lines[index])
		if match == nil {
			continue
		}
		unit := shownUnit{file: match[1], qualname: match[2]}
		var body []string
		for index+1 < len(lines) && lines[index+1] != "" && !unitHeader.MatchString(lines[index+1]) {
			index++
			body = append(body, lines[index])
		}
		unit.source = strings.Join(body, "\n")
		units = append(units, unit)
	}
	return units
}
