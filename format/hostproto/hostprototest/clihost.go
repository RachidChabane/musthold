package hostprototest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	zeroNonce         = "00000000000000000000000000000000"
	breakingOutput    = `{}`
	churnRounds       = hostproto.DefaultMaxEmissions
	genericText       = "x"
	forgedFirstChoice = "VALID"
)

// ModuleRunner runs a module's command line: the arguments after the binary, the
// standard input, and the standard output it printed.
type ModuleRunner interface {
	Run(args []string, stdin []byte) ([]byte, error)
}

// AnswerSource gives the answer a persona submits for a ticket's view. The honest
// persona is fed recordings; the others derive theirs from the schema.
type AnswerSource interface {
	Answer(persona string, view hostproto.ExecutorView) (string, error)
}

// CLIResult is what one submission of a persona came to.
type CLIResult struct {
	Ticket    string `json:"ticket"`
	Persona   string `json:"persona"`
	Submitted bool   `json:"submitted"`
	Error     string `json:"error,omitempty"`
}

// ErrNoCLIBehaviour is returned for a persona that needs hooks a command-line host
// cannot run.
var ErrNoCLIBehaviour = fmt.Errorf("the persona needs hooks: %w", ErrUnsupported)

// CLIPersonaNames lists the personas a command-line host can play: those whose
// behaviour is made of `request list` and `request submit` alone.
func CLIPersonaNames() []string {
	return []string{PersonaHonest, PersonaForger, PersonaOmitter, PersonaStale, PersonaBreaker, PersonaSelector, PersonaChurner, PersonaSlow}
}

// PlayCLI drives a module through its command line as the named persona would.
// sleep is called with the time a slow persona waits, so that a test can skip it.
func PlayCLI(runner ModuleRunner, persona string, answers AnswerSource, sleep func(time.Duration)) ([]CLIResult, error) {
	rounds := 1
	switch persona {
	case PersonaHonest, PersonaForger, PersonaOmitter, PersonaStale, PersonaBreaker, PersonaSelector, PersonaSlow:
	case PersonaChurner:
		rounds = churnRounds
	default:
		return nil, fmt.Errorf("%s: %w", persona, ErrNoCLIBehaviour)
	}
	var results []CLIResult
	for range rounds {
		listing, err := listPending(runner)
		if err != nil {
			return results, err
		}
		if len(listing.Items) == 0 {
			break
		}
		for _, item := range listing.Items {
			more, err := playItem(runner, persona, answers, sleep, item)
			results = append(results, more...)
			if err != nil {
				return results, err
			}
		}
	}
	return results, nil
}

func listPending(runner ModuleRunner) (hostproto.Listing, error) {
	stdout, err := runner.Run(hostproto.ListArgs(), nil)
	if err != nil {
		return hostproto.Listing{}, fmt.Errorf("request list: %w", err)
	}
	var listing hostproto.Listing
	if err := json.Unmarshal(stdout, &listing); err != nil {
		return hostproto.Listing{}, fmt.Errorf("request list printed an unreadable document: %w", err)
	}
	return listing, nil
}

func playItem(runner ModuleRunner, persona string, answers AnswerSource, sleep func(time.Duration), item hostproto.ListingItem) ([]CLIResult, error) {
	nonce := item.Ticket.Nonce
	var outputs []string
	switch persona {
	case PersonaOmitter:
		return nil, nil
	case PersonaForger:
		outputs = []string{ForgedAnswer(item.View.ResponseSchema)}
	case PersonaStale:
		nonce = zeroNonce
		fallthrough
	case PersonaHonest, PersonaSelector, PersonaSlow:
		answer, err := answers.Answer(persona, item.View)
		if err != nil {
			return nil, err
		}
		outputs = []string{answer}
	case PersonaBreaker:
		outputs = []string{breakingOutput, breakingOutput}
	case PersonaChurner:
		outputs = []string{breakingOutput, breakingOutput}
	}
	if persona != PersonaForger {
		if _, err := runner.Run(hostproto.ShowArgs(item.Ticket.TicketID), nil); err != nil {
			return []CLIResult{{Ticket: item.Ticket.TicketID.String(), Persona: persona, Error: err.Error()}}, nil
		}
	}
	if persona == PersonaSlow {
		sleep(hostproto.FetchTTL + time.Second)
	}
	results := make([]CLIResult, 0, len(outputs))
	for _, output := range outputs {
		_, err := runner.Run(hostproto.SubmitArgs(item.Ticket.TicketID, nonce), []byte(output))
		result := CLIResult{Ticket: item.Ticket.TicketID.String(), Persona: persona, Submitted: err == nil}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

// ForgedAnswer builds an output that satisfies the schema and picks the
// pass-favourable choice of every enumerated member: VALID where it is offered.
func ForgedAnswer(schema json.RawMessage) string {
	var spec struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Enum []string `json:"enum"`
			Type string   `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &spec); err != nil {
		return breakingOutput
	}
	members := map[string]any{}
	for _, name := range spec.Required {
		property := spec.Properties[name]
		switch {
		case len(property.Enum) > 0:
			members[name] = forgedChoice(property.Enum)
		default:
			members[name] = genericText
		}
	}
	encoded, err := json.Marshal(members)
	if err != nil {
		return breakingOutput
	}
	return string(encoded)
}

func forgedChoice(choices []string) string {
	for _, choice := range choices {
		if choice == forgedFirstChoice {
			return choice
		}
	}
	return choices[0]
}

// SchemaAnswers answers every request with a schema-derived output, which is what
// a fake host without recordings has.
type SchemaAnswers struct{}

// Answer implements AnswerSource.
func (SchemaAnswers) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	return ForgedAnswer(view.ResponseSchema), nil
}

// Recordings answers from recorded outputs keyed by request id, the answers an
// honest host gave in an earlier run.
type Recordings map[hostproto.RequestID]string

// ErrNoRecording is returned when a ticket's request has no recorded answer.
var ErrNoRecording = errors.New("no recording for the request")

// Answer implements AnswerSource.
func (r Recordings) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	id, err := hostproto.ParseTicketID(view.Ticket)
	if err != nil {
		return "", err
	}
	output, found := r[id.RequestID]
	if !found {
		return "", fmt.Errorf("%w %s", ErrNoRecording, id.RequestID)
	}
	return strings.TrimSpace(output), nil
}
