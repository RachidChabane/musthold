package hostproto_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"pgregory.net/rapid"
)

const (
	violates   = "VIOLATES"
	consistent = "CONSISTENT"
)

// classifyTester is what the helpers need of a test, so that a property test can use them.
type classifyTester interface {
	Helper()
	Fatal(args ...any)
}

var classifyRequestID = hostprototest.MustID(hostprototest.ClosedRequest())

func classifySource(t classifyTester) format.ClassifySource {
	t.Helper()
	source, err := format.SingleVoterClassifySource(hostproto.ClassifyCalls, hostproto.ClassifyScheme, hostprototest.ClassifySessionModel)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// reportWith builds the report of the closed fixture whose calls return the
// labels in order, an empty one being a call without a label.
func reportWith(t classifyTester, labels ...string) hostproto.ClassifyReport {
	t.Helper()
	set := hostprototest.ClosedSet()
	plan, err := set.ClassifyPlan()
	if err != nil {
		t.Fatal(err)
	}
	calls, err := plan.Calls(classifySource(t))
	if err != nil {
		t.Fatal(err)
	}
	report := hostproto.ClassifyReport{CLIVersion: hostprototest.ClassifyCLIVersion, SessionModel: hostprototest.ClassifySessionModel}
	for index, call := range calls {
		reported := hostproto.ReportedCall{Question: call.Question, Index: call.Index, Model: call.Model, Attempts: 1}
		if labels[index] != "" {
			label := labels[index]
			reported.Label = &label
		} else {
			reported.Attempts = hostproto.MaxClassifyAttempts
		}
		report.Calls = append(report.Calls, reported)
	}
	return report
}

func readLabels(t classifyTester, labels ...string) hostproto.ClassifyReading {
	t.Helper()
	reading, err := hostproto.ReadClassification(classifyRequestID, hostprototest.ClosedSet(), classifySource(t), reportWith(t, labels...), hostprototest.ClassifyTable())
	if err != nil {
		t.Fatal(err)
	}
	return reading
}

func TestReadClassification(t *testing.T) {
	cases := []struct {
		name       string
		labels     []string
		modal      string
		votes      int
		confidence float64
		gate       hostproto.ClassifyGate
		tied       bool
	}{
		{"five of five", []string{violates, violates, violates, violates, violates}, violates, 5, 0.99, hostproto.GateOpen, false},
		{"four of five", []string{violates, consistent, violates, violates, violates}, violates, 4, 0.96, hostproto.GateOpen, false},
		{"three of five stays below tau", []string{violates, consistent, violates, consistent, violates}, violates, 3, 0.90, hostproto.GateBelowTau, false},
		{"a missing call counts against the share", []string{violates, violates, violates, violates, ""}, violates, 4, 0.96, hostproto.GateOpen, false},
		{"two missing calls leave three", []string{consistent, consistent, consistent, "", ""}, consistent, 3, 0.90, hostproto.GateBelowTau, false},
		{"a tie has no prediction", []string{violates, violates, consistent, consistent, ""}, "", 2, 0, hostproto.GateTie, true},
		{"two against two and none", []string{consistent, consistent, violates, violates, ""}, "", 2, 0, hostproto.GateTie, true},
		{"no label at all", []string{"", "", "", "", ""}, "", 0, 0, hostproto.GateNoVotes, false},
		{"a count below the table is uncalibrated", []string{violates, "", "", "", ""}, violates, 1, 0, hostproto.GateUncalibrated, false},
	}
	for _, tt := range cases {
		t.Run(tt.name+" [verifies SP-1905~1]", func(t *testing.T) {
			reading := readLabels(t, tt.labels...)
			got := reading.Questions[0]
			if got.Modal != tt.modal || got.Votes != tt.votes || got.Tied != tt.tied || got.Confidence != tt.confidence || reading.Gate != tt.gate {
				t.Fatalf("reading %+v gate %q", got, reading.Gate)
			}
			if wantShare := float64(tt.votes) / hostproto.ClassifyCalls; got.Share != wantShare {
				t.Fatalf("share %v, want %v", got.Share, wantShare)
			}
			if reading.Answers() != (tt.gate == hostproto.GateOpen) {
				t.Fatalf("answers %v for the gate %q", reading.Answers(), reading.Gate)
			}
			if question := reading.Record.Questions[0]; question.Modal != got.Modal || question.Votes != got.Votes || question.Tied != got.Tied || question.Confidence != got.Confidence {
				t.Fatalf("record %+v", question)
			}
		})
	}

	t.Run("the gate of a request with several questions is the first closed one [verifies SP-1905~1]", func(t *testing.T) {
		set := hostproto.ClosedSet{State: "s", Questions: []hostproto.ClosedQuestion{
			hostproto.ChoiceQuestion("a", "A?", options("X", "Y")),
			hostproto.NoulQuestion("b", "B?", "yes", "no"),
		}}
		plan, _ := set.ClassifyPlan()
		calls, _ := plan.Calls(classifySource(t))
		report := hostproto.ClassifyReport{CLIVersion: hostprototest.ClassifyCLIVersion, SessionModel: hostprototest.ClassifySessionModel}
		for _, call := range calls {
			label := call.Labels[0]
			if call.Question == "a" {
				label = "X"
			} else if call.Index >= 3 {
				label = call.Labels[1]
			}
			report.Calls = append(report.Calls, hostproto.ReportedCall{Question: call.Question, Index: call.Index, Model: call.Model, Label: &label, Attempts: 1})
		}
		reading, err := hostproto.ReadClassification(classifyRequestID, set, classifySource(t), report, hostprototest.ClassifyTable())
		if err != nil {
			t.Fatal(err)
		}
		if reading.Gate != hostproto.GateBelowTau || !reading.Questions[0].Predicts() {
			t.Fatalf("gate %q, first question %+v", reading.Gate, reading.Questions[0])
		}
		if _, err := reading.Output(set); !errors.Is(err, hostproto.ErrClassifyReport) {
			t.Fatalf("a closed gate rendered an output: %v", err)
		}
	})

	t.Run("an open gate renders the modal label with the table's confidence and the rest of the probability shared [verifies SP-1905~1]", func(t *testing.T) {
		set := hostprototest.ClosedSet()
		reading := readLabels(t, violates, violates, violates, violates, consistent)
		output, err := reading.Output(set)
		if err != nil {
			t.Fatal(err)
		}
		answers, err := hostproto.ParseClosedAnswers(output)
		answer := answers[hostprototest.ClosedQuestionID]
		if err != nil || answer.Choice != violates || !answer.HasConfidence || answer.Confidence != 0.96 {
			t.Fatalf("answer %+v (%v) from %s", answer, err, output)
		}
		if p, given := answer.ProbabilityOf(violates); !given || p != 0.96 {
			t.Fatalf("probability %v %v", p, given)
		}
		if p, _ := answer.ProbabilityOf(consistent); p < 0.0399 || p > 0.0401 {
			t.Fatalf("the rest %v", p)
		}
		if schema, _ := set.ResponseSchema(); len(schema) == 0 {
			t.Fatal("no schema")
		}
	})

	t.Run("the share and the modal label do not depend on which calls returned them [verifies SP-1905~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			labels := rapid.SliceOfN(rapid.SampledFrom([]string{violates, consistent, ""}), hostproto.ClassifyCalls, hostproto.ClassifyCalls).Draw(t, "labels")
			shuffled := rapid.Permutation(labels).Draw(t, "shuffled")
			first := readLabels(t, labels...)
			second := readLabels(t, shuffled...)
			a, b := first.Questions[0], second.Questions[0]
			if a.Modal != b.Modal || a.Votes != b.Votes || a.Tied != b.Tied || first.Gate != second.Gate {
				t.Fatalf("%v gave %+v, %v gave %+v", labels, a, shuffled, b)
			}
		})
	})

	t.Run("a prediction is never made without a unique mode and a calibrated count [verifies SP-1905~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			labels := rapid.SliceOfN(rapid.SampledFrom([]string{violates, consistent, ""}), hostproto.ClassifyCalls, hostproto.ClassifyCalls).Draw(t, "labels")
			reading := readLabels(t, labels...)
			counts := map[string]int{}
			for _, label := range labels {
				if label != "" {
					counts[label]++
				}
			}
			unique := counts[violates] != counts[consistent]
			top := max(counts[violates], counts[consistent])
			if reading.Answers() && (!unique || top < 4) {
				t.Fatalf("%v answered with counts %v", labels, counts)
			}
			if !reading.Answers() && unique && top >= 4 {
				t.Fatalf("%v did not answer with counts %v", labels, counts)
			}
		})
	})
}

