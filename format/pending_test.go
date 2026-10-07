package format_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

const verifiesPending = " [verifies SP-196~1]"

func TestDecodePending(t *testing.T) {
	valid := `{"module":"discpower","pending":[` +
		`{"criterion":"PARK-1~1","reason":"awaiting_host"},` +
		`{"criterion":"PARK-2~3","reason":"awaiting_host_judgment"}],"unknown":true}`

	t.Run("a valid file is decoded and unknown members are ignored"+verifiesPending, func(t *testing.T) {
		pending, err := format.DecodePending([]byte(valid))
		if err != nil {
			t.Fatal(err)
		}
		if pending.Module != "discpower" || len(pending.Pending) != 2 ||
			pending.Pending[1].Criterion != "PARK-2~3" || pending.Pending[1].Reason != format.PendingAwaitingHostJudgment {
			t.Fatalf("decoded %+v", pending)
		}
	})

	t.Run("an empty list of entries is valid"+verifiesPending, func(t *testing.T) {
		if _, err := format.DecodePending([]byte(`{"module":"alignment","pending":[]}`)); err != nil {
			t.Fatal(err)
		}
	})

	invalid := []struct {
		name     string
		document string
		wantCode format.ViolationCode
	}{
		{"a missing module", `{"pending":[]}`, format.CodeMissingField},
		{"a module that is not a path segment", `{"module":"a/b","pending":[]}`, format.CodeMissingField},
		{"a criterion without revision", `{"module":"m","pending":[{"criterion":"PARK-1","reason":"awaiting_host"}]}`, format.CodeRequirement},
		{"a criterion with revision zero", `{"module":"m","pending":[{"criterion":"PARK-1~0","reason":"awaiting_host"}]}`, format.CodeRequirement},
		{"a criterion with a padded revision", `{"module":"m","pending":[{"criterion":"PARK-1~01","reason":"awaiting_host"}]}`, format.CodeRequirement},
		{"an unknown reason", `{"module":"m","pending":[{"criterion":"PARK-1~1","reason":"quota_paused"}]}`, format.CodeUnknownValue},
		{"entries out of order", `{"module":"m","pending":[{"criterion":"PARK-2~1","reason":"awaiting_host"},{"criterion":"PARK-1~1","reason":"awaiting_host"}]}`, format.CodeUnsorted},
		{"a duplicate entry", `{"module":"m","pending":[{"criterion":"PARK-1~1","reason":"awaiting_host"},{"criterion":"PARK-1~1","reason":"awaiting_host"}]}`, format.CodeUnsorted},
	}
	for _, tt := range invalid {
		t.Run(tt.name+" is refused"+verifiesPending, func(t *testing.T) {
			_, err := format.DecodePending([]byte(tt.document))
			var validation *format.ValidationError
			if !errors.As(err, &validation) || !slices.Contains(validation.Codes(), tt.wantCode) {
				t.Fatalf("error %v, want code %s", err, tt.wantCode)
			}
		})
	}

	t.Run("a document that is not JSON is refused"+verifiesPending, func(t *testing.T) {
		if _, err := format.DecodePending([]byte(`{`)); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestIsCriterion(t *testing.T) {
	cases := map[string]bool{
		"PARK-1~1": true, "A~12": true, "PARK-1": false, "~1": false, "A~": false,
		"A~1~2": false, "A~-1": false, "A~x": false, "": false, "A~1 ": false,
	}
	for name, want := range cases {
		t.Run(name+verifiesPending, func(t *testing.T) {
			if got := format.IsCriterion(name); got != want {
				t.Fatalf("IsCriterion(%q) = %v", name, got)
			}
		})
	}
}
