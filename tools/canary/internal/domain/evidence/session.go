package evidence

import "time"

// Reasons a session was ended by the runner and not by the harness.
const (
	KilledNone    = ""
	KilledTimeout = "timeout"
	KilledTurnCap = "turn-cap"
	// KilledInterrupted is the reason of a session ended by SIGINT or SIGTERM, which
	// is how the memory guard stops a batch.
	KilledInterrupted = "interrupted"
)

// Session describes how one session was set up and how it ended. It is written
// by the runner and holds no verdict.
type Session struct {
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Ticket      string `json:"ticket"`
	Role        string `json:"role"`
	ProbeTicket string `json:"probeTicket"`
	// Versions maps each CLI the run used to the version line it printed.
	Versions map[string]string `json:"versions"`
	// Argv is the command line of the harness CLI, the prompt included.
	Argv     []string `json:"argv"`
	ExitCode int      `json:"exitCode"`
	// Killed is the reason the runner ended the session, empty when it ended itself.
	Killed         string    `json:"killed"`
	EventCap       int       `json:"eventCap"`
	TimeoutSeconds int       `json:"timeoutSeconds"`
	EditFile       string    `json:"editFile"`
	EditFrom       string    `json:"editFrom"`
	EditTo         string    `json:"editTo"`
	StartedAt      time.Time `json:"startedAt"`
	EndedAt        time.Time `json:"endedAt"`
}

// NoCall is the outcome of the probes that run before the session and make no
// model call.
type NoCall struct {
	// CapProbe is what the executor server answered to a fetch with no capability.
	CapProbe CapProbe `json:"capProbe"`
	// PluginValidation is one outcome per plugin directory the harness CLI validated.
	PluginValidation []PluginValidation `json:"pluginValidation"`
}

// CapProbe records a fetch_request sent to the executor server over standard
// input and output without the capability a hook would have added.
type CapProbe struct {
	Attempted bool `json:"attempted"`
	// Responded is false when the server never answered.
	Responded bool `json:"responded"`
	// Refused is true when the answer is an error.
	Refused bool `json:"refused"`
	// ServedTicket is true when the answer carries the ticket's nonce and schema.
	ServedTicket bool   `json:"servedTicket"`
	Detail       string `json:"detail"`
}

// PluginValidation is the result of the harness CLI's own validation of a plugin.
type PluginValidation struct {
	Dir      string `json:"dir"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

// SandboxProbe records whether the file a read-only executor was asked to create
// exists after the session.
type SandboxProbe struct {
	File   string `json:"file"`
	Exists bool   `json:"exists"`
}
