package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	verifiesHostE2E     = " [verifies SP-2274~1]"
	verifiesHostReplay  = " [verifies SP-2275~1]"
	verifiesHostView    = " [verifies SP-2276~1]"
	verifiesHostResume  = " [verifies SP-2277~1]"
	verifiesHostBudget  = " [verifies SP-2278~1]"
	verifiesHostProve   = " [verifies SP-2279~1]"
	verifiesHostPull    = " [verifies SP-2280~1]"
	brokenPromiseFolder = "promise-1"
	routeWitness        = "witness_failure"
	findingRecord       = "finding.json"
	notHeld             = "NOT held"
	// noBudget lets every mutant run, so that the scan does not depend on
	// the speed of the machine.
	noBudget = "--budget=0"
	// The fixtures a host drives: a README of promises, a function whose
	// parameters no differential run draws, and a test that runs it and
	// asserts nothing.
	pytestHostFixture = "fixtures/scan/pytest-host"
	goHostFixture     = "fixtures/scan/go-host"
)

// hostProject is a scan fixture a host drives end to end.
type hostProject struct {
	name, fixture, tool string
	scenario            scenario
	// shown and hidden are units the generation tickets of the broken
	// promise show and do not show: the code its witnesses run.
	shown, hidden []string
	// killerFailure is what the broken promise's script prints when its
	// witness fails on the change.
	killerFailure []string
}

func hostProjects() []hostProject {
	return []hostProject{
		{
			name: "pytest", fixture: pytestHostFixture, tool: "python3", scenario: invoiceScenario(),
			shown: []string{"order_total"}, hidden: []string{"discount", "line_total", "shipping"},
			killerFailure: []string{"FAILED tests/test_specproof_", " failed"},
		},
		{
			name: "go", fixture: goHostFixture, tool: "go", scenario: tallyScenario(),
			shown: []string{"tally.OrderTotal"}, hidden: []string{"tally.Discount", "tally.LineTotal", "tally.Shipping"},
			killerFailure: []string{"--- FAIL: Test_specproof_witness_", "FAIL"},
		},
	}
}

