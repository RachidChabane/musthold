package analysis

import (
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/catch"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// The freeze tags of the study (ADR 0017, "Steps and stops").
const (
	TagPlan   = "plan-loop-2"
	TagPrereg = "prereg-loop-2"
	TagInputs = "loop-2-inputs"
)

// Provenance is what the report's provenance section shows.
type Provenance struct {
	Tags         []analyze.Tag `json:"tags"`
	InputsDigest string        `json:"inputsDigest"`
	Tasks        []string      `json:"tasks"`
}

// RunEntry is one run of the order with its attempts, its state and, when it
// is counted, loop-1's facts and what loop-2 adds.
type RunEntry struct {
	analyze.RunEntry
	FirstStop    *record.Grade        `json:"firstStop,omitempty"`
	Refusal      *record.Refusal      `json:"refusal,omitempty"`
	Adjudication *record.Adjudication `json:"adjudication,omitempty"`
}

// Analysis is every endpoint of the study on the counted runs.
type Analysis struct {
	Feasibility Feasibility `json:"feasibility"`
	Primary     Primary     `json:"primary"`
	// MutationOnly is the outcome with every spec-witness catch counted as an
	// escape.
	MutationOnly   Primary        `json:"mutationOnly"`
	Catches        catch.Summary  `json:"catches"`
	H2             H2             `json:"h2"`
	Gate           Gate           `json:"gate"`
	Readjudication Readjudication `json:"readjudication"`
	// Kept is loop-1's S1 to S8 and its sign tests over tasks.
	Kept analyze.Kept `json:"kept"`
}

// Summary is results/summary.json.
type Summary struct {
	Provenance Provenance         `json:"provenance"`
	Window     analyze.Window     `json:"window"`
	Population analyze.Population `json:"population"`
	Analysis   Analysis           `json:"analysis"`
	Runs       []RunEntry         `json:"runs"`
	Deviations analyze.Deviations `json:"deviations"`
	// Digests maps inputs.json and every record read, by its path in the work
	// directory, to its SHA-256.
	Digests map[string]string `json:"digests"`
}

// Analyse computes every endpoint on counted runs, the run phase having ended
// at its calendar cap or not.
func Analyse(capReached bool, runs []record.Run) Analysis {
	feasibility := Feasible(capReached, runs)
	primary := feasibility.apply(DecidePrimary(runs))
	readjudication := Readjudicate(runs)
	readjudication.Sensitivity = feasibility.apply(readjudication.Sensitivity)
	return Analysis{
		Feasibility:    feasibility,
		Primary:        primary,
		MutationOnly:   feasibility.apply(DecidePrimary(catch.MutationOnly(runs))),
		Catches:        catch.Summarise(catch.Find(runs), primary.Outcome),
		H2:             DecideH2(runs),
		Gate:           SetGate(runs),
		Readjudication: readjudication,
		Kept:           analyze.KeptEndpoints(factsOf(runs)),
	}
}

// BuildInput is what the summary is built from.
type BuildInput struct {
	Phase phase.Phase
	// Records holds the record of each counted run, by run name.
	Records    map[string]record.Run
	Deviations []study.Deviation
	Provenance Provenance
	Digests    map[string]string
}

// Build builds the summary.
func Build(in BuildInput) Summary {
	all := map[string]facts.Facts{}
	for name, run := range in.Records {
		all[name] = run.Facts
	}
	entries, population := analyze.Census(in.Phase, all)
	summary := Summary{
		Provenance: in.Provenance, Window: analyze.WindowOf(in.Phase), Population: population,
		Runs: make([]RunEntry, 0, len(entries)), Deviations: analyze.CountDeviations(in.Deviations), Digests: in.Digests,
	}
	var counted []record.Run
	for _, entry := range entries {
		wrapped := RunEntry{RunEntry: entry}
		if run, found := in.Records[entry.Name]; found && entry.Facts != nil {
			wrapped.FirstStop, wrapped.Refusal, wrapped.Adjudication = run.FirstStop, &run.Refusal, run.Adjudication
			counted = append(counted, run)
		}
		summary.Runs = append(summary.Runs, wrapped)
	}
	summary.Analysis = Analyse(in.Phase.CapReached, counted)
	return summary
}
