package format

import "slices"

// Channel tells how a response reached the module that consumed it.
type Channel string

// Channels of ADR 0005.
const (
	// ChannelHostSubagent is an isolated, blind subagent of the host harness.
	ChannelHostSubagent Channel = "host-subagent"
	// ChannelHostSampled is a response served through MCP sampling.
	ChannelHostSampled Channel = "host-sampled"
	// ChannelHostInline is an answer given by the main agent of the host.
	ChannelHostInline Channel = "host-inline"
	// ChannelSystemOneRelay is an answer served by a System One endpoint through
	// the relay.
	ChannelSystemOneRelay Channel = "systemone-relay"
)

// Isolation is the level of isolation an executor ran with. The levels are
// listed from the strongest to the weakest.
type Isolation string

// Isolation levels of ADR 0005.
const (
	IsolationRestrictedAttested   Isolation = "restricted-attested"
	IsolationRestricted           Isolation = "restricted"
	IsolationRestrictedUnverified Isolation = "restricted-unverified"
	IsolationUnrestricted         Isolation = "unrestricted"
	IsolationInline               Isolation = "inline"
)

// ModelSource tells how the model of an executor was learned.
type ModelSource string

// Model sources.
const (
	// ModelSourceResolved is a model reported by the harness after the run.
	ModelSourceResolved ModelSource = "resolved"
	// ModelSourceDeclared is a model the executor or its configuration declared.
	ModelSourceDeclared ModelSource = "declared"
	// ModelSourceUnknown is a model nobody reported.
	ModelSourceUnknown ModelSource = "unknown"
)

var (
	channels     = []Channel{ChannelHostSubagent, ChannelHostSampled, ChannelHostInline, ChannelSystemOneRelay}
	isolations   = []Isolation{IsolationRestrictedAttested, IsolationRestricted, IsolationRestrictedUnverified, IsolationUnrestricted, IsolationInline}
	modelSources = []ModelSource{ModelSourceResolved, ModelSourceDeclared, ModelSourceUnknown}
)

// Channels lists the closed set of channels.
func Channels() []Channel { return slices.Clone(channels) }

// Isolations lists the closed set of isolation levels, strongest first.
func Isolations() []Isolation { return slices.Clone(isolations) }

// ModelSources lists the closed set of model sources.
func ModelSources() []ModelSource { return slices.Clone(modelSources) }

// Strength ranks the level: the larger, the stronger. An unknown level ranks 0.
func (i Isolation) Strength() int {
	index := slices.Index(isolations, i)
	if index < 0 {
		return 0
	}
	return len(isolations) - index
}

// AtLeast reports whether the level is as strong as the floor. An unknown level
// is never at least any floor.
func (i Isolation) AtLeast(floor Isolation) bool {
	return i.Strength() > 0 && i.Strength() >= floor.Strength()
}

// Harness identifies the host harness that ran an executor.
type Harness struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Attestation is the provenance evidence of an executor: the agent that ran, the
// digest of the capability it presented and the digest of its transcript. Only a
// `restricted-attested` executor must fill it.
type Attestation struct {
	AgentID          string `json:"agentId"`
	CapDigest        string `json:"capDigest"`
	TranscriptDigest string `json:"transcriptDigest"`
}

// ExecutorModel names the model of an executor and how it was learned.
type ExecutorModel struct {
	Name   string      `json:"name"`
	Source ModelSource `json:"source"`
}

// ExecutorProfile describes who answered a request: the part of an executor that
// a response carries.
type ExecutorProfile struct {
	Channel     Channel       `json:"channel"`
	Harness     Harness       `json:"harness"`
	Isolation   Isolation     `json:"isolation"`
	Attestation Attestation   `json:"attestation"`
	Model       ExecutorModel `json:"model"`
}

// Executor is the profile of the executor that produced a judged or generated
// result, with the digest of the prompt it answered and whether it was blind.
type Executor struct {
	ExecutorProfile
	PromptDigest string `json:"promptDigest"`
	Blind        bool   `json:"blind"`
}

// Validate reports every violation of the profile rules as a *ValidationError.
func (e ExecutorProfile) Validate() error {
	var sink collector
	e.check(&sink, "executor")
	return sink.err()
}

func (e ExecutorProfile) check(sink *collector, path string) {
	checkMember(sink, path+".channel", e.Channel, channels)
	checkNotEmpty(sink, path+".harness.name", e.Harness.Name)
	checkMember(sink, path+".isolation", e.Isolation, isolations)
	checkMember(sink, path+".model.source", e.Model.Source, modelSources)
	if e.Model.Source != ModelSourceUnknown {
		checkNotEmpty(sink, path+".model.name", e.Model.Name)
	}
	e.Attestation.check(sink, path+".attestation", e.Isolation == IsolationRestrictedAttested)
}

func (a Attestation) check(sink *collector, path string, required bool) {
	if required {
		checkNotEmpty(sink, path+".agentId", a.AgentID)
		checkDigest(sink, path+".capDigest", a.CapDigest)
		checkDigest(sink, path+".transcriptDigest", a.TranscriptDigest)
		return
	}
	if a.CapDigest != "" {
		checkDigest(sink, path+".capDigest", a.CapDigest)
	}
	if a.TranscriptDigest != "" {
		checkDigest(sink, path+".transcriptDigest", a.TranscriptDigest)
	}
}

func (e Executor) check(sink *collector, path string) {
	e.ExecutorProfile.check(sink, path)
	checkDigest(sink, path+".promptDigest", e.PromptDigest)
}
