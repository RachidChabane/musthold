package hostprototest_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func newReferenceHost(observed hostproto.Observed, policy hostproto.Policy) hostprototest.Host {
	return hostprototest.NewReferenceHost(observed, policy)
}

func TestPersonas(t *testing.T) {
	t.Run("the fourteen personas of ADR 0005 are all defined [verifies SP-140~2]", func(t *testing.T) {
		names := hostprototest.PersonaNames()
		if len(names) != 14 || len(slices.Compact(slices.Sorted(slices.Values(names)))) != 14 {
			t.Fatalf("names = %v", names)
		}
		for _, name := range names {
			persona, err := hostprototest.NewPersona(name)
			if err != nil || persona.Name() != name {
				t.Errorf("%s: %v", name, err)
			}
		}
		if _, err := hostprototest.NewPersona("saboteur"); !errors.Is(err, hostprototest.ErrUnknownPersona) {
			t.Errorf("unknown persona: %v", err)
		}
	})

	for _, name := range hostprototest.PersonaNames() {
		t.Run(name+" receives the asserted verdict from the reference host [verifies SP-140~2]", func(t *testing.T) {
			persona, err := hostprototest.NewPersona(name)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := hostprototest.Drive(persona, newReferenceHost)
			if err != nil {
				t.Fatal(err)
			}
			if err := hostprototest.Check(persona, outcome); err != nil {
				t.Fatalf("%v\noutcome: %+v", err, outcome)
			}
		})
	}

	for _, name := range hostprototest.CopilotPersonaNames() {
		t.Run(name+" receives the asserted verdict from the reference host on the session log [verifies SP-1871~1]", func(t *testing.T) {
			persona, err := hostprototest.NewPersona(name)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := hostprototest.Drive(persona, newReferenceHost)
			if err != nil {
				t.Fatal(err)
			}
			if err := hostprototest.Check(persona, outcome); err != nil {
				t.Fatalf("%v\noutcome: %+v", err, outcome)
			}
		})
	}

	t.Run("a hookless executor is recorded as restricted when attestation is waived [verifies SP-140~2]", func(t *testing.T) {
		persona := hostprototest.Hookless{OptOut: true}
		outcome, err := hostprototest.Drive(persona, newReferenceHost)
		if err != nil {
			t.Fatal(err)
		}
		if err := hostprototest.Check(persona, outcome); err != nil {
			t.Fatalf("%v\noutcome: %+v", err, outcome)
		}
	})

	t.Run("a verdict that does not match the outcome is reported [verifies SP-140~2]", func(t *testing.T) {
		honest, _ := hostprototest.NewPersona(hostprototest.PersonaHonest)
		forger, _ := hostprototest.NewPersona(hostprototest.PersonaForger)
		outcome, err := hostprototest.Drive(forger, newReferenceHost)
		if err != nil {
			t.Fatal(err)
		}
		if err := hostprototest.Check(honest, outcome); err == nil {
			t.Fatal("the forger's outcome satisfied the honest verdict")
		}
	})
}
