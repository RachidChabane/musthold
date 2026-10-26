package differential

import "github.com/RachidChabane/musthold/discpower/internal/domain/power"

// CompiledForms are the canonical compiled forms of the units of one program, by
// site `file::qualname`: what an ecosystem's compiler makes of each unit, with
// every source position, file name and build identifier left out, digested.
type CompiledForms map[string]string

// CompareForms decides compiled-form equivalence (E0) over the sites a mutant
// edits: identical when every edited site compiles to the reference's form,
// differs when one does not, and unknown when a site has no form on either
// side or the mutant edits nothing.
func CompareForms(sites []string, reference, mutant CompiledForms) power.Identity {
	if len(sites) == 0 {
		return power.IdentityUnknown
	}
	identical := true
	for _, site := range sites {
		before, known := reference[site]
		after, compiled := mutant[site]
		if !known || !compiled || before == "" || after == "" {
			return power.IdentityUnknown
		}
		identical = identical && before == after
	}
	if identical {
		return power.IdentityIdentical
	}
	return power.IdentityDiffers
}
