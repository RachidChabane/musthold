package hostproto

import (
	"errors"
	"fmt"
	"slices"
)

// ClassifyMode is the value of the switch `systemone.classify`.
type ClassifyMode string

// Values of the switch.
const (
	// ClassifyAuto answers a request with a classification only for the
	// (decision kind, exact source, session model) entries of the admission table.
	ClassifyAuto ClassifyMode = "auto"
	// ClassifyShadow makes and journals the classifications and never answers.
	ClassifyShadow ClassifyMode = "shadow"
	// ClassifyOff makes none.
	ClassifyOff ClassifyMode = "off"

	// DefaultClassifyMode is the mode of a project that does not set the switch.
	DefaultClassifyMode = ClassifyAuto
)

// ErrClassifyMode is returned for a value the switch does not take.
var ErrClassifyMode = errors.New("invalid systemone.classify")

// ClassifyModes lists the values of the switch.
func ClassifyModes() []ClassifyMode { return []ClassifyMode{ClassifyAuto, ClassifyShadow, ClassifyOff} }

// ParseClassifyMode reads the switch; an empty text is the default.
func ParseClassifyMode(text string) (ClassifyMode, error) {
	if text == "" {
		return DefaultClassifyMode, nil
	}
	if mode := ClassifyMode(text); slices.Contains(ClassifyModes(), mode) {
		return mode, nil
	}
	return "", fmt.Errorf("%w: %q is not one of %v", ErrClassifyMode, text, ClassifyModes())
}
