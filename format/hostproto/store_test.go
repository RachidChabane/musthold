package hostproto_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	vectorsFile    = "testdata/vectors.json"
	vectorsMode    = 0o644
	vectorsDirMode = 0o755
)

var updateVectors = flag.Bool("update-golden", false, "rewrite the golden vectors from hostprototest")

func TestMain(m *testing.M) {
	flag.Parse()
	if *updateVectors {
		if err := writeVectors(); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}

func vectorBytes() ([]byte, error) {
	encoded, err := json.MarshalIndent(hostprototest.AllVectors(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func writeVectors() error {
	encoded, err := vectorBytes()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(vectorsFile), vectorsDirMode); err != nil {
		return err
	}
	return os.WriteFile(vectorsFile, encoded, vectorsMode)
}

func TestStoreContract(t *testing.T) {
	t.Run("the reference store satisfies the Store contract [verifies SP-125~1]", func(t *testing.T) {
		hostprototest.RunStoreContract(t, func(*testing.T) hostproto.Store { return hostprototest.NewMemoryStore() })
	})
}

func canonical(t *testing.T, document []byte) string {
	t.Helper()
	normalized, err := format.CanonicalizeJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(normalized)
}

func TestGoldenVectors(t *testing.T) {
	committed, err := os.ReadFile(vectorsFile)
	if err != nil {
		t.Fatalf("%v (run go test ./format/hostproto -update-golden)", err)
	}
	var vectors hostprototest.Vectors
	if err := json.Unmarshal(committed, &vectors); err != nil {
		t.Fatal(err)
	}

	t.Run("the committed vectors are the ones the generator produces [verifies SP-132~1]", func(t *testing.T) {
		generated, err := vectorBytes()
		if err != nil || !bytes.Equal(generated, committed) {
			t.Fatalf("the vectors are stale (run go test ./format/hostproto -update-golden): %v", err)
		}
	})

	t.Run("every request document has the committed id [verifies SP-132~1]", func(t *testing.T) {
		for _, vector := range vectors.RequestIDs {
			id, err := hostproto.RequestIDOfJSON(vector.Request)
			if err != nil || string(id) != vector.ID {
				t.Errorf("%s: id %q (%v), want %q", vector.Name, id, err, vector.ID)
			}
		}
		if vectors.RequestIDs[0].ID != vectors.RequestIDs[1].ID {
			t.Error("two documents that differ only in the module version have different ids")
		}
	})

	t.Run("every capability and ticket vector holds [verifies SP-132~1]", func(t *testing.T) {
		for _, vector := range vectors.Capabilities {
			capability, err := hostproto.FormatCap([]byte(vector.Secret), vector.AgentID)
			if err != nil || capability != vector.Cap || hostproto.CapDigest(capability) != vector.Digest {
				t.Errorf("%s: cap %q (%v)", vector.AgentID, capability, err)
			}
		}
		for _, vector := range vectors.Tickets {
			id, err := hostproto.ParseTicketID(vector.Text)
			if (err == nil) != vector.Valid || (vector.Valid && (id.Vote != vector.Vote || id.Emission != vector.Emission)) {
				t.Errorf("%q: %+v, %v", vector.Text, id, err)
			}
		}
	})

	t.Run("every transition vector holds and the matrix is complete [verifies SP-132~1]", func(t *testing.T) {
		states := len(hostproto.States())
		if len(vectors.Transitions) != states*states {
			t.Fatalf("%d vectors, want %d", len(vectors.Transitions), states*states)
		}
		for _, vector := range vectors.Transitions {
			if hostproto.Legal(vector.From, vector.To) != vector.Legal {
				t.Errorf("%s to %s", vector.From, vector.To)
			}
		}
	})

	t.Run("every hook payload decodes to its committed event [verifies SP-132~1]", func(t *testing.T) {
		for _, vector := range vectors.Hooks {
			event := decode(t, vector.Harness, vector.Kind, string(vector.Payload))
			if canonical(t, mustJSON(t, event)) != canonical(t, mustJSON(t, vector.Event)) {
				t.Errorf("%s %s decodes to %+v", vector.Harness, vector.Kind, event)
			}
		}
	})

	t.Run("every decision renders to its committed output [verifies SP-132~1]", func(t *testing.T) {
		for _, vector := range vectors.Decisions {
			var output []byte
			var err error
			if vector.Kind == string(hostproto.HookSubagentStop) {
				output, err = hostproto.EncodeStopDecision(vector.Harness, hostproto.StopDecision{Block: vector.Block, Reason: vector.Reason})
			} else {
				decision := hostproto.PreToolDecision{Permission: hostproto.PermissionDeny, Reason: vector.Reason, UpdatedInput: vector.Updated}
				if len(vector.Updated) > 0 {
					decision.Permission = hostproto.PermissionAllow
				}
				output, err = hostproto.EncodePreToolDecision(vector.Harness, decision)
			}
			if err != nil || canonical(t, output) != canonical(t, vector.Output) {
				t.Errorf("%s %s: %s (%v)", vector.Harness, vector.Kind, output, err)
			}
		}
	})
}

func TestMonotonicityHelper(t *testing.T) {
	type response struct{ severity hostprototest.Severity }
	generator := rapid.Custom(func(t *rapid.T) response {
		return response{hostprototest.Severity(rapid.IntRange(int(hostprototest.SeverityPass), int(hostprototest.SeverityBlock)).Draw(t, "severity"))}
	})
	hardens := func(r response) bool { return r.severity > hostprototest.SeverityPass }

	t.Run("a decision that takes the worst response passes the monotonicity check [verifies SP-133~1]", func(t *testing.T) {
		worst := func(responses []response) hostprototest.Severity {
			result := hostprototest.SeverityPass
			for _, r := range responses {
				result = max(result, r.severity)
			}
			return result
		}
		rapid.Check(t, func(t *rapid.T) { hostprototest.CheckMonotone(t, worst, hardens, generator) })
	})

	t.Run("a decision that a hardening response can turn into a pass is reported [verifies SP-133~1]", func(t *testing.T) {
		losesBlock := func(responses []response) hostprototest.Severity {
			if len(responses) > 1 {
				return hostprototest.SeverityPass
			}
			return hostprototest.SeverityBlock
		}
		block := response{hostprototest.SeverityBlock}
		if err := hostprototest.FindMonotonicityViolation(losesBlock, []response{block}, []response{block}); err == nil {
			t.Fatal("a non-monotone decision was not reported")
		}
		if err := hostprototest.FindMonotonicityViolation(losesBlock, nil, []response{block}); err != nil {
			t.Fatalf("a monotone step was reported: %v", err)
		}
	})

	t.Run("the vector directory exists next to the tests [verifies SP-132~1]", func(t *testing.T) {
		if _, err := os.Stat(filepath.Dir(vectorsFile)); err != nil {
			t.Fatal(err)
		}
	})
}
