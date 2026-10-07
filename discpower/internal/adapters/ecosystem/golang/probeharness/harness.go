// Package probeharness is the runtime of the Go prober: the program that calls
// the bound operations of a project with the typed literals of a spec example and
// reports what each call returned or raised.
//
// The file is compiled twice: as this package, where it is tested, and as the
// `main` package of a probe program built inside the project's module, from the
// source embedded by Source. It therefore imports the standard library only and
// keeps to constructs every supported Go language version accepts.
package probeharness

import (
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Kind classifies the type of a parameter the way the prober's go/types analysis
// does; it says how a literal is decoded into a value of that type.
type Kind string

// Kinds of parameters. A pointer to any of them is a Param with Pointer set.
const (
	KindBool        Kind = "bool"
	KindInt         Kind = "int"
	KindUint        Kind = "uint"
	KindFloat       Kind = "float"
	KindString      Kind = "string"
	KindTime        Kind = "time"
	KindInteger     Kind = "integer"
	KindDecimal     Kind = "decimal"
	KindText        Kind = "text"
	KindAny         Kind = "any"
	KindUnsupported Kind = "unsupported"
)

// Names of what a call raises when it never reached the operation or did not end.
const (
	RaisesArgument      = "ArgumentError"
	RaisesUnknownTarget = "UnknownTarget"
	RaisesTimeout       = "Timeout"
	RaisesPanic         = "Panic"
	RaisesCrash         = "Crash"
)

// Types of the literals and kinds of the observations of the spec grammar.
const (
	literalInteger  = "integer"
	literalDecimal  = "decimal"
	literalString   = "string"
	literalBoolean  = "boolean"
	literalNull     = "null"
	literalDatetime = "datetime"
	observeReturns  = "returns"
	observeRaises   = "raises"
	trueText        = "true"
	falseText       = "false"
)

const (
	// CallTimeout bounds one call of an operation.
	CallTimeout = 5 * time.Second

	// ExitClean is the exit status of a run that ended by itself, whether it
	// answered every call or stopped after a timeout.
	ExitClean = 0
	// ExitFailure is the exit status of a run that could not read its request or
	// write its response. It is distinct from the statuses of the Go runtime, so
	// that a call that kills the program is not mistaken for a broken harness.
	ExitFailure = 70

	responseMode         = 0o600
	decimalBase          = 10
	bitSize64            = 64
	argumentCount        = 2
	nameSeparator        = "_"
	timeOffsetLayout     = "2006-01-02T15:04:05.999999999-07:00"
	timeNaiveLayout      = "2006-01-02T15:04:05.999999999"
	timeDateLayout       = "2006-01-02"
	packageOfStandardFmt = "fmt"
)

// Param is one parameter of an operation, named as the spec writes it.
type Param struct {
	Name    string
	Kind    Kind
	Pointer bool
}

// Sentinel is an exported error variable of the package of an operation; a call
// that returns an error matching it raises the variable's name.
type Sentinel struct {
	Name  string
	Value error
}

// Entry is one operation the program can call.
type Entry struct {
	Target string
	Func   any
	// Context is true when the function takes a context.Context first, which the
	// program supplies.
	Context      bool
	Params       []Param
	ReturnsError bool
	Sentinels    []Sentinel
}

// Literal is a typed value written as text, as the spec grammar has it.
type Literal struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Call is one call to make.
type Call struct {
	Target string             `json:"target"`
	Args   map[string]Literal `json:"args"`
}

// Request is the file the program reads.
type Request struct {
	Calls []Call `json:"calls"`
}

// Observation is what one call did, one per line of the response file.
type Observation struct {
	Kind  string `json:"kind"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value"`
}

// Main reads the request file named by the first argument, calls each operation
// in order and appends one observation per call to the file named by the second
// argument. A call that exceeds CallTimeout ends the run after its observation,
// so a runaway call cannot slow the ones after it; the caller starts again at
// the next call.
func Main(arguments []string, entries []Entry) int {
	if len(arguments) != argumentCount {
		fmt.Fprintln(os.Stderr, "usage: probe <request> <response>")
		return ExitFailure
	}
	content, err := os.ReadFile(arguments[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitFailure
	}
	var request Request
	if err := json.Unmarshal(content, &request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitFailure
	}
	response, err := os.OpenFile(arguments[1], os.O_CREATE|os.O_WRONLY|os.O_APPEND, responseMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitFailure
	}
	defer response.Close()
	for _, call := range request.Calls {
		observation, timedOut := observe(entries, call)
		line, err := json.Marshal(observation)
		if err == nil {
			_, err = response.Write(append(line, '\n'))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ExitFailure
		}
		if timedOut {
			break
		}
	}
	return ExitClean
}

func raises(name string) Observation { return Observation{Kind: observeRaises, Value: name} }

func returns(literal Literal) Observation {
	return Observation{Kind: observeReturns, Type: literal.Type, Value: literal.Value}
}

func observe(entries []Entry, call Call) (Observation, bool) {
	entry, found := find(entries, call.Target)
	if !found {
		return raises(RaisesUnknownTarget), false
	}
	arguments, err := decodeArguments(entry, call.Args)
	if err != nil {
		return raises(RaisesArgument), false
	}
	return callWithDeadline(entry, arguments)
}

func find(entries []Entry, target string) (Entry, bool) {
	for _, entry := range entries {
		if entry.Target == target {
			return entry, true
		}
	}
	return Entry{}, false
}

type outcome struct {
	results  []reflect.Value
	panicked bool
	failure  any
}

func callWithDeadline(entry Entry, arguments []reflect.Value) (Observation, bool) {
	done := make(chan outcome, 1)
	go func() {
		var result outcome
		defer func() {
			if failure := recover(); failure != nil {
				result = outcome{panicked: true, failure: failure}
			}
			done <- result
		}()
		result.results = reflect.ValueOf(entry.Func).Call(arguments)
	}()
	timer := time.NewTimer(CallTimeout)
	defer timer.Stop()
	select {
	case result := <-done:
		return observation(entry, result), false
	case <-timer.C:
		return raises(RaisesTimeout), true
	}
}

func observation(entry Entry, result outcome) Observation {
	if result.panicked {
		if failure, isError := result.failure.(error); isError {
			return raises(errorName(entry.Sentinels, failure))
		}
		return raises(RaisesPanic)
	}
	values := result.results
	if entry.ReturnsError {
		last := values[len(values)-1]
		values = values[:len(values)-1]
		if !last.IsNil() {
			return raises(errorName(entry.Sentinels, last.Interface().(error)))
		}
	}
	switch len(values) {
	case 0:
		return returns(Literal{Type: literalNull, Value: literalNull})
	case 1:
		return returns(render(values[0]))
	}
	texts := make([]string, len(values))
	for index, value := range values {
		texts[index] = render(value).Value
	}
	encoded, _ := json.Marshal(texts)
	return returns(Literal{Type: literalString, Value: string(encoded)})
}

// errorName names an error by the exported variable of the package that it is,
// else by its type, looking through the wrappers of the fmt package.
func errorName(sentinels []Sentinel, err error) string {
	for _, sentinel := range sentinels {
		if errors.Is(err, sentinel.Value) {
			return sentinel.Name
		}
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if kind := concreteType(current); kind.PkgPath() != packageOfStandardFmt {
			return kind.Name()
		}
	}
	return concreteType(err).Name()
}

func concreteType(err error) reflect.Type {
	kind := reflect.TypeOf(err)
	for kind.Kind() == reflect.Ptr {
		kind = kind.Elem()
	}
	return kind
}

// normalize is the spelling parameter names are compared in: `unitPrice`,
// `unit_price` and `UnitPrice` are one name.
func normalize(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, nameSeparator, ""))
}

