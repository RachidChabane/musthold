package operator

import (
	"slices"
	"testing"

	"pgregory.net/rapid"
)

func TestShiftNumber(t *testing.T) {
	tests := []struct {
		literal string
		want    []string
	}{
		{"15", []string{"16", "14", "150"}},
		{"0", []string{"1", "(-1)"}},
		{"1", []string{"2", "0", "10"}},
		{"2.5", []string{"3.5", "1.5", "25.0"}},
		{"0.5", []string{"1.5", "(-0.5)", "5.0"}},
		{"1_000", nil},
		{"0x10", nil},
		{"0755", nil},
		{"1e3", nil},
		{"2i", nil},
		{"abc", nil},
	}
	for _, tt := range tests {
		t.Run(tt.literal+" [verifies SP-85~1]", func(t *testing.T) {
			if got := ShiftNumber(tt.literal); !slices.Equal(got, tt.want) {
				t.Fatalf("ShiftNumber(%q) = %v, want %v", tt.literal, got, tt.want)
			}
		})
	}

	t.Run("an integer stays an integer and differs from the original [verifies SP-85~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			literal := rapid.StringMatching(`(0|[1-9][0-9]{0,12})`).Draw(t, "literal")
			for _, shifted := range ShiftNumber(literal) {
				if shifted == literal {
					t.Fatalf("%q shifted to itself", literal)
				}
				if !integerLiteral.MatchString(shifted) && shifted != "(-1)" {
					t.Fatalf("%q shifted to %q", literal, shifted)
				}
			}
		})
	})
}

func TestShiftDecimalString(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"0.15", []string{"0.16", "0.14", "1.5"}},
		{"2.50", []string{"2.51", "2.49", "25.0"}},
		{"10", []string{"11", "9", "100"}},
		{"0.00", []string{"0.01", "-0.01"}},
		{"-1.5", []string{"-1.4", "-1.6", "-15"}},
		{"0.99", []string{"1.00", "0.98", "9.9"}},
		{"1e3", nil},
		{"abc", nil},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.text+" [verifies SP-85~1]", func(t *testing.T) {
			if got := ShiftDecimalString(tt.text); !slices.Equal(got, tt.want) {
				t.Fatalf("ShiftDecimalString(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}

	t.Run("the shifts keep a decimal form and differ from the original [verifies SP-85~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			text := rapid.StringMatching(`[0-9]{1,6}(\.[0-9]{1,4})?`).Draw(t, "text")
			for _, shifted := range ShiftDecimalString(text) {
				if shifted == text || !decimalString.MatchString(shifted) {
					t.Fatalf("%q shifted to %q", text, shifted)
				}
			}
		})
	})
}

func TestAlternatives(t *testing.T) {
	t.Run("the other members of a vocabulary are returned in order [verifies SP-85~1]", func(t *testing.T) {
		got := Alternatives("<", []string{"<", "<=", ">"})
		if !slices.Equal(got, []string{"<=", ">"}) {
			t.Fatalf("got %v", got)
		}
	})
}
