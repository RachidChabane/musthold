package probeharness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

var errSentinel = errors.New("sentinel")

type customError struct{ code int }

func (e *customError) Error() string { return fmt.Sprint("custom ", e.code) }

type valueError string

func (e valueError) Error() string { return string(e) }

type cents int64

func integerLiteral(value string) Literal { return Literal{Type: literalInteger, Value: value} }
func decimalLiteral(value string) Literal { return Literal{Type: literalDecimal, Value: value} }
func stringLiteral(value string) Literal  { return Literal{Type: literalString, Value: value} }
func boolLiteral(value string) Literal    { return Literal{Type: literalBoolean, Value: value} }
func nullLiteral() Literal                { return Literal{Type: literalNull, Value: literalNull} }
func timeLiteral(value string) Literal    { return Literal{Type: literalDatetime, Value: value} }

func entryOf(function any, kind Kind, names ...string) Entry {
	params := make([]Param, len(names))
	for index, name := range names {
		params[index] = Param{Name: name, Kind: kind}
	}
	return Entry{Target: "t", Func: function, Params: params}
}

func pointerEntryOf(function any, kind Kind, name string) Entry {
	return Entry{Target: "t", Func: function, Params: []Param{{Name: name, Kind: kind, Pointer: true}}}
}

func observeOne(entry Entry, args map[string]Literal) Observation {
	observation, _ := observe([]Entry{entry}, Call{Target: entry.Target, Args: args})
	return observation
}