func decodeArguments(entry Entry, given map[string]Literal) ([]reflect.Value, error) {
	function := reflect.TypeOf(entry.Func)
	byName := map[string]Literal{}
	for name, literal := range given {
		key := normalize(name)
		if _, duplicate := byName[key]; duplicate {
			return nil, fmt.Errorf("argument %q given twice", name)
		}
		byName[key] = literal
	}
	var arguments []reflect.Value
	offset := 0
	if entry.Context {
		arguments = append(arguments, reflect.ValueOf(context.Background()))
		offset = 1
	}
	for index, param := range entry.Params {
		key := normalize(param.Name)
		literal, present := byName[key]
		if !present {
			return nil, fmt.Errorf("argument %q missing", param.Name)
		}
		delete(byName, key)
		value, err := decode(param, function.In(index+offset), literal)
		if err != nil {
			return nil, fmt.Errorf("argument %q: %w", param.Name, err)
		}
		arguments = append(arguments, value)
	}
	if len(byName) > 0 {
		return nil, fmt.Errorf("%d unexpected arguments", len(byName))
	}
	return arguments, nil
}

func decode(param Param, target reflect.Type, literal Literal) (reflect.Value, error) {
	if literal.Type == literalNull {
		return decodeNull(target)
	}
	base := target
	if param.Pointer {
		base = target.Elem()
	}
	value := reflect.New(base).Elem()
	if err := fill(param.Kind, value, literal); err != nil {
		return reflect.Value{}, err
	}
	if !param.Pointer {
		return value, nil
	}
	pointer := reflect.New(base)
	pointer.Elem().Set(value)
	return pointer, nil
}

