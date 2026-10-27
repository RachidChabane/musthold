package analyze

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The freeze tags of the study (PREREG §13).
const (
	TagPrereg = "prereg-loop-1"
	TagInputs = "loop-1-inputs"
)

// Tag is one freeze tag with its commit and the time of its RFC 3161 token.
type Tag struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
	// Time is the token's time; null when the token is not there yet.
	Time *time.Time `json:"time"`
}

// Dropped is a construction domain dropped with its reason.
type Dropped struct {
	Domain string `json:"domain"`
	Reason string `json:"reason"`
}

// Provenance is what the report's provenance section shows.
type Provenance struct {
	Tags         []Tag     `json:"tags"`
	InputsDigest string    `json:"inputsDigest"`
	Tasks        []string  `json:"tasks"`
	Dropped      []Dropped `json:"dropped"`
}

// AttemptEntry is one attempt of a run.
type AttemptEntry struct {
	Number    int           `json:"number"`
	Outcome   study.Outcome `json:"outcome"`
	Reason    string        `json:"reason,omitempty"`
	StartedAt time.Time     `json:"startedAt"`
	EndedAt   time.Time     `json:"endedAt"`
}

// RunEntry is one run of the order with its attempts, its state and, when it is
// counted, its facts.
type RunEntry struct {
	Name       string         `json:"name"`
	Task       string         `json:"task"`
	Arm        string         `json:"arm"`
	Repetition int            `json:"repetition"`
	State      study.Outcome  `json:"state"`
	Attempts   []AttemptEntry `json:"attempts"`
	Facts      *facts.Facts   `json:"facts"`
}

// Population is the counted runs, the void attempts and the runs without a
// counted attempt, per arm and per cause (PREREG §10.1).
type Population struct {
	Counted map[string]int                   `json:"counted"`
	Void    map[string]map[study.Outcome]int `json:"void"`
	Missing map[string]map[study.Outcome]int `json:"missing"`
}

// Window is the run phase as it ended.
type Window struct {
	FirstStart time.Time `json:"firstStart"`
	Cap        time.Time `json:"cap"`
	CapReached bool      `json:"capReached"`
}

// Deviations is the deviation log by kind, and how many protocol entries touch
// the runs of each analysis.
type Deviations struct {
	File   string             `json:"file"`
	Counts map[study.Kind]int `json:"counts"`
}

// Analysis is H1, H2, S1 and S2 on every counted run, with the protocol
// deviations whose scope touches them.
type Analysis struct {
	Core
	ProtocolDeviations int `json:"protocolDeviations"`
}

// Sensitivities is the sensitivity analyses, each with the protocol deviations
// whose scope touches it.
type Sensitivities struct {
	Sensitivity
	ProtocolDeviations map[string]int `json:"protocolDeviations"`
}

// Summary is results/summary.json.
type Summary struct {
	Provenance  Provenance    `json:"provenance"`
	Window      Window        `json:"window"`
	Population  Population    `json:"population"`
	Analysis    Analysis      `json:"analysis"`
	Secondary   Secondary     `json:"secondary"`
	Sensitivity Sensitivities `json:"sensitivity"`
	Runs        []RunEntry    `json:"runs"`
	Deviations  Deviations    `json:"deviations"`
	// Digests maps inputs.json and every record read, by its path in the work
	// directory, to its SHA-256.
	Digests map[string]string `json:"digests"`
}

// The labels of the sensitivity populations.
const (
	SubsetWithoutInSample = "withoutInSample"
	SubsetWithoutHits     = "withoutHits"
	SubsetWithoutCapped   = "withoutCapped"
)

// BuildInput is what the summary is built from.
type BuildInput struct {
	Phase phase.Phase
	// Facts holds the facts of each counted run, by run name.
	Facts      map[string]facts.Facts
	Deviations []study.Deviation
	Provenance Provenance
	Digests    map[string]string
}

