package golang

import (
	"go/types"
	"strconv"
	"strings"
	"unicode"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang/probeharness"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
)

const (
	contextPackage   = "context"
	contextTypeName  = "Context"
	unmarshalMethod  = "UnmarshalText"
	wordSeparator    = "_"
	unnamedParameter = "arg"
)

// numericTypes are the library types that hold a number and read it from text,
// by the path of their package and their name.
var numericTypes = map[string]probeharness.Kind{
	"time.Time":                             probeharness.KindTime,
	"math/big.Int":                          probeharness.KindInteger,
	"math/big.Rat":                          probeharness.KindDecimal,
	"math/big.Float":                        probeharness.KindDecimal,
	"github.com/shopspring/decimal.Decimal": probeharness.KindDecimal,
}

// hintOfKind is the type hint of a parameter of the kind.
var hintOfKind = map[probeharness.Kind]differential.Hint{
	probeharness.KindBool:    differential.HintBoolean,
	probeharness.KindInt:     differential.HintInteger,
	probeharness.KindUint:    differential.HintInteger,
	probeharness.KindInteger: differential.HintInteger,
	probeharness.KindFloat:   differential.HintDecimal,
	probeharness.KindDecimal: differential.HintDecimal,
	probeharness.KindString:  differential.HintString,
	probeharness.KindText:    differential.HintString,
	probeharness.KindTime:    differential.HintDatetime,
}

// hintOf gives the hint of a parameter; a kind the search cannot draw for is
// unknown.
func hintOf(param probeharness.Param) differential.Hint {
	if hint, known := hintOfKind[param.Kind]; known {
		return hint
	}
	return differential.HintUnknown
}

// classify reads the type of a parameter: the kind of value a literal is decoded
// into and whether the parameter is a pointer to it.
func classify(kind types.Type) (probeharness.Kind, bool) {
	if pointer, isPointer := kind.(*types.Pointer); isPointer {
		pointee := kindOf(pointer.Elem())
		if pointee == probeharness.KindAny || pointee == probeharness.KindUnsupported {
			return probeharness.KindUnsupported, false
		}
		return pointee, true
	}
	return kindOf(kind), false
}

func kindOf(kind types.Type) probeharness.Kind {
	if named, isNamed := kind.(*types.Named); isNamed {
		if known, found := numericTypes[qualifiedName(named)]; found {
			return known
		}
	}
	switch underlying := kind.Underlying().(type) {
	case *types.Basic:
		return basicKind(underlying)
	case *types.Interface:
		if underlying.Empty() {
			return probeharness.KindAny
		}
	case *types.Struct:
		if readsText(kind) {
			return probeharness.KindText
		}
	}
	return probeharness.KindUnsupported
}

func basicKind(basic *types.Basic) probeharness.Kind {
	switch info := basic.Info(); {
	case info&types.IsBoolean != 0:
		return probeharness.KindBool
	case info&types.IsUnsigned != 0:
		return probeharness.KindUint
	case info&types.IsInteger != 0:
		return probeharness.KindInt
	case info&types.IsFloat != 0:
		return probeharness.KindFloat
	case info&types.IsString != 0:
		return probeharness.KindString
	}
	return probeharness.KindUnsupported
}

func qualifiedName(named *types.Named) string {
	if named.Obj().Pkg() == nil {
		return named.Obj().Name()
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

// readsText reports whether a pointer to the type has UnmarshalText.
func readsText(kind types.Type) bool {
	method, _, _ := types.LookupFieldOrMethod(types.NewPointer(kind), true, nil, unmarshalMethod)
	_, isMethod := method.(*types.Func)
	return isMethod
}

// isContext reports whether the type is context.Context.
func isContext(kind types.Type) bool {
	named, isNamed := kind.(*types.Named)
	return isNamed && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == contextPackage && named.Obj().Name() == contextTypeName
}

// parameterName is the name the spec writes a parameter by: its Go name in
// snake case, or `argN` for a parameter the code leaves unnamed.
func parameterName(name string, index int) string {
	if name == "" || name == "_" {
		return unnamedParameter + strconv.Itoa(index)
	}
	return snakeCase(name)
}

// snakeCase spells a Go identifier in lower case, words joined by underscores:
// `unitPrice` and `UnitPrice` become `unit_price`, `parseURLPath` becomes
// `parse_url_path`.
func snakeCase(name string) string {
	runes := []rune(name)
	var builder strings.Builder
	for index, current := range runes {
		if index > 0 && unicode.IsUpper(current) && startsWord(runes, index) {
			builder.WriteString(wordSeparator)
		}
		builder.WriteRune(unicode.ToLower(current))
	}
	return builder.String()
}

// startsWord tells whether the upper-case rune at index opens a new word: it
// follows a lower-case letter or a digit, or ends a run of capitals before a
// lower-case letter.
func startsWord(runes []rune, index int) bool {
	previous := runes[index-1]
	if unicode.IsLower(previous) || unicode.IsDigit(previous) {
		return true
	}
	return unicode.IsUpper(previous) && index+1 < len(runes) && unicode.IsLower(runes[index+1])
}