func decodeNull(target reflect.Type) (reflect.Value, error) {
	switch target.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
		return reflect.Zero(target), nil
	}
	return reflect.Value{}, fmt.Errorf("null for %s", target)
}

func fill(kind Kind, value reflect.Value, literal Literal) error {
	switch kind {
	case KindBool:
		return fillBool(value, literal)
	case KindInt:
		return fillInt(value, literal)
	case KindUint:
		return fillUint(value, literal)
	case KindFloat:
		return fillFloat(value, literal)
	case KindString:
		value.SetString(literal.Value)
		return nil
	case KindTime:
		return fillTime(value, literal)
	case KindInteger, KindDecimal, KindText:
		return fillText(value, literal)
	case KindAny:
		return fillAny(value, literal)
	}
	return fmt.Errorf("unsupported parameter type %s", value.Type())
}

func fillBool(value reflect.Value, literal Literal) error {
	if literal.Type != literalBoolean {
		return fmt.Errorf("%s literal for a boolean", literal.Type)
	}
	value.SetBool(literal.Value == trueText)
	return nil
}

func number(literal Literal) (*big.Rat, error) {
	if literal.Type != literalInteger && literal.Type != literalDecimal {
		return nil, fmt.Errorf("%s literal for a number", literal.Type)
	}
	parsed, ok := new(big.Rat).SetString(literal.Value)
	if !ok {
		return nil, fmt.Errorf("%q is not a number", literal.Value)
	}
	return parsed, nil
}

func wholeNumber(literal Literal) (*big.Int, error) {
	parsed, err := number(literal)
	if err != nil {
		return nil, err
	}
	if !parsed.IsInt() {
		return nil, fmt.Errorf("%q is not a whole number", literal.Value)
	}
	return parsed.Num(), nil
}

func fillInt(value reflect.Value, literal Literal) error {
	whole, err := wholeNumber(literal)
	if err != nil {
		return err
	}
	if !whole.IsInt64() || value.OverflowInt(whole.Int64()) {
		return fmt.Errorf("%s overflows %s", literal.Value, value.Type())
	}
	value.SetInt(whole.Int64())
	return nil
}

func fillUint(value reflect.Value, literal Literal) error {
	whole, err := wholeNumber(literal)
	if err != nil {
		return err
	}
	if !whole.IsUint64() || value.OverflowUint(whole.Uint64()) {
		return fmt.Errorf("%s overflows %s", literal.Value, value.Type())
	}
	value.SetUint(whole.Uint64())
	return nil
}

func fillFloat(value reflect.Value, literal Literal) error {
	if _, err := number(literal); err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(literal.Value, bitSize64)
	if err != nil || value.OverflowFloat(parsed) {
		return fmt.Errorf("%s overflows %s", literal.Value, value.Type())
	}
	value.SetFloat(parsed)
	return nil
}

