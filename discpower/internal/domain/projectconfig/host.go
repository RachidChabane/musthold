package projectconfig

// HostSettings are the settings of the host harness the module hands its model
// work to.
type HostSettings struct {
	// Sampling is `host.sampling: on`: the executor server offers `drain` when
	// the client declares sampling.
	Sampling bool
	// RequireAttestation is `host.require_attestation`; nil keeps the default.
	RequireAttestation *bool
	// ClassifyWitnessDir is `host.classify.witness_dir`: the directory of the
	// user's own raw API log, empty when the check is not opted into.
	ClassifyWitnessDir string
}

// DefaultRequireAttestation is the default of `host.require_attestation`: an
// executor answer without a capability is refused.
const DefaultRequireAttestation = true

// AttestationRequired reports whether an answer needs a capability issued by the
// harness's hooks.
func (h HostSettings) AttestationRequired() bool {
	if h.RequireAttestation == nil {
		return DefaultRequireAttestation
	}
	return *h.RequireAttestation
}

// IsDefault reports whether the settings equal the defaults, so that an encoder
// can leave them out.
func (h HostSettings) IsDefault() bool {
	return !h.Sampling && h.RequireAttestation == nil && h.ClassifyWitnessDir == ""
}
