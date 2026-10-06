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

// AssignmentLocator finds the module-level assignments of a source file, as
// definitions of kind ecosystem.KindAssignment. It is what makes the target
// kinds module assignment and module addition checkable.
type AssignmentLocator interface {
	LocateAssignments(path string, source []byte) ([]ecosystem.Definition, error)
}

const (
	// nestedSeparator joins a definition's qualname to the names nested in it.
	nestedSeparator = "."
	// additionSeparator surrounds an appended module-level statement.
	additionSeparator = "\n"
)

// Splicer applies edits to source bytes. Assignments may be nil, in which case
// only definition and addition edits that do not need it are supported.
type Splicer struct {
	Locator     DefinitionLocator
	Assignments AssignmentLocator
}

// Splice applies one edit and checks the post-splice invariant on the result:
// a definition or a module assignment is replaced by the new source, and a
// module addition is appended on its own line after the last statement.
func (s Splicer) Splice(file string, source []byte, edit Edit) ([]byte, error) {
	if edit.Kind() == TargetModuleAssignment && s.Assignments == nil {
		return nil, fmt.Errorf("%w: %s of %s in %q", ErrUnsupportedTarget, edit.Kind(), edit.Qualname, file)
	}
	before, err := s.units(file, source)
	if err != nil {
		return nil, fmt.Errorf("locate %q: %w", file, err)
	}
	span, replacement, err := plan(source, before, edit)
	if err != nil {
		return nil, fmt.Errorf("%w: %s in %q", err, edit.Qualname, file)
	}
	spliced, err := replace(source, span, replacement)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", edit.Qualname, err)
	}
	after, err := s.units(file, spliced)
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

// units lists the definitions and, when a locator is configured, the module
// assignments of a source.
func (s Splicer) units(file string, source []byte) ([]ecosystem.Definition, error) {
	units, err := s.Locator.Locate(file, source)
	if err != nil {
		return nil, err
	}
	if s.Assignments == nil {
		return units, nil
	}
	assignments, err := s.Assignments.LocateAssignments(file, source)
	if err != nil {
		return nil, err
	}
	return append(units[:len(units):len(units)], assignments...), nil
}

// plan decides which span of source an edit replaces, and with what.
func plan(source []byte, units []ecosystem.Definition, edit Edit) (ecosystem.Span, string, error) {
	if edit.Kind() == TargetModuleAddition {
		if declares(units, edit.Qualname) {
			return ecosystem.Span{}, "", ErrNameTaken
		}
		end := ecosystem.Span{Start: len(source), End: len(source)}
		return end, additionSeparator + edit.NewSource + additionSeparator, nil
	}
	target, err := unique(units, edit)
	if err != nil {
		return ecosystem.Span{}, "", err
	}
	return target.Span, edit.NewSource, nil
}

func replace(source []byte, span ecosystem.Span, replacement string) ([]byte, error) {
	if !span.Within(len(source)) {
		return nil, fmt.Errorf("%w: [%d,%d) outside a source of %d bytes", ecosystem.ErrInvalidSpan, span.Start, span.End, len(source))
	}
	spliced := make([]byte, 0, len(source)-span.Len()+len(replacement))
	spliced = append(spliced, source[:span.Start]...)
	spliced = append(spliced, replacement...)
	return append(spliced, source[span.End:]...), nil
}

// CheckInvariant verifies a splice: the target unit's text equals the new
// source (a module addition has no target before the splice, so only the units
// it leaves alone are checked), and every unit outside the target is
// byte-identical, in the same order.
func CheckInvariant(before []byte, beforeUnits []ecosystem.Definition, after []byte, afterUnits []ecosystem.Definition, edit Edit) error {
	if edit.Kind() != TargetModuleAddition {
		if err := checkTarget(after, afterUnits, edit); err != nil {
			return err
		}
	}
	want, err := outsideTexts(before, beforeUnits, edit)
	if err != nil {
		return err
	}
	got, err := outsideTexts(after, afterUnits, edit)
	if err != nil {
		return err
	}
	if len(want) != len(got) {
		return fmt.Errorf("%w: %d units outside %s became %d", ErrInvariantViolated, len(want), edit.Qualname, len(got))
	}
	for i := range want {
		if want[i].qualname != got[i].qualname || !bytes.Equal(want[i].text, got[i].text) {
			return fmt.Errorf("%w: unit %s changed", ErrInvariantViolated, want[i].qualname)
		}
	}
	return nil
}

func checkTarget(after []byte, units []ecosystem.Definition, edit Edit) error {
	target, err := unique(units, edit)
	if err != nil {
		return fmt.Errorf("%w: target %s: %v", ErrInvariantViolated, edit.Qualname, err)
	}
	text, err := target.Span.Slice(after)
	if err != nil || !bytes.Equal(text, []byte(edit.NewSource)) {
		return fmt.Errorf("%w: target %s differs from the new source", ErrInvariantViolated, edit.Qualname)
	}
	return nil
}

type unitText struct {
	qualname string
	text     []byte
}

// outsideTexts returns the texts of the units an edit leaves alone.
func outsideTexts(source []byte, units []ecosystem.Definition, edit Edit) ([]unitText, error) {
	var texts []unitText
	for _, u := range units {
		if touches(edit, u) {
			continue
		}
		text, err := u.Span.Slice(source)
		if err != nil {
			return nil, err
		}
		texts = append(texts, unitText{qualname: u.Qualname, text: text})
	}
	return texts, nil
}

// touches reports whether an edit replaces, nests inside or declares the unit.
func touches(edit Edit, u ecosystem.Definition) bool {
	switch edit.Kind() {
	case TargetModuleAddition:
		return u.Qualname == edit.Qualname
	case TargetModuleAssignment:
		return u.Kind == ecosystem.KindAssignment && u.Qualname == edit.Qualname
	default:
		return u.Kind != ecosystem.KindAssignment &&
			(u.Qualname == edit.Qualname || strings.HasPrefix(u.Qualname, edit.Qualname+nestedSeparator))
	}
}

func declares(units []ecosystem.Definition, qualname string) bool {
	for _, u := range units {
		if u.Qualname == qualname {
			return true
		}
	}
	return false
}

// unique finds the single unit of the edit's target kind with its qualname.
func unique(units []ecosystem.Definition, edit Edit) (ecosystem.Definition, error) {
	wantAssignment := edit.Kind() == TargetModuleAssignment
	var found []ecosystem.Definition
	for _, u := range units {
		if (u.Kind == ecosystem.KindAssignment) == wantAssignment && u.Qualname == edit.Qualname {
			found = append(found, u)
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
