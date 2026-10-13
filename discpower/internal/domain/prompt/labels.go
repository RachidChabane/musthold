package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Names of the embedded template and schema of the labels request.
const (
	labelsSystem       = "templates/labels.system.txt"
	labelsInstructions = "templates/labels.instructions.txt"
	labelsSchema       = "schemas/labels.json"
)

const (
	promiseIDPrefix  = "p"
	functionIDPrefix = "f"
)

// Errors of a labels response that cannot be used.
var (
	ErrUnknownLabelPromise  = errors.New("the labels name a promise the request did not show")
	ErrUnknownLabelFunction = errors.New("the labels name a function the request did not show")
)

// LabelPromise is one kept promise as the labels judge sees it.
type LabelPromise struct {
	ID    string
	Quote string
}

// LabelFunction is one function of the generation view as the labels judge sees
// it: where it lives, how it is called and its code.
type LabelFunction struct {
	ID        string
	Path      string
	Signature string
	Code      string
}

// LabelsInput is what a labels request is built from.
type LabelsInput struct {
	Promises  []LabelPromise
	Functions []LabelFunction
}

// PromiseID is the opaque id of the promise at index in a labels request.
func PromiseID(index int) string { return fmt.Sprintf("%s%d", promiseIDPrefix, index+1) }

// FunctionID is the opaque id of the function at index in a labels request.
func FunctionID(index int) string { return fmt.Sprintf("%s%d", functionIDPrefix, index+1) }

// BuildLabels builds the `judge.labels` prompt.
func BuildLabels(input LabelsInput) Built {
	var user strings.Builder
	user.WriteString("Promises:\n")
	for _, promise := range input.Promises {
		fmt.Fprintf(&user, "%s: %q\n", promise.ID, promise.Quote)
	}
	user.WriteString(sectionBreak + "Functions:\n")
	for _, function := range input.Functions {
		fmt.Fprintf(&user, "Function %s, %s, %s:\n%s\n%s\n%s\n\n", function.ID, function.Path, function.Signature, codeFence, strings.TrimRight(function.Code, "\n"), codeFence)
	}
	user.WriteString(embedded(labelsInstructions))
	return build(embedded(labelsSystem), user.String(), embedded(labelsSchema), nil, embedded(labelsInstructions))
}

// ParseLabels reads a response that satisfies the labels schema into the
// functions each promise labels. A promise id that is not among promiseIDs and
// a function id that is not among functionIDs are refused. A promise the
// response leaves out labels no function. Each list follows the order of
// functionIDs and holds a function once, so the result does not depend on the
// order of the response.
func ParseLabels(output []byte, promiseIDs, functionIDs []string) (map[string][]string, error) {
	var document struct {
		Labels map[string][]string `json:"labels"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("read the labels response: %w", err)
	}
	for promise, functions := range document.Labels {
		if !slices.Contains(promiseIDs, promise) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownLabelPromise, promise)
		}
		for _, function := range functions {
			if !slices.Contains(functionIDs, function) {
				return nil, fmt.Errorf("%w: %q", ErrUnknownLabelFunction, function)
			}
		}
	}
	labels := make(map[string][]string, len(promiseIDs))
	for _, promise := range promiseIDs {
		labels[promise] = []string{}
		for _, function := range functionIDs {
			if slices.Contains(document.Labels[promise], function) {
				labels[promise] = append(labels[promise], function)
			}
		}
	}
	return labels, nil
}