// Build builds the summary. It is pure.
func Build(in BuildInput) Summary {
	summary := Summary{
		Provenance: in.Provenance,
		Window:     Window{FirstStart: in.Phase.FirstStart, Cap: in.Phase.Cap, CapReached: in.Phase.CapReached},
		Population: Population{Counted: map[string]int{}, Void: map[string]map[study.Outcome]int{}, Missing: map[string]map[study.Outcome]int{}},
		Deviations: Deviations{File: study.DeviationsFile, Counts: map[study.Kind]int{}},
		Digests:    in.Digests,
		Runs:       []RunEntry{},
	}
	for _, arm := range Arms() {
		summary.Population.Void[arm] = map[study.Outcome]int{}
		summary.Population.Missing[arm] = map[study.Outcome]int{}
	}
	var counted []facts.Facts
	for _, state := range in.Phase.Runs {
		entry := runEntry(state, in.Facts)
		summary.Runs = append(summary.Runs, entry)
		tally(&summary.Population, entry)
		if entry.Facts != nil {
			counted = append(counted, *entry.Facts)
		}
	}
	for _, kind := range study.Kinds() {
		summary.Deviations.Counts[kind] = 0
	}
	for _, deviation := range in.Deviations {
		summary.Deviations.Counts[deviation.Kind]++
	}
	summary.Analysis = Analysis{Core: core(counted), ProtocolDeviations: touching(in.Deviations, counted)}
	summary.Secondary = secondary(counted)
	summary.Sensitivity = Sensitivities{Sensitivity: sensitivity(counted), ProtocolDeviations: map[string]int{
		SubsetWithoutInSample: touching(in.Deviations, filter(counted, func(run facts.Facts) bool { return run.Task != InSampleTask })),
		SubsetWithoutHits:     touching(in.Deviations, filter(counted, func(run facts.Facts) bool { return !run.HasHit() })),
		SubsetWithoutCapped:   touching(in.Deviations, filter(counted, func(run facts.Facts) bool { return !run.Capped })),
	}}
	return summary
}

func runEntry(state phase.RunState, all map[string]facts.Facts) RunEntry {
	task, arm, repetition := splitName(state.Name)
	entry := RunEntry{Name: state.Name, Task: task, Arm: arm, Repetition: repetition, State: state.State, Attempts: []AttemptEntry{}}
	for _, attempt := range state.Attempts {
		entry.Attempts = append(entry.Attempts, AttemptEntry{
			Number: attempt.Number, Outcome: attempt.Run.Outcome, Reason: attempt.Run.Reason,
			StartedAt: attempt.Run.StartedAt, EndedAt: attempt.Run.EndedAt,
		})
	}
	if found, ok := all[state.Name]; ok && state.Counted >= 0 {
		entry.Task, entry.Arm, entry.Repetition = found.Task, found.Arm, found.Repetition
		entry.Facts = &found
	}
	return entry
}

// nameSeparator joins a run's task, arm and repetition.
const nameSeparator = "-"

// splitName reads <task>-<arm>-<repetition> from the right, since a task's name
// may hold the separator.
func splitName(name string) (task, arm string, repetition int) {
	parts := strings.Split(name, nameSeparator)
	if len(parts) < 3 {
		return name, "", 0
	}
	repetition, _ = strconv.Atoi(parts[len(parts)-1])
	return strings.Join(parts[:len(parts)-2], nameSeparator), parts[len(parts)-2], repetition
}

func tally(population *Population, entry RunEntry) {
	for _, attempt := range entry.Attempts {
		if attempt.Outcome.Void() {
			bump(population.Void, entry.Arm, attempt.Outcome)
		}
	}
	switch {
	case entry.Facts != nil:
		population.Counted[entry.Arm]++
	case entry.State != "":
		bump(population.Missing, entry.Arm, entry.State)
	}
}

func bump(counts map[string]map[study.Outcome]int, arm string, outcome study.Outcome) {
	if counts[arm] == nil {
		counts[arm] = map[study.Outcome]int{}
	}
	counts[arm][outcome]++
}

// touching counts the protocol deviations whose scope is all, or names a run or
// a task of the runs.
func touching(deviations []study.Deviation, runs []facts.Facts) int {
	n := 0
	for _, deviation := range deviations {
		if deviation.Kind != study.KindProtocol {
			continue
		}
		for _, scope := range deviation.Scope {
			if scope == study.ScopeAll || slices.ContainsFunc(runs, func(run facts.Facts) bool { return run.Run == scope || run.Task == scope }) {
				n++
				break
			}
		}
	}
	return n
}