func TestDecodeByKind(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
		args  map[string]Literal
		want  Observation
	}{
		{"an integer", entryOf(func(a, b int) int { return a * b }, KindInt, "a", "b"), map[string]Literal{"a": integerLiteral("4"), "b": integerLiteral("-3")}, returns(integerLiteral("-12"))},
		{"a decimal literal of a whole value is an integer", entryOf(func(a int) int { return a }, KindInt, "a"), map[string]Literal{"a": decimalLiteral("4.0")}, returns(integerLiteral("4"))},
		{"a fractional value is no integer", entryOf(func(a int) int { return a }, KindInt, "a"), map[string]Literal{"a": decimalLiteral("4.5")}, raises(RaisesArgument)},
		{"an integer beyond its width", entryOf(func(a int8) int8 { return a }, KindInt, "a"), map[string]Literal{"a": integerLiteral("128")}, raises(RaisesArgument)},
		{"a negative unsigned integer", entryOf(func(a uint) uint { return a }, KindUint, "a"), map[string]Literal{"a": integerLiteral("-1")}, raises(RaisesArgument)},
		{"an unsigned integer", entryOf(func(a uint16) uint16 { return a }, KindUint, "a"), map[string]Literal{"a": integerLiteral("65535")}, returns(integerLiteral("65535"))},
		{"a float from an integer", entryOf(func(a float64) float64 { return a / 2 }, KindFloat, "a"), map[string]Literal{"a": integerLiteral("5")}, returns(decimalLiteral("2.5"))},
		{"a float32 renders its shortest text", entryOf(func(a float32) float32 { return a }, KindFloat, "a"), map[string]Literal{"a": decimalLiteral("0.1")}, returns(decimalLiteral("0.1"))},
		{"a float that overflows its width", entryOf(func(a float32) float32 { return a }, KindFloat, "a"), map[string]Literal{"a": decimalLiteral("1e300")}, raises(RaisesArgument)},
		{"a string for a float", entryOf(func(a float64) float64 { return a }, KindFloat, "a"), map[string]Literal{"a": stringLiteral("x")}, raises(RaisesArgument)},
		{"a string", entryOf(func(a string) string { return a + "!" }, KindString, "a"), map[string]Literal{"a": stringLiteral("hi")}, returns(stringLiteral("hi!"))},
		{"a number written as text is a string", entryOf(func(a string) string { return a }, KindString, "a"), map[string]Literal{"a": decimalLiteral("19.90")}, returns(stringLiteral("19.90"))},
		{"a boolean", entryOf(func(a bool) bool { return !a }, KindBool, "a"), map[string]Literal{"a": boolLiteral("true")}, returns(boolLiteral("false"))},
		{"an integer for a boolean", entryOf(func(a bool) bool { return a }, KindBool, "a"), map[string]Literal{"a": integerLiteral("1")}, raises(RaisesArgument)},
		{"a named integer type", entryOf(func(a cents) cents { return a * 2 }, KindInt, "a"), map[string]Literal{"a": integerLiteral("21")}, returns(integerLiteral("42"))},
		{"a datetime with an offset", entryOf(func(a time.Time) time.Time { return a }, KindTime, "a"), map[string]Literal{"a": timeLiteral("2026-03-01T10:00:00+02:00")}, returns(timeLiteral("2026-03-01T10:00:00+02:00"))},
		{"a datetime with Z", entryOf(func(a time.Time) time.Time { return a }, KindTime, "a"), map[string]Literal{"a": timeLiteral("2026-03-01T10:00:00Z")}, returns(timeLiteral("2026-03-01T10:00:00+00:00"))},
		{"a datetime with fractions", entryOf(func(a time.Time) time.Time { return a }, KindTime, "a"), map[string]Literal{"a": timeLiteral("2026-03-01T10:00:00.250")}, returns(timeLiteral("2026-03-01T10:00:00.25+00:00"))},
		{"a malformed datetime", entryOf(func(a time.Time) time.Time { return a }, KindTime, "a"), map[string]Literal{"a": timeLiteral("2026-13-01")}, raises(RaisesArgument)},
		{"a big integer", pointerEntryOf(func(a *big.Int) *big.Int { return new(big.Int).Mul(a, a) }, KindInteger, "a"), map[string]Literal{"a": integerLiteral("99999999999")}, returns(integerLiteral("9999999999800000000001"))},
		{"a big rational", pointerEntryOf(func(a *big.Rat) *big.Rat { return new(big.Rat).Add(a, a) }, KindDecimal, "a"), map[string]Literal{"a": decimalLiteral("0.25")}, returns(decimalLiteral("1/2"))},
		{"a boolean for a library number", pointerEntryOf(func(a *big.Rat) *big.Rat { return a }, KindDecimal, "a"), map[string]Literal{"a": boolLiteral("true")}, raises(RaisesArgument)},
		{"a malformed library number", pointerEntryOf(func(a *big.Rat) *big.Rat { return a }, KindDecimal, "a"), map[string]Literal{"a": stringLiteral("many")}, raises(RaisesArgument)},
		{"an empty interface", entryOf(func(a any) any { return a }, KindAny, "a"), map[string]Literal{"a": integerLiteral("3")}, returns(integerLiteral("3"))},
		{"an unsupported parameter", entryOf(func(a []int) int { return len(a) }, KindUnsupported, "a"), map[string]Literal{"a": integerLiteral("3")}, raises(RaisesArgument)},
		{"a null for an integer", entryOf(func(a int) int { return a }, KindInt, "a"), map[string]Literal{"a": nullLiteral()}, raises(RaisesArgument)},
		{"a missing argument", entryOf(func(a, b int) int { return a }, KindInt, "a", "b"), map[string]Literal{"a": integerLiteral("1")}, raises(RaisesArgument)},
		{"an unexpected argument", entryOf(func(a int) int { return a }, KindInt, "a"), map[string]Literal{"a": integerLiteral("1"), "b": integerLiteral("2")}, raises(RaisesArgument)},
		{"one argument given under two spellings", entryOf(func(a int) int { return a }, KindInt, "unit_price"), map[string]Literal{"unitPrice": integerLiteral("1"), "UnitPrice": integerLiteral("2")}, raises(RaisesArgument)},
		{"an argument under another spelling of its name", entryOf(func(a int) int { return a }, KindInt, "unit_price"), map[string]Literal{"UnitPrice": integerLiteral("1")}, returns(integerLiteral("1"))},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-952~1]", func(t *testing.T) {
			if got := observeOne(tt.entry, tt.args); got != tt.want {
				t.Fatalf("observed %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPointersAndContext(t *testing.T) {
	fallback := func(a *int) int {
		if a == nil {
			return -1
		}
		return *a
	}
	pointer := Entry{Target: "t", Func: fallback, Params: []Param{{Name: "a", Kind: KindInt, Pointer: true}}}
	t.Run("a null is a nil pointer [verifies SP-952~1]", func(t *testing.T) {
		if got := observeOne(pointer, map[string]Literal{"a": nullLiteral()}); got != returns(integerLiteral("-1")) {
			t.Fatalf("observed %+v", got)
		}
	})
	t.Run("a value is the pointee [verifies SP-952~1]", func(t *testing.T) {
		if got := observeOne(pointer, map[string]Literal{"a": integerLiteral("9")}); got != returns(integerLiteral("9")) {
			t.Fatalf("observed %+v", got)
		}
	})
	t.Run("a context is supplied before the parameters [verifies SP-952~1]", func(t *testing.T) {
		withContext := Entry{Target: "t", Func: func(ctx context.Context, a int) int {
			if ctx == nil {
				return -1
			}
			return a
		}, Context: true, Params: []Param{{Name: "a", Kind: KindInt}}}
		if got := observeOne(withContext, map[string]Literal{"a": integerLiteral("4")}); got != returns(integerLiteral("4")) {
			t.Fatalf("observed %+v", got)
		}
	})
}

func TestResultsAndErrors(t *testing.T) {
	sentinels := []Sentinel{{Name: "ErrSentinel", Value: errSentinel}}
	tests := []struct {
		name  string
		entry Entry
		want  Observation
	}{
		{"no result is null", Entry{Func: func() {}}, returns(nullLiteral())},
		{"a nil pointer result is null", Entry{Func: func() *int { return nil }}, returns(nullLiteral())},
		{"a nil error is no raise", Entry{Func: func() (int, error) { return 3, nil }, ReturnsError: true}, returns(integerLiteral("3"))},
		{"only a nil error is null", Entry{Func: func() error { return nil }, ReturnsError: true}, returns(nullLiteral())},
		{"an error variable is named by the variable", Entry{Func: func() (int, error) { return 0, errSentinel }, ReturnsError: true, Sentinels: sentinels}, raises("ErrSentinel")},
		{"a wrapped error variable is named by the variable", Entry{Func: func() error { return fmt.Errorf("context: %w", errSentinel) }, ReturnsError: true, Sentinels: sentinels}, raises("ErrSentinel")},
		{"an error of a pointer type is named by the type", Entry{Func: func() error { return &customError{code: 1} }, ReturnsError: true}, raises("customError")},
		{"an error of a value type is named by the type", Entry{Func: func() error { return valueError("x") }, ReturnsError: true}, raises("valueError")},
		{"a fmt wrapper is looked through", Entry{Func: func() error { return fmt.Errorf("context: %w", &customError{code: 2}) }, ReturnsError: true}, raises("customError")},
		{"an error made by fmt alone is named by what it is", Entry{Func: func() error { return fmt.Errorf("plain %d", 1) }, ReturnsError: true}, raises("errorString")},
		{"a panic with an error is named as the error", Entry{Func: func() int { panic(errSentinel) }, Sentinels: sentinels}, raises("ErrSentinel")},
		{"a panic with a value is a panic", Entry{Func: func() int { panic("boom") }}, raises(RaisesPanic)},
		{"a runtime failure is an error of its type", Entry{Func: func() int { var values []int; return values[3] }}, raises("boundsError")},
		{"two results are one string", Entry{Func: func() (int, string) { return 1, "a" }}, returns(stringLiteral(`["1","a"]`))},
		{"a float that is not a number is a string", Entry{Func: func() float64 { return math.NaN() }}, returns(stringLiteral("NaN"))},
		{"an unsigned integer", Entry{Func: func() uint8 { return 200 }}, returns(integerLiteral("200"))},
		{"a structure is its JSON", Entry{Func: func() any { return struct{ A int }{A: 1} }}, returns(stringLiteral(`{"A":1}`))},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-951~1]", func(t *testing.T) {
			tt.entry.Target = "t"
			if got := observeOne(tt.entry, map[string]Literal{}); got != tt.want {
				t.Fatalf("observed %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUnknownTarget(t *testing.T) {
	t.Run("a target the table lacks is raised as such [verifies SP-951~1]", testUnknownTarget)
}

func testUnknownTarget(t *testing.T) {
	got, timedOut := observe(nil, Call{Target: "absent"})
	if got != raises(RaisesUnknownTarget) || timedOut {
		t.Fatalf("observed %+v (timed out %t)", got, timedOut)
	}
}

func TestRunOfTheProgram(t *testing.T) {
	directory := t.TempDir()
	request := filepath.Join(directory, "request.json")
	response := filepath.Join(directory, "response.jsonl")
	entries := []Entry{
		entryOf(func(a int) int { return a + 1 }, KindInt, "a"),
	}
	entries[0].Target = "inc"
	write := func(calls ...Call) {
		encoded, err := json.Marshal(Request{Calls: calls})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(request, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("each call has a line of the response in order [verifies SP-954~1]", func(t *testing.T) {
		write(Call{Target: "inc", Args: map[string]Literal{"a": integerLiteral("1")}}, Call{Target: "absent"}, Call{Target: "inc", Args: map[string]Literal{"a": integerLiteral("5")}})
		if status := Main([]string{request, response}, entries); status != ExitClean {
			t.Fatalf("status %d", status)
		}
		content, err := os.ReadFile(response)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"kind":"returns","type":"integer","value":"2"}` + "\n" +
			`{"kind":"raises","value":"UnknownTarget"}` + "\n" +
			`{"kind":"returns","type":"integer","value":"6"}` + "\n"
		if string(content) != want {
			t.Fatalf("response:\n%s\nwant:\n%s", content, want)
		}
	})

	t.Run("a request that cannot be read fails with its own status [verifies SP-954~1]", func(t *testing.T) {
		if status := Main([]string{filepath.Join(directory, "absent.json"), response}, entries); status != ExitFailure {
			t.Fatalf("status %d", status)
		}
		if err := os.WriteFile(request, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if status := Main([]string{request, response}, entries); status != ExitFailure {
			t.Fatalf("status %d for a malformed request", status)
		}
		if status := Main([]string{request}, entries); status != ExitFailure {
			t.Fatalf("status %d for missing arguments", status)
		}
	})

	t.Run("a response that cannot be written fails with its own status [verifies SP-954~1]", func(t *testing.T) {
		write()
		if status := Main([]string{request, directory}, entries); status != ExitFailure {
			t.Fatalf("status %d", status)
		}
	})
}

func TestProgramSourceIsAMainPackage(t *testing.T) {
	t.Run("the harness is built as a main package [verifies SP-953~1]", testProgramSource)
}

func testProgramSource(t *testing.T) {
	source := string(ProgramSource())
	if !strings.Contains(source, "package main\n") {
		t.Fatal("the program source does not declare package main")
	}
	if strings.Contains(source, "package probeharness\n") {
		t.Fatal("the program source still declares package probeharness")
	}
}

func TestIntegersRoundTrip(t *testing.T) {
	t.Run("an integer is returned as it was given [verifies SP-952~1]", testIntegersRoundTrip)
}

func testIntegersRoundTrip(t *testing.T) {
	identity := Entry{Target: "t", Func: func(a int64) int64 { return a }, Params: []Param{{Name: "a", Kind: KindInt}}}
	rapid.Check(t, func(t *rapid.T) {
		value := rapid.Int64().Draw(t, "value")
		literal := integerLiteral(fmt.Sprint(value))
		if got := observeOne(identity, map[string]Literal{"a": literal}); got != returns(literal) {
			t.Fatalf("%d became %+v", value, got)
		}
	})
}

func TestNameSpellingsAreOneName(t *testing.T) {
	t.Run("the spellings of a name are one name [verifies SP-952~1]", testNameSpellings)
}

func testNameSpellings(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		words := rapid.SliceOfN(rapid.StringMatching(`[a-z]{1,6}`), 1, 4).Draw(t, "words")
		snake := strings.Join(words, "_")
		camel := words[0]
		for _, word := range words[1:] {
			camel += strings.ToUpper(word[:1]) + word[1:]
		}
		if normalize(snake) != normalize(camel) {
			t.Fatalf("%q and %q are different names", snake, camel)
		}
	})
}