func TestScanWithAHost(t *testing.T) {
	for _, project := range hostProjects() {
		t.Run(project.name, func(t *testing.T) {
			if _, err := exec.LookPath(project.tool); err != nil {
				t.Skipf("no %s: %v", project.tool, err)
			}
			r := newHostRepository(t, project.fixture)
			host := newScanHost(t, project.scenario.honest(t))
			d := r.driveScan(host, noBudget)
			out := d.last().stdout
			t.Run("the scan waits for its host with exit 3 and finishes with exit 0 once every ticket is answered"+verifiesHostE2E, func(t *testing.T) {
				if d.waves[0].code != exitAwaitingHost || len(d.waves) < 3 {
					t.Fatalf("%d invocations, the first exiting %d:\n%s", len(d.waves), d.waves[0].code, d.waves[0].output())
				}
			})
			t.Run("every answer of an honest host is used, a generation answer whose code ends with a line break included: no ticket is asked again"+verifiesHostE2E, func(t *testing.T) {
				for key, times := range host.times {
					if times > 1 {
						t.Errorf("ticket %s was answered %d times", key, times)
					}
				}
				if len(host.asked[answerGeneration]) == 0 {
					t.Error("no generation ticket was asked")
				}
			})
			t.Run("the broken promise is printed with its quote, a change the whole suite passes 3/3 and its witness script"+verifiesHostE2E, func(t *testing.T) {
				for _, want := range []string{
					fmt.Sprintf("#P1 %q (README.md) a change that passes the whole suite 3/3 fails a block-bearing witness written for it", orderQuote),
					"repro: .spec-proof/scan/" + r.short() + "/" + brokenPromiseFolder + "/killer.sh",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("missing %q:\n%s", want, out)
					}
				}
			})
			t.Run("a witness the reference fails is a question about its promise, never a finding"+verifiesHostE2E, func(t *testing.T) {
				if !strings.Contains(out, fmt.Sprintf("question: %q (README.md): witness ", fortyQuote)) || !strings.Contains(out, ", the reference fails it") {
					t.Fatalf("no question:\n%s", out)
				}
			})
			t.Run("a quote its source does not hold is dropped and counted"+verifiesHostE2E, func(t *testing.T) {
				if !strings.Contains(out, "unchecked: 1 extracted quote not found byte for byte in their source") || strings.Contains(out, refundsQuote) {
					t.Fatalf("the dropped quote is not reported as dropped:\n%s", out)
				}
			})
			t.Run("the statement is admitted with its promises checked, one promise broken and the questions"+verifiesHostE2E, func(t *testing.T) {
				predicate := r.statement().Predicate
				if !predicate.PromisesChecked || len(predicate.Promises) != 2 || len(predicate.BrokenPromises) == 0 || len(predicate.Questions) == 0 {
					t.Fatalf("checked %t, %d promises, %d broken, %d questions", predicate.PromisesChecked, len(predicate.Promises), len(predicate.BrokenPromises), len(predicate.Questions))
				}
				quotes := map[string]string{}
				for _, one := range predicate.Promises {
					quotes[one.ID] = one.Quote
				}
				for _, broken := range predicate.BrokenPromises {
					if quotes[broken.Promise] != orderQuote {
						t.Errorf("a broken promise %q", quotes[broken.Promise])
					}
				}
				for _, question := range predicate.Questions {
					if quotes[question.Promise] != fortyQuote {
						t.Errorf("a question about %q", quotes[question.Promise])
					}
				}
				if !strings.Contains(out, " of 18 model calls · promises checked") {
					t.Errorf("the footer does not say the promises were checked:\n%s", out)
				}
			})
			t.Run("the broken promise's script reproduces the witness failure on the change"+verifiesHostE2E, func(t *testing.T) {
				script := filepath.Join(".spec-proof", "scan", r.short(), brokenPromiseFolder, "killer.sh")
				if _, err := os.Stat(filepath.Join(r.root, script)); err != nil {
					t.Fatal(err)
				}
				killer := r.run("", "sh", script)
				if killer.code == 0 {
					t.Fatalf("the witness passes on the change:\n%s", killer.output())
				}
				for _, want := range project.killerFailure {
					if !strings.Contains(killer.output(), want) {
						t.Errorf("the script did not fail on its witness, %q is missing:\n%s", want, killer.output())
					}
				}
			})
			t.Run("the generation tickets of the broken promise show the code its witnesses run, and no other"+verifiesHostView, func(t *testing.T) {
				var tickets int
				for _, user := range host.asked[answerGeneration] {
					if quoteOf(generationQuote, user) != orderQuote {
						continue
					}
					tickets++
					var shown []string
					for _, unit := range shownUnits(user) {
						shown = append(shown, unit.qualname)
					}
					for _, unit := range project.shown {
						if !slices.Contains(shown, unit) {
							t.Errorf("%s is not shown: %v", unit, shown)
						}
					}
					for _, unit := range project.hidden {
						if slices.Contains(shown, unit) {
							t.Errorf("%s is shown: %v", unit, shown)
						}
					}
				}
				if tickets == 0 {
					t.Fatal("no generation ticket for the broken promise")
				}
			})
			t.Run("--resume of a scan whose flags changed exits 2 naming what changed"+verifiesHostResume, func(t *testing.T) {
				opened := r.dp("", scanCommand, hostFlag, noBudget, "--seed=5")
				if opened.code != exitAwaitingHost {
					t.Fatalf("exit %d:\n%s", opened.code, opened.output())
				}
				resumed := r.dp("", scanCommand, resumeFlag, noBudget, "--seed=6")
				if resumed.code != exitUsage || !strings.Contains(resumed.stderr, "seed 5 → 6") {
					t.Fatalf("exit %d:\n%s", resumed.code, resumed.output())
				}
			})
			t.Run("the witness_failure route and the broken promise replay with the host directory and the memo deleted"+verifiesHostReplay, func(t *testing.T) {
				for _, dir := range []string{r.state, r.cache} {
					if err := os.RemoveAll(dir); err != nil {
						t.Fatal(err)
					}
				}
				folders := witnessRouteFolders(t, r)
				if len(folders) == 0 {
					t.Fatal("no finding folder holds a witness_failure route")
				}
				folders = append(folders, r.reportPath(brokenPromiseFolder))
				for _, folder := range folders {
					replay := r.dp("", scanCommand, "replay", folder)
					if replay.code != 0 || strings.Contains(replay.stdout, notHeld) {
						t.Errorf("replay of %s: exit %d\n%s", folder, replay.code, replay.output())
					}
				}
			})
		})
	}
}