func TestMatchReportToPlan(t *testing.T) {
	good := reportWith(t, violates, violates, violates, violates, violates)
	read := func(mutate func(*hostproto.ClassifyReport)) error {
		report := good
		report.Calls = slices.Clone(good.Calls)
		mutate(&report)
		_, err := hostproto.ReadClassification(classifyRequestID, hostprototest.ClosedSet(), classifySource(t), report, hostprototest.ClassifyTable())
		return err
	}

	refusals := map[string]func(*hostproto.ClassifyReport){
		"a missing call":           func(r *hostproto.ClassifyReport) { r.Calls = r.Calls[:4] },
		"an extra call":            func(r *hostproto.ClassifyReport) { r.Calls = append(r.Calls, r.Calls[0]) },
		"a call reported twice":    func(r *hostproto.ClassifyReport) { r.Calls[4] = r.Calls[3] },
		"a call of another model":  func(r *hostproto.ClassifyReport) { r.Calls[0].Model = "claude-haiku-4-5-20251001" },
		"a label outside the call": func(r *hostproto.ClassifyReport) { other := "MAYBE"; r.Calls[0].Label = &other },
		"zero attempts":            func(r *hostproto.ClassifyReport) { r.Calls[0].Attempts = 0 },
		"three attempts":           func(r *hostproto.ClassifyReport) { r.Calls[0].Attempts = 3 },
		"a call of another question": func(r *hostproto.ClassifyReport) {
			r.Calls[0].Question = "other"
		},
	}
	for name, mutate := range refusals {
		t.Run("a report with "+name+" is refused [verifies SP-1906~1]", func(t *testing.T) {
			if err := read(mutate); !errors.Is(err, hostproto.ErrClassifyReport) {
				t.Fatalf("error %v", err)
			}
		})
	}

	t.Run("the order of the calls in the report does not matter [verifies SP-1906~1]", func(t *testing.T) {
		if err := read(func(r *hostproto.ClassifyReport) { slices.Reverse(r.Calls) }); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the record holds every call with its labels in the planned order, the seed input and the host version [verifies SP-1906~1]", func(t *testing.T) {
		reading := readLabels(t, violates, violates, "", violates, violates)
		record := reading.Record
		plan, _ := hostprototest.ClosedSet().ClassifyPlan()
		question := plan.Questions[0]
		if len(record.Calls) != hostproto.ClassifyCalls || record.CLIVersion != hostprototest.ClassifyCLIVersion || record.SessionModel != hostprototest.ClassifySessionModel {
			t.Fatalf("record %+v", record)
		}
		for index, call := range record.Calls {
			if !slices.Equal(call.Order, question.Calls[index].Labels) || call.Index != index {
				t.Errorf("call %+v", call)
			}
		}
		if record.Calls[2].Label != nil || record.Calls[2].Attempts != hostproto.MaxClassifyAttempts {
			t.Errorf("the missing call %+v", record.Calls[2])
		}
		if record.Questions[0].SeedInput != question.SeedInput || record.Questions[0].Seed != question.Seed || record.ClosedSetDigest != plan.ClosedSetDigest {
			t.Errorf("seed %+v digest %s", record.Questions[0], record.ClosedSetDigest)
		}
		digest, _ := hostprototest.ClassifyTable().Digest()
		if record.Mapping != "host-classify.share-isotonic@"+digest || record.Source != classifySource(t).Source() || record.RequestID != string(classifyRequestID) {
			t.Errorf("mapping %q source %q request %q", record.Mapping, record.Source, record.RequestID)
		}
	})

	t.Run("a report with an unknown member, or without a host version or session model, is refused [verifies SP-1906~1]", func(t *testing.T) {
		encoded, _ := json.Marshal(good)
		withExtra := strings.Replace(string(encoded), `"calls"`, `"extra":1,"calls"`, 1)
		withoutVersion := strings.Replace(string(encoded), hostprototest.ClassifyCLIVersion, "", 1)
		withoutModel := strings.Replace(string(encoded), `"sessionModel":"`+hostprototest.ClassifySessionModel+`"`, `"sessionModel":""`, 1)
		for name, document := range map[string]string{"extra": withExtra, "version": withoutVersion, "model": withoutModel, "text": "not json"} {
			if _, err := hostproto.ParseClassifyReport([]byte(document)); !errors.Is(err, hostproto.ErrClassifyReport) {
				t.Errorf("%s: error %v", name, err)
			}
		}
		if _, err := hostproto.ParseClassifyReport(encoded); err != nil {
			t.Fatal(err)
		}
	})
}

