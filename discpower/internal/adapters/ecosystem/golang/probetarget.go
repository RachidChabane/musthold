package golang

import (
	"errors"
	"fmt"
	"go/token"
	"strings"
)

// ErrInvalidTarget is returned for a bound operation that does not name a
// callable function or method of a package.
var ErrInvalidTarget = errors.New("invalid operation target")

const (
	targetSeparator   = ":"
	receiverSeparator = "."
)

// probeTarget is a bound operation: a function `import/path:Func` or a method
// `import/path:Type.Method` of an exported type, called on its zero value.
type probeTarget struct {
	// Text is the target as the configuration spells it.
	Text       string
	ImportPath string
	Receiver   string
	Name       string
}

// parseProbeTarget reads a target and refuses one whose function, method or
// receiver is not exported, since the probe program lives outside the package.
func parseProbeTarget(text string) (probeTarget, error) {
	separator := strings.LastIndex(text, targetSeparator)
	if separator <= 0 || separator == len(text)-len(targetSeparator) {
		return probeTarget{}, fmt.Errorf("%w: %q is not `import/path:Function`", ErrInvalidTarget, text)
	}
	target := probeTarget{Text: text, ImportPath: text[:separator], Name: text[separator+len(targetSeparator):]}
	if receiver, method, isMethod := strings.Cut(target.Name, receiverSeparator); isMethod {
		target.Receiver, target.Name = receiver, method
	}
	if !token.IsExported(target.Name) || (target.Receiver != "" && !token.IsExported(target.Receiver)) {
		return probeTarget{}, fmt.Errorf("%w: %q is not exported", ErrInvalidTarget, text)
	}
	return target, nil
}
