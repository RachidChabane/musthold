package mutation

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

// DefinitionLocator finds the definitions of a source file. It is the narrow
// view of the locator port this package needs.
type DefinitionLocator interface {
	Locate(path string, source []byte) ([]ecosystem.Definition, error)
}

// nestedSeparator joins a definition's qualname to the names nested in it.
const nestedSeparator = "."

// Splicer applies edits to source bytes.
type Splicer struct {
	Locator DefinitionLocator
}

// Splice replaces the definition an edit names with its new source and checks
// the post-splice invariant on the result.
func (s Splicer) Splice(file string, source []byte, edit Edit) ([]byte, error) {
	before, err := s.Locator.Locate(file, source)
	if err != nil {
		return nil, fmt.Errorf("locate %q: %w", file, err)
	}
	target, err := unique(before, edit.Qualname)
	if err != nil {
		return nil, fmt.Errorf("%w: %s in %q", err, edit.Qualname, file)
	}
	span := target.Span
	if !span.Within(len(source)) {
		return nil, fmt.Errorf("%w: span of %s", ecosystem.ErrInvalidSpan, edit.Qualname)
	}
	spliced := make([]byte, 0, len(source)-span.Len()+len(edit.NewSource))
	spliced = append(spliced, source[:span.Start]...)
	spliced = append(spliced, edit.NewSource...)
	spliced = append(spliced, source[span.End:]...)

	after, err := s.Locator.Locate(file, spliced)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReparse, err)
	}
	if err := CheckInvariant(source, before, spliced, after, edit); err != nil {
		return nil, err
	}
	return spliced, nil
}

// SpliceAll applies the edits in order, each on the result of the previous one.
func (s Splicer) SpliceAll(file string, source []byte, edits []Edit) ([]byte, error) {
	current := source
	for _, edit := range edits {
		next, err := s.Splice(file, current, edit)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

// CheckInvariant verifies a splice: the target definition's text equals the new
// source, and every definition outside the target is byte-identical, in the
// same order.
func CheckInvariant(before []byte, beforeDefs []ecosystem.Definition, after []byte, afterDefs []ecosystem.Definition, edit Edit) error {
	target, err := unique(afterDefs, edit.Qualname)
	if err != nil {
		return fmt.Errorf("%w: target %s: %v", ErrInvariantViolated, edit.Qualname, err)
	}
	text, err := target.Span.Slice(after)
	if err != nil || !bytes.Equal(text, []byte(edit.NewSource)) {
		return fmt.Errorf("%w: target %s differs from the new source", ErrInvariantViolated, edit.Qualname)
	}
	want, err := outsideTexts(before, beforeDefs, edit.Qualname)
	if err != nil {
		return err
	}
	got, err := outsideTexts(after, afterDefs, edit.Qualname)
	if err != nil {
		return err
	}
	if len(want) != len(got) {
		return fmt.Errorf("%w: %d definitions outside %s became %d", ErrInvariantViolated, len(want), edit.Qualname, len(got))
	}
	for i := range want {
		if want[i].qualname != got[i].qualname || !bytes.Equal(want[i].text, got[i].text) {
			return fmt.Errorf("%w: definition %s changed", ErrInvariantViolated, want[i].qualname)
		}
	}
	return nil
}

type definitionText struct {
	qualname string
	text     []byte
}

// outsideTexts returns the texts of the definitions that are neither the target
// nor nested in it.
func outsideTexts(source []byte, definitions []ecosystem.Definition, target string) ([]definitionText, error) {
	var texts []definitionText
	for _, d := range definitions {
		if d.Qualname == target || strings.HasPrefix(d.Qualname, target+nestedSeparator) {
			continue
		}
		text, err := d.Span.Slice(source)
		if err != nil {
			return nil, err
		}
		texts = append(texts, definitionText{qualname: d.Qualname, text: text})
	}
	return texts, nil
}

func unique(definitions []ecosystem.Definition, qualname string) (ecosystem.Definition, error) {
	var found []ecosystem.Definition
	for _, d := range definitions {
		if d.Qualname == qualname {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return ecosystem.Definition{}, ErrUnknownDefinition
	case 1:
		return found[0], nil
	default:
		return ecosystem.Definition{}, ErrAmbiguousDefinition
	}
}
