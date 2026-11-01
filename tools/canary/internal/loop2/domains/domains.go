// Package domains supplies the business-rule domains of the loop-2 study's
// tasks (ADR 0017, "Supply"): the fixed list of 60 domains, its permutation by
// the draw seed, the phases the permutation is cut into, and the replacement of
// each dropped domain by the next domain in reserve. It is pure.
package domains

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

// Count is how many domains the list holds.
const Count = 60

// Domain is a subject a task is written about: the task's name and the line
// the author is told.
type Domain struct {
	Name string `json:"name"`
	Line string `json:"line"`
}

// List is domains.json.
type List struct {
	Domains []Domain `json:"domains"`
}

// Errors of the domains.
var (
	ErrList      = errors.New("the domain list is invalid")
	ErrDraw      = errors.New("the committed draw is not the draw of the seed")
	ErrExhausted = errors.New("the reserve of domains is exhausted")
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// ParseList reads domains.json and requires 60 domains of distinct names, none
// a domain of loop-1.
func ParseList(data []byte) (List, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var list List
	if err := decoder.Decode(&list); err != nil {
		return List{}, fmt.Errorf("%w: %v", ErrList, err)
	}
	if len(list.Domains) != Count {
		return List{}, fmt.Errorf("%w: %d domains, %d are required", ErrList, len(list.Domains), Count)
	}
	loop1 := []string{construct.BoxofficeName}
	for _, domain := range construct.DomainOrder() {
		loop1 = append(loop1, domain.Name)
	}
	seen := map[string]bool{}
	for _, domain := range list.Domains {
		switch {
		case !namePattern.MatchString(domain.Name) || domain.Line == "":
			return List{}, fmt.Errorf("%w: %q needs a task name and a line", ErrList, domain.Name)
		case seen[domain.Name]:
			return List{}, fmt.Errorf("%w: %q is listed twice", ErrList, domain.Name)
		case slices.Contains(loop1, domain.Name):
			return List{}, fmt.Errorf("%w: %q is a domain of loop-1", ErrList, domain.Name)
		}
		seen[domain.Name] = true
	}
	return list, nil
}

// Line is the line of a named domain.
func (l List) Line(name string) (string, bool) {
	at := slices.IndexFunc(l.Domains, func(d Domain) bool { return d.Name == name })
	if at < 0 {
		return "", false
	}
	return l.Domains[at].Line, true
}

// Draw is draw.json: the seed and the order it gives the domains.
type Draw struct {
	Seed  uint64   `json:"seed"`
	Order []string `json:"order"`
}

// DrawOf orders the list's domains by a permutation seeded with seed.
func DrawOf(list List, seed uint64) Draw {
	order := make([]string, 0, len(list.Domains))
	for _, domain := range list.Domains {
		order = append(order, domain.Name)
	}
	shuffle := rand.New(rand.NewPCG(seed, seed))
	shuffle.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	return Draw{Seed: seed, Order: order}
}

// Matches refuses a committed draw that the seed does not reproduce.
func (d Draw) Matches(committed Draw) error {
	if committed.Seed != d.Seed || !slices.Equal(committed.Order, d.Order) {
		return fmt.Errorf("%w: seed %d", ErrDraw, d.Seed)
	}
	return nil
}

// Phase is a part of the study that builds tasks.
type Phase string

// The phases, in the order the draw is cut into them.
const (
	PhaseShakedown    Phase = "shakedown"
	PhaseC1           Phase = "C1"
	PhaseC2           Phase = "C2"
	PhaseConfirmatory Phase = "confirmatory"
)

// Phases lists the phases in order.
func Phases() []Phase {
	return []Phase{PhaseShakedown, PhaseC1, PhaseC2, PhaseConfirmatory}
}

// Sizes are the domains each phase takes from the draw; the rest is the
// reserve.
var Sizes = map[Phase]int{PhaseShakedown: 2, PhaseC1: 8, PhaseC2: 8, PhaseConfirmatory: 24}

// tiers are the tiers of the phases that fix their own; the confirmatory tier
// is the calibration's.
var tiers = map[Phase]task.Tier{PhaseShakedown: task.TierMax, PhaseC1: task.TierHard, PhaseC2: task.TierMax}

// TierOf is the tier of a phase's tasks, and false for the confirmatory phase.
func TierOf(phase Phase) (task.Tier, bool) {
	tier, fixed := tiers[phase]
	return tier, fixed
}

// Assignment is the domains each phase builds, in order.
type Assignment map[Phase][]string

// Assign cuts the draw into the phases and gives each dropped domain's place,
// in the order of drops, to the first reserve domain not yet given.
func Assign(draw Draw, drops []string) (Assignment, error) {
	assignment := Assignment{}
	next := 0
	for _, phase := range Phases() {
		size := Sizes[phase]
		assignment[phase] = append([]string(nil), draw.Order[next:min(next+size, len(draw.Order))]...)
		next += size
	}
	reserve := draw.Order[min(next, len(draw.Order)):]
	for _, dropped := range drops {
		phase, at, found := assignment.find(dropped)
		if !found {
			continue
		}
		if len(reserve) == 0 {
			return nil, fmt.Errorf("%w: %s cannot replace %s", ErrExhausted, phase, dropped)
		}
		assignment[phase][at], reserve = reserve[0], reserve[1:]
	}
	return assignment, nil
}

// find locates a domain in the assignment.
func (a Assignment) find(name string) (Phase, int, bool) {
	for _, phase := range Phases() {
		if at := slices.Index(a[phase], name); at >= 0 {
			return phase, at, true
		}
	}
	return "", 0, false
}