// witnessRouteFolders lists the finding folders of the scan whose route is a
// witness failure.
func witnessRouteFolders(t *testing.T, r *hostRepository) []string {
	t.Helper()
	records, err := filepath.Glob(r.reportPath("*", findingRecord))
	if err != nil {
		t.Fatal(err)
	}
	var folders []string
	for _, path := range records {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			Test  string `json:"test"`
			Route struct {
				Kind string `json:"kind"`
			} `json:"route"`
		}
		if err := json.Unmarshal(content, &record); err != nil {
			t.Fatal(err)
		}
		if record.Test != "" && record.Route.Kind == routeWitness {
			folders = append(folders, filepath.Dir(path))
		}
	}
	return folders
}

func TestScanWithARejectingHost(t *testing.T) {
	for _, project := range hostProjects() {
		t.Run(project.name, func(t *testing.T) {
			if _, err := exec.LookPath(project.tool); err != nil {
				t.Skipf("no %s: %v", project.tool, err)
			}
			r := newHostRepository(t, project.fixture)
			host := newScanHost(t, project.scenario.rejecting(t))
			d := r.driveScan(host, noBudget)
			rejectedWithinTheCap(t, r, host, d)
		})
	}
}

// rejectedWithinTheCap checks a scan whose host answered every ticket with an
// answer the scan rejects.
func rejectedWithinTheCap(t *testing.T, r *hostRepository, host *scanHost, d drive) {
	t.Run("a host whose every answer is rejected costs the session at most 18 emissions, and the scan still finishes"+verifiesHostBudget, func(t *testing.T) {
		emitted := r.emissions(d.session)
		if emitted == 0 || emitted > sessionCap {
			t.Fatalf("%d emissions", emitted)
		}
		if want := fmt.Sprintf("%d of %d model calls", emitted, sessionCap); !strings.Contains(d.last().stdout, want) {
			t.Errorf("missing %q:\n%s", want, d.last().stdout)
		}
	})
	t.Run("the asks the call cap left out are reported"+verifiesHostBudget, func(t *testing.T) {
		if !strings.Contains(d.last().stdout, "left unasked: the session reached its call cap") {
			t.Fatalf("no ask left unasked:\n%s", d.last().stdout)
		}
	})
	t.Run("every kind of answer is asked, the rejected ones again until the session reaches its cap"+verifiesHostBudget, func(t *testing.T) {
		again := 0
		for _, kind := range []string{answerWitnesses, answerScope, answerGeneration, answerLabels} {
			if len(host.asked[kind]) == 0 {
				t.Errorf("no %s ticket was asked", kind)
			}
			asked := map[string]int{}
			for _, user := range host.asked[kind] {
				asked[user]++
			}
			for _, times := range asked {
				again += times - 1
			}
		}
		if again == 0 || r.emissions(d.session) != sessionCap {
			t.Fatalf("%d asks again, %d emissions", again, r.emissions(d.session))
		}
	})
}

// The prove projects a scan shares its repository and host store with.
const (
	recordedShallnot   = "fixtures/prove/shallnot-recorded.sh"
	recordedReportVar  = "SHALLNOT_RECORDED_REPORT"
	proveCommand       = "prove"
	proveAll           = "--all"
	maxInterleavedRuns = 2 * maxScanWaves
)

// proveProject is a prove fixture with the Shallnot report it is proved
// against, the README a scan of it quotes and how its host witnesses it.
type proveProject struct {
	name, fixture, report, tool, readme string
	scenario                            scenario
}

func proveProjects() []proveProject {
	return []proveProject{
		{name: "pytest", fixture: fixtureProject, report: "fixtures/prove/parcel-report.json", tool: "python3", readme: readmeOf(parcelQuote), scenario: parcelScenario()},
		{name: "go", fixture: "fixtures/ecosystems/go", report: "fixtures/prove/go-report.json", tool: "go", readme: readmeOf(parcelGoQuote), scenario: parcelGoScenario()},
	}
}

// proveRunner runs the command line of the repository for a prove host, and
// fails when the command does.
type proveRunner struct{ r *hostRepository }

// Run implements hostprototest.ModuleRunner.
func (p proveRunner) Run(args []string, stdin []byte) ([]byte, error) {
	ran := p.r.dp(string(stdin), args...)
	if ran.code != 0 {
		return []byte(ran.stdout), fmt.Errorf("exit %d: %s", ran.code, strings.TrimSpace(ran.stderr))
	}
	return []byte(ran.stdout), nil
}

// proveTickets lists the tickets the prove session waits for.
func proveTickets(t *testing.T, r *hostRepository) []string {
	t.Helper()
	listed := r.dp("", hostproto.ListArgs(hostproto.SessionProve)...)
	if listed.code != 0 {
		t.Fatalf("request list: %s", listed.output())
	}
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(listed.stdout), &listing); err != nil {
		t.Fatal(err)
	}
	tickets := make([]string, len(listing.Items))
	for index, item := range listing.Items {
		tickets[index] = item.Ticket.TicketID.String()
	}
	return tickets
}

