package kit

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"time"
)

var (
	ErrNegative = errors.New("negative amount")
	ErrLimit    = errors.New("limit reached")
)

// RangeError reports a value outside its bounds.
type RangeError struct{ Low, High int }

func (e *RangeError) Error() string { return fmt.Sprintf("outside %d..%d", e.Low, e.High) }

// Money counts cents.
type Money int64

// Counter adds to a base.
type Counter struct{ Base int }

func (c *Counter) Add(amount int) int { return c.Base + amount }

func (c Counter) Double(amount int) int { return 2 * (c.Base + amount) }

func Sqrt(value int) (int, error) {
	if value < 0 {
		return 0, ErrNegative
	}
	root := 0
	for (root+1)*(root+1) <= value {
		root++
	}
	return root, nil
}

func Clamp(value, low, high int) (int, error) {
	if value < low || value > high {
		return 0, &RangeError{Low: low, High: high}
	}
	return value, nil
}

func Limit(value int) error {
	if value > 10 {
		return fmt.Errorf("value %d: %w", value, ErrLimit)
	}
	return nil
}

func Half(value float64) float64 { return value / 2 }

func Greet(name string, loud bool) string {
	greeting := "hello " + name
	if loud {
		return strings.ToUpper(greeting)
	}
	return greeting
}

func NextDay(day time.Time) time.Time { return day.Add(24 * time.Hour) }

func Scale(rate *big.Rat, factor *big.Int) *big.Rat {
	return new(big.Rat).Mul(rate, new(big.Rat).SetInt(factor))
}

func Fallback(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}

func Echo(value any) any { return value }

func Cents(amount Money) Money { return amount * 100 }

func Narrow(value uint8) uint8 { return value + 1 }

func Divmod(dividend, divisor int) (int, int) { return dividend / divisor, dividend % divisor }

func Divide(dividend, divisor int) int { return dividend / divisor }

func Join(separator string, parts ...string) string { return strings.Join(parts, separator) }

func Version() string { return "1.0" }

func Deadline(ctx context.Context, seconds int) int {
	if ctx == nil {
		return -1
	}
	return seconds
}

func Boom(value int) int { panic(fmt.Sprintf("boom %d", value)) }

func Refuse(value int) int { panic(ErrNegative) }

func Spin(value int) int {
	for {
		value++
	}
}

func Quit(value int) int {
	os.Exit(3)
	return value
}

func Records(count int) []string {
	records := make([]string, count)
	for index := range records {
		records[index] = fmt.Sprint("r", index)
	}
	return records
}

func Zone(value int) string {
	name, _ := time.Now().Zone()
	return name
}

func Roll(sides int) int { return rand.Intn(sides) }

func Generic[T any](value T) T { return value }
