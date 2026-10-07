package differential

import (
	"encoding/json"
	"hash/fnv"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

// DefaultDraws is the number of inputs drawn for one operation, boundary inputs
// first.
const DefaultDraws = 48

const (
	numberPattern  = `\b\d+(?:\.\d+)?\b`
	stringPattern  = `"([^"\\\n]{0,24})"|'([^'\\\n]{0,24})'`
	decimalPoint   = "."
	maxFractionLen = 6
	decimalBase    = 10
	// The constants of splitmix64.
	mixIncrement = 0x9e3779b97f4a7c15
	mixFirst     = 0xbf58476d1ce4e5b9
	mixSecond    = 0x94d049bb133111eb
	mixShiftA    = 30
	mixShiftB    = 27
	mixShiftC    = 31
	// attemptsPerDraw bounds the random attempts made to find distinct inputs.
	attemptsPerDraw = 8
)

var (
	numberRegexp = regexp.MustCompile(numberPattern)
	stringRegexp = regexp.MustCompile(stringPattern)
	// basicIntegers are drawn for an integer parameter whatever the code says.
	basicIntegers = []string{"0", "1", "-1", "2", "10", "100"}
	// basicDecimals are drawn for a decimal parameter whatever the code says.
	basicDecimals = []string{"0", "0.5", "1", "-1", "99.99", "100.01"}
	// basicStrings are drawn for a string parameter whatever the code says.
	basicStrings = []string{"", "a"}
)

// Pool is the set of values inputs are drawn from: the constants of the code, on
// both sides, and the argument values of the spec examples.
type Pool struct {
	numbers []spec.Literal
	strings []spec.Literal
	byName  map[string][]spec.Literal
}

// NewPool builds the pool from the source text of the code under test and the
// calls of the spec examples. Every number of the code enters with its neighbours
// one unit and one smallest decimal step away, every short string literal enters
// as is, and every example argument enters under its parameter name.
func NewPool(sources []string, examples []spec.Call) Pool {
	pool := Pool{byName: map[string][]spec.Literal{}}
	for _, source := range sources {
		for _, text := range numberRegexp.FindAllString(source, -1) {
			pool.addNumberWithNeighbours(text)
		}
		for _, match := range stringRegexp.FindAllStringSubmatch(source, -1) {
			pool.strings = appendUnique(pool.strings, spec.Literal{Type: spec.TypeString, Value: match[1] + match[2]})
		}
	}
	for _, call := range examples {
		for name, value := range call.Args {
			pool.byName[name] = appendUnique(pool.byName[name], value)
			if value.Type == spec.TypeInteger || value.Type == spec.TypeDecimal {
				pool.addNumberWithNeighbours(value.Value)
			}
		}
	}
	return pool
}

func (p *Pool) addNumberWithNeighbours(text string) {
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		return
	}
	fraction := 0
	if _, after, found := strings.Cut(text, decimalPoint); found {
		fraction = min(len(after), maxFractionLen)
	}
	one := big.NewRat(1, 1)
	step := new(big.Rat).SetFrac64(1, pow10(fraction))
	for _, neighbour := range []*big.Rat{value, new(big.Rat).Add(value, one), new(big.Rat).Sub(value, one), new(big.Rat).Add(value, step), new(big.Rat).Sub(value, step)} {
		p.numbers = appendUnique(p.numbers, numberLiteral(neighbour, fraction))
	}
}

func pow10(exponent int) int64 {
	result := int64(1)
	for range exponent {
		result *= decimalBase
	}
	return result
}

func numberLiteral(value *big.Rat, fraction int) spec.Literal {
	if value.IsInt() {
		return spec.Literal{Type: spec.TypeInteger, Value: value.Num().String()}
	}
	return spec.Literal{Type: spec.TypeDecimal, Value: value.FloatString(fraction)}
}