func TestShareTableAndAdmission(t *testing.T) {
	table := hostprototest.ClassifyTable()

	t.Run("a table gives a count the confidence of the greatest point at or below it and none below the first [verifies SP-1904~1]", func(t *testing.T) {
		for count, want := range map[int]float64{3: 0.90, 4: 0.96, 5: 0.99} {
			if got, found := table.ConfidenceAt(count); !found || got != want {
				t.Errorf("count %d: %v %v", count, got, found)
			}
		}
		if _, found := table.ConfidenceAt(2); found {
			t.Error("a count below the first point has a confidence")
		}
		if got, _ := (hostproto.ShareTable{K: 5, Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 2, Confidence: 0.5}, {Count: 5, Confidence: 0.9}}}).ConfidenceAt(4); got != 0.5 {
			t.Errorf("a step table gave %v", got)
		}
	})

	invalid := map[string]hostproto.ShareTable{
		"no point":                    {K: 5, Scheme: "rot1"},
		"counts that do not increase": {K: 5, Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 3, Confidence: 0.5}, {Count: 3, Confidence: 0.6}}},
		"a count above k":             {K: 5, Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 6, Confidence: 0.5}}},
		"a count of zero":             {K: 5, Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 0, Confidence: 0.5}}},
		"a falling confidence":        {K: 5, Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 3, Confidence: 0.9}, {Count: 4, Confidence: 0.8}}},
		"a confidence above one":      {K: 5, Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 3, Confidence: 1.1}}},
		"an unknown scheme":           {K: 5, Scheme: "rot9", Points: []hostproto.SharePoint{{Count: 3, Confidence: 0.5}}},
		"no calls":                    {Scheme: "rot1", Points: []hostproto.SharePoint{{Count: 1, Confidence: 0.5}}},
	}
	for name, bad := range invalid {
		t.Run("a table with "+name+" is refused [verifies SP-1904~1]", func(t *testing.T) {
			if err := bad.Validate(); !errors.Is(err, hostproto.ErrClassifyTable) {
				t.Fatalf("error %v", err)
			}
		})
	}

	t.Run("a table is identified by the digest of its canonical form [verifies SP-1904~1]", func(t *testing.T) {
		a, _ := table.Digest()
		other := table
		other.Points = slices.Clone(table.Points)
		other.Points[0].Confidence = 0.91
		b, _ := other.Digest()
		again, _ := hostprototest.ClassifyTable().Digest()
		if a == b || a != again || !format.IsSHA256Hex(a) {
			t.Fatalf("digests %s %s %s", a, b, again)
		}
	})

	t.Run("the shipped admission table holds no entry [verifies SP-1904~1]", func(t *testing.T) {
		admission, err := hostproto.ShippedClassifyAdmission()
		if err != nil || len(admission.Entries) != 0 {
			t.Fatalf("%+v %v", admission, err)
		}
		key := hostproto.ClassifyAdmissionKey{Kind: format.KindAlignment, Source: classifySource(t).Source(), Model: hostprototest.ClassifySessionModel}
		if _, admitted := admission.Lookup(key); admitted {
			t.Fatal("the shipped table admits a classification")
		}
	})

	t.Run("an admission table admits the entry's kind, exact source and session model and nothing else [verifies SP-1904~1]", func(t *testing.T) {
		source := classifySource(t)
		admission := hostprototest.ClassifyAdmissionOf(t, format.KindAlignment, source, hostprototest.ClassifySessionModel)
		key := hostproto.ClassifyAdmissionKey{Kind: format.KindAlignment, Source: source.Source(), Model: hostprototest.ClassifySessionModel}
		if _, admitted := admission.Lookup(key); !admitted {
			t.Fatal("the entry does not admit its own key")
		}
		for name, other := range map[string]hostproto.ClassifyAdmissionKey{
			"another kind":          {Kind: format.KindCompleteness, Source: key.Source, Model: key.Model},
			"another source":        {Kind: key.Kind, Source: "host-classify(5,rot1):claude-haiku-4-5-20251001*5", Model: key.Model},
			"another session model": {Kind: key.Kind, Source: key.Source, Model: "claude-opus-5-5"},
		} {
			if _, admitted := admission.Lookup(other); admitted {
				t.Errorf("%s is admitted", name)
			}
		}
	})

	t.Run("an admission table with a bad entry is refused [verifies SP-1904~1]", func(t *testing.T) {
		source := classifySource(t)
		good := hostprototest.ClassifyAdmissionOf(t, format.KindAlignment, source, hostprototest.ClassifySessionModel).Entries[0]
		mutations := map[string]func(*hostproto.ClassifyAdmissionEntry){
			"a wrong digest":       func(e *hostproto.ClassifyAdmissionEntry) { e.TableDigest = strings.Repeat("0", 64) },
			"a source off grammar": func(e *hostproto.ClassifyAdmissionEntry) { e.Source = "host-classify(5,rot1):a*4" },
			"an unknown kind":      func(e *hostproto.ClassifyAdmissionEntry) { e.Kind = "other" },
			"no session model":     func(e *hostproto.ClassifyAdmissionEntry) { e.Model = "" },
			"a failed outcome":     func(e *hostproto.ClassifyAdmissionEntry) { e.Outcome = "NOT_MET" },
			"no CLI version":       func(e *hostproto.ClassifyAdmissionEntry) { e.CLIVersion = "" },
			"a table of another k": func(e *hostproto.ClassifyAdmissionEntry) {
				e.Table.K = 3
				e.Table.Points = []hostproto.SharePoint{{Count: 3, Confidence: 0.9}}
				e.TableDigest, _ = e.Table.Digest()
			},
		}
		for name, mutate := range mutations {
			entry := good
			mutate(&entry)
			encoded, _ := json.Marshal(hostproto.ClassifyAdmission{Entries: []hostproto.ClassifyAdmissionEntry{entry}})
			if _, err := hostproto.ParseClassifyAdmission(encoded); !errors.Is(err, hostproto.ErrClassifyTable) {
				t.Errorf("%s: error %v", name, err)
			}
		}
		twice, _ := json.Marshal(hostproto.ClassifyAdmission{Entries: []hostproto.ClassifyAdmissionEntry{good, good}})
		if _, err := hostproto.ParseClassifyAdmission(twice); !errors.Is(err, hostproto.ErrClassifyTable) {
			t.Errorf("a key twice: error %v", err)
		}
	})
}

func TestClassifyMode(t *testing.T) {
	t.Run("the switch takes auto, shadow or off and defaults to auto [verifies SP-1907~1]", func(t *testing.T) {
		for text, want := range map[string]hostproto.ClassifyMode{"": hostproto.ClassifyAuto, "auto": hostproto.ClassifyAuto, "shadow": hostproto.ClassifyShadow, "off": hostproto.ClassifyOff} {
			if got, err := hostproto.ParseClassifyMode(text); err != nil || got != want {
				t.Errorf("%q: %q %v", text, got, err)
			}
		}
		for _, text := range []string{"on", "AUTO", "true", "shadow "} {
			if _, err := hostproto.ParseClassifyMode(text); !errors.Is(err, hostproto.ErrClassifyMode) {
				t.Errorf("%q: error %v", text, err)
			}
		}
	})
}
