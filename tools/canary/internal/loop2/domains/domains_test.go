package domains_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/domains"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

const repoFromHere = "../../../../.."

func read(t *testing.T, relative string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoFromHere, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func committedList(t *testing.T) domains.List {
	t.Helper()
	list, err := domains.ParseList(read(t, task.DomainsFile))
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func synthetic(n int) domains.List {
	var list domains.List
	for i := range n {
		list.Domains = append(list.Domains, domains.Domain{Name: fmt.Sprintf("domain-%02d", i), Line: "a rule"})
	}
	return list
}

func encode(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestList(t *testing.T) {
	t.Run("the committed list holds 60 business-rule domains of distinct names, none of loop-1 [verifies SP-2928~1]", func(t *testing.T) {
		if list := committedList(t); len(list.Domains) != domains.Count {
			t.Fatalf("%d domains", len(list.Domains))
		}
	})

	t.Run("a short list, a name twice and a domain of loop-1 are refused [verifies SP-2928~1]", func(t *testing.T) {
		twice := synthetic(domains.Count)
		twice.Domains[1].Name = twice.Domains[0].Name
		loop1 := synthetic(domains.Count)
		loop1.Domains[5].Name = "train-fare"
		for name, list := range map[string]domains.List{"short": synthetic(59), "twice": twice, "loop-1": loop1} {
			if _, err := domains.ParseList(encode(t, list)); !errors.Is(err, domains.ErrList) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

func TestDraw(t *testing.T) {
	list := synthetic(domains.Count)
	t.Run("the same seed gives the same order, another seed another one [verifies SP-2929~1]", func(t *testing.T) {
		first, again, other := domains.DrawOf(list, 7), domains.DrawOf(list, 7), domains.DrawOf(list, 8)
		if !slices.Equal(first.Order, again.Order) || slices.Equal(first.Order, other.Order) || len(first.Order) != domains.Count {
			t.Fatal("the draw is not a function of its seed")
		}
		if first.Matches(again) != nil || !errors.Is(first.Matches(other), domains.ErrDraw) {
			t.Fatal("matching")
		}
	})

	t.Run("the order is cut into 2 shakedown max, 8 C1 hard, 8 C2 max, 24 confirmatory and 18 in reserve [verifies SP-2929~1]", func(t *testing.T) {
		draw := domains.DrawOf(list, 7)
		assignment, err := domains.Assign(draw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(assignment[domains.PhaseShakedown], draw.Order[0:2]) || !slices.Equal(assignment[domains.PhaseC1], draw.Order[2:10]) ||
			!slices.Equal(assignment[domains.PhaseC2], draw.Order[10:18]) || !slices.Equal(assignment[domains.PhaseConfirmatory], draw.Order[18:42]) {
			t.Fatalf("%v", assignment)
		}
		for phase, want := range map[domains.Phase]task.Tier{domains.PhaseShakedown: task.TierMax, domains.PhaseC1: task.TierHard, domains.PhaseC2: task.TierMax} {
			if tier, fixed := domains.TierOf(phase); !fixed || tier != want {
				t.Errorf("%s: %s", phase, tier)
			}
		}
		if _, fixed := domains.TierOf(domains.PhaseConfirmatory); fixed {
			t.Error("the confirmatory tier is the calibration's")
		}
	})

	t.Run("a dropped domain's place goes to the first reserve domain not yet given, in the order of the drops [verifies SP-2930~1]", func(t *testing.T) {
		draw := domains.DrawOf(list, 7)
		assignment, err := domains.Assign(draw, []string{draw.Order[3], draw.Order[0], draw.Order[42]})
		if err != nil {
			t.Fatal(err)
		}
		// C1's second domain gives way to the first reserve domain, the first
		// shakedown domain to the second, and the dropped replacement to the third.
		if assignment[domains.PhaseC1][1] != draw.Order[44] || assignment[domains.PhaseShakedown][0] != draw.Order[43] {
			t.Fatalf("%v", assignment)
		}
	})

	t.Run("a drop with the reserve exhausted names the phase [verifies SP-2930~1]", func(t *testing.T) {
		draw := domains.DrawOf(list, 7)
		var drops []string
		current := slices.Clone(draw.Order[2:3])
		for range 19 {
			drops = append(drops, current[0])
			assignment, err := domains.Assign(draw, drops)
			if err != nil {
				if !errors.Is(err, domains.ErrExhausted) || !strings.Contains(err.Error(), "C1") {
					t.Fatalf("got %v", err)
				}
				return
			}
			current = assignment[domains.PhaseC1][:1]
		}
		t.Fatal("the reserve never ran out")
	})
}

func TestCommittedDraw(t *testing.T) {
	t.Run("the committed draw is the draw of loop-2.json's seed over the committed list [verifies SP-2929~1] [verifies SP-2931~1]", func(t *testing.T) {
		config, err := task.ParseConfig(read(t, task.ConfigFile))
		if err != nil {
			t.Fatal(err)
		}
		var committed domains.Draw
		if err := json.Unmarshal(read(t, task.DrawFile), &committed); err != nil {
			t.Fatal(err)
		}
		if err := domains.DrawOf(committedList(t), config.DrawSeed).Matches(committed); err != nil {
			t.Fatal(err)
		}
	})
}

// repository is a copy of the study's list and configuration.
func repository(t *testing.T) task.Flags {
	t.Helper()
	repo := t.TempDir()
	for _, file := range []string{task.DomainsFile, task.ConfigFile} {
		location := filepath.Join(repo, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(location, read(t, file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return task.Flags{Repo: repo, Config: task.ConfigFile, Work: t.TempDir()}
}

func TestDrawVerb(t *testing.T) {
	flags := repository(t)
	args := []string{"--repo", flags.Repo, "--config", flags.Config, "--work", flags.Work}

	t.Run("draw-domains commits the draw when it is absent and prints each phase [verifies SP-2931~1] [verifies SP-2946~1]", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := domains.Main(context.Background(), args, &stdout, &stderr); code != task.ExitOK {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
		if _, err := os.Stat(flags.Path(task.DrawFile)); err != nil {
			t.Fatal(err)
		}
		for _, phase := range domains.Phases() {
			if !strings.Contains(stdout.String(), string(phase)+" (") {
				t.Errorf("no line for %s:\n%s", phase, stdout.String())
			}
		}
	})

	t.Run("a drop of the records is printed with its check and reason, and its place given [verifies SP-2931~1]", func(t *testing.T) {
		supply, err := domains.Read(flags, false)
		if err != nil {
			t.Fatal(err)
		}
		dropped := supply.Assignment[domains.PhaseC1][0]
		writer := records.NewFileWriter(study.Layout{Root: flags.Work})
		if err := writer.Append(records.Entry{Drop: &records.Drop{Domain: dropped, Check: "A5 diagonal", Reason: "fails", Time: time.Now()}}); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		if code := domains.Main(context.Background(), args, &stdout, &bytes.Buffer{}); code != task.ExitOK {
			t.Fatalf("exit %d", code)
		}
		if !strings.Contains(stdout.String(), "dropped "+dropped+" at A5 diagonal: fails") || !strings.Contains(stdout.String(), supply.Draw.Order[42]) {
			t.Fatalf("%s", stdout.String())
		}
	})

	t.Run("a committed draw the seed does not reproduce is refused, and an unknown flag is a usage error [verifies SP-2931~1] [verifies SP-2946~1]", func(t *testing.T) {
		if err := os.WriteFile(flags.Path(task.DrawFile), []byte(`{"seed":1,"order":["a"]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := domains.Main(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); code != task.ExitFailure {
			t.Fatalf("exit %d", code)
		}
		if code := domains.Main(context.Background(), append(args, "--seed", "3"), &bytes.Buffer{}, &bytes.Buffer{}); code != task.ExitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}