func appendUnique(values []spec.Literal, value spec.Literal) []spec.Literal {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func literals(kind spec.LiteralType, texts []string) []spec.Literal {
	out := make([]spec.Literal, len(texts))
	for index, text := range texts {
		out[index] = spec.Literal{Type: kind, Value: text}
	}
	return out
}

// candidates lists the values a parameter is drawn from, in a stable order.
func (p Pool) candidates(param Param) []spec.Literal {
	values := slices.Clone(p.byName[param.Name])
	add := func(more ...spec.Literal) {
		for _, value := range more {
			values = appendUnique(values, value)
		}
	}
	switch param.Hint {
	case HintInteger:
		add(literals(spec.TypeInteger, basicIntegers)...)
		add(filterType(p.numbers, spec.TypeInteger)...)
	case HintDecimal:
		add(literals(spec.TypeDecimal, basicDecimals)...)
		add(p.numbers...)
	case HintString:
		add(literals(spec.TypeString, basicStrings)...)
		add(p.strings...)
	case HintBoolean:
		add(spec.Literal{Type: spec.TypeBoolean, Value: "true"}, spec.Literal{Type: spec.TypeBoolean, Value: "false"})
	case HintDatetime:
	default:
		add(literals(spec.TypeInteger, basicIntegers)...)
		add(p.numbers...)
		add(p.strings...)
	}
	return values
}

func filterType(values []spec.Literal, kind spec.LiteralType) []spec.Literal {
	var out []spec.Literal
	for _, value := range values {
		if value.Type == kind {
			out = append(out, value)
		}
	}
	return out
}

// Draw returns up to `draws` distinct inputs for an operation, the same ones for
// the same pool, signature and seed. The boundary inputs come first: each
// parameter is swept over every number of the pool while the others stay on their
// anchor value. Seeded random combinations fill the rest.
func (p Pool) Draw(signature Signature, seed uint64, draws int) []spec.Call {
	if len(signature.Params) == 0 || draws < 1 {
		return nil
	}
	options := make([][]spec.Literal, len(signature.Params))
	for index, param := range signature.Params {
		options[index] = p.candidates(param)
		if len(options[index]) == 0 {
			return nil
		}
	}
	var calls []spec.Call
	seen := map[string]bool{}
	add := func(values []spec.Literal) {
		call := spec.Call{Operation: signature.Operation, Args: map[string]spec.Literal{}}
		for index, param := range signature.Params {
			call.Args[param.Name] = values[index]
		}
		if key := callKey(call); !seen[key] && len(calls) < draws {
			seen[key] = true
			calls = append(calls, call)
		}
	}
	anchor := make([]spec.Literal, len(options))
	for index, values := range options {
		anchor[index] = values[0]
	}
	for index, values := range options {
		for _, value := range values {
			combination := slices.Clone(anchor)
			combination[index] = value
			add(combination)
		}
	}
	rng := newStream(seed, signature.Operation)
	for attempt := 0; len(calls) < draws && attempt < draws*attemptsPerDraw; attempt++ {
		combination := make([]spec.Literal, len(options))
		for index, values := range options {
			combination[index] = values[rng.intn(len(values))]
		}
		add(combination)
	}
	return calls
}

// callKey is a canonical text of a call, the identity inputs are deduplicated and
// ordered by.
func callKey(call spec.Call) string {
	encoded, err := json.Marshal(call)
	if err != nil {
		return call.Operation
	}
	return string(encoded)
}

// stream is a splitmix64 generator: deterministic, with no state outside itself.
type stream struct{ state uint64 }

func newStream(seed uint64, label string) *stream {
	hash := fnv.New64a()
	hash.Write([]byte(label))
	return &stream{state: seed ^ hash.Sum64()}
}

func (s *stream) next() uint64 {
	s.state += mixIncrement
	z := s.state
	z = (z ^ (z >> mixShiftA)) * mixFirst
	z = (z ^ (z >> mixShiftB)) * mixSecond
	return z ^ (z >> mixShiftC)
}

func (s *stream) intn(n int) int { return int(s.next() % uint64(n)) }
