package analyze

import (
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// S3 is the yield and the false rejection of one arm (PREREG §9.4).
type S3 struct {
	Yield          Rate `json:"yield"`
	FalseRejection Rate `json:"falseRejection"`
}

// S4 is the blocks of one arm (PREREG §9.5).
type S4 struct {
	Runs int `json:"runs"`
	// Blocks counts the blocks of each kind; RunsWith the runs with one.
	Blocks       map[facts.BlockKind]int      `json:"blocks"`
	RunsWith     map[facts.BlockKind]Fraction `json:"runsWith"`
	GiveUps      int                          `json:"giveUps"`
	FindingCodes map[string]int               `json:"findingCodes"`
}

// The cost measures of S6 (PREREG §9.9).
const (
	MeasureMessages    = "messages"
	MeasureExecutors   = "executors"
	MeasureProves      = "prove commands"
	MeasureNotionalUSD = "notional USD"
	MeasureTokens      = "tokens"
	MeasureWall        = "wall seconds"
	MeasureGate        = "gate seconds"
)

// Measures lists the cost measures in order.
func Measures() []string {
	return []string{MeasureMessages, MeasureExecutors, MeasureProves, MeasureNotionalUSD, MeasureTokens, MeasureWall, MeasureGate}
}

// measure reads one cost measure of a run.
func measure(run facts.Facts, name string) float64 {
	cost := run.Cost
	switch name {
	case MeasureMessages:
		return float64(cost.Messages)
	case MeasureExecutors:
		return float64(cost.Executors)
	case MeasureProves:
		return float64(cost.ProveCommands)
	case MeasureNotionalUSD:
		if cost.NotionalUSD == nil {
			return 0
		}
		return *cost.NotionalUSD
	case MeasureTokens:
		total := int64(0)
		for _, tokens := range cost.Tokens {
			total += tokens.Total()
		}
		return float64(total)
	case MeasureWall:
		return cost.WallSeconds
	default:
		return cost.GateSeconds
	}
}

// CostMeasure is one measure of one arm: its median and maximum per run, and
// the arm's sum over its accepted runs, null with no accepted run.
type CostMeasure struct {
	Measure     string   `json:"measure"`
	Median      float64  `json:"median"`
	Max         float64  `json:"max"`
	PerAccepted *float64 `json:"perAccepted"`
}

// CostTask is one task's mean cost per run in the treatment arm minus the
// control arm, by measure.
type CostTask struct {
	Task        string             `json:"task"`
	Differences map[string]float64 `json:"differences"`
}

// S6 is the cost per arm and per task.
type S6 struct {
	PerArm  map[string][]CostMeasure `json:"perArm"`
	PerTask []CostTask               `json:"perTask"`
}

// VoluntaryRun is one control run's voluntary proofs.
type VoluntaryRun struct {
	Run      string   `json:"run"`
	Calls    int      `json:"calls"`
	Findings []string `json:"findings"`
}

// S7 is the voluntary proofs of the control arm (PREREG §9.10).
type S7 struct {
	Runs Fraction       `json:"runs"`
	List []VoluntaryRun `json:"list"`
}

// CriterionRow is one criterion's violations per arm in one task.
type CriterionRow struct {
	Task      string   `json:"task"`
	Criterion string   `json:"criterion"`
	Control   Fraction `json:"control"`
	Treatment Fraction `json:"treatment"`
}

// Secondary is S3 to S8; S1 and S2 are computed with the primary endpoint.
type Secondary struct {
	S3 map[string]S3                          `json:"s3"`
	S4 map[string]S4                          `json:"s4"`
	S5 map[string]map[study.Detector]Fraction `json:"s5"`
	S6 S6                                     `json:"s6"`
	S7 S7                                     `json:"s7"`
	S8 []CriterionRow                         `json:"s8"`
}

// secondary computes S3 to S8 on counted runs.
func secondary(runs []facts.Facts) Secondary {
	arms := byArm(runs)
	found := Secondary{S3: map[string]S3{}, S4: map[string]S4{}, S5: map[string]map[study.Detector]Fraction{}, S6: s6(runs), S7: s7(arms[ArmControl]), S8: s8(runs)}
	for _, arm := range Arms() {
		list := arms[arm]
		found.S3[arm] = s3(list)
		found.S4[arm] = s4(list)
		detectors := map[study.Detector]Fraction{}
		for _, detector := range study.Detectors() {
			detectors[detector] = frac(count(list, func(run facts.Facts) bool { return run.Hits[detector] > 0 }), len(list))
		}
		found.S5[arm] = detectors
	}
	return found
}

func s3(runs []facts.Facts) S3 {
	clean := filter(runs, func(run facts.Facts) bool { return !run.Defective })
	return S3{
		Yield:          rate(count(runs, func(run facts.Facts) bool { return run.Accepted }), len(runs)),
		FalseRejection: rate(count(clean, func(run facts.Facts) bool { return !run.Accepted }), len(clean)),
	}
}

func s4(runs []facts.Facts) S4 {
	found := S4{Runs: len(runs), Blocks: map[facts.BlockKind]int{}, RunsWith: map[facts.BlockKind]Fraction{}, FindingCodes: map[string]int{}}
	for _, kind := range facts.BlockKinds() {
		found.RunsWith[kind] = frac(count(runs, func(run facts.Facts) bool { return run.BlockCounts[kind] > 0 }), len(runs))
		found.Blocks[kind] = 0
	}
	for _, run := range runs {
		for kind, n := range run.BlockCounts {
			found.Blocks[kind] += n
		}
		for code, n := range run.FindingCodes {
			found.FindingCodes[code] += n
		}
		found.GiveUps += run.GiveUps
	}
	return found
}

// median is the middle value, or the mean of the two middle values; 0 with no
// value.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func values(runs []facts.Facts, name string) []float64 {
	found := make([]float64, 0, len(runs))
	for _, run := range runs {
		found = append(found, measure(run, name))
	}
	return found
}

