package hostproto_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"pgregory.net/rapid"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files")

const classifyGoldenPath = "testdata/classify-plan.golden.json"

func classifySet() hostproto.ClosedSet {
	return hostproto.ClosedSet{State: "the state", Questions: []hostproto.ClosedQuestion{
		hostproto.ChoiceQuestion("pick", "Which one?", options("A", "B", "NONE")),
		hostproto.NoulQuestion("claims", "Does it claim it?", "It claims it.", "It does not."),
		hostproto.ScoreQuestion("coverage", "How much?", options("LOW", "MID", "HIGH")),
		hostproto.ChoiceQuestion("screen", "Does the edit violate it?", options("VIOLATES", "CONSISTENT")),
	}}
}

func mustPlan(t testing.TB, set hostproto.ClosedSet) hostproto.ClassifyPlan {
	t.Helper()
	plan, err := set.ClassifyPlan()
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestClassifyPlanIsFrozenByAGoldenFile(t *testing.T) {
	t.Run("the plan of a fixed set equals the golden file, seed input, seed, permutations and texts included [verifies SP-1902~1]", func(t *testing.T) {
		encoded, err := json.MarshalIndent(mustPlan(t, classifySet()), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')
		if *updateGolden {
			if err := os.WriteFile(classifyGoldenPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(classifyGoldenPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded, want) {
			t.Fatalf("the plan differs from %s; run the test with -update to see the new bytes in the diff", classifyGoldenPath)
		}
	})
}

func TestClassifyPlan(t *testing.T) {
	t.Run("the seed input is host-classify|rot1| then the digest of the set, | and the question id [verifies SP-1902~1]", func(t *testing.T) {
		set := classifySet()
		digest, err := set.Digest()
		if err != nil {
			t.Fatal(err)
		}
		plan := mustPlan(t, set)
		if plan.ClosedSetDigest != digest || plan.Scheme != "rot1" || plan.K != 5 {
			t.Fatalf("plan %q %q %d", plan.ClosedSetDigest, plan.Scheme, plan.K)
		}
		for _, question := range plan.Questions {
			wantInput := "host-classify|rot1|" + digest + "|" + question.ID
			sum := sha256.Sum256([]byte(wantInput))
			if question.SeedInput != wantInput || question.Seed != hex.EncodeToString(sum[:16]) {
				t.Errorf("%s: seed input %q seed %q", question.ID, question.SeedInput, question.Seed)
			}
		}
	})

	t.Run("a plan is the same on every run [verifies SP-1902~1]", func(t *testing.T) {
		first, second := mustPlan(t, classifySet()), mustPlan(t, classifySet())
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(second)
		if !bytes.Equal(a, b) {
			t.Fatal("two plans of one set differ")
		}
	})

	t.Run("a choice or noul question lists the shuffled order rotated by the call index [verifies SP-1902~1]", func(t *testing.T) {
		plan := mustPlan(t, classifySet())
		for _, id := range []string{"pick", "claims", "screen"} {
			question, _ := plan.Question(id)
			first := question.Calls[0].Labels
			for call, planned := range question.Calls {
				for position, label := range planned.Labels {
					if want := first[(call+position)%len(first)]; label != want {
						t.Errorf("%s call %d position %d: %q, want %q", id, call, position, label, want)
					}
				}
			}
		}
	})

	t.Run("a score alternates ascending and descending, and the first direction is the low bit of the seed [verifies SP-1902~1]", func(t *testing.T) {
		plan := mustPlan(t, classifySet())
		question, _ := plan.Question("coverage")
		ascending := []string{"LOW", "MID", "HIGH"}
		descending := []string{"HIGH", "MID", "LOW"}
		seed, _ := hex.DecodeString(question.Seed)
		firstDescending := seed[7]&1 == 1
		for call, planned := range question.Calls {
			want := ascending
			if firstDescending != (call%2 == 1) {
				want = descending
			}
			if !slices.Equal(planned.Labels, want) {
				t.Errorf("call %d: %v, want %v", call, planned.Labels, want)
			}
		}
	})

	t.Run("a question the plan names has the same labels whichever model answers [verifies SP-1902~1]", func(t *testing.T) {
		plan := mustPlan(t, classifySet())
		single, _ := format.SingleVoterClassifySource(5, format.ClassifySchemeRot1, "claude-haiku-4-5-20251001")
		mixed, _ := format.NewClassifySource(5, format.ClassifySchemeRot1, []format.ClassifyVoter{{Model: "claude-sonnet-5-5", Calls: 3}, {Model: "claude-haiku-4-5-20251001", Calls: 2}})
		a, errA := plan.Calls(single)
		b, errB := plan.Calls(mixed)
		if errA != nil || errB != nil || len(a) != len(b) {
			t.Fatalf("%v %v %d %d", errA, errB, len(a), len(b))
		}
		for index := range a {
			if !slices.Equal(a[index].Labels, b[index].Labels) || a[index].Text != b[index].Text || a[index].Question != b[index].Question || a[index].Index != b[index].Index {
				t.Errorf("call %d differs between the voter sets", index)
			}
		}
		if got := b[0].Model + " " + b[3].Model; got != "claude-sonnet-5-5 claude-haiku-4-5-20251001" {
			t.Errorf("models %q", got)
		}
	})

	t.Run("a call whose source plans another k or scheme is refused [verifies SP-1902~1]", func(t *testing.T) {
		plan := mustPlan(t, classifySet())
		other, _ := format.SingleVoterClassifySource(3, format.ClassifySchemeRot1, "m")
		if _, err := plan.Calls(other); err == nil {
			t.Fatal("a source of 3 calls was accepted")
		}
	})

	t.Run("the text lists the options in the order of the labels with their descriptions [verifies SP-1902~1]", func(t *testing.T) {
		plan := mustPlan(t, classifySet())
		question, _ := plan.Question("pick")
		for _, call := range question.Calls {
			var listed []string
			for _, line := range strings.Split(call.Text, "\n") {
				if name, _, found := strings.Cut(strings.TrimPrefix(line, "- "), ": when the answer is "); found && strings.HasPrefix(line, "- ") {
					listed = append(listed, name)
				}
			}
			if !slices.Equal(listed, call.Labels) {
				t.Errorf("call %d lists %v for the labels %v", call.Index, listed, call.Labels)
			}
			if !strings.Contains(call.Text, "the state") || !strings.Contains(call.Text, "Which one?") {
				t.Errorf("call %d text %q", call.Index, call.Text)
			}
		}
	})

	t.Run("a set that breaks the closed rules cannot be planned [verifies SP-1902~1]", func(t *testing.T) {
		if _, err := (hostproto.ClosedSet{}).ClassifyPlan(); err == nil {
			t.Fatal("an empty set was planned")
		}
	})

	t.Run("the digest of the set changes the order, and the same set always gives the same digest [verifies SP-1902~1]", func(t *testing.T) {
		set := classifySet()
		other := classifySet()
		other.State = "another state"
		a, _ := set.Digest()
		b, _ := other.Digest()
		again, _ := classifySet().Digest()
		if a == b || a != again {
			t.Fatalf("digests %s %s %s", a, b, again)
		}
	})
}

func drawQuestion(t *rapid.T, id string) hostproto.ClosedQuestion {
	kind := rapid.SampledFrom([]hostproto.ClosedType{hostproto.ClosedChoice, hostproto.ClosedNoul, hostproto.ClosedScore}).Draw(t, "kind")
	if kind == hostproto.ClosedNoul {
		return hostproto.NoulQuestion(id, "Is it so?", "It is.", "It is not.")
	}
	count := rapid.IntRange(2, 9).Draw(t, "options")
	names := make([]string, count)
	for index := range names {
		names[index] = string(rune('A'+index)) + "-name"
	}
	if kind == hostproto.ClosedScore {
		return hostproto.ScoreQuestion(id, "How much?", options(names...))
	}
	return hostproto.ChoiceQuestion(id, "Which?", options(names...))
}

func drawSet(t *rapid.T) hostproto.ClosedSet {
	count := rapid.IntRange(1, 5).Draw(t, "questions")
	set := hostproto.ClosedSet{State: rapid.StringMatching(`[a-z ]{1,30}`).Draw(t, "state")}
	for index := range count {
		set.Questions = append(set.Questions, drawQuestion(t, "q"+string(rune('a'+index))))
	}
	return set
}

func TestClassifyPlanProperties(t *testing.T) {
	t.Run("every call of every question lists each option exactly once, so the order is a permutation [verifies SP-1902~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			set := drawSet(t)
			plan, err := set.ClassifyPlan()
			if err != nil {
				t.Fatal(err)
			}
			for index, question := range set.Questions {
				canonical := slices.Clone(question.OptionNames())
				if question.Type == hostproto.ClosedNoul {
					canonical = []string{"true", "false"}
				}
				planned := plan.Questions[index]
				if len(planned.Calls) != 5 {
					t.Fatalf("%d calls", len(planned.Calls))
				}
				for _, call := range planned.Calls {
					sorted := slices.Clone(call.Labels)
					slices.Sort(sorted)
					want := slices.Clone(canonical)
					slices.Sort(want)
					if !slices.Equal(sorted, want) {
						t.Fatalf("call %d lists %v of %v", call.Index, call.Labels, canonical)
					}
				}
			}
		})
	})

	t.Run("a choice question shows each label in each position at most once per n calls [verifies SP-1902~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			question := hostproto.ChoiceQuestion("q", "Which?", options("A", "B", "C", "D", "E", "F", "G"))
			set := hostproto.ClosedSet{State: rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "state"), Questions: []hostproto.ClosedQuestion{question}}
			plan, err := set.ClassifyPlan()
			if err != nil {
				t.Fatal(err)
			}
			seen := map[int]map[string]bool{}
			for _, call := range plan.Questions[0].Calls {
				for position, label := range call.Labels {
					if seen[position] == nil {
						seen[position] = map[string]bool{}
					}
					if seen[position][label] {
						t.Fatalf("%s sits at position %d twice in 5 calls of 7 options", label, position)
					}
					seen[position][label] = true
				}
			}
		})
	})

	t.Run("the same set gives the same plan, and the plan does not depend on the question order of other questions [verifies SP-1902~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			set := drawSet(t)
			first, err := set.ClassifyPlan()
			if err != nil {
				t.Fatal(err)
			}
			second, _ := set.ClassifyPlan()
			a, _ := json.Marshal(first)
			b, _ := json.Marshal(second)
			if !bytes.Equal(a, b) {
				t.Fatal("two plans of one set differ")
			}
		})
	})

	t.Run("the labels of a call are the option names and the text names every option and no other [verifies SP-1902~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			set := drawSet(t)
			plan, err := set.ClassifyPlan()
			if err != nil {
				t.Fatal(err)
			}
			for _, question := range plan.Questions {
				for _, call := range question.Calls {
					for _, label := range call.Labels {
						if !strings.Contains(call.Text, "- "+label) {
							t.Fatalf("the text of call %d omits %q", call.Index, label)
						}
					}
					if got := strings.Count(call.Text, "\n- "); got != len(call.Labels) {
						t.Fatalf("the text of call %d lists %d options for %d labels", call.Index, got, len(call.Labels))
					}
				}
			}
		})
	})
}
