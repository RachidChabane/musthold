package domains

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

const (
	jsonIndent = "  "
	fileMode   = 0o644
)

// ErrNotDrawn is returned when the draw is read before draw-domains committed
// it.
var ErrNotDrawn = errors.New(task.DrawFile + " is not committed: run draw-domains")

// Supply is what the construction verbs read of the domains: the list, the
// committed draw, the drops the construction records hold, and the assignment.
type Supply struct {
	List       List
	Draw       Draw
	Drops      []records.Drop
	Assignment Assignment
}

// Read reads the list, the run configuration's seed, the committed draw, which
// the seed must reproduce, and the drops of the construction records. A draw
// that is not committed yet is drawn and, when write is set, committed.
func Read(f task.Flags, write bool) (Supply, error) {
	listData, err := os.ReadFile(f.Path(task.DomainsFile))
	if err != nil {
		return Supply{}, err
	}
	list, err := ParseList(listData)
	if err != nil {
		return Supply{}, err
	}
	configData, err := os.ReadFile(f.ConfigPath())
	if err != nil {
		return Supply{}, err
	}
	config, err := task.ParseConfig(configData)
	if err != nil {
		return Supply{}, err
	}
	draw, err := committedDraw(f, DrawOf(list, config.DrawSeed), write)
	if err != nil {
		return Supply{}, err
	}
	log, err := records.Read(records.Path(f.Layout()))
	if err != nil {
		return Supply{}, err
	}
	var dropped []string
	for _, drop := range log.Drops {
		dropped = append(dropped, drop.Domain)
	}
	assignment, err := Assign(draw, dropped)
	if err != nil {
		return Supply{}, err
	}
	return Supply{List: list, Draw: draw, Drops: log.Drops, Assignment: assignment}, nil
}

// committedDraw checks the committed draw against the seed's, or commits it.
func committedDraw(f task.Flags, drawn Draw, write bool) (Draw, error) {
	location := f.Path(task.DrawFile)
	data, err := os.ReadFile(location)
	if errors.Is(err, os.ErrNotExist) {
		if !write {
			return Draw{}, ErrNotDrawn
		}
		encoded, err := json.MarshalIndent(drawn, "", jsonIndent)
		if err != nil {
			return Draw{}, err
		}
		return drawn, os.WriteFile(location, append(encoded, '\n'), fileMode)
	}
	if err != nil {
		return Draw{}, err
	}
	var committed Draw
	if err := json.Unmarshal(data, &committed); err != nil {
		return Draw{}, fmt.Errorf("%s: %w", task.DrawFile, err)
	}
	return drawn, drawn.Matches(committed)
}

// Main runs draw-domains: it commits or checks the draw and prints each
// phase's domains and every drop with its check and reason. It returns 0 when
// the draw holds, 1 when it does not or cannot be read, and 2 for a usage error.
func Main(_ context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("draw-domains", flag.ContinueOnError)
	set.SetOutput(stderr)
	flags := task.RegisterFlags(set)
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return task.ExitUsage
	}
	supply, err := Read(*flags, true)
	if err != nil {
		fmt.Fprintf(stderr, "draw-domains: %v\n", err)
		return task.ExitFailure
	}
	fmt.Fprintf(stdout, "draw of seed %d\n", supply.Draw.Seed)
	for _, phase := range Phases() {
		label := "tier of the calibration"
		if tier, fixed := TierOf(phase); fixed {
			label = string(tier)
		}
		fmt.Fprintf(stdout, "%s (%s): %v\n", phase, label, supply.Assignment[phase])
	}
	for _, drop := range supply.Drops {
		fmt.Fprintf(stdout, "dropped %s at %s: %s\n", drop.Domain, drop.Check, drop.Reason)
	}
	return task.ExitOK
}
