package operator

import (
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	decimalBase       = 10
	floatPrecisionAny = -1
	floatBits         = 64
	maxShiftedFloat   = 1e15
	floatMarker       = ".0"
)

var (
	integerLiteral = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	floatLiteral   = regexp.MustCompile(`^(0|[1-9][0-9]*)?\.[0-9]+$|^(0|[1-9][0-9]*)\.[0-9]*$`)
	decimalString  = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]+))?$`)
)

// ShiftNumber returns the literals a numeric constant is mutated to: the value
// plus one, minus one and times ten, written in the form of the original
// (integer stays integer, float stays float). Negative results are
// parenthesized so they are valid wherever the literal stood. Forms with
// separators, prefixes, exponents or imaginary suffixes yield nothing.
func ShiftNumber(literal string) []string {
	switch {
	case integerLiteral.MatchString(literal):
		return shiftInteger(literal)
	case floatLiteral.MatchString(literal):
		return shiftFloat(literal)
	default:
		return nil
	}
}

func shiftInteger(literal string) []string {
	value, ok := new(big.Int).SetString(literal, decimalBase)
	if !ok {
		return nil
	}
	one, ten := big.NewInt(1), big.NewInt(decimalBase)
	shifted := []*big.Int{
		new(big.Int).Add(value, one),
		new(big.Int).Sub(value, one),
		new(big.Int).Mul(value, ten),
	}
	var out []string
	for _, candidate := range shifted {
		out = appendDistinct(out, literal, parenthesizeNegative(candidate.String()))
	}
	return out
}

func shiftFloat(literal string) []string {
	value, err := strconv.ParseFloat(literal, floatBits)
	if err != nil || value > maxShiftedFloat {
		return nil
	}
	var out []string
	for _, candidate := range []float64{value + 1, value - 1, value * decimalBase} {
		text := strconv.FormatFloat(candidate, 'f', floatPrecisionAny, floatBits)
		if !strings.Contains(text, ".") {
			text += floatMarker
		}
		out = appendDistinct(out, literal, parenthesizeNegative(text))
	}
	return out
}

func parenthesizeNegative(text string) string {
	if strings.HasPrefix(text, "-") {
		return "(" + text + ")"
	}
	return text
}

func appendDistinct(list []string, original, candidate string) []string {
	if candidate == original || slices.Contains(list, candidate) {
		return list
	}
	return append(list, candidate)
}

// ShiftDecimalString returns the decimal strings a `Decimal("…")` argument is
// mutated to: the last digit plus one, the last digit minus one and times ten,
// all keeping the scale of the original where times ten allows. Anything that is
// not a plain signed decimal yields nothing.
func ShiftDecimalString(text string) []string {
	match := decimalString.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	sign, whole, fraction := match[1], match[2], match[3]
	unscaled, _ := new(big.Int).SetString(whole+fraction, decimalBase)
	if sign == "-" {
		unscaled.Neg(unscaled)
	}
	scale := len(fraction)
	one := big.NewInt(1)
	ten := big.NewInt(decimalBase)

	timesTen, timesTenScale := new(big.Int).Set(unscaled), scale-1
	if scale == 0 {
		timesTen, timesTenScale = new(big.Int).Mul(unscaled, ten), 0
	}
	candidates := []string{
		renderDecimal(new(big.Int).Add(unscaled, one), scale),
		renderDecimal(new(big.Int).Sub(unscaled, one), scale),
	}
	if unscaled.Sign() != 0 {
		candidates = append(candidates, renderDecimal(timesTen, timesTenScale))
	}
	var out []string
	for _, candidate := range candidates {
		out = appendDistinct(out, text, candidate)
	}
	return out
}

func renderDecimal(unscaled *big.Int, scale int) string {
	negative := unscaled.Sign() < 0
	digits := new(big.Int).Abs(unscaled).String()
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	text := digits
	if scale > 0 {
		text = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}
	if negative {
		return "-" + text
	}
	return text
}

// Alternatives returns the members of vocabulary other than current, in
// vocabulary order.
func Alternatives(current string, vocabulary []string) []string {
	var others []string
	for _, item := range vocabulary {
		if item != current {
			others = append(others, item)
		}
	}
	return others
}
