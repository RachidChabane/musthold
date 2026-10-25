package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Names of the embedded template and schema of the promise extraction request.
const (
	promisesSystem       = "templates/promises.system.txt"
	promisesInstructions = "templates/promises.instructions.txt"
	promisesSchema       = "schemas/promises.json"
)

// MaxExtractedPromises is the most promises one extraction response lists.
const MaxExtractedPromises = 8

const sourceTextOpen, sourceTextClose = "<<<", ">>>"

// Errors of a promise extraction response that cannot be used.
var (
	ErrUnknownPromiseSource = errors.New("a promise names a source the request did not show")
	ErrEmptyPromiseQuote    = errors.New("a promise has an empty quote")
	ErrTooManyPromises      = errors.New("the response lists more promises than the request allows")
)

// PromiseSource is one quotable source of a change: its name and its exact text.
type PromiseSource struct {
	Name string
	Text string
}

// PromisesInput is what a promise extraction request is built from. The
// sources are a closed set. A change adds the paths of its changed files and
// the public signatures of its changed functions: never a hunk, never a body.
type PromisesInput struct {
	Sources      []PromiseSource
	ChangedPaths []string
	Signatures   []string
}

// BuildPromises builds the `generate.promises` prompt.
func BuildPromises(input PromisesInput) Built {
	var user strings.Builder
	user.WriteString("Sources, each under its name with its exact text between the markers:\n\n")
	for _, source := range input.Sources {
		fmt.Fprintf(&user, "Source %s:\n%s\n%s\n%s\n\n", source.Name, sourceTextOpen, source.Text, sourceTextClose)
	}
	if len(input.ChangedPaths) > 0 || len(input.Signatures) > 0 {
		writeList(&user, "Files the change touches:", input.ChangedPaths)
		writeList(&user, "Public signatures of the functions the change touches:", input.Signatures)
	}
	user.WriteString(embedded(promisesInstructions))
	return build(embedded(promisesSystem), user.String(), embedded(promisesSchema), nil, embedded(promisesInstructions))
}

// ExtractedPromise is one promise an extraction response states: a quote and
// the name of the source it claims to be copied from. The quote is not checked
// against the source here.
type ExtractedPromise struct {
	Quote  string
	Source string
}

// ParsePromises reads a response that satisfies the promises schema into its
// promises, in the order given. A promise whose source is not among the names
// the request showed, a promise with an empty quote, and a list longer than
// MaxExtractedPromises are refused.
func ParsePromises(output []byte, sourceNames []string) ([]ExtractedPromise, error) {
	var document struct {
		Promises []struct {
			Quote  string `json:"quote"`
			Source string `json:"source"`
		} `json:"promises"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("read the promises response: %w", err)
	}
	if len(document.Promises) > MaxExtractedPromises {
		return nil, fmt.Errorf("%w: %d", ErrTooManyPromises, len(document.Promises))
	}
	promises := make([]ExtractedPromise, len(document.Promises))
	for index, item := range document.Promises {
		if !slices.Contains(sourceNames, item.Source) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownPromiseSource, item.Source)
		}
		if item.Quote == "" {
			return nil, fmt.Errorf("%w: promise %d", ErrEmptyPromiseQuote, index+1)
		}
		promises[index] = ExtractedPromise{Quote: item.Quote, Source: item.Source}
	}
	return promises, nil
}
