package prompt_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
)

const (
	verifiesViewNote = " [verifies SP-2272~1]"
	omittedNote      = "3 more units of this code are not shown: the code above is cut to a size bound."
)

func TestOmittedUnitsNote(t *testing.T) {
	slice := []prompt.SliceEntry{{File: "invoice/totals.py", Qualname: "discount", Source: "def discount(total, percent):\n    return total\n"}}
	t.Run("a generation request whose view the bound cut says how many units it leaves out"+verifiesViewNote, func(t *testing.T) {
		built := prompt.BuildGeneration(prompt.GenerationInput{Statement: "S", Slice: slice, Omitted: 3})
		if !strings.Contains(built.Prompt.User, omittedNote) {
			t.Fatalf("user prompt:\n%s", built.Prompt.User)
		}
	})
	t.Run("a generation request whose view is whole says nothing of a bound, and is the request it always was"+verifiesViewNote, func(t *testing.T) {
		built := prompt.BuildGeneration(prompt.GenerationInput{Statement: "S", Slice: slice})
		if strings.Contains(built.Prompt.User, "size bound") {
			t.Fatalf("user prompt:\n%s", built.Prompt.User)
		}
		cut := prompt.BuildGeneration(prompt.GenerationInput{Statement: "S", Slice: slice, Omitted: 3})
		if strings.Replace(cut.Prompt.User, omittedNote+"\n\n", "", 1) != built.Prompt.User {
			t.Fatalf("whole:\n%s\ncut:\n%s", built.Prompt.User, cut.Prompt.User)
		}
	})
	t.Run("a labels request whose view the bound cut says how many functions it leaves out, after the functions"+verifiesViewNote, func(t *testing.T) {
		input := prompt.LabelsInput{
			Promises:  []prompt.LabelPromise{{ID: prompt.PromiseID(0), Quote: "Q"}},
			Functions: []prompt.LabelFunction{{ID: prompt.FunctionID(0), Path: "invoice/totals.py", Signature: "def discount(total, percent):", Code: "def discount(total, percent):\n    return total\n"}},
			Omitted:   3,
		}
		user := prompt.BuildLabels(input).Prompt.User
		if !strings.Contains(user, omittedNote) || strings.Index(user, omittedNote) < strings.Index(user, "Function f1") {
			t.Fatalf("user prompt:\n%s", user)
		}
		input.Omitted = 0
		if strings.Contains(prompt.BuildLabels(input).Prompt.User, "size bound") {
			t.Fatal("a whole labels view mentions a bound")
		}
	})
}