func TestScanWithAHostBesideProve(t *testing.T) {
	for _, project := range proveProjects() {
		t.Run(project.name, func(t *testing.T) {
			if _, err := exec.LookPath(project.tool); err != nil {
				t.Skipf("no %s: %v", project.tool, err)
			}
			r := newHostRepository(t, project.fixture)
			if err := os.WriteFile(filepath.Join(r.root, format.ScanSourceReadme), []byte(project.readme), fileMode); err != nil {
				t.Fatal(err)
			}
			r.commit("add a README")
			r.extra = []string{shallnotVariable + "=" + abs(t, filepath.Join(discpowerDir, recordedShallnot)), recordedReportVar + "=" + abs(t, filepath.Join(discpowerDir, project.report))}
			interleaved(t, r, newScanHost(t, project.scenario.honest(t)))
		})
	}
}

// interleaved runs a scan that asks its host and `prove` in the repository,
// each waiting for the host at the same time and answered between its own
// invocations, and checks that both finish and that no invocation of the
// scan retires a ticket prove waits for.
func interleaved(t *testing.T, r *hostRepository, host *scanHost) {
	t.Helper()
	scanned := r.dp("", scanCommand, hostFlag, noBudget)
	proved := r.dp("", proveCommand, proveAll)
	if scanned.code != exitAwaitingHost || proved.code != exitAwaitingHost {
		t.Fatalf("scan exit %d, prove exit %d:\n%s\n%s", scanned.code, proved.code, scanned.output(), proved.output())
	}
	session := sessionOf(t, scanned.stdout)
	var retired []string
	for runs := 0; (scanned.code == exitAwaitingHost || proved.code == exitAwaitingHost) && runs < maxInterleavedRuns; runs++ {
		if scanned.code == exitAwaitingHost {
			waiting := proveTickets(t, r)
			host.answerSession(r, session)
			scanned = r.dp("", scanCommand, resumeFlag, noBudget)
			still := proveTickets(t, r)
			for _, ticket := range waiting {
				if !slices.Contains(still, ticket) {
					retired = append(retired, ticket)
				}
			}
		}
		if proved.code == exitAwaitingHost {
			if _, err := hostprototest.PlayCLI(proveRunner{r: r}, hostprototest.PersonaHonest, parcel.Oracle{}, func(time.Duration) {}); err != nil {
				t.Fatalf("the prove host failed: %v", err)
			}
			proved = r.dp("", proveCommand, proveAll, resumeFlag)
		}
	}
	t.Run("a scan whose waves interleave with prove's on one repository finishes, and so does prove"+verifiesHostProve, func(t *testing.T) {
		if scanned.code != 0 {
			t.Errorf("scan exit %d:\n%s", scanned.code, scanned.output())
		}
		if proved.code != 0 && proved.code != exitBlocked {
			t.Errorf("prove exit %d:\n%s", proved.code, proved.output())
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			if !strings.Contains(proved.stdout, criterion+" ") {
				t.Errorf("prove printed no verdict for %s:\n%s", criterion, proved.stdout)
			}
		}
	})
	t.Run("no wave of the scan retires a ticket prove waits for"+verifiesHostProve, func(t *testing.T) {
		if len(retired) > 0 {
			t.Fatalf("the scan retired prove's tickets %v", retired)
		}
	})
	t.Run("the scan's statement is admitted with its promise checked"+verifiesHostProve, func(t *testing.T) {
		if predicate := r.statement().Predicate; !predicate.PromisesChecked || len(predicate.Promises) != 1 {
			t.Fatalf("checked %t, %d promises", predicate.PromisesChecked, len(predicate.Promises))
		}
	})
}

