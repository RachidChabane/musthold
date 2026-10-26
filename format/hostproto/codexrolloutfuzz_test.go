package hostproto_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// parentItem is an assistant message of the spawning agent, as a rollout line.
const parentItem = `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I will spawn a decider."}]}}`

var childHistories = []hostproto.ChildHistory{hostproto.ChildBlind, hostproto.ChildInherited, hostproto.ChildUnproven}

// withParentItem puts a parent item right after the first line of a rollout,
// before any item the rollout holds.
func withParentItem(rollout []byte) []byte {
	first, rest, found := bytes.Cut(rollout, []byte("\n"))
	if !found {
		return append(append(slices.Clone(rollout), '\n'), parentItem+"\n"...)
	}
	out := append(slices.Clone(first), '\n')
	out = append(out, parentItem+"\n"...)
	return append(out, rest...)
}

func childRolloutBody(t *testing.T, rollout []byte) {
	t.Helper()
	run := hostproto.ReadCodexChildRollout(rollout)
	if !slices.Contains(childHistories, run.History) {
		t.Fatalf("history %q is none of %v", run.History, childHistories)
	}
	if (run.History == hostproto.ChildBlind) != (run.Reason == "") {
		t.Fatalf("history %s with reason %q", run.History, run.Reason)
	}
	if run.TicketFound && run.Ticket.Validate() != nil {
		t.Fatalf("the ticket %+v is not valid", run.Ticket)
	}
	if again := hostproto.ReadCodexChildRollout(withParentItem(rollout)); again.History == hostproto.ChildBlind {
		t.Fatalf("a parent item before every item left the thread blind: %q", rollout)
	}
}

func childRolloutSeeds(t testing.TB) [][]byte {
	seeds := [][]byte{nil, []byte("\n"), []byte("{}"), []byte(`{"type":"session_meta"`), []byte("[1]\n{"), []byte(parentItem + "\n")}
	for _, name := range []string{"codex-child-rollout.jsonl", "codex-child-rollout-agent-message.jsonl"} {
		content, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, content, bytes.TrimSuffix(content, []byte("\n")))
	}
	return seeds
}

func FuzzCodexChildRollout(f *testing.F) {
	for _, seed := range childRolloutSeeds(f) {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, rollout []byte) { childRolloutBody(t, rollout) })
}

func TestCodexChildRolloutSeedCorpus(t *testing.T) {
	t.Run("the rollout rule holds its invariants on the seed corpus, and a parent item before the task never leaves a thread blind [verifies SP-1934~2]", func(t *testing.T) {
		for _, seed := range childRolloutSeeds(t) {
			childRolloutBody(t, seed)
		}
	})
}