func s6(runs []facts.Facts) S6 {
	arms := byArm(runs)
	found := S6{PerArm: map[string][]CostMeasure{}}
	for _, arm := range Arms() {
		list := arms[arm]
		accepted := count(list, func(run facts.Facts) bool { return run.Accepted })
		for _, name := range Measures() {
			all := values(list, name)
			row := CostMeasure{Measure: name, Median: median(all), Max: slices.Max(append([]float64{0}, all...))}
			if accepted > 0 {
				perAccepted := mean(all) * float64(len(all)) / float64(accepted)
				row.PerAccepted = &perAccepted
			}
			found.PerArm[arm] = append(found.PerArm[arm], row)
		}
	}
	split, tasks := byTask(runs)
	for _, task := range tasks {
		taskArms := byArm(split[task])
		row := CostTask{Task: task, Differences: map[string]float64{}}
		for _, name := range Measures() {
			row.Differences[name] = mean(values(taskArms[ArmTreatment], name)) - mean(values(taskArms[ArmControl], name))
		}
		found.PerTask = append(found.PerTask, row)
	}
	return found
}

func s7(control []facts.Facts) S7 {
	found := S7{List: []VoluntaryRun{}}
	for _, run := range control {
		if len(run.Voluntary) == 0 {
			continue
		}
		entry := VoluntaryRun{Run: run.Run, Calls: len(run.Voluntary), Findings: []string{}}
		for _, call := range run.Voluntary {
			for _, code := range call.Findings {
				if !slices.Contains(entry.Findings, code) {
					entry.Findings = append(entry.Findings, code)
				}
			}
		}
		slices.Sort(entry.Findings)
		found.List = append(found.List, entry)
	}
	found.Runs = frac(len(found.List), len(control))
	return found
}

func s8(runs []facts.Facts) []CriterionRow {
	split, tasks := byTask(runs)
	rows := []CriterionRow{}
	for _, task := range tasks {
		var criteria []string
		for _, run := range split[task] {
			for _, criterion := range run.Violated {
				if !slices.Contains(criteria, criterion) {
					criteria = append(criteria, criterion)
				}
			}
		}
		slices.Sort(criteria)
		arms := byArm(split[task])
		for _, criterion := range criteria {
			violates := func(run facts.Facts) bool { return slices.Contains(run.Violated, criterion) }
			rows = append(rows, CriterionRow{
				Task: task, Criterion: criterion,
				Control:   frac(count(arms[ArmControl], violates), len(arms[ArmControl])),
				Treatment: frac(count(arms[ArmTreatment], violates), len(arms[ArmTreatment])),
			})
		}
	}
	return rows
}

// InSampleTask is the task built for the demonstration of the loop, which the
// sensitivity analyses leave out (PREREG §3.2, §10.5).
const InSampleTask = "boxoffice"

// Sensitivity is the analyses of PREREG §10.5.
type Sensitivity struct {
	WithoutInSample Core `json:"withoutInSample"`
	WithoutHits     Core `json:"withoutHits"`
	WithoutCapped   Core `json:"withoutCapped"`
	// SignH1 and SignS1 test the per-task difference of escape and defect shares,
	// control minus treatment; SignH2 the per-task majority of corrected runs.
	SignH1 Sign `json:"signH1"`
	SignS1 Sign `json:"signS1"`
	SignH2 Sign `json:"signH2"`
}

func sensitivity(runs []facts.Facts) Sensitivity {
	return Sensitivity{
		WithoutInSample: core(filter(runs, func(run facts.Facts) bool { return run.Task != InSampleTask })),
		WithoutHits:     core(filter(runs, func(run facts.Facts) bool { return !run.HasHit() })),
		WithoutCapped:   core(filter(runs, func(run facts.Facts) bool { return !run.Capped })),
		SignH1:          sign(shareDifferences(runs, func(run facts.Facts) bool { return run.Escaped })),
		SignS1:          sign(shareDifferences(runs, func(run facts.Facts) bool { return run.Defective })),
		SignH2:          sign(majorityDifferences(runs)),
	}
}