// stubGH writes a gh that answers `pr view` with the pull request and `issue
// view` with the issue it closes, and logs its arguments.
func stubGH(t *testing.T, base, head string) (dir, log string) {
	t.Helper()
	dir = t.TempDir()
	log = filepath.Join(dir, "calls")
	pull := map[string]any{
		"number": pullNumber, "title": pullTitle, "body": pullBody, "url": fmt.Sprintf("https://github.com/acme/invoice/pull/%d", pullNumber),
		"baseRefOid": base, "headRefOid": head,
		"closingIssuesReferences": []any{map[string]any{
			"id": "I_node", "number": issueNumber, "url": fmt.Sprintf("https://github.com/acme/invoice/issues/%d", issueNumber),
			"repository": map[string]any{"name": "invoice", "owner": map[string]any{"login": "acme"}},
		}},
	}
	issue := map[string]any{"number": issueNumber, "title": issueTitle, "body": issueBody, "url": fmt.Sprintf("https://github.com/acme/invoice/issues/%d", issueNumber), "state": "OPEN"}
	for name, value := range map[string]any{"pr.json": pull, "issue.json": issue} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(encode(t, value)), fileMode); err != nil {
			t.Fatal(err)
		}
	}
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\n"+
		"if [ \"$1\" = pr ] && [ \"$3\" = %d ]; then cat %q; exit 0; fi\n"+
		"if [ \"$1\" = issue ] && [ \"$3\" = %d ]; then cat %q; exit 0; fi\n"+
		"echo \"unexpected gh $*\" >&2\nexit 9\n",
		log, pullNumber, filepath.Join(dir, "pr.json"), issueNumber, filepath.Join(dir, "issue.json"))
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, log
}

// changelog starts the changelog of the base branch after the pull request
// forked from it.
const changelog = "# Changelog\n\n## Unreleased\n"

func TestScanOfAPullRequestWithAHost(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("no python3: %v", err)
	}
	r := newHostRepository(t, pytestHostFixture)
	forkPoint := r.git("rev-parse", "HEAD")
	r.git("checkout", "-qb", "document-discount")
	totals := filepath.Join(r.root, "invoice", "totals.py")
	content, err := os.ReadFile(totals)
	if err != nil {
		t.Fatal(err)
	}
	documented := strings.Replace(string(content), "def discount(total: int, percent: int) -> int:\n", documentedTotal, 1)
	if err := os.WriteFile(totals, []byte(documented), fileMode); err != nil {
		t.Fatal(err)
	}
	head := r.commit("Document the discount")
	r.git("checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(r.root, "CHANGELOG.md"), []byte(changelog), fileMode); err != nil {
		t.Fatal(err)
	}
	base := r.commit("Start a changelog")
	r.git("checkout", "-q", "document-discount")
	gh, log := stubGH(t, base, head)
	r.path = gh
	host := newScanHost(t, pullScenario().honest(t))
	d := r.driveScan(host, noBudget, strconv.Itoa(pullNumber))

	t.Run("scan <PR> reads the pull request and the issue it closes through gh"+verifiesHostPull, func(t *testing.T) {
		calls, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{fmt.Sprintf("pr view %d ", pullNumber), fmt.Sprintf("issue view %d ", issueNumber)} {
			if !strings.Contains(string(calls), want) {
				t.Errorf("gh was not called with %q:\n%s", want, calls)
			}
		}
	})
	t.Run("the extraction shows the description and the closing issue as sources, byte for byte"+verifiesHostPull, func(t *testing.T) {
		extraction := host.asked[answerPromises]
		if len(extraction) == 0 {
			t.Fatal("no extraction was asked")
		}
		for _, want := range []string{pullBody, issueBody, format.ScanSourceDescription, format.ScanSourceIssue(issueNumber)} {
			if !strings.Contains(extraction[0], strings.TrimSpace(want)) {
				t.Errorf("the extraction does not show %q:\n%s", want, extraction[0])
			}
		}
		saved, err := os.ReadDir(filepath.Join(r.root, ".spec-proof", "scan", head, "sources"))
		if err != nil || len(saved) != 3 {
			t.Fatalf("saved sources %v, %v", saved, err)
		}
	})
	t.Run("the change is scanned from the merge base with a promise of each source checked"+verifiesHostPull, func(t *testing.T) {
		predicate := r.statement().Predicate
		if predicate.Mode != format.ScanModeChange || predicate.Base != forkPoint || !predicate.PromisesChecked {
			t.Fatalf("mode %s, base %s, checked %t", predicate.Mode, predicate.Base, predicate.PromisesChecked)
		}
		sources := map[string]bool{}
		for _, one := range predicate.Promises {
			sources[one.Source.Name] = true
		}
		if !sources[format.ScanSourceDescription] || !sources[format.ScanSourceIssue(issueNumber)] {
			t.Fatalf("promises of %v", sources)
		}
		if !strings.Contains(d.last().stdout, fmt.Sprintf("%q (%s)", zeroQuote, format.ScanSourceDescription)) {
			t.Errorf("the description's promise is not listed:\n%s", d.last().stdout)
		}
	})
}
