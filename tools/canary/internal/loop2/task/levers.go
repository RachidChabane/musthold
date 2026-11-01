package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

// Lever names one of the five properties that make a task hard (ADR 0017).
type Lever string

// The levers, by the check that verifies each.
const (
	LeverInteracting Lever = "L1"
	LeverBoundaries  Lever = "L2"
	LeverShared      Lever = "L3"
	LeverRevision    Lever = "L4"
	LeverSize        Lever = "L5"
)

// AllLevers lists the levers in order.
func AllLevers() []Lever {
	return []Lever{LeverInteracting, LeverBoundaries, LeverShared, LeverRevision, LeverSize}
}

// LeverNames names each lever in a report.
var LeverNames = map[Lever]string{
	LeverInteracting: "interacting rules",
	LeverBoundaries:  "boundaries",
	LeverShared:      "shared helper",
	LeverRevision:    "revision",
	LeverSize:        "size",
}

// The declarations a tier requires.
const (
	MinHardLevers  = 3
	MinBoundaries  = 2
	MinInteracting = 1
)

// Interaction says that a violation of an implemented criterion fails a test of
// a task criterion.
type Interaction struct {
	Criterion     string `json:"criterion"`
	TaskCriterion string `json:"task_criterion"`
}

// Boundary is a boundary stated in the prose of a criterion: the value the
// prose names, the argument values on its two sides, and the violation that
// moves it.
type Boundary struct {
	Criterion string `json:"criterion"`
	Value     string `json:"value"`
	Inside    string `json:"inside"`
	Outside   string `json:"outside"`
	// Violation is `<criterion>/<k>`, a violation of the boundary's criterion.
	Violation string `json:"violation"`
}

// SharedHelper is the function the reference changes, as
// `<module>:<qualified name>`.
type SharedHelper struct {
	Function string `json:"function"`
}

// Module and Name split the helper's function.
func (s SharedHelper) Module() string {
	module, _, _ := strings.Cut(s.Function, functionSeparator)
	return module
}

// Name is the helper's qualified name in its module.
func (s SharedHelper) Name() string {
	_, name, _ := strings.Cut(s.Function, functionSeparator)
	return name
}

// Revision names the revised criterion whose tests the reference fails.
type Revision struct {
	Criterion string `json:"criterion"`
}

// Levers is levers.json: each lever is declared when its member is present.
type Levers struct {
	Interacting  []Interaction `json:"interacting,omitempty"`
	Boundaries   []Boundary    `json:"boundaries,omitempty"`
	SharedHelper *SharedHelper `json:"shared_helper,omitempty"`
	Revision     *Revision     `json:"revision,omitempty"`
	Size         bool          `json:"size,omitempty"`
}

const (
	functionSeparator  = ":"
	violationSeparator = "/"
)

// ErrMalformedLevers is returned for a levers.json of another shape.
var ErrMalformedLevers = errors.New("levers.json is not a JSON document of the expected shape")

// ParseLevers reads levers.json, refusing unknown members and a second document.
func ParseLevers(data []byte) (Levers, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var levers Levers
	if err := decoder.Decode(&levers); err != nil {
		return Levers{}, fmt.Errorf("%w: %v", ErrMalformedLevers, err)
	}
	if decoder.More() {
		return Levers{}, fmt.Errorf("%w: more than one document", ErrMalformedLevers)
	}
	return levers, nil
}

// Declared lists the declared levers, in order.
func (l Levers) Declared() []Lever {
	var declared []Lever
	if l.Interacting != nil {
		declared = append(declared, LeverInteracting)
	}
	if l.Boundaries != nil {
		declared = append(declared, LeverBoundaries)
	}
	if l.SharedHelper != nil {
		declared = append(declared, LeverShared)
	}
	if l.Revision != nil {
		declared = append(declared, LeverRevision)
	}
	if l.Size {
		declared = append(declared, LeverSize)
	}
	return declared
}

// Has reports whether a lever is declared.
func (l Levers) Has(lever Lever) bool { return slices.Contains(l.Declared(), lever) }

// Problems lists what makes the levers unusable for a task of a tier: too few
// declared levers, and declarations whose members are malformed.
func (l Levers) Problems(tier Tier) []string {
	var found []string
	declared := len(l.Declared())
	switch {
	case tier == TierMax && declared != len(AllLevers()):
		found = append(found, fmt.Sprintf("levers.json declares %d properties, a %s task has all %d", declared, tier, len(AllLevers())))
	case tier == TierHard && declared < MinHardLevers:
		found = append(found, fmt.Sprintf("levers.json declares %d properties, a %s task has at least %d", declared, tier, MinHardLevers))
	}
	for index, boundary := range l.Boundaries {
		criterion, _, named := strings.Cut(boundary.Violation, violationSeparator)
		if !named || criterion != boundary.Criterion {
			found = append(found, fmt.Sprintf("levers.json: boundaries[%d].violation %q is not <%s>/<k>", index, boundary.Violation, boundary.Criterion))
		}
		if boundary.Value == "" || boundary.Inside == "" || boundary.Outside == "" {
			found = append(found, fmt.Sprintf("levers.json: boundaries[%d] needs its value and both sides", index))
		}
	}
	if l.SharedHelper != nil {
		module, name, separated := strings.Cut(l.SharedHelper.Function, functionSeparator)
		if !separated || !dottedName.MatchString(module) || !dottedName.MatchString(name) {
			found = append(found, fmt.Sprintf("levers.json: shared_helper.function %q is not <module>:<name>", l.SharedHelper.Function))
		}
	}
	return found
}

// ViolationLabel is the label acceptance gives the violation a boundary names.
func (b Boundary) ViolationLabel() string {
	criterion, number, _ := strings.Cut(b.Violation, violationSeparator)
	return l1accept.Variant{Criterion: criterion, Number: number}.Label()
}

// InteractsWith reports whether levers.json pairs an implemented criterion with
// a task criterion.
func (l Levers) InteractsWith(criterion, taskCriterion string) bool {
	return slices.Contains(l.Interacting, Interaction{Criterion: criterion, TaskCriterion: taskCriterion})
}

// HeldOutName is the name of a violation in the held-out records,
// `<criterion>/<k>`.
func HeldOutName(v l1accept.Variant) string { return v.Criterion + violationSeparator + v.Number }