func fillTime(value reflect.Value, literal Literal) error {
	if literal.Type != literalDatetime && literal.Type != literalString {
		return fmt.Errorf("%s literal for a time", literal.Type)
	}
	for _, layout := range []string{timeOffsetLayout, time.RFC3339Nano, timeNaiveLayout, timeDateLayout} {
		if parsed, err := time.ParseInLocation(layout, literal.Value, time.UTC); err == nil {
			value.Set(reflect.ValueOf(parsed).Convert(value.Type()))
			return nil
		}
	}
	return fmt.Errorf("%q is not an ISO-8601 datetime", literal.Value)
}

func fillText(value reflect.Value, literal Literal) error {
	if literal.Type == literalBoolean {
		return fmt.Errorf("boolean literal for %s", value.Type())
	}
	unmarshaler, ok := value.Addr().Interface().(encoding.TextUnmarshaler)
	if !ok {
		return fmt.Errorf("%s cannot read text", value.Type())
	}
	return unmarshaler.UnmarshalText([]byte(literal.Value))
}

func fillAny(value reflect.Value, literal Literal) error {
	if value.NumMethod() != 0 {
		return fmt.Errorf("%s is not the empty interface", value.Type())
	}
	var natural any
	switch literal.Type {
	case literalInteger:
		parsed, err := strconv.ParseInt(literal.Value, decimalBase, bitSize64)
		if err != nil {
			return err
		}
		natural = int(parsed)
	case literalDecimal:
		parsed, err := strconv.ParseFloat(literal.Value, bitSize64)
		if err != nil {
			return err
		}
		natural = parsed
	case literalBoolean:
		natural = literal.Value == trueText
	default:
		natural = literal.Value
	}
	value.Set(reflect.ValueOf(natural))
	return nil
}

var timeType = reflect.TypeOf(time.Time{})

// render writes a value as the literal that stands for it.
func render(value reflect.Value) Literal {
	for value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return Literal{Type: literalNull, Value: literalNull}
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Bool:
		return Literal{Type: literalBoolean, Value: strconv.FormatBool(value.Bool())}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Literal{Type: literalInteger, Value: strconv.FormatInt(value.Int(), decimalBase)}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return Literal{Type: literalInteger, Value: strconv.FormatUint(value.Uint(), decimalBase)}
	case reflect.Float32, reflect.Float64:
		return renderFloat(value)
	case reflect.String:
		return Literal{Type: literalString, Value: value.String()}
	}
	return renderStructured(value)
}

func renderFloat(value reflect.Value) Literal {
	bits := bitSize64
	if value.Kind() == reflect.Float32 {
		bits = 32
	}
	text := strconv.FormatFloat(value.Float(), 'f', -1, bits)
	if _, ok := new(big.Rat).SetString(text); !ok {
		return Literal{Type: literalString, Value: text}
	}
	return Literal{Type: literalDecimal, Value: text}
}

func renderStructured(value reflect.Value) Literal {
	if value.Type() == timeType {
		return Literal{Type: literalDatetime, Value: value.Interface().(time.Time).Format(timeOffsetLayout)}
	}
	holder := reflect.New(value.Type())
	holder.Elem().Set(value)
	if marshaler, ok := holder.Interface().(encoding.TextMarshaler); ok {
		if text, err := marshaler.MarshalText(); err == nil {
			return renderText(string(text))
		}
	}
	encoded, err := json.Marshal(value.Interface())
	if err != nil {
		return Literal{Type: literalString, Value: fmt.Sprint(value.Interface())}
	}
	return Literal{Type: literalString, Value: string(encoded)}
}

// renderText types the text of a value that marshals itself: an integer or a
// decimal when it reads as a number, else a string. A fraction such as `1/3`
// stays as written, since no decimal spelling of it is exact.
func renderText(text string) Literal {
	number, ok := new(big.Rat).SetString(text)
	switch {
	case !ok:
		return Literal{Type: literalString, Value: text}
	case number.IsInt():
		return Literal{Type: literalInteger, Value: number.Num().String()}
	}
	return Literal{Type: literalDecimal, Value: text}
}
